// BundlePlan 套餐计划 Schema
// 定义可供用户购买的套餐计划（如 starter/pro/enterprise），
// 包含价格、有效期、并发/RPM 限制等核心属性。
// 每个套餐计划通过 BundlePlanGroupQuota 关联多个渠道组（Group）及其额度上限。

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

// BundlePlan 定义套餐计划数据库实体
type BundlePlan struct {
	ent.Schema
}

// Annotations 指定数据库表名
func (BundlePlan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_plans"},
	}
}

// Fields 定义套餐计划的所有字段
func (BundlePlan) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty().Comment("套餐名称"),
		field.String("description").Default("").Comment("套餐描述"),
		field.String("tier").NotEmpty().Comment("套餐层级: starter/pro/enterprise"),
		field.Float("price").Default(0).Comment("售价"),
		field.Float("original_price").Default(0).Comment("原价（划线价）"),
		field.String("currency").Default("USD").Comment("货币: USD/CNY"),
		field.Int("validity_days").Default(30).Positive().Comment("有效天数"),
		field.Int("concurrency_limit").Default(0).NonNegative().Comment("并发上限（0=不限）"),
		field.Int("rpm_limit").Default(0).NonNegative().Comment("RPM上限（0=不限）"),
		field.Strings("features").Optional().Comment("功能特性列表"),
		field.Bool("for_sale").Default(true).Comment("是否在售"),
		field.Int("sort_order").Default(0).NonNegative().Comment("排序"),
		field.String("status").Default("active").Comment("状态: active/disabled"),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("创建时间"),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Comment("更新时间"),
	}
}

// Edges 暂无关联边（通过 plan_id 外键手动关联）
func (BundlePlan) Edges() []ent.Edge {
	return nil
}

// Indexes 定义常用查询索引（状态+在售、层级）
func (BundlePlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "for_sale"),
		index.Fields("tier"),
	}
}
