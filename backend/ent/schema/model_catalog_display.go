package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelCatalogDisplay struct{ ent.Schema }

func (ModelCatalogDisplay) Fields() []ent.Field {
	return []ent.Field{
		field.String("platform").NotEmpty().MaxLen(64),
		field.String("model_name").NotEmpty().MaxLen(128),
		field.Bool("pinned").Default(false),
		field.Int("sort_weight").Default(0),
		field.JSON("custom_tags", []string{}).Default([]string{}),
		field.Time("featured_until").Optional().Nillable(),
		field.Bool("hidden").Default(false),
		field.Time("first_seen_at").Default(time.Now),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (ModelCatalogDisplay) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform", "model_name").Unique(),
	}
}
