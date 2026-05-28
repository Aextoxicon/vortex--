use axum::Json;
use axum::extract::{Query, State};
use axum::http::StatusCode;
use axum::response::IntoResponse;
use chrono::Utc;
use serde::{Deserialize, Serialize};
use serde_json::json;
use std::collections::HashMap;

use crate::error::AppError;
use crate::shared::Service;
use crate::store::{ConversationParticipant, Message, MessagePage};

#[derive(Debug, Deserialize)]
pub struct SendMessageRequest {
    pub conv_id: String,
    pub content: String,
    #[serde(default)]
    pub text: Option<String>,
    #[serde(default)]
    pub content_type: Option<String>,
    #[serde(default)]
    pub client_msg_id: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct SendMessageResponse {
    pub message: MessageResponseData,
}

#[derive(Debug, Serialize)]
pub struct MessageResponseData {
    pub id: String,
    pub conv_id: String,
    pub sender_id: String,
    pub content: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub content_type: Option<String>,
    pub created_at: String,
}

#[derive(Debug)]
pub struct SendMessageResult {
    pub msg_id: i64,
}

#[derive(Debug, Deserialize)]
pub struct GetMessagesQuery {
    pub conv_id: String,
    pub page: Option<String>,
    pub page_size: Option<String>,
    #[serde(rename = "lastMsgId")]
    pub last_msg_id: Option<i64>,
}

#[derive(Debug, Deserialize)]
pub struct CheckNewMessagesQuery {
    #[serde(rename = "lastMsgId")]
    pub last_msg_id: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct ConversationListResponse {
    pub conversations: Vec<ConversationItem>,
    pub total: usize,
}

#[derive(Debug, Serialize)]
pub struct ConversationItem {
    pub conv_id: String,
    #[serde(rename = "type")]
    pub type_field: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub public_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub username: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub group_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub member_count: Option<usize>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub last_message: Option<LastMessageInfo>,
    pub unread_count: usize,
}

#[derive(Debug, Serialize)]
pub struct LastMessageInfo {
    pub msg_id: i64,
    pub content: String,
    pub from_uid: i64,
    pub ts: i64,
    pub is_recalled: bool,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ImageContent {
    #[serde(rename = "type")]
    pub type_field: String,
    pub content: String,
    pub text: String,
}

#[derive(Debug, Serialize)]
pub struct MessageResponseItem {
    pub id: String,
    pub conv_id: String,
    pub sender_id: String,
    pub content: String,
    pub content_type: String,
    pub created_at: String,
}

fn messages_to_response_items(
    messages: &[Message],
    users_map: &HashMap<i64, crate::store::User>,
    epoch_time: i64,
) -> Vec<MessageResponseItem> {
    messages
        .iter()
        .map(|m| {
            let sender_id = users_map
                .get(&m.from_uid)
                .map(|u| u.public_id.clone())
                .unwrap_or_default();

            let ts_ms = epoch_time + m.ts;
            let secs = ts_ms / 1000;
            let nsecs = ((ts_ms % 1000) * 1_000_000) as u32;
            let created_at = chrono::DateTime::from_timestamp(secs, nsecs)
                .map(|dt| dt.to_rfc3339())
                .unwrap_or_default();

            let content_type = if m.content.starts_with("{\"type\":\"image\"") {
                "image".to_string()
            } else {
                "text".to_string()
            };

            let content = if content_type == "image" {
                if let Ok(img) = serde_json::from_str::<ImageContent>(&m.content) {
                    img.text.clone()
                } else {
                    m.content.clone()
                }
            } else {
                m.content.clone()
            };

            MessageResponseItem {
                id: format!("msg_{}", m.msg_id),
                conv_id: m.conv_id.clone(),
                sender_id,
                content,
                content_type,
                created_at,
            }
        })
        .collect()
}

impl Service {
    pub async fn send_message(
        &self,
        uid: i64,
        public_id: &str,
        conv_id: &str,
        content: &str,
        client_msg_id: Option<&str>,
    ) -> Result<SendMessageResult, AppError> {
        if content.len() > 1000 {
            return Err(AppError::bad_request(
                "message content too long (max 1000 characters)",
            ));
        }

        if let Some(cmid) = client_msg_id {
            let (is_dup, existing_msg_id) =
                self.idempotency_store.check_and_insert(uid, cmid).await?;

            if is_dup {
                return Ok(SendMessageResult {
                    msg_id: existing_msg_id,
                });
            }
        }

        let result = self
            .send_message_inner(uid, public_id, conv_id, content, client_msg_id)
            .await;

        if result.is_err()
            && let Some(cmid) = client_msg_id
            && let Err(e) = self
                .idempotency_store
                .update_msg_id(uid, cmid, 0, conv_id)
                .await
        {
            tracing::warn!("idempotency cleanup failed: {}", e);
        }

        result
    }

    async fn send_message_inner(
        &self,
        uid: i64,
        public_id: &str,
        conv_id: &str,
        content: &str,
        client_msg_id: Option<&str>,
    ) -> Result<SendMessageResult, AppError> {
        let msg_type = crate::shared::extract_conversation_type(conv_id);

        let msg_id = self
            .id_gen
            .generate_id()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e))?;

        let epoch_time = self.id_gen.get_epoch_time();
        let ts = Utc::now().timestamp_millis() - epoch_time;

        let msg = Message {
            msg_id,
            conv_id: conv_id.to_string(),
            from_uid: uid,
            content: content.to_string(),
            ts,
            is_recalled: 0,
        };

        let mut tx = self.pool.begin().await?;

        let target_user = if msg_type == "p" || msg_type == "user" {
            let target_public_id = crate::shared::get_other_public_id(conv_id, public_id);
            if target_public_id.is_empty() {
                return Err(AppError::bad_request("invalid target ID"));
            }
            Some(self.get_user_by_public_id(&target_public_id).await?)
        } else {
            None
        };

        self.ensure_session_permission_tx(&mut tx, uid, conv_id, msg_type, target_user.as_ref())
            .await?;

        self.msg_store.insert_message_tx(&mut tx, &msg).await?;

        if let Some(cmid) = client_msg_id {
            self.idempotency_store
                .update_msg_id_tx(&mut tx, uid, cmid, msg_id, conv_id)
                .await?;
        }

        tx.commit().await?;

        Ok(SendMessageResult { msg_id })
    }

    async fn ensure_conv_access(&self, conv_id: &str, user_id: i64) -> Result<(), AppError> {
        let has_perm = self.conv_part_store.exists(conv_id, user_id).await?;

        if !has_perm {
            if let Some(group_id) = crate::shared::extract_group_id(conv_id) {
                let is_member = self.is_user_in_group(&group_id, user_id).await?;
                if !is_member {
                    return Err(AppError::forbidden());
                }
            } else {
                return Err(AppError::forbidden());
            }
        }

        Ok(())
    }

    pub async fn get_conversation_messages_paginated(
        &self,
        conv_id: &str,
        limit: usize,
        offset: usize,
        user_id: i64,
    ) -> Result<MessagePage, AppError> {
        self.ensure_conv_access(conv_id, user_id).await?;

        let query_limit = (limit + 1) as i64;
        let messages = self
            .msg_store
            .get_conversation_messages(conv_id, query_limit, offset as i64)
            .await?;

        let has_more = messages.len() > limit;
        let messages = if has_more {
            messages[..limit].to_vec()
        } else {
            messages
        };

        let max_msg_id = messages.last().map(|m| m.msg_id).unwrap_or(0);

        Ok(MessagePage {
            messages,
            has_more,
            max_msg_id,
        })
    }

    pub async fn get_conversation_messages_after(
        &self,
        conv_id: &str,
        last_msg_id: i64,
        limit: usize,
        user_id: i64,
    ) -> Result<MessagePage, AppError> {
        self.ensure_conv_access(conv_id, user_id).await?;

        self.msg_store
            .get_conversation_messages_after(conv_id, last_msg_id, limit as i64)
            .await
            .map_err(AppError::from)
    }

    pub async fn check_new_messages(
        &self,
        user_id: i64,
        last_msg_id: i64,
    ) -> Result<(i32, Vec<String>), AppError> {
        let updated = self
            .msg_store
            .get_updated_conversations(user_id, last_msg_id)
            .await?;

        let has_pending = self.friend_store.has_pending_requests(user_id).await?;

        let mut status = 0;
        if !updated.is_empty() {
            status |= 1;
        }
        if has_pending {
            status |= 2;
        }

        Ok((status, updated))
    }

    pub async fn recall_message(&self, msg_id: i64, user_id: i64) -> Result<(), AppError> {
        let msg = self.msg_store.get_message(msg_id).await?;

        let msg = msg.ok_or_else(|| AppError::not_found("message not found"))?;

        if msg.is_recalled == 1 {
            return Err(AppError::conflict("message already recalled"));
        }

        if msg.from_uid != user_id {
            return Err(AppError::forbidden());
        }

        let now = Utc::now().timestamp_millis();
        let msg_age = now - (msg.ts + self.id_gen.get_epoch_time());
        if msg_age > self.cfg.message_recall_window_ms {
            return Err(AppError::bad_request("recall window expired"));
        }

        let mut updated_msg = msg.clone();
        updated_msg.is_recalled = 1;
        updated_msg.content = String::new();

        self.msg_store.update_message(&updated_msg).await?;

        if self.s3_service.is_some() {
            let file_key = extract_file_key_from_message(&msg);
            if !file_key.is_empty() {
                tracing::warn!("S3 delete not implemented yet: file_key={}", file_key);
            }
        }

        let recall_msg_id = self
            .id_gen
            .generate_id()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e))?;

        let epoch_time = self.id_gen.get_epoch_time();
        let recall_ts = Utc::now().timestamp_millis() - epoch_time;

        let recall_msg = Message {
            msg_id: recall_msg_id,
            conv_id: msg.conv_id.clone(),
            from_uid: user_id,
            content: msg_id.to_string(),
            ts: recall_ts,
            is_recalled: 1,
        };

        self.msg_store.insert_message(&recall_msg).await?;

        Ok(())
    }

    async fn ensure_session_permission_tx(
        &self,
        tx: &mut sqlx::Transaction<'_, sqlx::Postgres>,
        uid: i64,
        conv_id: &str,
        msg_type: &str,
        target_user: Option<&crate::store::User>,
    ) -> Result<(), AppError> {
        match msg_type {
            "p" | "user" => {
                let my_public_id = self.get_public_id_by_user_id(uid).await?;
                if !crate::shared::can_access_private_conv(conv_id, &my_public_id) {
                    return Err(AppError::forbidden());
                }

                let any_blocked = self.conv_part_store.is_any_blocked(conv_id).await?;
                if any_blocked {
                    return Err(AppError::forbidden());
                }

                if let Some(target) = target_user {
                    let are_friends = self.friend_store.are_friends(uid, target.id).await?;
                    if !are_friends {
                        return Err(AppError::new(StatusCode::FORBIDDEN, "not friends"));
                    }
                }

                let has_perm = self.conv_part_store.exists_tx(tx, conv_id, uid).await?;

                if !has_perm {
                    let target_user =
                        target_user.ok_or_else(|| AppError::bad_request("invalid target ID"))?;
                    let now = Utc::now().timestamp_millis();
                    let participants = vec![
                        ConversationParticipant {
                            conv_id: conv_id.to_string(),
                            user_id: uid,
                            join_ts: now,
                            is_blocked: 0,
                        },
                        ConversationParticipant {
                            conv_id: conv_id.to_string(),
                            user_id: target_user.id,
                            join_ts: now,
                            is_blocked: 0,
                        },
                    ];
                    self.conv_part_store
                        .insert_batch_tx(tx, &participants)
                        .await?;
                }
                Ok(())
            }
            "g" | "group" => {
                let group_id = crate::shared::extract_group_id(conv_id)
                    .ok_or_else(|| AppError::bad_request("invalid group ID"))?;
                let is_member = self.is_user_in_group(&group_id, uid).await?;
                if !is_member {
                    return Err(AppError::new(StatusCode::FORBIDDEN, "not a group member"));
                }
                Ok(())
            }
            _ => Err(AppError::bad_request("invalid message type")),
        }
    }

    pub async fn get_conversation_list(
        &self,
        user_id: i64,
        limit: usize,
        offset: usize,
    ) -> Result<ConversationListResponse, AppError> {
        let items = self
            .conv_part_store
            .get_conversation_list(user_id, limit as i64, offset as i64)
            .await?;

        let mut user_ids = Vec::new();
        for item in &items {
            if item.r#type == "private"
                && let Some(target_uid) = item.target_uid
            {
                user_ids.push(target_uid);
            }
        }

        let users_map = self.get_users_by_ids(&user_ids).await?;

        let mut conversations = Vec::new();
        for item in &items {
            let mut conv = ConversationItem {
                conv_id: item.conv_id.clone(),
                type_field: item.r#type.clone(),
                name: String::new(),
                public_id: None,
                username: None,
                group_id: None,
                member_count: None,
                last_message: None,
                unread_count: 0,
            };

            if item.r#type == "private"
                && let Some(target_uid) = item.target_uid
                && let Some(target_user) = users_map.get(&target_uid)
            {
                conv.name = target_user.username.clone();
                conv.public_id = Some(target_user.public_id.clone());
                conv.username = Some(target_user.username.clone());
            } else if item.r#type == "group"
                && let Some(ref group_id) = item.group_id
            {
                conv.group_id = Some(group_id.clone());
                if let Ok(Some(g)) = self.group_store.get_by_id(group_id).await {
                    conv.name = g.name.clone();
                    if let Ok(members) = self.group_mem_store.get_members(group_id).await {
                        conv.member_count = Some(members.len());
                    }
                }
            }

            if let (Some(msg_id), Some(from_uid), Some(content), Some(ts), Some(is_recalled)) = (
                item.last_msg_id,
                item.from_uid,
                &item.content,
                item.last_msg_ts,
                item.is_recalled,
            ) {
                conv.last_message = Some(LastMessageInfo {
                    msg_id,
                    content: content.clone(),
                    from_uid,
                    ts,
                    is_recalled: is_recalled != 0,
                });
            }

            conversations.push(conv);
        }

        let total = self
            .conv_part_store
            .count_user_conversations(user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(ConversationListResponse {
            conversations,
            total: total as usize,
        })
    }
}

fn extract_file_key_from_message(msg: &Message) -> String {
    if let Ok(img) = serde_json::from_str::<ImageContent>(&msg.content)
        && img.type_field == "image"
        && !img.content.is_empty()
    {
        if let Ok(url) = url::Url::parse(&img.content) {
            return url.path().trim_start_matches('/').to_string();
        }
        return img.content.clone();
    }
    String::new()
}

pub async fn send_message(
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Extension(public_id): axum::extract::Extension<String>,
    req: Json<SendMessageRequest>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let req = req.0;

    let content_type = req
        .content_type
        .clone()
        .unwrap_or_else(|| "text".to_string());

    let mut content = req.content.clone();
    if content_type == "image" {
        let img = ImageContent {
            type_field: "image".to_string(),
            content: req.content,
            text: req.text.unwrap_or_default(),
        };
        content = serde_json::to_string(&img)
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
    }

    let result = state
        .svc
        .send_message(
            user_id,
            &public_id,
            &req.conv_id,
            &content,
            req.client_msg_id.as_deref(),
        )
        .await?;

    let created_at = chrono::Utc::now().to_rfc3339();

    Ok((
        StatusCode::CREATED,
        Json(SendMessageResponse {
            message: MessageResponseData {
                id: result.msg_id.to_string(),
                conv_id: req.conv_id,
                sender_id: public_id,
                content,
                content_type: Some(content_type),
                created_at,
            },
        }),
    ))
}

pub async fn get_messages(
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    query: Query<GetMessagesQuery>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let q = query.0;

    let page_size = if let Some(v) = &q.page_size {
        let n = v
            .parse::<usize>()
            .map_err(|_| AppError::bad_request("invalid pageSize"))?;
        if n > state.svc.cfg.max_page_size as usize {
            state.svc.cfg.max_page_size as usize
        } else if n < 1 {
            state.svc.cfg.default_page_size as usize
        } else {
            n
        }
    } else {
        state.svc.cfg.default_page_size as usize
    };

    let cursor_mode = q.last_msg_id.filter(|&id| id > 0).is_some();

    let result = if let Some(last_msg_id) = q.last_msg_id.filter(|&id| id > 0) {
        state
            .svc
            .get_conversation_messages_after(&q.conv_id, last_msg_id, page_size, user_id)
            .await?
    } else {
        let page = if let Some(v) = &q.page {
            let n = v
                .parse::<usize>()
                .map_err(|_| AppError::bad_request("invalid page"))?;
            if n < 1 { 1 } else { n }
        } else {
            1
        };

        let offset = (page - 1) * page_size;

        state
            .svc
            .get_conversation_messages_paginated(&q.conv_id, page_size, offset, user_id)
            .await?
    };

    let users_map = state
        .svc
        .get_users_by_ids(
            &result
                .messages
                .iter()
                .map(|m| m.from_uid)
                .collect::<Vec<_>>(),
        )
        .await?;

    let epoch_time = state.svc.id_gen.get_epoch_time();
    let messages = messages_to_response_items(&result.messages, &users_map, epoch_time);

    let mut response = json!({
        "messages": messages,
        "page_size": page_size,
        "has_more": result.has_more,
    });

    if cursor_mode {
        response["last_msg_id"] = json!(result.max_msg_id);
    }

    Ok(Json(response))
}

pub async fn recall_message(
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Path(msg_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let msg_id: i64 = msg_id
        .parse()
        .map_err(|_| AppError::bad_request("invalid message ID"))?;
    state.svc.recall_message(msg_id, user_id).await?;

    Ok(Json(json!({
        "message": "message recalled successfully",
    })))
}

pub async fn check_new_messages(
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    query: Query<CheckNewMessagesQuery>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let last_msg_id: i64 = query
        .last_msg_id
        .as_deref()
        .and_then(|s| s.parse().ok())
        .unwrap_or(0);

    let (status, updated) = state.svc.check_new_messages(user_id, last_msg_id).await?;

    Ok(Json(json!({ "status": status, "updated": updated })))
}

pub async fn get_conversations(
    State(state): State<crate::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    query: Query<GetConversationsQuery>,
) -> Result<impl IntoResponse + use<>, AppError> {
    let limit = query.0.limit.unwrap_or(20);
    let offset = query.0.offset.unwrap_or(0);

    if !(1..=100).contains(&limit) {
        return Err(AppError::bad_request("invalid limit"));
    }
    if offset < 0 {
        return Err(AppError::bad_request("invalid offset"));
    }

    let result = state
        .svc
        .get_conversation_list(user_id, limit as usize, offset as usize)
        .await?;

    Ok(Json(result))
}

#[derive(Debug, Deserialize)]
pub struct GetConversationsQuery {
    pub limit: Option<i32>,
    pub offset: Option<i32>,
}
