package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// stubCatalogRepo 实现 modelCatalogRepo（service 包内 interface），供单测。
// 不依赖 repository 包，避免 depguard service→repository 限制。
type stubCatalogRepo struct {
	// getByModelKeys 预填配置，key 须用 modelCatalogMapKey(platform, modelName)。
	getByModelKeys map[string]*modelCatalogDisplay
	getErr         error
	listResult     []*modelCatalogDisplay
	listErr        error
	upsertErr      error
	batchErr       error
	// 捕获调用入参，供断言。
	upsertSeen []ModelKey
	batchSeen  []*modelCatalogDisplay
}

func (s *stubCatalogRepo) GetByModelKeys(_ context.Context, keys []ModelKey) (map[string]*modelCatalogDisplay, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	out := make(map[string]*modelCatalogDisplay, len(keys))
	for _, k := range keys {
		mk := modelCatalogMapKey(k.Platform, k.ModelName)
		if c, ok := s.getByModelKeys[mk]; ok {
			out[mk] = c
		}
	}
	return out, nil
}

func (s *stubCatalogRepo) UpsertMissing(_ context.Context, keys []ModelKey) error {
	s.upsertSeen = keys
	return s.upsertErr
}

func (s *stubCatalogRepo) ListAll(_ context.Context) ([]*modelCatalogDisplay, error) {
	return s.listResult, s.listErr
}

func (s *stubCatalogRepo) BatchUpsert(_ context.Context, cfgs []*modelCatalogDisplay) error {
	s.batchSeen = cfgs
	return s.batchErr
}

// stubCatalogSettings 实现 modelCatalogSettings，供单测。
type stubCatalogSettings struct {
	enabled bool
	newDays int
}

func (s stubCatalogSettings) IsModelCatalogOpsEnabled(_ context.Context) bool   { return s.enabled }
func (s stubCatalogSettings) GetModelCatalogNewModelDays(_ context.Context) int { return s.newDays }

func newSvc(repo *stubCatalogRepo, enabled bool, newDays int) ModelCatalogService {
	return NewModelCatalogService(repo, stubCatalogSettings{enabled: enabled, newDays: newDays})
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func TestModelCatalogService_IsEnabled(t *testing.T) {
	if !newSvc(&stubCatalogRepo{}, true, 30).IsEnabled(context.Background()) {
		t.Fatal("IsEnabled=true expected when setting enabled")
	}
	if newSvc(&stubCatalogRepo{}, false, 30).IsEnabled(context.Background()) {
		t.Fatal("IsEnabled=false expected when setting disabled")
	}
}

func TestModelCatalogService_MergeDisplayConfig_HiddenFiltered(t *testing.T) {
	repo := &stubCatalogRepo{getByModelKeys: map[string]*modelCatalogDisplay{
		modelCatalogMapKey("openai", "gpt-4"):     {Platform: "openai", ModelName: "gpt-4", Hidden: true},
		modelCatalogMapKey("anthropic", "claude"): {Platform: "anthropic", ModelName: "claude"},
	}}
	svc := newSvc(repo, true, 30)

	items := []CatalogItem{
		{Platform: "openai", ModelName: "gpt-4", Name: "GPT-4"},
		{Platform: "anthropic", ModelName: "claude", Name: "Claude"},
	}
	out, err := svc.MergeDisplayConfig(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("hidden item must be filtered, got %d items", len(out))
	}
	if out[0].ModelName != "claude" {
		t.Fatalf("expected claude to remain, got %s", out[0].ModelName)
	}
}

func TestModelCatalogService_MergeDisplayConfig_FeaturedExpiry(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)
	past := now.Add(-1 * time.Hour)

	repo := &stubCatalogRepo{getByModelKeys: map[string]*modelCatalogDisplay{
		modelCatalogMapKey("p", "active"):  {Platform: "p", ModelName: "active", FeaturedUntil: &future},
		modelCatalogMapKey("p", "expired"): {Platform: "p", ModelName: "expired", FeaturedUntil: &past},
	}}
	svc := newSvc(repo, true, 30)

	out, _ := svc.MergeDisplayConfig(context.Background(), []CatalogItem{
		{Platform: "p", ModelName: "active"},
		{Platform: "p", ModelName: "expired"},
	})
	byName := map[string]*CatalogDisplayInfo{}
	for _, it := range out {
		byName[it.ModelName] = it.Display
	}
	if !byName["active"].Featured {
		t.Fatal("active (future featured_until) should be Featured=true")
	}
	if byName["expired"].Featured {
		t.Fatal("expired (past featured_until) should be Featured=false")
	}
	if contains(byName["expired"].Tags, "featured") {
		t.Fatal("expired item tags must not contain featured")
	}
	if !contains(byName["active"].Tags, "featured") {
		t.Fatal("active item tags should contain featured")
	}
}

func TestModelCatalogService_MergeDisplayConfig_NewWindow(t *testing.T) {
	now := time.Now()
	repo := &stubCatalogRepo{getByModelKeys: map[string]*modelCatalogDisplay{
		// 刚登记（5 天前），在 30 天窗口内 → new。
		modelCatalogMapKey("p", "fresh"): {Platform: "p", ModelName: "fresh", FirstSeenAt: now.Add(-5 * 24 * time.Hour)},
		// 31 天前，超出 30 天窗口 → 非 new。
		modelCatalogMapKey("p", "old"): {Platform: "p", ModelName: "old", FirstSeenAt: now.Add(-31 * 24 * time.Hour)},
	}}
	svc := newSvc(repo, true, 30)

	out, _ := svc.MergeDisplayConfig(context.Background(), []CatalogItem{
		{Platform: "p", ModelName: "fresh"},
		{Platform: "p", ModelName: "old"},
	})
	byName := map[string]*CatalogDisplayInfo{}
	for _, it := range out {
		byName[it.ModelName] = it.Display
	}
	if !byName["fresh"].IsNew {
		t.Fatal("fresh (within window) should be IsNew=true")
	}
	if !contains(byName["fresh"].Tags, "new") {
		t.Fatal("fresh tags should contain new")
	}
	if byName["old"].IsNew {
		t.Fatal("old (outside window) should be IsNew=false")
	}
	if contains(byName["old"].Tags, "new") {
		t.Fatal("old tags must not contain new")
	}
}

func TestModelCatalogService_MergeDisplayConfig_TagsMergeDedup(t *testing.T) {
	repo := &stubCatalogRepo{getByModelKeys: map[string]*modelCatalogDisplay{
		modelCatalogMapKey("p", "m"): {Platform: "p", ModelName: "m", CustomTags: []string{"recommended", "multimodal"}},
	}}
	svc := newSvc(repo, true, 30)

	// 自动能力标签含 "multimodal"（与 custom_tags 重复）+ "vision"。
	out, _ := svc.MergeDisplayConfig(context.Background(), []CatalogItem{
		{Platform: "p", ModelName: "m", Capabilities: []string{"multimodal", "vision"}},
	})
	if len(out) != 1 {
		t.Fatalf("expected 1 item, got %d", len(out))
	}
	got := out[0].Display.Tags
	// 期望保序去重：自动标签先（multimodal, vision）→ custom（recommended；multimodal 已存在跳过）。
	want := []string{"multimodal", "vision", "recommended"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tags = %v, want %v", got, want)
	}
}

func TestModelCatalogService_MergeDisplayConfig_DegradeOnRepoError(t *testing.T) {
	repo := &stubCatalogRepo{getErr: errors.New("db down")}
	svc := newSvc(repo, true, 30)

	items := []CatalogItem{
		{Platform: "p", ModelName: "a"},
		{Platform: "p", ModelName: "b"},
	}
	out, err := svc.MergeDisplayConfig(context.Background(), items)
	if err != nil {
		t.Fatalf("degrade must not return error, got %v", err)
	}
	if len(out) != len(items) {
		t.Fatalf("degrade must return original items unchanged, got %d want %d", len(out), len(items))
	}
	for i, it := range out {
		if it.Display != nil {
			t.Fatalf("degraded item %d Display must be nil, got %+v", i, it.Display)
		}
	}
}

func TestModelCatalogService_MergeDisplayConfig_EmptyInput(t *testing.T) {
	svc := newSvc(&stubCatalogRepo{}, true, 30)
	out, err := svc.MergeDisplayConfig(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("nil input should yield empty output, got %d", len(out))
	}
}

func TestModelCatalogService_EnsureFirstSeen(t *testing.T) {
	repo := &stubCatalogRepo{upsertErr: errors.New("nope")}
	svc := newSvc(repo, true, 30)

	keys := []ModelKey{{Platform: "p", ModelName: "a"}, {Platform: "p", ModelName: "b"}}
	if err := svc.EnsureFirstSeen(context.Background(), keys); err == nil {
		t.Fatal("expected error propagated from repo")
	}
	if !reflect.DeepEqual(repo.upsertSeen, keys) {
		t.Fatalf("EnsureFirstSeen must forward keys to repo, got %v", repo.upsertSeen)
	}
}

func TestModelCatalogService_BatchSave_DropsFirstSeenAt(t *testing.T) {
	repo := &stubCatalogRepo{}
	svc := newSvc(repo, true, 30)

	ft := time.Now().Add(2 * 24 * time.Hour)
	cfgs := []AdminCatalogConfig{{
		Platform: "p", ModelName: "m", Pinned: true, SortWeight: 5,
		CustomTags: []string{"recommended"}, FeaturedUntil: &ft, Hidden: false,
		FirstSeenAt: time.Now().Add(-100 * 24 * time.Hour), // 管理员提交应被忽略
	}}
	if err := svc.BatchSave(context.Background(), cfgs); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(repo.batchSeen) != 1 {
		t.Fatalf("expected 1 row forwarded, got %d", len(repo.batchSeen))
	}
	row := repo.batchSeen[0]
	if !row.Pinned || row.SortWeight != 5 || row.FeaturedUntil != &ft {
		t.Fatalf("operational fields not forwarded correctly: %+v", row)
	}
	// FirstSeenAt 不可由管理员修改：转发的行不应携带提交值。
	if !row.FirstSeenAt.IsZero() {
		t.Fatalf("FirstSeenAt must not be written by BatchSave, got %v", row.FirstSeenAt)
	}
}

func TestModelCatalogService_ListAllForAdmin_DerivedFields(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)
	repo := &stubCatalogRepo{listResult: []*modelCatalogDisplay{
		{Platform: "p", ModelName: "fresh", CustomTags: []string{"recommended"}, FirstSeenAt: now.Add(-5 * 24 * time.Hour), FeaturedUntil: &future, Pinned: true, SortWeight: 9},
		{Platform: "p", ModelName: "hidden", Hidden: true},
	}}
	svc := newSvc(repo, true, 30)

	out, err := svc.ListAllForAdmin(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 rows (admin lists hidden too), got %d", len(out))
	}
	// 第一行：fresh，派生 IsNew/Featured，tags = recommended + new + featured。
	fresh := out[0]
	if !fresh.IsNew || !fresh.Featured {
		t.Fatalf("fresh derived flags wrong: IsNew=%v Featured=%v", fresh.IsNew, fresh.Featured)
	}
	want := []string{"recommended", "new", "featured"}
	if !reflect.DeepEqual(fresh.Tags, want) {
		t.Fatalf("fresh tags = %v, want %v", fresh.Tags, want)
	}
	// hidden 行仍返回给管理页（管理员需可见隐藏项以解除隐藏）。
	if !out[1].Hidden {
		t.Fatal("hidden row must be visible to admin")
	}
}
