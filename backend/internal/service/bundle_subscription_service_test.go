//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// ──────────────────────────────────────────────────────
// Noop base structs (panic on unexpected calls)
// ──────────────────────────────────────────────────────

type bundleSubRepoNoop struct{}

func (bundleSubRepoNoop) Create(context.Context, *BundleSubscription) error {
	panic("unexpected Create call")
}
func (bundleSubRepoNoop) GetByID(context.Context, int64) (*BundleSubscription, error) {
	panic("unexpected GetByID call")
}
func (bundleSubRepoNoop) GetActiveByUserID(context.Context, int64) ([]BundleSubscription, error) {
	panic("unexpected GetActiveByUserID call")
}
func (bundleSubRepoNoop) GetByIDWithUsages(context.Context, int64) (*BundleSubscription, error) {
	panic("unexpected GetByIDWithUsages call")
}
func (bundleSubRepoNoop) List(context.Context, pagination.PaginationParams, *int64, string) ([]BundleSubscription, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (bundleSubRepoNoop) UpdateStatus(context.Context, int64, string) error {
	panic("unexpected UpdateStatus call")
}
func (bundleSubRepoNoop) UpdateExpiry(context.Context, int64, time.Time) error {
	panic("unexpected UpdateExpiry call")
}
func (bundleSubRepoNoop) ExtendExpiryByDays(context.Context, int64, int) error {
	panic("unexpected ExtendExpiryByDays call")
}

type bundleUsageRepoNoop struct{}

func (bundleUsageRepoNoop) GetBySubscriptionAndGroup(context.Context, int64, int64, string) (*BundleSubscriptionUsage, error) {
	panic("unexpected GetBySubscriptionAndGroup call")
}
func (bundleUsageRepoNoop) Create(context.Context, *BundleSubscriptionUsage) error {
	panic("unexpected Create call")
}
func (bundleUsageRepoNoop) IncrementUsage(context.Context, int64, float64, int, int, time.Time) error {
	panic("unexpected IncrementUsage call")
}
func (bundleUsageRepoNoop) GetOrCreateUsage(context.Context, int64, int64, string, time.Time) (*BundleSubscriptionUsage, error) {
	panic("unexpected GetOrCreateUsage call")
}
func (bundleUsageRepoNoop) ResetDailyWindow(context.Context, int64, time.Time) error {
	panic("unexpected ResetDailyWindow call")
}
func (bundleUsageRepoNoop) ResetWeeklyWindow(context.Context, int64, time.Time) error {
	panic("unexpected ResetWeeklyWindow call")
}
func (bundleUsageRepoNoop) ResetMonthlyWindow(context.Context, int64, time.Time) error {
	panic("unexpected ResetMonthlyWindow call")
}
func (bundleUsageRepoNoop) ListBySubscription(context.Context, int64) ([]BundleSubscriptionUsage, error) {
	panic("unexpected ListBySubscription call")
}
func (bundleUsageRepoNoop) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	panic("unexpected BatchUpdateExpiredStatus call")
}

// ──────────────────────────────────────────────────────
// Stubs for BundleSubscriptionService tests
// ──────────────────────────────────────────────────────

// activateBundleSubRepoStub supports GetActiveByUserID + Create + GetByIDWithUsages + GetByID + UpdateStatus + UpdateExpiry.
type activateBundleSubRepoStub struct {
	bundleSubRepoNoop

	activeBundles   []BundleSubscription
	created         *BundleSubscription
	createErr       error
	updateStatusErr error
	updateExpiryErr error

	extendByDaysErr    error   // ExtendExpiryByDays 返回的错误（中危1 增量延期）
	extendedByDaysIDs  []int64 // 记录 ExtendExpiryByDays 调用
	updateExpiryCalled bool    // 是否仍走旧的覆盖写 UpdateExpiry（中危1 后应为 false）
}

func (s *activateBundleSubRepoStub) GetActiveByUserID(_ context.Context, _ int64) ([]BundleSubscription, error) {
	if s.createErr != nil && s.activeBundles == nil {
		return nil, s.createErr
	}
	return s.activeBundles, nil
}

func (s *activateBundleSubRepoStub) Create(_ context.Context, sub *BundleSubscription) error {
	if s.createErr != nil {
		return s.createErr
	}
	sub.ID = 100
	s.created = sub
	return nil
}

func (s *activateBundleSubRepoStub) GetByID(_ context.Context, id int64) (*BundleSubscription, error) {
	if s.created == nil || s.created.ID != id {
		return nil, ErrBundleNotFound
	}
	cp := *s.created
	return &cp, nil
}

func (s *activateBundleSubRepoStub) GetByIDWithUsages(_ context.Context, id int64) (*BundleSubscription, error) {
	if s.created == nil || s.created.ID != id {
		return nil, ErrBundleNotFound
	}
	cp := *s.created
	return &cp, nil
}

func (s *activateBundleSubRepoStub) UpdateStatus(_ context.Context, _ int64, _ string) error {
	return s.updateStatusErr
}

func (s *activateBundleSubRepoStub) UpdateExpiry(_ context.Context, _ int64, _ time.Time) error {
	s.updateExpiryCalled = true
	return s.updateExpiryErr
}

// ExtendExpiryByDays 记录增量延期调用（中危1：ExtendBundle 改用原子增量延期）。
func (s *activateBundleSubRepoStub) ExtendExpiryByDays(_ context.Context, id int64, _ int) error {
	if s.extendByDaysErr != nil {
		return s.extendByDaysErr
	}
	s.extendedByDaysIDs = append(s.extendedByDaysIDs, id)
	return nil
}

// activateBundlePlanRepoStub supports GetByID for plan loading.
type activateBundlePlanRepoStub struct {
	bundlePlanRepoNoop

	plan   *BundlePlan
	getErr error
}

func (s *activateBundlePlanRepoStub) GetByID(_ context.Context, _ int64) (*BundlePlan, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.plan, nil
}

// activateBundleUsageRepoStub supports Create.
type activateBundleUsageRepoStub struct {
	bundleUsageRepoNoop

	createdUsages []BundleSubscriptionUsage
	createErr     error
}

func (s *activateBundleUsageRepoStub) Create(_ context.Context, usage *BundleSubscriptionUsage) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.createdUsages = append(s.createdUsages, *usage)
	return nil
}

// activateUserSubRepoStub supports Create, ListByUserID, UpdateStatus, ExtendExpiry, Delete.
type activateUserSubRepoStub struct {
	userSubRepoNoop

	createdSubs     []UserSubscription
	existingSubs    []UserSubscription // pre-existing subs for ListByUserID
	createErr       error
	updateStatusErr error
	extendExpiryErr error
	deleteErr       error

	updatedStatusIDs  []int64
	deletedIDs        []int64 // 记录 Delete 调用（UpgradeBundle ②步软删除旧桥接 userSub）
	extendedIDs       []int64
	extendedByDaysIDs []int64 // 记录 ExtendExpiryByDays 调用（中危1 增量延期）
}

func (s *activateUserSubRepoStub) Create(_ context.Context, sub *UserSubscription) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.createdSubs = append(s.createdSubs, *sub)
	return nil
}

func (s *activateUserSubRepoStub) ListByUserID(_ context.Context, _ int64) ([]UserSubscription, error) {
	return s.existingSubs, nil
}

func (s *activateUserSubRepoStub) UpdateStatus(_ context.Context, id int64, _ string) error {
	if s.updateStatusErr != nil {
		return s.updateStatusErr
	}
	s.updatedStatusIDs = append(s.updatedStatusIDs, id)
	return nil
}

func (s *activateUserSubRepoStub) ExtendExpiry(_ context.Context, id int64, _ time.Time) error {
	if s.extendExpiryErr != nil {
		return s.extendExpiryErr
	}
	s.extendedIDs = append(s.extendedIDs, id)
	return nil
}

// ExtendExpiryByDays 记录增量延期调用（中危1：ExtendBundle 桥接 userSub 也用原子增量）。
func (s *activateUserSubRepoStub) ExtendExpiryByDays(_ context.Context, id int64, _ int) error {
	if s.extendExpiryErr != nil {
		return s.extendExpiryErr
	}
	s.extendedByDaysIDs = append(s.extendedByDaysIDs, id)
	return nil
}

// Delete 记录软删除调用（UpgradeBundle ②步：旧桥接 userSub 软删除以释放 (user_id,group_id) 唯一槽）。
func (s *activateUserSubRepoStub) Delete(_ context.Context, id int64) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deletedIDs = append(s.deletedIDs, id)
	return nil
}

// ──────────────────────────────────────────────────────
// Helper constructors
// ──────────────────────────────────────────────────────

func newBundleSubSvc(
	subRepo BundleSubscriptionRepository,
	planRepo BundlePlanRepository,
	usageRepo BundleUsageRepository,
	userSubRepo UserSubscriptionRepository,
) *BundleSubscriptionService {
	return NewBundleSubscriptionService(subRepo, planRepo, usageRepo, userSubRepo, nil, nil, nil, nil) // nil cache + nil entClient + nil paidAmountReader for unit tests
}

func sampleActivePlan() *BundlePlan {
	return &BundlePlan{
		ID:               1,
		Name:             "Pro Bundle",
		Tier:             BundleTierPro,
		Price:            29.99,
		Currency:         "USD",
		ValidityDays:     30,
		ConcurrencyLimit: 10,
		RPMLimit:         120,
		ForSale:          true,
		Status:           domain.StatusActive,
		GroupQuotas: []BundlePlanGroupQuota{
			{GroupID: 10, QuotaScope: QuotaScopePlatform, DailyLimitUSD: 5.0, WeeklyLimitUSD: 25.0, MonthlyLimitUSD: 100.0, DailyImageLimitCount: 50, WeeklyImageLimitCount: 250, MonthlyImageLimitCount: 1000, DailyVideoLimitCount: 5, WeeklyVideoLimitCount: 25, MonthlyVideoLimitCount: 100},
			{GroupID: 20, QuotaScope: QuotaScopeModel, ModelPattern: "gpt-4*", DailyLimitUSD: 3.0, WeeklyLimitUSD: 15.0, MonthlyLimitUSD: 60.0, DailyImageLimitCount: 30, WeeklyImageLimitCount: 150, MonthlyImageLimitCount: 600},
		},
	}
}

// ──────────────────────────────────────────────────────
// Tests: RevokeBundle / ExtendBundle 事务原子化（L2）
// ──────────────────────────────────────────────────────

// TestRevokeBundle_UserSubSyncFailureReturnsError 守护 L2：桥接 userSub 同步失败时，RevokeBundle
// 必须返回 error（旧实现只 slog.Warn 后返回 nil，留「bundle revoked 但桥接 userSub 仍 active」
// 的状态不一致）。entClient=nil 时 withTx 退化为直执行，故本用例验证「不再静默」；真实事务回滚
// （bundle 状态不变）由集成测试覆盖。
func TestRevokeBundle_UserSubSyncFailureReturnsError(t *testing.T) {
	bundleSubID := int64(100)
	bundleSub := &BundleSubscription{ID: bundleSubID, UserID: 7, PlanID: 1, Status: BundleStatusActive}
	subRepo := &activateBundleSubRepoStub{created: bundleSub}
	userSubRepo := &activateUserSubRepoStub{
		existingSubs: []UserSubscription{{ID: 55, UserID: 7, BundleSubscriptionID: &bundleSubID}},
		deleteErr:    errors.New("db down"),
	}
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, userSubRepo)

	if err := svc.RevokeBundle(context.Background(), bundleSubID); err == nil {
		t.Fatal("RevokeBundle should return error when bridged userSub sync fails (no longer silent Warn)")
	}
}

// TestExtendBundle_UserSubSyncFailureReturnsError 同上，守护 ExtendBundle 的桥接同步失败不再静默。
func TestExtendBundle_UserSubSyncFailureReturnsError(t *testing.T) {
	bundleSubID := int64(100)
	bundleSub := &BundleSubscription{
		ID: bundleSubID, UserID: 7, PlanID: 1, Status: BundleStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	subRepo := &activateBundleSubRepoStub{created: bundleSub}
	userSubRepo := &activateUserSubRepoStub{
		existingSubs:    []UserSubscription{{ID: 55, UserID: 7, BundleSubscriptionID: &bundleSubID}},
		extendExpiryErr: errors.New("db down"),
	}
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, userSubRepo)

	if err := svc.ExtendBundle(context.Background(), bundleSubID, 7); err == nil {
		t.Fatal("ExtendBundle should return error when bridged userSub sync fails (no longer silent Warn)")
	}
}

// ──────────────────────────────────────────────────────
// Tests: AccumulateUsage (count dimension)
// ──────────────────────────────────────────────────────

// accumulateUsageRepoStub supports GetBySubscriptionAndGroup + IncrementUsage and
// records the count argument passed to IncrementUsage so tests can assert on it.
type accumulateUsageRepoStub struct {
	bundleUsageRepoNoop

	existing     *BundleSubscriptionUsage // returned by GetBySubscriptionAndGroup
	getErr       error
	incrementErr error

	lastIncrementID       int64
	lastIncrementCost     float64
	lastIncrementImageCnt int
	lastIncrementVideoCnt int
	incrementCalls        int
}

func (s *accumulateUsageRepoStub) GetBySubscriptionAndGroup(_ context.Context, _, _ int64, _ string) (*BundleSubscriptionUsage, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.existing == nil {
		return nil, ErrBundleNotFound
	}
	cp := *s.existing
	return &cp, nil
}

func (s *accumulateUsageRepoStub) IncrementUsage(_ context.Context, id int64, costUSD float64, imageCount, videoCount int, _ time.Time) error {
	if s.incrementErr != nil {
		return s.incrementErr
	}
	s.lastIncrementID = id
	s.lastIncrementCost = costUSD
	s.lastIncrementImageCnt = imageCount
	s.lastIncrementVideoCnt = videoCount
	s.incrementCalls++
	return nil
}

func TestAccumulateUsage_IncrementsCount(t *testing.T) {
	// Arrange: a pre-existing usage record that GetBySubscriptionAndGroup will hit.
	usageRepo := &accumulateUsageRepoStub{existing: &BundleSubscriptionUsage{ID: 777, GroupID: 10}}
	// AccumulateUsage 现经 resolveMatchingQuota 解析额度（取 ModelPattern），需注入 sub/plan repo。
	plan := &BundlePlan{GroupQuotas: []BundlePlanGroupQuota{{GroupID: 10}}}
	sub := &BundleSubscription{PlanID: 1, Status: BundleStatusActive}
	svc := NewBundleUsageService(usageRepo, &fakeSubRepo{sub: sub}, &fakePlanRepo{plan: plan})

	// Act: accumulate with costUSD=0, 3 images, 0 videos.
	err := svc.AccumulateUsage(context.Background(), 1 /*subID*/, 10 /*groupID*/, 0.0, 3, 0)
	require.NoError(t, err)

	// Assert: IncrementUsage was called once with imageCount==3.
	require.Equal(t, 1, usageRepo.incrementCalls, "IncrementUsage should be called exactly once")
	require.Equal(t, 3, usageRepo.lastIncrementImageCnt, "IncrementUsage imageCount argument must equal 3")
	require.Equal(t, 0, usageRepo.lastIncrementVideoCnt, "IncrementUsage videoCount argument must equal 0")
	require.Equal(t, int64(777), usageRepo.lastIncrementID, "IncrementUsage id argument must match the pre-existing record ID")
}

// ──────────────────────────────────────────────────────
// Tests: ActivateBundle
// ──────────────────────────────────────────────────────

func TestBundleSubscriptionService_ActivateBundle_Success(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{activeBundles: nil} // no active bundle
	planRepo := &activateBundlePlanRepoStub{plan: sampleActivePlan()}
	usageRepo := &activateBundleUsageRepoStub{}
	userSubRepo := &activateUserSubRepoStub{}
	svc := newBundleSubSvc(subRepo, planRepo, usageRepo, userSubRepo)

	result, err := svc.ActivateBundle(context.Background(), &ActivateBundleRequest{
		UserID: 42,
		PlanID: 1,
		Source: BundleSourcePurchase,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(100), result.ID)
	require.Equal(t, int64(42), result.UserID)
	require.Equal(t, int64(1), result.PlanID)
	require.Equal(t, BundleStatusActive, result.Status)
	require.Equal(t, BundleSourcePurchase, result.Source)
	require.Equal(t, 10, result.ConcurrencyLimit)
	require.Equal(t, 120, result.RPMLimit)
	require.True(t, result.StartsAt.Before(time.Now()))
	require.True(t, result.ExpiresAt.After(time.Now()))

	// Verify usage trackers created (one per GroupQuota).
	require.Len(t, usageRepo.createdUsages, 2)
	require.Equal(t, int64(10), usageRepo.createdUsages[0].GroupID)
	require.Equal(t, int64(20), usageRepo.createdUsages[1].GroupID)
	// window_start 对齐到激活日（购买日）0 点：周/月窗口锚定购买日，重置落在「购买日+N 天」的 0 点。
	purchaseMidnight := timezone.StartOfDay(time.Now())
	require.Equal(t, purchaseMidnight, usageRepo.createdUsages[0].DailyWindowStart, "daily window_start must align to purchase-day 00:00")
	require.Equal(t, purchaseMidnight, usageRepo.createdUsages[0].WeeklyWindowStart, "weekly window_start must align to purchase-day 00:00")
	require.Equal(t, purchaseMidnight, usageRepo.createdUsages[0].MonthlyWindowStart, "monthly window_start must align to purchase-day 00:00")

	// Verify bridged UserSubscriptions created.
	require.Len(t, userSubRepo.createdSubs, 2)
	require.Equal(t, int64(42), userSubRepo.createdSubs[0].UserID)
	require.Equal(t, int64(10), userSubRepo.createdSubs[0].GroupID)
	require.Equal(t, int64(100), *userSubRepo.createdSubs[0].BundleSubscriptionID)
	require.Equal(t, 5.0, userSubRepo.createdSubs[0].DailyLimitUSD)
	require.Equal(t, domain.SubscriptionStatusActive, userSubRepo.createdSubs[0].Status)
	// count limit snapshotted symmetrically with USD limit.
	require.Equal(t, 50, userSubRepo.createdSubs[0].DailyImageLimitCount, "daily count limit must be snapshotted from plan quota")
	require.Equal(t, 250, userSubRepo.createdSubs[0].WeeklyImageLimitCount)
	require.Equal(t, 1000, userSubRepo.createdSubs[0].MonthlyImageLimitCount)
	require.Equal(t, 30, userSubRepo.createdSubs[1].DailyImageLimitCount)
	// video limit snapshotted symmetrically with image limit.
	require.Equal(t, 5, userSubRepo.createdSubs[0].DailyVideoLimitCount, "daily video limit must be snapshotted from plan quota")
	require.Equal(t, 25, userSubRepo.createdSubs[0].WeeklyVideoLimitCount)
	require.Equal(t, 100, userSubRepo.createdSubs[0].MonthlyVideoLimitCount)
}


// bundleKeyRebinderStub 记录 BundleKeyRebinder 调用，用于断言套餐切换时 APIKey 迁移 + 缓存失效。
type bundleKeyRebinderStub struct {
	rebindCalls     []bundleKeyRebindCall
	rebindErr       error
	invalidateCalls []int64
}

type bundleKeyRebindCall struct {
	userID      int64
	bundleSubID int64
}

func (s *bundleKeyRebinderStub) RebindUserBundleKeys(_ context.Context, userID, newBundleSubID int64) error {
	s.rebindCalls = append(s.rebindCalls, bundleKeyRebindCall{userID: userID, bundleSubID: newBundleSubID})
	return s.rebindErr
}

func (s *bundleKeyRebinderStub) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.invalidateCalls = append(s.invalidateCalls, userID)
}

// TestActivateBundle_RebindsAPIKeys 验证激活套餐后把用户的 bundle APIKey 迁移到新套餐 +
// 失效认证缓存（升级/换绑/重购三类切换统一在此触发），避免旧 key 指向失效 bundle 报 BUNDLE_EXPIRED。
func TestActivateBundle_RebindsAPIKeys(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{activeBundles: nil}
	planRepo := &activateBundlePlanRepoStub{plan: sampleActivePlan()}
	usageRepo := &activateBundleUsageRepoStub{}
	userSubRepo := &activateUserSubRepoStub{}
	rebinder := &bundleKeyRebinderStub{}
	svc := NewBundleSubscriptionService(subRepo, planRepo, usageRepo, userSubRepo, nil, nil, nil, rebinder)

	result, err := svc.ActivateBundle(context.Background(), &ActivateBundleRequest{
		UserID: 42, PlanID: 1, Source: BundleSourcePurchase,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, rebinder.rebindCalls, 1, "激活后应迁移该用户 bundle APIKey 到新套餐")
	require.Equal(t, int64(42), rebinder.rebindCalls[0].userID)
	require.Equal(t, result.ID, rebinder.rebindCalls[0].bundleSubID)
	require.Contains(t, rebinder.invalidateCalls, int64(42), "应失效该用户的 APIKey 认证缓存")
}

func TestBundleSubscriptionService_ActivateBundle_ConflictExistingBundle(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{
		activeBundles: []BundleSubscription{{ID: 50, UserID: 42, Status: BundleStatusActive}},
	}
	planRepo := &activateBundlePlanRepoStub{plan: sampleActivePlan()}
	usageRepo := &activateBundleUsageRepoStub{}
	userSubRepo := &activateUserSubRepoStub{}
	svc := newBundleSubSvc(subRepo, planRepo, usageRepo, userSubRepo)

	result, err := svc.ActivateBundle(context.Background(), &ActivateBundleRequest{
		UserID: 42,
		PlanID: 1,
		Source: BundleSourcePurchase,
	})

	require.ErrorIs(t, err, ErrBundleConflict)
	require.Nil(t, result)
	// Should not create any usage or user subscriptions.
	require.Empty(t, usageRepo.createdUsages)
	require.Empty(t, userSubRepo.createdSubs)
}

func TestBundleSubscriptionService_ActivateBundle_PlanNotFound(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{activeBundles: nil}
	planRepo := &activateBundlePlanRepoStub{getErr: ErrBundlePlanNotFound}
	usageRepo := &activateBundleUsageRepoStub{}
	userSubRepo := &activateUserSubRepoStub{}
	svc := newBundleSubSvc(subRepo, planRepo, usageRepo, userSubRepo)

	result, err := svc.ActivateBundle(context.Background(), &ActivateBundleRequest{
		UserID: 42,
		PlanID: 999,
		Source: BundleSourcePurchase,
	})

	require.Error(t, err)
	require.Nil(t, result)
}

func TestBundleSubscriptionService_ActivateBundle_PlanDisabled(t *testing.T) {
	plan := sampleActivePlan()
	plan.ForSale = false
	subRepo := &activateBundleSubRepoStub{activeBundles: nil}
	planRepo := &activateBundlePlanRepoStub{plan: plan}
	usageRepo := &activateBundleUsageRepoStub{}
	userSubRepo := &activateUserSubRepoStub{}
	svc := newBundleSubSvc(subRepo, planRepo, usageRepo, userSubRepo)

	result, err := svc.ActivateBundle(context.Background(), &ActivateBundleRequest{
		UserID: 42,
		PlanID: 1,
		Source: BundleSourceRedeem,
	})

	require.ErrorIs(t, err, ErrBundlePlanDisabled)
	require.Nil(t, result)
}

func TestBundleSubscriptionService_ActivateBundle_PlanStatusNotActive(t *testing.T) {
	plan := sampleActivePlan()
	plan.Status = "disabled"
	subRepo := &activateBundleSubRepoStub{activeBundles: nil}
	planRepo := &activateBundlePlanRepoStub{plan: plan}
	usageRepo := &activateBundleUsageRepoStub{}
	userSubRepo := &activateUserSubRepoStub{}
	svc := newBundleSubSvc(subRepo, planRepo, usageRepo, userSubRepo)

	result, err := svc.ActivateBundle(context.Background(), &ActivateBundleRequest{
		UserID: 42,
		PlanID: 1,
		Source: BundleSourceAdminAssign,
	})

	require.ErrorIs(t, err, ErrBundlePlanDisabled)
	require.Nil(t, result)
}

func TestBundleSubscriptionService_ActivateBundle_NilRequest(t *testing.T) {
	svc := newBundleSubSvc(&activateBundleSubRepoStub{}, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	result, err := svc.ActivateBundle(context.Background(), nil)

	require.ErrorIs(t, err, ErrBundleNotFound)
	require.Nil(t, result)
}

func TestBundleSubscriptionService_ActivateBundle_UsageCreateError(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{activeBundles: nil}
	planRepo := &activateBundlePlanRepoStub{plan: sampleActivePlan()}
	usageRepo := &activateBundleUsageRepoStub{createErr: errors.New("db error")}
	userSubRepo := &activateUserSubRepoStub{}
	svc := newBundleSubSvc(subRepo, planRepo, usageRepo, userSubRepo)

	result, err := svc.ActivateBundle(context.Background(), &ActivateBundleRequest{
		UserID: 42,
		PlanID: 1,
		Source: BundleSourcePurchase,
	})

	require.Error(t, err)
	require.Nil(t, result)
}

// ──────────────────────────────────────────────────────
// Tests: RevokeBundle
// ──────────────────────────────────────────────────────

func TestBundleSubscriptionService_RevokeBundle_Success(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{}
	subRepo.created = &BundleSubscription{ID: 100, Status: BundleStatusActive}
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	err := svc.RevokeBundle(context.Background(), 100)

	require.NoError(t, err)
}

func TestBundleSubscriptionService_RevokeBundle_NotActive(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{}
	subRepo.created = &BundleSubscription{ID: 100, Status: BundleStatusExpired}
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	err := svc.RevokeBundle(context.Background(), 100)

	require.ErrorIs(t, err, ErrBundleExpired)
}

func TestBundleSubscriptionService_RevokeBundle_NotFound(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{} // created is nil → GetByIDWithUsages returns ErrBundleNotFound
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	err := svc.RevokeBundle(context.Background(), 999)

	require.Error(t, err)
}

func TestBundleSubscriptionService_RevokeBundle_UpdateStatusError(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{updateStatusErr: errors.New("db error")}
	subRepo.created = &BundleSubscription{ID: 100, Status: BundleStatusActive}
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	err := svc.RevokeBundle(context.Background(), 100)

	require.Error(t, err)
}

// ──────────────────────────────────────────────────────
// Tests: ExtendBundle
// ──────────────────────────────────────────────────────

func TestBundleSubscriptionService_ExtendBundle_Success(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{}
	subRepo.created = &BundleSubscription{
		ID:        100,
		Status:    BundleStatusActive,
		ExpiresAt: time.Now().Add(5 * 24 * time.Hour),
	}
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	err := svc.ExtendBundle(context.Background(), 100, 10)

	require.NoError(t, err)
}

func TestBundleSubscriptionService_ExtendBundle_NotActive(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{}
	subRepo.created = &BundleSubscription{ID: 100, Status: BundleStatusRevoked, ExpiresAt: time.Now()}
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	err := svc.ExtendBundle(context.Background(), 100, 10)

	require.ErrorIs(t, err, ErrBundleExpired)
}

func TestBundleSubscriptionService_ExtendBundle_NotFound(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{} // created is nil
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	err := svc.ExtendBundle(context.Background(), 999, 10)

	require.Error(t, err)
}

func TestBundleSubscriptionService_ExtendBundle_ExtendByDaysError(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{extendByDaysErr: errors.New("db error")}
	subRepo.created = &BundleSubscription{ID: 100, Status: BundleStatusActive, ExpiresAt: time.Now().Add(24 * time.Hour)}
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	err := svc.ExtendBundle(context.Background(), 100, 10)

	require.Error(t, err)
}

// ──────────────────────────────────────────────────────
// Tests: GetBundleUsageProgress
// ──────────────────────────────────────────────────────

func TestBundleSubscriptionService_GetBundleUsageProgress_Success(t *testing.T) {
	bundleSubID := int64(100)
	today0 := timezone.StartOfDay(time.Now()) // 窗口未过期，累计应保留展示
	usages := []BundleSubscriptionUsage{
		{BundleSubscriptionID: 100, GroupID: 10, DailyWindowStart: today0, WeeklyWindowStart: today0, MonthlyWindowStart: today0, DailyUsageUSD: 2.5, WeeklyUsageUSD: 10.0, MonthlyUsageUSD: 40.0},
		{BundleSubscriptionID: 100, GroupID: 20, ModelPattern: "gpt-4*", DailyWindowStart: today0, WeeklyWindowStart: today0, MonthlyWindowStart: today0, DailyUsageUSD: 1.0, WeeklyUsageUSD: 5.0, MonthlyUsageUSD: 20.0},
	}

	subRepo := &activateBundleSubRepoStub{}
	subRepo.created = &BundleSubscription{ID: 100, UserID: 42, PlanID: 1, Status: BundleStatusActive, Usages: usages}

	// Bridged UserSubscriptions with snapshotted limits.
	userSubRepo := &activateUserSubRepoStub{}
	userSubRepo.existingSubs = []UserSubscription{
		{ID: 200, UserID: 42, GroupID: 10, BundleSubscriptionID: &bundleSubID, DailyLimitUSD: 5.0, WeeklyLimitUSD: 25.0, MonthlyLimitUSD: 100.0, DailyImageLimitCount: 50, WeeklyImageLimitCount: 250, MonthlyImageLimitCount: 1000},
		{ID: 201, UserID: 42, GroupID: 20, BundleSubscriptionID: &bundleSubID, DailyLimitUSD: 3.0, WeeklyLimitUSD: 15.0, MonthlyLimitUSD: 60.0, DailyImageLimitCount: 30, WeeklyImageLimitCount: 150, MonthlyImageLimitCount: 600},
	}

	// Plan repo returns a plan whose count quotas DIFFER from the snapshot — proves
	// count limit is read from the snapshot, not the live plan.
	planRepo := &activateBundlePlanRepoStub{plan: &BundlePlan{
		ID: 1, GroupQuotas: []BundlePlanGroupQuota{
			{GroupID: 10, DailyImageLimitCount: 9999},
			{GroupID: 20, DailyImageLimitCount: 9999},
		},
	}}

	svc := newBundleSubSvc(subRepo, planRepo, &activateBundleUsageRepoStub{}, userSubRepo)

	progress, err := svc.GetBundleUsageProgress(context.Background(), 100)

	require.NoError(t, err)
	require.Len(t, progress, 2)

	// First quota: group 10 - limits from bridged UserSubscription snapshot.
	require.Equal(t, int64(10), progress[0].GroupID)
	require.Equal(t, 2.5, progress[0].DailyUsageUSD)
	require.Equal(t, 5.0, progress[0].DailyLimitUSD)
	require.Equal(t, 10.0, progress[0].WeeklyUsageUSD)
	require.Equal(t, 25.0, progress[0].WeeklyLimitUSD)
	// count limit from snapshot, NOT live plan (9999).
	require.Equal(t, 50, progress[0].DailyImageLimitCount, "daily count limit must come from snapshot, not live plan")
	require.Equal(t, 250, progress[0].WeeklyImageLimitCount)
	require.Equal(t, 1000, progress[0].MonthlyImageLimitCount)

	// Second quota: group 20 - model-level.
	require.Equal(t, int64(20), progress[1].GroupID)
	require.Equal(t, "gpt-4*", progress[1].ModelPattern)
	require.Equal(t, 1.0, progress[1].DailyUsageUSD)
	require.Equal(t, 3.0, progress[1].DailyLimitUSD)
	require.Equal(t, 30, progress[1].DailyImageLimitCount, "model-level count limit from snapshot")
}

func TestBundleSubscriptionService_GetBundleUsageProgress_SubscriptionNotFound(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{} // created is nil
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	progress, err := svc.GetBundleUsageProgress(context.Background(), 999)

	require.Error(t, err)
	require.Nil(t, progress)
}

// TestBundleSubscriptionService_GetBundleUsageProgress_ZeroesExpiredWindows 守护前端展示死锁回归：
// 窗口已过期的累计必须在响应中归零展示，避免用户次日打开界面仍看到昨天满额误判达限额。
// 与 CheckQuotaEligibility / IncrementUsage 共用 rolledBundleUsage 的过期判定。
func TestBundleSubscriptionService_GetBundleUsageProgress_ZeroesExpiredWindows(t *testing.T) {
	bundleSubID := int64(100)
	// 30 天前的 window_start → 日/周/月三窗口全部过期。
	monthAgo := timezone.StartOfDay(time.Now()).AddDate(0, 0, -30)
	usages := []BundleSubscriptionUsage{
		{
			BundleSubscriptionID: 100, GroupID: 10,
			DailyWindowStart: monthAgo, WeeklyWindowStart: monthAgo, MonthlyWindowStart: monthAgo,
			DailyUsageUSD: 5.0, WeeklyUsageUSD: 7.0, MonthlyUsageUSD: 9.0,
		},
	}

	subRepo := &activateBundleSubRepoStub{}
	subRepo.created = &BundleSubscription{ID: 100, UserID: 42, PlanID: 1, Status: BundleStatusActive, Usages: usages}

	userSubRepo := &activateUserSubRepoStub{}
	userSubRepo.existingSubs = []UserSubscription{
		{ID: 200, UserID: 42, GroupID: 10, BundleSubscriptionID: &bundleSubID, DailyLimitUSD: 10.0, WeeklyLimitUSD: 10.0, MonthlyLimitUSD: 10.0},
	}

	planRepo := &activateBundlePlanRepoStub{plan: &BundlePlan{ID: 1, GroupQuotas: []BundlePlanGroupQuota{{GroupID: 10}}}}
	svc := newBundleSubSvc(subRepo, planRepo, &activateBundleUsageRepoStub{}, userSubRepo)

	progress, err := svc.GetBundleUsageProgress(context.Background(), 100)

	require.NoError(t, err)
	require.Len(t, progress, 1)
	require.Equal(t, 0.0, progress[0].DailyUsageUSD, "expired daily window should display as 0")
	require.Equal(t, 0.0, progress[0].WeeklyUsageUSD, "expired weekly window should display as 0")
	require.Equal(t, 0.0, progress[0].MonthlyUsageUSD, "expired monthly window should display as 0")
}

// TestBundleSubscriptionService_GetBundleUsageProgress_Ordering 守护各 group 用量卡片的统一排序：
// repo GetByIDWithUsages 对 Usages edge 无 ORDER BY（PG 不保证返回序），service 层必须集中排序，
// 否则前端卡片顺序随写入/物理序漂移。验证规则：platform↑ → group_name(拼音/字母)↑ →
// model_pattern(空=整组通配优先) → quota_scope(日<周<月)。
func TestBundleSubscriptionService_GetBundleUsageProgress_Ordering(t *testing.T) {
	bundleSubID := int64(100)
	today0 := timezone.StartOfDay(time.Now())

	// 故意按 ID 乱序输入（20,15,40,30,50,10），排序后应得到稳定可读顺序。
	usages := []BundleSubscriptionUsage{
		{BundleSubscriptionID: 100, GroupID: 20, ModelPattern: "gpt-4*", DailyWindowStart: today0, WeeklyWindowStart: today0, MonthlyWindowStart: today0},
		{BundleSubscriptionID: 100, GroupID: 15, DailyWindowStart: today0, WeeklyWindowStart: today0, MonthlyWindowStart: today0},
		{BundleSubscriptionID: 100, GroupID: 40, ModelPattern: "gemini-*", DailyWindowStart: today0, WeeklyWindowStart: today0, MonthlyWindowStart: today0},
		{BundleSubscriptionID: 100, GroupID: 30, DailyWindowStart: today0, WeeklyWindowStart: today0, MonthlyWindowStart: today0},
		{BundleSubscriptionID: 100, GroupID: 50, ModelPattern: "gemini-1.5*", DailyWindowStart: today0, WeeklyWindowStart: today0, MonthlyWindowStart: today0},
		{BundleSubscriptionID: 100, GroupID: 10, DailyWindowStart: today0, WeeklyWindowStart: today0, MonthlyWindowStart: today0},
	}

	subRepo := &activateBundleSubRepoStub{}
	subRepo.created = &BundleSubscription{ID: 100, UserID: 42, PlanID: 1, Status: BundleStatusActive, Usages: usages}

	userSubRepo := &activateUserSubRepoStub{}
	userSubRepo.existingSubs = []UserSubscription{
		{ID: 1, UserID: 42, GroupID: 10, BundleSubscriptionID: &bundleSubID, Group: &Group{Name: "Claude Pro", Platform: "anthropic"}},
		{ID: 2, UserID: 42, GroupID: 15, BundleSubscriptionID: &bundleSubID, Group: &Group{Name: "Claude Basic", Platform: "anthropic"}},
		{ID: 3, UserID: 42, GroupID: 20, BundleSubscriptionID: &bundleSubID, Group: &Group{Name: "GPT组", Platform: "openai"}},
		{ID: 4, UserID: 42, GroupID: 30, BundleSubscriptionID: &bundleSubID, Group: &Group{Name: "GPT组", Platform: "openai"}},
		{ID: 5, UserID: 42, GroupID: 40, BundleSubscriptionID: &bundleSubID, Group: &Group{Platform: "gemini"}}, // 空 group_name
		{ID: 6, UserID: 42, GroupID: 50, BundleSubscriptionID: &bundleSubID, Group: &Group{Platform: "gemini"}}, // 空 group_name
	}

	planRepo := &activateBundlePlanRepoStub{plan: &BundlePlan{ID: 1, GroupQuotas: []BundlePlanGroupQuota{
		{GroupID: 10, ModelPattern: "", QuotaScope: "daily"},
		{GroupID: 15, ModelPattern: "", QuotaScope: "daily"},
		{GroupID: 20, ModelPattern: "gpt-4*", QuotaScope: "monthly"},
		{GroupID: 30, ModelPattern: "", QuotaScope: "weekly"},
		{GroupID: 40, ModelPattern: "gemini-*", QuotaScope: "daily"},
		{GroupID: 50, ModelPattern: "gemini-1.5*", QuotaScope: "daily"},
	}}}

	svc := newBundleSubSvc(subRepo, planRepo, &activateBundleUsageRepoStub{}, userSubRepo)

	progress, err := svc.GetBundleUsageProgress(context.Background(), 100)
	require.NoError(t, err)
	require.Len(t, progress, 6)

	// 期望：anthropic(字母序 Basic<Pro) → gemini(空名退回 group_id 40<50) → openai(空 pattern 排前)。
	wantGroupIDs := []int64{15, 10, 40, 50, 30, 20}
	gotIDs := make([]int64, len(progress))
	for i, p := range progress {
		gotIDs[i] = p.GroupID
	}
	require.Equal(t, wantGroupIDs, gotIDs, "usage progress must be sorted by platform→name→pattern→scope")

	// 抽查：platform 升序聚类（anthropic < gemini < openai）。
	require.Equal(t, "anthropic", progress[0].Platform)
	require.Equal(t, "gemini", progress[2].Platform)
	require.Equal(t, "openai", progress[4].Platform)

	// 抽查：同平台内英文分组名按字母序——Claude Basic 排在 Claude Pro 前。
	require.Equal(t, "Claude Basic", progress[0].GroupName)
	require.Equal(t, "Claude Pro", progress[1].GroupName)

	// 抽查：同名同平台分组内，空 model_pattern（整组通配）排在具体 pattern 之前。
	require.Equal(t, "", progress[4].ModelPattern, "empty model_pattern (group-wide) sorts first within same group")
	require.Equal(t, "gpt-4*", progress[5].ModelPattern)
}

// ──────────────────────────────────────────────────────
// Tests: RevokeBundle syncs bridged UserSubscriptions
// ──────────────────────────────────────────────────────

func TestBundleSubscriptionService_RevokeBundle_SyncsBridgedUserSubs(t *testing.T) {
	bundleSubID := int64(100)
	subRepo := &activateBundleSubRepoStub{}
	subRepo.created = &BundleSubscription{ID: 100, UserID: 42, Status: BundleStatusActive}

	userSubRepo := &activateUserSubRepoStub{}
	userSubRepo.existingSubs = []UserSubscription{
		{ID: 200, UserID: 42, GroupID: 10, BundleSubscriptionID: &bundleSubID},
		{ID: 201, UserID: 42, GroupID: 20, BundleSubscriptionID: &bundleSubID},
		{ID: 300, UserID: 42, GroupID: 30}, // not a bundle sub, should be skipped
	}

	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, userSubRepo)

	err := svc.RevokeBundle(context.Background(), 100)

	require.NoError(t, err)
	// Should have soft-deleted the 2 bridged subs only（RevokeBundle 软删除释放唯一槽）。
	require.Len(t, userSubRepo.deletedIDs, 2)
	require.Contains(t, userSubRepo.deletedIDs, int64(200))
	require.Contains(t, userSubRepo.deletedIDs, int64(201))
}

// ──────────────────────────────────────────────────────
// Tests: ExtendBundle syncs bridged UserSubscriptions
// ──────────────────────────────────────────────────────

func TestBundleSubscriptionService_ExtendBundle_SyncsBridgedUserSubs(t *testing.T) {
	bundleSubID := int64(100)
	subRepo := &activateBundleSubRepoStub{}
	subRepo.created = &BundleSubscription{
		ID:        100,
		UserID:    42,
		Status:    BundleStatusActive,
		ExpiresAt: time.Now().Add(5 * 24 * time.Hour),
	}

	userSubRepo := &activateUserSubRepoStub{}
	userSubRepo.existingSubs = []UserSubscription{
		{ID: 200, UserID: 42, GroupID: 10, BundleSubscriptionID: &bundleSubID, ExpiresAt: time.Now().Add(5 * 24 * time.Hour)},
		{ID: 201, UserID: 42, GroupID: 20, BundleSubscriptionID: &bundleSubID, ExpiresAt: time.Now().Add(5 * 24 * time.Hour)},
	}

	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, userSubRepo)

	err := svc.ExtendBundle(context.Background(), 100, 10)

	require.NoError(t, err)
	// Should have extended the 2 bridged subs via incremental ExtendExpiryByDays (中危1).
	require.Len(t, userSubRepo.extendedByDaysIDs, 2)
	require.Contains(t, userSubRepo.extendedByDaysIDs, int64(200))
	require.Contains(t, userSubRepo.extendedByDaysIDs, int64(201))
}
func (s *activateUserSubRepoStub) ExpireBridgedSubscriptionsForExpiredBundles(ctx context.Context) (int64, error) {
	return 0, nil
}

// TestExtendBundle_UsesIncrementalExpiry 守护中危1：ExtendBundle 必须用增量延期
// (ExtendExpiryByDays) 而非基于事务外快照的覆盖写 (UpdateExpiry)。
//
// 旧实现先在事务外 GetByID 读 ExpiresAt 快照、算 newExpiry=ExpiresAt+days、再事务内
// SetExpiresAt(newExpiry) 覆盖写——两个并发延期各自基于同一快照算出相同 newExpiry，
// 互相覆盖，丢失一次延期（lost update）。改用 DB 原子加 expires_at = expires_at + interval
// 后，并发延期在 DB 层原子累加，不再丢失。
func TestExtendBundle_UsesIncrementalExpiry(t *testing.T) {
	bundleSubID := int64(100)
	subRepo := &activateBundleSubRepoStub{}
	subRepo.created = &BundleSubscription{
		ID:        100,
		UserID:    42,
		Status:    BundleStatusActive,
		ExpiresAt: time.Now().Add(5 * 24 * time.Hour),
	}
	userSubRepo := &activateUserSubRepoStub{
		existingSubs: []UserSubscription{{ID: 200, UserID: 42, BundleSubscriptionID: &bundleSubID, ExpiresAt: time.Now().Add(5 * 24 * time.Hour)}},
	}
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, userSubRepo)

	err := svc.ExtendBundle(context.Background(), 100, 10)
	require.NoError(t, err)

	require.Contains(t, subRepo.extendedByDaysIDs, int64(100),
		"ExtendBundle must call ExtendExpiryByDays (atomic increment) on the bundle subscription")
	require.False(t, subRepo.updateExpiryCalled,
		"ExtendBundle must NOT use UpdateExpiry (overwrite) — overwrite from a stale snapshot causes lost update on concurrent extends")
	require.Contains(t, userSubRepo.extendedByDaysIDs, int64(200),
		"ExtendBundle must call ExtendExpiryByDays on bridged UserSubscriptions too")
}
