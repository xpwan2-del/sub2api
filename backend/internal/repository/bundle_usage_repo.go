package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/bundlesubscription"
	"github.com/Wei-Shaw/sub2api/ent/bundlesubscriptionusage"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type bundleUsageRepository struct {
	client *dbent.Client
}

func NewBundleUsageRepository(client *dbent.Client) service.BundleUsageRepository {
	return &bundleUsageRepository{client: client}
}

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

func (r *bundleUsageRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscriptionUsage.UpdateOneID(id).
		AddDailyUsageUsd(costUSD).
		AddWeeklyUsageUsd(costUSD).
		AddMonthlyUsageUsd(costUSD).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}

func (r *bundleUsageRepository) ResetDailyWindow(ctx context.Context, id int64, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscriptionUsage.UpdateOneID(id).
		SetDailyUsageUsd(0).
		SetDailyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}

func (r *bundleUsageRepository) ResetWeeklyWindow(ctx context.Context, id int64, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscriptionUsage.UpdateOneID(id).
		SetWeeklyUsageUsd(0).
		SetWeeklyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}

func (r *bundleUsageRepository) ResetMonthlyWindow(ctx context.Context, id int64, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscriptionUsage.UpdateOneID(id).
		SetMonthlyUsageUsd(0).
		SetMonthlyWindowStart(newWindowStart).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}

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
