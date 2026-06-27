package main

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	mathrand "math/rand"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ==================== Middleware ====================

func timeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// ==================== NanoId ====================

func goSafe(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panic recovered", "panic", r)
			}
		}()
		fn()
	}()
}

const nanoIDAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

func GenerateNanoID(size int) string {
	if size <= 0 {
		size = 21
	}
	bytes := make([]byte, size)
	if _, err := crand.Read(bytes); err != nil {
		for i := range bytes {
			bytes[i] = nanoIDAlphabet[mathrand.Intn(len(nanoIDAlphabet))]
		}
	}
	for i, b := range bytes {
		bytes[i] = nanoIDAlphabet[int(b)%len(nanoIDAlphabet)]
	}
	return string(bytes)
}

// ==================== Handler ====================

type Handler struct {
	svc *Service
	jwt *JwtService
	cfg *Config
}

func NewHandler(svc *Service, jwt *JwtService, cfg *Config) *Handler {
	return &Handler{svc: svc, jwt: jwt, cfg: cfg}
}

// HealthCheck 健康检查
// @Summary      健康检查
// @Description  返回服务运行状态
// @Tags         health
// @Success      200 {object} map[string]interface{} "服务正常"
// @Router       /health [get]
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"node_id":   h.cfg.NodeID,
		"timestamp": time.Now().UnixMilli(),
	})
}

// ReadinessCheck 就绪检查
// @Summary      就绪检查
// @Description  检查服务是否就绪（数据库连接是否正常）
// @Tags         health
// @Success      200 {object} map[string]interface{} "服务就绪"
// @Failure      503 {object} ErrorResponse          "服务未就绪"
// @Router       /ready [get]
func (h *Handler) ReadinessCheck(c *gin.Context) {
	if err := h.svc.msgStore.DB().Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"reason": "database unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "ready",
		"node_id":   h.cfg.NodeID,
		"timestamp": time.Now().UnixMilli(),
	})
}

// ==================== Service ====================

type Service struct {
	cfg              *Config
	userStore        *UserStore
	msgStore         *MessageStore
	groupStore       *GroupStore
	groupMemStore    *GroupMemberStore
	friendStore      *FriendRequestStore
	convPartStore    *ConversationParticipantStore
	idGenStore       *IdGeneratorStateStore
	idempotencyStore *MessageIdempotencyStore
	idGen            *IdGenerator
	s3Service        *S3Service
}

// checkFound 通用辅助：如果 err != nil 则返回 err，如果 item == nil 则返回 ErrNotFound
func checkFound[T any](item *T, err error) (*T, error) {
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrNotFound
	}
	return item, nil
}

// withTx 泛型事务辅助：自动处理 BeginTx → defer Rollback → Commit → 置 nil
// 注意：不要在 fn 内部直接使用 defer 做资源清理，如有需要请使用更细粒度的控制
func withTx[T any](db *sql.DB, ctx context.Context, opts *sql.TxOptions, fn func(*sql.Tx) (T, error)) (T, error) {
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		var zero T
		return zero, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	result, err := fn(tx)
	if err != nil {
		return result, err
	}

	if err := tx.Commit(); err != nil {
		var zero T
		return zero, err
	}
	tx = nil
	return result, nil
}

func NewService(
	cfg *Config,
	userStore *UserStore,
	msgStore *MessageStore,
	groupStore *GroupStore,
	groupMemStore *GroupMemberStore,
	friendStore *FriendRequestStore,
	convPartStore *ConversationParticipantStore,
	idGenStore *IdGeneratorStateStore,
	idempotencyStore *MessageIdempotencyStore,
	idGen *IdGenerator,
	s3Service *S3Service,
) *Service {
	return &Service{
		cfg:              cfg,
		userStore:        userStore,
		msgStore:         msgStore,
		groupStore:       groupStore,
		groupMemStore:    groupMemStore,
		friendStore:      friendStore,
		convPartStore:    convPartStore,
		idGenStore:       idGenStore,
		idempotencyStore: idempotencyStore,
		idGen:            idGen,
		s3Service:        s3Service,
	}
}

// ==================== Notification ====================

// 通知子类型常量
const (
	NotifFriendRequest        = "friend_request"
	NotifFriendRequestSent    = "friend_request_sent"
	NotifGroupInvite          = "group_invite"
)

type NotificationPayload struct {
	Type    string                 `json:"type"`
	SubType string                 `json:"sub_type"`
	Data    map[string]interface{} `json:"data"`
}

type NotifData map[string]interface{}

// buildSystemMessage 构建系统消息（FromUID=0），避免两处重复
func (s *Service) buildSystemMessage(ctx context.Context, convID string, content []byte) (*Message, error) {
	msgID, err := s.idGen.GenerateID(ctx)
	if err != nil {
		return nil, err
	}
	ts := time.Now().UnixMilli() - s.idGen.GetEpochTime()
	return &Message{
		MsgID:   msgID,
		ConvID:  convID,
		FromUID: 0,
		Content: string(content),
		Ts:      ts,
	}, nil
}

// SendNotificationToUser 核心通知发送方法（无事务）
func (s *Service) SendNotificationToUser(ctx context.Context, uid int64, notifType string, data NotifData) (int64, error) {
	convID := fmt.Sprintf("system_%d", uid)

	payload := &NotificationPayload{
		Type:    "system_notification",
		SubType: notifType,
		Data:    data,
	}

	content, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("notification marshal failed: %w", err)
	}

	msg, err := s.buildSystemMessage(ctx, convID, content)
	if err != nil {
		return 0, err
	}

	_, err = s.msgStore.InsertMessage(ctx, s.msgStore.DB(), msg)
	if err != nil {
		return 0, err
	}

	return msg.MsgID, nil
}

// sendSystemMessageTx 事务内发送系统消息
func (s *Service) sendSystemMessageTx(ctx context.Context, tx *sql.Tx, convID string, content []byte) (int64, error) {
	msg, err := s.buildSystemMessage(context.Background(), convID, content)
	if err != nil {
		return 0, err
	}

	_, err = s.msgStore.InsertMessage(context.Background(), tx, msg)
	if err != nil {
		return 0, err
	}

	return msg.MsgID, nil
}


