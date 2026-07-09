// bundle_concurrency_cache.go 套餐并发数 Redis 实现。
//
// 设计说明：
//   - key 形式：concurrency:bundle:{bundleSubID}
//   - 原子操作：INCR/DECR，通过 TxPipeline 保证原子性。
//   - TTL：300s，覆盖最长请求时间 + 冗余，防止客户端断连导致计数器泄漏。
//   - goroutine-safe：Redis 单线程模型天然保证原子性。

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	bundleConcurrencyKeyPrefix = "concurrency:bundle:"
	bundleConcurrencyKeyTTL    = 300 * time.Second
)

type bundleConcurrencyCacheImpl struct {
	rdb *redis.Client
}

// NewBundleConcurrencyCache 创建套餐并发数计数器。
func NewBundleConcurrencyCache(rdb *redis.Client) service.BundleConcurrencyCache {
	return &bundleConcurrencyCacheImpl{rdb: rdb}
}

// Increment 递增并发计数，设置 TTL 防止泄漏。
func (c *bundleConcurrencyCacheImpl) Increment(ctx context.Context, bundleSubID int64) (int64, error) {
	key := fmt.Sprintf("%s%d", bundleConcurrencyKeyPrefix, bundleSubID)
	pipe := c.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, bundleConcurrencyKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("bundle concurrency increment: %w", err)
	}
	return incr.Val(), nil
}

// Decrement 递减并发计数。如果计数已为 0 或 key 不存在则返回 0。
func (c *bundleConcurrencyCacheImpl) Decrement(ctx context.Context, bundleSubID int64) (int64, error) {
	key := fmt.Sprintf("%s%d", bundleConcurrencyKeyPrefix, bundleSubID)
	// Use Lua to avoid negative counts.
	script := redis.NewScript(`
		local v = redis.call('DECR', KEYS[1])
		if v < 0 then
			redis.call('SET', KEYS[1], 0)
			v = 0
		end
		return v
	`)
	result, err := script.Run(ctx, c.rdb, []string{key}).Int64()
	if err != nil {
		return 0, fmt.Errorf("bundle concurrency decrement: %w", err)
	}
	return result, nil
}

// Get 获取当前并发计数（只读）。
func (c *bundleConcurrencyCacheImpl) Get(ctx context.Context, bundleSubID int64) (int64, error) {
	key := fmt.Sprintf("%s%d", bundleConcurrencyKeyPrefix, bundleSubID)
	val, err := c.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("bundle concurrency get: %w", err)
	}
	return val, nil
}
