// bundle_usage_repo.go 套餐用量数据访问实现
// 基于 Ent ORM 实现 BundleUsageRepository 接口，
// 提供用量的创建、累加、查询和日/周/月窗口重置操作。

package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/bundlesubscription"
	"github.com/Wei-Shaw/sub2api/ent/bundlesubscriptionusage"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// bundleUsageRepository 套餐用量仓库实现
type bundleUsageRepository struct {
	client *dbent.Client
}

// NewBundleUsageRepository 创建套餐用量仓库
func NewBundleUsageRepository(client *dbent.Client) service.BundleUsageRepository {
	return &bundleUsageRepository{client: client}
}

// GetBySubscriptionAndGroup 按订阅ID和渠道组ID查询用量记录
func (r *bundleUsageRepository) GetBySubscriptionAndGroup(ctx context.Context, subscriptionID, groupID int64, modelPattern string) (*service.BundleSubscriptionUsage, error) {
	client := clientFromContext(ctx, r.client)

	m, err := client.BundleSubscriptionUsage.Query().
		Where(
			bundlesubscriptionusage.BundleSubscriptionIDEQ(subscriptionID),
			bundlesubscriptionusage.GroupIDEQ(groupID),
			bundlesubscriptionusage.ModelPatternEQ(modelPattern),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, nil, nil)
	}

	result := bundleSubscriptionUsageToService(m)
	return &result, nil
}

// Create 创建用量记录，初始化各时间窗口
func (r *bundleUsageRepository) Create(ctx context.Context, usage *service.BundleSubscriptionUsage) error {
	if usage == nil {
		return nil
	}

	client := clientFromContext(ctx, r.client)

	created, err := client.BundleSubscriptionUsage.Create().
		SetBundleSubscriptionID(usage.BundleSubscriptionID).
		SetGroupID(usage.GroupID).
		SetModelPattern(usage.ModelPattern).
		SetDailyUsageUsd(usage.DailyUsageUSD).
		SetDailyWindowStart(usage.DailyWindowStart).
		SetWeeklyUsageUsd(usage.WeeklyUsageUSD).
		SetWeeklyWindowStart(usage.WeeklyWindowStart).
		SetMonthlyUsageUsd(usage.MonthlyUsageUSD).
		SetMonthlyWindowStart(usage.MonthlyWindowStart).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, nil)
	}

	usage.ID = created.ID
	return nil
}

// IncrementUsage 累加日/周/月用量（原子操作）
func (r *bundleUsageRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscriptionUsage.UpdateOneID(id).
		AddDailyUsageUsd(costUSD).
		AddWeeklyUsageUsd(costUSD).
		AddMonthlyUsageUsd(costUSD).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}

// ResetDailyWindow 重置日窗口：清零日用量并更新窗口起点
func (r *bundleUsageRepository) ResetDailyWindow(ctx context.Context, id int64, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscriptionUsage.UpdateOneID(id).
		SetDailyUsageUsd(0).
		SetDailyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}

// ResetWeeklyWindow 重置周窗口：清零周用量并更新窗口起点
func (r *bundleUsageRepository) ResetWeeklyWindow(ctx context.Context, id int64, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscriptionUsage.UpdateOneID(id).
		SetWeeklyUsageUsd(0).
		SetWeeklyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}

// ResetMonthlyWindow 重置月窗口：清零月用量并更新窗口起点
func (r *bundleUsageRepository) ResetMonthlyWindow(ctx context.Context, id int64, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscriptionUsage.UpdateOneID(id).
		SetMonthlyUsageUsd(0).
		SetMonthlyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}

// ListBySubscription 查询订阅下的所有用量记录
func (r *bundleUsageRepository) ListBySubscription(ctx context.Context, subscriptionID int64) ([]service.BundleSubscriptionUsage, error) {
	client := clientFromContext(ctx, r.client)

	usages, err := client.BundleSubscriptionUsage.Query().
		Where(bundlesubscriptionusage.BundleSubscriptionIDEQ(subscriptionID)).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, nil, nil)
	}

	results := make([]service.BundleSubscriptionUsage, len(usages))
	for i, u := range usages {
		results[i] = bundleSubscriptionUsageToService(u)
	}
	return results, nil
}

// BatchUpdateExpiredStatus 批量将已过期但仍为 active 的订阅标记为 expired
func (r *bundleUsageRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	client := clientFromContext(ctx, r.client)

	// Find all bundle subscriptions that are active but expired.
	expiredIDs, err := client.BundleSubscription.Query().
		Where(
			bundlesubscription.StatusEQ("active"),
			bundlesubscription.ExpiresAtLTE(time.Now()),
		).
		IDs(ctx)
	if err != nil {
		return 0, err
	}

	if len(expiredIDs) == 0 {
		return 0, nil
	}

	n, err := client.BundleSubscription.Update().
		Where(bundlesubscription.IDIn(expiredIDs...)).
		SetStatus("expired").
		Save(ctx)
	if err != nil {
		return 0, err
	}

	return int64(n), nil
}
