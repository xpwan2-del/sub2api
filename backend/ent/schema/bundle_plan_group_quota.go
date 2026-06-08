package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type BundlePlanGroupQuota struct {
	ent.Schema
}

func (BundlePlanGroupQuota) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_plan_group_quotas"},
	}
}

func (BundlePlanGroupQuota) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("plan_id").Comment("→ BundlePlan"),
		field.Int64("group_id").Comment("→ Group（复用现有 Group）"),
		field.String("quota_scope").Default("platform").Comment("额度粒度: platform/model"),
		field.String("model_pattern").Default("").Comment("仅 model 级别生效，glob 模式"),
		field.Float("daily_limit_usd").Default(0).Comment("日额度（0=不限）"),
		field.Float("weekly_limit_usd").Default(0).Comment("周额度（0=不限）"),
		field.Float("monthly_limit_usd").Default(0).Comment("月额度（0=不限）"),
	}
}

func (BundlePlanGroupQuota) Edges() []ent.Edge {
	return nil
}

func (BundlePlanGroupQuota) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("plan_id", "group_id"),
	}
}
