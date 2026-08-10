package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// channelApplier 解耦对 ChannelService 的依赖(便于测试)。
// *ChannelService 通过 ApplyUpstreamPricingEntry + GetByID 隐式实现该接口;
// 测试可用最小 fake 替换,无需构造完整 ChannelService。
type channelApplier interface {
	ApplyUpstreamPricingEntry(ctx context.Context, channelID int64, platform string, models []string, price ConvertedPrice) (*ChannelModelPricing, error)
	GetByID(ctx context.Context, id int64) (*Channel, error)
}

// UpstreamPriceSyncService 上游 new-api 定价同步的编排核心:
// 拉取(Task 6)→ 还原/推断/diff(Task 4/5)→ 落审批单(Task 7 repo)→ 应用(Task 8)。
type UpstreamPriceSyncService struct {
	repo           UpstreamPriceSyncRepository
	client         *UpstreamPricingClient
	channelService channelApplier
	cfg            *config.Config
}

// NewUpstreamPriceSyncService 构造同步服务。chSvc 通常是 *ChannelService
// (满足 channelApplier);声明为接口类型以便测试注入 fake,生产侧传 *ChannelService 即可。
func NewUpstreamPriceSyncService(repo UpstreamPriceSyncRepository, client *UpstreamPricingClient, chSvc channelApplier, cfg *config.Config) *UpstreamPriceSyncService {
	return &UpstreamPriceSyncService{repo: repo, client: client, channelService: chSvc, cfg: cfg}
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
	snap, err := s.client.FetchPricing(ctx, baseURL, cfgRec.PricingSource)
	if err != nil {
		_ = s.repo.UpdateConfigSyncState(ctx, configID, time.Now(), "", err.Error())
		return 0, fmt.Errorf("fetch upstream: %w", err)
	}

	// 还原 USD 单价 + 推断平台 + 保留原始倍率快照
	upstream := make(map[string]ConvertedPrice, len(snap.Models))
	platforms := make(map[string]string, len(snap.Models))
	rawByName := make(map[string]map[string]any, len(snap.Models))
	for _, m := range snap.Models {
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
