use crate::config::Config;
use crate::store::MessageStore;
use chrono::{Datelike, Duration, NaiveDate, Timelike, Utc};
use sqlx::PgPool;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use tokio::sync::Mutex;
use tokio::time::{interval, sleep, Duration as TokioDuration};

pub struct Worker {
    cfg: Config,
    pool: PgPool,
    msg_store: MessageStore,
    stop_flag: Arc<AtomicBool>,
    started: Arc<Mutex<bool>>,
}

impl Worker {
    pub fn new(cfg: Config, pool: PgPool, msg_store: MessageStore) -> Self {
        Self {
            cfg,
            pool,
            msg_store,
            stop_flag: Arc::new(AtomicBool::new(false)),
            started: Arc::new(Mutex::new(false)),
        }
    }

    pub async fn start(&self) {
        let mut started = self.started.lock().await;
        if *started {
            return;
        }
        *started = true;
        drop(started);

        let stop_flag = self.stop_flag.clone();
        let pool = self.pool.clone();
        let msg_store = self.msg_store.clone();
        let cfg = self.cfg.clone();

        tokio::spawn(async move {
            run_table_manager(stop_flag.clone(), pool.clone(), msg_store.clone(), cfg.clone()).await;
        });

        let stop_flag = self.stop_flag.clone();
        let pool = self.pool.clone();
        let msg_store = self.msg_store.clone();
        let cfg = self.cfg.clone();

        tokio::spawn(async move {
            run_maintenance(stop_flag.clone(), pool.clone(), msg_store.clone(), cfg.clone()).await;
        });

        tracing::info!("Worker started");
    }

    pub async fn stop(&self) {
        let mut started = self.started.lock().await;
        if !*started {
            return;
        }
        *started = false;
        drop(started);

        self.stop_flag.store(true, Ordering::SeqCst);
        tracing::info!("Worker stopped");
    }

    pub fn create_tables_from_today_to_sunday(&self) {
        let now = Utc::now();
        let day_of_week = now.weekday().num_days_from_sunday();
        let days_to_sunday = if day_of_week == 0 { 0 } else { 7 - day_of_week as i32 };

        for offset in 0..=days_to_sunday {
            let date = now + Duration::days(offset as i64);
            let table_name = message_table_name_by_date(date);
            if let Err(e) = self.msg_store.ensure_partition(&table_name) {
                tracing::error!("failed to create table: table={} error={}", table_name, e);
            }
        }
        tracing::info!("initial message tables created");
    }

    pub fn create_tables_from_today_to_sunday_with_error(&self) -> Result<(), String> {
        let now = Utc::now();
        let day_of_week = now.weekday().num_days_from_sunday();
        let days_to_sunday = if day_of_week == 0 { 0 } else { 7 - day_of_week as i32 };

        let mut last_err = None;
        for offset in 0..=days_to_sunday {
            let date = now + Duration::days(offset as i64);
            let table_name = message_table_name_by_date(date);
            if let Err(e) = self.msg_store.ensure_partition(&table_name) {
                tracing::error!("failed to create table: table={} error={}", table_name, e);
                last_err = Some(e);
            }
        }
        tracing::info!("initial message tables created");

        if let Some(e) = last_err {
            Err(e)
        } else {
            Ok(())
        }
    }
}

async fn run_table_manager(
    stop_flag: Arc<AtomicBool>,
    pool: PgPool,
    msg_store: MessageStore,
    cfg: Config,
) {
    let next_monday = calculate_next_monday_delay();
    let delay = TokioDuration::from_secs(next_monday.as_secs());

    tokio::select! {
        _ = sleep(delay) => {}
        _ = async {
            while !stop_flag.load(Ordering::SeqCst) {
                tokio::time::sleep(TokioDuration::from_millis(100)).await;
            }
        } => return,
    }

    let interval_hours = cfg.worker_table_create_interval_hours;
    let mut ticker = interval(TokioDuration::from_secs(interval_hours * 3600));

    loop {
        ticker.tick().await;
        if stop_flag.load(Ordering::SeqCst) {
            return;
        }
        create_week_tables(&pool, &msg_store).await;
    }
}

async fn run_maintenance(
    stop_flag: Arc<AtomicBool>,
    pool: PgPool,
    msg_store: MessageStore,
    cfg: Config,
) {
    let initial_delay = cfg.worker_maintenance_initial_delay_minutes;
    let delay = TokioDuration::from_secs(initial_delay * 60);

    tokio::select! {
        _ = sleep(delay) => {}
        _ = async {
            while !stop_flag.load(Ordering::SeqCst) {
                tokio::time::sleep(TokioDuration::from_millis(100)).await;
            }
        } => return,
    }

    let interval_hours = cfg.worker_maintenance_interval_hours;
    let mut ticker = interval(TokioDuration::from_secs(interval_hours * 3600));

    loop {
        ticker.tick().await;
        if stop_flag.load(Ordering::SeqCst) {
            return;
        }
        run_analyze(&pool).await;
        drop_expired_partitions(&pool, &msg_store, cfg.message_retention_days).await;
    }
}

async fn run_analyze(pool: &PgPool) {
    match sqlx::query("ANALYZE messages").execute(pool).await {
        Ok(_) => tracing::info!("maintenance: ANALYZE completed"),
        Err(e) => tracing::error!("maintenance: ANALYZE failed: error={}", e),
    }
}

async fn drop_expired_partitions(pool: &PgPool, _msg_store: &MessageStore, retention_days: i32) {
    let cutoff = Utc::now() - Duration::days(retention_days as i64);

    let rows = match sqlx::query_scalar::<_, String>(
        r#"
        SELECT inhrelid::regclass::text
        FROM pg_inherits
        WHERE inhparent = 'messages'::regclass
        "#,
    )
    .fetch_all(pool)
    .await
    {
        Ok(rows) => rows,
        Err(e) => {
            tracing::error!("maintenance: failed to list partitions: error={}", e);
            return;
        }
    };

    for partition in rows {
        if partition.len() < 10 || !partition.starts_with("messages_") {
            continue;
        }
        let date_str = &partition[9..];
        let partition_date = match NaiveDate::parse_from_str(date_str, "%Y%m%d") {
            Ok(d) => d,
            Err(_) => continue,
        };

        let partition_utc = partition_date.and_hms_opt(0, 0, 0).unwrap().and_utc();

        if partition_utc < cutoff {
            let quoted = format!("\"{}\"", partition.replace('"', "\"\""));
            let query = format!("DROP TABLE IF EXISTS {}", quoted);
            match sqlx::query(&query).execute(pool).await {
                Ok(_) => tracing::info!("maintenance: dropped partition: partition={}", partition),
                Err(e) => tracing::error!(
                    "maintenance: failed to drop partition: partition={} error={}",
                    partition,
                    e
                ),
            }
        }
    }
    tracing::info!("maintenance: drop expired partitions completed");
}

async fn create_week_tables(pool: &PgPool, msg_store: &MessageStore) {
    let now = Utc::now();
    let weekday = now.weekday().num_days_from_sunday();
    let weekday = if weekday == 0 { 7 } else { weekday as i32 };
    let next_monday = now + Duration::days((8 - weekday) as i64);

    for offset in 0..7 {
        let date = next_monday + Duration::days(offset as i64);
        let table_name = message_table_name_by_date(date);
        if let Err(e) = msg_store.ensure_partition(&table_name) {
            tracing::error!("failed to create table: table={} error={}", table_name, e);
        }
    }
    tracing::info!("weekly message tables created");
}

fn calculate_next_monday_delay() -> Duration {
    let now = Utc::now();
    let days_until_monday = (8 - now.weekday().num_days_from_sunday() as i32) % 7;
    let days_until_monday = if days_until_monday == 0 { 7 } else { days_until_monday };
    let next_monday = now.date_naive() + Duration::days(days_until_monday as i64);
    let next_monday_utc = next_monday.and_hms_opt(0, 0, 0).unwrap().and_utc();
    next_monday_utc - now
}

pub fn message_table_name_by_date(date: chrono::DateTime<Utc>) -> String {
    format!("messages_{}", date.format("%Y%m%d"))
}

pub fn message_table_name_by_ts(ts: i64) -> String {
    let dt = chrono::DateTime::<Utc>::from_timestamp_millis(ts).unwrap_or_default();
    message_table_name_by_date(dt)
}
