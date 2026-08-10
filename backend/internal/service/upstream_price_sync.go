package service

import (
	"math"
	"strings"
	"time"
)

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
func DiffPricing(upstream map[string]ConvertedPrice, upstreamPlatforms map[string]string, local []ChannelModelPricing, channelID int64) []PriceChangeItemDraft {
	type key struct{ platform, model string }
	localIdx := map[key]*ChannelModelPricing{}
	for i := range local {
		p := &local[i]
		for _, m := range p.Models {
			localIdx[key{p.Platform, m}] = p
		}
	}
	seen := map[key]bool{}
	var drafts []PriceChangeItemDraft
	for name, up := range upstream {
		plat := upstreamPlatforms[name]
		k := key{plat, name}
		seen[k] = true
		lp := localIdx[k]
		if lp == nil {
			upCopy := up
			drafts = append(drafts, PriceChangeItemDraft{Kind: ItemKindModelAdded, Platform: plat, ModelName: name, Upstream: &upCopy})
			continue
		}
		localPrice := channelPricingToConverted(lp)
		if !convertedEqual(&up, localPrice) {
			upCopy := up
			drafts = append(drafts, PriceChangeItemDraft{Kind: ItemKindModelPrice, Platform: plat, ModelName: name, Upstream: &upCopy, Local: localPrice})
		}
	}
	// 本地有、上游无
	for k, lp := range localIdx {
		if !seen[k] {
			localPrice := channelPricingToConverted(lp)
			drafts = append(drafts, PriceChangeItemDraft{Kind: ItemKindModelRemoved, Platform: k.platform, ModelName: k.model, Local: localPrice})
		}
	}
	return drafts
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
