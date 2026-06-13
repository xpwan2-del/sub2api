package service

import (
	"context"
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// adminNotificationRepoStub 实现 AdminNotificationRepository，记录所有调用以便断言。
type adminNotificationRepoStub struct {
	// Create
	createCalled int
	createArg    *dbent.AdminNotification
	createErr    error

	// ListUnreadByUser
	listUnreadCalled   int
	listUnreadUserID   int64
	listUnreadLimit    int
	listUnreadResult   []*dbent.AdminNotification
	listUnreadErr      error
	// forceNilListUnread 让 stub 返回 nil（模拟 repo 未兜底的边界），用于验证 service 兜底成空切片。
	forceNilListUnread bool

	// CountUnreadByUser
	countUnreadCalled int
	countUnreadUserID int64
	countUnreadResult int64
	countUnreadErr    error

	// MarkRead
	markReadCalled         int
	markReadNotificationID int64
	markReadUserID         int64
	markReadReadAt         time.Time
	markReadErr            error

	// MarkAllRead
	markAllReadCalled int
	markAllReadUserID int64
	markAllReadReadAt time.Time
	markAllReadErr    error

	// ListAll
	listAllCalled int
	listAllResult []*dbent.AdminNotification
	listAllPage   *pagination.PaginationResult
	listAllErr    error
}

func (s *adminNotificationRepoStub) Create(_ context.Context, n *dbent.AdminNotification) error {
	s.createCalled++
	s.createArg = n
	return s.createErr
}

func (s *adminNotificationRepoStub) ListUnreadByUser(_ context.Context, userID int64, limit int) ([]*dbent.AdminNotification, error) {
	s.listUnreadCalled++
	s.listUnreadUserID = userID
	s.listUnreadLimit = limit
	if s.listUnreadErr != nil {
		return nil, s.listUnreadErr
	}
	if s.forceNilListUnread {
		return nil, nil
	}
	if s.listUnreadResult == nil {
		return make([]*dbent.AdminNotification, 0), nil
	}
	return s.listUnreadResult, nil
}

func (s *adminNotificationRepoStub) CountUnreadByUser(_ context.Context, userID int64) (int64, error) {
	s.countUnreadCalled++
	s.countUnreadUserID = userID
	return s.countUnreadResult, s.countUnreadErr
}

func (s *adminNotificationRepoStub) MarkRead(_ context.Context, notificationID, userID int64, readAt time.Time) error {
	s.markReadCalled++
	s.markReadNotificationID = notificationID
	s.markReadUserID = userID
	s.markReadReadAt = readAt
	return s.markReadErr
}

func (s *adminNotificationRepoStub) MarkAllRead(_ context.Context, userID int64, readAt time.Time) error {
	s.markAllReadCalled++
	s.markAllReadUserID = userID
	s.markAllReadReadAt = readAt
	return s.markAllReadErr
}

func (s *adminNotificationRepoStub) ListAll(_ context.Context, _ pagination.PaginationParams) ([]*dbent.AdminNotification, *pagination.PaginationResult, error) {
	s.listAllCalled++
	return s.listAllResult, s.listAllPage, s.listAllErr
}

// ====================================================================
// Create
// ====================================================================

func TestAdminNotificationServiceCreateRequiresType(t *testing.T) {
	repo := &adminNotificationRepoStub{}
	svc := NewAdminNotificationService(repo)

	_, err := svc.Create(context.Background(), "  ", "title", "content", "", "", nil)
	require.ErrorIs(t, err, ErrAdminNotificationTypeRequired)
	require.Zero(t, repo.createCalled, "repo.Create should not be called on validation failure")
}

func TestAdminNotificationServiceCreateRequiresTitle(t *testing.T) {
	repo := &adminNotificationRepoStub{}
	svc := NewAdminNotificationService(repo)

	_, err := svc.Create(context.Background(), "system", "  ", "content", "", "", nil)
	require.ErrorIs(t, err, ErrAdminNotificationTitleRequired)
	require.Zero(t, repo.createCalled)
}

func TestAdminNotificationServiceCreateRequiresContent(t *testing.T) {
	repo := &adminNotificationRepoStub{}
	svc := NewAdminNotificationService(repo)

	_, err := svc.Create(context.Background(), "system", "title", "  ", "", "", nil)
	require.ErrorIs(t, err, ErrAdminNotificationContentRequired)
	require.Zero(t, repo.createCalled)
}

func TestAdminNotificationServiceCreateDefaultsSeverityToInfo(t *testing.T) {
	repo := &adminNotificationRepoStub{}
	svc := NewAdminNotificationService(repo)

	n, err := svc.Create(context.Background(), "price_change", "title", "content", "   ", "/admin/prices", []int64{1, 2})
	require.NoError(t, err)
	require.Equal(t, 1, repo.createCalled)

	require.NotNil(t, repo.createArg)
	require.Equal(t, "info", repo.createArg.Severity, "empty severity should default to info")
	require.Equal(t, "price_change", repo.createArg.Type)
	require.Equal(t, "title", repo.createArg.Title)
	require.Equal(t, "content", repo.createArg.Content)
	require.Equal(t, "/admin/prices", repo.createArg.TargetLink)
	require.Equal(t, []int64{1, 2}, repo.createArg.RelatedIds)

	// 返回值就是 repo 回写的同一对象
	require.Same(t, repo.createArg, n)
}

func TestAdminNotificationServiceCreatePreservesExplicitSeverity(t *testing.T) {
	repo := &adminNotificationRepoStub{}
	svc := NewAdminNotificationService(repo)

	_, err := svc.Create(context.Background(), "ops_alert", "title", "content", "critical", "", nil)
	require.NoError(t, err)
	require.Equal(t, "critical", repo.createArg.Severity)
	require.Nil(t, repo.createArg.RelatedIds, "nil relatedIDs should pass through as nil")
}

func TestAdminNotificationServiceCreateTrimsWhitespace(t *testing.T) {
	repo := &adminNotificationRepoStub{}
	svc := NewAdminNotificationService(repo)

	_, err := svc.Create(context.Background(), "  system  ", "  title  ", "  content  ", " warning ", "  /link  ", []int64{9})
	require.NoError(t, err)
	require.Equal(t, "system", repo.createArg.Type)
	require.Equal(t, "title", repo.createArg.Title)
	require.Equal(t, "content", repo.createArg.Content)
	require.Equal(t, "warning", repo.createArg.Severity)
	require.Equal(t, "/link", repo.createArg.TargetLink)
	require.Equal(t, []int64{9}, repo.createArg.RelatedIds)
}

func TestAdminNotificationServiceCreateDoesNotMutateCallerSlice(t *testing.T) {
	repo := &adminNotificationRepoStub{}
	svc := NewAdminNotificationService(repo)

	caller := []int64{1, 2, 3}
	_, err := svc.Create(context.Background(), "system", "t", "c", "", "", caller)
	require.NoError(t, err)

	// service 内部拷贝，不应影响调用方原始切片
	caller[0] = 999
	require.Equal(t, int64(1), repo.createArg.RelatedIds[0], "internal slice should be decoupled from caller's")
}

func TestAdminNotificationServiceCreateWrapsRepoError(t *testing.T) {
	repoErr := errors.New("db down")
	repo := &adminNotificationRepoStub{createErr: repoErr}
	svc := NewAdminNotificationService(repo)

	_, err := svc.Create(context.Background(), "system", "t", "c", "", "", nil)
	require.ErrorIs(t, err, repoErr, "repo error should be wrapped and unwrappable")
}

// ====================================================================
// ListUnread limit 边界
// ====================================================================

func TestAdminNotificationServiceListUnreadDefaultLimitWhenZeroOrNegative(t *testing.T) {
	for _, limit := range []int{0, -1, -100} {
		t.Run("limit", func(t *testing.T) {
			repo := &adminNotificationRepoStub{}
			svc := NewAdminNotificationService(repo)

			_, err := svc.ListUnread(context.Background(), 42, limit)
			require.NoError(t, err)
			require.Equal(t, 1, repo.listUnreadCalled)
			require.Equal(t, int64(42), repo.listUnreadUserID)
			require.Equal(t, adminNotificationDefaultLimit, repo.listUnreadLimit, "limit<=0 should use default %d", adminNotificationDefaultLimit)
		})
	}
}

func TestAdminNotificationServiceListUnreadCapsLimitAtMax(t *testing.T) {
	repo := &adminNotificationRepoStub{}
	svc := NewAdminNotificationService(repo)

	_, err := svc.ListUnread(context.Background(), 1, 1000)
	require.NoError(t, err)
	require.Equal(t, adminNotificationMaxLimit, repo.listUnreadLimit, "limit>max should be capped at %d", adminNotificationMaxLimit)
}

func TestAdminNotificationServiceListUnreadKeepsInBoundsLimit(t *testing.T) {
	repo := &adminNotificationRepoStub{}
	svc := NewAdminNotificationService(repo)

	_, err := svc.ListUnread(context.Background(), 1, 25)
	require.NoError(t, err)
	require.Equal(t, 25, repo.listUnreadLimit)
}

func TestAdminNotificationServiceListUnreadReturnsRepoResult(t *testing.T) {
	expected := []*dbent.AdminNotification{{ID: 1}, {ID: 2}}
	repo := &adminNotificationRepoStub{listUnreadResult: expected}
	svc := NewAdminNotificationService(repo)

	got, err := svc.ListUnread(context.Background(), 7, 10)
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

func TestAdminNotificationServiceListUnreadWrapsRepoError(t *testing.T) {
	repoErr := errors.New("boom")
	repo := &adminNotificationRepoStub{listUnreadErr: repoErr}
	svc := NewAdminNotificationService(repo)

	_, err := svc.ListUnread(context.Background(), 1, 10)
	require.ErrorIs(t, err, repoErr)
}

func TestAdminNotificationServiceListUnreadNilBecomesEmptySlice(t *testing.T) {
	// stub 强制返回 nil（模拟 repo 未兜底的边界）→ service 必须兜底成空切片，满足前端 [] 规范
	repo := &adminNotificationRepoStub{forceNilListUnread: true}
	svc := NewAdminNotificationService(repo)

	got, err := svc.ListUnread(context.Background(), 1, 10)
	require.NoError(t, err)
	require.NotNil(t, got, "nil from repo must be normalized to empty slice")
	require.Len(t, got, 0)
}

// ====================================================================
// CountUnread
// ====================================================================

func TestAdminNotificationServiceCountUnreadDelegatesToRepo(t *testing.T) {
	repo := &adminNotificationRepoStub{countUnreadResult: 7}
	svc := NewAdminNotificationService(repo)

	got, err := svc.CountUnread(context.Background(), 99)
	require.NoError(t, err)
	require.Equal(t, int64(7), got)
	require.Equal(t, 1, repo.countUnreadCalled)
	require.Equal(t, int64(99), repo.countUnreadUserID)
}

func TestAdminNotificationServiceCountUnreadWrapsRepoError(t *testing.T) {
	repoErr := errors.New("count failed")
	repo := &adminNotificationRepoStub{countUnreadErr: repoErr}
	svc := NewAdminNotificationService(repo)

	_, err := svc.CountUnread(context.Background(), 1)
	require.ErrorIs(t, err, repoErr)
}

// ====================================================================
// MarkRead
// ====================================================================

func TestAdminNotificationServiceMarkReadDelegatesWithFreshTimestamp(t *testing.T) {
	repo := &adminNotificationRepoStub{}
	svc := NewAdminNotificationService(repo)

	before := time.Now()
	err := svc.MarkRead(context.Background(), 5, 42)
	require.NoError(t, err)
	after := time.Now()

	require.Equal(t, 1, repo.markReadCalled)
	require.Equal(t, int64(42), repo.markReadNotificationID, "notificationID should be passed as 2nd arg to repo")
	require.Equal(t, int64(5), repo.markReadUserID, "userID should be passed as 1st arg to repo")
	// service 应传一个 now-ish 时间戳
	require.True(t, !repo.markReadReadAt.Before(before) && !repo.markReadReadAt.After(after), "readAt should be ~time.Now()")
}

func TestAdminNotificationServiceMarkReadWrapsRepoError(t *testing.T) {
	repoErr := errors.New("insert failed")
	repo := &adminNotificationRepoStub{markReadErr: repoErr}
	svc := NewAdminNotificationService(repo)

	err := svc.MarkRead(context.Background(), 1, 1)
	require.ErrorIs(t, err, repoErr)
}

// ====================================================================
// MarkAllRead
// ====================================================================

func TestAdminNotificationServiceMarkAllReadDelegatesToRepo(t *testing.T) {
	repo := &adminNotificationRepoStub{}
	svc := NewAdminNotificationService(repo)

	before := time.Now()
	err := svc.MarkAllRead(context.Background(), 88)
	require.NoError(t, err)
	after := time.Now()

	require.Equal(t, 1, repo.markAllReadCalled)
	require.Equal(t, int64(88), repo.markAllReadUserID)
	require.True(t, !repo.markAllReadReadAt.Before(before) && !repo.markAllReadReadAt.After(after))
}

func TestAdminNotificationServiceMarkAllReadWrapsRepoError(t *testing.T) {
	repoErr := errors.New("bulk insert failed")
	repo := &adminNotificationRepoStub{markAllReadErr: repoErr}
	svc := NewAdminNotificationService(repo)

	err := svc.MarkAllRead(context.Background(), 1)
	require.ErrorIs(t, err, repoErr)
}

// ====================================================================
// ListAll
// ====================================================================

func TestAdminNotificationServiceListAllDelegatesToRepo(t *testing.T) {
	items := []*dbent.AdminNotification{{ID: 1}, {ID: 2}}
	page := &pagination.PaginationResult{Total: 2}
	repo := &adminNotificationRepoStub{listAllResult: items, listAllPage: page}
	svc := NewAdminNotificationService(repo)

	params := pagination.PaginationParams{Page: 1, PageSize: 10}
	got, gotPage, err := svc.ListAll(context.Background(), params)
	require.NoError(t, err)
	require.Equal(t, 1, repo.listAllCalled)
	require.Equal(t, items, got)
	require.Equal(t, page, gotPage)
}

func TestAdminNotificationServiceListAllWrapsRepoError(t *testing.T) {
	repoErr := errors.New("query failed")
	repo := &adminNotificationRepoStub{listAllErr: repoErr}
	svc := NewAdminNotificationService(repo)

	_, _, err := svc.ListAll(context.Background(), pagination.PaginationParams{})
	require.ErrorIs(t, err, repoErr)
}
