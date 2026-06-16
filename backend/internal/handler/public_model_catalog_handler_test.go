package handler

import (
	"testing"

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
