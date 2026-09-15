package service

import (
	"math"
	"testing"
)

func TestConvertPricing_RatioMode(t *testing.T) {
	// model_ratio=1.5, completion_ratio=2, cache_ratio=0.5, base=0.002
	m := UpstreamModelPricing{
		ModelName: "claude-test", ModelRatio: 1.5, CompletionRatio: 2, QuotaType: 0,
		CacheRatio: floatPtr(0.5), CreateCacheRatio: floatPtr(1.25),
	}
	got := ConvertPricing(m, 0.002)
	// input = 1.5 * 0.002 / 1000 = 0.000003
	if math.Abs(*got.InputPrice-0.000003) > 1e-12 {
		t.Fatalf("input = %v, want 0.000003", *got.InputPrice)
	}
	// output = input * 2
	if math.Abs(*got.OutputPrice-0.000006) > 1e-12 {
		t.Fatalf("output = %v, want 0.000006", *got.OutputPrice)
	}
	// cache_read = input * 0.5
	if math.Abs(*got.CacheReadPrice-0.0000015) > 1e-12 {
		t.Fatalf("cache_read = %v, want 0.0000015", *got.CacheReadPrice)
	}
	if got.BillingMode != BillingModeToken {
		t.Fatalf("mode = %v, want token", got.BillingMode)
	}
}

func TestConvertPricing_PerRequestMode(t *testing.T) {
	m := UpstreamModelPricing{ModelName: "x", QuotaType: 1, ModelPrice: floatPtr(0.05)}
	got := ConvertPricing(m, 0.002)
	if got.PerRequestPrice == nil || math.Abs(*got.PerRequestPrice-0.05) > 1e-12 {
		t.Fatalf("per_request = %v, want 0.05", got.PerRequestPrice)
	}
	if got.BillingMode != BillingModePerRequest {
		t.Fatalf("mode = %v, want per_request", got.BillingMode)
	}
}

func TestConvertPricing_NilCache(t *testing.T) {
	m := UpstreamModelPricing{ModelName: "x", ModelRatio: 1, CompletionRatio: 1, QuotaType: 0}
	got := ConvertPricing(m, 0.002)
	if got.CacheReadPrice != nil || got.CacheWritePrice != nil {
		t.Fatalf("cache should be nil when upstream ratio nil")
	}
}

func TestInferPlatform(t *testing.T) {
	cases := map[string]string{
		"gpt-4o": PlatformOpenAI, "chatgpt-4o-latest": PlatformOpenAI, "o3-mini": PlatformOpenAI,
		"claude-sonnet-4":  PlatformAnthropic,
		"gemini-2.0-flash": PlatformGemini,
		"grok-2":           PlatformGrok,
		"unknown-model":    "",
	}
	for name, want := range cases {
		if got := InferPlatform(name); got != want {
			t.Errorf("InferPlatform(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestDiffPricing_PriceChange(t *testing.T) {
	up := map[string]ConvertedPrice{"claude-x": {BillingMode: BillingModeToken, InputPrice: floatPtr(3e-6), OutputPrice: floatPtr(6e-6)}}
	plat := map[string]string{"claude-x": PlatformAnthropic}
	local := []ChannelModelPricing{{ID: 10, ChannelID: 1, Platform: PlatformAnthropic, Models: []string{"claude-x"},
		BillingMode: BillingModeToken, InputPrice: floatPtr(2e-6), OutputPrice: floatPtr(6e-6)}}
	drafts := DiffPricing(up, plat, local, 1)
	if len(drafts) != 1 {
		t.Fatalf("got %d drafts, want 1", len(drafts))
	}
	if drafts[0].Kind != ItemKindModelPrice {
		t.Fatalf("kind = %v, want model_price", drafts[0].Kind)
	}
}

func TestDiffPricing_AddedAndRemoved(t *testing.T) {
	up := map[string]ConvertedPrice{"new-model": {BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6)}}
	plat := map[string]string{"new-model": PlatformOpenAI}
	local := []ChannelModelPricing{{ID: 9, ChannelID: 1, Platform: PlatformOpenAI, Models: []string{"gone-model"},
		BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6)}}
	drafts := DiffPricing(up, plat, local, 1)
	kinds := map[string]bool{}
	for _, d := range drafts {
		kinds[string(d.Kind)] = true
	}
	if !kinds[string(ItemKindModelAdded)] || !kinds[string(ItemKindModelRemoved)] {
		t.Fatalf("expected model_added + model_removed, got %v", kinds)
	}
}

func TestDiffPricing_NoChange(t *testing.T) {
	up := map[string]ConvertedPrice{"m": {BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6), OutputPrice: floatPtr(2e-6)}}
	plat := map[string]string{"m": PlatformOpenAI}
	local := []ChannelModelPricing{{ID: 1, ChannelID: 1, Platform: PlatformOpenAI, Models: []string{"m"},
		BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6), OutputPrice: floatPtr(2e-6)}}
	got := DiffPricing(up, plat, local, 1)
	// 价格一致 → 仍生成 1 条 model_unchanged 草稿(供审批单完整展示),而非 0 条。
	if len(got) != 1 {
		t.Fatalf("expected 1 unchanged draft, got %d", len(got))
	}
	if got[0].Kind != ItemKindModelUnchanged {
		t.Fatalf("kind = %v, want model_unchanged", got[0].Kind)
	}
	if got[0].Upstream == nil || got[0].Local == nil {
		t.Fatal("unchanged draft should carry both upstream and local prices")
	}
}

// TestDiffPricing_NilFieldEqualsZero 锁死「补0对比」语义:
// 上游未返回的字段(nil)与本地显式 0 视为等价,不再生成价格变更。
func TestDiffPricing_NilFieldEqualsZero(t *testing.T) {
	up := map[string]ConvertedPrice{"m": {BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6)}}
	plat := map[string]string{"m": PlatformOpenAI}
	local := []ChannelModelPricing{{ID: 1, ChannelID: 1, Platform: PlatformOpenAI, Models: []string{"m"},
		BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6), CacheReadPrice: floatPtr(0), CacheWritePrice: floatPtr(0)}}
	got := DiffPricing(up, plat, local, 1)
	if len(got) != 1 || got[0].Kind != ItemKindModelUnchanged {
		t.Fatalf("expected 1 model_unchanged, got %+v", got)
	}
}

// TestDiffPricing_PlatformEmptyMatchesByName 锁死「平台缺失兜底」语义:
// 上游推断不出平台(渠道字段缺失,InferPlatform 返回空串)时,仅按模型名匹配本地,
// 价格一致判为 model_unchanged、不一致判为 model_price,不再误判为「移除 + 新增」。
// 场景:模型名无法推断平台,由管理员手动指定平台加入渠道;再次同步上游仍无平台信息。
func TestDiffPricing_PlatformEmptyMatchesByName(t *testing.T) {
	plat := map[string]string{"deepseek-chat": ""}
	local := []ChannelModelPricing{{ID: 1, ChannelID: 1, Platform: PlatformAnthropic, Models: []string{"deepseek-chat"},
		BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6), OutputPrice: floatPtr(2e-6)}}

	// 价格一致 → model_unchanged(仅 1 条,无移除/新增)。
	up := map[string]ConvertedPrice{"deepseek-chat": {BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6), OutputPrice: floatPtr(2e-6)}}
	got := DiffPricing(up, plat, local, 1)
	if len(got) != 1 || got[0].Kind != ItemKindModelUnchanged {
		t.Fatalf("expected 1 model_unchanged, got %+v", got)
	}
	if got[0].Platform != PlatformAnthropic {
		t.Fatalf("platform = %q, want %q", got[0].Platform, PlatformAnthropic)
	}

	// 价格不一致 → model_price(仍按本地平台落库)。
	up2 := map[string]ConvertedPrice{"deepseek-chat": {BillingMode: BillingModeToken, InputPrice: floatPtr(3e-6)}}
	got2 := DiffPricing(up2, plat, local, 1)
	if len(got2) != 1 || got2[0].Kind != ItemKindModelPrice {
		t.Fatalf("expected 1 model_price, got %+v", got2)
	}
	if got2[0].Platform != PlatformAnthropic {
		t.Fatalf("platform = %q, want %q", got2[0].Platform, PlatformAnthropic)
	}
}

// TestDefaultMissingPriceFields 锁死「应用值补 0」语义:
// 上游还原值缺失(nil)的价格字段在应用值中补为 0,已存在字段保持原值。
func TestDefaultMissingPriceFields(t *testing.T) {
	got := defaultMissingPriceFields(&ConvertedPrice{
		BillingMode: BillingModeToken,
		InputPrice:  floatPtr(1e-6),
	})
	if got.InputPrice == nil || *got.InputPrice != 1e-6 {
		t.Fatalf("InputPrice = %v, want 1e-6", got.InputPrice)
	}
	for name, v := range map[string]*float64{
		"OutputPrice":     got.OutputPrice,
		"CacheReadPrice":  got.CacheReadPrice,
		"CacheWritePrice": got.CacheWritePrice,
		"PerRequestPrice": got.PerRequestPrice,
	} {
		if v == nil || *v != 0 {
			t.Fatalf("%s = %v, want 0", name, v)
		}
	}
	// nil 输入 → nil 输出(移除模型无 apply_value)。
	if got := defaultMissingPriceFields(nil); got != nil {
		t.Fatalf("nil input should yield nil, got %+v", got)
	}
}

func TestFloatEq_NilTreatedAsZero(t *testing.T) {
	cases := []struct {
		name string
		a, b *float64
		want bool
	}{
		{"nil vs nil", nil, nil, true},
		{"nil vs zero", nil, floatPtr(0), true},
		{"zero vs nil", floatPtr(0), nil, true},
		{"nil vs nonzero", nil, floatPtr(0.001), false},
		{"nonzero vs nil", floatPtr(0.001), nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := floatEq(tc.a, tc.b); got != tc.want {
				t.Fatalf("floatEq(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestDiffPricing_MixedUnchanged 同批含一致与变更:一致 → model_unchanged,变更 → model_price。
func TestDiffPricing_MixedUnchanged(t *testing.T) {
	up := map[string]ConvertedPrice{
		"same-model": {BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6), OutputPrice: floatPtr(2e-6)},
		"diff-model": {BillingMode: BillingModeToken, InputPrice: floatPtr(3e-6), OutputPrice: floatPtr(6e-6)},
	}
	plat := map[string]string{"same-model": PlatformOpenAI, "diff-model": PlatformOpenAI}
	local := []ChannelModelPricing{{ID: 1, ChannelID: 1, Platform: PlatformOpenAI, Models: []string{"same-model", "diff-model"},
		BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6), OutputPrice: floatPtr(2e-6)}}
	got := DiffPricing(up, plat, local, 1)
	if len(got) != 2 {
		t.Fatalf("expected 2 drafts (unchanged + price), got %d", len(got))
	}
	kinds := map[string]bool{}
	for _, d := range got {
		kinds[string(d.Kind)] = true
	}
	if !kinds[string(ItemKindModelUnchanged)] || !kinds[string(ItemKindModelPrice)] {
		t.Fatalf("expected model_unchanged + model_price, got %v", kinds)
	}
}

// TestDiffPricing_PreservesChannelOrder 锁死排序对齐「渠道管理 → 模型定价」添加顺序
// (= channel_model_pricing.id 升序,即 local 切片传入顺序):
//   - 本地已有模型(价格变更/无变化/移除)按 local 顺序输出,而非按 kind 聚集;
//   - 上游有、本地无的新增模型按模型名稳定排序追加。
//
// 落库后 item.id 升序继承该顺序,审批单同类型分组内(ListItems ORDER BY id ASC)即按渠道顺序展示。
func TestDiffPricing_PreservesChannelOrder(t *testing.T) {
	// local 故意按 id 升序、且 kind 交织(price/removed/unchanged/price)。
	local := []ChannelModelPricing{
		{ID: 10, ChannelID: 7, Platform: PlatformAnthropic, Models: []string{"claude-a"}, BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6)},
		{ID: 20, ChannelID: 7, Platform: PlatformOpenAI, Models: []string{"gpt-old"}, BillingMode: BillingModeToken, InputPrice: floatPtr(2e-6)},
		{ID: 30, ChannelID: 7, Platform: PlatformAnthropic, Models: []string{"claude-b"}, BillingMode: BillingModeToken, InputPrice: floatPtr(3e-6)},
		{ID: 40, ChannelID: 7, Platform: PlatformOpenAI, Models: []string{"gpt-a"}, BillingMode: BillingModeToken, InputPrice: floatPtr(4e-6)},
	}
	upstream := map[string]ConvertedPrice{
		"claude-a": {BillingMode: BillingModeToken, InputPrice: floatPtr(9e-6)}, // ≠ local 1e-6 → price
		"claude-b": {BillingMode: BillingModeToken, InputPrice: floatPtr(3e-6)}, // == local 3e-6 → unchanged
		"gpt-a":    {BillingMode: BillingModeToken, InputPrice: floatPtr(9e-6)}, // ≠ local 4e-6 → price
		"new-x":    {BillingMode: BillingModeToken, InputPrice: floatPtr(5e-6)}, // 本地无 → added
		"new-a":    {BillingMode: BillingModeToken, InputPrice: floatPtr(6e-6)}, // 本地无 → added
	}
	plat := map[string]string{
		"claude-a": PlatformAnthropic, "claude-b": PlatformAnthropic,
		"gpt-a": PlatformOpenAI, "new-x": PlatformOpenAI, "new-a": PlatformAnthropic,
	}

	drafts := DiffPricing(upstream, plat, local, 7)

	want := []struct {
		kind PriceChangeItemKind
		name string
	}{
		{ItemKindModelPrice, "claude-a"},     // 渠道第1条:价格变更
		{ItemKindModelRemoved, "gpt-old"},    // 渠道第2条:上游无 → 移除
		{ItemKindModelUnchanged, "claude-b"}, // 渠道第3条:与上游一致
		{ItemKindModelPrice, "gpt-a"},        // 渠道第4条:价格变更
		{ItemKindModelAdded, "new-a"},        // 新增:按模型名,new-a < new-x
		{ItemKindModelAdded, "new-x"},        // 新增
	}
	if len(drafts) != len(want) {
		t.Fatalf("drafts len = %d, want %d", len(drafts), len(want))
	}
	for i, w := range want {
		if drafts[i].Kind != w.kind {
			t.Errorf("drafts[%d].Kind = %s, want %s", i, drafts[i].Kind, w.kind)
		}
		if drafts[i].ModelName != w.name {
			t.Errorf("drafts[%d].ModelName = %s, want %s", i, drafts[i].ModelName, w.name)
		}
	}
}

func TestSameModelSet(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"identical single", []string{"claude-opus"}, []string{"claude-opus"}, true},
		{"identical multi same order", []string{"a", "b"}, []string{"a", "b"}, true},
		{"identical multi diff order", []string{"a", "b"}, []string{"b", "a"}, true},
		{"case insensitive", []string{"Claude-Opus", "Claude-Sonnet"}, []string{"claude-opus", "claude-sonnet"}, true},
		{"duplicates collapse", []string{"a", "a", "b"}, []string{"b", "a"}, true},
		{"partial overlap", []string{"a", "b"}, []string{"a"}, false},
		{"disjoint", []string{"a"}, []string{"b"}, false},
		{"subset other direction", []string{"a"}, []string{"a", "b"}, false},
		{"both empty", nil, nil, false},
		{"one empty", []string{"a"}, nil, false},
		{"same single dup vs single", []string{"a", "a"}, []string{"a"}, true},
	}
	for _, tc := range cases {
		if got := sameModelSet(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: sameModelSet(%v, %v) = %v, want %v", tc.name, tc.a, tc.b, got, tc.want)
		}
	}
}

func TestBuildGroupRateChanges_ProportionalAndSorted(t *testing.T) {
	targets := []GroupRateTarget{
		{ID: 3, Name: "特价", SortOrder: 30, RateMultiplier: 0.3},
		{ID: 1, Name: "default", SortOrder: 10, RateMultiplier: 1},
		{ID: 2, Name: "优惠", SortOrder: 20, RateMultiplier: 0.7},
	}
	changes, err := BuildGroupRateChanges(targets, "default", "默认", 1, 1.2)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 {
		t.Fatalf("len = %d, want 3", len(changes))
	}
	wantIDs := []int64{1, 2, 3}
	wantRates := []float64{1.2, 0.84, 0.36}
	for i := range changes {
		if changes[i].LocalGroupID != wantIDs[i] || changes[i].SuggestedRate != wantRates[i] {
			t.Fatalf("change[%d] = %+v", i, changes[i])
		}
	}
}

func TestBuildGroupRateChanges_RoundsAndRejectsInvalid(t *testing.T) {
	changes, err := BuildGroupRateChanges([]GroupRateTarget{{ID: 1, RateMultiplier: 0.33333}}, "vip", "VIP", 1.1, 1.2)
	if err != nil {
		t.Fatal(err)
	}
	if changes[0].LocalCurrentRate != 0.3333 || changes[0].SuggestedRate != 0.3636 {
		t.Fatalf("unexpected rounded change: %+v", changes[0])
	}
	for _, tc := range []struct{ old, next float64 }{{0, 1.2}, {1, 0}, {-1, 1}, {1, math.NaN()}} {
		if _, err := BuildGroupRateChanges(nil, "x", "x", tc.old, tc.next); err == nil {
			t.Fatalf("old=%v next=%v: expected error", tc.old, tc.next)
		}
	}
}

// TestConvertedPriceNullRoundtrip 锁死「nil 指针落库为 JSON null,读回仍为 nil」的往返语义。
// 否则 model_added 的 local_current 会被读回成空对象,前端 priceDetailStrings 渲染成 "0" 而非 "-"。
func TestConvertedPriceNullRoundtrip(t *testing.T) {
	raw, err := MarshalConverted(nil)
	if err != nil {
		t.Fatalf("MarshalConverted(nil): %v", err)
	}
	if string(raw) != "null" {
		t.Fatalf("MarshalConverted(nil) = %q, want \"null\"", raw)
	}
	got, err := UnmarshalConverted(raw)
	if err != nil {
		t.Fatalf("UnmarshalConverted: %v", err)
	}
	if got != nil {
		t.Fatalf("UnmarshalConverted(\"null\") = %+v, want nil", got)
	}
	// 非空对象仍应正常往返。
	price := &ConvertedPrice{BillingMode: BillingModeToken, InputPrice: floatPtr(1e-6)}
	raw2, _ := MarshalConverted(price)
	got2, err := UnmarshalConverted(raw2)
	if err != nil || got2 == nil || got2.InputPrice == nil || *got2.InputPrice != 1e-6 {
		t.Fatalf("roundtrip non-null failed: got=%+v err=%v", got2, err)
	}
}
