package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *Service
	jwt *JwtService
}

func NewHandler(svc *Service, jwt *JwtService) *Handler {
	return &Handler{svc: svc, jwt: jwt}
}

// ==================== request types ====================

type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type UpdateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

type SendMessageRequest struct {
	TargetPublicID string `json:"target_public_id" binding:"required"`
	Type           string `json:"type" binding:"required"`
	Content        string `json:"content" binding:"required"`
	Text           string `json:"text"`
	ContentType    string `json:"content_type"`
}

type CreateGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type UpdateGroupRequest struct {
	Name string `json:"name"`
}

// ==================== Auth handler ====================

func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_input"})
		return
	}

	userID, err := h.svc.CreateUser(req.Username, req.Password, req.Email)
	if err != nil {
		h.handleError(c, err)
		return
	}

	user, err := h.svc.GetUserByID(userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	deviceToken := c.GetHeader("X-Device-Token")
	token, err := h.jwt.GenerateToken(user, deviceToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "token generation failed"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"public_id": user.PublicID,
		"username":  user.Username,
		"email":     user.Email,
		"token":     token,
	})
}

func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_input"})
		return
	}

	user, err := h.svc.GetUserByUsername(req.Username)
	if err != nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Invalid username or password"})
		return
	}

	valid, err := h.svc.ValidateCredentials(req.Username, req.Password)
	if err != nil || !valid {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Invalid username or password"})
		return
	}

	deviceToken := c.GetHeader("X-Device-Token")
	token, err := h.jwt.GenerateToken(user, deviceToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "token generation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":     token,
		"public_id": user.PublicID,
		"message":   "Login successful",
	})
}

func (h *Handler) GetMe(c *gin.Context) {
	userID := c.GetInt64("user_id")
	user, err := h.svc.GetUserByID(userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":        user.ID,
			"username":  user.Username,
			"email":     user.Email,
			"public_id": user.PublicID,
		},
	})
}

func (h *Handler) UpdateUser(c *gin.Context) {
	publicID := c.Param("publicId")
	currentPublicID := c.GetString("public_id")
	if currentPublicID != publicID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "forbidden"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_input"})
		return
	}

	user, err := h.svc.GetUserByPublicID(publicID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Email != "" {
		user.Email = req.Email
	}

	if err := h.svc.UpdateUser(user); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"public_id": user.PublicID,
		"username":  user.Username,
		"email":     user.Email,
	})
}

func (h *Handler) DeleteUser(c *gin.Context) {
	publicID := c.Param("publicId")
	currentPublicID := c.GetString("public_id")
	if currentPublicID != publicID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "forbidden"})
		return
	}

	user, err := h.svc.GetUserByPublicID(publicID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	if err := h.svc.DeleteUser(user.ID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *Handler) Logout(c *gin.Context) {
	userID := c.GetInt64("user_id")
	deviceToken := c.GetHeader("X-Device-Token")

	if userID != 0 && deviceToken != "" {
		_, _ = h.svc.DeviceStore.ClearDeviceToken(userID, deviceToken)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Logged out successfully"})
}

// ==================== Message handler ====================

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
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, result)
}

func (h *Handler) GetMessages(c *gin.Context) {
	userID := c.GetInt64("user_id")

	convID := c.Query("convId")
	dateStr := c.Query("date")
	pageSize := 100
	offset := 0

	if v := c.Query("pageSize"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &pageSize); err != nil || n != 1 {
			pageSize = 100
		}
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

	messages, err := h.svc.GetConversationMessages(convID, date, pageSize, offset, userID)
	if err != nil {
		h.handleError(c, err)
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
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{Success: true, Message: "Message recalled successfully"})
}

// ==================== Group handler ====================

func (h *Handler) CreateGroup(c *gin.Context) {
	userID := c.GetInt64("user_id")
	publicID := c.GetString("public_id")

	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_input"})
		return
	}

	groupID, err := h.svc.CreateGroup(req.Name, req.Description, userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	if err := h.svc.AddMember(groupID, userID, "owner"); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"group_id":       groupID,
		"name":           req.Name,
		"owner_public_id": publicID,
	})
}

func (h *Handler) GetGroup(c *gin.Context) {
	id := c.Param("id")

	group, err := h.svc.GetGroupByID(id)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id": group.GroupID,
		"name":     group.Name,
		"owner_id": group.OwnerID,
	})
}

func (h *Handler) UpdateGroup(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetInt64("user_id")

	var req UpdateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_input"})
		return
	}

	group, err := h.svc.GetGroupByID(id)
	if err != nil {
		h.handleError(c, err)
		return
	}

	if group.OwnerID != userID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Only group owner can update group"})
		return
	}

	if req.Name != "" {
		group.Name = req.Name
	}

	if err := h.svc.UpdateGroup(group); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"group_id": group.GroupID,
		"name":     group.Name,
	})
}

func (h *Handler) DeleteGroup(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetInt64("user_id")

	group, err := h.svc.GetGroupByID(id)
	if err != nil {
		h.handleError(c, err)
		return
	}

	if group.OwnerID != userID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Only group owner can delete group"})
		return
	}

	if err := h.svc.DeleteGroup(id); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *Handler) JoinGroup(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetInt64("user_id")

	if err := h.svc.AddMember(id, userID, "member"); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully joined group"})
}

func (h *Handler) LeaveGroup(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetInt64("user_id")

	if err := h.svc.RemoveMember(id, userID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully left group"})
}

// ==================== Friend handler ====================

func (h *Handler) SendFriendRequest(c *gin.Context) {
	userID := c.GetInt64("user_id")
	publicID := c.GetString("public_id")
	targetPublicID := c.Param("targetPublicId")

	targetUser, err := h.svc.GetUserByPublicID(targetPublicID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Target user not found"})
		return
	}

	requestID, autoAccepted, err := h.svc.SendFriendRequest(userID, targetUser.ID, "")
	if err != nil {
		h.handleError(c, err)
		return
	}

	status := "pending"
	if autoAccepted {
		status = "auto_accepted"
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":               requestID,
		"status":           status,
		"sender_public_id": publicID,
		"receiver_public_id": targetPublicID,
	})
}

func (h *Handler) GetFriendRequests(c *gin.Context) {
	userID := c.GetInt64("user_id")

	sent, err := h.svc.GetSentRequests(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get requests"})
		return
	}

	received, err := h.svc.GetReceivedRequests(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get requests"})
		return
	}

	formatRequest := func(r *FriendRequest) gin.H {
		return gin.H{
			"id":          r.ID,
			"sender_id":   r.FromUserID,
			"receiver_id": r.ToUserID,
			"status":      r.Status,
			"ts":          r.CreatedAt,
		}
	}

	sentList := make([]gin.H, len(sent))
	for i, r := range sent {
		sentList[i] = formatRequest(r)
	}

	receivedList := make([]gin.H, len(received))
	for i, r := range received {
		receivedList[i] = formatRequest(r)
	}

	c.JSON(http.StatusOK, gin.H{
		"sent":     sentList,
		"received": receivedList,
	})
}

func (h *Handler) AcceptFriendRequest(c *gin.Context) {
	var requestID int64
	if _, err := fmt.Sscanf(c.Param("requestId"), "%d", &requestID); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_request_id"})
		return
	}
	userID := c.GetInt64("user_id")

	if err := h.svc.AcceptFriendRequest(requestID, userID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Friend request accepted"})
}

func (h *Handler) RejectFriendRequest(c *gin.Context) {
	var requestID int64
	if _, err := fmt.Sscanf(c.Param("requestId"), "%d", &requestID); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_request_id"})
		return
	}
	userID := c.GetInt64("user_id")

	if err := h.svc.RejectFriendRequest(requestID, userID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Friend request rejected"})
}

func (h *Handler) CancelFriendRequest(c *gin.Context) {
	var requestID int64
	if _, err := fmt.Sscanf(c.Param("requestId"), "%d", &requestID); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid_request_id"})
		return
	}
	userID := c.GetInt64("user_id")

	if err := h.svc.CancelFriendRequest(requestID, userID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// ==================== Error handler ====================

func (h *Handler) handleError(c *gin.Context, err error) {
	if appErr, ok := err.(*AppError); ok {
		switch appErr.Code {
		case "not_found":
			c.JSON(http.StatusNotFound, ErrorResponse{Error: appErr.Message})
		case "forbidden":
			c.JSON(http.StatusForbidden, ErrorResponse{Error: appErr.Message})
		case "unauthorized", "invalid_credentials":
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: appErr.Message})
		case "already_exists", "conflict", "self_request":
			c.JSON(http.StatusConflict, ErrorResponse{Error: appErr.Message})
		case "rate_limit_exceeded":
			c.JSON(http.StatusTooManyRequests, ErrorResponse{Error: appErr.Message})
		case "invalid_input", "invalid_type", "invalid_target_id", "not_pending", "not_member":
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: appErr.Message})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		}
		return
	}

	c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
}
