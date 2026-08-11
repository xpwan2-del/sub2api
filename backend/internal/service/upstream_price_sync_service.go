package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// channelApplier 解耦对 ChannelService 的依赖(便于测试)。
// *ChannelService 通过 ApplyUpstreamPricingEntry + GetByID 隐式实现该接口;
// 测试可用最小 fake 替换,无需构造完整 ChannelService。
type channelApplier interface {
	ApplyUpstreamPricingEntry(ctx context.Context, channelID int64, platform string, models []string, price ConvertedPrice) (*ChannelModelPricing, error)
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
	cfg                  *config.Config
}

// NewUpstreamPriceSyncService 构造同步服务。chSvc 通常是 *ChannelService
// (满足 channelApplier);声明为接口类型以便测试注入 fake,生产侧传 *ChannelService 即可。
func NewUpstreamPriceSyncService(repo UpstreamPriceSyncRepository, client *UpstreamPricingClient, chSvc channelApplier, proxyResolver upstreamProxyResolver, balanceNotifyService *BalanceNotifyService, cfg *config.Config) *UpstreamPriceSyncService {
	return &UpstreamPriceSyncService{
		repo:                 repo,
		client:               client,
		channelService:       chSvc,
		proxyResolver:        proxyResolver,
		balanceNotifyService: balanceNotifyService,
		cfg:                  cfg,
	}
}

// SyncNow 立即拉取上游定价、diff 出变更草稿并落成审批单。
// 无变更时不建空审批单,返回 0;成功返回新建审批单 ID。
func (s *UpstreamPriceSyncService) SyncNow(ctx context.Context, configID, createdBy int64) (int64, error) {
	cfgRec, err := s.repo.GetConfig(ctx, configID)
	if err != nil {
		return 0, fmt.Errorf("get source config: %w", err)
	}
	if !cfgRec.Enabled {
		return 0, fmt.Errorf("source disabled")
	}
	baseURL, err := s.validateBaseURL(cfgRec.BaseURL)
	if err != nil {
		return 0, fmt.Errorf("invalid base url: %w", err)
	}
	proxyURL, err := s.resolveProxyURL(ctx, cfgRec.ProxyID)
	if err != nil {
		return 0, fmt.Errorf("resolve proxy: %w", err)
	}
	snap, err := s.client.FetchPricing(ctx, baseURL, cfgRec.PricingSource, proxyURL, cfgRec.APIKey)
	if err != nil {
		_ = s.repo.UpdateConfigSyncState(ctx, configID, time.Now(), "", err.Error())
		return 0, fmt.Errorf("fetch upstream: %w", err)
	}

	// 还原 USD 单价 + 推断平台 + 保留原始倍率快照
	upstream := make(map[string]ConvertedPrice, len(snap.Models))
	platforms := make(map[string]string, len(snap.Models))
	rawByName := make(map[string]map[string]any, len(snap.Models))
	for _, m := range snap.Models {
		// 按配置的目标上游分组过滤:modelGroupEnabled 在 target 为空时返回 true(不过滤,向后兼容)。
		if !modelGroupEnabled(m.EnableGroups, cfgRec.TargetUpstreamGroup) {
			continue
		}
		upstream[m.ModelName] = ConvertPricing(m, cfgRec.BasePricePer1k)
		platforms[m.ModelName] = InferPlatform(m.ModelName)
		rawByName[m.ModelName] = map[string]any{
			"model_ratio":      m.ModelRatio,
			"completion_ratio": m.CompletionRatio,
			"quota_type":       m.QuotaType,
		}
	}

	// 本地渠道现有定价(ChannelService.GetByID 经 repo.ListModelPricing 装填 ModelPricing)
	ch, err := s.channelService.GetByID(ctx, cfgRec.TargetChannelID)
	if err != nil {
		return 0, fmt.Errorf("load target channel: %w", err)
	}
	drafts := DiffPricing(upstream, platforms, ch.ModelPricing, cfgRec.TargetChannelID)
	if len(drafts) == 0 {
		_ = s.repo.UpdateConfigSyncState(ctx, configID, time.Now(), snap.Version, "")
		return 0, nil // 无变化,不建空审批单
	}

	// 旧 open 单批量过期,避免同源堆积多份并发审批
	if _, err := s.repo.ExpireOpenRequests(ctx, configID); err != nil {
		return 0, fmt.Errorf("expire open requests: %w", err)
	}

	items := make([]PriceChangeItem, 0, len(drafts))
	for _, d := range drafts {
		items = append(items, PriceChangeItem{
			Kind:              d.Kind,
			Platform:          d.Platform,
			ModelName:         d.ModelName,
			TargetChannelID:   cfgRec.TargetChannelID,
			UpstreamRaw:       rawByName[d.ModelName],
			UpstreamConverted: d.Upstream,
			LocalCurrent:      d.Local,
			// 默认以上游还原值作为 apply_value;model_removed 无上游值 → nil
			ApplyValue: d.Upstream,
		})
	}
	req := &PriceChangeRequest{
		SourceConfigID:         configID,
		TriggerType:            "manual",
		UpstreamPricingVersion: snap.Version,
		CreatedBy:              createdBy,
	}
	if err := s.repo.CreateRequest(ctx, req, items); err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	_ = s.repo.UpdateConfigSyncState(ctx, configID, time.Now(), snap.Version, "")
	return req.ID, nil
}

// ReviewItem 对单条审批条目执行 apply / reject / ignore。
//   - apply: 写入目标渠道定价(Task 8 ApplyUpstreamPricingEntry)+ item.status=applied;
//     应用失败则 item.status=failed 并返回错误(失败原因记录到 review_note)。
//   - reject / ignore: 仅更新 item 状态,不触碰渠道。
func (s *UpstreamPriceSyncService) ReviewItem(ctx context.Context, itemID int64, action ReviewAction, applyValue *ConvertedPrice, reviewerID int64, note string) error {
	it, err := s.repo.GetItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("get item: %w", err)
	}
	if it.Status != "pending" {
		return fmt.Errorf("item not pending (status=%s)", it.Status)
	}
	switch action {
	case ReviewReject, ReviewIgnore:
		status := "rejected"
		if action == ReviewIgnore {
			status = "ignored"
		}
		return s.repo.UpdateItemStatus(ctx, itemID, status, reviewerID, note, nil)
	case ReviewApply:
		val := applyValue
		if val == nil {
			val = it.ApplyValue
		}
		if val == nil {
			val = it.UpstreamConverted
		}
		if val == nil {
			return fmt.Errorf("no apply value for item %d", itemID)
		}
		if _, err := s.channelService.ApplyUpstreamPricingEntry(ctx, it.TargetChannelID, it.Platform, []string{it.ModelName}, *val); err != nil {
			_ = s.repo.UpdateItemStatus(ctx, itemID, "failed", reviewerID, err.Error(), nil)
			return fmt.Errorf("apply: %w", err)
		}
		now := time.Now()
		return s.repo.UpdateItemStatus(ctx, itemID, "applied", reviewerID, note, &now)
	}
	return fmt.Errorf("unknown action: %s", action)
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
	existing, err := s.repo.GetConfig(ctx, c.ID)
	if err != nil {
		return err
	}
	c.ResetBalanceSnapshot = strings.TrimSpace(existing.BaseURL) != strings.TrimSpace(c.BaseURL) ||
		strings.TrimSpace(existing.DashboardToken) != strings.TrimSpace(c.DashboardToken)
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
	populateBalanceStatus(c)
	return nil
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
func (s *UpstreamPriceSyncService) resolveUpstreamGroups(ctx context.Context, baseURL string, proxyID *int64, apiKey string) (map[string]string, error) {
	baseURL, err := s.validateBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}
	proxyURL, err := s.resolveProxyURL(ctx, proxyID)
	if err != nil {
		return nil, fmt.Errorf("resolve proxy: %w", err)
	}
	snap, err := s.client.FetchPricing(ctx, baseURL, PricingSourceAuto, proxyURL, apiKey)
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
	return s.resolveUpstreamGroups(ctx, cfgRec.BaseURL, cfgRec.ProxyID, cfgRec.APIKey)
}

// PreviewUpstreamGroups 按 base_url 直接拉取分组(新建 source 尚未保存时用)。
func (s *UpstreamPriceSyncService) PreviewUpstreamGroups(ctx context.Context, baseURL string, proxyID *int64, apiKey string) (map[string]string, error) {
	return s.resolveUpstreamGroups(ctx, baseURL, proxyID, apiKey)
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
	return s.repo.DeleteConfig(ctx, id)
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
