package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func (s *OpenAIGatewayService) ForwardVideos(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	method string,
	endpoint string,
	body []byte,
	contentType string,
	requestModel string,
	mappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	originalModel := strings.TrimSpace(requestModel)
	if originalModel == "" {
		writeOpenAIVideosError(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return nil, fmt.Errorf("missing model in request")
	}

	billingModel := resolveOpenAIForwardModel(account, originalModel, mappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	upstreamBody := body
	upstreamContentType := contentType
	if len(body) > 0 && upstreamModel != originalModel {
		var err error
		upstreamBody, upstreamContentType, err = rewriteOpenAIVideosModel(body, contentType, upstreamModel)
		if err != nil {
			return nil, err
		}
	}

	apiKey := account.GetOpenAIApiKey()
	if apiKey == "" {
		return nil, fmt.Errorf("account %d missing api_key", account.ID)
	}
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildOpenAIEndpointURL(validatedURL, endpoint)

	var reader io.Reader
	if len(upstreamBody) > 0 {
		reader = bytes.NewReader(upstreamBody)
	}
	upstreamReq, err := http.NewRequestWithContext(ctx, method, targetURL, reader)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq = upstreamReq.WithContext(WithHTTPUpstreamProfile(upstreamReq.Context(), HTTPUpstreamProfileOpenAI))
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	upstreamReq.Header.Set("Accept", firstNonEmptyString(c.GetHeader("Accept"), "application/json"))
	if upstreamContentType != "" && len(upstreamBody) > 0 {
		upstreamReq.Header.Set("Content-Type", upstreamContentType)
	}
	for key, values := range c.Request.Header {
		lowerKey := strings.ToLower(key)
		if !openaiPassthroughAllowedHeaders[lowerKey] {
			continue
		}
		for _, value := range values {
			upstreamReq.Header.Add(key, value)
		}
	}
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		upstreamReq.Header.Set("User-Agent", customUA)
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		writeOpenAIVideosError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	isContentEndpoint := isOpenAIVideosContentEndpoint(endpoint)
	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, upstreamModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		if isContentEndpoint && isOpenAIVideosContentFallbackStatus(resp.StatusCode) {
			mediaResp, fallbackErr := s.forwardOpenAIVideosContentFromTaskURL(ctx, account, validatedURL, endpoint, apiKey, proxyURL)
			if fallbackErr == nil {
				defer func() { _ = mediaResp.Body.Close() }()
				writeOpenAIVideosUpstreamStream(c, mediaResp, s.responseHeaderFilter)
				return &OpenAIForwardResult{
					RequestID:       firstNonEmptyString(mediaResp.Header.Get("x-request-id"), mediaResp.Header.Get("request-id")),
					Model:           originalModel,
					BillingModel:    billingModel,
					UpstreamModel:   upstreamModel,
					ResponseHeaders: mediaResp.Header.Clone(),
					Stream:          false,
					Duration:        time.Since(startTime),
					VideoCount:      1,
				}, nil
			}
			if !c.Writer.Written() {
				writeOpenAIVideosError(c, http.StatusBadGateway, "upstream_error", "Video content is unavailable")
			}
			return nil, fmt.Errorf("openai videos content fallback failed after upstream status %d: %w", resp.StatusCode, fallbackErr)
		}
		writeOpenAIVideosUpstreamResponse(c, resp, respBody, s.responseHeaderFilter)
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	if isContentEndpoint {
		writeOpenAIVideosUpstreamStream(c, resp, s.responseHeaderFilter)
		return &OpenAIForwardResult{
			RequestID:       firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id")),
			Model:           originalModel,
			BillingModel:    billingModel,
			UpstreamModel:   upstreamModel,
			ResponseHeaders: resp.Header.Clone(),
			Stream:          false,
			Duration:        time.Since(startTime),
			VideoCount:      1,
		}, nil
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if !strings.Contains(err.Error(), "response body too large") {
			writeOpenAIVideosError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	writeOpenAIVideosUpstreamResponse(c, resp, respBody, s.responseHeaderFilter)

	return &OpenAIForwardResult{
		RequestID:       firstNonEmptyString(resp.Header.Get("x-request-id"), resp.Header.Get("request-id")),
		Model:           originalModel,
		BillingModel:    billingModel,
		UpstreamModel:   upstreamModel,
		ResponseHeaders: resp.Header.Clone(),
		Stream:          false,
		Duration:        time.Since(startTime),
		VideoCount:      1,
	}, nil
}

func (s *OpenAIGatewayService) forwardOpenAIVideosContentFromTaskURL(
	ctx context.Context,
	account *Account,
	validatedBaseURL string,
	contentEndpoint string,
	apiKey string,
	proxyURL string,
) (*http.Response, error) {
	taskEndpoint, ok := openAIVideosTaskEndpointFromContent(contentEndpoint)
	if !ok {
		return nil, fmt.Errorf("invalid videos content endpoint: %s", contentEndpoint)
	}

	taskReq, err := http.NewRequestWithContext(ctx, http.MethodGet, buildOpenAIEndpointURL(validatedBaseURL, taskEndpoint), nil)
	if err != nil {
		return nil, fmt.Errorf("build video task request: %w", err)
	}
	taskReq = taskReq.WithContext(WithHTTPUpstreamProfile(taskReq.Context(), HTTPUpstreamProfileOpenAI))
	taskReq.Header.Set("Authorization", "Bearer "+apiKey)
	taskReq.Header.Set("Accept", "application/json")
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		taskReq.Header.Set("User-Agent", customUA)
	}

	taskResp, err := s.httpUpstream.Do(taskReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("video task request failed: %w", err)
	}
	defer func() { _ = taskResp.Body.Close() }()
	if taskResp.StatusCode >= http.StatusBadRequest {
		body := s.readUpstreamErrorBody(taskResp)
		return nil, fmt.Errorf("video task returned status %d: %s", taskResp.StatusCode, sanitizeUpstreamErrorMessage(string(body)))
	}

	taskBody, err := ReadUpstreamResponseBody(taskResp.Body, s.cfg, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("read video task response: %w", err)
	}
	if status := openAIVideosTaskStatus(taskBody); status != "" && !isCompletedOpenAIVideosTaskStatus(status) {
		return nil, fmt.Errorf("video task is not completed: %s", status)
	}
	mediaURL := extractOpenAIVideosContentURL(taskBody)
	if mediaURL == "" {
		return nil, fmt.Errorf("video task response does not include a media url")
	}
	if err := validateOpenAIVideosMediaURL(ctx, mediaURL); err != nil {
		return nil, err
	}

	mediaReq, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build video media request: %w", err)
	}
	mediaReq = mediaReq.WithContext(WithHTTPUpstreamProfile(mediaReq.Context(), HTTPUpstreamProfileOpenAI))
	mediaReq.Header.Set("Accept", "video/*, application/octet-stream, */*")
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		mediaReq.Header.Set("User-Agent", customUA)
	} else {
		mediaReq.Header.Set("User-Agent", "sub2api-openai-videos")
	}

	mediaResp, err := s.httpUpstream.Do(mediaReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, fmt.Errorf("video media request failed: %w", err)
	}
	if mediaResp.StatusCode < http.StatusOK || mediaResp.StatusCode >= http.StatusMultipleChoices {
		body := s.readUpstreamErrorBody(mediaResp)
		_ = mediaResp.Body.Close()
		return nil, fmt.Errorf("video media returned status %d: %s", mediaResp.StatusCode, sanitizeUpstreamErrorMessage(string(body)))
	}
	if !isOpenAIVideosMediaContentType(mediaResp.Header.Get("Content-Type")) {
		_ = mediaResp.Body.Close()
		return nil, fmt.Errorf("video media content-type is not supported: %s", mediaResp.Header.Get("Content-Type"))
	}
	if mediaResp.Header.Get("Content-Type") == "" {
		mediaResp.Header.Set("Content-Type", "video/mp4")
	}
	return mediaResp, nil
}

func isOpenAIVideosContentEndpoint(endpoint string) bool {
	return strings.HasSuffix(strings.TrimRight(strings.TrimSpace(endpoint), "/"), "/content")
}

func isOpenAIVideosContentFallbackStatus(statusCode int) bool {
	return statusCode == http.StatusNotFound || statusCode == http.StatusMethodNotAllowed
}

func openAIVideosTaskEndpointFromContent(endpoint string) (string, bool) {
	trimmed := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if !strings.HasSuffix(trimmed, "/content") {
		return "", false
	}
	taskEndpoint := strings.TrimSuffix(trimmed, "/content")
	return taskEndpoint, strings.TrimSpace(taskEndpoint) != ""
}

func openAIVideosTaskStatus(body []byte) string {
	for _, path := range []string{"status", "data.status", "task.status", "result.status"} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func isCompletedOpenAIVideosTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done", "completed", "succeeded", "success":
		return true
	default:
		return false
	}
}

func extractOpenAIVideosContentURL(body []byte) string {
	for _, path := range []string{
		"video.url",
		"data.video.url",
		"url",
		"data.url",
		"output.video_url",
		"output.url",
		"output.0.url",
		"data.output.0.url",
		"videos.0.url",
		"data.videos.0.url",
		"result.video_url",
		"result.video.url",
		"content.video_url",
		"content.video.url",
		"data.content.video_url",
	} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func validateOpenAIVideosMediaURL(ctx context.Context, raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid video media url: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("invalid video media url scheme: %s", parsed.Scheme)
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("video media url host is empty")
	}
	if parsed.User != nil {
		return fmt.Errorf("video media url userinfo is not allowed")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resolveCtx, cancel := context.WithTimeout(ctx, monitorEndpointResolveTimeout)
	defer cancel()
	blocked, err := isPrivateOrLoopbackHost(resolveCtx, parsed.Hostname())
	if err != nil {
		return fmt.Errorf("resolve video media url host: %w", err)
	}
	if blocked {
		return fmt.Errorf("video media url host is private or blocked")
	}
	return nil
}

func isOpenAIVideosMediaContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		mediaType = strings.TrimSpace(contentType)
	}
	mediaType = strings.ToLower(mediaType)
	if mediaType == "" {
		return true
	}
	return strings.HasPrefix(mediaType, "video/") ||
		mediaType == "application/octet-stream" ||
		mediaType == "application/mp4" ||
		mediaType == "binary/octet-stream"
}

func writeOpenAIVideosUpstreamResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil || c.Writer.Written() {
		return
	}
	if resp.Header != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, filter)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Writer.Header().Set("Content-Type", ct)
	} else {
		c.Writer.Header().Set("Content-Type", "application/json")
	}
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(body)
}

func writeOpenAIVideosUpstreamStream(c *gin.Context, resp *http.Response, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil || c.Writer.Written() {
		return
	}
	if resp.Header != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, filter)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Writer.Header().Set("Content-Type", ct)
	}
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}

func writeOpenAIVideosError(c *gin.Context, statusCode int, errType, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

func rewriteOpenAIVideosModel(body []byte, contentType string, model string) ([]byte, string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return body, contentType, nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		return rewriteOpenAIVideosMultipartModel(body, contentType, model)
	}
	return ReplaceModelInBody(body, model), contentType, nil
}

func rewriteOpenAIVideosMultipartModel(body []byte, contentType string, model string) ([]byte, string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", fmt.Errorf("parse multipart content-type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, "", fmt.Errorf("multipart boundary is required")
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	modelWritten := false

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("read multipart body: %w", err)
		}
		partHeader := cloneOpenAIVideosMultipartHeader(part.Header)
		target, err := writer.CreatePart(partHeader)
		if err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("create multipart part: %w", err)
		}
		if strings.TrimSpace(part.FormName()) == "model" && part.FileName() == "" {
			if _, err := target.Write([]byte(model)); err != nil {
				_ = part.Close()
				return nil, "", fmt.Errorf("rewrite multipart model: %w", err)
			}
			modelWritten = true
			_ = part.Close()
			continue
		}
		if _, err := io.Copy(target, part); err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("copy multipart part: %w", err)
		}
		_ = part.Close()
	}
	if !modelWritten {
		if err := writer.WriteField("model", model); err != nil {
			return nil, "", fmt.Errorf("append multipart model field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize multipart body: %w", err)
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func cloneOpenAIVideosMultipartHeader(src textproto.MIMEHeader) textproto.MIMEHeader {
	dst := make(textproto.MIMEHeader, len(src))
	for key, values := range src {
		copied := make([]string, len(values))
		copy(copied, values)
		dst[key] = copied
	}
	return dst
}
