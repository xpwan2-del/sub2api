package handler

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildPublicModelCatalogEmptyChannelsReturnEmptyCatalog(t *testing.T) {
	catalog := buildPublicModelCatalog(nil)
	require.Empty(t, catalog)
}

func TestBuildPublicModelCatalogDeduplicatesByPlatformAndModel(t *testing.T) {
	cheap := 0.000001
	expensive := 0.00001
	channels := []service.AvailableChannel{
		{
			Status: service.StatusActive,
			SupportedModels: []service.SupportedModel{
				{
					Name:     "gpt-4o-mini",
					Platform: service.PlatformOpenAI,
					Pricing: &service.ChannelModelPricing{
						BillingMode: service.BillingModeToken,
						InputPrice:  &expensive,
					},
				},
				{
					Name:     "gpt-4o-mini",
					Platform: service.PlatformOpenAI,
					Pricing: &service.ChannelModelPricing{
						BillingMode: service.BillingModeToken,
						InputPrice:  &cheap,
					},
				},
			},
		},
		{
			Status: "inactive",
			SupportedModels: []service.SupportedModel{
				{
					Name:     "hidden-model",
					Platform: service.PlatformOpenAI,
				},
			},
		},
	}

	catalog := buildPublicModelCatalog(channels)

	require.Len(t, catalog, 1)
	require.Equal(t, "gpt-4o-mini", catalog[0].Name)
	require.Equal(t, service.PlatformOpenAI, catalog[0].Platform)
	require.NotNil(t, catalog[0].Pricing)
	require.Equal(t, cheap, *catalog[0].Pricing.InputPrice)
}

func TestBuildPublicModelCatalogScalesPricesByGroupMultiplier(t *testing.T) {
	input := 0.00001
	output := 0.00002
	image := 0.05
	perRequest := 0.10
	channels := []service.AvailableChannel{
		{
			Status: service.StatusActive,
			Groups: []service.AvailableGroupRef{
				{
					Platform:             service.PlatformOpenAI,
					RateMultiplier:       0.4,
					ImageRateIndependent: true,
					ImageRateMultiplier:  2,
				},
			},
			SupportedModels: []service.SupportedModel{
				{
					Name:     "gpt-5.5",
					Platform: service.PlatformOpenAI,
					Pricing: &service.ChannelModelPricing{
						BillingMode: service.BillingModeToken,
						InputPrice:  &input,
						OutputPrice: &output,
						Intervals: []service.PricingInterval{
							{
								MinTokens:   272000,
								InputPrice:  &input,
								OutputPrice: &output,
							},
						},
					},
				},
				{
					Name:     "grok-imagine-image",
					Platform: service.PlatformOpenAI,
					Pricing: &service.ChannelModelPricing{
						BillingMode:      service.BillingModeImage,
						ImageOutputPrice: &image,
						PerRequestPrice:  &perRequest,
					},
				},
			},
		},
	}

	catalog := buildPublicModelCatalog(channels)

	require.Len(t, catalog, 2)
	byName := make(map[string]publicModelCatalogItem)
	for _, item := range catalog {
		byName[item.Name] = item
	}

	tokenPricing := byName["gpt-5.5"].Pricing
	require.NotNil(t, tokenPricing)
	require.InDelta(t, 0.000004, *tokenPricing.InputPrice, 1e-12)
	require.InDelta(t, 0.000008, *tokenPricing.OutputPrice, 1e-12)
	require.Len(t, tokenPricing.Intervals, 1)
	require.InDelta(t, 0.000004, *tokenPricing.Intervals[0].InputPrice, 1e-12)

	imagePricing := byName["grok-imagine-image"].Pricing
	require.NotNil(t, imagePricing)
	require.InDelta(t, 0.10, *imagePricing.ImageOutputPrice, 1e-12)
	require.InDelta(t, 0.20, *imagePricing.PerRequestPrice, 1e-12)
}

func TestBuildPublicModelHealthUsesCompactHistory(t *testing.T) {
	start := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	rate := 50.0
	buckets := map[time.Time]*service.OpsModelHealthBucket{
		start.Add(47 * time.Hour): {
			OpsHealthHistoryPoint: service.OpsHealthHistoryPoint{
				BucketStart:  start.Add(47 * time.Hour),
				RequestCount: 2,
				SuccessCount: 1,
				SuccessRate:  &rate,
			},
		},
	}

	health := buildPublicModelHealth(start, buckets)

	require.NotNil(t, health)
	require.Equal(t, string(service.OpsModelStatusFailed), health.Status)
	require.Equal(t, int64(2), health.RequestCount)
	require.NotNil(t, health.SuccessRate)
	require.InDelta(t, 50, *health.SuccessRate, 1e-12)
	require.Len(t, health.History, publicModelHealthBucketCount)
	require.Equal(t, "idle", health.History[0].Status)
	require.Equal(t, string(service.OpsModelStatusFailed), health.History[47].Status)
	require.Equal(t, int64(2), health.History[47].RequestCount)
}

func TestBuildPublicModelHealthNoTraffic(t *testing.T) {
	start := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)

	health := buildPublicModelHealth(start, nil)

	require.NotNil(t, health)
	require.Equal(t, string(service.OpsModelStatusNoRecentTraffic), health.Status)
	require.Equal(t, int64(0), health.RequestCount)
	require.Nil(t, health.SuccessRate)
	require.Len(t, health.History, publicModelHealthBucketCount)
	for _, point := range health.History {
		require.Equal(t, "idle", point.Status)
	}
}
