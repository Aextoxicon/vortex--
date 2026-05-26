pub mod account;
pub mod config;
pub mod error;
pub mod friend;
pub mod groups;
pub mod idgen;
pub mod jwt;
pub mod messaging;
pub mod metrics;
pub mod migration;
pub mod ratelimit;
pub mod s3;
pub mod shared;
pub mod store;
pub mod worker;

use jwt::JwtService;
use shared::Service;

pub use config::Config;
pub use ratelimit::RateLimiter;

#[derive(Clone)]
pub struct AppState {
    pub svc: Service,
    pub jwt_service: JwtService,
    pub rate_limiter: RateLimiter,
}
