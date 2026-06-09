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

type bundleUsageRepoNoop struct{}

func (bundleUsageRepoNoop) GetBySubscriptionAndGroup(context.Context, int64, int64, string) (*BundleSubscriptionUsage, error) {
	panic("unexpected GetBySubscriptionAndGroup call")
}
func (bundleUsageRepoNoop) Create(context.Context, *BundleSubscriptionUsage) error {
	panic("unexpected Create call")
}
func (bundleUsageRepoNoop) IncrementUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementUsage call")
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

	activeBundles []BundleSubscription
	created       *BundleSubscription
	createErr     error
	updateStatusErr error
	updateExpiryErr error
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
	return s.updateExpiryErr
}

// activateBundlePlanRepoStub supports GetByID for plan loading.
type activateBundlePlanRepoStub struct {
	bundlePlanRepoNoop

	plan    *BundlePlan
	getErr  error
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

// activateUserSubRepoStub supports Create, ListByUserID, UpdateStatus, ExtendExpiry.
type activateUserSubRepoStub struct {
	userSubRepoNoop

	createdSubs     []UserSubscription
	existingSubs    []UserSubscription // pre-existing subs for ListByUserID
	createErr       error
	updateStatusErr error
	extendExpiryErr error

	updatedStatusIDs []int64
	extendedIDs      []int64
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

// ──────────────────────────────────────────────────────
// Helper constructors
// ──────────────────────────────────────────────────────

func newBundleSubSvc(
	subRepo BundleSubscriptionRepository,
	planRepo BundlePlanRepository,
	usageRepo BundleUsageRepository,
	userSubRepo UserSubscriptionRepository,
) *BundleSubscriptionService {
	return NewBundleSubscriptionService(subRepo, planRepo, usageRepo, userSubRepo, nil) // nil cache for unit tests
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
			{GroupID: 10, QuotaScope: QuotaScopePlatform, DailyLimitUSD: 5.0, WeeklyLimitUSD: 25.0, MonthlyLimitUSD: 100.0},
			{GroupID: 20, QuotaScope: QuotaScopeModel, ModelPattern: "gpt-4*", DailyLimitUSD: 3.0, WeeklyLimitUSD: 15.0, MonthlyLimitUSD: 60.0},
		},
	}
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

	// Verify bridged UserSubscriptions created.
	require.Len(t, userSubRepo.createdSubs, 2)
	require.Equal(t, int64(42), userSubRepo.createdSubs[0].UserID)
	require.Equal(t, int64(10), userSubRepo.createdSubs[0].GroupID)
	require.Equal(t, int64(100), *userSubRepo.createdSubs[0].BundleSubscriptionID)
	require.Equal(t, 5.0, userSubRepo.createdSubs[0].DailyLimitUSD)
	require.Equal(t, domain.SubscriptionStatusActive, userSubRepo.createdSubs[0].Status)
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

func TestBundleSubscriptionService_ExtendBundle_UpdateExpiryError(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{updateExpiryErr: errors.New("db error")}
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
	usages := []BundleSubscriptionUsage{
		{BundleSubscriptionID: 100, GroupID: 10, DailyUsageUSD: 2.5, WeeklyUsageUSD: 10.0, MonthlyUsageUSD: 40.0},
		{BundleSubscriptionID: 100, GroupID: 20, ModelPattern: "gpt-4*", DailyUsageUSD: 1.0, WeeklyUsageUSD: 5.0, MonthlyUsageUSD: 20.0},
	}

	subRepo := &activateBundleSubRepoStub{}
	subRepo.created = &BundleSubscription{ID: 100, UserID: 42, PlanID: 1, Status: BundleStatusActive, Usages: usages}

	// Bridged UserSubscriptions with snapshotted limits.
	userSubRepo := &activateUserSubRepoStub{}
	userSubRepo.existingSubs = []UserSubscription{
		{ID: 200, UserID: 42, GroupID: 10, BundleSubscriptionID: &bundleSubID, DailyLimitUSD: 5.0, WeeklyLimitUSD: 25.0, MonthlyLimitUSD: 100.0},
		{ID: 201, UserID: 42, GroupID: 20, BundleSubscriptionID: &bundleSubID, DailyLimitUSD: 3.0, WeeklyLimitUSD: 15.0, MonthlyLimitUSD: 60.0},
	}

	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, userSubRepo)

	progress, err := svc.GetBundleUsageProgress(context.Background(), 100)

	require.NoError(t, err)
	require.Len(t, progress, 2)

	// First quota: group 10 - limits from bridged UserSubscription snapshot.
	require.Equal(t, int64(10), progress[0].GroupID)
	require.Equal(t, 2.5, progress[0].DailyUsageUSD)
	require.Equal(t, 5.0, progress[0].DailyLimitUSD)
	require.Equal(t, 10.0, progress[0].WeeklyUsageUSD)
	require.Equal(t, 25.0, progress[0].WeeklyLimitUSD)

	// Second quota: group 20 - model-level.
	require.Equal(t, int64(20), progress[1].GroupID)
	require.Equal(t, "gpt-4*", progress[1].ModelPattern)
	require.Equal(t, 1.0, progress[1].DailyUsageUSD)
	require.Equal(t, 3.0, progress[1].DailyLimitUSD)
}

func TestBundleSubscriptionService_GetBundleUsageProgress_SubscriptionNotFound(t *testing.T) {
	subRepo := &activateBundleSubRepoStub{} // created is nil
	svc := newBundleSubSvc(subRepo, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	progress, err := svc.GetBundleUsageProgress(context.Background(), 999)

	require.Error(t, err)
	require.Nil(t, progress)
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
	// Should have updated status for the 2 bridged subs only.
	require.Len(t, userSubRepo.updatedStatusIDs, 2)
	require.Contains(t, userSubRepo.updatedStatusIDs, int64(200))
	require.Contains(t, userSubRepo.updatedStatusIDs, int64(201))
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
	// Should have extended the 2 bridged subs.
	require.Len(t, userSubRepo.extendedIDs, 2)
	require.Contains(t, userSubRepo.extendedIDs, int64(200))
	require.Contains(t, userSubRepo.extendedIDs, int64(201))
}
