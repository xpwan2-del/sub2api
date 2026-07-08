//go:build unit

// payment_bundle_dedup_test.go 套餐订单严格防重测试（中危3）
// 守护：同一用户存在未完成 bundle 订单（PENDING/PAID/RECHARGING）时拒绝新建，
// 杜绝双击/并发下单导致两个订单都支付后第二个激活失败、资金滞留平台。
package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

// newBundleDedupClient 为 bundle 防重测试构建隔离的 sqlite 内存 client。
// cache=shared 下同名共享内存 DB，必须按测试名唯一化以避免子测试数据污染。
func newBundleDedupClient(t *testing.T) *dbent.Client {
	t.Helper()
	name := "bdedup_" + sanitizeSQLiteName(t.Name())
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func sanitizeSQLiteName(s string) string {
	return strings.NewReplacer("/", "_", " ", "_", "-", "_").Replace(s)
}

// createDedupUser 建一个最小可用用户，返回其 ID（PaymentOrder.user_id 外键依赖）。
func createDedupUser(t *testing.T, client *dbent.Client, email string) int64 {
	t.Helper()
	u, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("hash").
		SetUsername(email).
		Save(context.Background())
	require.NoError(t, err)
	return u.ID
}

// createDedupOrder 建一条指定状态/类型的订单（status 用 PaymentOrder 大写常量）。
func createDedupOrder(t *testing.T, client *dbent.Client, userID int64, status, orderType, email string) {
	t.Helper()
	_, err := client.PaymentOrder.Create().
		SetUserID(userID).
		SetUserEmail(email).
		SetUserName(email).
		SetAmount(10).
		SetPayAmount(10).
		SetFeeRate(0).
		SetRechargeCode("RC-" + status + "-" + orderType).
		SetOutTradeNo("sub2_dedup_" + status + "_" + orderType).
		SetPaymentType("balance").
		SetPaymentTradeNo("").
		SetOrderType(orderType).
		SetStatus(status).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(context.Background())
	require.NoError(t, err)
}

// TestEnsureNoDuplicateBundleOrder 守护中危3：bundle 订单严格防重。
func TestEnsureNoDuplicateBundleOrder(t *testing.T) {
	ctx := context.Background()

	t.Run("no_order_allows", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "none@example.com")
		svc := &PaymentService{entClient: client}
		require.NoError(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	t.Run("pending_bundle_blocks", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "pending@example.com")
		createDedupOrder(t, client, uid, OrderStatusPending, payment.OrderTypeBundle, "pending@example.com")
		svc := &PaymentService{entClient: client}
		require.Error(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	t.Run("paid_bundle_blocks", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "paid@example.com")
		createDedupOrder(t, client, uid, OrderStatusPaid, payment.OrderTypeBundle, "paid@example.com")
		svc := &PaymentService{entClient: client}
		require.Error(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	t.Run("recharging_bundle_blocks", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "recharging@example.com")
		createDedupOrder(t, client, uid, OrderStatusRecharging, payment.OrderTypeBundle, "recharging@example.com")
		svc := &PaymentService{entClient: client}
		require.Error(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	t.Run("completed_bundle_allows", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "done@example.com")
		createDedupOrder(t, client, uid, OrderStatusCompleted, payment.OrderTypeBundle, "done@example.com")
		svc := &PaymentService{entClient: client}
		require.NoError(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	t.Run("failed_bundle_allows", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "failed@example.com")
		createDedupOrder(t, client, uid, OrderStatusFailed, payment.OrderTypeBundle, "failed@example.com")
		svc := &PaymentService{entClient: client}
		require.NoError(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	t.Run("cancelled_bundle_allows", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "cancelled@example.com")
		createDedupOrder(t, client, uid, OrderStatusCancelled, payment.OrderTypeBundle, "cancelled@example.com")
		svc := &PaymentService{entClient: client}
		require.NoError(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	t.Run("non_bundle_pending_order_allows", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "balance@example.com")
		// 余额充值订单（非 bundle）即使 PENDING 也不阻塞 bundle 防重
		createDedupOrder(t, client, uid, OrderStatusPending, payment.OrderTypeBalance, "balance@example.com")
		svc := &PaymentService{entClient: client}
		require.NoError(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	// --- bundle_upgrade 防重覆盖（Task 6 硬要求） ---
	// 守护：同一用户存在未完成 bundle_upgrade 订单时拒绝新建，杜绝两个升级支付回调并发
	// 履约产生两个 active 新订阅（UpgradeBundle 的 status 校验在真实并发下是 TOCTOU，
	// 订单防重是 UpgradeBundle 并发防护的上游主防线）。

	t.Run("pending_bundle_upgrade_blocks", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "upg-pending@example.com")
		createDedupOrder(t, client, uid, OrderStatusPending, payment.OrderTypeBundleUpgrade, "upg-pending@example.com")
		svc := &PaymentService{entClient: client}
		require.Error(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	t.Run("paid_bundle_upgrade_blocks", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "upg-paid@example.com")
		createDedupOrder(t, client, uid, OrderStatusPaid, payment.OrderTypeBundleUpgrade, "upg-paid@example.com")
		svc := &PaymentService{entClient: client}
		require.Error(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	t.Run("recharging_bundle_upgrade_blocks", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "upg-recharging@example.com")
		createDedupOrder(t, client, uid, OrderStatusRecharging, payment.OrderTypeBundleUpgrade, "upg-recharging@example.com")
		svc := &PaymentService{entClient: client}
		require.Error(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	t.Run("completed_bundle_upgrade_allows", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "upg-done@example.com")
		createDedupOrder(t, client, uid, OrderStatusCompleted, payment.OrderTypeBundleUpgrade, "upg-done@example.com")
		svc := &PaymentService{entClient: client}
		require.NoError(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	t.Run("cancelled_bundle_upgrade_allows", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "upg-cancelled@example.com")
		createDedupOrder(t, client, uid, OrderStatusCancelled, payment.OrderTypeBundleUpgrade, "upg-cancelled@example.com")
		svc := &PaymentService{entClient: client}
		require.NoError(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})

	// 跨类型互斥：未完成的 bundle_upgrade 订单应阻塞后续 bundle 新建（反之亦然，
	// pending_bundle_blocks 已覆盖 bundle→bundle_upgrade 方向，因查询同时匹配两种类型）。
	t.Run("bundle_upgrade_blocks_cross_type", func(t *testing.T) {
		client := newBundleDedupClient(t)
		uid := createDedupUser(t, client, "cross@example.com")
		// 既有未完成 bundle_upgrade 订单
		createDedupOrder(t, client, uid, OrderStatusPending, payment.OrderTypeBundleUpgrade, "cross@example.com")
		svc := &PaymentService{entClient: client}
		// 同一用户即便想下普通 bundle 订单也应被拦截（ensureNoDuplicateBundleOrder 不区分类型）
		require.Error(t, svc.ensureNoDuplicateBundleOrder(ctx, uid))
	})
}
