package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// computeProrateCredit 按剩余有效期线性折算旧套餐剩余价值。
// credit = paidAmount × max(0, 剩余秒) / 总秒。过期或总额为0返回0。
// 不追溯已消费的请求额度——套餐卖的是"有效期内使用权"而非预付token。
func computeProrateCredit(paidAmount float64, startsAt, expiresAt, now time.Time) float64 {
	if paidAmount <= 0 {
		return 0
	}
	totalSec := expiresAt.Sub(startsAt).Seconds()
	if totalSec <= 0 {
		return 0
	}
	remainSec := expiresAt.Sub(now).Seconds()
	if remainSec <= 0 {
		return 0
	}
	credit := paidAmount * remainSec / totalSec
	if credit < 0 || math.IsNaN(credit) {
		return 0
	}
	// 货币精度：2 位小数
	return math.Round(credit*100) / 100
}

// PreviewUpgrade 预览套餐升级差价：校验旧订阅归属 + active，目标套餐在售，
// 再反查旧订阅实付算出按剩余有效期折算的 credit，返回应补差价与是否可升级。
// 只读、不落库；IDOR 防护：归属不符按"不存在"处理（ErrBundleNotFound），不泄露存在性。
// PreviewUpgrade returns the prorated upgrade cost without persisting anything.
func (s *BundleSubscriptionService) PreviewUpgrade(ctx context.Context, userID, sourceSubID, targetPlanID int64) (*UpgradePreview, error) {
	// 1. 加载旧订阅，校验归属 + active（IDOR 防护：不泄露存在性）
	old, err := s.bundleSubRepo.GetByID(ctx, sourceSubID)
	if err != nil {
		return nil, ErrBundleNotFound
	}
	if old.UserID != userID {
		// 归属不符按"不存在"处理，避免攻击者通过响应差异探测他人订阅（与 IDOR 修复 ecb747d9/024c7879 同原则）
		return nil, ErrBundleNotFound
	}
	if old.Status != BundleStatusActive {
		return nil, ErrBundleExpired
	}

	// 2. 加载目标套餐，校验在售 + 启用
	plan, err := s.planRepo.GetByID(ctx, targetPlanID)
	if err != nil {
		return nil, ErrBundlePlanNotFound
	}
	if !plan.ForSale || plan.Status != BundlePlanStatusActive {
		return nil, ErrBundlePlanDisabled
	}

	// 3. 反查旧订阅实付（兑换/赠送来源找不到订单返回 0,nil），算 credit
	paid, err := s.paidAmountReader.GetPaidAmountByBundleSub(ctx, sourceSubID)
	if err != nil {
		return nil, fmt.Errorf("lookup paid amount: %w", err)
	}
	credit := computeProrateCredit(paid, old.StartsAt, old.ExpiresAt, time.Now())
	due := plan.Price - credit

	oldPlanName := ""
	if old.Plan != nil {
		oldPlanName = old.Plan.Name
	}
	return &UpgradePreview{
		Credit:       credit,
		TargetPrice:  plan.Price,
		DueAmount:    due,
		ValidityDays: plan.ValidityDays,
		Upgradeable:  due > 0, // 差价<=0 即降级/同级，不允许
		OldPlanName:  oldPlanName,
		NewPlanName:  plan.Name,
	}, nil
}

// UpgradeBundle 原子切换：旧订阅 → upgraded + 桥接 userSub 失效；新订阅 active + 桥接 userSub 激活。
// 全流程在单个 withTx 内完成，任一步失败整体回滚，杜绝「旧已 upgraded 但新未激活」的悬空状态。
// 这是支付成功履约时被调用的核心切换方法（被 doBundleUpgrade 调用）。
//
// 事务 5 步：
//  1. 校验旧订阅：归属（IDOR 防护：不泄露存在性）+ active（防并发升级 / 支付期间过期）
//  2. 旧订阅 → upgraded，桥接 userSub → expired
//  3. 加载目标 plan，校验 ForSale + Active
//  4. 创建新订阅：source=upgrade, upgraded_from_id=旧ID, 完整有效期从当下起算
//  5. 每个 plan.GroupQuota 建 usage tracker + 桥接 userSub(active) —— 字段集与 ActivateBundle 完全一致
//
// 并发防护点在 ①：两个并发 UpgradeBundle，第一个把旧置 upgraded 提交后，第二个进 ① 发现非 active → 失败。
// 缓存失效在事务提交后执行，避免回滚后脏失效。
// UpgradeBundle atomically swaps an active bundle subscription to a new plan.
func (s *BundleSubscriptionService) UpgradeBundle(ctx context.Context, req *UpgradeBundleRequest) (*BundleSubscription, error) {
	if req == nil {
		return nil, ErrBundleNotFound
	}

	var upgraded *BundleSubscription
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		// ① 校验旧订阅：归属（IDOR 防护：不泄露存在性）+ active（防并发升级 / 支付期间过期）
		old, err := s.bundleSubRepo.GetByIDWithUsages(txCtx, req.SourceSubID)
		if err != nil {
			return ErrBundleNotFound
		}
		if old.UserID != req.UserID {
			// 归属不符按"不存在"处理，避免攻击者通过响应差异探测他人订阅（与 PreviewUpgrade / IDOR 修复 ecb747d9 同原则）
			return ErrBundleNotFound
		}
		if old.Status != BundleStatusActive {
			return ErrBundleExpired
		}

		// ② 旧订阅 → upgraded，桥接 userSub → expired
		if err := s.bundleSubRepo.UpdateStatus(txCtx, old.ID, BundleStatusUpgraded); err != nil {
			return fmt.Errorf("mark old subscription upgraded: %w", err)
		}
		if err := s.syncBridgedUserSubscriptions(txCtx, old.UserID, old.ID, func(sub *UserSubscription) error {
			return s.userSubRepo.UpdateStatus(txCtx, sub.ID, domain.SubscriptionStatusExpired)
		}); err != nil {
			return fmt.Errorf("expire bridged user subscriptions: %w", err)
		}

		// ③ 加载目标 plan，校验 ForSale + Active
		plan, err := s.planRepo.GetByID(txCtx, req.TargetPlanID)
		if err != nil {
			return fmt.Errorf("load target plan: %w", err)
		}
		if !plan.ForSale || plan.Status != BundlePlanStatusActive {
			return ErrBundlePlanDisabled
		}

		// ④ 创建新订阅：source=upgrade, upgraded_from_id=旧ID, 从当下起算完整有效期（不沿用旧订阅剩余期）
		now := time.Now()
		newSub := &BundleSubscription{
			UserID:           req.UserID,
			PlanID:           req.TargetPlanID,
			Status:           BundleStatusActive,
			StartsAt:         now,
			ExpiresAt:        now.AddDate(0, 0, plan.ValidityDays),
			ConcurrencyLimit: plan.ConcurrencyLimit,
			RPMLimit:         plan.RPMLimit,
			Source:           BundleSourceUpgrade,
			UpgradedFromID:   old.ID,
			Usages:           make([]BundleSubscriptionUsage, 0, len(plan.GroupQuotas)),
		}
		if err := s.bundleSubRepo.Create(txCtx, newSub); err != nil {
			return fmt.Errorf("create new subscription: %w", err)
		}

		// ⑤ 每个渠道组建 usage tracker + 桥接 userSub(active) —— 字段集与 ActivateBundle 完全一致
		for _, gq := range plan.GroupQuotas {
			usage := &BundleSubscriptionUsage{
				BundleSubscriptionID: newSub.ID,
				GroupID:              gq.GroupID,
				ModelPattern:         gq.ModelPattern,
				DailyWindowStart:     now,
				WeeklyWindowStart:    now,
				MonthlyWindowStart:   now,
			}
			if err := s.usageRepo.Create(txCtx, usage); err != nil {
				return fmt.Errorf("create usage tracker for group %d: %w", gq.GroupID, err)
			}
			newSub.Usages = append(newSub.Usages, *usage)

			bundleSubID := newSub.ID
			userSub := &UserSubscription{
				UserID:                 req.UserID,
				GroupID:                gq.GroupID,
				StartsAt:               now,
				ExpiresAt:              newSub.ExpiresAt,
				Status:                 domain.SubscriptionStatusActive,
				DailyUsageUSD:          0,
				WeeklyUsageUSD:         0,
				MonthlyUsageUSD:        0,
				BundleSubscriptionID:   &bundleSubID,
				DailyLimitUSD:          gq.DailyLimitUSD,
				WeeklyLimitUSD:         gq.WeeklyLimitUSD,
				MonthlyLimitUSD:        gq.MonthlyLimitUSD,
				DailyImageLimitCount:   gq.DailyImageLimitCount,
				WeeklyImageLimitCount:  gq.WeeklyImageLimitCount,
				MonthlyImageLimitCount: gq.MonthlyImageLimitCount,
				DailyVideoLimitCount:   gq.DailyVideoLimitCount,
				WeeklyVideoLimitCount:  gq.WeeklyVideoLimitCount,
				MonthlyVideoLimitCount: gq.MonthlyVideoLimitCount,
				Notes:                  fmt.Sprintf("Bridged from bundle plan %q (ID:%d) via upgrade", plan.Name, plan.ID),
			}
			if err := s.userSubRepo.Create(txCtx, userSub); err != nil {
				return fmt.Errorf("bridge user subscription for group %d: %w", gq.GroupID, err)
			}
		}
		upgraded = newSub
		return nil
	}); err != nil {
		return nil, err
	}

	// 缓存失效在事务提交后执行，避免回滚后脏失效（与 ActivateBundle / RevokeBundle 同款）。
	if s.cache != nil {
		_ = s.cache.InvalidateBundleSubscriptionCache(ctx, req.UserID)
	}
	return upgraded, nil
}
