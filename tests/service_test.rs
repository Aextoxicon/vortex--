mod test_utils;

use test_utils::{TestFixture, unique_username};
use vortex__::{config, idgen, shared, store};

async fn setup_test_service() -> (TestFixture, shared::Service) {
    let fixture = TestFixture::new().await;

    let cfg = config::Config {
        node_id: 1,
        jwt_secret: "test-secret-key-min-32-chars-long!!".to_string(),
        port: ":0".to_string(),
        database_url: "postgres://postgres:postgres@localhost:5432/test".to_string(),
        db_max_open_conns: 10,
        db_max_idle_conns: 5,
        jwt_issuer: "test-issuer".to_string(),
        jwt_expires_minutes: 60,
        bcrypt_cost: 4,
        message_recall_window_ms: 120_000,
        epoch_time: 1700000000000,
        segment_duration_ms: 3600000,
        segment_size: 1000,
        message_retention_days: 7,
        default_page_size: 20,
        max_page_size: 100,
        public_id_length: 12,
        group_id_random_length: 8,
        worker_table_create_interval_hours: 24,
        worker_maintenance_initial_delay_minutes: 1,
        worker_maintenance_interval_hours: 24,
        idempotency_retention_hours: 24,
        s3_url: String::new(),
    };

    let epoch_time = 1609459200000i64;
    let store = store::Store::new(fixture.pool.clone(), epoch_time);

    let user_store = store::UserStore::new(store.clone());
    let msg_store = store::MessageStore::new(store.clone());
    let group_store = store::GroupStore::new(store.clone());
    let group_mem_store = store::GroupMemberStore::new(store.clone());
    let friend_store = store::FriendRequestStore::new(store.clone());
    let conv_part_store = store::ConversationParticipantStore::new(store.clone());
    let id_gen_state_store = store::IdGeneratorStateStore::new(store.clone());
    let idempotency_store = store::MessageIdempotencyStore::new(store.clone());

    let id_gen = idgen::IdGenerator::new(
        fixture.pool.clone(),
        id_gen_state_store.clone(),
        msg_store.clone(),
        1,
        cfg.epoch_time,
    );
    id_gen.init().await;

    let user_cache = shared::UserCache::builder()
        .max_capacity(5000)
        .time_to_live(std::time::Duration::from_secs(300))
        .time_to_idle(std::time::Duration::from_secs(60))
        .build();

    let svc = shared::Service::new(
        cfg.clone(),
        fixture.pool.clone(),
        user_store,
        msg_store,
        group_store,
        group_mem_store,
        friend_store,
        conv_part_store,
        idempotency_store,
        id_gen,
        None,
        user_cache,
    );

    (fixture, svc)
}

#[tokio::test]
async fn test_create_user() {
    let (_fixture, svc) = setup_test_service().await;

    let username = unique_username();
    let password = "Test1234!".to_string();
    let email = format!("{}@example.com", username);

    let user = svc
        .create_user(username.clone(), password, email.clone())
        .await
        .unwrap();

    assert!(user.id > 0);
    assert_eq!(user.username, username);
    assert_eq!(user.email, email);
    assert!(!user.public_id.is_empty());
}

#[tokio::test]
async fn test_create_user_duplicate_username() {
    let (_fixture, svc) = setup_test_service().await;

    let username = unique_username();
    let password = "Test1234!".to_string();
    let email = format!("{}@example.com", username);

    svc.create_user(username.clone(), password.clone(), email)
        .await
        .unwrap();

    let result = svc
        .create_user(username, password, "different@example.com".to_string())
        .await;
    assert!(result.is_err());
}

#[tokio::test]
async fn test_get_user_by_username() {
    let (_fixture, svc) = setup_test_service().await;

    let username = unique_username();
    let password = "Test1234!".to_string();
    let email = format!("{}@example.com", username);

    svc.create_user(username.clone(), password, email)
        .await
        .unwrap();

    let user = svc.get_user_by_username(&username).await.unwrap();
    assert_eq!(user.username, username);
}

#[tokio::test]
async fn test_get_user_by_public_id() {
    let (_fixture, svc) = setup_test_service().await;

    let username = unique_username();
    let password = "Test1234!".to_string();
    let email = format!("{}@example.com", username);

    let created_user = svc
        .create_user(username.clone(), password, email)
        .await
        .unwrap();

    let user = svc
        .get_user_by_public_id(&created_user.public_id)
        .await
        .unwrap();
    assert_eq!(user.public_id, created_user.public_id);
}

#[tokio::test]
async fn test_validate_credentials() {
    let (_fixture, svc) = setup_test_service().await;

    let username = unique_username();
    let password = "Test1234!".to_string();
    let email = format!("{}@example.com", username);

    let user = svc
        .create_user(username.clone(), password.clone(), email)
        .await
        .unwrap();

    assert!(svc.validate_credentials(&user, &password).await.is_ok());
    assert!(svc.validate_credentials(&user, "wrongpassword").await.is_err());
}

#[tokio::test]
async fn test_create_group() {
    let (_fixture, svc) = setup_test_service().await;

    let username = unique_username();
    let user = svc
        .create_user(
            username.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", username),
        )
        .await
        .unwrap();

    let group_id = svc
        .create_group("Test Group", "A test group", user.id)
        .await
        .unwrap();
    assert!(!group_id.is_empty());
}

#[tokio::test]
async fn test_get_group() {
    let (_fixture, svc) = setup_test_service().await;

    let username = unique_username();
    let user = svc
        .create_user(
            username.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", username),
        )
        .await
        .unwrap();

    let group_id = svc
        .create_group("Test Group", "A test group", user.id)
        .await
        .unwrap();

    let group = svc.get_group_by_id(&group_id).await.unwrap();
    assert_eq!(group.name, "Test Group");
}

#[tokio::test]
async fn test_add_member() {
    let (_fixture, svc) = setup_test_service().await;

    let owner_name = unique_username();
    let owner = svc
        .create_user(
            owner_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", owner_name),
        )
        .await
        .unwrap();

    let member_name = unique_username();
    let member = svc
        .create_user(
            member_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", member_name),
        )
        .await
        .unwrap();

    let group_id = svc.create_group("Join Group", "", owner.id).await.unwrap();

    svc.add_member(&group_id, member.id, "member")
        .await
        .unwrap();

    let is_member = svc.is_user_in_group(&group_id, member.id).await.unwrap();
    assert!(is_member);
}

#[tokio::test]
async fn test_remove_member() {
    let (_fixture, svc) = setup_test_service().await;

    let owner_name = unique_username();
    let owner = svc
        .create_user(
            owner_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", owner_name),
        )
        .await
        .unwrap();

    let member_name = unique_username();
    let member = svc
        .create_user(
            member_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", member_name),
        )
        .await
        .unwrap();

    let group_id = svc.create_group("Leave Group", "", owner.id).await.unwrap();

    svc.add_member(&group_id, member.id, "member")
        .await
        .unwrap();
    svc.remove_member(&group_id, member.id).await.unwrap();

    let is_member = svc.is_user_in_group(&group_id, member.id).await.unwrap();
    assert!(!is_member);
}

#[tokio::test]
async fn test_send_friend_request() {
    let (_fixture, svc) = setup_test_service().await;

    let user1_name = unique_username();
    let user1 = svc
        .create_user(
            user1_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", user1_name),
        )
        .await
        .unwrap();

    let user2_name = unique_username();
    let user2 = svc
        .create_user(
            user2_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", user2_name),
        )
        .await
        .unwrap();

    let (request_id, _auto_accepted) = svc
        .send_friend_request(user1.id, user2.id, "".to_string())
        .await
        .unwrap();
    assert!(request_id > 0);
}

#[tokio::test]
async fn test_accept_friend_request() {
    let (_fixture, svc) = setup_test_service().await;

    let user1_name = unique_username();
    let user1 = svc
        .create_user(
            user1_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", user1_name),
        )
        .await
        .unwrap();

    let user2_name = unique_username();
    let user2 = svc
        .create_user(
            user2_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", user2_name),
        )
        .await
        .unwrap();

    let (request_id, _) = svc
        .send_friend_request(user1.id, user2.id, "".to_string())
        .await
        .unwrap();

    svc.accept_friend_request(request_id, user2.id)
        .await
        .unwrap();
}

#[tokio::test]
async fn test_get_sent_friend_requests() {
    let (_fixture, svc) = setup_test_service().await;

    let user1_name = unique_username();
    let user1 = svc
        .create_user(
            user1_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", user1_name),
        )
        .await
        .unwrap();

    let user2_name = unique_username();
    let user2 = svc
        .create_user(
            user2_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", user2_name),
        )
        .await
        .unwrap();

    svc.send_friend_request(user1.id, user2.id, "".to_string())
        .await
        .unwrap();

    let requests = svc.get_sent_requests(user1.id).await.unwrap();
    assert!(!requests.is_empty());
}

#[tokio::test]
async fn test_get_received_friend_requests() {
    let (_fixture, svc) = setup_test_service().await;

    let user1_name = unique_username();
    let user1 = svc
        .create_user(
            user1_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", user1_name),
        )
        .await
        .unwrap();

    let user2_name = unique_username();
    let user2 = svc
        .create_user(
            user2_name.clone(),
            "Test1234!".to_string(),
            format!("{}@example.com", user2_name),
        )
        .await
        .unwrap();

    svc.send_friend_request(user1.id, user2.id, "".to_string())
        .await
        .unwrap();

    let requests = svc.get_received_requests(user2.id).await.unwrap();
    assert!(!requests.is_empty());
}
