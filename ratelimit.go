package main

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu     sync.Mutex
	cache  map[string]int64
	stopCh chan struct{}
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		cache:  make(map[string]int64),
		stopCh: make(chan struct{}),
	}
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
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UnixNano()
	last, ok := r.cache[publicID]
	if ok && now-last < interval.Nanoseconds() {
		return false
	}
	r.cache[publicID] = now
	return true
}

func (r *RateLimiter) CleanupExpired(ttl time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UnixNano()
	for id, last := range r.cache {
		if now-last > ttl.Nanoseconds() {
			delete(r.cache, id)
		}
	}
}
