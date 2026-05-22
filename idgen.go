package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	vfTimestampBits = 41
	vfNodeIDBits    = 5
	vfSequenceBits  = 17

	vfMaxTimestamp = (1 << vfTimestampBits) - 1
	vfMaxNodeID    = (1 << vfNodeIDBits) - 1
	vfMaxSequence  = (1 << vfSequenceBits) - 1

	vfNodeIDShift    = vfSequenceBits
	vfTimestampShift = vfSequenceBits + vfNodeIDBits
)

type IdSegment struct {
	StartID int64
	EndID   int64
	BaseTs  int64
	EndTs   int64
	NodeID  int64
	current atomic.Int64
}

func (s *IdSegment) Remaining() int {
	return int(s.EndID - s.current.Load())
}

type IdGenerator struct {
	cfg        *Config
	nodeID     int64
	mu         sync.Mutex
	segments   []*IdSegment
	prefetchCh chan struct{}
	idGenSt    *IdGeneratorStateStore
	msgSt      *MessageStore
	initOnce   sync.Once
	initDone   chan struct{}
}

func NewIdGenerator(cfg *Config, idGenSt *IdGeneratorStateStore, msgSt *MessageStore, nodeID int64) *IdGenerator {
	return &IdGenerator{
		cfg:        cfg,
		nodeID:     nodeID,
		segments:   make([]*IdSegment, 0, 2),
		prefetchCh: make(chan struct{}, 1),
		idGenSt:    idGenSt,
		msgSt:      msgSt,
		initDone:   make(chan struct{}),
	}
}

func (g *IdGenerator) Init() {
	g.initOnce.Do(func() {
		if err := g.initFromDB(); err != nil {
			slog.Warn("id generator init warning", "error", err)
		}
		if err := g.fetchNewSegment(context.Background()); err != nil {
			slog.Error("id generator first segment fetch failed", "error", err)
		}
		close(g.initDone)
	})
}

func (g *IdGenerator) WaitInit() {
	<-g.initDone
}

func (g *IdGenerator) initFromDB() error {
	state, err := g.idGenSt.GetFirst(context.Background())
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	if state != nil {
		return nil
	}

	now := time.Now().UnixMilli()
	maxID, err := g.msgSt.GetMaxMessageID()
	if err != nil {
		return fmt.Errorf("load max id: %w", err)
	}

	startTs := now
	if maxID > 0 {
		existingTs := maxID >> vfTimestampShift
		if existingTs+1 > startTs {
			startTs = existingTs + 1
		}
	}

	initState := &IdGeneratorState{
		LastTs: startTs + g.cfg.SegmentDurationMs,
	}

	_, err = g.idGenSt.Insert(initState)
	return err
}

func (g *IdGenerator) GenerateID(ctx context.Context) (int64, error) {
	g.WaitInit()

	for {
		seg := g.peekSegment()
		if seg == nil {
			if err := g.fetchNewSegmentSync(ctx); err != nil {
				return 0, err
			}
			continue
		}

		id := seg.current.Add(1)
		if id <= seg.EndID {
			g.tryPrefetch(seg)
			return id, nil
		}

		g.popSegment()
		if err := g.fetchNewSegmentSync(ctx); err != nil {
			return 0, err
		}
	}
}

func (g *IdGenerator) peekSegment() *IdSegment {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.segments) == 0 {
		return nil
	}
	return g.segments[0]
}

func (g *IdGenerator) popSegment() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.segments) > 0 {
		g.segments = g.segments[1:]
	}
}

func (g *IdGenerator) tryPrefetch(current *IdSegment) {
	g.mu.Lock()
	if int64(current.Remaining()) > g.cfg.SegmentSize/4 || len(g.segments) >= 2 {
		g.mu.Unlock()
		return
	}
	g.mu.Unlock()

	select {
	case g.prefetchCh <- struct{}{}:
		go func() {
			defer func() { <-g.prefetchCh }()
			_ = g.fetchNewSegmentSync(context.Background())
		}()
	default:
	}
}

func (g *IdGenerator) fetchNewSegmentSync(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.segments) >= 1 {
		return nil
	}

	return g.fetchNewSegmentLocked(ctx)
}

func (g *IdGenerator) fetchNewSegmentLocked(ctx context.Context) error {
	tx, err := g.msgSt.DB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	state, err := g.idGenSt.GetFirstForUpdate(tx)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	var startTs int64
	if state != nil {
		startTs = state.LastTs + 1
	} else {
		startTs = time.Now().UnixMilli()
	}

	endTs := startTs + g.cfg.SegmentDurationMs

	startID := (startTs << vfTimestampShift) | (g.nodeID << vfNodeIDShift)
	endID := (endTs << vfTimestampShift) | (g.nodeID << vfNodeIDShift) | vfMaxSequence

	seg := &IdSegment{
		StartID: startID,
		EndID:   endID,
		BaseTs:  startTs,
		EndTs:   endTs,
		NodeID:  g.nodeID,
	}
	seg.current.Store(startID - 1)

	if state != nil {
		state.LastTs = endTs
		_, err = g.idGenSt.UpdateWithTx(tx, state)
	} else {
		newState := &IdGeneratorState{LastTs: endTs}
		_, err = g.idGenSt.InsertWithTx(tx, newState)
	}
	if err != nil {
		return fmt.Errorf("persist state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	tx = nil

	g.segments = append(g.segments, seg)

	return nil
}

func (g *IdGenerator) fetchNewSegment(ctx context.Context) error {
	return g.fetchNewSegmentSync(ctx)
}

func (g *IdGenerator) GetNodeID() int64 {
	return g.nodeID
}

func (g *IdGenerator) CalculateNextID(currentTs, currentSeq int64) (id, newTs, newSeq int64, err error) {
	if g.nodeID < 0 || g.nodeID > vfMaxNodeID {
		return 0, 0, 0, fmt.Errorf("node ID must be between 0 and %d", vfMaxNodeID)
	}

	newTs = currentTs
	newSeq = currentSeq

	if currentSeq < vfMaxSequence {
		newSeq = currentSeq + 1
	} else {
		newTs = currentTs + 1
		newSeq = 0
	}

	if newTs > vfMaxTimestamp {
		return 0, 0, 0, fmt.Errorf("timestamp overflow")
	}

	id = (newTs << vfTimestampShift) | (g.nodeID << vfNodeIDShift) | newSeq
	return
}

func (g *IdGenerator) ExtractTimestampFromMsgID(msgID int64) int64 {
	return msgID >> vfTimestampShift
}

func (g *IdGenerator) ExtractNodeIDFromMsgID(msgID int64) int64 {
	return (msgID >> vfNodeIDShift) & vfMaxNodeID
}

func (g *IdGenerator) ExtractSequenceFromMsgID(msgID int64) int64 {
	return msgID & vfMaxSequence
}
