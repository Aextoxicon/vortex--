package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// vf开头的常量定义了ID结构的位分配和相关的最大值、位移等参数，ID由时间戳、节点ID和序列号组成
// IdSegment结构表示一个ID段，包含起始ID、结束ID、基础时间戳、结束时间戳、节点ID和当前ID的原子计数器
// IdGenerator是ID生成器的核心结构，包含配置、节点ID、纪元时间、锁、ID段列表、预取通道、状态存储和消息存储等字段
// NewIdGenerator函数创建一个新的ID生成器实例，Init方法初始化生成器，GenerateID方法生成一个新的ID，其他方法用于管理ID段和提取ID信息等功能
const (
	vfTimestampBits = 41 // 约69年
	vfNodeIDBits    = 5  // 支持最多32个节点
	vfSequenceBits  = 17 // 支持每个节点最多131072个序列号(以后小概率可能会压缩这个序列号然后腾出一个regionID之类的东西，但是目前没有打算提前优化)

	vfMaxTimestamp = (1 << vfTimestampBits) - 1 // 为什么-1？因为ID是从0开始的，所以最大值是2的位数次方减1，而不是2的位数次方
	vfMaxNodeID    = (1 << vfNodeIDBits) - 1    // 如果不-1，那么最大节点ID就是32，但实际上节点ID是从0开始的，所以最大节点ID应该是31
	vfMaxSequence  = (1 << vfSequenceBits) - 1  // 同理，如果不-1，那么最大序列号就是131072，但实际上序列号是从0开始的，所以最大序列号应该是131071

	vfNodeIDShift    = vfSequenceBits                // 节点ID在ID中的位移位置，序列号占17位，所以节点ID需要左移17位
	vfTimestampShift = vfSequenceBits + vfNodeIDBits // 时间戳在ID中的位移位置，序列号占17位，节点ID占5位，所以时间戳需要左移22位
)

type IdSegment struct {
	StartID int64
	EndID   int64
	BaseTs  int64
	EndTs   int64
	NodeID  int64
	current atomic.Int64 // atomic是因为可能会有多个goroutine同时访问这个字段，所以需要保证线程安全，这不是elixir不能直接无所畏惧地修改这个字段可惜了
}

func (s *IdSegment) Remaining() int {
	return int(s.EndID - s.current.Load())
}

// 其实这个东西并不是标准的snowflake，准确来说这个是针对这个后端定制的id生成器，至于为什么会变成这样
// 最开始我直接数据库自增，但是在超级垃圾的小机器上面性能太差了，后来我就想能不能自己生成ID，然后就是直接随机数，但是这样不能表示顺序
// 然后直接时间戳，但是问题是我不能保证在同一毫秒内不会有重复的ID，其次如果我改成传客户端时间戳那更不对劲了，鬼知道会不会有坏人传一些奇奇怪怪的ts
// 后来我就想能不能把时间戳和序列号结合起来，时间戳保证了ID的递增性，然后在这里面序列号纯粹是防止1ms以内多个消息重复
// 但是这俩单纯放一起又有个问题，就是如果我直接每次生成ID都去数据库更新一个全局的时间戳，那和自增有什么区别？还是频繁访问数据库
// 然后参考snowflake折腾了给简易版本，后面加上了nodeID
// 现在加上了段生成感觉还行，每次生成ID只需要访问内存，只有在段用完的时候才需要访问数据库生成新的段，这样就大大减少了数据库的访问频率，同时又保证了ID的唯一性和递增性
type IdGenerator struct {
	cfg        *Config
	nodeID     int64
	epochTime  int64
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
			slog.Error("id generator init from db failed", "error", err)
			os.Exit(1)
		}
		if err := g.fetchNewSegment(context.Background()); err != nil {
			slog.Error("id generator first segment fetch failed", "error", err)
			os.Exit(1)
		}
		close(g.initDone)
	})
}

func (g *IdGenerator) WaitInit() {
	<-g.initDone
}

// 如果数据库中没有状态，则根据当前时间和消息表中的最大ID来初始化状态
func (g *IdGenerator) initFromDB() error {
	state, err := g.idGenSt.GetFirst(context.Background())
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	if state != nil {
		g.epochTime = state.EpochTime
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

	g.epochTime = g.cfg.EpochTime
	initState := &IdGeneratorState{
		LastTs:    startTs + g.cfg.SegmentDurationMs,
		EpochTime: g.epochTime,
	}

	_, err = g.idGenSt.Insert(initState)
	return err
}

func (g *IdGenerator) GetEpochTime() int64 {
	g.WaitInit()
	return g.epochTime
}

// ID生成器的核心方法，它首先等待初始化完成，然后进入一个循环，尝试从当前的ID段中生成一个ID，如果当前段没有剩余ID了，就弹出当前段并且同步获取一个新的段，直到成功生成一个ID为止
// 不过我好像没必要讲的如此详细，go本身很简洁，函数名我就不信我起的名字不够直白（虽然说长
func (g *IdGenerator) GenerateID(ctx context.Context) (int64, error) {
	g.WaitInit()

	for {
		id, ok := g.tryGenerateFromCurrent()
		if ok {
			return id, nil
		}

		if err := g.fetchNewSegmentSync(ctx); err != nil {
			return 0, err
		}
	}
}

// tryGenerateFromCurrent 在单次锁操作内完成：peek当前段、尝试分配ID、段耗尽时pop。
// 返回 (id, true) 表示分配成功；
// 返回 (0, false) 表示当前无可用段或段已耗尽，调用者应 fetch 新段后重试。
func (g *IdGenerator) tryGenerateFromCurrent() (int64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(g.segments) == 0 {
		return 0, false
	}

	seg := g.segments[0]
	id := seg.current.Add(1)
	if id <= seg.EndID {
		// 在锁内检查是否需要预取下一个段，避免锁外检查时状态已被修改
		if int64(seg.Remaining()) <= g.cfg.SegmentSize/4 && len(g.segments) < 2 {
			select {
			case g.prefetchCh <- struct{}{}:
				go func() {
					defer func() { <-g.prefetchCh }()
					g.fetchNewSegmentForPrefetch(context.Background())
				}()
			default:
			}
		}
		return id, true
	}

	// 当前段已耗尽，pop掉它
	g.segments = g.segments[1:]
	return 0, false
}

func (g *IdGenerator) fetchNewSegmentSync(ctx context.Context) error {
	// 第一次检查：快速判断是否已有可用段，避免不必要的 DB 操作
	g.mu.Lock()
	if len(g.segments) >= 1 {
		g.mu.Unlock()
		return nil
	}
	g.mu.Unlock()

	// DB 事务期间不持有 g.mu，其他 goroutine 可以正常从当前段分配 ID
	seg, err := g.fetchNewSegmentFromDB(ctx)
	if err != nil {
		return err
	}

	// 第二次检查：另一个 goroutine 可能已经 fetch 了段并 append 了
	// 如果是，丢弃这个段（段已由 DB 持久化，不会被浪费）
	g.mu.Lock()
	if len(g.segments) == 0 {
		g.segments = append(g.segments, seg)
	}
	g.mu.Unlock()
	return nil
}

// fetchNewSegmentForPrefetch 专供预取使用：不检查 segments 长度，直接去 DB fetch。
// 预取时当前段尚未耗尽，所以 len(segments) >= 1，走 fetchNewSegmentSync 会直接返回。
// 预取成功后把新段追加到 segments 末尾，使 len(segments) 变为 2，实现"预取"效果。
func (g *IdGenerator) fetchNewSegmentForPrefetch(ctx context.Context) {
	seg, err := g.fetchNewSegmentFromDB(ctx)
	if err != nil {
		slog.Debug("id generator prefetch failed", "error", err)
		return
	}

	g.mu.Lock()
	// 只保留最多 2 个段，防止预取 goroutine 之间相互覆盖
	if len(g.segments) < 2 {
		g.segments = append(g.segments, seg)
	}
	g.mu.Unlock()
}

// fetchNewSegmentFromDB 从数据库获取新段，不持有 g.mu。
// 调用者自行决定何时将段加入 segments 列表。
func (g *IdGenerator) fetchNewSegmentFromDB(ctx context.Context) (*IdSegment, error) {
	tx, err := g.msgSt.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	state, err := g.idGenSt.GetFirstForUpdate(tx)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	var startTs int64
	if state != nil {
		startTs = state.LastTs + 1
	} else {
		startTs = time.Now().UnixMilli()
	}

	endTs := startTs + g.cfg.SegmentDurationMs

	startID := (startTs << vfTimestampShift) | (g.nodeID << vfNodeIDShift)             // 序列号从0开始
	endID := (endTs << vfTimestampShift) | (g.nodeID << vfNodeIDShift) | vfMaxSequence // 时间戳部分是endTs，节点ID部分是g.nodeID，序列号部分是全1，这样就表示这个段的最后一个ID

	// 如果数据库中已经有状态了，那么就以状态中的LastTs为基础生成新的段，新的段的起始时间戳就是LastTs+1，这样就保证了新段的ID不会和旧段的ID重叠
	// 如果数据库中没有状态了，那么就以当前时间为基础生成新的段，新的段的起始时间戳就是当前时间，这样就保证了新段的ID不会和旧段的ID重叠（因为旧段的ID是根据之前的状态生成的，所以它们的时间戳部分一定是小于当前时间的）
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
		return nil, fmt.Errorf("persist state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	tx = nil

	return seg, nil
}

func (g *IdGenerator) fetchNewSegment(ctx context.Context) error {
	return g.fetchNewSegmentSync(ctx)
}

func (g *IdGenerator) GetNodeID() int64 {
	return g.nodeID
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
