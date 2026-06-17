use sqlx::PgPool;

pub async fn run_migrations(pool: &PgPool) -> Result<(), sqlx::Error> {
    create_users_table(pool).await?;
    create_groups_table(pool).await?;
    create_group_members_table(pool).await?;
    create_friend_requests_table(pool).await?;
    create_conversation_participants_table(pool).await?;
    create_id_generator_state_table(pool).await?;
    create_message_idempotency_table(pool).await?;
    create_messages_parent_table(pool).await?;
    create_jwt_blacklist_table(pool).await?;
    add_is_blocked_column(pool).await?;
    add_epoch_time_column(pool).await?;
    drop_email_unique_constraint(pool).await?;

    tracing::info!("migrations completed successfully");
    Ok(())
}

async fn create_users_table(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        CREATE TABLE IF NOT EXISTS users (
            id BIGSERIAL PRIMARY KEY,
            username TEXT NOT NULL,
            pwd_hash TEXT NOT NULL,
            email TEXT,
            public_id TEXT NOT NULL UNIQUE,
            created_at BIGINT NOT NULL,
            updated_at BIGINT NOT NULL
        )
        "#,
    )
    .execute(pool)
    .await?;

    sqlx::query(r#"CREATE INDEX IF NOT EXISTS idx_users_username ON users (username)"#)
        .execute(pool)
        .await?;

    sqlx::query(r#"CREATE INDEX IF NOT EXISTS idx_users_public_id ON users (public_id)"#)
        .execute(pool)
        .await?;

    Ok(())
}

async fn create_groups_table(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        CREATE TABLE IF NOT EXISTS groups (
            group_id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            description TEXT,
            owner_id BIGINT NOT NULL,
            created_at BIGINT NOT NULL,
            updated_at BIGINT NOT NULL,
            is_deleted INTEGER NOT NULL DEFAULT 0
        )
        "#,
    )
    .execute(pool)
    .await?;

    sqlx::query(r#"CREATE INDEX IF NOT EXISTS idx_groups_owner_id ON groups (owner_id)"#)
        .execute(pool)
        .await?;

    Ok(())
}

async fn create_group_members_table(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        CREATE TABLE IF NOT EXISTS group_members (
            id BIGSERIAL PRIMARY KEY,
            group_id TEXT NOT NULL,
            uid BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            role TEXT NOT NULL,
            joined_at BIGINT NOT NULL,
            UNIQUE (group_id, uid)
        )
        "#,
    )
    .execute(pool)
    .await?;

    sqlx::query(r#"CREATE INDEX IF NOT EXISTS idx_group_members_uid ON group_members (uid)"#)
        .execute(pool)
        .await?;

    Ok(())
}

async fn create_friend_requests_table(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        CREATE TABLE IF NOT EXISTS friend_requests (
            id BIGSERIAL PRIMARY KEY,
            from_user_id BIGINT NOT NULL,
            to_user_id BIGINT NOT NULL,
            message TEXT,
            status TEXT NOT NULL,
            created_at BIGINT NOT NULL,
            updated_at BIGINT NOT NULL
        )
        "#,
    )
    .execute(pool)
    .await?;

    sqlx::query(
        r#"CREATE UNIQUE INDEX IF NOT EXISTS idx_friend_requests_pending_unique ON friend_requests (from_user_id, to_user_id) WHERE status = 'pending'"#,
    )
    .execute(pool)
    .await?;

    sqlx::query(
        r#"CREATE INDEX IF NOT EXISTS idx_friend_requests_from_user_id ON friend_requests (from_user_id)"#,
    )
    .execute(pool)
    .await?;

    sqlx::query(
        r#"CREATE INDEX IF NOT EXISTS idx_friend_requests_to_user_id ON friend_requests (to_user_id)"#,
    )
    .execute(pool)
    .await?;

    Ok(())
}

async fn create_conversation_participants_table(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        CREATE TABLE IF NOT EXISTS conversation_participants (
            conv_id TEXT NOT NULL,
            user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
            join_ts BIGINT NOT NULL,
            is_blocked INTEGER NOT NULL DEFAULT 0,
            PRIMARY KEY (conv_id, user_id)
        )
        "#,
    )
    .execute(pool)
    .await?;

    sqlx::query(
        r#"CREATE INDEX IF NOT EXISTS idx_conversation_participants_user_id ON conversation_participants (user_id)"#,
    )
    .execute(pool)
    .await?;

    Ok(())
}

async fn create_id_generator_state_table(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        CREATE TABLE IF NOT EXISTS id_generator_state (
            id BIGSERIAL PRIMARY KEY,
            last_ts BIGINT NOT NULL
        )
        "#,
    )
    .execute(pool)
    .await?;

    Ok(())
}

async fn create_message_idempotency_table(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        CREATE TABLE IF NOT EXISTS message_idempotency (
            id BIGSERIAL PRIMARY KEY,
            user_id BIGINT NOT NULL,
            client_msg_id TEXT NOT NULL,
            msg_id BIGINT NOT NULL DEFAULT 0,
            created_at BIGINT NOT NULL,
            UNIQUE (user_id, client_msg_id)
        )
        "#,
    )
    .execute(pool)
    .await?;

    sqlx::query(
        r#"CREATE INDEX IF NOT EXISTS idx_message_idempotency_user_id ON message_idempotency (user_id)"#,
    )
    .execute(pool)
    .await?;

    sqlx::query(
        r#"CREATE INDEX IF NOT EXISTS idx_message_idempotency_created_at ON message_idempotency (created_at)"#,
    )
    .execute(pool)
    .await?;

    Ok(())
}

async fn create_messages_parent_table(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        CREATE TABLE IF NOT EXISTS messages (
            msg_id BIGINT NOT NULL,
            conv_id TEXT NOT NULL,
            from_uid BIGINT NOT NULL,
            content TEXT NOT NULL,
            ts BIGINT NOT NULL,
            is_recalled INTEGER NOT NULL DEFAULT 0,
            PRIMARY KEY (msg_id, ts)
        ) PARTITION BY RANGE (ts)
        "#,
    )
    .execute(pool)
    .await?;

    Ok(())
}

async fn create_jwt_blacklist_table(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        CREATE TABLE IF NOT EXISTS jwt_blacklist (
            jti TEXT PRIMARY KEY,
            expires_at BIGINT NOT NULL
        )
        "#,
    )
    .execute(pool)
    .await?;

    sqlx::query(
        r#"CREATE INDEX IF NOT EXISTS idx_jwt_blacklist_expires_at ON jwt_blacklist (expires_at)"#,
    )
    .execute(pool)
    .await?;

    Ok(())
}

async fn add_is_blocked_column(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        ALTER TABLE conversation_participants
        ADD COLUMN IF NOT EXISTS is_blocked INTEGER NOT NULL DEFAULT 0
        "#,
    )
    .execute(pool)
    .await?;

    Ok(())
}

async fn add_epoch_time_column(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(
        r#"
        ALTER TABLE id_generator_state
        ADD COLUMN IF NOT EXISTS epoch_time BIGINT NOT NULL DEFAULT 0
        "#,
    )
    .execute(pool)
    .await?;

    Ok(())
}

async fn drop_email_unique_constraint(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query(r#"ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key"#)
        .execute(pool)
        .await?;
    Ok(())
}
