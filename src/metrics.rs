use axum::response::IntoResponse;
use axum::Json;
use serde_json::json;

pub async fn metrics() -> impl IntoResponse {
    let pid = std::process::id();

    let mem_info = get_memory_info();
    let thread_count = get_thread_count();

    Json(json!({
        "pid": pid,
        "threads": thread_count,
        "memory": mem_info,
    }))
}

pub fn print_startup_info() {
    let cpu_count = std::thread::available_parallelism()
        .map(|n| n.get())
        .unwrap_or(1);

    println!("=== Vortex 启动信息 ===");
    println!("CPU 核心: {}", cpu_count);
    println!("Rust 版本: {}", rustc_version_runtime::version());
    println!();
    println!("建议配置:");

    if cpu_count <= 2 {
        println!("  - BCRYPT_COST=8 (当前 CPU 核心较少)");
        println!("  - DB_MAX_OPEN_CONNS=20");
        println!("  - DB_MAX_IDLE_CONNS=10");
    } else if cpu_count <= 4 {
        println!("  - BCRYPT_COST=9");
        println!("  - DB_MAX_OPEN_CONNS=30");
        println!("  - DB_MAX_IDLE_CONNS=15");
    } else {
        println!("  - BCRYPT_COST=10");
        println!("  - DB_MAX_OPEN_CONNS=50");
        println!("  - DB_MAX_IDLE_CONNS=20");
    }
    println!();
}

#[derive(Debug)]
struct MemoryInfo {
    resident_set_size_kb: u64,
    virtual_memory_size_kb: u64,
}

fn get_memory_info() -> serde_json::Value {
    #[cfg(target_os = "linux")]
    {
        if let Ok(status) = std::fs::read_to_string("/proc/self/status") {
            let mut rss = 0;
            let mut vm = 0;
            for line in status.lines() {
                if let Some(val) = line.strip_prefix("VmRSS:") {
                    if let Ok(v) = val.trim().trim_end_matches("kB").parse::<u64>() {
                        rss = v;
                    }
                }
                if let Some(val) = line.strip_prefix("VmSize:") {
                    if let Ok(v) = val.trim().trim_end_matches("kB").parse::<u64>() {
                        vm = v;
                    }
                }
            }
            return json!({
                "rss_mb": rss / 1024,
                "vm_mb": vm / 1024,
            });
        }
    }

    json!({
        "rss_mb": 0,
        "vm_mb": 0,
    })
}

fn get_thread_count() -> usize {
    #[cfg(target_os = "linux")]
    {
        if let Ok(status) = std::fs::read_to_string("/proc/self/status") {
            for line in status.lines() {
                if let Some(val) = line.strip_prefix("Threads:") {
                    if let Ok(v) = val.trim().parse::<usize>() {
                        return v;
                    }
                }
            }
        }
    }

    0
}
