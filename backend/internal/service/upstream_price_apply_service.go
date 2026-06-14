package service

import (
	"context"
	"fmt"
	"log/slog"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ApplyMode 价格变动的应用模式。
//
// follow_cost：跟随上游成本变化——改 channel 单价，group 倍率不动（售价随成本浮动）。
// lock_price：锁价——同时改 channel 单价与 group 倍率，维持最终售价不变。
type ApplyMode string

const (
	ApplyFollowCost ApplyMode = "follow_cost" // 改 channel 单价，倍率不动
	ApplyLockPrice  ApplyMode = "lock_price"  // 改单价 + group 倍率（维持售价）
)

// 应用目标类型（写入 change.applied_target）
const (
	appliedTargetChannelPricing  = "channel_pricing"
	appliedTargetGroupMultiplier = "group_multiplier"
)

// ChannelPricingWriter 写入 channel_model_pricing 的最小接口。
// 由 ChannelService 适配（避免 ApplyService 直接依赖 ChannelRepository）。
type ChannelPricingWriter interface {
	// ReplaceModelPricingForModel 更新指定 channel 下匹配 modelName 的定价行单价
	// （per-token USD）。未命中则插入一条 token 模式新行。
	ReplaceModelPricingForModel(ctx context.Context, channelID int64, modelName string, inputPrice, outputPrice float64) error
	// InvalidateChannelCache 通知渠道缓存失效（让新价格立即生效）。
	InvalidateChannelCache()
}

// GroupRateWriter 写入 group.rate_multiplier 的最小接口。
type GroupRateWriter interface {
	UpdateRateMultiplier(ctx context.Context, groupID int64, multiplier float64) error
}

// ApplyTargetReader 读取 apply 目标下拉列表（follow_cost→channels，lock_price→groups）。
// 由 ApplyTargetReaderAdapter 适配 ChannelRepository，避免 ApplyService 直接依赖 repo。
type ApplyTargetReader interface {
	ListChannelsByModel(ctx context.Context, modelName string) ([]ChannelApplyTarget, error)
	ListGroupsByChannels(ctx context.Context, channelIDs []int64) ([]GroupApplyTarget, error)
	// CountDistinctModelsByGroups 返回每个 group 绑定的去重模型数（误伤检测用）。
	CountDistinctModelsByGroups(ctx context.Context, groupIDs []int64) (map[int64]int, error)
}

// ApplyTargetsResponse apply 弹窗下拉数据。
type ApplyTargetsResponse struct {
	Channels []ChannelApplyTarget `json:"channels"`       // follow_cost 用
	Groups   []GroupApplyTarget   `json:"groups"`         // lock_price 用
	Warnings []ApplyTargetWarning `json:"warnings,omitempty"` // lock_price 误伤软警告
}

// ApplyTargetWarning lock_price 误伤警告（软警告，不阻止 apply）。
// 当 group 绑定多个模型时，lock_price 改 group.rate_multiplier 会连带改变其它模型的售价，
// 前端据此展示琥珀色提示横幅。
type ApplyTargetWarning struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Message   string `json:"message"`
}

// AuditEvent 审计事件载荷。
type AuditEvent struct {
	Action  string         // 如 "upstream_price.apply"
	AdminID int64          // 操作人
	Detail  map[string]any // 详情
}

// AuditLogger 管理员操作审计接口。无通用审计表时，默认实现落 slog。
type AuditLogger interface {
	Log(ctx context.Context, e AuditEvent)
}

// slogAuditLogger AuditLogger 的默认实现：记录到 slog（不持久化到 DB）。
// 满足"全程记审计"的要求，且不引入 repository 依赖。
type slogAuditLogger struct{}

// NewSlogAuditLogger 创建一个基于 slog 的审计 logger。
func NewSlogAuditLogger() AuditLogger { return &slogAuditLogger{} }

func (slogAuditLogger) Log(_ context.Context, e AuditEvent) {
	slog.Info("admin audit",
		"action", e.Action,
		"admin_id", e.AdminID,
		"detail", e.Detail,
	)
}

// ApplyRequest 应用一条价格变动的请求。
type ApplyRequest struct {
	ChangeID int64
	Mode     ApplyMode
	TargetID int64 // follow_cost: channel_id；lock_price: group_id
}

// UpstreamPriceApplyService 管理员人工审计闸门：把上游参考价写入本地计费链路。
//
// 参考价表（upstream_model_prices）不直接进计费，只有本服务 Apply 才会落
// channel_model_pricing / group.rate_multiplier。
type UpstreamPriceApplyService struct {
	priceRepo     UpstreamPriceRepository
	channelWriter ChannelPricingWriter
	groupWriter   GroupRateWriter
	targetReader  ApplyTargetReader
	auditLogger   AuditLogger
}

// NewUpstreamPriceApplyService 构造 ApplyService。
// channelWriter / groupWriter / targetReader / auditLogger 可为 nil（测试或部分接线场景），
// nil 时对应写入会被跳过并记 slog 警告（仅审计仍尽力记录）。
func NewUpstreamPriceApplyService(
	priceRepo UpstreamPriceRepository,
	channelWriter ChannelPricingWriter,
	groupWriter GroupRateWriter,
	targetReader ApplyTargetReader,
	auditLogger AuditLogger,
) *UpstreamPriceApplyService {
	if auditLogger == nil {
		auditLogger = NewSlogAuditLogger()
	}
	return &UpstreamPriceApplyService{
		priceRepo:     priceRepo,
		channelWriter: channelWriter,
		groupWriter:   groupWriter,
		targetReader:  targetReader,
		auditLogger:   auditLogger,
	}
}

// Apply 应用一条价格变动到本地计费链路。
//
//	follow_cost → 写 channel_model_pricing（单价=change 的 curr 价格）
//	lock_price  → 写单价 + group.rate_multiplier（change.SuggestedMultiplier）
//
// 必须校验 change.Status=="pending"，否则 BadRequest。
// 成功后 UpdateChangeApplied（status=applied + applied_by + applied_target + applied_target_id）。
// 全程记审计日志。
func (s *UpstreamPriceApplyService) Apply(ctx context.Context, req ApplyRequest, adminID int64) error {
	if req.Mode != ApplyFollowCost && req.Mode != ApplyLockPrice {
		return errors.BadRequest("UPSTREAM_PRICE_INVALID_MODE",
			fmt.Sprintf("invalid apply mode: %s", req.Mode))
	}

	change, err := s.priceRepo.GetChange(ctx, req.ChangeID)
	if err != nil {
		return fmt.Errorf("get price change: %w", err)
	}
	if change.Status != UpstreamPriceChangeStatusPending {
		return errors.BadRequest("CHANGE_NOT_PENDING",
			fmt.Sprintf("change %d status is %q, only pending can be applied", req.ChangeID, change.Status))
	}
	if req.Mode == ApplyLockPrice && change.SuggestedMultiplier == nil {
		return errors.BadRequest("LOCK_PRICE_NO_MULTIPLIER",
			"lock_price mode requires a suggested multiplier on the change")
	}

	modelName := change.LocalModelName
	if modelName == "" {
		modelName = change.ModelName
	}

	// 1) 写 channel 单价（两种 mode 都写）。
	// follow_cost: TargetID = channel_id
	// lock_price:  TargetID = group_id → 单价仍写到对应 channel（取 group 关联渠道）
	var channelID int64
	var appliedTarget string
	switch req.Mode {
	case ApplyFollowCost:
		channelID = req.TargetID
		appliedTarget = appliedTargetChannelPricing
	case ApplyLockPrice:
		// lock_price 的 target 是 group，需先解析 group → channel。
		chID, lookupErr := s.resolveChannelForGroup(ctx, req.TargetID)
		if lookupErr != nil {
			return lookupErr
		}
		channelID = chID
		appliedTarget = appliedTargetGroupMultiplier
	}

	if s.channelWriter != nil {
		if err := s.channelWriter.ReplaceModelPricingForModel(ctx, channelID, modelName,
			change.CurrInputPrice, change.CurrOutputPrice); err != nil {
			return fmt.Errorf("replace channel pricing for model: %w", err)
		}
		s.channelWriter.InvalidateChannelCache()
	} else {
		slog.Warn("upstream price apply: channel writer is nil, skipping channel pricing write",
			"change_id", req.ChangeID, "channel_id", channelID)
	}

	// 2) lock_price：额外写 group 倍率。
	if req.Mode == ApplyLockPrice && s.groupWriter != nil {
		if err := s.groupWriter.UpdateRateMultiplier(ctx, req.TargetID, *change.SuggestedMultiplier); err != nil {
			return fmt.Errorf("update group rate_multiplier: %w", err)
		}
	}

	// 3) 更新 change 状态。
	// follow_cost: target_id = channel_id；lock_price: target_id = group_id。
	if err := s.priceRepo.UpdateChangeApplied(ctx, req.ChangeID, adminID, appliedTarget, req.TargetID); err != nil {
		return fmt.Errorf("mark change applied: %w", err)
	}

	// 4) 审计。
	s.auditLogger.Log(ctx, AuditEvent{
		Action:  "upstream_price.apply",
		AdminID: adminID,
		Detail: map[string]any{
			"change_id":      req.ChangeID,
			"mode":           string(req.Mode),
			"target_id":      req.TargetID,
			"channel_id":     channelID,
			"model":          modelName,
			"input_price":    change.CurrInputPrice,
			"output_price":   change.CurrOutputPrice,
			"multiplier":     change.SuggestedMultiplier,
			"applied_target": appliedTarget,
		},
	})
	return nil
}

// 读 apply 前实际值（用于覆盖保护 + 撤销回滚）。
// 通过 type assertion 从 channelWriter / groupWriter 扩展获取，不增构造参数。
type channelPricingSnapshotReader interface {
	GetCurrentPriceForModel(ctx context.Context, channelID int64, modelName string) (inputPrice, outputPrice float64, err error)
}
type groupRateSnapshotReader interface {
	GetRateMultiplierByGroupID(ctx context.Context, groupID int64) (float64, error)
}

// resolveChannelForGroup 解析 group → channel。ApplyService 不持有 ChannelService
// 的 group 映射能力，因此通过 ChannelPricingWriter 扩展方法获取（若实现）；
// 若 writer 不支持则返回 BadRequest 引导管理员用 follow_cost + channel_id。
func (s *UpstreamPriceApplyService) resolveChannelForGroup(ctx context.Context, groupID int64) (int64, error) {
	type groupChannelResolver interface {
		GetChannelIDForGroup(ctx context.Context, groupID int64) (int64, error)
	}
	resolver, ok := s.channelWriter.(groupChannelResolver)
	if !ok {
		return 0, errors.BadRequest("LOCK_PRICE_NO_CHANNEL_RESOLVER",
			"lock_price mode needs a channel resolver on the channel writer; use follow_cost with channel_id instead")
	}
	chID, err := resolver.GetChannelIDForGroup(ctx, groupID)
	if err != nil {
		return 0, fmt.Errorf("resolve channel for group: %w", err)
	}
	if chID == 0 {
		return 0, errors.BadRequest("GROUP_HAS_NO_CHANNEL",
			fmt.Sprintf("group %d has no associated channel", groupID))
	}
	return chID, nil
}

// Dismiss 忽略一条变动（status=dismissed），不改计费。
func (s *UpstreamPriceApplyService) Dismiss(ctx context.Context, changeID, adminID int64) error {
	change, err := s.priceRepo.GetChange(ctx, changeID)
	if err != nil {
		return fmt.Errorf("get price change: %w", err)
	}
	if change.Status != UpstreamPriceChangeStatusPending {
		return errors.BadRequest("CHANGE_NOT_PENDING",
			fmt.Sprintf("change %d status is %q, only pending can be dismissed", changeID, change.Status))
	}
	if err := s.priceRepo.UpdateChangeDismissed(ctx, changeID, adminID); err != nil {
		return fmt.Errorf("mark change dismissed: %w", err)
	}
	s.auditLogger.Log(ctx, AuditEvent{
		Action:  "upstream_price.dismiss",
		AdminID: adminID,
		Detail: map[string]any{
			"change_id": changeID,
		},
	})
	return nil
}

// BatchApplyResult 批量应用结果。
type BatchApplyResult struct {
	Succeeded []int64         // 成功的 change_id 列表
	Failed    map[int64]error // 失败的 change_id → error
}

// BatchApply 批量应用。逐条执行（非事务），部分失败不影响其他条目。
// 返回每个 change_id 的成功/失败结果，err 仅在不可恢复（如构造失败）时非 nil。
func (s *UpstreamPriceApplyService) BatchApply(ctx context.Context, reqs []ApplyRequest, adminID int64) (*BatchApplyResult, error) {
	result := &BatchApplyResult{
		Succeeded: make([]int64, 0, len(reqs)),
		Failed:    make(map[int64]error, len(reqs)),
	}
	for _, req := range reqs {
		if err := s.Apply(ctx, req, adminID); err != nil {
			result.Failed[req.ChangeID] = err
			continue
		}
		result.Succeeded = append(result.Succeeded, req.ChangeID)
	}
	return result, nil
}

// ListChanges lists price changes matching the given filters. Thin pass-through
// over the repo so handlers stay free of repository dependencies.
func (s *UpstreamPriceApplyService) ListChanges(ctx context.Context, filters ChangeFilters) ([]*dbent.UpstreamPriceChange, error) {
	return s.priceRepo.ListPendingChanges(ctx, filters)
}

// PriceCompareRow is one row of the upstream-reference-vs-local-channel compare view.
type PriceCompareRow struct {
	ModelName      string   `json:"model_name"`
	LocalModelName string   `json:"local_model_name"`
	UpstreamInput  *float64 `json:"upstream_input_price"`
	UpstreamOutput *float64 `json:"upstream_output_price"`
	LocalInput     *float64 `json:"local_input_price"`
	LocalOutput    *float64 `json:"local_output_price"`
	InputDeltaPct  *float64 `json:"input_delta_pct"`
	OutputDeltaPct *float64 `json:"output_delta_pct"`
}

// ComparePrices returns a per-model comparison of the upstream reference prices
// (from upstream_model_prices for the given source) against themselves — the
// handler layers the local channel price on top. The sync subsystem already
// persists upstream_model_prices; this method just exposes them. Returns an
// empty slice when the source has no reference prices.
//
// The local-vs-upstream delta is computed here (not in the repo) to keep the
// repository a thin data-access layer. localInput/localOutput are left nil in
// this base implementation; the handler enriches them with channel pricing if
// available. This keeps the service self-contained and testable.
func (s *UpstreamPriceApplyService) ComparePrices(ctx context.Context, sourceID int64) ([]PriceCompareRow, error) {
	prices, err := s.priceRepo.ListModelPrices(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("list model prices: %w", err)
	}
	rows := make([]PriceCompareRow, 0, len(prices))
	for _, p := range prices {
		if p == nil {
			continue
		}
		row := PriceCompareRow{
			ModelName:      p.ModelName,
			LocalModelName: p.LocalModelName,
		}
		if p.InputPrice > 0 {
			in := p.InputPrice
			row.UpstreamInput = &in
		}
		if p.OutputPrice > 0 {
			out := p.OutputPrice
			row.UpstreamOutput = &out
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// GetApplyTargets 返回某 change 在本地相关的 channel + group 列表，供 apply 弹窗下拉填充。
//
// channels：channel_model_pricing.models JSONB 含 change.LocalModelName 的所有渠道（follow_cost 用）。
// groups：与上述 channels 通过 channel_groups 关联的 group（lock_price 用）。
// targetReader 为 nil（未接线）时返回空 channels + groups，前端退化为输入框。
func (s *UpstreamPriceApplyService) GetApplyTargets(ctx context.Context, changeID int64) (*ApplyTargetsResponse, error) {
	resp := &ApplyTargetsResponse{
		Channels: []ChannelApplyTarget{},
		Groups:   []GroupApplyTarget{},
	}
	if s.targetReader == nil {
		return resp, nil
	}

	change, err := s.priceRepo.GetChange(ctx, changeID)
	if err != nil {
		return nil, fmt.Errorf("get price change: %w", err)
	}
	modelName := change.LocalModelName
	if modelName == "" {
		modelName = change.ModelName
	}
	if modelName == "" {
		return resp, nil
	}

	channels, err := s.targetReader.ListChannelsByModel(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("list channels by model: %w", err)
	}
	if len(channels) == 0 {
		return resp, nil
	}
	resp.Channels = channels

	channelIDs := make([]int64, 0, len(channels))
	for _, c := range channels {
		channelIDs = append(channelIDs, c.ID)
	}
	groups, err := s.targetReader.ListGroupsByChannels(ctx, channelIDs)
	if err != nil {
		return nil, fmt.Errorf("list groups by channels: %w", err)
	}
	resp.Groups = groups

	// 误伤检测：对 ModelCount > 1 的 group 生成软警告。
	// lock_price 改 group.rate_multiplier 会连带改变该 group 下其它模型的售价，
	// 提示但不阻止管理员 apply。
	if len(groups) > 0 {
		groupIDs := make([]int64, 0, len(groups))
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
		}
		counts, err := s.targetReader.CountDistinctModelsByGroups(ctx, groupIDs)
		if err != nil {
			return nil, fmt.Errorf("count distinct models by groups: %w", err)
		}
		for i := range groups {
			groups[i].ModelCount = counts[groups[i].ID]
			if groups[i].ModelCount > 1 {
				resp.Warnings = append(resp.Warnings, ApplyTargetWarning{
					GroupID:   groups[i].ID,
					GroupName: groups[i].Name,
					Message:   fmt.Sprintf("此分组还绑定 %d 个模型，lock_price 会连带改变它们的售价", groups[i].ModelCount),
				})
			}
		}
	}

	return resp, nil
}
