// bundle_plan_port.go 套餐计划数据访问接口
// 定义 BundlePlanRepository 接口，解耦服务层与具体数据访问实现。

package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// BundlePlanRepository 套餐计划数据访问接口，提供 CRUD 和查询能力
// BundlePlanRepository defines the data-access interface for bundle plans.
type BundlePlanRepository interface {
	Create(ctx context.Context, plan *BundlePlan) error
	Update(ctx context.Context, plan *BundlePlan) error
	GetByID(ctx context.Context, id int64) (*BundlePlan, error)
	List(ctx context.Context, params pagination.PaginationParams, tier, status string) ([]BundlePlan, *pagination.PaginationResult, error)
	ListForSale(ctx context.Context) ([]BundlePlan, error)
	Delete(ctx context.Context, id int64) error
}
