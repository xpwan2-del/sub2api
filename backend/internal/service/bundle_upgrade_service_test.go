//go:build unit

package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/domain"
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
// byID 提供"按 plan id 精确返回"能力：PreviewUpgrade 修复后分别按 source/target id
// 查询套餐名（与真实 planRepo.GetByID 按 id 查一致），stub 必须能区分两者。
// byID 命中优先；未命中回落到 plan（保持旧用例 {plan: ...} 构造向后兼容）。
type previewPlanRepo struct {
	bundlePlanRepoNoop
	plan *BundlePlan
	byID map[int64]*BundlePlan
	err  error
}

func (s *previewPlanRepo) GetByID(_ context.Context, id int64) (*BundlePlan, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.byID != nil {
		if p, ok := s.byID[id]; ok {
			return p, nil
		}
	}
	return s.plan, nil
}

// newPreviewSvc 构造仅用于 PreviewUpgrade 的 service（paidAmountReader 必须非 nil）。
func newPreviewSvc(subRepo *previewSubRepo, planRepo *previewPlanRepo, paid *previewPaidReader) *BundleSubscriptionService {
	return NewBundleSubscriptionService(subRepo, planRepo, bundleUsageRepoNoop{}, userSubRepoNoop{}, nil, nil, paid, nil)
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
	// planRepo 按 id 区分 source(starter,id=5)/target(pro,id=6)：修复后 PreviewUpgrade
	// 经 planRepo.GetByID(old.PlanID) 解析当前套餐名，stub 须按 id 精确返回。
	planRepo := &previewPlanRepo{byID: map[int64]*BundlePlan{
		5: {ID: 5, Name: "starter", Price: 100},
		6: previewProPlan(200, true, BundlePlanStatusActive),
	}}
	svc := newPreviewSvc(&previewSubRepo{sub: previewActiveOld()}, planRepo, &previewPaidReader{amt: 100})
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

// TestPreviewUpgrade_OldPlanNameResolvedWithoutPreloadedPlan 回归守护：
// 真实仓储 GetByID 不预加载 Plan 关联（old.Plan 恒为 nil）。PreviewUpgrade 必须通过
// planRepo.GetByID(old.PlanID) 补载当前套餐名，否则前端"当前套餐"展示恒为空。
func TestPreviewUpgrade_OldPlanNameResolvedWithoutPreloadedPlan(t *testing.T) {
	now := time.Now()
	oldNoPlan := &BundleSubscription{
		ID:        1,
		UserID:    10,
		PlanID:    5,
		Status:    BundleStatusActive,
		StartsAt:  now.AddDate(0, 0, -2),
		ExpiresAt: now.AddDate(0, 0, 28),
		// Plan 故意留空，模拟 GetByID 未预加载关联
	}
	planRepo := &previewPlanRepo{byID: map[int64]*BundlePlan{
		5: {ID: 5, Name: "starter", Price: 100},
		6: previewProPlan(200, true, BundlePlanStatusActive),
	}}
	svc := newPreviewSvc(&previewSubRepo{sub: oldNoPlan}, planRepo, &previewPaidReader{amt: 100})
	pv, err := svc.PreviewUpgrade(context.Background(), 10, 1, 6)
	require.NoError(t, err)
	require.Equal(t, "starter", pv.OldPlanName, "old.Plan 未预加载时也应经 planRepo 解析出当前套餐名")
	require.Equal(t, "pro", pv.NewPlanName)
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

// ──────────────────────────────────────────────────────
// UpgradeBundle stubs & tests
// ──────────────────────────────────────────────────────

// upgradeStatusCall 记录一次 UpdateStatus(id, status) 调用，用于断言旧订阅被标 upgraded。
type upgradeStatusCall struct {
	id     int64
	status string
}

// upgradeSubRepoStub 专供 UpgradeBundle：独立持有「旧订阅」与「新建订阅」。
// UpdateStatus 仅记录调用、不原地改 old.Status——模拟事务回滚后 GetByIDWithUsages 仍读到
// 提交前状态（active），便于回滚用例断言「桥接失败 → 旧订阅可观测状态仍 active」。
type upgradeSubRepoStub struct {
	bundleSubRepoNoop

	old             *BundleSubscription // 预存的活跃旧订阅（SourceSubID 命中）
	newCreated      *BundleSubscription // Create 调用写入的新订阅
	createErr       error
	updateStatusErr error
	updateStatusCalls []upgradeStatusCall
}

func (s *upgradeSubRepoStub) GetByIDWithUsages(_ context.Context, id int64) (*BundleSubscription, error) {
	if s.old != nil && id == s.old.ID {
		cp := *s.old
		return &cp, nil
	}
	if s.newCreated != nil && id == s.newCreated.ID {
		cp := *s.newCreated
		return &cp, nil
	}
	return nil, ErrBundleNotFound
}

func (s *upgradeSubRepoStub) UpdateStatus(_ context.Context, id int64, status string) error {
	if s.updateStatusErr != nil {
		return s.updateStatusErr
	}
	s.updateStatusCalls = append(s.updateStatusCalls, upgradeStatusCall{id: id, status: status})
	return nil
}

func (s *upgradeSubRepoStub) Create(_ context.Context, sub *BundleSubscription) error {
	if s.createErr != nil {
		return s.createErr
	}
	if s.old != nil {
		sub.ID = s.old.ID + 100 // 新订阅 ID 与旧订阅区分，避免 GetByIDWithUsages 命中冲突
	} else {
		sub.ID = 100
	}
	s.newCreated = sub
	return nil
}

// newUpgradeSvc 构造专用于 UpgradeBundle 的 service（entClient=nil → withTx 退化为直执行；
// paidAmountReader 对 UpgradeBundle 无用，传 nil）。
func newUpgradeSvc(
	subRepo *upgradeSubRepoStub,
	planRepo *activateBundlePlanRepoStub,
	usageRepo *activateBundleUsageRepoStub,
	userSubRepo *activateUserSubRepoStub,
) *BundleSubscriptionService {
	return NewBundleSubscriptionService(subRepo, planRepo, usageRepo, userSubRepo, nil, nil, nil, nil)
}

// upgradeActiveOld 构造属 userID=10、PlanID=5 的活跃旧订阅（已桥接的 userSub 由调用方注入 userSubRepo.existingSubs）。
func upgradeActiveOld() *BundleSubscription {
	now := time.Now()
	return &BundleSubscription{
		ID:        1,
		UserID:    10,
		PlanID:    5,
		Status:    BundleStatusActive,
		StartsAt:  now.AddDate(0, 0, -2),
		ExpiresAt: now.AddDate(0, 0, 28),
		Plan:      &BundlePlan{ID: 5, Name: "starter"},
	}
}

// upgradeTargetPlan 构造目标套餐（pro，2 个 group quotas，覆盖 USD + image/video count 全字段）。
func upgradeTargetPlan(forSale bool, status string) *BundlePlan {
	return &BundlePlan{
		ID:               6,
		Name:             "pro",
		Price:            200,
		ValidityDays:     30,
		ConcurrencyLimit: 20,
		RPMLimit:         240,
		ForSale:          forSale,
		Status:           status,
		GroupQuotas: []BundlePlanGroupQuota{
			{GroupID: 100, QuotaScope: QuotaScopePlatform, DailyLimitUSD: 5.0, WeeklyLimitUSD: 25.0, MonthlyLimitUSD: 100.0, DailyImageLimitCount: 50, WeeklyImageLimitCount: 250, MonthlyImageLimitCount: 1000, DailyVideoLimitCount: 5, WeeklyVideoLimitCount: 25, MonthlyVideoLimitCount: 100},
			{GroupID: 200, QuotaScope: QuotaScopeModel, ModelPattern: "gpt-4*", DailyLimitUSD: 3.0, WeeklyLimitUSD: 15.0, MonthlyLimitUSD: 60.0, DailyImageLimitCount: 30, WeeklyImageLimitCount: 150, MonthlyImageLimitCount: 600, DailyVideoLimitCount: 3, WeeklyVideoLimitCount: 15, MonthlyVideoLimitCount: 60},
		},
	}
}

func TestUpgradeBundle_Success(t *testing.T) {
	oldSubID := int64(1)
	subRepo := &upgradeSubRepoStub{old: upgradeActiveOld()}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	usageRepo := &activateBundleUsageRepoStub{}
	// 旧订阅已桥接的 userSub（应被 step 2 置为 expired）。
	userSubRepo := &activateUserSubRepoStub{
		existingSubs: []UserSubscription{
			{ID: 900, UserID: 10, GroupID: 100, BundleSubscriptionID: &oldSubID},
		},
	}
	svc := newUpgradeSvc(subRepo, planRepo, usageRepo, userSubRepo)

	got, err := svc.UpgradeBundle(context.Background(), &UpgradeBundleRequest{
		UserID:       10,
		SourceSubID:  1,
		TargetPlanID: 6,
	})
	require.NoError(t, err)
	require.NotNil(t, got)

	// 新订阅字段：source=upgrade, upgraded_from_id=旧ID, status=active, 限额从 plan 快照
	require.Equal(t, BundleStatusActive, got.Status)
	require.Equal(t, BundleSourceUpgrade, got.Source)
	require.Equal(t, oldSubID, got.UpgradedFromID)
	require.Equal(t, int64(6), got.PlanID)
	require.Equal(t, int64(10), got.UserID)
	require.Equal(t, 20, got.ConcurrencyLimit)
	require.Equal(t, 240, got.RPMLimit)
	require.True(t, got.StartsAt.Before(time.Now()))
	require.True(t, got.ExpiresAt.After(got.StartsAt))

	// 旧订阅被标 upgraded
	require.Len(t, subRepo.updateStatusCalls, 1)
	require.Equal(t, upgradeStatusCall{id: oldSubID, status: BundleStatusUpgraded}, subRepo.updateStatusCalls[0])

	// 旧桥接 userSub → 软删除（释放 (user_id,group_id) 唯一槽，详见 UpgradeBundle ②步注释）
	require.Contains(t, userSubRepo.deletedIDs, int64(900))

	// 每个渠道组建 1 条 usage + 1 条桥接 userSub(active)，usage 回填到新订阅
	require.Len(t, usageRepo.createdUsages, 2)
	require.Len(t, got.Usages, 2)
	require.Len(t, userSubRepo.createdSubs, 2)
	for _, us := range userSubRepo.createdSubs {
		require.Equal(t, domain.SubscriptionStatusActive, us.Status)
		require.NotNil(t, us.BundleSubscriptionID)
		require.Equal(t, got.ID, *us.BundleSubscriptionID)
	}
	// 第一条桥接 userSub 限额快照与 plan quota 一致（USD + image/video count 全量，与 ActivateBundle 同款字段集）
	first := userSubRepo.createdSubs[0]
	require.Equal(t, int64(100), first.GroupID)
	require.Equal(t, 5.0, first.DailyLimitUSD)
	require.Equal(t, 25.0, first.WeeklyLimitUSD)
	require.Equal(t, 100.0, first.MonthlyLimitUSD)
	require.Equal(t, 50, first.DailyImageLimitCount)
	require.Equal(t, 250, first.WeeklyImageLimitCount)
	require.Equal(t, 1000, first.MonthlyImageLimitCount)
	require.Equal(t, 5, first.DailyVideoLimitCount)
	require.Equal(t, 25, first.WeeklyVideoLimitCount)
	require.Equal(t, 100, first.MonthlyVideoLimitCount)
}


// TestUpgradeBundle_RebindsAPIKeys 验证升级套餐后把用户的 bundle APIKey 迁移到新套餐 +
// 失效认证缓存，避免旧 key 指向 upgraded 旧 bundle 报 BUNDLE_EXPIRED。
func TestUpgradeBundle_RebindsAPIKeys(t *testing.T) {
	oldSubID := int64(1)
	subRepo := &upgradeSubRepoStub{old: upgradeActiveOld()}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	usageRepo := &activateBundleUsageRepoStub{}
	userSubRepo := &activateUserSubRepoStub{
		existingSubs: []UserSubscription{{ID: 900, UserID: 10, GroupID: 100, BundleSubscriptionID: &oldSubID}},
	}
	rebinder := &bundleKeyRebinderStub{}
	svc := NewBundleSubscriptionService(subRepo, planRepo, usageRepo, userSubRepo, nil, nil, nil, rebinder)

	got, err := svc.UpgradeBundle(context.Background(), &UpgradeBundleRequest{
		UserID: 10, SourceSubID: 1, TargetPlanID: 6,
	})
	require.NoError(t, err)
	require.NotNil(t, got)

	require.Len(t, rebinder.rebindCalls, 1, "升级后应迁移 bundle APIKey 到新套餐")
	require.Equal(t, int64(10), rebinder.rebindCalls[0].userID)
	require.Equal(t, got.ID, rebinder.rebindCalls[0].bundleSubID)
	require.Contains(t, rebinder.invalidateCalls, int64(10), "应失效该用户的 APIKey 认证缓存")
}

func TestUpgradeBundle_AtomicRollbackOnBridgeFailure(t *testing.T) {
	oldSubID := int64(1)
	subRepo := &upgradeSubRepoStub{old: upgradeActiveOld()}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	usageRepo := &activateBundleUsageRepoStub{}
	// 新订阅桥接 userSub.Create 失败（step 5）→ 整个事务应回滚
	userSubRepo := &activateUserSubRepoStub{
		existingSubs: []UserSubscription{
			{ID: 900, UserID: 10, GroupID: 100, BundleSubscriptionID: &oldSubID},
		},
		createErr: errors.New("bridge db down"),
	}
	svc := newUpgradeSvc(subRepo, planRepo, usageRepo, userSubRepo)

	got, err := svc.UpgradeBundle(context.Background(), &UpgradeBundleRequest{
		UserID:       10,
		SourceSubID:  1,
		TargetPlanID: 6,
	})
	require.Error(t, err, "桥接失败应返回错误（不再静默吞掉）")
	require.ErrorContains(t, err, "bridge user subscription", "应来自 step 5 的桥接错误")
	require.Nil(t, got, "失败时不返回新订阅")

	// 原子性灵魂：旧订阅的可观测状态仍为 active（事务回滚后未提交 upgraded）。
	// entClient=nil 时 withTx 退化为直执行，本 stub 的 UpdateStatus 不原地改 old.Status
	// 以模拟「回滚后读到的仍是提交前状态」；真实 DB 事务回滚由集成测试覆盖。
	reFetched, gErr := subRepo.GetByIDWithUsages(context.Background(), oldSubID)
	require.NoError(t, gErr)
	require.Equal(t, BundleStatusActive, reFetched.Status,
		"桥接失败 → 旧订阅必须回滚至 active，不能残留 upgraded")
}

func TestUpgradeBundle_OwnerMismatch(t *testing.T) {
	subRepo := &upgradeSubRepoStub{old: upgradeActiveOld()} // 属 userID=10
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	svc := newUpgradeSvc(subRepo, planRepo, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	// 请求 userID=20 ≠ 属主 10 → IDOR 防护：ErrBundleNotFound（不泄露存在性）
	_, err := svc.UpgradeBundle(context.Background(), &UpgradeBundleRequest{
		UserID:       20,
		SourceSubID:  1,
		TargetPlanID: 6,
	})
	require.ErrorIs(t, err, ErrBundleNotFound)
	require.Empty(t, subRepo.updateStatusCalls, "IDOR 失败不应改动旧订阅状态")
	require.Nil(t, subRepo.newCreated, "IDOR 失败不应创建新订阅")
}

func TestUpgradeBundle_OldSubNotActive(t *testing.T) {
	old := upgradeActiveOld()
	old.Status = BundleStatusExpired
	subRepo := &upgradeSubRepoStub{old: old}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	svc := newUpgradeSvc(subRepo, planRepo, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	_, err := svc.UpgradeBundle(context.Background(), &UpgradeBundleRequest{
		UserID:       10,
		SourceSubID:  1,
		TargetPlanID: 6,
	})
	require.ErrorIs(t, err, ErrBundleExpired, "非 active 旧订阅应返回 ErrBundleExpired（防并发升级/支付期间过期）")
	require.Empty(t, subRepo.updateStatusCalls, "非 active 不应继续标记 upgraded")
}

func TestUpgradeBundle_TargetPlanNotForSale(t *testing.T) {
	subRepo := &upgradeSubRepoStub{old: upgradeActiveOld()}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(false, BundlePlanStatusActive)}
	svc := newUpgradeSvc(subRepo, planRepo, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	_, err := svc.UpgradeBundle(context.Background(), &UpgradeBundleRequest{
		UserID: 10, SourceSubID: 1, TargetPlanID: 6,
	})
	require.ErrorIs(t, err, ErrBundlePlanDisabled)
}

func TestUpgradeBundle_TargetPlanDisabled(t *testing.T) {
	subRepo := &upgradeSubRepoStub{old: upgradeActiveOld()}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusDisabled)}
	svc := newUpgradeSvc(subRepo, planRepo, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	_, err := svc.UpgradeBundle(context.Background(), &UpgradeBundleRequest{
		UserID: 10, SourceSubID: 1, TargetPlanID: 6,
	})
	require.ErrorIs(t, err, ErrBundlePlanDisabled)
}

func TestUpgradeBundle_SourceSubNotFound(t *testing.T) {
	subRepo := &upgradeSubRepoStub{old: nil} // old=nil → GetByIDWithUsages 返回 ErrBundleNotFound
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	svc := newUpgradeSvc(subRepo, planRepo, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})

	_, err := svc.UpgradeBundle(context.Background(), &UpgradeBundleRequest{
		UserID: 10, SourceSubID: 999, TargetPlanID: 6,
	})
	require.ErrorIs(t, err, ErrBundleNotFound, "订阅不存在与 IDOR 同口径（不泄露存在性）")
}

func TestUpgradeBundle_NilRequest(t *testing.T) {
	svc := newUpgradeSvc(&upgradeSubRepoStub{old: upgradeActiveOld()}, &activateBundlePlanRepoStub{}, &activateBundleUsageRepoStub{}, &activateUserSubRepoStub{})
	_, err := svc.UpgradeBundle(context.Background(), nil)
	require.ErrorIs(t, err, ErrBundleNotFound)
}
