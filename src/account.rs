use axum::extract::State;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use axum::Json;
use regex::Regex;
use serde::Deserialize;
use serde_json::json;
use std::sync::LazyLock;
use std::time::Duration;

use crate::error::AppError;
use crate::shared::Service;
use crate::store::User;

static USERNAME_REGEX: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"^[a-zA-Z0-9_\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}]{3,20}$").unwrap()
});

static EMAIL_REGEX: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$").unwrap()
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

pub fn validate_password(password: &str) -> bool {
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

pub fn validate_username(username: &str) -> bool {
    USERNAME_REGEX.is_match(username)
}

pub fn validate_email(email: &str) -> bool {
    if email.is_empty() {
        return true;
    }
    EMAIL_REGEX.is_match(email) && email.len() <= 100
}

impl Service {
    pub async fn create_user(
        &self,
        username: String,
        password: String,
        email: String,
    ) -> Result<User, AppError> {
        if !validate_username(&username) {
            return Err(AppError::bad_request("invalid username"));
        }

        if !validate_email(&email) {
            return Err(AppError::bad_request("invalid email"));
        }

        if !validate_password(&password) {
            return Err(AppError::bad_request("weak password"));
        }

        let exists = self
            .user_store
            .username_exists(&username)
            .await
            ?;

        if exists {
            return Err(AppError::new(StatusCode::CONFLICT, "username already exists"));
        }

        if !email.is_empty() {
            let email_exists = self
                .user_store
                .email_exists(&email)
                .await
                ?;

            if email_exists {
                return Err(AppError::new(StatusCode::CONFLICT, "email already exists"));
            }
        }

        let pwd_hash = bcrypt::hash(password.as_bytes(), self.cfg.bcrypt_cost as u32)
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &format!("password hashing failed: {}", e)))?;

        let public_id = crate::shared::generate_nano_id(self.cfg.public_id_length as usize);

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
            ?;

        Ok(User { id, ..user })
    }

    pub async fn update_user_profile(
        &self,
        target_public_id: &str,
        caller_public_id: &str,
        username: Option<String>,
        email: Option<String>,
    ) -> Result<User, AppError> {
        if caller_public_id != target_public_id {
            return Err(AppError::forbidden());
        }

        if let Some(ref u) = username && !validate_username(u) {
            return Err(AppError::bad_request("invalid username"));
        }

        if let Some(ref e) = email && !validate_email(e) {
            return Err(AppError::bad_request("invalid email"));
        }

        let mut user = self.get_user_by_public_id(target_public_id).await?;

        if let Some(u) = username {
            user.username = u;
        }
        if let Some(e) = email {
            user.email = e;
        }

        self.update_user(&user).await?;

        Ok(user)
    }

    pub async fn delete_user_by_public_id(
        &self,
        target_public_id: &str,
        caller_public_id: &str,
    ) -> Result<(), AppError> {
        if caller_public_id != target_public_id {
            return Err(AppError::forbidden());
        }

        let user = self.get_user_by_public_id(target_public_id).await?;
        self.delete_user(user.id).await
    }

    pub async fn update_user(&self, user: &User) -> Result<(), AppError> {
        let mut user = user.clone();
        user.updated_at = chrono::Utc::now().timestamp_millis();

        self.user_store
            .update(&user)
            .await
            ?;

        Ok(())
    }

    pub async fn delete_user(&self, user_id: i64) -> Result<(), AppError> {
        let groups = self
            .group_store
            .get_groups_by_owner(user_id)
            .await
            ?;

        for g in groups {
            self.delete_group(&g.group_id).await?;
        }

        let mut tx = self
            .pool
            .begin()
            .await
            ?;

        self.group_mem_store
            .delete_by_user_tx(&mut tx, user_id)
            .await
            ?;

        self.conv_part_store
            .delete_by_user_tx(&mut tx, user_id)
            .await
            ?;

        self.friend_store
            .delete_by_user_tx(&mut tx, user_id)
            .await?;

        self.user_store
            .delete_tx(&mut tx, user_id)
            .await
            ?;

        tx.commit()
            .await
            ?;

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
    State(state): State<crate::AppState>,
    req: Json<RegisterRequest>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let req = req.0;
    let user = state
        .svc
        .create_user(req.username, req.password, req.email.unwrap_or_default())
        .await?;

    Ok((
        StatusCode::CREATED,
        Json(json!({
            "user": {
                "public_id": user.public_id,
                "username": user.username,
                "email": user.email,
            },
        })),
    ))
}

pub async fn login(
    State(state): State<crate::AppState>,
    req: Json<LoginRequest>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let req = req.0;
    let username = req.username.clone();
    let rate_limiter = state.rate_limiter.clone();

    if !rate_limiter.allow_request_with_max_failures(
        &username,
        Duration::from_secs(900),
        5,
    ) {
        return Err(AppError::new(StatusCode::TOO_MANY_REQUESTS, "rate limit exceeded"));
    }

    let user = state.svc.get_user_by_username(&req.username).await?;

    let result = state.svc.validate_credentials(&user, &req.password);

    if let Err(e) = result {
        rate_limiter.record_failure(&username);
        return Err(e);
    }

    let token = state
        .jwt_service
        .generate_token(user.id, &user.public_id, &user.username)
        .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e))?;

    Ok(Json(json!({
        "token": token,
        "user": {
            "username": user.username,
            "email": user.email,
            "public_id": user.public_id,
        },
    })))
}

pub async fn get_me(
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let user = state.svc.get_user_by_id(user_id).await?;

    Ok(Json(json!({
        "user": {
            "username": user.username,
            "email": user.email,
            "public_id": user.public_id,
        }
    })))
}

pub async fn logout(
    State(state): State<crate::AppState>,
    headers: axum::http::HeaderMap,
) -> Result<impl IntoResponse + use<>, AppError> {
    if let Some(token_str) = headers
        .get("authorization")
        .and_then(|v| v.to_str().ok())
        .map(|s| s.to_string())
    {
        let token_str = if token_str.len() > 7 {
            token_str[7..].to_string()
        } else {
            token_str
        };

        if let Ok(claims) = state.jwt_service.validate_token(&token_str) {
            let expires_at = claims.exp;
            let _ = state.jwt_service.blacklist_token(&claims.jti, expires_at).await;
        }
    }

    Ok(Json(json!({
        "success": true,
        "message": "Logged out successfully",
    })))
}

pub async fn update_user(
    State(state): State<crate::AppState>,
    axum::extract::Path(public_id): axum::extract::Path<String>,
    axum::extract::Extension(current_public_id): axum::extract::Extension<String>,
    req: Json<UpdateUserRequest>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let req = req.0;
    let user = state
        .svc
        .update_user_profile(&public_id, &current_public_id, req.username, req.email)
        .await?;

    Ok(Json(json!({
        "public_id": user.public_id,
        "username": user.username,
        "email": user.email,
    })))
}

pub async fn delete_user(
    State(state): State<crate::AppState>,
    axum::extract::Path(public_id): axum::extract::Path<String>,
    axum::extract::Extension(current_public_id): axum::extract::Extension<String>,
) -> Result<impl IntoResponse + use<>, AppError> {
    state
        .svc
        .delete_user_by_public_id(&public_id, &current_public_id)
        .await?;

    Ok(Json(json!({ "message": "user deleted successfully" })))
}
