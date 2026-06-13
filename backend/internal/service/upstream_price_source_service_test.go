package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// upstreamPlainEncryptor 测试用加密器：密文 = "ENC:" + 明文，可往返。
type upstreamPlainEncryptor struct{}

func (e *upstreamPlainEncryptor) Encrypt(plaintext string) (string, error) {
	return "ENC:" + plaintext, nil
}

func (e *upstreamPlainEncryptor) Decrypt(ciphertext string) (string, error) {
	if strings.HasPrefix(ciphertext, "ENC:") {
		return strings.TrimPrefix(ciphertext, "ENC:"), nil
	}
	return ciphertext, fmt.Errorf("not encrypted")
}

// noopHTTPUpstream 直接走 http.DefaultClient，供 TestConnection 用 httptest.Server 验证。
type noopHTTPUpstream struct{}

func (n *noopHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func (n *noopHTTPUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

// errHTTPUpstream 模拟网络层失败。
type errHTTPUpstream struct{ err error }

func (e *errHTTPUpstream) Do(*http.Request, string, int64, int) (*http.Response, error) { return nil, e.err }
func (e *errHTTPUpstream) DoWithTLS(*http.Request, string, int64, int, *tlsfingerprint.Profile) (*http.Response, error) {
	return nil, e.err
}

// fakeUpstreamPriceRepo 内存实现 UpstreamPriceRepository 的 source 相关方法。
// 其余方法 panic——本测试不覆盖。
type fakeUpstreamPriceRepo struct {
	createCalls  []*dbent.UpstreamPriceSource
	updateCalls  []*dbent.UpstreamPriceSource
	sources      map[int64]*dbent.UpstreamPriceSource
	nextID       int64
	createErr    error
	updateErr    error
}

func newFakeUpstreamPriceRepo() *fakeUpstreamPriceRepo {
	return &fakeUpstreamPriceRepo{sources: map[int64]*dbent.UpstreamPriceSource{}}
}

func (r *fakeUpstreamPriceRepo) CreateSource(_ context.Context, s *dbent.UpstreamPriceSource) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.nextID++
	s.ID = r.nextID
	cp := *s
	r.sources[s.ID] = &cp
	r.createCalls = append(r.createCalls, &cp)
	return nil
}

func (r *fakeUpstreamPriceRepo) UpdateSource(_ context.Context, s *dbent.UpstreamPriceSource) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	cp := *s
	r.sources[s.ID] = &cp
	r.updateCalls = append(r.updateCalls, &cp)
	return nil
}

func (r *fakeUpstreamPriceRepo) DeleteSource(_ context.Context, id int64) error {
	delete(r.sources, id)
	return nil
}

func (r *fakeUpstreamPriceRepo) GetSource(_ context.Context, id int64) (*dbent.UpstreamPriceSource, error) {
	s, ok := r.sources[id]
	if !ok {
		return nil, ErrUpstreamPriceSourceNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *fakeUpstreamPriceRepo) ListSources(_ context.Context) ([]*dbent.UpstreamPriceSource, error) {
	out := make([]*dbent.UpstreamPriceSource, 0, len(r.sources))
	for _, s := range r.sources {
		cp := *s
		out = append(out, &cp)
	}
	return out, nil
}

func (r *fakeUpstreamPriceRepo) ListEnabledSources(ctx context.Context) ([]*dbent.UpstreamPriceSource, error) {
	all, err := r.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*dbent.UpstreamPriceSource, 0, len(all))
	for _, s := range all {
		if s.Enabled {
			out = append(out, s)
		}
	}
	return out, nil
}

// 以下方法本测试不覆盖，panic 防止误用。
func (r *fakeUpstreamPriceRepo) UpdateSourceSyncResult(context.Context, int64, string, string, string, time.Time) error {
	panic("unused")
}
func (r *fakeUpstreamPriceRepo) ReplaceModelPrices(context.Context, int64, []*dbent.UpstreamModelPrice) error {
	panic("unused")
}
func (r *fakeUpstreamPriceRepo) ListModelPrices(context.Context, int64) ([]*dbent.UpstreamModelPrice, error) {
	panic("unused")
}
func (r *fakeUpstreamPriceRepo) ListAllModelPricesAsMap(context.Context, int64) (map[string]*dbent.UpstreamModelPrice, error) {
	panic("unused")
}
func (r *fakeUpstreamPriceRepo) InsertChanges(context.Context, []*dbent.UpstreamPriceChange) error {
	panic("unused")
}
func (r *fakeUpstreamPriceRepo) ListPendingChanges(context.Context, ChangeFilters) ([]*dbent.UpstreamPriceChange, error) {
	panic("unused")
}
func (r *fakeUpstreamPriceRepo) GetChange(context.Context, int64) (*dbent.UpstreamPriceChange, error) {
	panic("unused")
}
func (r *fakeUpstreamPriceRepo) UpdateChangeApplied(context.Context, int64, int64, string, int64) error {
	panic("unused")
}
func (r *fakeUpstreamPriceRepo) UpdateChangeDismissed(context.Context, int64, int64) error {
	panic("unused")
}
func (r *fakeUpstreamPriceRepo) MarkChangesNotified(context.Context, []int64) error { panic("unused") }

// --- tests ---

func TestUpstreamPriceSourceService_Create_EncryptsAPIKey(t *testing.T) {
	repo := newFakeUpstreamPriceRepo()
	enc := &upstreamPlainEncryptor{}
	svc := NewUpstreamPriceSourceService(repo, enc, &noopHTTPUpstream{})

	src := &dbent.UpstreamPriceSource{
		Name:           "test-src",
		BaseURL:        "https://api.example.com",
		APIKey:         "sk-secret-123",
		ParserType:     "one_api",
		Enabled:        true,
	}
	created, err := svc.Create(context.Background(), src)
	require.NoError(t, err)
	require.NotNil(t, created)

	// 传给 repo 的 api_key 必须已加密（≠ 明文）
	stored := repo.sources[created.ID]
	assert.NotEqual(t, "sk-secret-123", stored.APIKey)
	assert.Equal(t, "ENC:sk-secret-123", stored.APIKey)

	// 密文能被 Decrypt 还原为明文
	plain, decErr := enc.Decrypt(stored.APIKey)
	assert.NoError(t, decErr)
	assert.Equal(t, "sk-secret-123", plain)

	// 返回给调用方的 api_key 已脱敏
	assert.Equal(t, upstreamPriceAPIKeyMask, created.APIKey)
}

func TestUpstreamPriceSourceService_Update_EmptyAPIKeyPreservesCipher(t *testing.T) {
	repo := newFakeUpstreamPriceRepo()
	enc := &upstreamPlainEncryptor{}
	svc := NewUpstreamPriceSourceService(repo, enc, &noopHTTPUpstream{})

	// 预置一条已加密的记录
	created, err := svc.Create(context.Background(), &dbent.UpstreamPriceSource{
		Name:    "src",
		BaseURL: "https://api.example.com",
		APIKey:  "sk-original",
	})
	require.NoError(t, err)
	originalCipher := repo.sources[created.ID].APIKey
	require.Equal(t, "ENC:sk-original", originalCipher)

	// 更新时 api_key 为空：应保持原密文
	updateErr := svc.Update(context.Background(), &dbent.UpstreamPriceSource{
		ID:      created.ID,
		Name:    "src-renamed",
		BaseURL: "https://api.example.com",
		APIKey:  "",
	})
	require.NoError(t, updateErr)
	assert.Equal(t, originalCipher, repo.sources[created.ID].APIKey)
}

func TestUpstreamPriceSourceService_Update_NewAPIKeyReencrypts(t *testing.T) {
	repo := newFakeUpstreamPriceRepo()
	svc := NewUpstreamPriceSourceService(repo, &upstreamPlainEncryptor{}, &noopHTTPUpstream{})

	created, err := svc.Create(context.Background(), &dbent.UpstreamPriceSource{
		Name:    "src",
		BaseURL: "https://api.example.com",
		APIKey:  "sk-old",
	})
	require.NoError(t, err)

	err = svc.Update(context.Background(), &dbent.UpstreamPriceSource{
		ID:      created.ID,
		Name:    "src",
		BaseURL: "https://api.example.com",
		APIKey:  "sk-new",
	})
	require.NoError(t, err)
	assert.Equal(t, "ENC:sk-new", repo.sources[created.ID].APIKey)
}

func TestUpstreamPriceSourceService_Get_MasksAPIKey(t *testing.T) {
	repo := newFakeUpstreamPriceRepo()
	svc := NewUpstreamPriceSourceService(repo, &upstreamPlainEncryptor{}, &noopHTTPUpstream{})

	created, err := svc.Create(context.Background(), &dbent.UpstreamPriceSource{
		Name:    "src",
		BaseURL: "https://api.example.com",
		APIKey:  "sk-secret",
	})
	require.NoError(t, err)

	got, err := svc.Get(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, upstreamPriceAPIKeyMask, got.APIKey)
	// 不泄露密文
	assert.NotContains(t, got.APIKey, "ENC:")
	assert.NotContains(t, got.APIKey, "sk-secret")
}

func TestUpstreamPriceSourceService_List_MasksAPIKey(t *testing.T) {
	repo := newFakeUpstreamPriceRepo()
	svc := NewUpstreamPriceSourceService(repo, &upstreamPlainEncryptor{}, &noopHTTPUpstream{})

	_, err := svc.Create(context.Background(), &dbent.UpstreamPriceSource{
		Name: "src", BaseURL: "https://api.example.com", APIKey: "sk-1", Enabled: true,
	})
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), &dbent.UpstreamPriceSource{
		Name: "src2", BaseURL: "https://api.example.com", APIKey: "sk-2", Enabled: false,
	})
	require.NoError(t, err)

	items, err := svc.List(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2)
	for _, it := range items {
		assert.Equal(t, upstreamPriceAPIKeyMask, it.APIKey)
	}

	enabled, err := svc.ListEnabled(context.Background())
	require.NoError(t, err)
	require.Len(t, enabled, 1)
	assert.Equal(t, upstreamPriceAPIKeyMask, enabled[0].APIKey)
}

func TestUpstreamPriceSourceService_TestConnection_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 校验 Bearer 头被正确注入
		assert.Equal(t, "Bearer sk-decrypted", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"model_name":"gpt-4","model_ratio":3,"completion_ratio":4}]}`))
	}))
	defer srv.Close()

	repo := newFakeUpstreamPriceRepo()
	enc := &upstreamPlainEncryptor{}
	svc := NewUpstreamPriceSourceService(repo, enc, &noopHTTPUpstream{})

	// api_key 为密文（ENC: 前缀），TestConnection 内部应解密后用于 Bearer 头
	src := &dbent.UpstreamPriceSource{
		Name:            "src",
		BaseURL:         srv.URL,
		PricingEndpoint: "/api/pricing",
		APIKey:          "ENC:sk-decrypted",
		ParserType:      "one_api",
	}
	reachable, modelCount, err := svc.TestConnection(context.Background(), src)
	require.NoError(t, err)
	assert.True(t, reachable)
	assert.Equal(t, 1, modelCount)
}

func TestUpstreamPriceSourceService_TestConnection_UnreachableOnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := NewUpstreamPriceSourceService(newFakeUpstreamPriceRepo(), &upstreamPlainEncryptor{}, &noopHTTPUpstream{})
	src := &dbent.UpstreamPriceSource{
		BaseURL:         srv.URL,
		PricingEndpoint: "/api/pricing",
		ParserType:      "one_api",
	}
	reachable, _, err := svc.TestConnection(context.Background(), src)
	require.Error(t, err)
	assert.False(t, reachable)
}

func TestUpstreamPriceSourceService_TestConnection_NetworkFailureUnreachable(t *testing.T) {
	svc := NewUpstreamPriceSourceService(
		newFakeUpstreamPriceRepo(),
		&upstreamPlainEncryptor{},
		&errHTTPUpstream{err: errors.New("connection refused")},
	)
	src := &dbent.UpstreamPriceSource{
		BaseURL:         "https://nonexistent.invalid",
		PricingEndpoint: "/api/pricing",
		ParserType:      "one_api",
	}
	reachable, _, err := svc.TestConnection(context.Background(), src)
	require.Error(t, err)
	assert.False(t, reachable)
}

func TestUpstreamPriceSourceService_TestConnection_ReachableButNoData(t *testing.T) {
	// 接口可达（200），但 data 为空数组 → reachable=true, modelCount=0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	svc := NewUpstreamPriceSourceService(newFakeUpstreamPriceRepo(), &upstreamPlainEncryptor{}, &noopHTTPUpstream{})
	src := &dbent.UpstreamPriceSource{
		BaseURL:    srv.URL,
		ParserType: "one_api",
	}
	reachable, modelCount, err := svc.TestConnection(context.Background(), src)
	require.NoError(t, err)
	assert.True(t, reachable)
	assert.Equal(t, 0, modelCount)
}

func TestBuildSourceURL(t *testing.T) {
	cases := []struct {
		name     string
		base     string
		endpoint string
		want     string
		wantErr  bool
	}{
		{"concat", "https://api.example.com", "/api/pricing", "https://api.example.com/api/pricing", false},
		{"missing slash", "https://api.example.com", "api/pricing", "https://api.example.com/api/pricing", false},
		{"default endpoint", "https://api.example.com", "", "https://api.example.com/api/pricing", false},
		{"absolute endpoint", "https://api.example.com", "https://other.example.com/pricing", "https://other.example.com/pricing", false},
		{"empty base", "", "/api/pricing", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildSourceURL(c.base, c.endpoint)
			if c.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}
