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

type BundleSubscriptionUsage struct {
	ent.Schema
}

func (BundleSubscriptionUsage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_subscription_usages"},
	}
}

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

func (BundleSubscriptionUsage) Edges() []ent.Edge {
	return nil
}

func (BundleSubscriptionUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("bundle_subscription_id", "group_id"),
	}
}
