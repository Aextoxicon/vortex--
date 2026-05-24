use vortex--::ratelimit;

#[test]
fn test_rate_limiter_allow_request() {
    let limiter = ratelimit::RateLimiter::new();
    
    let key = "test_key";
    
    assert!(limiter.allow_request(key));
}

#[test]
fn test_rate_limiter_block_after_limit() {
    let limiter = ratelimit::RateLimiter::new();
    
    let key = "test_key";
    
    for _ in 0..10 {
        assert!(limiter.allow_request(key));
    }
    
    assert!(!limiter.allow_request(key));
}

#[test]
fn test_rate_limiter_different_keys() {
    let limiter = ratelimit::RateLimiter::new();
    
    assert!(limiter.allow_request("key1"));
    assert!(limiter.allow_request("key2"));
    
    for _ in 0..9 {
        limiter.allow_request("key1");
    }
    
    assert!(!limiter.allow_request("key1"));
    assert!(limiter.allow_request("key2"));
}

#[test]
fn test_rate_limiter_cleanup() {
    let limiter = ratelimit::RateLimiter::new();
    limiter.start_cleanup();
    
    let key = "test_key";
    
    for _ in 0..10 {
        limiter.allow_request(key);
    }
    
    assert!(!limiter.allow_request(key));
    
    limiter.stop();
}

#[test]
fn test_rate_limiter_concurrent_access() {
    let limiter = std::sync::Arc::new(ratelimit::RateLimiter::new());
    
    let mut handles = vec![];
    
    for i in 0..10 {
        let limiter = limiter.clone();
        let handle = std::thread::spawn(move || {
            let key = format!("key_{}", i);
            limiter.allow_request(&key)
        });
        handles.push(handle);
    }
    
    for handle in handles {
        assert!(handle.join().unwrap());
    }
}
