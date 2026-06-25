// bundle_plan_repo.go 套餐计划数据访问实现
// 基于 Ent ORM 实现 BundlePlanRepository 接口，
// 提供套餐计划的 CRUD 操作，包括渠道组额度的级联创建和删除。

package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/bundleplan"
	"github.com/Wei-Shaw/sub2api/ent/bundleplangroupquota"
	groupent "github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// bundlePlanRepository 套餐计划仓库实现
type bundlePlanRepository struct {
	client *dbent.Client
}

// NewBundlePlanRepository 创建套餐计划仓库
func NewBundlePlanRepository(client *dbent.Client) service.BundlePlanRepository {
	return &bundlePlanRepository{client: client}
}

// Create 创建套餐计划，同时批量创建关联的渠道组额度
func (r *bundlePlanRepository) Create(ctx context.Context, plan *service.BundlePlan) error {
	if plan == nil {
		return service.ErrBundlePlanNotFound
	}

	client := clientFromContext(ctx, r.client)

	created, err := client.BundlePlan.Create().
		SetName(plan.Name).
		SetDescription(plan.Description).
		SetTier(plan.Tier).
		SetPrice(plan.Price).
		SetOriginalPrice(plan.OriginalPrice).
		SetCurrency(plan.Currency).
		SetValidityDays(plan.ValidityDays).
		SetConcurrencyLimit(plan.ConcurrencyLimit).
		SetRpmLimit(plan.RPMLimit).
		SetFeatures(plan.Features).
		SetForSale(plan.ForSale).
		SetSortOrder(plan.SortOrder).
		SetStatus(plan.Status).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, nil)
	}

	// Batch create group quotas.
	if len(plan.GroupQuotas) > 0 {
		builders := make([]*dbent.BundlePlanGroupQuotaCreate, 0, len(plan.GroupQuotas))
		for i := range plan.GroupQuotas {
			gq := plan.GroupQuotas[i]
			b := client.BundlePlanGroupQuota.Create().
				SetPlanID(created.ID).
				SetGroupID(gq.GroupID).
				SetQuotaScope(gq.QuotaScope).
				SetModelPattern(gq.ModelPattern).
				SetDailyLimitUsd(gq.DailyLimitUSD).
				SetWeeklyLimitUsd(gq.WeeklyLimitUSD).
				SetMonthlyLimitUsd(gq.MonthlyLimitUSD)
			builders = append(builders, b)
		}
		createdQuotas, err := client.BundlePlanGroupQuota.CreateBulk(builders...).Save(ctx)
		if err != nil {
			return translatePersistenceError(err, nil, nil)
		}
		for i, cq := range createdQuotas {
			plan.GroupQuotas[i].ID = cq.ID
			plan.GroupQuotas[i].PlanID = cq.PlanID
		}
	}

	plan.ID = created.ID
	plan.CreatedAt = created.CreatedAt
	plan.UpdatedAt = created.UpdatedAt
	return nil
}

// Update 更新套餐计划，采用"删除旧额度 → 重建新额度"策略
func (r *bundlePlanRepository) Update(ctx context.Context, plan *service.BundlePlan) error {
	if plan == nil || plan.ID == 0 {
		return service.ErrBundlePlanNotFound
	}

	client := clientFromContext(ctx, r.client)

	_, err := client.BundlePlan.UpdateOneID(plan.ID).
		SetName(plan.Name).
		SetDescription(plan.Description).
		SetTier(plan.Tier).
		SetPrice(plan.Price).
		SetOriginalPrice(plan.OriginalPrice).
		SetCurrency(plan.Currency).
		SetValidityDays(plan.ValidityDays).
		SetConcurrencyLimit(plan.ConcurrencyLimit).
		SetRpmLimit(plan.RPMLimit).
		SetFeatures(plan.Features).
		SetForSale(plan.ForSale).
		SetSortOrder(plan.SortOrder).
		SetStatus(plan.Status).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
	}

	// Delete old group quotas and recreate.
	_, err = client.BundlePlanGroupQuota.Delete().
		Where(bundleplangroupquota.PlanIDEQ(plan.ID)).
		Exec(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, nil)
	}

	if len(plan.GroupQuotas) > 0 {
		builders := make([]*dbent.BundlePlanGroupQuotaCreate, 0, len(plan.GroupQuotas))
		for i := range plan.GroupQuotas {
			gq := plan.GroupQuotas[i]
			b := client.BundlePlanGroupQuota.Create().
				SetPlanID(plan.ID).
				SetGroupID(gq.GroupID).
				SetQuotaScope(gq.QuotaScope).
				SetModelPattern(gq.ModelPattern).
				SetDailyLimitUsd(gq.DailyLimitUSD).
				SetWeeklyLimitUsd(gq.WeeklyLimitUSD).
				SetMonthlyLimitUsd(gq.MonthlyLimitUSD)
			builders = append(builders, b)
		}
		createdQuotas, err := client.BundlePlanGroupQuota.CreateBulk(builders...).Save(ctx)
		if err != nil {
			return translatePersistenceError(err, nil, nil)
		}
		plan.GroupQuotas = make([]service.BundlePlanGroupQuota, len(createdQuotas))
		for i, cq := range createdQuotas {
			plan.GroupQuotas[i] = bundlePlanGroupQuotaToService(cq)
		}
		enrichGroupQuotas(ctx, client, plan.GroupQuotas)
	} else {
		plan.GroupQuotas = nil
	}

	return nil
}

// GetByID 按ID获取套餐计划，同时加载关联的渠道组额度
func (r *bundlePlanRepository) GetByID(ctx context.Context, id int64) (*service.BundlePlan, error) {
	client := clientFromContext(ctx, r.client)

	planEntity, err := client.BundlePlan.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
	}

	quotas, err := client.BundlePlanGroupQuota.Query().
		Where(bundleplangroupquota.PlanIDEQ(id)).
		All(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, nil, nil)
	}

	result := bundlePlanToService(planEntity)
	result.GroupQuotas = make([]service.BundlePlanGroupQuota, len(quotas))
	for i, q := range quotas {
		result.GroupQuotas[i] = bundlePlanGroupQuotaToService(q)
	}
	enrichGroupQuotas(ctx, client, result.GroupQuotas)
	return result, nil
}

// List 分页查询套餐计划，支持按层级和状态过滤
func (r *bundlePlanRepository) List(ctx context.Context, params pagination.PaginationParams, tier, status string) ([]service.BundlePlan, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)

	var preds []predicate.BundlePlan
	if tier != "" {
		preds = append(preds, bundleplan.TierEQ(tier))
	}
	if status != "" {
		preds = append(preds, bundleplan.StatusEQ(status))
	}

	q := client.BundlePlan.Query()
	if len(preds) > 0 {
		q = q.Where(preds...)
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	plans, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(bundleplan.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Fetch group quotas for each plan.
	results := make([]service.BundlePlan, len(plans))
	for i, p := range plans {
		results[i] = *bundlePlanToService(p)
		quotas, err := client.BundlePlanGroupQuota.Query().
			Where(bundleplangroupquota.PlanIDEQ(p.ID)).
			All(ctx)
		if err != nil {
			return nil, nil, err
		}
		results[i].GroupQuotas = make([]service.BundlePlanGroupQuota, len(quotas))
		for j, q := range quotas {
			results[i].GroupQuotas[j] = bundlePlanGroupQuotaToService(q)
		}
		enrichGroupQuotas(ctx, client, results[i].GroupQuotas)
	}

	return results, paginationResultFromTotal(int64(total), params), nil
}

// ListForSale 获取所有在售且启用的套餐计划，按排序字段升序排列
func (r *bundlePlanRepository) ListForSale(ctx context.Context) ([]service.BundlePlan, error) {
	client := clientFromContext(ctx, r.client)

	plans, err := client.BundlePlan.Query().
		Where(
			bundleplan.StatusEQ("active"),
			bundleplan.ForSaleEQ(true),
		).
		Order(dbent.Asc(bundleplan.FieldSortOrder)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]service.BundlePlan, len(plans))
	for i, p := range plans {
		results[i] = *bundlePlanToService(p)
		quotas, err := client.BundlePlanGroupQuota.Query().
			Where(bundleplangroupquota.PlanIDEQ(p.ID)).
			All(ctx)
		if err != nil {
			return nil, err
		}
		results[i].GroupQuotas = make([]service.BundlePlanGroupQuota, len(quotas))
		for j, q := range quotas {
			results[i].GroupQuotas[j] = bundlePlanGroupQuotaToService(q)
		}
		enrichGroupQuotas(ctx, client, results[i].GroupQuotas)
	}
	return results, nil
}

// Delete 删除套餐计划，先删除关联的渠道组额度再删除计划
func (r *bundlePlanRepository) Delete(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)

	// Delete group quotas first.
	_, err := client.BundlePlanGroupQuota.Delete().
		Where(bundleplangroupquota.PlanIDEQ(id)).
		Exec(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, nil)
	}

	err = client.BundlePlan.DeleteOneID(id).Exec(ctx)
	return translatePersistenceError(err, service.ErrBundlePlanNotFound, nil)
}

// bundlePlanToService 将 Ent 实体转换为服务层模型
// bundlePlanToService converts an Ent BundlePlan entity to a service-layer model.
func bundlePlanToService(src *dbent.BundlePlan) *service.BundlePlan {
	if src == nil {
		return nil
	}
	return &service.BundlePlan{
		ID:               src.ID,
		Name:             src.Name,
		Description:      src.Description,
		Tier:             src.Tier,
		Price:            src.Price,
		OriginalPrice:    src.OriginalPrice,
		Currency:         src.Currency,
		ValidityDays:     src.ValidityDays,
		ConcurrencyLimit: src.ConcurrencyLimit,
		RPMLimit:         src.RpmLimit,
		Features:         src.Features,
		ForSale:          src.ForSale,
		SortOrder:        src.SortOrder,
		Status:           src.Status,
		CreatedAt:        src.CreatedAt,
		UpdatedAt:        src.UpdatedAt,
	}
}

// bundlePlanGroupQuotaToService 将 Ent 额度实体转换为服务层模型
// bundlePlanGroupQuotaToService converts an Ent BundlePlanGroupQuota entity to a service-layer model.
func bundlePlanGroupQuotaToService(src *dbent.BundlePlanGroupQuota) service.BundlePlanGroupQuota {
	return service.BundlePlanGroupQuota{
		ID:              src.ID,
		PlanID:          src.PlanID,
		GroupID:         src.GroupID,
		QuotaScope:      src.QuotaScope,
		ModelPattern:    src.ModelPattern,
		DailyLimitUSD:   src.DailyLimitUsd,
		WeeklyLimitUSD:  src.WeeklyLimitUsd,
		MonthlyLimitUSD: src.MonthlyLimitUsd,
	}
}

// enrichGroupQuotas 批量填充 GroupQuotas 的 GroupName / GroupPlatform 字段。
// 通过一次 IN 查询获取所有相关 group，避免 N+1 问题。
func enrichGroupQuotas(ctx context.Context, client *dbent.Client, quotas []service.BundlePlanGroupQuota) {
	if len(quotas) == 0 {
		return
	}

	// 收集所有不重复的 group_id
	seen := make(map[int64]struct{}, len(quotas))
	ids := make([]int64, 0, len(quotas))
	for _, q := range quotas {
		if _, ok := seen[q.GroupID]; !ok {
			seen[q.GroupID] = struct{}{}
			ids = append(ids, q.GroupID)
		}
	}

	groups, err := client.Group.Query().
		Where(groupent.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return // 查询失败时不阻塞，group_name 留空走前端 fallback
	}

	// 构建 id → group 映射
	groupMap := make(map[int64]*dbent.Group, len(groups))
	for _, g := range groups {
		groupMap[g.ID] = g
	}

	// 回填
	for i := range quotas {
		if g, ok := groupMap[quotas[i].GroupID]; ok {
			quotas[i].GroupName = g.Name
			quotas[i].GroupPlatform = g.Platform
		}
	}
}
