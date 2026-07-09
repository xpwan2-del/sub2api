//go:build integration

// payment_order_repo_integration_test.go PaymentOrderReader 集成测试
// 重点覆盖 GetPaidAmountByBundleSub 对 bundle_upgrade 订单类型的反查
// （二次升级场景：pro→enterprise 时 pro 订阅的购买订单是 bundle_upgrade 类型）。

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// newPaymentOrderReaderForTest 用共享集成 ent client 构造 PaymentOrderReader。
func newPaymentOrderReaderForTest(t *testing.T) (service.PaymentOrderReader, *dbent.Client) {
	t.Helper()
	client := testEntClient(t)
	return NewPaymentOrderReader(client), client
}

// createBundleOrderForTest 在集成 DB 中创建一笔套餐相关订单并返回其 ID。
// bundleSubID 用于关联订单与套餐订阅；orderType 决定订单类型。
func createBundleOrderForTest(t *testing.T, ctx context.Context, client *dbent.Client, bundleSubID int64, orderType, status string, amount float64) *dbent.PaymentOrder {
	t.Helper()
	user, err := client.User.Create().
		SetEmail("po-repo-" + t.Name() + "@example.com").
		SetPasswordHash("hash").
		SetUsername("po-repo-user-" + t.Name()).
		Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = client.PaymentOrder.Delete().Where().Exec(ctx)
		_, _ = client.User.Delete().Where().Exec(ctx)
	})
	o, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(amount).
		SetPayAmount(amount).
		SetFeeRate(0).
		SetRechargeCode("PO-REPO-" + t.Name()).
		SetOutTradeNo("po_repo_" + t.Name()).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-" + t.Name()).
		SetOrderType(orderType).
		SetStatus(status).
		SetBundleSubscriptionID(bundleSubID).
		SetPaidAt(time.Now()).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)
	return o
}

// TestGetPaidAmountByBundleSub_ReturnsZeroWhenNoOrder 验证无订单时返回 0,nil（兑换/赠送来源）。
func TestGetPaidAmountByBundleSub_ReturnsZeroWhenNoOrder(t *testing.T) {
	ctx := context.Background()
	reader, _ := newPaymentOrderReaderForTest(t)

	amount, err := reader.GetPaidAmountByBundleSub(ctx, 999999991)
	require.NoError(t, err)
	require.Equal(t, float64(0), amount)
}

// TestGetPaidAmountByBundleSub_MatchesBundleOrder 验证普通 bundle 订单可被反查（回归保护）。
func TestGetPaidAmountByBundleSub_MatchesBundleOrder(t *testing.T) {
	ctx := context.Background()
	reader, client := newPaymentOrderReaderForTest(t)
	const bundleSubID = 99001

	createBundleOrderForTest(t, ctx, client, bundleSubID, payment.OrderTypeBundle, payment.OrderStatusCompleted, 99.5)

	amount, err := reader.GetPaidAmountByBundleSub(ctx, bundleSubID)
	require.NoError(t, err)
	require.Equal(t, 99.5, amount)
}

// TestGetPaidAmountByBundleSub_MatchesBundleUpgradeOrder 覆盖修复核心：
// 二次升级场景下，来源订阅的购买订单是 bundle_upgrade 类型（而非 bundle），
// 修复前 OrderTypeEQ(bundle) 查不到该订单 → 返回 0 → 来源订阅剩余价值被算成 0；
// 修复后 OrderTypeIn(bundle, bundle_upgrade) 能查到并返回真实实付金额。
func TestGetPaidAmountByBundleSub_MatchesBundleUpgradeOrder(t *testing.T) {
	ctx := context.Background()
	reader, client := newPaymentOrderReaderForTest(t)
	const bundleSubID = 99002

	createBundleOrderForTest(t, ctx, client, bundleSubID, payment.OrderTypeBundleUpgrade, payment.OrderStatusCompleted, 150.25)

	amount, err := reader.GetPaidAmountByBundleSub(ctx, bundleSubID)
	require.NoError(t, err)
	require.Equal(t, 150.25, amount, "bundle_upgrade 订单应被反查到，二次升级 credit 不能算成 0")
}

// TestGetPaidAmountByBundleSub_IgnoresNonCompletedOrders 验证只匹配 COMPLETED 订单，
// PENDING 的 bundle_upgrade 订单不应被计入。
func TestGetPaidAmountByBundleSub_IgnoresNonCompletedOrders(t *testing.T) {
	ctx := context.Background()
	reader, client := newPaymentOrderReaderForTest(t)
	const bundleSubID = 99003

	createBundleOrderForTest(t, ctx, client, bundleSubID, payment.OrderTypeBundleUpgrade, payment.OrderStatusPending, 200)

	amount, err := reader.GetPaidAmountByBundleSub(ctx, bundleSubID)
	require.NoError(t, err)
	require.Equal(t, float64(0), amount, "非 COMPLETED 订单不应被计入实付金额")
}

// TestGetPaidAmountByBundleSub_PicksEarliestOrder 验证多笔匹配订单时取最早一笔
// （按 created_at ASC + First）。
func TestGetPaidAmountByBundleSub_PicksEarliestOrder(t *testing.T) {
	ctx := context.Background()
	reader, client := newPaymentOrderReaderForTest(t)
	const bundleSubID = 99004

	earliest := createBundleOrderForTest(t, ctx, client, bundleSubID, payment.OrderTypeBundle, payment.OrderStatusCompleted, 50)
	// 第二笔稍晚创建
	_, err := client.PaymentOrder.Create().
		SetUserID(earliest.UserID).
		SetUserEmail(earliest.UserEmail).
		SetUserName(earliest.UserName).
		SetAmount(300).
		SetPayAmount(300).
		SetFeeRate(0).
		SetRechargeCode("PO-REPO-LATE-" + t.Name()).
		SetOutTradeNo("po_repo_late_" + t.Name()).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-late-" + t.Name()).
		SetOrderType(payment.OrderTypeBundleUpgrade).
		SetStatus(payment.OrderStatusCompleted).
		SetBundleSubscriptionID(bundleSubID).
		SetPaidAt(time.Now()).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	amount, err := reader.GetPaidAmountByBundleSub(ctx, bundleSubID)
	require.NoError(t, err)
	require.Equal(t, 50.0, amount, "应取最早一笔订单的实付金额")
}
