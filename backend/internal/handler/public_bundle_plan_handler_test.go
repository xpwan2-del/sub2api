package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// fakeBundlePlanLister 捕获 ListForSale 调用次数，返回预设套餐/错误，隔离测试 handler。
type fakeBundlePlanLister struct {
	plans []service.BundlePlan
	err   error
	calls int
}

func (f *fakeBundlePlanLister) ListForSale(context.Context) ([]service.BundlePlan, error) {
	f.calls++
	return f.plans, f.err
}

// fakeModelCatalogEnabledSvc 是可开关的 ModelCatalogService 桩，控制 IsEnabled 返回值。
type fakeModelCatalogEnabledSvc struct {
	enabled bool
}

func (f *fakeModelCatalogEnabledSvc) MergeDisplayConfig(context.Context, []service.CatalogItem) ([]service.CatalogItem, error) {
	return nil, nil
}
func (f *fakeModelCatalogEnabledSvc) EnsureFirstSeen(context.Context, []service.ModelKey) error {
	return nil
}
func (f *fakeModelCatalogEnabledSvc) ListAllForAdmin(context.Context) ([]service.AdminCatalogConfig, error) {
	return nil, nil
}
func (f *fakeModelCatalogEnabledSvc) BatchSave(context.Context, []service.AdminCatalogConfig) error {
	return nil
}
func (f *fakeModelCatalogEnabledSvc) IsEnabled(context.Context) bool { return f.enabled }

// --- dedupPlatforms 纯函数 ---

func TestDedupPlatformsAggregatesGroupPlatformDeduped(t *testing.T) {
	quotas := []service.BundlePlanGroupQuota{
		{GroupID: 100, GroupPlatform: "openai"},
		{GroupID: 101, GroupPlatform: "anthropic"},
		{GroupID: 102, GroupPlatform: "gemini"},
		{GroupID: 100, GroupPlatform: "openai"}, // 重复 platform
		{GroupID: 103, GroupPlatform: ""},       // 空白跳过
	}

	got := dedupPlatforms(quotas)

	require.Equal(t, []string{"openai", "anthropic", "gemini"}, got, "去重保序，跳过空白")
}

func TestDedupPlatformsEmptyReturnsEmptySlice(t *testing.T) {
	require.Equal(t, []string{}, dedupPlatforms(nil), "nil quotas → 非 nil 空切片")
	require.Equal(t, []string{}, dedupPlatforms([]service.BundlePlanGroupQuota{}))
}

// --- toPublicBundlePlans 纯函数 ---

func TestToPublicBundlePlansMapsFieldsAndPreservesOrder(t *testing.T) {
	plans := []service.BundlePlan{
		{
			ID: 1, Name: "Starter", Tier: "starter", Description: "入门套餐",
			Price: 9.9, OriginalPrice: 19.9, Currency: "CNY", ValidityDays: 30,
			Features: []string{"50万 tokens"}, SortOrder: 10,
			ConcurrencyLimit: 5, RPMLimit: 20, // 敏感：不应出现在 DTO
			GroupQuotas: []service.BundlePlanGroupQuota{
				{GroupID: 100, GroupName: "g-openai", GroupPlatform: "openai", MonthlyLimitUSD: 10, QuotaScope: "platform"},
				{GroupID: 101, GroupName: "g-anthropic", GroupPlatform: "anthropic", ModelPattern: "claude-*"},
			},
		},
		{
			ID: 2, Name: "Pro", Tier: "pro", SortOrder: 20,
			GroupQuotas: []service.BundlePlanGroupQuota{
				{GroupPlatform: "gemini"},
				{GroupPlatform: "openai"},
			},
		},
	}

	out := toPublicBundlePlans(plans)

	require.Len(t, out, 2)
	// 顺序保持（按 ListForSale 返回的 sort_order）
	require.Equal(t, "Starter", out[0].Name)
	require.Equal(t, "Pro", out[1].Name)
	// 展示字段映射
	require.Equal(t, "starter", out[0].Tier)
	require.InDelta(t, 9.9, out[0].Price, 1e-12)
	require.InDelta(t, 19.9, out[0].OriginalPrice, 1e-12)
	require.Equal(t, "CNY", out[0].Currency)
	require.Equal(t, 30, out[0].ValidityDays)
	require.Equal(t, []string{"50万 tokens"}, out[0].Features)
	require.Equal(t, 10, out[0].SortOrder)
	// Platforms 聚合
	require.Equal(t, []string{"openai", "anthropic"}, out[0].Platforms)
	require.Equal(t, []string{"gemini", "openai"}, out[1].Platforms)
}

func TestToPublicBundlePlansEmptyReturnsEmptySlice(t *testing.T) {
	out := toPublicBundlePlans(nil)
	require.NotNil(t, out, "nil plans → 非 nil 空切片（JSON [] 非 null）")
	require.Empty(t, out)
}

func TestToPublicBundlePlansFeaturesNeverNil(t *testing.T) {
	out := toPublicBundlePlans([]service.BundlePlan{{Name: "X", Features: nil}})
	require.Equal(t, []string{}, out[0].Features, "nil Features 归一为空切片，避免 JSON null")
}

func TestToPublicBundlePlansStripsSensitiveFields(t *testing.T) {
	plans := []service.BundlePlan{{
		Name: "Pro", ConcurrencyLimit: 8, RPMLimit: 30,
		GroupQuotas: []service.BundlePlanGroupQuota{
			{GroupID: 7, GroupName: "secret-group", GroupPlatform: "openai",
				DailyLimitUSD: 1, MonthlyLimitUSD: 30, QuotaScope: "model", ModelPattern: "gpt-*"},
		},
	}}

	data, err := json.Marshal(toPublicBundlePlans(plans))
	require.NoError(t, err)
	body := string(data)

	// 敏感字段不得出现在公开 DTO 的 JSON 中
	for _, bad := range []string{
		`"group_id"`, `"group_name"`, `"daily_limit_usd"`, `"monthly_limit_usd"`,
		`"weekly_limit_usd"`, `"concurrency_limit"`, `"rpm_limit"`,
		`"quota_scope"`, `"model_pattern"`, `"for_sale"`, `"status"`, `"id"`,
	} {
		require.NotContains(t, body, bad, "敏感字段 %s 不得泄漏到公开 DTO", bad)
	}
	// 聚合字段必须存在
	require.Contains(t, body, `"platforms":["openai"]`)
}

// --- handler.List（gin 端到端） ---

func TestPublicBundlePlanListDisabledReturnsEmptyAndDoesNotList(t *testing.T) {
	lister := &fakeBundlePlanLister{plans: []service.BundlePlan{{Name: "should-not-leak"}}}
	h := &PublicBundlePlanHandler{
		bundlePlanSvc:   lister,
		modelCatalogSvc: &fakeModelCatalogEnabledSvc{enabled: false},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/bundles/plans", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Zero(t, lister.calls, "开关关闭时不应调用 ListForSale")

	var resp struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "[]", string(resp.Data), "开关关闭 → 空数组（非 null）")
}

func TestPublicBundlePlanListReturnsMappedPlansOrderedBySortOrder(t *testing.T) {
	// 模拟 ListForSale 已按 sort_order 返回（active+for_sale 过滤由 service 负责）
	lister := &fakeBundlePlanLister{plans: []service.BundlePlan{
		{ID: 1, Name: "Starter", Tier: "starter", Price: 9.9, Currency: "CNY", ValidityDays: 30,
			Features: []string{"基础"}, SortOrder: 10,
			GroupQuotas: []service.BundlePlanGroupQuota{{GroupPlatform: "openai"}}},
		{ID: 2, Name: "Pro", Tier: "pro", Price: 49.9, SortOrder: 20,
			GroupQuotas: []service.BundlePlanGroupQuota{{GroupPlatform: "openai"}, {GroupPlatform: "anthropic"}}},
	}}
	h := &PublicBundlePlanHandler{
		bundlePlanSvc:   lister,
		modelCatalogSvc: &fakeModelCatalogEnabledSvc{enabled: true},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/bundles/plans", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, lister.calls)

	var resp struct {
		Code int                `json:"code"`
		Data []PublicBundlePlan `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data, 2)
	// 顺序保持（按 sort_order）
	require.Equal(t, "Starter", resp.Data[0].Name)
	require.Equal(t, "Pro", resp.Data[1].Name)
	require.Equal(t, []string{"openai"}, resp.Data[0].Platforms)
	require.Equal(t, []string{"openai", "anthropic"}, resp.Data[1].Platforms)
}

func TestPublicBundlePlanListEmptyReturnsEmptyArray(t *testing.T) {
	h := &PublicBundlePlanHandler{
		bundlePlanSvc:   &fakeBundlePlanLister{plans: nil}, // ListForSale → nil
		modelCatalogSvc: &fakeModelCatalogEnabledSvc{enabled: true},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/bundles/plans", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, "[]", string(resp.Data), "无在售套餐 → [] 非 null")
}

func TestPublicBundlePlanListErrorPropagatesAsNonSuccess(t *testing.T) {
	h := &PublicBundlePlanHandler{
		bundlePlanSvc:   &fakeBundlePlanLister{err: errFakeListForSale},
		modelCatalogSvc: &fakeModelCatalogEnabledSvc{enabled: true},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/public/bundles/plans", nil)

	h.List(c)

	require.NotEqual(t, http.StatusOK, recorder.Code, "service 错误应转为非 200 响应")
}

// errFakeListForSale 是用于错误路径测试的哨兵错误。
var errFakeListForSale = errors.New("fake list-for-sale failure")
