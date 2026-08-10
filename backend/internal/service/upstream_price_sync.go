package service

import "time"

// PricingSource 上游定价数据源选择
type UpstreamPricingSource string

const (
	PricingSourceAuto        UpstreamPricingSource = "auto"
	PricingSourceRatioConfig UpstreamPricingSource = "ratio_config"
	PricingSourcePricing     UpstreamPricingSource = "pricing"
)

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
type ConvertedPrice struct {
	BillingMode     BillingMode // token / per_request
	InputPrice      *float64
	OutputPrice     *float64
	CacheReadPrice  *float64
	CacheWritePrice *float64
	PerRequestPrice *float64
}

// PriceChangeItemKind 审批条目类型
type PriceChangeItemKind string

const (
	ItemKindModelPrice   PriceChangeItemKind = "model_price"
	ItemKindModelAdded   PriceChangeItemKind = "model_added"
	ItemKindModelRemoved PriceChangeItemKind = "model_removed"
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
	ID                     int64
	SourceConfigID         int64
	TriggerType            string // manual/scheduled
	Status                 string // open/partially_applied/closed/expired
	UpstreamPricingVersion string
	Summary                map[string]int
	CreatedBy              int64
	CreatedAt              time.Time
	ClosedAt               *time.Time
}

// PriceChangeItem 审批条目
type PriceChangeItem struct {
	ID                int64
	RequestID         int64
	Kind              PriceChangeItemKind
	Platform          string
	ModelName         string
	TargetChannelID   int64
	UpstreamRaw       map[string]any
	UpstreamConverted *ConvertedPrice
	LocalCurrent      *ConvertedPrice
	ApplyValue        *ConvertedPrice
	Status            string // pending/approved/rejected/ignored/applied/failed
	ReviewerID        int64
	ReviewNote        string
	ReviewedAt        *time.Time
	AppliedAt         *time.Time
	CreatedAt         time.Time
}
