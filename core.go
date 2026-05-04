package main

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ==================== IdGenerator (with ViolenceFlake integrated) ====================

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
	nodeID    int64
	mu        sync.Mutex
	segMu     sync.Mutex
	segments  []*IdSegment
	prefixCh  chan struct{}
	idGenSt   *IdGeneratorStateStore
	msgSt     *MessageStore
	initOnce  sync.Once
	initDone  chan struct{}
}

func NewIdGenerator(idGenSt *IdGeneratorStateStore, msgSt *MessageStore, nodeID int64) *IdGenerator {
	return &IdGenerator{
		nodeID:   nodeID,
		segments: make([]*IdSegment, 0, 2),
		prefixCh: make(chan struct{}, 1),
		idGenSt:  idGenSt,
		msgSt:    msgSt,
		initDone: make(chan struct{}),
	}
}

func (g *IdGenerator) Init() {
	g.initOnce.Do(func() {
		if err := g.initFromDB(); err != nil {
			log.Printf("id generator init warning: %v", err)
		}
		if err := g.fetchNewSegment(); err != nil {
			log.Printf("id generator first segment fetch failed: %v", err)
		}
		close(g.initDone)
	})
}

func (g *IdGenerator) WaitInit() {
	<-g.initDone
}

func (g *IdGenerator) initFromDB() error {
	state, err := g.idGenSt.GetFirst()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	if state != nil {
		return nil
	}

	now := time.Now().UnixMilli()
	maxIDs, err := g.msgSt.GetMaxMessageIDsFromRecentTables(7)
	if err != nil {
		return fmt.Errorf("load max ids: %w", err)
	}

	startTs := now
	if len(maxIDs) > 0 {
		maxID := maxIDs[0]
		for _, id := range maxIDs[1:] {
			if id > maxID {
				maxID = id
			}
		}
		existingTs := maxID >> vfTimestampShift
		if existingTs+1 > startTs {
			startTs = existingTs + 1
		}
	}

	initState := &IdGeneratorState{
		LastTs:  startTs + AppCfg.IdGenerator.SegmentDurationMs,
		LastSeq: 0,
	}

	_, err = g.idGenSt.Insert(initState)
	return err
}

func (g *IdGenerator) GenerateID() (int64, error) {
	g.WaitInit()

	for {
		seg := g.peekSegment()
		if seg == nil {
			if err := g.fetchNewSegment(); err != nil {
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
	if int64(current.Remaining()) > AppCfg.IdGenerator.SegmentSize/4 || len(g.segments) >= 2 {
		return
	}
	select {
	case g.prefixCh <- struct{}{}:
		go func() {
			if err := g.fetchNewSegment(); err != nil {
				log.Printf("prefetch segment failed: %v", err)
			}
			<-g.prefixCh
		}()
	default:
	}
}

func (g *IdGenerator) fetchNewSegment() error {
	g.mu.Lock()
	if len(g.segments) >= 2 {
		g.mu.Unlock()
		return nil
	}
	g.mu.Unlock()

	g.segMu.Lock()
	defer g.segMu.Unlock()

	state, err := g.idGenSt.GetFirst()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	now := time.Now().UnixMilli()

	var startTs int64
	if state != nil {
		startTs = max(now, state.LastTs)
	} else {
		startTs = now
	}

	endTs := startTs + AppCfg.IdGenerator.SegmentDurationMs

	startID := (startTs << vfTimestampShift) | (g.nodeID << vfNodeIDShift)
	endID := (endTs << vfTimestampShift) | (g.nodeID << vfNodeIDShift) | (1<<vfSequenceBits - 1)

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
		state.LastSeq = 0
		_, err = g.idGenSt.Update(state)
	} else {
		newState := &IdGeneratorState{LastTs: endTs, LastSeq: 0}
		_, err = g.idGenSt.Insert(newState)
	}
	if err != nil {
		return fmt.Errorf("persist state: %w", err)
	}

	g.mu.Lock()
	g.segments = append(g.segments, seg)
	g.mu.Unlock()

	return nil
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

// ==================== RateLimiter ====================

type RateLimiter struct {
	mu    sync.Mutex
	cache map[string]time.Time
	ttl   time.Duration
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		cache: make(map[string]time.Time),
		ttl:   2 * time.Second,
	}
}

func (r *RateLimiter) AllowRequest(publicID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := "rate_limit:" + publicID
	last, ok := r.cache[key]
	now := time.Now()

	if !ok || now.After(last) {
		r.cache[key] = now.Add(r.ttl)
		return true
	}
	return false
}

// ==================== JwtService ====================

type JwtClaims struct {
	UserID      int64  `json:"sub"`
	PublicID    string `json:"public_id"`
	Username    string `json:"username"`
	DeviceToken string `json:"device_token,omitempty"`
	jwt.RegisteredClaims
}

type JwtService struct {
	secret           []byte
	issuer           string
	expiresInMinutes int
}

func NewJwtService(secret, issuer string) *JwtService {
	expires := 10080
	return &JwtService{
		secret:           []byte(secret),
		issuer:           issuer,
		expiresInMinutes: expires,
	}
}

func (j *JwtService) GenerateToken(user *User, deviceToken string) (string, error) {
	now := time.Now()
	claims := JwtClaims{
		UserID:   user.ID,
		PublicID: user.PublicID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(j.expiresInMinutes) * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    j.issuer,
			Audience:  []string{j.issuer},
			ID:        fmt.Sprintf("%d", now.UnixNano()),
		},
	}

	if deviceToken != "" {
		claims.DeviceToken = deviceToken
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secret)
}

func (j *JwtService) ValidateToken(tokenStr string) (*JwtClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &JwtClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*JwtClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}
