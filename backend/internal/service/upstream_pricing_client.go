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

func (c *UpstreamPricingClient) FetchPricing(ctx context.Context, baseURL string, source UpstreamPricingSource, proxyURL, dashboardToken, apiKey, authMode string, userID *int64) (*PricingSnapshot, error) {
	client, err := c.httpClient(proxyURL)
	if err != nil {
		return nil, err
	}
	attempts := pricingAuthAttempts(dashboardToken, apiKey, authMode, userID)
	if source == PricingSourceRatioConfig || source == PricingSourceAuto {
		snap, err := fetchWithAttempts(ctx, baseURL, "ratio_config", attempts, func(attempt pricingAuthAttempt) (*PricingSnapshot, error) {
			return c.fetchRatioConfig(ctx, client, baseURL, attempt)
		})
		if err == nil {
			return snap, nil
		}
		if source == PricingSourceRatioConfig {
			return nil, err
		}
		slog.WarnContext(ctx, "upstream ratio_config failed, falling back to pricing", "base_url", baseURL, "err", err)
	}
	return fetchWithAttempts(ctx, baseURL, "pricing", attempts, func(attempt pricingAuthAttempt) (*PricingSnapshot, error) {
		return c.fetchPricing(ctx, client, baseURL, attempt)
	})
}

func (c *UpstreamPricingClient) fetchRatioConfig(ctx context.Context, client *http.Client, baseURL string, attempt pricingAuthAttempt) (*PricingSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/ratio_config", nil)
	if err != nil {
		return nil, fmt.Errorf("create ratio_config request: %w", err)
	}
	applyPricingAuthHeaders(req, attempt)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logUpstreamExchange(ctx, "ratio_config", req, resp.StatusCode, body, true)
		return nil, fmt.Errorf("ratio_config status %d", resp.StatusCode)
	}
	var r ratioConfigResp
	if err := json.Unmarshal(body, &r); err != nil || !r.Success {
		logUpstreamExchange(ctx, "ratio_config", req, resp.StatusCode, body, true)
		return nil, fmt.Errorf("ratio_config parse failed")
	}
	logUpstreamExchange(ctx, "ratio_config", req, resp.StatusCode, body, false)
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

func (c *UpstreamPricingClient) fetchPricing(ctx context.Context, client *http.Client, baseURL string, attempt pricingAuthAttempt) (*PricingSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/pricing", nil)
	if err != nil {
		return nil, fmt.Errorf("create pricing request: %w", err)
	}
	applyPricingAuthHeaders(req, attempt)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logUpstreamExchange(ctx, "pricing", req, resp.StatusCode, body, true)
		return nil, fmt.Errorf("pricing status %d", resp.StatusCode)
	}
	var r pricingResp
	if err := json.Unmarshal(body, &r); err != nil {
		logUpstreamExchange(ctx, "pricing", req, resp.StatusCode, body, true)
		return nil, fmt.Errorf("pricing parse failed: %w (body: %s)", err, truncateUpstreamBody(body))
	}
	// 宽容:部分 new-api 分叉不返回顶层 success 字段;若已拿到 data 或 group_ratio 即视为有效响应。
	if !r.Success && len(r.Data) == 0 && len(r.GroupRatio) == 0 {
		logUpstreamExchange(ctx, "pricing", req, resp.StatusCode, body, true)
		return nil, fmt.Errorf("pricing response unsuccessful (body: %s)", truncateUpstreamBody(body))
	}
	logUpstreamExchange(ctx, "pricing", req, resp.StatusCode, body, false)
	snap := &PricingSnapshot{Source: "pricing", Version: r.PricingVersion, GroupRatio: r.GroupRatio, UsableGroup: r.UsableGroup, FetchedAt: time.Now()}
	if len(snap.GroupRatio) == 0 && len(snap.UsableGroup) == 0 {
		slog.WarnContext(ctx, "upstream pricing returned no group info", "base_url", baseURL, "models", len(snap.Models), "body_head", truncateUpstreamBody(body))
	}
	for _, d := range r.Data {
		snap.Models = append(snap.Models, UpstreamModelPricing{
			ModelName: d.ModelName, ModelRatio: d.ModelRatio, CompletionRatio: d.CompletionRatio,
			CacheRatio: d.CacheRatio, CreateCacheRatio: d.CreateCacheRatio, ModelPrice: d.ModelPrice,
			QuotaType: d.QuotaType, EnableGroups: d.EnableGroups,
		})
	}
	return snap, nil
}

// fetchWithAttempts 按 attempts 顺序调用 once;鉴权类错误(401/403 或 success:false)时
// fallback 到下一个鉴权变体,其他错误(网络/5xx/解析)立即返回。低频操作,多次请求可接受。
func fetchWithAttempts(ctx context.Context, baseURL, endpoint string, attempts []pricingAuthAttempt, once func(attempt pricingAuthAttempt) (*PricingSnapshot, error)) (*PricingSnapshot, error) {
	var lastErr error
	for i, attempt := range attempts {
		snap, err := once(attempt)
		if err == nil {
			return snap, nil
		}
		lastErr = err
		if !isUpstreamAuthError(err) {
			return nil, err
		}
		slog.DebugContext(ctx, "upstream auth failed, trying next variant", "endpoint", endpoint, "base_url", baseURL, "attempt", i+1, "total", len(attempts), "err", err)
	}
	return nil, lastErr
}

// isUpstreamAuthError 报告错误是否为可触发候选 fallback 的鉴权类错误。
// 覆盖三种上游鉴权拒绝:HTTP 401、HTTP 403、200+success:false(部分分叉用 body 表达鉴权失败)。
func isUpstreamAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "status 401") ||
		strings.Contains(msg, "status 403") ||
		strings.Contains(msg, "response unsuccessful")
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

// pricingAuthAttempt 描述一次 /api/pricing 或 /api/ratio_config 请求的鉴权头组合。
// 比 balanceAuthAttempt 多一个 token 维度:pricing 路径需在 dashboardToken/""(公开)/apiKey
// 多个 token 之间 fallback,而 balance 仅用 dashboardToken。
type pricingAuthAttempt struct {
	token     string // dashboardToken / ""(公开模式) / apiKey
	useBearer bool   // true=Bearer 前缀,false=raw token(旧版 one-api)
	userIDStr string // 非空时发送 New-Api-User 头
}

// pricingAuthAttempts 构造 pricing 路径的鉴权变体序列(复用 dashboardAuthVariants,去重,保持优先级):
//  1. dashboardToken 按 authMode 展开的鉴权变体(bearer/raw/raw_user/bearer_user,auto 时全探测)
//  2. ""(公开模式):兼容 pricing 模块公开的上游(标准 new-api 默认配置)
//  3. apiKey:模型调用令牌(sk-xxx),仅作少数魔改上游兜底
//
// new-api 各部署对 pricing 可见性配置不同,硬编码任一组合都会在某种上游下失效,
// 故按"最可能成功 → 最特殊"顺序尝试,鉴权失败再 fallback。
func pricingAuthAttempts(dashboardToken, apiKey, authMode string, userID *int64) []pricingAuthAttempt {
	dash := strings.TrimSpace(dashboardToken)
	var uidStr string
	hasUserID := false
	if userID != nil && *userID > 0 {
		uidStr = strconv.FormatInt(*userID, 10)
		hasUserID = true
	}
	seen := map[string]bool{}
	var out []pricingAuthAttempt
	add := func(token string, useBearer, withUser bool) {
		u := ""
		if withUser {
			u = uidStr
		}
		key := token + "|" + strconv.FormatBool(useBearer) + "|" + u
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, pricingAuthAttempt{token: token, useBearer: useBearer, userIDStr: u})
	}
	// 1. dashboardToken 的鉴权变体(复用 balance 路径已验证的 dashboardAuthVariants)。
	if dash != "" {
		for _, v := range dashboardAuthVariants(NormalizeDashboardAuthMode(authMode), hasUserID) {
			add(dash, v.useBearer, v.withUser)
		}
	}
	// 2. 公开模式(pricing 模块默认公开的上游);去重保证 dash 为空时不重复。
	add("", true, false)
	// 3. apiKey 兜底(少数魔改上游接受模型令牌)。
	if key := strings.TrimSpace(apiKey); key != "" {
		add(key, true, false)
	}
	return out
}

// applyPricingAuthHeaders 按 attempt 设置 pricing/ratio_config 请求的鉴权头,
// 与 fetchBalanceOnce 的 header 设置同构(bearer/raw + New-Api-User 双因子)。
func applyPricingAuthHeaders(req *http.Request, attempt pricingAuthAttempt) {
	if attempt.token != "" {
		if attempt.useBearer {
			req.Header.Set("Authorization", "Bearer "+attempt.token)
		} else {
			req.Header.Set("Authorization", attempt.token)
		}
	}
	if attempt.userIDStr != "" {
		req.Header.Set("New-Api-User", attempt.userIDStr)
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
		logUpstreamExchange(ctx, "balance", req, resp.StatusCode, body, true)
		return nil, fmt.Errorf("balance status %d: %s", resp.StatusCode, sanitizeUpstreamBalanceError(body))
	}
	var r balanceResp
	if err := json.Unmarshal(body, &r); err != nil {
		logUpstreamExchange(ctx, "balance", req, resp.StatusCode, body, true)
		return nil, fmt.Errorf("balance parse failed: %w", err)
	}
	if !r.Success {
		logUpstreamExchange(ctx, "balance", req, resp.StatusCode, body, true)
		return nil, fmt.Errorf("balance response unsuccessful: %s", sanitizeUpstreamBalanceError(body))
	}
	logUpstreamExchange(ctx, "balance", req, resp.StatusCode, body, false)
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

// truncateUpstreamBody 截断上游响应体用于错误诊断,避免日志膨胀。
func truncateUpstreamBody(body []byte) string {
	const limit = 256
	s := strings.TrimSpace(string(body))
	if len(s) > limit {
		return s[:limit] + "..."
	}
	return s
}

// maskAuthHeader 脱敏 Authorization 头用于诊断日志,绝不回显 token 本体。
// 仅暴露鉴权方式(bearer/raw/unset)与 token 长度,便于判断 token 是否注入、长度是否异常。
func maskAuthHeader(req *http.Request) string {
	v := req.Header.Get("Authorization")
	if v == "" {
		return "unset"
	}
	if strings.HasPrefix(v, "Bearer ") {
		return fmt.Sprintf("Bearer <redacted,len=%d>", len(v)-len("Bearer "))
	}
	return fmt.Sprintf("raw <redacted,len=%d>", len(v))
}

// logUpstreamExchange 记录一次上游 HTTP 调用的请求与响应诊断信息。
// failed=true 时用 Warn(生产默认可见,含响应体头部)——这是排查 401/403 的关键;
// 否则用 Debug(避免定时同步刷屏,需要时调高日志级别即可见)。
// body_head 已截断且上游 pricing/balance 响应不含 token,可安全记录。
func logUpstreamExchange(ctx context.Context, endpoint string, req *http.Request, status int, body []byte, failed bool) {
	if failed {
		slog.WarnContext(ctx, "upstream http exchange failed",
			"endpoint", endpoint,
			"method", req.Method,
			"url", req.URL.String(),
			"auth", maskAuthHeader(req),
			"new_api_user", req.Header.Get("New-Api-User"),
			"status", status,
			"body_head", truncateUpstreamBody(body),
		)
		return
	}
	slog.DebugContext(ctx, "upstream http exchange ok",
		"endpoint", endpoint,
		"method", req.Method,
		"url", req.URL.String(),
		"auth", maskAuthHeader(req),
		"status", status,
		"body_len", len(body),
	)
}
