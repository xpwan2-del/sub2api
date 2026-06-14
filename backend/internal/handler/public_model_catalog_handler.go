package handler

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const publicModelCatalogCacheTTL = 120 * time.Second
const publicModelCatalogRateWindow = time.Minute
const publicModelCatalogRateLimit = 120

// PublicModelCatalogHandler exposes a public, read-only model catalog.
//
// The DTO below is intentionally narrow. It does not expose channel names,
// channel IDs, account data, upstream URLs, routing weights, balances, or
// operational metrics.
type PublicModelCatalogHandler struct {
	channelService *service.ChannelService
	cacheMu        sync.RWMutex
	cachedAt       time.Time
	cachedCatalog  []publicModelCatalogItem
	rateMu         sync.Mutex
	rateBuckets    map[string]publicModelCatalogRateBucket
}

func NewPublicModelCatalogHandler(channelService *service.ChannelService) *PublicModelCatalogHandler {
	return &PublicModelCatalogHandler{
		channelService: channelService,
		rateBuckets:    make(map[string]publicModelCatalogRateBucket),
	}
}

type publicModelCatalogItem struct {
	Name         string              `json:"name"`
	Provider     string              `json:"provider"`
	Platform     string              `json:"platform"`
	Status       string              `json:"status"`
	Description  string              `json:"description"`
	Capabilities []string            `json:"capabilities"`
	Pricing      *publicModelPricing `json:"pricing"`
}

type publicModelPricing struct {
	BillingMode      string                     `json:"billing_mode"`
	InputPrice       *float64                   `json:"input_price"`
	OutputPrice      *float64                   `json:"output_price"`
	CacheWritePrice  *float64                   `json:"cache_write_price"`
	CacheReadPrice   *float64                   `json:"cache_read_price"`
	ImageOutputPrice *float64                   `json:"image_output_price"`
	PerRequestPrice  *float64                   `json:"per_request_price"`
	Intervals        []publicPricingIntervalDTO `json:"intervals"`
}

type publicPricingIntervalDTO struct {
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       *int     `json:"max_tokens"`
	TierLabel       string   `json:"tier_label,omitempty"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	PerRequestPrice *float64 `json:"per_request_price"`
}

type publicModelCatalogRateBucket struct {
	windowStart time.Time
	count       int
}

// List returns the public catalog.
// GET /api/v1/public/models/catalog
func (h *PublicModelCatalogHandler) List(c *gin.Context) {
	if !h.allowRequest(c.ClientIP()) {
		c.Header("Retry-After", "60")
		response.Error(c, http.StatusTooManyRequests, "Too many requests")
		return
	}

	if cached, ok := h.cached(); ok {
		response.Success(c, cached)
		return
	}

	channels, err := h.channelService.ListAvailable(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	catalog := buildPublicModelCatalog(channels)
	h.storeCache(catalog)

	response.Success(c, catalog)
}

func (h *PublicModelCatalogHandler) allowRequest(clientIP string) bool {
	now := time.Now()
	key := strings.TrimSpace(clientIP)
	if key == "" {
		key = "unknown"
	}

	h.rateMu.Lock()
	defer h.rateMu.Unlock()

	for bucketKey, bucket := range h.rateBuckets {
		if now.Sub(bucket.windowStart) > publicModelCatalogRateWindow*2 {
			delete(h.rateBuckets, bucketKey)
		}
	}

	bucket := h.rateBuckets[key]
	if bucket.windowStart.IsZero() || now.Sub(bucket.windowStart) >= publicModelCatalogRateWindow {
		h.rateBuckets[key] = publicModelCatalogRateBucket{windowStart: now, count: 1}
		return true
	}
	if bucket.count >= publicModelCatalogRateLimit {
		return false
	}
	bucket.count++
	h.rateBuckets[key] = bucket
	return true
}

func (h *PublicModelCatalogHandler) cached() ([]publicModelCatalogItem, bool) {
	h.cacheMu.RLock()
	defer h.cacheMu.RUnlock()

	if time.Since(h.cachedAt) > publicModelCatalogCacheTTL || h.cachedCatalog == nil {
		return nil, false
	}
	return copyPublicCatalog(h.cachedCatalog), true
}

func (h *PublicModelCatalogHandler) storeCache(catalog []publicModelCatalogItem) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	h.cachedAt = time.Now()
	h.cachedCatalog = copyPublicCatalog(catalog)
}

func buildPublicModelCatalog(channels []service.AvailableChannel) []publicModelCatalogItem {
	byModel := make(map[string]publicModelCatalogItem)

	for _, ch := range channels {
		if ch.Status != service.StatusActive {
			continue
		}

		for _, model := range ch.SupportedModels {
			if strings.TrimSpace(model.Name) == "" || strings.TrimSpace(model.Platform) == "" {
				continue
			}

			item := publicModelCatalogItem{
				Name:         model.Name,
				Provider:     providerLabel(model.Platform),
				Platform:     model.Platform,
				Status:       "available",
				Description:  publicModelDescription(model.Name, model.Platform, model.Pricing),
				Capabilities: publicModelCapabilities(model.Name, model.Platform, model.Pricing),
				Pricing:      toPublicPricing(model.Pricing),
			}

			key := strings.ToLower(model.Platform) + "\x00" + strings.ToLower(model.Name)
			existing, ok := byModel[key]
			if !ok || preferPublicPricing(item.Pricing, existing.Pricing) {
				byModel[key] = item
			}
		}
	}

	out := make([]publicModelCatalogItem, 0, len(byModel))
	for _, item := range byModel {
		out = append(out, item)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		if out[i].Platform != out[j].Platform {
			return out[i].Platform < out[j].Platform
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})

	return out
}

func toPublicPricing(p *service.ChannelModelPricing) *publicModelPricing {
	if p == nil {
		return nil
	}

	intervals := make([]publicPricingIntervalDTO, 0, len(p.Intervals))
	for _, iv := range p.Intervals {
		intervals = append(intervals, publicPricingIntervalDTO{
			MinTokens:       iv.MinTokens,
			MaxTokens:       iv.MaxTokens,
			TierLabel:       iv.TierLabel,
			InputPrice:      iv.InputPrice,
			OutputPrice:     iv.OutputPrice,
			CacheWritePrice: iv.CacheWritePrice,
			CacheReadPrice:  iv.CacheReadPrice,
			PerRequestPrice: iv.PerRequestPrice,
		})
	}

	billingMode := string(p.BillingMode)
	if billingMode == "" {
		billingMode = string(service.BillingModeToken)
	}

	return &publicModelPricing{
		BillingMode:      billingMode,
		InputPrice:       p.InputPrice,
		OutputPrice:      p.OutputPrice,
		CacheWritePrice:  p.CacheWritePrice,
		CacheReadPrice:   p.CacheReadPrice,
		ImageOutputPrice: p.ImageOutputPrice,
		PerRequestPrice:  p.PerRequestPrice,
		Intervals:        intervals,
	}
}

func preferPublicPricing(next, current *publicModelPricing) bool {
	if current == nil {
		return next != nil
	}
	if next == nil {
		return false
	}
	return publicPricingScore(next) < publicPricingScore(current)
}

func publicPricingScore(p *publicModelPricing) float64 {
	if p == nil {
		return math.Inf(1)
	}

	values := []*float64{
		p.InputPrice,
		p.OutputPrice,
		p.CacheWritePrice,
		p.CacheReadPrice,
		p.ImageOutputPrice,
		p.PerRequestPrice,
	}

	total := 0.0
	seen := false
	for _, v := range values {
		if v == nil {
			continue
		}
		total += *v
		seen = true
	}
	if !seen {
		return math.Inf(1)
	}
	return total
}

func copyPublicCatalog(src []publicModelCatalogItem) []publicModelCatalogItem {
	out := make([]publicModelCatalogItem, len(src))
	copy(out, src)
	for i := range out {
		out[i].Capabilities = append([]string(nil), src[i].Capabilities...)
	}
	return out
}

func providerLabel(platform string) string {
	return strings.TrimSpace(platform)
}

func publicModelCapabilities(name, platform string, pricing *service.ChannelModelPricing) []string {
	text := strings.ToLower(strings.TrimSpace(name) + " " + strings.TrimSpace(platform))
	capabilities := make([]string, 0, len(capabilityOrderForPublicCatalog()))
	add := func(value string) {
		for _, existing := range capabilities {
			if existing == value {
				return
			}
		}
		capabilities = append(capabilities, value)
	}

	if containsAny(text, "o1", "o3", "o4", "r1", "reason", "think", "deepseek", "sonnet", "opus", "grok", "gemini-2.5", "gemini-3", "gpt-5") {
		add("reasoning")
	}
	if containsAny(text, "code", "coder", "claude", "sonnet", "gpt", "deepseek", "qwen", "glm", "kimi") {
		add("coding")
	}
	if containsAny(text, "long", "context", "128k", "200k", "1m", "gemini", "claude", "kimi", "qwen") {
		add("longContext")
	}
	if containsAny(text, "4o", "omni", "vision", "image", "video", "grok", "gemini", "claude", "sora", "veo", "kling", "wan", "hailuo", "seedream", "seedance") {
		add("multimodal")
	}
	if containsAny(text, "flash", "mini", "haiku", "turbo", "fast", "lite") {
		add("fast")
	}
	if containsAny(text, "mini", "flash", "haiku", "lite", "cheap") || publicPricingScore(toPublicPricing(pricing)) <= 0.000002 {
		add("lowCost")
	}

	sort.SliceStable(capabilities, func(i, j int) bool {
		return capabilityRank(capabilities[i]) < capabilityRank(capabilities[j])
	})
	return capabilities
}

func publicModelDescription(name, platform string, pricing *service.ChannelModelPricing) string {
	capabilities := publicModelCapabilities(name, platform, pricing)
	if len(capabilities) == 0 {
		return "适合通过 TOP-AI 网关调用的通用模型。"
	}

	labels := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		switch capability {
		case "reasoning":
			labels = append(labels, "推理")
		case "coding":
			labels = append(labels, "编程")
		case "longContext":
			labels = append(labels, "长上下文")
		case "lowCost":
			labels = append(labels, "低成本")
		case "multimodal":
			labels = append(labels, "多模态")
		case "fast":
			labels = append(labels, "快速响应")
		}
	}
	if len(labels) == 0 {
		return "适合通过 TOP-AI 网关调用的通用模型。"
	}
	if len(labels) > 3 {
		labels = labels[:3]
	}
	return "适合" + strings.Join(labels, "、") + "场景。"
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}

func capabilityOrderForPublicCatalog() []string {
	return []string{"reasoning", "coding", "longContext", "lowCost", "multimodal", "fast"}
}

func capabilityRank(value string) int {
	for i, item := range capabilityOrderForPublicCatalog() {
		if item == value {
			return i
		}
	}
	return 999
}
