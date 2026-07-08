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

// GetPaidAmountByBundleSub 取该 bundle 订阅对应已完成 bundle 购买订单的实付金额（取最早一笔）。
// 找不到订单（兑换/赠送来源）返回 0,nil，不视作错误。
func (r *paymentOrderReaderRepository) GetPaidAmountByBundleSub(ctx context.Context, bundleSubID int64) (float64, error) {
	client := clientFromContext(ctx, r.client)
	o, err := client.PaymentOrder.Query().
		Where(
			paymentorder.BundleSubscriptionIDEQ(bundleSubID),
			paymentorder.OrderTypeEQ(payment.OrderTypeBundle),
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
	return o.Amount, nil
}
