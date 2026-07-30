package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// fakeAdminCatalogSvc 捕获 BatchSave 入参与 EnsureFirstSeen 模型键，返回预设的 ListAllForAdmin
// 结果与可选错误，用于隔离测试 AdminModelCatalogHandler 的 List/Update 行为。
type fakeAdminCatalogSvc struct {
	adminCfgs   []service.AdminCatalogConfig
	listErr     error
	saveErr     error
	saved       []service.AdminCatalogConfig
	seenKeys    []service.ModelKey
	ensureErr   error
	ensureCalls int
}

func (f *fakeAdminCatalogSvc) MergeDisplayConfig(context.Context, []service.CatalogItem) ([]service.CatalogItem, error) {
	return nil, nil
}
func (f *fakeAdminCatalogSvc) EnsureFirstSeen(_ context.Context, keys []service.ModelKey) error {
	f.ensureCalls++
	f.seenKeys = keys
	return f.ensureErr
}
func (f *fakeAdminCatalogSvc) ListAllForAdmin(context.Context) ([]service.AdminCatalogConfig, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.adminCfgs, nil
}
func (f *fakeAdminCatalogSvc) BatchSave(_ context.Context, cfgs []service.AdminCatalogConfig) error {
	f.saved = cfgs
	return f.saveErr
}
func (f *fakeAdminCatalogSvc) IsEnabled(context.Context) bool { return true }

// --- modelKeysFromChannels 纯函数 ---

func TestModelKeysFromChannelsEmptyReturnsEmptySlice(t *testing.T) {
	got := modelKeysFromChannels(nil)
	require.NotNil(t, got, "empty input must yield non-nil slice (JSON [] not null)")
	require.Empty(t, got)

	got = modelKeysFromChannels([]service.AvailableChannel{})
	require.NotNil(t, got)
	require.Empty(t, got)
}

func TestModelKeysFromChannelsDedupesAcrossActiveChannels(t *testing.T) {
	channels := []service.AvailableChannel{
		{Status: service.StatusActive, SupportedModels: []service.SupportedModel{
			{Name: "claude-opus-4", Platform: service.PlatformAnthropic},
			{Name: "gpt-4o", Platform: service.PlatformOpenAI},
		}},
		// 同模型重复出现于另一活跃渠道 → 去重
		{Status: service.StatusActive, SupportedModels: []service.SupportedModel{
			{Name: "Claude-Opus-4", Platform: service.PlatformAnthropic},
		}},
		// 非活跃渠道的模型不应纳入（与 public catalog 同源）
		{Status: service.StatusDisabled, SupportedModels: []service.SupportedModel{
			{Name: "disabled-model", Platform: service.PlatformOpenAI},
		}},
		// 空 platform/name 跳过
		{Status: service.StatusActive, SupportedModels: []service.SupportedModel{
			{Name: "", Platform: service.PlatformOpenAI},
			{Name: "no-platform", Platform: ""},
		}},
	}

	got := modelKeysFromChannels(channels)

	require.Len(t, got, 2, "dedup case-insensitive, skip inactive + empty")
	// 保序：按首次出现顺序
	require.Equal(t, "claude-opus-4", got[0].ModelName)
	require.Equal(t, service.PlatformAnthropic, got[0].Platform)
	require.Equal(t, "gpt-4o", got[1].ModelName)
}

// --- List ---

func TestAdminModelCatalogListEmptyReturnsEmptyArray(t *testing.T) {
	h := &AdminModelCatalogHandler{
		modelCatalogSvc: &fakeAdminCatalogSvc{adminCfgs: nil}, // ListAllForAdmin → nil
		// channelService 留 nil：currentModelKeys 返回 nil，EnsureFirstSeen no-op
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/catalog/config", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	// 关键：data 必须是 `[]`，不能是 `null`
	var resp struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, "[]", string(resp.Data),
		"empty list data must serialize to [] not null (前端无需判 null)")
}

func TestAdminModelCatalogListReturnsConfigsAndEnsuresFirstSeen(t *testing.T) {
	svc := &fakeAdminCatalogSvc{adminCfgs: []service.AdminCatalogConfig{
		{Platform: service.PlatformAnthropic, ModelName: "claude-opus-4", Pinned: true, Tags: []string{"featured"}},
		{Platform: service.PlatformOpenAI, ModelName: "gpt-4o", Hidden: true},
	}}
	h := &AdminModelCatalogHandler{modelCatalogSvc: svc}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/catalog/config", nil)

	h.List(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, svc.ensureCalls, "List must call EnsureFirstSeen (with nil keys when no channelService)")

	var resp struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	// data 是数组，含 hidden 行（管理页需可见）
	var arr []service.AdminCatalogConfig
	require.NoError(t, json.Unmarshal(resp.Data, &arr))
	require.Len(t, arr, 2)
	require.True(t, arr[0].Pinned)
	require.True(t, arr[1].Hidden, "admin list must include hidden models")
}

// TestAdminModelCatalogListSerializesSnakeCase 锁定线上契约：AdminCatalogConfig 的 json tag
// 必须是 snake_case，与 public catalog 字段名对齐，否则前端 (A9) 字段名对不上。
func TestAdminModelCatalogListSerializesSnakeCase(t *testing.T) {
	h := &AdminModelCatalogHandler{
		modelCatalogSvc: &fakeAdminCatalogSvc{adminCfgs: []service.AdminCatalogConfig{
			{Platform: "anthropic", ModelName: "claude-opus-4", Pinned: true, SortWeight: 3,
				CustomTags: []string{"official"}, Hidden: false, IsNew: true, Featured: true,
				Tags: []string{"official", "new", "featured"}},
		}},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/catalog/config", nil)

	h.List(c)

	body := recorder.Body.String()
	for _, key := range []string{`"platform"`, `"model_name"`, `"pinned"`, `"sort_weight"`, `"custom_tags"`, `"featured_until"`, `"hidden"`, `"first_seen_at"`, `"tags"`, `"is_new"`, `"featured"`} {
		require.Contains(t, body, key, "wire contract must use snake_case key %s", key)
	}
	// PascalCase（无 tag 时的默认）不得出现
	for _, bad := range []string{`"ModelName"`, `"SortWeight"`, `"IsNew"`, `"FirstSeenAt"`} {
		require.NotContains(t, body, bad, "PascalCase key %s must not leak into wire contract", bad)
	}
}

// --- Update ---

func TestAdminModelCatalogUpdateBindsAndSaves(t *testing.T) {
	svc := &fakeAdminCatalogSvc{}
	h := &AdminModelCatalogHandler{modelCatalogSvc: svc}

	body := `[` +
		`{"platform":"anthropic","model_name":"claude-opus-4","pinned":true,"sort_weight":7,"custom_tags":["official"],"featured_until":"2026-08-15T00:00:00Z","hidden":false},` +
		`{"platform":"openai","model_name":"gpt-4o","hidden":true}` +
		`]`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/catalog/config", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Update(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Len(t, svc.saved, 2, "BatchSave must receive both bound configs")
	require.Equal(t, "claude-opus-4", svc.saved[0].ModelName)
	require.True(t, svc.saved[0].Pinned)
	require.Equal(t, 7, svc.saved[0].SortWeight)
	require.Equal(t, []string{"official"}, svc.saved[0].CustomTags)
	require.NotNil(t, svc.saved[0].FeaturedUntil)
	require.Equal(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), *svc.saved[0].FeaturedUntil)
	require.True(t, svc.saved[1].Hidden)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Updated int `json:"updated"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 2, resp.Data.Updated)
}

func TestAdminModelCatalogUpdateRejectsInvalidJSON(t *testing.T) {
	h := &AdminModelCatalogHandler{modelCatalogSvc: &fakeAdminCatalogSvc{}}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/catalog/config", strings.NewReader("not-json"))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Update(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

// --- Update → 失效公开广场缓存 ---

// TestAdminModelCatalogUpdateInvalidatesPublicCacheOnSuccess 验证保存成功后必须触发公开广场
// 缓存失效回调，使首页下次请求读到最新运营配置（修复"管理页取消推荐后首页最长 120s 仍显示旧标签"）。
func TestAdminModelCatalogUpdateInvalidatesPublicCacheOnSuccess(t *testing.T) {
	svc := &fakeAdminCatalogSvc{}
	calls := 0
	h := &AdminModelCatalogHandler{
		modelCatalogSvc:       svc,
		invalidatePublicCache: func() { calls++ },
	}

	body := `[{"platform":"openai","model_name":"gpt-4o","featured":false,"custom_tags":[]}]`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/catalog/config", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Update(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, 1, calls, "cache invalidator must fire exactly once on successful save")
}

// TestAdminModelCatalogUpdateSkipsInvalidationOnSaveFailure 验证 BatchSave 失败时不触发失效：
// 写入未落库，不应让缓存提前失效而暴露一个"既非旧也非新"的中间态。
func TestAdminModelCatalogUpdateSkipsInvalidationOnSaveFailure(t *testing.T) {
	svc := &fakeAdminCatalogSvc{saveErr: errors.New("db down")}
	calls := 0
	h := &AdminModelCatalogHandler{
		modelCatalogSvc:       svc,
		invalidatePublicCache: func() { calls++ },
	}

	body := `[{"platform":"openai","model_name":"gpt-4o"}]`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/catalog/config", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Update(c)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, 0, calls, "must not invalidate cache when save failed")
}

// TestAdminModelCatalogUpdateWithoutInvalidatorIsNoop 验证未注入失效回调时 Update 不 panic
// （向后兼容：测试直接构造、或 wire 未接线时的安全降级）。
func TestAdminModelCatalogUpdateWithoutInvalidatorIsNoop(t *testing.T) {
	h := &AdminModelCatalogHandler{modelCatalogSvc: &fakeAdminCatalogSvc{}}

	body := `[{"platform":"openai","model_name":"gpt-4o"}]`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/catalog/config", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	require.NotPanics(t, func() { h.Update(c) })
	require.Equal(t, http.StatusOK, recorder.Code)
}
