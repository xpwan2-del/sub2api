package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestIsOpenAIVideoUsage(t *testing.T) {
	groupID := int64(7)
	const genericModel = "kling-video"

	// build 构造一个 OpenAIGatewayService，其渠道定价为 genericModel 配置指定 mode。
	// mode 传空表示该模型无渠道定价。
	build := func(mode BillingMode, intervals []PricingInterval) *OpenAIGatewayService {
		cache := newEmptyChannelCache()
		if mode != "" {
			cache.pricingByGroupModel[channelModelKey{groupID: groupID, model: genericModel}] = &ChannelModelPricing{
				BillingMode: mode,
				Intervals:   intervals,
			}
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
	price := 0.05
	tiers := []PricingInterval{{TierLabel: "1080p", PerRequestPrice: &price}}

	cases := []struct {
		name   string
		svc    *OpenAIGatewayService
		model  string
		result *OpenAIForwardResult
		want   bool
	}{
		{"generic + per_second", build(BillingModePerSecond, tiers), genericModel, &OpenAIForwardResult{VideoCount: 1}, true},
		{"generic + video mode", build(BillingModeVideo, tiers), genericModel, &OpenAIForwardResult{VideoCount: 1}, true},
		{"generic + per_request", build(BillingModePerRequest, tiers), genericModel, &OpenAIForwardResult{VideoCount: 1}, true},
		{"generic + image mode", build(BillingModeImage, tiers), genericModel, &OpenAIForwardResult{VideoCount: 1}, true},
		{"generic + token mode (not video)", build(BillingModeToken, nil), genericModel, &OpenAIForwardResult{VideoCount: 1}, false},
		{"generic + no channel pricing", build("", nil), genericModel, &OpenAIForwardResult{VideoCount: 1}, false},
		{"generic + per_second but VideoCount=0", build(BillingModePerSecond, tiers), genericModel, &OpenAIForwardResult{VideoCount: 0}, false},
		{"grok-imagine-video regardless of pricing", build("", nil), "grok-imagine-video", &OpenAIForwardResult{VideoCount: 1}, true},
		{"grok via result.BillingModel", build("", nil), "other", &OpenAIForwardResult{VideoCount: 1, BillingModel: "grok-imagine-video-1.5"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.svc.isOpenAIVideoUsage(context.Background(), tc.result, []string{tc.model}, apiKey)
			require.Equal(t, tc.want, got)
		})
	}
}
