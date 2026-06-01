use axum::Router;
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Duration;
use tokio::signal;
use tower_http::limit::RequestBodyLimitLayer;
use tower_http::trace::TraceLayer;
use tracing::info_span;
use tracing_subscriber::layer::SubscriberExt;
use tracing_subscriber::util::SubscriberInitExt;
use vortex__::*;

const MAX_REQUEST_BODY_SIZE: usize = 1 << 20;

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
    rate_limiter.start_cleanup(
        std::time::Duration::from_secs(300),
        std::time::Duration::from_secs(600),
    );

    let user_store = store::UserStore::new(store.clone());
    let msg_store = store::MessageStore::new(store.clone());
    let group_store = store::GroupStore::new(store.clone());
    let group_mem_store = store::GroupMemberStore::new(store.clone());
    let friend_store = store::FriendRequestStore::new(store.clone());
    let conv_part_store = store::ConversationParticipantStore::new(store.clone());
    let id_gen_state_store = store::IdGeneratorStateStore::new(store.clone());
    let idempotency_store = store::MessageIdempotencyStore::new(store.clone());

    let id_gen = idgen::IdGenerator::new(
        pool.clone(),
        id_gen_state_store.clone(),
        msg_store.clone(),
        cfg.node_id,
        epoch_time,
    );
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

    let user_cache = shared::UserCache::builder()
        .max_capacity(5000)
        .time_to_live(std::time::Duration::from_secs(300))
        .time_to_idle(std::time::Duration::from_secs(60))
        .build();

    let svc = shared::Service::new(
        cfg.clone(),
        pool.clone(),
        user_store,
        msg_store.clone(),
        group_store,
        group_mem_store,
        friend_store,
        conv_part_store,
        idempotency_store.clone(),
        id_gen,
        s3_service,
        user_cache,
    );

    let jwt_service = jwt::JwtService::new(
        pool.clone(),
        &cfg.jwt_secret,
        &cfg.jwt_issuer,
        cfg.jwt_expires_minutes,
    )
    .await;
    jwt_service.start_cleanup(std::time::Duration::from_secs(3600));

    let app_state = Arc::new(AppState {
        svc,
        jwt_service,
        rate_limiter,
    });

    let app = setup_routes(app_state);

    let worker = worker::Worker::new(
        cfg.clone(),
        pool.clone(),
        msg_store.clone(),
        idempotency_store.clone(),
    );

    let mut create_err = None;
    for i in 0..3 {
        match worker.create_tables_from_today_to_sunday_with_error().await {
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

    worker.start().await;

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

    axum::serve(listener, app.into_make_service())
        .with_graceful_shutdown(async move {
            shutdown_signal.await;
            tracing::info!("shutting down gracefully...");

            let cleanup_done = tokio::spawn(async move {
                worker.stop().await;
            });

            let force_shutdown = tokio::spawn(async {
                if let Ok(()) = signal::ctrl_c().await {
                    tracing::warn!("second signal received, forcing shutdown");
                    std::process::exit(1);
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
        .with(tracing_subscriber::fmt::layer().json().with_target(false))
        .init();
}

async fn init_db(cfg: &Config) -> Result<sqlx::PgPool, String> {
    let pool = sqlx::postgres::PgPoolOptions::new()
        .max_connections(cfg.db_max_open_conns as u32)
        .idle_timeout(Duration::from_secs(600)) // 空闲连接 10 分钟回收
        .max_lifetime(Duration::from_secs(3600)) // 连接最大生命周期 1 小时
        .connect(&cfg.database_url)
        .await
        .map_err(|e| format!("failed to connect to database: {}", e))?;

    Ok(pool)
}

fn setup_routes(app_state: Arc<AppState>) -> Router<()> {
    let app = Router::new()
        .route("/health", axum::routing::get(shared::health_check))
        .route("/ready", axum::routing::get(shared::readiness_check))
        .route("/metrics", axum::routing::get(metrics::metrics))
        .route("/api/auth/register", axum::routing::post(account::register))
        .route("/api/auth/login", axum::routing::post(account::login))
        .route("/api/auth/me", axum::routing::get(account::get_me))
        .route("/api/auth/logout", axum::routing::post(account::logout))
        .route(
            "/api/auth/:public_id",
            axum::routing::put(account::update_user),
        )
        .route(
            "/api/auth/:public_id",
            axum::routing::delete(account::delete_user),
        )
        .route(
            "/api/messages/send",
            axum::routing::post(messaging::send_message),
        )
        .route("/api/messages", axum::routing::get(messaging::get_messages))
        .route(
            "/api/messages/recall/:msg_id",
            axum::routing::post(messaging::recall_message),
        )
        .route(
            "/api/check",
            axum::routing::get(messaging::check_new_messages),
        )
        .route(
            "/api/conversations",
            axum::routing::get(messaging::get_conversations),
        )
        .route(
            "/api/conversations/count",
            axum::routing::get(messaging::get_conversation_count),
        )
        .route(
            "/api/conversations/:conv_id/participants",
            axum::routing::get(messaging::get_conversation_participants),
        )
        .route(
            "/api/conversations/:conv_id/blocked/:user_id",
            axum::routing::get(messaging::check_blocked),
        )
        .route(
            "/api/messages/:msg_id",
            axum::routing::get(messaging::get_message),
        )
        .route(
            "/api/blocks/:target_public_id",
            axum::routing::post(friend::block_user),
        )
        .route(
            "/api/blocks/:target_public_id",
            axum::routing::delete(friend::unblock_user),
        )
        .route("/api/groups", axum::routing::post(groups::create_group))
        .route("/api/groups/:id", axum::routing::get(groups::get_group))
        .route("/api/groups/:id", axum::routing::put(groups::update_group))
        .route(
            "/api/groups/:id",
            axum::routing::delete(groups::delete_group),
        )
        .route(
            "/api/groups/:id/join",
            axum::routing::post(groups::join_group),
        )
        .route(
            "/api/groups/:id/leave",
            axum::routing::post(groups::leave_group),
        )
        .route(
            "/api/groups/:id/members/:member_public_id",
            axum::routing::delete(groups::kick_member),
        )
        .route(
            "/api/groups/:id/members/count",
            axum::routing::get(groups::get_group_member_count),
        )
        .route(
            "/api/friends/request/send/:target_public_id",
            axum::routing::post(friend::send_friend_request),
        )
        .route(
            "/api/friends/requests",
            axum::routing::get(friend::get_friend_requests),
        )
        .route(
            "/api/friends/requests/pending",
            axum::routing::get(friend::get_pending_requests),
        )
        .route(
            "/api/friends/request/:request_id/accept",
            axum::routing::post(friend::accept_friend_request),
        )
        .route(
            "/api/friends/request/:request_id/reject",
            axum::routing::post(friend::reject_friend_request),
        )
        .route(
            "/api/friends/request/:request_id",
            axum::routing::delete(friend::cancel_friend_request),
        )
        .route("/api/files/presign", axum::routing::post(s3::presign))
        .route_layer(axum::middleware::from_fn_with_state(
            (*app_state).clone(),
            jwt::auth_middleware,
        ));

    let app = app
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
                .on_response(
                    |response: &axum::http::Response<_>,
                     latency: Duration,
                     _span: &tracing::Span| {
                        let path = response.extensions().get::<String>();
                        if let Some(path_str) = path
                            && (path_str == "/health" || path_str == "/ready")
                        {
                            return;
                        }

                        let status = response.status();
                        if latency.as_millis() > 500 || status.as_u16() >= 400 {
                            tracing::info!(
                                latency_ms = latency.as_millis(),
                                status = status.as_u16(),
                                "slow/error request"
                            );
                        }
                    },
                ),
        );

    app.with_state((*app_state).clone())
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
