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

const nanoIDAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"

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

func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"node_id":   h.cfg.NodeID,
		"timestamp": time.Now().UnixMilli(),
	})
}

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

type NotificationPayload struct {
	Type    string                 `json:"type"`
	SubType string                 `json:"sub_type"`
	Data    map[string]interface{} `json:"data"`
}

func (s *Service) SendNotificationToUser(ctx context.Context, uid int64, notifType string, data map[string]interface{}) (int64, error) {
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

	msgID, err := s.idGen.GenerateID()
	if err != nil {
		return 0, err
	}

	ts := time.Now().UnixMilli() - s.cfg.EpochTime
	tableName := MessageTableNameByTs(ts)

	msg := &Message{
		MsgID:   msgID,
		ConvID:  convID,
		FromUID: 0,
		Content: string(content),
		Ts:      ts,
	}

	_, err = s.msgStore.InsertMessage(ctx, tableName, msg)
	if err != nil {
		return 0, err
	}

	return msgID, nil
}

func (s *Service) sendSystemMessageTx(tx *sql.Tx, convID string, content []byte) (int64, error) {
	msgID, err := s.idGen.GenerateID()
	if err != nil {
		return 0, err
	}

	ts := time.Now().UnixMilli() - s.cfg.EpochTime
	tableName := MessageTableNameByTs(ts)

	msg := &Message{
		MsgID:   msgID,
		ConvID:  convID,
		FromUID: 0,
		Content: string(content),
		Ts:      ts,
	}

	_, err = s.msgStore.InsertMessageTx(tx, tableName, msg)
	if err != nil {
		return 0, err
	}

	return msgID, nil
}

func (s *Service) SendFriendNotification(ctx context.Context, receiverUID, senderUID int64, requestID string) error {
	_, err := s.SendNotificationToUser(ctx, receiverUID, "friend_request", map[string]interface{}{
		"request_id": requestID,
		"sender_uid": senderUID,
		"action":     "received",
	})
	return err
}

func (s *Service) SendFriendAcceptedNotification(ctx context.Context, senderUID, receiverUID int64, requestID string) error {
	_, err := s.SendNotificationToUser(ctx, senderUID, "friend_request", map[string]interface{}{
		"request_id":   requestID,
		"receiver_uid": receiverUID,
		"action":       "accepted",
	})
	return err
}

func (s *Service) SendGroupInviteNotification(ctx context.Context, uid int64, groupID, groupName string, inviterUID int64) error {
	_, err := s.SendNotificationToUser(ctx, uid, "group_invite", map[string]interface{}{
		"group_id":    groupID,
		"group_name":  groupName,
		"inviter_uid": inviterUID,
		"action":      "invited",
	})
	return err
}
