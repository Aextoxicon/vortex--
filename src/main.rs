mod account;
mod config;
mod error;
mod friend;
mod groups;
mod idgen;
mod jwt;
mod messaging;
mod metrics;
mod migration;
mod ratelimit;
mod s3;
mod shared;
mod store;
mod worker;

use axum::extract::State;
use axum::Router;
use config::Config;
use error::ErrorResponse;
use ratelimit::RateLimiter;
use shared::Handler;
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Duration;
use tokio::signal;
use tower::timeout::TimeoutLayer;
use tower_http::add_extension::AddExtensionLayer;
use tower_http::limit::RequestBodyLimitLayer;
use tower_http::trace::TraceLayer;
use tracing::info_span;
use tracing_subscriber::layer::SubscriberExt;
use tracing_subscriber::util::SubscriberInitExt;

pub struct AppState {
    pub handler: Handler,
    pub jwt_service: jwt::JwtService,
    pub rate_limiter: RateLimiter,
}

const REQUEST_TIMEOUT: Duration = Duration::from_secs(30);
const MAX_REQUEST_BODY_SIZE: usize = 1 << 20;
const MAX_MESSAGE_DAYS: i64 = 7;
const MAX_CONVERSATIONS: i64 = 100;

#[tokio::main]
async fn main() {
    init_logging();

    metrics::print_startup_info();
    let cfg = config::load_config();

    tracing::info!(
        node_id = cfg.node_id,
        port = %cfg.port,
        s3_enabled = !cfg.s3_url.is_empty(),
        "configuration loaded"
    );

    let pool = match init_db(&cfg).await {
        Ok(pool) => pool,
        Err(err) => {
            tracing::error!(error = %err, "failed to init database");
            std::process::exit(1);
        }
    };

    if let Err(err) = migration::run_migrations(&pool).await {
        tracing::error!(error = %err, "failed to run migrations");
        std::process::exit(1);
    }

    let epoch_time = 1609459200000i64;
    let store = store::Store::new(pool.clone(), epoch_time);
    let rate_limiter = RateLimiter::new();
    rate_limiter.start_cleanup();

    let user_store = store::UserStore::new(store.clone());
    let msg_store = store::MessageStore::new(store.clone());
    let group_store = store::GroupStore::new(store.clone());
    let group_mem_store = store::GroupMemberStore::new(store.clone());
    let friend_store = store::FriendRequestStore::new(store.clone());
    let conv_part_store = store::ConversationParticipantStore::new(store.clone());
    let id_gen_state_store = store::IdGeneratorStateStore::new(store.clone());
    let idempotency_store = store::MessageIdempotencyStore::new(store.clone());

    let id_gen = idgen::IdGenerator::new(store.clone());
    id_gen.init().await;

    let s3_service = if !cfg.s3_url.is_empty() {
        let s3_cfg = match config::parse_s3_url(&cfg.s3_url) {
            Ok(cfg) => cfg,
            Err(err) => {
                tracing::error!(error = %err, "failed to parse S3 URL");
                std::process::exit(1);
            }
        };
        match s3::S3Service::new(
            &s3_cfg.bucket,
            &s3_cfg.region,
            &s3_cfg.endpoint,
            &s3_cfg.access_key,
            &s3_cfg.secret_key,
        )
        .await
        {
            Ok(svc) => Some(svc),
            Err(err) => {
                tracing::error!(error = %err, "failed to init S3 service");
                std::process::exit(1);
            }
        }
    } else {
        None
    };

    let svc = shared::Service::new(
        cfg.clone(),
        pool.clone(),
        user_store,
        msg_store,
        group_store,
        group_mem_store,
        friend_store,
        conv_part_store,
        id_gen_state_store,
        idempotency_store,
        id_gen,
        s3_service,
    );

    let jwt_service = jwt::JwtService::new(
        pool.clone(),
        &cfg.jwt_secret,
        &cfg.jwt_issuer,
        cfg.jwt_expires_minutes,
    );
    jwt_service.start_cleanup(std::time::Duration::from_secs(3600));

    let handler = shared::Handler::new(svc.clone(), jwt_service.clone(), cfg.clone());

    let app_state = Arc::new(AppState {
        handler,
        jwt_service,
        rate_limiter,
    });

    let app = setup_routes(app_state);

    let worker = worker::Worker::new(&cfg);

    let mut create_err = None;
    for i in 0..3 {
        match worker.create_tables_from_today_to_sunday_with_error() {
            Ok(()) => {
                create_err = None;
                break;
            }
            Err(err) => {
                create_err = Some(err);
                if i < 2 {
                    tracing::warn!(
                        error = %create_err.as_ref().unwrap(),
                        attempt = i + 1,
                        "failed to create partition tables, retrying..."
                    );
                    tokio::time::sleep(Duration::from_secs((i + 1) as u64)).await;
                }
            }
        }
    }

    if let Some(err) = create_err {
        tracing::error!(error = %err, "failed to create initial partition tables after retries");
        std::process::exit(1);
    }

    worker.start();

    let addr: SocketAddr = cfg
        .port
        .parse()
        .unwrap_or_else(|_| format!("0.0.0.0{}", cfg.port).parse().unwrap());

    let listener = match tokio::net::TcpListener::bind(addr).await {
        Ok(l) => l,
        Err(err) => {
            tracing::error!(error = %err, "failed to bind address");
            std::process::exit(1);
        }
    };

    tracing::info!("server listening on {}", addr);

    let shutdown_signal = shutdown_signal();

    axum::serve(
        listener,
        app.into_make_service_with_connect_info::<SocketAddr>(),
    )
    .with_graceful_shutdown(async move {
        shutdown_signal.await;
        tracing::info!("shutting down gracefully...");

        let cleanup_done = tokio::spawn(async move {
            worker.stop();
            rate_limiter.stop();
            jwt_service.stop();
        });

        let force_shutdown = tokio::spawn(async {
            match signal::ctrl_c().await {
                Ok(()) => {
                    tracing::warn!("second signal received, forcing shutdown");
                    std::process::exit(1);
                }
                Err(_) => {}
            }
        });

        tokio::select! {
            _ = cleanup_done => {}
            _ = tokio::time::sleep(Duration::from_secs(10)) => {
                tracing::warn!("cleanup timed out, proceeding with server shutdown");
            }
            _ = force_shutdown => {}
        }
    })
    .await
    .unwrap_or_else(|err| {
        tracing::error!(error = %err, "server forced to shutdown");
    });
}

fn init_logging() {
    tracing_subscriber::registry()
        .with(
            tracing_subscriber::fmt::layer()
                .json()
                .with_target(false),
        )
        .init();
}

async fn init_db(cfg: &Config) -> Result<sqlx::PgPool, String> {
    let pool = sqlx::PgPool::connect(&cfg.database_url)
        .await
        .map_err(|e| format!("failed to connect to database: {}", e))?;

    pool.set_max_connections(cfg.db_max_open_conns as u32);

    Ok(pool)
}

fn setup_routes(app_state: Arc<AppState>) -> Router<SocketAddr> {
    let public_routes = Router::new()
        .route(
            "/auth/register",
            axum::routing::post(account::register),
        )
        .route(
            "/auth/login",
            axum::routing::post(account::login),
        )
        .layer(axum::extract::Extension(app_state.clone()));

    let auth_middleware = axum::middleware::from_fn(move |req, next| {
        let app_state = app_state.clone();
        async move {
            let auth_header = req
                .headers()
                .get(axum::http::header::AUTHORIZATION)
                .and_then(|h| h.to_str().ok());

            match auth_header {
                Some(token) => {
                    let token_str = if token.starts_with("Bearer ") {
                        &token[7..]
                    } else {
                        token
                    };

                    match app_state.jwt_service.validate_token(token_str) {
                        Ok(claims) => {
                            if app_state.jwt_service.is_blacklisted(&claims.jti) {
                                return axum::response::Response::builder()
                                    .status(axum::http::StatusCode::UNAUTHORIZED)
                                    .body(axum::body::Body::from("Token is blacklisted"))
                                    .unwrap();
                            }

                            let mut req = req;
                            req.extensions_mut().insert(claims.user_id);
                            req.extensions_mut().insert(claims.public_id.clone());
                            next.run(req).await
                        }
                        Err(_) => {
                            axum::response::Response::builder()
                                .status(axum::http::StatusCode::UNAUTHORIZED)
                                .body(axum::body::Body::from("Invalid token"))
                                .unwrap()
                        }
                    }
                }
                None => {
                    axum::response::Response::builder()
                        .status(axum::http::StatusCode::UNAUTHORIZED)
                        .body(axum::body::Body::from("Missing token"))
                        .unwrap()
                }
            }
        }
    });

    let protected_routes = Router::new()
        .route("/auth/me", axum::routing::get(account::get_me))
        .route("/auth/logout", axum::routing::post(account::logout))
        .route(
            "/auth/:public_id",
            axum::routing::put(account::update_user),
        )
        .route(
            "/auth/:public_id",
            axum::routing::delete(account::delete_user),
        )
        .route(
            "/messages/send",
            axum::routing::post(messaging::send_message),
        )
        .route(
            "/messages",
            axum::routing::get(messaging::get_messages),
        )
        .route(
            "/messages/recall/:msg_id",
            axum::routing::post(messaging::recall_message),
        )
        .route(
            "/check",
            axum::routing::get(messaging::check_new_messages),
        )
        .route(
            "/conversations",
            axum::routing::get(messaging::get_conversations),
        )
        .route(
            "/blocks/:target_public_id",
            axum::routing::post(messaging::block_user),
        )
        .route(
            "/blocks/:target_public_id",
            axum::routing::delete(messaging::unblock_user),
        )
        .route(
            "/groups",
            axum::routing::post(groups::create_group),
        )
        .route(
            "/groups/:id",
            axum::routing::get(groups::get_group),
        )
        .route(
            "/groups/:id",
            axum::routing::put(groups::update_group),
        )
        .route(
            "/groups/:id",
            axum::routing::delete(groups::delete_group),
        )
        .route(
            "/groups/:id/join",
            axum::routing::post(groups::join_group),
        )
        .route(
            "/groups/:id/leave",
            axum::routing::post(groups::leave_group),
        )
        .route(
            "/groups/:id/members/:member_public_id",
            axum::routing::delete(groups::kick_member),
        )
        .route(
            "/friends/request/send/:target_public_id",
            axum::routing::post(friend::send_friend_request),
        )
        .route(
            "/friends/requests",
            axum::routing::get(friend::get_friend_requests),
        )
        .route(
            "/friends/request/:request_id/accept",
            axum::routing::post(friend::accept_friend_request),
        )
        .route(
            "/friends/request/:request_id/reject",
            axum::routing::post(friend::reject_friend_request),
        )
        .route(
            "/friends/request/:request_id",
            axum::routing::delete(friend::cancel_friend_request),
        )
        .layer(auth_middleware)
        .layer(axum::extract::Extension(app_state.clone()));

    let api_routes = public_routes.merge(protected_routes);

    Router::new()
        .route("/health", axum::routing::get(shared::health_check))
        .route("/ready", axum::routing::get(shared::readiness_check))
        .route("/metrics", axum::routing::get(metrics::metrics))
        .nest("/api", api_routes)
        .layer(axum::extract::Extension(app_state))
        .layer(TimeoutLayer::new(REQUEST_TIMEOUT))
        .layer(RequestBodyLimitLayer::new(MAX_REQUEST_BODY_SIZE))
        .layer(
            TraceLayer::new_for_http()
                .make_span_with(|request: &axum::http::Request<_>| {
                    info_span!(
                        "request",
                        method = %request.method(),
                        uri = %request.uri(),
                    )
                })
                .on_response(|response: &axum::http::Response<_>, latency: Duration, _span: &tracing::Span| {
                    let path = response.extensions().get::<String>();
                    if let Some(path_str) = path {
                        if path_str == "/health" || path_str == "/ready" {
                            return;
                        }
                    }

                    let status = response.status();
                    if latency.as_millis() > 500 || status.as_u16() >= 400 {
                        tracing::info!(
                            latency_ms = latency.as_millis(),
                            status = status.as_u16(),
                            "slow/error request"
                        );
                    }
                }),
        )
}

async fn shutdown_signal() {
    let ctrl_c = async {
        signal::ctrl_c()
            .await
            .expect("failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        signal::unix::signal(signal::unix::SignalKind::terminate())
            .expect("failed to install signal handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}
