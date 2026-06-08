package service

import (
	"context"
	"time"
)

// BundleUsageRepository defines the data-access interface for bundle subscription usage tracking.
type BundleUsageRepository interface {
	GetBySubscriptionAndGroup(ctx context.Context, subscriptionID, groupID int64, modelPattern string) (*BundleSubscriptionUsage, error)
	Create(ctx context.Context, usage *BundleSubscriptionUsage) error
	IncrementUsage(ctx context.Context, id int64, costUSD float64) error
	ResetDailyWindow(ctx context.Context, id int64, newWindowStart time.Time) error
	ResetWeeklyWindow(ctx context.Context, id int64, newWindowStart time.Time) error
	ResetMonthlyWindow(ctx context.Context, id int64, newWindowStart time.Time) error
	ListBySubscription(ctx context.Context, subscriptionID int64) ([]BundleSubscriptionUsage, error)
	BatchUpdateExpiredStatus(ctx context.Context) (int64, error)
}
