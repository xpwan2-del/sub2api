package repository

import (
	"context"
	"reflect"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// fakeCatalogRepo 实现 repository.ModelCatalogRepo，供 adapter 单测（无 DB）。
type fakeCatalogRepo struct {
	// getByKeys 的 key 故意用任意串，验证 adapter 不透传、按字段重建 key。
	getByKeys  map[string]*ModelCatalogDisplay
	listResult []*ModelCatalogDisplay
	upsertSeen []ModelKey
	batchSeen  []*ModelCatalogDisplay
}

func (f *fakeCatalogRepo) GetByModelKeys(_ context.Context, _ []ModelKey) (map[string]*ModelCatalogDisplay, error) {
	return f.getByKeys, nil
}

func (f *fakeCatalogRepo) UpsertMissing(_ context.Context, keys []ModelKey) error {
	f.upsertSeen = keys
	return nil
}

func (f *fakeCatalogRepo) ListAll(_ context.Context) ([]*ModelCatalogDisplay, error) {
	return f.listResult, nil
}

func (f *fakeCatalogRepo) BatchUpsert(_ context.Context, cfgs []*ModelCatalogDisplay) error {
	f.batchSeen = cfgs
	return nil
}

// TestModelCatalogServiceAdapter_GetByModelKeys_RebuildsKey 验证 adapter 用记录的
// Platform/ModelName 字段按 modelKey 算法重建 map key，而非透传底层 repo 的内部 key。
func TestModelCatalogServiceAdapter_GetByModelKeys_RebuildsKey(t *testing.T) {
	rec := &ModelCatalogDisplay{Platform: "openai", ModelName: "gpt-4", Pinned: true, CustomTags: []string{"vision"}}
	// 底层 repo 返回的 map 用一个与字段无关的伪 key。
	fake := &fakeCatalogRepo{getByKeys: map[string]*ModelCatalogDisplay{"BOGUS_KEY": rec}}

	adapter := NewModelCatalogServiceAdapter(fake)
	out, err := adapter.GetByModelKeys(context.Background(), []service.ModelKey{{Platform: "openai", ModelName: "gpt-4"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	wantKey := modelKey("openai", "gpt-4")
	got, ok := out[wantKey]
	if !ok || got == nil {
		t.Fatalf("expected field-derived key %q, got map keys: %v", wantKey, mapKeys(out))
	}
	if _, leaked := out["BOGUS_KEY"]; leaked {
		t.Fatal("adapter must NOT passthrough underlying repo's internal key")
	}
	if got.Platform != "openai" || got.ModelName != "gpt-4" || !got.Pinned || !reflect.DeepEqual(got.CustomTags, []string{"vision"}) {
		t.Fatalf("DTO conversion wrong: %+v", got)
	}
}

// TestModelCatalogServiceAdapter_ListAll_ConvertsDTO 验证 ListAll 的 DTO 转换。
func TestModelCatalogServiceAdapter_ListAll_ConvertsDTO(t *testing.T) {
	fake := &fakeCatalogRepo{listResult: []*ModelCatalogDisplay{
		{Platform: "anthropic", ModelName: "claude", Hidden: true},
		{Platform: "google", ModelName: "gemini"},
	}}
	adapter := NewModelCatalogServiceAdapter(fake)

	out, err := adapter.ListAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(out))
	}
	if out[0].Platform != "anthropic" || !out[0].Hidden {
		t.Fatalf("row0 conversion wrong: %+v", out[0])
	}
}

// TestModelCatalogServiceAdapter_BatchSave_RoundTrip 验证 UpsertMissing 的 key 转换
// 与 BatchUpsert 的 service→repo DTO 转换。
func TestModelCatalogServiceAdapter_BatchSave_RoundTrip(t *testing.T) {
	fake := &fakeCatalogRepo{}
	adapter := NewModelCatalogServiceAdapter(fake)

	keys := []service.ModelKey{{Platform: "p", ModelName: "a"}, {Platform: "p", ModelName: "b"}}
	if err := adapter.UpsertMissing(context.Background(), keys); err != nil {
		t.Fatalf("UpsertMissing err: %v", err)
	}
	wantRepoKeys := []ModelKey{{Platform: "p", ModelName: "a"}, {Platform: "p", ModelName: "b"}}
	if !reflect.DeepEqual(fake.upsertSeen, wantRepoKeys) {
		t.Fatalf("UpsertMissing key conversion wrong: got %v want %v", fake.upsertSeen, wantRepoKeys)
	}

	cfgs := []*service.ModelCatalogDisplay{{Platform: "p", ModelName: "m", Pinned: true, SortWeight: 7}}
	if err := adapter.BatchUpsert(context.Background(), cfgs); err != nil {
		t.Fatalf("BatchUpsert err: %v", err)
	}
	if len(fake.batchSeen) != 1 || fake.batchSeen[0].Platform != "p" || !fake.batchSeen[0].Pinned || fake.batchSeen[0].SortWeight != 7 {
		t.Fatalf("BatchUpsert DTO conversion wrong: %+v", fake.batchSeen)
	}
}

func mapKeys(m map[string]*service.ModelCatalogDisplay) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
