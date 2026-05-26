#![allow(dead_code)]

use sqlx::PgPool;
use std::sync::atomic::{AtomicI64, Ordering};
use testcontainers::runners::AsyncRunner;
use testcontainers_modules::postgres::Postgres;

static TEST_USER_COUNTER: AtomicI64 = AtomicI64::new(0);

pub fn unique_username() -> String {
    let n = TEST_USER_COUNTER.fetch_add(1, Ordering::SeqCst) + 1;
    format!("testuser{}", n)
}

pub struct TestFixture {
    pub pool: PgPool,
    pub container: testcontainers::ContainerAsync<Postgres>,
}

impl TestFixture {
    pub async fn new() -> Self {
        let container = Postgres::default().start().await.unwrap();
        
        let host = container.get_host().await.unwrap();
        let host_port = container.get_host_port_ipv4(5432).await.unwrap();
        
        let database_url = format!(
            "postgres://postgres:postgres@{}:{}/postgres",
            host, host_port
        );
        
        let pool = PgPool::connect(&database_url)
            .await
            .expect("failed to connect to test database");
        
        vortex__::migration::run_migrations(&pool)
            .await
            .expect("failed to run migrations");
        
        Self { pool, container }
    }
    
    pub async fn cleanup(&self) {
        let _ = sqlx::query("DROP TABLE IF EXISTS users CASCADE")
            .execute(&self.pool)
            .await;
        let _ = sqlx::query("DROP TABLE IF EXISTS messages CASCADE")
            .execute(&self.pool)
            .await;
        let _ = sqlx::query("DROP TABLE IF EXISTS groups CASCADE")
            .execute(&self.pool)
            .await;
        let _ = sqlx::query("DROP TABLE IF EXISTS group_members CASCADE")
            .execute(&self.pool)
            .await;
        let _ = sqlx::query("DROP TABLE IF EXISTS friend_requests CASCADE")
            .execute(&self.pool)
            .await;
        let _ = sqlx::query("DROP TABLE IF EXISTS conversation_participants CASCADE")
            .execute(&self.pool)
            .await;
        let _ = sqlx::query("DROP TABLE IF EXISTS id_generator_state CASCADE")
            .execute(&self.pool)
            .await;
        let _ = sqlx::query("DROP TABLE IF EXISTS message_idempotency CASCADE")
            .execute(&self.pool)
            .await;
        let _ = sqlx::query("DROP TABLE IF EXISTS jwt_blacklist CASCADE")
            .execute(&self.pool)
            .await;
    }
}
