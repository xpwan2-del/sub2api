// bundle_rpm_cache.go 套餐 RPM 计数器缓存接口
// 用于 bundle subscription 级别的每分钟请求数限制（跨 Group 聚合）。

package service

import "context"

// BundleRPMCache 套餐 RPM 计数器缓存接口。
// 按套餐订阅维度聚合，杜绝同一用户同时使用多个 API Key 绕过套餐 RPM 限制的路径。
// key 形如 rpm:bundle:{bundleSubID}:{minute}。
type BundleRPMCache interface {
	// IncrementBundleRPM 原子递增套餐订阅的分钟计数并返回最新值。
	IncrementBundleRPM(ctx context.Context, bundleSubID int64) (count int, err error)

	// GetBundleRPM 获取当前分钟已用 RPM（只读，不递增）。
	GetBundleRPM(ctx context.Context, bundleSubID int64) (count int, err error)
}
