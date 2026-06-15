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

// UpstreamPriceSource 定义上游价格来源实体 schema。
//
// 表示一个可定期同步价格的上游定价源（如 one-api / new-api / 官方 pricing 接口）。
// 记录同步配置、解析器类型、告警阈值及上次同步状态。
//
// 删除策略：硬删除（关联的 ModelPrice 通过外键级联删除）
type UpstreamPriceSource struct {
	ent.Schema
}

func (UpstreamPriceSource) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "upstream_price_sources"},
	}
}

func (UpstreamPriceSource) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			MaxLen(100).
			NotEmpty().
			Comment("来源名称"),
		field.String("base_url").
			MaxLen(500).
			NotEmpty().
			Comment("上游基础地址，如 https://api.example.com"),
		field.String("pricing_endpoint").
			MaxLen(500).
			Default("/api/pricing").
			Comment("价格接口路径"),
		// api_key: 上游 API 密钥。
		// 与 channel_monitor.go 的 api_key_encrypted 一致：
		// 使用 ent 内置 Sensitive() 标记，实际 AES-256-GCM 加解密在 service 层完成
		// （Sensitive 仅影响序列化/日志脱敏，不自动加解密）。
		field.String("api_key").
			MaxLen(500).
			Optional().
			Sensitive().
			Comment("AES-256-GCM 加密后的上游 API Key（service 层加解密）"),
		field.String("parser_type").
			MaxLen(30).
			Default("one_api").
			Comment("解析器类型: one_api / new_api / custom"),
		field.JSON("parser_config", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("解析器配置（JSON）"),
		field.JSON("model_alias_map", map[string]string{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}).
			Comment("上游模型名 → 本地模型名 映射"),
		field.Int("sync_interval_minutes").
			Default(360).
			Comment("同步间隔（分钟）"),
		field.Float("alert_threshold_pct").
			Default(0).
			Comment("价格变动告警阈值（百分比，0=不告警）"),
		field.Int("cooldown_minutes").
			Default(60).
			Comment("告警冷却时间（分钟）"),
		field.Bool("enabled").
			Default(true).
			Comment("是否启用"),
		field.Time("last_sync_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Comment("上次同步时间"),
		field.String("last_sync_status").
			MaxLen(20).
			Default("").
			Comment("上次同步状态: success / failed / partial"),
		field.String("last_sync_error").
			MaxLen(1000).
			Optional().
			Comment("上次同步错误信息"),
		field.String("last_hash").
			MaxLen(128).
			Optional().
			Comment("上次同步内容的哈希值（用于变更检测）"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamPriceSource) Edges() []ent.Edge {
	return []ent.Edge{
		// prices: 该来源拉取到的全部模型价格
		edge.To("prices", UpstreamModelPrice.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		// changes: 该来源检测到的全部价格变动记录
		edge.To("changes", UpstreamPriceChange.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (UpstreamPriceSource) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
		index.Fields("last_sync_at"),
	}
}
