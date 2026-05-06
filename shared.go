package main

import (
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	mathrand "math/rand"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

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

// ==================== Service ====================

type Service struct {
	cfg           *Config
	userStore     *UserStore
	msgStore      *MessageStore
	groupStore    *GroupStore
	groupMemStore *GroupMemberStore
	friendStore   *FriendRequestStore
	convPartStore *ConversationParticipantStore
	idGenStore    *IdGeneratorStateStore
	idGen         *IdGenerator
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
	idGen *IdGenerator,
) *Service {
	return &Service{
		cfg:           cfg,
		userStore:     userStore,
		msgStore:      msgStore,
		groupStore:    groupStore,
		groupMemStore: groupMemStore,
		friendStore:   friendStore,
		convPartStore: convPartStore,
		idGenStore:    idGenStore,
		idGen:         idGen,
	}
}

// ==================== Notification ====================

type NotificationPayload struct {
	Type    string                 `json:"type"`
	SubType string                 `json:"sub_type"`
	Data    map[string]interface{} `json:"data"`
}

func (s *Service) SendNotificationToUser(uid int64, notifType string, data map[string]interface{}) (int64, error) {
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

	_, err = s.msgStore.InsertMessage(tableName, msg)
	if err != nil {
		return 0, err
	}

	return msgID, nil
}

func (s *Service) SendFriendNotification(receiverUID, senderUID int64, requestID string) error {
	_, err := s.SendNotificationToUser(receiverUID, "friend_request", map[string]interface{}{
		"request_id": requestID,
		"sender_uid": senderUID,
		"action":     "received",
	})
	return err
}

func (s *Service) SendFriendAcceptedNotification(senderUID, receiverUID int64, requestID string) error {
	_, err := s.SendNotificationToUser(senderUID, "friend_request", map[string]interface{}{
		"request_id":   requestID,
		"receiver_uid": receiverUID,
		"action":       "accepted",
	})
	return err
}

func (s *Service) SendGroupInviteNotification(uid int64, groupID, groupName string, inviterUID int64) error {
	_, err := s.SendNotificationToUser(uid, "group_invite", map[string]interface{}{
		"group_id":    groupID,
		"group_name":  groupName,
		"inviter_uid": inviterUID,
		"action":      "invited",
	})
	return err
}
