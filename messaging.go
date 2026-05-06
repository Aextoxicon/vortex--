package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type SendMessageRequest struct {
	TargetPublicID string `json:"target_public_id" binding:"required"`
	Type           string `json:"type" binding:"required"`
	Content        string `json:"content" binding:"required"`
	Text           string `json:"text"`
	ContentType    string `json:"content_type"`
}

type SendMessageResult struct {
	MsgID      string `json:"msg_id"`
	ConvID     string `json:"conv_id"`
	FromUID    int64  `json:"from_uid"`
	Content    string `json:"content"`
	Ts         int64  `json:"ts"`
	IsRecalled int    `json:"is_recalled"`
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
	result, err := h.svc.SendMessage(user, req.TargetPublicID, req.Type, content)
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

	messages, err := h.svc.GetConversationMessages(convID, date, days, pageSize, lastMsgId, userID)
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

	if err := h.svc.RecallMessage(msgID, msgTimestamp, userID); err != nil {
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

	result, err := h.svc.CheckNewMessages(userID, lastMsgID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": result})
}

func (s *Service) SendMessage(currentUser *User, targetPublicID, msgType, content string) (*SendMessageResult, error) {
	uid := currentUser.ID

	convID, err := s.generateConversationID(msgType, uid, targetPublicID)
	if err != nil {
		return nil, err
	}

	msgID, err := s.idGen.GenerateID()
	if err != nil {
		return nil, err
	}

	ts := time.Now().UnixMilli() - s.cfg.EpochTime
	tableName := MessageTableNameByTs(ts)

	msg := &Message{
		MsgID:   msgID,
		ConvID:  convID,
		FromUID: uid,
		Content: content,
		Ts:      ts,
	}

	tx, err := s.msgStore.DB().Begin()
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	if err := s.ensureSessionPermissionTx(tx, uid, convID, msgType, targetPublicID); err != nil {
		return nil, err
	}

	_, err = s.msgStore.InsertMessageTx(tx, tableName, msg)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
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

func (s *Service) GetMessage(msgID, msgTimestamp int64) (*Message, error) {
	tableName := MessageTableNameByTs(msgTimestamp)
	msg, err := s.msgStore.GetMessage(tableName, msgID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, ErrNotFound
	}
	return msg, nil
}

func (s *Service) GetConversationMessages(convID string, endDate time.Time, days, pageSize int, lastMsgId int64, userID int64) (*MessagePage, error) {
	hasPerm, err := s.convPartStore.Exists(convID, userID)
	if err != nil {
		return nil, err
	}

	if !hasPerm {
		if groupID := ExtractGroupID(convID); groupID != "" {
			isMember, err := s.IsUserInGroup(groupID, userID)
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

	startDate := endDate.AddDate(0, 0, -(days - 1))
	startTs := startDate.UnixMilli() - s.cfg.EpochTime
	endTs := endDate.AddDate(0, 0, 1).UnixMilli() - s.cfg.EpochTime

	return s.msgStore.GetConversationMessagesByRange(convID, startTs, endTs, pageSize, lastMsgId)
}

func (s *Service) CheckNewMessages(userID, lastMsgID int64) (int, error) {
	hasNewMessages, err := s.msgStore.HasNewMessagesAfter(userID, lastMsgID)
	if err != nil {
		return 0, err
	}

	hasPendingRequests, err := s.friendStore.HasPendingRequests(userID)
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

func (s *Service) RecallMessage(msgID, msgTimestamp, userID int64) error {
	now := time.Now().UnixMilli()
	msgAge := now - msgTimestamp
	if msgAge > s.cfg.MessageRecallWindowMs {
		return ErrRecallWindowExpired
	}

	tableName := MessageTableNameByTs(msgTimestamp)
	msg, err := s.msgStore.GetMessage(tableName, msgID)
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
	_, err = s.msgStore.UpdateMessage(tableName, msg)
	return err
}

func (s *Service) generateConversationID(msgType string, uid int64, targetPublicID string) (string, error) {
	switch msgType {
	case "p", "user":
		targetUser, err := s.GetUserByPublicID(targetPublicID)
		if err != nil {
			return "", err
		}
		if targetUser == nil {
			return "", ErrInvalidTargetID
		}
		return PrivateConvID(uid, targetUser.ID), nil
	case "g", "group":
		if !s.IsValidGroupID(targetPublicID) {
			return "", ErrInvalidType
		}
		return targetPublicID, nil
	default:
		return "", ErrInvalidType
	}
}

func (s *Service) ensureSessionPermission(uid int64, convID, msgType, targetPublicID string) error {
	switch msgType {
	case "p", "user":
		if !CanAccessPrivateConv(convID, uid) {
			return ErrForbidden
		}

		hasPerm, err := s.convPartStore.Exists(convID, uid)
		if err != nil {
			return err
		}
		if !hasPerm {
			targetUser, err := s.GetUserByPublicID(targetPublicID)
			if err != nil {
				return err
			}
			if targetUser == nil {
				return ErrInvalidTargetID
			}

			now := time.Now().UnixMilli()
			participants := []*ConversationParticipant{
				{ConvID: convID, UserID: uid, JoinTs: now},
				{ConvID: convID, UserID: targetUser.ID, JoinTs: now},
			}
			_, err = s.convPartStore.InsertBatch(participants)
			if err != nil {
				return err
			}
		}
		return nil
	case "g", "group":
		isMember, err := s.IsUserInGroup(targetPublicID, uid)
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

func (s *Service) ensureSessionPermissionTx(tx *sql.Tx, uid int64, convID, msgType, targetPublicID string) error {
	switch msgType {
	case "p", "user":
		if !CanAccessPrivateConv(convID, uid) {
			return ErrForbidden
		}

		hasPerm, err := s.convPartStore.ExistsTx(tx, convID, uid)
		if err != nil {
			return err
		}
		if !hasPerm {
			targetUser, err := s.GetUserByPublicID(targetPublicID)
			if err != nil {
				return err
			}
			if targetUser == nil {
				return ErrInvalidTargetID
			}

			now := time.Now().UnixMilli()
			participants := []*ConversationParticipant{
				{ConvID: convID, UserID: uid, JoinTs: now},
				{ConvID: convID, UserID: targetUser.ID, JoinTs: now},
			}
			_, err = s.convPartStore.InsertBatchTx(tx, participants)
			if err != nil {
				return err
			}
		}
		return nil
	case "g", "group":
		isMember, err := s.IsUserInGroup(targetPublicID, uid)
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

func MessageTableNameByDate(date time.Time) string {
	return fmt.Sprintf("messages_%s", date.Format("20060102"))
}

func MessageTableNameByTs(ts int64) string {
	t := time.UnixMilli(ts)
	return MessageTableNameByDate(t)
}
