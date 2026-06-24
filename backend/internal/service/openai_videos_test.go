package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayService_ForwardVideosContentStreamsDirectResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec, c := newOpenAIVideosTestContext()
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIVideosTestResponse(http.StatusOK, "video/mp4", "direct-video"),
	}}
	svc := newOpenAIVideosTestService(upstream)

	result, err := svc.ForwardVideos(context.Background(), c, newOpenAIVideosTestAccount(), http.MethodGet, "/v1/videos/task_123/content", nil, "", "grok-imagine-video", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "video/mp4", rec.Header().Get("Content-Type"))
	require.Equal(t, "direct-video", rec.Body.String())
	require.Len(t, upstream.requests, 1)
	require.Equal(t, "https://upstream.example.com/v1/videos/task_123/content", upstream.requests[0].URL.String())
	require.Equal(t, 1, result.VideoCount)
}

func TestOpenAIGatewayService_ForwardVideosContentFallsBackToTaskVideoURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec, c := newOpenAIVideosTestContext()
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIVideosTestResponse(http.StatusNotFound, "application/json", `{"error":{"message":"not found"}}`),
		openAIVideosTestResponse(http.StatusOK, "application/json", `{"id":"task_123","status":"done","video":{"url":"https://93.184.216.34/generated.mp4"}}`),
		openAIVideosTestResponse(http.StatusOK, "video/mp4", "fallback-video"),
	}}
	svc := newOpenAIVideosTestService(upstream)

	result, err := svc.ForwardVideos(context.Background(), c, newOpenAIVideosTestAccount(), http.MethodGet, "/v1/videos/task_123/content", nil, "", "grok-imagine-video", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "video/mp4", rec.Header().Get("Content-Type"))
	require.Equal(t, "fallback-video", rec.Body.String())
	require.Len(t, upstream.requests, 3)
	require.Equal(t, "https://upstream.example.com/v1/videos/task_123/content", upstream.requests[0].URL.String())
	require.Equal(t, "https://upstream.example.com/v1/videos/task_123", upstream.requests[1].URL.String())
	require.Equal(t, "https://93.184.216.34/generated.mp4", upstream.requests[2].URL.String())
	require.Equal(t, "Bearer sk-test", upstream.requests[1].Header.Get("Authorization"))
	require.Empty(t, upstream.requests[2].Header.Get("Authorization"))
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.requests[2].Context()))
	require.Equal(t, 1, result.VideoCount)
}

func TestOpenAIGatewayService_ForwardVideosContentDoesNotFallbackForAuthErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec, c := newOpenAIVideosTestContext()
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIVideosTestResponse(http.StatusUnauthorized, "application/json", `{"error":{"message":"invalid api key"}}`),
	}}
	svc := newOpenAIVideosTestService(upstream)

	result, err := svc.ForwardVideos(context.Background(), c, newOpenAIVideosTestAccount(), http.MethodGet, "/v1/videos/task_123/content", nil, "", "grok-imagine-video", "")
	require.Error(t, err)
	require.Nil(t, result)
	require.NotEqual(t, "fallback-video", rec.Body.String())
	require.Len(t, upstream.requests, 1)
}

func TestOpenAIGatewayService_ForwardVideosContentDoesNotDownloadPendingTask(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec, c := newOpenAIVideosTestContext()
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIVideosTestResponse(http.StatusNotFound, "application/json", `{"error":{"message":"not found"}}`),
		openAIVideosTestResponse(http.StatusOK, "application/json", `{"id":"task_123","status":"in_progress","video":{"url":"https://93.184.216.34/generated.mp4"}}`),
	}}
	svc := newOpenAIVideosTestService(upstream)

	result, err := svc.ForwardVideos(context.Background(), c, newOpenAIVideosTestAccount(), http.MethodGet, "/v1/videos/task_123/content", nil, "", "grok-imagine-video", "")
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Len(t, upstream.requests, 2)
}

func TestOpenAIGatewayService_ForwardVideosContentRejectsPrivateMediaURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec, c := newOpenAIVideosTestContext()
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		openAIVideosTestResponse(http.StatusNotFound, "application/json", `{"error":{"message":"not found"}}`),
		openAIVideosTestResponse(http.StatusOK, "application/json", `{"id":"task_123","status":"done","video":{"url":"http://localhost/generated.mp4"}}`),
	}}
	svc := newOpenAIVideosTestService(upstream)

	result, err := svc.ForwardVideos(context.Background(), c, newOpenAIVideosTestAccount(), http.MethodGet, "/v1/videos/task_123/content", nil, "", "grok-imagine-video", "")
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Len(t, upstream.requests, 2)
}

func TestExtractOpenAIVideosContentURL(t *testing.T) {
	for _, item := range []struct {
		name string
		body string
		want string
	}{
		{"video_url", `{"video":{"url":"https://example.com/video.mp4"}}`, "https://example.com/video.mp4"},
		{"data_video_url", `{"data":{"video":{"url":"https://example.com/data.mp4"}}}`, "https://example.com/data.mp4"},
		{"root_url", `{"url":"https://example.com/root.mp4"}`, "https://example.com/root.mp4"},
		{"output_first_url", `{"output":[{"url":"https://example.com/output.mp4"}]}`, "https://example.com/output.mp4"},
		{"videos_first_url", `{"videos":[{"url":"https://example.com/videos.mp4"}]}`, "https://example.com/videos.mp4"},
		{"content_video_url", `{"content":{"video_url":"https://example.com/content.mp4"}}`, "https://example.com/content.mp4"},
	} {
		t.Run(item.name, func(t *testing.T) {
			require.Equal(t, item.want, extractOpenAIVideosContentURL([]byte(item.body)))
		})
	}
}

func newOpenAIVideosTestService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}
}

func newOpenAIVideosTestAccount() *Account {
	return &Account{
		ID:          1,
		Name:        "openai-video-test",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://upstream.example.com",
		},
		Status:      StatusActive,
		Schedulable: true,
	}
}

func newOpenAIVideosTestContext() (*httptest.ResponseRecorder, *gin.Context) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_123/content?model=grok-imagine-video", nil)
	return rec, c
}

func openAIVideosTestResponse(statusCode int, contentType string, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
