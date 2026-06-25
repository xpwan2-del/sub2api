// bundle_usage_service.go 套餐用量服务实现
// 提供用量累加（AccumulateUsage）和额度资格检查（CheckQuotaEligibility）。
// 用量按日/周/月三个周期独立跟踪，支持平台级和模型级两种粒度。

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
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

// resolveMatchingQuota 加载套餐订阅及其匹配指定渠道组的额度配置，返回 (bundleSub, matchingQuota)。
// 若该 group 无 quota 配置，matchingQuota 为 nil。AccumulateUsage 与 CheckQuotaEligibility 共用，
// 保证两者用同一 ModelPattern 定位 usage 记录（模型级套餐尤为关键）。
func (s *BundleUsageService) resolveMatchingQuota(ctx context.Context, bundleSubID, groupID int64) (*BundleSubscription, *BundlePlanGroupQuota, error) {
	bundleSub, err := s.bundleSubRepo.GetByID(ctx, bundleSubID)
	if err != nil {
		return nil, nil, fmt.Errorf("load bundle subscription: %w", err)
	}
	plan, err := s.planRepo.GetByID(ctx, bundleSub.PlanID)
	if err != nil {
		return nil, nil, fmt.Errorf("load bundle plan: %w", err)
	}
	for i := range plan.GroupQuotas {
		if plan.GroupQuotas[i].GroupID == groupID {
			return bundleSub, &plan.GroupQuotas[i], nil
		}
	}
	return bundleSub, nil, nil
}

// BundleResolvedQuotaFromContext 从 ctx 读取路由中间件已解析的套餐分组额度（含 ModelPattern）。
// 命中时 AccumulateUsage 复用之、跳过 plan 查询；未命中（测试 / 直接调用）则 fallback 到 resolveMatchingQuota。
func BundleResolvedQuotaFromContext(ctx context.Context) *BundlePlanGroupQuota {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(ctxkey.BundleResolvedQuota).(*BundlePlanGroupQuota); ok {
		return v
	}
	return nil
}

// AccumulateUsage 累加套餐订阅在指定渠道组上的用量（USD + 次数）
// AccumulateUsage increments the usage counters (USD and request count) for a
// bundle subscription + group. count is the number of billable media outputs
// (e.g. generated images / video segments) produced by this request.
func (s *BundleUsageService) AccumulateUsage(ctx context.Context, bundleSubID, groupID int64, costUSD float64, count int) error {
	// 定位 usage 的 ModelPattern：优先复用路由中间件已解析的 quota（ctx 携带，省去重复 load plan），
	// 未注入（测试 / 直接调用）时 fallback 到 resolveMatchingQuota。模型级套餐必须用正确 pattern 才能命中。
	pattern := ""
	if q := BundleResolvedQuotaFromContext(ctx); q != nil {
		pattern = q.ModelPattern
	} else {
		_, q, err := s.resolveMatchingQuota(ctx, bundleSubID, groupID)
		if err != nil {
			return fmt.Errorf("resolve bundle quota: %w", err)
		}
		if q != nil {
			pattern = q.ModelPattern
		}
	}
	usage, err := s.usageRepo.GetBySubscriptionAndGroup(ctx, bundleSubID, groupID, pattern)
	if err != nil {
		return fmt.Errorf("find bundle usage: %w", err)
	}
	if usage == nil {
		return ErrBundleNotFound
	}

	// 窗口滚动：日/周/月独立判断是否过期。过期窗口在 repo 内清零（USD + count）
	// 并把 window_start 推进到 now 后再累加本次值；未过期窗口直接 Add。
	now := time.Now()
	roll := WindowRoll{
		Daily:           IsWindowExpired(&usage.DailyWindowStart, BundleDailyWindow),
		Weekly:          IsWindowExpired(&usage.WeeklyWindowStart, BundleWeeklyWindow),
		Monthly:         IsWindowExpired(&usage.MonthlyWindowStart, BundleMonthlyWindow),
		NewDailyStart:   now,
		NewWeeklyStart:  now,
		NewMonthlyStart: now,
	}
	if err := s.usageRepo.IncrementUsage(ctx, usage.ID, costUSD, count, roll); err != nil {
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
	bundleSub, matchingQuota, err := s.resolveMatchingQuota(ctx, bundleSubID, groupID)
	if err != nil {
		return nil, err
	}
	if bundleSub.Status != BundleStatusActive {
		return nil, ErrBundleExpired
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

		result.DailyRemainingCount = matchingQuota.DailyLimitCount - usage.DailyUsageCount
		result.WeeklyRemainingCount = matchingQuota.WeeklyLimitCount - usage.WeeklyUsageCount
		result.MonthlyRemainingCount = matchingQuota.MonthlyLimitCount - usage.MonthlyUsageCount

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
		if matchingQuota.DailyLimitCount > 0 && result.DailyRemainingCount <= 0 {
			result.Eligible = false
		}
		if matchingQuota.WeeklyLimitCount > 0 && result.WeeklyRemainingCount <= 0 {
			result.Eligible = false
		}
		if matchingQuota.MonthlyLimitCount > 0 && result.MonthlyRemainingCount <= 0 {
			result.Eligible = false
		}
	} else {
		// No usage record yet means full quota is available.
		result.DailyRemaining = matchingQuota.DailyLimitUSD
		result.WeeklyRemaining = matchingQuota.WeeklyLimitUSD
		result.MonthlyRemaining = matchingQuota.MonthlyLimitUSD

		result.DailyRemainingCount = matchingQuota.DailyLimitCount
		result.WeeklyRemainingCount = matchingQuota.WeeklyLimitCount
		result.MonthlyRemainingCount = matchingQuota.MonthlyLimitCount
	}

	return result, nil
}
