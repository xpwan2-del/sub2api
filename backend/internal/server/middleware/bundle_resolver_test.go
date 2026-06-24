// bundle_resolver_test.go 套餐路由解析中间件测试
// 验证额度预检：当 CheckQuotaEligibility 判定额度用尽时，中间件返回 429。

package middleware

import (
	"bytes"
	"context"
	"encoding/json"
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
func (f *mwFakeUsageRepo) IncrementUsage(_ context.Context, _ int64, _ float64, _ int) error {
	return nil
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
			GroupID:           groupID,
			QuotaScope:        service.QuotaScopeModel,
			ModelPattern:      "gpt-4o",
			MonthlyLimitCount: 5,
		}},
	}
	sub := &service.BundleSubscription{ID: bundleSubID, PlanID: 1, Status: service.BundleStatusActive}
	// Usage already at limit -> count exhausted.
	usage := &service.BundleSubscriptionUsage{MonthlyUsageCount: 5}
	group := &service.Group{ID: groupID, Platform: "openai"}

	resolver := service.NewBundleRouteResolver(
		&mwFakeSubRepo{sub: sub},
		&mwFakePlanRepo{plan: plan},
		&mwFakeGroupRepo{group: group},
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
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
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
