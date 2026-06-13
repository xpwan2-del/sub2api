package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// upstreamPriceTestConnTimeout 是 TestConnection 探测请求的超时。
// 探测必须快速失败——不应阻塞管理员 UI 的"测试连接"动作。
const upstreamPriceTestConnTimeout = 10 * time.Second

// upstreamPriceAPIKeyMask 是 Get/List 返回时对 api_key 的脱敏占位符。
// 选择固定占位符而非部分明文（如 "sk-***ab"）：后者仍泄露密文长度后缀，
// 固定占位符更安全；前端如需判断"是否已配置"看该字段是否非空即可。
const upstreamPriceAPIKeyMask = "****"

// UpstreamPriceSourceService 上游价格来源管理服务。
//
// 职责：
//   - source 的 CRUD（Create/Update 时加密 api_key，Get/List 时脱敏）
//   - TestConnection：拉取上游定价接口并用 ParserByType 验证可解析性
//
// 加密策略（与 channel_monitor_service 一致）：
//   - Create/Update 入参的 APIKey 为明文，写库前用 SecretEncryptor.Encrypt 加密为密文
//   - 内部读取（TestConnection）用 Decrypt 还原明文用于 Bearer 头
//   - 对外返回（Get/List）一律脱敏为 upstreamPriceAPIKeyMask，不泄露密文也不泄露明文
type UpstreamPriceSourceService struct {
	repo     UpstreamPriceRepository
	encryptor SecretEncryptor
	httpClient HTTPUpstream
}

// NewUpstreamPriceSourceService 创建上游价格来源服务实例。
func NewUpstreamPriceSourceService(repo UpstreamPriceRepository, encryptor SecretEncryptor, httpClient HTTPUpstream) *UpstreamPriceSourceService {
	return &UpstreamPriceSourceService{repo: repo, encryptor: encryptor, httpClient: httpClient}
}

// Create 创建来源。入参 src.APIKey 为明文，写库前加密。
// 返回的记录 APIKey 已脱敏（调用方拿到的是 mask，不是密文也不是明文）。
func (s *UpstreamPriceSourceService) Create(ctx context.Context, src *dbent.UpstreamPriceSource) (*dbent.UpstreamPriceSource, error) {
	if src == nil {
		return nil, errors.New("source is nil")
	}
	if strings.TrimSpace(src.APIKey) != "" {
		enc, err := s.encryptor.Encrypt(strings.TrimSpace(src.APIKey))
		if err != nil {
			return nil, fmt.Errorf("encrypt api key: %w", err)
		}
		src.APIKey = enc
	}
	if err := s.repo.CreateSource(ctx, src); err != nil {
		return nil, fmt.Errorf("create upstream price source: %w", err)
	}
	src.APIKey = upstreamPriceAPIKeyMask
	return src, nil
}

// Update 更新来源。
//   - src.APIKey 非空：视为明文，重新加密后覆盖
//   - src.APIKey 为空：保持原密文（先查库回填密文，避免被空串覆盖）
func (s *UpstreamPriceSourceService) Update(ctx context.Context, src *dbent.UpstreamPriceSource) error {
	if src == nil {
		return errors.New("source is nil")
	}
	if src.ID == 0 {
		return errors.New("source id is required")
	}
	if strings.TrimSpace(src.APIKey) != "" {
		enc, err := s.encryptor.Encrypt(strings.TrimSpace(src.APIKey))
		if err != nil {
			return fmt.Errorf("encrypt api key: %w", err)
		}
		src.APIKey = enc
	} else {
		// 保持原密文：未提供新 api_key 时从库回填，避免被空串覆盖。
		existing, err := s.repo.GetSource(ctx, src.ID)
		if err != nil {
			return fmt.Errorf("load existing source for api_key preservation: %w", err)
		}
		src.APIKey = existing.APIKey
	}
	if err := s.repo.UpdateSource(ctx, src); err != nil {
		return fmt.Errorf("update upstream price source: %w", err)
	}
	src.APIKey = upstreamPriceAPIKeyMask
	return nil
}

// Delete 删除来源。
func (s *UpstreamPriceSourceService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.DeleteSource(ctx, id); err != nil {
		return fmt.Errorf("delete upstream price source: %w", err)
	}
	return nil
}

// Get 按 ID 查询来源。返回的 APIKey 已脱敏。
func (s *UpstreamPriceSourceService) Get(ctx context.Context, id int64) (*dbent.UpstreamPriceSource, error) {
	src, err := s.repo.GetSource(ctx, id)
	if err != nil {
		return nil, err
	}
	maskAPIKey(src)
	return src, nil
}

// List 列出全部来源。返回的 APIKey 均已脱敏。
func (s *UpstreamPriceSourceService) List(ctx context.Context) ([]*dbent.UpstreamPriceSource, error) {
	items, err := s.repo.ListSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list upstream price sources: %w", err)
	}
	for _, it := range items {
		maskAPIKey(it)
	}
	return items, nil
}

// ListEnabled 列出全部启用的来源。返回的 APIKey 均已脱敏。
func (s *UpstreamPriceSourceService) ListEnabled(ctx context.Context) ([]*dbent.UpstreamPriceSource, error) {
	items, err := s.repo.ListEnabledSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled upstream price sources: %w", err)
	}
	for _, it := range items {
		maskAPIKey(it)
	}
	return items, nil
}

// TestConnection 测试上游定价接口可达性 + 解析器能否产出非空结果。
//
// 流程：
//  1. 构造请求 URL = base_url + pricing_endpoint，带 Authorization: Bearer <解密后的 api_key>（若非空）
//  2. 用 httpClient 发起 GET，拿响应 body
//  3. 用 ParserByType(src.ParserType).Parse(body, ParserConfig{AliasMap}) 解析
//
// 返回值：
//   - reachable=true, err=nil：HTTP 2xx 且解析无错（modelCount 可能为 0，表示接口可达但无数据）
//   - reachable=false, err!=nil：HTTP 错误或解析错误
func (s *UpstreamPriceSourceService) TestConnection(ctx context.Context, src *dbent.UpstreamPriceSource) (reachable bool, modelCount int, err error) {
	if src == nil {
		return false, 0, errors.New("source is nil")
	}
	targetURL, err := buildSourceURL(src.BaseURL, src.PricingEndpoint)
	if err != nil {
		return false, 0, fmt.Errorf("invalid source url: %w", err)
	}

	plainKey, err := s.decryptAPIKey(src.APIKey)
	if err != nil {
		return false, 0, fmt.Errorf("decrypt api key: %w", err)
	}

	body, status, reqErr := s.fetchUpstreamBody(ctx, targetURL, plainKey)
	if reqErr != nil {
		return false, 0, fmt.Errorf("fetch upstream pricing: %w", reqErr)
	}
	if status < 200 || status >= 300 {
		return false, 0, fmt.Errorf("upstream returned status %d", status)
	}

	aliasMap := src.ModelAliasMap
	if aliasMap == nil {
		aliasMap = map[string]string{}
	}
	prices, parseErr := ParserByType(src.ParserType).Parse(body, ParserConfig{AliasMap: aliasMap})
	if parseErr != nil {
		// 接口可达但响应无法解析：视为 reachable=false（配置错误）
		return false, 0, fmt.Errorf("parse upstream response: %w", parseErr)
	}
	return true, len(prices), nil
}

// fetchUpstreamBody 发起 GET 请求并返回响应 body + status。
// 超时由 upstreamPriceTestConnTimeout 控制。
func (s *UpstreamPriceSourceService) fetchUpstreamBody(ctx context.Context, targetURL, plainKey string) ([]byte, int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, upstreamPriceTestConnTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if plainKey != "" {
		req.Header.Set("Authorization", "Bearer "+plainKey)
	}

	resp, doErr := s.httpClient.Do(req, "", 0, 0)
	if doErr != nil {
		return nil, 0, doErr
	}
	defer func() { _ = resp.Body.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8MB 上限，防止恶意超大响应
	if readErr != nil {
		return nil, resp.StatusCode, readErr
	}
	return body, resp.StatusCode, nil
}

// decryptAPIKey 把密文 api_key 解密为明文。空串返回空串（无需解密）。
func (s *UpstreamPriceSourceService) decryptAPIKey(cipher string) (string, error) {
	if strings.TrimSpace(cipher) == "" {
		return "", nil
	}
	return s.encryptor.Decrypt(cipher)
}

// maskAPIKey 把单条记录的 APIKey 替换为脱敏占位符。
// 仅当原值非空时替换，保持"未配置"与"已配置"的区分能力。
func maskAPIKey(src *dbent.UpstreamPriceSource) {
	if src == nil {
		return
	}
	if src.APIKey != "" {
		src.APIKey = upstreamPriceAPIKeyMask
	}
}

// buildSourceURL 拼接 base_url + pricing_endpoint 为完整 URL 并校验合法性。
func buildSourceURL(baseURL, endpoint string) (string, error) {
	base := strings.TrimSpace(baseURL)
	ep := strings.TrimSpace(endpoint)
	if ep == "" {
		ep = "/api/pricing"
	}
	// 若 endpoint 已是绝对 URL（含 scheme），直接使用。
	if u, err := url.Parse(ep); err == nil && u.IsAbs() {
		return ep, nil
	}
	if base == "" {
		return "", errors.New("base_url is empty")
	}
	if !strings.HasPrefix(ep, "/") {
		ep = "/" + ep
	}
	full := base + ep
	if _, err := url.Parse(full); err != nil {
		return "", err
	}
	return full, nil
}
