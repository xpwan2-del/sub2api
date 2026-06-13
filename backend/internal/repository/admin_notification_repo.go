package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/adminnotification"
	"github.com/Wei-Shaw/sub2api/ent/adminnotificationread"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// adminNotificationRepository 实现 service.AdminNotificationRepository，
// 封装 admin_notifications / admin_notification_reads 两张表的 CRUD。
//
// DTO 直接使用 ent 生成的实体类型（*dbent.AdminNotification），
// 与 upstream_price_repo / announcement_read_repo 保持一致的范式。
type adminNotificationRepository struct {
	client *dbent.Client
}

// NewAdminNotificationRepository 构造 AdminNotificationRepository 的 ent 实现。
func NewAdminNotificationRepository(client *dbent.Client) service.AdminNotificationRepository {
	return &adminNotificationRepository{client: client}
}

// ============================================================
// notification
// ============================================================

// Create 插入一条新通知，回写生成的 ID / 时间戳。
func (r *adminNotificationRepository) Create(ctx context.Context, n *dbent.AdminNotification) error {
	if n == nil {
		return service.ErrAdminNotificationNotFound
	}
	client := clientFromContext(ctx, r.client)

	builder := client.AdminNotification.Create().
		SetType(n.Type).
		SetTitle(n.Title).
		SetContent(n.Content).
		SetSeverity(n.Severity)

	if n.TargetLink != "" {
		builder.SetTargetLink(n.TargetLink)
	}
	if n.RelatedIds != nil {
		builder.SetRelatedIds(n.RelatedIds)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, nil)
	}

	applyNotificationFields(n, created)
	return nil
}

// ListUnreadByUser 返回该 admin 用户未读的通知。
//
// 实现方式：通过 ent 的 edge predicate Not(HasReadsWith(UserIDEQ(userID)))
// 生成 NOT EXISTS 子查询（admin_notification_reads 中无该 user 对该 notification 的
// read 记录），与 announcement_read 现有查询保持一致的反向"未读"语义。
//
// 按 created_at DESC（同 created_at 再按 id DESC 稳定排序），limit 控制条数。
// 空结果返回空切片（非 nil），满足前端"列表空必须返回 []"的规范。
func (r *adminNotificationRepository) ListUnreadByUser(
	ctx context.Context,
	userID int64,
	limit int,
) ([]*dbent.AdminNotification, error) {
	q := r.client.AdminNotification.Query().Where(
		adminnotification.Not(
			adminnotification.HasReadsWith(
				adminnotificationread.UserIDEQ(userID),
			),
		),
	).Order(
		dbent.Desc(adminnotification.FieldCreatedAt),
		dbent.Desc(adminnotification.FieldID),
	)

	if limit > 0 {
		q = q.Limit(limit)
	}

	items, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return make([]*dbent.AdminNotification, 0), nil
	}
	return items, nil
}

// CountUnreadByUser 统计该 admin 用户未读通知数量。
// 复用与 ListUnreadByUser 相同的 NOT EXISTS 谓词。
func (r *adminNotificationRepository) CountUnreadByUser(
	ctx context.Context,
	userID int64,
) (int64, error) {
	count, err := r.client.AdminNotification.Query().Where(
		adminnotification.Not(
			adminnotification.HasReadsWith(
				adminnotificationread.UserIDEQ(userID),
			),
		),
	).Count(ctx)
	if err != nil {
		return 0, err
	}
	return int64(count), nil
}

// MarkRead 标记某通知对该用户已读。幂等：重复标记不报错、不重复插入。
//
// 通过 (notification_id, user_id) 唯一索引上的 OnConflict DoNothing 实现：
// - 首次插入：写入 read 记录。
// - 已存在：DoNothing 不报错（sqlite 驱动此时返回 sql.ErrNoRows，由 isSQLNoRowsError 吸收）。
//
// 与 announcement_read_repo.MarkRead 保持一致的幂等处理。
func (r *adminNotificationRepository) MarkRead(
	ctx context.Context,
	notificationID, userID int64,
	readAt time.Time,
) error {
	client := clientFromContext(ctx, r.client)
	err := client.AdminNotificationRead.Create().
		SetNotificationID(notificationID).
		SetUserID(userID).
		SetReadAt(readAt).
		OnConflictColumns(
			adminnotificationread.FieldNotificationID,
			adminnotificationread.FieldUserID,
		).
		DoNothing().
		Exec(ctx)
	if isSQLNoRowsError(err) {
		return nil
	}
	return translatePersistenceError(err, nil, nil)
}

// MarkAllRead 标记该用户所有未读通知为已读（批量、幂等）。
//
// 先查出该用户所有未读的 notification_id 列表，再用 CreateBulk + OnConflict DoNothing
// 批量幂等插入 read 记录。无未读时不做任何写入，直接返回。
func (r *adminNotificationRepository) MarkAllRead(
	ctx context.Context,
	userID int64,
	readAt time.Time,
) error {
	client := clientFromContext(ctx, r.client)

	// 1) 查出该用户所有未读的 notification_id
	unread, err := r.client.AdminNotification.Query().Where(
		adminnotification.Not(
			adminnotification.HasReadsWith(
				adminnotificationread.UserIDEQ(userID),
			),
		),
	).All(ctx)
	if err != nil {
		return err
	}
	if len(unread) == 0 {
		return nil
	}

	// 2) 批量幂等插入 read 记录（OnConflict DoNothing 在 bulk 层应用）
	builders := make([]*dbent.AdminNotificationReadCreate, 0, len(unread))
	for _, n := range unread {
		builders = append(builders, client.AdminNotificationRead.Create().
			SetNotificationID(n.ID).
			SetUserID(userID).
			SetReadAt(readAt))
	}

	// DoNothing 在已存在时 sqlite 仍可能返回 ErrNoRows，按单条 MarkRead 的方式吸收。
	if err := client.AdminNotificationRead.CreateBulk(builders...).
		OnConflictColumns(
			adminnotificationread.FieldNotificationID,
			adminnotificationread.FieldUserID,
		).
		DoNothing().
		Exec(ctx); err != nil {
		if isSQLNoRowsError(err) {
			return nil
		}
		return translatePersistenceError(err, nil, nil)
	}
	return nil
}

// ListAll 分页返回全部通知，按 created_at DESC（同 created_at 再按 id DESC 稳定排序）。
// 空结果返回空切片（非 nil）。
func (r *adminNotificationRepository) ListAll(
	ctx context.Context,
	params pagination.PaginationParams,
) ([]*dbent.AdminNotification, *pagination.PaginationResult, error) {
	q := r.client.AdminNotification.Query()

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	items, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(
			dbent.Desc(adminnotification.FieldCreatedAt),
			dbent.Desc(adminnotification.FieldID),
		).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	if items == nil {
		items = make([]*dbent.AdminNotification, 0)
	}
	return items, paginationResultFromTotal(int64(total), params), nil
}

// ============================================================
// helpers
// ============================================================

// applyNotificationFields 将 ent 写回结果的关键字段同步回入参 DTO（ID 与时间戳）。
func applyNotificationFields(dst *dbent.AdminNotification, src *dbent.AdminNotification) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
}
