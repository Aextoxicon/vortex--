package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_input"})
		return
	}

	content := req.Content
	if req.ContentType == "image" {
		img := ImageContent{Type: "image", Content: req.Content, Text: req.Text}
		data, _ := json.Marshal(img)
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
	pageSize := Cfg.DefaultPageSize
	offset := 0

	if v := c.Query("days"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &days); err != nil || n != 1 {
			days = 1
		}
	}
	if days < 1 {
		days = 1
	}
	if days > 7 {
		days = 7
	}

	if v := c.Query("pageSize"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &pageSize); err != nil || n != 1 {
			pageSize = Cfg.DefaultPageSize
		}
	}
	if pageSize > Cfg.MaxPageSize {
		pageSize = Cfg.MaxPageSize
	}
	if v := c.Query("offset"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &offset); err != nil || n != 1 {
			offset = 0
		}
	}

	var date time.Time
	if dateStr != "" {
		var err error
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid date format, use YYYY-MM-DD"})
			return
		}
	} else {
		date = time.Now()
	}

	messages, err := h.svc.GetConversationMessages(convID, date, days, pageSize, offset, userID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

func (h *Handler) RecallMessage(c *gin.Context) {
	userID := c.GetInt64("user_id")

	var msgID int64
	if _, err := fmt.Sscanf(c.Param("msgId"), "%d", &msgID); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid msg_id"})
		return
	}

	var msgTimestamp int64
	tsStr := c.Query("msgTimestamp")
	if tsStr == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "msgTimestamp is required"})
		return
	}
	if _, err := fmt.Sscanf(tsStr, "%d", &msgTimestamp); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid msgTimestamp"})
		return
	}

	if err := h.svc.RecallMessage(msgID, msgTimestamp, userID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{Success: true, Message: "Message recalled successfully"})
}

func (s *Service) SendMessage(currentUser *User, targetPublicID, msgType, content string) (*SendMessageResult, error) {
	uid := currentUser.ID

	convID, err := s.generateConversationID(msgType, uid, targetPublicID)
	if err != nil {
		return nil, err
	}

	if err := s.ensureSessionPermission(uid, convID, msgType, targetPublicID); err != nil {
		return nil, err
	}

	msgID, err := s.idGen.GenerateID()
	if err != nil {
		return nil, err
	}

	ts := time.Now().UnixMilli() - Cfg.EpochTime
	tableName := MessageTableNameByTs(ts)

	msg := &Message{
		MsgID:   msgID,
		ConvID:  convID,
		FromUID: uid,
		Content: content,
		Ts:      ts,
	}

	_, err = s.msgStore.InsertMessage(tableName, msg)
	if err != nil {
		return nil, err
	}

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

func (s *Service) GetConversationMessages(convID string, endDate time.Time, days, pageSize, offset int, userID int64) ([]*Message, error) {
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

	var allMsgs []*Message
	startDate := endDate.AddDate(0, 0, -(days - 1))

	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		tableName := MessageTableNameByDate(d)
		msgs, err := s.msgStore.GetConversationMessages(tableName, convID, pageSize+offset, 0)
		if err != nil {
			continue
		}
		allMsgs = append(allMsgs, msgs...)
	}

	sort.Slice(allMsgs, func(i, j int) bool {
		return allMsgs[i].Ts > allMsgs[j].Ts
	})

	if offset > 0 && offset < len(allMsgs) {
		allMsgs = allMsgs[offset:]
	} else if offset >= len(allMsgs) {
		allMsgs = nil
	}

	if len(allMsgs) > pageSize {
		allMsgs = allMsgs[:pageSize]
	}

	return allMsgs, nil
}

func (s *Service) RecallMessage(msgID, msgTimestamp, userID int64) error {
	now := time.Now().UnixMilli()
	msgAge := now - msgTimestamp
	if msgAge > Cfg.MessageRecallWindowMs {
		return ErrInvalidInput
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
		if !IsValidGroupID(targetPublicID) {
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
