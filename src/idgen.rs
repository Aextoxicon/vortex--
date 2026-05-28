use sqlx::PgPool;
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::{Arc, Mutex};
use tokio::sync::Semaphore;

const VF_TIMESTAMP_BITS: i64 = 41;
const VF_NODE_ID_BITS: i64 = 5;
const VF_SEQUENCE_BITS: i64 = 17;

const VF_MAX_TIMESTAMP: i64 = (1 << VF_TIMESTAMP_BITS) - 1;
const VF_MAX_NODE_ID: i64 = (1 << VF_NODE_ID_BITS) - 1;
const VF_MAX_SEQUENCE: i64 = (1 << VF_SEQUENCE_BITS) - 1;

const VF_NODE_ID_SHIFT: i64 = VF_SEQUENCE_BITS;
const VF_TIMESTAMP_SHIFT: i64 = VF_SEQUENCE_BITS + VF_NODE_ID_BITS;

#[derive(Debug, Clone)]
pub struct IdSegment {
    pub start_id: i64,
    pub end_id: i64,
    pub base_ts: i64,
    pub end_ts: i64,
    pub node_id: i64,
    current: Arc<AtomicI64>,
}

impl IdSegment {
    pub fn new(start_id: i64, end_id: i64, base_ts: i64, end_ts: i64, node_id: i64) -> Self {
        Self {
            start_id,
            end_id,
            base_ts,
            end_ts,
            node_id,
            current: Arc::new(AtomicI64::new(start_id - 1)),
        }
    }

    pub fn remaining(&self) -> i64 {
        self.end_id - self.current.load(Ordering::Relaxed)
    }

    pub fn next_id(&self) -> i64 {
        self.current.fetch_add(1, Ordering::Relaxed) + 1
    }
}

pub struct IdGenerator {
    pool: PgPool,
    node_id: i64,
    epoch_time: Arc<AtomicI64>,
    segments: Arc<Mutex<Vec<IdSegment>>>,
    prefetch_semaphore: Arc<Semaphore>,
    id_gen_state_store: crate::store::IdGeneratorStateStore,
    message_store: crate::store::MessageStore,
    init_done: Arc<tokio::sync::Notify>,
}

impl Clone for IdGenerator {
    fn clone(&self) -> Self {
        Self {
            pool: self.pool.clone(),
            node_id: self.node_id,
            epoch_time: self.epoch_time.clone(),
            segments: self.segments.clone(),
            prefetch_semaphore: self.prefetch_semaphore.clone(),
            id_gen_state_store: self.id_gen_state_store.clone(),
            message_store: self.message_store.clone(),
            init_done: self.init_done.clone(),
        }
    }
}

impl IdGenerator {
    pub fn new(
        pool: PgPool,
        id_gen_state_store: crate::store::IdGeneratorStateStore,
        message_store: crate::store::MessageStore,
        node_id: i64,
        epoch_time: i64,
    ) -> Self {
        Self {
            pool,
            node_id,
            epoch_time: Arc::new(AtomicI64::new(epoch_time)),
            segments: Arc::new(Mutex::new(Vec::with_capacity(2))),
            prefetch_semaphore: Arc::new(Semaphore::new(1)),
            id_gen_state_store,
            message_store,
            init_done: Arc::new(tokio::sync::Notify::new()),
        }
    }

    pub async fn init(&self) {
        let pool = self.pool.clone();
        let node_id = self.node_id;
        let epoch_time = self.epoch_time.clone();
        let segments = self.segments.clone();
        let id_gen_state_store = self.id_gen_state_store.clone();
        let message_store = self.message_store.clone();
        let init_done = self.init_done.clone();
        let cfg_epoch_time = epoch_time.load(Ordering::Relaxed);

        let result = async {
            let state = id_gen_state_store
                .get_first()
                .await
                .map_err(|e| format!("load state: {}", e))?;

            let new_epoch_time = if let Some(s) = state {
                s.epoch_time
            } else {
                let now = std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .expect("system time should always be valid")
                    .as_millis() as i64;

                let max_id = message_store.get_max_message_id().await.unwrap_or(0);

                let mut start_ts = now;
                if max_id > 0 {
                    let existing_ts = max_id >> VF_TIMESTAMP_SHIFT;
                    if existing_ts + 1 > start_ts {
                        start_ts = existing_ts + 1;
                    }
                }

                let init_state = crate::store::IdGeneratorState {
                    id: 0,
                    last_ts: start_ts + 10_000,
                    epoch_time: cfg_epoch_time,
                };

                id_gen_state_store
                    .insert(&init_state)
                    .await
                    .map_err(|e| format!("insert state: {}", e))?;

                cfg_epoch_time
            };

            epoch_time.store(new_epoch_time, Ordering::Relaxed);

            Ok::<_, String>(())
        }
        .await;

        if let Err(e) = result {
            tracing::error!("id generator init from db failed: {}", e);
            std::process::exit(1);
        }

        let result =
            Self::fetch_new_segment_locked(&pool, &id_gen_state_store, node_id, &segments).await;

        if let Err(e) = result {
            tracing::error!("id generator first segment fetch failed: {}", e);
            std::process::exit(1);
        }

        init_done.notify_waiters();
    }

    pub async fn wait_init(&self) {
        self.init_done.notified().await;
    }

    pub async fn generate_id(&self) -> Result<i64, String> {
        self.wait_init().await;

        loop {
            let seg = {
                let segments = self.segments.lock().expect("segments mutex poisoned");
                segments.first().cloned()
            };

            if seg.is_none() {
                Self::fetch_new_segment_locked(
                    &self.pool,
                    &self.id_gen_state_store,
                    self.node_id,
                    &self.segments,
                )
                .await?;
                continue;
            }

            let seg = seg.unwrap();
            let id = seg.next_id();

            if id <= seg.end_id {
                self.try_prefetch(&seg);
                return Ok(id);
            }

            {
                let mut segments = self.segments.lock().expect("segments mutex poisoned");
                if !segments.is_empty() {
                    segments.remove(0);
                }
            }

            Self::fetch_new_segment_locked(
                &self.pool,
                &self.id_gen_state_store,
                self.node_id,
                &self.segments,
            )
            .await?;
        }
    }

    fn try_prefetch(&self, current: &IdSegment) {
        let should_prefetch = {
            let segments = self.segments.lock().expect("segments mutex poisoned");
            current.remaining() > 32768 || segments.len() >= 2
        };

        if !should_prefetch {
            return;
        }

        let pool = self.pool.clone();
        let id_gen_state_store = self.id_gen_state_store.clone();
        let node_id = self.node_id;
        let segments = self.segments.clone();

        if self.prefetch_semaphore.try_acquire().is_ok() {
            tokio::spawn(async move {
                let _ =
                    Self::fetch_new_segment_locked(&pool, &id_gen_state_store, node_id, &segments)
                        .await;
            });
        }
    }

    async fn fetch_new_segment_locked(
        pool: &PgPool,
        id_gen_state_store: &crate::store::IdGeneratorStateStore,
        node_id: i64,
        segments: &Arc<Mutex<Vec<IdSegment>>>,
    ) -> Result<(), String> {
        {
            let segs = segments.lock().expect("segments mutex poisoned");
            if !segs.is_empty() {
                return Ok(());
            }
        }

        let mut tx = pool.begin().await.map_err(|e| format!("begin tx: {}", e))?;

        let state = id_gen_state_store
            .get_first_for_update(&mut tx)
            .await
            .map_err(|e| format!("load state: {}", e))?;

        let start_ts = if let Some(ref s) = state {
            s.last_ts + 1
        } else {
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .expect("system time should always be valid")
                .as_millis() as i64
        };

        let end_ts = start_ts + 10_000;

        let start_id = (start_ts << VF_TIMESTAMP_SHIFT) | (node_id << VF_NODE_ID_SHIFT);
        let end_id =
            (end_ts << VF_TIMESTAMP_SHIFT) | (node_id << VF_NODE_ID_SHIFT) | VF_MAX_SEQUENCE;

        let seg = IdSegment::new(start_id, end_id, start_ts, end_ts, node_id);

        if let Some(mut s) = state {
            s.last_ts = end_ts;
            id_gen_state_store
                .update_with_tx(&mut tx, &s)
                .await
                .map_err(|e| format!("persist state: {}", e))?;
        } else {
            let new_state = crate::store::IdGeneratorState {
                id: 0,
                last_ts: end_ts,
                epoch_time: 0,
            };
            id_gen_state_store
                .insert_with_tx(&mut tx, &new_state)
                .await
                .map_err(|e| format!("persist state: {}", e))?;
        }

        tx.commit().await.map_err(|e| format!("commit tx: {}", e))?;

        {
            let mut segs = segments.lock().expect("segments mutex poisoned");
            segs.push(seg);
        }

        Ok(())
    }

    pub fn get_node_id(&self) -> i64 {
        self.node_id
    }

    pub fn get_epoch_time(&self) -> i64 {
        self.epoch_time.load(Ordering::Relaxed)
    }

    pub fn calculate_next_id(
        &self,
        current_ts: i64,
        current_seq: i64,
    ) -> Result<(i64, i64, i64), String> {
        if self.node_id < 0 || self.node_id > VF_MAX_NODE_ID {
            return Err(format!("node ID must be between 0 and {}", VF_MAX_NODE_ID));
        }

        let mut new_ts = current_ts;
        let new_seq = if current_seq < VF_MAX_SEQUENCE {
            current_seq + 1
        } else {
            new_ts = current_ts + 1;
            0
        };

        if new_ts > VF_MAX_TIMESTAMP {
            return Err("timestamp overflow".to_string());
        }

        let id = (new_ts << VF_TIMESTAMP_SHIFT) | (self.node_id << VF_NODE_ID_SHIFT) | new_seq;
        Ok((id, new_ts, new_seq))
    }

    pub fn extract_timestamp_from_msg_id(&self, msg_id: i64) -> i64 {
        msg_id >> VF_TIMESTAMP_SHIFT
    }

    pub fn extract_node_id_from_msg_id(&self, msg_id: i64) -> i64 {
        (msg_id >> VF_NODE_ID_SHIFT) & VF_MAX_NODE_ID
    }

    pub fn extract_sequence_from_msg_id(&self, msg_id: i64) -> i64 {
        msg_id & VF_MAX_SEQUENCE
    }
}
