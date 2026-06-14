package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)


// ===== mocks =====

// syncPriceRepo 是同步服务专用的内存 repo，实现全部 UpstreamPriceRepository 方法。
type syncPriceRepo struct {
	mu sync.Mutex

	sources        map[int64]*dbent.UpstreamPriceSource
	modelPrices    map[int64]map[string]*dbent.UpstreamModelPrice // sourceID -> modelName -> price
	changes        []*dbent.UpstreamPriceChange
	nextSourceID   int64
	nextPriceID    int64
	nextChangeID   int64
	replaceCalls   int
	insertCalls    int
	markNotifiedIDs []int64

	listEnabledErr error
	listPrevErr    error
	replaceErr     error
	insertErr      error
}

func newSyncPriceRepo() *syncPriceRepo {
	return &syncPriceRepo{
		sources:     map[int64]*dbent.UpstreamPriceSource{},
		modelPrices: map[int64]map[string]*dbent.UpstreamModelPrice{},
	}
}

func (r *syncPriceRepo) seedSource(s *dbent.UpstreamPriceSource) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextSourceID++
	s.ID = r.nextSourceID
	cp := *s
	r.sources[s.ID] = &cp
	return s.ID
}

func (r *syncPriceRepo) CreateSource(_ context.Context, s *dbent.UpstreamPriceSource) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextSourceID++
	s.ID = r.nextSourceID
	cp := *s
	r.sources[s.ID] = &cp
	return nil
}

func (r *syncPriceRepo) UpdateSource(_ context.Context, s *dbent.UpstreamPriceSource) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *s
	r.sources[s.ID] = &cp
	return nil
}

func (r *syncPriceRepo) DeleteSource(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sources, id)
	return nil
}

func (r *syncPriceRepo) GetSource(_ context.Context, id int64) (*dbent.UpstreamPriceSource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sources[id]
	if !ok {
		return nil, ErrUpstreamPriceSourceNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *syncPriceRepo) ListSources(_ context.Context) ([]*dbent.UpstreamPriceSource, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*dbent.UpstreamPriceSource, 0, len(r.sources))
	for _, s := range r.sources {
		cp := *s
		out = append(out, &cp)
	}
	return out, nil
}

func (r *syncPriceRepo) ListEnabledSources(ctx context.Context) ([]*dbent.UpstreamPriceSource, error) {
	if r.listEnabledErr != nil {
		return nil, r.listEnabledErr
	}
	all, _ := r.ListSources(ctx)
	out := make([]*dbent.UpstreamPriceSource, 0, len(all))
	for _, s := range all {
		if s.Enabled {
			out = append(out, s)
		}
	}
	return out, nil
}

func (r *syncPriceRepo) UpdateSourceSyncResult(_ context.Context, id int64, status, hash, lastErr string, syncedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sources[id]
	if !ok {
		return ErrUpstreamPriceSourceNotFound
	}
	s.LastSyncStatus = status
	if hash != "" {
		s.LastHash = hash
	}
	s.LastSyncError = lastErr
	s.LastSyncAt = &syncedAt
	return nil
}

func (r *syncPriceRepo) ReplaceModelPrices(_ context.Context, sourceID int64, prices []*dbent.UpstreamModelPrice) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.replaceErr != nil {
		return r.replaceErr
	}
	r.replaceCalls++
	r.modelPrices[sourceID] = map[string]*dbent.UpstreamModelPrice{}
	for _, p := range prices {
		r.nextPriceID++
		cp := *p
		cp.ID = r.nextPriceID
		cp.SourceID = sourceID
		r.modelPrices[sourceID][cp.ModelName] = &cp
	}
	return nil
}

func (r *syncPriceRepo) ListModelPrices(_ context.Context, sourceID int64) ([]*dbent.UpstreamModelPrice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.modelPrices[sourceID]
	out := make([]*dbent.UpstreamModelPrice, 0, len(m))
	for _, p := range m {
		cp := *p
		out = append(out, &cp)
	}
	return out, nil
}

func (r *syncPriceRepo) ListAllModelPricesAsMap(_ context.Context, sourceID int64) (map[string]*dbent.UpstreamModelPrice, error) {
	if r.listPrevErr != nil {
		return nil, r.listPrevErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	src := r.modelPrices[sourceID]
	out := make(map[string]*dbent.UpstreamModelPrice, len(src))
	for k, p := range src {
		cp := *p
		out[k] = &cp
	}
	return out, nil
}

func (r *syncPriceRepo) InsertChanges(_ context.Context, changes []*dbent.UpstreamPriceChange) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.insertErr != nil {
		return r.insertErr
	}
	r.insertCalls++
	for _, c := range changes {
		r.nextChangeID++
		c.ID = r.nextChangeID
		cp := *c
		r.changes = append(r.changes, &cp)
	}
	return nil
}

func (r *syncPriceRepo) ListPendingChanges(context.Context, ChangeFilters) ([]*dbent.UpstreamPriceChange, error) {
	return nil, nil
}
func (r *syncPriceRepo) GetChange(_ context.Context, id int64) (*dbent.UpstreamPriceChange, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.changes {
		if c.ID == id {
			cp := *c
			return &cp, nil
		}
	}
	return nil, ErrUpstreamPriceChangeNotFound
}
func (r *syncPriceRepo) UpdateChangeApplied(context.Context, int64, int64, string, int64) error { return nil }
func (r *syncPriceRepo) UpdateChangeDismissed(context.Context, int64, int64) error               { return nil }
func (r *syncPriceRepo) MarkChangesNotified(_ context.Context, ids []int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.markNotifiedIDs = append(r.markNotifiedIDs, ids...)
	for _, c := range r.changes {
		for _, id := range ids {
			if c.ID == id {
				c.Notified = true
			}
		}
	}
	return nil
}

// fakeGroupRateReader 返回固定倍率。
type fakeGroupRateReader struct {
	mult float64
	err  error
}

func (f *fakeGroupRateReader) RateMultiplierForModel(_ context.Context, _ string) (float64, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.mult <= 0 {
		return 1.0, nil
	}
	return f.mult, nil
}

// fakeRecipientReader 返回固定收件人列表。
type fakeRecipientReader struct {
	emails []string
	err    error
}

func (f *fakeRecipientReader) ListAlertRecipients(_ context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.emails, nil
}

// recordingNotifRepo 记录 AdminNotificationService.Create 写入的通知。
type recordingNotifRepo struct {
	mu      sync.Mutex
	created []*dbent.AdminNotification
}

func (r *recordingNotifRepo) Create(_ context.Context, n *dbent.AdminNotification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created = append(r.created, n)
	return nil
}
func (r *recordingNotifRepo) ListUnreadByUser(context.Context, int64, int) ([]*dbent.AdminNotification, error) {
	return nil, nil
}
func (r *recordingNotifRepo) CountUnreadByUser(context.Context, int64) (int64, error) { return 0, nil }
func (r *recordingNotifRepo) MarkRead(context.Context, int64, int64, time.Time) error { return nil }
func (r *recordingNotifRepo) MarkAllRead(context.Context, int64, time.Time) error     { return nil }
func (r *recordingNotifRepo) ListAll(context.Context, pagination.PaginationParams) ([]*dbent.AdminNotification, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

// noopSettingRepo 让 NotificationEmailService 走 official 模板（GetValue 返回 NotFound）。
type noopSettingRepo struct{}

func (noopSettingRepo) Get(_ context.Context, _ string) (*Setting, error) { return nil, ErrSettingNotFound }
func (noopSettingRepo) GetValue(_ context.Context, _ string) (string, error) {
	return "", ErrSettingNotFound
}
func (noopSettingRepo) Set(context.Context, string, string) error { return nil }
func (noopSettingRepo) Delete(context.Context, string) error      { return nil }
func (noopSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return nil, nil
}
func (noopSettingRepo) SetMultiple(context.Context, map[string]string) error { return nil }
func (noopSettingRepo) GetAll(context.Context) (map[string]string, error)    { return nil, nil }

// ===== helpers =====

// oneAPIPayload 构造 one_api 风格的上游响应。
// 入参 ratio/compRatio 既可传 int（字面量 1）也可传 float64，内部统一转 float64。
func oneAPIPayload(models ...[3]any) []byte {
	var b strings.Builder
	_, _ = b.WriteString(`{"data":[`)
	for i, m := range models {
		if i > 0 {
			_, _ = b.WriteString(",")
		}
		name, _ := m[0].(string)
		ratio := toTestFloat(m[1])
		compRatio := toTestFloat(m[2])
		_, _ = fmt.Fprintf(&b, `{"model_name":%q,"model_ratio":%.6f,"completion_ratio":%.6f}`, name, ratio, compRatio)
	}
	_, _ = b.WriteString(`]}`)
	return []byte(b.String())
}

// toTestFloat 把 int/float64/int64/float32 统一转 float64（测试 helper）。
func toTestFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

// upstreamTestServer 启动一个返回固定 payload 的 httptest.Server。
func upstreamTestServer(t *testing.T, payload []byte, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(payload)
	}))
}

// ===== tests =====

func TestUpstreamPriceSyncService_SyncSource_HashUnchanged_NoChanges(t *testing.T) {
	repo := newSyncPriceRepo()
	payload := oneAPIPayload([3]any{"gpt-4o", 1, 1})
	src := &dbent.UpstreamPriceSource{
		Name:                "src-a",
		APIKey:              "",
		ParserType:          "one_api",
		Enabled:             true,
		SyncIntervalMinutes: 60,
		LastHash:            sha256Hex(payload),
	}
	id := repo.seedSource(src)

	srv := upstreamTestServer(t, payload, 0)
	defer srv.Close()
	repo.sources[id].PricingEndpoint = srv.URL // 绝对 URL，buildSourceURL 直接使用

	svc := NewUpstreamPriceSyncService(repo, nil, nil, nil, nil, nil, nil,
		&upstreamPlainEncryptor{}, &noopHTTPUpstream{}, UpstreamPriceSyncConfig{})

	err := svc.SyncSource(context.Background(), id)
	require.NoError(t, err)

	// 未变：不应产 change / replace，status=success
	assert.Equal(t, 0, repo.insertCalls)
	assert.Equal(t, 0, repo.replaceCalls)
	assert.Equal(t, UpstreamSyncStatusSuccess, repo.sources[id].LastSyncStatus)
	assert.Empty(t, repo.changes)
}

func TestUpstreamPriceSyncService_SyncSource_PriceUp_EmitsAlert(t *testing.T) {
	repo := newSyncPriceRepo()
	// 上游新价：gpt-4o ratio=2（涨价，旧 ratio=1）。
	newPayload := oneAPIPayload([3]any{"gpt-4o", 2, 1})
	src := &dbent.UpstreamPriceSource{
		Name:                "src-b",
		ParserType:          "one_api",
		Enabled:             true,
		SyncIntervalMinutes: 60,
		APIKey:              "",
	}
	id := repo.seedSource(src)
	// 旧价快照：ratio=1 → input = 1*2/1e6 = 2e-6
	oldInput := 1 * 2.0 / 1e6
	repo.modelPrices[id] = map[string]*dbent.UpstreamModelPrice{
		"gpt-4o": {SourceID: id, ModelName: "gpt-4o", InputPrice: oldInput, OutputPrice: oldInput},
	}

	srv := upstreamTestServer(t, newPayload, 0)
	defer srv.Close()
	repo.sources[id].PricingEndpoint = srv.URL

	notifRepo := &recordingNotifRepo{}
	svc := NewUpstreamPriceSyncService(
		repo,
		NewAdminNotificationService(notifRepo),
		nil, // emailService=nil：跳过邮件，仅验证 admin 通知 + markNotified
		&fakeGroupRateReader{mult: 1.5},
		&fakeRecipientReader{emails: []string{"ops@example.com"}},
		nil, nil, &upstreamPlainEncryptor{}, &noopHTTPUpstream{},
		UpstreamPriceSyncConfig{},
	)

	err := svc.SyncSource(context.Background(), id)
	require.NoError(t, err)

	// 1 条 price_up change
	require.Len(t, repo.changes, 1)
	ch := repo.changes[0]
	assert.Equal(t, string(PriceChangeUp), ch.ChangeType)
	assert.Equal(t, UpstreamPriceChangeStatusPending, ch.Status)
	assert.Greater(t, ch.SuggestedInputPrice, 0.0)
	// 新倍率 = 1.5 * (old/new) = 1.5 * (2e-6/4e-6) = 0.75
	require.NotNil(t, ch.SuggestedMultiplier)
	assert.InDelta(t, 0.75, *ch.SuggestedMultiplier, 1e-9)

	// emitAlert：admin 通知 + markNotified（emailService=nil 跳过邮件）
	require.Len(t, notifRepo.created, 1)
	assert.Equal(t, upstreamPriceChangeNotificationType, notifRepo.created[0].Type)
	assert.NotEmpty(t, notifRepo.created[0].Title)
	assert.NotEmpty(t, repo.markNotifiedIDs)

	assert.Equal(t, UpstreamSyncStatusSuccess, repo.sources[id].LastSyncStatus)
}

// TestUpstreamPriceSyncService_EmailEventRegistered 验证邮件事件已注册到模板系统
// （这样当 Task 11 注入真实 EmailService 后，emitAlert 的 emailService.Send 不会因
// "event not found" 失败）。通过 NotificationEmailService.GetTemplate 间接验证。
func TestUpstreamPriceSyncService_EmailEventRegistered(t *testing.T) {
	notifEmailSvc := NewNotificationEmailService(noopSettingRepo{}, nil)
	tmpl, err := notifEmailSvc.GetTemplate(context.Background(), NotificationEmailEventUpstreamPriceChange, "en")
	require.NoError(t, err)
	assert.NotEmpty(t, tmpl.Subject)
	assert.Contains(t, tmpl.Subject, "price change")
	assert.NotEmpty(t, tmpl.HTML)
}

func TestUpstreamPriceSyncService_SyncSource_ParseFailed_NoChanges(t *testing.T) {
	repo := newSyncPriceRepo()
	src := &dbent.UpstreamPriceSource{
		Name:                "src-c",
		ParserType:          "litellm", // litellm 要求 JSON map；非法 JSON 会解析失败
		Enabled:             true,
		SyncIntervalMinutes: 60,
	}
	id := repo.seedSource(src)

	srv := upstreamTestServer(t, []byte("not-json-at-all"), 0)
	defer srv.Close()
	repo.sources[id].PricingEndpoint = srv.URL

	svc := NewUpstreamPriceSyncService(repo, nil, nil, nil, nil, nil, nil,
		&upstreamPlainEncryptor{}, &noopHTTPUpstream{}, UpstreamPriceSyncConfig{})

	err := svc.SyncSource(context.Background(), id)
	require.Error(t, err)

	assert.Equal(t, 0, repo.insertCalls)
	assert.Equal(t, 0, repo.replaceCalls)
	assert.Equal(t, UpstreamSyncStatusFailed, repo.sources[id].LastSyncStatus)
	assert.Contains(t, repo.sources[id].LastSyncError, "parse")
}

func TestUpstreamPriceSyncService_SyncSource_HTTP500_StatusFailed(t *testing.T) {
	repo := newSyncPriceRepo()
	src := &dbent.UpstreamPriceSource{
		Name:                "src-d",
		ParserType:          "one_api",
		Enabled:             true,
		SyncIntervalMinutes: 60,
	}
	id := repo.seedSource(src)

	srv := upstreamTestServer(t, []byte("internal error"), http.StatusInternalServerError)
	defer srv.Close()
	repo.sources[id].PricingEndpoint = srv.URL

	svc := NewUpstreamPriceSyncService(repo, nil, nil, nil, nil, nil, nil,
		&upstreamPlainEncryptor{}, &noopHTTPUpstream{}, UpstreamPriceSyncConfig{})

	err := svc.SyncSource(context.Background(), id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
	assert.Equal(t, UpstreamSyncStatusFailed, repo.sources[id].LastSyncStatus)
	assert.Equal(t, 0, repo.insertCalls)
}

func TestUpstreamPriceSyncService_SyncSource_HTTPNetworkError_StatusFailed(t *testing.T) {
	repo := newSyncPriceRepo()
	src := &dbent.UpstreamPriceSource{
		Name:                "src-e",
		ParserType:          "one_api",
		Enabled:             true,
		SyncIntervalMinutes: 60,
		BaseURL:             "http://127.0.0.1:1", // 不可达端口
		PricingEndpoint:     "/api/pricing",
	}
	id := repo.seedSource(src)

	svc := NewUpstreamPriceSyncService(repo, nil, nil, nil, nil, nil, nil,
		&upstreamPlainEncryptor{}, &noopHTTPUpstream{}, UpstreamPriceSyncConfig{})

	err := svc.SyncSource(context.Background(), id)
	require.Error(t, err)
	assert.Equal(t, UpstreamSyncStatusFailed, repo.sources[id].LastSyncStatus)
}

func TestUpstreamPriceSyncService_EmitAlert_SeverityClassification(t *testing.T) {
	// >20% → critical
	assert.Equal(t, "critical", classifySeverity([]*dbent.UpstreamPriceChange{
		{InputDeltaPct: 0.25}, {InputDeltaPct: 0.10},
	}))
	// >5% → warning
	assert.Equal(t, "warning", classifySeverity([]*dbent.UpstreamPriceChange{
		{InputDeltaPct: 0.10},
	}))
	// <=5% → info
	assert.Equal(t, "info", classifySeverity([]*dbent.UpstreamPriceChange{
		{InputDeltaPct: 0.02},
	}))
	// 负向变动按绝对值定级
	assert.Equal(t, "critical", classifySeverity([]*dbent.UpstreamPriceChange{
		{InputDeltaPct: -0.30},
	}))
}

func TestUpstreamPriceSyncService_SyncSource_Disabled_NoOp(t *testing.T) {
	repo := newSyncPriceRepo()
	id := repo.seedSource(&dbent.UpstreamPriceSource{Name: "src-disabled", Enabled: false})

	svc := NewUpstreamPriceSyncService(repo, nil, nil, nil, nil, nil, nil,
		&upstreamPlainEncryptor{}, &noopHTTPUpstream{}, UpstreamPriceSyncConfig{})

	err := svc.SyncSource(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, 0, repo.insertCalls)
}

func TestUpstreamPriceSyncService_SyncSource_NewModel_ProducesNewChange(t *testing.T) {
	repo := newSyncPriceRepo()
	newPayload := oneAPIPayload([3]any{"claude-opus", 1, 1})
	src := &dbent.UpstreamPriceSource{
		Name:                "src-new",
		ParserType:          "one_api",
		Enabled:             true,
		SyncIntervalMinutes: 60,
	}
	id := repo.seedSource(src)

	srv := upstreamTestServer(t, newPayload, 0)
	defer srv.Close()
	repo.sources[id].PricingEndpoint = srv.URL

	svc := NewUpstreamPriceSyncService(repo, nil, nil, nil, nil, nil, nil,
		&upstreamPlainEncryptor{}, &noopHTTPUpstream{}, UpstreamPriceSyncConfig{})

	err := svc.SyncSource(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, repo.changes, 1)
	assert.Equal(t, string(PriceChangeNew), repo.changes[0].ChangeType)
}

func TestUpstreamPriceSyncService_StartStop_Idempotent(t *testing.T) {
	repo := newSyncPriceRepo()
	svc := NewUpstreamPriceSyncService(repo, nil, nil, nil, nil, nil, nil,
		&upstreamPlainEncryptor{}, &noopHTTPUpstream{}, UpstreamPriceSyncConfig{TickInterval: 50 * time.Millisecond})

	svc.Start()
	svc.Start() // 幂等
	// 给 goroutine 一点时间启动
	time.Sleep(20 * time.Millisecond)
	svc.Stop()
	svc.Stop() // 幂等
}

func TestUpstreamPriceSyncService_SyncDueSources_OnlyDueOnes(t *testing.T) {
	repo := newSyncPriceRepo()
	// src1：从未同步（到期）
	src1 := &dbent.UpstreamPriceSource{
		Name: "due", ParserType: "one_api", Enabled: true, SyncIntervalMinutes: 60,
	}
	id1 := repo.seedSource(src1)
	srv := upstreamTestServer(t, oneAPIPayload([3]any{"m1", 1, 1}), 0)
	defer srv.Close()
	repo.sources[id1].PricingEndpoint = srv.URL

	// src2：刚同步过（未到期）
	src2 := &dbent.UpstreamPriceSource{
		Name: "not-due", ParserType: "one_api", Enabled: true, SyncIntervalMinutes: 60,
	}
	id2 := repo.seedSource(src2)
	recent := time.Now().UTC()
	repo.sources[id2].LastSyncAt = &recent

	svc := NewUpstreamPriceSyncService(repo, nil, nil, nil, nil, nil, nil,
		&upstreamPlainEncryptor{}, &noopHTTPUpstream{}, UpstreamPriceSyncConfig{})

	err := svc.syncDueSources(context.Background())
	require.NoError(t, err)
	// 只有 src1 应被同步（replaceCalls=1）
	assert.Equal(t, 1, repo.replaceCalls)
	// src2 的 last_sync_at 不应被改动（保持 recent）
	require.NotNil(t, repo.sources[id2].LastSyncAt)
}

// 编译期断言：复用同包已定义的测试 helpers。
var _ HTTPUpstream = (*noopHTTPUpstream)(nil)
var _ SecretEncryptor = (*upstreamPlainEncryptor)(nil)
var _ = tlsfingerprint.Profile{}
