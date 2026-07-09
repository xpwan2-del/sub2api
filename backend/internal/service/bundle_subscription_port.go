// bundle_subscription_port.go 套餐订阅数据访问接口
// 定义 BundleSubscriptionRepository 接口，解耦服务层与具体数据访问实现。

package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// BundleSubscriptionRepository 套餐订阅数据访问接口，提供订阅的创建、查询、状态更新和延期操作
// BundleSubscriptionRepository defines the data-access interface for bundle subscriptions.
type BundleSubscriptionRepository interface {
	Create(ctx context.Context, sub *BundleSubscription) error
	GetByID(ctx context.Context, id int64) (*BundleSubscription, error)
	GetActiveByUserID(ctx context.Context, userID int64) ([]BundleSubscription, error)
	GetByIDWithUsages(ctx context.Context, id int64) (*BundleSubscription, error)
	List(ctx context.Context, params pagination.PaginationParams, userID *int64, status string) ([]BundleSubscription, *pagination.PaginationResult, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateExpiry(ctx context.Context, id int64, expiresAt time.Time) error
	// ExtendExpiryByDays 原子增量延期：expires_at = expires_at + days 天。
	// 用于 ExtendBundle，避免基于事务外快照的覆盖写导致并发延期丢失（lost update）。
	ExtendExpiryByDays(ctx context.Context, id int64, days int) error
}
