// bundle_concurrency_cache.go 套餐并发数缓存接口
// 用于 bundle subscription 级别的并发请求数限制。

package service

import "context"

// BundleConcurrencyCache 套餐并发数缓存接口。
// 使用 Redis INCR/DECR 跟踪每个套餐订阅当前正在处理中的请求数。
// key 形如 concurrency:bundle:{bundleSubID}，TTL 防止客户端断连导致的泄漏。
type BundleConcurrencyCache interface {
	// Increment 递增并发计数并返回当前值。
	// 用于请求开始时检查并发数是否超限。
	Increment(ctx context.Context, bundleSubID int64) (count int64, err error)

	// Decrement 递减并发计数并返回当前值。
	// 用于请求完成时释放并发槽位。调用方应在 defer 中调用以确保释放。
	Decrement(ctx context.Context, bundleSubID int64) (count int64, err error)

	// Get 获取当前并发计数（只读，不递增）。
	Get(ctx context.Context, bundleSubID int64) (count int64, err error)
}
