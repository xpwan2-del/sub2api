package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// BundlePlanRepository defines the data-access interface for bundle plans.
type BundlePlanRepository interface {
	Create(ctx context.Context, plan *BundlePlan) error
	Update(ctx context.Context, plan *BundlePlan) error
	GetByID(ctx context.Context, id int64) (*BundlePlan, error)
	List(ctx context.Context, params pagination.PaginationParams, tier, status string) ([]BundlePlan, *pagination.PaginationResult, error)
	ListForSale(ctx context.Context) ([]BundlePlan, error)
	Delete(ctx context.Context, id int64) error
}
