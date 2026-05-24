mod test_utils;

use test_utils::{TestFixture, unique_username};
use vortex--::{config, idgen, shared, store};

async fn setup_test_store() -> (TestFixture, store::Store) {
    let fixture = TestFixture::new().await;
    
    let epoch_time = 1609459200000i64;
    let store = store::Store::new(fixture.pool.clone(), epoch_time);
    
    (fixture, store)
}

#[tokio::test]
async fn test_user_store_insert() {
    let (_fixture, store) = setup_test_store().await;
    
    let user_store = store::UserStore::new(store.clone());
    
    let user = store::User {
        id: 1,
        public_id: "test123".to_string(),
        username: unique_username(),
        pwd_hash: "hashed_password".to_string(),
        email: "test@example.com".to_string(),
        created_at: chrono::Utc::now(),
    };
    
    user_store.insert(&user).await.unwrap();
    
    let found = user_store.get_by_id(1).await.unwrap().unwrap();
    assert_eq!(found.username, user.username);
}

#[tokio::test]
async fn test_user_store_get_by_username() {
    let (_fixture, store) = setup_test_store().await;
    
    let user_store = store::UserStore::new(store.clone());
    
    let username = unique_username();
    let user = store::User {
        id: 1,
        public_id: "test123".to_string(),
        username: username.clone(),
        pwd_hash: "hashed_password".to_string(),
        email: "test@example.com".to_string(),
        created_at: chrono::Utc::now(),
    };
    
    user_store.insert(&user).await.unwrap();
    
    let found = user_store.get_by_username(&username).await.unwrap().unwrap();
    assert_eq!(found.username, username);
}

#[tokio::test]
async fn test_user_store_get_by_public_id() {
    let (_fixture, store) = setup_test_store().await;
    
    let user_store = store::UserStore::new(store.clone());
    
    let public_id = "test_public_id_123".to_string();
    let user = store::User {
        id: 1,
        public_id: public_id.clone(),
        username: unique_username(),
        pwd_hash: "hashed_password".to_string(),
        email: "test@example.com".to_string(),
        created_at: chrono::Utc::now(),
    };
    
    user_store.insert(&user).await.unwrap();
    
    let found = user_store.get_by_public_id(&public_id).await.unwrap().unwrap();
    assert_eq!(found.public_id, public_id);
}

#[tokio::test]
async fn test_message_store_ensure_partition() {
    let (_fixture, store) = setup_test_store().await;
    
    let msg_store = store::MessageStore::new(store.clone());
    
    let table_name = format!("messages_{}", chrono::Utc::now().format("%Y_%m_%d"));
    
    let result = msg_store.ensure_partition(&table_name).await;
    assert!(result.is_ok());
}

#[tokio::test]
async fn test_group_store_insert() {
    let (_fixture, store) = setup_test_store().await;
    
    let group_store = store::GroupStore::new(store.clone());
    
    let group = store::Group {
        id: 1,
        public_id: "group123".to_string(),
        name: "Test Group".to_string(),
        description: "A test group".to_string(),
        owner_id: 1,
        created_at: chrono::Utc::now(),
    };
    
    group_store.insert(&group).await.unwrap();
    
    let found = group_store.get_by_id(1).await.unwrap().unwrap();
    assert_eq!(found.name, "Test Group");
}

#[tokio::test]
async fn test_group_member_store_insert() {
    let (_fixture, store) = setup_test_store().await;
    
    let group_mem_store = store::GroupMemberStore::new(store.clone());
    
    group_mem_store.insert("group123", 1).await.unwrap();
    
    let is_member = group_mem_store.is_member("group123", 1).await.unwrap();
    assert!(is_member);
}

#[tokio::test]
async fn test_group_member_store_remove() {
    let (_fixture, store) = setup_test_store().await;
    
    let group_mem_store = store::GroupMemberStore::new(store.clone());
    
    group_mem_store.insert("group123", 1).await.unwrap();
    group_mem_store.remove("group123", 1).await.unwrap();
    
    let is_member = group_mem_store.is_member("group123", 1).await.unwrap();
    assert!(!is_member);
}

#[tokio::test]
async fn test_friend_request_store_insert() {
    let (_fixture, store) = setup_test_store().await;
    
    let friend_store = store::FriendRequestStore::new(store.clone());
    
    let request = store::FriendRequest {
        id: 1,
        from_user_id: 1,
        to_user_id: 2,
        status: "pending".to_string(),
        created_at: chrono::Utc::now(),
    };
    
    friend_store.insert(&request).await.unwrap();
    
    let found = friend_store.get_by_id(1).await.unwrap().unwrap();
    assert_eq!(found.from_user_id, 1);
    assert_eq!(found.to_user_id, 2);
}

#[tokio::test]
async fn test_friend_request_store_get_sent() {
    let (_fixture, store) = setup_test_store().await;
    
    let friend_store = store::FriendRequestStore::new(store.clone());
    
    let request = store::FriendRequest {
        id: 1,
        from_user_id: 1,
        to_user_id: 2,
        status: "pending".to_string(),
        created_at: chrono::Utc::now(),
    };
    
    friend_store.insert(&request).await.unwrap();
    
    let requests = friend_store.get_sent(1).await.unwrap();
    assert!(!requests.is_empty());
    assert_eq!(requests[0].to_user_id, 2);
}

#[tokio::test]
async fn test_friend_request_store_get_received() {
    let (_fixture, store) = setup_test_store().await;
    
    let friend_store = store::FriendRequestStore::new(store.clone());
    
    let request = store::FriendRequest {
        id: 1,
        from_user_id: 1,
        to_user_id: 2,
        status: "pending".to_string(),
        created_at: chrono::Utc::now(),
    };
    
    friend_store.insert(&request).await.unwrap();
    
    let requests = friend_store.get_received(2).await.unwrap();
    assert!(!requests.is_empty());
    assert_eq!(requests[0].from_user_id, 1);
}

#[tokio::test]
async fn test_conversation_participant_store() {
    let (_fixture, store) = setup_test_store().await;
    
    let conv_part_store = store::ConversationParticipantStore::new(store.clone());
    
    conv_part_store.add_participant("conv123", 1).await.unwrap();
    conv_part_store.add_participant("conv123", 2).await.unwrap();
    
    let participants = conv_part_store.get_participants("conv123").await.unwrap();
    assert_eq!(participants.len(), 2);
}
