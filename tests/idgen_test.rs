mod test_utils;

use test_utils::TestFixture;
use vortex--::idgen::IdGenerator;
use vortex--::store::{IdGeneratorStateStore, MessageStore};
use vortex--::config::Config;
use std::sync::Arc;
use std::collections::HashSet;

async fn setup_id_generator(node_id: u64) -> (TestFixture, IdGenerator) {
    let fixture = TestFixture::new().await;
    
    let cfg = Config {
        node_id,
        segment_duration_ms: 10000,
        segment_size: 128 * 1024,
        epoch_time: chrono::Utc::now().timestamp_millis(),
    };
    
    let id_gen_state_store = IdGeneratorStateStore::new(fixture.pool.clone());
    let msg_store = MessageStore::new(fixture.pool.clone());
    
    let mut gen = IdGenerator::new(&cfg, id_gen_state_store, msg_store, node_id);
    gen.init().await.expect("failed to init generator");
    
    (fixture, gen)
}

#[tokio::test]
async fn test_id_generator_generate_id() {
    let (_fixture, mut gen) = setup_id_generator(1).await;
    
    let mut ids = Vec::new();
    for _ in 0..100 {
        let id = gen.generate_id().await.expect("failed to generate ID");
        ids.push(id);
    }
    
    for i in 1..ids.len() {
        assert!(ids[i] > ids[i-1], "ID not increasing");
    }
}

#[tokio::test]
async fn test_id_generator_concurrent() {
    let (_fixture, gen) = setup_id_generator(1).await;
    let gen = Arc::new(std::sync::Mutex::new(gen));
    
    const THREADS: usize = 50;
    const IDS_PER_THREAD: usize = 100;
    
    let mut handles = vec![];
    
    for _ in 0..THREADS {
        let gen = gen.clone();
        let handle = tokio::spawn(async move {
            let mut ids = Vec::new();
            for _ in 0..IDS_PER_THREAD {
                let id = gen.lock().unwrap().generate_id().await.expect("failed to generate ID");
                ids.push(id);
            }
            ids
        });
        handles.push(handle);
    }
    
    let mut all_ids = HashSet::new();
    for handle in handles {
        let ids = handle.await.unwrap();
        for id in ids {
            assert!(!all_ids.contains(&id), "duplicate ID detected: {}", id);
            all_ids.insert(id);
        }
    }
    
    assert_eq!(all_ids.len(), THREADS * IDS_PER_THREAD);
}

#[tokio::test]
async fn test_id_generator_timestamp_extraction() {
    let (_fixture, mut gen) = setup_id_generator(1).await;
    
    let id = gen.generate_id().await.expect("failed to generate ID");
    
    let ts = gen.extract_timestamp_from_msg_id(id);
    assert!(ts > 0, "expected positive timestamp");
    
    let node_id = gen.extract_node_id_from_msg_id(id);
    assert_eq!(node_id, 1);
    
    let seq = gen.extract_sequence_from_msg_id(id);
    assert!(seq >= 0);
}

#[tokio::test]
async fn test_id_generator_node_id() {
    let (_fixture, gen) = setup_id_generator(5).await;
    
    assert_eq!(gen.get_node_id(), 5);
}
