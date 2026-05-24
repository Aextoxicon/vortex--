use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::response::IntoResponse;
use axum::Json;
use serde::{Deserialize, Serialize};
use serde_json::json;

use crate::error::AppError;
use crate::shared::{Handler, Service};
use crate::store::FriendRequest;

#[derive(Debug, Serialize)]
pub struct SendFriendRequestResponse {
    pub id: i64,
    pub status: String,
    pub sender_public_id: String,
    pub receiver_public_id: String,
}

#[derive(Debug, Serialize)]
pub struct FriendRequestResponse {
    pub id: i64,
    pub sender_id: i64,
    pub receiver_id: i64,
    pub status: String,
    pub ts: i64,
}

#[derive(Debug, Serialize)]
pub struct GetFriendRequestsResponse {
    pub sent: Vec<FriendRequestResponse>,
    pub received: Vec<FriendRequestResponse>,
}

impl Handler {
    pub async fn send_friend_request(
        &self,
        user_id: i64,
        public_id: String,
        target_public_id: Path<String>,
    ) -> Result<impl IntoResponse, AppError> {
        let target_public_id = target_public_id.0;

        let target_user = self.svc.get_user_by_public_id(&target_public_id).await?;

        let (request_id, auto_accepted) = self
            .svc
            .send_friend_request(user_id, target_user.id, "")
            .await?;

        let status = if auto_accepted {
            "auto_accepted".to_string()
        } else {
            "pending".to_string()
        };

        Ok((
            StatusCode::CREATED,
            Json(SendFriendRequestResponse {
                id: request_id,
                status,
                sender_public_id: public_id,
                receiver_public_id: target_public_id,
            }),
        ))
    }

    pub async fn get_friend_requests(
        &self,
        user_id: i64,
    ) -> Result<impl IntoResponse, AppError> {
        let sent = self.svc.get_sent_requests(user_id).await?;
        let received = self.svc.get_received_requests(user_id).await?;

        let format_request = |r: &FriendRequest| FriendRequestResponse {
            id: r.id,
            sender_id: r.from_user_id,
            receiver_id: r.to_user_id,
            status: r.status.clone(),
            ts: r.created_at,
        };

        let sent_list: Vec<FriendRequestResponse> = sent.iter().map(format_request).collect();
        let received_list: Vec<FriendRequestResponse> = received.iter().map(format_request).collect();

        Ok(Json(GetFriendRequestsResponse {
            sent: sent_list,
            received: received_list,
        }))
    }

    pub async fn accept_friend_request(
        &self,
        user_id: i64,
        request_id: Path<i64>,
    ) -> Result<impl IntoResponse, AppError> {
        let request_id = request_id.0;

        self.svc.accept_friend_request(request_id, user_id).await?;

        Ok(Json(json!({ "message": "Friend request accepted" })))
    }

    pub async fn reject_friend_request(
        &self,
        user_id: i64,
        request_id: Path<i64>,
    ) -> Result<impl IntoResponse, AppError> {
        let request_id = request_id.0;

        self.svc.reject_friend_request(request_id, user_id).await?;

        Ok(Json(json!({ "message": "Friend request rejected" })))
    }

    pub async fn cancel_friend_request(
        &self,
        user_id: i64,
        request_id: Path<i64>,
    ) -> Result<impl IntoResponse, AppError> {
        let request_id = request_id.0;

        self.svc.cancel_friend_request(request_id, user_id).await?;

        Ok(StatusCode::NO_CONTENT)
    }
}

impl Service {
    pub async fn send_friend_request(
        &self,
        from_user_id: i64,
        to_user_id: i64,
        message: String,
    ) -> Result<(i64, bool), AppError> {
        if from_user_id == to_user_id {
            return Err(AppError::new(StatusCode::CONFLICT, "Cannot request self"));
        }

        self.get_user_by_id(to_user_id).await?;

        let mut tx = self
            .pool
            .begin()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let reverse_id = self
            .friend_store
            .accept_pending_tx(&mut tx, to_user_id, from_user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        if reverse_id > 0 {
            tx.commit()
                .await
                .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

            let svc = self.clone();
            let reverse_id_str = reverse_id.to_string();
            tokio::spawn(async move {
                let _ = svc
                    .send_friend_accepted_notification(from_user_id, to_user_id, &reverse_id_str)
                    .await;
            });

            return Ok((0, true));
        }

        let now = chrono::Utc::now().timestamp_millis();
        let req = crate::store::FriendRequest {
            id: 0,
            from_user_id,
            to_user_id,
            message,
            status: "pending".to_string(),
            created_at: now,
            updated_at: now,
        };

        let result = self
            .friend_store
            .insert_tx(&mut tx, &req)
            .await
            .map_err(|e| {
                if is_unique_violation(&e) {
                    AppError::new(StatusCode::CONFLICT, "Friend request already exists")
                } else {
                    AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string())
                }
            })?;

        tx.commit()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let svc = self.clone();
        let result_str = result.to_string();
        tokio::spawn(async move {
            let _ = svc
                .send_friend_notification(to_user_id, from_user_id, &result_str)
                .await;
        });

        Ok((result, false))
    }

    pub async fn get_sent_requests(
        &self,
        from_user_id: i64,
    ) -> Result<Vec<FriendRequest>, AppError> {
        self.friend_store
            .get_sent_requests(from_user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))
    }

    pub async fn get_received_requests(
        &self,
        to_user_id: i64,
    ) -> Result<Vec<FriendRequest>, AppError> {
        self.friend_store
            .get_received_requests(to_user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))
    }

    pub async fn accept_friend_request(
        &self,
        request_id: i64,
        user_id: i64,
    ) -> Result<(), AppError> {
        let mut tx = self
            .pool
            .begin()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let from_user_id = self
            .friend_store
            .accept_by_id_tx(&mut tx, request_id, user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        if from_user_id == 0 {
            return Err(AppError::not_found("Friend request not found"));
        }
        if from_user_id < 0 {
            return Err(AppError::forbidden());
        }

        tx.commit()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let svc = self.clone();
        let request_id_str = request_id.to_string();
        tokio::spawn(async move {
            let _ = svc
                .send_friend_accepted_notification(from_user_id, user_id, &request_id_str)
                .await;
        });

        Ok(())
    }

    pub async fn reject_friend_request(
        &self,
        request_id: i64,
        user_id: i64,
    ) -> Result<(), AppError> {
        let mut tx = self
            .pool
            .begin()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let affected = self
            .friend_store
            .reject_tx(&mut tx, request_id, user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        if !affected {
            return Err(AppError::not_found("Friend request not found"));
        }

        tx.commit()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(())
    }

    pub async fn cancel_friend_request(
        &self,
        request_id: i64,
        from_user_id: i64,
    ) -> Result<(), AppError> {
        let mut tx = self
            .pool
            .begin()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let affected = self
            .friend_store
            .cancel_tx(&mut tx, request_id, from_user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        if !affected {
            return Err(AppError::not_found("Friend request not found"));
        }

        tx.commit()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(())
    }

    pub async fn get_user_by_id(&self, id: i64) -> Result<crate::store::User, AppError> {
        let user = self
            .user_store
            .get_by_id(id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
        user.ok_or_else(|| AppError::not_found("user not found"))
    }
}

fn is_unique_violation(err: &sqlx::Error) -> bool {
    if let sqlx::Error::Database(db_err) = err {
        if let Some(code) = db_err.code() {
            return code == "23505";
        }
    }
    false
}

pub fn private_conv_id(public_id_1: &str, public_id_2: &str) -> String {
    if public_id_1 < public_id_2 {
        format!("p_{}_{}", public_id_1, public_id_2)
    } else {
        format!("p_{}_{}", public_id_2, public_id_1)
    }
}

pub fn is_private_conv(conv_id: &str) -> bool {
    !conv_id.is_empty() && conv_id.starts_with('p')
}

pub fn parse_private_conv(conv_id: &str) -> Result<(String, String), String> {
    if !is_private_conv(conv_id) {
        return Err("not a private conversation".to_string());
    }

    if conv_id.len() < 3 || !conv_id.starts_with("p_") {
        return Err("invalid private conversation format".to_string());
    }

    let rest = &conv_id[2..];
    let last_underscore = rest.rfind('_').ok_or("invalid private conversation format")?;

    let a = rest[..last_underscore].to_string();
    let b = rest[last_underscore + 1..].to_string();
    Ok((a, b))
}

pub async fn send_friend_request(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Path(target_public_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse, AppError> {
    let target_user = state.handler.svc.get_user_by_public_id(&target_public_id).await?;
    state.handler.send_friend_request(user_id, target_user.id, String::new()).await
}

pub async fn get_friend_requests(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Query(params): axum::extract::Query<std::collections::HashMap<String, String>>,
) -> Result<impl IntoResponse, AppError> {
    let direction = params.get("direction").cloned().unwrap_or_else(|| "received".to_string());
    match direction.as_str() {
        "sent" => state.handler.get_sent_requests(user_id).await,
        "received" => state.handler.get_received_requests(user_id).await,
        _ => Err(AppError::bad_request("invalid direction, must be 'sent' or 'received'")),
    }
}

pub async fn accept_friend_request(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Path(request_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse, AppError> {
    let request_id: i64 = request_id.parse().map_err(|_| AppError::bad_request("invalid request_id"))?;
    state.handler.accept_friend_request(user_id, request_id).await
}

pub async fn reject_friend_request(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Path(request_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse, AppError> {
    let request_id: i64 = request_id.parse().map_err(|_| AppError::bad_request("invalid request_id"))?;
    state.handler.reject_friend_request(user_id, request_id).await
}

pub async fn cancel_friend_request(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Path(request_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse, AppError> {
    let request_id: i64 = request_id.parse().map_err(|_| AppError::bad_request("invalid request_id"))?;
    state.handler.cancel_friend_request(user_id, request_id).await
}

pub fn can_access_private_conv(conv_id: &str, public_id: &str) -> bool {
    match parse_private_conv(conv_id) {
        Ok((a, b)) => a == public_id || b == public_id,
        Err(_) => false,
    }
}

pub fn extract_conversation_type(conv_id: &str) -> &str {
    if conv_id.is_empty() {
        return "";
    }
    if conv_id.starts_with('p') {
        return "p";
    }
    if conv_id.starts_with('g') {
        return "g";
    }
    ""
}

pub fn get_other_public_id(conv_id: &str, my_public_id: &str) -> String {
    if !is_private_conv(conv_id) {
        return String::new();
    }
    match parse_private_conv(conv_id) {
        Ok((a, b)) => {
            if a == my_public_id {
                b
            } else if b == my_public_id {
                a
            } else {
                String::new()
            }
        }
        Err(_) => String::new(),
    }
}
