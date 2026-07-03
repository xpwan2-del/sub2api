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

// AccumulateUsage 累加套餐订阅在指定渠道组上的用量（USD + 图片/视频次数）
// AccumulateUsage increments the usage counters (USD, image count, video count)
// for a bundle subscription + group. imageCount/videoCount are the number of
// billable media outputs (generated images / video segments) produced by this
// request, tracked independently so image/video limits apply separately.
func (s *BundleUsageService) AccumulateUsage(ctx context.Context, bundleSubID, groupID int64, costUSD float64, imageCount, videoCount int) error {
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
	if err := s.usageRepo.IncrementUsage(ctx, usage.ID, costUSD, imageCount, videoCount, roll); err != nil {
		return fmt.Errorf("increment bundle usage: %w", err)
	}
	return nil
}

// UsageModality 标识本次请求消耗的媒体维度，决定 pre-flight 校验哪些 count 限额。
// UsageModality identifies the media dimension a request consumes and decides which
// count limits the pre-flight eligibility check enforces.
type UsageModality string

const (
	ModalityImage UsageModality = "image" // 图片请求:校验 image count
	ModalityVideo UsageModality = "video" // 视频请求:校验 video count
	ModalityAny   UsageModality = "any"   // 文本/不确定:仅校验 USD,不校验 count
)

// QuotaEligibilityResult 额度检查结果，包含是否可用和各周期剩余额度。
// count 维度按 image/video 分别暴露剩余，便于上层展示与按 modality 校验。
// QuotaEligibilityResult holds the result of a quota eligibility check.
type QuotaEligibilityResult struct {
	Eligible                   bool
	DailyRemaining             float64
	WeeklyRemaining            float64
	MonthlyRemaining           float64
	DailyRemainingImageCount   int
	WeeklyRemainingImageCount  int
	MonthlyRemainingImageCount int
	DailyRemainingVideoCount   int
	WeeklyRemainingVideoCount  int
	MonthlyRemainingVideoCount int
}

// CheckQuotaEligibility 检查套餐订阅在指定渠道组上是否还有剩余额度。
// USD 维度对所有请求都校验；count 维度按 modality 校验对应轨道
// (ModalityImage→image, ModalityVideo→video, ModalityAny→不校验 count)。
// CheckQuotaEligibility checks whether the bundle subscription has remaining quota
// for the given group. Returns eligibility result with remaining amounts.
func (s *BundleUsageService) CheckQuotaEligibility(ctx context.Context, bundleSubID, groupID int64, modality UsageModality) (*QuotaEligibilityResult, error) {
	bundleSub, matchingQuota, err := s.resolveMatchingQuota(ctx, bundleSubID, groupID)
	if err != nil {
		return nil, err
	}
	// 优先用路由中间件注入的 quota（glob 命中的正确 pattern + 激活快照 limit），覆盖
	// resolveMatchingQuota 按 GroupID 取首条可能取错的 quota（同一 group 配多条 quota 的场景）。
	if q := BundleResolvedQuotaFromContext(ctx); q != nil {
		matchingQuota = q
	}
	if bundleSub.Status != BundleStatusActive {
		return nil, ErrBundleExpired
	}
	// 实时过期兜底：与 ResolveGroup 一致，按 ExpiresAt 拦截，避免依赖后台扫描周期。
	// ExpiresAt 零值（仅测试构造场景）跳过；生产中 ActivateBundle 总设未来时间。
	// Real-time expiry fallback aligned with ResolveGroup.
	if !bundleSub.ExpiresAt.IsZero() && time.Now().After(bundleSub.ExpiresAt) {
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

	result := &QuotaEligibilityResult{Eligible: true}

	if usage != nil {
		result.DailyRemaining = matchingQuota.DailyLimitUSD - usage.DailyUsageUSD
		result.WeeklyRemaining = matchingQuota.WeeklyLimitUSD - usage.WeeklyUsageUSD
		result.MonthlyRemaining = matchingQuota.MonthlyLimitUSD - usage.MonthlyUsageUSD
		result.DailyRemainingImageCount = matchingQuota.DailyImageLimitCount - usage.DailyImageUsageCount
		result.WeeklyRemainingImageCount = matchingQuota.WeeklyImageLimitCount - usage.WeeklyImageUsageCount
		result.MonthlyRemainingImageCount = matchingQuota.MonthlyImageLimitCount - usage.MonthlyImageUsageCount
		result.DailyRemainingVideoCount = matchingQuota.DailyVideoLimitCount - usage.DailyVideoUsageCount
		result.WeeklyRemainingVideoCount = matchingQuota.WeeklyVideoLimitCount - usage.WeeklyVideoUsageCount
		result.MonthlyRemainingVideoCount = matchingQuota.MonthlyVideoLimitCount - usage.MonthlyVideoUsageCount
	} else {
		// No usage record yet means full quota is available.
		result.DailyRemaining = matchingQuota.DailyLimitUSD
		result.WeeklyRemaining = matchingQuota.WeeklyLimitUSD
		result.MonthlyRemaining = matchingQuota.MonthlyLimitUSD
		result.DailyRemainingImageCount = matchingQuota.DailyImageLimitCount
		result.WeeklyRemainingImageCount = matchingQuota.WeeklyImageLimitCount
		result.MonthlyRemainingImageCount = matchingQuota.MonthlyImageLimitCount
		result.DailyRemainingVideoCount = matchingQuota.DailyVideoLimitCount
		result.WeeklyRemainingVideoCount = matchingQuota.WeeklyVideoLimitCount
		result.MonthlyRemainingVideoCount = matchingQuota.MonthlyVideoLimitCount
	}

	// USD 维度:所有请求都校验(0=不限,>0 才校验)。
	if matchingQuota.DailyLimitUSD > 0 && result.DailyRemaining <= 0 {
		result.Eligible = false
	}
	if matchingQuota.WeeklyLimitUSD > 0 && result.WeeklyRemaining <= 0 {
		result.Eligible = false
	}
	if matchingQuota.MonthlyLimitUSD > 0 && result.MonthlyRemaining <= 0 {
		result.Eligible = false
	}
	// count 维度:仅按 modality 校验对应轨道。ModalityAny 不校验 count(文本请求不被媒体限额误拒)。
	if modality == ModalityImage {
		if matchingQuota.DailyImageLimitCount > 0 && result.DailyRemainingImageCount <= 0 {
			result.Eligible = false
		}
		if matchingQuota.WeeklyImageLimitCount > 0 && result.WeeklyRemainingImageCount <= 0 {
			result.Eligible = false
		}
		if matchingQuota.MonthlyImageLimitCount > 0 && result.MonthlyRemainingImageCount <= 0 {
			result.Eligible = false
		}
	}
	if modality == ModalityVideo {
		if matchingQuota.DailyVideoLimitCount > 0 && result.DailyRemainingVideoCount <= 0 {
			result.Eligible = false
		}
		if matchingQuota.WeeklyVideoLimitCount > 0 && result.WeeklyRemainingVideoCount <= 0 {
			result.Eligible = false
		}
		if matchingQuota.MonthlyVideoLimitCount > 0 && result.MonthlyRemainingVideoCount <= 0 {
			result.Eligible = false
		}
	}

	return result, nil
}
