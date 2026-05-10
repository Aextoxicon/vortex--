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
	shards [rateLimiterShards]*rateLimiterShard
	stopCh chan struct{}
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
	close(r.stopCh)
}

func (r *RateLimiter) AllowRequest(publicID string) bool {
	return r.AllowRequestWithInterval(publicID, time.Second)
}

func (r *RateLimiter) AllowRequestWithInterval(publicID string, interval time.Duration) bool {
	shard := r.getShard(publicID)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	now := time.Now().UnixNano()
	last, ok := shard.cache[publicID]
	if ok && now-last < interval.Nanoseconds() {
		return false
	}
	shard.cache[publicID] = now
	return true
}

func (r *RateLimiter) CleanupExpired(ttl time.Duration) {
	now := time.Now().UnixNano()
	ttlNs := ttl.Nanoseconds()

	for _, shard := range r.shards {
		shard.mu.Lock()
		for id, last := range shard.cache {
			if now-last > ttlNs {
				delete(shard.cache, id)
				delete(shard.failCounts, id)
			}
		}
		shard.mu.Unlock()
	}
}

func (r *RateLimiter) RecordFailure(key string) {
	shard := r.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	shard.failCounts[key]++
}

func (r *RateLimiter) ResetFailure(key string) {
	shard := r.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	delete(shard.failCounts, key)
}

func (r *RateLimiter) GetFailureCount(key string) int {
	shard := r.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	return shard.failCounts[key]
}

func (r *RateLimiter) AllowRequestWithMaxFailures(publicID string, interval time.Duration, maxFailures int) bool {
	shard := r.getShard(publicID)
	shard.mu.Lock()
	defer shard.mu.Unlock()

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
}
