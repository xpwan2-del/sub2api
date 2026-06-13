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

// UpstreamModelPrice 定义上游模型价格实体 schema。
//
// 记录某来源下单个模型的最新价格快照（输入/输出/缓存/图片/单次计费等）。
// 每个 (source_id, model_name) 唯一。
type UpstreamModelPrice struct {
	ent.Schema
}

func (UpstreamModelPrice) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "upstream_model_prices"},
	}
}

func (UpstreamModelPrice) Fields() []ent.Field {
	return []ent.Field{
		// source_id: 所属价格来源（外键，由 edge 维护）
		field.Int64("source_id"),
		field.String("model_name").
			NotEmpty().
			Comment("上游模型名"),
		field.String("local_model_name").
			Optional().
			Comment("映射后的本地模型名（可空，表示使用上游原名）"),
		field.Float("input_price").
			Comment("输入价格（per-token USD，与 channel_model_pricing / LiteLLM input_cost_per_token 一致）"),
		field.Float("output_price").
			Comment("输出价格（per-token USD，与 channel_model_pricing / LiteLLM output_cost_per_token 一致）"),
		field.Float("cache_write_price").
			Optional().
			Nillable().
			Comment("缓存写入价格（可空）"),
		field.Float("cache_read_price").
			Optional().
			Nillable().
			Comment("缓存读取价格（可空）"),
		field.Float("image_output_price").
			Optional().
			Nillable().
			Comment("图片输出价格（可空）"),
		field.Float("per_request_price").
			Optional().
			Nillable().
			Comment("单次请求价格（可空）"),
		field.String("currency").
			MaxLen(10).
			Default("USD").
			Comment("货币单位"),
		field.JSON("raw_payload", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("上游原始响应载荷（JSON）"),
		field.Time("fetched_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("价格抓取时间"),
	}
}

func (UpstreamModelPrice) Edges() []ent.Edge {
	return []ent.Edge{
		// source: 所属价格来源（多对一）
		edge.From("source", UpstreamPriceSource.Type).
			Ref("prices").
			Field("source_id").
			Unique().
			Required(),
	}
}

func (UpstreamModelPrice) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_id", "model_name").Unique(),
	}
}
