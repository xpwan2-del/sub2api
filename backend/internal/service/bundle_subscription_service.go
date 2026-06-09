package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// BundleSubscriptionService handles bundle subscription lifecycle.
type BundleSubscriptionService struct {
	bundleSubRepo   BundleSubscriptionRepository
	planRepo        BundlePlanRepository
	usageRepo       BundleUsageRepository
	userSubRepo     UserSubscriptionRepository
}

// NewBundleSubscriptionService creates a new BundleSubscriptionService.
func NewBundleSubscriptionService(
	bundleSubRepo BundleSubscriptionRepository,
	planRepo BundlePlanRepository,
	usageRepo BundleUsageRepository,
	userSubRepo UserSubscriptionRepository,
) *BundleSubscriptionService {
	return &BundleSubscriptionService{
		bundleSubRepo: bundleSubRepo,
		planRepo:      planRepo,
		usageRepo:     usageRepo,
		userSubRepo:   userSubRepo,
	}
}

// ActivateBundleRequest is the input DTO for activating a bundle for a user.
type ActivateBundleRequest struct {
	UserID int64
	PlanID int64
	Source string // purchase, redeem, admin_assign
}

// ActivateBundle creates a bundle subscription and bridges per-group UserSubscriptions.
func (s *BundleSubscriptionService) ActivateBundle(ctx context.Context, req *ActivateBundleRequest) (*BundleSubscription, error) {
	if req == nil {
		return nil, ErrBundleNotFound
	}

	// 1. Check user has no active bundle subscription.
	activeBundles, err := s.bundleSubRepo.GetActiveByUserID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("check active bundles: %w", err)
	}
	if len(activeBundles) > 0 {
		return nil, ErrBundleConflict
	}

	// 2. Load plan and validate.
	plan, err := s.planRepo.GetByID(ctx, req.PlanID)
	if err != nil {
		return nil, fmt.Errorf("load bundle plan: %w", err)
	}
	if !plan.ForSale || plan.Status != domain.StatusActive {
		return nil, ErrBundlePlanDisabled
	}

	// 3. Create BundleSubscription with snapshot concurrency/rpm.
	now := time.Now()
	expiresAt := now.AddDate(0, 0, plan.ValidityDays)

	bundleSub := &BundleSubscription{
		UserID:           req.UserID,
		PlanID:           req.PlanID,
		Status:           BundleStatusActive,
		StartsAt:         now,
		ExpiresAt:        expiresAt,
		ConcurrencyLimit: plan.ConcurrencyLimit,
		RPMLimit:         plan.RPMLimit,
		Source:           req.Source,
		Usages:           make([]BundleSubscriptionUsage, 0, len(plan.GroupQuotas)),
	}

	if err := s.bundleSubRepo.Create(ctx, bundleSub); err != nil {
		return nil, fmt.Errorf("create bundle subscription: %w", err)
	}

	// 4. For each GroupQuota, create BundleSubscriptionUsage + bridge UserSubscription.
	for _, gq := range plan.GroupQuotas {
		// Create usage tracker.
		usage := &BundleSubscriptionUsage{
			BundleSubscriptionID: bundleSub.ID,
			GroupID:              gq.GroupID,
			ModelPattern:         gq.ModelPattern,
			DailyWindowStart:     now,
			WeeklyWindowStart:    now,
			MonthlyWindowStart:   now,
		}
		if err := s.usageRepo.Create(ctx, usage); err != nil {
			return nil, fmt.Errorf("create bundle usage for group %d: %w", gq.GroupID, err)
		}
		bundleSub.Usages = append(bundleSub.Usages, *usage)

		// Bridge: create UserSubscription linked to this bundle.
		bundleSubID := bundleSub.ID
		userSub := &UserSubscription{
			UserID:               req.UserID,
			GroupID:              gq.GroupID,
			StartsAt:             now,
			ExpiresAt:            expiresAt,
			Status:               domain.SubscriptionStatusActive,
			DailyUsageUSD:        0,
			WeeklyUsageUSD:       0,
			MonthlyUsageUSD:      0,
			BundleSubscriptionID: &bundleSubID,
			DailyLimitUSD:        gq.DailyLimitUSD,
			WeeklyLimitUSD:       gq.WeeklyLimitUSD,
			MonthlyLimitUSD:      gq.MonthlyLimitUSD,
			Notes:                fmt.Sprintf("Bridged from bundle plan %q (ID:%d)", plan.Name, plan.ID),
		}
		if err := s.userSubRepo.Create(ctx, userSub); err != nil {
			return nil, fmt.Errorf("bridge user subscription for group %d: %w", gq.GroupID, err)
		}
	}

	return bundleSub, nil
}

// RevokeBundle revokes an active bundle subscription and its bridged UserSubscriptions.
func (s *BundleSubscriptionService) RevokeBundle(ctx context.Context, bundleSubID int64) error {
	bundleSub, err := s.bundleSubRepo.GetByIDWithUsages(ctx, bundleSubID)
	if err != nil {
		return fmt.Errorf("load bundle subscription: %w", err)
	}
	if bundleSub.Status != BundleStatusActive {
		return ErrBundleExpired
	}

	if err := s.bundleSubRepo.UpdateStatus(ctx, bundleSubID, BundleStatusRevoked); err != nil {
		return fmt.Errorf("revoke bundle subscription: %w", err)
	}

	// Sync: revoke bridged UserSubscriptions.
	s.syncBridgedUserSubscriptions(ctx, bundleSub.UserID, bundleSubID, func(sub *UserSubscription) error {
		return s.userSubRepo.UpdateStatus(ctx, sub.ID, domain.SubscriptionStatusExpired)
	})

	return nil
}

// GetUserActiveBundle returns the active bundle subscription for a user.
func (s *BundleSubscriptionService) GetUserActiveBundle(ctx context.Context, userID int64) ([]BundleSubscription, error) {
	subs, err := s.bundleSubRepo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get active bundles: %w", err)
	}
	return subs, nil
}

// GetBundleUsageProgress returns usage progress for a bundle subscription.
// Limits are read from the bridged UserSubscriptions (snapshotted at activation time)
// rather than the latest plan, ensuring consistency with the actual active limits.
func (s *BundleSubscriptionService) GetBundleUsageProgress(ctx context.Context, bundleSubID int64) ([]BundleUsageProgress, error) {
	bundleSub, err := s.bundleSubRepo.GetByIDWithUsages(ctx, bundleSubID)
	if err != nil {
		return nil, fmt.Errorf("load bundle subscription: %w", err)
	}

	// Load bridged UserSubscriptions to get snapshotted limits per group.
	userSubs, err := s.userSubRepo.ListByUserID(ctx, bundleSub.UserID)
	if err != nil {
		return nil, fmt.Errorf("load user subscriptions: %w", err)
	}

	// Build a lookup for snapshotted limits by groupID from bridged UserSubscriptions.
	type groupLimit struct {
		dailyLimit   float64
		weeklyLimit  float64
		monthlyLimit float64
	}
	limitMap := make(map[int64]groupLimit)
	for _, sub := range userSubs {
		if sub.BundleSubscriptionID != nil && *sub.BundleSubscriptionID == bundleSubID {
			limitMap[sub.GroupID] = groupLimit{
				dailyLimit:   sub.DailyLimitUSD,
				weeklyLimit:  sub.WeeklyLimitUSD,
				monthlyLimit: sub.MonthlyLimitUSD,
			}
		}
	}

	progress := make([]BundleUsageProgress, 0, len(bundleSub.Usages))
	for _, usage := range bundleSub.Usages {
		lim, hasLimit := limitMap[usage.GroupID]
		if !hasLimit {
			lim = groupLimit{} // zero limits = unlimited
		}
		progress = append(progress, BundleUsageProgress{
			GroupID:         usage.GroupID,
			ModelPattern:    usage.ModelPattern,
			DailyUsageUSD:   usage.DailyUsageUSD,
			DailyLimitUSD:   lim.dailyLimit,
			WeeklyUsageUSD:  usage.WeeklyUsageUSD,
			WeeklyLimitUSD:  lim.weeklyLimit,
			MonthlyUsageUSD: usage.MonthlyUsageUSD,
			MonthlyLimitUSD: lim.monthlyLimit,
		})
	}
	return progress, nil
}

// List returns a paginated list of bundle subscriptions with optional filters.
func (s *BundleSubscriptionService) List(ctx context.Context, params pagination.PaginationParams, userID *int64, status string) ([]BundleSubscription, *pagination.PaginationResult, error) {
	subs, result, err := s.bundleSubRepo.List(ctx, params, userID, status)
	if err != nil {
		return nil, nil, fmt.Errorf("list bundle subscriptions: %w", err)
	}
	return subs, result, nil
}

// ExtendBundle extends a bundle subscription's expiry by the given number of days.
func (s *BundleSubscriptionService) ExtendBundle(ctx context.Context, bundleSubID int64, days int) error {
	bundleSub, err := s.bundleSubRepo.GetByID(ctx, bundleSubID)
	if err != nil {
		return fmt.Errorf("load bundle subscription: %w", err)
	}
	if bundleSub.Status != BundleStatusActive {
		return ErrBundleExpired
	}

	newExpiry := bundleSub.ExpiresAt.AddDate(0, 0, days)
	if err := s.bundleSubRepo.UpdateExpiry(ctx, bundleSubID, newExpiry); err != nil {
		return fmt.Errorf("extend bundle subscription: %w", err)
	}

	// Sync: extend bridged UserSubscriptions' expiry.
	s.syncBridgedUserSubscriptions(ctx, bundleSub.UserID, bundleSubID, func(sub *UserSubscription) error {
		extendedExpiry := sub.ExpiresAt.AddDate(0, 0, days)
		return s.userSubRepo.ExtendExpiry(ctx, sub.ID, extendedExpiry)
	})

	return nil
}

// syncBridgedUserSubscriptions finds all bridged UserSubscriptions for a bundle
// and applies the given mutation function to each one.
func (s *BundleSubscriptionService) syncBridgedUserSubscriptions(ctx context.Context, userID, bundleSubID int64, mutFn func(*UserSubscription) error) {
	userSubs, err := s.userSubRepo.ListByUserID(ctx, userID)
	if err != nil {
		return
	}
	for i := range userSubs {
		sub := &userSubs[i]
		if sub.BundleSubscriptionID != nil && *sub.BundleSubscriptionID == bundleSubID {
			_ = mutFn(sub)
		}
	}
}
