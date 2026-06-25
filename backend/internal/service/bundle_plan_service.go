// bundle_plan_service.go 套餐计划服务实现
// 提供套餐计划的 CRUD 业务逻辑，包括创建（含渠道组额度）、
// 更新（支持部分字段合并）、查询（分页+过滤）等操作。

package service

import (
	"encoding/json"
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// BundlePlanService 套餐计划服务，封装计划管理的业务逻辑
// BundlePlanService handles CRUD operations for bundle plans.
type BundlePlanService struct {
	planRepo BundlePlanRepository
	cache  BillingCache
}

// NewBundlePlanService 创建套餐计划服务实例
// NewBundlePlanService creates a new BundlePlanService.
func NewBundlePlanService(planRepo BundlePlanRepository, cache BillingCache) *BundlePlanService {
	return &BundlePlanService{planRepo: planRepo, cache: cache}
}

// CreatePlan 创建套餐计划，将请求 DTO 转换为领域模型后持久化
// CreatePlan creates a new bundle plan with its group quotas.
func (s *BundlePlanService) CreatePlan(ctx context.Context, req *CreateBundlePlanRequest) (*BundlePlan, error) {
	if req == nil {
		return nil, ErrBundlePlanNotFound
	}

	// Default for_sale to true when not explicitly provided in the request.
	// Go's bool zero-value is false, which would hide new plans from the storefront.
	forSale := true
	if req.ForSale != nil {
		forSale = *req.ForSale
	}

	plan := &BundlePlan{
		Name:             req.Name,
		Description:      req.Description,
		Tier:             req.Tier,
		Price:            req.Price,
		OriginalPrice:    req.OriginalPrice,
		Currency:         req.Currency,
		ValidityDays:     req.ValidityDays,
		ConcurrencyLimit: req.ConcurrencyLimit,
		RPMLimit:         req.RPMLimit,
		Features:         req.Features,
		ForSale:          forSale,
		SortOrder:        req.SortOrder,
		Status:           domain.StatusActive,
		GroupQuotas:      make([]BundlePlanGroupQuota, 0, len(req.GroupQuotas)),
	}

	for _, gq := range req.GroupQuotas {
		plan.GroupQuotas = append(plan.GroupQuotas, BundlePlanGroupQuota{
			GroupID:         gq.GroupID,
			QuotaScope:      gq.QuotaScope,
			ModelPattern:    gq.ModelPattern,
			DailyLimitUSD:   gq.DailyLimitUSD,
			WeeklyLimitUSD:  gq.WeeklyLimitUSD,
			MonthlyLimitUSD: gq.MonthlyLimitUSD,
		})
	}

	if err := s.planRepo.Create(ctx, plan); err != nil {
		return nil, fmt.Errorf("create bundle plan: %w", err)
	}
	// Invalidate plans cache after creation.
	if s.cache != nil {
		_ = s.cache.InvalidateBundlePlansForSaleCache(ctx)
	}

	return plan, nil
}

// UpdatePlan 更新套餐计划，仅合并请求中非 nil 的字段；如果包含 group_quotas 则整体替换
// UpdatePlan updates an existing bundle plan by merging non-nil fields from the request.
func (s *BundlePlanService) UpdatePlan(ctx context.Context, planID int64, req *UpdateBundlePlanRequest) (*BundlePlan, error) {
	if req == nil {
		return nil, ErrBundlePlanNotFound
	}

	existing, err := s.planRepo.GetByID(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("load bundle plan: %w", err)
	}

	// Merge non-nil scalar fields.
	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Tier != nil {
		existing.Tier = *req.Tier
	}
	if req.Price != nil {
		existing.Price = *req.Price
	}
	if req.OriginalPrice != nil {
		existing.OriginalPrice = *req.OriginalPrice
	}
	if req.Currency != nil {
		existing.Currency = *req.Currency
	}
	if req.ValidityDays != nil {
		existing.ValidityDays = *req.ValidityDays
	}
	if req.ConcurrencyLimit != nil {
		existing.ConcurrencyLimit = *req.ConcurrencyLimit
	}
	if req.RPMLimit != nil {
		existing.RPMLimit = *req.RPMLimit
	}
	if req.Features != nil {
		existing.Features = *req.Features
	}
	if req.ForSale != nil {
		existing.ForSale = *req.ForSale
	}
	if req.SortOrder != nil {
		existing.SortOrder = *req.SortOrder
	}
	if req.Status != nil {
		existing.Status = *req.Status
	}

	// Replace group quotas if provided.
	if req.GroupQuotas != nil {
		quotas := make([]BundlePlanGroupQuota, 0, len(*req.GroupQuotas))
		for _, gq := range *req.GroupQuotas {
			quotas = append(quotas, BundlePlanGroupQuota{
				PlanID:          planID,
				GroupID:         gq.GroupID,
				QuotaScope:      gq.QuotaScope,
				ModelPattern:    gq.ModelPattern,
				DailyLimitUSD:   gq.DailyLimitUSD,
				WeeklyLimitUSD:  gq.WeeklyLimitUSD,
				MonthlyLimitUSD: gq.MonthlyLimitUSD,
			})
		}
		existing.GroupQuotas = quotas
	}

	if err := s.planRepo.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("update bundle plan: %w", err)
	}
	// Invalidate plans cache after update.
	if s.cache != nil {
		_ = s.cache.InvalidateBundlePlansForSaleCache(ctx)
	}

	return existing, nil
}

// GetPlanDetail 按 ID 获取套餐计划详情
// GetPlanDetail returns a single bundle plan by ID.
func (s *BundlePlanService) GetPlanDetail(ctx context.Context, planID int64) (*BundlePlan, error) {
	plan, err := s.planRepo.GetByID(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("get bundle plan: %w", err)
	}
	return plan, nil
}

// ListPlans 分页查询套餐计划，支持按层级（tier）和状态（status）过滤
// ListPlans returns a paginated list of bundle plans with optional filters.
func (s *BundlePlanService) ListPlans(ctx context.Context, params pagination.PaginationParams, tier, status string) ([]BundlePlan, *pagination.PaginationResult, error) {
	plans, result, err := s.planRepo.List(ctx, params, tier, status)
	if err != nil {
		return nil, nil, fmt.Errorf("list bundle plans: %w", err)
	}
	return plans, result, nil
}

// ListForSale 获取所有在售且启用的套餐计划（供用户端浏览）
// ListForSale returns all plans that are currently for sale and active.
func (s *BundlePlanService) ListForSale(ctx context.Context) ([]BundlePlan, error) {
	// Cache-aside: try Redis first.
	if s.cache != nil {
		cached, err := s.cache.GetBundlePlansForSaleCache(ctx)
		if err == nil && cached != nil {
			var plans []BundlePlan
			if jsonErr := json.Unmarshal(cached, &plans); jsonErr == nil {
				return plans, nil
			}
		}
	}

	plans, err := s.planRepo.ListForSale(ctx)
	if err != nil {
		return nil, fmt.Errorf("list for-sale bundle plans: %w", err)
	}

	// Write back to cache.
	if s.cache != nil && len(plans) > 0 {
		if data, jsonErr := json.Marshal(plans); jsonErr == nil {
			_ = s.cache.SetBundlePlansForSaleCache(ctx, data, BundlePlanCacheTTL)
		}
	}

	return plans, nil
}
