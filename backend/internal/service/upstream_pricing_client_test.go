package service

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	snap, err := newTestClient().FetchPricing(context.Background(), srv.URL, PricingSourceRatioConfig, "")
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

	_, err := newTestClient().FetchPricing(context.Background(), "http://upstream.invalid", PricingSourceRatioConfig, proxy.URL)
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

	snapshot, err := newTestClient().FetchBalance(context.Background(), srv.URL, "dashboard-token", "")
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

	snapshot, err := newTestClient().FetchBalance(context.Background(), srv.URL, "token", "")
	if err != nil {
		t.Fatalf("FetchBalance err: %v", err)
	}
	if snapshot.BalanceUSD != -0.5 {
		t.Fatalf("balance = %v, want -0.5", snapshot.BalanceUSD)
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
	snap, err := newTestClient().FetchPricing(context.Background(), srv.URL, PricingSourceAuto, "")
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
