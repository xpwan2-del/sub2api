// bundle_usage_service.go 套餐用量服务实现
// 提供用量累加（AccumulateUsage）和额度资格检查（CheckQuotaEligibility）。
// 用量按日/周/月三个周期独立跟踪，支持平台级和模型级两种粒度。

package service

import (
	"context"
	"fmt"
)

// BundleUsageService 套餐用量服务，处理用量累加和额度检查
// BundleUsageService handles usage accumulation and quota eligibility checks.
type BundleUsageService struct {
	usageRepo     BundleUsageRepository
	bundleSubRepo BundleSubscriptionRepository
	planRepo      BundlePlanRepository
}

// NewBundleUsageService 创建套餐用量服务实例
// NewBundleUsageService creates a new BundleUsageService.
func NewBundleUsageService(
	usageRepo BundleUsageRepository,
	bundleSubRepo BundleSubscriptionRepository,
	planRepo BundlePlanRepository,
) *BundleUsageService {
	return &BundleUsageService{
		usageRepo:     usageRepo,
		bundleSubRepo: bundleSubRepo,
		planRepo:      planRepo,
	}
}

// AccumulateUsage 累加套餐订阅在指定渠道组上的用量（USD + 次数）
// AccumulateUsage increments the usage counters (USD and request count) for a
// bundle subscription + group. count is the number of billable media outputs
// (e.g. generated images / video segments) produced by this request.
func (s *BundleUsageService) AccumulateUsage(ctx context.Context, bundleSubID, groupID int64, costUSD float64, count int) error {
	// Find the usage record for this subscription + group.
	usage, err := s.usageRepo.GetBySubscriptionAndGroup(ctx, bundleSubID, groupID, "")
	if err != nil {
		return fmt.Errorf("find bundle usage: %w", err)
	}
	if usage == nil {
		return ErrBundleNotFound
	}

	if err := s.usageRepo.IncrementUsage(ctx, usage.ID, costUSD, count); err != nil {
		return fmt.Errorf("increment bundle usage: %w", err)
	}
	return nil
}

// QuotaEligibilityResult 额度检查结果，包含是否可用和各周期剩余额度
// QuotaEligibilityResult holds the result of a quota eligibility check.
type QuotaEligibilityResult struct {
	Eligible              bool
	DailyRemaining        float64
	DailyRemainingCount   int
	WeeklyRemaining       float64
	WeeklyRemainingCount  int
	MonthlyRemaining      float64
	MonthlyRemainingCount int
}

// CheckQuotaEligibility 检查套餐订阅在指定渠道组上是否还有剩余额度
// CheckQuotaEligibility checks whether the bundle subscription has remaining quota
// for the given group. Returns eligibility result with remaining amounts.
func (s *BundleUsageService) CheckQuotaEligibility(ctx context.Context, bundleSubID, groupID int64) (*QuotaEligibilityResult, error) {
	// Load bundle subscription.
	bundleSub, err := s.bundleSubRepo.GetByID(ctx, bundleSubID)
	if err != nil {
		return nil, fmt.Errorf("load bundle subscription: %w", err)
	}
	if bundleSub.Status != BundleStatusActive {
		return nil, ErrBundleExpired
	}

	// Load plan for limits.
	plan, err := s.planRepo.GetByID(ctx, bundleSub.PlanID)
	if err != nil {
		return nil, fmt.Errorf("load bundle plan: %w", err)
	}

	// Find matching group quota.
	var matchingQuota *BundlePlanGroupQuota
	for i := range plan.GroupQuotas {
		if plan.GroupQuotas[i].GroupID == groupID {
			matchingQuota = &plan.GroupQuotas[i]
			break
		}
	}
	if matchingQuota == nil {
		return nil, ErrBundleGroupQuotaExceeded
	}

	// Load current usage.
	usage, err := s.usageRepo.GetBySubscriptionAndGroup(ctx, bundleSubID, groupID, matchingQuota.ModelPattern)
	if err != nil {
		return nil, fmt.Errorf("load bundle usage: %w", err)
	}

	result := &QuotaEligibilityResult{
		Eligible: true,
	}

	if usage != nil {
		result.DailyRemaining = matchingQuota.DailyLimitUSD - usage.DailyUsageUSD
		result.WeeklyRemaining = matchingQuota.WeeklyLimitUSD - usage.WeeklyUsageUSD
		result.MonthlyRemaining = matchingQuota.MonthlyLimitUSD - usage.MonthlyUsageUSD

		// 0 means unlimited — only enforce limits that are explicitly set (>0).
		if matchingQuota.DailyLimitUSD > 0 && result.DailyRemaining <= 0 {
			result.Eligible = false
		}
		if matchingQuota.WeeklyLimitUSD > 0 && result.WeeklyRemaining <= 0 {
			result.Eligible = false
		}
		if matchingQuota.MonthlyLimitUSD > 0 && result.MonthlyRemaining <= 0 {
			result.Eligible = false
		}
	} else {
		// No usage record yet means full quota is available.
		result.DailyRemaining = matchingQuota.DailyLimitUSD
		result.WeeklyRemaining = matchingQuota.WeeklyLimitUSD
		result.MonthlyRemaining = matchingQuota.MonthlyLimitUSD
	}

	return result, nil
}
