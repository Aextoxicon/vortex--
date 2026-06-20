package main

import (
	"hash/fnv"
	"sync"
	"time"
)

const rateLimiterShards = 16

type rateLimiterShard struct {
	mu         sync.Mutex
	cache      map[string]int64
	failCounts map[string]int
}

type RateLimiter struct {
	shards    [rateLimiterShards]*rateLimiterShard
	stopCh    chan struct{}
	closeOnce sync.Once
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		stopCh: make(chan struct{}),
	}
	for i := 0; i < rateLimiterShards; i++ {
		rl.shards[i] = &rateLimiterShard{
			cache:      make(map[string]int64),
			failCounts: make(map[string]int),
		}
	}
	return rl
}

func (r *RateLimiter) getShard(key string) *rateLimiterShard {
	h := fnv.New32a()
	h.Write([]byte(key))
	return r.shards[h.Sum32()%rateLimiterShards]
}

// withShardLock 泛型辅助：获取分片 → 加锁 → 执行 → 返回结果
func withShardLock[T any](r *RateLimiter, key string, fn func(shard *rateLimiterShard) T) T {
	shard := r.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	return fn(shard)
}

func (r *RateLimiter) StartCleanup(interval, ttl time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.CleanupExpired(ttl)
			case <-r.stopCh:
				return
			}
		}
	}()
}

func (r *RateLimiter) Stop() {
	r.closeOnce.Do(func() { close(r.stopCh) })
}

func (r *RateLimiter) AllowRequest(publicID string) bool {
	return r.AllowRequestWithInterval(publicID, time.Second)
}

func (r *RateLimiter) AllowRequestWithInterval(publicID string, interval time.Duration) bool {
	return withShardLock(r, publicID, func(shard *rateLimiterShard) bool {
		now := time.Now().UnixNano()
		last, ok := shard.cache[publicID]
		if ok && now-last < interval.Nanoseconds() {
			return false
		}
		shard.cache[publicID] = now
		return true
	})
}

func (r *RateLimiter) CleanupExpired(ttl time.Duration) {
	now := time.Now().UnixNano()
	ttlNs := ttl.Nanoseconds()

	for _, shard := range r.shards {
		// 策略：先收集过期项，再批量删除，减少锁持有时间

		// 第1步：收集过期项（持有锁，但只读不删，很快）
		shard.mu.Lock()
		expiredItems := make([]string, 0, 50)
		for id, last := range shard.cache {
			if now-last > ttlNs {
				expiredItems = append(expiredItems, id)
				if len(expiredItems) >= 100 {
					break // 限制单次处理数量
				}
			}
		}
		shard.mu.Unlock()

		// 第2步：批量删除（再次加锁，但持有时间很短）
		if len(expiredItems) > 0 {
			shard.mu.Lock()
			for _, id := range expiredItems {
				// 双重检查：确认仍然过期
				if last, ok := shard.cache[id]; ok && now-last > ttlNs {
					delete(shard.cache, id)
					delete(shard.failCounts, id)
				}
			}
			shard.mu.Unlock()
		}
	}
}

func (r *RateLimiter) RecordFailure(key string) {
	withShardLock(r, key, func(shard *rateLimiterShard) struct{} {
		shard.failCounts[key]++
		return struct{}{}
	})
}

func (r *RateLimiter) ResetFailure(key string) {
	withShardLock(r, key, func(shard *rateLimiterShard) struct{} {
		delete(shard.failCounts, key)
		return struct{}{}
	})
}

func (r *RateLimiter) GetFailureCount(key string) int {
	return withShardLock(r, key, func(shard *rateLimiterShard) int {
		return shard.failCounts[key]
	})
}

func (r *RateLimiter) AllowRequestWithMaxFailures(publicID string, interval time.Duration, maxFailures int) bool {
	return withShardLock(r, publicID, func(shard *rateLimiterShard) bool {
		failCount := shard.failCounts[publicID]
		if failCount >= maxFailures {
			now := time.Now().UnixNano()
			last, ok := shard.cache[publicID]
			if ok && now-last < interval.Nanoseconds() {
				return false
			}
			shard.failCounts[publicID] = 0
		}
		return true
	})
}
