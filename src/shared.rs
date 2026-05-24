use axum::extract::State;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use axum::Json;
use chrono::Utc;
use rand::Rng;
use serde::{Deserialize, Serialize};
use serde_json::json;
use sqlx::PgPool;
use std::collections::HashMap;
use std::time::Duration;

use crate::config::Config;
use crate::idgen::IdGenerator;
use crate::store::{
    ConversationParticipantStore, FriendRequestStore, GroupMemberStore, GroupStore,
    IdGeneratorStateStore, Message, MessageIdempotencyStore, MessageStore, UserStore,
};

pub struct Handler {
    svc: Service,
    jwt: crate::jwt::JwtService,
    cfg: Config,
}

impl Handler {
    pub fn new(svc: Service, jwt: crate::jwt::JwtService, cfg: Config) -> Self {
        Self { svc, jwt, cfg }
    }

    pub async fn health_check(&self) -> impl IntoResponse {
        Json(json!({
            "status": "ok",
            "node_id": self.cfg.node_id,
            "timestamp": Utc::now().timestamp_millis(),
        }))
    }

    pub async fn readiness_check(&self) -> impl IntoResponse {
        if let Err(e) = self.svc.pool.acquire().await {
            return (
                StatusCode::SERVICE_UNAVAILABLE,
                Json(json!({
                    "status": "not ready",
                    "reason": format!("database unavailable: {}", e),
                })),
            );
        }

        if !self.cfg.s3_url.is_empty() && self.svc.s3_service.is_none() {
            return (
                StatusCode::SERVICE_UNAVAILABLE,
                Json(json!({
                    "status": "not ready",
                    "reason": "S3 service unavailable",
                })),
            );
        }

        (
            StatusCode::OK,
            Json(json!({
                "status": "ready",
                "node_id": self.cfg.node_id,
                "timestamp": Utc::now().timestamp_millis(),
            })),
        )
    }
}

#[derive(Clone)]
pub struct Service {
    cfg: Config,
    pool: PgPool,
    user_store: UserStore,
    msg_store: MessageStore,
    group_store: GroupStore,
    group_mem_store: GroupMemberStore,
    friend_store: FriendRequestStore,
    conv_part_store: ConversationParticipantStore,
    id_gen_store: IdGeneratorStateStore,
    idempotency_store: MessageIdempotencyStore,
    id_gen: IdGenerator,
    s3_service: Option<S3Service>,
}

impl Service {
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        cfg: Config,
        pool: PgPool,
        user_store: UserStore,
        msg_store: MessageStore,
        group_store: GroupStore,
        group_mem_store: GroupMemberStore,
        friend_store: FriendRequestStore,
        conv_part_store: ConversationParticipantStore,
        id_gen_store: IdGeneratorStateStore,
        idempotency_store: MessageIdempotencyStore,
        id_gen: IdGenerator,
        s3_service: Option<S3Service>,
    ) -> Self {
        Self {
            cfg,
            pool,
            user_store,
            msg_store,
            group_store,
            group_mem_store,
            friend_store,
            conv_part_store,
            id_gen_store,
            idempotency_store,
            id_gen,
            s3_service,
        }
    }
}

#[derive(Debug, Clone)]
pub struct S3Service;

impl S3Service {
    pub fn new() -> Self {
        Self
    }
}

const NANO_ID_ALPHABET: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

pub fn generate_nano_id(size: usize) -> String {
    let size = if size == 0 { 21 } else { size };
    let mut rng = rand::thread_rng();
    let mut id = String::with_capacity(size);
    for _ in 0..size {
        let idx = rng.gen_range(0..NANO_ID_ALPHABET.len());
        id.push(NANO_ID_ALPHABET[idx] as char);
    }
    id
}

#[derive(Debug, Serialize, Deserialize)]
pub struct NotificationPayload {
    #[serde(rename = "type")]
    pub type_field: String,
    pub sub_type: String,
    pub data: HashMap<String, serde_json::Value>,
}

impl Service {
    pub async fn send_notification_to_user(
        &self,
        uid: i64,
        notif_type: &str,
        data: HashMap<String, serde_json::Value>,
    ) -> Result<i64, String> {
        let conv_id = format!("system_{}", uid);

        let payload = NotificationPayload {
            type_field: "system_notification".to_string(),
            sub_type: notif_type.to_string(),
            data,
        };

        let content = serde_json::to_string(&payload)
            .map_err(|e| format!("notification marshal failed: {}", e))?;

        let msg_id = self.id_gen.generate_id().await?;

        let epoch_time = self.id_gen.get_epoch_time();
        let ts = Utc::now().timestamp_millis() - epoch_time;

        let msg = Message {
            msg_id,
            conv_id,
            from_uid: 0,
            content,
            ts,
            is_recalled: false,
        };

        self.msg_store.insert_message(&msg).await.map_err(|e| e.to_string())?;

        Ok(msg_id)
    }

    pub async fn send_friend_notification(
        &self,
        receiver_uid: i64,
        sender_uid: i64,
        request_id: &str,
    ) -> Result<(), String> {
        let mut data = HashMap::new();
        data.insert("request_id".to_string(), serde_json::json!(request_id));
        data.insert("sender_uid".to_string(), serde_json::json!(sender_uid));
        data.insert("action".to_string(), serde_json::json!("received"));

        self.send_notification_to_user(receiver_uid, "friend_request", data)
            .await?;
        Ok(())
    }

    pub async fn send_friend_accepted_notification(
        &self,
        sender_uid: i64,
        receiver_uid: i64,
        request_id: &str,
    ) -> Result<(), String> {
        let mut data = HashMap::new();
        data.insert("request_id".to_string(), serde_json::json!(request_id));
        data.insert("receiver_uid".to_string(), serde_json::json!(receiver_uid));
        data.insert("action".to_string(), serde_json::json!("accepted"));

        self.send_notification_to_user(sender_uid, "friend_request", data)
            .await?;
        Ok(())
    }

    pub async fn send_group_invite_notification(
        &self,
        uid: i64,
        group_id: &str,
        group_name: &str,
        inviter_uid: i64,
    ) -> Result<(), String> {
        let mut data = HashMap::new();
        data.insert("group_id".to_string(), serde_json::json!(group_id));
        data.insert("group_name".to_string(), serde_json::json!(group_name));
        data.insert("inviter_uid".to_string(), serde_json::json!(inviter_uid));
        data.insert("action".to_string(), serde_json::json!("invited"));

        self.send_notification_to_user(uid, "group_invite", data)
            .await?;
        Ok(())
    }

    pub async fn get_user_by_public_id(&self, public_id: &str) -> Result<crate::store::User, crate::error::AppError> {
        let user = self
            .user_store
            .get_by_public_id(public_id)
            .await
            .map_err(|e| crate::error::AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
        user.ok_or_else(|| crate::error::AppError::not_found("user not found"))
    }

    pub async fn get_public_id_by_user_id(&self, user_id: i64) -> Result<String, crate::error::AppError> {
        let user = self
            .user_store
            .get_by_id(user_id)
            .await
            .map_err(|e| crate::error::AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
        let user = user.ok_or_else(|| crate::error::AppError::not_found("user not found"))?;
        Ok(user.public_id)
    }

    pub async fn get_users_by_ids(
        &self,
        ids: &[i64],
    ) -> Result<std::collections::HashMap<i64, crate::store::User>, crate::error::AppError> {
        self.user_store
            .get_by_ids(ids)
            .await
            .map_err(|e| crate::error::AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))
    }

    pub async fn is_user_in_group(&self, group_id: &str, user_id: i64) -> Result<bool, crate::error::AppError> {
        self.group_mem_store
            .is_member(group_id, user_id)
            .await
            .map_err(|e| crate::error::AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))
    }
}

pub async fn health_check(
    State(state): State<crate::main::AppState>,
) -> impl IntoResponse {
    state.handler.health_check().await
}

pub async fn readiness_check(
    State(state): State<crate::main::AppState>,
) -> impl IntoResponse {
    state.handler.readiness_check().await
}
