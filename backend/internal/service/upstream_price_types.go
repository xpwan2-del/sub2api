package service

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// upstream price change 处理状态
const (
	UpstreamPriceChangeStatusPending   = "pending"
	UpstreamPriceChangeStatusApplied   = "applied"
	UpstreamPriceChangeStatusDismissed = "dismissed"
)

// 上游同步状态
const (
	UpstreamSyncStatusSuccess = "success"
	UpstreamSyncStatusFailed  = "failed"
	UpstreamSyncStatusPartial = "partial"
)

var (
	ErrUpstreamPriceSourceNotFound = infraerrors.NotFound("UPSTREAM_PRICE_SOURCE_NOT_FOUND", "upstream price source not found")
	ErrUpstreamPriceChangeNotFound = infraerrors.NotFound("UPSTREAM_PRICE_CHANGE_NOT_FOUND", "upstream price change not found")
)

// ChangeFilters 过滤 ListPendingChanges 的查询条件。
type ChangeFilters struct {
	SourceID int64
	Status   string // "" = pending(默认); 传具体状态值(如 applied/dismissed)查其他状态
	Limit    int
}

// UpstreamPriceRepository 封装上游价格三张表（source / model_price / change）的持久化操作。
//
// DTO 直接使用 ent 生成的实体类型（*dbent.UpstreamPriceSource 等），
// 因为这些表是纯数据载体，无中间业务转换需求。
type UpstreamPriceRepository interface {
	// ===== source =====
	CreateSource(ctx context.Context, s *dbent.UpstreamPriceSource) error
	UpdateSource(ctx context.Context, s *dbent.UpstreamPriceSource) error
	DeleteSource(ctx context.Context, id int64) error
	GetSource(ctx context.Context, id int64) (*dbent.UpstreamPriceSource, error)
	ListSources(ctx context.Context) ([]*dbent.UpstreamPriceSource, error)
	ListEnabledSources(ctx context.Context) ([]*dbent.UpstreamPriceSource, error)
	UpdateSourceSyncResult(ctx context.Context, id int64, status, hash, lastErr string, syncedAt time.Time) error

	// ===== model_price =====
	// ReplaceModelPrices 事务内删除 sourceID 的全部旧 model_price 后批量插入新记录。
	ReplaceModelPrices(ctx context.Context, sourceID int64, prices []*dbent.UpstreamModelPrice) error
	ListModelPrices(ctx context.Context, sourceID int64) ([]*dbent.UpstreamModelPrice, error)
	ListAllModelPricesAsMap(ctx context.Context, sourceID int64) (map[string]*dbent.UpstreamModelPrice, error)

	// ===== change =====
	InsertChanges(ctx context.Context, changes []*dbent.UpstreamPriceChange) error
	ListPendingChanges(ctx context.Context, filters ChangeFilters) ([]*dbent.UpstreamPriceChange, error)
	GetChange(ctx context.Context, id int64) (*dbent.UpstreamPriceChange, error)
	UpdateChangeApplied(ctx context.Context, id, adminID int64, target string, targetID int64) error
	MarkChangesNotified(ctx context.Context, ids []int64) error
}
