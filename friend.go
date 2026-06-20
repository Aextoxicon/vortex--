package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

// GetPendingFriendRequests 获取待处理的好友请求
// @Summary      获取待处理的好友请求
// @Description  获取当前用户收到但尚未处理的好友请求列表
// @Tags         friends
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Success      200  {object}  map[string]interface{}  "待处理请求列表"
// @Router       /api/friends/requests/pending [get]
func (h *Handler) GetPendingFriendRequests(c *gin.Context) {
	userID := c.GetInt64("user_id")
	requests, err := h.svc.friendStore.GetPendingRequests(c.Request.Context(), userID)
	if err != nil {
		handleError(c, ErrGetRequestsFailed)
		return
	}
	list := make([]gin.H, len(requests))
	for i, r := range requests {
		list[i] = gin.H{
			"id":          r.ID,
			"sender_id":   r.FromUserID,
			"receiver_id": r.ToUserID,
			"status":      r.Status,
			"ts":          r.CreatedAt,
		}
	}
	c.JSON(http.StatusOK, gin.H{"requests": list})
}

// SendFriendRequest 发送好友请求
// @Summary      发送好友请求
// @Description  向指定用户发送好友请求
// @Tags         friends
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        targetPublicId  path  string  true  "目标用户公钥 ID"
// @Success      201  {object}  map[string]interface{}  "请求发送成功"
// @Failure      404  {object}  ErrorResponse  "用户不存在"
// @Failure      409  {object}  ErrorResponse  "请求冲突"
// @Router       /api/friends/request/send/{targetPublicId} [post]
func (h *Handler) SendFriendRequest(c *gin.Context) {
	userID := c.GetInt64("user_id")
	publicID := c.GetString("public_id")
	targetPublicID := c.Param("targetPublicId")

	targetUser, err := h.svc.GetUserByPublicID(c.Request.Context(), targetPublicID)
	if err != nil {
		handleError(c, ErrNotFound)
		return
	}

	requestID, autoAccepted, err := h.svc.SendFriendRequest(c.Request.Context(), userID, targetUser.ID, "")
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

// GetFriendRequests 获取好友请求列表
// @Summary      获取好友请求列表
// @Description  获取当前用户发送和收到的所有好友请求
// @Tags         friends
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Success      200  {object}  map[string]interface{}  "请求列表"
// @Router       /api/friends/requests [get]
func (h *Handler) GetFriendRequests(c *gin.Context) {
	userID := c.GetInt64("user_id")

	sent, err := h.svc.GetSentRequests(c.Request.Context(), userID)
	if err != nil {
		handleError(c, ErrGetRequestsFailed)
		return
	}

	received, err := h.svc.GetReceivedRequests(c.Request.Context(), userID)
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

// AcceptFriendRequest 接受好友请求
// @Summary      接受好友请求
// @Description  接受指定的好友请求
// @Tags         friends
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        requestId  path  int  true  "请求 ID"
// @Success      200  {object}  SuccessResponse  "接受成功"
// @Failure      400  {object}  ErrorResponse  "请求无效"
// @Failure      403  {object}  ErrorResponse  "无权限"
// @Failure      404  {object}  ErrorResponse  "请求不存在"
// @Router       /api/friends/request/{requestId}/accept [post]
func (h *Handler) AcceptFriendRequest(c *gin.Context) {
	requestID, err := strconv.ParseInt(c.Param("requestId"), 10, 64)
	if err != nil {
		handleError(c, ErrInvalidInput)
		return
	}
	userID := c.GetInt64("user_id")

	if err := h.svc.AcceptFriendRequest(c.Request.Context(), requestID, userID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Friend request accepted"})
}

// RejectFriendRequest 拒绝好友请求
// @Summary      拒绝好友请求
// @Description  拒绝指定的好友请求
// @Tags         friends
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        requestId  path  int  true  "请求 ID"
// @Success      200  {object}  SuccessResponse  "拒绝成功"
// @Failure      400  {object}  ErrorResponse  "请求无效"
// @Failure      404  {object}  ErrorResponse  "请求不存在"
// @Router       /api/friends/request/{requestId}/reject [post]
func (h *Handler) RejectFriendRequest(c *gin.Context) {
	requestID, err := strconv.ParseInt(c.Param("requestId"), 10, 64)
	if err != nil {
		handleError(c, ErrInvalidInput)
		return
	}
	userID := c.GetInt64("user_id")

	if err := h.svc.RejectFriendRequest(c.Request.Context(), requestID, userID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Friend request rejected"})
}

// CancelFriendRequest 取消好友请求
// @Summary      取消好友请求
// @Description  取消自己发送的好友请求
// @Tags         friends
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        requestId  path  int  true  "请求 ID"
// @Success      204  {object}  map[string]interface{}  "取消成功"
// @Failure      400  {object}  ErrorResponse  "请求无效"
// @Failure      404  {object}  ErrorResponse  "请求不存在"
// @Router       /api/friends/request/{requestId} [delete]
func (h *Handler) CancelFriendRequest(c *gin.Context) {
	requestID, err := strconv.ParseInt(c.Param("requestId"), 10, 64)
	if err != nil {
		handleError(c, ErrInvalidInput)
		return
	}
	userID := c.GetInt64("user_id")

	if err := h.svc.CancelFriendRequest(c.Request.Context(), requestID, userID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (s *Service) SendFriendRequest(ctx context.Context, fromUserID, toUserID int64, message string) (requestID int64, autoAccepted bool, err error) {
	if fromUserID == toUserID {
		return 0, false, ErrSelfRequest
	}

	_, err = s.GetUserByID(ctx, toUserID)
	if err != nil {
		return 0, false, err
	}

	tx, err := s.friendStore.DB().BeginTx(ctx, nil)
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
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.SendNotificationToUser(ctx, fromUserID, NotifFriendRequest, NotifData{"request_id": fmt.Sprintf("%d", reverseID), "receiver_uid": toUserID, "action": "accepted"})
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

	result, err := s.friendStore.Insert(context.Background(), tx, req)
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.SendNotificationToUser(ctx, toUserID, NotifFriendRequest, NotifData{"request_id": fmt.Sprintf("%d", result), "sender_uid": fromUserID, "action": "received"})
	})

	return result, false, nil
}

func isUniqueViolation(err error) bool {
	if pqErr, ok := err.(*pq.Error); ok {
		return pqErr.Code == "23505"
	}
	return false
}

func (s *Service) GetSentRequests(ctx context.Context, fromUserID int64) ([]*FriendRequest, error) {
	return s.friendStore.GetSentRequests(ctx, fromUserID)
}

func (s *Service) GetReceivedRequests(ctx context.Context, toUserID int64) ([]*FriendRequest, error) {
	return s.friendStore.GetReceivedRequests(ctx, toUserID)
}

func (s *Service) AcceptFriendRequest(ctx context.Context, requestID, userID int64) error {
	tx, err := s.friendStore.DB().BeginTx(ctx, nil)
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.SendNotificationToUser(ctx, fromUserID, NotifFriendRequest, NotifData{"request_id": fmt.Sprintf("%d", requestID), "receiver_uid": userID, "action": "accepted"})
	})

	return nil
}

func (s *Service) RejectFriendRequest(ctx context.Context, requestID, userID int64) error {
	tx, err := s.friendStore.DB().BeginTx(ctx, nil)
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

// 要不要把取消等同于拒绝呢，哦baby不要拒绝我~
func (s *Service) CancelFriendRequest(ctx context.Context, requestID, fromUserID int64) error {
	tx, err := s.friendStore.DB().BeginTx(ctx, nil)
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

// 生成私聊会话 ID，格式为 "p_{较小的 publicID}_{较大的 publicID}"
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
	// 按第一个下划线分割，剩余部分再按最后一个下划线分割
	if len(convID) < 3 || convID[0] != 'p' || convID[1] != '_' {
		return "", "", errors.New("invalid private conversation format")
	}

	rest := convID[2:] // 跳过 "p_"
	lastUnderscore := -1
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == '_' {
			lastUnderscore = i
			break
		}
	}

	if lastUnderscore == -1 {
		return "", "", errors.New("invalid private conversation format")
	}

	a := rest[:lastUnderscore]
	b := rest[lastUnderscore+1:]
	return a, b, nil
}

func CanAccessPrivateConv(convID string, publicID string) bool {
	a, b, err := ParsePrivateConv(convID)
	if err != nil {
		return false
	}
	return a == publicID || b == publicID
}

// 这一段说不定可能会丢到shared里面，但是收益不明显
func ExtractConversationType(convID string) string {
	if len(convID) == 0 {
		return ""
	}
	if convID[0] == 'p' {
		return "p"
	}
	if convID[0] == 'g' {
		return "g"
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
