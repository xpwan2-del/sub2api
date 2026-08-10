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
	snap, err := newTestClient().FetchPricing(context.Background(), srv.URL, PricingSourceRatioConfig)
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
	snap, err := newTestClient().FetchPricing(context.Background(), srv.URL, PricingSourceAuto)
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
