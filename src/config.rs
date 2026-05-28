use serde::Deserialize;
use std::collections::HashMap;
use std::env;
use std::fs;
use std::io::{self, BufRead, Write};
use std::path::Path;
use std::process;
use url::Url;

const DEFAULT_JWT_ISSUER: &str = "vortex";
const DEFAULT_JWT_EXPIRES_MINUTES: i32 = 10080;
const DEFAULT_BCRYPT_COST: i32 = 10;
const DEFAULT_MESSAGE_RECALL_WINDOW_MS: i64 = 120_000;
const DEFAULT_EPOCH_TIME: i64 = 1_767_225_600_000;
const DEFAULT_SEGMENT_DURATION_MS: i64 = 10_000;
const DEFAULT_SEGMENT_SIZE: i64 = 1 << 17;
const DEFAULT_MESSAGE_RETENTION_DAYS: i32 = 7;
const DEFAULT_PAGE_SIZE: i32 = 100;
const DEFAULT_MAX_PAGE_SIZE: i32 = 500;
const DEFAULT_PUBLIC_ID_LENGTH: i32 = 21;
const DEFAULT_GROUP_ID_RANDOM_LENGTH: i32 = 8;
const DEFAULT_WORKER_CREATE_INTERVAL: i32 = 168;
const DEFAULT_MAINTENANCE_DELAY: i32 = 5;
const DEFAULT_MAINTENANCE_INTERVAL: i32 = 24;
const DEFAULT_DB_MAX_OPEN_CONNS: i32 = 50;
const DEFAULT_DB_MAX_IDLE_CONNS: i32 = 20;
const DEFAULT_IDEMPOTENCY_RETENTION_HOURS: i64 = 24;

const VF_MAX_NODE_ID: i64 = (1 << 5) - 1;

#[derive(Debug, Clone)]
pub struct Config {
    pub node_id: i64,
    pub jwt_secret: String,
    pub port: String,
    pub database_url: String,
    pub db_max_open_conns: i32,
    pub db_max_idle_conns: i32,
    pub jwt_issuer: String,
    pub jwt_expires_minutes: i64,
    pub bcrypt_cost: i32,
    pub message_recall_window_ms: i64,
    pub epoch_time: i64,
    pub segment_duration_ms: i64,
    pub segment_size: i64,
    pub message_retention_days: i32,
    pub default_page_size: i32,
    pub max_page_size: i32,
    pub public_id_length: i32,
    pub group_id_random_length: i32,
    pub worker_table_create_interval_hours: i64,
    pub worker_maintenance_initial_delay_minutes: i64,
    pub worker_maintenance_interval_hours: i64,
    pub idempotency_retention_hours: i64,
    pub s3_url: String,
}

#[derive(Debug, Deserialize)]
pub struct S3Config {
    pub bucket: String,
    pub region: String,
    pub endpoint: String,
    pub access_key: String,
    pub secret_key: String,
}

fn load_dotenv_map() -> HashMap<String, String> {
    let mut map = HashMap::new();
    let dotenv_path = Path::new(".env");
    if !dotenv_path.exists() {
        return map;
    }
    let content = match fs::read_to_string(dotenv_path) {
        Ok(c) => c,
        Err(_) => return map,
    };
    for line in content.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        if let Some(eq_pos) = line.find('=') {
            let key = line[..eq_pos].trim().to_string();
            let value = line[eq_pos + 1..].trim().to_string();
            if !key.is_empty() {
                map.insert(key, value);
            }
        }
    }
    map
}

fn env_string(dotenv_map: &HashMap<String, String>, key: &str, fallback: &str) -> String {
    if let Some(v) = dotenv_map.get(key)
        && !v.is_empty()
    {
        return v.clone();
    }
    match env::var(key) {
        Ok(v) if !v.is_empty() => v,
        _ => fallback.to_string(),
    }
}

fn env_int(dotenv_map: &HashMap<String, String>, key: &str, fallback: i32) -> i32 {
    if let Some(v) = dotenv_map.get(key)
        && !v.is_empty()
    {
        match v.parse::<i32>() {
            Ok(n) => return n,
            Err(_) => {
                eprintln!("Error: invalid value for {} in .env: {}", key, v);
                process::exit(1);
            }
        }
    }
    match env::var(key) {
        Ok(v) if !v.is_empty() => match v.parse::<i32>() {
            Ok(n) => n,
            Err(_) => {
                eprintln!("Error: invalid value for {}: {}", key, v);
                process::exit(1);
            }
        },
        _ => fallback,
    }
}

fn env_int64(dotenv_map: &HashMap<String, String>, key: &str, fallback: i64) -> i64 {
    if let Some(v) = dotenv_map.get(key)
        && !v.is_empty()
    {
        match v.parse::<i64>() {
            Ok(n) => return n,
            Err(_) => {
                eprintln!("Error: invalid value for {} in .env: {}", key, v);
                process::exit(1);
            }
        }
    }
    match env::var(key) {
        Ok(v) if !v.is_empty() => match v.parse::<i64>() {
            Ok(n) => n,
            Err(_) => {
                eprintln!("Error: invalid value for {}: {}", key, v);
                process::exit(1);
            }
        },
        _ => fallback,
    }
}

fn generate_jwt_secret() -> Result<String, String> {
    use rand::RngCore;
    let mut bytes = [0u8; 32];
    rand::thread_rng().fill_bytes(&mut bytes);
    Ok(base64::Engine::encode(
        &base64::engine::general_purpose::STANDARD,
        bytes,
    ))
}

fn prompt_interactive_jwt() -> Result<String, String> {
    let stdin = io::stdin();
    let mut stdout = io::stdout();

    println!("JWT_SECRET is not configured.");
    println!("Options:");
    println!("  1. Auto-generate a new JWT secret (recommended)");
    println!("  2. Exit and configure manually");
    print!("Choose [1-2]: ");
    stdout.flush().unwrap();

    let mut input = String::new();
    stdin.lock().read_line(&mut input).unwrap();
    let input = input.trim();

    match input {
        "1" => {
            let secret = generate_jwt_secret()?;
            println!("\nGenerated JWT secret: {}", secret);
            println!("Please save this secret securely!");
            println!("You can set it with: export JWT_SECRET=\"{}\"\n", secret);
            Ok(secret)
        }
        "2" => {
            println!("Exiting. Please set JWT_SECRET environment variable and restart.");
            process::exit(0);
        }
        _ => {
            println!("Invalid option. Exiting.");
            process::exit(1);
        }
    }
}

fn prompt_interactive_s3() -> Result<String, String> {
    let stdin = io::stdin();
    let mut stdout = io::stdout();

    println!("S3_URL is not configured.");
    println!("Options:");
    println!("  1. Enable S3 storage (enter connection string)");
    println!("  2. Skip S3 configuration (file upload will be disabled)");
    println!("  3. Exit and configure manually");
    print!("Choose [1-3]: ");
    stdout.flush().unwrap();

    let mut input = String::new();
    stdin.lock().read_line(&mut input).unwrap();
    let input = input.trim();

    match input {
        "1" => {
            print!(
                "\nEnter S3 connection string (s3://bucket?endpoint=...&region=...&access_key=...&secret_key=...): "
            );
            stdout.flush().unwrap();
            let mut input_url = String::new();
            stdin.lock().read_line(&mut input_url).unwrap();
            let input_url = input_url.trim().to_string();
            if input_url.is_empty() {
                println!("Empty URL. S3 will be disabled.");
                return Ok(String::new());
            }
            Ok(input_url)
        }
        "2" => {
            println!("S3 storage disabled. File upload will be disabled.");
            Ok(String::new())
        }
        "3" => {
            println!("Exiting. Please set S3_URL environment variable and restart.");
            process::exit(0);
        }
        _ => {
            println!("Invalid option. S3 will be disabled.");
            Ok(String::new())
        }
    }
}

fn prompt_interactive_database() -> Result<String, String> {
    let stdin = io::stdin();
    let mut stdout = io::stdout();

    println!("DATABASE_URL is not configured.");
    println!("Options:");
    println!("  1. Use default PostgreSQL connection (localhost:5432/vortex)");
    println!("  2. Enter custom connection string");
    println!("  3. Exit and configure manually");
    print!("Choose [1-3]: ");
    stdout.flush().unwrap();

    let mut input = String::new();
    stdin.lock().read_line(&mut input).unwrap();
    let input = input.trim();

    match input {
        "1" => {
            println!("Using default: postgres://localhost:5432/vortex?sslmode=disable");
            Ok("postgres://localhost:5432/vortex?sslmode=disable".to_string())
        }
        "2" => {
            print!("\nEnter database connection string: ");
            stdout.flush().unwrap();
            let mut input_url = String::new();
            stdin.lock().read_line(&mut input_url).unwrap();
            let input_url = input_url.trim().to_string();
            if input_url.is_empty() {
                return Err("empty database URL".to_string());
            }
            Ok(input_url)
        }
        "3" => {
            println!("Exiting. Please set DATABASE_URL environment variable and restart.");
            process::exit(0);
        }
        _ => {
            println!("Invalid option. Exiting.");
            process::exit(1);
        }
    }
}

pub fn parse_s3_url(s3_url: &str) -> Result<S3Config, String> {
    if s3_url.is_empty() {
        return Err("empty S3 URL".to_string());
    }

    let u = Url::parse(s3_url).map_err(|e| format!("invalid S3 URL: {}", e))?;

    if u.scheme() != "s3" {
        return Err(format!(
            "invalid S3 URL scheme: {} (expected s3://)",
            u.scheme()
        ));
    }

    let bucket = u.host_str().ok_or("missing bucket in S3 URL")?;

    let mut region = String::new();
    let mut endpoint = String::new();
    let mut access_key = String::new();
    let mut secret_key = String::new();

    for (key, value) in u.query_pairs() {
        match key.as_ref() {
            "region" => region = value.into_owned(),
            "endpoint" => endpoint = value.into_owned(),
            "access_key" => access_key = value.into_owned(),
            "secret_key" => secret_key = value.into_owned(),
            _ => {}
        }
    }

    if let Some(pwd) = u.password() {
        secret_key = pwd.to_string();
    }
    if !u.username().is_empty() {
        access_key = u.username().to_string();
    }

    if region.is_empty() {
        region = "us-east-1".to_string();
    }

    Ok(S3Config {
        bucket: bucket.to_string(),
        region,
        endpoint,
        access_key,
        secret_key,
    })
}

impl Config {
    pub fn validate(&self) -> Result<(), String> {
        if self.node_id < 0 {
            return Err(format!(
                "NODE_ID is required (must be between 0 and {})",
                VF_MAX_NODE_ID
            ));
        }
        if self.node_id > VF_MAX_NODE_ID {
            return Err(format!("NODE_ID must be between 0 and {}", VF_MAX_NODE_ID));
        }
        if self.bcrypt_cost < 10 || self.bcrypt_cost > 15 {
            return Err("BCRYPT_COST must be between 10 and 15".to_string());
        }
        if self.jwt_secret.is_empty() {
            return Err("JWT_SECRET is required".to_string());
        }
        if self.port.is_empty() {
            return Err("PORT is required".to_string());
        }
        let port_str = self.port.trim_start_matches(':');
        match port_str.parse::<u16>() {
            Ok(port_num) if port_num >= 1 => {}
            _ => {
                return Err(format!(
                    "PORT must be a valid port number (1-65535), got: {}",
                    port_str
                ));
            }
        }
        if self.database_url.is_empty() {
            return Err("DATABASE_URL is required".to_string());
        }
        if self.segment_duration_ms <= 0 {
            return Err("SEGMENT_DURATION_MS must be positive".to_string());
        }
        if self.segment_size <= 0 {
            return Err("SEGMENT_SIZE must be positive".to_string());
        }
        if self.message_retention_days <= 0 {
            return Err("MESSAGE_RETENTION_DAYS must be positive".to_string());
        }
        if self.default_page_size <= 0 || self.max_page_size <= 0 {
            return Err("PAGE_SIZE must be positive".to_string());
        }
        if self.default_page_size > self.max_page_size {
            return Err("DEFAULT_PAGE_SIZE cannot exceed MAX_PAGE_SIZE".to_string());
        }
        Ok(())
    }
}

pub fn load_config() -> Config {
    let dotenv_map = load_dotenv_map();

    let jwt_secret = match dotenv_map.get("JWT_SECRET") {
        Some(v) if !v.is_empty() => v.clone(),
        _ => match env::var("JWT_SECRET") {
            Ok(v) if !v.is_empty() => v,
            _ => match prompt_interactive_jwt() {
                Ok(s) => s,
                Err(e) => {
                    eprintln!("Error: {}", e);
                    process::exit(1);
                }
            },
        },
    };

    let database_url = match dotenv_map.get("DATABASE_URL") {
        Some(v) if !v.is_empty() => v.clone(),
        _ => match env::var("DATABASE_URL") {
            Ok(v) if !v.is_empty() => v,
            _ => match prompt_interactive_database() {
                Ok(s) => s,
                Err(e) => {
                    eprintln!("Error: {}", e);
                    process::exit(1);
                }
            },
        },
    };

    let s3_url = match dotenv_map.get("S3_URL") {
        Some(v) if !v.is_empty() => v.clone(),
        _ => match env::var("S3_URL") {
            Ok(v) if !v.is_empty() => v,
            _ => match prompt_interactive_s3() {
                Ok(s) => s,
                Err(e) => {
                    eprintln!("Error: {}", e);
                    process::exit(1);
                }
            },
        },
    };

    let mut cfg = Config {
        node_id: env_int64(&dotenv_map, "NODE_ID", -1),
        port: env_string(&dotenv_map, "PORT", ":8080"),
        database_url,
        jwt_secret,
        db_max_open_conns: env_int(&dotenv_map, "DB_MAX_OPEN_CONNS", DEFAULT_DB_MAX_OPEN_CONNS),
        db_max_idle_conns: env_int(&dotenv_map, "DB_MAX_IDLE_CONNS", DEFAULT_DB_MAX_IDLE_CONNS),
        jwt_issuer: env_string(&dotenv_map, "JWT_ISSUER", DEFAULT_JWT_ISSUER),
        jwt_expires_minutes: env_int64(
            &dotenv_map,
            "JWT_EXPIRES_MINUTES",
            DEFAULT_JWT_EXPIRES_MINUTES as i64,
        ),
        bcrypt_cost: env_int(&dotenv_map, "BCRYPT_COST", DEFAULT_BCRYPT_COST),
        message_recall_window_ms: env_int64(
            &dotenv_map,
            "MESSAGE_RECALL_WINDOW_MS",
            DEFAULT_MESSAGE_RECALL_WINDOW_MS,
        ),
        epoch_time: env_int64(&dotenv_map, "EPOCH_TIME", DEFAULT_EPOCH_TIME),
        segment_duration_ms: env_int64(
            &dotenv_map,
            "ID_SEGMENT_DURATION_MS",
            DEFAULT_SEGMENT_DURATION_MS,
        ),
        segment_size: env_int64(&dotenv_map, "ID_SEGMENT_SIZE", DEFAULT_SEGMENT_SIZE),
        message_retention_days: env_int(
            &dotenv_map,
            "MESSAGE_RETENTION_DAYS",
            DEFAULT_MESSAGE_RETENTION_DAYS,
        ),
        default_page_size: env_int(&dotenv_map, "DEFAULT_PAGE_SIZE", DEFAULT_PAGE_SIZE),
        max_page_size: env_int(&dotenv_map, "MAX_PAGE_SIZE", DEFAULT_MAX_PAGE_SIZE),
        public_id_length: env_int(&dotenv_map, "PUBLIC_ID_LENGTH", DEFAULT_PUBLIC_ID_LENGTH),
        group_id_random_length: env_int(
            &dotenv_map,
            "GROUP_ID_RANDOM_LENGTH",
            DEFAULT_GROUP_ID_RANDOM_LENGTH,
        ),
        worker_table_create_interval_hours: env_int64(
            &dotenv_map,
            "WORKER_TABLE_CREATE_INTERVAL_HOURS",
            DEFAULT_WORKER_CREATE_INTERVAL as i64,
        ),
        worker_maintenance_initial_delay_minutes: env_int64(
            &dotenv_map,
            "WORKER_MAINTENANCE_INITIAL_DELAY_MINUTES",
            DEFAULT_MAINTENANCE_DELAY as i64,
        ),
        worker_maintenance_interval_hours: env_int64(
            &dotenv_map,
            "WORKER_MAINTENANCE_INTERVAL_HOURS",
            DEFAULT_MAINTENANCE_INTERVAL as i64,
        ),
        idempotency_retention_hours: env_int64(
            &dotenv_map,
            "IDEMPOTENCY_RETENTION_HOURS",
            DEFAULT_IDEMPOTENCY_RETENTION_HOURS,
        ),
        s3_url,
    };

    if !cfg.port.is_empty() && !cfg.port.starts_with(':') {
        cfg.port = format!(":{}", cfg.port);
    }

    if let Err(e) = cfg.validate() {
        eprintln!("Error: {}\n", e);
        process::exit(1);
    }

    cfg
}
