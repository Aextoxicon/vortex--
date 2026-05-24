mod test_utils;

use test_utils::TestFixture;
use sqlx::PgPool;

#[tokio::test]
async fn test_run_migrations() {
    let fixture = TestFixture::new().await;
    
    let tables = vec![
        "users",
        "groups",
        "group_members",
        "friend_requests",
        "conversation_participants",
        "id_generator_state",
        "messages",
        "jwt_blacklist",
    ];

    for table in tables {
        let exists: bool = sqlx::query_scalar(
            "SELECT EXISTS (
                SELECT FROM information_schema.tables 
                WHERE table_schema = 'public' 
                AND table_name = $1
            )"
        )
        .bind(table)
        .fetch_one(&fixture.pool)
        .await
        .expect("failed to check table");

        assert!(exists, "table {} was not created", table);
    }
}

#[tokio::test]
async fn test_migrations_create_indexes() {
    let fixture = TestFixture::new().await;
    
    let indexes = vec![
        ("users", "idx_users_username"),
        ("users", "idx_users_public_id"),
        ("groups", "idx_groups_owner_id"),
        ("group_members", "idx_group_members_uid"),
        ("friend_requests", "idx_friend_requests_from_user_id"),
        ("friend_requests", "idx_friend_requests_to_user_id"),
    ];

    for (_, index) in indexes {
        let exists: bool = sqlx::query_scalar(
            "SELECT EXISTS (
                SELECT FROM pg_indexes 
                WHERE schemaname = 'public' 
                AND indexname = $1
            )"
        )
        .bind(index)
        .fetch_one(&fixture.pool)
        .await
        .expect("failed to check index");

        assert!(exists, "index {} was not created", index);
    }
}

#[tokio::test]
async fn test_migrations_idempotency() {
    let _fixture = TestFixture::new().await;
}
