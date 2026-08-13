package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// ErrRequestNotCloseable 审批批次无法关闭(不存在、或状态非 open/partially_applied、或仍有 pending 条目)。
var (
	ErrRequestNotCloseable        = errors.New("price change request not closeable")
	ErrSourceSyncBusy             = errors.New("upstream source sync is already running")
	ErrPendingGroupApproval       = errors.New("pending group ratio approval exists")
	ErrGroupRateDrift             = errors.New("group rate multiplier changed after approval was created")
	ErrGroupRateTargetUnavailable = errors.New("group rate target is no longer eligible")
	ErrItemNotPending             = errors.New("price change item is not pending")
	ErrItemRequestMismatch        = errors.New("price change item does not belong to request")
)

// PricingSource 上游定价数据源选择
type UpstreamPricingSource string

const (
	PricingSourceAuto        UpstreamPricingSource = "auto"
	PricingSourceRatioConfig UpstreamPricingSource = "ratio_config"
	PricingSourcePricing     UpstreamPricingSource = "pricing"
)

// Dashboard 鉴权模式：不同 new-api/one-api 分支对 /api/user/self 的鉴权协议不一致。
const (
	// DashboardAuthModeAuto 自动探测：依次尝试 bearer → raw，若配置了 user_id 再尝试 raw_user、bearer_user；
	// 仅在 401/403 时回退，避免无效 Token 产生过多请求。
	DashboardAuthModeAuto = "auto"
	// DashboardAuthModeBearer 使用 Authorization: Bearer <token>（新版 QuantumNous/new-api，2026-07-20 之后）。
	DashboardAuthModeBearer = "bearer"
	// DashboardAuthModeRaw 使用 Authorization: <token>（旧版 one-api）。
	DashboardAuthModeRaw = "raw"
	// DashboardAuthModeRawUser 使用 Authorization: <token> + New-Api-User: <userID>（旧版 QuantumNous/new-api，2026-07-20 之前）。
	DashboardAuthModeRawUser = "raw_user"
	// DashboardAuthModeBearerUser 使用 Authorization: Bearer <token> + New-Api-User: <userID>（部分分叉）。
	DashboardAuthModeBearerUser = "bearer_user"
)

// NormalizeDashboardAuthMode 规范化鉴权模式；空值默认 auto。
func NormalizeDashboardAuthMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "", DashboardAuthModeAuto:
		return DashboardAuthModeAuto
	case DashboardAuthModeBearer, DashboardAuthModeRaw, DashboardAuthModeRawUser, DashboardAuthModeBearerUser:
		return strings.TrimSpace(strings.ToLower(mode))
	default:
		return DashboardAuthModeAuto
	}
}

// DashboardAuthModeNeedsUserID 报告该鉴权模式是否需要 Dashboard User ID。
func DashboardAuthModeNeedsUserID(mode string) bool {
	return mode == DashboardAuthModeRawUser || mode == DashboardAuthModeBearerUser
}

// UpstreamModelPricing 从上游拉到的单个模型原始定价
type UpstreamModelPricing struct {
	ModelName        string
	ModelRatio       float64
	CompletionRatio  float64
	CacheRatio       *float64
	CreateCacheRatio *float64
	ModelPrice       *float64 // 按次 USD;<0 或 nil = 未启用按次
	QuotaType        int      // 0=ratio, 1=按次
	EnableGroups     []string
}

// PricingSnapshot 一次拉取的快照
type PricingSnapshot struct {
	Version     string // pricing_version
	Models      []UpstreamModelPricing
	GroupRatio  map[string]float64 // P2
	UsableGroup map[string]string  // P2
	FetchedAt   time.Time
	Source      string
}

// ConvertedPrice 还原后的 USD 定价(对应 channel_model_pricing 字段)
//
// JSON tag 同时服务于:① HTTP 响应序列化(gin c.JSON);② JSONB 落库
// (MarshalConverted/UnmarshalConverted → upstream_converted/local_current/apply_value)。
// 两端字段名保持一致,确保 marshal→unmarshal 可往返。
type ConvertedPrice struct {
	BillingMode     BillingMode `json:"billing_mode"` // token / per_request
	InputPrice      *float64    `json:"input_price"`
	OutputPrice     *float64    `json:"output_price"`
	CacheReadPrice  *float64    `json:"cache_read_price"`
	CacheWritePrice *float64    `json:"cache_write_price"`
	PerRequestPrice *float64    `json:"per_request_price"`
}

// PriceChangeItemKind 审批条目类型
type PriceChangeItemKind string

const (
	ItemKindModelPrice     PriceChangeItemKind = "model_price"     // 价格变更
	ItemKindModelAdded     PriceChangeItemKind = "model_added"     // 新增模型
	ItemKindModelRemoved   PriceChangeItemKind = "model_removed"   // 移除模型
	ItemKindModelUnchanged PriceChangeItemKind = "model_unchanged" // 与本地一致(无变化,仅展示)
	ItemKindGroupRatio     PriceChangeItemKind = "group_ratio"     // 上游所选分组倍率变化
)

// ReviewAction 审批动作
type ReviewAction string

const (
	ReviewApply  ReviewAction = "apply"
	ReviewReject ReviewAction = "reject"
	ReviewIgnore ReviewAction = "ignore"
)

// PriceChangeRequest 审批批次
type PriceChangeRequest struct {
	ID                     int64          `json:"id"`
	SourceConfigID         int64          `json:"source_config_id"`
	TriggerType            string         `json:"trigger_type"` // manual/scheduled
	Status                 string         `json:"status"`       // open/partially_applied/closed/expired
	UpstreamPricingVersion string         `json:"upstream_pricing_version"`
	Summary                map[string]int `json:"summary"`
	CreatedBy              int64          `json:"created_by"`
	CreatedAt              time.Time      `json:"created_at"`
	ClosedAt               *time.Time     `json:"closed_at"`
}

// PriceChangeItem 审批条目
// RateMultiplierScale 对齐 groups.rate_multiplier 的 DECIMAL(10,4) 精度。
const RateMultiplierScale = 4

// RoundRateMultiplier 统一审批建议、漂移比较与数据库写入精度。
func RoundRateMultiplier(value float64) float64 {
	factor := math.Pow10(RateMultiplierScale)
	return math.Round(value*factor) / factor
}

// GroupRateTarget 是生成倍率审批时读取到的本地分组快照。
type GroupRateTarget struct {
	ID             int64
	Name           string
	SortOrder      int
	RateMultiplier float64
}

// GroupRateChange 冻结一次上游倍率变化对单个本地分组的影响。
type GroupRateChange struct {
	Strategy            string  `json:"strategy"`
	UpstreamGroupKey    string  `json:"upstream_group_key"`
	UpstreamGroupName   string  `json:"upstream_group_name"`
	UpstreamOldRatio    float64 `json:"upstream_old_ratio"`
	UpstreamNewRatio    float64 `json:"upstream_new_ratio"`
	LocalGroupID        int64   `json:"local_group_id"`
	LocalGroupName      string  `json:"local_group_name"`
	LocalGroupSortOrder int     `json:"local_group_sort_order"`
	LocalCurrentRate    float64 `json:"local_current_rate"`
	SuggestedRate       float64 `json:"suggested_rate"`
}

// BuildGroupRateChanges 按上游变化比例为每个目标分组生成建议值。
func BuildGroupRateChanges(targets []GroupRateTarget, upstreamKey, upstreamName string, oldRatio, newRatio float64) ([]GroupRateChange, error) {
	if !validPositiveFinite(oldRatio) || !validPositiveFinite(newRatio) {
		return nil, errors.New("upstream group ratio must be finite and > 0")
	}
	sorted := append([]GroupRateTarget(nil), targets...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].SortOrder == sorted[j].SortOrder {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].SortOrder < sorted[j].SortOrder
	})
	changes := make([]GroupRateChange, 0, len(sorted))
	for _, target := range sorted {
		if !validPositiveFinite(target.RateMultiplier) {
			return nil, fmt.Errorf("group %d rate multiplier must be finite and > 0", target.ID)
		}
		current := RoundRateMultiplier(target.RateMultiplier)
		changes = append(changes, GroupRateChange{
			Strategy:            "proportional_v1",
			UpstreamGroupKey:    upstreamKey,
			UpstreamGroupName:   upstreamName,
			UpstreamOldRatio:    oldRatio,
			UpstreamNewRatio:    newRatio,
			LocalGroupID:        target.ID,
			LocalGroupName:      target.Name,
			LocalGroupSortOrder: target.SortOrder,
			LocalCurrentRate:    current,
			SuggestedRate:       RoundRateMultiplier(current * newRatio / oldRatio),
		})
	}
	return changes, nil
}

func validPositiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

type PriceChangeItem struct {
	ID                int64               `json:"id"`
	RequestID         int64               `json:"request_id"`
	Kind              PriceChangeItemKind `json:"kind"`
	Platform          string              `json:"platform"`
	ModelName         string              `json:"model_name"`
	TargetChannelID   int64               `json:"target_channel_id"`
	TargetGroupID     *int64              `json:"target_group_id,omitempty"`
	UpstreamRaw       map[string]any      `json:"upstream_raw"`
	UpstreamConverted *ConvertedPrice     `json:"upstream_converted"`
	LocalCurrent      *ConvertedPrice     `json:"local_current"`
	ApplyValue        *ConvertedPrice     `json:"apply_value"`
	GroupRateChange   *GroupRateChange    `json:"group_rate_change,omitempty"`
	ApplyRate         *float64            `json:"apply_rate,omitempty"`
	Status            string              `json:"status"` // pending/rejected/ignored/applied/failed/no_change
	ReviewerID        int64               `json:"reviewer_id"`
	ReviewNote        string              `json:"review_note"`
	ReviewedAt        *time.Time          `json:"reviewed_at"`
	AppliedAt         *time.Time          `json:"applied_at"`
	CreatedAt         time.Time           `json:"created_at"`
}

// ConvertPricing 把上游倍率还原成 USD 绝对单价。
func ConvertPricing(m UpstreamModelPricing, basePer1k float64) ConvertedPrice {
	if m.QuotaType == 1 && m.ModelPrice != nil && *m.ModelPrice >= 0 {
		p := *m.ModelPrice
		return ConvertedPrice{BillingMode: BillingModePerRequest, PerRequestPrice: &p}
	}
	input := m.ModelRatio * basePer1k / 1000
	out := ConvertedPrice{BillingMode: BillingModeToken, InputPrice: &input}
	if m.CompletionRatio != 0 {
		o := input * m.CompletionRatio
		out.OutputPrice = &o
	}
	if m.CacheRatio != nil {
		c := input * *m.CacheRatio
		out.CacheReadPrice = &c
	}
	if m.CreateCacheRatio != nil {
		cw := input * *m.CreateCacheRatio
		out.CacheWritePrice = &cw
	}
	return out
}

// defaultMissingPriceFields 生成审批单「应用值」的默认值:把上游还原值中缺失(nil)
// 的价格字段补为 0。上游未返回的字段(如无 cache_ratio → cache_read=nil)在应用值里
// 显式写 0,与本地定价的 0 语义一致,避免 apply 落库后该字段悬空为 nil。
func defaultMissingPriceFields(up *ConvertedPrice) *ConvertedPrice {
	if up == nil {
		return nil
	}
	out := *up
	if out.InputPrice == nil {
		z := 0.0
		out.InputPrice = &z
	}
	if out.OutputPrice == nil {
		z := 0.0
		out.OutputPrice = &z
	}
	if out.CacheReadPrice == nil {
		z := 0.0
		out.CacheReadPrice = &z
	}
	if out.CacheWritePrice == nil {
		z := 0.0
		out.CacheWritePrice = &z
	}
	if out.PerRequestPrice == nil {
		z := 0.0
		out.PerRequestPrice = &z
	}
	return &out
}

// InferPlatform 按模型名前缀推断平台;未识别返回空串。
func InferPlatform(modelName string) string {
	name := strings.ToLower(modelName)
	switch {
	case strings.HasPrefix(name, "gpt-") || strings.HasPrefix(name, "chatgpt-") ||
		strings.HasPrefix(name, "o1-") || strings.HasPrefix(name, "o3-") || strings.HasPrefix(name, "text-"):
		return PlatformOpenAI
	case strings.HasPrefix(name, "claude-"):
		return PlatformAnthropic
	case strings.HasPrefix(name, "gemini-"):
		return PlatformGemini
	case strings.HasPrefix(name, "grok-"):
		return PlatformGrok
	}
	return ""
}

// validSyncPlatform 报告 platform 是否为渠道支持的平台之一。
// 与 domain/model 两处的平台常量保持一致(anthropic/openai/gemini/antigravity/grok)。
// 上游模型名推断不出平台时(InferPlatform 返回空串),审批时可人工补充,须限制在此集合内。
func validSyncPlatform(platform string) bool {
	switch platform {
	case PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok:
		return true
	}
	return false
}

const priceTolerance = 1e-9

// PriceChangeItemDraft diff 产出的待落库草稿
type PriceChangeItemDraft struct {
	Kind        PriceChangeItemKind
	Platform    string
	ModelName   string
	UpstreamRaw map[string]any
	Upstream    *ConvertedPrice
	Local       *ConvertedPrice
}

// DiffPricing 对比上游快照(已还原)与本地渠道现有定价。
// 以 (platform, model_name) 为键;local 一条 ChannelModelPricing 可能含多模型,展开。
//
// 输出顺序对齐「渠道管理 → 模型定价」的添加顺序(= channel_model_pricing.id 升序,
// 即 local 切片的传入顺序):第一遍按 local 顺序遍历本地模型 → 价格变更 / 无变化 / 移除;
// 第二遍把上游有、本地无的新增模型按模型名稳定排序后追加(渠道里没有对应顺序,
// 按名排序避免上游 map 随机迭代导致每次同步顺序都变)。落库后 item.id 升序继承该顺序,
// 审批单同类型分组内(ListItems ORDER BY id ASC)即按渠道顺序展示。
func DiffPricing(upstream map[string]ConvertedPrice, upstreamPlatforms map[string]string, local []ChannelModelPricing, channelID int64) []PriceChangeItemDraft {
	type key struct{ platform, model string }
	// localIdx: (platform, model) → 本地定价记录;localOrder 保持 local 切片顺序(渠道添加顺序)。
	localIdx := map[key]*ChannelModelPricing{}
	var localOrder []key
	for i := range local {
		p := &local[i]
		for _, m := range p.Models {
			k := key{p.Platform, m}
			if _, dup := localIdx[k]; !dup {
				localOrder = append(localOrder, k)
			}
			localIdx[k] = p
		}
	}
	// 上游每个模型名唯一对应一个平台;upstreamKey[name] = (platform, name)。
	upstreamKey := make(map[string]key, len(upstream))
	for name := range upstream {
		upstreamKey[name] = key{upstreamPlatforms[name], name}
	}

	var drafts []PriceChangeItemDraft
	// matchedNames: 已被本地认领的上游模型名。上游模型名唯一,按名记认领即可覆盖
	// 平台缺失兜底(上游推断不出平台时按模型名匹配),无需再按 (platform, model) 记。
	matchedNames := map[string]bool{}
	for _, k := range localOrder {
		lp := localIdx[k]
		localPrice := channelPricingToConverted(lp)
		uk, hasUp := upstreamKey[k.model]
		// 平台一致才按 (platform, model) 精确匹配;上游推断不出平台(渠道字段缺失,
		// InferPlatform 返回空串)时,退化为按模型名匹配。场景:模型名无法推断平台,
		// 由管理员手动指定平台加入渠道;再次同步时上游仍无平台信息,仅凭模型名 + 价格
		// 判等即可认定为同一模型,避免误判为「移除 + 新增」。
		if hasUp && (uk.platform == k.platform || uk.platform == "") {
			matchedNames[k.model] = true
			upCopy := upstream[k.model]
			if convertedEqual(&upCopy, localPrice) {
				// 与本地完全一致:仍生成条目(kind=model_unchanged)供审批单完整展示,
				// 但落库即终态 no_change,不参与审批、不阻塞状态机闭环。
				drafts = append(drafts, PriceChangeItemDraft{Kind: ItemKindModelUnchanged, Platform: k.platform, ModelName: k.model, Upstream: &upCopy, Local: localPrice})
			} else {
				drafts = append(drafts, PriceChangeItemDraft{Kind: ItemKindModelPrice, Platform: k.platform, ModelName: k.model, Upstream: &upCopy, Local: localPrice})
			}
			continue
		}
		// 本地有、上游无(或上游推断平台与本地记录不一致) → 移除。
		drafts = append(drafts, PriceChangeItemDraft{Kind: ItemKindModelRemoved, Platform: k.platform, ModelName: k.model, Local: localPrice})
	}
	// 第二遍:上游有、本地无 → 新增。按模型名稳定排序追加。
	var addedNames []string
	for name := range upstreamKey {
		if !matchedNames[name] {
			addedNames = append(addedNames, name)
		}
	}
	sort.Strings(addedNames)
	for _, name := range addedNames {
		upCopy := upstream[name]
		drafts = append(drafts, PriceChangeItemDraft{Kind: ItemKindModelAdded, Platform: upstreamPlatforms[name], ModelName: name, Upstream: &upCopy})
	}
	return drafts
}

// modelGroupEnabled 报告模型是否应当进入本次同步。
//   - target 为空 → 调用方未配置过滤,全部保留(向后兼容);
//   - enableGroups 为空 → new-api 语义视为全分组可用,保留;
//   - enableGroups 含 target → 命中,保留;
//   - 否则 → 该模型未在目标分组启用,跳过。
func modelGroupEnabled(enableGroups []string, target string) bool {
	if target == "" {
		return true
	}
	if len(enableGroups) == 0 {
		return true
	}
	for _, g := range enableGroups {
		if g == target {
			return true
		}
	}
	return false
}

func channelPricingToConverted(p *ChannelModelPricing) *ConvertedPrice {
	return &ConvertedPrice{
		BillingMode:     p.BillingMode,
		InputPrice:      p.InputPrice,
		OutputPrice:     p.OutputPrice,
		CacheReadPrice:  p.CacheReadPrice,
		CacheWritePrice: p.CacheWritePrice,
		PerRequestPrice: p.PerRequestPrice,
	}
}

func convertedEqual(a, b *ConvertedPrice) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.BillingMode == b.BillingMode &&
		floatEq(a.InputPrice, b.InputPrice) && floatEq(a.OutputPrice, b.OutputPrice) &&
		floatEq(a.CacheReadPrice, b.CacheReadPrice) && floatEq(a.CacheWritePrice, b.CacheWritePrice) &&
		floatEq(a.PerRequestPrice, b.PerRequestPrice)
}

// floatEq 数值容差比较;nil 视为 0 参与对比。
// 上游未返回的字段(如无 cache_ratio → cache_read=nil)与本地显式 0 视为等价,
// 避免「上游无缓存价 vs 本地缓存价=0」被误判为价格变更。
func floatEq(a, b *float64) bool {
	av, bv := 0.0, 0.0
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	return math.Abs(av-bv) <= priceTolerance
}

// UpstreamSourceConfig 上游 new-api 同步源配置。APIKey/DashboardToken 在内存中
// 为明文,落库时由 repository 层加密存储。
type UpstreamSourceConfig struct {
	ID                          int64                 `json:"id"`
	Name                        string                `json:"name"`
	BaseURL                     string                `json:"base_url"`
	APIKey                      string                `json:"api_key"`         // 内存明文;落库加密
	DashboardToken              string                `json:"dashboard_token"` // 内存明文;落库加密(余额查询用)
	DashboardAuthMode           string                `json:"dashboard_auth_mode"`
	DashboardUserID             *int64                `json:"dashboard_user_id"`
	ProxyID                     *int64                `json:"proxy_id"`
	TargetChannelID             int64                 `json:"target_channel_id"`
	Enabled                     bool                  `json:"enabled"`
	BasePricePer1k              float64               `json:"base_price_per_1k"`
	PricingSource               UpstreamPricingSource `json:"pricing_source"`
	SyncModelPrice              bool                  `json:"sync_model_price"`
	SyncGroupRatio              bool                  `json:"sync_group_ratio"`
	GroupMapping                map[string]int64      `json:"group_mapping"`
	TargetUpstreamGroup         string                `json:"target_upstream_group"` // 上游 group_ratio key;空=不过滤(全量同步)
	ExcludedGroupIDs            []int64               `json:"excluded_group_ids"`
	GroupRatioBaselineKey       string                `json:"group_ratio_baseline_key"`
	GroupRatioBaselineValue     *float64              `json:"group_ratio_baseline_value"`
	GroupRatioBaselineObserved  *time.Time            `json:"group_ratio_baseline_observed_at"`
	BalanceThresholdUSD         *float64              `json:"balance_threshold_usd"`
	LastBalanceQuota            *int64                `json:"last_balance_quota"`
	LastUsedQuota               *int64                `json:"last_used_quota"`
	LastBalanceUSD              *float64              `json:"last_balance_usd"`
	LastBalanceAt               *time.Time            `json:"last_balance_at"`
	LastBalanceCheckedAt        *time.Time            `json:"last_balance_checked_at"`
	LastBalanceError            string                `json:"last_balance_error"`
	BalanceStatus               string                `json:"balance_status"`
	LastSyncAt                  *time.Time            `json:"last_sync_at"`
	LastPricingVersion          string                `json:"last_pricing_version"`
	LastError                   string                `json:"last_error"`
	CreatedAt                   time.Time             `json:"created_at"`
	UpdatedAt                   time.Time             `json:"updated_at"`
	ResetBalanceSnapshot        bool                  `json:"-"`
	ResetGroupRatioBaseline     bool                  `json:"-"`
	ExpirePendingApprovals      bool                  `json:"-"`
	ExpirePendingGroupApprovals bool                  `json:"-"`
}

// RequestFilter 审批批次列表过滤 + 分页参数。
type RequestFilter struct {
	SourceConfigID *int64
	Status         string
	Page           int
	PageSize       int
}

// UpstreamPriceSyncRepository 上游定价同步的持久化接口(config + request/items CRUD)。
// SyncPersistInput 是一次同步的原子持久化结果。
type SyncPersistInput struct {
	ConfigID        int64
	ObservedAt      time.Time
	PricingVersion  string
	BaselineKey     string
	BaselineValue   *float64
	AdvanceBaseline bool
	Request         *PriceChangeRequest
	Items           []PriceChangeItem
}

// GroupRateApplyInput 是倍率审批事务的输入。
type GroupRateApplyInput struct {
	RequestID  int64
	ItemID     int64
	ReviewerID int64
	Note       string
	ApplyRate  float64
}

type UpstreamPriceSyncRepository interface {
	CreateConfig(ctx context.Context, c *UpstreamSourceConfig) error
	GetConfig(ctx context.Context, id int64) (*UpstreamSourceConfig, error)
	GetConfigByName(ctx context.Context, name string) (*UpstreamSourceConfig, error)
	ListConfigs(ctx context.Context) ([]UpstreamSourceConfig, error)
	UpdateConfig(ctx context.Context, c *UpstreamSourceConfig) error
	DeleteConfig(ctx context.Context, id int64) error
	UpdateConfigSyncState(ctx context.Context, id int64, lastSyncAt time.Time, version, lastErr string) error
	UpdateConfigBalanceSuccess(ctx context.Context, id int64, snapshot BalanceSnapshot) error
	UpdateConfigBalanceError(ctx context.Context, id int64, checkedAt time.Time, lastErr string) error
	WithSourceSyncLock(ctx context.Context, configID int64, fn func(context.Context) error) error
	ListGroupRateTargets(ctx context.Context, configID, channelID int64) ([]GroupRateTarget, error)
	HasPendingGroupRateItems(ctx context.Context, configID int64) (bool, error)
	PersistSyncResult(ctx context.Context, input SyncPersistInput) error

	CreateRequest(ctx context.Context, req *PriceChangeRequest, items []PriceChangeItem) error
	GetRequest(ctx context.Context, id int64) (*PriceChangeRequest, error)
	ListRequests(ctx context.Context, f RequestFilter) ([]PriceChangeRequest, int64, error)
	ListItems(ctx context.Context, requestID int64) ([]PriceChangeItem, error)
	GetItem(ctx context.Context, id int64) (*PriceChangeItem, error)
	UpdateItemStatus(ctx context.Context, id int64, status string, reviewerID int64, note string, appliedAt *time.Time) error
	FinalizeItemCAS(ctx context.Context, requestID, itemID int64, status string, reviewerID int64, note string, applyValue *ConvertedPrice) error
	UpdateItemPlatform(ctx context.Context, requestID, itemID int64, platform string) error
	ApplyGroupRateItem(ctx context.Context, input GroupRateApplyInput) (int64, float64, error)
	ExpireOpenRequests(ctx context.Context, configID int64) (int, error)
	CloseRequest(ctx context.Context, requestID int64) error
	UpdateRequestStatus(ctx context.Context, requestID int64, status string, summary map[string]int) error
}

// MarshalConverted / UnmarshalConverted — JSONB 落库辅助。
func MarshalConverted(c *ConvertedPrice) ([]byte, error) { return json.Marshal(c) }
func UnmarshalConverted(b []byte) (*ConvertedPrice, error) {
	// nil 指针经 MarshalConverted 落库为 JSON "null";读回时须还原为 nil,
	// 否则 local_current/upstream_converted/apply_value 会变成空对象(字段全 nil),
	// 前端 priceDetailStrings 会把空对象渲染成 "0" 而非 "-"。
	if len(b) == 0 || string(b) == "null" {
		return nil, nil
	}
	var c ConvertedPrice
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
