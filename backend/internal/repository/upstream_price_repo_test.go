package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

// newUpstreamPriceRepoSQLite 构造一个基于 sqlite 内存库的 ent client + UpstreamPriceRepository。
// 模式照搬 api_key_repo_last_used_unit_test.go / usage_cleanup_repo_ent_test.go。
func newUpstreamPriceRepoSQLite(t *testing.T) (service.UpstreamPriceRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:upstream_price_repo_test?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return NewUpstreamPriceRepository(client), client
}

func mustCreateUpstreamSource(t *testing.T, ctx context.Context, repo service.UpstreamPriceRepository, name string) *dbent.UpstreamPriceSource {
	t.Helper()
	src := &dbent.UpstreamPriceSource{
		Name:                name,
		Platform:            "openai",
		BaseURL:             "https://api.example.com",
		PricingEndpoint:     "/v1/prices",
		APIKey:              "secret-key",
		ParserType:          "one_api",
		SyncIntervalMinutes: 30,
		AlertThresholdPct:   20,
		CooldownMinutes:     60,
		Enabled:             true,
	}
	require.NoError(t, repo.CreateSource(ctx, src))
	require.NotZero(t, src.ID)
	return src
}

func floatPtr(v float64) *float64 { return &v }

// ============================================================
// source
// ============================================================

func TestUpstreamPriceRepo_CreateAndGetSource(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()

	src := mustCreateUpstreamSource(t, ctx, repo, "src1")
	require.False(t, src.CreatedAt.IsZero())

	got, err := repo.GetSource(ctx, src.ID)
	require.NoError(t, err)
	require.Equal(t, "src1", got.Name)
	require.Equal(t, "openai", got.Platform)
	require.True(t, got.Enabled)
	require.Equal(t, 30, got.SyncIntervalMinutes)
}

func TestUpstreamPriceRepo_GetSourceNotFound(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()

	_, err := repo.GetSource(ctx, 99999)
	require.ErrorIs(t, err, service.ErrUpstreamPriceSourceNotFound)
}

func TestUpstreamPriceRepo_UpdateSource(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()

	src := mustCreateUpstreamSource(t, ctx, repo, "src-update")
	src.Enabled = false
	src.AlertThresholdPct = 50
	require.NoError(t, repo.UpdateSource(ctx, src))

	got, err := repo.GetSource(ctx, src.ID)
	require.NoError(t, err)
	require.False(t, got.Enabled)
	require.Equal(t, float64(50), got.AlertThresholdPct)
}

func TestUpstreamPriceRepo_DeleteSource(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()

	src := mustCreateUpstreamSource(t, ctx, repo, "src-delete")
	require.NoError(t, repo.DeleteSource(ctx, src.ID))

	_, err := repo.GetSource(ctx, src.ID)
	require.ErrorIs(t, err, service.ErrUpstreamPriceSourceNotFound)
}

func TestUpstreamPriceRepo_ListSourcesEmpty(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()

	items, err := repo.ListSources(ctx)
	require.NoError(t, err)
	require.NotNil(t, items)
	require.Len(t, items, 0)
}

func TestUpstreamPriceRepo_ListSourcesAndEnabled(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()

	mustCreateUpstreamSource(t, ctx, repo, "enabled-1")
	disabled := mustCreateUpstreamSource(t, ctx, repo, "disabled-1")
	disabled.Enabled = false
	require.NoError(t, repo.UpdateSource(ctx, disabled))

	all, err := repo.ListSources(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)

	enabled, err := repo.ListEnabledSources(ctx)
	require.NoError(t, err)
	require.Len(t, enabled, 1)
	require.Equal(t, "enabled-1", enabled[0].Name)
}

func TestUpstreamPriceRepo_UpdateSourceSyncResult(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()

	src := mustCreateUpstreamSource(t, ctx, repo, "src-sync")
	syncedAt := time.Date(2024, 6, 14, 10, 0, 0, 0, time.UTC)

	require.NoError(t, repo.UpdateSourceSyncResult(ctx, src.ID, service.UpstreamSyncStatusSuccess, "hash-abc", "", syncedAt))

	got, err := repo.GetSource(ctx, src.ID)
	require.NoError(t, err)
	require.Equal(t, service.UpstreamSyncStatusSuccess, got.LastSyncStatus)
	require.Equal(t, "hash-abc", got.LastHash)
	require.Equal(t, "", got.LastSyncError)
	require.NotNil(t, got.LastSyncAt)
	require.WithinDuration(t, syncedAt, *got.LastSyncAt, time.Second)
}

// ============================================================
// model_price
// ============================================================

func TestUpstreamPriceRepo_ReplaceModelPricesIdempotent(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()
	src := mustCreateUpstreamSource(t, ctx, repo, "src-replace")

	now := time.Now().UTC().Truncate(time.Second)

	// 第一次：插入 2 条
	first := []*dbent.UpstreamModelPrice{
		{
			ModelName:       "gpt-4",
			InputPrice:      10,
			OutputPrice:     30,
			Currency:        "USD",
			FetchedAt:       now,
			CacheReadPrice:  floatPtr(1.5),
			CacheWritePrice: floatPtr(2.5),
		},
		{
			ModelName:   "gpt-3.5",
			InputPrice:  1,
			OutputPrice: 2,
			Currency:    "USD",
			FetchedAt:   now,
		},
	}
	require.NoError(t, repo.ReplaceModelPrices(ctx, src.ID, first))
	for _, p := range first {
		require.NotZero(t, p.ID)
	}

	listed, err := repo.ListModelPrices(ctx, src.ID)
	require.NoError(t, err)
	require.Len(t, listed, 2)

	// 第二次：替换为 3 条（gpt-4 不在其中，验证旧的删除干净）
	second := []*dbent.UpstreamModelPrice{
		{ModelName: "claude-3", InputPrice: 5, OutputPrice: 15, Currency: "USD", FetchedAt: now},
		{ModelName: "gemini-pro", InputPrice: 3, OutputPrice: 9, Currency: "USD", FetchedAt: now},
		{ModelName: "llama-3", InputPrice: 0.5, OutputPrice: 1, Currency: "USD", FetchedAt: now},
	}
	require.NoError(t, repo.ReplaceModelPrices(ctx, src.ID, second))

	listed, err = repo.ListModelPrices(ctx, src.ID)
	require.NoError(t, err)
	require.Len(t, listed, 3, "old rows must be deleted, replaced by new ones")

	names := map[string]bool{}
	for _, p := range listed {
		names[p.ModelName] = true
	}
	require.Contains(t, names, "claude-3")
	require.Contains(t, names, "gemini-pro")
	require.Contains(t, names, "llama-3")
	require.NotContains(t, names, "gpt-4", "old model rows should have been removed")
}

func TestUpstreamPriceRepo_ReplaceModelPricesEmptyClearsAll(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()
	src := mustCreateUpstreamSource(t, ctx, repo, "src-clear")

	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.ReplaceModelPrices(ctx, src.ID, []*dbent.UpstreamModelPrice{
		{ModelName: "m1", InputPrice: 1, OutputPrice: 2, Currency: "USD", FetchedAt: now},
	}))

	// 用空切片替换 → 全部清空
	require.NoError(t, repo.ReplaceModelPrices(ctx, src.ID, nil))

	listed, err := repo.ListModelPrices(ctx, src.ID)
	require.NoError(t, err)
	require.Len(t, listed, 0)
}

func TestUpstreamPriceRepo_ListModelPricesEmpty(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()
	src := mustCreateUpstreamSource(t, ctx, repo, "src-empty-models")

	listed, err := repo.ListModelPrices(ctx, src.ID)
	require.NoError(t, err)
	require.NotNil(t, listed)
	require.Len(t, listed, 0)
}

func TestUpstreamPriceRepo_ListModelPricesAsMap(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()
	src := mustCreateUpstreamSource(t, ctx, repo, "src-map")

	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.ReplaceModelPrices(ctx, src.ID, []*dbent.UpstreamModelPrice{
		{ModelName: "alpha", InputPrice: 1, OutputPrice: 2, Currency: "USD", FetchedAt: now},
		{ModelName: "beta", InputPrice: 3, OutputPrice: 4, Currency: "USD", FetchedAt: now},
	}))

	m, err := repo.ListAllModelPricesAsMap(ctx, src.ID)
	require.NoError(t, err)
	require.Len(t, m, 2)
	require.Contains(t, m, "alpha")
	require.Contains(t, m, "beta")
	require.Equal(t, float64(3), m["beta"].InputPrice)
}

// ============================================================
// change
// ============================================================

func TestUpstreamPriceRepo_ListPendingChangesEmpty(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()

	changes, err := repo.ListPendingChanges(ctx, service.ChangeFilters{})
	require.NoError(t, err)
	require.NotNil(t, changes)
	require.Len(t, changes, 0, "must return empty slice, not nil")
}

func TestUpstreamPriceRepo_InsertAndListPendingChanges(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()
	src := mustCreateUpstreamSource(t, ctx, repo, "src-change")

	now := time.Now().UTC().Truncate(time.Second)
	changes := []*dbent.UpstreamPriceChange{
		{
			SourceID:       src.ID,
			ModelName:      "model-a",
			ChangeType:     "price_change",
			PrevInputPrice: floatPtr(10),
			CurrInputPrice: 12,
			InputDeltaPct:  20,
			DetectedAt:     now,
			Status:         service.UpstreamPriceChangeStatusPending,
		},
		{
			SourceID:       src.ID,
			ModelName:      "model-b",
			ChangeType:     "added",
			CurrInputPrice: 5,
			DetectedAt:     now.Add(-time.Minute),
			Status:         service.UpstreamPriceChangeStatusPending,
		},
	}
	require.NoError(t, repo.InsertChanges(ctx, changes))
	for _, c := range changes {
		require.NotZero(t, c.ID)
	}

	// 空过滤器：默认 pending
	pending, err := repo.ListPendingChanges(ctx, service.ChangeFilters{})
	require.NoError(t, err)
	require.Len(t, pending, 2)

	// 按 source 过滤
	pendingSrc, err := repo.ListPendingChanges(ctx, service.ChangeFilters{SourceID: src.ID})
	require.NoError(t, err)
	require.Len(t, pendingSrc, 2)

	// 不存在的 source
	pendingOther, err := repo.ListPendingChanges(ctx, service.ChangeFilters{SourceID: 99999})
	require.NoError(t, err)
	require.Len(t, pendingOther, 0)

	// Limit
	pendingLimit, err := repo.ListPendingChanges(ctx, service.ChangeFilters{Limit: 1})
	require.NoError(t, err)
	require.Len(t, pendingLimit, 1)
}

func TestUpstreamPriceRepo_UpdateChangeApplied(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()
	src := mustCreateUpstreamSource(t, ctx, repo, "src-apply")

	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.InsertChanges(ctx, []*dbent.UpstreamPriceChange{
		{
			SourceID:       src.ID,
			ModelName:      "model-x",
			ChangeType:     "price_change",
			CurrInputPrice: 8,
			DetectedAt:     now,
			Status:         service.UpstreamPriceChangeStatusPending,
		},
	}))

	pending, err := repo.ListPendingChanges(ctx, service.ChangeFilters{SourceID: src.ID})
	require.NoError(t, err)
	require.Len(t, pending, 1)
	changeID := pending[0].ID
	require.Equal(t, service.UpstreamPriceChangeStatusPending, pending[0].Status)
	require.Nil(t, pending[0].AppliedBy)
	require.Nil(t, pending[0].AppliedAt)

	// 应用建议
	require.NoError(t, repo.UpdateChangeApplied(ctx, changeID, 42, "account", 100))

	got, err := repo.GetChange(ctx, changeID)
	require.NoError(t, err)
	require.Equal(t, service.UpstreamPriceChangeStatusApplied, got.Status)
	require.NotNil(t, got.AppliedBy)
	require.Equal(t, int64(42), *got.AppliedBy)
	require.NotNil(t, got.AppliedAt)
	require.Equal(t, "account", got.AppliedTarget)
	require.Equal(t, int64(100), got.AppliedTargetID)

	// 应用的 change 不再出现在 pending 列表
	pending2, err := repo.ListPendingChanges(ctx, service.ChangeFilters{SourceID: src.ID})
	require.NoError(t, err)
	require.Len(t, pending2, 0)
}

func TestUpstreamPriceRepo_GetChangeNotFound(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()

	_, err := repo.GetChange(ctx, 88888)
	require.ErrorIs(t, err, service.ErrUpstreamPriceChangeNotFound)
}

func TestUpstreamPriceRepo_MarkChangesNotified(t *testing.T) {
	repo, _ := newUpstreamPriceRepoSQLite(t)
	ctx := context.Background()
	src := mustCreateUpstreamSource(t, ctx, repo, "src-notify")

	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.InsertChanges(ctx, []*dbent.UpstreamPriceChange{
		{SourceID: src.ID, ModelName: "m1", ChangeType: "price_change", CurrInputPrice: 1, DetectedAt: now, Status: service.UpstreamPriceChangeStatusPending},
		{SourceID: src.ID, ModelName: "m2", ChangeType: "price_change", CurrInputPrice: 2, DetectedAt: now, Status: service.UpstreamPriceChangeStatusPending},
	}))

	pending, err := repo.ListPendingChanges(ctx, service.ChangeFilters{SourceID: src.ID})
	require.NoError(t, err)
	require.Len(t, pending, 2)
	require.False(t, pending[0].Notified)

	ids := []int64{pending[0].ID, pending[1].ID}
	require.NoError(t, repo.MarkChangesNotified(ctx, ids))

	pending2, err := repo.ListPendingChanges(ctx, service.ChangeFilters{SourceID: src.ID})
	require.NoError(t, err)
	require.True(t, pending2[0].Notified)
	require.True(t, pending2[1].Notified)

	// 空 ids 不应报错（不发空 IN 查询）
	require.NoError(t, repo.MarkChangesNotified(ctx, nil))
	require.NoError(t, repo.MarkChangesNotified(ctx, []int64{}))
}
