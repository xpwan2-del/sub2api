package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// ---------- test helpers ----------

// testConfig 构造一个允许 httptest(127.0.0.1 + http + 私网)的 *config.Config。
// Enabled=false 意味着不强制白名单,但 NewAPIHosts 仍包含 127.0.0.1 以便
// urlvalidator 在显式传入 AllowedHosts 时放行。
func testConfig(baseURL string) *config.Config {
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

// errFakeNotFound fakeRepo 的 not-found 错误。
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
	cfg := testConfig(srv.URL) // NewAPIHosts 含 127.0.0.1, AllowPrivateHosts=true
	fakeRepo := newFakeRepo()
	client := &UpstreamPricingClient{httpOpts: testHTTPOpts()}
	chSvc := newFakeChannelService() // 记录 ApplyUpstreamPricingEntry 调用
	svc := NewUpstreamPriceSyncService(fakeRepo, client, chSvc, cfg)

	cfgRec := &UpstreamSourceConfig{ID: 1, BaseURL: srv.URL, TargetChannelID: 7, BasePricePer1k: 0.002, Enabled: true, PricingSource: PricingSourceAuto}
	_ = fakeRepo.CreateConfig(context.Background(), cfgRec)

	reqID, err := svc.SyncNow(context.Background(), 1, 99)
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
	cfg := testConfig("")
	svc := NewUpstreamPriceSyncService(fakeRepo, &UpstreamPricingClient{httpOpts: testHTTPOpts()}, chSvc, cfg)

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
	if err := svc.ReviewItem(context.Background(), itemID, ReviewApply, nil, 42, "lgtm"); err != nil {
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
	svc := NewUpstreamPriceSyncService(fakeRepo, &UpstreamPricingClient{httpOpts: testHTTPOpts()}, chSvc, testConfig(""))

	cfgRec := &UpstreamSourceConfig{BaseURL: "https://example.com", TargetChannelID: 7, BasePricePer1k: 0.002, Enabled: true, PricingSource: PricingSourceAuto}
	_ = fakeRepo.CreateConfig(context.Background(), cfgRec)
	items := []PriceChangeItem{{
		Kind: ItemKindModelAdded, Platform: PlatformAnthropic, ModelName: "claude-x",
		TargetChannelID: 7, Status: "pending",
	}}
	req := &PriceChangeRequest{SourceConfigID: cfgRec.ID, TriggerType: "manual"}
	_ = fakeRepo.CreateRequest(context.Background(), req, items)
	itemID := items[0].ID

	if err := svc.ReviewItem(context.Background(), itemID, ReviewReject, nil, 5, "nope"); err != nil {
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
	svc := NewUpstreamPriceSyncService(fakeRepo, &UpstreamPricingClient{httpOpts: testHTTPOpts()}, chSvc, testConfig(""))

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

	if err := svc.ReviewItem(context.Background(), itemID, ReviewApply, nil, 9, ""); err == nil {
		t.Fatal("expected ReviewItem to return error when apply fails")
	}
	got, _ := fakeRepo.GetItem(context.Background(), itemID)
	if got.Status != "failed" {
		t.Fatalf("item status = %s, want failed", got.Status)
	}
}
