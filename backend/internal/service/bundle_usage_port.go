// bundle_usage_port.go 套餐用量数据访问接口
// 定义 BundleUsageRepository 接口，解耦服务层与具体数据访问实现。

package service

import (
	"context"
	"time"
)

// BundleUsageRepository 套餐用量数据访问接口，提供用量累加、查询和时间窗口重置操作
// BundleUsageRepository defines the data-access interface for bundle subscription usage tracking.
type BundleUsageRepository interface {
	GetBySubscriptionAndGroup(ctx context.Context, subscriptionID, groupID int64, modelPattern string) (*BundleSubscriptionUsage, error)
	Create(ctx context.Context, usage *BundleSubscriptionUsage) error
	// IncrementUsage 原子累加 costUSD/imageCount/videoCount 到日/周/月三个滚动窗口。
	// 窗口是否过期由 repo 在事务内基于 DB 真实 window_start 判断（FOR UPDATE 锁行），
	// 过期则置为本次值并推进 window_start，否则在原值上累加。now 为本次累加基准时刻。
	// 设计要点：判断与写入必须同一原子操作，否则 service 层 read-modify-write 会在
	// 窗口边界并发下互相 Set 覆盖、丢失计费（历史 bug H1）。
	IncrementUsage(ctx context.Context, id int64, costUSD float64, imageCount, videoCount int, now time.Time) error
	ResetDailyWindow(ctx context.Context, id int64, newWindowStart time.Time) error
	ResetWeeklyWindow(ctx context.Context, id int64, newWindowStart time.Time) error
	ResetMonthlyWindow(ctx context.Context, id int64, newWindowStart time.Time) error
	ListBySubscription(ctx context.Context, subscriptionID int64) ([]BundleSubscriptionUsage, error)
	BatchUpdateExpiredStatus(ctx context.Context) (int64, error)
}
