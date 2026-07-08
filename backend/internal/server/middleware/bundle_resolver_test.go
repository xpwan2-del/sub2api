// bundle_resolver_test.go 套餐路由解析中间件测试
// 验证额度预检：当 CheckQuotaEligibility 判定额度用尽时，中间件返回 429。

package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ----- 最小化的 fake repos，仅满足测试需要的接口方法 -----

type mwFakeUsageRepo struct {
	usage *service.BundleSubscriptionUsage
}

func (f *mwFakeUsageRepo) GetBySubscriptionAndGroup(_ context.Context, _, _ int64, _ string) (*service.BundleSubscriptionUsage, error) {
	return f.usage, nil
}
func (f *mwFakeUsageRepo) Create(_ context.Context, _ *service.BundleSubscriptionUsage) error {
	return nil
}
func (f *mwFakeUsageRepo) IncrementUsage(_ context.Context, _ int64, _ float64, _ int, _ int, _ time.Time) error {
	return nil
}
func (f *mwFakeUsageRepo) GetOrCreateUsage(_ context.Context, _ int64, _ int64, _ string, _ time.Time) (*service.BundleSubscriptionUsage, error) {
	return nil, nil
}
func (f *mwFakeUsageRepo) ResetDailyWindow(_ context.Context, _ int64, _ time.Time) error { return nil }
func (f *mwFakeUsageRepo) ResetWeeklyWindow(_ context.Context, _ int64, _ time.Time) error {
	return nil
}
func (f *mwFakeUsageRepo) ResetMonthlyWindow(_ context.Context, _ int64, _ time.Time) error {
	return nil
}
func (f *mwFakeUsageRepo) ListBySubscription(_ context.Context, _ int64) ([]service.BundleSubscriptionUsage, error) {
	return nil, nil
}
func (f *mwFakeUsageRepo) BatchUpdateExpiredStatus(_ context.Context) (int64, error) { return 0, nil }

// compile-time interface checks (params typed to mirror the real interface)
var (
	_ service.BundleUsageRepository = (*mwFakeUsageRepo)(nil)
)

type mwFakeSubRepo struct{ sub *service.BundleSubscription }

func (f *mwFakeSubRepo) Create(_ context.Context, _ *service.BundleSubscription) error { return nil }
func (f *mwFakeSubRepo) GetByID(_ context.Context, _ int64) (*service.BundleSubscription, error) {
	return f.sub, nil
}
func (f *mwFakeSubRepo) GetActiveByUserID(_ context.Context, _ int64) ([]service.BundleSubscription, error) {
	return nil, nil
}
func (f *mwFakeSubRepo) GetByIDWithUsages(_ context.Context, _ int64) (*service.BundleSubscription, error) {
	return f.sub, nil
}
func (f *mwFakeSubRepo) List(_ context.Context, _ pagination.PaginationParams, _ *int64, _ string) ([]service.BundleSubscription, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (f *mwFakeSubRepo) UpdateStatus(_ context.Context, _ int64, _ string) error    { return nil }
func (f *mwFakeSubRepo) UpdateExpiry(_ context.Context, _ int64, _ time.Time) error { return nil }

var _ service.BundleSubscriptionRepository = (*mwFakeSubRepo)(nil)

type mwFakePlanRepo struct{ plan *service.BundlePlan }

func (f *mwFakePlanRepo) Create(_ context.Context, _ *service.BundlePlan) error { return nil }
func (f *mwFakePlanRepo) Update(_ context.Context, _ *service.BundlePlan) error { return nil }
func (f *mwFakePlanRepo) GetByID(_ context.Context, _ int64) (*service.BundlePlan, error) {
	return f.plan, nil
}
func (f *mwFakePlanRepo) List(_ context.Context, _ pagination.PaginationParams, _, _ string) ([]service.BundlePlan, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (f *mwFakePlanRepo) ListForSale(_ context.Context) ([]service.BundlePlan, error) {
	return nil, nil
}
func (f *mwFakePlanRepo) Delete(_ context.Context, _ int64) error { return nil }

var _ service.BundlePlanRepository = (*mwFakePlanRepo)(nil)

// mwFakeGroupRepo 实现 GroupRepository 全部方法，但只填充 GetByIDLite。
type mwFakeGroupRepo struct{ group *service.Group }

func (f *mwFakeGroupRepo) Create(_ context.Context, _ *service.Group) error { return nil }
func (f *mwFakeGroupRepo) GetByID(_ context.Context, id int64) (*service.Group, error) {
	return f.group, nil
}
func (f *mwFakeGroupRepo) GetByIDLite(_ context.Context, _ int64) (*service.Group, error) {
	return f.group, nil
}
func (f *mwFakeGroupRepo) Update(_ context.Context, _ *service.Group) error { return nil }
func (f *mwFakeGroupRepo) Delete(_ context.Context, _ int64) error          { return nil }
func (f *mwFakeGroupRepo) DeleteCascade(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}
func (f *mwFakeGroupRepo) List(_ context.Context, _ pagination.PaginationParams) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (f *mwFakeGroupRepo) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _, _ string, _ *bool) ([]service.Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (f *mwFakeGroupRepo) ListActive(_ context.Context) ([]service.Group, error) { return nil, nil }
func (f *mwFakeGroupRepo) ListActiveByPlatform(_ context.Context, _ string) ([]service.Group, error) {
	return nil, nil
}
func (f *mwFakeGroupRepo) ExistsByName(_ context.Context, _ string) (bool, error) { return false, nil }
func (f *mwFakeGroupRepo) GetAccountCount(_ context.Context, _ int64) (int64, int64, error) {
	return 0, 0, nil
}
func (f *mwFakeGroupRepo) DeleteAccountGroupsByGroupID(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}
func (f *mwFakeGroupRepo) GetAccountIDsByGroupIDs(_ context.Context, _ []int64) ([]int64, error) {
	return nil, nil
}
func (f *mwFakeGroupRepo) BindAccountsToGroup(_ context.Context, _ int64, _ []int64) error {
	return nil
}
func (f *mwFakeGroupRepo) UpdateSortOrders(_ context.Context, _ []service.GroupSortOrderUpdate) error {
	return nil
}

var _ service.GroupRepository = (*mwFakeGroupRepo)(nil)

// TestBundleResolver_QuotaExceededReturns429 额度用尽时，中间件应返回 429 并中断。
func TestBundleResolver_QuotaExceededReturns429(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		bundleSubID int64 = 7
		groupID     int64 = 100
	)
	// Plan: model-scope quota, monthly count limit = 5.
	plan := &service.BundlePlan{
		ID: 1,
		GroupQuotas: []service.BundlePlanGroupQuota{{
			GroupID:                groupID,
			QuotaScope:             service.QuotaScopeModel,
			ModelPattern:           "gpt-4o",
			MonthlyImageLimitCount: 5,
		}},
	}
	sub := &service.BundleSubscription{ID: bundleSubID, PlanID: 1, Status: service.BundleStatusActive}
	// Usage already at limit -> count exhausted.
	usage := &service.BundleSubscriptionUsage{MonthlyImageUsageCount: 5}
	group := &service.Group{ID: groupID, Platform: "openai"}

	resolver := service.NewBundleRouteResolver(
		&mwFakeSubRepo{sub: sub},
		&mwFakePlanRepo{plan: plan},
		&mwFakeGroupRepo{group: group},
		nil,
	)
	usageSvc := service.NewBundleUsageService(
		&mwFakeUsageRepo{usage: usage},
		&mwFakeSubRepo{sub: sub},
		&mwFakePlanRepo{plan: plan},
	)
	mw := NewBundleRouteResolverMiddleware(resolver, nil, nil, nil, usageSvc)

	bundleSubIDVal := bundleSubID
	apiKey := &service.APIKey{
		ID:                   1,
		BundleSubscriptionID: &bundleSubIDVal,
	}

	body, _ := json.Marshal(map[string]string{"model": "gpt-4o"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// 路径含 /images → 推断为 ModalityImage → 触发 image count 校验(已耗尽)。
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(ContextKeyAPIKey), apiKey)

	mw.BundleResolver()(c)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 when quota exhausted, got %d (body=%s)", w.Code, w.Body.String())
	}
	var resp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error.Type != "BUNDLE_GROUP_QUOTA_EXCEEDED" {
		t.Fatalf("expected error type BUNDLE_GROUP_QUOTA_EXCEEDED, got %q", resp.Error.Type)
	}
	if !c.IsAborted() {
		t.Fatalf("expected request to be aborted")
	}
}

// TestExtractModelFromRequest_MultipartFormData 验证 bundle key 在 multipart/form-data
// 请求（如 /v1/videos）下能正确提取 model，且提取后 body 仍可被下游 handler 重读。
//
// 回归背景：视频/图片编辑接口用 multipart 提交，旧实现只用 json.Unmarshal 解析 body，
// 导致 bundle key 取不到 model → 跳过 group 注入 → 平台门控 404。
func TestExtractModelFromRequest_MultipartFormData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	fw := multipart.NewWriter(&buf)
	if err := fw.WriteField("model", "grok-imagine-video"); err != nil {
		t.Fatalf("write model field: %v", err)
	}
	if err := fw.WriteField("prompt", "一只猫慵懒地躺在桌子上"); err != nil {
		t.Fatalf("write prompt field: %v", err)
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	rawBody := buf.Bytes()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(rawBody))
	c.Request.Header.Set("Content-Type", fw.FormDataContentType())

	got := extractModelFromRequest(c)
	if got != "grok-imagine-video" {
		t.Fatalf("expected model %q from multipart form, got %q", "grok-imagine-video", got)
	}

	// body 必须可重读：下游 Videos handler 还要原样转发 multipart body 给上游。
	if c.Request.Body == nil {
		t.Fatal("request body is nil after extraction")
	}
	second, err := io.ReadAll(c.Request.Body)
	if err != nil {
		t.Fatalf("re-read body: %v", err)
	}
	if !bytes.Equal(second, rawBody) {
		t.Fatalf("body changed after extraction: got %d bytes, want %d", len(second), len(rawBody))
	}
}

// TestExtractModelFromRequest_JSONRegression 保护既有 JSON body 提取路径不被 multipart 改动破坏。
func TestExtractModelFromRequest_JSONRegression(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body, _ := json.Marshal(map[string]string{"model": "gpt-4o"})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	if got := extractModelFromRequest(c); got != "gpt-4o" {
		t.Fatalf("expected model %q from JSON body, got %q", "gpt-4o", got)
	}
}

// mwFakeVideoCache 实现 GatewayCache 但仅填充 GetVideoTaskBinding，
// 用于 bundle GET videos 反查测试。bindings key 形如 "groupID:taskID"；
// 未命中返回零值（Model 空），resolver 视为未命中。
type mwFakeVideoCache struct {
	bindings map[string]service.VideoTaskBinding
}

func (c mwFakeVideoCache) GetSessionAccountID(_ context.Context, _ int64, _ string) (int64, error) {
	return 0, nil
}
func (c mwFakeVideoCache) SetSessionAccountID(_ context.Context, _ int64, _ string, _ int64, _ time.Duration) error {
	return nil
}
func (c mwFakeVideoCache) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	return nil
}
func (c mwFakeVideoCache) DeleteSessionAccountID(_ context.Context, _ int64, _ string) error {
	return nil
}
func (c mwFakeVideoCache) SetVideoTaskBinding(_ context.Context, _ int64, _ string, _ service.VideoTaskBinding, _ time.Duration) error {
	return nil
}
func (c mwFakeVideoCache) GetVideoTaskBinding(_ context.Context, groupID int64, taskID string) (service.VideoTaskBinding, error) {
	if c.bindings != nil {
		if b, ok := c.bindings[fmt.Sprintf("%d:%s", groupID, taskID)]; ok {
			return b, nil
		}
	}
	return service.VideoTaskBinding{}, nil
}
func (c mwFakeVideoCache) DeleteVideoTaskBinding(_ context.Context, _ int64, _ string) error {
	return nil
}

// TestBundleResolver_GETVideosResolvesGroupViaTaskBinding 验证通用 Key 查询视频进度
// （GET /v1/videos/:id，按 OpenAI 规范不带 model）时，中间件用 taskID 反查创建时写入的
// 绑定恢复 group 并注入，使下游平台门控与 handler 能正常工作（不再 404）。
func TestBundleResolver_GETVideosResolvesGroupViaTaskBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		bundleSubID int64 = 9
		groupID     int64 = 200
	)
	plan := &service.BundlePlan{
		ID: 1,
		GroupQuotas: []service.BundlePlanGroupQuota{{
			GroupID:      groupID,
			QuotaScope:   service.QuotaScopeModel,
			ModelPattern: "grok-imagine-video",
		}},
	}
	sub := &service.BundleSubscription{ID: bundleSubID, PlanID: 1, Status: service.BundleStatusActive}
	group := &service.Group{ID: groupID, Platform: "openai"}

	owningSub := bundleSubID
	cache := mwFakeVideoCache{bindings: map[string]service.VideoTaskBinding{
		fmt.Sprintf("%d:%s", groupID, "task-abc"): {AccountID: 5, Model: "grok-imagine-video", BundleSubID: &owningSub},
	}}
	resolver := service.NewBundleRouteResolver(
		&mwFakeSubRepo{sub: sub},
		&mwFakePlanRepo{plan: plan},
		&mwFakeGroupRepo{group: group},
		cache,
	)
	usageSvc := service.NewBundleUsageService(&mwFakeUsageRepo{}, &mwFakeSubRepo{sub: sub}, &mwFakePlanRepo{plan: plan})
	mw := NewBundleRouteResolverMiddleware(resolver, nil, nil, nil, usageSvc)

	bundleSubIDVal := bundleSubID
	apiKey := &service.APIKey{ID: 1, BundleSubscriptionID: &bundleSubIDVal}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task-abc", nil)
	c.Set(string(ContextKeyAPIKey), apiKey)

	mw.BundleResolver()(c)

	if c.IsAborted() {
		t.Fatalf("GET videos should not be aborted, status=%d", c.Writer.Status())
	}
	if apiKey.Group == nil {
		t.Fatal("expected group injected for GET videos via task binding, got nil")
	}
	if apiKey.Group.Platform != "openai" {
		t.Fatalf("expected openai platform, got %q", apiKey.Group.Platform)
	}
	if apiKey.GroupID == nil || *apiKey.GroupID != groupID {
		t.Fatalf("expected groupID %d injected, got %v", groupID, apiKey.GroupID)
	}
}

// TestBundleResolver_GETVideosWithoutBindingSkips 验证 task 绑定未命中（task 不属于本订阅
// 或已过期）时，中间件不注入 group 也不 abort，回退到原有 skipping 行为交由下游处理。
func TestBundleResolver_GETVideosWithoutBindingSkips(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		bundleSubID int64 = 9
		groupID     int64 = 200
	)
	plan := &service.BundlePlan{
		ID:          1,
		GroupQuotas: []service.BundlePlanGroupQuota{{GroupID: groupID, QuotaScope: service.QuotaScopeModel, ModelPattern: "grok-imagine-video"}},
	}
	sub := &service.BundleSubscription{ID: bundleSubID, PlanID: 1, Status: service.BundleStatusActive}
	group := &service.Group{ID: groupID, Platform: "openai"}

	// 空 cache → task 反查未命中。
	resolver := service.NewBundleRouteResolver(
		&mwFakeSubRepo{sub: sub},
		&mwFakePlanRepo{plan: plan},
		&mwFakeGroupRepo{group: group},
		mwFakeVideoCache{},
	)
	usageSvc := service.NewBundleUsageService(&mwFakeUsageRepo{}, &mwFakeSubRepo{sub: sub}, &mwFakePlanRepo{plan: plan})
	mw := NewBundleRouteResolverMiddleware(resolver, nil, nil, nil, usageSvc)

	bundleSubIDVal := bundleSubID
	apiKey := &service.APIKey{ID: 1, BundleSubscriptionID: &bundleSubIDVal}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/unknown-task", nil)
	c.Set(string(ContextKeyAPIKey), apiKey)

	mw.BundleResolver()(c)

	if c.IsAborted() {
		t.Fatalf("should not abort when task binding missing, status=%d", c.Writer.Status())
	}
	if apiKey.Group != nil {
		t.Fatalf("expected nil group when binding missing, got platform=%q", apiKey.Group.Platform)
	}
}

// TestBundleResolver_GETVideosDoesNotLeakAcrossSubscriptions 验证视频任务反查 scope 到订阅：
// 订阅 B 不能通过 taskID 查到订阅 A 创建的视频任务（防跨订阅 IDOR 越权查询/取内容）。
// 即便两者共享同一 group（同一上游账号池是常见配置），绑定归属也必须隔离。
func TestBundleResolver_GETVideosDoesNotLeakAcrossSubscriptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		subA    int64 = 9
		subB    int64 = 11
		groupID int64 = 200
	)
	// A 与 B 共享同一 plan/group（同一上游账号池是常见配置）。
	plan := &service.BundlePlan{
		ID:          1,
		GroupQuotas: []service.BundlePlanGroupQuota{{GroupID: groupID, QuotaScope: service.QuotaScopeModel, ModelPattern: "grok-imagine-video"}},
	}
	subBObj := &service.BundleSubscription{ID: subB, PlanID: 1, Status: service.BundleStatusActive}
	group := &service.Group{ID: groupID, Platform: "openai"}

	// task-X 由订阅 A 创建，绑定归属 subA。
	subARef := subA
	cache := mwFakeVideoCache{bindings: map[string]service.VideoTaskBinding{
		fmt.Sprintf("%d:%s", groupID, "task-X"): {AccountID: 5, Model: "grok-imagine-video", BundleSubID: &subARef},
	}}
	// 用订阅 B 的解析器查询 task-X。
	resolver := service.NewBundleRouteResolver(
		&mwFakeSubRepo{sub: subBObj},
		&mwFakePlanRepo{plan: plan},
		&mwFakeGroupRepo{group: group},
		cache,
	)
	usageSvc := service.NewBundleUsageService(&mwFakeUsageRepo{}, &mwFakeSubRepo{sub: subBObj}, &mwFakePlanRepo{plan: plan})
	mw := NewBundleRouteResolverMiddleware(resolver, nil, nil, nil, usageSvc)

	subBRef := subB
	apiKey := &service.APIKey{ID: 1, BundleSubscriptionID: &subBRef}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task-X", nil)
	c.Set(string(ContextKeyAPIKey), apiKey)

	mw.BundleResolver()(c)

	if c.IsAborted() {
		t.Fatalf("should not abort when task belongs to another subscription, status=%d", c.Writer.Status())
	}
	if apiKey.Group != nil {
		t.Fatalf("subscription B must not resolve task owned by subscription A (IDOR): got platform=%q", apiKey.Group.Platform)
	}
}

// TestBundleResolver_GETVideosSkipsQuotaCheckWhenExhausted 验证只读视频查询
// （GET /v1/videos/:id）在套餐 video 配额已耗尽时仍应放行——视频任务在 POST 创建时
// 已通过配额检查（合法创建），轮询进度/取内容是只读操作、不消耗配额，不应被 pre-check
// 拒绝，否则用户一旦套餐达上限就再也查不到自己已创建任务的结果。
//
// 回归背景：commit 5756aacf 给 GET 视频查询加了 taskID 反查 group，使 resolveBundleGroup
// 成功，但中间件在反查成功后无条件执行配额 pre-check，导致 GET 查询进度被错误拒绝。
func TestBundleResolver_GETVideosSkipsQuotaCheckWhenExhausted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		bundleSubID int64 = 9
		groupID     int64 = 200
	)
	// Plan: model-scope quota，月度 video count 上限 = 5。
	plan := &service.BundlePlan{
		ID: 1,
		GroupQuotas: []service.BundlePlanGroupQuota{{
			GroupID:                groupID,
			QuotaScope:             service.QuotaScopeModel,
			ModelPattern:           "grok-imagine-video",
			MonthlyVideoLimitCount: 5,
		}},
	}
	sub := &service.BundleSubscription{ID: bundleSubID, PlanID: 1, Status: service.BundleStatusActive}
	group := &service.Group{ID: groupID, Platform: "openai"}
	// video count 已达上限 → ModalityVideo 轨道耗尽，pre-check 本应拒绝。
	usage := &service.BundleSubscriptionUsage{MonthlyVideoUsageCount: 5}

	owningSub := bundleSubID
	cache := mwFakeVideoCache{bindings: map[string]service.VideoTaskBinding{
		fmt.Sprintf("%d:%s", groupID, "task-abc"): {AccountID: 5, Model: "grok-imagine-video", BundleSubID: &owningSub},
	}}
	resolver := service.NewBundleRouteResolver(
		&mwFakeSubRepo{sub: sub},
		&mwFakePlanRepo{plan: plan},
		&mwFakeGroupRepo{group: group},
		cache,
	)
	usageSvc := service.NewBundleUsageService(&mwFakeUsageRepo{usage: usage}, &mwFakeSubRepo{sub: sub}, &mwFakePlanRepo{plan: plan})
	mw := NewBundleRouteResolverMiddleware(resolver, nil, nil, nil, usageSvc)

	bundleSubIDVal := bundleSubID
	apiKey := &service.APIKey{ID: 1, BundleSubscriptionID: &bundleSubIDVal}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task-abc", nil)
	c.Set(string(ContextKeyAPIKey), apiKey)

	mw.BundleResolver()(c)

	if c.IsAborted() {
		t.Fatalf("GET video progress query must not be aborted by quota pre-check when bundle is exhausted, status=%d", c.Writer.Status())
	}
	if apiKey.Group == nil || apiKey.GroupID == nil || *apiKey.GroupID != groupID {
		t.Fatalf("expected group %d injected for read-only GET video query, got group=%v", groupID, apiKey.Group)
	}
}

// TestBundleResolver_GETVideosContentSkipsQuotaCheckWhenExhausted 同上，覆盖取内容路径
// （GET /v1/videos/:id/content）——同样是只读查询，套餐耗尽时应放行。
func TestBundleResolver_GETVideosContentSkipsQuotaCheckWhenExhausted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		bundleSubID int64 = 9
		groupID     int64 = 200
	)
	plan := &service.BundlePlan{
		ID: 1,
		GroupQuotas: []service.BundlePlanGroupQuota{{
			GroupID:                groupID,
			QuotaScope:             service.QuotaScopeModel,
			ModelPattern:           "grok-imagine-video",
			MonthlyVideoLimitCount: 5,
		}},
	}
	sub := &service.BundleSubscription{ID: bundleSubID, PlanID: 1, Status: service.BundleStatusActive}
	group := &service.Group{ID: groupID, Platform: "openai"}
	usage := &service.BundleSubscriptionUsage{MonthlyVideoUsageCount: 5}

	owningSub := bundleSubID
	cache := mwFakeVideoCache{bindings: map[string]service.VideoTaskBinding{
		fmt.Sprintf("%d:%s", groupID, "task-abc"): {AccountID: 5, Model: "grok-imagine-video", BundleSubID: &owningSub},
	}}
	resolver := service.NewBundleRouteResolver(
		&mwFakeSubRepo{sub: sub},
		&mwFakePlanRepo{plan: plan},
		&mwFakeGroupRepo{group: group},
		cache,
	)
	usageSvc := service.NewBundleUsageService(&mwFakeUsageRepo{usage: usage}, &mwFakeSubRepo{sub: sub}, &mwFakePlanRepo{plan: plan})
	mw := NewBundleRouteResolverMiddleware(resolver, nil, nil, nil, usageSvc)

	bundleSubIDVal := bundleSubID
	apiKey := &service.APIKey{ID: 1, BundleSubscriptionID: &bundleSubIDVal}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task-abc/content", nil)
	c.Set(string(ContextKeyAPIKey), apiKey)

	mw.BundleResolver()(c)

	if c.IsAborted() {
		t.Fatalf("GET video content query must not be aborted by quota pre-check when bundle is exhausted, status=%d", c.Writer.Status())
	}
	if apiKey.GroupID == nil || *apiKey.GroupID != groupID {
		t.Fatalf("expected group %d injected for read-only GET video content query, got %v", groupID, apiKey.GroupID)
	}
}

// TestBundleResolver_GETVideosWithModelParamStillChecksQuota 验证只读豁免的精确性：
// 带 ?model= 参数的 GET /v1/videos/:id 走的是 model 路由（ResolveGroup），并非 task 反查，
// 语义上不属于"查询已创建任务"。配额耗尽时仍应被 pre-check 拒绝——豁免仅限无 model
// 参数、通过 task binding 反查解析的纯查询请求。
func TestBundleResolver_GETVideosWithModelParamStillChecksQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		bundleSubID int64 = 9
		groupID     int64 = 200
	)
	plan := &service.BundlePlan{
		ID: 1,
		GroupQuotas: []service.BundlePlanGroupQuota{{
			GroupID:                groupID,
			QuotaScope:             service.QuotaScopeModel,
			ModelPattern:           "grok-imagine-video",
			MonthlyVideoLimitCount: 5,
		}},
	}
	sub := &service.BundleSubscription{ID: bundleSubID, PlanID: 1, Status: service.BundleStatusActive}
	group := &service.Group{ID: groupID, Platform: "openai"}
	usage := &service.BundleSubscriptionUsage{MonthlyVideoUsageCount: 5}

	owningSub := bundleSubID
	cache := mwFakeVideoCache{bindings: map[string]service.VideoTaskBinding{
		fmt.Sprintf("%d:%s", groupID, "task-abc"): {AccountID: 5, Model: "grok-imagine-video", BundleSubID: &owningSub},
	}}
	resolver := service.NewBundleRouteResolver(
		&mwFakeSubRepo{sub: sub},
		&mwFakePlanRepo{plan: plan},
		&mwFakeGroupRepo{group: group},
		cache,
	)
	usageSvc := service.NewBundleUsageService(&mwFakeUsageRepo{usage: usage}, &mwFakeSubRepo{sub: sub}, &mwFakePlanRepo{plan: plan})
	mw := NewBundleRouteResolverMiddleware(resolver, nil, nil, nil, usageSvc)

	bundleSubIDVal := bundleSubID
	apiKey := &service.APIKey{ID: 1, BundleSubscriptionID: &bundleSubIDVal}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	// 带 ?model= → 走 model 路由，不享受只读豁免。
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task-abc?model=grok-imagine-video", nil)
	c.Set(string(ContextKeyAPIKey), apiKey)

	mw.BundleResolver()(c)

	if !c.IsAborted() {
		t.Fatal("GET video request routed by model param must still be gated by quota pre-check (not skipped)")
	}
	if c.Writer.Status() != http.StatusTooManyRequests {
		t.Fatalf("expected 429 when quota exhausted for model-routed GET, got status=%d", c.Writer.Status())
	}
}

// TestExtractVideoTaskIDFromPath 保护 task id 提取逻辑（进度查询与取内容两种路径）。
func TestExtractVideoTaskIDFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/v1/videos/task-abc", "task-abc"},
		{"/v1/videos/task-abc/content", "task-abc"},
		{"/v1/videos/task-abc/", "task-abc"},
		{"/videos/x", "x"},
		{"/v1/messages", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := extractVideoTaskIDFromPath(tt.path); got != tt.want {
			t.Fatalf("extractVideoTaskIDFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
