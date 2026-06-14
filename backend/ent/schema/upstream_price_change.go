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

// UpstreamPriceChange 定义上游价格变动记录实体 schema。
//
// 当 DiffEngine 检测到某来源某模型的价格发生变化时，写入一条记录。
// 记录包含变动类型、前后价格、百分比变动、建议价格及处理状态（pending/applied/dismissed）。
type UpstreamPriceChange struct {
	ent.Schema
}

func (UpstreamPriceChange) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "upstream_price_changes"},
	}
}

func (UpstreamPriceChange) Fields() []ent.Field {
	return []ent.Field{
		// source_id: 所属价格来源（外键，由 edge 维护）
		field.Int64("source_id"),
		field.String("model_name").
			NotEmpty().
			Comment("发生变动的上游模型名"),
		field.String("local_model_name").
			Optional().
			Comment("映射后的本地模型名（可空）"),
		field.String("change_type").
			MaxLen(20).
			Comment("变动类型: added / removed / price_change"),
		field.Float("prev_input_price").
			Optional().
			Nillable().
			Comment("变动前输入价格（新增模型时为空）"),
		field.Float("prev_output_price").
			Optional().
			Nillable().
			Comment("变动前输出价格（新增模型时为空）"),
		field.Float("curr_input_price").
			Comment("当前输入价格"),
		field.Float("curr_output_price").
			Comment("当前输出价格"),
		field.Float("input_delta_pct").
			Default(0).
			Comment("输入价格变动百分比"),
		field.Float("output_delta_pct").
			Default(0).
			Comment("输出价格变动百分比"),
		field.Time("detected_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("检测到变动的时间"),
		field.Bool("notified").
			Default(false).
			Comment("是否已发送告警通知"),
		field.String("status").
			MaxLen(20).
			Default("pending").
			Comment("处理状态: pending / applied / dismissed"),
		field.Float("suggested_input_price").
			Default(0).
			Comment("建议输入价格（SuggestionCalculator 生成）"),
		field.Float("suggested_output_price").
			Default(0).
			Comment("建议输出价格（SuggestionCalculator 生成）"),
		field.Float("suggested_multiplier").
			Optional().
			Nillable().
			Comment("建议计费倍率（可空）"),
		field.Time("applied_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("应用建议的时间"),
		field.Int64("applied_by").
			Optional().
			Nillable().
			Comment("操作人用户ID"),
		field.String("applied_target").
			MaxLen(30).
			Optional().
			Comment("应用目标类型: account / group / model_config"),
		field.Int64("applied_target_id").
			Default(0).
			Comment("应用目标记录ID"),
		field.Float("applied_prev_input_price").
			Optional().
			Nillable().
			Comment("应用前 channel 该模型的实际输入单价快照（用于撤销回滚）"),
		field.Float("applied_prev_output_price").
			Optional().
			Nillable().
			Comment("应用前 channel 该模型的实际输出单价快照（用于撤销回滚）"),
		field.Int64("applied_channel_id").
			Optional().
			Nillable().
			Comment("应用时实际写入单价的 channel_id（撤销回滚锚点）"),
		field.Float("prev_multiplier").
			Optional().
			Nillable().
			Comment("应用前 group 的实际倍率快照（仅 lock_price，用于撤销回滚）"),
		field.Time("reverted_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("撤销应用的时间"),
		field.Int64("reverted_by").
			Optional().
			Nillable().
			Comment("执行撤销的管理员用户ID"),
		field.Bytes("applied_channels_snapshot").
			Optional().
			Nillable().
			Comment("批量 apply 时记录所有命中 channel 的 prev 价快照（JSON），供撤销遍历恢复"),
	}
}

func (UpstreamPriceChange) Edges() []ent.Edge {
	return []ent.Edge{
		// source: 触发变动的价格来源（多对一）
		edge.From("source", UpstreamPriceSource.Type).
			Ref("changes").
			Field("source_id").
			Unique().
			Required(),
	}
}

func (UpstreamPriceChange) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("source_id", "detected_at"),
	}
}
