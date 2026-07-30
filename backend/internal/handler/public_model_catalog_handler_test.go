package handler

import (
	"context"
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

// fakeModelCatalogSvc 记录收到的 CatalogItem 并返回预设结果，用于验证 handler 的 merge 接入。
type fakeModelCatalogSvc struct {
	received []service.CatalogItem
	merged   []service.CatalogItem
	err      error
}

func (f *fakeModelCatalogSvc) MergeDisplayConfig(_ context.Context, items []service.CatalogItem) ([]service.CatalogItem, error) {
	f.received = items
	if f.err != nil {
		return nil, f.err
	}
	return f.merged, nil
}
func (f *fakeModelCatalogSvc) EnsureFirstSeen(_ context.Context, _ []service.ModelKey) error {
	return nil
}
func (f *fakeModelCatalogSvc) ListAllForAdmin(_ context.Context) ([]service.AdminCatalogConfig, error) {
	return nil, nil
}
func (f *fakeModelCatalogSvc) BatchSave(_ context.Context, _ []service.AdminCatalogConfig) error {
	return nil
}
func (f *fakeModelCatalogSvc) IsEnabled(_ context.Context) bool { return true }

// TestMergeCatalogNilSvcDegradesAndKeepsEmptyTags 验证未注入服务时降级返回原 catalog，
// 且 Tags 为非 nil 空切片（避免 JSON null）。
func TestMergeCatalogNilSvcDegradesAndKeepsEmptyTags(t *testing.T) {
	h := &PublicModelCatalogHandler{}
	catalog := buildPublicModelCatalog([]service.AvailableChannel{{
		Status: service.StatusActive,
		SupportedModels: []service.SupportedModel{{
			Name:     "gpt-4o",
			Platform: service.PlatformOpenAI,
		}},
	}})

	out := h.mergeCatalog(context.Background(), catalog)

	require.Len(t, out, 1)
	require.Equal(t, []string{}, out[0].Tags, "Tags must be non-nil empty slice to avoid JSON null")
}

// TestMergeCatalogMapsCapabilitiesIntoCatalogItem 是关键回归守卫：
// handler 必须把 publicModelCatalogItem.Capabilities 映射进 CatalogItem.Capabilities，
// 否则 service 合并 tags 时会丢掉自动推断的能力标签（multimodal/reasoning 等）。
func TestMergeCatalogMapsCapabilitiesIntoCatalogItem(t *testing.T) {
	catalog := buildPublicModelCatalog([]service.AvailableChannel{{
		Status: service.StatusActive,
		SupportedModels: []service.SupportedModel{{
			Name:     "claude-opus-4",
			Platform: service.PlatformAnthropic,
		}},
	}})
	require.NotEmpty(t, catalog[0].Capabilities, "precondition: auto capabilities inferred")

	// fake 模拟真实 mergeDisplay：把收到的 Capabilities 折进 Display.Tags 回传。
	svc := &fakeModelCatalogSvc{}
	svc.merged = withDisplayTags(catalogToCatalogItems(t, catalog))
	h := &PublicModelCatalogHandler{modelCatalogSvc: svc}

	out := h.mergeCatalog(context.Background(), catalog)

	require.Len(t, svc.received, 1)
	require.Equal(t, catalog[0].Capabilities, svc.received[0].Capabilities,
		"Capabilities must be forwarded into CatalogItem so merge can fold them into tags")
	require.Len(t, out, 1)
	require.Equal(t, catalog[0].Capabilities, out[0].Tags,
		"Display.Tags (含能力标签) 必须回填到响应项的 Tags")
}

// withDisplayTags 给每个 CatalogItem 填充 Display.Tags = 自身 Capabilities，模拟真实 service 合并。
func withDisplayTags(items []service.CatalogItem) []service.CatalogItem {
	for i := range items {
		caps := append([]string(nil), items[i].Capabilities...)
		items[i].Display = &service.CatalogDisplayInfo{Tags: caps}
	}
	return items
}

// TestMergeCatalogAppliesDisplayFieldsAndFiltersHidden 验证合并回填 pinned/sort_weight/tags/
// is_new/featured，并按 hidden 过滤掉模型，同时保留 handler 自身承载的 pricing。
func TestMergeCatalogAppliesDisplayFieldsAndFiltersHidden(t *testing.T) {
	cheap := 0.000001
	catalog := buildPublicModelCatalog([]service.AvailableChannel{{
		Status: service.StatusActive,
		SupportedModels: []service.SupportedModel{
			{Name: "gpt-4o", Platform: service.PlatformOpenAI, Pricing: &service.ChannelModelPricing{InputPrice: &cheap}},
			{Name: "hidden-model", Platform: service.PlatformOpenAI},
		},
	}})

	// 模拟 service 返回：过滤掉 hidden-model，给 gpt-4o 回填运营展示字段。
	merged := []service.CatalogItem{}
	for _, it := range catalogToCatalogItems(t, catalog) {
		if it.ModelName == "hidden-model" {
			continue
		}
		it.Display = &service.CatalogDisplayInfo{
			Pinned:     true,
			SortWeight: 7,
			Tags:       []string{"reasoning", "official"},
			IsNew:      true,
			Featured:   true,
		}
		merged = append(merged, it)
	}
	svc := &fakeModelCatalogSvc{merged: merged}
	h := &PublicModelCatalogHandler{modelCatalogSvc: svc}

	out := h.mergeCatalog(context.Background(), catalog)

	require.Len(t, out, 1, "hidden model must be filtered out")
	require.Equal(t, "gpt-4o", out[0].Name)
	require.True(t, out[0].Pinned)
	require.Equal(t, 7, out[0].SortWeight)
	require.True(t, out[0].IsNew)
	require.True(t, out[0].Featured)
	require.Equal(t, []string{"reasoning", "official"}, out[0].Tags)
	require.NotNil(t, out[0].Pricing, "handler-owned pricing must be preserved after merge")
	require.Equal(t, cheap, *out[0].Pricing.InputPrice)
}

func catalogToCatalogItems(t *testing.T, catalog []publicModelCatalogItem) []service.CatalogItem {
	t.Helper()
	out := make([]service.CatalogItem, len(catalog))
	for i := range catalog {
		out[i] = service.CatalogItem{
			Platform:     catalog[i].Platform,
			ModelName:    catalog[i].Name,
			Name:         catalog[i].Name,
			Capabilities: catalog[i].Capabilities,
		}
	}
	return out
}

// TestPublicModelCatalogInvalidateCacheClearsCachedEntry 验证 InvalidateCache 立即清除内存缓存：
// 管理员保存运营配置（推荐/精选/置顶/隐藏）后必须能让首页下次请求重新构建目录，
// 而非继续吃 publicModelCatalogCacheTTL（120s）的自然过期——否则会出现"管理页已取消推荐、
// 首页最长 120s 仍显示推荐标签"的不一致。
func TestPublicModelCatalogInvalidateCacheClearsCachedEntry(t *testing.T) {
	h := &PublicModelCatalogHandler{}
	h.storeCache([]publicModelCatalogItem{
		{Name: "gpt-4o", Platform: service.PlatformOpenAI, Tags: []string{}},
	})

	// 前置：缓存命中
	cached, ok := h.cached()
	require.True(t, ok, "precondition: cache should hit after storeCache")
	require.Len(t, cached, 1)

	h.InvalidateCache()

	cached, ok = h.cached()
	require.False(t, ok, "after InvalidateCache, cached() must miss")
	require.Nil(t, cached, "cached entry must be cleared")

	// 失效后可重新填充（下次请求重新构建）
	h.storeCache([]publicModelCatalogItem{
		{Name: "claude-opus-4", Platform: service.PlatformAnthropic, Tags: []string{}},
	})
	cached, ok = h.cached()
	require.True(t, ok, "cache should hit again after re-store")
	require.Len(t, cached, 1)
	require.Equal(t, "claude-opus-4", cached[0].Name)
}
