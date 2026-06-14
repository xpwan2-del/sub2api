package repository

import (
	"context"
	"errors"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/upstreammodelprice"
	"github.com/Wei-Shaw/sub2api/ent/upstreampricechange"
	"github.com/Wei-Shaw/sub2api/ent/upstreampricesource"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// upstreamPriceRepository 实现 service.UpstreamPriceRepository，封装三张表
// （upstream_price_sources / upstream_model_prices / upstream_price_changes）的 CRUD。
//
// DTO 直接使用 ent 生成的实体类型，避免在纯数据载体表上重复定义领域类型。
type upstreamPriceRepository struct {
	client *dbent.Client
}

// NewUpstreamPriceRepository 构造 UpstreamPriceRepository 的 ent 实现。
func NewUpstreamPriceRepository(client *dbent.Client) service.UpstreamPriceRepository {
	return &upstreamPriceRepository{client: client}
}

// ============================================================
// source
// ============================================================

func (r *upstreamPriceRepository) CreateSource(ctx context.Context, s *dbent.UpstreamPriceSource) error {
	if s == nil {
		return errors.New("upstream price source is nil")
	}
	client := clientFromContext(ctx, r.client)

	created, err := client.UpstreamPriceSource.Create().
		SetName(s.Name).
		SetPlatform(s.Platform).
		SetBaseURL(s.BaseURL).
		SetPricingEndpoint(s.PricingEndpoint).
		SetAPIKey(s.APIKey).
		SetParserType(s.ParserType).
		SetParserConfig(s.ParserConfig).
		SetModelAliasMap(s.ModelAliasMap).
		SetSyncIntervalMinutes(s.SyncIntervalMinutes).
		SetAlertThresholdPct(s.AlertThresholdPct).
		SetCooldownMinutes(s.CooldownMinutes).
		SetEnabled(s.Enabled).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, nil)
	}

	applySourceFields(s, created)
	return nil
}

func (r *upstreamPriceRepository) UpdateSource(ctx context.Context, s *dbent.UpstreamPriceSource) error {
	if s == nil || s.ID == 0 {
		return service.ErrUpstreamPriceSourceNotFound
	}
	client := clientFromContext(ctx, r.client)

	builder := client.UpstreamPriceSource.UpdateOneID(s.ID).
		SetName(s.Name).
		SetPlatform(s.Platform).
		SetBaseURL(s.BaseURL).
		SetPricingEndpoint(s.PricingEndpoint).
		SetAPIKey(s.APIKey).
		SetParserType(s.ParserType).
		SetParserConfig(s.ParserConfig).
		SetModelAliasMap(s.ModelAliasMap).
		SetSyncIntervalMinutes(s.SyncIntervalMinutes).
		SetAlertThresholdPct(s.AlertThresholdPct).
		SetCooldownMinutes(s.CooldownMinutes).
		SetEnabled(s.Enabled)

	updated, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrUpstreamPriceSourceNotFound, nil)
	}

	s.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *upstreamPriceRepository) DeleteSource(ctx context.Context, id int64) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UpstreamPriceSource.Delete().Where(upstreampricesource.IDEQ(id)).Exec(ctx)
	return translatePersistenceError(err, nil, nil)
}

func (r *upstreamPriceRepository) GetSource(ctx context.Context, id int64) (*dbent.UpstreamPriceSource, error) {
	m, err := r.client.UpstreamPriceSource.Query().
		Where(upstreampricesource.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUpstreamPriceSourceNotFound, nil)
	}
	return m, nil
}

func (r *upstreamPriceRepository) ListSources(ctx context.Context) ([]*dbent.UpstreamPriceSource, error) {
	items, err := r.client.UpstreamPriceSource.Query().
		Order(dbent.Desc(upstreampricesource.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return make([]*dbent.UpstreamPriceSource, 0), nil
	}
	return items, nil
}

func (r *upstreamPriceRepository) ListEnabledSources(ctx context.Context) ([]*dbent.UpstreamPriceSource, error) {
	items, err := r.client.UpstreamPriceSource.Query().
		Where(upstreampricesource.EnabledEQ(true)).
		Order(dbent.Asc(upstreampricesource.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return make([]*dbent.UpstreamPriceSource, 0), nil
	}
	return items, nil
}

func (r *upstreamPriceRepository) UpdateSourceSyncResult(
	ctx context.Context,
	id int64,
	status, hash, lastErr string,
	syncedAt time.Time,
) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UpstreamPriceSource.UpdateOneID(id).
		SetLastSyncStatus(status).
		SetLastHash(hash).
		SetLastSyncError(lastErr).
		SetLastSyncAt(syncedAt).
		Save(ctx)
	return translatePersistenceError(err, service.ErrUpstreamPriceSourceNotFound, nil)
}

// ============================================================
// model_price
// ============================================================

// ReplaceModelPrices 在一个事务内删除 sourceID 的全部旧 model_price 记录后批量插入新记录，
// 失败时整体回滚。支持在外部事务上下文中复用 client（ErrTxStarted 时退化为当前 client）。
func (r *upstreamPriceRepository) ReplaceModelPrices(
	ctx context.Context,
	sourceID int64,
	prices []*dbent.UpstreamModelPrice,
) error {
	tx, err := r.client.Tx(ctx)
	if err != nil && !errors.Is(err, dbent.ErrTxStarted) {
		return err
	}

	var txClient *dbent.Client
	if err == nil {
		defer func() { _ = tx.Rollback() }()
		txClient = tx.Client()
	} else {
		txClient = r.client
	}

	// 删除旧记录
	if _, derr := txClient.UpstreamModelPrice.Delete().
		Where(upstreammodelprice.SourceIDEQ(sourceID)).
		Exec(ctx); derr != nil {
		return derr
	}

	// 无新记录：直接提交
	if len(prices) == 0 {
		if tx != nil {
			return tx.Commit()
		}
		return nil
	}

	builders := make([]*dbent.UpstreamModelPriceCreate, 0, len(prices))
	for i := range prices {
		p := prices[i]
		b := txClient.UpstreamModelPrice.Create().
			SetSourceID(sourceID).
			SetModelName(p.ModelName).
			SetInputPrice(p.InputPrice).
			SetOutputPrice(p.OutputPrice).
			SetCurrency(p.Currency).
			SetFetchedAt(p.FetchedAt)

		if p.LocalModelName != "" {
			b.SetLocalModelName(p.LocalModelName)
		}
		if p.CacheWritePrice != nil {
			b.SetCacheWritePrice(*p.CacheWritePrice)
		}
		if p.CacheReadPrice != nil {
			b.SetCacheReadPrice(*p.CacheReadPrice)
		}
		if p.ImageOutputPrice != nil {
			b.SetImageOutputPrice(*p.ImageOutputPrice)
		}
		if p.PerRequestPrice != nil {
			b.SetPerRequestPrice(*p.PerRequestPrice)
		}
		if p.RawPayload != nil {
			b.SetRawPayload(p.RawPayload)
		}
		builders = append(builders, b)
	}

	created, cerr := txClient.UpstreamModelPrice.CreateBulk(builders...).Save(ctx)
	if cerr != nil {
		return translatePersistenceError(cerr, nil, nil)
	}

	// 回填生成的 ID / 时间戳，便于调用方引用
	for i, c := range created {
		if i < len(prices) {
			prices[i].ID = c.ID
			prices[i].SourceID = c.SourceID
		}
	}

	if tx != nil {
		return tx.Commit()
	}
	return nil
}

func (r *upstreamPriceRepository) ListModelPrices(
	ctx context.Context,
	sourceID int64,
) ([]*dbent.UpstreamModelPrice, error) {
	items, err := r.client.UpstreamModelPrice.Query().
		Where(upstreammodelprice.SourceIDEQ(sourceID)).
		Order(dbent.Asc(upstreammodelprice.FieldModelName)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return make([]*dbent.UpstreamModelPrice, 0), nil
	}
	return items, nil
}

func (r *upstreamPriceRepository) ListAllModelPricesAsMap(
	ctx context.Context,
	sourceID int64,
) (map[string]*dbent.UpstreamModelPrice, error) {
	items, err := r.ListModelPrices(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*dbent.UpstreamModelPrice, len(items))
	for _, it := range items {
		out[it.ModelName] = it
	}
	return out, nil
}

// ============================================================
// change
// ============================================================

func (r *upstreamPriceRepository) InsertChanges(
	ctx context.Context,
	changes []*dbent.UpstreamPriceChange,
) error {
	if len(changes) == 0 {
		return nil
	}
	client := clientFromContext(ctx, r.client)

	builders := make([]*dbent.UpstreamPriceChangeCreate, 0, len(changes))
	for i := range changes {
		c := changes[i]
		b := client.UpstreamPriceChange.Create().
			SetSourceID(c.SourceID).
			SetModelName(c.ModelName).
			SetChangeType(c.ChangeType).
			SetCurrInputPrice(c.CurrInputPrice).
			SetCurrOutputPrice(c.CurrOutputPrice).
			SetInputDeltaPct(c.InputDeltaPct).
			SetOutputDeltaPct(c.OutputDeltaPct).
			SetDetectedAt(c.DetectedAt).
			SetStatus(c.Status)

		if c.LocalModelName != "" {
			b.SetLocalModelName(c.LocalModelName)
		}
		if c.PrevInputPrice != nil {
			b.SetPrevInputPrice(*c.PrevInputPrice)
		}
		if c.PrevOutputPrice != nil {
			b.SetPrevOutputPrice(*c.PrevOutputPrice)
		}
		if c.SuggestedInputPrice != 0 {
			b.SetSuggestedInputPrice(c.SuggestedInputPrice)
		}
		if c.SuggestedOutputPrice != 0 {
			b.SetSuggestedOutputPrice(c.SuggestedOutputPrice)
		}
		if c.SuggestedMultiplier != nil {
			b.SetSuggestedMultiplier(*c.SuggestedMultiplier)
		}
		builders = append(builders, b)
	}

	created, err := client.UpstreamPriceChange.CreateBulk(builders...).Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, nil)
	}
	for i, c := range created {
		if i < len(changes) {
			changes[i].ID = c.ID
		}
	}
	return nil
}

func (r *upstreamPriceRepository) ListPendingChanges(
	ctx context.Context,
	filters service.ChangeFilters,
) ([]*dbent.UpstreamPriceChange, error) {
	q := r.client.UpstreamPriceChange.Query()

	if filters.SourceID > 0 {
		q = q.Where(upstreampricechange.SourceIDEQ(filters.SourceID))
	}
	if filters.Status != "" {
		q = q.Where(upstreampricechange.StatusEQ(filters.Status))
	}
	// 默认只看 pending（status="" 时按 pending 语义返回，与命名一致）
	if filters.Status == "" {
		q = q.Where(upstreampricechange.StatusEQ(service.UpstreamPriceChangeStatusPending))
	}

	q = q.Order(dbent.Desc(upstreampricechange.FieldDetectedAt), dbent.Desc(upstreampricechange.FieldID))
	if filters.Limit > 0 {
		q = q.Limit(filters.Limit)
	}

	items, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return make([]*dbent.UpstreamPriceChange, 0), nil
	}
	return items, nil
}

func (r *upstreamPriceRepository) GetChange(
	ctx context.Context,
	id int64,
) (*dbent.UpstreamPriceChange, error) {
	m, err := r.client.UpstreamPriceChange.Query().
		Where(upstreampricechange.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrUpstreamPriceChangeNotFound, nil)
	}
	return m, nil
}

func (r *upstreamPriceRepository) UpdateChangeApplied(
	ctx context.Context,
	id, adminID int64,
	target string,
	targetID int64,
) error {
	client := clientFromContext(ctx, r.client)
	now := time.Now().UTC()
	_, err := client.UpstreamPriceChange.UpdateOneID(id).
		SetStatus(service.UpstreamPriceChangeStatusApplied).
		SetAppliedBy(adminID).
		SetAppliedTarget(target).
		SetAppliedTargetID(targetID).
		SetAppliedAt(now).
		Save(ctx)
	return translatePersistenceError(err, service.ErrUpstreamPriceChangeNotFound, nil)
}

// UpdateChangeDismissed 标记一条 change 为 dismissed（管理员忽略，不进计费）。
// 仅更新 status / applied_by / applied_at，applied_target / applied_target_id 保持默认。
func (r *upstreamPriceRepository) UpdateChangeDismissed(
	ctx context.Context,
	id, adminID int64,
) error {
	client := clientFromContext(ctx, r.client)
	now := time.Now().UTC()
	_, err := client.UpstreamPriceChange.UpdateOneID(id).
		SetStatus(service.UpstreamPriceChangeStatusDismissed).
		SetAppliedBy(adminID).
		SetAppliedAt(now).
		Save(ctx)
	return translatePersistenceError(err, service.ErrUpstreamPriceChangeNotFound, nil)
}

func (r *upstreamPriceRepository) MarkChangesNotified(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	client := clientFromContext(ctx, r.client)
	_, err := client.UpstreamPriceChange.Update().
		Where(upstreampricechange.IDIn(ids...)).
		SetNotified(true).
		Save(ctx)
	return translatePersistenceError(err, nil, nil)
}

// SetAppliedSnapshot 记录 apply 前的实际值快照（覆盖保护 + 撤销回滚用）。
// prevMultiplier 为 nil 时不写倍率字段（follow_cost 模式）。
func (r *upstreamPriceRepository) SetAppliedSnapshot(
	ctx context.Context,
	id, channelID int64,
	prevInputPrice, prevOutputPrice float64,
	prevMultiplier *float64,
) error {
	client := clientFromContext(ctx, r.client)
	upd := client.UpstreamPriceChange.UpdateOneID(id).
		SetAppliedPrevInputPrice(prevInputPrice).
		SetAppliedPrevOutputPrice(prevOutputPrice).
		SetAppliedChannelID(channelID)
	if prevMultiplier != nil {
		upd = upd.SetPrevMultiplier(*prevMultiplier)
	}
	if _, err := upd.Save(ctx); err != nil {
		return translatePersistenceError(err, service.ErrUpstreamPriceChangeNotFound, nil)
	}
	return nil
}

// MarkReverted 标记 change 已撤销（reverted_at + reverted_by）。status 保持 applied。
func (r *upstreamPriceRepository) MarkReverted(ctx context.Context, id, adminID int64) error {
	client := clientFromContext(ctx, r.client)
	now := time.Now().UTC()
	_, err := client.UpstreamPriceChange.UpdateOneID(id).
		SetRevertedAt(now).
		SetRevertedBy(adminID).
		Save(ctx)
	return translatePersistenceError(err, service.ErrUpstreamPriceChangeNotFound, nil)
}

// ============================================================
// helpers
// ============================================================

// applySourceFields 将 ent 写回结果的关键字段同步回入参 DTO（ID 与时间戳）。
func applySourceFields(dst *dbent.UpstreamPriceSource, src *dbent.UpstreamPriceSource) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
}
