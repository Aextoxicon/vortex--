use axum::extract::State;
use chrono::{Duration, Utc};
use jsonwebtoken::{decode, encode, DecodingKey, EncodingKey, Header, Validation};
use serde::{Deserialize, Serialize};
use sqlx::PgPool;
use std::collections::HashMap;
use std::sync::Arc;
use std::sync::RwLock;
use tokio::time;

const MAX_BLACKLIST_CACHE_SIZE: usize = 10000;

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct JwtClaims {
    pub user_id: i64,
    pub public_id: String,
    pub username: String,
    pub exp: i64,
    pub iat: i64,
    pub nbf: i64,
    pub iss: String,
    pub aud: String,
    pub jti: String,
}

#[derive(Clone)]
pub struct JwtService {
    pool: PgPool,
    secret: Vec<u8>,
    issuer: String,
    expires_in_minutes: i64,
    blacklist_cache: Arc<RwLock<HashMap<String, i64>>>,
}

impl JwtService {
    pub fn new(pool: PgPool, secret: &str, issuer: &str, expires_in_minutes: i64) -> Self {
        let service = Self {
            pool,
            secret: secret.as_bytes().to_vec(),
            issuer: issuer.to_string(),
            expires_in_minutes,
            blacklist_cache: Arc::new(RwLock::new(HashMap::new())),
        };
        service.load_blacklist_from_db();
        service
    }

    fn load_blacklist_from_db(&self) {
        let rt = match tokio::runtime::Runtime::new() {
            Ok(rt) => rt,
            Err(e) => {
                tracing::error!("failed to create runtime for loading JWT blacklist: {}", e);
                return;
            }
        };
        
        let result = rt.block_on(async {
            let query = r#"SELECT jti, expires_at FROM jwt_blacklist"#;
            sqlx::query_as::<_, (String, i64)>(query)
                .fetch_all(&self.pool)
                .await
        });

        match result {
            Ok(rows) => {
                let now = Utc::now().timestamp_millis();
                let mut cache = HashMap::new();
                for (jti, expires_at) in rows {
                    if expires_at > now {
                        cache.insert(jti, expires_at);
                    }
                }
                self.blacklist_cache.write().unwrap().extend(cache);
            }
            Err(e) => {
                tracing::warn!("failed to load blacklist from database, cache will be empty: {}", e);
            }
        }
    }

    pub async fn blacklist_token(&self, jti: &str, expires_at: i64) -> Result<(), sqlx::Error> {
        let query = r#"
            INSERT INTO jwt_blacklist (jti, expires_at)
            VALUES ($1, $2)
            ON CONFLICT (jti) DO NOTHING"#;
        sqlx::query(query)
            .bind(jti)
            .bind(expires_at)
            .execute(&self.pool)
            .await?;

        let mut cache = self.blacklist_cache.write().unwrap();
        if cache.len() >= MAX_BLACKLIST_CACHE_SIZE {
            let now = Utc::now().timestamp_millis();
            cache.retain(|_, v| *v > now);
        }
        cache.insert(jti.to_string(), expires_at);
        Ok(())
    }

    pub fn is_blacklisted(&self, jti: &str) -> bool {
        let now = Utc::now().timestamp_millis();

        let mut cache = self.blacklist_cache.write().unwrap();
        if let Some(&exp) = cache.get(jti) {
            if exp <= now {
                cache.remove(jti);
                false
            } else {
                true
            }
        } else {
            false
        }
    }

    pub async fn cleanup_blacklist(&self) {
        let now = Utc::now().timestamp_millis();

        let expired_items: Vec<String> = {
            let cache = self.blacklist_cache.read().unwrap();
            cache
                .iter()
                .filter(|(_, exp)| **exp < now)
                .take(1000)
                .map(|(jti, _)| jti.clone())
                .collect()
        };

        if !expired_items.is_empty() {
            {
                let mut cache = self.blacklist_cache.write().unwrap();
                for jti in &expired_items {
                    if let Some(&exp) = cache.get(jti) && exp < now {
                        cache.remove(jti);
                    }
                }
            }

            let pool = self.pool.clone();
            let items = expired_items.clone();
            tokio::spawn(async move {
                if items.len() == 1 {
                    let _ = sqlx::query(r#"DELETE FROM jwt_blacklist WHERE jti = $1"#)
                        .bind(&items[0])
                        .execute(&pool)
                        .await;
                } else {
                    let _ = sqlx::query(r#"DELETE FROM jwt_blacklist WHERE jti = ANY($1)"#)
                        .bind(&items)
                        .execute(&pool)
                        .await;
                }
            });
        }
    }

    pub fn start_cleanup(&self, interval: std::time::Duration) {
        let service = self.clone();
        tokio::spawn(async move {
            let mut ticker = time::interval(interval);
            loop {
                ticker.tick().await;
                service.cleanup_blacklist().await;
            }
        });
    }

    pub fn generate_token(&self, user_id: i64, public_id: &str, username: &str) -> Result<String, String> {
        let now = Utc::now();
        let expires_at = now + Duration::minutes(self.expires_in_minutes);
        let jti = uuid::Uuid::new_v4().to_string();

        let claims = JwtClaims {
            user_id,
            public_id: public_id.to_string(),
            username: username.to_string(),
            exp: expires_at.timestamp(),
            iat: now.timestamp(),
            nbf: now.timestamp(),
            iss: self.issuer.clone(),
            aud: self.issuer.clone(),
            jti,
        };

        let token = encode(
            &Header::default(),
            &claims,
            &EncodingKey::from_secret(&self.secret),
        )
        .map_err(|e| format!("failed to generate token: {}", e))?;

        Ok(token)
    }

    pub fn validate_token(&self, token_str: &str) -> Result<JwtClaims, String> {
        let mut validation = Validation::default();
        validation.set_issuer(std::slice::from_ref(&self.issuer));
        validation.set_audience(std::slice::from_ref(&self.issuer));

        let token_data = decode::<JwtClaims>(
            token_str,
            &DecodingKey::from_secret(&self.secret),
            &validation,
        )
        .map_err(|e| format!("invalid token: {}", e))?;

        let claims = token_data.claims;

        if self.is_blacklisted(&claims.jti) {
            return Err("token is blacklisted".to_string());
        }

        Ok(claims)
    }
}

pub async fn auth_middleware(
    app_state: State<crate::AppState>,
    req: axum::http::Request<axum::body::Body>,
    next: axum::middleware::Next,
) -> axum::response::Response {
    let auth_header = req
        .headers()
        .get(axum::http::header::AUTHORIZATION)
        .and_then(|h| h.to_str().ok());

    match auth_header {
        Some(token) => {
            let token_str = token.strip_prefix("Bearer ").unwrap_or(token);

            match app_state.jwt_service.validate_token(token_str) {
                Ok(claims) => {
                    if app_state.jwt_service.is_blacklisted(&claims.jti) {
                        return axum::response::Response::builder()
                            .status(axum::http::StatusCode::UNAUTHORIZED)
                            .body(axum::body::Body::from("Token is blacklisted"))
                            .unwrap();
                    }

                    let mut req = req;
                    req.extensions_mut().insert(claims.user_id);
                    req.extensions_mut().insert(claims.public_id);
                    next.run(req).await
                }
                Err(_) => {
                    axum::response::Response::builder()
                        .status(axum::http::StatusCode::UNAUTHORIZED)
                        .body(axum::body::Body::from("Invalid token"))
                        .unwrap()
                }
            }
        }
        None => {
            axum::response::Response::builder()
                .status(axum::http::StatusCode::UNAUTHORIZED)
                .body(axum::body::Body::from("Missing token"))
                .unwrap()
        }
    }
}
