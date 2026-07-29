//go:build integration

// model_catalog_integration_test.go 模型广场运营配置 Postgres 集成测试（Task A10）
//
// 覆盖 ModelCatalogRepo 真实 Postgres SQL 路径（ON CONFLICT DO NOTHING / DO UPDATE、
// 复合 (platform, model_name) IN VALUES 查询、JSONB custom_tags 序列化）+ 经 adapter 的
// ModelCatalogService 端到端合并，验证 sqlite 单测无法覆盖的 PG 特定语义（upsert 冲突解决、
// JSONB、复合索引命中）。Repo 与 Service 的纯逻辑分支已由 sqlite/stub 单测覆盖，此处聚焦真实 DB。
//
// 放置说明（为何在 package repository 而非 service）：testEntTx / testEntClient 的 Postgres
// harness（testcontainers 启 PG + ApplyMigrations 建表）定义在本包且为 unexported，跨包无法复用。
// service 包仅有 sqlite/stub 单测、无 PG harness。本测试 import service 构造真实
// ModelCatalogService（与 bundle_upgrade_e2e_integration_test.go 同范式：service 层集成测试因
// harness 限制统一落在 repository 包）。
//
// 数据隔离：每个测试用 testEntTx 开独立 ent 事务（t.Cleanup 自动回滚），repo 经 withTx 的
// ErrTxStarted 复用路径加入同一事务，测试结束回滚，测试间互不污染。
//
// 运行：cd backend && go test -tags=integration ./internal/repository/ -run ModelCatalog -v
// 无 Docker/Postgres 时，integration_harness_test.go 的 TestMain 检测到 docker 不可用会
// os.Exit(0) 跳过本包全部集成测试（本开发环境即如此，待 CI 带 Docker 运行）。
package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/modelcatalogdisplay"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// catalogTestSettings 满足 service.modelCatalogSettings（unexported interface，但其方法均为
// 导出名 → 跨包结构化满足，无需命名该类型即可传入 NewModelCatalogService）。提供可控的
// ops_enabled / new_model_days，避免依赖真实 SettingService + DB 系统设置。
type catalogTestSettings struct {
	enabled bool
	newDays int
}

func (s catalogTestSettings) IsModelCatalogOpsEnabled(context.Context) bool   { return s.enabled }
func (s catalogTestSettings) GetModelCatalogNewModelDays(context.Context) int { return s.newDays }

// newCatalogIntegrationFixture 在隔离事务内构造真实 repo → adapter → service 链。
// client 绑定到 testEntTx 的事务；repo.withTx 检测到 r.client 已在事务中（ErrTxStarted）即复用，
// 故所有写操作落在该事务内，测试结束自动回滚。
func newCatalogIntegrationFixture(t *testing.T, enabled bool, newDays int) (svc service.ModelCatalogService, client *dbent.Client, ctx context.Context) {
	t.Helper()
	tx := testEntTx(t)
	client = tx.Client()
	ctx = context.Background()
	repo := NewModelCatalogRepo(client)
	adapter := NewModelCatalogServiceAdapter(repo)
	svc = service.NewModelCatalogService(adapter, catalogTestSettings{enabled: enabled, newDays: newDays})
	return svc, client, ctx
}

// seedCatalogRow 直接用 ent client 插入一行运营配置，可显式设 first_seen_at。用于构造
// MergeDisplayConfig / BatchSave 所需的既有持久化状态（绕过 service 派生逻辑精确控制）。
func seedCatalogRow(t *testing.T, ctx context.Context, client *dbent.Client, d *ModelCatalogDisplay) {
	t.Helper()
	b := client.ModelCatalogDisplay.Create().
		SetPlatform(d.Platform).
		SetModelName(d.ModelName).
		SetPinned(d.Pinned).
		SetSortWeight(d.SortWeight).
		SetCustomTags(d.CustomTags).
		SetHidden(d.Hidden)
	if d.FeaturedUntil != nil {
		b.SetFeaturedUntil(*d.FeaturedUntil)
	}
	if !d.FirstSeenAt.IsZero() {
		b.SetFirstSeenAt(d.FirstSeenAt)
	}
	_, err := b.Save(ctx)
	require.NoError(t, err)
}

// TestModelCatalogIntegration_UpsertMissing_PreservesFirstSeenAndInsertsMissing 守护：
// EnsureFirstSeen（→ repo.UpsertMissing，ON CONFLICT DO NOTHING）只插缺失键、不覆盖已存在行，
// 关键是不重置已存在行的 first_seen_at（一经设定不可变）。覆盖真实 PG 冲突路径。
func TestModelCatalogIntegration_UpsertMissing_PreservesFirstSeenAndInsertsMissing(t *testing.T) {
	svc, client, ctx := newCatalogIntegrationFixture(t, true, 30)

	old := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	seedCatalogRow(t, ctx, client, &ModelCatalogDisplay{
		Platform: "openai", ModelName: "gpt-5", FirstSeenAt: old,
	})

	// 同一批含已存在键 + 全新键。
	require.NoError(t, svc.EnsureFirstSeen(ctx, []service.ModelKey{
		{Platform: "openai", ModelName: "gpt-5"},     // 已存在 → DO NOTHING
		{Platform: "anthropic", ModelName: "claude"}, // 缺失 → 插入
	}))

	all, err := svc.ListAllForAdmin(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2, "one existing + one inserted")

	byKey := make(map[string]service.AdminCatalogConfig, len(all))
	for _, c := range all {
		byKey[c.Platform+"/"+c.ModelName] = c
	}
	// 已存在行：first_seen_at 必须保持旧值（不被重置为 now）。
	require.WithinDuration(t, old, byKey["openai/gpt-5"].FirstSeenAt, time.Second,
		"UpsertMissing must not overwrite first_seen_at of existing rows")
	// 新插入行：first_seen_at 由 DB 默认值填充（非零）。
	require.False(t, byKey["anthropic/claude"].FirstSeenAt.IsZero(),
		"missing key should be inserted with default first_seen_at")
}

// TestModelCatalogIntegration_BatchSave_UpsertOnUniqueConflict 守护：BatchSave（→ repo.BatchUpsert，
// ON CONFLICT (platform, model_name) DO UPDATE）在命中 unique 键时执行更新而非插入重复行，
// 且不改动 first_seen_at。覆盖真实 PG upsert 冲突解决路径。
func TestModelCatalogIntegration_BatchSave_UpsertOnUniqueConflict(t *testing.T) {
	svc, client, ctx := newCatalogIntegrationFixture(t, true, 30)

	old := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	seedCatalogRow(t, ctx, client, &ModelCatalogDisplay{
		Platform: "openai", ModelName: "gpt-5", FirstSeenAt: old,
	})

	future := time.Now().Add(48 * time.Hour).UTC()
	require.NoError(t, svc.BatchSave(ctx, []service.AdminCatalogConfig{
		{
			Platform: "openai", ModelName: "gpt-5",
			Pinned: true, SortWeight: 100,
			CustomTags:    []string{"recommended", "fast"},
			FeaturedUntil: &future,
			Hidden:        false,
		},
	}))

	// 唯一冲突走 UPDATE：表中仍只有一行，不产生重复。
	count, err := client.ModelCatalogDisplay.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count, "upsert on unique conflict must not duplicate row")

	got, err := svc.ListAllForAdmin(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	row := got[0]
	require.True(t, row.Pinned)
	require.Equal(t, 100, row.SortWeight)
	require.Equal(t, []string{"recommended", "fast"}, row.CustomTags)
	// 关键：BatchUpsert 不得覆盖 first_seen_at（旧值保留）。
	require.WithinDuration(t, old, row.FirstSeenAt, time.Second,
		"BatchUpsert must not overwrite first_seen_at")
}

// TestModelCatalogIntegration_MergeDisplayConfig_EndToEnd 端到端：真实 PG 配置经 adapter 合并进
// CatalogItem——验证 hidden 过滤、pinned/sort_weight 持久化透出、tags 合并（自动能力 + custom_tags +
// featured 条件标签）、featured 判定、未配置模型仍获非 nil 默认 Display。
func TestModelCatalogIntegration_MergeDisplayConfig_EndToEnd(t *testing.T) {
	svc, client, ctx := newCatalogIntegrationFixture(t, true, 30)

	// 用旧 first_seen_at 避免被判为 new（隔离 new 窗口逻辑到边界专项测试）。
	old := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	future := time.Now().Add(1 * time.Hour).UTC()
	seedCatalogRow(t, ctx, client, &ModelCatalogDisplay{
		Platform: "openai", ModelName: "gpt-5", FirstSeenAt: old,
		Pinned: true, SortWeight: 50, CustomTags: []string{"recommended"},
	})
	seedCatalogRow(t, ctx, client, &ModelCatalogDisplay{
		Platform: "anthropic", ModelName: "opus", FirstSeenAt: old, FeaturedUntil: &future,
	})
	seedCatalogRow(t, ctx, client, &ModelCatalogDisplay{
		Platform: "google", ModelName: "gemini", FirstSeenAt: old, Hidden: true,
	})

	items := []service.CatalogItem{
		{Platform: "openai", ModelName: "gpt-5", Capabilities: []string{"vision"}},
		{Platform: "anthropic", ModelName: "opus"},
		{Platform: "google", ModelName: "gemini"}, // hidden → 被过滤
		{Platform: "meta", ModelName: "llama"},    // 未配置 → Display 非 nil 默认
	}
	out, err := svc.MergeDisplayConfig(ctx, items)
	require.NoError(t, err)

	byName := make(map[string]*service.CatalogDisplayInfo, len(out))
	names := make([]string, 0, len(out))
	for _, it := range out {
		byName[it.ModelName] = it.Display
		names = append(names, it.ModelName)
	}
	// hidden 模型从广场过滤。
	require.NotContains(t, names, "gemini", "hidden model must be filtered from catalog")
	require.Len(t, out, 3)

	// gpt-5：pinned/sort_weight 透出；tags = 自动能力(vision) + custom(recommended)。
	gpt5 := byName["gpt-5"]
	require.NotNil(t, gpt5)
	require.True(t, gpt5.Pinned)
	require.Equal(t, 50, gpt5.SortWeight)
	require.Equal(t, []string{"vision", "recommended"}, gpt5.Tags)

	// opus：featured（未来 featured_until）→ Featured=true 且 tags 含 featured。
	opus := byName["opus"]
	require.NotNil(t, opus)
	require.True(t, opus.Featured)
	require.Contains(t, opus.Tags, "featured")

	// llama：未配置模型仍获非 nil Display（零值默认），不被丢弃。
	llama := byName["llama"]
	require.NotNil(t, llama, "unconfigured model must still receive a non-nil Display")
	require.False(t, llama.Pinned)
	require.False(t, llama.Featured)
}

// TestModelCatalogIntegration_MergeDisplayConfig_NewModelWindowBoundary 守护 new 窗口边界：
// classifyDisplay 用严格小于（now-first_seen_at < newModelDays*24h）。刚进窗口 → new；刚出窗口 → 非 new。
// 用 ±10s 余量规避 seed 与 merge 间的真实时间漂移。
func TestModelCatalogIntegration_MergeDisplayConfig_NewModelWindowBoundary(t *testing.T) {
	const newDays = 30
	svc, client, ctx := newCatalogIntegrationFixture(t, true, newDays)

	now := time.Now()
	window := time.Duration(newDays) * 24 * time.Hour
	inside := now.Add(-window + 10*time.Second)  // now-first_seen ≈ window-10s < window → new
	outside := now.Add(-window - 10*time.Second) // now-first_seen ≈ window+10s ≮ window → 非 new
	seedCatalogRow(t, ctx, client, &ModelCatalogDisplay{Platform: "p", ModelName: "fresh", FirstSeenAt: inside})
	seedCatalogRow(t, ctx, client, &ModelCatalogDisplay{Platform: "p", ModelName: "stale", FirstSeenAt: outside})

	out, err := svc.MergeDisplayConfig(ctx, []service.CatalogItem{
		{Platform: "p", ModelName: "fresh"},
		{Platform: "p", ModelName: "stale"},
	})
	require.NoError(t, err)

	byName := make(map[string]*service.CatalogDisplayInfo, len(out))
	for _, it := range out {
		byName[it.ModelName] = it.Display
	}
	require.True(t, byName["fresh"].IsNew, "first_seen within window should be IsNew=true")
	require.Contains(t, byName["fresh"].Tags, "new")
	require.False(t, byName["stale"].IsNew, "first_seen outside window should be IsNew=false")
	require.NotContains(t, byName["stale"].Tags, "new")
}

// TestModelCatalogIntegration_MergeDisplayConfig_DisabledNoOp 守护 ops_enabled=false：运营总开关
// 关闭时 merge 整体 no-op——不过滤 hidden、不置顶、Display=nil（回归原始价格排序展示）。
func TestModelCatalogIntegration_MergeDisplayConfig_DisabledNoOp(t *testing.T) {
	svc, client, ctx := newCatalogIntegrationFixture(t, false, 30) // ops_enabled=false

	old := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	seedCatalogRow(t, ctx, client, &ModelCatalogDisplay{
		Platform: "openai", ModelName: "gpt-5", FirstSeenAt: old, Hidden: true, Pinned: true,
	})

	out, err := svc.MergeDisplayConfig(ctx, []service.CatalogItem{
		{Platform: "openai", ModelName: "gpt-5", Capabilities: []string{"vision"}},
	})
	require.NoError(t, err)
	require.Len(t, out, 1, "disabled must not filter hidden items")
	require.Nil(t, out[0].Display, "disabled must not merge any display config")
}

// 编译期引用 modelcatalogdisplay 包（seedCatalogRow 未直接命名其导出常量，保留静态引用防止
// import 被误删，并确认 schema 生成常量存在）。
var _ = modelcatalogdisplay.FieldPlatform
