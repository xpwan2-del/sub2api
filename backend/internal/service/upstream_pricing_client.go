package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

type UpstreamPricingClient struct {
	httpOpts httpclient.Options
}

func NewUpstreamPricingClient(cfg *config.Config) *UpstreamPricingClient {
	allow := cfg.Security.URLAllowlist
	return &UpstreamPricingClient{httpOpts: httpclient.Options{
		Timeout:            20 * time.Second,
		ValidateResolvedIP: allow.Enabled,
		AllowPrivateHosts:  allow.AllowPrivateHosts,
	}}
}

type ratioConfigResp struct {
	Success bool `json:"success"`
	Data    struct {
		ModelRatio       map[string]float64 `json:"ModelRatio"`
		CompletionRatio  map[string]float64 `json:"CompletionRatio"`
		CacheRatio       map[string]float64 `json:"CacheRatio"`
		CreateCacheRatio map[string]float64 `json:"CreateCacheRatio"`
		ModelPrice       map[string]float64 `json:"ModelPrice"`
		GroupRatio       map[string]float64 `json:"GroupRatio"`
	} `json:"data"`
}

type pricingResp struct {
	Success        bool   `json:"success"`
	PricingVersion string `json:"pricing_version"`
	Data           []struct {
		ModelName        string   `json:"model_name"`
		ModelRatio       float64  `json:"model_ratio"`
		CompletionRatio  float64  `json:"completion_ratio"`
		CacheRatio       *float64 `json:"cache_ratio"`
		CreateCacheRatio *float64 `json:"create_cache_ratio"`
		ModelPrice       *float64 `json:"model_price"`
		QuotaType        int      `json:"quota_type"`
		EnableGroups     []string `json:"enable_groups"`
	} `json:"data"`
	GroupRatio map[string]float64 `json:"group_ratio"`
}

func (c *UpstreamPricingClient) FetchPricing(ctx context.Context, baseURL string, source UpstreamPricingSource) (*PricingSnapshot, error) {
	client, err := httpclient.GetClient(c.httpOpts)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}
	if source == PricingSourceRatioConfig || source == PricingSourceAuto {
		snap, err := c.fetchRatioConfig(ctx, client, baseURL)
		if err == nil {
			return snap, nil
		}
		if source == PricingSourceRatioConfig {
			return nil, err
		}
		// auto: 回退 pricing
	}
	return c.fetchPricing(ctx, client, baseURL)
}

func (c *UpstreamPricingClient) fetchRatioConfig(ctx context.Context, client *http.Client, baseURL string) (*PricingSnapshot, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/ratio_config", nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ratio_config status %d", resp.StatusCode)
	}
	var r ratioConfigResp
	if err := json.Unmarshal(body, &r); err != nil || !r.Success {
		return nil, fmt.Errorf("ratio_config parse failed")
	}
	snap := &PricingSnapshot{Source: "ratio_config", GroupRatio: r.Data.GroupRatio, FetchedAt: time.Now()}
	for name, ratio := range r.Data.ModelRatio {
		m := UpstreamModelPricing{ModelName: name, ModelRatio: ratio, QuotaType: 0}
		m.CompletionRatio = r.Data.CompletionRatio[name]
		if v, ok := r.Data.CacheRatio[name]; ok {
			m.CacheRatio = &v
		}
		if v, ok := r.Data.CreateCacheRatio[name]; ok {
			m.CreateCacheRatio = &v
		}
		if p, ok := r.Data.ModelPrice[name]; ok && p >= 0 {
			m.ModelPrice = &p
		}
		snap.Models = append(snap.Models, m)
	}
	return snap, nil
}

func (c *UpstreamPricingClient) fetchPricing(ctx context.Context, client *http.Client, baseURL string) (*PricingSnapshot, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/pricing", nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pricing status %d", resp.StatusCode)
	}
	var r pricingResp
	if err := json.Unmarshal(body, &r); err != nil || !r.Success {
		return nil, fmt.Errorf("pricing parse failed")
	}
	snap := &PricingSnapshot{Source: "pricing", Version: r.PricingVersion, GroupRatio: r.GroupRatio, FetchedAt: time.Now()}
	for _, d := range r.Data {
		snap.Models = append(snap.Models, UpstreamModelPricing{
			ModelName: d.ModelName, ModelRatio: d.ModelRatio, CompletionRatio: d.CompletionRatio,
			CacheRatio: d.CacheRatio, CreateCacheRatio: d.CreateCacheRatio, ModelPrice: d.ModelPrice,
			QuotaType: d.QuotaType, EnableGroups: d.EnableGroups,
		})
	}
	return snap, nil
}
