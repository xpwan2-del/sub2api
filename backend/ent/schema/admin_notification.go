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

// AdminNotification 定义管理员通知实体 schema。
//
// 系统向管理员推送的通知（如价格变动告警、系统事件等）。
// 支持分级（info/warning/critical）、关联链接与多实体关联ID。
// content 字段支持 Markdown 渲染。
//
// 删除策略：硬删除（已读记录通过外键级联删除）
type AdminNotification struct {
	ent.Schema
}

func (AdminNotification) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "admin_notifications"},
	}
}

func (AdminNotification) Fields() []ent.Field {
	return []ent.Field{
		field.String("type").
			MaxLen(40).
			Default("system").
			Comment("通知类型: system / price_change / ops_alert 等"),
		field.String("title").
			MaxLen(200).
			NotEmpty().
			Comment("通知标题"),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("通知内容（支持 Markdown）"),
		field.String("severity").
			MaxLen(20).
			Default("info").
			Comment("严重级别: info / warning / critical"),
		field.String("target_link").
			MaxLen(500).
			Optional().
			Comment("关联跳转链接（可空）"),
		field.JSON("related_ids", []int64{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("关联实体ID列表（JSON 数组）"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (AdminNotification) Edges() []ent.Edge {
	return []ent.Edge{
		// reads: 该通知的已读记录
		edge.To("reads", AdminNotificationRead.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (AdminNotification) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("severity"),
		index.Fields("created_at"),
	}
}
