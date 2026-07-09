//go:build unit

// payment_order_upgrade_test.go 套餐升级订单建单测试（Task 6）
// 守护：bundle_upgrade 订单创建时必须写入 SourceBundleSubscriptionID、ProrateCredit、PlanID，
// 否则支付回调履约阶段无法关联旧订阅（差价抵扣丢失）且 fulfillment 因 PlanID 缺失直接失败。
package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

// TestCreateOrderInTxWritesUpgradeFields 验证 createOrderInTx 对 bundle_upgrade 订单
// 写入升级专属字段（source 订阅、prorate credit）及目标 plan_id。
func TestCreateOrderInTxWritesUpgradeFields(t *testing.T) {
	ctx := context.Background()
	client := newBundleDedupClient(t)
	uid := createDedupUser(t, client, "upgrade-create@example.com")

	svc := &PaymentService{entClient: client}

	const (
		targetPlanID    int64 = 777
		sourceSubID     int64 = 42
		prorateCredit         = 30.0
		upgradePayAmount      = 50.0
	)

	req := CreateOrderRequest{
		UserID:                     uid,
		Amount:                     upgradePayAmount,
		PaymentType:                "alipay",
		OrderType:                  payment.OrderTypeBundleUpgrade,
		PlanID:                     targetPlanID,
		SourceBundleSubscriptionID: sourceSubID,
		ProrateCredit:              prorateCredit,
	}
	// service.User（非 dbent.User）；createOrderInTx 只读取 Email/Username/Notes。
	user := &User{
		ID:       uid,
		Email:    "upgrade-create@example.com",
		Username: "upgrade-create",
	}
	// PaymentConfig 零值：MaxPendingOrders=0→默认 3，DailyLimit=0→跳过日限校验，
	// OrderTimeoutMin=0→默认 30min。plan 传 nil（bundle plan 非 SubscriptionPlan）。
	cfg := &PaymentConfig{}

	order, err := svc.createOrderInTx(ctx, req, user, nil, cfg, upgradePayAmount, upgradePayAmount, 0, upgradePayAmount, 0, nil)
	require.NoError(t, err)
	require.NotNil(t, order)

	require.Equal(t, payment.OrderTypeBundleUpgrade, order.OrderType)
	require.NotNil(t, order.PlanID, "bundle_upgrade order must persist plan_id for fulfillment")
	require.Equal(t, targetPlanID, *order.PlanID)
	require.Equal(t, sourceSubID, order.SourceBundleSubscriptionID, "source bundle subscription id must be persisted")
	require.Equal(t, prorateCredit, order.ProrateCredit, "prorate credit must be persisted")
}

// TestCreateOrderInTxOmitsUpgradeFieldsForBundle 验证普通 bundle 订单不写入升级专属字段，
// 防止字段误写扩散到非升级订单（SourceBundleSubscriptionID 应保持零值）。
func TestCreateOrderInTxOmitsUpgradeFieldsForBundle(t *testing.T) {
	ctx := context.Background()
	client := newBundleDedupClient(t)
	uid := createDedupUser(t, client, "bundle-create@example.com")

	svc := &PaymentService{entClient: client}

	req := CreateOrderRequest{
		UserID:      uid,
		Amount:      20,
		PaymentType: "alipay",
		OrderType:   payment.OrderTypeBundle,
		PlanID:      123,
	}
	user := &User{ID: uid, Email: "bundle-create@example.com", Username: "bundle-create"}
	cfg := &PaymentConfig{}

	order, err := svc.createOrderInTx(ctx, req, user, nil, cfg, 20, 20, 0, 20, 0, nil)
	require.NoError(t, err)
	require.NotNil(t, order)

	require.Equal(t, payment.OrderTypeBundle, order.OrderType)
	require.Zero(t, order.SourceBundleSubscriptionID, "non-upgrade order must not set source subscription id")
	require.Zero(t, order.ProrateCredit, "non-upgrade order must not set prorate credit")
}
