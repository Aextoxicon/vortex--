use dashmap::DashMap;
use std::hash::{BuildHasherDefault, Hasher};
use std::sync::Arc;
use std::time::Duration;
use tokio::time;

struct Fnv32Hasher(u32);

impl Default for Fnv32Hasher {
    fn default() -> Self {
        Fnv32Hasher(0x811c9dc5)
    }
}

impl Hasher for Fnv32Hasher {
    fn finish(&self) -> u64 {
        self.0 as u64
    }

    fn write(&mut self, bytes: &[u8]) {
        for byte in bytes {
            self.0 ^= *byte as u32;
            self.0 = self.0.wrapping_mul(0x01000193);
        }
    }
}

const RATE_LIMITER_SHARDS: usize = 16;

struct RateLimiterShard {
    cache: DashMap<String, i64, BuildHasherDefault<Fnv32Hasher>>,
    fail_counts: DashMap<String, i32, BuildHasherDefault<Fnv32Hasher>>,
}

#[derive(Clone)]
pub struct RateLimiter {
    shards: Vec<Arc<RateLimiterShard>>,
}

impl Default for RateLimiter {
    fn default() -> Self {
        Self::new()
    }
}

impl RateLimiter {
    pub fn new() -> Self {
        let mut shards = Vec::with_capacity(RATE_LIMITER_SHARDS);
        for _ in 0..RATE_LIMITER_SHARDS {
            shards.push(Arc::new(RateLimiterShard {
                cache: DashMap::with_hasher(BuildHasherDefault::default()),
                fail_counts: DashMap::with_hasher(BuildHasherDefault::default()),
            }));
        }
        Self { shards }
    }

    fn get_shard(&self, key: &str) -> Arc<RateLimiterShard> {
        let mut hasher = Fnv32Hasher::default();
        hasher.write(key.as_bytes());
        let index = (hasher.0 as usize) % RATE_LIMITER_SHARDS;
        self.shards[index].clone()
    }

    pub fn start_cleanup(&self, interval: Duration, ttl: Duration) {
        let limiter = self.clone();
        tokio::spawn(async move {
            let mut ticker = time::interval(interval);
            loop {
                ticker.tick().await;
                limiter.cleanup_expired(ttl);
            }
        });
    }

    pub fn allow_request(&self, public_id: &str) -> bool {
        self.allow_request_with_interval(public_id, Duration::from_secs(1))
    }

    pub fn allow_request_with_interval(&self, public_id: &str, interval: Duration) -> bool {
        let shard = self.get_shard(public_id);
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("system time should always be valid")
            .as_nanos() as i64;
        let interval_ns = interval.as_nanos() as i64;

        if let Some(entry) = shard.cache.get(public_id) {
            let last = *entry.value();
            if now - last < interval_ns {
                return false;
            }
        }
        shard.cache.insert(public_id.to_string(), now);
        true
    }

    pub fn cleanup_expired(&self, ttl: Duration) {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("system time should always be valid")
            .as_nanos() as i64;
        let ttl_ns = ttl.as_nanos() as i64;

        for shard in &self.shards {
            let expired_keys: Vec<String> = shard
                .cache
                .iter()
                .filter(|entry| now - *entry.value() > ttl_ns)
                .take(100)
                .map(|entry| entry.key().clone())
                .collect();

            for key in expired_keys {
                if let Some(entry) = shard.cache.get(&key)
                    && now - *entry.value() > ttl_ns
                {
                    shard.cache.remove(&key);
                    shard.fail_counts.remove(&key);
                }
            }
        }
    }

    pub fn record_failure(&self, key: &str) {
        let shard = self.get_shard(key);
        let mut count = shard.fail_counts.entry(key.to_string()).or_insert(0);
        *count += 1;
    }

    pub fn reset_failure(&self, key: &str) {
        let shard = self.get_shard(key);
        shard.fail_counts.remove(key);
    }

    pub fn get_failure_count(&self, key: &str) -> i32 {
        let shard = self.get_shard(key);
        shard.fail_counts.get(key).map(|e| *e.value()).unwrap_or(0)
    }

    pub fn allow_request_with_max_failures(
        &self,
        public_id: &str,
        interval: Duration,
        max_failures: i32,
    ) -> bool {
        let shard = self.get_shard(public_id);
        let fail_count = shard
            .fail_counts
            .get(public_id)
            .map(|e| *e.value())
            .unwrap_or(0);

        if fail_count >= max_failures {
            let now = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time should always be valid")
                .as_nanos() as i64;
            let interval_ns = interval.as_nanos() as i64;

            if let Some(entry) = shard.cache.get(public_id) {
                let last = *entry.value();
                if now - last < interval_ns {
                    return false;
                }
            }
            shard.fail_counts.insert(public_id.to_string(), 0);
        }
        true
    }
}
