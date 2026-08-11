package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

const (
	upstreamPricingTimeout = 20 * time.Second
	upstreamBalanceBodyMax = 1 << 20
	upstreamQuotaPerUSD    = 500_000
)

type UpstreamPricingClient struct {
	httpOpts httpclient.Options
}

func NewUpstreamPricingClient(cfg *config.Config) *UpstreamPricingClient {
	allow := cfg.Security.URLAllowlist
	return &UpstreamPricingClient{httpOpts: httpclient.Options{
		Timeout:            upstreamPricingTimeout,
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
	GroupRatio  map[string]float64 `json:"group_ratio"`
	UsableGroup map[string]string  `json:"usable_group"`
}

type BalanceSnapshot struct {
	Quota      int64
	UsedQuota  int64
	BalanceUSD float64
	FetchedAt  time.Time
}

type balanceResp struct {
	Success bool `json:"success"`
	Data    struct {
		Quota     int64 `json:"quota"`
		UsedQuota int64 `json:"used_quota"`
	} `json:"data"`
}

func (c *UpstreamPricingClient) httpClient(proxyURL string) (*http.Client, error) {
	opts := c.httpOpts
	opts.ProxyURL = proxyURL
	client, err := httpclient.GetClient(opts)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}
	return client, nil
}

func (c *UpstreamPricingClient) FetchPricing(ctx context.Context, baseURL string, source UpstreamPricingSource, proxyURL string) (*PricingSnapshot, error) {
	client, err := c.httpClient(proxyURL)
	if err != nil {
		return nil, err
	}
	if source == PricingSourceRatioConfig || source == PricingSourceAuto {
		snap, err := c.fetchRatioConfig(ctx, client, baseURL)
		if err == nil {
			return snap, nil
		}
		if source == PricingSourceRatioConfig {
			return nil, err
		}
		slog.WarnContext(ctx, "upstream ratio_config failed, falling back to pricing", "base_url", baseURL, "err", err)
	}
	return c.fetchPricing(ctx, client, baseURL)
}

func (c *UpstreamPricingClient) fetchRatioConfig(ctx context.Context, client *http.Client, baseURL string) (*PricingSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/ratio_config", nil)
	if err != nil {
		return nil, fmt.Errorf("create ratio_config request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/pricing", nil)
	if err != nil {
		return nil, fmt.Errorf("create pricing request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pricing status %d", resp.StatusCode)
	}
	var r pricingResp
	if err := json.Unmarshal(body, &r); err != nil || !r.Success {
		return nil, fmt.Errorf("pricing parse failed")
	}
	snap := &PricingSnapshot{Source: "pricing", Version: r.PricingVersion, GroupRatio: r.GroupRatio, UsableGroup: r.UsableGroup, FetchedAt: time.Now()}
	for _, d := range r.Data {
		snap.Models = append(snap.Models, UpstreamModelPricing{
			ModelName: d.ModelName, ModelRatio: d.ModelRatio, CompletionRatio: d.CompletionRatio,
			CacheRatio: d.CacheRatio, CreateCacheRatio: d.CreateCacheRatio, ModelPrice: d.ModelPrice,
			QuotaType: d.QuotaType, EnableGroups: d.EnableGroups,
		})
	}
	return snap, nil
}

// balanceAuthAttempt 描述一次余额请求的鉴权头组合。
// 不同 new-api/one-api 分支对 /api/user/self 的鉴权协议不一致：
//   - bearer      Authorization: Bearer <token>（新版 QuantumNous/new-api）
//   - raw         Authorization: <token>（旧版 one-api）
//   - raw_user    Authorization: <token> + New-Api-User: <userID>（旧版 QuantumNous/new-api）
//   - bearer_user Authorization: Bearer <token> + New-Api-User: <userID>（部分分叉）
type balanceAuthAttempt struct {
	useBearer bool
	withUser  bool
}

// dashboardAuthVariants 返回指定鉴权模式下的尝试序列。
// auto 模式按"最常见 → 最特殊"顺序排列，仅在 401/403 时回退，避免无效 Token 产生过多请求。
func dashboardAuthVariants(mode string, hasUserID bool) []balanceAuthAttempt {
	switch mode {
	case DashboardAuthModeBearer:
		return []balanceAuthAttempt{{useBearer: true}}
	case DashboardAuthModeRaw:
		return []balanceAuthAttempt{{useBearer: false}}
	case DashboardAuthModeRawUser:
		return []balanceAuthAttempt{{useBearer: false, withUser: true}}
	case DashboardAuthModeBearerUser:
		return []balanceAuthAttempt{{useBearer: true, withUser: true}}
	default: // auto
		variants := []balanceAuthAttempt{
			{useBearer: true},  // 新版 new-api 最常见
			{useBearer: false}, // 旧版 one-api
		}
		if hasUserID {
			// 旧版 QuantumNous/new-api 需要 New-Api-User，作为最后回退。
			variants = append(variants,
				balanceAuthAttempt{useBearer: false, withUser: true},
				balanceAuthAttempt{useBearer: true, withUser: true},
			)
		}
		return variants
	}
}

func (c *UpstreamPricingClient) FetchBalance(ctx context.Context, baseURL, dashboardToken, authMode string, userID *int64, proxyURL string) (*BalanceSnapshot, error) {
	if strings.TrimSpace(dashboardToken) == "" {
		return nil, fmt.Errorf("dashboard_token is required to refresh balance")
	}
	client, err := c.httpClient(proxyURL)
	if err != nil {
		return nil, err
	}
	var userIDStr string
	if userID != nil && *userID > 0 {
		userIDStr = strconv.FormatInt(*userID, 10)
	}
	variants := dashboardAuthVariants(NormalizeDashboardAuthMode(authMode), userIDStr != "")

	var lastErr error
	for _, attempt := range variants {
		snapshot, attemptErr := c.fetchBalanceOnce(ctx, client, baseURL, dashboardToken, attempt, userIDStr)
		if attemptErr == nil {
			return snapshot, nil
		}
		lastErr = attemptErr
		// 仅对鉴权类错误（401/403）回退到下一种模式；其他错误（网络、5xx、解析）立即返回。
		if !isBalanceAuthError(attemptErr) {
			return nil, attemptErr
		}
	}
	return nil, lastErr
}

// isBalanceAuthError 报告错误是否为可触发 auto 回退的鉴权类错误（401/403）。
func isBalanceAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "balance status 401") || strings.Contains(msg, "balance status 403")
}

// fetchBalanceOnce 执行一次带特定鉴权头的余额请求。
func (c *UpstreamPricingClient) fetchBalanceOnce(ctx context.Context, client *http.Client, baseURL, dashboardToken string, attempt balanceAuthAttempt, userIDStr string) (*BalanceSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/user/self", nil)
	if err != nil {
		return nil, fmt.Errorf("create balance request: %w", err)
	}
	if attempt.useBearer {
		req.Header.Set("Authorization", "Bearer "+dashboardToken)
	} else {
		req.Header.Set("Authorization", dashboardToken)
	}
	if attempt.withUser && userIDStr != "" {
		req.Header.Set("New-Api-User", userIDStr)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch balance: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, upstreamBalanceBodyMax+1))
	if err != nil {
		return nil, fmt.Errorf("read balance response: %w", err)
	}
	if len(body) > upstreamBalanceBodyMax {
		return nil, fmt.Errorf("balance response too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("balance status %d: %s", resp.StatusCode, sanitizeUpstreamBalanceError(body))
	}
	var r balanceResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("balance parse failed: %w", err)
	}
	if !r.Success {
		return nil, fmt.Errorf("balance response unsuccessful: %s", sanitizeUpstreamBalanceError(body))
	}
	return &BalanceSnapshot{
		Quota:      r.Data.Quota,
		UsedQuota:  r.Data.UsedQuota,
		BalanceUSD: float64(r.Data.Quota) / upstreamQuotaPerUSD,
		FetchedAt:  time.Now(),
	}, nil
}

// sanitizeUpstreamBalanceError 从上游响应体提取并清洗错误信息，截断后返回。
// 绝不回显 Token；非 JSON / HTML / WAF 页面回退为占位提示。
var balanceErrorBodyPrintLimit = 256

func sanitizeUpstreamBalanceError(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	// 尝试解析 new-api 风格的 {success, message} 错误结构。
	var parsed struct {
		Message string `json:"message"`
		Msg     string `json:"msg"`
	}
	if json.Unmarshal(body, &parsed) == nil {
		if msg := strings.TrimSpace(parsed.Message); msg != "" {
			return truncateBalanceError(msg)
		}
		if msg := strings.TrimSpace(parsed.Msg); msg != "" {
			return truncateBalanceError(msg)
		}
	}
	// 非 JSON（HTML/WAF/空）：不把整页写进数据库，回退占位。
	if strings.HasPrefix(trimmed, "<") || strings.Contains(strings.ToLower(trimmed[:min(len(trimmed), 64)]), "<!doctype") {
		return "non-json response"
	}
	return truncateBalanceError(trimmed)
}

func truncateBalanceError(s string) string {
	if len(s) > balanceErrorBodyPrintLimit {
		return s[:balanceErrorBodyPrintLimit] + "..."
	}
	return s
}
