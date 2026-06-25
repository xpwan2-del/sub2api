// bundle_rpm_cache.go 套餐 RPM 计数器 Redis 实现。
//
// 设计说明：
//   - key 形式：rpm:bundle:{bundleSubID}:{minute}
//   - 时间来源：rdb.Time()（Redis 服务端时间），避免多实例时钟漂移。
//   - 原子操作：TxPipeline (MULTI/EXEC) 执行 INCR+EXPIRE，兼容 Redis Cluster。
//   - TTL：120s，覆盖当前分钟窗口 + 少量冗余。

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	bundleRPMKeyPrefix = "rpm:bundle:"
	bundleRPMKeyTTL    = 120 * time.Second
)

type bundleRPMCacheImpl struct {
	rdb *redis.Client
}

// NewBundleRPMCache 创建套餐 RPM 计数器。
func NewBundleRPMCache(rdb *redis.Client) service.BundleRPMCache {
	return &bundleRPMCacheImpl{rdb: rdb}
}

func (c *bundleRPMCacheImpl) minuteTS(ctx context.Context) (int64, error) {
	t, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return 0, fmt.Errorf("redis TIME: %w", err)
	}
	return t.Unix() / 60, nil
}

func (c *bundleRPMCacheImpl) atomicIncr(ctx context.Context, key string) (int, error) {
	pipe := c.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, bundleRPMKeyTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("bundle rpm increment: %w", err)
	}
	return int(incr.Val()), nil
}

// IncrementBundleRPM 递增套餐订阅的分钟计数。
func (c *bundleRPMCacheImpl) IncrementBundleRPM(ctx context.Context, bundleSubID int64) (int, error) {
	minute, err := c.minuteTS(ctx)
	if err != nil {
		return 0, err
	}
	key := fmt.Sprintf("%s%d:%d", bundleRPMKeyPrefix, bundleSubID, minute)
	return c.atomicIncr(ctx, key)
}

// GetBundleRPM 获取当前分钟已用 RPM（只读）。
func (c *bundleRPMCacheImpl) GetBundleRPM(ctx context.Context, bundleSubID int64) (int, error) {
	minute, err := c.minuteTS(ctx)
	if err != nil {
		return 0, err
	}
	key := fmt.Sprintf("%s%d:%d", bundleRPMKeyPrefix, bundleSubID, minute)
	val, err := c.rdb.Get(ctx, key).Int()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("bundle rpm get: %w", err)
	}
	return val, nil
}
