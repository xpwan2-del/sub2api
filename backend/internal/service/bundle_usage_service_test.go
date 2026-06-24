// bundle_usage_service_test.go 套餐用量服务单元测试
// 覆盖 CheckQuotaEligibility 的 USD + 次数双重判断逻辑。

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// fakeUsageRepo 内存实现的 BundleUsageRepository，仅覆盖测试需要的子集。
type fakeUsageRepo struct {
	usage *BundleSubscriptionUsage
}

func (f *fakeUsageRepo) GetBySubscriptionAndGroup(_ context.Context, _, _ int64, _ string) (*BundleSubscriptionUsage, error) {
	return f.usage, nil
}
func (f *fakeUsageRepo) Create(_ context.Context, _ *BundleSubscriptionUsage) error { return nil }
func (f *fakeUsageRepo) IncrementUsage(_ context.Context, _ int64, _ float64, _ int) error {
	return nil
}
func (f *fakeUsageRepo) ResetDailyWindow(_ context.Context, _ int64, _ time.Time) error  { return nil }
func (f *fakeUsageRepo) ResetWeeklyWindow(_ context.Context, _ int64, _ time.Time) error { return nil }
func (f *fakeUsageRepo) ResetMonthlyWindow(_ context.Context, _ int64, _ time.Time) error {
	return nil
}
func (f *fakeUsageRepo) ListBySubscription(_ context.Context, _ int64) ([]BundleSubscriptionUsage, error) {
	return nil, nil
}
func (f *fakeUsageRepo) BatchUpdateExpiredStatus(_ context.Context) (int64, error) { return 0, nil }

// fakeSubRepo 内存实现的 BundleSubscriptionRepository 子集。
type fakeSubRepo struct {
	sub *BundleSubscription
}

func (f *fakeSubRepo) Create(_ context.Context, _ *BundleSubscription) error { return nil }
func (f *fakeSubRepo) GetByID(_ context.Context, _ int64) (*BundleSubscription, error) {
	return f.sub, nil
}
func (f *fakeSubRepo) GetActiveByUserID(_ context.Context, _ int64) ([]BundleSubscription, error) {
	return nil, nil
}
func (f *fakeSubRepo) GetByIDWithUsages(_ context.Context, _ int64) (*BundleSubscription, error) {
	return f.sub, nil
}
func (f *fakeSubRepo) List(_ context.Context, _ pagination.PaginationParams, _ *int64, _ string) ([]BundleSubscription, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (f *fakeSubRepo) UpdateStatus(_ context.Context, _ int64, _ string) error    { return nil }
func (f *fakeSubRepo) UpdateExpiry(_ context.Context, _ int64, _ time.Time) error { return nil }

// fakePlanRepo 内存实现的 BundlePlanRepository 子集。
type fakePlanRepo struct {
	plan *BundlePlan
}

func (f *fakePlanRepo) Create(_ context.Context, _ *BundlePlan) error { return nil }
func (f *fakePlanRepo) Update(_ context.Context, _ *BundlePlan) error { return nil }
func (f *fakePlanRepo) GetByID(_ context.Context, _ int64) (*BundlePlan, error) {
	return f.plan, nil
}
func (f *fakePlanRepo) List(_ context.Context, _ pagination.PaginationParams, _, _ string) ([]BundlePlan, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (f *fakePlanRepo) ListForSale(_ context.Context) ([]BundlePlan, error) { return nil, nil }
func (f *fakePlanRepo) Delete(_ context.Context, _ int64) error             { return nil }

// newSvcWith 构造一个注入 fake repo 的 BundleUsageService。
func newSvcWith(plan *BundlePlan, sub *BundleSubscription, usage *BundleSubscriptionUsage) *BundleUsageService {
	return NewBundleUsageService(
		&fakeUsageRepo{usage: usage},
		&fakeSubRepo{sub: sub},
		&fakePlanRepo{plan: plan},
	)
}

func TestCheckQuotaEligibility_CountLimitExceeded(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{
		GroupQuotas: []BundlePlanGroupQuota{{
			GroupID:           groupID,
			MonthlyLimitCount: 10,
			MonthlyLimitUSD:   100,
		}},
	}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{MonthlyUsageCount: 10}

	svc := newSvcWith(plan, sub, usage)
	res, err := svc.CheckQuotaEligibility(context.Background(), 1, groupID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Eligible {
		t.Fatalf("expected Eligible=false when monthly count exhausted, got true")
	}
	if res.MonthlyRemainingCount > 0 {
		t.Fatalf("expected MonthlyRemainingCount<=0, got %d", res.MonthlyRemainingCount)
	}
}

func TestCheckQuotaEligibility_CountZeroNoLimit(t *testing.T) {
	const groupID int64 = 100
	plan := &BundlePlan{
		GroupQuotas: []BundlePlanGroupQuota{{
			GroupID:           groupID,
			MonthlyLimitCount: 0, // 0 = 不限次数
			MonthlyLimitUSD:   0, // 0 = 不限额度
		}},
	}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	usage := &BundleSubscriptionUsage{MonthlyUsageCount: 999}

	svc := newSvcWith(plan, sub, usage)
	res, err := svc.CheckQuotaEligibility(context.Background(), 1, groupID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Eligible {
		t.Fatalf("expected Eligible=true when count limit is 0 (unlimited), got false")
	}
}
