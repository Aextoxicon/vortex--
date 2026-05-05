package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

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
		handleError(c, err)
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
		handleError(c, err)
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
		handleError(c, err)
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
		handleError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (s *Service) SendFriendRequest(fromUserID, toUserID int64, message string) (requestID int64, autoAccepted bool, err error) {
	if fromUserID == toUserID {
		return 0, false, ErrSelfRequest
	}

	_, err = s.GetUserByID(toUserID)
	if err != nil {
		return 0, false, err
	}

	existing, err := s.friendStore.GetByUsers(fromUserID, toUserID)
	if err != nil {
		return 0, false, err
	}
	if existing != nil {
		return 0, false, ErrConflict
	}

	reverse, err := s.friendStore.GetByUsers(toUserID, fromUserID)
	if err != nil {
		return 0, false, err
	}
	if reverse != nil && reverse.Status == "pending" {
		reverse.Status = "accepted"
		reverse.UpdatedAt = time.Now().UnixMilli()
		if _, err := s.friendStore.Update(reverse); err != nil {
			return 0, false, err
		}
		_ = s.SendFriendAcceptedNotification(fromUserID, toUserID, fmt.Sprintf("%d", reverse.ID))
		return 0, true, nil
	}

	req := &FriendRequest{
		FromUserID: fromUserID,
		ToUserID:   toUserID,
		Message:    message,
		Status:     "pending",
		CreatedAt:  time.Now().UnixMilli(),
		UpdatedAt:  time.Now().UnixMilli(),
	}

	result, err := s.friendStore.Insert(req)
	if err != nil {
		return 0, false, err
	}

	goSafe(func() {
		_ = s.SendFriendNotification(toUserID, fromUserID, fmt.Sprintf("%d", result))
	})

	return result, false, nil
}

func (s *Service) GetFriendRequestByID(requestID int64) (*FriendRequest, error) {
	req, err := s.friendStore.GetByID(requestID)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, ErrNotFound
	}
	return req, nil
}

func (s *Service) GetSentRequests(fromUserID int64) ([]*FriendRequest, error) {
	return s.friendStore.GetSentRequests(fromUserID)
}

func (s *Service) GetReceivedRequests(toUserID int64) ([]*FriendRequest, error) {
	return s.friendStore.GetReceivedRequests(toUserID)
}

func (s *Service) AcceptFriendRequest(requestID, userID int64) error {
	req, err := s.friendStore.GetByID(requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return ErrNotFound
	}
	if req.ToUserID != userID {
		return ErrForbidden
	}
	if req.Status != "pending" {
		return ErrNotPending
	}

	req.Status = "accepted"
	req.UpdatedAt = time.Now().UnixMilli()
	_, err = s.friendStore.Update(req)
	if err != nil {
		return err
	}

	goSafe(func() {
		_ = s.SendFriendAcceptedNotification(req.FromUserID, req.ToUserID, fmt.Sprintf("%d", req.ID))
	})

	return nil
}

func (s *Service) RejectFriendRequest(requestID, userID int64) error {
	req, err := s.friendStore.GetByID(requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return ErrNotFound
	}
	if req.ToUserID != userID {
		return ErrForbidden
	}
	if req.Status != "pending" {
		return ErrNotPending
	}

	req.Status = "rejected"
	req.UpdatedAt = time.Now().UnixMilli()
	_, err = s.friendStore.Update(req)
	return err
}

func (s *Service) CancelFriendRequest(requestID, fromUserID int64) error {
	req, err := s.friendStore.GetByID(requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return ErrNotFound
	}
	if req.FromUserID != fromUserID {
		return ErrForbidden
	}
	if req.Status != "pending" {
		return ErrNotPending
	}

	_, err = s.friendStore.Delete(requestID)
	return err
}

func (s *Service) DeleteFriendRequestsByUser(tx *sql.Tx, userID int64) error {
	_, err := s.friendStore.DeleteByUserTx(tx, userID)
	return err
}

func PrivateConvID(uid1, uid2 int64) string {
	if uid1 < uid2 {
		return fmt.Sprintf("p_%d_%d", uid1, uid2)
	}
	return fmt.Sprintf("p_%d_%d", uid2, uid1)
}

func IsPrivateConv(convID string) bool {
	return len(convID) > 0 && convID[0] == 'p'
}

func ParsePrivateConv(convID string) (int64, int64, error) {
	if !IsPrivateConv(convID) {
		return 0, 0, errors.New("not a private conversation")
	}
	var a, b int64
	if _, err := fmt.Sscanf(convID, "p_%d_%d", &a, &b); err != nil {
		return 0, 0, err
	}
	return a, b, nil
}

func CanAccessPrivateConv(convID string, uid int64) bool {
	a, b, err := ParsePrivateConv(convID)
	if err != nil {
		return false
	}
	return a == uid || b == uid
}
