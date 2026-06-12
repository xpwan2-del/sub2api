// bundle_subscription_repo.go 套餐订阅数据访问实现
// 基于 Ent ORM 实现 BundleSubscriptionRepository 接口，
// 提供订阅的创建、按用户查询活跃订阅、含用量详情查询等操作。

package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/bundleplangroupquota"
	"github.com/Wei-Shaw/sub2api/ent/bundlesubscription"
	"github.com/Wei-Shaw/sub2api/ent/bundlesubscriptionusage"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// bundleSubscriptionRepository 套餐订阅仓库实现
type bundleSubscriptionRepository struct {
	client *dbent.Client
}

// NewBundleSubscriptionRepository 创建套餐订阅仓库
func NewBundleSubscriptionRepository(client *dbent.Client) service.BundleSubscriptionRepository {
	return &bundleSubscriptionRepository{client: client}
}

// Create 创建套餐订阅记录
func (r *bundleSubscriptionRepository) Create(ctx context.Context, sub *service.BundleSubscription) error {
	if sub == nil {
		return service.ErrBundleNotFound
	}

	client := clientFromContext(ctx, r.client)

	created, err := client.BundleSubscription.Create().
		SetUserID(sub.UserID).
		SetPlanID(sub.PlanID).
		SetStatus(sub.Status).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetConcurrencyLimit(sub.ConcurrencyLimit).
		SetRpmLimit(sub.RPMLimit).
		SetSource(sub.Source).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrBundleConflict)
	}

	sub.ID = created.ID
	sub.CreatedAt = created.CreatedAt
	sub.UpdatedAt = created.UpdatedAt
	return nil
}

// GetByID 按ID获取套餐订阅
func (r *bundleSubscriptionRepository) GetByID(ctx context.Context, id int64) (*service.BundleSubscription, error) {
	client := clientFromContext(ctx, r.client)

	m, err := client.BundleSubscription.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundleNotFound, nil)
	}
	return bundleSubscriptionToService(m), nil
}

// GetActiveByUserID 获取用户的所有活跃（active 且未过期）套餐订阅
func (r *bundleSubscriptionRepository) GetActiveByUserID(ctx context.Context, userID int64) ([]service.BundleSubscription, error) {
	client := clientFromContext(ctx, r.client)

	subs, err := client.BundleSubscription.Query().
		Where(
			bundlesubscription.UserIDEQ(userID),
			bundlesubscription.StatusEQ("active"),
			bundlesubscription.ExpiresAtGT(time.Now()),
		).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, nil, nil)
	}

	results := make([]service.BundleSubscription, len(subs))
	for i, s := range subs {
		result := bundleSubscriptionToService(s)
		// Load associated plan with group quotas for each subscription
		enrichSubscriptionPlan(ctx, client, result)
		results[i] = *result
	}
	return results, nil
}

// GetByIDWithUsages 按ID获取套餐订阅，同时加载关联的用量记录
func (r *bundleSubscriptionRepository) GetByIDWithUsages(ctx context.Context, id int64) (*service.BundleSubscription, error) {
	client := clientFromContext(ctx, r.client)

	m, err := client.BundleSubscription.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundleNotFound, nil)
	}

	usages, err := client.BundleSubscriptionUsage.Query().
		Where(bundlesubscriptionusage.BundleSubscriptionIDEQ(id)).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, nil, nil)
	}

	result := bundleSubscriptionToService(m)
	result.Usages = make([]service.BundleSubscriptionUsage, len(usages))
	for i, u := range usages {
		result.Usages[i] = bundleSubscriptionUsageToService(u)
	}
	return result, nil
}

// List 分页查询套餐订阅，支持按用户ID和状态过滤
func (r *bundleSubscriptionRepository) List(ctx context.Context, params pagination.PaginationParams, userID *int64, status string) ([]service.BundleSubscription, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)

	var preds []predicate.BundleSubscription
	if userID != nil {
		preds = append(preds, bundlesubscription.UserIDEQ(*userID))
	}
	if status != "" {
		preds = append(preds, bundlesubscription.StatusEQ(status))
	}

	q := client.BundleSubscription.Query()
	if len(preds) > 0 {
		q = q.Where(preds...)
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	subs, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(bundlesubscription.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	results := make([]service.BundleSubscription, len(subs))
	for i, s := range subs {
		results[i] = *bundleSubscriptionToService(s)
	}

	return results, paginationResultFromTotal(int64(total), params), nil
}

// UpdateStatus 更新订阅状态
func (r *bundleSubscriptionRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscription.UpdateOneID(id).
		SetStatus(status).
		Save(ctx)
	return translatePersistenceError(err, service.ErrBundleNotFound, nil)
}

// UpdateExpiry 更新订阅到期时间
func (r *bundleSubscriptionRepository) UpdateExpiry(ctx context.Context, id int64, expiresAt time.Time) error {
	client := clientFromContext(ctx, r.client)

	_, err := client.BundleSubscription.UpdateOneID(id).
		SetExpiresAt(expiresAt).
		Save(ctx)
	return translatePersistenceError(err, service.ErrBundleNotFound, nil)
}

// bundleSubscriptionToService 将 Ent 订阅实体转换为服务层模型
// bundleSubscriptionToService converts an Ent BundleSubscription entity to a service-layer model.
func bundleSubscriptionToService(src *dbent.BundleSubscription) *service.BundleSubscription {
	if src == nil {
		return nil
	}
	return &service.BundleSubscription{
		ID:               src.ID,
		UserID:           src.UserID,
		PlanID:           src.PlanID,
		Status:           src.Status,
		StartsAt:         src.StartsAt,
		ExpiresAt:        src.ExpiresAt,
		ConcurrencyLimit: src.ConcurrencyLimit,
		RPMLimit:         src.RpmLimit,
		Source:           src.Source,
		CreatedAt:        src.CreatedAt,
		UpdatedAt:        src.UpdatedAt,
		DeletedAt:        src.DeletedAt,
	}
}

// bundleSubscriptionUsageToService 将 Ent 用量实体转换为服务层模型
// bundleSubscriptionUsageToService converts an Ent BundleSubscriptionUsage entity to a service-layer model.
func bundleSubscriptionUsageToService(src *dbent.BundleSubscriptionUsage) service.BundleSubscriptionUsage {
	return service.BundleSubscriptionUsage{
		ID:                   src.ID,
		BundleSubscriptionID: src.BundleSubscriptionID,
		GroupID:              src.GroupID,
		ModelPattern:         src.ModelPattern,
		DailyUsageUSD:        src.DailyUsageUsd,
		DailyWindowStart:     src.DailyWindowStart,
		WeeklyUsageUSD:       src.WeeklyUsageUsd,
		WeeklyWindowStart:    src.WeeklyWindowStart,
		MonthlyUsageUSD:      src.MonthlyUsageUsd,
		MonthlyWindowStart:   src.MonthlyWindowStart,
	}
}

// enrichSubscriptionPlan 加载订阅关联的套餐计划及其渠道组额度
// enrichSubscriptionPlan loads the associated plan with group quotas for a subscription.
func enrichSubscriptionPlan(ctx context.Context, client *dbent.Client, sub *service.BundleSubscription) {
	planEntity, err := client.BundlePlan.Get(ctx, sub.PlanID)
	if err != nil {
		return // Plan not found or deleted; leave Plan nil
	}

	plan := bundlePlanToService(planEntity)

	quotas, err := client.BundlePlanGroupQuota.Query().
		Where(bundleplangroupquota.PlanIDEQ(sub.PlanID)).
		All(ctx)
	if err != nil {
		return
	}

	plan.GroupQuotas = make([]service.BundlePlanGroupQuota, len(quotas))
	for i, q := range quotas {
		plan.GroupQuotas[i] = bundlePlanGroupQuotaToService(q)
	}
	enrichGroupQuotas(ctx, client, plan.GroupQuotas)

	sub.Plan = plan
}
