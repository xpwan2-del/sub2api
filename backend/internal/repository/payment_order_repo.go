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

// GetFaceValueByBundleSub 取该 bundle 订阅对应已完成购买订单的【套餐标价】（订单 Amount，取最早一笔）。
// 套餐标价 = 订单 Amount：
//   - bundle 订单：Amount = 套餐总价（用户全款购买）。
//   - bundle_upgrade 订单：Amount = 目标套餐总价（已回归主干语义 orderAmount=Amount+ProrateCredit）。
//
// credit 折算基于【套餐标价】而非"实付差额(Amount-ProrateCredit)"：升级得到的套餐持有完整使用权，
// 剩余价值按标价线性折算才公允；若按当初升级的实付差额折算，连续升级(pro→ent→ultimate)时 credit 会被
// 逐级压缩、用户被反复折价。建模验证：在"线性时间折算+升级重置有效期+只能升更贵套餐"约束下，标价基准
// 不会套利（用户累计实付 ≥ 最终套餐标价）——防套利由退款环节 refundUpgradeToBalance 退实付差额保证。
// 二次升级（pro→enterprise）时，pro 订阅的购买订单是 bundle_upgrade 类型，credit 基于该订单反查。
// 找不到订单（兑换/赠送来源）返回 0,nil，不视作错误。
//
// ⚠️ 依赖历史数据迁移（174_bundle_upgrade_amount_to_total.sql）：迁移前老 bundle_upgrade 订单的
// Amount 仍是差价，本方法返回的标价会偏低。部署顺序必须先跑迁移再上此代码。
func (r *paymentOrderReaderRepository) GetFaceValueByBundleSub(ctx context.Context, bundleSubID int64) (float64, error) {
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
	return o.Amount, nil
}
