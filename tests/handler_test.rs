mod test_utils;

use axum::Router;
use axum::http::{StatusCode, header};
use serde_json::json;
use std::sync::Arc;
use test_utils::{TestFixture, unique_username};
use tower::util::ServiceExt;
use vortex__::{AppState, account, config, idgen, jwt, ratelimit, shared, store};

async fn setup_test_app() -> (TestFixture, Router, Arc<AppState>) {
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
    let rate_limiter = ratelimit::RateLimiter::new();

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
    );

    let jwt_service = jwt::JwtService::new(
        fixture.pool.clone(),
        &cfg.jwt_secret,
        &cfg.jwt_issuer,
        cfg.jwt_expires_minutes,
    );

    let app_state = Arc::new(AppState {
        svc,
        jwt_service,
        rate_limiter,
    });

    let app = Router::new()
        .route("/api/auth/register", axum::routing::post(account::register))
        .route("/api/auth/login", axum::routing::post(account::login))
        .with_state((*app_state).clone());

    (fixture, app, app_state)
}

#[tokio::test]
async fn test_register_success() {
    let (_fixture, app, _state) = setup_test_app().await;

    let username = unique_username();
    let body = json!({
        "username": username,
        "password": "Test1234!",
        "email": format!("{}@example.com", username)
    });

    let response = app
        .oneshot(
            axum::http::Request::builder()
                .method("POST")
                .uri("/api/auth/register")
                .header(header::CONTENT_TYPE, "application/json")
                .body(axum::body::Body::from(
                    serde_json::to_string(&body).unwrap(),
                ))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::CREATED);
}

#[tokio::test]
async fn test_register_weak_password() {
    let (_fixture, app, _state) = setup_test_app().await;

    let body = json!({
        "username": "weakuser",
        "password": "123",
        "email": "weak@example.com"
    });

    let response = app
        .oneshot(
            axum::http::Request::builder()
                .method("POST")
                .uri("/api/auth/register")
                .header(header::CONTENT_TYPE, "application/json")
                .body(axum::body::Body::from(
                    serde_json::to_string(&body).unwrap(),
                ))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

#[tokio::test]
async fn test_register_invalid_username() {
    let (_fixture, app, _state) = setup_test_app().await;

    let body = json!({
        "username": "ab",
        "password": "Test1234!",
        "email": "ab@example.com"
    });

    let response = app
        .oneshot(
            axum::http::Request::builder()
                .method("POST")
                .uri("/api/auth/register")
                .header(header::CONTENT_TYPE, "application/json")
                .body(axum::body::Body::from(
                    serde_json::to_string(&body).unwrap(),
                ))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

#[tokio::test]
async fn test_register_duplicate() {
    let (_fixture, app, _state) = setup_test_app().await;

    let username = unique_username();
    let body = json!({
        "username": &username,
        "password": "Test1234!",
        "email": format!("{}@example.com", username)
    });

    let response = app
        .clone()
        .oneshot(
            axum::http::Request::builder()
                .method("POST")
                .uri("/api/auth/register")
                .header(header::CONTENT_TYPE, "application/json")
                .body(axum::body::Body::from(
                    serde_json::to_string(&body).unwrap(),
                ))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::CREATED);

    let response = app
        .oneshot(
            axum::http::Request::builder()
                .method("POST")
                .uri("/api/auth/register")
                .header(header::CONTENT_TYPE, "application/json")
                .body(axum::body::Body::from(
                    serde_json::to_string(&body).unwrap(),
                ))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::CONFLICT);
}

#[tokio::test]
async fn test_login_success() {
    let (_fixture, app, _state) = setup_test_app().await;

    let username = unique_username();
    let body = json!({
        "username": &username,
        "password": "Test1234!",
        "email": format!("{}@example.com", username)
    });

    app.clone()
        .oneshot(
            axum::http::Request::builder()
                .method("POST")
                .uri("/api/auth/register")
                .header(header::CONTENT_TYPE, "application/json")
                .body(axum::body::Body::from(
                    serde_json::to_string(&body).unwrap(),
                ))
                .unwrap(),
        )
        .await
        .unwrap();

    let login_body = json!({
        "username": &username,
        "password": "Test1234!"
    });

    let response = app
        .oneshot(
            axum::http::Request::builder()
                .method("POST")
                .uri("/api/auth/login")
                .header(header::CONTENT_TYPE, "application/json")
                .body(axum::body::Body::from(
                    serde_json::to_string(&login_body).unwrap(),
                ))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);
}

#[tokio::test]
async fn test_login_wrong_password() {
    let (_fixture, app, _state) = setup_test_app().await;

    let username = unique_username();
    let body = json!({
        "username": &username,
        "password": "Test1234!",
        "email": format!("{}@example.com", username)
    });

    app.clone()
        .oneshot(
            axum::http::Request::builder()
                .method("POST")
                .uri("/api/auth/register")
                .header(header::CONTENT_TYPE, "application/json")
                .body(axum::body::Body::from(
                    serde_json::to_string(&body).unwrap(),
                ))
                .unwrap(),
        )
        .await
        .unwrap();

    let login_body = json!({
        "username": &username,
        "password": "wrongpass1!"
    });

    let response = app
        .oneshot(
            axum::http::Request::builder()
                .method("POST")
                .uri("/api/auth/login")
                .header(header::CONTENT_TYPE, "application/json")
                .body(axum::body::Body::from(
                    serde_json::to_string(&login_body).unwrap(),
                ))
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}
