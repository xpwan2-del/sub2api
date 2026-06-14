package service

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	// ErrAdminNotificationNotFound 当通知不存在时返回。
	ErrAdminNotificationNotFound = infraerrors.NotFound("ADMIN_NOTIFICATION_NOT_FOUND", "admin notification not found")
)

// AdminNotificationRepository 封装管理员通知（admin_notifications）及其已读状态
// （admin_notification_reads）两张表的持久化操作。
//
// DTO 直接使用 ent 生成的实体类型 *dbent.AdminNotification，因为这些表是纯数据载体，
// 无中间业务转换需求（与 UpstreamPriceRepository 保持一致的范式）。
type AdminNotificationRepository interface {
	// Create 插入一条新通知，回写生成的 ID 与时间戳。
	Create(ctx context.Context, n *dbent.AdminNotification) error

	// ListUnreadByUser 返回该 admin 用户未读的通知（即 admin_notification_reads 中
	// 不存在 (notification_id=n.id, user_id=userID) 记录的通知）。
	// 按 created_at DESC 排序，limit 控制条数。空结果返回空切片（非 nil）。
	ListUnreadByUser(ctx context.Context, userID int64, limit int) ([]*dbent.AdminNotification, error)

	// CountUnreadByUser 统计该 admin 用户未读通知数量。
	CountUnreadByUser(ctx context.Context, userID int64) (int64, error)

	// MarkRead 标记某通知对该用户已读。幂等：重复标记不报错、不重复插入。
	MarkRead(ctx context.Context, notificationID, userID int64, readAt time.Time) error

	// MarkAllRead 标记该用户所有未读通知为已读（批量幂等）。
	MarkAllRead(ctx context.Context, userID int64, readAt time.Time) error

	// ListAll 分页返回全部通知（管理后台用），按 created_at DESC。
	ListAll(ctx context.Context, params pagination.PaginationParams) ([]*dbent.AdminNotification, *pagination.PaginationResult, error)
}
