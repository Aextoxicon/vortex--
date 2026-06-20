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

// 这里属于是篇幅长但是很多都是样板代码，干了什么基本就相当于函数的名字
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

type SendMessageResponse struct {
	Message MessageResponseData `json:"message"`
}

type MessageResponseData struct {
	ID          string `json:"id"`
	ConvID      string `json:"conv_id"`
	SenderID    string `json:"sender_id"`
	Content     string `json:"content"`
	ContentType string `json:"content_type,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// SendMessage 发送消息
// @Summary      发送消息
// @Description  发送一条消息到指定会话
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        request  body  SendMessageRequest  true  "消息请求"
// @Success      201  {object}  map[string]interface{}  "消息发送成功"
// @Failure      400  {object}  ErrorResponse  "请求错误"
// @Failure      429  {object}  ErrorResponse  "请求频率限制"
// @Router       /api/messages/send [post]
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

	createdAt := time.Now().UTC().Format(time.RFC3339)
	resp := SendMessageResponse{
		Message: MessageResponseData{
			ID:          fmt.Sprintf("msg_%s", result.MsgID),
			ConvID:      result.ConvID,
			SenderID:    publicID,
			Content:     result.Content,
			ContentType: req.ContentType,
			CreatedAt:   createdAt,
		},
	}
	c.JSON(http.StatusCreated, resp)
}

// GetMessages 获取消息列表
// @Summary      获取消息列表
// @Description  获取指定会话的消息列表，支持分页和游标
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        conv_id    query  string  true   "会话ID"
// @Param        page       query  int     false  "页码"
// @Param        page_size  query  int     false  "每页条数"
// @Param        lastMsgId  query  int     false  "游标消息ID"
// @Success      200  {object}  map[string]interface{}  "消息列表"
// @Failure      400  {object}  ErrorResponse  "请求错误"
// @Router       /api/messages [get]
func (h *Handler) GetMessages(c *gin.Context) {
	// userID is unused in current implementation
	convID := c.Query("conv_id")
	page := 1
	pageSize := h.cfg.DefaultPageSize
	var lastMsgId int64
	useCursor := false

	if convID == "" {
		convID = c.Query("convId")
	}
	// 即使默认聊天记录在服务器只待七天，但是客户端可以自行设置
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			page = n
		} else {
			handleError(c, ErrInvalidInput)
			return
		}
	}

	if v := c.Query("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			pageSize = n
		} else {
			handleError(c, ErrInvalidInput)
			return
		}
	} else if v := c.Query("pageSize"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			pageSize = n
		} else {
			handleError(c, ErrInvalidInput)
			return
		}
	}

	if pageSize < 1 {
		pageSize = h.cfg.DefaultPageSize
	}
	if pageSize > h.cfg.MaxPageSize {
		pageSize = h.cfg.MaxPageSize
	}

	if v := c.Query("lastMsgId"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			lastMsgId = n
			useCursor = true
		} else {
			handleError(c, ErrInvalidInput)
			return
		}
	}

	var messages *MessagePage
	var err error

	if useCursor {
		messages, err = h.svc.msgStore.GetMessagesAfter(c.Request.Context(), convID, lastMsgId, pageSize+1)
		if err != nil {
			handleError(c, err)
			return
		}
	} else {
		offset := (page - 1) * pageSize
		allMsgs, err := h.svc.msgStore.GetConversationMessages(c.Request.Context(), convID, pageSize+1, offset)
		if err != nil {
			handleError(c, err)
			return
		}
		hasMore := len(allMsgs) > pageSize
		if hasMore {
			allMsgs = allMsgs[:pageSize]
		}
		var maxID int64
		if len(allMsgs) > 0 {
			maxID = allMsgs[len(allMsgs)-1].MsgID
		}
		messages = &MessagePage{
			Messages: allMsgs,
			HasMore:  hasMore,
			MaxMsgID: maxID,
		}
	}

	// 构建响应
	type msgItem struct {
		ID          string `json:"id"`
		ConvID      string `json:"conv_id"`
		SenderID    string `json:"sender_id"`
		Content     string `json:"content"`
		ContentType string `json:"content_type"`
		CreatedAt   string `json:"created_at"`
	}

	items := make([]msgItem, 0, len(messages.Messages))
	for _, m := range messages.Messages {
		ct := "text"
		if len(m.Content) > 0 && m.Content[0] == '{' {
			var img ImageContent
			if err := json.Unmarshal([]byte(m.Content), &img); err == nil && img.Type == "image" {
				ct = "image"
			}
		}
		ts := m.Ts + h.svc.idGen.GetEpochTime()
		createdAt := time.UnixMilli(ts).UTC().Format(time.RFC3339)

		content := m.Content
		if ct == "image" {
			var img ImageContent
			if err := json.Unmarshal([]byte(m.Content), &img); err == nil {
				content = img.Text
			}
		}

		items = append(items, msgItem{
			ID:          fmt.Sprintf("msg_%d", m.MsgID),
			ConvID:      m.ConvID,
			SenderID:    fmt.Sprintf("%d", m.FromUID),
			Content:     content,
			ContentType: ct,
			CreatedAt:   createdAt,
		})
	}

	resp := gin.H{
		"messages":  items,
		"page_size": pageSize,
		"has_more":  messages.HasMore,
	}
	if useCursor {
		resp["last_msg_id"] = messages.MaxMsgID
	}

	c.JSON(http.StatusOK, resp)
}

// RecallMessage 撤回消息
// @Summary      撤回消息
// @Description  撤回指定消息
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        msgId  path  int  true  "消息ID"
// @Success      200  {object}  SuccessResponse  "撤回成功"
// @Failure      400  {object}  ErrorResponse  "请求错误"
// @Failure      403  {object}  ErrorResponse  "无权操作"
// @Router       /api/messages/recall/{msgId} [post]
func (h *Handler) RecallMessage(c *gin.Context) {
	userID := c.GetInt64("user_id")

	msgID, err := strconv.ParseInt(c.Param("msgId"), 10, 64)
	if err != nil {
		handleError(c, ErrInvalidMsgID)
		return
	}

	if err := h.svc.RecallMessage(c.Request.Context(), msgID, userID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "message recalled successfully"})
}

// CheckNewMessages 检查新消息
// @Summary      检查新消息
// @Description  检查用户是否有新消息
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        lastMsgId  query  int  false  "最后消息ID"
// @Success      200  {object}  map[string]interface{}  "检查结果"
// @Router       /api/check [get]
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

	c.JSON(http.StatusOK, gin.H{"status": result.Status, "updated": result.Updated})
}

func (s *Service) SendMessage(ctx context.Context, currentUser *User, convID, content, clientMsgID string) (*SendMessageResult, error) {
	uid := currentUser.ID

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
	}

	msgType := ExtractConversationType(convID)

	msgID, err := s.idGen.GenerateID(ctx)
	if err != nil {
		return nil, err
	}

	ts := time.Now().UnixMilli() - s.idGen.GetEpochTime()

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

	_, err = s.msgStore.InsertMessage(context.Background(), tx, msg)
	if err != nil {
		return nil, err
	}

	if clientMsgID != "" {
		if err := s.idempotencyStore.UpdateMsgID(ctx, tx, uid, clientMsgID, msgID, convID); err != nil {
			return nil, err
		}
	}

	// 更新会话缓存
	if err := s.convPartStore.UpdateLastMsgCache(tx, msg); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		// 清理幂等记录
		if clientMsgID != "" {
			if cleanErr := s.idempotencyStore.UpdateMsgID(ctx, s.idempotencyStore.DB(), uid, clientMsgID, 0, convID); cleanErr != nil {
				slog.Warn("failed to clean up idempotency record", "error", cleanErr)
			}
		}
		return nil, err
	}
	tx = nil

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

type CheckNewMessagesResult struct {
	Status  int
	Updated []string
}

func (s *Service) CheckNewMessages(ctx context.Context, userID, lastMsgID int64) (*CheckNewMessagesResult, error) {
	updated, err := s.msgStore.GetUpdatedConversations(ctx, userID, lastMsgID)
	if err != nil {
		return nil, err
	}

	hasPendingRequests, err := s.friendStore.HasPendingRequests(ctx, userID)
	if err != nil {
		return nil, err
	}

	status := 0
	if len(updated) > 0 {
		status |= 1
	}
	if hasPendingRequests {
		status |= 2
	}

	return &CheckNewMessagesResult{Status: status, Updated: updated}, nil
}

func (s *Service) RecallMessage(ctx context.Context, msgID, userID int64) error {
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

	now := time.Now().UnixMilli()
	msgAge := now - (msg.Ts + s.idGen.GetEpochTime())
	if msgAge > s.cfg.MessageRecallWindowMs {
		return ErrRecallWindowExpired
	}

	tx, err := s.msgStore.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	// 更新消息
	msg.IsRecalled = 1
	msg.Content = ""
	_, err = s.msgStore.UpdateMessage(context.Background(), tx, msg)
	if err != nil {
		return err
	}

	// 插入撤回通知消息（content 存储原消息 ID）
	recallMsgID, err := s.idGen.GenerateID(ctx)
	if err != nil {
		return err
	}

	recallTs := time.Now().UnixMilli() - s.idGen.GetEpochTime()

	recallMsg := &Message{
		MsgID:      recallMsgID,
		ConvID:     msg.ConvID,
		FromUID:    userID,
		Content:    fmt.Sprintf("%d", msgID),
		Ts:         recallTs,
		IsRecalled: 1,
	}

	_, err = s.msgStore.InsertMessage(context.Background(), tx, recallMsg)
	if err != nil {
		return err
	}

	// 更新会话缓存：如果撤回的是最后一条消息，将 content 设为 [已撤回]，is_recalled 设为 1
	if err := s.convPartStore.UpdateLastMsgCacheOnRecall(tx, msg.ConvID, msg.MsgID, "[已撤回]", 1); err != nil {
		slog.Warn("failed to update last message cache on recall", "error", err)
		// 不影响主流程
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil

	// 删除 S3 文件（如果是文件消息）
	if s.s3Service != nil {
		fileKey := extractFileKeyFromMessage(msg)
		if fileKey != "" {
			if err := s.s3Service.DeleteObject(ctx, fileKey); err != nil {
				slog.Warn("failed to delete S3 object during message recall", "file_key", fileKey, "error", err)
			}
		}
	}

	return nil
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

		// 检查私聊中任一方是否 block 了对方
		anyBlocked, err := s.convPartStore.IsAnyBlocked(ctx, convID)
		if err != nil {
			return err
		}
		if anyBlocked {
			return ErrForbidden
		}

		areFriends, err := s.friendStore.AreFriends(ctx, uid, targetUser.ID)
		if err != nil {
			return err
		}
		if !areFriends {
			return ErrNotFriend
		}

		hasPerm, err := s.convPartStore.Exists(context.Background(), tx, convID, uid)
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

// GetConversationCount 获取会话数量
// @Summary      获取会话数量
// @Description  获取用户会话总数
// @Tags         conversations
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Success      200  {object}  map[string]interface{}  "会话数量"
// @Router       /api/conversations/count [get]
func (h *Handler) GetConversationCount(c *gin.Context) {
	userID := c.GetInt64("user_id")
	count, err := h.svc.convPartStore.CountConversations(c.Request.Context(), userID)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_id": userID, "count": count})
}

// GetConversationParticipants 获取会话参与者
// @Summary      获取会话参与者
// @Description  获取指定会话的参与者列表
// @Tags         conversations
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        convId  path  string  true  "会话ID"
// @Success      200  {object}  map[string]interface{}  "参与者列表"
// @Router       /api/conversations/{convId}/participants [get]
func (h *Handler) GetConversationParticipants(c *gin.Context) {
	convID := c.Param("convId")
	participants, err := h.svc.convPartStore.GetParticipants(c.Request.Context(), convID)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"conv_id": convID, "participants": participants})
}

// CheckBlocked 检查是否被屏蔽
// @Summary      检查是否被屏蔽
// @Description  检查指定用户在会话中是否被屏蔽
// @Tags         conversations
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        convId  path  string  true  "会话ID"
// @Param        userId  path  int     true  "用户ID"
// @Success      200  {object}  map[string]interface{}  "屏蔽状态"
// @Router       /api/conversations/{convId}/blocked/{userId} [get]
func (h *Handler) CheckBlocked(c *gin.Context) {
	convID := c.Param("convId")
	userIDStr := c.Param("userId")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		handleError(c, ErrInvalidInput)
		return
	}
	isBlocked, err := h.svc.convPartStore.IsBlocked(c.Request.Context(), convID, userID)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"conv_id": convID, "user_id": userID, "is_blocked": isBlocked})
}

// GetMessageByID 获取消息详情
// @Summary      获取消息详情
// @Description  根据消息ID获取消息的详细信息
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        msgId  path  int  true  "消息ID"
// @Success      200  {object}  map[string]interface{}  "消息详情"
// @Failure      400  {object}  ErrorResponse  "请求错误"
// @Router       /api/messages/{msgId} [get]
func (h *Handler) GetMessageByID(c *gin.Context) {
	msgIDStr := c.Param("msgId")
	msgID, err := strconv.ParseInt(msgIDStr, 10, 64)
	if err != nil {
		handleError(c, ErrInvalidMsgID)
		return
	}
	msg, err := h.svc.GetMessage(c.Request.Context(), msgID)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"msg_id":      msg.MsgID,
		"conv_id":     msg.ConvID,
		"from_uid":    msg.FromUID,
		"content":     msg.Content,
		"ts":          msg.Ts,
		"is_recalled": msg.IsRecalled == 1,
	})
}

// BlockUser 屏蔽用户
// @Summary      屏蔽用户
// @Description  屏蔽指定用户
// @Tags         blocks
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        targetPublicId  path  string  true  "目标用户公钥ID"
// @Success      200  {object}  SuccessResponse  "屏蔽成功"
// @Failure      401  {object}  ErrorResponse  "未授权"
// @Failure      404  {object}  ErrorResponse  "用户不存在"
// @Router       /api/blocks/{targetPublicId} [post]
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

// UnblockUser 取消屏蔽用户
// @Summary      取消屏蔽用户
// @Description  取消屏蔽指定用户
// @Tags         blocks
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        targetPublicId  path  string  true  "目标用户公钥ID"
// @Success      200  {object}  SuccessResponse  "取消屏蔽成功"
// @Failure      401  {object}  ErrorResponse  "未授权"
// @Failure      404  {object}  ErrorResponse  "用户不存在"
// @Router       /api/blocks/{targetPublicId} [delete]
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

// GetConversations 获取会话列表
// @Summary      获取会话列表
// @Description  获取用户的所有会话列表
// @Tags         conversations
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        limit   query  int  false  "每页条数"
// @Param        offset  query  int  false  "偏移量"
// @Success      200  {object}  ConversationListResponse  "会话列表"
// @Router       /api/conversations [get]
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

	total, err := s.convPartStore.CountConversations(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &ConversationListResponse{
		Conversations: conversations,
		Total:         total,
	}, nil
}
