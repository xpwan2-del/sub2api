//go:build unit

// payment_order_upgrade_balance_test.go 套餐升级余额支付测试（final-review Important #1）
// 守护余额抵扣 gate 扩展到 bundle_upgrade 后的纯余额升级路径：
//  1. createPureBalanceBundleOrder 对 bundle_upgrade 正确扣余额（=差价 due）、建单
//     （Amount/BalanceDeductAmount/SourceBundleSubscriptionID 齐全）、走 executeFulfillment
//     路由到 ExecuteBundleUpgradeFulfillment（换套），而非 ExecuteBundleFulfillment（建新订阅）。
//  2. 升级成功闭环：旧订阅 upgraded + 新订阅创建 + 订单 Completed + bundle_subscription_id 回写。
package service

import (
	"context"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

// TestCreatePureBalanceBundleOrder_UpgradeRoutesToUpgradeFulfillment 验证纯余额升级路径
// 路由正确性（final-review Important #1 核心）：bundle_upgrade 订单经 createPureBalanceBundleOrder
// 必须走 ExecuteBundleUpgradeFulfillment（换套），而非旧的 ExecuteBundleFulfillment（ActivateBundle 建新订阅）。
// 同时守护：余额扣 due、订单字段（SourceBundleSubscriptionID/ProrateCredit）写入。
func TestCreatePureBalanceBundleOrder_UpgradeRoutesToUpgradeFulfillment(t *testing.T) {
	ctx := context.Background()
	client := newUpgradeFulfillTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	const upgradeDue = 50.0
	user := createUpgradeFulfillUser(t, ctx, client, "upgrade-pure-balance@example.com")

	// userRepo stub：DeductBalance 记录扣款金额；UpdateBalance 供回滚路径使用（本用例成功不触发）。
	var deducted float64
	userRepo := &mockUserRepo{
		getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Balance: upgradeDue + 100},
	}
	userRepo.deductBalanceFn = func(_ context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		deducted += amount
		return nil
	}
	userRepo.updateBalanceFn = func(_ context.Context, _ int64, _ float64) error { return nil }

	// 真实 BundleSubscriptionService（entClient=nil → withTx 直执行），stub 使 UpgradeBundle 成功：
	// 旧订阅 active 且归属 user.ID，目标 plan ForSale+Active。
	subRepo := &upgradeSubRepoStub{old: upgradeActiveOldForUser(user.ID)}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	usageRepo := &activateBundleUsageRepoStub{}
	userSubRepo := &activateUserSubRepoStub{}
	bundleSvc := NewBundleSubscriptionService(subRepo, planRepo, usageRepo, userSubRepo, nil, nil, nil)

	svc := &PaymentService{
		entClient:             client,
		bundleSubscriptionSvc: bundleSvc,
		userRepo:              userRepo,
	}

	req := CreateOrderRequest{
		UserID:                     user.ID,
		Amount:                     upgradeDue,
		OrderType:                  payment.OrderTypeBundleUpgrade,
		PlanID:                     6,
		SourceBundleSubscriptionID: 1,
		ProrateCredit:              30.0,
		UseBalance:                 true,
		PaymentType:                payment.TypeAlipay,
		ClientIP:                   "127.0.0.1",
		SrcHost:                    "api.example.com",
	}
	// createPureBalanceBundleOrder(ctx, req, user, cfg, orderAmount=due, feeRate)
	resp, err := svc.createPureBalanceBundleOrder(ctx, req, userRepo.getByIDUser, &PaymentConfig{}, upgradeDue, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.DirectSuccess, "纯余额支付应直接成功")
	require.Equal(t, upgradeDue, resp.BalanceDeductAmount, "余额抵扣应等于差价 due")
	require.Equal(t, 0.0, resp.PayAmount, "纯余额支付渠道金额为 0")

	// 余额扣款金额 = due（一次性扣足）。
	require.Equal(t, upgradeDue, deducted, "应从余额扣足差价")

	// 订单落库：升级专属字段齐全，状态 Completed。
	reloaded, err := client.PaymentOrder.Get(ctx, resp.OrderID)
	require.NoError(t, err)
	require.Equal(t, payment.OrderTypeBundleUpgrade, reloaded.OrderType)
	require.Equal(t, upgradeDue, reloaded.Amount)
	require.Equal(t, upgradeDue, reloaded.BalanceDeductAmount)
	require.Equal(t, 0.0, reloaded.PayAmount)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.NotNil(t, reloaded.PlanID)
	require.Equal(t, int64(6), *reloaded.PlanID)
	require.Equal(t, int64(1), reloaded.SourceBundleSubscriptionID, "纯余额升级订单必须写入 SourceBundleSubscriptionID")
	require.NotNil(t, reloaded.BundleSubscriptionID, "升级成功应回写 bundle_subscription_id")

	// 灵魂断言：路由到升级履约（换套），而非 ActivateBundle 建新订阅。
	// upgradeSubRepoStub.Create 把新订阅 ID 设为 old.ID+100=101；ExecuteBundleUpgradeFulfillment
	// 成功回写该 ID。若误走 ExecuteBundleFulfillment(ActivateBundle)，旧订阅不会被标 upgraded。
	require.Len(t, subRepo.updateStatusCalls, 1, "旧订阅应被标记 upgraded（证明走升级换套）")
	require.Equal(t, upgradeStatusCall{id: 1, status: BundleStatusUpgraded}, subRepo.updateStatusCalls[0])
	require.NotNil(t, subRepo.newCreated, "应创建新订阅（升级目标）")

	// 成功留痕。
	successAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(reloaded.ID, 10)), paymentauditlog.ActionEQ("BUNDLE_UPGRADE_SUCCESS")).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, successAudit)
}

// TestCreatePureBalanceBundleOrder_BundleStillRoutesToBundleFulfillment 回归守护：
// 普通 bundle 纯余额支付仍走 ExecuteBundleFulfillment（ActivateBundle），不被升级路由改动影响。
// 复用 bundle_subscription_service_test.go 的 activateBundleSubRepoStub（GetActiveByUserID 返回空→无冲突）。
func TestCreatePureBalanceBundleOrder_BundleStillRoutesToBundleFulfillment(t *testing.T) {
	ctx := context.Background()
	client := newUpgradeFulfillTestClient(t)
	ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)

	const planPrice = 200.0
	user := createUpgradeFulfillUser(t, ctx, client, "bundle-pure-balance-regression@example.com")

	var deducted float64
	userRepo := &mockUserRepo{
		getByIDUser: &User{ID: user.ID, Email: user.Email, Username: user.Username, Balance: planPrice},
	}
	userRepo.deductBalanceFn = func(_ context.Context, _ int64, amount float64) error {
		deducted += amount
		return nil
	}
	userRepo.updateBalanceFn = func(_ context.Context, _ int64, _ float64) error { return nil }

	// 普通 bundle 激活路径：ActivateBundle 用 activateBundleSubRepoStub（无活跃旧订阅→无冲突）。
	subRepo := &activateBundleSubRepoStub{}
	planRepo := &activateBundlePlanRepoStub{plan: upgradeTargetPlan(true, BundlePlanStatusActive)}
	usageRepo := &activateBundleUsageRepoStub{}
	userSubRepo := &activateUserSubRepoStub{}
	bundleSvc := NewBundleSubscriptionService(subRepo, planRepo, usageRepo, userSubRepo, nil, nil, nil)

	svc := &PaymentService{
		entClient:             client,
		bundleSubscriptionSvc: bundleSvc,
		userRepo:              userRepo,
	}

	req := CreateOrderRequest{
		UserID:      user.ID,
		Amount:      planPrice,
		OrderType:   payment.OrderTypeBundle,
		PlanID:      6,
		UseBalance:  true,
		PaymentType: payment.TypeAlipay,
		ClientIP:    "127.0.0.1",
		SrcHost:     "api.example.com",
	}
	resp, err := svc.createPureBalanceBundleOrder(ctx, req, userRepo.getByIDUser, &PaymentConfig{}, planPrice, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.DirectSuccess)
	require.Equal(t, planPrice, deducted)

	reloaded, err := client.PaymentOrder.Get(ctx, resp.OrderID)
	require.NoError(t, err)
	require.Equal(t, payment.OrderTypeBundle, reloaded.OrderType)
	require.Equal(t, OrderStatusCompleted, reloaded.Status)
	require.Zero(t, reloaded.SourceBundleSubscriptionID, "普通 bundle 订单不应写 SourceBundleSubscriptionID")

	// 走的是 ActivateBundle（BUNDLE_ACTIVATION_SUCCESS），不是升级（BUNDLE_UPGRADE_SUCCESS）。
	activationAudit, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(reloaded.ID, 10)), paymentauditlog.ActionEQ("BUNDLE_ACTIVATION_SUCCESS")).
		Only(ctx)
	require.NoError(t, err)
	require.NotNil(t, activationAudit, "普通 bundle 应走 ActivateBundle 激活路径")
}
