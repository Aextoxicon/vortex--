package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

// 和上面的handler测试一样，这个东西主要是测试ID生成器的功能，确保它能正确生成唯一且递增的ID，并且能够正确提取时间戳、节点ID和序列号等信息
func TestIDGenerator_GenerateID(t *testing.T) {
	store, _, _ := setupTestStore(t)

	idGenSt := &IdGeneratorStateStore{Store: store}
	msgSt := &MessageStore{Store: store}

	cfg := &Config{
		NodeID:            1,
		SegmentDurationMs: 10000,
		SegmentSize:       128 * 1024,
		EpochTime:         time.Now().UnixMilli(),
	}

	gen := NewIdGenerator(cfg, idGenSt, msgSt, cfg.NodeID)
	gen.Init()

	// 生成100个ID并检查它们是否递增
	ids := make([]int64, 100)
	for i := 0; i < 100; i++ {
		id, err := gen.GenerateID(context.Background())
		if err != nil {
			t.Fatalf("failed to generate ID: %v", err)
		}
		ids[i] = id
	}

	// 检查ID是否递增
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("ID not increasing: ids[%d]=%d <= ids[%d]=%d",
				i, ids[i], i-1, ids[i-1])
		}
	}
}

func TestIDGenerator_Concurrent(t *testing.T) {
	store, _, _ := setupTestStore(t)

	idGenSt := &IdGeneratorStateStore{Store: store}
	msgSt := &MessageStore{Store: store}

	cfg := &Config{
		NodeID:            1,
		SegmentDurationMs: 10000,
		SegmentSize:       128 * 1024,
		EpochTime:         time.Now().UnixMilli(),
	}

	gen := NewIdGenerator(cfg, idGenSt, msgSt, cfg.NodeID)
	gen.Init()

	const goroutines = 50
	const idsPerGoroutine = 100

	ids := make(chan int64, goroutines*idsPerGoroutine)
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				id, err := gen.GenerateID(context.Background())
				if err != nil {
					t.Errorf("failed to generate ID: %v", err)
					return
				}
				ids <- id
			}
		}()
	}

	wg.Wait()
	close(ids)

	// Check for duplicates
	seen := make(map[int64]bool)
	count := 0
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate ID detected: %d", id)
		}
		seen[id] = true
		count++
	}

	t.Logf("generated %d unique IDs", count)
	if count != goroutines*idsPerGoroutine {
		t.Errorf("expected %d IDs, got %d", goroutines*idsPerGoroutine, count)
	}
}

func TestIDGenerator_TimestampExtraction(t *testing.T) {
	store, _, _ := setupTestStore(t)

	idGenSt := &IdGeneratorStateStore{Store: store}
	msgSt := &MessageStore{Store: store}

	cfg := &Config{
		NodeID:            1,
		SegmentDurationMs: 10000,
		SegmentSize:       128 * 1024,
		EpochTime:         time.Now().UnixMilli(),
	}

	gen := NewIdGenerator(cfg, idGenSt, msgSt, cfg.NodeID)
	gen.Init()

	id, err := gen.GenerateID(context.Background())
	if err != nil {
		t.Fatalf("failed to generate ID: %v", err)
	}

	// 检查时间戳是否可以提取并且是合理的
	ts := gen.ExtractTimestampFromMsgID(id)
	if ts <= 0 {
		t.Errorf("expected positive timestamp, got %d", ts)
	}

	// 检查nodeID是否可以提取并且正确
	nodeID := gen.ExtractNodeIDFromMsgID(id)
	if nodeID != cfg.NodeID {
		t.Errorf("expected node ID %d, got %d", cfg.NodeID, nodeID)
	}

	// 检查序列号是否可以提取并且是非负的
	seq := gen.ExtractSequenceFromMsgID(id)
	if seq < 0 {
		t.Errorf("expected non-negative sequence, got %d", seq)
	}
}

func TestIDGenerator_NodeID(t *testing.T) {
	store, _, _ := setupTestStore(t)

	idGenSt := &IdGeneratorStateStore{Store: store}
	msgSt := &MessageStore{Store: store}

	cfg := &Config{
		NodeID:            5,
		SegmentDurationMs: 10000,
		SegmentSize:       128 * 1024,
		EpochTime:         time.Now().UnixMilli(),
	}

	gen := NewIdGenerator(cfg, idGenSt, msgSt, cfg.NodeID)
	gen.Init()

	if gen.GetNodeID() != cfg.NodeID {
		t.Errorf("expected node ID %d, got %d", cfg.NodeID, gen.GetNodeID())
	}
}
