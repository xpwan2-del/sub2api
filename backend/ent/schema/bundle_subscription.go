// BundleSubscription 套餐订阅实例 Schema
// 记录用户购买的套餐订阅实例，包含订阅状态、生效/到期时间、
// 以及购买时的并发/RPM 限制快照（不随计划修改而变化）。
// 使用 SoftDeleteMixin 支持软删除。

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

// BundleSubscription 定义用户套餐订阅实例
type BundleSubscription struct {
	ent.Schema
}

// Annotations 指定数据库表名
func (BundleSubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "bundle_subscriptions"},
	}
}

// Mixin 引入时间戳和软删除混入
func (BundleSubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

// Fields 定义订阅实例字段，含状态、时间范围和来源
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

// Edges 暂无关联边
func (BundleSubscription) Edges() []ent.Edge {
	return nil
}

// Indexes 定义用户+状态+到期时间、计划ID 的查询索引
func (BundleSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status", "expires_at"),
		index.Fields("plan_id"),
	}
}
