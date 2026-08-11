package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

func testHTTPOpts() httpclient.Options {
	return httpclient.Options{Timeout: 5 * time.Second, AllowPrivateHosts: true}
}

func newTestClient() *UpstreamPricingClient {
	return &UpstreamPricingClient{httpOpts: testHTTPOpts()}
}

func TestFetchPricing_RatioConfig(t *testing.T) {
	body := `{"success":true,"data":{"ModelRatio":{"claude-x":1.5},"CompletionRatio":{"claude-x":2},"CacheRatio":{"claude-x":0.5},"CreateCacheRatio":{"claude-x":1.25},"ModelPrice":{},"GroupRatio":{"default":1}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ratio_config" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	snap, err := newTestClient().FetchPricing(context.Background(), srv.URL, PricingSourceRatioConfig, "", "", "", "", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(snap.Models) != 1 || snap.Models[0].ModelName != "claude-x" {
		t.Fatalf("unexpected models: %+v", snap.Models)
	}
	if snap.Models[0].ModelRatio != 1.5 {
		t.Fatalf("ratio = %v", snap.Models[0].ModelRatio)
	}
}

func TestFetchPricing_UsesConfiguredProxy(t *testing.T) {
	proxyCalled := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalled = true
		if r.URL.Host != "upstream.invalid" || r.URL.Path != "/api/ratio_config" {
			t.Fatalf("unexpected proxied URL: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"ModelRatio":{},"CompletionRatio":{},"ModelPrice":{},"GroupRatio":{}}}`))
	}))
	defer proxy.Close()

	_, err := newTestClient().FetchPricing(context.Background(), "http://upstream.invalid", PricingSourceRatioConfig, proxy.URL, "", "", "", nil)
	if err != nil {
		t.Fatalf("FetchPricing via proxy err: %v", err)
	}
	if !proxyCalled {
		t.Fatal("configured proxy was not used")
	}
}

func TestFetchBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer dashboard-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"quota":500000,"used_quota":125000}}`))
	}))
	defer srv.Close()

	snapshot, err := newTestClient().FetchBalance(context.Background(), srv.URL, "dashboard-token", DashboardAuthModeBearer, nil, "")
	if err != nil {
		t.Fatalf("FetchBalance err: %v", err)
	}
	if snapshot.Quota != 500000 || snapshot.UsedQuota != 125000 || snapshot.BalanceUSD != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestFetchBalance_AllowsNegativeQuota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"quota":-250000,"used_quota":750000}}`))
	}))
	defer srv.Close()

	snapshot, err := newTestClient().FetchBalance(context.Background(), srv.URL, "token", DashboardAuthModeBearer, nil, "")
	if err != nil {
		t.Fatalf("FetchBalance err: %v", err)
	}
	if snapshot.BalanceUSD != -0.5 {
		t.Fatalf("balance = %v, want -0.5", snapshot.BalanceUSD)
	}
}

// TestFetchBalance_AutoFallsBackToRaw 验证 auto 模式在 bearer 返回 401 时回退到裸 Token。
func TestFetchBalance_AutoFallsBackToRaw(t *testing.T) {
	var seenAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = append(seenAuth, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") == "Bearer token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"success":false,"message":"invalid bearer token"}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"quota":500000,"used_quota":0}}`))
	}))
	defer srv.Close()

	snapshot, err := newTestClient().FetchBalance(context.Background(), srv.URL, "token", DashboardAuthModeAuto, nil, "")
	if err != nil {
		t.Fatalf("FetchBalance err: %v", err)
	}
	if snapshot.BalanceUSD != 1 {
		t.Fatalf("balance = %v", snapshot.BalanceUSD)
	}
	if len(seenAuth) != 2 || seenAuth[0] != "Bearer token" || seenAuth[1] != "token" {
		t.Fatalf("expected bearer then raw fallback, got %v", seenAuth)
	}
}

// TestFetchBalance_RawUserSendsNewApiUserHeader 验证 raw_user 模式发送裸 Token + New-Api-User。
func TestFetchBalance_RawUserSendsNewApiUserHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token" {
			t.Fatalf("authorization = %q, want raw token", got)
		}
		if got := r.Header.Get("New-Api-User"); got != "12" {
			t.Fatalf("New-Api-User = %q, want 12", got)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"quota":1000000,"used_quota":0}}`))
	}))
	defer srv.Close()

	uid := int64(12)
	snapshot, err := newTestClient().FetchBalance(context.Background(), srv.URL, "token", DashboardAuthModeRawUser, &uid, "")
	if err != nil {
		t.Fatalf("FetchBalance err: %v", err)
	}
	if snapshot.BalanceUSD != 2 {
		t.Fatalf("balance = %v", snapshot.BalanceUSD)
	}
}

// TestFetchBalance_AutoFallsBackToRawUser 验证 auto 模式在配置了 user_id 时最终回退到 raw_user。
func TestFetchBalance_AutoFallsBackToRawUser(t *testing.T) {
	var attempts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		user := r.Header.Get("New-Api-User")
		attempts = append(attempts, auth+"|"+user)
		// 仅裸 Token + New-Api-User 通过（旧版 QuantumNous/new-api 协议）。
		if auth == "token" && user == "5" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota":250000,"used_quota":0}}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"message":"New-Api-User header not provided"}`))
	}))
	defer srv.Close()

	uid := int64(5)
	snapshot, err := newTestClient().FetchBalance(context.Background(), srv.URL, "token", DashboardAuthModeAuto, &uid, "")
	if err != nil {
		t.Fatalf("FetchBalance err: %v", err)
	}
	if snapshot.BalanceUSD != 0.5 {
		t.Fatalf("balance = %v", snapshot.BalanceUSD)
	}
	// 应当依次尝试 bearer → raw → raw_user（命中）。
	if len(attempts) < 3 {
		t.Fatalf("expected at least 3 attempts, got %d: %v", len(attempts), attempts)
	}
}

// TestFetchBalance_NonAuthErrorDoesNotFallback 验证 500 错误不触发 auto 回退。
func TestFetchBalance_NonAuthErrorDoesNotFallback(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"message":"upstream database error"}`))
	}))
	defer srv.Close()

	_, err := newTestClient().FetchBalance(context.Background(), srv.URL, "token", DashboardAuthModeAuto, nil, "")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if calls != 1 {
		t.Fatalf("expected no fallback on 500, got %d calls", calls)
	}
}

// TestFetchBalance_ErrorIncludesUpstreamMessage 验证错误信息包含上游 message 且被截断清洗。
func TestFetchBalance_ErrorIncludesUpstreamMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"success":false,"message":"access token invalid"}`))
	}))
	defer srv.Close()

	_, err := newTestClient().FetchBalance(context.Background(), srv.URL, "token", DashboardAuthModeBearer, nil, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "access token invalid") {
		t.Fatalf("error should include status and message: %v", err)
	}
}

func TestFetchPricing_AutoFallback(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/api/ratio_config" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		// /api/pricing
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"pricing_version":"v9","data":[{"model_name":"gpt-4o","model_ratio":2.5,"completion_ratio":4,"quota_type":0,"enable_groups":["default"]}],"group_ratio":{"default":1}}`))
	}))
	defer srv.Close()
	snap, err := newTestClient().FetchPricing(context.Background(), srv.URL, PricingSourceAuto, "", "", "", "", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected fallback, calls=%d", calls)
	}
	if snap.Source != "pricing" || snap.Models[0].ModelName != "gpt-4o" {
		t.Fatalf("unexpected snap: %+v", snap)
	}
}

// TestFetchPricing_RawUserSendsNewApiUserHeader 验证 raw_user 模式发送裸 Token + New-Api-User。
func TestFetchPricing_RawUserSendsNewApiUserHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ratio_config" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if got := r.Header.Get("Authorization"); got != "token" {
			t.Fatalf("authorization = %q, want raw token", got)
		}
		if got := r.Header.Get("New-Api-User"); got != "12" {
			t.Fatalf("New-Api-User = %q, want 12", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[{"model_name":"gpt-4o","model_ratio":2.5,"completion_ratio":4,"quota_type":0,"enable_groups":["default"]}],"group_ratio":{"default":1}}`))
	}))
	defer srv.Close()

	uid := int64(12)
	snap, err := newTestClient().FetchPricing(context.Background(), srv.URL, PricingSourceAuto, "", "token", "", DashboardAuthModeRawUser, &uid)
	if err != nil {
		t.Fatalf("FetchPricing err: %v", err)
	}
	if len(snap.Models) != 1 || snap.Models[0].ModelName != "gpt-4o" {
		t.Fatalf("unexpected models: %+v", snap.Models)
	}
}

// TestFetchPricing_AutoFallsBackToRawUser 验证 auto 模式配置 user_id 时最终回退到 raw + New-Api-User。
func TestFetchPricing_AutoFallsBackToRawUser(t *testing.T) {
	var attempts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ratio_config" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		auth := r.Header.Get("Authorization")
		user := r.Header.Get("New-Api-User")
		attempts = append(attempts, auth+"|"+user)
		// 仅裸 Token + New-Api-User 通过(旧版 QuantumNous/new-api 协议)。
		if auth == "token" && user == "5" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":[{"model_name":"gpt-4o","model_ratio":2.5,"completion_ratio":4,"quota_type":0,"enable_groups":["default"]}],"group_ratio":{"default":1}}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"message":"New-Api-User header not provided"}`))
	}))
	defer srv.Close()

	uid := int64(5)
	snap, err := newTestClient().FetchPricing(context.Background(), srv.URL, PricingSourceAuto, "", "token", "", DashboardAuthModeAuto, &uid)
	if err != nil {
		t.Fatalf("FetchPricing err: %v", err)
	}
	if len(snap.Models) != 1 || snap.Models[0].ModelName != "gpt-4o" {
		t.Fatalf("unexpected models: %+v", snap.Models)
	}
	// 确认确实经过了 raw_user 变体(否则可能误命中公开模式)。
	hit := false
	for _, a := range attempts {
		if a == "token|5" {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("never attempted raw token + New-Api-User, attempts=%v", attempts)
	}
}
