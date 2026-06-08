package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
)

type BundleSubscription struct {
	ent.Schema
}

func (BundleSubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_subscriptions"},
	}
}

func (BundleSubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (BundleSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id").Comment("→ User"),
		field.Int64("plan_id").Comment("→ BundlePlan"),
		field.String("status").Default("active").Comment("active/expired/revoked"),
		field.Time("starts_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("生效时间"),
		field.Time("expires_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("到期时间"),
		field.Int("concurrency_limit").Default(0).NonNegative().Comment("快照：并发上限"),
		field.Int("rpm_limit").Default(0).NonNegative().Comment("快照：RPM上限"),
		field.String("source").Default("purchase").Comment("来源: purchase/redeem/admin_assign"),
	}
}

func (BundleSubscription) Edges() []ent.Edge {
	return nil
}

func (BundleSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status", "expires_at"),
		index.Fields("plan_id"),
	}
}
