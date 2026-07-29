package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/modelcatalogdisplay"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

// newModelCatalogRepoTestClient 构造一个 sqlite in-memory ent client，用于 ModelCatalogRepo 单测。
// 沿用 security_secret_bootstrap_test.go / api_key_repo_last_used_unit_test.go 的惯例。
func newModelCatalogRepoTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name)

	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// seedRowWithFirstSeenAt 直接用 ent client 插入一行并显式设置 first_seen_at，
// 用于验证 UpsertMissing / BatchUpsert 不会覆盖该时间戳（区别于「重置为 now」）。
func seedRowWithFirstSeenAt(t *testing.T, ctx context.Context, client *dbent.Client, platform, model string, firstSeen time.Time) {
	t.Helper()
	_, err := client.ModelCatalogDisplay.Create().
		SetPlatform(platform).
		SetModelName(model).
		SetFirstSeenAt(firstSeen).
		Save(ctx)
	require.NoError(t, err)
}

func TestModelCatalogRepo_UpsertMissing_InsertsNewKeys(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	keys := []ModelKey{
		{Platform: "openai", ModelName: "gpt-5"},
		{Platform: "anthropic", ModelName: "claude-opus-4"},
	}
	require.NoError(t, repo.UpsertMissing(ctx, keys))

	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
	// first_seen_at 应由 DB 默认值填充（非零）。
	for _, d := range all {
		require.False(t, d.FirstSeenAt.IsZero(), "first_seen_at should default to now on insert")
	}
}

func TestModelCatalogRepo_UpsertMissing_DoesNotOverwriteExisting(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	old := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	seedRowWithFirstSeenAt(t, ctx, client, "openai", "gpt-5", old)

	// 再次 UpsertMissing 同一 key：应 do-nothing，first_seen_at 不变，不产生重复行。
	require.NoError(t, repo.UpsertMissing(ctx, []ModelKey{{Platform: "openai", ModelName: "gpt-5"}}))

	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.WithinDuration(t, old, all[0].FirstSeenAt, time.Second)
}

func TestModelCatalogRepo_UpsertMissing_DedupesInputKeys(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	// 同一 key 重复传入，最终只应有一行。
	dup := []ModelKey{
		{Platform: "openai", ModelName: "gpt-5"},
		{Platform: "openai", ModelName: "gpt-5"},
		{Platform: "openai", ModelName: "gpt-5"},
	}
	require.NoError(t, repo.UpsertMissing(ctx, dup))

	count, err := client.ModelCatalogDisplay.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestModelCatalogRepo_UpsertMissing_EmptyKeys(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	require.NoError(t, repo.UpsertMissing(ctx, nil))
	require.NoError(t, repo.UpsertMissing(ctx, []ModelKey{}))
}

func TestModelCatalogRepo_GetByModelKeys_Empty(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	out, err := repo.GetByModelKeys(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Empty(t, out)
}

func TestModelCatalogRepo_GetByModelKeys_ReturnsOnlyRequestedCompositeKeys(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	require.NoError(t, repo.UpsertMissing(ctx, []ModelKey{
		{Platform: "openai", ModelName: "gpt-5"},
		{Platform: "openai", ModelName: "gpt-4o"},
		{Platform: "anthropic", ModelName: "claude-opus-4"},
		{Platform: "google", ModelName: "gemini-2.5-pro"},
	}))

	// 复合查询：openai/gpt-5 与 google/gemini-2.5-pro；openai/gpt-4o 不应返回。
	out, err := repo.GetByModelKeys(ctx, []ModelKey{
		{Platform: "openai", ModelName: "gpt-5"},
		{Platform: "google", ModelName: "gemini-2.5-pro"},
		{Platform: "openai", ModelName: "does-not-exist"},
	})
	require.NoError(t, err)
	require.Len(t, out, 2)
	require.Contains(t, out, modelKey("openai", "gpt-5"))
	require.Contains(t, out, modelKey("google", "gemini-2.5-pro"))
	require.NotContains(t, out, modelKey("openai", "gpt-4o"))
	require.Equal(t, "openai", out[modelKey("openai", "gpt-5")].Platform)
	require.Equal(t, "gpt-5", out[modelKey("openai", "gpt-5")].ModelName)
}

func TestModelCatalogRepo_ListAll_EmptyReturnsNonNilSlice(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	require.NotNil(t, all)
	require.Empty(t, all)
	// 防止 nil slice 序列化为 null（API 规范：空列表返回 []）。
	require.Equal(t, []*ModelCatalogDisplay{}, all)
}

func TestModelCatalogRepo_ListAll_DeterministicOrder(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	require.NoError(t, repo.UpsertMissing(ctx, []ModelKey{
		{Platform: "google", ModelName: "gemini"},
		{Platform: "anthropic", ModelName: "claude"},
		{Platform: "openai", ModelName: "gpt"},
	}))

	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 3)
	// 期望按 platform, model_name 升序。
	require.Equal(t, "anthropic", all[0].Platform)
	require.Equal(t, "google", all[1].Platform)
	require.Equal(t, "openai", all[2].Platform)
}

func TestModelCatalogRepo_BatchUpsert_UpdatesOperationalFields(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	// 预置一行，first_seen_at 设为明显的旧时间。
	old := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	seedRowWithFirstSeenAt(t, ctx, client, "openai", "gpt-5", old)

	featured := time.Now().Add(48 * time.Hour).UTC()
	require.NoError(t, repo.BatchUpsert(ctx, []*ModelCatalogDisplay{
		{
			Platform:      "openai",
			ModelName:     "gpt-5",
			Pinned:        true,
			SortWeight:    100,
			CustomTags:    []string{"recommended", "fast"},
			FeaturedUntil: &featured,
			Hidden:        false,
			IsNew:         true,
			Featured:      true,
		},
	}))

	got, err := repo.GetByModelKeys(ctx, []ModelKey{{Platform: "openai", ModelName: "gpt-5"}})
	require.NoError(t, err)
	d := got[modelKey("openai", "gpt-5")]
	require.NotNil(t, d)
	require.True(t, d.Pinned)
	require.Equal(t, 100, d.SortWeight)
	require.Equal(t, []string{"recommended", "fast"}, d.CustomTags)
	require.False(t, d.Hidden)
	require.True(t, d.IsNew)
	require.True(t, d.Featured)
	require.NotNil(t, d.FeaturedUntil)
	require.WithinDuration(t, featured, *d.FeaturedUntil, time.Second)
	// 关键：BatchUpsert 不得覆盖 first_seen_at（旧时间被保留）。
	require.WithinDuration(t, old, d.FirstSeenAt, time.Second)
}

func TestModelCatalogRepo_BatchUpsert_InsertsNewRowWithDefaults(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	require.NoError(t, repo.BatchUpsert(ctx, []*ModelCatalogDisplay{
		{
			Platform:   "anthropic",
			ModelName:  "claude-opus-4",
			Pinned:     true,
			SortWeight: 5,
			Hidden:     true,
		},
	}))

	got, err := repo.GetByModelKeys(ctx, []ModelKey{{Platform: "anthropic", ModelName: "claude-opus-4"}})
	require.NoError(t, err)
	d := got[modelKey("anthropic", "claude-opus-4")]
	require.NotNil(t, d)
	require.True(t, d.Pinned)
	require.Equal(t, 5, d.SortWeight)
	require.True(t, d.Hidden)
	// 新行 first_seen_at 由默认值填充。
	require.False(t, d.FirstSeenAt.IsZero())
}

func TestModelCatalogRepo_BatchUpsert_ClearsFeaturedUntilWhenNil(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	// 先用 BatchUpsert 设置 featured_until。
	featured := time.Now().Add(48 * time.Hour).UTC()
	require.NoError(t, repo.BatchUpsert(ctx, []*ModelCatalogDisplay{
		{Platform: "openai", ModelName: "gpt-5", FeaturedUntil: &featured},
	}))
	// 再次 BatchUpsert 且 featured_until=nil：应清除（全量状态 upsert 语义）。
	require.NoError(t, repo.BatchUpsert(ctx, []*ModelCatalogDisplay{
		{Platform: "openai", ModelName: "gpt-5"},
	}))

	got, err := repo.GetByModelKeys(ctx, []ModelKey{{Platform: "openai", ModelName: "gpt-5"}})
	require.NoError(t, err)
	d := got[modelKey("openai", "gpt-5")]
	require.NotNil(t, d)
	require.Nil(t, d.FeaturedUntil)
}

func TestModelCatalogRepo_BatchUpsert_Empty(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	require.NoError(t, repo.BatchUpsert(ctx, nil))
	require.NoError(t, repo.BatchUpsert(ctx, []*ModelCatalogDisplay{}))
}

// TestModelCatalogRepo_BatchUpsert_RollsBackOnPartialFailure 验证事务原子性：
// 批内第一行合法（事务内先写入），第二行校验失败 → 整批回滚，无任何行持久化。
func TestModelCatalogRepo_BatchUpsert_RollsBackOnPartialFailure(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo := NewModelCatalogRepo(client)
	ctx := context.Background()

	// 注入 model_name 校验器：第二行的 model_name 触发 Go 级校验失败（早于 SQL 执行，
	// 在 sqlite/postgres 上行为一致）。
	orig := modelcatalogdisplay.ModelNameValidator
	modelcatalogdisplay.ModelNameValidator = func(s string) error {
		if s == "INVALID" {
			return errors.New("invalid model name")
		}
		return nil
	}
	t.Cleanup(func() { modelcatalogdisplay.ModelNameValidator = orig })

	err := repo.BatchUpsert(ctx, []*ModelCatalogDisplay{
		{Platform: "openai", ModelName: "gpt-5"},      // 合法：事务内先写入
		{Platform: "anthropic", ModelName: "INVALID"}, // 触发校验失败
	})
	require.Error(t, err)

	// 整批回滚 → 无任何行持久化。
	count, err := client.ModelCatalogDisplay.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, count, "batch should roll back on partial failure (all-or-nothing)")
}

// TestModelCatalogRepo_WithTx_RollsBackOnError 直接验证 withTx 回滚语义。
func TestModelCatalogRepo_WithTx_RollsBackOnError(t *testing.T) {
	client := newModelCatalogRepoTestClient(t)
	repo, ok := NewModelCatalogRepo(client).(*modelCatalogRepo)
	require.True(t, ok, "expected *modelCatalogRepo concrete type")
	ctx := context.Background()

	boom := errors.New("boom")
	err := repo.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		// 事务内写入一行。
		_, e := txClient.ModelCatalogDisplay.Create().
			SetPlatform("openai").
			SetModelName("gpt-5").
			Save(txCtx)
		if e != nil {
			return e
		}
		// 模拟后续步骤失败。
		return boom
	})
	require.ErrorIs(t, err, boom)

	// 事务回滚 → 行未持久化。
	count, err := client.ModelCatalogDisplay.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

// 静态保证 ModelCatalogRepo 接口被具体实现满足（编译期约束）。
var _ ModelCatalogRepo = (*modelCatalogRepo)(nil)

// modelcatalogdisplay 包被测试引用（防止 import 被优化移除，同时确认常量存在）。
var _ = modelcatalogdisplay.FieldPlatform
