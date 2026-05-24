use axum::extract::{Query, State};
use axum::http::StatusCode;
use axum::response::IntoResponse;
use axum::Json;
use chrono::{Duration, Utc};
use serde::{Deserialize, Serialize};
use serde_json::json;
use std::collections::HashMap;

use crate::error::AppError;
use crate::shared::{Handler, Service};
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
pub struct SendMessageResult {
    pub msg_id: String,
    pub conv_id: String,
    pub from_uid: i64,
    pub content: String,
    pub ts: i64,
    pub is_recalled: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub duplicate: Option<bool>,
}

#[derive(Debug, Deserialize)]
pub struct GetMessagesQuery {
    pub conv_id: String,
    pub date: Option<String>,
    pub days: Option<String>,
    pub page_size: Option<String>,
    pub last_msg_id: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct CheckNewMessagesQuery {
    pub last_msg_id: Option<String>,
}

#[derive(Debug, Deserialize)]
pub struct GetConversationsQuery {
    pub limit: Option<String>,
    pub offset: Option<String>,
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

impl Handler {
    pub async fn send_message(
        &self,
        user_id: i64,
        public_id: String,
        req: Json<SendMessageRequest>,
    ) -> Result<impl IntoResponse, AppError> {
        let req = req.0;

        let mut content = req.content;
        if req.content_type.as_deref() == Some("image") {
            let img = ImageContent {
                type_field: "image".to_string(),
                content: req.content,
                text: req.text.unwrap_or_default(),
            };
            content = serde_json::to_string(&img)
                .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
        }

        let result = self
            .svc
            .send_message(user_id, &public_id, &req.conv_id, &content, req.client_msg_id.as_deref())
            .await?;

        Ok((StatusCode::CREATED, Json(result)))
    }

    pub async fn get_messages(
        &self,
        user_id: i64,
        query: Query<GetMessagesQuery>,
    ) -> Result<impl IntoResponse, AppError> {
        let q = query.0;

        let mut days = 1;
        if let Some(v) = &q.days {
            days = v.parse::<i32>().map_err(|_| AppError::bad_request("invalid days"))?;
        }
        if days < 1 {
            days = 1;
        }
        if days > 7 {
            days = 7;
        }

        let mut page_size = self.svc.cfg.default_page_size as i64;
        if let Some(v) = &q.page_size {
            page_size = v.parse::<i64>().map_err(|_| AppError::bad_request("invalid pageSize"))?;
        }
        if page_size > self.svc.cfg.max_page_size as i64 {
            page_size = self.svc.cfg.max_page_size as i64;
        }

        let last_msg_id = if let Some(v) = &q.last_msg_id {
            v.parse::<i64>().map_err(|_| AppError::bad_request("invalid lastMsgId"))?
        } else {
            0
        };

        let end_date = if let Some(date_str) = &q.date {
            chrono::NaiveDate::parse_from_str(date_str, "%Y-%m-%d")
                .map_err(|_| AppError::bad_request("invalid date format"))?
                .and_hms_opt(0, 0, 0)
                .unwrap()
                .and_utc()
        } else {
            Utc::now()
        };

        let messages = self
            .svc
            .get_conversation_messages(
                &q.conv_id,
                end_date,
                days as usize,
                page_size as usize,
                last_msg_id,
                user_id,
            )
            .await?;

        Ok(Json(json!({
            "messages": messages.messages,
            "has_more": messages.has_more,
            "max_msg_id": messages.max_msg_id,
        })))
    }

    pub async fn recall_message(
        &self,
        user_id: i64,
        msg_id: i64,
    ) -> Result<impl IntoResponse, AppError> {
        self.svc.recall_message(msg_id, user_id).await?;

        Ok(Json(json!({
            "success": true,
            "message": "Message recalled successfully",
        })))
    }

    pub async fn check_new_messages(
        &self,
        user_id: i64,
        query: Query<CheckNewMessagesQuery>,
    ) -> Result<impl IntoResponse, AppError> {
        let last_msg_id = if let Some(v) = &query.0.last_msg_id {
            v.parse::<i64>().map_err(|_| AppError::bad_request("invalid lastMsgId"))?
        } else {
            0
        };

        let status = self.svc.check_new_messages(user_id, last_msg_id).await?;

        Ok(Json(json!({ "status": status })))
    }

    pub async fn block_user(
        &self,
        user_id: i64,
        public_id: String,
        target_public_id: String,
    ) -> Result<impl IntoResponse, AppError> {
        let target_user = self.svc.get_user_by_public_id(&target_public_id).await?;

        let conv_id = private_conv_id(&public_id, &target_public_id);

        self.svc.block_user(&conv_id, target_user.id).await?;

        Ok(Json(json!({ "message": "User blocked successfully" })))
    }

    pub async fn unblock_user(
        &self,
        user_id: i64,
        public_id: String,
        target_public_id: String,
    ) -> Result<impl IntoResponse, AppError> {
        let target_user = self.svc.get_user_by_public_id(&target_public_id).await?;

        let conv_id = private_conv_id(&public_id, &target_public_id);

        self.svc.unblock_user(&conv_id, target_user.id).await?;

        Ok(Json(json!({ "message": "User unblocked successfully" })))
    }

    pub async fn get_conversations(
        &self,
        user_id: i64,
        query: Query<GetConversationsQuery>,
    ) -> Result<impl IntoResponse, AppError> {
        let limit = if let Some(v) = &query.0.limit {
            let n = v.parse::<i32>().map_err(|_| AppError::bad_request("invalid limit"))?;
            if n < 1 || n > 100 {
                return Err(AppError::bad_request("invalid limit"));
            }
            n
        } else {
            20
        };

        let offset = if let Some(v) = &query.0.offset {
            let n = v.parse::<i32>().map_err(|_| AppError::bad_request("invalid offset"))?;
            if n < 0 {
                return Err(AppError::bad_request("invalid offset"));
            }
            n
        } else {
            0
        };

        let result = self
            .svc
            .get_conversation_list(user_id, limit as usize, offset as usize)
            .await?;

        Ok(Json(result))
    }
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
        if let Some(cmid) = client_msg_id {
            let (is_dup, existing_msg_id) = self
                .idempotency_store
                .check_and_insert(uid, cmid)
                .await
                .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

            if is_dup {
                return Ok(SendMessageResult {
                    msg_id: existing_msg_id.to_string(),
                    conv_id: conv_id.to_string(),
                    from_uid: uid,
                    content: content.to_string(),
                    ts: 0,
                    is_recalled: 0,
                    duplicate: Some(true),
                });
            }
        }

        let msg_type = extract_conversation_type(conv_id);

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

        let mut tx = self
            .pool
            .begin()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let target_user = if msg_type == "p" || msg_type == "user" {
            let target_public_id = get_other_public_id(conv_id, public_id);
            if target_public_id.is_empty() {
                return Err(AppError::bad_request("invalid target ID"));
            }
            Some(self.get_user_by_public_id(&target_public_id).await?)
        } else {
            None
        };

        self.ensure_session_permission_tx(
            &mut tx,
            uid,
            conv_id,
            &msg_type,
            target_user.as_ref(),
        )
        .await?;

        self.msg_store
            .insert_message_tx(&mut tx, &msg)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        if let Some(cmid) = client_msg_id {
            self.idempotency_store
                .update_msg_id_tx(&mut tx, uid, cmid, msg_id, conv_id)
                .await
                .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
        }

        tx.commit()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(SendMessageResult {
            msg_id: msg_id.to_string(),
            conv_id: conv_id.to_string(),
            from_uid: uid,
            content: content.to_string(),
            ts,
            is_recalled: 0,
            duplicate: None,
        })
    }

    pub async fn get_conversation_messages(
        &self,
        conv_id: &str,
        end_date: chrono::DateTime<Utc>,
        days: usize,
        page_size: usize,
        last_msg_id: i64,
        user_id: i64,
    ) -> Result<MessagePage, AppError> {
        let has_perm = self
            .conv_part_store
            .exists(conv_id, user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        if !has_perm {
            if let Some(group_id) = extract_group_id(conv_id) {
                let is_member = self
                    .is_user_in_group(&group_id, user_id)
                    .await
                    .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
                if !is_member {
                    return Err(AppError::forbidden());
                }
            } else {
                return Err(AppError::forbidden());
            }
        }

        let start_date = end_date - Duration::days((days - 1) as i64);
        let epoch_time = self.id_gen.get_epoch_time();
        let start_ts = start_date.timestamp_millis() - epoch_time;
        let end_ts = (end_date + Duration::days(1)).timestamp_millis() - epoch_time;

        self.msg_store
            .get_conversation_messages_by_range(conv_id, start_ts, end_ts, page_size as i64, last_msg_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))
    }

    pub async fn check_new_messages(
        &self,
        user_id: i64,
        last_msg_id: i64,
    ) -> Result<i32, AppError> {
        let has_new = self
            .msg_store
            .has_new_messages_after(user_id, last_msg_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let has_pending = self
            .friend_store
            .has_pending_requests(user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let mut status = 0;
        if has_new {
            status |= 1;
        }
        if has_pending {
            status |= 2;
        }

        Ok(status)
    }

    pub async fn recall_message(
        &self,
        msg_id: i64,
        user_id: i64,
    ) -> Result<(), AppError> {
        let msg = self
            .msg_store
            .get_message(msg_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

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

        self.msg_store
            .update_message(&updated_msg)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

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

        self.msg_store
            .insert_message(&recall_msg)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

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
                if !can_access_private_conv(conv_id, &my_public_id) {
                    return Err(AppError::forbidden());
                }

                let any_blocked = self
                    .conv_part_store
                    .is_any_blocked(conv_id)
                    .await
                    .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
                if any_blocked {
                    return Err(AppError::forbidden());
                }

                if let Some(target) = target_user {
                    let are_friends = self
                        .friend_store
                        .are_friends(uid, target.id)
                        .await
                        .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
                    if !are_friends {
                        return Err(AppError::new(StatusCode::FORBIDDEN, "not friends"));
                    }
                }

                let has_perm = self
                    .conv_part_store
                    .exists_tx(tx, conv_id, uid)
                    .await
                    .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

                if !has_perm {
                    let target_user = target_user.ok_or_else(|| AppError::bad_request("invalid target ID"))?;
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
                        .await
                        .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
                }
                Ok(())
            }
            "g" | "group" => {
                let group_id = extract_group_id(conv_id)
                    .ok_or_else(|| AppError::bad_request("invalid group ID"))?;
                let is_member = self
                    .is_user_in_group(&group_id, uid)
                    .await
                    .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
                if !is_member {
                    return Err(AppError::new(StatusCode::FORBIDDEN, "not a group member"));
                }
                Ok(())
            }
            _ => Err(AppError::bad_request("invalid message type")),
        }
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

    pub async fn get_conversation_list(
        &self,
        user_id: i64,
        limit: usize,
        offset: usize,
    ) -> Result<ConversationListResponse, AppError> {
        let items = self
            .conv_part_store
            .get_conversation_list(user_id, limit as i64, offset as i64)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let mut user_ids = Vec::new();
        for item in &items {
            if item.r#type == "private" {
                if let Some(target_uid) = item.target_uid {
                    user_ids.push(target_uid);
                }
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

            if item.r#type == "private" {
                if let Some(target_uid) = item.target_uid {
                    if let Some(target_user) = users_map.get(&target_uid) {
                        conv.name = target_user.username.clone();
                        conv.public_id = Some(target_user.public_id.clone());
                        conv.username = Some(target_user.username.clone());
                    }
                }
            } else if item.r#type == "group" {
                if let Some(ref group_id) = item.group_id {
                    if let Some(group) = self
                        .group_store
                        .get_by_id(group_id)
                        .await
                        .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?
                    {
                        let member_count = self
                            .group_store
                            .get_member_count(&group.group_id)
                            .await
                            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
                        conv.name = group.name;
                        conv.group_id = Some(group.group_id);
                        conv.member_count = Some(member_count as usize);
                    }
                }
            }

            if let Some(last_msg_id) = item.last_msg_id {
                conv.last_message = Some(LastMessageInfo {
                    msg_id: last_msg_id,
                    content: item.content.clone().unwrap_or_default(),
                    from_uid: item.from_uid.unwrap_or(0),
                    ts: item.last_msg_ts.unwrap_or(0),
                    is_recalled: item.is_recalled.map(|v| v == 1).unwrap_or(false),
                });
            }

            conversations.push(conv);
        }

        Ok(ConversationListResponse {
            conversations,
            total: conversations.len(),
        })
    }
}

fn extract_conversation_type(conv_id: &str) -> String {
    if conv_id.starts_with("p_") {
        "p".to_string()
    } else if conv_id.starts_with("g_") {
        "g".to_string()
    } else {
        "unknown".to_string()
    }
}

fn extract_group_id(conv_id: &str) -> Option<String> {
    if conv_id.starts_with("g_") {
        Some(conv_id[2..].to_string())
    } else {
        None
    }
}

fn get_other_public_id(conv_id: &str, my_public_id: &str) -> String {
    if !conv_id.starts_with("p_") {
        return String::new();
    }
    let parts: Vec<&str> = conv_id[2..].split('_').collect();
    if parts.len() != 2 {
        return String::new();
    }
    if parts[0] == my_public_id {
        parts[1].to_string()
    } else {
        parts[0].to_string()
    }
}

fn private_conv_id(public_id_1: &str, public_id_2: &str) -> String {
    if public_id_1 < public_id_2 {
        format!("p_{}_{}", public_id_1, public_id_2)
    } else {
        format!("p_{}_{}", public_id_2, public_id_1)
    }
}

pub async fn send_message(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Extension(public_id): axum::extract::Extension<String>,
    req: Json<SendMessageRequest>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.send_message(user_id, public_id, req).await
}

pub async fn get_messages(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Query(params): axum::extract::Query<std::collections::HashMap<String, String>>,
) -> Result<impl IntoResponse, AppError> {
    let conv_id = params.get("conv_id").cloned().unwrap_or_default();
    let limit: usize = params.get("limit").and_then(|s| s.parse().ok()).unwrap_or(50);
    let offset: usize = params.get("offset").and_then(|s| s.parse().ok()).unwrap_or(0);
    state.handler.get_messages(user_id, conv_id, limit, offset).await
}

pub async fn recall_message(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Path(msg_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse, AppError> {
    let msg_id: i64 = msg_id.parse().map_err(|_| AppError::bad_request("invalid msg_id"))?;
    state.handler.recall_message(user_id, msg_id).await
}

pub async fn check_new_messages(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.check_new_messages(user_id).await
}

pub async fn get_conversations(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Query(params): axum::extract::Query<std::collections::HashMap<String, String>>,
) -> Result<impl IntoResponse, AppError> {
    let limit: usize = params.get("limit").and_then(|s| s.parse().ok()).unwrap_or(100);
    let offset: usize = params.get("offset").and_then(|s| s.parse().ok()).unwrap_or(0);
    state.handler.get_conversations(user_id, limit, offset).await
}

pub async fn block_user(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Extension(public_id): axum::extract::Extension<String>,
    axum::extract::Path(target_public_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.block_user(user_id, &public_id, &target_public_id).await
}

pub async fn unblock_user(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Extension(public_id): axum::extract::Extension<String>,
    axum::extract::Path(target_public_id): axum::extract::Path<String>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.unblock_user(user_id, &public_id, &target_public_id).await
}

fn can_access_private_conv(conv_id: &str, my_public_id: &str) -> bool {
    if !conv_id.starts_with("p_") {
        return false;
    }
    conv_id.contains(my_public_id)
}

fn extract_file_key_from_message(msg: &Message) -> String {
    if let Ok(img) = serde_json::from_str::<ImageContent>(&msg.content) {
        if img.type_field == "image" && !img.content.is_empty() {
            return img.content;
        }
    }
    String::new()
}
