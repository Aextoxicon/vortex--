package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

func (h *Handler) SendFriendRequest(c *gin.Context) {
	userID := c.GetInt64("user_id")
	publicID := c.GetString("public_id")
	targetPublicID := c.Param("targetPublicId")

	targetUser, err := h.svc.GetUserByPublicID(targetPublicID)
	if err != nil {
		handleError(c, ErrNotFound)
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
		"id":                 requestID,
		"status":             status,
		"sender_public_id":   publicID,
		"receiver_public_id": targetPublicID,
	})
}

func (h *Handler) GetFriendRequests(c *gin.Context) {
	userID := c.GetInt64("user_id")

	sent, err := h.svc.GetSentRequests(userID)
	if err != nil {
		handleError(c, ErrGetRequestsFailed)
		return
	}

	received, err := h.svc.GetReceivedRequests(userID)
	if err != nil {
		handleError(c, ErrGetRequestsFailed)
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
	requestID, err := strconv.ParseInt(c.Param("requestId"), 10, 64)
	if err != nil {
		handleError(c, ErrInvalidInput)
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
	requestID, err := strconv.ParseInt(c.Param("requestId"), 10, 64)
	if err != nil {
		handleError(c, ErrInvalidInput)
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
	requestID, err := strconv.ParseInt(c.Param("requestId"), 10, 64)
	if err != nil {
		handleError(c, ErrInvalidInput)
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

	tx, err := s.friendStore.DB().Begin()
	if err != nil {
		return 0, false, err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	reverseID, err := s.friendStore.AcceptPendingTx(tx, toUserID, fromUserID)
	if err != nil {
		return 0, false, err
	}
	if reverseID > 0 {
		if err := tx.Commit(); err != nil {
			return 0, false, err
		}
		tx = nil
		goSafe(func() {
			_ = s.SendFriendAcceptedNotification(fromUserID, toUserID, fmt.Sprintf("%d", reverseID))
		})
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
		if isUniqueViolation(err) {
			return 0, false, ErrConflict
		}
		return 0, false, err
	}

	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	tx = nil

	goSafe(func() {
		_ = s.SendFriendNotification(toUserID, fromUserID, fmt.Sprintf("%d", result))
	})

	return result, false, nil
}

func isUniqueViolation(err error) bool {
	if pqErr, ok := err.(*pq.Error); ok {
		return pqErr.Code == "23505"
	}
	return false
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
	tx, err := s.friendStore.DB().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	fromUserID, err := s.friendStore.AcceptByIDTx(tx, requestID, userID)
	if err != nil {
		return err
	}
	if fromUserID == 0 {
		return ErrNotFound
	}
	if fromUserID < 0 {
		return ErrForbidden
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil

	goSafe(func() {
		_ = s.SendFriendAcceptedNotification(fromUserID, userID, fmt.Sprintf("%d", requestID))
	})

	return nil
}

func (s *Service) RejectFriendRequest(requestID, userID int64) error {
	tx, err := s.friendStore.DB().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	affected, err := s.friendStore.RejectTx(tx, requestID, userID)
	if err != nil {
		return err
	}
	if !affected {
		return ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (s *Service) CancelFriendRequest(requestID, fromUserID int64) error {
	tx, err := s.friendStore.DB().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	affected, err := s.friendStore.CancelTx(tx, requestID, fromUserID)
	if err != nil {
		return err
	}
	if !affected {
		return ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (s *Service) DeleteFriendRequestsByUser(tx *sql.Tx, userID int64) error {
	_, err := s.friendStore.DeleteByUserTx(tx, userID)
	return err
}

func PrivateConvID(publicID1, publicID2 string) string {
	if publicID1 < publicID2 {
		return fmt.Sprintf("p_%s_%s", publicID1, publicID2)
	}
	return fmt.Sprintf("p_%s_%s", publicID2, publicID1)
}

func IsPrivateConv(convID string) bool {
	return len(convID) > 0 && convID[0] == 'p'
}

func ParsePrivateConv(convID string) (string, string, error) {
	if !IsPrivateConv(convID) {
		return "", "", errors.New("not a private conversation")
	}
	var a, b string
	if _, err := fmt.Sscanf(convID, "p_%s_%s", &a, &b); err != nil {
		return "", "", err
	}
	return a, b, nil
}

func CanAccessPrivateConv(convID string, publicID string) bool {
	a, b, err := ParsePrivateConv(convID)
	if err != nil {
		return false
	}
	return a == publicID || b == publicID
}

func ExtractConversationType(convID string) string {
	if len(convID) == 0 {
		return ""
	}
	if convID[0] == 'p' {
		return "private"
	}
	if convID[0] == 'g' {
		return "group"
	}
	return ""
}

func GetOtherPublicID(convID string, myPublicID string) string {
	if !IsPrivateConv(convID) {
		return ""
	}
	a, b, err := ParsePrivateConv(convID)
	if err != nil {
		return ""
	}
	if a == myPublicID {
		return b
	}
	if b == myPublicID {
		return a
	}
	return ""
}
