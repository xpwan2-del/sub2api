// bundle_usage_port.go 套餐用量数据访问接口
// 定义 BundleUsageRepository 接口，解耦服务层与具体数据访问实现。

package service

import (
	"context"
	"time"
)

// WindowRoll 描述一次用量累加时各周期窗口是否需要滚动重置。
// 过期窗口（Daily/Weekly/Monthly=true）在累加前清零（USD + count）并把窗口起点
// 更新为对应的 NewXxxStart；未过期窗口直接在原值上累加（Add）。
type WindowRoll struct {
	Daily           bool
	Weekly          bool
	Monthly         bool
	NewDailyStart   time.Time
	NewWeeklyStart  time.Time
	NewMonthlyStart time.Time
}

// BundleUsageRepository 套餐用量数据访问接口，提供用量累加、查询和时间窗口重置操作
// BundleUsageRepository defines the data-access interface for bundle subscription usage tracking.
type BundleUsageRepository interface {
	GetBySubscriptionAndGroup(ctx context.Context, subscriptionID, groupID int64, modelPattern string) (*BundleSubscriptionUsage, error)
	Create(ctx context.Context, usage *BundleSubscriptionUsage) error
	IncrementUsage(ctx context.Context, id int64, costUSD float64, count int, roll WindowRoll) error
	ResetDailyWindow(ctx context.Context, id int64, newWindowStart time.Time) error
	ResetWeeklyWindow(ctx context.Context, id int64, newWindowStart time.Time) error
	ResetMonthlyWindow(ctx context.Context, id int64, newWindowStart time.Time) error
	ListBySubscription(ctx context.Context, subscriptionID int64) ([]BundleSubscriptionUsage, error)
	BatchUpdateExpiredStatus(ctx context.Context) (int64, error)
}
