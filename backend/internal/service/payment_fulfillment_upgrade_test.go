//go:build unit

// payment_fulfillment_upgrade_test.go 套餐升级履约测试（Task 7）
// 守护三条核心路径：
//  1. 幂等 —— 已有 BUNDLE_UPGRADE_SUCCESS audit log 时不再调 UpgradeBundle，直接 markCompleted。
//  2. 退余额 —— 旧订阅支付期间过期（UpgradeBundle 返回 ErrBundleExpired）→ 差价退到用户余额，
//     资金不出平台（设计 §10 边界）。ErrBundleNotFound（IDOR / 并发已升级）走同一退款路径。
//  3. 成功 —— UpgradeBundle 成功 → 回写 bundle_subscription_id + markCompleted。
package service

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

// upgradeFulfillRedeemRepo 是支持 Create 的兑换码 repo stub。
// paymentOrderLifecycleRedeemRepo.Create 会 panic，而退款路径首次运行必须 CreateCode，
// 故此处独立实现 Create/GetByCode/Use 三个真实路径，其余方法保持 panic 守护。
type upgradeFulfillRedeemRepo struct {
	codes    map[string]*RedeemCode
	useCalls []struct {
		id     int64
		userID int64
	}
	nextID int64
}

func newUpgradeFulfillRedeemRepo() *upgradeFulfillRedeemRepo {
	return &upgradeFulfillRedeemRepo{codes: map[string]*RedeemCode{}, nextID: 1000}
}

func (r *upgradeFulfillRedeemRepo) Create(_ context.Context, code *RedeemCode) error {
	if code.ID == 0 {
		r.nextID++
		code.ID = r.nextID
	}
	cloned := *code
	r.codes[code.Code] = &cloned
	return nil
}

func (r *upgradeFulfillRedeemRepo) CreateBatch(context.Context, []RedeemCode) error {
	panic("unexpected CreateBatch call")
}

func (r *upgradeFulfillRedeemRepo) GetByID(_ context.Context, id int64) (*RedeemCode, error) {
	for _, rc := range r.codes {
		if rc.ID != id {
			continue
		}
		cloned := *rc
		return &cloned, nil
	}
	return nil, ErrRedeemCodeNotFound
}

func (r *upgradeFulfillRedeemRepo) GetByCode(_ context.Context, code string) (*RedeemCode, error) {
	rc, ok := r.codes[code]
	if !ok {
		return nil, ErrRedeemCodeNotFound
	}
	cloned := *rc
	return &cloned, nil
}

func (r *upgradeFulfillRedeemRepo) Update(context.Context, *RedeemCode) error {
	panic("unexpected Update call")
}

func (r *upgradeFulfillRedeemRepo) BatchUpdate(context.Context, []int64, RedeemCodeBatchUpdateFields) (int64, error) {
	panic("unexpected BatchUpdate call")
}

func (r *upgradeFulfillRedeemRepo) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (r *upgradeFulfillRedeemRepo) Use(_ context.Context, id, userID int64) error {
	for _, rc := range r.codes {
		if rc.ID != id {
			continue
		}
		rc.Status = StatusUsed
		rc.UsedBy = &userID
		now := time.Now().UTC()
		rc.UsedAt = &now
		r.useCalls = append(r.useCalls, struct {
			id     int64
			userID int64
		}{id: id, userID: userID})
		return nil
	}
	return ErrRedeemCodeNotFound
}

func (r *upgradeFulfillRedeemRepo) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (r *upgradeFulfillRedeemRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (r *upgradeFulfillRedeemRepo) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	panic("unexpected ListByUser call")
}

func (r *upgradeFulfillRedeemRepo) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
}

func (r *upgradeFulfillRedeemRepo) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
}

// newUpgradeFulfillTestClient 起一个内存 sqlite ent client（与 lifecycle 测试同款）。
func newUpgradeFulfillTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:payment_upgrade_fulfill?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// createUpgradeFulfillUser 建一个真实 ent User（PaymentOrder.UserID 有外键约束，
// 必须先建用户）。返回的 ID 同时用于订单 UserID 与 bundle 旧订阅的归属，保持一致。
func createUpgradeFulfillUser(t *testing.T, ctx context.Context, client *dbent.Client, email string) *dbent.User {
	t.Helper()
	u, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("hash").
		SetUsername(email).
		Save(ctx)
	require.NoError(t, err)
	return u
}

// createUpgradeFulfillOrder 在 ent client 中建一个 Paid 状态的 bundle_upgrade 订单。
// userID 必须指向已存在的 User（外键）。targetPlanID/sourceSubID/amount 可定制。
func createUpgradeFulfillOrder(t *testing.T, ctx context.Context, client *dbent.Client, userID, targetPlanID, sourceSubID int64, amount float64) *dbent.PaymentOrder {
	t.Helper()
	order, err := client.PaymentOrder.Create().
		SetUserID(userID).
		SetUserEmail("upgrade-fulfill@example.com").
		SetUserName("upgrade-fulfill-user").
		SetAmount(amount).
		SetPayAmount(amount).
		SetFeeRate(0).
		SetRechargeCode("UPGRADE-FULFILL").
		SetOutTradeNo("sub2_upgrade_fulfill_" + strconv.FormatInt(sourceSubID, 10)).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-upgrade-fulfill").
		SetOrderType(payment.OrderTypeBundleUpgrade).
		SetPlanID(targetPlanID).
		SetSourceBundleSubscriptionID(sourceSubID).
		SetProrateCredit(30.0).
		SetStatus(OrderStatusPaid).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)
	return order
}

// upgradeActiveOldForUser 复制 upgradeActiveOld() 并改写 UserID，使订单与旧订阅归属一致
// （UpgradeBundle 的 IDOR 防护会比对 req.UserID == old.UserID）。
func upgradeActiveOldForUser(userID int64) *BundleSubscription {
	old := upgradeActiveOld()
	old.UserID = userID
	return old
}

// newUpgradeFulfillBundleSvc 构造一个真实 *BundleSubscriptionService（entClient=nil →
// withTx 退化为直执行），repo 用 bundle_upgrade_service_test.go 的 stub。success 路径需要
// usageRepo/userSubRepo 真实可用；幂等/退款路径因 UpgradeBundle 不走完整流程也兼容。
func newUpgradeFulfillBundleSvc(subRepo *upgradeSubRepoStub, planRepo *activateBundlePlanRepoStub) *BundleSubscriptionService {
	return NewBundleSubscriptionService(
		subRepo, planRepo,
		&activateBundleUsageRepoStub{}, &activateUserSubRepoStub{},
		nil, nil, nil, nil,
	)
}

// TestDoBundleUpgrade_IdempotentSkipsWhenSuccessAuditExists 验证幂等：
// 已有 BUNDLE_UPGRADE_SUCCESS audit log → 不再调 UpgradeBundle（subRepo 无任何状态变更），
// 订单直接 markCompleted。
func TestDoBundleUpgrade_IdempotentSkipsWhenSuccessAuditExists(t *testing.T) {
	ctx := context.Background()
	client := newUpgradeFulfillTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	user := createUpgradeFulfillUser(t, ctx, client, "upgrade-idempotent@example.com")
	order := createUpgradeFulfillOrder(t, ctx, client, user.ID, 6 /*targetPlan*/, 1 /*sourceSub*/, 50.0)

	// 预置 SUCCESS audit log（模拟上一次履约已完成核心步骤，仅 markCompleted 未落库的崩溃恢复场景）。
	_, err := client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("BUNDLE_UPGRADE_SUCCESS").
		SetDetail(`{}`).SetOperator("system").Save(ctx)
	require.NoError(t, err)

	subRepo := &upgradeSubRepoStub{old: upgradeActiveOldForUser(user.ID)}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	bundleSvc := newUpgradeFulfillBundleSvc(subRepo, planRepo)

	svc := &PaymentService{
		entClient:             client,
		bundleSubscriptionSvc: bundleSvc,
	}

	err = svc.ExecuteBundleUpgradeFulfillment(ctx, order.ID)
	require.NoError(t, err)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status, "幂等路径应将订单置 Completed")

	// 灵魂断言：UpgradeBundle 未被调用 —— 旧订阅无 upgraded 标记、无新订阅创建。
	require.Empty(t, subRepo.updateStatusCalls, "幂等命中后不应再标记旧订阅 upgraded")
	require.Nil(t, subRepo.newCreated, "幂等命中后不应创建新订阅")
}

// TestDoBundleUpgrade_RefundsToBalanceWhenOldSubExpired 验证退余额边界：
// 旧订阅支付期间过期（UpgradeBundle 返回 ErrBundleExpired）→ refundUpgradeToBalance
// 把已付差价退到用户余额，资金不出平台；订单以 BUNDLE_UPGRADE_REFUND_BALANCE 标 Completed。
func TestDoBundleUpgrade_RefundsToBalanceWhenOldSubExpired(t *testing.T) {
	ctx := context.Background()
	client := newUpgradeFulfillTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	const upgradeDue = 50.0
	user := createUpgradeFulfillUser(t, ctx, client, "upgrade-refund-expired@example.com")
	order := createUpgradeFulfillOrder(t, ctx, client, user.ID, 6, 1, upgradeDue)

	// userRepo stub：updateBalance 直接累加到 getByIDUser.Balance（模拟余额入账）。
	userRepo := &mockUserRepo{
		getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Balance: 0},
	}
	userRepo.updateBalanceFn = func(_ context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		if userRepo.getByIDUser != nil {
			userRepo.getByIDUser.Balance += amount
		}
		return nil
	}
	redeemRepo := newUpgradeFulfillRedeemRepo()
	redeemService := NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil)

	// 旧订阅 expired（支付期间过期）→ UpgradeBundle 返回 ErrBundleExpired。
	expiredOld := upgradeActiveOldForUser(user.ID)
	expiredOld.Status = BundleStatusExpired
	subRepo := &upgradeSubRepoStub{old: expiredOld}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	bundleSvc := newUpgradeFulfillBundleSvc(subRepo, planRepo)

	svc := &PaymentService{
		entClient:             client,
		bundleSubscriptionSvc: bundleSvc,
		redeemService:         redeemService,
		userRepo:              userRepo,
	}

	err := svc.ExecuteBundleUpgradeFulfillment(ctx, order.ID)
	require.NoError(t, err, "退余额路径应成功闭环（非 markFailed）")

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status, "退余额后订单应 Completed")

	// 用户余额 += 已付差价（资金不出平台）。
	require.Equal(t, upgradeDue, userRepo.getByIDUser.Balance, "退款应等于订单已付金额")
	require.Len(t, redeemRepo.useCalls, 1, "退款兑换码应被 Redeem 消费一次")

	// 退款闭环以 BUNDLE_UPGRADE_REFUND_BALANCE 留痕。
	refundAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("BUNDLE_UPGRADE_REFUND_BALANCE")).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, refundAudit)

	// 旧订阅不应被标记 upgraded（UpgradeBundle 在 active 校验即失败）。
	require.Empty(t, subRepo.updateStatusCalls)
}

// TestDoBundleUpgrade_SuccessWritesBackSubscriptionID 验证成功路径：
// UpgradeBundle 成功 → 回写 bundle_subscription_id + BUNDLE_UPGRADE_SUCCESS 标 Completed。
func TestDoBundleUpgrade_SuccessWritesBackSubscriptionID(t *testing.T) {
	ctx := context.Background()
	client := newUpgradeFulfillTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	user := createUpgradeFulfillUser(t, ctx, client, "upgrade-success@example.com")
	order := createUpgradeFulfillOrder(t, ctx, client, user.ID, 6, 1, 50.0)

	subRepo := &upgradeSubRepoStub{old: upgradeActiveOldForUser(user.ID)}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	usageRepo := &activateBundleUsageRepoStub{}
	userSubRepo := &activateUserSubRepoStub{}
	// success 路径需要完整 stub（usage 创建 + 桥接 userSub 创建）。
	bundleSvc := NewBundleSubscriptionService(subRepo, planRepo, usageRepo, userSubRepo, nil, nil, nil, nil)

	svc := &PaymentService{
		entClient:             client,
		bundleSubscriptionSvc: bundleSvc,
	}

	err := svc.ExecuteBundleUpgradeFulfillment(ctx, order.ID)
	require.NoError(t, err)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)

	// 回写新订阅 ID（upgradeSubRepoStub.Create 把新订阅 ID 设为 old.ID+100 = 101）。
	require.NotNil(t, reloaded.BundleSubscriptionID, "成功后应回写 bundle_subscription_id")
	require.Equal(t, int64(101), *reloaded.BundleSubscriptionID)

	// 旧订阅被标 upgraded。
	require.Len(t, subRepo.updateStatusCalls, 1)
	require.Equal(t, upgradeStatusCall{id: 1, status: BundleStatusUpgraded}, subRepo.updateStatusCalls[0])

	// SUCCESS 留痕。
	successAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("BUNDLE_UPGRADE_SUCCESS")).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, successAudit)
}

// TestDoBundleUpgrade_RefundsToBalanceWhenOldSubNotFound 验证 IDOR / 并发已升级边界：
// UpgradeBundle 返回 ErrBundleNotFound（旧订阅不存在 / 归属不符 / 并发已被升级）→ 同样退余额。
func TestDoBundleUpgrade_RefundsToBalanceWhenOldSubNotFound(t *testing.T) {
	ctx := context.Background()
	client := newUpgradeFulfillTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	const upgradeDue = 80.0
	user := createUpgradeFulfillUser(t, ctx, client, "upgrade-refund-notfound@example.com")
	order := createUpgradeFulfillOrder(t, ctx, client, user.ID, 6, 999, upgradeDue)

	userRepo := &mockUserRepo{
		getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Balance: 0},
	}
	userRepo.updateBalanceFn = func(_ context.Context, id int64, amount float64) error {
		if userRepo.getByIDUser != nil {
			userRepo.getByIDUser.Balance += amount
		}
		return nil
	}
	redeemRepo := newUpgradeFulfillRedeemRepo()
	redeemService := NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil)

	// old=nil → GetByIDWithUsages 返回 ErrBundleNotFound。
	subRepo := &upgradeSubRepoStub{old: nil}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	bundleSvc := newUpgradeFulfillBundleSvc(subRepo, planRepo)

	svc := &PaymentService{
		entClient:             client,
		bundleSubscriptionSvc: bundleSvc,
		redeemService:         redeemService,
		userRepo:              userRepo,
	}

	err := svc.ExecuteBundleUpgradeFulfillment(ctx, order.ID)
	require.NoError(t, err)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Equal(t, upgradeDue, userRepo.getByIDUser.Balance, "ErrBundleNotFound 走同一退款路径")
}

// TestDoBundleUpgrade_RefundsToBalanceWhenPlanDisabled 验证目标套餐下架退余额边界
// （final-review Important #2）：
// 支付窗口期内目标套餐被运营下架（UpgradeBundle 返回 ErrBundlePlanDisabled）→ 同样走
// refundUpgradeToBalance，把已付差价退到余额，避免资金滞留。与 ErrBundleExpired/NotFound
// 退余额哲学一致（设计 §10"资金不出平台"）。
func TestDoBundleUpgrade_RefundsToBalanceWhenPlanDisabled(t *testing.T) {
	ctx := context.Background()
	client := newUpgradeFulfillTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	const upgradeDue = 60.0
	user := createUpgradeFulfillUser(t, ctx, client, "upgrade-refund-plan-disabled@example.com")
	order := createUpgradeFulfillOrder(t, ctx, client, user.ID, 6, 1, upgradeDue)

	userRepo := &mockUserRepo{
		getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Balance: 0},
	}
	userRepo.updateBalanceFn = func(_ context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		if userRepo.getByIDUser != nil {
			userRepo.getByIDUser.Balance += amount
		}
		return nil
	}
	redeemRepo := newUpgradeFulfillRedeemRepo()
	redeemService := NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil)

	// 旧订阅 active（通过 active 校验），但目标套餐 ForSale=false → UpgradeBundle 在加载
	// plan 校验时返回 ErrBundlePlanDisabled。注：entClient=nil 时 withTx 退化为直执行，
	// step ②（标记旧订阅 upgraded）会先于错误落在 stub 上；生产真实事务会整体回滚，此处
	// 不断言 subRepo.updateStatusCalls（no-tx 测试工件，非生产行为）。
	subRepo := &upgradeSubRepoStub{old: upgradeActiveOldForUser(user.ID)}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(false, BundlePlanStatusActive)}
	bundleSvc := newUpgradeFulfillBundleSvc(subRepo, planRepo)

	svc := &PaymentService{
		entClient:             client,
		bundleSubscriptionSvc: bundleSvc,
		redeemService:         redeemService,
		userRepo:              userRepo,
	}

	err := svc.ExecuteBundleUpgradeFulfillment(ctx, order.ID)
	require.NoError(t, err, "ErrBundlePlanDisabled 应走退余额闭环（非 markFailed）")

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status, "退余额后订单应 Completed")

	// 用户余额 += 已付差价（资金不出平台）。
	require.Equal(t, upgradeDue, userRepo.getByIDUser.Balance, "ErrBundlePlanDisabled 退款应等于已付差价")
	require.Len(t, redeemRepo.useCalls, 1, "退款兑换码应被 Redeem 消费一次")

	// 退款闭环以 BUNDLE_UPGRADE_REFUND_BALANCE 留痕。
	refundAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("BUNDLE_UPGRADE_REFUND_BALANCE")).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, refundAudit)
}

// TestRefundUpgradeToBalance_IsIdempotent 验证退款自身的幂等：
// 已有 BUNDLE_UPGRADE_REFUND_BALANCE audit log → 不再 Redeem，直接 markCompleted。
func TestRefundUpgradeToBalance_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	client := newUpgradeFulfillTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	user := createUpgradeFulfillUser(t, ctx, client, "upgrade-refund-idempotent@example.com")
	order := createUpgradeFulfillOrder(t, ctx, client, user.ID, 6, 1, 50.0)
	// 订单先锁到 Recharging，模拟退款进行中状态。
	_, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRecharging).Save(ctx)
	require.NoError(t, err)
	// 预置退款 audit log（上次退款已成功，markCompleted 前崩溃）。
	_, err = client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("BUNDLE_UPGRADE_REFUND_BALANCE").
		SetDetail(`{}`).SetOperator("system").Save(ctx)
	require.NoError(t, err)

	redeemRepo := newUpgradeFulfillRedeemRepo()
	userRepo := &mockUserRepo{getByIDUser: &User{ID: user.ID, Balance: 0}}
	redeemService := NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil)

	svc := &PaymentService{
		entClient:     client,
		redeemService: redeemService,
		userRepo:      userRepo,
	}

	o, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	err = svc.refundUpgradeToBalance(ctx, o)
	require.NoError(t, err)

	// 幂等：Redeem 未被调用，余额未变。
	require.Empty(t, redeemRepo.useCalls, "幂等命中后不应再次 Redeem")
	require.Equal(t, 0.0, userRepo.getByIDUser.Balance, "幂等命中后余额不应增加")

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
}

// TestExecuteBundleUpgradeFulfillment_RejectsMissingSourceSubID 守护升级订单必需字段校验：
// 缺 SourceBundleSubscriptionID（或 PlanID）应在锁前直接拒绝，不进入 doBundleUpgrade。
func TestExecuteBundleUpgradeFulfillment_RejectsMissingSourceSubID(t *testing.T) {
	ctx := context.Background()
	client := newUpgradeFulfillTestClient(t)

	user := createUpgradeFulfillUser(t, ctx, client, "upgrade-missing@example.com")
	planID := int64(6)
	// 故意不设 SourceBundleSubscriptionID（默认 0）。
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail("upgrade-missing@example.com").
		SetUserName("upgrade-missing-user").
		SetAmount(50).
		SetPayAmount(50).
		SetFeeRate(0).
		SetRechargeCode("UPGRADE-MISSING").
		SetOutTradeNo("sub2_upgrade_missing").
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-missing").
		SetOrderType(payment.OrderTypeBundleUpgrade).
		SetPlanID(planID).
		SetStatus(OrderStatusPaid).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	err = svc.ExecuteBundleUpgradeFulfillment(ctx, order.ID)
	require.Error(t, err)

	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPaid, reloaded.Status, "字段缺失应锁前拒绝，不改状态")
}
