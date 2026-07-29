package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// stubCatalogRepo 实现 ModelCatalogRepo（service 包内 interface），供单测。
// 不依赖 repository 包，避免 depguard service→repository 限制。
type stubCatalogRepo struct {
	// getByModelKeys 预填配置，key 须用 modelCatalogMapKey(platform, modelName)。
	getByModelKeys map[string]*ModelCatalogDisplay
	getCalled      bool
	getErr         error
	listResult     []*ModelCatalogDisplay
	listErr        error
	upsertErr      error
	batchErr       error
	// 捕获调用入参，供断言。
	upsertSeen []ModelKey
	batchSeen  []*ModelCatalogDisplay
}

func (s *stubCatalogRepo) GetByModelKeys(_ context.Context, keys []ModelKey) (map[string]*ModelCatalogDisplay, error) {
	s.getCalled = true
	if s.getErr != nil {
		return nil, s.getErr
	}
	out := make(map[string]*ModelCatalogDisplay, len(keys))
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

func (s *stubCatalogRepo) ListAll(_ context.Context) ([]*ModelCatalogDisplay, error) {
	return s.listResult, s.listErr
}

func (s *stubCatalogRepo) BatchUpsert(_ context.Context, cfgs []*ModelCatalogDisplay) error {
	s.batchSeen = cfgs
	return s.batchErr
}

// stubCatalogSettings 实现 modelCatalogSettings，供单测。
// new_model_days 时间窗已下线：接口只需运营总开关。
type stubCatalogSettings struct {
	enabled bool
}

func (s stubCatalogSettings) IsModelCatalogOpsEnabled(_ context.Context) bool { return s.enabled }

func newSvc(repo *stubCatalogRepo, enabled bool) ModelCatalogService {
	return NewModelCatalogService(repo, stubCatalogSettings{enabled: enabled})
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
	if !newSvc(&stubCatalogRepo{}, true).IsEnabled(context.Background()) {
		t.Fatal("IsEnabled=true expected when setting enabled")
	}
	if newSvc(&stubCatalogRepo{}, false).IsEnabled(context.Background()) {
		t.Fatal("IsEnabled=false expected when setting disabled")
	}
}

func TestModelCatalogService_MergeDisplayConfig_DisabledNoOp(t *testing.T) {
	repo := &stubCatalogRepo{getByModelKeys: map[string]*ModelCatalogDisplay{
		modelCatalogMapKey("openai", "gpt-4"): {Platform: "openai", ModelName: "gpt-4", Hidden: true, Pinned: true},
	}}
	// ops_enabled=false：运营总开关关闭，merge 必须整体 no-op。
	svc := newSvc(repo, false)

	items := []CatalogItem{{Platform: "openai", ModelName: "gpt-4", Capabilities: []string{"vision"}}}
	out, err := svc.MergeDisplayConfig(context.Background(), items)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("disabled must not filter hidden items, got %d", len(out))
	}
	if out[0].Display != nil {
		t.Fatalf("disabled must not merge display config, got %+v", out[0].Display)
	}
	if repo.getCalled {
		t.Fatal("disabled must not query repo at all")
	}
}

func TestModelCatalogService_MergeDisplayConfig_HiddenFiltered(t *testing.T) {
	repo := &stubCatalogRepo{getByModelKeys: map[string]*ModelCatalogDisplay{
		modelCatalogMapKey("openai", "gpt-4"):     {Platform: "openai", ModelName: "gpt-4", Hidden: true},
		modelCatalogMapKey("anthropic", "claude"): {Platform: "anthropic", ModelName: "claude"},
	}}
	svc := newSvc(repo, true)

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

func TestModelCatalogService_MergeDisplayConfig_FeaturedManualAndExpiry(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)
	past := now.Add(-1 * time.Hour)

	repo := &stubCatalogRepo{getByModelKeys: map[string]*ModelCatalogDisplay{
		// 手动 featured + 未到期 → Featured
		modelCatalogMapKey("p", "on"): {Platform: "p", ModelName: "on", Featured: true, FeaturedUntil: &future},
		// 手动 featured + 已过期 → 不 Featured
		modelCatalogMapKey("p", "expired"): {Platform: "p", ModelName: "expired", Featured: true, FeaturedUntil: &past},
		// 手动 featured 无限期 → Featured
		modelCatalogMapKey("p", "forever"): {Platform: "p", ModelName: "forever", Featured: true},
	}}
	svc := newSvc(repo, true)

	out, _ := svc.MergeDisplayConfig(context.Background(), []CatalogItem{
		{Platform: "p", ModelName: "on"},
		{Platform: "p", ModelName: "expired"},
		{Platform: "p", ModelName: "forever"},
	})
	byName := map[string]*CatalogDisplayInfo{}
	for _, it := range out {
		byName[it.ModelName] = it.Display
	}
	if !byName["on"].Featured {
		t.Fatal("featured=true with future until should be Featured")
	}
	if byName["expired"].Featured {
		t.Fatal("featured=true with past until should not be Featured")
	}
	if contains(byName["expired"].Tags, "featured") {
		t.Fatal("expired tags must not contain featured")
	}
	if !byName["forever"].Featured {
		t.Fatal("featured=true with nil until should be Featured")
	}
}

func TestModelCatalogService_MergeDisplayConfig_NewManual(t *testing.T) {
	repo := &stubCatalogRepo{getByModelKeys: map[string]*ModelCatalogDisplay{
		// 手动 NEW 开关：IsNew=true → 显示
		modelCatalogMapKey("p", "flagged"): {Platform: "p", ModelName: "flagged", IsNew: true},
		// first_seen_at 不再驱动 NEW：即便刚登记，IsNew 仍为 false
		modelCatalogMapKey("p", "off"): {Platform: "p", ModelName: "off", FirstSeenAt: time.Now()},
	}}
	svc := newSvc(repo, true)

	out, _ := svc.MergeDisplayConfig(context.Background(), []CatalogItem{
		{Platform: "p", ModelName: "flagged"},
		{Platform: "p", ModelName: "off"},
	})
	byName := map[string]*CatalogDisplayInfo{}
	for _, it := range out {
		byName[it.ModelName] = it.Display
	}
	if !byName["flagged"].IsNew {
		t.Fatal("IsNew=true should surface")
	}
	if !contains(byName["flagged"].Tags, "new") {
		t.Fatal("flagged tags should contain new")
	}
	if byName["off"].IsNew {
		t.Fatal("first_seen_at must not drive IsNew anymore")
	}
	if contains(byName["off"].Tags, "new") {
		t.Fatal("off tags must not contain new")
	}
}

func TestModelCatalogService_MergeDisplayConfig_TagsMergeDedup(t *testing.T) {
	repo := &stubCatalogRepo{getByModelKeys: map[string]*ModelCatalogDisplay{
		modelCatalogMapKey("p", "m"): {Platform: "p", ModelName: "m", CustomTags: []string{"recommended", "multimodal"}},
	}}
	svc := newSvc(repo, true)

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
	svc := newSvc(repo, true)

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
	svc := newSvc(&stubCatalogRepo{}, true)
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
	svc := newSvc(repo, true)

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
	svc := newSvc(repo, true)

	ft := time.Now().Add(2 * 24 * time.Hour)
	cfgs := []AdminCatalogConfig{{
		Platform: "p", ModelName: "m", Pinned: true, SortWeight: 5,
		CustomTags: []string{"recommended"}, FeaturedUntil: &ft, Hidden: false,
		IsNew: true, Featured: true,
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
	if !row.IsNew || !row.Featured {
		t.Fatalf("IsNew/Featured must be forwarded: %+v", row)
	}
	// FirstSeenAt 不可由管理员修改：转发的行不应携带提交值。
	if !row.FirstSeenAt.IsZero() {
		t.Fatalf("FirstSeenAt must not be written by BatchSave, got %v", row.FirstSeenAt)
	}
}

func TestModelCatalogService_ListAllForAdmin_DerivedFields(t *testing.T) {
	now := time.Now()
	future := now.Add(1 * time.Hour)
	repo := &stubCatalogRepo{listResult: []*ModelCatalogDisplay{
		{Platform: "p", ModelName: "star", CustomTags: []string{"recommended"}, IsNew: true, Featured: true, FeaturedUntil: &future, Pinned: true, SortWeight: 9},
		{Platform: "p", ModelName: "hidden", Hidden: true},
	}}
	svc := newSvc(repo, true)

	out, err := svc.ListAllForAdmin(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 rows (admin lists hidden too), got %d", len(out))
	}
	// 第一行：star，手动 IsNew/Featured，tags = recommended + new + featured。
	star := out[0]
	if !star.IsNew || !star.Featured {
		t.Fatalf("star manual flags wrong: IsNew=%v Featured=%v", star.IsNew, star.Featured)
	}
	want := []string{"recommended", "new", "featured"}
	if !reflect.DeepEqual(star.Tags, want) {
		t.Fatalf("star tags = %v, want %v", star.Tags, want)
	}
	// hidden 行仍返回给管理页（管理员需可见隐藏项以解除隐藏）。
	if !out[1].Hidden {
		t.Fatal("hidden row must be visible to admin")
	}
}
