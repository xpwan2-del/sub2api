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
	userA := int64(10)

	if _, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "t1", nil, &userA); ok {
		t.Fatal("expected miss before bind")
	}

	if err := svc.BindVideoTask(context.Background(), &groupID, "t1", 99, "sora-2", nil, &userA, time.Minute); err != nil {
		t.Fatalf("bind: %v", err)
	}

	model, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "t1", nil, &userA)
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
	if _, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "  ", nil, nil); ok {
		t.Fatal("empty taskID should miss")
	}
}

func TestUnbindVideoTask(t *testing.T) {
	cache := newFakeVideoCache()
	svc := newBindingService(cache)
	groupID := int64(3)
	userA := int64(10)
	_ = svc.BindVideoTask(context.Background(), &groupID, "t2", 7, "sora-2", nil, &userA, time.Minute)

	svc.UnbindVideoTask(context.Background(), &groupID, "t2")

	if _, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "t2", nil, &userA); ok {
		t.Fatal("expected miss after unbind")
	}
	if _, err := cache.GetSessionAccountID(context.Background(), groupID, stickySessionKey("t2")); !errors.Is(err, redis.Nil) {
		t.Fatal("expected sticky cleared after unbind")
	}
}

// TestGetVideoTaskModel_BundleOwnershipIDOR 验证 bundle key 查询视频 task 时的订阅归属校验：
// 订阅 B 不得读取订阅 A 创建的视频任务（防跨订阅 IDOR）。该校验补齐了带 ?model= 的 GET
// 走 model 路由路径的缺口——中间件 ResolveGroup 不校验 task 归属，handler 统一把关，
// 与无 model 反查路径（ResolveGroupByVideoTask）语义一致。
func TestGetVideoTaskModel_BundleOwnershipIDOR(t *testing.T) {
	cache := newFakeVideoCache()
	svc := newBindingService(cache)
	groupID := int64(3)
	userA := int64(10)
	subA := int64(100)
	subB := int64(200)

	// 订阅 A 创建 task（同一 group，模拟两订阅共享上游账号池的常见配置）。
	if err := svc.BindVideoTask(context.Background(), &groupID, "taskX", 99, "sora-2", &subA, &userA, time.Minute); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// 归属订阅 A 查询 → 命中。
	if model, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "taskX", &subA, nil); !ok || model != "sora-2" {
		t.Fatalf("owner subscription should hit, got model=%q ok=%v", model, ok)
	}

	// 订阅 B 查询同一 task → 必须拒绝（跨订阅 IDOR）。
	if _, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "taskX", &subB, nil); ok {
		t.Fatal("non-owner bundle subscription must not read another subscription's video task (IDOR)")
	}

	// bundle key 查询标准 key 创建的 task（binding.BundleSubID=nil）→ 拒绝：
	// bundle key 只能查本订阅创建的 task。
	if err := svc.BindVideoTask(context.Background(), &groupID, "taskStd", 7, "sora-2", nil, &userA, time.Minute); err != nil {
		t.Fatalf("bind std: %v", err)
	}
	if _, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "taskStd", &subA, nil); ok {
		t.Fatal("bundle key must not read a task created by a standard key")
	}
}

// TestGetVideoTaskModel_StandardKeyCrossUser 验证标准 Key 查询视频 task 时的用户归属校验：
// 同一 group 下，用户 B 的标准 Key 不得读取用户 A 创建的视频任务（防同 group 跨用户 IDOR）。
// 标准 Key 无订阅维度，仅靠 groupID 无法隔离共享同一渠道组的多个用户，故按 userID 校验。
func TestGetVideoTaskModel_StandardKeyCrossUser(t *testing.T) {
	cache := newFakeVideoCache()
	svc := newBindingService(cache)
	groupID := int64(3)
	userA := int64(10)
	userB := int64(20)

	// 用户 A 的标准 Key 创建 task（bundleSubID=nil，同一 group 模拟两用户共享渠道组）。
	if err := svc.BindVideoTask(context.Background(), &groupID, "taskY", 99, "sora-2", nil, &userA, time.Minute); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// 用户 A 查询 → 命中。
	if _, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "taskY", nil, &userA); !ok {
		t.Fatal("owner user should hit own task")
	}

	// 用户 B 查询同一 task → 必须拒绝（同 group 跨用户 IDOR）。
	if _, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "taskY", nil, &userB); ok {
		t.Fatal("different user's standard key must not read another user's video task (IDOR)")
	}

	// userID 为 nil（调用方漏传）→ fail-closed 拒绝，避免绕过归属校验。
	if _, ok := svc.GetVideoTaskModel(context.Background(), &groupID, "taskY", nil, nil); ok {
		t.Fatal("nil userID must not bypass ownership check (fail-closed)")
	}
}
