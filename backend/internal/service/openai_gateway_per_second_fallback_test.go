package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// TestCalculateOpenAIVideoCost_PerSecondFallbackToChannelDefault 验证 per_second 模式下：
// 档位命中用档位价；档位未命中回退到渠道配置的默认每秒价（父行 per_request_price），而非系统默认价表。
func TestCalculateOpenAIVideoCost_PerSecondFallbackToChannelDefault(t *testing.T) {
	groupID := int64(9)
	const model = "kling-video"
	defaultPrice := 0.08
	tierPrice := 0.10
	cache := newEmptyChannelCache()
	cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: model}] = &ChannelModelPricing{
		BillingMode:     BillingModePerSecond,
		PerRequestPrice: &defaultPrice,
		Intervals: []PricingInterval{{
			TierLabel:       "720p",
			PerRequestPrice: &tierPrice,
		}},
	}
	cache.channelByGroupID[groupID] = &Channel{ID: groupID, Status: StatusActive}
	cache.groupPlatform[groupID] = ""
	cache.loadedAt = time.Now()
	cs := &ChannelService{}
	cs.cache.Store(cache)
	bs := &BillingService{cfg: &config.Config{}, fallbackPrices: map[string]*ModelPricing{}}
	resolver := NewModelPricingResolver(cs, bs)
	svc := &OpenAIGatewayService{resolver: resolver, billingService: bs}
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, VideoRateIndependent: true}}

	t.Run("tier hit uses tier price", func(t *testing.T) {
		r := &OpenAIForwardResult{VideoCount: 1, VideoResolution: "720p"} // duration 默认 8
		c := svc.calculateOpenAIVideoCost(context.Background(), model, apiKey, r, 1.0)
		require.InDelta(t, 0.80, c.TotalCost, 1e-10) // 0.10 × 8
		require.Equal(t, string(BillingModePerSecond), c.BillingMode)
	})

	t.Run("tier miss falls back to channel default price", func(t *testing.T) {
		// 1080p 未配档 → 应回退渠道默认每秒价 0.08 × 10 = 0.80，而非系统默认价表（对通用模型通常为 0）
		r := &OpenAIForwardResult{VideoCount: 1, VideoResolution: "1080p", VideoDurationSeconds: 10}
		c := svc.calculateOpenAIVideoCost(context.Background(), model, apiKey, r, 1.0)
		require.InDelta(t, 0.80, c.TotalCost, 1e-10) // 0.08 × 10
		require.Equal(t, string(BillingModePerSecond), c.BillingMode)
	})
}

// TestCalculateOpenAIVideoCost_GenericVideoDurationNotCappedToXAILimit 验证通用视频模型
// 不受 xAI 15s 上游规格截断（防长视频少扣/套利），而 grok-imagine-video 系列仍受 15s 约束。
func TestCalculateOpenAIVideoCost_GenericVideoDurationNotCappedToXAILimit(t *testing.T) {
	groupID := int64(11)
	tierPrice := 0.10
	build := func(model string) *OpenAIGatewayService {
		cache := newEmptyChannelCache()
		cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: model}] = &ChannelModelPricing{
			BillingMode: BillingModePerSecond,
			Intervals: []PricingInterval{
				{TierLabel: "1080p", PerRequestPrice: &tierPrice},
				{TierLabel: "720p", PerRequestPrice: &tierPrice},
			},
		}
		cache.channelByGroupID[groupID] = &Channel{ID: groupID, Status: StatusActive}
		cache.groupPlatform[groupID] = ""
		cache.loadedAt = time.Now()
		cs := &ChannelService{}
		cs.cache.Store(cache)
		bs := &BillingService{cfg: &config.Config{}, fallbackPrices: map[string]*ModelPricing{}}
		return &OpenAIGatewayService{resolver: NewModelPricingResolver(cs, bs), billingService: bs}
	}
	apiKey := &APIKey{GroupID: &groupID, Group: &Group{ID: groupID, VideoRateIndependent: true}}

	t.Run("generic 60s billed in full (not capped to 15)", func(t *testing.T) {
		svc := build("kling-video")
		r := &OpenAIForwardResult{VideoCount: 1, VideoResolution: "1080p", VideoDurationSeconds: 60}
		c := svc.calculateOpenAIVideoCost(context.Background(), "kling-video", apiKey, r, 1.0)
		require.InDelta(t, 6.00, c.TotalCost, 1e-10) // 0.10 × 60
	})

	t.Run("grok 60s still capped to 15 (xAI upstream spec)", func(t *testing.T) {
		svc := build("grok-imagine-video")
		r := &OpenAIForwardResult{VideoCount: 1, VideoResolution: "720p", VideoDurationSeconds: 60}
		c := svc.calculateOpenAIVideoCost(context.Background(), "grok-imagine-video", apiKey, r, 1.0)
		require.InDelta(t, 1.50, c.TotalCost, 1e-10) // 0.10 × 15
	})
}
