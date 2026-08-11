package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

// ErrRequestNotCloseable 审批批次无法关闭(不存在、或状态非 open/partially_applied、或仍有 pending 条目)。
var ErrRequestNotCloseable = errors.New("price change request not closeable")

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
type PriceChangeItem struct {
	ID                int64               `json:"id"`
	RequestID         int64               `json:"request_id"`
	Kind              PriceChangeItemKind `json:"kind"`
	Platform          string              `json:"platform"`
	ModelName         string              `json:"model_name"`
	TargetChannelID   int64               `json:"target_channel_id"`
	UpstreamRaw       map[string]any      `json:"upstream_raw"`
	UpstreamConverted *ConvertedPrice     `json:"upstream_converted"`
	LocalCurrent      *ConvertedPrice     `json:"local_current"`
	ApplyValue        *ConvertedPrice     `json:"apply_value"`
	Status            string              `json:"status"` // pending/approved/rejected/ignored/applied/failed/no_change
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
	// 第一遍:按渠道添加顺序遍历本地已有模型 → 价格变更 / 无变化 / 移除。
	matched := map[key]bool{} // 上游 key 中已被本地认领的(平台,模型)
	for _, k := range localOrder {
		lp := localIdx[k]
		localPrice := channelPricingToConverted(lp)
		uk, hasUp := upstreamKey[k.model]
		if hasUp && uk == k {
			matched[uk] = true
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
	for name, uk := range upstreamKey {
		if !matched[uk] {
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

func floatEq(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return math.Abs(*a-*b) <= priceTolerance
}

// UpstreamSourceConfig 上游 new-api 同步源配置。APIKey/DashboardToken 在内存中
// 为明文,落库时由 repository 层加密存储。
type UpstreamSourceConfig struct {
	ID                   int64                 `json:"id"`
	Name                 string                `json:"name"`
	BaseURL              string                `json:"base_url"`
	APIKey               string                `json:"api_key"`         // 内存明文;落库加密
	DashboardToken       string                `json:"dashboard_token"` // 内存明文;落库加密(余额查询用)
	DashboardAuthMode    string                `json:"dashboard_auth_mode"`
	DashboardUserID      *int64                `json:"dashboard_user_id"`
	ProxyID              *int64                `json:"proxy_id"`
	TargetChannelID      int64                 `json:"target_channel_id"`
	Enabled              bool                  `json:"enabled"`
	BasePricePer1k       float64               `json:"base_price_per_1k"`
	PricingSource        UpstreamPricingSource `json:"pricing_source"`
	SyncModelPrice       bool                  `json:"sync_model_price"`
	SyncGroupRatio       bool                  `json:"sync_group_ratio"`
	GroupMapping         map[string]int64      `json:"group_mapping"`
	TargetUpstreamGroup  string                `json:"target_upstream_group"` // 上游 group_ratio key;空=不过滤(全量同步)
	BalanceThresholdUSD  *float64              `json:"balance_threshold_usd"`
	LastBalanceQuota     *int64                `json:"last_balance_quota"`
	LastUsedQuota        *int64                `json:"last_used_quota"`
	LastBalanceUSD       *float64              `json:"last_balance_usd"`
	LastBalanceAt        *time.Time            `json:"last_balance_at"`
	LastBalanceCheckedAt *time.Time            `json:"last_balance_checked_at"`
	LastBalanceError     string                `json:"last_balance_error"`
	BalanceStatus        string                `json:"balance_status"`
	LastSyncAt           *time.Time            `json:"last_sync_at"`
	LastPricingVersion   string                `json:"last_pricing_version"`
	LastError            string                `json:"last_error"`
	CreatedAt            time.Time             `json:"created_at"`
	UpdatedAt            time.Time             `json:"updated_at"`
	ResetBalanceSnapshot bool                  `json:"-"`
}

// RequestFilter 审批批次列表过滤 + 分页参数。
type RequestFilter struct {
	SourceConfigID *int64
	Status         string
	Page           int
	PageSize       int
}

// UpstreamPriceSyncRepository 上游定价同步的持久化接口(config + request/items CRUD)。
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

	CreateRequest(ctx context.Context, req *PriceChangeRequest, items []PriceChangeItem) error
	GetRequest(ctx context.Context, id int64) (*PriceChangeRequest, error)
	ListRequests(ctx context.Context, f RequestFilter) ([]PriceChangeRequest, int64, error)
	ListItems(ctx context.Context, requestID int64) ([]PriceChangeItem, error)
	GetItem(ctx context.Context, id int64) (*PriceChangeItem, error)
	UpdateItemStatus(ctx context.Context, id int64, status string, reviewerID int64, note string, appliedAt *time.Time) error
	ExpireOpenRequests(ctx context.Context, configID int64) (int, error)
	CloseRequest(ctx context.Context, requestID int64) error
	UpdateRequestStatus(ctx context.Context, requestID int64, status string, summary map[string]int) error
}

// MarshalConverted / UnmarshalConverted — JSONB 落库辅助。
func MarshalConverted(c *ConvertedPrice) ([]byte, error) { return json.Marshal(c) }
func UnmarshalConverted(b []byte) (*ConvertedPrice, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var c ConvertedPrice
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
