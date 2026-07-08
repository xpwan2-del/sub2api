//go:build unit

package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// fakeVideoCache 内存实现 GatewayCache，覆盖 video binding 与 sticky session。
type fakeVideoCache struct {
	mu       sync.Mutex
	bindings map[string]VideoTaskBinding
	sticky   map[string]int64
}

func newFakeVideoCache() *fakeVideoCache {
	return &fakeVideoCache{bindings: map[string]VideoTaskBinding{}, sticky: map[string]int64{}}
}

func bindingKey(groupID int64, taskID string) string {
	return "video_task:" + strconv.FormatInt(groupID, 10) + ":" + taskID
}

// stickySessionKey mirrors the production key derivation in
// OpenAIGatewayService.openAISessionCacheKey (prepends "openai:"), so the fake
// cache lookup reads the same key that setStickySessionAccountID writes.
// The raw VideoTaskSessionHash is namespaced by setStickySessionAccountID.
func stickySessionKey(taskID string) string {
	return "openai:" + VideoTaskSessionHash(taskID)
}

func (c *fakeVideoCache) SetVideoTaskBinding(_ context.Context, groupID int64, taskID string, b VideoTaskBinding, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindings[bindingKey(groupID, taskID)] = b
	return nil
}
func (c *fakeVideoCache) GetVideoTaskBinding(_ context.Context, groupID int64, taskID string) (VideoTaskBinding, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.bindings[bindingKey(groupID, taskID)]
	if !ok {
		return VideoTaskBinding{}, redis.Nil
	}
	return v, nil
}
func (c *fakeVideoCache) DeleteVideoTaskBinding(_ context.Context, groupID int64, taskID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.bindings, bindingKey(groupID, taskID))
	return nil
}

// 以下 session 方法满足 GatewayCache 接口的其余部分。
func (c *fakeVideoCache) GetSessionAccountID(_ context.Context, _ int64, key string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.sticky[key]
	if !ok {
		return 0, redis.Nil
	}
	return v, nil
}
func (c *fakeVideoCache) SetSessionAccountID(_ context.Context, _ int64, key string, accountID int64, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sticky[key] = accountID
	return nil
}
func (c *fakeVideoCache) RefreshSessionTTL(_ context.Context, _ int64, _ string, _ time.Duration) error {
	return nil
}
func (c *fakeVideoCache) DeleteSessionAccountID(_ context.Context, _ int64, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sticky, key)
	return nil
}

func TestVideoTaskSessionHash(t *testing.T) {
	if got := VideoTaskSessionHash(""); got != "" {
		t.Fatalf("empty taskID should yield empty hash, got %q", got)
	}
	if got := VideoTaskSessionHash("abc-123"); got != "video:abc-123" {
		t.Fatalf("unexpected hash %q", got)
	}
}

func TestIsTerminalOpenAIVideosTaskStatus(t *testing.T) {
	terminals := []string{"completed", "COMPLETED", "done", "succeeded", "success", "failed", "error", "cancelled", "canceled"}
	for _, s := range terminals {
		if !IsTerminalOpenAIVideosTaskStatus(s) {
			t.Errorf("expected %q to be terminal", s)
		}
	}
	for _, s := range []string{"queued", "in_progress", "running", ""} {
		if IsTerminalOpenAIVideosTaskStatus(s) {
			t.Errorf("expected %q to be non-terminal", s)
		}
	}
}

func newBindingService(cache GatewayCache) *OpenAIGatewayService {
	return &OpenAIGatewayService{cache: cache}
}

func TestBindAndGetVideoTaskModel(t *testing.T) {
	cache := newFakeVideoCache()
	svc := newBindingService(cache)
	groupID := int64(3)

	if _, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "t1"); ok {
		t.Fatal("expected miss before bind")
	}

	if err := svc.BindVideoTask(context.Background(), &groupID, "t1", 99, "sora-2", nil, time.Minute); err != nil {
		t.Fatalf("bind: %v", err)
	}

	model, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "t1")
	if !ok || model != "sora-2" {
		t.Fatalf("expected hit sora-2, got %q ok=%v", model, ok)
	}

	// 粘性账号也应被写入（setStickySessionAccountID 会用 "openai:" 前缀派生 key）
	sid, err := cache.GetSessionAccountID(context.Background(), groupID, stickySessionKey("t1"))
	if err != nil || sid != 99 {
		t.Fatalf("expected sticky account 99, got %d err=%v", sid, err)
	}
}

func TestGetVideoTaskModel_EmptyTaskID(t *testing.T) {
	svc := newBindingService(newFakeVideoCache())
	groupID := int64(3)
	if _, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "  "); ok {
		t.Fatal("empty taskID should miss")
	}
}

func TestUnbindVideoTask(t *testing.T) {
	cache := newFakeVideoCache()
	svc := newBindingService(cache)
	groupID := int64(3)
	_ = svc.BindVideoTask(context.Background(), &groupID, "t2", 7, "sora-2", nil, time.Minute)

	svc.UnbindVideoTask(context.Background(), &groupID, "t2")

	if _, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "t2"); ok {
		t.Fatal("expected miss after unbind")
	}
	if _, err := cache.GetSessionAccountID(context.Background(), groupID, stickySessionKey("t2")); !errors.Is(err, redis.Nil) {
		t.Fatal("expected sticky cleared after unbind")
	}
}
