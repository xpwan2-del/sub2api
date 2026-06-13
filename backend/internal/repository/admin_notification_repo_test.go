package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

// newAdminNotificationRepoSQLite 构造一个基于 sqlite 内存库的 ent client + AdminNotificationRepository。
// 模式照搬 upstream_price_repo_test.go / announcement_read_repo_test.go。
func newAdminNotificationRepoSQLite(t *testing.T) (service.AdminNotificationRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:admin_notification_repo_test?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return NewAdminNotificationRepository(client), client
}

func mustCreateAdminNotification(t *testing.T, ctx context.Context, repo service.AdminNotificationRepository, title, severity string) *dbent.AdminNotification {
	t.Helper()
	n := &dbent.AdminNotification{
		Type:     "system",
		Title:    title,
		Content:  "body of " + title,
		Severity: severity,
	}
	require.NoError(t, repo.Create(ctx, n))
	require.NotZero(t, n.ID)
	require.False(t, n.CreatedAt.IsZero())
	return n
}

// ============================================================
// Create
// ============================================================

func TestAdminNotificationRepo_CreateAndGetFields(t *testing.T) {
	repo, client := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	n := &dbent.AdminNotification{
		Type:       "price_change",
		Title:      "GPT-4 涨价",
		Content:    "上游 OpenAI 上调了 gpt-4 价格",
		Severity:   "warning",
		TargetLink: "/admin/upstream-prices",
		RelatedIds: []int64{10, 20},
	}
	require.NoError(t, repo.Create(ctx, n))
	require.NotZero(t, n.ID)
	require.False(t, n.CreatedAt.IsZero())

	got, err := client.AdminNotification.Get(ctx, n.ID)
	require.NoError(t, err)
	require.Equal(t, "price_change", got.Type)
	require.Equal(t, "GPT-4 涨价", got.Title)
	require.Equal(t, "warning", got.Severity)
	require.Equal(t, "/admin/upstream-prices", got.TargetLink)
	require.Equal(t, []int64{10, 20}, got.RelatedIds)
}

func TestAdminNotificationRepo_CreateNil(t *testing.T) {
	repo, _ := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()
	err := repo.Create(ctx, nil)
	require.Error(t, err)
}

// ============================================================
// ListUnreadByUser / CountUnreadByUser
// ============================================================

func TestAdminNotificationRepo_ListUnreadEmptyReturnsSlice(t *testing.T) {
	repo, _ := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	items, err := repo.ListUnreadByUser(ctx, 1, 10)
	require.NoError(t, err)
	require.NotNil(t, items, "must return empty slice, not nil")
	require.Len(t, items, 0)

	count, err := repo.CountUnreadByUser(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}

func TestAdminNotificationRepo_ListUnreadHitsUnread(t *testing.T) {
	repo, _ := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	mustCreateAdminNotification(t, ctx, repo, "n1", "info")
	mustCreateAdminNotification(t, ctx, repo, "n2", "warning")

	items, err := repo.ListUnreadByUser(ctx, 42, 10)
	require.NoError(t, err)
	require.Len(t, items, 2)
	// created_at DESC 排序：后插入的 n2 在前
	require.Equal(t, "n2", items[0].Title)
	require.Equal(t, "n1", items[1].Title)

	count, err := repo.CountUnreadByUser(ctx, 42)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

func TestAdminNotificationRepo_ListUnreadLimit(t *testing.T) {
	repo, _ := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		mustCreateAdminNotification(t, ctx, repo, "n", "info")
	}

	items, err := repo.ListUnreadByUser(ctx, 1, 2)
	require.NoError(t, err)
	require.Len(t, items, 2)
}

func TestAdminNotificationRepo_MarkReadClearsUnread(t *testing.T) {
	repo, _ := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	n := mustCreateAdminNotification(t, ctx, repo, "read-me", "info")
	readAt := time.Date(2024, 6, 14, 10, 0, 0, 0, time.UTC)

	// 标记已读前：1 条未读
	items, err := repo.ListUnreadByUser(ctx, 7, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)

	// 标记已读
	require.NoError(t, repo.MarkRead(ctx, n.ID, 7, readAt))

	// 标记已读后：0 条未读
	items, err = repo.ListUnreadByUser(ctx, 7, 10)
	require.NoError(t, err)
	require.Len(t, items, 0)

	count, err := repo.CountUnreadByUser(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)
}

func TestAdminNotificationRepo_MarkReadIdempotent(t *testing.T) {
	repo, client := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	n := mustCreateAdminNotification(t, ctx, repo, "idempotent", "info")
	readAt := time.Date(2024, 6, 14, 10, 0, 0, 0, time.UTC)

	// 第一次 MarkRead
	require.NoError(t, repo.MarkRead(ctx, n.ID, 7, readAt))

	// 第二次 MarkRead（重复）— 不报错、不重复插入
	require.NoError(t, repo.MarkRead(ctx, n.ID, 7, readAt))

	// read 表中只应有一条记录
	rows, err := client.AdminNotificationRead.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

// ============================================================
// 多用户隔离
// ============================================================

func TestAdminNotificationRepo_UserIsolation(t *testing.T) {
	repo, _ := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	n := mustCreateAdminNotification(t, ctx, repo, "shared", "info")
	readAt := time.Now().UTC()

	// user A 标记已读
	require.NoError(t, repo.MarkRead(ctx, n.ID, 1, readAt))

	// user A: 0 未读
	aUnread, err := repo.ListUnreadByUser(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, aUnread, 0)

	// user B: 仍看到该通知为未读（用户隔离）
	bUnread, err := repo.ListUnreadByUser(ctx, 2, 10)
	require.NoError(t, err)
	require.Len(t, bUnread, 1)
	require.Equal(t, n.ID, bUnread[0].ID)

	bCount, err := repo.CountUnreadByUser(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, int64(1), bCount)
}

// ============================================================
// MarkAllRead
// ============================================================

func TestAdminNotificationRepo_MarkAllRead(t *testing.T) {
	repo, _ := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	// 插 3 条未读（user 7）
	mustCreateAdminNotification(t, ctx, repo, "a1", "info")
	mustCreateAdminNotification(t, ctx, repo, "a2", "warning")
	mustCreateAdminNotification(t, ctx, repo, "a3", "critical")

	// user 7 看见 3 条未读
	count, err := repo.CountUnreadByUser(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)

	// 标记全部已读
	require.NoError(t, repo.MarkAllRead(ctx, 7, time.Now().UTC()))

	// 0 条未读
	count, err = repo.CountUnreadByUser(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, int64(0), count)

	items, err := repo.ListUnreadByUser(ctx, 7, 10)
	require.NoError(t, err)
	require.Len(t, items, 0)
}

func TestAdminNotificationRepo_MarkAllReadNoUnreadIsNoop(t *testing.T) {
	repo, _ := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	// 没有任何通知时 MarkAllRead 不应报错
	require.NoError(t, repo.MarkAllRead(ctx, 99, time.Now().UTC()))
}

func TestAdminNotificationRepo_MarkAllReadUserIsolation(t *testing.T) {
	repo, _ := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	// 3 条通知，user A MarkAllRead 后 user B 仍看到全部未读
	mustCreateAdminNotification(t, ctx, repo, "x1", "info")
	mustCreateAdminNotification(t, ctx, repo, "x2", "info")
	mustCreateAdminNotification(t, ctx, repo, "x3", "info")

	require.NoError(t, repo.MarkAllRead(ctx, 1, time.Now().UTC()))

	aCount, err := repo.CountUnreadByUser(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, int64(0), aCount)

	bCount, err := repo.CountUnreadByUser(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, int64(3), bCount)
}

// ============================================================
// ListAll 分页
// ============================================================

func TestAdminNotificationRepo_ListAllEmptyReturnsSlice(t *testing.T) {
	repo, _ := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	items, page, err := repo.ListAll(ctx, defaultPagination())
	require.NoError(t, err)
	require.NotNil(t, items)
	require.Len(t, items, 0)
	require.NotNil(t, page)
	require.Equal(t, int64(0), page.Total)
}

func TestAdminNotificationRepo_ListAllPaginates(t *testing.T) {
	repo, _ := newAdminNotificationRepoSQLite(t)
	ctx := context.Background()

	// 插 3 条
	mustCreateAdminNotification(t, ctx, repo, "p1", "info")
	mustCreateAdminNotification(t, ctx, repo, "p2", "info")
	mustCreateAdminNotification(t, ctx, repo, "p3", "info")

	// 第一页 size=2
	page1, page, err := repo.ListAll(ctx, paginationFor(1, 2))
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.Equal(t, int64(3), page.Total)
	require.Equal(t, 2, page.Pages)
	// created_at DESC：最新的 p3, p2 在第一页
	require.Equal(t, "p3", page1[0].Title)
	require.Equal(t, "p2", page1[1].Title)

	// 第二页 size=2 → 只剩 1 条
	page2, _, err := repo.ListAll(ctx, paginationFor(2, 2))
	require.NoError(t, err)
	require.Len(t, page2, 1)
	require.Equal(t, "p1", page2[0].Title)
}

// ============================================================
// helpers
// ============================================================

func defaultPagination() pagination.PaginationParams {
	return pagination.PaginationParams{Page: 1, PageSize: 20}
}

func paginationFor(page, size int) pagination.PaginationParams {
	return pagination.PaginationParams{Page: page, PageSize: size}
}
