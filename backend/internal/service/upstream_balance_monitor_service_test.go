package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpstreamBalanceMonitorRunOnceRefreshesEligibleSources(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"quota":1000000,"used_quota":0}}`))
	}))
	defer srv.Close()

	repo := newFakeRepo()
	eligible := &UpstreamSourceConfig{Name: "eligible", BaseURL: srv.URL, DashboardToken: "token", Enabled: true, BasePricePer1k: 0.002}
	disabled := &UpstreamSourceConfig{Name: "disabled", BaseURL: srv.URL, DashboardToken: "token", Enabled: false, BasePricePer1k: 0.002}
	withoutToken := &UpstreamSourceConfig{Name: "without-token", BaseURL: srv.URL, Enabled: true, BasePricePer1k: 0.002}
	for _, cfgRec := range []*UpstreamSourceConfig{eligible, disabled, withoutToken} {
		if err := repo.CreateConfig(context.Background(), cfgRec); err != nil {
			t.Fatal(err)
		}
	}

	syncSvc := NewUpstreamPriceSyncService(repo, newTestClient(), newFakeChannelService(), nil, nil, testConfig(srv.URL))
	monitor := NewUpstreamBalanceMonitorService(syncSvc, nil, nil, nil)
	monitor.runOnce()

	if repo.configs[eligible.ID].LastBalanceUSD == nil || *repo.configs[eligible.ID].LastBalanceUSD != 2 {
		t.Fatalf("eligible source was not refreshed: %+v", repo.configs[eligible.ID])
	}
	if repo.configs[disabled.ID].LastBalanceUSD != nil {
		t.Fatal("disabled source should be skipped")
	}
	if repo.configs[withoutToken.ID].LastBalanceUSD != nil {
		t.Fatal("source without dashboard token should be skipped")
	}
}

func TestUpstreamBalanceMonitorRunOnceSkipsWhenLeaderLockHeld(t *testing.T) {
	repo := newFakeRepo()
	cfgRec := &UpstreamSourceConfig{Name: "source", BaseURL: "https://example.com", DashboardToken: "token", Enabled: true, BasePricePer1k: 0.002}
	if err := repo.CreateConfig(context.Background(), cfgRec); err != nil {
		t.Fatal(err)
	}
	lockCache := &fakeLeaderLockCache{}
	_, _ = lockCache.TryAcquireLeaderLock(context.Background(), upstreamBalanceMonitorLockKey, "peer", upstreamBalanceMonitorLockTTL)

	syncSvc := NewUpstreamPriceSyncService(repo, newTestClient(), newFakeChannelService(), nil, nil, testConfig(""))
	monitor := NewUpstreamBalanceMonitorService(syncSvc, nil, lockCache, nil)
	monitor.runOnce()

	if repo.configs[cfgRec.ID].LastBalanceCheckedAt != nil {
		t.Fatal("monitor should skip refresh while another instance holds leader lock")
	}
}
