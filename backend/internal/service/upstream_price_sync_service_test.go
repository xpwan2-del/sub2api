package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// ---------- test helpers ----------

// testConfig 构造一个允许 httptest(127.0.0.1 + http + 私网)的 *config.Config。
// Enabled=false 意味着不强制白名单,但 NewAPIHosts 仍包含 127.0.0.1 以便
// urlvalidator 在显式传入 AllowedHosts 时放行。
func testUpstreamConfig(baseURL string) *config.Config {
	_ = baseURL
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				NewAPIHosts:       []string{"127.0.0.1"},
				AllowPrivateHosts: true,
				AllowInsecureHTTP: true,
			},
		},
	}
}

// ---------- fakeRepo: in-memory UpstreamPriceSyncRepository ----------

type fakeRepo struct {
	configs       map[int64]*UpstreamSourceConfig
	requests      map[int64]*PriceChangeRequest
	items         map[int64]*PriceChangeItem
	itemsByReq    map[int64][]int64
	groupTargets  []GroupRateTarget
	nextConfigID  int64
	nextRequestID int64
	nextItemID    int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		configs:    map[int64]*UpstreamSourceConfig{},
		requests:   map[int64]*PriceChangeRequest{},
		items:      map[int64]*PriceChangeItem{},
		itemsByReq: map[int64][]int64{},
	}
}

func (r *fakeRepo) CreateConfig(_ context.Context, c *UpstreamSourceConfig) error {
	r.nextConfigID++
	c.ID = r.nextConfigID
	r.configs[c.ID] = c
	return nil
}

func (r *fakeRepo) GetConfig(_ context.Context, id int64) (*UpstreamSourceConfig, error) {
	c, ok := r.configs[id]
	if !ok {
		return nil, errFakeNotFound
	}
	cp := *c
	return &cp, nil
}

func (r *fakeRepo) GetConfigByName(_ context.Context, name string) (*UpstreamSourceConfig, error) {
	for _, c := range r.configs {
		if c.Name == name {
			cp := *c
			return &cp, nil
		}
	}
	return nil, errFakeNotFound
}

func (r *fakeRepo) ListConfigs(_ context.Context) ([]UpstreamSourceConfig, error) {
	out := make([]UpstreamSourceConfig, 0, len(r.configs))
	for _, c := range r.configs {
		out = append(out, *c)
	}
	return out, nil
}

func (r *fakeRepo) UpdateConfig(_ context.Context, c *UpstreamSourceConfig) error {
	if _, ok := r.configs[c.ID]; !ok {
		return errFakeNotFound
	}
	r.configs[c.ID] = c
	return nil
}

func (r *fakeRepo) DeleteConfig(_ context.Context, id int64) error {
	delete(r.configs, id)
	return nil
}

func (r *fakeRepo) UpdateConfigSyncState(_ context.Context, id int64, lastSyncAt time.Time, version, lastErr string) error {
	c, ok := r.configs[id]
	if !ok {
		return errFakeNotFound
	}
	c.LastSyncAt = &lastSyncAt
	c.LastPricingVersion = version
	c.LastError = lastErr
	return nil
}

func (r *fakeRepo) UpdateConfigBalanceSuccess(_ context.Context, id int64, snapshot BalanceSnapshot) error {
	c, ok := r.configs[id]
	if !ok {
		return errFakeNotFound
	}
	c.LastBalanceQuota = upstreamInt64Ptr(snapshot.Quota)
	c.LastUsedQuota = upstreamInt64Ptr(snapshot.UsedQuota)
	c.LastBalanceUSD = upstreamFloat64Ptr(snapshot.BalanceUSD)
	c.LastBalanceAt = &snapshot.FetchedAt
	c.LastBalanceCheckedAt = &snapshot.FetchedAt
	c.LastBalanceError = ""
	return nil
}

func (r *fakeRepo) UpdateConfigBalanceError(_ context.Context, id int64, checkedAt time.Time, lastErr string) error {
	c, ok := r.configs[id]
	if !ok {
		return errFakeNotFound
	}
	c.LastBalanceCheckedAt = &checkedAt
	c.LastBalanceError = lastErr
	return nil
}

func (r *fakeRepo) CreateRequest(_ context.Context, req *PriceChangeRequest, items []PriceChangeItem) error {
	r.nextRequestID++
	req.ID = r.nextRequestID
	if req.Status == "" {
		req.Status = "open"
	}
	r.requests[req.ID] = req
	for i := range items {
		r.nextItemID++
		items[i].ID = r.nextItemID
		items[i].RequestID = req.ID
		if items[i].Status == "" {
			items[i].Status = "pending"
		}
		it := items[i]
		r.items[it.ID] = &it
		r.itemsByReq[req.ID] = append(r.itemsByReq[req.ID], it.ID)
	}
	return nil
}

func (r *fakeRepo) GetRequest(_ context.Context, id int64) (*PriceChangeRequest, error) {
	req, ok := r.requests[id]
	if !ok {
		return nil, errFakeNotFound
	}
	return req, nil
}

func (r *fakeRepo) ListRequests(_ context.Context, f RequestFilter) ([]PriceChangeRequest, int64, error) {
	var out []PriceChangeRequest
	for _, req := range r.requests {
		if f.SourceConfigID != nil && req.SourceConfigID != *f.SourceConfigID {
			continue
		}
		if f.Status != "" && req.Status != f.Status {
			continue
		}
		out = append(out, *req)
	}
	return out, int64(len(out)), nil
}

func (r *fakeRepo) ListItems(_ context.Context, requestID int64) ([]PriceChangeItem, error) {
	ids := r.itemsByReq[requestID]
	out := make([]PriceChangeItem, 0, len(ids))
	for _, id := range ids {
		out = append(out, *r.items[id])
	}
	return out, nil
}

func (r *fakeRepo) GetItem(_ context.Context, id int64) (*PriceChangeItem, error) {
	it, ok := r.items[id]
	if !ok {
		return nil, errFakeNotFound
	}
	return it, nil
}

func (r *fakeRepo) UpdateItemStatus(_ context.Context, id int64, status string, reviewerID int64, note string, appliedAt *time.Time) error {
	it, ok := r.items[id]
	if !ok {
		return errFakeNotFound
	}
	it.Status = status
	it.ReviewerID = reviewerID
	it.ReviewNote = note
	it.AppliedAt = appliedAt
	return nil
}

func (r *fakeRepo) ExpireOpenRequests(_ context.Context, configID int64) (int, error) {
	n := 0
	for _, req := range r.requests {
		if req.SourceConfigID == configID && req.Status == "open" {
			req.Status = "expired"
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) CloseRequest(_ context.Context, requestID int64) error {
	req, ok := r.requests[requestID]
	if !ok {
		return errFakeNotFound
	}
	req.Status = "closed"
	return nil
}

func (r *fakeRepo) UpdateRequestStatus(_ context.Context, requestID int64, status string, summary map[string]int) error {
	req, ok := r.requests[requestID]
	if !ok {
		return errFakeNotFound
	}
	req.Status = status
	if summary != nil {
		req.Summary = summary
	}
	return nil
}

func (r *fakeRepo) recomputeRequestStatus(requestID int64) error {
	req, ok := r.requests[requestID]
	if !ok {
		return errFakeNotFound
	}
	summary := make(map[string]int)
	pending, total := 0, 0
	for _, itemID := range r.itemsByReq[requestID] {
		item := r.items[itemID]
		summary[item.Status]++
		total++
		if item.Status == "pending" {
			pending++
		}
	}
	req.Summary = summary
	switch {
	case pending == 0:
		req.Status = "closed"
	case pending < total:
		req.Status = "partially_applied"
	default:
		req.Status = "open"
	}
	return nil
}

// errFakeNotFound fakeRepo 的 not-found 错误。
func (f *fakeRepo) WithSourceSyncLock(ctx context.Context, _ int64, fn func(context.Context) error) error {
	return fn(ctx)
}

func (f *fakeRepo) ListGroupRateTargets(context.Context, int64, int64) ([]GroupRateTarget, error) {
	return append([]GroupRateTarget(nil), f.groupTargets...), nil
}

func (f *fakeRepo) HasPendingGroupRateItems(context.Context, int64) (bool, error) {
	return false, nil
}

func (f *fakeRepo) PersistSyncResult(ctx context.Context, input SyncPersistInput) error {
	if input.AdvanceBaseline {
		if cfg := f.configs[input.ConfigID]; cfg != nil {
			cfg.GroupRatioBaselineKey = input.BaselineKey
			if input.BaselineValue != nil {
				value := *input.BaselineValue
				cfg.GroupRatioBaselineValue = &value
			}
			observedAt := input.ObservedAt
			cfg.GroupRatioBaselineObserved = &observedAt
		}
	}
	if input.Request != nil {
		return f.CreateRequest(ctx, input.Request, input.Items)
	}
	return nil
}

func (f *fakeRepo) FinalizeItemCAS(ctx context.Context, requestID, itemID int64, status string, reviewerID int64, note string, applyValue *ConvertedPrice) error {
	item, err := f.GetItem(ctx, itemID)
	if err != nil {
		return err
	}
	if item.RequestID != requestID {
		return ErrItemRequestMismatch
	}
	if item.Status != "pending" {
		return ErrItemNotPending
	}
	item.Status = status
	item.ReviewerID = reviewerID
	item.ReviewNote = note
	if status == "applied" {
		now := time.Now()
		item.AppliedAt = &now
	}
	if applyValue != nil {
		item.ApplyValue = applyValue
	}
	return f.recomputeRequestStatus(requestID)
}

func (f *fakeRepo) ApplyGroupRateItem(context.Context, GroupRateApplyInput) (int64, float64, error) {
	return 0, 0, errors.New("not implemented by fake")
}

var errFakeNotFound = errFakeNotFoundErr{}

type errFakeNotFoundErr struct{}

func (errFakeNotFoundErr) Error() string { return "fake: not found" }

// ---------- fakeChannelService: channelApplier 的内存实现 ----------

type fakeApplyCall struct {
	ChannelID int64
	Platform  string
	Models    []string
	Price     ConvertedPrice
}

type fakeChannelService struct {
	channel  *Channel       // GetByID 返回值(nil → 空 Channel,ModelPricing 为空)
	applied  *fakeApplyCall // 记录最近一次 ApplyUpstreamPricingEntry 调用(nil = 未调用)
	applyErr error          // 注入 ApplyUpstreamPricingEntry 错误
	applyRet *ChannelModelPricing
}

func newFakeChannelService() *fakeChannelService {
	return &fakeChannelService{}
}

func (f *fakeChannelService) GetByID(_ context.Context, id int64) (*Channel, error) {
	if f.channel != nil {
		cp := *f.channel
		if cp.ID == 0 {
			cp.ID = id
		}
		return &cp, nil
	}
	return &Channel{ID: id}, nil
}

func (f *fakeChannelService) ApplyUpstreamPricingEntry(_ context.Context, channelID int64, platform string, models []string, price ConvertedPrice) (*ChannelModelPricing, error) {
	f.applied = &fakeApplyCall{
		ChannelID: channelID,
		Platform:  platform,
		Models:    append([]string(nil), models...),
		Price:     price,
	}
	if f.applyErr != nil {
		return nil, f.applyErr
	}
	if f.applyRet != nil {
		return f.applyRet, nil
	}
	return &ChannelModelPricing{ChannelID: channelID, Platform: platform, Models: models}, nil
}

// ---------- tests ----------

func TestSyncNow_BuildsRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"ModelRatio":{"claude-x":1.5},"CompletionRatio":{"claude-x":2},"ModelPrice":{},"GroupRatio":{}}}`))
	}))
	defer srv.Close()
	cfg := testUpstreamConfig(srv.URL) // NewAPIHosts 含 127.0.0.1, AllowPrivateHosts=true
	fakeRepo := newFakeRepo()
	client := &UpstreamPricingClient{httpOpts: testHTTPOpts()}
	chSvc := newFakeChannelService() // 记录 ApplyUpstreamPricingEntry 调用
	svc := NewUpstreamPriceSyncService(fakeRepo, client, chSvc, nil, nil, nil, cfg)

	cfgRec := &UpstreamSourceConfig{ID: 1, BaseURL: srv.URL, TargetChannelID: 7, BasePricePer1k: 0.002, Enabled: true, PricingSource: PricingSourceAuto}
	_ = fakeRepo.CreateConfig(context.Background(), cfgRec)

	outcome, err := svc.SyncNow(context.Background(), 1, 99)
	reqID := outcome.RequestID
	if err != nil {
		t.Fatalf("SyncNow err: %v", err)
	}
	if reqID == 0 {
		t.Fatal("expected request id")
	}
	items, _ := fakeRepo.ListItems(context.Background(), reqID)
	if len(items) == 0 {
		t.Fatal("expected diff items (claude-x is new -> model_added)")
	}
	// claude-x 在目标渠道不存在,应为 model_added
	if items[0].Kind != ItemKindModelAdded {
		t.Fatalf("item kind = %s, want model_added", items[0].Kind)
	}
	if items[0].ModelName != "claude-x" {
		t.Fatalf("item model = %s, want claude-x", items[0].ModelName)
	}
	if items[0].Platform != PlatformAnthropic {
		t.Fatalf("item platform = %s, want anthropic", items[0].Platform)
	}
	if items[0].TargetChannelID != 7 {
		t.Fatalf("item target channel = %d, want 7", items[0].TargetChannelID)
	}
	// 审批单应记录创建人
	req, _ := fakeRepo.GetRequest(context.Background(), reqID)
	if req.CreatedBy != 99 {
		t.Fatalf("request created_by = %d, want 99", req.CreatedBy)
	}
}

func TestReviewItem_ApplyWrites(t *testing.T) {
	fakeRepo := newFakeRepo()
	chSvc := newFakeChannelService()
	cfg := testUpstreamConfig("")
	svc := NewUpstreamPriceSyncService(fakeRepo, &UpstreamPricingClient{httpOpts: testHTTPOpts()}, chSvc, nil, nil, nil, cfg)

	// 预置 config + request + 一条 pending item(model_added: claude-x)
	cfgRec := &UpstreamSourceConfig{BaseURL: "https://example.com", TargetChannelID: 7, BasePricePer1k: 0.002, Enabled: true, PricingSource: PricingSourceAuto}
	if err := fakeRepo.CreateConfig(context.Background(), cfgRec); err != nil {
		t.Fatalf("create config: %v", err)
	}
	price := ConvertedPrice{BillingMode: BillingModeToken, InputPrice: floatPtr(0.000003), OutputPrice: floatPtr(0.000006)}
	items := []PriceChangeItem{{
		Kind: ItemKindModelAdded, Platform: PlatformAnthropic, ModelName: "claude-x",
		TargetChannelID: 7, UpstreamConverted: &price, ApplyValue: &price, Status: "pending",
	}}
	req := &PriceChangeRequest{SourceConfigID: cfgRec.ID, TriggerType: "manual", CreatedBy: 1}
	if err := fakeRepo.CreateRequest(context.Background(), req, items); err != nil {
		t.Fatalf("create request: %v", err)
	}
	itemID := items[0].ID

	// ReviewItem(apply) → 应写入渠道定价 + item.status=applied
	if err := svc.ReviewItem(context.Background(), req.ID, itemID, ReviewApply, nil, nil, 42, "lgtm"); err != nil {
		t.Fatalf("ReviewItem err: %v", err)
	}
	if chSvc.applied == nil {
		t.Fatal("expected ApplyUpstreamPricingEntry to be called")
	}
	if chSvc.applied.ChannelID != 7 {
		t.Fatalf("apply channelID = %d, want 7", chSvc.applied.ChannelID)
	}
	if chSvc.applied.Platform != PlatformAnthropic {
		t.Fatalf("apply platform = %s, want anthropic", chSvc.applied.Platform)
	}
	if len(chSvc.applied.Models) != 1 || chSvc.applied.Models[0] != "claude-x" {
		t.Fatalf("apply models = %v, want [claude-x]", chSvc.applied.Models)
	}
	got, _ := fakeRepo.GetItem(context.Background(), itemID)
	if got.Status != "applied" {
		t.Fatalf("item status = %s, want applied", got.Status)
	}
	if got.ReviewerID != 42 {
		t.Fatalf("item reviewer = %d, want 42", got.ReviewerID)
	}
	if got.ReviewNote != "lgtm" {
		t.Fatalf("item note = %q, want lgtm", got.ReviewNote)
	}
	if got.AppliedAt == nil {
		t.Fatal("item applied_at should be set")
	}
}

func TestReviewItem_RejectMarksRejected(t *testing.T) {
	fakeRepo := newFakeRepo()
	chSvc := newFakeChannelService()
	svc := NewUpstreamPriceSyncService(fakeRepo, &UpstreamPricingClient{httpOpts: testHTTPOpts()}, chSvc, nil, nil, nil, testUpstreamConfig(""))

	cfgRec := &UpstreamSourceConfig{BaseURL: "https://example.com", TargetChannelID: 7, BasePricePer1k: 0.002, Enabled: true, PricingSource: PricingSourceAuto}
	_ = fakeRepo.CreateConfig(context.Background(), cfgRec)
	items := []PriceChangeItem{{
		Kind: ItemKindModelAdded, Platform: PlatformAnthropic, ModelName: "claude-x",
		TargetChannelID: 7, Status: "pending",
	}}
	req := &PriceChangeRequest{SourceConfigID: cfgRec.ID, TriggerType: "manual"}
	_ = fakeRepo.CreateRequest(context.Background(), req, items)
	itemID := items[0].ID

	if err := svc.ReviewItem(context.Background(), req.ID, itemID, ReviewReject, nil, nil, 5, "nope"); err != nil {
		t.Fatalf("ReviewItem err: %v", err)
	}
	if chSvc.applied != nil {
		t.Fatal("ApplyUpstreamPricingEntry should NOT be called on reject")
	}
	got, _ := fakeRepo.GetItem(context.Background(), itemID)
	if got.Status != "rejected" {
		t.Fatalf("item status = %s, want rejected", got.Status)
	}
}

func TestReviewItem_ApplyErrorMarksFailed(t *testing.T) {
	fakeRepo := newFakeRepo()
	chSvc := newFakeChannelService()
	chSvc.applyErr = errFakeNotFound // 任意非 nil 错误
	svc := NewUpstreamPriceSyncService(fakeRepo, &UpstreamPricingClient{httpOpts: testHTTPOpts()}, chSvc, nil, nil, nil, testUpstreamConfig(""))

	cfgRec := &UpstreamSourceConfig{BaseURL: "https://example.com", TargetChannelID: 7, BasePricePer1k: 0.002, Enabled: true, PricingSource: PricingSourceAuto}
	_ = fakeRepo.CreateConfig(context.Background(), cfgRec)
	price := ConvertedPrice{BillingMode: BillingModeToken, InputPrice: floatPtr(0.000003)}
	items := []PriceChangeItem{{
		Kind: ItemKindModelAdded, Platform: PlatformAnthropic, ModelName: "claude-x",
		TargetChannelID: 7, UpstreamConverted: &price, ApplyValue: &price, Status: "pending",
	}}
	req := &PriceChangeRequest{SourceConfigID: cfgRec.ID, TriggerType: "manual"}
	_ = fakeRepo.CreateRequest(context.Background(), req, items)
	itemID := items[0].ID

	if err := svc.ReviewItem(context.Background(), req.ID, itemID, ReviewApply, nil, nil, 9, ""); err == nil {
		t.Fatal("expected ReviewItem to return error when apply fails")
	}
	got, _ := fakeRepo.GetItem(context.Background(), itemID)
	if got.Status != "failed" {
		t.Fatalf("item status = %s, want failed", got.Status)
	}
}

func TestPopulateBalanceStatus(t *testing.T) {
	threshold := 10.0
	balance := 10.0
	cases := []struct {
		name string
		cfg  UpstreamSourceConfig
		want string
	}{
		{name: "not configured", cfg: UpstreamSourceConfig{}, want: "not_configured"},
		{name: "unknown", cfg: UpstreamSourceConfig{DashboardToken: "token"}, want: "unknown"},
		{name: "error", cfg: UpstreamSourceConfig{DashboardToken: "token", LastBalanceUSD: &balance, LastBalanceError: "timeout"}, want: "error"},
		{name: "low at threshold", cfg: UpstreamSourceConfig{DashboardToken: "token", LastBalanceUSD: &balance, BalanceThresholdUSD: &threshold}, want: "low"},
		{name: "healthy without threshold", cfg: UpstreamSourceConfig{DashboardToken: "token", LastBalanceUSD: &balance}, want: "healthy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			populateBalanceStatus(&tc.cfg)
			if tc.cfg.BalanceStatus != tc.want {
				t.Fatalf("status = %q, want %q", tc.cfg.BalanceStatus, tc.want)
			}
		})
	}
}

func TestRefreshBalancePersistsSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"quota":250000,"used_quota":750000}}`))
	}))
	defer srv.Close()

	repo := newFakeRepo()
	cfgRec := &UpstreamSourceConfig{Name: "source", BaseURL: srv.URL, DashboardToken: "token", Enabled: true, BasePricePer1k: 0.002}
	if err := repo.CreateConfig(context.Background(), cfgRec); err != nil {
		t.Fatal(err)
	}
	svc := NewUpstreamPriceSyncService(repo, newTestClient(), newFakeChannelService(), nil, nil, nil, testUpstreamConfig(srv.URL))
	got, err := svc.RefreshBalance(context.Background(), cfgRec.ID)
	if err != nil {
		t.Fatalf("RefreshBalance err: %v", err)
	}
	if got.LastBalanceUSD == nil || *got.LastBalanceUSD != 0.5 {
		t.Fatalf("balance = %v", got.LastBalanceUSD)
	}
	if got.BalanceStatus != "healthy" || got.LastBalanceAt == nil || got.LastBalanceCheckedAt == nil {
		t.Fatalf("unexpected refreshed config: %+v", got)
	}
}

// TestCreateConfig_RejectsNonPositiveBasePrice 验证 I4 后端守卫:
// base_price_per_1k <= 0 时拒绝(避免 ConvertPricing 全零美元价)。
func TestCreateConfig_RejectsNonPositiveBasePrice(t *testing.T) {
	fakeRepo := newFakeRepo()
	svc := NewUpstreamPriceSyncService(fakeRepo, &UpstreamPricingClient{httpOpts: testHTTPOpts()}, newFakeChannelService(), nil, nil, nil, testUpstreamConfig(""))

	for _, base := range []float64{0, -0.002} {
		cfg := &UpstreamSourceConfig{Name: "x", BaseURL: "https://example.com", BasePricePer1k: base}
		err := svc.CreateConfig(context.Background(), cfg)
		if err == nil {
			t.Fatalf("CreateConfig base=%v: expected error, got nil", base)
		}
		if !infraerrors.IsBadRequest(err) {
			t.Fatalf("CreateConfig base=%v: expected BadRequest, got %T: %v", base, err, err)
		}
		if len(fakeRepo.configs) != 0 {
			t.Fatalf("CreateConfig base=%v: should not persist, got %d configs", base, len(fakeRepo.configs))
		}
	}
}

// TestUpdateConfig_RejectsNonPositiveBasePrice 同上,针对 UpdateConfig。
func TestUpdateConfig_RejectsNonPositiveBasePrice(t *testing.T) {
	fakeRepo := newFakeRepo()
	svc := NewUpstreamPriceSyncService(fakeRepo, &UpstreamPricingClient{httpOpts: testHTTPOpts()}, newFakeChannelService(), nil, nil, nil, testUpstreamConfig(""))

	// 先正常建一条。
	good := &UpstreamSourceConfig{Name: "x", BaseURL: "https://example.com", BasePricePer1k: 0.002}
	if err := svc.CreateConfig(context.Background(), good); err != nil {
		t.Fatalf("CreateConfig (good) err: %v", err)
	}
	savedPrice := fakeRepo.configs[good.ID].BasePricePer1k

	// 用非法 base 更新应被拒,且不应改动已存值。
	bad := *good
	bad.BasePricePer1k = 0
	if err := svc.UpdateConfig(context.Background(), &bad); err == nil {
		t.Fatal("UpdateConfig base=0: expected error, got nil")
	} else if !infraerrors.IsBadRequest(err) {
		t.Fatalf("UpdateConfig base=0: expected BadRequest, got %T: %v", err, err)
	}
	if fakeRepo.configs[good.ID].BasePricePer1k != savedPrice {
		t.Fatalf("UpdateConfig base=0: persisted value changed to %v", fakeRepo.configs[good.ID].BasePricePer1k)
	}
}

// TestSyncNow_AllUnchangedNoRequest 全部模型与本地一致时(无实际变更)不建审批单,
// 保持「无价格变更」语义;unchanged 条目只在已有实际变更时随单一并展示。
func TestSyncNow_AllUnchangedNoRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// ModelRatio=1, base=0.002 → input=2e-6;不传 CompletionRatio → output=nil。
		_, _ = w.Write([]byte(`{"success":true,"data":{"ModelRatio":{"claude-m":1},"ModelPrice":{},"GroupRatio":{}}}`))
	}))
	defer srv.Close()
	cfg := testUpstreamConfig(srv.URL)
	fakeRepo := newFakeRepo()
	client := &UpstreamPricingClient{httpOpts: testHTTPOpts()}
	chSvc := newFakeChannelService()
	// 目标渠道已有与上游还原值完全一致的定价 → 全部 unchanged → 无 actionable → 不建单。
	inputPrice := 2e-6 // 1 * 0.002 / 1000
	chSvc.channel = &Channel{ModelPricing: []ChannelModelPricing{{
		ID: 1, ChannelID: 7, Platform: PlatformAnthropic, Models: []string{"claude-m"},
		BillingMode: BillingModeToken, InputPrice: &inputPrice,
	}}}
	svc := NewUpstreamPriceSyncService(fakeRepo, client, chSvc, nil, nil, nil, cfg)

	cfgRec := &UpstreamSourceConfig{ID: 1, BaseURL: srv.URL, TargetChannelID: 7, BasePricePer1k: 0.002, Enabled: true, PricingSource: PricingSourceAuto}
	_ = fakeRepo.CreateConfig(context.Background(), cfgRec)

	outcome, err := svc.SyncNow(context.Background(), 1, 99)
	reqID := outcome.RequestID
	if err != nil {
		t.Fatalf("SyncNow err: %v", err)
	}
	if reqID != 0 {
		t.Fatalf("expected no request (all unchanged), got request #%d", reqID)
	}
}

// TestReviewItem_RecomputeRequestStatus 锁死 recompute 状态机(68bdb277 修复):
// review 后审批单状态应随 item 终态推进——部分处理 → partially_applied,全部终态 → closed,
// 并同步刷新 summary。此前的 bug 是 ReviewItem 只改 item 状态、request 恒停 open。
func TestReviewItem_RecomputeRequestStatus(t *testing.T) {
	fakeRepo := newFakeRepo()
	chSvc := newFakeChannelService()
	svc := NewUpstreamPriceSyncService(fakeRepo, &UpstreamPricingClient{httpOpts: testHTTPOpts()}, chSvc, nil, nil, nil, testUpstreamConfig(""))

	cfgRec := &UpstreamSourceConfig{BaseURL: "https://example.com", TargetChannelID: 7, BasePricePer1k: 0.002, Enabled: true, PricingSource: PricingSourceAuto}
	_ = fakeRepo.CreateConfig(context.Background(), cfgRec)
	price := ConvertedPrice{BillingMode: BillingModeToken, InputPrice: floatPtr(3e-6)}
	items := []PriceChangeItem{
		{Kind: ItemKindModelAdded, Platform: PlatformAnthropic, ModelName: "claude-a", TargetChannelID: 7, UpstreamConverted: &price, ApplyValue: &price, Status: "pending"},
		{Kind: ItemKindModelAdded, Platform: PlatformAnthropic, ModelName: "claude-b", TargetChannelID: 7, UpstreamConverted: &price, ApplyValue: &price, Status: "pending"},
	}
	req := &PriceChangeRequest{SourceConfigID: cfgRec.ID, TriggerType: "manual"}
	_ = fakeRepo.CreateRequest(context.Background(), req, items)

	// ignore 第 1 条 → 仍有 pending → partially_applied;summary 反映 ignored=1、pending=1。
	if err := svc.ReviewItem(context.Background(), req.ID, items[0].ID, ReviewIgnore, nil, nil, 1, ""); err != nil {
		t.Fatalf("ReviewItem ignore #1 err: %v", err)
	}
	r1, _ := fakeRepo.GetRequest(context.Background(), req.ID)
	if r1.Status != "partially_applied" {
		t.Fatalf("after 1st ignore: status=%s, want partially_applied", r1.Status)
	}
	if r1.Summary["ignored"] != 1 || r1.Summary["pending"] != 1 {
		t.Fatalf("after 1st ignore: summary=%v, want ignored=1 pending=1", r1.Summary)
	}

	// ignore 第 2 条 → 全部终态 → closed;summary 反映 ignored=2。
	if err := svc.ReviewItem(context.Background(), req.ID, items[1].ID, ReviewIgnore, nil, nil, 1, ""); err != nil {
		t.Fatalf("ReviewItem ignore #2 err: %v", err)
	}
	r2, _ := fakeRepo.GetRequest(context.Background(), req.ID)
	if r2.Status != "closed" {
		t.Fatalf("after 2nd ignore: status=%s, want closed", r2.Status)
	}
	if r2.Summary["ignored"] != 2 {
		t.Fatalf("after 2nd ignore: summary=%v, want ignored=2", r2.Summary)
	}
}

func TestSyncNow_GroupRatioBaselineThenCreatesPerGroupItems(t *testing.T) {
	upstreamRatio := 1.0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"success":true,"data":{"ModelRatio":{},"ModelPrice":{},"GroupRatio":{"default":%v}}}`, upstreamRatio)
	}))
	defer srv.Close()
	repo := newFakeRepo()
	repo.groupTargets = []GroupRateTarget{
		{ID: 1, Name: "default", SortOrder: 1, RateMultiplier: 1},
		{ID: 2, Name: "优惠", SortOrder: 2, RateMultiplier: 0.7},
	}
	cfgRec := &UpstreamSourceConfig{
		ID: 1, BaseURL: srv.URL, TargetChannelID: 7, BasePricePer1k: 0.002,
		Enabled: true, PricingSource: PricingSourceRatioConfig, SyncGroupRatio: true,
		TargetUpstreamGroup: "default",
	}
	_ = repo.CreateConfig(context.Background(), cfgRec)
	svc := NewUpstreamPriceSyncService(repo, newTestClient(), newFakeChannelService(), nil, nil, nil, testUpstreamConfig(srv.URL))

	outcome, err := svc.SyncNow(context.Background(), cfgRec.ID, 9)
	requestID := outcome.RequestID
	if err != nil || requestID != 0 {
		t.Fatalf("baseline sync: request=%d err=%v", requestID, err)
	}
	if !outcome.GroupRatioEstablished || outcome.GroupRatioCurrent == nil || *outcome.GroupRatioCurrent != 1 {
		t.Fatalf("baseline outcome: %+v", outcome)
	}
	if cfgRec.GroupRatioBaselineValue == nil || *cfgRec.GroupRatioBaselineValue != 1 {
		t.Fatalf("baseline = %v", cfgRec.GroupRatioBaselineValue)
	}

	upstreamRatio = 1.2
	outcome, err = svc.SyncNow(context.Background(), cfgRec.ID, 9)
	requestID = outcome.RequestID
	if err != nil || requestID == 0 {
		t.Fatalf("change sync: request=%d err=%v", requestID, err)
	}
	if outcome.GroupRatioItemsCreated != 2 {
		t.Fatalf("group items created: %d", outcome.GroupRatioItemsCreated)
	}
	items, _ := repo.ListItems(context.Background(), requestID)
	if len(items) != 2 || items[0].Kind != ItemKindGroupRatio || items[1].Kind != ItemKindGroupRatio {
		t.Fatalf("items = %+v", items)
	}
	if items[0].GroupRateChange.SuggestedRate != 1.2 || items[1].GroupRateChange.SuggestedRate != 0.84 {
		t.Fatalf("suggestions = %v, %v", items[0].GroupRateChange.SuggestedRate, items[1].GroupRateChange.SuggestedRate)
	}
}
