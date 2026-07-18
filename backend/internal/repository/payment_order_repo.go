// payment_order_repo.go 支付订单只读仓储
// 实现 service.PaymentOrderReader 接口，供套餐升级差价计算反查旧订阅实付金额。
// 仅暴露读取能力，写操作仍由 payment service 直接经 entClient 完成。

package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
)

// paymentOrderReaderRepository 支付订单只读仓储实现
type paymentOrderReaderRepository struct {
	client *dbent.Client
}

// NewPaymentOrderReader 创建支付订单只读仓储，实现 service.PaymentOrderReader。
func NewPaymentOrderReader(client *dbent.Client) service.PaymentOrderReader {
	return &paymentOrderReaderRepository{client: client}
}

// GetPaidAmountByBundleSub 取该 bundle 订阅对应已完成购买订单的"实付金额"（取最早一笔）。
// 实付金额 = 订单 Amount − ProrateCredit：
//   - bundle 订单：Amount = 套餐总价，ProrateCredit = 0 → 实付 = 总价（用户全款购买）。
//   - bundle_upgrade 订单：Amount = 目标套餐总价，ProrateCredit = 旧套餐剩余价值抵扣
//     → 实付 = 总价 − 抵扣 = 升级差价（用户本次实际掏的钱）。
// credit 折算必须基于"实付金额"而非套餐总价，否则会出现"花差价升级、按总价抵扣"的套利。
// 二次升级（pro→enterprise）时，pro 订阅的购买订单是 bundle_upgrade 类型，credit 基于该订单反查
// （见 docs/BUNDLE_UPGRADE_DESIGN.md §6.3）。
// 找不到订单（兑换/赠送来源）返回 0,nil，不视作错误。
//
// ⚠️ 依赖历史数据迁移（bundle_upgrade_amount_to_total.sql）：迁移前老 bundle_upgrade 订单的
// Amount 仍是差价，本公式 (Amount − ProrateCredit) 会算错。部署顺序必须先跑迁移再上此代码。
func (r *paymentOrderReaderRepository) GetPaidAmountByBundleSub(ctx context.Context, bundleSubID int64) (float64, error) {
	client := clientFromContext(ctx, r.client)
	o, err := client.PaymentOrder.Query().
		Where(
			paymentorder.BundleSubscriptionIDEQ(bundleSubID),
			paymentorder.OrderTypeIn(payment.OrderTypeBundle, payment.OrderTypeBundleUpgrade),
			paymentorder.StatusEQ(payment.OrderStatusCompleted),
		).
		Order(dbent.Asc(paymentorder.FieldCreatedAt)).
		First(ctx)
	if dbent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, translatePersistenceError(err, nil, nil)
	}
	return o.Amount - o.ProrateCredit, nil
}
