//go:build unit

package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestComputeProrateCredit(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	expires := start.AddDate(0, 0, 30) // 30 天套餐
	paid := 100.0

	cases := []struct {
		name    string
		now     time.Time
		wantLow float64 // 下界（舍入波动）
		wantHi  float64 // 上界
	}{
		{"用了2天剩28天", start.AddDate(0, 0, 2), 93.0, 93.5},
		{"刚买1分钟", start.Add(time.Minute), 99.0, 100.0},
		{"剩1天", start.AddDate(0, 0, 29), 0.5, 4.0},
		{"已过期剩0", expires.Add(time.Hour), 0, 0},
		{"已过期(now==expires)", expires, 0, 0},
		{"无实付(兑换套餐)", time.Time{}, 0, 0}, // paidAmount=0 特例
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			amt := paid
			if c.name == "无实付(兑换套餐)" {
				amt = 0
			}
			got := computeProrateCredit(amt, start, expires, c.now)
			if math.IsNaN(got) || got < c.wantLow-0.01 || got > c.wantHi+0.01 {
				t.Errorf("credit=%.4f 不在 [%.2f, %.2f]", got, c.wantLow, c.wantHi)
			}
			if c.now.Equal(expires) || c.now.After(expires) {
				if got != 0 {
					t.Errorf("过期应得 0，得 %.4f", got)
				}
			}
		})
	}
}

func TestComputeProrateCredit_ZeroTotal(t *testing.T) {
	// startsAt==expiresAt 退化保护：总秒数为 0 不能除零
	zero := time.Now()
	if got := computeProrateCredit(100, zero, zero, zero); got != 0 {
		t.Errorf("总秒数为0应返回0，得 %.4f", got)
	}
}

// ──────────────────────────────────────────────────────
// PreviewUpgrade stubs
// ──────────────────────────────────────────────────────

// previewPaidReader stubs PaymentOrderReader。
// PreviewUpgrade 会调 GetPaidAmountByBundleSub，传 nil 会 panic，故单测必须传真实 stub。
type previewPaidReader struct {
	amt float64
	err error
}

func (r previewPaidReader) GetPaidAmountByBundleSub(context.Context, int64) (float64, error) {
	return r.amt, r.err
}

// previewSubRepo 仅实现 GetByID，嵌入 bundleSubRepoNoop 防止其他方法被误调即 panic。
type previewSubRepo struct {
	bundleSubRepoNoop
	sub *BundleSubscription
	err error
}

func (s *previewSubRepo) GetByID(_ context.Context, _ int64) (*BundleSubscription, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.sub, nil
}

// previewPlanRepo 仅实现 GetByID，嵌入 bundlePlanRepoNoop 防止其他方法被误调即 panic。
type previewPlanRepo struct {
	bundlePlanRepoNoop
	plan *BundlePlan
	err  error
}

func (s *previewPlanRepo) GetByID(_ context.Context, _ int64) (*BundlePlan, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.plan, nil
}

// newPreviewSvc 构造仅用于 PreviewUpgrade 的 service（paidAmountReader 必须非 nil）。
func newPreviewSvc(subRepo *previewSubRepo, planRepo *previewPlanRepo, paid *previewPaidReader) *BundleSubscriptionService {
	return NewBundleSubscriptionService(subRepo, planRepo, bundleUsageRepoNoop{}, userSubRepoNoop{}, nil, nil, paid)
}

// previewActiveOld 构造一个 active 且属 userID=10 的旧订阅（套餐 starter，实付由 paidReader 注入）。
func previewActiveOld() *BundleSubscription {
	now := time.Now()
	return &BundleSubscription{
		ID:        1,
		UserID:    10,
		PlanID:    5,
		Status:    BundleStatusActive,
		StartsAt:  now.AddDate(0, 0, -2),
		ExpiresAt: now.AddDate(0, 0, 28),
		Plan:      &BundlePlan{ID: 5, Name: "starter", Price: 100},
	}
}

func previewProPlan(price float64, forSale bool, status string) *BundlePlan {
	return &BundlePlan{ID: 6, Name: "pro", Price: price, ValidityDays: 30, Status: status, ForSale: forSale}
}

func TestPreviewUpgrade_UpgradeableWhenDuePositive(t *testing.T) {
	// 实付 100，剩 28/30 天 → credit≈93.33；目标 200 → due≈106.67 > 0
	svc := newPreviewSvc(&previewSubRepo{sub: previewActiveOld()}, &previewPlanRepo{plan: previewProPlan(200, true, BundlePlanStatusActive)}, &previewPaidReader{amt: 100})
	pv, err := svc.PreviewUpgrade(context.Background(), 10, 1, 6)
	require.NoError(t, err)
	require.True(t, pv.Upgradeable, "差价>0 应 upgradeable")
	require.True(t, pv.DueAmount > 0, "due 应 > 0")
	require.InDelta(t, 200, pv.TargetPrice, 0.0001)
	require.InDelta(t, 93.33, pv.Credit, 0.5)
	require.Equal(t, "starter", pv.OldPlanName)
	require.Equal(t, "pro", pv.NewPlanName)
	require.Equal(t, 30, pv.ValidityDays)
}

func TestPreviewUpgrade_NotUpgradeableWhenDueLeqZero(t *testing.T) {
	// 便宜的目标套餐：credit≈93.33 > price 30 → due<=0，不允许降级
	svc := newPreviewSvc(&previewSubRepo{sub: previewActiveOld()}, &previewPlanRepo{plan: previewProPlan(30, true, BundlePlanStatusActive)}, &previewPaidReader{amt: 100})
	pv, err := svc.PreviewUpgrade(context.Background(), 10, 1, 6)
	require.NoError(t, err)
	require.False(t, pv.Upgradeable, "差价<=0 应 not upgradeable")
	require.True(t, pv.DueAmount <= 0, "due 应 <= 0")
}

func TestPreviewUpgrade_OwnerMismatch(t *testing.T) {
	// IDOR 防护：sourceSub 属用户 10，请求用户 20 → ErrBundleNotFound（不泄露存在性）
	svc := newPreviewSvc(&previewSubRepo{sub: previewActiveOld()}, &previewPlanRepo{plan: previewProPlan(200, true, BundlePlanStatusActive)}, &previewPaidReader{amt: 100})
	_, err := svc.PreviewUpgrade(context.Background(), 20, 1, 6)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBundleNotFound, "归属不符应返回 ErrBundleNotFound（不泄露存在性）")
}

func TestPreviewUpgrade_OldSubNotActive(t *testing.T) {
	old := previewActiveOld()
	old.Status = BundleStatusExpired
	svc := newPreviewSvc(&previewSubRepo{sub: old}, &previewPlanRepo{plan: previewProPlan(200, true, BundlePlanStatusActive)}, &previewPaidReader{amt: 100})
	_, err := svc.PreviewUpgrade(context.Background(), 10, 1, 6)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBundleExpired, "非 active 旧订阅应返回 ErrBundleExpired")
}

func TestPreviewUpgrade_TargetPlanNotForSale(t *testing.T) {
	svc := newPreviewSvc(&previewSubRepo{sub: previewActiveOld()}, &previewPlanRepo{plan: previewProPlan(200, false, BundlePlanStatusActive)}, &previewPaidReader{amt: 100})
	_, err := svc.PreviewUpgrade(context.Background(), 10, 1, 6)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBundlePlanDisabled, "下架（ForSale=false）应返回 ErrBundlePlanDisabled")
}

func TestPreviewUpgrade_TargetPlanDisabled(t *testing.T) {
	svc := newPreviewSvc(&previewSubRepo{sub: previewActiveOld()}, &previewPlanRepo{plan: previewProPlan(200, true, BundlePlanStatusDisabled)}, &previewPaidReader{amt: 100})
	_, err := svc.PreviewUpgrade(context.Background(), 10, 1, 6)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBundlePlanDisabled, "停用（Status=disabled）应返回 ErrBundlePlanDisabled")
}

func TestPreviewUpgrade_SubNotFound(t *testing.T) {
	// 订阅不存在 → ErrBundleNotFound（与 IDOR 同口径，不泄露存在性）
	svc := newPreviewSvc(&previewSubRepo{err: ErrBundleNotFound}, &previewPlanRepo{plan: previewProPlan(200, true, BundlePlanStatusActive)}, &previewPaidReader{})
	_, err := svc.PreviewUpgrade(context.Background(), 10, 999, 6)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBundleNotFound)
}

func TestPreviewUpgrade_PaidLookupErrorBubblesUp(t *testing.T) {
	// credit 反查错误应冒泡，不应被吞成 ErrBundleNotFound 等业务错误
	dbErr := errors.New("db unavailable")
	svc := newPreviewSvc(&previewSubRepo{sub: previewActiveOld()}, &previewPlanRepo{plan: previewProPlan(200, true, BundlePlanStatusActive)}, &previewPaidReader{err: dbErr})
	_, err := svc.PreviewUpgrade(context.Background(), 10, 1, 6)
	require.Error(t, err)
	require.ErrorIs(t, err, dbErr, "反查错误应透传")
	require.ErrorContains(t, err, "lookup paid amount")
	require.NotErrorIs(t, err, ErrBundleNotFound, "反查失败不应伪装成 not-found")
}
