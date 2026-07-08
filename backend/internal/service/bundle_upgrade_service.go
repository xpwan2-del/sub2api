package service

import (
	"context"
	"fmt"
	"math"
	"time"
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
