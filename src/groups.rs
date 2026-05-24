use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::response::IntoResponse;
use axum::Json;
use rand::RngCore;
use serde::{Deserialize, Serialize};
use serde_json::json;

use crate::error::AppError;
use crate::shared::{Handler, Service};
use crate::store::{Group, GroupMember};

#[derive(Debug, Deserialize)]
pub struct CreateGroupRequest {
    pub name: String,
    #[serde(default)]
    pub description: String,
}

#[derive(Debug, Deserialize)]
pub struct UpdateGroupRequest {
    pub name: String,
}

#[derive(Debug, Serialize)]
pub struct CreateGroupResponse {
    pub group_id: String,
    pub name: String,
    pub owner_public_id: String,
}

#[derive(Debug, Serialize)]
pub struct GetGroupResponse {
    pub group_id: String,
    pub name: String,
    pub owner_id: i64,
}

impl Handler {
    pub async fn create_group(
        &self,
        user_id: i64,
        public_id: String,
        req: Json<CreateGroupRequest>,
    ) -> Result<impl IntoResponse, AppError> {
        let req = req.0;

        if !validate_group_name(&req.name) {
            return Err(AppError::bad_request("Group name must be 1-50 characters"));
        }

        let group_id = self.svc.create_group(&req.name, &req.description, user_id).await?;

        Ok((
            StatusCode::CREATED,
            Json(CreateGroupResponse {
                group_id,
                name: req.name,
                owner_public_id: public_id,
            }),
        ))
    }

    pub async fn get_group(
        &self,
        id: Path<String>,
    ) -> Result<impl IntoResponse, AppError> {
        let id = id.0;

        let group = self.svc.get_group_by_id(&id).await?;

        Ok(Json(GetGroupResponse {
            group_id: group.group_id,
            name: group.name,
            owner_id: group.owner_id,
        }))
    }

    pub async fn update_group(
        &self,
        id: Path<String>,
        user_id: i64,
        req: Json<UpdateGroupRequest>,
    ) -> Result<impl IntoResponse, AppError> {
        let id = id.0;
        let req = req.0;

        let group = self.svc.get_group_by_id(&id).await?;

        if group.owner_id != user_id {
            return Err(AppError::forbidden());
        }

        let mut updated_group = group;
        if !req.name.is_empty() {
            updated_group.name = req.name;
        }

        self.svc.update_group(&updated_group).await?;

        Ok(Json(json!({
            "group_id": updated_group.group_id,
            "name": updated_group.name,
        })))
    }

    pub async fn delete_group(
        &self,
        id: Path<String>,
        user_id: i64,
    ) -> Result<impl IntoResponse, AppError> {
        let id = id.0;

        let group = self.svc.get_group_by_id(&id).await?;

        if group.owner_id != user_id {
            return Err(AppError::forbidden());
        }

        self.svc.delete_group(&id).await?;

        Ok(StatusCode::NO_CONTENT)
    }

    pub async fn join_group(
        &self,
        id: Path<String>,
        user_id: i64,
    ) -> Result<impl IntoResponse, AppError> {
        let id = id.0;

        self.svc.add_member(&id, user_id, "member").await?;

        Ok(Json(json!({ "message": "Successfully joined group" })))
    }

    pub async fn leave_group(
        &self,
        id: Path<String>,
        user_id: i64,
    ) -> Result<impl IntoResponse, AppError> {
        let id = id.0;

        self.svc.remove_member(&id, user_id).await?;

        Ok(Json(json!({ "message": "Successfully left group" })))
    }

    pub async fn kick_member(
        &self,
        id: Path<String>,
        member_public_id: Path<String>,
        owner_id: i64,
    ) -> Result<impl IntoResponse, AppError> {
        let group_id = id.0;
        let member_public_id = member_public_id.0;

        let member_user = self.svc.get_user_by_public_id(&member_public_id).await?;

        self.svc.kick_member(&group_id, member_user.id, owner_id).await?;

        Ok(Json(json!({ "message": "Member kicked successfully" })))
    }
}

impl Service {
    pub async fn create_group(
        &self,
        name: &str,
        description: &str,
        owner_id: i64,
    ) -> Result<String, AppError> {
        let mut tx = self
            .pool
            .begin()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let now = chrono::Utc::now().timestamp_millis();
        let group_id = self.generate_group_id();

        let group = Group {
            group_id: group_id.clone(),
            name: name.to_string(),
            description: description.to_string(),
            owner_id,
            created_at: now,
            updated_at: now,
            is_deleted: 0,
        };

        sqlx::query(
            r#"INSERT INTO groups (group_id, name, description, owner_id, created_at, updated_at, is_deleted)
               VALUES ($1, $2, $3, $4, $5, $6, $7)"#,
        )
        .bind(&group.group_id)
        .bind(&group.name)
        .bind(&group.description)
        .bind(group.owner_id)
        .bind(group.created_at)
        .bind(group.updated_at)
        .bind(group.is_deleted)
        .execute(&mut *tx)
        .await
        .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        sqlx::query(
            r#"INSERT INTO group_members (group_id, uid, role, joined_at)
               VALUES ($1, $2, $3, $4)"#,
        )
        .bind(&group_id)
        .bind(owner_id)
        .bind("owner")
        .bind(now)
        .execute(&mut *tx)
        .await
        .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        tx.commit()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(group_id)
    }

    pub async fn get_group_by_id(&self, group_id: &str) -> Result<Group, AppError> {
        let group = self
            .group_store
            .get_by_id(group_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
        group.ok_or_else(|| AppError::not_found("group not found"))
    }

    pub async fn update_group(&self, group: &Group) -> Result<(), AppError> {
        let mut updated = group.clone();
        updated.updated_at = chrono::Utc::now().timestamp_millis();

        self.group_store
            .update(&updated)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(())
    }

    pub async fn delete_group(&self, group_id: &str) -> Result<(), AppError> {
        let mut tx = self
            .pool
            .begin()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        self.group_mem_store
            .delete_by_group_tx(&mut tx, group_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        sqlx::query(r#"UPDATE groups SET is_deleted = 1 WHERE group_id = $1"#)
            .bind(group_id)
            .execute(&mut *tx)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        tx.commit()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(())
    }

    pub async fn add_member(
        &self,
        group_id: &str,
        user_id: i64,
        role: &str,
    ) -> Result<(), AppError> {
        let group = self.get_group_by_id(group_id).await?;

        let now = chrono::Utc::now().timestamp_millis();
        let member = GroupMember {
            id: 0,
            group_id: group_id.to_string(),
            uid: user_id,
            role: role.to_string(),
            joined_at: now,
        };

        let id = self
            .group_mem_store
            .insert(&member)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        if id == 0 {
            return Err(AppError::new(StatusCode::CONFLICT, "Already a member"));
        }

        let svc = self.clone();
        let group_id_str = group_id.to_string();
        let group_name = group.name.clone();
        let owner_id = group.owner_id;
        tokio::spawn(async move {
            let _ = svc
                .send_group_invite_notification(user_id, &group_id_str, &group_name, owner_id)
                .await;
        });

        Ok(())
    }

    pub async fn remove_member(
        &self,
        group_id: &str,
        user_id: i64,
    ) -> Result<(), AppError> {
        let mut tx = self
            .pool
            .begin()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let member = self
            .group_mem_store
            .get_tx(&mut tx, group_id, user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        match member {
            None => return Err(AppError::new(StatusCode::NOT_FOUND, "Not a group member")),
            Some(m) if m.role == "owner" => return Err(AppError::forbidden()),
            _ => {}
        }

        self.group_mem_store
            .delete_by_group_and_user_tx(&mut tx, group_id, user_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        tx.commit()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(())
    }

    pub async fn kick_member(
        &self,
        group_id: &str,
        member_id: i64,
        owner_id: i64,
    ) -> Result<(), AppError> {
        let group = self.get_group_by_id(group_id).await?;

        if group.owner_id != owner_id {
            return Err(AppError::forbidden());
        }

        if member_id == owner_id {
            return Err(AppError::forbidden());
        }

        let kicked_user = self.get_user_by_id(member_id).await?;

        let mut tx = self
            .pool
            .begin()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let is_member = self
            .group_mem_store
            .is_member_tx(&mut tx, group_id, member_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        if !is_member {
            return Err(AppError::new(StatusCode::NOT_FOUND, "Not a group member"));
        }

        self.group_mem_store
            .delete_by_group_and_user_tx(&mut tx, group_id, member_id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        let conv_id = format!("g_{}", group_id);
        let system_msg = json!({
            "type": "system",
            "content": "member_kicked",
            "data": {
                "kicked_public_id": kicked_user.public_id,
            },
        });

        let content_bytes = serde_json::to_string(&system_msg)
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        self.send_system_message_tx(&mut tx, &conv_id, &content_bytes)
            .await?;

        tx.commit()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        tracing::info!(
            group_id = group_id,
            member_public_id = kicked_user.public_id,
            owner_id = owner_id,
            "KickMember"
        );

        Ok(())
    }

    pub async fn send_system_message_tx(
        &self,
        tx: &mut sqlx::Transaction<'_, sqlx::Postgres>,
        conv_id: &str,
        content: &str,
    ) -> Result<i64, AppError> {
        let msg_id = self
            .id_gen
            .generate_id()
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e))?;

        let epoch_time = self.id_gen.get_epoch_time();
        let ts = chrono::Utc::now().timestamp_millis() - epoch_time;

        sqlx::query(
            r#"INSERT INTO messages (msg_id, conv_id, from_uid, content, ts, is_recalled)
               VALUES ($1, $2, $3, $4, $5, $6)"#,
        )
        .bind(msg_id)
        .bind(conv_id)
        .bind(0i64)
        .bind(content)
        .bind(ts)
        .bind(0i32)
        .execute(&mut **tx)
        .await
        .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;

        Ok(msg_id)
    }

    pub async fn get_user_by_id(&self, id: i64) -> Result<crate::store::User, AppError> {
        let user = self
            .user_store
            .get_by_id(id)
            .await
            .map_err(|e| AppError::new(StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()))?;
        user.ok_or_else(|| AppError::not_found("user not found"))
    }

    pub fn generate_group_id(&self) -> String {
        const CHARS: &[u8] = b"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
        let length = if self.cfg.group_id_random_length <= 0 {
            8
        } else {
            self.cfg.group_id_random_length as usize
        };

        let mut buf = vec![0u8; 2 + length];
        buf[0] = b'g';
        buf[1] = b'_';

        let mut rng = rand::thread_rng();
        for i in 0..length {
            let idx = (rng.next_u32() as usize) % CHARS.len();
            buf[2 + i] = CHARS[idx];
        }

        String::from_utf8(buf).unwrap_or_default()
    }
}

fn validate_group_name(name: &str) -> bool {
    let len = name.chars().count();
    len >= 1 && len <= 50
}

pub fn is_group_conv(conv_id: &str) -> bool {
    conv_id.len() >= 2 && conv_id.starts_with("g_")
}

pub fn extract_group_id(conv_id: &str) -> String {
    if is_group_conv(conv_id) {
        conv_id[2..].to_string()
    } else {
        String::new()
    }
}

pub async fn create_group(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    axum::extract::Extension(public_id): axum::extract::Extension<String>,
    req: Json<CreateGroupRequest>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.create_group(user_id, public_id, req).await
}

pub async fn get_group(
    State(state): State<crate::main::AppState>,
    id: Path<String>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.get_group(id).await
}

pub async fn update_group(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    id: Path<String>,
    req: Json<UpdateGroupRequest>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.update_group(id, user_id, req).await
}

pub async fn delete_group(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    id: Path<String>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.delete_group(id, user_id).await
}

pub async fn join_group(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    id: Path<String>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.join_group(id, user_id).await
}

pub async fn leave_group(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    id: Path<String>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.leave_group(id, user_id).await
}

pub async fn kick_member(
    State(state): State<crate::main::AppState>,
    axum::extract::Extension(user_id): axum::extract::Extension<i64>,
    Path((id, member_public_id)): Path<(String, String)>,
) -> Result<impl IntoResponse, AppError> {
    state.handler.kick_member(Path(id), Path(member_public_id), user_id).await
}
