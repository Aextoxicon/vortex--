mod test_utils;

use test_utils::{TestFixture, unique_username};
use vortex__::store;

async fn setup_test_store() -> (TestFixture, store::Store) {
    let fixture = TestFixture::new().await;

    let epoch_time = 1609459200000i64;
    let store = store::Store::new(fixture.pool.clone(), epoch_time);

    (fixture, store)
}

async fn create_test_user(user_store: &store::UserStore) -> store::User {
    let now = chrono::Utc::now().timestamp_millis();
    let user = store::User {
        id: 0,
        public_id: format!("pub_{}", unique_username()),
        username: unique_username(),
        pwd_hash: "hashed_password".to_string(),
        email: "test@example.com".to_string(),
        created_at: now,
        updated_at: now,
    };
    let id = user_store.insert(&user).await.unwrap();
    store::User { id, ..user }
}

#[tokio::test]
async fn test_user_store_insert() {
    let (_fixture, store) = setup_test_store().await;

    let user_store = store::UserStore::new(store.clone());

    let now = chrono::Utc::now().timestamp_millis();
    let user = store::User {
        id: 0,
        public_id: "test123".to_string(),
        username: unique_username(),
        pwd_hash: "hashed_password".to_string(),
        email: "test@example.com".to_string(),
        created_at: now,
        updated_at: now,
    };

    let inserted_id = user_store.insert(&user).await.unwrap();

    let found = user_store.get_by_id(inserted_id).await.unwrap().unwrap();
    assert_eq!(found.username, user.username);
}

#[tokio::test]
async fn test_user_store_get_by_username() {
    let (_fixture, store) = setup_test_store().await;

    let user_store = store::UserStore::new(store.clone());

    let username = unique_username();
    let now = chrono::Utc::now().timestamp_millis();
    let user = store::User {
        id: 0,
        public_id: "test123".to_string(),
        username: username.clone(),
        pwd_hash: "hashed_password".to_string(),
        email: "test@example.com".to_string(),
        created_at: now,
        updated_at: now,
    };

    user_store.insert(&user).await.unwrap();

    let found = user_store
        .get_by_username(&username)
        .await
        .unwrap()
        .unwrap();
    assert_eq!(found.username, username);
}

#[tokio::test]
async fn test_user_store_get_by_public_id() {
    let (_fixture, store) = setup_test_store().await;

    let user_store = store::UserStore::new(store.clone());

    let public_id = "test_public_id_123".to_string();
    let now = chrono::Utc::now().timestamp_millis();
    let user = store::User {
        id: 0,
        public_id: public_id.clone(),
        username: unique_username(),
        pwd_hash: "hashed_password".to_string(),
        email: "test@example.com".to_string(),
        created_at: now,
        updated_at: now,
    };

    user_store.insert(&user).await.unwrap();

    let found = user_store
        .get_by_public_id(&public_id)
        .await
        .unwrap()
        .unwrap();
    assert_eq!(found.public_id, public_id);
}

#[tokio::test]
async fn test_message_store_ensure_partition() {
    let (_fixture, store) = setup_test_store().await;

    let msg_store = store::MessageStore::new(store.clone());

    let table_name = format!("messages_{}", chrono::Utc::now().format("%Y%m%d"));

    let result = msg_store.ensure_partition(&table_name).await;
    assert!(result.is_ok());
}

#[tokio::test]
async fn test_group_store_insert() {
    let (_fixture, store) = setup_test_store().await;

    let group_store = store::GroupStore::new(store.clone());

    let now = chrono::Utc::now().timestamp_millis();
    let group = store::Group {
        group_id: "group123".to_string(),
        name: "Test Group".to_string(),
        description: "A test group".to_string(),
        owner_id: 1,
        created_at: now,
        updated_at: now,
        is_deleted: 0,
    };

    group_store.insert(&group).await.unwrap();

    let found = group_store.get_by_id("group123").await.unwrap().unwrap();
    assert_eq!(found.name, "Test Group");
}

#[tokio::test]
async fn test_group_member_store_insert() {
    let (_fixture, store) = setup_test_store().await;

    let user_store = store::UserStore::new(store.clone());
    let user = create_test_user(&user_store).await;

    let group_mem_store = store::GroupMemberStore::new(store.clone());

    let now = chrono::Utc::now().timestamp_millis();
    let member = store::GroupMember {
        id: 0,
        group_id: "group123".to_string(),
        uid: user.id,
        role: "member".to_string(),
        joined_at: now,
    };

    group_mem_store.insert(&member).await.unwrap();

    let is_member = group_mem_store
        .is_member("group123", user.id)
        .await
        .unwrap();
    assert!(is_member);
}

#[tokio::test]
async fn test_group_member_store_remove() {
    let (_fixture, store) = setup_test_store().await;

    let user_store = store::UserStore::new(store.clone());
    let user = create_test_user(&user_store).await;

    let group_mem_store = store::GroupMemberStore::new(store.clone());

    let now = chrono::Utc::now().timestamp_millis();
    let member = store::GroupMember {
        id: 0,
        group_id: "group123".to_string(),
        uid: user.id,
        role: "member".to_string(),
        joined_at: now,
    };

    group_mem_store.insert(&member).await.unwrap();
    group_mem_store
        .delete_by_group_and_user("group123", user.id)
        .await
        .unwrap();

    let is_member = group_mem_store
        .is_member("group123", user.id)
        .await
        .unwrap();
    assert!(!is_member);
}

#[tokio::test]
async fn test_friend_request_store_insert() {
    let (_fixture, store) = setup_test_store().await;

    let friend_store = store::FriendRequestStore::new(store.clone());

    let now = chrono::Utc::now().timestamp_millis();
    let request = store::FriendRequest {
        id: 0,
        from_user_id: 1,
        to_user_id: 2,
        message: "Hello!".to_string(),
        status: "pending".to_string(),
        created_at: now,
        updated_at: now,
    };

    friend_store.insert(&request).await.unwrap();

    let found = friend_store.get_by_id(1).await.unwrap().unwrap();
    assert_eq!(found.from_user_id, 1);
    assert_eq!(found.to_user_id, 2);
}

#[tokio::test]
async fn test_friend_request_store_get_sent_requests() {
    let (_fixture, store) = setup_test_store().await;

    let friend_store = store::FriendRequestStore::new(store.clone());

    let now = chrono::Utc::now().timestamp_millis();
    let request = store::FriendRequest {
        id: 0,
        from_user_id: 1,
        to_user_id: 2,
        message: "".to_string(),
        status: "pending".to_string(),
        created_at: now,
        updated_at: now,
    };

    friend_store.insert(&request).await.unwrap();

    let requests = friend_store.get_sent_requests(1).await.unwrap();
    assert!(!requests.is_empty());
    assert_eq!(requests[0].to_user_id, 2);
}

#[tokio::test]
async fn test_friend_request_store_get_received_requests() {
    let (_fixture, store) = setup_test_store().await;

    let friend_store = store::FriendRequestStore::new(store.clone());

    let now = chrono::Utc::now().timestamp_millis();
    let request = store::FriendRequest {
        id: 0,
        from_user_id: 1,
        to_user_id: 2,
        message: "".to_string(),
        status: "pending".to_string(),
        created_at: now,
        updated_at: now,
    };

    friend_store.insert(&request).await.unwrap();

    let requests = friend_store.get_received_requests(2).await.unwrap();
    assert!(!requests.is_empty());
    assert_eq!(requests[0].from_user_id, 1);
}

#[tokio::test]
async fn test_conversation_participant_store() {
    let (_fixture, store) = setup_test_store().await;

    let user_store = store::UserStore::new(store.clone());
    let user1 = create_test_user(&user_store).await;
    let user2 = create_test_user(&user_store).await;

    let conv_part_store = store::ConversationParticipantStore::new(store.clone());

    let now = chrono::Utc::now().timestamp_millis();
    let p1 = store::ConversationParticipant {
        conv_id: "conv123".to_string(),
        user_id: user1.id,
        is_blocked: 0,
        join_ts: now,
    };
    let p2 = store::ConversationParticipant {
        conv_id: "conv123".to_string(),
        user_id: user2.id,
        is_blocked: 0,
        join_ts: now,
    };

    conv_part_store.insert(&p1).await.unwrap();
    conv_part_store.insert(&p2).await.unwrap();

    let participants = conv_part_store.get_participants("conv123").await.unwrap();
    assert_eq!(participants.len(), 2);
}

#[tokio::test]
async fn test_group_store_get_member_count() {
    let (_fixture, store) = setup_test_store().await;

    let user_store = store::UserStore::new(store.clone());
    let group_mem_store = store::GroupMemberStore::new(store.clone());
    let group_store = store::GroupStore::new(store.clone());

    let now = chrono::Utc::now().timestamp_millis();
    let group = store::Group {
        group_id: "group_count_test".to_string(),
        name: "Count Test".to_string(),
        description: String::new(),
        owner_id: 1,
        created_at: now,
        updated_at: now,
        is_deleted: 0,
    };
    group_store.insert(&group).await.unwrap();

    let count_before = group_store.get_member_count("group_count_test").await.unwrap();
    assert_eq!(count_before, 0);

    let user1 = create_test_user(&user_store).await;
    let user2 = create_test_user(&user_store).await;

    group_mem_store
        .insert(&store::GroupMember {
            id: 0,
            group_id: "group_count_test".to_string(),
            uid: user1.id,
            role: "owner".to_string(),
            joined_at: now,
        })
        .await
        .unwrap();
    group_mem_store
        .insert(&store::GroupMember {
            id: 0,
            group_id: "group_count_test".to_string(),
            uid: user2.id,
            role: "member".to_string(),
            joined_at: now,
        })
        .await
        .unwrap();

    let count_after = group_store.get_member_count("group_count_test").await.unwrap();
    assert_eq!(count_after, 2);
}

#[tokio::test]
async fn test_friend_request_store_get_pending_requests() {
    let (_fixture, store) = setup_test_store().await;

    let friend_store = store::FriendRequestStore::new(store.clone());

    let now = chrono::Utc::now().timestamp_millis();

    friend_store
        .insert(&store::FriendRequest {
            id: 0,
            from_user_id: 1,
            to_user_id: 10,
            message: String::new(),
            status: "pending".to_string(),
            created_at: now,
            updated_at: now,
        })
        .await
        .unwrap();

    friend_store
        .insert(&store::FriendRequest {
            id: 0,
            from_user_id: 2,
            to_user_id: 10,
            message: String::new(),
            status: "pending".to_string(),
            created_at: now,
            updated_at: now,
        })
        .await
        .unwrap();

    friend_store
        .insert(&store::FriendRequest {
            id: 0,
            from_user_id: 3,
            to_user_id: 10,
            message: String::new(),
            status: "accepted".to_string(),
            created_at: now,
            updated_at: now,
        })
        .await
        .unwrap();

    let pending = friend_store.get_pending_requests(10).await.unwrap();
    assert_eq!(pending.len(), 2);
    assert!(pending.iter().all(|r| r.status == "pending"));
}

#[tokio::test]
async fn test_conversation_participant_is_blocked() {
    let (_fixture, store) = setup_test_store().await;

    let user_store = store::UserStore::new(store.clone());
    let conv_part_store = store::ConversationParticipantStore::new(store.clone());

    let user1 = create_test_user(&user_store).await;
    let user2 = create_test_user(&user_store).await;

    let now = chrono::Utc::now().timestamp_millis();
    conv_part_store
        .insert(&store::ConversationParticipant {
            conv_id: "conv_blocked_test".to_string(),
            user_id: user1.id,
            is_blocked: 0,
            join_ts: now,
        })
        .await
        .unwrap();
    conv_part_store
        .insert(&store::ConversationParticipant {
            conv_id: "conv_blocked_test".to_string(),
            user_id: user2.id,
            is_blocked: 1,
            join_ts: now,
        })
        .await
        .unwrap();

    let blocked1 = conv_part_store
        .is_blocked("conv_blocked_test", user1.id)
        .await
        .unwrap();
    assert!(!blocked1);

    let blocked2 = conv_part_store
        .is_blocked("conv_blocked_test", user2.id)
        .await
        .unwrap();
    assert!(blocked2);
}

#[tokio::test]
async fn test_conversation_participant_count_user_conversations() {
    let (_fixture, store) = setup_test_store().await;

    let user_store = store::UserStore::new(store.clone());
    let conv_part_store = store::ConversationParticipantStore::new(store.clone());

    let user = create_test_user(&user_store).await;

    let now = chrono::Utc::now().timestamp_millis();
    conv_part_store
        .insert(&store::ConversationParticipant {
            conv_id: "conv_count_1".to_string(),
            user_id: user.id,
            is_blocked: 0,
            join_ts: now,
        })
        .await
        .unwrap();
    conv_part_store
        .insert(&store::ConversationParticipant {
            conv_id: "conv_count_2".to_string(),
            user_id: user.id,
            is_blocked: 0,
            join_ts: now,
        })
        .await
        .unwrap();

    let count = conv_part_store
        .count_user_conversations(user.id)
        .await
        .unwrap();
    assert_eq!(count, 2);
}

#[tokio::test]
async fn test_message_store_get_message() {
    let (_fixture, store) = setup_test_store().await;

    let msg_store = store::MessageStore::new(store.clone());

    let table_name = format!("messages_{}", chrono::Utc::now().format("%Y%m%d"));
    msg_store.ensure_partition(&table_name).await.unwrap();

    let conv_id = format!("conv_msg_test_{}", chrono::Utc::now().timestamp_millis());
    let from_uid: i64 = 999;
    let content = "test message content";

    let msg_id = msg_store
        .insert_message(&conv_id, from_uid, content)
        .await
        .unwrap();

    let found = msg_store.get_message(msg_id).await.unwrap();
    assert!(found.is_some());
    let msg = found.unwrap();
    assert_eq!(msg.msg_id, msg_id);
    assert_eq!(msg.conv_id, conv_id);
    assert_eq!(msg.from_uid, from_uid);
    assert_eq!(msg.content, content);
    assert!(!msg.is_recalled);

    let not_found = msg_store.get_message(9999999).await.unwrap();
    assert!(not_found.is_none());
}
