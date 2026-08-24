package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// registryTagsClient 经 OCI Distribution API 查询自有镜像仓库（如 Harbor）的
// tag 列表，供 managed 模式更新检查比较最新 CalVer Build（见
// UpdateService.checkUpdateViaRegistry）。
//
// 配置经环境变量注入（部署方 compose，与 UPGRADE_GUIDE_* 同模式）：
//
//	UPDATE_REGISTRY        registry 地址（host[:port]，默认 https；显式 http:// 前缀走内网明文）
//	UPDATE_REGISTRY_IMAGE  镜像名（如 topai/sub2api）
//	UPDATE_REGISTRY_AUTH   可选 "user:password"（私有仓库；匿名仓库可省略）
type registryTagsClient struct {
	baseURL    string // 形如 https://harbor.example.com:8077
	image      string // 形如 topai/sub2api
	basicAuth  string // 已编码的 "Basic xxx" 头值，匿名为空
	httpClient *http.Client
}

// NewRegistryTagsClient 创建 registry tag 查询客户端；registry 或 image 未配置时
// 返回 nil（managed 更新检查回退到「已是最新」静态应答）。
func NewRegistryTagsClient(registry, image, auth string) *registryTagsClient {
	registry = strings.TrimSpace(registry)
	image = strings.TrimSpace(image)
	if registry == "" || image == "" {
		return nil
	}
	baseURL := registry
	if !strings.Contains(baseURL, "://") {
		baseURL = "https://" + baseURL
	}
	basicAuth := ""
	if auth = strings.TrimSpace(auth); auth != "" {
		basicAuth = "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	}
	return &registryTagsClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		image:      image,
		basicAuth:  basicAuth,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type registryTagList struct {
	Tags []string `json:"tags"`
}

// LatestBuildTag 返回字典序最大的 CalVer Build tag（Build 日期零填充，字典序即
// 时间序）。无任何 CalVer tag 时返回空串（如仓库只有 latest 别名）。
//
// tags/list 不带 n 参数时 registry 返回全量 tag（Harbor 行为），自研构建频率下
// 数量有限，不处理 Link 分页。
func (c *registryTagsClient) LatestBuildTag(ctx context.Context) (string, error) {
	tags, err := c.listTags(ctx)
	if err != nil {
		return "", err
	}
	latest := ""
	for _, tag := range tags {
		if !service.IsCalVerBuild(tag) {
			continue // 跳过 latest 别名等非版本 tag
		}
		if tag > latest {
			latest = tag
		}
	}
	return latest, nil
}

// listTags 拉取 /v2/<name>/tags/list，遇 401 时按 WWW-Authenticate challenge
// 完成认证后重试一次（Harbor 走 Bearer token 流程）。
func (c *registryTagsClient) listTags(ctx context.Context) ([]string, error) {
	resp, err := c.fetchWithAuth(ctx, c.tagsURL())
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry tags/list: unexpected status %s", resp.Status)
	}
	var payload registryTagList
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("registry tags/list: decode response: %w", err)
	}
	return payload.Tags, nil
}

func (c *registryTagsClient) tagsURL() string {
	return fmt.Sprintf("%s/v2/%s/tags/list", c.baseURL, c.image)
}

// fetchWithAuth 请求 tags/list；401 时按 challenge 换取认证后重试一次。
func (c *registryTagsClient) fetchWithAuth(ctx context.Context, tagsURL string) (*http.Response, error) {
	resp, err := c.do(ctx, tagsURL, c.basicAuth)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	challenge := resp.Header.Get("WWW-Authenticate")
	_, _ = io.Copy(io.Discard, resp.Body) // 排空并关闭，归还连接
	_ = resp.Body.Close()

	authorization, err := c.authorize(ctx, challenge)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, tagsURL, authorization)
}

func (c *registryTagsClient) do(ctx context.Context, rawURL, authorization string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	return c.httpClient.Do(req)
}

// authorize 按 WWW-Authenticate challenge 生成重试用的 Authorization 值。
// 支持 OCI 标准 Bearer realm（Harbor）与 Basic 直传；challenge 缺失时沿用
// 已配置的 Basic 凭据重试。
func (c *registryTagsClient) authorize(ctx context.Context, challenge string) (string, error) {
	scheme, params := parseChallenge(challenge)
	switch strings.ToLower(scheme) {
	case "bearer":
		realm, ok := params["realm"]
		if !ok {
			return "", fmt.Errorf("registry auth: Bearer challenge without realm")
		}
		token, err := c.fetchToken(ctx, realm, params["service"])
		if err != nil {
			return "", err
		}
		return "Bearer " + token, nil
	default:
		// Basic challenge 或无 challenge：有凭据则直传，匿名重试无意义。
		if c.basicAuth == "" {
			return "", fmt.Errorf("registry auth: %s challenge but no credentials configured", scheme)
		}
		return c.basicAuth, nil
	}
}

// fetchToken 走 OCI token 端点（Harbor 为 service/token），凭据存在时以 Basic
// 认证换取 pull scope 的 Bearer token。
func (c *registryTagsClient) fetchToken(ctx context.Context, realm, service string) (string, error) {
	q := url.Values{}
	q.Set("service", service)
	q.Set("scope", "repository:"+c.image+":pull")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	if c.basicAuth != "" {
		req.Header.Set("Authorization", c.basicAuth)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("registry auth token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry auth token: unexpected status %s", resp.Status)
	}
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("registry auth token: decode: %w", err)
	}
	if token := payload.Token; token != "" {
		return token, nil
	}
	if token := payload.AccessToken; token != "" {
		return token, nil
	}
	return "", fmt.Errorf("registry auth token: empty token")
}

// parseChallenge 解析 WWW-Authenticate 值，形如：
//
//	Bearer realm="https://host/service/token",service="harbor-registry"
//
// 返回 scheme 与小写键的参数表。
func parseChallenge(challenge string) (string, map[string]string) {
	parts := strings.SplitN(strings.TrimSpace(challenge), " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", nil
	}
	params := map[string]string{}
	if len(parts) == 2 {
		for _, kv := range strings.Split(parts[1], ",") {
			key, value, ok := strings.Cut(kv, "=")
			if !ok {
				continue
			}
			params[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	return parts[0], params
}
