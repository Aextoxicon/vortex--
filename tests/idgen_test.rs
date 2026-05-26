mod test_utils;

use std::collections::HashSet;
use std::sync::Arc;
use test_utils::TestFixture;
use vortex__::idgen::IdGenerator;
use vortex__::store::{IdGeneratorStateStore, MessageStore, Store};

async fn setup_id_generator(node_id: i64) -> (TestFixture, IdGenerator) {
    let fixture = TestFixture::new().await;

    let epoch_time = chrono::Utc::now().timestamp_millis();
    let store = Store::new(fixture.pool.clone(), epoch_time);

    let id_gen_state_store = IdGeneratorStateStore::new(store.clone());
    let msg_store = MessageStore::new(store.clone());

    let id_gen = IdGenerator::new(
        fixture.pool.clone(),
        id_gen_state_store,
        msg_store,
        node_id,
        epoch_time,
    );
    id_gen.init().await;

    (fixture, id_gen)
}

#[tokio::test]
async fn test_id_generator_generate_id() {
    let (_fixture, id_gen) = setup_id_generator(1).await;

    let mut ids = Vec::new();
    for _ in 0..100 {
        let id = id_gen.generate_id().await.expect("failed to generate ID");
        ids.push(id);
    }

    for i in 1..ids.len() {
        assert!(ids[i] > ids[i - 1], "ID not increasing");
    }
}

#[tokio::test]
async fn test_id_generator_concurrent() {
    let (_fixture, id_gen) = setup_id_generator(1).await;
    let id_gen = Arc::new(id_gen);

    const THREADS: usize = 50;
    const IDS_PER_THREAD: usize = 100;

    let mut handles = vec![];

    for _ in 0..THREADS {
        let id_gen = id_gen.clone();
        let handle = tokio::spawn(async move {
            let mut ids = Vec::new();
            for _ in 0..IDS_PER_THREAD {
                let id = id_gen.generate_id().await.expect("failed to generate ID");
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
    let (_fixture, id_gen) = setup_id_generator(1).await;

    let id = id_gen.generate_id().await.expect("failed to generate ID");

    let ts = id_gen.extract_timestamp_from_msg_id(id);
    assert!(ts > 0, "expected positive timestamp");

    let extracted_node_id = id_gen.extract_node_id_from_msg_id(id);
    assert_eq!(extracted_node_id, 1);

    let seq = id_gen.extract_sequence_from_msg_id(id);
    assert!(seq >= 0);
}

#[tokio::test]
async fn test_id_generator_node_id() {
    let (_fixture, id_gen) = setup_id_generator(5).await;

    assert_eq!(id_gen.get_node_id(), 5);
}
