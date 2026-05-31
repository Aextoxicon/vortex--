use axum::Json;
use axum::extract::State;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use serde::Serialize;
use serde_json::json;

use crate::error::AppError;
use crate::shared::Service;
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

        let mut tx = self.pool.begin().await?;

        let reverse_id = self
            .friend_store
            .accept_pending_tx(&mut tx, to_user_id, from_user_id)
            .await?;

        if reverse_id > 0 {
            tx.commit().await?;

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
        let req = FriendRequest {
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

        tx.commit().await?;

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
        let mut tx = self.pool.begin().await?;

        let from_user_id = self
            .friend_store
            .accept_by_id_tx(&mut tx, request_id, user_id)
            .await?;

        if from_user_id == 0 {
            return Err(AppError::not_found("Friend request not found"));
        }
        if from_user_id < 0 {
            return Err(AppError::forbidden());
        }

        tx.commit().await?;

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
        let mut tx = self.pool.begin().await?;

        let affected = self
            .friend_store
            .reject_tx(&mut tx, request_id, user_id)
            .await?;

        if !affected {
            return Err(AppError::not_found("Friend request not found"));
        }

        tx.commit().await?;

        Ok(())
    }

    pub async fn cancel_friend_request(
        &self,
        request_id: i64,
        from_user_id: i64,
    ) -> Result<(), AppError> {
        let mut tx = self.pool.begin().await?;

        let affected = self
            .friend_store
            .cancel_tx(&mut tx, request_id, from_user_id)
            .await?;

        if !affected {
            return Err(AppError::not_found("Friend request not found"));
        }

        tx.commit().await?;

        Ok(())
    }

    pub async fn block_user(&self, conv_id: &str, target_user_id: i64) -> Result<(), AppError> {
        self.conv_part_store
            .set_blocked(conv_id, target_user_id, true)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))
    }

    pub async fn unblock_user(&self, conv_id: &str, target_user_id: i64) -> Result<(), AppError> {
        self.conv_part_store
            .set_blocked(conv_id, target_user_id, false)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))
    }
}

fn is_unique_violation(err: &sqlx::Error) -> bool {
    if let sqlx::Error::Database(db_err) = err
        && let Some(code) = db_err.code()
    {
        return code == "23505";
    }
    false
}

pub async fn send_friend_request(
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Extension(public_id): axum::extract::Extension<String>,
    axum::extract::Path(target_public_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let target_user = state.svc.get_user_by_public_id(&target_public_id).await?;

    let (request_id, auto_accepted) = state
        .svc
        .send_friend_request(user_id, target_user.id, "".to_string())
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
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let sent = state.svc.get_sent_requests(user_id).await?;
    let received = state.svc.get_received_requests(user_id).await?;

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
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Path(request_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let request_id: i64 = request_id
        .parse()
        .map_err(|_| AppError::bad_request("invalid request_id"))?;
    state.svc.accept_friend_request(request_id, user_id).await?;

    Ok(Json(json!({ "message": "Friend request accepted" })))
}

pub async fn reject_friend_request(
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Path(request_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let request_id: i64 = request_id
        .parse()
        .map_err(|_| AppError::bad_request("invalid request_id"))?;
    state.svc.reject_friend_request(request_id, user_id).await?;

    Ok(Json(json!({ "message": "Friend request rejected" })))
}

pub async fn cancel_friend_request(
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Path(request_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let request_id: i64 = request_id
        .parse()
        .map_err(|_| AppError::bad_request("invalid request_id"))?;
    state.svc.cancel_friend_request(request_id, user_id).await?;

    Ok(StatusCode::NO_CONTENT)
}

pub async fn block_user(
    State(state): State<crate::AppState>,
    axum::extract::Extension(_user_id): axum::extract::Extension<i64>,
    axum::extract::Extension(public_id): axum::extract::Extension<String>,
    axum::extract::Path(target_public_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let target_user = state.svc.get_user_by_public_id(&target_public_id).await?;
    let conv_id = crate::shared::private_conv_id(&public_id, &target_public_id);
    state.svc.block_user(&conv_id, target_user.id).await?;
    Ok(Json(json!({ "message": "User blocked successfully" })))
}

pub async fn unblock_user(
    State(state): State<crate::AppState>,
    axum::extract::Extension(_user_id): axum::extract::Extension<i64>,
    axum::extract::Extension(public_id): axum::extract::Extension<String>,
    axum::extract::Path(target_public_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let target_user = state.svc.get_user_by_public_id(&target_public_id).await?;
    let conv_id = crate::shared::private_conv_id(&public_id, &target_public_id);
    state.svc.unblock_user(&conv_id, target_user.id).await?;
    Ok(Json(json!({ "message": "User unblocked successfully" })))
}

#[derive(Debug, Serialize)]
pub struct GetPendingRequestsResponse {
    pub requests: Vec<FriendRequestResponse>,
}

pub async fn get_pending_requests(
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let requests = state
        .svc
        .friend_store
        .get_pending_requests(user_id)
        .await
        .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

    let list: Vec<FriendRequestResponse> = requests
        .iter()
        .map(|r| FriendRequestResponse {
            id: r.id,
            sender_id: r.from_user_id,
            receiver_id: r.to_user_id,
            status: r.status.clone(),
            ts: r.created_at,
        })
        .collect();

    Ok(Json(GetPendingRequestsResponse { requests: list }))
}
