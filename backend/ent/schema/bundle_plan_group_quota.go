// BundlePlanGroupQuota 套餐计划-渠道组额度映射 Schema
// 定义套餐计划中每个渠道组（Group）的日/周/月额度上限。
// 支持 platform（平台级）和 model（模型级，通过 glob 匹配）两种额度粒度。

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// BundlePlanGroupQuota 定义套餐计划与渠道组之间的额度映射关系
type BundlePlanGroupQuota struct {
	ent.Schema
}

// Annotations 指定数据库表名
func (BundlePlanGroupQuota) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_plan_group_quotas"},
	}
}

// Fields 定义额度映射字段，包括额度粒度、模型匹配模式和日/周/月上限
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

// Edges 暂无关联边
func (BundlePlanGroupQuota) Edges() []ent.Edge {
	return nil
}

// Indexes 定义按计划ID+渠道组ID的联合索引
func (BundlePlanGroupQuota) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("plan_id", "group_id"),
	}
}
