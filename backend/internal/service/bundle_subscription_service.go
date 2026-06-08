package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
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
func (s *BundleSubscriptionService) GetBundleUsageProgress(ctx context.Context, bundleSubID int64) ([]BundleUsageProgress, error) {
	bundleSub, err := s.bundleSubRepo.GetByIDWithUsages(ctx, bundleSubID)
	if err != nil {
		return nil, fmt.Errorf("load bundle subscription: %w", err)
	}

	plan, err := s.planRepo.GetByID(ctx, bundleSub.PlanID)
	if err != nil {
		return nil, fmt.Errorf("load bundle plan: %w", err)
	}

	// Build a lookup for group quotas by groupID.
	quotaMap := make(map[int64]BundlePlanGroupQuota)
	for _, gq := range plan.GroupQuotas {
		quotaMap[gq.GroupID] = gq
	}

	progress := make([]BundleUsageProgress, 0, len(bundleSub.Usages))
	for _, usage := range bundleSub.Usages {
		quota, ok := quotaMap[usage.GroupID]
		if !ok {
			continue
		}
		progress = append(progress, BundleUsageProgress{
			GroupID:         usage.GroupID,
			QuotaScope:      quota.QuotaScope,
			ModelPattern:    usage.ModelPattern,
			DailyUsageUSD:   usage.DailyUsageUSD,
			DailyLimitUSD:   quota.DailyLimitUSD,
			WeeklyUsageUSD:  usage.WeeklyUsageUSD,
			WeeklyLimitUSD:  quota.WeeklyLimitUSD,
			MonthlyUsageUSD: usage.MonthlyUsageUSD,
			MonthlyLimitUSD: quota.MonthlyLimitUSD,
		})
	}
	return progress, nil
}
