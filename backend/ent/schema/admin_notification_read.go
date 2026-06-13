package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AdminNotificationRead 定义管理员通知已读记录实体 schema。
//
// 记录某管理员用户对某通知的已读状态（首次已读时间）。
// 每个 (notification_id, user_id) 唯一。
type AdminNotificationRead struct {
	ent.Schema
}

func (AdminNotificationRead) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "admin_notification_reads"},
	}
}

func (AdminNotificationRead) Fields() []ent.Field {
	return []ent.Field{
		// notification_id: 所属通知（外键，由 edge 维护）
		field.Int64("notification_id"),
		field.Int64("user_id").
			Comment("已读用户ID（管理员）"),
		field.Time("read_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("用户首次已读时间"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (AdminNotificationRead) Edges() []ent.Edge {
	return []ent.Edge{
		// notification: 所属通知（多对一）
		edge.From("notification", AdminNotification.Type).
			Ref("reads").
			Field("notification_id").
			Unique().
			Required(),
	}
}

func (AdminNotificationRead) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("notification_id"),
		index.Fields("user_id"),
		index.Fields("notification_id", "user_id").Unique(),
	}
}
