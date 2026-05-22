package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type SendMessageRequest struct {
	ConvID      string `json:"conv_id" binding:"required"`
	Content     string `json:"content" binding:"required,max=1000"`
	Text        string `json:"text" binding:"max=1000"`
	ContentType string `json:"content_type"`
	ClientMsgID string `json:"client_msg_id"`
}

type SendMessageResult struct {
	MsgID      string `json:"msg_id"`
	ConvID     string `json:"conv_id"`
	FromUID    int64  `json:"from_uid"`
	Content    string `json:"content"`
	Ts         int64  `json:"ts"`
	IsRecalled int    `json:"is_recalled"`
	Duplicate  bool   `json:"duplicate,omitempty"`
}

func (h *Handler) SendMessage(c *gin.Context) {
	userID := c.GetInt64("user_id")
	publicID := c.GetString("public_id")

	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, ErrInvalidInput)
		return
	}

	content := req.Content
	if req.ContentType == "image" {
		img := ImageContent{Type: "image", Content: req.Content, Text: req.Text}
		data, err := json.Marshal(img)
		if err != nil {
			handleError(c, ErrInternalServer)
			return
		}
		content = string(data)
	}

	user := &User{ID: userID, PublicID: publicID}
	result, err := h.svc.SendMessage(c.Request.Context(), user, req.ConvID, content, req.ClientMsgID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, result)
}

func (h *Handler) GetMessages(c *gin.Context) {
	userID := c.GetInt64("user_id")

	convID := c.Query("convId")
	dateStr := c.Query("date")
	days := 1
	pageSize := h.cfg.DefaultPageSize
	var lastMsgId int64

	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			days = n
		} else {
			handleError(c, ErrInvalidInput)
			return
		}
	}
	if days < 1 {
		days = 1
	}
	if days > 7 {
		days = 7
	}

	if v := c.Query("pageSize"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			pageSize = n
		} else {
			handleError(c, ErrInvalidInput)
			return
		}
	}
	if pageSize > h.cfg.MaxPageSize {
		pageSize = h.cfg.MaxPageSize
	}
	if v := c.Query("lastMsgId"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			lastMsgId = n
		} else {
			handleError(c, ErrInvalidInput)
			return
		}
	}

	var date time.Time
	if dateStr != "" {
		var err error
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			handleError(c, ErrInvalidDateFormat)
			return
		}
	} else {
		date = time.Now()
	}

	messages, err := h.svc.GetConversationMessages(c.Request.Context(), convID, date, days, pageSize, lastMsgId, userID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"messages":   messages.Messages,
		"has_more":   messages.HasMore,
		"max_msg_id": messages.MaxMsgID,
	})
}

func (h *Handler) RecallMessage(c *gin.Context) {
	userID := c.GetInt64("user_id")

	msgID, err := strconv.ParseInt(c.Param("msgId"), 10, 64)
	if err != nil {
		handleError(c, ErrInvalidMsgID)
		return
	}

	msgTimestamp := h.svc.idGen.ExtractTimestampFromMsgID(msgID)

	if err := h.svc.RecallMessage(c.Request.Context(), msgID, msgTimestamp, userID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{Success: true, Message: "Message recalled successfully"})
}

func (h *Handler) CheckNewMessages(c *gin.Context) {
	userID := c.GetInt64("user_id")

	var lastMsgID int64
	if v := c.Query("lastMsgId"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			lastMsgID = n
		} else {
			handleError(c, ErrInvalidInput)
			return
		}
	}

	result, err := h.svc.CheckNewMessages(c.Request.Context(), userID, lastMsgID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": result})
}

func (s *Service) SendMessage(ctx context.Context, currentUser *User, convID, content, clientMsgID string) (*SendMessageResult, error) {
	uid := currentUser.ID

	needsIdempotencyCleanup := false
	if clientMsgID != "" {
		isDup, existingMsgID, err := s.idempotencyStore.CheckAndInsert(ctx, uid, clientMsgID)
		if err != nil {
			return nil, err
		}
		if isDup {
			return &SendMessageResult{
				MsgID:     fmt.Sprintf("%d", existingMsgID),
				ConvID:    convID,
				FromUID:   uid,
				Content:   content,
				Duplicate: true,
			}, nil
		}
		needsIdempotencyCleanup = true
		defer func() {
			if needsIdempotencyCleanup {
				s.idempotencyStore.UpdateMsgID(ctx, uid, clientMsgID, 0, convID)
			}
		}()
	}

	msgType := ExtractConversationType(convID)

	msgID, err := s.idGen.GenerateID(ctx)
	if err != nil {
		return nil, err
	}

	ts := time.Now().UnixMilli() - s.cfg.EpochTime

	msg := &Message{
		MsgID:   msgID,
		ConvID:  convID,
		FromUID: uid,
		Content: content,
		Ts:      ts,
	}

	var targetUser *User
	if msgType == "p" || msgType == "user" {
		targetPublicID, err := s.GetPublicIDByUserID(ctx, uid)
		if err != nil {
			return nil, err
		}
		otherPublicID := GetOtherPublicID(convID, targetPublicID)
		if otherPublicID == "" {
			return nil, ErrInvalidTargetID
		}
		targetUser, err = s.GetUserByPublicID(ctx, otherPublicID)
		if err != nil {
			return nil, err
		}
		if targetUser == nil {
			return nil, ErrInvalidTargetID
		}
	}

	tx, err := s.msgStore.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	if err := s.ensureSessionPermissionTx(ctx, tx, uid, convID, msgType, targetUser); err != nil {
		return nil, err
	}

	_, err = s.msgStore.InsertMessageTx(tx, msg)
	if err != nil {
		return nil, err
	}

	if clientMsgID != "" {
		if err := s.idempotencyStore.UpdateMsgIDTx(ctx, tx, uid, clientMsgID, msgID, convID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	needsIdempotencyCleanup = false

	result := &SendMessageResult{
		MsgID:   fmt.Sprintf("%d", msgID),
		ConvID:  convID,
		FromUID: uid,
		Content: content,
		Ts:      ts,
	}

	return result, nil
}

func (s *Service) GetMessage(ctx context.Context, msgID int64) (*Message, error) {
	msg, err := s.msgStore.GetMessage(ctx, msgID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, ErrNotFound
	}
	return msg, nil
}

func (s *Service) GetConversationMessages(ctx context.Context, convID string, endDate time.Time, days, pageSize int, lastMsgId int64, userID int64) (*MessagePage, error) {
	hasPerm, err := s.convPartStore.Exists(ctx, convID, userID)
	if err != nil {
		return nil, err
	}

	if !hasPerm {
		if groupID := ExtractGroupID(convID); groupID != "" {
			isMember, err := s.IsUserInGroup(ctx, groupID, userID)
			if err != nil {
				return nil, err
			}
			if !isMember {
				return nil, ErrForbidden
			}
		} else {
			return nil, ErrForbidden
		}
	}

	if IsPrivateConv(convID) {
		participants, err := s.convPartStore.GetParticipants(ctx, convID)
		if err != nil {
			return nil, err
		}
		for _, participantID := range participants {
			if participantID != userID {
				isBlocked, err := s.convPartStore.IsBlocked(ctx, convID, participantID)
				if err != nil {
					return nil, err
				}
				if isBlocked {
					return nil, ErrForbidden
				}
				break
			}
		}
	}

	startDate := endDate.AddDate(0, 0, -(days - 1))
	startTs := startDate.UnixMilli() - s.cfg.EpochTime
	endTs := endDate.AddDate(0, 0, 1).UnixMilli() - s.cfg.EpochTime

	return s.msgStore.GetConversationMessagesByRange(ctx, convID, startTs, endTs, pageSize, lastMsgId)
}

func (s *Service) CheckNewMessages(ctx context.Context, userID, lastMsgID int64) (int, error) {
	hasNewMessages, err := s.msgStore.HasNewMessagesAfter(ctx, userID, lastMsgID)
	if err != nil {
		return 0, err
	}

	hasPendingRequests, err := s.friendStore.HasPendingRequests(ctx, userID)
	if err != nil {
		return 0, err
	}

	status := 0
	if hasNewMessages {
		status |= 1
	}
	if hasPendingRequests {
		status |= 2
	}

	return status, nil
}

func (s *Service) RecallMessage(ctx context.Context, msgID, msgTimestamp, userID int64) error {
	now := time.Now().UnixMilli()
	msgAge := now - msgTimestamp
	if msgAge > s.cfg.MessageRecallWindowMs {
		return ErrRecallWindowExpired
	}

	msg, err := s.msgStore.GetMessage(ctx, msgID)
	if err != nil {
		return err
	}
	if msg == nil {
		return ErrNotFound
	}
	if msg.IsRecalled == 1 {
		return ErrConflict
	}
	if msg.FromUID != userID {
		return ErrForbidden
	}

	msg.IsRecalled = 1
	msg.Content = ""
	_, err = s.msgStore.UpdateMessage(ctx, msg)
	if err != nil {
		return err
	}

	// 删除 S3 文件（如果是文件消息）
	if s.s3Service != nil {
		fileKey := extractFileKeyFromMessage(msg)
		if fileKey != "" {
			if err := s.s3Service.DeleteObject(ctx, fileKey); err != nil {
				slog.Warn("failed to delete S3 object during message recall", "file_key", fileKey, "error", err)
			}
		}
	}

	// 插入撤回通知消息（content 存储原消息 ID）
	recallMsgID, err := s.idGen.GenerateID(ctx)
	if err != nil {
		return err
	}

	recallTs := time.Now().UnixMilli() - s.cfg.EpochTime

	recallMsg := &Message{
		MsgID:      recallMsgID,
		ConvID:     msg.ConvID,
		FromUID:    userID,
		Content:    fmt.Sprintf("%d", msgID),
		Ts:         recallTs,
		IsRecalled: 1,
	}

	_, err = s.msgStore.InsertMessage(ctx, recallMsg)
	return err
}

func extractFileKeyFromMessage(msg *Message) string {
	var img ImageContent
	if err := json.Unmarshal([]byte(msg.Content), &img); err == nil {
		if img.Type == "image" && img.Content != "" {
			return img.Content
		}
	}
	return ""
}

func (s *Service) generateConversationID(ctx context.Context, msgType string, uid int64, targetPublicID string) (string, error) {
	switch msgType {
	case "p", "user":
		targetUser, err := s.GetUserByPublicID(ctx, targetPublicID)
		if err != nil {
			return "", err
		}
		if targetUser == nil {
			return "", ErrInvalidTargetID
		}
		myPublicID, err := s.GetPublicIDByUserID(ctx, uid)
		if err != nil {
			return "", err
		}
		return PrivateConvID(myPublicID, targetPublicID), nil
	case "g", "group":
		if !s.IsValidGroupID(targetPublicID) {
			return "", ErrInvalidType
		}
		return targetPublicID, nil
	default:
		return "", ErrInvalidType
	}
}

func (s *Service) ensureSessionPermissionTx(ctx context.Context, tx *sql.Tx, uid int64, convID, msgType string, targetUser *User) error {
	switch msgType {
	case "p", "user":
		myPublicID, err := s.GetPublicIDByUserID(ctx, uid)
		if err != nil {
			return err
		}
		if !CanAccessPrivateConv(convID, myPublicID) {
			return ErrForbidden
		}

		isBlocked, err := s.convPartStore.IsBlocked(ctx, convID, targetUser.ID)
		if err != nil {
			return err
		}
		if isBlocked {
			return ErrForbidden
		}

		areFriends, err := s.friendStore.AreFriends(ctx, uid, targetUser.ID)
		if err != nil {
			return err
		}
		if !areFriends {
			return ErrNotFriend
		}

		hasPerm, err := s.convPartStore.ExistsTx(tx, convID, uid)
		if err != nil {
			return err
		}
		if !hasPerm {
			if targetUser == nil {
				return ErrInvalidTargetID
			}

			now := time.Now().UnixMilli()
			participants := []*ConversationParticipant{
				{ConvID: convID, UserID: uid, JoinTs: now, IsBlocked: 0},
				{ConvID: convID, UserID: targetUser.ID, JoinTs: now, IsBlocked: 0},
			}
			_, err = s.convPartStore.InsertBatchTx(tx, participants)
			if err != nil {
				return err
			}
		}
		return nil
	case "g", "group":
		groupID := ExtractGroupID(convID)
		if groupID == "" {
			return ErrInvalidType
		}
		isMember, err := s.IsUserInGroup(ctx, groupID, uid)
		if err != nil {
			return err
		}
		if !isMember {
			return ErrNotMember
		}
		return nil
	default:
		return ErrInvalidType
	}
}

type ImageContent struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Text    string `json:"text"`
}

func (h *Handler) BlockUser(c *gin.Context) {
	targetPublicID := c.Param("targetPublicId")

	targetUser, err := h.svc.GetUserByPublicID(c.Request.Context(), targetPublicID)
	if err != nil {
		handleError(c, ErrNotFound)
		return
	}

	convID := PrivateConvID(c.GetString("public_id"), targetPublicID)

	if err := h.svc.BlockUser(c.Request.Context(), convID, targetUser.ID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User blocked successfully"})
}

func (h *Handler) UnblockUser(c *gin.Context) {
	targetPublicID := c.Param("targetPublicId")

	targetUser, err := h.svc.GetUserByPublicID(c.Request.Context(), targetPublicID)
	if err != nil {
		handleError(c, ErrNotFound)
		return
	}

	convID := PrivateConvID(c.GetString("public_id"), targetPublicID)

	if err := h.svc.UnblockUser(c.Request.Context(), convID, targetUser.ID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User unblocked successfully"})
}

func (h *Handler) GetConversations(c *gin.Context) {
	userID := c.GetInt64("user_id")

	limitStr := c.DefaultQuery("limit", "20")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		handleError(c, ErrInvalidInput)
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		handleError(c, ErrInvalidInput)
		return
	}

	result, err := h.svc.GetConversationList(c.Request.Context(), userID, limit, offset)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Service) BlockUser(ctx context.Context, convID string, targetUserID int64) error {
	return s.convPartStore.SetBlocked(ctx, convID, targetUserID, true)
}

func (s *Service) UnblockUser(ctx context.Context, convID string, targetUserID int64) error {
	return s.convPartStore.SetBlocked(ctx, convID, targetUserID, false)
}

type ConversationListResponse struct {
	Conversations []*ConversationItem `json:"conversations"`
	Total         int                 `json:"total"`
}

type ConversationItem struct {
	ConvID      string           `json:"conv_id"`
	Type        string           `json:"type"`
	Name        string           `json:"name"`
	PublicID    string           `json:"public_id,omitempty"`
	Username    string           `json:"username,omitempty"`
	GroupID     string           `json:"group_id,omitempty"`
	MemberCount int              `json:"member_count,omitempty"`
	LastMessage *LastMessageInfo `json:"last_message,omitempty"`
	UnreadCount int              `json:"unread_count"`
}

type LastMessageInfo struct {
	MsgID      int64  `json:"msg_id"`
	Content    string `json:"content"`
	FromUID    int64  `json:"from_uid"`
	Ts         int64  `json:"ts"`
	IsRecalled bool   `json:"is_recalled"`
}

func (s *Service) GetConversationList(ctx context.Context, userID int64, limit, offset int) (*ConversationListResponse, error) {
	items, err := s.convPartStore.GetConversationList(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}

	var userIDs []int64
	for _, item := range items {
		if item.Type == "private" && item.TargetUID != nil {
			userIDs = append(userIDs, *item.TargetUID)
		}
	}

	usersMap, err := s.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	conversations := make([]*ConversationItem, 0, len(items))
	for _, item := range items {
		conv := &ConversationItem{
			ConvID: item.ConvID,
			Type:   item.Type,
		}

		if item.Type == "private" && item.TargetUID != nil {
			if targetUser, ok := usersMap[*item.TargetUID]; ok {
				conv.Name = targetUser.Username
				conv.PublicID = targetUser.PublicID
				conv.Username = targetUser.Username
			}
		} else if item.Type == "group" && item.GroupID != nil {
			group, err := s.GetGroupByID(ctx, *item.GroupID)
			if err != nil {
				return nil, err
			}
			if group != nil {
				memberCount, err := s.GetGroupMemberCount(ctx, *item.GroupID)
				if err != nil {
					return nil, err
				}
				conv.Name = group.Name
				conv.GroupID = group.GroupID
				conv.MemberCount = memberCount
			}
		}

		if item.LastMsgID != nil {
			conv.LastMessage = &LastMessageInfo{
				MsgID:      *item.LastMsgID,
				Content:    *item.Content,
				FromUID:    *item.FromUID,
				Ts:         *item.LastMsgTs,
				IsRecalled: item.IsRecalled != nil && *item.IsRecalled == 1,
			}
		}

		conversations = append(conversations, conv)
	}

	return &ConversationListResponse{
		Conversations: conversations,
		Total:         len(conversations),
	}, nil
}
