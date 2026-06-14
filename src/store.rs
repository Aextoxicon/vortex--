use sqlx::{FromRow, PgPool, Postgres, Transaction};

#[derive(Debug, Clone)]
pub struct Store {
    pub pool: PgPool,
    pub epoch_time: i64,
}

impl Store {
    pub fn new(pool: PgPool, epoch_time: i64) -> Self {
        Self { pool, epoch_time }
    }

    pub fn set_epoch_time(&mut self, epoch_time: i64) {
        self.epoch_time = epoch_time;
    }
}

#[derive(Debug, Clone, FromRow)]
pub struct User {
    pub id: i64,
    pub username: String,
    pub pwd_hash: String,
    pub email: String,
    pub public_id: String,
    pub created_at: i64,
    pub updated_at: i64,
}

#[derive(Debug, Clone, FromRow)]
pub struct Group {
    pub group_id: String,
    pub name: String,
    pub description: String,
    pub owner_id: i64,
    pub created_at: i64,
    pub updated_at: i64,
    pub is_deleted: i32,
}

#[derive(Debug, Clone, FromRow)]
pub struct GroupMember {
    pub id: i64,
    pub group_id: String,
    pub uid: i64,
    pub role: String,
    pub joined_at: i64,
}

#[derive(Debug, Clone, FromRow)]
pub struct FriendRequest {
    pub id: i64,
    pub from_user_id: i64,
    pub to_user_id: i64,
    pub message: String,
    pub status: String,
    pub created_at: i64,
    pub updated_at: i64,
}

#[derive(Debug, Clone, FromRow)]
pub struct Message {
    pub msg_id: i64,
    pub conv_id: String,
    pub from_uid: i64,
    pub content: String,
    pub ts: i64,
    pub is_recalled: i32,
}

#[derive(Debug, Clone, FromRow)]
pub struct ConversationParticipant {
    pub conv_id: String,
    pub user_id: i64,
    pub join_ts: i64,
    pub is_blocked: i32,
}

#[derive(Debug, Clone, FromRow)]
pub struct IdGeneratorState {
    pub id: i64,
    pub last_ts: i64,
    pub epoch_time: i64,
}

#[derive(Debug, Clone, FromRow)]
pub struct MessageIdempotency {
    pub id: i64,
    pub user_id: i64,
    pub client_msg_id: String,
    pub msg_id: i64,
    pub created_at: i64,
}

#[derive(Debug, Clone, FromRow)]
pub struct ConversationListItem {
    pub conv_id: String,
    pub r#type: String,
    pub target_uid: Option<i64>,
    pub group_id: Option<String>,
    pub last_msg_id: Option<i64>,
    pub from_uid: Option<i64>,
    pub content: Option<String>,
    pub last_msg_ts: Option<i64>,
    pub is_recalled: Option<i32>,
}

pub struct MessagePage {
    pub messages: Vec<Message>,
    pub has_more: bool,
    pub max_msg_id: i64,
}

#[derive(Debug, Clone)]
pub struct UserStore {
    pub store: Store,
}

impl UserStore {
    pub fn new(store: Store) -> Self {
        Self { store }
    }

    pub async fn get_by_ids(
        &self,
        ids: &[i64],
    ) -> Result<std::collections::HashMap<i64, User>, sqlx::Error> {
        if ids.is_empty() {
            return Ok(std::collections::HashMap::new());
        }

        let query = r#"SELECT id, username, pwd_hash, email, public_id, created_at, updated_at 
                       FROM users WHERE id = ANY($1)"#;
        let rows = sqlx::query_as::<_, User>(query)
            .bind(ids)
            .fetch_all(&self.store.pool)
            .await?;

        let mut result = std::collections::HashMap::new();
        for user in rows {
            result.insert(user.id, user);
        }
        Ok(result)
    }

    pub async fn get_by_id(&self, id: i64) -> Result<Option<User>, sqlx::Error> {
        let query = r#"SELECT id, username, pwd_hash, email, public_id, created_at, updated_at 
                       FROM users WHERE id = $1"#;
        sqlx::query_as::<_, User>(query)
            .bind(id)
            .fetch_optional(&self.store.pool)
            .await
    }

    pub async fn get_by_username(&self, username: &str) -> Result<Option<User>, sqlx::Error> {
        let query = r#"SELECT id, username, pwd_hash, email, public_id, created_at, updated_at 
                       FROM users WHERE username = $1"#;
        sqlx::query_as::<_, User>(query)
            .bind(username)
            .fetch_optional(&self.store.pool)
            .await
    }

    pub async fn get_by_public_id(&self, public_id: &str) -> Result<Option<User>, sqlx::Error> {
        let query = r#"SELECT id, username, pwd_hash, email, public_id, created_at, updated_at 
                       FROM users WHERE public_id = $1"#;
        sqlx::query_as::<_, User>(query)
            .bind(public_id)
            .fetch_optional(&self.store.pool)
            .await
    }

    pub async fn insert(&self, user: &User) -> Result<i64, sqlx::Error> {
        let query = r#"
            INSERT INTO users (username, pwd_hash, email, public_id, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5, $6)
            RETURNING id"#;
        let id: i64 = sqlx::query_scalar(query)
            .bind(&user.username)
            .bind(&user.pwd_hash)
            .bind(&user.email)
            .bind(&user.public_id)
            .bind(user.created_at)
            .bind(user.updated_at)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(id)
    }

    pub async fn update(&self, user: &User) -> Result<u64, sqlx::Error> {
        let query = r#"
            UPDATE users SET username = $1, email = $2, updated_at = $3
            WHERE id = $4"#;
        let result = sqlx::query(query)
            .bind(&user.username)
            .bind(&user.email)
            .bind(user.updated_at)
            .bind(user.id)
            .execute(&self.store.pool)
            .await?;
        Ok(result.rows_affected())
    }

    pub async fn delete_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        id: i64,
    ) -> Result<u64, sqlx::Error> {
        let query = r#"DELETE FROM users WHERE id = $1"#;
        let result = sqlx::query(query).bind(id).execute(&mut **tx).await?;
        Ok(result.rows_affected())
    }

    pub async fn username_exists(&self, username: &str) -> Result<bool, sqlx::Error> {
        let query = r#"SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)"#;
        let exists: bool = sqlx::query_scalar(query)
            .bind(username)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(exists)
    }

    pub async fn email_exists(&self, email: &str) -> Result<bool, sqlx::Error> {
        if email.is_empty() {
            return Ok(false);
        }
        let query = r#"SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)"#;
        let exists: bool = sqlx::query_scalar(query)
            .bind(email)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(exists)
    }
}

#[derive(Debug, Clone)]
pub struct MessageIdempotencyStore {
    pub store: Store,
}

impl MessageIdempotencyStore {
    pub fn new(store: Store) -> Self {
        Self { store }
    }

    pub async fn check_and_insert(
        &self,
        user_id: i64,
        client_msg_id: &str,
    ) -> Result<(bool, i64), sqlx::Error> {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("system time should always be valid")
            .as_millis() as i64;

        let query = r#"
            INSERT INTO message_idempotency (user_id, client_msg_id, msg_id, created_at)
            VALUES ($1, $2, 0, $3)
            ON CONFLICT (user_id, client_msg_id) DO UPDATE SET user_id = EXCLUDED.user_id
            RETURNING msg_id"#;
        let existing_id: i64 = sqlx::query_scalar(query)
            .bind(user_id)
            .bind(client_msg_id)
            .bind(now)
            .fetch_one(&self.store.pool)
            .await?;

        if existing_id > 0 {
            Ok((true, existing_id))
        } else {
            Ok((false, 0))
        }
    }

    pub async fn update_msg_id(
        &self,
        user_id: i64,
        client_msg_id: &str,
        msg_id: i64,
    ) -> Result<(), sqlx::Error> {
        let query = r#"
            UPDATE message_idempotency 
            SET msg_id = $1 
            WHERE user_id = $2 AND client_msg_id = $3"#;
        sqlx::query(query)
            .bind(msg_id)
            .bind(user_id)
            .bind(client_msg_id)
            .execute(&self.store.pool)
            .await?;
        Ok(())
    }

    pub async fn update_msg_id_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        user_id: i64,
        client_msg_id: &str,
        msg_id: i64,
    ) -> Result<(), sqlx::Error> {
        let query = r#"
            UPDATE message_idempotency 
            SET msg_id = $1 
            WHERE user_id = $2 AND client_msg_id = $3"#;
        sqlx::query(query)
            .bind(msg_id)
            .bind(user_id)
            .bind(client_msg_id)
            .execute(&mut **tx)
            .await?;
        Ok(())
    }

    pub async fn delete_stale(&self, retention_ms: i64) -> Result<u64, sqlx::Error> {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .expect("system time should always be valid")
            .as_millis() as i64;
        let cutoff = now - retention_ms;

        let query = r#"
            DELETE FROM message_idempotency 
            WHERE created_at < $1"#;
        let result = sqlx::query(query)
            .bind(cutoff)
            .execute(&self.store.pool)
            .await?;
        Ok(result.rows_affected())
    }
}

#[derive(Debug, Clone)]
pub struct MessageStore {
    pub store: Store,
}

impl MessageStore {
    pub fn new(store: Store) -> Self {
        Self { store }
    }

    pub async fn get_message(&self, msg_id: i64) -> Result<Option<Message>, sqlx::Error> {
        let query = r#"
            SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
            FROM messages WHERE msg_id = $1"#;
        sqlx::query_as::<_, Message>(query)
            .bind(msg_id)
            .fetch_optional(&self.store.pool)
            .await
    }

    pub async fn get_conversation_messages(
        &self,
        conv_id: &str,
        limit: i64,
        offset: i64,
    ) -> Result<Vec<Message>, sqlx::Error> {
        let query = r#"
            SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
            FROM messages
            WHERE conv_id = $1
            ORDER BY ts DESC
            LIMIT $2 OFFSET $3"#;
        sqlx::query_as::<_, Message>(query)
            .bind(conv_id)
            .bind(limit)
            .bind(offset)
            .fetch_all(&self.store.pool)
            .await
    }

    pub async fn insert_message(&self, msg: &Message) -> Result<i64, sqlx::Error> {
        let query = r#"
            INSERT INTO messages (msg_id, conv_id, from_uid, content, ts, is_recalled)
            VALUES ($1, $2, $3, $4, $5, $6)
            RETURNING msg_id"#;
        let id: i64 = sqlx::query_scalar(query)
            .bind(msg.msg_id)
            .bind(&msg.conv_id)
            .bind(msg.from_uid)
            .bind(&msg.content)
            .bind(msg.ts)
            .bind(msg.is_recalled)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(id)
    }

    pub async fn insert_message_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        msg: &Message,
    ) -> Result<i64, sqlx::Error> {
        let query = r#"
            INSERT INTO messages (msg_id, conv_id, from_uid, content, ts, is_recalled)
            VALUES ($1, $2, $3, $4, $5, $6)
            RETURNING msg_id"#;
        let id: i64 = sqlx::query_scalar(query)
            .bind(msg.msg_id)
            .bind(&msg.conv_id)
            .bind(msg.from_uid)
            .bind(&msg.content)
            .bind(msg.ts)
            .bind(msg.is_recalled)
            .fetch_one(&mut **tx)
            .await?;
        Ok(id)
    }

    pub async fn update_message(&self, msg: &Message) -> Result<u64, sqlx::Error> {
        let query = r#"
            UPDATE messages SET conv_id = $1, from_uid = $2, content = $3, ts = $4, is_recalled = $5
            WHERE msg_id = $6"#;
        let result = sqlx::query(query)
            .bind(&msg.conv_id)
            .bind(msg.from_uid)
            .bind(&msg.content)
            .bind(msg.ts)
            .bind(msg.is_recalled)
            .bind(msg.msg_id)
            .execute(&self.store.pool)
            .await?;
        Ok(result.rows_affected())
    }

    pub async fn get_conversation_messages_by_range(
        &self,
        conv_id: &str,
        start_ts: i64,
        end_ts: i64,
        limit: i64,
        last_msg_id: i64,
    ) -> Result<MessagePage, sqlx::Error> {
        let query_limit = limit + 1;

        let messages = if last_msg_id > 0 {
            let query = r#"
                SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
                FROM messages
                WHERE conv_id = $1 AND ts >= $2 AND ts < $3 AND msg_id > $4
                ORDER BY ts ASC, msg_id ASC
                LIMIT $5"#;
            sqlx::query_as::<_, Message>(query)
                .bind(conv_id)
                .bind(start_ts)
                .bind(end_ts)
                .bind(last_msg_id)
                .bind(query_limit)
                .fetch_all(&self.store.pool)
                .await?
        } else {
            let query = r#"
                SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
                FROM messages
                WHERE conv_id = $1 AND ts >= $2 AND ts < $3
                ORDER BY ts ASC, msg_id ASC
                LIMIT $4"#;
            sqlx::query_as::<_, Message>(query)
                .bind(conv_id)
                .bind(start_ts)
                .bind(end_ts)
                .bind(query_limit)
                .fetch_all(&self.store.pool)
                .await?
        };

        let has_more = messages.len() > limit as usize;
        let messages = if has_more {
            messages[..limit as usize].to_vec()
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
        limit: i64,
    ) -> Result<MessagePage, sqlx::Error> {
        let query_limit = limit + 1;
        let query = r#"
            SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
            FROM messages
            WHERE conv_id = $1 AND msg_id > $2
            ORDER BY msg_id ASC
            LIMIT $3"#;
        let messages = sqlx::query_as::<_, Message>(query)
            .bind(conv_id)
            .bind(last_msg_id)
            .bind(query_limit)
            .fetch_all(&self.store.pool)
            .await?;

        let has_more = messages.len() as i64 > limit;
        let messages = if has_more {
            messages[..limit as usize].to_vec()
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

    pub async fn ensure_partition(&self, table_name: &str) -> Result<(), sqlx::Error> {
        if table_name.len() < 10 {
            return Err(sqlx::Error::Decode("invalid table name".into()));
        }

        let date_str = &table_name[9..];
        let date = chrono::NaiveDate::parse_from_str(date_str, "%Y%m%d")
            .map_err(|e| sqlx::Error::Decode(format!("invalid date: {}", e).into()))?;

        let start_of_day = date
            .and_hms_opt(0, 0, 0)
            .ok_or_else(|| {
                sqlx::Error::Decode("midnight (00:00:00) should always be valid".into())
            })?
            .and_utc()
            .timestamp_millis()
            - self.store.epoch_time;

        let next_day = date
            .succ_opt()
            .ok_or_else(|| sqlx::Error::Decode("date should have a next day".into()))?;
        let end_of_day = next_day
            .and_hms_opt(0, 0, 0)
            .ok_or_else(|| {
                sqlx::Error::Decode("midnight (00:00:00) should always be valid".into())
            })?
            .and_utc()
            .timestamp_millis()
            - self.store.epoch_time;

        let quoted = table_name.replace("'", "''");

        let create_table_query = format!(
            r#"CREATE TABLE IF NOT EXISTS {} PARTITION OF messages
               FOR VALUES FROM ({}) TO ({})"#,
            quoted, start_of_day, end_of_day
        );
        sqlx::query(&create_table_query)
            .execute(&self.store.pool)
            .await?;

        let create_index_query = format!(
            r#"CREATE INDEX IF NOT EXISTS idx_msg_conv_ts_{} ON {} (conv_id, ts DESC)"#,
            &table_name[9..],
            quoted
        );
        sqlx::query(&create_index_query)
            .execute(&self.store.pool)
            .await?;

        let create_index_query2 = format!(
            r#"CREATE INDEX IF NOT EXISTS idx_msg_from_uid_{} ON {} (from_uid)"#,
            &table_name[9..],
            quoted
        );
        sqlx::query(&create_index_query2)
            .execute(&self.store.pool)
            .await?;

        Ok(())
    }

    pub async fn get_max_message_id(&self) -> Result<i64, sqlx::Error> {
        let query = r#"SELECT MAX(msg_id) FROM messages"#;
        let max_id: Option<i64> = sqlx::query_scalar(query)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(max_id.unwrap_or(0))
    }

    pub async fn get_updated_conversations(
        &self,
        user_id: i64,
        last_msg_id: i64,
    ) -> Result<Vec<String>, sqlx::Error> {
        let query = r#"
            SELECT DISTINCT conv_id
            FROM messages
            WHERE msg_id > $1
            AND conv_id IN (
                SELECT conv_id FROM conversation_participants WHERE user_id = $2
            )"#;
        let conv_ids: Vec<String> = sqlx::query_scalar(query)
            .bind(last_msg_id)
            .bind(user_id)
            .fetch_all(&self.store.pool)
            .await?;
        Ok(conv_ids)
    }
}

#[derive(Debug, Clone)]
pub struct GroupStore {
    pub store: Store,
}

impl GroupStore {
    pub fn new(store: Store) -> Self {
        Self { store }
    }

    pub async fn get_by_id(&self, group_id: &str) -> Result<Option<Group>, sqlx::Error> {
        let query = r#"
            SELECT group_id, name, description, owner_id, created_at, updated_at, is_deleted
            FROM groups WHERE group_id = $1 AND is_deleted = 0"#;
        sqlx::query_as::<_, Group>(query)
            .bind(group_id)
            .fetch_optional(&self.store.pool)
            .await
    }

    pub async fn get_groups_by_owner(&self, owner_id: i64) -> Result<Vec<Group>, sqlx::Error> {
        let query = r#"
            SELECT group_id, name, description, owner_id, created_at, updated_at, is_deleted
            FROM groups WHERE owner_id = $1 AND is_deleted = 0
            ORDER BY created_at DESC"#;
        sqlx::query_as::<_, Group>(query)
            .bind(owner_id)
            .fetch_all(&self.store.pool)
            .await
    }

    pub async fn insert(&self, group: &Group) -> Result<u64, sqlx::Error> {
        let query = r#"
            INSERT INTO groups (group_id, name, description, owner_id, created_at, updated_at, is_deleted)
            VALUES ($1, $2, $3, $4, $5, $6, $7)"#;
        let result = sqlx::query(query)
            .bind(&group.group_id)
            .bind(&group.name)
            .bind(&group.description)
            .bind(group.owner_id)
            .bind(group.created_at)
            .bind(group.updated_at)
            .bind(group.is_deleted)
            .execute(&self.store.pool)
            .await?;
        Ok(result.rows_affected())
    }

    pub async fn update(&self, group: &Group) -> Result<u64, sqlx::Error> {
        let query = r#"
            UPDATE groups SET name = $1, description = $2, owner_id = $3, updated_at = $4
            WHERE group_id = $5"#;
        let result = sqlx::query(query)
            .bind(&group.name)
            .bind(&group.description)
            .bind(group.owner_id)
            .bind(group.updated_at)
            .bind(&group.group_id)
            .execute(&self.store.pool)
            .await?;
        Ok(result.rows_affected())
    }

    pub async fn get_member_count(&self, group_id: &str) -> Result<i64, sqlx::Error> {
        let query = r#"SELECT COUNT(*) FROM group_members WHERE group_id = $1"#;
        let count: i64 = sqlx::query_scalar(query)
            .bind(group_id)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(count)
    }
}

#[derive(Debug, Clone)]
pub struct GroupMemberStore {
    pub store: Store,
}

impl GroupMemberStore {
    pub fn new(store: Store) -> Self {
        Self { store }
    }

    pub async fn get_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        group_id: &str,
        uid: i64,
    ) -> Result<Option<GroupMember>, sqlx::Error> {
        let query = r#"
            SELECT id, group_id, uid, role, joined_at
            FROM group_members WHERE group_id = $1 AND uid = $2"#;
        sqlx::query_as::<_, GroupMember>(query)
            .bind(group_id)
            .bind(uid)
            .fetch_optional(&mut **tx)
            .await
    }

    pub async fn insert(&self, member: &GroupMember) -> Result<i64, sqlx::Error> {
        let query = r#"
            INSERT INTO group_members (group_id, uid, role, joined_at)
            VALUES ($1, $2, $3, $4)
            ON CONFLICT (group_id, uid) DO NOTHING
            RETURNING id"#;
        let result = sqlx::query_scalar::<_, i64>(query)
            .bind(&member.group_id)
            .bind(member.uid)
            .bind(&member.role)
            .bind(member.joined_at)
            .fetch_optional(&self.store.pool)
            .await?;
        Ok(result.unwrap_or(0))
    }

    pub async fn delete_by_group_and_user_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        group_id: &str,
        uid: i64,
    ) -> Result<u64, sqlx::Error> {
        let query = r#"DELETE FROM group_members WHERE group_id = $1 AND uid = $2"#;
        let result = sqlx::query(query)
            .bind(group_id)
            .bind(uid)
            .execute(&mut **tx)
            .await?;
        Ok(result.rows_affected())
    }

    pub async fn delete_by_user_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        uid: i64,
    ) -> Result<u64, sqlx::Error> {
        let query = r#"DELETE FROM group_members WHERE uid = $1"#;
        let result = sqlx::query(query).bind(uid).execute(&mut **tx).await?;
        Ok(result.rows_affected())
    }

    pub async fn delete_by_group_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        group_id: &str,
    ) -> Result<u64, sqlx::Error> {
        let query = r#"DELETE FROM group_members WHERE group_id = $1"#;
        let result = sqlx::query(query).bind(group_id).execute(&mut **tx).await?;
        Ok(result.rows_affected())
    }

    pub async fn is_member(&self, group_id: &str, uid: i64) -> Result<bool, sqlx::Error> {
        let query =
            r#"SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id = $1 AND uid = $2)"#;
        let exists: bool = sqlx::query_scalar(query)
            .bind(group_id)
            .bind(uid)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(exists)
    }

    pub async fn is_member_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        group_id: &str,
        uid: i64,
    ) -> Result<bool, sqlx::Error> {
        let query =
            r#"SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id = $1 AND uid = $2)"#;
        let exists: bool = sqlx::query_scalar(query)
            .bind(group_id)
            .bind(uid)
            .fetch_one(&mut **tx)
            .await?;
        Ok(exists)
    }

    pub async fn get_members(&self, group_id: &str) -> Result<Vec<GroupMember>, sqlx::Error> {
        let query = r#"SELECT id, group_id, uid, role, joined_at
                       FROM group_members WHERE group_id = $1
                       ORDER BY joined_at ASC"#;
        sqlx::query_as::<_, GroupMember>(query)
            .bind(group_id)
            .fetch_all(&self.store.pool)
            .await
    }
}

#[derive(Debug, Clone)]
pub struct FriendRequestStore {
    pub store: Store,
}

impl FriendRequestStore {
    pub fn new(store: Store) -> Self {
        Self { store }
    }

    pub async fn get_by_id(&self, id: i64) -> Result<Option<FriendRequest>, sqlx::Error> {
        let query = r#"SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
                       FROM friend_requests WHERE id = $1"#;
        sqlx::query_as::<_, FriendRequest>(query)
            .bind(id)
            .fetch_optional(&self.store.pool)
            .await
    }

    pub async fn get_pending_requests(
        &self,
        to_user_id: i64,
    ) -> Result<Vec<FriendRequest>, sqlx::Error> {
        let query = r#"
            SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
            FROM friend_requests
            WHERE to_user_id = $1 AND status = 'pending'
            ORDER BY created_at DESC"#;
        sqlx::query_as::<_, FriendRequest>(query)
            .bind(to_user_id)
            .fetch_all(&self.store.pool)
            .await
    }

    pub async fn get_sent_requests(
        &self,
        from_user_id: i64,
    ) -> Result<Vec<FriendRequest>, sqlx::Error> {
        let query = r#"
            SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
            FROM friend_requests
            WHERE from_user_id = $1 AND status = 'pending'
            ORDER BY created_at DESC"#;
        sqlx::query_as::<_, FriendRequest>(query)
            .bind(from_user_id)
            .fetch_all(&self.store.pool)
            .await
    }

    pub async fn insert(&self, request: &FriendRequest) -> Result<i64, sqlx::Error> {
        let query = r#"
            INSERT INTO friend_requests (from_user_id, to_user_id, message, status, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5, $6)
            RETURNING id"#;
        let id: i64 = sqlx::query_scalar(query)
            .bind(request.from_user_id)
            .bind(request.to_user_id)
            .bind(&request.message)
            .bind(&request.status)
            .bind(request.created_at)
            .bind(request.updated_at)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(id)
    }

    pub async fn update_status(
        &self,
        id: i64,
        status: &str,
        updated_at: i64,
    ) -> Result<u64, sqlx::Error> {
        let query = r#"UPDATE friend_requests SET status = $1, updated_at = $2 WHERE id = $3"#;
        let result = sqlx::query(query)
            .bind(status)
            .bind(updated_at)
            .bind(id)
            .execute(&self.store.pool)
            .await?;
        Ok(result.rows_affected())
    }

    pub async fn has_pending_requests(&self, user_id: i64) -> Result<bool, sqlx::Error> {
        let query = r#"
            SELECT EXISTS(
                SELECT 1 FROM friend_requests
                WHERE to_user_id = $1 AND status = 'pending'
            )"#;
        let exists: bool = sqlx::query_scalar(query)
            .bind(user_id)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(exists)
    }

    pub async fn are_friends(&self, user_id_1: i64, user_id_2: i64) -> Result<bool, sqlx::Error> {
        let query = r#"
            SELECT EXISTS(
                SELECT 1 FROM friend_requests
                WHERE ((from_user_id = $1 AND to_user_id = $2)
                    OR (from_user_id = $2 AND to_user_id = $1))
                    AND status = 'accepted'
            )"#;
        let exists: bool = sqlx::query_scalar(query)
            .bind(user_id_1)
            .bind(user_id_2)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(exists)
    }

    pub async fn delete_by_user_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        user_id: i64,
    ) -> Result<u64, sqlx::Error> {
        let query = r#"
            DELETE FROM friend_requests
            WHERE from_user_id = $1 OR to_user_id = $1"#;
        let result = sqlx::query(query)
            .bind(user_id)
            .execute(&mut **tx)
            .await
            .map(|r| r.rows_affected())?;
        Ok(result)
    }

    pub async fn get_received_requests(
        &self,
        to_user_id: i64,
    ) -> Result<Vec<FriendRequest>, sqlx::Error> {
        let query = r#"
            SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
            FROM friend_requests
            WHERE to_user_id = $1
            ORDER BY created_at DESC"#;
        sqlx::query_as::<_, FriendRequest>(query)
            .bind(to_user_id)
            .fetch_all(&self.store.pool)
            .await
    }

    pub async fn insert_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        request: &FriendRequest,
    ) -> Result<i64, sqlx::Error> {
        let query = r#"
            INSERT INTO friend_requests (from_user_id, to_user_id, message, status, created_at, updated_at)
            VALUES ($1, $2, $3, $4, $5, $6)
            RETURNING id"#;
        let id: i64 = sqlx::query_scalar(query)
            .bind(request.from_user_id)
            .bind(request.to_user_id)
            .bind(&request.message)
            .bind(&request.status)
            .bind(request.created_at)
            .bind(request.updated_at)
            .fetch_one(&mut **tx)
            .await?;
        Ok(id)
    }

    pub async fn accept_pending_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        to_user_id: i64,
        from_user_id: i64,
    ) -> Result<i64, sqlx::Error> {
        let query = r#"
            UPDATE friend_requests
            SET status = 'accepted', updated_at = $1
            WHERE from_user_id = $2 AND to_user_id = $3 AND status = 'pending'
            RETURNING id"#;
        let now = chrono::Utc::now().timestamp_millis();
        let id: Option<i64> = sqlx::query_scalar(query)
            .bind(now)
            .bind(to_user_id)
            .bind(from_user_id)
            .fetch_optional(&mut **tx)
            .await?;
        Ok(id.unwrap_or(0))
    }

    pub async fn accept_by_id_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        request_id: i64,
        user_id: i64,
    ) -> Result<i64, sqlx::Error> {
        let query = r#"
            SELECT from_user_id, to_user_id, status FROM friend_requests WHERE id = $1"#;
        let row: Option<(i64, i64, String)> = sqlx::query_as(query)
            .bind(request_id)
            .fetch_optional(&mut **tx)
            .await?;

        match row {
            None => Ok(0),
            Some((from_uid, to_uid, status)) => {
                if status != "pending" {
                    return Ok(-1);
                }
                if to_uid != user_id {
                    return Ok(-1);
                }
                let now = chrono::Utc::now().timestamp_millis();
                let update_query = r#"
                    UPDATE friend_requests
                    SET status = 'accepted', updated_at = $1
                    WHERE id = $2"#;
                sqlx::query(update_query)
                    .bind(now)
                    .bind(request_id)
                    .execute(&mut **tx)
                    .await?;
                Ok(from_uid)
            }
        }
    }

    pub async fn reject_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        request_id: i64,
        user_id: i64,
    ) -> Result<bool, sqlx::Error> {
        let query = r#"
            UPDATE friend_requests
            SET status = 'rejected', updated_at = $1
            WHERE id = $2 AND to_user_id = $3 AND status = 'pending'"#;
        let now = chrono::Utc::now().timestamp_millis();
        let result = sqlx::query(query)
            .bind(now)
            .bind(request_id)
            .bind(user_id)
            .execute(&mut **tx)
            .await?;
        Ok(result.rows_affected() > 0)
    }

    pub async fn cancel_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        request_id: i64,
        from_user_id: i64,
    ) -> Result<bool, sqlx::Error> {
        let query = r#"
            DELETE FROM friend_requests
            WHERE id = $1 AND from_user_id = $2 AND status = 'pending'"#;
        let result = sqlx::query(query)
            .bind(request_id)
            .bind(from_user_id)
            .execute(&mut **tx)
            .await?;
        Ok(result.rows_affected() > 0)
    }
}

#[derive(Debug, Clone)]
pub struct ConversationParticipantStore {
    pub store: Store,
}

impl ConversationParticipantStore {
    pub fn new(store: Store) -> Self {
        Self { store }
    }

    pub async fn insert(&self, participant: &ConversationParticipant) -> Result<u64, sqlx::Error> {
        let query = r#"
            INSERT INTO conversation_participants (conv_id, user_id, join_ts, is_blocked)
            VALUES ($1, $2, $3, $4)
            ON CONFLICT (conv_id, user_id) DO NOTHING"#;
        let result = sqlx::query(query)
            .bind(&participant.conv_id)
            .bind(participant.user_id)
            .bind(participant.join_ts)
            .bind(participant.is_blocked)
            .execute(&self.store.pool)
            .await?;
        Ok(result.rows_affected())
    }

    pub async fn update_blocked(
        &self,
        conv_id: &str,
        user_id: i64,
        blocked: bool,
    ) -> Result<(), sqlx::Error> {
        let blocked_int = if blocked { 1 } else { 0 };
        let query = r#"
            UPDATE conversation_participants 
            SET is_blocked = $1
            WHERE conv_id = $2 AND user_id = $3"#;
        sqlx::query(query)
            .bind(blocked_int)
            .bind(conv_id)
            .bind(user_id)
            .execute(&self.store.pool)
            .await?;
        Ok(())
    }

    pub async fn is_any_blocked(&self, conv_id: &str) -> Result<bool, sqlx::Error> {
        let query = r#"
            SELECT EXISTS(
                SELECT 1 FROM conversation_participants 
                WHERE conv_id = $1 AND is_blocked = 1
            )"#;
        let exists: bool = sqlx::query_scalar(query)
            .bind(conv_id)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(exists)
    }

    pub async fn is_blocked(&self, conv_id: &str, user_id: i64) -> Result<bool, sqlx::Error> {
        let query = r#"
            SELECT is_blocked 
            FROM conversation_participants 
            WHERE conv_id = $1 AND user_id = $2"#;
        let is_blocked: Option<i32> = sqlx::query_scalar(query)
            .bind(conv_id)
            .bind(user_id)
            .fetch_optional(&self.store.pool)
            .await?;
        Ok(is_blocked.map(|v| v == 1).unwrap_or(false))
    }

    pub async fn get_participants(&self, conv_id: &str) -> Result<Vec<i64>, sqlx::Error> {
        let query = r#"
            SELECT user_id 
            FROM conversation_participants 
            WHERE conv_id = $1"#;
        let rows = sqlx::query_scalar::<_, i64>(query)
            .bind(conv_id)
            .fetch_all(&self.store.pool)
            .await?;
        Ok(rows)
    }

    pub async fn get_conversation_list(
        &self,
        user_id: i64,
        limit: i64,
        offset: i64,
    ) -> Result<Vec<ConversationListItem>, sqlx::Error> {
        let query = r#"
            WITH user_conversations AS (
                SELECT cp.conv_id
                FROM conversation_participants cp
                WHERE cp.user_id = $1
            ),
            last_messages AS (
                SELECT DISTINCT ON (m.conv_id)
                    m.conv_id,
                    m.msg_id,
                    m.from_uid,
                    m.content,
                    m.ts,
                    m.is_recalled
                FROM messages m
                WHERE m.conv_id IN (SELECT conv_id FROM user_conversations)
                ORDER BY m.conv_id, m.ts DESC, m.msg_id DESC
            )
            SELECT 
                uc.conv_id,
                CASE 
                    WHEN uc.conv_id LIKE 'p_%' THEN 'private'
                    WHEN uc.conv_id LIKE 'g_%' THEN 'group'
                END as type,
                CASE 
                    WHEN uc.conv_id LIKE 'p_%' THEN (
                        SELECT p.user_id 
                        FROM conversation_participants p 
                        WHERE p.conv_id = uc.conv_id AND p.user_id != $1
                        LIMIT 1
                    )
                    ELSE NULL
                END as target_uid,
                CASE 
                    WHEN uc.conv_id LIKE 'g_%' THEN (
                        SELECT g.group_id 
                        FROM groups g 
                        WHERE g.group_id = SUBSTRING(uc.conv_id FROM 2)
                    )
                    ELSE NULL
                END as group_id,
                lm.msg_id as last_msg_id,
                lm.from_uid,
                lm.content,
                lm.ts as last_msg_ts,
                lm.is_recalled
            FROM user_conversations uc
            LEFT JOIN last_messages lm ON uc.conv_id = lm.conv_id
            ORDER BY lm.ts DESC NULLS LAST
            LIMIT $2 OFFSET $3"#;

        sqlx::query_as::<_, ConversationListItem>(query)
            .bind(user_id)
            .bind(limit)
            .bind(offset)
            .fetch_all(&self.store.pool)
            .await
    }

    pub async fn exists(&self, conv_id: &str, user_id: i64) -> Result<bool, sqlx::Error> {
        let query = r#"
            SELECT EXISTS(
                SELECT 1 FROM conversation_participants 
                WHERE conv_id = $1 AND user_id = $2
            )"#;
        let exists: bool = sqlx::query_scalar(query)
            .bind(conv_id)
            .bind(user_id)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(exists)
    }

    pub async fn exists_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        conv_id: &str,
        user_id: i64,
    ) -> Result<bool, sqlx::Error> {
        let query = r#"
            SELECT EXISTS(
                SELECT 1 FROM conversation_participants 
                WHERE conv_id = $1 AND user_id = $2
            )"#;
        let exists: bool = sqlx::query_scalar(query)
            .bind(conv_id)
            .bind(user_id)
            .fetch_one(&mut **tx)
            .await?;
        Ok(exists)
    }

    pub async fn set_blocked(
        &self,
        conv_id: &str,
        user_id: i64,
        blocked: bool,
    ) -> Result<(), sqlx::Error> {
        self.update_blocked(conv_id, user_id, blocked).await
    }

    pub async fn insert_batch_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        participants: &[ConversationParticipant],
    ) -> Result<u64, sqlx::Error> {
        let mut total = 0;
        for p in participants {
            let query = r#"
                INSERT INTO conversation_participants (conv_id, user_id, join_ts, is_blocked)
                VALUES ($1, $2, $3, $4)
                ON CONFLICT (conv_id, user_id) DO NOTHING"#;
            let result = sqlx::query(query)
                .bind(&p.conv_id)
                .bind(p.user_id)
                .bind(p.join_ts)
                .bind(p.is_blocked)
                .execute(&mut **tx)
                .await?;
            total += result.rows_affected();
        }
        Ok(total)
    }

    pub async fn delete_by_user_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        user_id: i64,
    ) -> Result<u64, sqlx::Error> {
        let query = r#"
            DELETE FROM conversation_participants
            WHERE user_id = $1"#;
        sqlx::query(query)
            .bind(user_id)
            .execute(&mut **tx)
            .await
            .map(|r| r.rows_affected())
    }

    pub async fn count_user_conversations(&self, user_id: i64) -> Result<i64, sqlx::Error> {
        let query = r#"
            SELECT COUNT(*) FROM conversation_participants
            WHERE user_id = $1"#;
        let count: i64 = sqlx::query_scalar(query)
            .bind(user_id)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(count)
    }
}

#[derive(Debug, Clone)]
pub struct IdGeneratorStateStore {
    pub store: Store,
}

impl IdGeneratorStateStore {
    pub fn new(store: Store) -> Self {
        Self { store }
    }

    pub async fn get_first(&self) -> Result<Option<IdGeneratorState>, sqlx::Error> {
        let query = r#"SELECT id, last_ts, epoch_time FROM id_generator_state ORDER BY id LIMIT 1"#;
        sqlx::query_as::<_, IdGeneratorState>(query)
            .fetch_optional(&self.store.pool)
            .await
    }

    pub async fn get_first_for_update(
        &self,
        tx: &mut Transaction<'_, Postgres>,
    ) -> Result<Option<IdGeneratorState>, sqlx::Error> {
        let query = r#"SELECT id, last_ts, epoch_time FROM id_generator_state ORDER BY id LIMIT 1 FOR UPDATE"#;
        sqlx::query_as::<_, IdGeneratorState>(query)
            .fetch_optional(&mut **tx)
            .await
    }

    pub async fn insert(&self, state: &IdGeneratorState) -> Result<i64, sqlx::Error> {
        let query = r#"
            INSERT INTO id_generator_state (last_ts, epoch_time)
            VALUES ($1, $2)
            RETURNING id"#;
        let id: i64 = sqlx::query_scalar(query)
            .bind(state.last_ts)
            .bind(state.epoch_time)
            .fetch_one(&self.store.pool)
            .await?;
        Ok(id)
    }

    pub async fn insert_with_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        state: &IdGeneratorState,
    ) -> Result<i64, sqlx::Error> {
        let query = r#"
            INSERT INTO id_generator_state (last_ts, epoch_time)
            VALUES ($1, $2)
            RETURNING id"#;
        let id: i64 = sqlx::query_scalar(query)
            .bind(state.last_ts)
            .bind(state.epoch_time)
            .fetch_one(&mut **tx)
            .await?;
        Ok(id)
    }

    pub async fn update_with_tx(
        &self,
        tx: &mut Transaction<'_, Postgres>,
        state: &IdGeneratorState,
    ) -> Result<u64, sqlx::Error> {
        let query = r#"
            UPDATE id_generator_state SET last_ts = $1, epoch_time = $2
            WHERE id = $3"#;
        let result = sqlx::query(query)
            .bind(state.last_ts)
            .bind(state.epoch_time)
            .bind(state.id)
            .execute(&mut **tx)
            .await?;
        Ok(result.rows_affected())
    }
}
