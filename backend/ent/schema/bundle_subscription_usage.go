// BundleSubscriptionUsage 套餐订阅用量跟踪 Schema
// 跟踪每个套餐订阅实例在各渠道组上的日/周/月实际用量（USD），
// 配合时间窗口实现按周期重置的用量统计。

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"time"
)

// BundleSubscriptionUsage 定义套餐订阅的用量跟踪记录
type BundleSubscriptionUsage struct {
	ent.Schema
}

// Annotations 指定数据库表名
func (BundleSubscriptionUsage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_subscription_usages"},
	}
}

// Fields 定义用量字段，包括日/周/月用量和对应的窗口起始时间
func (BundleSubscriptionUsage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("bundle_subscription_id").Comment("→ BundleSubscription"),
		field.Int64("group_id").Comment("→ Group"),
		field.String("model_pattern").Default("").Comment("空=平台级，有值=模型级"),
		field.Float("daily_usage_usd").Default(0).Comment("当日已用"),
		field.Time("daily_window_start").Default(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("日窗口起点"),
		field.Float("weekly_usage_usd").Default(0).Comment("当周已用"),
		field.Time("weekly_window_start").Default(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("周窗口起点"),
		field.Float("monthly_usage_usd").Default(0).Comment("当月已用"),
		field.Time("monthly_window_start").Default(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("月窗口起点"),
	}
}

// Edges 暂无关联边
func (BundleSubscriptionUsage) Edges() []ent.Edge {
	return nil
}

// Indexes 定义订阅ID+渠道组ID 的联合索引
func (BundleSubscriptionUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("bundle_subscription_id", "group_id"),
	}
}
