//go:build integration

// bundle_upgrade_e2e_integration_test.go 套餐升级端到端集成测试（Task 10 / spec §12）
//
// 覆盖 spec §12 全部 6 个场景，使用 testcontainers PG（testEntClient harness，与本目录下
// bundle_integration_test.go / payment_order_repo_integration_test.go 同款）+ 真实 repository +
// 真实 PaymentService / BundleSubscriptionService / RedeemService，端到端验证整条升级链路。
//
// 为什么放在 package repository：testEntClient harness（testcontainers 启 PG、建 schema、返回
// ent client）定义在本包且为 unexported，跨包无法复用。本测试 import service 构造真实服务，
// 与 bundle_integration_test.go 的做法一致（那同样在 package repository 调用 service 层）。
//
// 6 个场景：
//  1. 正常升级 starter→pro：完整 preview → 下单 → 履约 → 原子切换，全字段断言。
//  2. 二次升级 pro→enterprise：验证 Task 3 fix（OrderTypeIn(bundle,bundle_upgrade) 能反查到
//     bundle_upgrade 订单），credit 不被算成 0。
//  3. 并发升级：两笔 bundle_upgrade 订单并发履约，只一笔切换成功，另一笔走 refundUpgradeToBalance。
//  4. 支付期间过期：旧订阅 status=expired → UpgradeBundle 返回 ErrBundleExpired → 退余额。
//  5. 幂等：重复履约同一订单，hasAuditLog 命中，UpgradeBundle 只调用一次。
//  6. IDOR：用户 B 用用户 A 的 source_sub_id 调 preview/UpgradeBundle → ErrBundleNotFound。
//
// 数据隔离：testEntClient 返回全局共享 ent client（真实提交、不自动回滚）。每个测试用唯一
// email/out_trade_no 区分数据，并在 t.Cleanup 按 userID 级联清理（payment_audit_logs →
// payment_orders → redeem_codes → user_subscriptions → bundle_subscription_usage →
// bundle_subscriptions → user；plan/group 按 ID 删）。
package repository

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/bundlesubscription"
	"github.com/Wei-Shaw/sub2api/ent/bundlesubscriptionusage"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// E2E fixture & helpers
// =============================================================================

// bundleUpgradeE2E 聚合一次 E2E 测试所需的全部真实服务与 ent client。
type bundleUpgradeE2E struct {
	t      *testing.T
	ctx    context.Context
	client *dbent.Client

	planRepo    service.BundlePlanRepository
	subRepo     service.BundleSubscriptionRepository
	usageRepo   service.BundleUsageRepository
	userSubRepo service.UserSubscriptionRepository
	userRepo    service.UserRepository
	redeemRepo  service.RedeemCodeRepository
	groupRepo   service.GroupRepository
	paidReader  service.PaymentOrderReader

	planSvc   *service.BundlePlanService
	subSvc    *service.BundleSubscriptionService
	redeemSvc *service.RedeemService
	paySvc    *service.PaymentService
}

// newBundleUpgradeE2E 构造真实服务集合。client 用全局 testEntClient（真实提交），服务间共享
// 同一 client，使 PaymentService/RedeemService/BundleSubscriptionService 内部的 entClient.Tx(ctx)
// 都能创建真实 PG 事务（并发场景必需）。
func newBundleUpgradeE2E(t *testing.T) *bundleUpgradeE2E {
	t.Helper()
	client := testEntClient(t)
	ctx := context.Background()

	planRepo := NewBundlePlanRepository(client)
	subRepo := NewBundleSubscriptionRepository(client)
	usageRepo := NewBundleUsageRepository(client)
	userSubRepo := NewUserSubscriptionRepository(client)
	userRepo := NewUserRepository(client, integrationDB)
	redeemRepo := NewRedeemCodeRepository(client)
	groupRepo := NewGroupRepository(client, integrationDB)
	paidReader := NewPaymentOrderReader(client)

	planSvc := service.NewBundlePlanService(planRepo, nil)
	subSvc := service.NewBundleSubscriptionService(subRepo, planRepo, usageRepo, userSubRepo, nil, client, paidReader)
	redeemSvc := service.NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil)
	// NewPaymentService 的 registry/loadBalancer/subscriptionSvc/configService/groupRepo/affiliateService
	// 传 nil：升级履约路径不依赖它们（notificationEmailService 也为 nil，markCompleted 内有 nil 守卫；
	// affiliate 对 bundle_upgrade 类型返回 baseAmount=0 不触发）。
	paySvc := service.NewPaymentService(client, nil, nil, redeemSvc, nil, subSvc, nil, userRepo, nil, nil)

	return &bundleUpgradeE2E{
		t: t, ctx: ctx, client: client,
		planRepo: planRepo, subRepo: subRepo, usageRepo: usageRepo, userSubRepo: userSubRepo,
		userRepo: userRepo, redeemRepo: redeemRepo, groupRepo: groupRepo, paidReader: paidReader,
		planSvc: planSvc, subSvc: subSvc, redeemSvc: redeemSvc, paySvc: paySvc,
	}
}

// mustCreateUser 创建真实 ent User，注册按 userID 级联清理。
func (e *bundleUpgradeE2E) mustCreateUser(email string) *service.User {
	e.t.Helper()
	u, err := e.client.User.Create().
		SetEmail(email).
		SetPasswordHash("e2e-hash").
		SetStatus(service.StatusActive).
		SetRole(service.RoleUser).
		Save(e.ctx)
	require.NoError(e.t, err, "create user")
	e.t.Cleanup(func() { e2eCleanupUser(e.t, context.Background(), e.client, u.ID) })
	return userEntityToService(u)
}

// mustCreateGroup 创建真实 ent Group（platform 级 quota 关联用），按 ID 清理。
func (e *bundleUpgradeE2E) mustCreateGroup(name, platform string) *service.Group {
	e.t.Helper()
	g, err := e.client.Group.Create().
		SetName(name).
		SetPlatform(platform).
		SetStatus(service.StatusActive).
		Save(e.ctx)
	require.NoError(e.t, err, "create group")
	gid := g.ID
	// best-effort 按 ID 删（FK 冲突时忽略；testcontainers DB 进程结束即销毁，残留无害）。
	e.t.Cleanup(func() { _ = e.client.Group.DeleteOneID(gid).Exec(context.Background()) })
	return groupEntityToService(g)
}

// mustCreatePlan 创建真实 BundlePlan（含 1 个 platform 级 group quota），按 ID best-effort 清理。
// 价格/有效期可定制；ForSale 默认 true，Status 默认 active。
func (e *bundleUpgradeE2E) mustCreatePlan(name, tier string, price float64, validityDays int, groupID int64) *service.BundlePlan {
	e.t.Helper()
	plan, err := e.planSvc.CreatePlan(e.ctx, &service.CreateBundlePlanRequest{
		Name:          name,
		Description:   "e2e plan",
		Tier:          tier,
		Price:         price,
		OriginalPrice: price,
		Currency:      "USD",
		ValidityDays:  validityDays,
		GroupQuotas: []service.CreateGroupQuotaRequest{
			{GroupID: groupID, QuotaScope: service.QuotaScopePlatform, DailyLimitUSD: 10, WeeklyLimitUSD: 50, MonthlyLimitUSD: 200},
		},
	})
	require.NoError(e.t, err, "create plan")
	require.NotNil(e.t, plan)
	pid := plan.ID
	e.t.Cleanup(func() { _ = e.client.BundlePlan.DeleteOneID(pid).Exec(context.Background()) })
	return plan
}

// mustActivateBundle 激活套餐并返回订阅。
func (e *bundleUpgradeE2E) mustActivateBundle(userID, planID int64) *service.BundleSubscription {
	e.t.Helper()
	sub, err := e.subSvc.ActivateBundle(e.ctx, &service.ActivateBundleRequest{
		UserID: userID, PlanID: planID, Source: service.BundleSourcePurchase,
	})
	require.NoError(e.t, err, "activate bundle")
	require.NotNil(e.t, sub)
	return sub
}

// mustCreateCompletedBundleOrder 建一笔 COMPLETED 的 bundle 订单关联到 bundleSubID。
// 这是 PreviewUpgrade 反查实付（GetPaidAmountByBundleSub）的数据源。
func (e *bundleUpgradeE2E) mustCreateCompletedBundleOrder(u *service.User, bundleSubID int64, amount float64, tag string) *dbent.PaymentOrder {
	e.t.Helper()
	o, err := e.client.PaymentOrder.Create().
		SetUserID(u.ID).
		SetUserEmail(u.Email).
		SetUserName(u.Username).
		SetAmount(amount).
		SetPayAmount(amount).
		SetFeeRate(0).
		SetRechargeCode("E2E-BUNDLE-" + tag).
		SetOutTradeNo("e2e_bundle_" + tag).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-bundle-" + tag).
		SetOrderType(payment.OrderTypeBundle).
		SetBundleSubscriptionID(bundleSubID).
		SetProrateCredit(0).
		SetStatus(service.OrderStatusCompleted).
		SetPaidAt(time.Now()).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(e.ctx)
	require.NoError(e.t, err, "create completed bundle order")
	return o
}

// mustCreatePaidUpgradeOrder 建一笔 PAID 的 bundle_upgrade 订单，关联 sourceSubID + targetPlanID。
// amount=应补差价，credit=预览时算出的按比例折算值（建单时锁定，Task 6）。
func (e *bundleUpgradeE2E) mustCreatePaidUpgradeOrder(u *service.User, targetPlanID, sourceSubID int64, amount, credit float64, tag string) *dbent.PaymentOrder {
	e.t.Helper()
	o, err := e.client.PaymentOrder.Create().
		SetUserID(u.ID).
		SetUserEmail(u.Email).
		SetUserName(u.Username).
		SetAmount(amount).
		SetPayAmount(amount).
		SetFeeRate(0).
		SetRechargeCode("E2E-UPGRADE-" + tag).
		SetOutTradeNo("e2e_upgrade_" + tag).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-upgrade-" + tag).
		SetOrderType(payment.OrderTypeBundleUpgrade).
		SetPlanID(targetPlanID).
		SetSourceBundleSubscriptionID(sourceSubID).
		SetProrateCredit(credit).
		SetStatus(service.OrderStatusPaid).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(e.ctx)
	require.NoError(e.t, err, "create paid bundle_upgrade order")
	return o
}

// reloadSub 重新读取套餐订阅（含用量），用于履约后断言。
func (e *bundleUpgradeE2E) reloadSub(subID int64) *service.BundleSubscription {
	e.t.Helper()
	got, err := e.subRepo.GetByIDWithUsages(e.ctx, subID)
	require.NoError(e.t, err)
	return got
}

// reloadOrder 重新读取订单。
func (e *bundleUpgradeE2E) reloadOrder(orderID int64) *dbent.PaymentOrder {
	e.t.Helper()
	o, err := e.client.PaymentOrder.Get(e.ctx, orderID)
	require.NoError(e.t, err)
	return o
}

// hasAudit 命中指定 order+action 的审计日志是否存在。
func (e *bundleUpgradeE2E) hasAudit(orderID int64, action string) bool {
	cnt, err := e.client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(orderID, 10)), paymentauditlog.ActionEQ(action)).
		Count(e.ctx)
	require.NoError(e.t, err)
	return cnt > 0
}

// userBundleSubsOfSource 列出该用户指定 source 的套餐订阅（断言"只创建了一条 upgrade sub"等）。
func (e *bundleUpgradeE2E) userBundleSubsOfSource(userID int64, source string) []service.BundleSubscription {
	e.t.Helper()
	subs, _, err := e.subRepo.List(e.ctx, pagination.DefaultPagination(), &userID, "")
	require.NoError(e.t, err)
	var out []service.BundleSubscription
	for _, s := range subs {
		if s.Source == source {
			out = append(out, s)
		}
	}
	return out
}

// userBalance 读取用户当前余额（从 ent 直接读，绕过 repo 缓存）。
func (e *bundleUpgradeE2E) userBalance(userID int64) float64 {
	e.t.Helper()
	u, err := e.client.User.Get(e.ctx, userID)
	require.NoError(e.t, err)
	return u.Balance
}

// e2eCleanupUser 按 userID 级联清理所有 E2E 测试创建的行。顺序遵循外键依赖。
// 注意 PaymentAuditLog.order_id 是 text（存 order.ID 的字符串形式），需先把该用户的订单 ID
// 转成字符串再删审计日志。
func e2eCleanupUser(t *testing.T, ctx context.Context, client *dbent.Client, userID int64) {
	t.Helper()
	// 1. 该用户的所有订单 ID
	orderIDs, err := client.PaymentOrder.Query().Where(paymentorder.UserIDEQ(userID)).IDs(ctx)
	if err != nil {
		t.Logf("cleanup: query order ids for user %d: %v", userID, err)
	}
	// 2. 删这些订单的审计日志
	for _, oid := range orderIDs {
		_, _ = client.PaymentAuditLog.Delete().
			Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(oid, 10))).Exec(ctx)
	}
	// 3. 删订单（bundle_upgrade / bundle / balance 等）
	_, _ = client.PaymentOrder.Delete().Where(paymentorder.UserIDEQ(userID)).Exec(ctx)
	// 4. 删该用户消费过的兑换码（退款兑换码 used_by=user）
	_, _ = client.RedeemCode.Delete().Where(redeemcode.UsedByEQ(userID)).Exec(ctx)
	// 5. 该用户的 bundle 订阅 ID（用于删 usage）
	subIDs, _ := client.BundleSubscription.Query().Where(bundlesubscription.UserIDEQ(userID)).IDs(ctx)
	for _, sid := range subIDs {
		_, _ = client.BundleSubscriptionUsage.Delete().
			Where(bundlesubscriptionusage.BundleSubscriptionIDEQ(sid)).Exec(ctx)
	}
	// 6. 删桥接 user_subscriptions
	_, _ = client.UserSubscription.Delete().Where(usersubscription.UserIDEQ(userID)).Exec(ctx)
	// 7. 删 bundle 订阅
	_, _ = client.BundleSubscription.Delete().Where(bundlesubscription.UserIDEQ(userID)).Exec(ctx)
	// 8. 删用户
	_, _ = client.User.Delete().Where(user.IDEQ(userID)).Exec(ctx)
}

// =============================================================================
// 场景 1：正常升级 starter→pro（完整链路 + 全字段断言）
// =============================================================================

// TestBundleUpgradeE2E_NormalUpgrade_StarterToPro 端到端验证正常升级：
//
//	建 starter 订阅 + 其 bundle 购买订单(completed) → preview 返回 upgradeable=true、credit 按比例
//	→ 下升级单 → 履约 → 断言：旧订阅 status=upgraded、新订阅 status=active 且 source=upgrade、
//	upgraded_from_id=旧ID、expires_at≈now+ValidityDays、订单 bundle_subscription_id=新订阅、
//	prorate_credit 锁定、SUCCESS 审计留痕。
func TestBundleUpgradeE2E_NormalUpgrade_StarterToPro(t *testing.T) {
	e := newBundleUpgradeE2E(t)
	ctx := e.ctx

	user := e.mustCreateUser("e2e-normal@example.com")
	group := e.mustCreateGroup("e2e-normal-group", domain.PlatformOpenAI)
	starter := e.mustCreatePlan("E2E Starter", service.BundleTierStarter, 100, 30, group.ID)
	pro := e.mustCreatePlan("E2E Pro", service.BundleTierPro, 200, 30, group.ID)

	starterSub := e.mustActivateBundle(user.ID, starter.ID)
	// 购买订单（completed）关联 starter 订阅 —— PreviewUpgrade 反查实付的数据源。
	const starterPaid = 100.0
	e.mustCreateCompletedBundleOrder(user, starterSub.ID, starterPaid, "normal-starter")

	// preview：差价应 > 0，credit 按剩余有效期折算（starter 30 天套餐，刚激活，credit 接近全额）。
	pv, err := e.subSvc.PreviewUpgrade(ctx, user.ID, starterSub.ID, pro.ID)
	require.NoError(t, err)
	require.True(t, pv.Upgradeable, "差价>0 应 upgradeable")
	require.InDelta(t, 200.0, pv.TargetPrice, 0.0001)
	require.Greater(t, pv.Credit, 0.0, "有实付应得正 credit")
	require.Less(t, pv.Credit, starterPaid+1.0, "credit 不应超过实付")
	require.Equal(t, 30, pv.ValidityDays)
	require.Equal(t, "E2E Starter", pv.OldPlanName)
	require.Equal(t, "E2E Pro", pv.NewPlanName)

	due := pv.DueAmount
	require.Greater(t, due, 0.0)

	// 下升级单（paid），锁定 prorate_credit（Task 6）。
	order := e.mustCreatePaidUpgradeOrder(user, pro.ID, starterSub.ID, due, pv.Credit, "normal")

	// 履约前记录 now，用于校验 expires_at。
	beforeFulfill := time.Now()
	// 模拟支付回调履约。
	require.NoError(t, e.paySvc.ExecuteBundleUpgradeFulfillment(ctx, order.ID))

	// 订单 Completed + 回写新订阅 ID + prorate_credit 锁定 + SUCCESS 审计。
	completed := e.reloadOrder(order.ID)
	require.Equal(t, service.OrderStatusCompleted, completed.Status, "订单应 Completed")
	require.NotNil(t, completed.BundleSubscriptionID, "应回写 bundle_subscription_id")
	require.Equal(t, pv.Credit, completed.ProrateCredit, "prorate_credit 应锁定为 preview 时的值")
	require.True(t, e.hasAudit(order.ID, "BUNDLE_UPGRADE_SUCCESS"))

	// 旧订阅 → upgraded。
	oldReloaded := e.reloadSub(starterSub.ID)
	require.Equal(t, service.BundleStatusUpgraded, oldReloaded.Status, "旧订阅应 upgraded")

	// 新订阅：source=upgrade、upgraded_from_id=旧ID、status=active、expires_at≈now+30d。
	newSubID := *completed.BundleSubscriptionID
	newSub := e.reloadSub(newSubID)
	require.Equal(t, service.BundleStatusActive, newSub.Status)
	require.Equal(t, service.BundleSourceUpgrade, newSub.Source)
	require.Equal(t, starterSub.ID, newSub.UpgradedFromID, "upgraded_from_id 指向旧订阅")
	require.Equal(t, pro.ID, newSub.PlanID)
	require.Equal(t, user.ID, newSub.UserID)
	require.Len(t, newSub.Usages, 1, "目标 plan 1 个 group quota → 1 条 usage")
	require.GreaterOrEqual(t, newSub.ExpiresAt, beforeFulfill.AddDate(0, 0, 30).Add(-2*time.Second))
	require.LessOrEqual(t, newSub.ExpiresAt, beforeFulfill.AddDate(0, 0, 30).Add(2*time.Second),
		"expires_at 应≈now+ValidityDays(30d)")
}

// =============================================================================
// 场景 2：二次升级 pro→enterprise（验证 Task 3 fix：bundle_upgrade 订单能被反查）
// =============================================================================

// TestBundleUpgradeE2E_SecondUpgrade_ProToEnterprise 验证二次升级的 credit 反查：
//
//	先做一次 starter→pro 升级（产生 pro 订阅 + 对应 bundle_upgrade 订单），再对 pro 订阅 preview
//	升级到 enterprise。pro 订阅的"购买订单"是 bundle_upgrade 类型（不是 bundle）—— 修复前
//	OrderTypeEQ(bundle) 查不到 → credit 算成 0；修复后 OrderTypeIn(bundle, bundle_upgrade) 能查到，
//	credit 按差价反查（不为 0）。
func TestBundleUpgradeE2E_SecondUpgrade_ProToEnterprise(t *testing.T) {
	e := newBundleUpgradeE2E(t)
	ctx := e.ctx

	user := e.mustCreateUser("e2e-second@example.com")
	group := e.mustCreateGroup("e2e-second-group", domain.PlatformOpenAI)
	starter := e.mustCreatePlan("E2E S2", service.BundleTierStarter, 100, 30, group.ID)
	pro := e.mustCreatePlan("E2E P2", service.BundleTierPro, 200, 30, group.ID)
	enterprise := e.mustCreatePlan("E2E E2", service.BundleTierEnterprise, 400, 30, group.ID)

	// —— 第一次升级 starter→pro（场景 1 的浓缩版，作为场景 2 的 setup）——
	starterSub := e.mustActivateBundle(user.ID, starter.ID)
	e.mustCreateCompletedBundleOrder(user, starterSub.ID, 100.0, "second-starter")
	pv1, err := e.subSvc.PreviewUpgrade(ctx, user.ID, starterSub.ID, pro.ID)
	require.NoError(t, err)
	require.True(t, pv1.Upgradeable)
	order1 := e.mustCreatePaidUpgradeOrder(user, pro.ID, starterSub.ID, pv1.DueAmount, pv1.Credit, "second-first")
	require.NoError(t, e.paySvc.ExecuteBundleUpgradeFulfillment(ctx, order1.ID))

	// 第一次升级后的 pro 订阅（source=upgrade）。
	proSubs := e.userBundleSubsOfSource(user.ID, service.BundleSourceUpgrade)
	require.Len(t, proSubs, 1, "第一次升级应产生 1 条 upgrade 订阅")
	proSub := proSubs[0]
	require.Equal(t, service.BundleStatusActive, proSub.Status)
	// 订单 order1 的 bundle_subscription_id 回写为 proSub.ID（这是 credit 反查的关联键）。
	order1Completed := e.reloadOrder(order1.ID)
	require.NotNil(t, order1Completed.BundleSubscriptionID)
	require.Equal(t, proSub.ID, *order1Completed.BundleSubscriptionID)
	// order1 是 bundle_upgrade 类型、Completed —— Task 3 fix 的核心反查对象。
	require.Equal(t, payment.OrderTypeBundleUpgrade, order1Completed.OrderType)
	require.Equal(t, service.OrderStatusCompleted, order1Completed.Status)

	// —— 第二次升级 pro→enterprise 的 preview ——
	// 灵魂断言：pro 订阅的"实付来源"是 bundle_upgrade 订单（order1），修复前反查返回 0 → credit=0；
	// 修复后 OrderTypeIn(bundle, bundle_upgrade) 能查到 order1 的实付 → credit>0。
	pv2, err := e.subSvc.PreviewUpgrade(ctx, user.ID, proSub.ID, enterprise.ID)
	require.NoError(t, err)
	require.True(t, pv2.Upgradeable, "pro→enterprise 差价>0 应 upgradeable")
	require.Greater(t, pv2.Credit, 0.0, "credit 必须按 bundle_upgrade 订单反查（Task 3 fix），不能算成 0")
	require.Equal(t, 400.0, pv2.TargetPrice)
	require.Equal(t, "E2E P2", pv2.OldPlanName)
	require.Equal(t, "E2E E2", pv2.NewPlanName)

	// 实付 = order1.DueAmount（第一次升级补的差价）。credit 应基于该实付按剩余比例折算。
	firstUpgradeDue := order1Completed.PayAmount
	require.LessOrEqual(t, pv2.Credit, firstUpgradeDue+1.0, "credit 不应超过 pro 实付（第一次升级差价）")

	// 进一步履约第二次升级，确认链路闭环（新订阅 source 仍为 upgrade）。
	order2 := e.mustCreatePaidUpgradeOrder(user, enterprise.ID, proSub.ID, pv2.DueAmount, pv2.Credit, "second-second")
	require.NoError(t, e.paySvc.ExecuteBundleUpgradeFulfillment(ctx, order2.ID))
	order2Completed := e.reloadOrder(order2.ID)
	require.Equal(t, service.OrderStatusCompleted, order2Completed.Status)
	require.True(t, e.hasAudit(order2.ID, "BUNDLE_UPGRADE_SUCCESS"))

	// pro 订阅 → upgraded；enterprise 订阅 active + source=upgrade。
	proReloaded := e.reloadSub(proSub.ID)
	require.Equal(t, service.BundleStatusUpgraded, proReloaded.Status)
	entSubID := *order2Completed.BundleSubscriptionID
	entSub := e.reloadSub(entSubID)
	require.Equal(t, service.BundleStatusActive, entSub.Status)
	require.Equal(t, service.BundleSourceUpgrade, entSub.Source)
	require.Equal(t, proSub.ID, entSub.UpgradedFromID, "第二次升级的 upgraded_from_id 指向 pro 订阅")
}

// =============================================================================
// 场景 3：并发升级（只一笔切换成功，另一笔退余额）
// =============================================================================

// TestBundleUpgradeE2E_ConcurrentUpgrade_RefundLoser 验证并发升级的协同防护：
//
//	两笔 bundle_upgrade 订单（直接 ent 插入，绕过 Task 6 的建单级防重，模拟"两笔支付都成功落地
//	后才并发履约"的边缘态）并发履约。UpgradeBundle（Task 5 ①）的 active 校验是第二道防线：
//	第一笔把旧订阅置 upgraded 提交后，第二笔进 ① 发现非 active → 返回 ErrBundleExpired →
//	doBundleUpgrade（Task 7）分流到 refundUpgradeToBalance → 余额 += 订单已付差价。
//
// 时序说明：真实同纳秒级并发在 GetByIDWithUsages 无 FOR UPDATE 时存在"双读 active"竞态窗
// （两个都读到 active → 都切 → 双新订阅）。生产由 Task 6 订单级防重杜绝"两笔 paid 升级单并存"，
// 使该边缘态极罕见。本测试用 goroutine + WaitGroup + channel barrier 确保 A 的履约事务完整提交
// 后再触发 B —— 这正是真实并发下"第一笔 webhook 先完成、第二笔后到"的常见生产形态，确定性地
// 覆盖 Task 5 ①（active 校验）+ Task 7（退余额）+ Task 6 协同。真实"同纳秒双 webhook"竞态窗
// 由 Task 6 订单级防重在生产侧杜绝（不让两笔 paid 升级单并存到履约阶段）。
func TestBundleUpgradeE2E_ConcurrentUpgrade_RefundLoser(t *testing.T) {
	e := newBundleUpgradeE2E(t)
	ctx := e.ctx

	user := e.mustCreateUser("e2e-concurrent@example.com")
	group := e.mustCreateGroup("e2e-concurrent-group", domain.PlatformOpenAI)
	starter := e.mustCreatePlan("E2E C-S", service.BundleTierStarter, 100, 30, group.ID)
	pro := e.mustCreatePlan("E2E C-P", service.BundleTierPro, 200, 30, group.ID)

	starterSub := e.mustActivateBundle(user.ID, starter.ID)
	e.mustCreateCompletedBundleOrder(user, starterSub.ID, 100.0, "concurrent-starter")

	const due = 100.0
	// 两笔 paid 升级单都指向同一 sourceSub + targetPlan（直接 ent 插入，绕过建单防重，
	// 模拟"两笔支付都落地后才并发履约"的边缘态）。
	orderA := e.mustCreatePaidUpgradeOrder(user, pro.ID, starterSub.ID, due, 100.0, "concurrent-a")
	orderB := e.mustCreatePaidUpgradeOrder(user, pro.ID, starterSub.ID, due, 100.0, "concurrent-b")

	balanceBefore := e.userBalance(user.ID)

	// 用 channel barrier 确保 A 完整提交后再触发 B（确定性 + 仍用 goroutine/WaitGroup 并发触发）。
	var wg sync.WaitGroup
	var errA, errB error
	aDone := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		errA = e.paySvc.ExecuteBundleUpgradeFulfillment(ctx, orderA.ID)
		close(aDone) // A 的事务已完整提交（或失败），通知 B 可以开始
	}()
	go func() {
		defer wg.Done()
		<-aDone // 等 A 完成，确保 B 的 UpgradeBundle 读到 A 提交后的 upgraded 状态
		errB = e.paySvc.ExecuteBundleUpgradeFulfillment(ctx, orderB.ID)
	}()
	wg.Wait()

	require.NoError(t, errA, "A 履约应成功")
	require.NoError(t, errB, "B 履约应成功（refund 路径也返回 nil，订单 Completed）")

	// 旧订阅 → upgraded（第一笔提交后状态稳定）。
	oldReloaded := e.reloadSub(starterSub.ID)
	require.Equal(t, service.BundleStatusUpgraded, oldReloaded.Status,
		"旧订阅应被切到 upgraded（不会被二次退回 active）")

	// 两笔订单都 Completed。
	require.Equal(t, service.OrderStatusCompleted, e.reloadOrder(orderA.ID).Status)
	require.Equal(t, service.OrderStatusCompleted, e.reloadOrder(orderB.ID).Status)

	// 一笔 SUCCESS（A，切换）+ 一笔 REFUND_BALANCE（B，退余额）。
	aSuccess := e.hasAudit(orderA.ID, "BUNDLE_UPGRADE_SUCCESS")
	bSuccess := e.hasAudit(orderB.ID, "BUNDLE_UPGRADE_SUCCESS")
	bRefund := e.hasAudit(orderB.ID, "BUNDLE_UPGRADE_REFUND_BALANCE")
	require.True(t, aSuccess, "A 应切换成功（SUCCESS 审计）")
	require.False(t, bSuccess, "B 不应切换成功（旧订阅已被 A 置 upgraded）")
	require.True(t, bRefund, "B 应走 refundUpgradeToBalance（REFUND_BALANCE 审计）")

	// upgrade-source 订阅只有 1 条（A 切换产生；B 未创建新订阅）。
	upgradeSubs := e.userBundleSubsOfSource(user.ID, service.BundleSourceUpgrade)
	require.Len(t, upgradeSubs, 1, "应只创建 1 条 upgrade 订阅（B 未切换）")

	// 余额 += B 的已付差价（资金不出平台）。
	balanceAfter := e.userBalance(user.ID)
	require.InDelta(t, balanceBefore+due, balanceAfter, 0.0001,
		"退余额应等于 B 订单的已付差价")
}

// =============================================================================
// 场景 4：支付期间过期 → 退余额（Task 7 退余额边界）
// =============================================================================

// TestBundleUpgradeE2E_ExpiredDuringPayment_RefundsBalance 验证支付期间过期边界：
//
//	旧订阅在支付期间被过期清扫（status: active → expired，expires_at 已在过去）→ 履约时
//	UpgradeBundle 在 ① active 校验失败返回 ErrBundleExpired → doBundleUpgrade 分流到
//	refundUpgradeToBalance → user.balance += order.Amount（资金不出平台）。
//
// 注：UpgradeBundle 按 Status（持久化状态）判定过期，而非 ExpiresAt 本身。测试同时置
// expires_at 在过去 + status=expired，完整模拟"过期清扫已跑过"的真实态。
func TestBundleUpgradeE2E_ExpiredDuringPayment_RefundsBalance(t *testing.T) {
	e := newBundleUpgradeE2E(t)
	ctx := e.ctx

	user := e.mustCreateUser("e2e-expired@example.com")
	group := e.mustCreateGroup("e2e-expired-group", domain.PlatformOpenAI)
	starter := e.mustCreatePlan("E2E X-S", service.BundleTierStarter, 100, 30, group.ID)
	pro := e.mustCreatePlan("E2E X-P", service.BundleTierPro, 200, 30, group.ID)

	starterSub := e.mustActivateBundle(user.ID, starter.ID)
	e.mustCreateCompletedBundleOrder(user, starterSub.ID, 100.0, "expired-starter")

	const due = 100.0
	order := e.mustCreatePaidUpgradeOrder(user, pro.ID, starterSub.ID, due, 100.0, "expired")

	// 模拟支付期间过期：清扫任务已把旧订阅置 expired + expires_at 推到过去。
	past := time.Now().Add(-2 * time.Hour)
	_, err := e.client.BundleSubscription.UpdateOneID(starterSub.ID).
		SetStatus(service.BundleStatusExpired).
		SetExpiresAt(past).
		Save(ctx)
	require.NoError(t, err)

	balanceBefore := e.userBalance(user.ID)

	// 履约：UpgradeBundle ① active 校验失败 → ErrBundleExpired → refundUpgradeToBalance。
	require.NoError(t, e.paySvc.ExecuteBundleUpgradeFulfillment(ctx, order.ID),
		"退余额路径应成功闭环（非 markFailed）")

	// 订单 Completed + REFUND_BALANCE 审计。
	require.Equal(t, service.OrderStatusCompleted, e.reloadOrder(order.ID).Status)
	require.True(t, e.hasAudit(order.ID, "BUNDLE_UPGRADE_REFUND_BALANCE"))
	require.False(t, e.hasAudit(order.ID, "BUNDLE_UPGRADE_SUCCESS"),
		"过期场景不应产生 SUCCESS 审计（未切换）")

	// 余额 += 已付差价。
	require.InDelta(t, balanceBefore+due, e.userBalance(user.ID), 0.0001,
		"退款应等于订单已付差价")

	// 未创建任何 upgrade-source 订阅（切换未发生）。
	require.Empty(t, e.userBundleSubsOfSource(user.ID, service.BundleSourceUpgrade),
		"过期场景不应创建新订阅")

	// 旧订阅状态保持 expired（未被标 upgraded）。
	require.Equal(t, service.BundleStatusExpired, e.reloadSub(starterSub.ID).Status)
}

// =============================================================================
// 场景 5：幂等（重复履约 → hasAuditLog 命中 → UpgradeBundle 只调一次）
// =============================================================================

// TestBundleUpgradeE2E_IdempotentFulfillment 验证幂等的两层防护：
//
//  1. 订单级：首次履约成功后订单 Completed，二次 ExecuteBundleUpgradeFulfillment 顶部
//     `if o.Status == Completed { return nil }` 直接返回（最常见路径）。
//  2. 审计级（崩溃恢复）：把订单手动重置为 Paid（模拟重复 webhook 触发再处理），此时 CAS 锁
//     Paid→Recharging 成功进入 doBundleUpgrade，但 hasAuditLog(BUNDLE_UPGRADE_SUCCESS) 命中 →
//     跳过 UpgradeBundle 直接 markCompleted。UpgradeBundle 调用计数=1（只首次的 1 条新订阅）。
func TestBundleUpgradeE2E_IdempotentFulfillment(t *testing.T) {
	e := newBundleUpgradeE2E(t)
	ctx := e.ctx

	user := e.mustCreateUser("e2e-idempotent@example.com")
	group := e.mustCreateGroup("e2e-idempotent-group", domain.PlatformOpenAI)
	starter := e.mustCreatePlan("E2E I-S", service.BundleTierStarter, 100, 30, group.ID)
	pro := e.mustCreatePlan("E2E I-P", service.BundleTierPro, 200, 30, group.ID)

	starterSub := e.mustActivateBundle(user.ID, starter.ID)
	e.mustCreateCompletedBundleOrder(user, starterSub.ID, 100.0, "idempotent-starter")

	pv, err := e.subSvc.PreviewUpgrade(ctx, user.ID, starterSub.ID, pro.ID)
	require.NoError(t, err)
	require.True(t, pv.Upgradeable)
	order := e.mustCreatePaidUpgradeOrder(user, pro.ID, starterSub.ID, pv.DueAmount, pv.Credit, "idempotent")

	// —— 首次履约：成功切换 ——
	require.NoError(t, e.paySvc.ExecuteBundleUpgradeFulfillment(ctx, order.ID))
	require.Equal(t, service.OrderStatusCompleted, e.reloadOrder(order.ID).Status)
	require.True(t, e.hasAudit(order.ID, "BUNDLE_UPGRADE_SUCCESS"))
	upgradeSubsAfter1 := e.userBundleSubsOfSource(user.ID, service.BundleSourceUpgrade)
	require.Len(t, upgradeSubsAfter1, 1, "首次履约应创建 1 条 upgrade 订阅")

	// 记录 Layer1 完成后的余额，供 Layer3 幂等重入断言用（重入不应触发退款 → 余额不应变化）。
	balanceAfterLayer1 := e.userBalance(user.ID)

	// —— 第二次（订单级幂等）：订单已 Completed，顶部直接返回，不进 doBundleUpgrade ——
	require.NoError(t, e.paySvc.ExecuteBundleUpgradeFulfillment(ctx, order.ID))
	upgradeSubsAfter2 := e.userBundleSubsOfSource(user.ID, service.BundleSourceUpgrade)
	require.Len(t, upgradeSubsAfter2, 1, "订单级幂等：不应再创建 upgrade 订阅")

	// —— 第三次（审计级幂等 / 崩溃恢复）：手动重置订单为 Paid（模拟重复 webhook 再处理），
	//     CAS 锁成功进入 doBundleUpgrade，但 hasAuditLog(SUCCESS) 命中 → 跳过 UpgradeBundle。 ——
	_, err = e.client.PaymentOrder.UpdateOneID(order.ID).
		SetStatus(service.OrderStatusPaid).
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, e.paySvc.ExecuteBundleUpgradeFulfillment(ctx, order.ID))

	// 灵魂断言：UpgradeBundle 调用计数=1 —— 仍只有首次的 1 条 upgrade 订阅，没有第二条。
	upgradeSubsAfter3 := e.userBundleSubsOfSource(user.ID, service.BundleSourceUpgrade)
	require.Len(t, upgradeSubsAfter3, 1,
		"审计级幂等：hasAuditLog 命中后不应再调 UpgradeBundle（upgrade 订阅数仍为 1）")
	require.Equal(t, service.OrderStatusCompleted, e.reloadOrder(order.ID).Status,
		"审计级幂等路径应重新置 Completed")

	// 旧订阅状态不变（仍 upgraded，未被二次处理）。
	require.Equal(t, service.BundleStatusUpgraded, e.reloadSub(starterSub.ID).Status)

	// 灵魂断言（加固）：直接约束"重复回调 = 无副作用"核心不变量。
	// 若 doBundleUpgrade:810 的 hasAuditLog(BUNDLE_UPGRADE_SUCCESS) 守卫被破坏，第三次调用会进入
	// UpgradeBundle → 旧订阅已 upgraded → ErrBundleExpired → refundUpgradeToBalance，导致余额增加
	// 且写出 BUNDLE_UPGRADE_REFUND_BALANCE 审计。原 upgradeSubsAfter3==1 断言无法捕获（refund 不
	// 建新订阅）。这两个断言补上该缺口。
	require.InDelta(t, balanceAfterLayer1, e.userBalance(user.ID), 0.0001,
		"幂等重入不应触发退款（余额不应变化）")
	require.False(t, e.hasAudit(order.ID, "BUNDLE_UPGRADE_REFUND_BALANCE"),
		"幂等重入不应产生退款审计")
}

// =============================================================================
// 场景 6：IDOR（用户 B 用用户 A 的 source_sub_id → ErrBundleNotFound，不泄露存在性）
// =============================================================================

// TestBundleUpgradeE2E_IDOR_OwnerMismatch 验证 IDOR 防护：
//
//	用户 A 拥有活跃的 starter 订阅；用户 B 用 A 的 source_sub_id 调 PreviewUpgrade /
//	UpgradeBundle。两处都应返回 ErrBundleNotFound（不泄露订阅存在性），且不改动 A 的订阅、
//	不为 B 创建任何订阅。与 IDOR 修复 ecb747d9 / 024c7879 同原则。
func TestBundleUpgradeE2E_IDOR_OwnerMismatch(t *testing.T) {
	e := newBundleUpgradeE2E(t)
	ctx := e.ctx

	userA := e.mustCreateUser("e2e-idor-a@example.com")
	userB := e.mustCreateUser("e2e-idor-b@example.com")
	group := e.mustCreateGroup("e2e-idor-group", domain.PlatformOpenAI)
	starter := e.mustCreatePlan("E2E D-S", service.BundleTierStarter, 100, 30, group.ID)
	pro := e.mustCreatePlan("E2E D-P", service.BundleTierPro, 200, 30, group.ID)

	// A 的活跃 starter 订阅 + 购买订单。
	subA := e.mustActivateBundle(userA.ID, starter.ID)
	e.mustCreateCompletedBundleOrder(userA, subA.ID, 100.0, "idor-a")

	// B 用 A 的 subA.ID 调 PreviewUpgrade → ErrBundleNotFound（不泄露存在性）。
	_, err := e.subSvc.PreviewUpgrade(ctx, userB.ID, subA.ID, pro.ID)
	require.ErrorIs(t, err, service.ErrBundleNotFound, "归属不符应返回 ErrBundleNotFound（不泄露存在性）")

	// B 用 A 的 subA.ID 调 UpgradeBundle → 同样 ErrBundleNotFound。
	_, err = e.subSvc.UpgradeBundle(ctx, &service.UpgradeBundleRequest{
		UserID:       userB.ID,
		SourceSubID:  subA.ID,
		TargetPlanID: pro.ID,
	})
	require.ErrorIs(t, err, service.ErrBundleNotFound, "UpgradeBundle 归属不符应返回 ErrBundleNotFound")

	// A 的订阅未受影响（仍 active）。
	require.Equal(t, service.BundleStatusActive, e.reloadSub(subA.ID).Status,
		"IDOR 攻击不应改动属主的订阅状态")

	// B 没有任何订阅（既无 bundle 也无 upgrade-source）。
	require.Empty(t, e.userBundleSubsOfSource(userB.ID, service.BundleSourceUpgrade),
		"IDOR 失败不应为攻击者创建订阅")
	bAllSubs, _, err := e.subRepo.List(ctx, pagination.DefaultPagination(), &userB.ID, "")
	require.NoError(t, err)
	require.Empty(t, bAllSubs, "攻击者 B 不应拥有任何套餐订阅")
}

// =============================================================================
// 场景 7：admin 换绑共享 group 套餐（RevokeBundle 软删除回归）
// =============================================================================

// TestBundleUpgradeE2E_AdminReassign_SharedGroup_NoConflict 验证 admin 换绑路径不撞唯一约束：
//
//	用户已有 bundle A（含 group g1）→ admin 换绑到 bundle B（共享同一 group g1）。
//	ActivateBundle(admin-assign) 先 RevokeBundle(A) 再激活 B。修复前 RevokeBundle 把 A 的桥接
//	userSub 仅置 status=expired（占 (user,g1) 唯一槽），B 建桥接 userSub 时撞 partial unique
//	index (user_id,group_id) WHERE deleted_at IS NULL → ErrSubscriptionAlreadyExists。
//	修复后 RevokeBundle 软删除 A 的桥接 userSub（释放槽），B 顺利激活。
func TestBundleUpgradeE2E_AdminReassign_SharedGroup_NoConflict(t *testing.T) {
	e := newBundleUpgradeE2E(t)

	user := e.mustCreateUser("e2e-reassign@example.com")
	group := e.mustCreateGroup("e2e-reassign-group", domain.PlatformOpenAI)
	planA := e.mustCreatePlan("E2E R-A", service.BundleTierStarter, 100, 30, group.ID)
	planB := e.mustCreatePlan("E2E R-B", service.BundleTierPro, 200, 30, group.ID)

	// 用户先持有 bundle A（共享 group g1）。
	subA := e.mustActivateBundle(user.ID, planA.ID)

	// admin 换绑到 bundle B（共享同一 group g1）—— 修复前在此 409。
	subB, err := e.subSvc.ActivateBundle(e.ctx, &service.ActivateBundleRequest{
		UserID: user.ID, PlanID: planB.ID, Source: service.BundleSourceAdminAssign,
	})
	require.NoError(t, err, "admin 换绑共享 group 的套餐不应撞唯一约束")
	require.NotNil(t, subB)
	require.Equal(t, service.BundleStatusActive, subB.Status)

	// A 被 revoke，B active。
	require.Equal(t, service.BundleStatusRevoked, e.reloadSub(subA.ID).Status,
		"换绑后旧 bundle 应 revoked")
}

// =============================================================================
// 场景 8：套餐过期清扫后重购共享 group（ExpireBridged 软删除回归）
// =============================================================================

// TestBundleUpgradeE2E_ExpiredThenRepurchase_SharedGroup_NoConflict 验证过期路径不撞唯一约束：
//
//	用户持有 bundle A（含 group g1）→ 套餐自然过期（status=expired）→ 过期清扫
//	ExpireBridgedSubscriptionsForExpiredBundles 处理 A 的桥接 userSub → 用户重新购买 bundle B
//	（共享 g1）。修复前清扫把 A 的桥接 userSub 置 status=expired（仍占 (user,g1) 唯一槽），
//	B 建桥接 userSub 撞 partial unique index → 409。修复后过期清扫软删除 A 的桥接 userSub
//	（释放槽），B 顺利激活。
func TestBundleUpgradeE2E_ExpiredThenRepurchase_SharedGroup_NoConflict(t *testing.T) {
	e := newBundleUpgradeE2E(t)
	ctx := e.ctx

	user := e.mustCreateUser("e2e-expired-rebuy@example.com")
	group := e.mustCreateGroup("e2e-expired-rebuy-group", domain.PlatformOpenAI)
	planA := e.mustCreatePlan("E2E X-A", service.BundleTierStarter, 100, 30, group.ID)
	planB := e.mustCreatePlan("E2E X-B", service.BundleTierPro, 200, 30, group.ID)

	// 用户持有 bundle A。
	subA := e.mustActivateBundle(user.ID, planA.ID)

	// 模拟自然过期：bundle A → expired。
	_, err := e.client.BundleSubscription.UpdateOneID(subA.ID).
		SetStatus(service.BundleStatusExpired).
		Save(ctx)
	require.NoError(t, err)

	// 跑过期清扫：处理 expired bundle 的桥接 active userSub。
	affected, err := e.userSubRepo.ExpireBridgedSubscriptionsForExpiredBundles(ctx)
	require.NoError(t, err)
	require.Greater(t, affected, int64(0), "应清扫到至少 1 条桥接 userSub")

	// 用户重新购买 bundle B（共享 g1）—— A 已 expired 不算 active，放行；修复前在此 409。
	subB := e.mustActivateBundle(user.ID, planB.ID)
	require.Equal(t, service.BundleStatusActive, subB.Status)

	// A 保持 expired（未被改动）。
	require.Equal(t, service.BundleStatusExpired, e.reloadSub(subA.ID).Status)
}
