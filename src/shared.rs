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

use crate::config::Config;
use crate::idgen::IdGenerator;
use crate::s3::S3Service;
use crate::store::{
    ConversationParticipantStore, FriendRequestStore, GroupMemberStore, GroupStore, Message,
    MessageIdempotencyStore, MessageStore, UserStore,
};

#[derive(Clone)]
pub struct Service {
    pub(crate) cfg: Config,
    pub(crate) pool: PgPool,
    pub(crate) user_store: UserStore,
    pub(crate) msg_store: MessageStore,
    pub(crate) group_store: GroupStore,
    pub(crate) group_mem_store: GroupMemberStore,
    pub(crate) friend_store: FriendRequestStore,
    pub(crate) conv_part_store: ConversationParticipantStore,
    pub(crate) idempotency_store: MessageIdempotencyStore,
    pub(crate) id_gen: IdGenerator,
    pub(crate) s3_service: Option<S3Service>,
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
            idempotency_store,
            id_gen,
            s3_service,
        }
    }

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
            is_recalled: 0,
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

    pub async fn get_user_by_id(&self, user_id: i64) -> Result<crate::store::User, crate::error::AppError> {
        let user = self
            .user_store
            .get_by_id(user_id)
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

    pub async fn get_user_by_username(&self, username: &str) -> Result<crate::store::User, crate::error::AppError> {
        let user = self
            .user_store
            .get_by_username(username)
            .await
            .map_err(|e| crate::error::AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
        user.ok_or_else(|| crate::error::AppError::not_found("user not found"))
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

pub async fn health_check(
    State(state): State<crate::AppState>,
) -> impl IntoResponse + use<> {
    Json(json!({
        "status": "ok",
        "node_id": state.svc.cfg.node_id,
        "timestamp": Utc::now().timestamp_millis(),
    }))
}

pub async fn readiness_check(
    State(state): State<crate::AppState>,
) -> impl IntoResponse + use<> {
    if let Err(e) = state.svc.pool.acquire().await {
        return (
            StatusCode::SERVICE_UNAVAILABLE,
            Json(json!({
                "status": "not ready",
                "reason": format!("database unavailable: {}", e),
            })),
        );
    }

    if !state.svc.cfg.s3_url.is_empty() && state.svc.s3_service.is_none() {
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
            "node_id": state.svc.cfg.node_id,
            "timestamp": Utc::now().timestamp_millis(),
        })),
    )
}

pub fn is_private_conv(conv_id: &str) -> bool {
    !conv_id.is_empty() && conv_id.starts_with('p')
}

pub fn private_conv_id(public_id_1: &str, public_id_2: &str) -> String {
    if public_id_1 < public_id_2 {
        format!("p_{}_{}", public_id_1, public_id_2)
    } else {
        format!("p_{}_{}", public_id_2, public_id_1)
    }
}

pub fn parse_private_conv(conv_id: &str) -> Option<(String, String)> {
    if !is_private_conv(conv_id) || conv_id.len() < 3 {
        return None;
    }
    let rest = &conv_id[2..];
    let last_underscore = rest.rfind('_')?;
    let a = rest[..last_underscore].to_string();
    let b = rest[last_underscore + 1..].to_string();
    Some((a, b))
}

pub fn can_access_private_conv(conv_id: &str, public_id: &str) -> bool {
    match parse_private_conv(conv_id) {
        Some((a, b)) => a == public_id || b == public_id,
        None => false,
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

pub fn extract_group_id(conv_id: &str) -> Option<String> {
    conv_id.strip_prefix("g_").map(|s| s.to_string())
}

pub fn get_other_public_id(conv_id: &str, my_public_id: &str) -> String {
    if !is_private_conv(conv_id) {
        return String::new();
    }
    match parse_private_conv(conv_id) {
        Some((a, b)) => {
            if a == my_public_id {
                b
            } else if b == my_public_id {
                a
            } else {
                String::new()
            }
        }
        None => String::new(),
    }
}
