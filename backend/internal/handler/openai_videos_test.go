//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExtractVideoTaskID(t *testing.T) {
	cases := map[string]string{
		"/v1/videos/528af40e-9d31-91a9-9e1f-605a06cc6cd6":         "528af40e-9d31-91a9-9e1f-605a06cc6cd6",
		"/v1/videos/528af40e-9d31-91a9-9e1f-605a06cc6cd6/content": "528af40e-9d31-91a9-9e1f-605a06cc6cd6",
		"/v1/videos/abc/": "abc",
		"/v1/videos/":     "",
		"":                "",
	}
	for endpoint, want := range cases {
		require.Equalf(t, want, extractVideoTaskID(endpoint), "endpoint=%s", endpoint)
	}
}

type openAIVideosFakeCache struct {
	bindings map[string]service.VideoTaskBinding
}

func newOpenAIVideosFakeCache() *openAIVideosFakeCache {
	return &openAIVideosFakeCache{bindings: map[string]service.VideoTaskBinding{}}
}

func (c *openAIVideosFakeCache) SetVideoTaskBinding(_ context.Context, _ int64, key string, b service.VideoTaskBinding, _ time.Duration) error {
	c.bindings[key] = b
	return nil
}
func (c *openAIVideosFakeCache) GetVideoTaskBinding(_ context.Context, _ int64, key string) (service.VideoTaskBinding, error) {
	if v, ok := c.bindings[key]; ok {
		return v, nil
	}
	return service.VideoTaskBinding{}, redis.Nil
}
func (c *openAIVideosFakeCache) DeleteVideoTaskBinding(_ context.Context, _ int64, key string) error {
	delete(c.bindings, key)
	return nil
}
func (c *openAIVideosFakeCache) GetSessionAccountID(_ context.Context, _ int64, _ string) (int64, error) {
	return 0, redis.Nil
}
func (c *openAIVideosFakeCache) SetSessionAccountID(_ context.Context, _ int64, _ string, _ int64, _ time.Duration) error {
	return nil
}
func (c *openAIVideosFakeCache) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	return nil
}
func (c *openAIVideosFakeCache) DeleteSessionAccountID(_ context.Context, _ int64, _ string) error {
	return nil
}

func TestOpenAIVideosHandler_GetPollMissingBinding_Returns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(5510)
	cache := newOpenAIVideosFakeCache() // 空 → 所有 taskID miss
	cfg := &config.Config{RunMode: config.RunModeSimple}
	gatewayService := service.NewOpenAIGatewayService(
		nil, nil, nil, nil, nil, nil,
		cache, // 第 7 个参数：cache
		cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingService.Stop)
	concurrencyService := service.NewConcurrencyService(nil)
	handler := NewOpenAIGatewayHandler(
		gatewayService, concurrencyService, billingService,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil, cfg,
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/videos/528af40e-9d31-91a9-9e1f-605a06cc6cd6", nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID: 99, GroupID: &groupID,
		Group: &service.Group{ID: groupID},
		User:  &service.User{ID: 100},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 100, Concurrency: 0})

	handler.Videos(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "expired or not found")
}
