package main

import (
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_AllowRequest(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	key := "test_user"

	// First request should be allowed
	if !rl.AllowRequest(key) {
		t.Error("expected first request to be allowed")
	}

	// Immediate second request should be rejected (1 second interval)
	if rl.AllowRequest(key) {
		t.Error("expected second request to be rejected")
	}

	// After 1 second delay, should be allowed
	time.Sleep(time.Second + 50*time.Millisecond)
	if !rl.AllowRequest(key) {
		t.Error("expected request after delay to be allowed")
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	key := "concurrent_user"
	const requests = 100

	allowed := make(chan bool, requests)
	var wg sync.WaitGroup

	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- rl.AllowRequest(key)
		}()
	}

	wg.Wait()
	close(allowed)

	// Only one request should be allowed in 1 second
	allowedCount := 0
	for ok := range allowed {
		if ok {
			allowedCount++
		}
	}

	if allowedCount != 1 {
		t.Errorf("expected 1 allowed request, got %d", allowedCount)
	}
}

func TestRateLimiter_RecordFailure(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	key := "failure_test"

	// Record failures
	rl.RecordFailure(key)
	rl.RecordFailure(key)
	rl.RecordFailure(key)

	count := rl.GetFailureCount(key)
	if count != 3 {
		t.Errorf("expected 3 failures, got %d", count)
	}

	// Reset failure count
	rl.ResetFailure(key)

	count = rl.GetFailureCount(key)
	if count != 0 {
		t.Errorf("expected 0 failures after reset, got %d", count)
	}
}

func TestRateLimiter_AllowRequestWithMaxFailures(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	key := "max_failures_test"

	// Record 3 failures
	rl.RecordFailure(key)
	rl.RecordFailure(key)
	rl.RecordFailure(key)

	// First call after failures should be allowed (it resets the counter)
	if !rl.AllowRequestWithMaxFailures(key, time.Second, 3) {
		t.Error("expected request to be allowed after reaching max failures (counter resets)")
	}

	// Record 3 more failures
	rl.RecordFailure(key)
	rl.RecordFailure(key)
	rl.RecordFailure(key)

	// Make a request to set the timestamp in cache
	rl.AllowRequest(key)

	// Record 3 more failures
	rl.RecordFailure(key)
	rl.RecordFailure(key)
	rl.RecordFailure(key)

	// Now should be rejected within interval
	if rl.AllowRequestWithMaxFailures(key, time.Second, 3) {
		t.Error("expected request to be rejected within interval after reaching max failures again")
	}
}

func TestRateLimiter_CleanupExpired(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	key := "cleanup_test"

	// Record a request
	rl.AllowRequest(key)

	// Wait for expiration
	time.Sleep(2 * time.Second)

	// Cleanup
	rl.CleanupExpired(time.Second)

	// Should be allowed again after cleanup
	if !rl.AllowRequest(key) {
		t.Error("expected request to be allowed after cleanup")
	}
}

func TestRateLimiter_MultipleKeys(t *testing.T) {
	rl := NewRateLimiter()
	defer rl.Stop()

	keys := []string{"user1", "user2", "user3"}

	// Each key should be independent
	for _, key := range keys {
		if !rl.AllowRequest(key) {
			t.Errorf("expected first request for %s to be allowed", key)
		}
		if rl.AllowRequest(key) {
			t.Errorf("expected second request for %s to be rejected", key)
		}
	}

	// After delay, all should be allowed again
	time.Sleep(time.Second + 50*time.Millisecond)
	for _, key := range keys {
		if !rl.AllowRequest(key) {
			t.Errorf("expected request for %s to be allowed after delay", key)
		}
	}
}

func TestRateLimiter_GetShard(t *testing.T) {
	rl := NewRateLimiter()

	// Same key should always get same shard
	shard1 := rl.getShard("test_key")
	shard2 := rl.getShard("test_key")
	if shard1 != shard2 {
		t.Error("expected same key to get same shard")
	}

	// Different keys might get different shards
	shard3 := rl.getShard("other_key")
	// Note: we don't assert they're different due to hash collisions
	_ = shard3
}
