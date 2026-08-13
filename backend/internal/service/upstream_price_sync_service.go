package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// channelApplier 解耦对 ChannelService 的依赖(便于测试)。
// *ChannelService 通过 ApplyUpstreamPricingEntry + RemoveUpstreamPricingEntry + GetByID 隐式实现该接口;
// 测试可用最小 fake 替换,无需构造完整 ChannelService。
type channelApplier interface {
	ApplyUpstreamPricingEntry(ctx context.Context, channelID int64, platform string, models []string, price ConvertedPrice) (*ChannelModelPricing, error)
	RemoveUpstreamPricingEntry(ctx context.Context, channelID int64, platform string, model string) error
	GetByID(ctx context.Context, id int64) (*Channel, error)
}

type upstreamProxyResolver interface {
	GetByID(ctx context.Context, id int64) (*Proxy, error)
}

// UpstreamPriceSyncService 上游 new-api 定价同步的编排核心:
// 拉取(Task 6)→ 还原/推断/diff(Task 4/5)→ 落审批单(Task 7 repo)→ 应用(Task 8)。
type UpstreamPriceSyncService struct {
	repo                 UpstreamPriceSyncRepository
	client               *UpstreamPricingClient
	channelService       channelApplier
	proxyResolver        upstreamProxyResolver
	balanceNotifyService *BalanceNotifyService
	authCacheInvalidator APIKeyAuthCacheInvalidator
	cfg                  *config.Config
}

// NewUpstreamPriceSyncService 构造同步服务。chSvc 通常是 *ChannelService
// (满足 channelApplier);声明为接口类型以便测试注入 fake,生产侧传 *ChannelService 即可。
func NewUpstreamPriceSyncService(repo UpstreamPriceSyncRepository, client *UpstreamPricingClient, chSvc channelApplier, proxyResolver upstreamProxyResolver, balanceNotifyService *BalanceNotifyService, authCacheInvalidator APIKeyAuthCacheInvalidator, cfg *config.Config) *UpstreamPriceSyncService {
	return &UpstreamPriceSyncService{
		repo:                 repo,
		client:               client,
		channelService:       chSvc,
		proxyResolver:        proxyResolver,
		balanceNotifyService: balanceNotifyService,
		authCacheInvalidator: authCacheInvalidator,
		cfg:                  cfg,
	}
}

// SyncOutcome SyncNow 返回的同步结果,供前端区分"建立基线/无变化/检测到变化"三种状态。
type SyncOutcome struct {
	RequestID              int64    `json:"request_id"`
	GroupRatioEnabled      bool     `json:"group_ratio_enabled"`
	GroupRatioCurrent      *float64 `json:"group_ratio_current"`
	GroupRatioBaseline     *float64 `json:"group_ratio_baseline"`
	GroupRatioEstablished  bool     `json:"group_ratio_established"`
	GroupRatioItemsCreated int      `json:"group_ratio_items_created"`
}

// SyncNow 立即拉取上游定价、diff 出变更草稿并落成审批单。
// 返回 SyncOutcome:RequestID>0 表示生成了审批单;GroupRatio* 字段反映倍率同步状态。
func (s *UpstreamPriceSyncService) SyncNow(ctx context.Context, configID, createdBy int64) (SyncOutcome, error) {
	var outcome SyncOutcome
	err := s.repo.WithSourceSyncLock(ctx, configID, func(ctx context.Context) error {
		cfgRec, err := s.repo.GetConfig(ctx, configID)
		if err != nil {
			return fmt.Errorf("get source config: %w", err)
		}
		if !cfgRec.Enabled {
			return infraerrors.BadRequest("upstream_source_disabled", "upstream source is disabled")
		}
		baseURL, err := s.validateBaseURL(cfgRec.BaseURL)
		if err != nil {
			return fmt.Errorf("invalid base url: %w", err)
		}
		proxyURL, err := s.resolveProxyURL(ctx, cfgRec.ProxyID)
		if err != nil {
			return fmt.Errorf("resolve proxy: %w", err)
		}
		snap, err := s.client.FetchPricing(ctx, baseURL, cfgRec.PricingSource, proxyURL, cfgRec.DashboardToken, cfgRec.APIKey, cfgRec.DashboardAuthMode, cfgRec.DashboardUserID)
		if err != nil {
			_ = s.repo.UpdateConfigSyncState(ctx, configID, time.Now(), "", err.Error())
			return fmt.Errorf("fetch upstream: %w", err)
		}
		observedAt := time.Now()
		ch, err := s.channelService.GetByID(ctx, cfgRec.TargetChannelID)
		if err != nil {
			return fmt.Errorf("load target channel: %w", err)
		}

		items := make([]PriceChangeItem, 0)
		syncModelPrice := cfgRec.SyncModelPrice || (!cfgRec.SyncModelPrice && !cfgRec.SyncGroupRatio)
		if syncModelPrice {
			upstream := make(map[string]ConvertedPrice, len(snap.Models))
			platforms := make(map[string]string, len(snap.Models))
			rawByName := make(map[string]map[string]any, len(snap.Models))
			for _, model := range snap.Models {
				if !modelGroupEnabled(model.EnableGroups, cfgRec.TargetUpstreamGroup) {
					continue
				}
				upstream[model.ModelName] = ConvertPricing(model, cfgRec.BasePricePer1k)
				platforms[model.ModelName] = InferPlatform(model.ModelName)
				rawByName[model.ModelName] = map[string]any{
					"model_ratio": model.ModelRatio, "completion_ratio": model.CompletionRatio, "quota_type": model.QuotaType,
				}
			}
			drafts := DiffPricing(upstream, platforms, ch.ModelPricing, cfgRec.TargetChannelID)
			hasModelChange := false
			for _, draft := range drafts {
				if draft.Kind != ItemKindModelUnchanged {
					hasModelChange = true
					break
				}
			}
			if hasModelChange {
				for _, draft := range drafts {
					status := "pending"
					if draft.Kind == ItemKindModelUnchanged {
						status = "no_change"
					}
					items = append(items, PriceChangeItem{
						Kind: draft.Kind, Platform: draft.Platform, ModelName: draft.ModelName,
						TargetChannelID: cfgRec.TargetChannelID, UpstreamRaw: rawByName[draft.ModelName],
						UpstreamConverted: draft.Upstream, LocalCurrent: draft.Local, ApplyValue: draft.Upstream, Status: status,
					})
				}
			}
		}

		persist := SyncPersistInput{ConfigID: configID, ObservedAt: observedAt, PricingVersion: snap.Version}
		if cfgRec.SyncGroupRatio {
			key := strings.TrimSpace(cfgRec.TargetUpstreamGroup)
			newRatio, ok := snap.GroupRatio[key]
			if !ok {
				return infraerrors.BadRequest("UPSTREAM_GROUP_RATIO_MISSING", "selected upstream group ratio is missing")
			}
			if !validPositiveFinite(newRatio) {
				return infraerrors.BadRequest("INVALID_UPSTREAM_GROUP_RATIO", "selected upstream group ratio must be finite and > 0")
			}
			newRatio = math.Round(newRatio*1e8) / 1e8
			persist.BaselineKey = key
			persist.BaselineValue = &newRatio
			persist.AdvanceBaseline = true
			outcome.GroupRatioEnabled = true
			outcome.GroupRatioCurrent = &newRatio
			outcome.GroupRatioBaseline = &newRatio
			baselineMissing := cfgRec.GroupRatioBaselineValue == nil || cfgRec.GroupRatioBaselineKey != key || !validPositiveFinite(*cfgRec.GroupRatioBaselineValue)
			outcome.GroupRatioEstablished = baselineMissing
			if !baselineMissing && math.Abs(*cfgRec.GroupRatioBaselineValue-newRatio) > 1e-8 {
				pending, err := s.repo.HasPendingGroupRateItems(ctx, configID)
				if err != nil {
					return err
				}
				if pending {
					return infraerrors.Conflict("PENDING_GROUP_APPROVAL", ErrPendingGroupApproval.Error())
				}
				targets, err := s.repo.ListGroupRateTargets(ctx, configID, cfgRec.TargetChannelID)
				if err != nil {
					return err
				}
				name := snap.UsableGroup[key]
				if name == "" {
					name = key
				}
				changes, err := BuildGroupRateChanges(targets, key, name, *cfgRec.GroupRatioBaselineValue, newRatio)
				if err != nil {
					return err
				}
				outcome.GroupRatioItemsCreated = len(changes)
				for i := range changes {
					change := changes[i]
					groupID := change.LocalGroupID
					items = append(items, PriceChangeItem{
						Kind: ItemKindGroupRatio, TargetChannelID: cfgRec.TargetChannelID,
						TargetGroupID: &groupID, GroupRateChange: &change, Status: "pending",
					})
				}
			}
		}

		if len(items) > 0 {
			req := &PriceChangeRequest{
				SourceConfigID: configID, TriggerType: "manual",
				UpstreamPricingVersion: snap.Version, CreatedBy: createdBy,
			}
			persist.Request = req
			persist.Items = items
		}
		if err := s.repo.PersistSyncResult(ctx, persist); err != nil {
			return fmt.Errorf("persist sync result: %w", err)
		}
		if persist.Request != nil {
			outcome.RequestID = persist.Request.ID
		}
		return nil
	})
	if errors.Is(err, ErrSourceSyncBusy) {
		return SyncOutcome{}, infraerrors.Conflict("SOURCE_SYNC_BUSY", err.Error())
	}
	return outcome, err
}

// ReviewItem 对单条审批条目执行 apply / reject / ignore。
//   - apply: 写入目标渠道定价(Task 8 ApplyUpstreamPricingEntry)+ item.status=applied;
//     应用失败则 item.status=failed 并返回错误(失败原因记录到 review_note)。
//   - reject / ignore: 仅更新 item 状态,不触碰渠道。
func (s *UpstreamPriceSyncService) ReviewItem(ctx context.Context, requestID, itemID int64, action ReviewAction, applyValue *ConvertedPrice, applyRate *float64, platform string, reviewerID int64, note string) error {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("get item: %w", err)
	}
	if it.RequestID != requestID {
		return infraerrors.Conflict("ITEM_REQUEST_MISMATCH", ErrItemRequestMismatch.Error())
	}
	if it.Status != "pending" {
		return infraerrors.Conflict("ITEM_NOT_PENDING", ErrItemNotPending.Error())
	}
	if action == ReviewReject || action == ReviewIgnore {
		status := "rejected"
		if action == ReviewIgnore {
			status = "ignored"
		}
		if err := s.repo.FinalizeItemCAS(ctx, requestID, itemID, status, reviewerID, note, nil); err != nil {
			return mapReviewConflict(err)
		}
		return nil
	}
	if action != ReviewApply {
		return infraerrors.BadRequest("UNKNOWN_REVIEW_ACTION", "unknown review action")
	}
	if it.Kind == ItemKindGroupRatio {
		if applyRate == nil || !validPositiveFinite(*applyRate) {
			return infraerrors.BadRequest("INVALID_APPLY_RATE", "apply_rate must be finite and > 0")
		}
		groupID, actualRate, err := s.repo.ApplyGroupRateItem(ctx, GroupRateApplyInput{
			RequestID: requestID, ItemID: itemID, ReviewerID: reviewerID, Note: note, ApplyRate: *applyRate,
		})
		if err != nil {
			return mapReviewConflict(err)
		}
		if s.authCacheInvalidator != nil {
			s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, groupID)
		}
		it.ApplyRate = &actualRate
		return nil
	}
	if it.Kind == ItemKindModelRemoved {
		// 移除模型:把该模型从目标渠道定价中删除,不涉及 apply_value。
		if err := s.channelService.RemoveUpstreamPricingEntry(ctx, it.TargetChannelID, it.Platform, it.ModelName); err != nil {
			if ferr := s.repo.FinalizeItemCAS(ctx, requestID, itemID, "failed", reviewerID, err.Error(), nil); ferr != nil {
				slog.WarnContext(ctx, "finalize failed item after remove error", "item_id", itemID, "err", ferr)
			}
			return fmt.Errorf("remove: %w", err)
		}
		if err := s.repo.FinalizeItemCAS(ctx, requestID, itemID, "applied", reviewerID, note, nil); err != nil {
			return mapReviewConflict(err)
		}
		return nil
	}
	val := applyValue
	if val == nil {
		val = it.ApplyValue
	}
	if val == nil {
		val = it.UpstreamConverted
	}
	if val == nil {
		return infraerrors.BadRequest("NO_APPLY_VALUE", "no apply value for item")
	}
	// 平台补充/修正:仅当显式传入且与已存值不同时更新。上游模型名推断不出平台
	// (InferPlatform 返回空串)时,管理员在审批时通过下拉框人工指定,随 apply 一起落库。
	if platform != "" {
		if !validSyncPlatform(platform) {
			return infraerrors.BadRequest("INVALID_PLATFORM", "invalid platform")
		}
		if platform != it.Platform {
			if err := s.repo.UpdateItemPlatform(ctx, requestID, itemID, platform); err != nil {
				return mapReviewConflict(err)
			}
			it.Platform = platform
		}
	}
	if _, err := s.channelService.ApplyUpstreamPricingEntry(ctx, it.TargetChannelID, it.Platform, []string{it.ModelName}, *val); err != nil {
		if ferr := s.repo.FinalizeItemCAS(ctx, requestID, itemID, "failed", reviewerID, err.Error(), val); ferr != nil {
			slog.WarnContext(ctx, "finalize failed item after apply error", "item_id", itemID, "err", ferr)
		}
		return fmt.Errorf("apply: %w", err)
	}
	if err := s.repo.FinalizeItemCAS(ctx, requestID, itemID, "applied", reviewerID, note, val); err != nil {
		return mapReviewConflict(err)
	}
	return nil
}

func mapReviewConflict(err error) error {
	switch {
	case errors.Is(err, ErrItemRequestMismatch):
		return infraerrors.Conflict("ITEM_REQUEST_MISMATCH", err.Error())
	case errors.Is(err, ErrItemNotPending):
		return infraerrors.Conflict("ITEM_NOT_PENDING", err.Error())
	case errors.Is(err, ErrGroupRateDrift):
		return infraerrors.Conflict("GROUP_RATE_DRIFT", err.Error())
	case errors.Is(err, ErrGroupRateTargetUnavailable):
		return infraerrors.Conflict("GROUP_RATE_TARGET_UNAVAILABLE", err.Error())
	default:
		return err
	}
}

// finalizeItem 落库 item 终态后,顺带重算所属审批单的状态与汇总。
// recompute 为 best-effort:其失败只记日志,不覆盖 item 已成功落库的事实。
func (s *UpstreamPriceSyncService) finalizeItem(ctx context.Context, itemID, requestID int64, status string, reviewerID int64, note string, appliedAt *time.Time) error {
	if err := s.repo.UpdateItemStatus(ctx, itemID, status, reviewerID, note, appliedAt); err != nil {
		return err
	}
	if err := s.recomputeRequestStatus(ctx, requestID); err != nil {
		slog.WarnContext(ctx, "recompute request status after review",
			"request_id", requestID, "item_id", itemID, "err", err)
	}
	return nil
}

// recomputeRequestStatus 在单条 item 落库后,根据该审批单全部 item 的状态重算其状态与汇总:
//   - 无 pending 条目(全部到达终态) → closed,自动闭环,免去手动「关闭审批单」;
//   - 仍有 pending 且已有非 pending 条目 → partially_applied(打通此前从未写入的中间态);
//   - 全部 pending(防御性,review 后理论不发生) → open。
//
// summary 同步刷新为各 item status 的计数,供列表「汇总」列展示操作结果分布。
func (s *UpstreamPriceSyncService) recomputeRequestStatus(ctx context.Context, requestID int64) error {
	items, err := s.repo.ListItems(ctx, requestID)
	if err != nil {
		return fmt.Errorf("list items for recompute: %w", err)
	}
	summary := make(map[string]int, len(items))
	pending := 0
	for _, it := range items {
		summary[it.Status]++
		if it.Status == "pending" {
			pending++
		}
	}
	var newStatus string
	switch {
	case pending == 0:
		newStatus = "closed" // 全部终态 → 自动闭环(空单亦归此分支,防御性)
	case pending < len(items):
		newStatus = "partially_applied" // 有 pending 且有已处理 → 部分应用
	default:
		newStatus = "open" // 全 pending(防御性)
	}
	return s.repo.UpdateRequestStatus(ctx, requestID, newStatus, summary)
}

// --- 上游源配置 CRUD(薄封装,落库加密由 repository 层负责)---

// ListConfigs 列出全部上游源配置。
func (s *UpstreamPriceSyncService) ListConfigs(ctx context.Context) ([]UpstreamSourceConfig, error) {
	configs, err := s.repo.ListConfigs(ctx)
	if err != nil {
		return nil, err
	}
	for i := range configs {
		populateBalanceStatus(&configs[i])
	}
	return configs, nil
}

// GetConfig 按 id 取单个上游源配置。
func (s *UpstreamPriceSyncService) GetConfig(ctx context.Context, id int64) (*UpstreamSourceConfig, error) {
	cfgRec, err := s.repo.GetConfig(ctx, id)
	if err != nil {
		return nil, err
	}
	populateBalanceStatus(cfgRec)
	return cfgRec, nil
}

// CreateConfig 新建上游源配置(APIKey/DashboardToken 由 repo 加密落库)。
func (s *UpstreamPriceSyncService) CreateConfig(ctx context.Context, c *UpstreamSourceConfig) error {
	if err := s.validateConfig(ctx, c); err != nil {
		return err
	}
	if err := s.repo.CreateConfig(ctx, c); err != nil {
		return err
	}
	populateBalanceStatus(c)
	return nil
}

// UpdateConfig 更新上游源配置。
func (s *UpstreamPriceSyncService) UpdateConfig(ctx context.Context, c *UpstreamSourceConfig) error {
	if err := s.validateConfig(ctx, c); err != nil {
		return err
	}
	err := s.repo.WithSourceSyncLock(ctx, c.ID, func(ctx context.Context) error {
		existing, err := s.repo.GetConfig(ctx, c.ID)
		if err != nil {
			return err
		}
		c.ResetBalanceSnapshot = strings.TrimSpace(existing.BaseURL) != strings.TrimSpace(c.BaseURL) ||
			strings.TrimSpace(existing.DashboardToken) != strings.TrimSpace(c.DashboardToken)
		c.ResetGroupRatioBaseline = strings.TrimSpace(existing.BaseURL) != strings.TrimSpace(c.BaseURL) ||
			existing.TargetChannelID != c.TargetChannelID ||
			strings.TrimSpace(existing.TargetUpstreamGroup) != strings.TrimSpace(c.TargetUpstreamGroup)
		c.ExpirePendingApprovals = c.ResetGroupRatioBaseline
		c.ExpirePendingGroupApprovals = !c.ResetGroupRatioBaseline && !sameUpstreamInt64Set(existing.ExcludedGroupIDs, c.ExcludedGroupIDs)
		if err := s.repo.UpdateConfig(ctx, c); err != nil {
			return err
		}
		if c.ResetBalanceSnapshot {
			c.LastBalanceQuota = nil
			c.LastUsedQuota = nil
			c.LastBalanceUSD = nil
			c.LastBalanceAt = nil
			c.LastBalanceCheckedAt = nil
			c.LastBalanceError = ""
		} else {
			c.LastBalanceQuota = existing.LastBalanceQuota
			c.LastUsedQuota = existing.LastUsedQuota
			c.LastBalanceUSD = existing.LastBalanceUSD
			c.LastBalanceAt = existing.LastBalanceAt
			c.LastBalanceCheckedAt = existing.LastBalanceCheckedAt
			c.LastBalanceError = existing.LastBalanceError
		}
		if !c.ResetGroupRatioBaseline {
			c.GroupRatioBaselineKey = existing.GroupRatioBaselineKey
			c.GroupRatioBaselineValue = existing.GroupRatioBaselineValue
			c.GroupRatioBaselineObserved = existing.GroupRatioBaselineObserved
		}
		populateBalanceStatus(c)
		return nil
	})
	if errors.Is(err, ErrSourceSyncBusy) {
		return infraerrors.Conflict("SOURCE_SYNC_BUSY", err.Error())
	}
	return err
}

func (s *UpstreamPriceSyncService) RefreshBalance(ctx context.Context, id int64) (*UpstreamSourceConfig, error) {
	cfgRec, err := s.repo.GetConfig(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get source config: %w", err)
	}
	if !cfgRec.Enabled {
		return nil, infraerrors.BadRequest("upstream_source_disabled", "upstream source is disabled")
	}
	if strings.TrimSpace(cfgRec.DashboardToken) == "" {
		return nil, infraerrors.BadRequest("dashboard_token_required", "dashboard_token is required to refresh balance")
	}
	baseURL, err := s.validateBaseURL(cfgRec.BaseURL)
	if err != nil {
		return s.balanceRefreshError(ctx, cfgRec, infraerrors.BadRequest("invalid_base_url", "invalid base_url"))
	}
	proxyURL, err := s.resolveProxyURL(ctx, cfgRec.ProxyID)
	if err != nil {
		return s.balanceRefreshError(ctx, cfgRec, fmt.Errorf("resolve proxy: %w", err))
	}
	snapshot, err := s.client.FetchBalance(ctx, baseURL, cfgRec.DashboardToken, cfgRec.DashboardAuthMode, cfgRec.DashboardUserID, proxyURL)
	if err != nil {
		return s.balanceRefreshError(ctx, cfgRec, fmt.Errorf("fetch upstream balance: %w", err))
	}
	if err := s.repo.UpdateConfigBalanceSuccess(ctx, id, *snapshot); err != nil {
		return nil, err
	}
	cfgRec.LastBalanceQuota = upstreamInt64Ptr(snapshot.Quota)
	cfgRec.LastUsedQuota = upstreamInt64Ptr(snapshot.UsedQuota)
	cfgRec.LastBalanceUSD = upstreamFloat64Ptr(snapshot.BalanceUSD)
	cfgRec.LastBalanceAt = &snapshot.FetchedAt
	cfgRec.LastBalanceCheckedAt = &snapshot.FetchedAt
	cfgRec.LastBalanceError = ""
	populateBalanceStatus(cfgRec)
	if cfgRec.BalanceStatus == "low" && s.balanceNotifyService != nil {
		s.balanceNotifyService.NotifyUpstreamBalanceLow(ctx, cfgRec)
	}
	return cfgRec, nil
}

// resolveUpstreamGroups 拉取上游可用分组字典({key: 展示名}),供前端 source 配置下拉。
// 强制走 /api/pricing(/api/ratio_config 不返回分组信息)。
// 数据源:优先 usable_group({key: 展示名},部分 new-api 版本返回);
// 回退 group_ratio 的 keys(key 即分组标识,与每模型 enable_groups 对齐,
// 所有 new-api 版本的 /api/pricing 都返回 group_ratio)。两者合并,保证有数据。
func (s *UpstreamPriceSyncService) resolveUpstreamGroups(ctx context.Context, baseURL string, proxyID *int64, dashboardToken, apiKey, authMode string, userID *int64) (map[string]string, error) {
	baseURL, err := s.validateBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	proxyURL, err := s.resolveProxyURL(ctx, proxyID)
	if err != nil {
		return nil, fmt.Errorf("resolve proxy: %w", err)
	}
	// 诊断:记录传入的鉴权参数是否非空(脱敏,只记存在性)。
	slog.InfoContext(ctx, "resolve upstream groups",
		"base_url", baseURL,
		"proxy_id", proxyID,
		"auth_mode", authMode,
		"has_dashboard_token", strings.TrimSpace(dashboardToken) != "",
		"has_api_key", strings.TrimSpace(apiKey) != "",
		"has_user_id", userID != nil && *userID > 0,
	)
	snap, err := s.client.FetchPricing(ctx, baseURL, PricingSourceAuto, proxyURL, dashboardToken, apiKey, authMode, userID)
	if err != nil {
		return nil, fmt.Errorf("fetch upstream groups: %w", err)
	}
	groups := map[string]string{}
	if snap.UsableGroup != nil {
		for k, v := range snap.UsableGroup {
			groups[k] = v
		}
	}
	if snap.GroupRatio != nil {
		for k := range snap.GroupRatio {
			if _, ok := groups[k]; !ok {
				groups[k] = k // 无展示名时用 key 本身
			}
		}
	}
	return groups, nil
}

// ListUpstreamGroups 按已保存 source 的配置拉取分组(编辑现有 source 用)。
func (s *UpstreamPriceSyncService) ListUpstreamGroups(ctx context.Context, configID int64) (map[string]string, error) {
	cfgRec, err := s.repo.GetConfig(ctx, configID)
	if err != nil {
		return nil, fmt.Errorf("get source config: %w", err)
	}
	if !cfgRec.Enabled {
		return nil, infraerrors.BadRequest("upstream_source_disabled", "upstream source is disabled")
	}
	return s.resolveUpstreamGroups(ctx, cfgRec.BaseURL, cfgRec.ProxyID, cfgRec.DashboardToken, cfgRec.APIKey, cfgRec.DashboardAuthMode, cfgRec.DashboardUserID)
}

// PreviewUpstreamGroups 按 base_url 直接拉取分组(新建 source 尚未保存时用)。
func (s *UpstreamPriceSyncService) PreviewUpstreamGroups(ctx context.Context, baseURL string, proxyID *int64, dashboardToken, apiKey, authMode string, userID *int64) (map[string]string, error) {
	return s.resolveUpstreamGroups(ctx, baseURL, proxyID, dashboardToken, apiKey, authMode, userID)
}

func (s *UpstreamPriceSyncService) balanceRefreshError(ctx context.Context, cfgRec *UpstreamSourceConfig, refreshErr error) (*UpstreamSourceConfig, error) {
	checkedAt := time.Now()
	if err := s.repo.UpdateConfigBalanceError(ctx, cfgRec.ID, checkedAt, refreshErr.Error()); err != nil {
		return nil, fmt.Errorf("persist balance refresh error: %w", err)
	}
	cfgRec.LastBalanceCheckedAt = &checkedAt
	cfgRec.LastBalanceError = refreshErr.Error()
	populateBalanceStatus(cfgRec)
	return cfgRec, refreshErr
}

func (s *UpstreamPriceSyncService) validateConfig(ctx context.Context, c *UpstreamSourceConfig) error {
	if c == nil {
		return infraerrors.BadRequest("invalid_upstream_source", "upstream source is required")
	}
	if c.BasePricePer1k <= 0 || math.IsNaN(c.BasePricePer1k) || math.IsInf(c.BasePricePer1k, 0) {
		return infraerrors.BadRequest("invalid_base_price", "base_price_per_1k must be finite and > 0")
	}
	// 兼容 183 之前未提交开关字段的客户端：两个 false 按历史行为回退模型同步。
	if !c.SyncModelPrice && !c.SyncGroupRatio {
		c.SyncModelPrice = true
	}
	if c.SyncGroupRatio && strings.TrimSpace(c.TargetUpstreamGroup) == "" {
		return infraerrors.BadRequest("target_upstream_group_required", "target_upstream_group is required when group ratio sync is enabled")
	}
	if c.BalanceThresholdUSD != nil {
		threshold := *c.BalanceThresholdUSD
		if threshold < 0 || math.IsNaN(threshold) || math.IsInf(threshold, 0) {
			return infraerrors.BadRequest("invalid_balance_threshold", "balance_threshold_usd must be finite and >= 0")
		}
		if strings.TrimSpace(c.DashboardToken) == "" {
			return infraerrors.BadRequest("dashboard_token_required", "dashboard_token is required when balance threshold is configured")
		}
	}
	// 规范化 Dashboard 鉴权模式；raw_user/bearer_user 必须提供 Dashboard User ID。
	c.DashboardAuthMode = NormalizeDashboardAuthMode(c.DashboardAuthMode)
	if DashboardAuthModeNeedsUserID(c.DashboardAuthMode) && (c.DashboardUserID == nil || *c.DashboardUserID <= 0) {
		return infraerrors.BadRequest("invalid_dashboard_user_id", "dashboard_user_id is required for the selected dashboard auth mode")
	}
	if _, err := s.resolveProxyURL(ctx, c.ProxyID); err != nil {
		return infraerrors.BadRequest("invalid_proxy", err.Error())
	}
	return nil
}

func (s *UpstreamPriceSyncService) resolveProxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil || *proxyID == 0 {
		return "", nil
	}
	if s.proxyResolver == nil {
		return "", fmt.Errorf("proxy resolver is unavailable")
	}
	proxy, err := s.proxyResolver.GetByID(ctx, *proxyID)
	if err != nil {
		return "", err
	}
	if proxy == nil || !proxy.IsActive() || proxy.IsExpired(time.Now()) {
		return "", fmt.Errorf("proxy %d is not active", *proxyID)
	}
	return proxy.URL(), nil
}

func sameUpstreamInt64Set(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[int64]int, len(a))
	for _, value := range a {
		set[value]++
	}
	for _, value := range b {
		if set[value] == 0 {
			return false
		}
		set[value]--
	}
	return true
}

func populateBalanceStatus(c *UpstreamSourceConfig) {
	if c == nil {
		return
	}
	switch {
	case strings.TrimSpace(c.DashboardToken) == "":
		c.BalanceStatus = "not_configured"
	case c.LastBalanceError != "":
		c.BalanceStatus = "error"
	case c.LastBalanceUSD == nil:
		c.BalanceStatus = "unknown"
	case c.BalanceThresholdUSD != nil && *c.LastBalanceUSD <= *c.BalanceThresholdUSD:
		c.BalanceStatus = "low"
	default:
		c.BalanceStatus = "healthy"
	}
}

func upstreamInt64Ptr(v int64) *int64       { return &v }
func upstreamFloat64Ptr(v float64) *float64 { return &v }

// DeleteConfig 按 id 删除上游源配置。
func (s *UpstreamPriceSyncService) DeleteConfig(ctx context.Context, id int64) error {
	err := s.repo.WithSourceSyncLock(ctx, id, func(ctx context.Context) error {
		return s.repo.DeleteConfig(ctx, id)
	})
	if errors.Is(err, ErrSourceSyncBusy) {
		return infraerrors.Conflict("SOURCE_SYNC_BUSY", err.Error())
	}
	return err
}

// ListItems 列审批批次下的条目(薄封装)。
func (s *UpstreamPriceSyncService) ListItems(ctx context.Context, requestID int64) ([]PriceChangeItem, error) {
	return s.repo.ListItems(ctx, requestID)
}

// GetRequest 取审批批次详情(薄封装)。
func (s *UpstreamPriceSyncService) GetRequest(ctx context.Context, id int64) (*PriceChangeRequest, error) {
	return s.repo.GetRequest(ctx, id)
}

// ListRequests 列审批批次(薄封装)。
func (s *UpstreamPriceSyncService) ListRequests(ctx context.Context, f RequestFilter) ([]PriceChangeRequest, int64, error) {
	return s.repo.ListRequests(ctx, f)
}

// CloseRequest 关闭审批批次:仅当无 pending 条目时把 open / partially_applied 单置 closed。
// 仍有 pending 条目或批次状态不可关闭时返回 ErrRequestNotCloseable。
func (s *UpstreamPriceSyncService) CloseRequest(ctx context.Context, requestID int64) error {
	req, err := s.repo.GetRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("get request: %w", err)
	}
	if req.Status != "open" && req.Status != "partially_applied" {
		return ErrRequestNotCloseable
	}
	items, err := s.repo.ListItems(ctx, requestID)
	if err != nil {
		return fmt.Errorf("list items: %w", err)
	}
	for _, it := range items {
		if it.Status == "pending" {
			return ErrRequestNotCloseable
		}
	}
	if err := s.repo.CloseRequest(ctx, requestID); err != nil {
		return fmt.Errorf("close request: %w", err)
	}
	return nil
}

// validateBaseURL 用 NewAPIHosts allowlist + 私网/HTTP 策略校验上游 base url。
func (s *UpstreamPriceSyncService) validateBaseURL(raw string) (string, error) {
	allow := s.cfg.Security.URLAllowlist
	return urlvalidator.ValidateHTTPURL(raw, allow.AllowInsecureHTTP, urlvalidator.ValidationOptions{
		AllowedHosts:     allow.NewAPIHosts,
		RequireAllowlist: allow.Enabled,
		AllowPrivate:     allow.AllowPrivateHosts,
	})
}
