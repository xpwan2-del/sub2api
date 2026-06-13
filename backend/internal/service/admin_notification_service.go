package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	// ErrAdminNotificationTitleRequired 当通知标题为空时返回。
	ErrAdminNotificationTitleRequired = infraerrors.BadRequest("ADMIN_NOTIFICATION_TITLE_REQUIRED", "notification title is required")
	// ErrAdminNotificationContentRequired 当通知内容为空时返回。
	ErrAdminNotificationContentRequired = infraerrors.BadRequest("ADMIN_NOTIFICATION_CONTENT_REQUIRED", "notification content is required")
	// ErrAdminNotificationTypeRequired 当通知类型为空时返回。
	ErrAdminNotificationTypeRequired = infraerrors.BadRequest("ADMIN_NOTIFICATION_TYPE_REQUIRED", "notification type is required")
)

const (
	// adminNotificationDefaultSeverity 默认严重级别。
	adminNotificationDefaultSeverity = "info"
	// adminNotificationDefaultLimit ListUnread 默认条数。
	adminNotificationDefaultLimit = 50
	// adminNotificationMaxLimit ListUnread 最大条数。
	adminNotificationMaxLimit = 200
)

// AdminNotificationService 封装管理员通知的业务逻辑：创建、未读查询、已读标记、分页。
//
// 仅供 admin 后台使用，与面向终端用户的 AnnouncementService 解耦。
type AdminNotificationService struct {
	repo AdminNotificationRepository
}

// NewAdminNotificationService 构造 AdminNotificationService。
func NewAdminNotificationService(repo AdminNotificationRepository) *AdminNotificationService {
	return &AdminNotificationService{repo: repo}
}

// Create 创建一条 admin 通知（用于价格变动等系统事件）。
//
// type/title/content 必填；severity 为空时默认 "info"；relatedIDs 与 targetLink 可空。
// 写入成功后回写生成的 ID 与时间戳。
func (s *AdminNotificationService) Create(
	ctx context.Context,
	notifType, title, content, severity, targetLink string,
	relatedIDs []int64,
) (*dbent.AdminNotification, error) {
	t := strings.TrimSpace(notifType)
	if t == "" {
		return nil, ErrAdminNotificationTypeRequired
	}
	ttl := strings.TrimSpace(title)
	if ttl == "" {
		return nil, ErrAdminNotificationTitleRequired
	}
	c := strings.TrimSpace(content)
	if c == "" {
		return nil, ErrAdminNotificationContentRequired
	}

	sev := strings.TrimSpace(severity)
	if sev == "" {
		sev = adminNotificationDefaultSeverity
	}

	// 拷贝 relatedIDs，避免持有调用方切片底层数组的引用。
	var ids []int64
	if len(relatedIDs) > 0 {
		ids = make([]int64, len(relatedIDs))
		copy(ids, relatedIDs)
	}

	n := &dbent.AdminNotification{
		Type:       t,
		Title:      ttl,
		Content:    c,
		Severity:   sev,
		TargetLink: strings.TrimSpace(targetLink),
		RelatedIds: ids,
	}

	if err := s.repo.Create(ctx, n); err != nil {
		return nil, fmt.Errorf("create admin notification: %w", err)
	}
	return n, nil
}

// ListUnread 列出某 admin 的未读通知。
//
// limit<=0 时使用默认值（50）；limit>200 时截断为 200，避免一次拉取过多。
// 空结果返回空切片（非 nil），满足前端"列表空必须返回 []"规范。
func (s *AdminNotificationService) ListUnread(ctx context.Context, userID int64, limit int) ([]*dbent.AdminNotification, error) {
	if limit <= 0 {
		limit = adminNotificationDefaultLimit
	} else if limit > adminNotificationMaxLimit {
		limit = adminNotificationMaxLimit
	}
	items, err := s.repo.ListUnreadByUser(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list unread admin notifications: %w", err)
	}
	if items == nil {
		return make([]*dbent.AdminNotification, 0), nil
	}
	return items, nil
}

// CountUnread 统计某 admin 的未读通知数量。
func (s *AdminNotificationService) CountUnread(ctx context.Context, userID int64) (int64, error) {
	count, err := s.repo.CountUnreadByUser(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("count unread admin notifications: %w", err)
	}
	return count, nil
}

// MarkRead 标记单条已读（幂等，repo 已处理重复标记）。
func (s *AdminNotificationService) MarkRead(ctx context.Context, userID, notificationID int64) error {
	if err := s.repo.MarkRead(ctx, notificationID, userID, time.Now()); err != nil {
		return fmt.Errorf("mark admin notification read: %w", err)
	}
	return nil
}

// MarkAllRead 标记该用户所有未读通知为已读。
func (s *AdminNotificationService) MarkAllRead(ctx context.Context, userID int64) error {
	if err := s.repo.MarkAllRead(ctx, userID, time.Now()); err != nil {
		return fmt.Errorf("mark all admin notifications read: %w", err)
	}
	return nil
}

// ListAll 分页返回全部通知（管理后台用）。
func (s *AdminNotificationService) ListAll(
	ctx context.Context,
	params pagination.PaginationParams,
) ([]*dbent.AdminNotification, *pagination.PaginationResult, error) {
	items, result, err := s.repo.ListAll(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list admin notifications: %w", err)
	}
	if items == nil {
		items = make([]*dbent.AdminNotification, 0)
	}
	return items, result, nil
}
