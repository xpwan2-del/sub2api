package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/modelcatalogdisplay"
	"github.com/Wei-Shaw/sub2api/ent/predicate"

	entsql "entgo.io/ent/dialect/sql"
)

// ModelCatalogDisplay 是模型广场可运营展示配置的 repository 层 DTO。
// 与 ent 实体 ModelCatalogDisplay 解耦，避免 service/handler 直接依赖 ent。
type ModelCatalogDisplay struct {
	Platform      string
	ModelName     string
	Pinned        bool
	SortWeight    int
	CustomTags    []string
	FeaturedUntil *time.Time
	Hidden        bool
	FirstSeenAt   time.Time
}

// ModelKey 标识一个 (platform, model_name) 二元组。
type ModelKey struct {
	Platform  string
	ModelName string
}

// ModelCatalogRepo 提供模型广场展示配置的持久化访问。
type ModelCatalogRepo interface {
	// GetByModelKeys 按 (platform, model_name) 复合键批量查询，返回 map。
	// map key 为 platform + "\x00" + model_name；未命中的 key 不出现在结果中。
	GetByModelKeys(ctx context.Context, keys []ModelKey) (map[string]*ModelCatalogDisplay, error)
	// UpsertMissing 为尚未存在的 (platform, model_name) 插入默认行（不覆盖已存在行的可运营字段与 first_seen_at）。
	UpsertMissing(ctx context.Context, keys []ModelKey) error
	// ListAll 返回全部展示配置（按 platform、model_name 升序），无数据时返回非 nil 空切片。
	ListAll(ctx context.Context) ([]*ModelCatalogDisplay, error)
	// BatchUpsert 按 (platform, model_name) 复合键 upsert 可运营字段（pinned/sort_weight/custom_tags/featured_until/hidden），
	// 不改动 first_seen_at 与 created_at。
	BatchUpsert(ctx context.Context, cfgs []*ModelCatalogDisplay) error
}

type modelCatalogRepo struct {
	client *dbent.Client
}

// NewModelCatalogRepo 创建模型广场展示配置的数据访问实例。
func NewModelCatalogRepo(client *dbent.Client) ModelCatalogRepo {
	return &modelCatalogRepo{client: client}
}

// modelKey 生成 map 复合键：platform + NUL + model_name（NUL 不会出现在正常字符串中，避免歧义碰撞）。
func modelKey(platform, modelName string) string {
	return platform + "\x00" + modelName
}

// GetByModelKeys 按 (platform, model_name) 复合键批量查询。
// 使用真正的复合 IN 查询（(platform, model_name) IN (VALUES ...)），避免全表扫描 + 内存过滤。
func (r *modelCatalogRepo) GetByModelKeys(ctx context.Context, keys []ModelKey) (map[string]*ModelCatalogDisplay, error) {
	out := make(map[string]*ModelCatalogDisplay, len(keys))
	if len(keys) == 0 {
		return out, nil
	}

	rows, err := r.client.ModelCatalogDisplay.Query().
		Where(modelCatalogKeyIn(keys)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("get model catalog displays by keys: %w", err)
	}

	for _, e := range rows {
		out[modelKey(e.Platform, e.ModelName)] = entToModelCatalogDisplay(e)
	}
	return out, nil
}

// UpsertMissing 为尚不存在的 (platform, model_name) 插入默认行。
// 采用 ON CONFLICT (platform, model_name) DO NOTHING：已存在的行原样保留（含 first_seen_at），
// 且天然规避并发先查后写（read-then-write）的竞态。参考 user_repo.AddGroupToAllowedGroups。
func (r *modelCatalogRepo) UpsertMissing(ctx context.Context, keys []ModelKey) error {
	// 整批在一个事务内执行：任一 key 失败则全部回滚，避免半写入。
	return r.withTx(ctx, func(ctx context.Context, client *dbent.Client) error {
		seen := make(map[string]bool, len(keys))
		for _, k := range keys {
			mk := modelKey(k.Platform, k.ModelName)
			if seen[mk] {
				continue // 去重，避免重复无操作调用
			}
			seen[mk] = true

			err := client.ModelCatalogDisplay.Create().
				SetPlatform(k.Platform).
				SetModelName(k.ModelName).
				OnConflictColumns(modelcatalogdisplay.FieldPlatform, modelcatalogdisplay.FieldModelName).
				DoNothing().
				Exec(ctx)
			// DoNothing 在命中冲突（未插入）时返回 sql.ErrNoRows，属预期，忽略。
			if isSQLNoRowsError(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("upsert missing model catalog display %s/%s: %w", k.Platform, k.ModelName, err)
			}
		}
		return nil
	})
}

// ListAll 返回全部展示配置，按 platform、model_name 升序排列以保证确定性（便于展示与测试）。
func (r *modelCatalogRepo) ListAll(ctx context.Context) ([]*ModelCatalogDisplay, error) {
	rows, err := r.client.ModelCatalogDisplay.Query().
		Order(
			modelcatalogdisplay.ByPlatform(),
			modelcatalogdisplay.ByModelName(),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all model catalog displays: %w", err)
	}

	out := make([]*ModelCatalogDisplay, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToModelCatalogDisplay(e))
	}
	return out, nil
}

// BatchUpsert 按 (platform, model_name) 复合键 upsert 可运营字段。
//
// 冲突时用 Update*()（SetExcluded → EXCLUDED.col）引用 INSERT 侧已序列化的值，
// 而非 Set*()：custom_tags 是 field.JSON([]string)，SetCustomTags(slice) 在 upsert 的
// SET 子句里会绕过 JSON 序列化、把原始 []string 直接传给驱动（sqlite 报 unsupported type）。
// 同时刻意只更新运营字段——不调用 UpdateFirstSeenAt()，避免覆盖一经设定即不可变的 first_seen_at
// （UpdateNewValues() 会把它重置为默认值 now，故不可用）。
// featured_until 为 nil 时清除该字段（全量状态 upsert 语义：管理员提交完整期望状态）。
func (r *modelCatalogRepo) BatchUpsert(ctx context.Context, cfgs []*ModelCatalogDisplay) error {
	// 整批在一个事务内执行：任一行失败则全部回滚（all-or-nothing），避免管理员保存半写入。
	return r.withTx(ctx, func(ctx context.Context, client *dbent.Client) error {
		for _, c := range cfgs {
			builder := client.ModelCatalogDisplay.Create().
				SetPlatform(c.Platform).
				SetModelName(c.ModelName).
				SetPinned(c.Pinned).
				SetSortWeight(c.SortWeight).
				SetCustomTags(c.CustomTags).
				SetHidden(c.Hidden)
			if c.FeaturedUntil != nil {
				builder.SetFeaturedUntil(*c.FeaturedUntil)
			}

			err := builder.
				OnConflictColumns(modelcatalogdisplay.FieldPlatform, modelcatalogdisplay.FieldModelName).
				Update(func(u *dbent.ModelCatalogDisplayUpsert) {
					u.UpdatePinned().
						UpdateSortWeight().
						UpdateCustomTags().
						UpdateHidden()
					if c.FeaturedUntil != nil {
						u.UpdateFeaturedUntil()
					} else {
						u.ClearFeaturedUntil()
					}
				}).
				Exec(ctx)
			if err != nil {
				return fmt.Errorf("batch upsert model catalog display %s/%s: %w", c.Platform, c.ModelName, err)
			}
		}
		return nil
	})
}

// withTx 在数据库事务中执行 fn；若 ctx 已携带外层事务（上层 service 事务 / 集成测试隔离事务）
// 则直接复用，不再嵌套开事务。与 bundle_usage_repo / affiliate_repo 的 withTx 同范式。
func (r *modelCatalogRepo) withTx(ctx context.Context, fn func(ctx context.Context, client *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		if errors.Is(err, dbent.ErrTxStarted) {
			return fn(ctx, r.client)
		}
		return fmt.Errorf("begin model catalog transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit model catalog transaction: %w", err)
	}
	return nil
}

// entToModelCatalogDisplay 将 ent 实体转换为 repository DTO。
func entToModelCatalogDisplay(e *dbent.ModelCatalogDisplay) *ModelCatalogDisplay {
	return &ModelCatalogDisplay{
		Platform:      e.Platform,
		ModelName:     e.ModelName,
		Pinned:        e.Pinned,
		SortWeight:    e.SortWeight,
		CustomTags:    e.CustomTags,
		FeaturedUntil: e.FeaturedUntil,
		Hidden:        e.Hidden,
		FirstSeenAt:   e.FirstSeenAt,
	}
}

// modelCatalogKeyIn 构造 (platform, model_name) 复合 IN 谓词：
//
//	(platform, model_name) IN (VALUES ($p1,$m1), ($p2,$m2), ...)
//
// 通过 ent selector 的原生谓词构建（参考 user_repo.userEmailLookupPredicate），
// 在 postgres 与 sqlite 上均可命中 (platform, model_name) 复合唯一索引。
func modelCatalogKeyIn(keys []ModelKey) predicate.ModelCatalogDisplay {
	return predicate.ModelCatalogDisplay(func(s *entsql.Selector) {
		s.Where(entsql.P(func(b *entsql.Builder) {
			b.WriteString("(").
				Ident(s.C(modelcatalogdisplay.FieldPlatform)).
				WriteString(", ").
				Ident(s.C(modelcatalogdisplay.FieldModelName)).
				WriteString(") IN (VALUES ")
			for i, k := range keys {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString("(").Arg(k.Platform).WriteString(", ").Arg(k.ModelName).WriteString(")")
			}
			b.WriteString(")")
		}))
	})
}
