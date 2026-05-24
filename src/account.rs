use axum::extract::State;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use axum::Json;
use regex::Regex;
use serde::{Deserialize, Serialize};
use serde_json::json;
use std::sync::LazyLock;
use std::time::Duration;

use crate::config::Config;
use crate::error::AppError;
use crate::ratelimit::RateLimiter;
use crate::shared::{Handler, Service};
use crate::store::User;

static USERNAME_REGEX: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"^[a-zA-Z0-9_\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}]{3,20}$").unwrap()
});

static EMAIL_REGEX: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$").unwrap()
});

static GROUP_NAME_REGEX: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"^[\w\s\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}_-]{1,50}$").unwrap()
});

#[derive(Debug, Deserialize)]
pub struct RegisterRequest {
    pub username: String,
    pub password: String,
    pub email: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct LoginRequest {
    pub username: String,
    pub password: String,
}

#[derive(Debug, Deserialize)]
pub struct UpdateUserRequest {
    pub username: Option<String>,
    pub email: Option<String>,
}

fn validate_password(password: &str) -> bool {
    if password.len() < 8 || password.len() > 128 {
        return false;
    }

    let mut has_lower = false;
    let mut has_upper = false;
    let mut has_digit = false;
    let mut has_special = false;

    for c in password.chars() {
        match c {
            'a'..='z' => has_lower = true,
            'A'..='Z' => has_upper = true,
            '0'..='9' => has_digit = true,
            '!' | '@' | '#' | '$' | '%' | '^' | '&' | '*' | '(' | ')' => has_special = true,
            _ => {}
        }
    }

    has_lower && has_upper && has_digit && has_special
}

fn validate_username(username: &str) -> bool {
    USERNAME_REGEX.is_match(username)
}

fn validate_email(email: &str) -> bool {
    if email.is_empty() {
        return true;
    }
    EMAIL_REGEX.is_match(email) && email.len() <= 100
}

fn validate_group_name(name: &str) -> bool {
    GROUP_NAME_REGEX.is_match(name)
}

impl Handler {
    pub async fn register(
        &self,
        req: Json<RegisterRequest>,
    ) -> Result<impl IntoResponse, AppError> {
        let req = req.0;

        if !validate_username(&req.username) {
            return Err(AppError::new(StatusCode::BAD_REQUEST, "invalid username"));
        }

        if !validate_email(req.email.as_deref().unwrap_or("")) {
            return Err(AppError::new(StatusCode::BAD_REQUEST, "invalid email"));
        }

        if !validate_password(&req.password) {
            return Err(AppError::new(StatusCode::BAD_REQUEST, "weak password"));
        }

        let user = self
            .svc
            .create_user(
                req.username,
                req.password,
                req.email.unwrap_or_default(),
            )
            .await?;

        Ok((
            StatusCode::CREATED,
            Json(json!({
                "public_id": user.public_id,
                "username": user.username,
                "email": user.email,
            })),
        ))
    }

    pub async fn login(
        &self,
        state: State<crate::main::AppState>,
        req: Json<LoginRequest>,
    ) -> Result<impl IntoResponse, AppError> {
        let req = req.0;

        let user = self.svc.get_user_by_username(&req.username).await?;

        self.svc.validate_credentials(&user, &req.password)?;

        let token = self.jwt.generate_token(user.id, &user.public_id, &user.username)?;

        Ok(Json(json!({
            "token": token,
            "public_id": user.public_id,
            "message": "Login successful",
        })))
    }

    pub async fn get_me(&self, user_id: i64) -> Result<impl IntoResponse, AppError> {
        let user = self.svc.get_user_by_id(user_id).await?;

        Ok(Json(json!({
            "user": {
                "username": user.username,
                "email": user.email,
                "public_id": user.public_id,
            }
        })))
    }

    pub async fn update_user(
        &self,
        public_id: &str,
        current_public_id: &str,
        req: Json<UpdateUserRequest>,
    ) -> Result<impl IntoResponse, AppError> {
        if current_public_id != public_id {
            return Err(AppError::new(StatusCode::FORBIDDEN, "forbidden"));
        }

        let req = req.0;

        if let Some(ref username) = req.username {
            if !validate_username(username) {
                return Err(AppError::new(StatusCode::BAD_REQUEST, "invalid username"));
            }
        }

        if let Some(ref email) = req.email {
            if !validate_email(email) {
                return Err(AppError::new(StatusCode::BAD_REQUEST, "invalid email"));
            }
        }

        let mut user = self.svc.get_user_by_public_id(public_id).await?;

        if let Some(username) = req.username {
            user.username = username;
        }
        if let Some(email) = req.email {
            user.email = email;
        }

        self.svc.update_user(&user).await?;

        Ok(Json(json!({
            "public_id": user.public_id,
            "username": user.username,
            "email": user.email,
        })))
    }

    pub async fn delete_user(
        &self,
        public_id: &str,
        current_public_id: &str,
    ) -> Result<impl IntoResponse, AppError> {
        if current_public_id != public_id {
            return Err(AppError::new(StatusCode::FORBIDDEN, "forbidden"));
        }

        let user = self.svc.get_user_by_public_id(public_id).await?;

        self.svc.delete_user(user.id).await?;

        Ok(StatusCode::NO_CONTENT)
    }

    pub async fn logout(
        &self,
        auth_header: Option<String>,
    ) -> Result<impl IntoResponse, AppError> {
        if let Some(token_str) = auth_header {
            let token_str = if token_str.len() > 7 {
                token_str[7..].to_string()
            } else {
                token_str
            };

            if let Ok(claims) = self.jwt.validate_token(&token_str) {
                let expires_at = claims.exp;
                let _ = self.jwt.blacklist_token(&claims.jti, expires_at).await;
            }
        }

        Ok(Json(json!({
            "success": true,
            "message": "Logged out successfully",
        })))
    }
}

impl Service {
    pub async fn get_user_by_id(&self, user_id: i64) -> Result<User, AppError> {
        let user = self
            .user_store
            .get_by_id(user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        user.ok_or_else(|| AppError::new(StatusCode::NOT_FOUND, "user not found"))
    }

    pub async fn get_user_by_username(&self, username: &str) -> Result<User, AppError> {
        let user = self
            .user_store
            .get_by_username(username)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        user.ok_or_else(|| AppError::new(StatusCode::NOT_FOUND, "user not found"))
    }

    pub async fn get_user_by_public_id(&self, public_id: &str) -> Result<User, AppError> {
        let user = self
            .user_store
            .get_by_public_id(public_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        user.ok_or_else(|| AppError::new(StatusCode::NOT_FOUND, "user not found"))
    }

    pub async fn get_users_by_ids(
        &self,
        user_ids: &[i64],
    ) -> Result<std::collections::HashMap<i64, User>, AppError> {
        if user_ids.is_empty() {
            return Ok(std::collections::HashMap::new());
        }

        self.user_store
            .get_by_ids(user_ids)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))
    }

    pub async fn get_public_id_by_user_id(&self, user_id: i64) -> Result<String, AppError> {
        let user = self.get_user_by_id(user_id).await?;
        Ok(user.public_id)
    }

    pub async fn create_user(
        &self,
        username: String,
        password: String,
        email: String,
    ) -> Result<User, AppError> {
        let exists = self
            .user_store
            .username_exists(&username)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        if exists {
            return Err(AppError::new(StatusCode::CONFLICT, "username already exists"));
        }

        if !email.is_empty() {
            let email_exists = self
                .user_store
                .email_exists(&email)
                .await
                .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

            if email_exists {
                return Err(AppError::new(StatusCode::CONFLICT, "email already exists"));
            }
        }

        let pwd_hash = bcrypt::hash(password.as_bytes(), self.cfg.bcrypt_cost)
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &format!("password hashing failed: {}", e)))?;

        let public_id = crate::shared::generate_nano_id(self.cfg.public_id_length);

        let now = chrono::Utc::now().timestamp_millis();
        let user = User {
            id: 0,
            username,
            pwd_hash,
            email,
            public_id,
            created_at: now,
            updated_at: now,
        };

        let id = self
            .user_store
            .insert(&user)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(User { id, ..user })
    }

    pub async fn update_user(&self, user: &User) -> Result<(), AppError> {
        let mut user = user.clone();
        user.updated_at = chrono::Utc::now().timestamp_millis();

        self.user_store
            .update(&user)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(())
    }

    pub async fn delete_user(&self, user_id: i64) -> Result<(), AppError> {
        let user = self.get_user_by_id(user_id).await?;

        let groups = self
            .group_store
            .get_groups_by_owner(user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        for g in groups {
            self.delete_group(&g.group_id).await?;
        }

        let mut tx = self
            .pool
            .begin()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        self.group_mem_store
            .delete_by_user_tx(&mut tx, user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        self.conv_part_store
            .delete_by_user_tx(&mut tx, user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        self.delete_friend_requests_by_user(&mut tx, user_id)
            .await?;

        self.user_store
            .delete_tx(&mut tx, user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        tx.commit()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(())
    }

    pub fn validate_credentials(&self, user: &User, password: &str) -> Result<(), AppError> {
        bcrypt::verify(password, &user.pwd_hash)
            .map_err(|_| AppError::new(StatusCode::UNAUTHORIZED, "invalid credentials"))
            .and_then(|valid| {
                if valid {
                    Ok(())
                } else {
                    Err(AppError::new(StatusCode::UNAUTHORIZED, "invalid credentials"))
                }
            })
    }
}

pub async fn register(
    State(state): State<crate::main::AppState>,
    req: Json<RegisterRequest>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.register(req).await
}

pub async fn login(
    State(state): State<crate::main::AppState>,
    req: Json<LoginRequest>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.login(req).await
}

pub async fn get_me(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.get_me(user_id).await
}

pub async fn logout(
    State(state): State<crate::main::AppState>,
    axum::http::HeaderMap,
) -> Result<impl IntoResponse, AppError> {
    state.handler.logout(None).await
}

pub async fn update_user(
    State(state): State<crate::main::AppState>,
    axum::extract::Path(public_id): axum::extract::Path<String>,
    axum::extract::Extension(current_public_id): axum::extract::Extension<String>,
    req: Json<UpdateUserRequest>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.update_user(&public_id, &current_public_id, req).await
}

pub async fn delete_user(
    State(state): State<crate::main::AppState>,
    axum::extract::Path(public_id): axum::extract::Path<String>,
    axum::extract::Extension(current_public_id): axum::extract::Extension<String>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.delete_user(&public_id, &current_public_id).await
}
