package main

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type CreateGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type UpdateGroupRequest struct {
	Name string `json:"name"`
}

func (h *Handler) CreateGroup(c *gin.Context) {
	userID := c.GetInt64("user_id")
	publicID := c.GetString("public_id")

	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, ErrInvalidInput)
		return
	}

	if !validateGroupName(req.Name) {
		handleError(c, ErrInvalidGroupName)
		return
	}

	groupID, err := h.svc.CreateGroup(c.Request.Context(), req.Name, req.Description, userID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"group_id":        groupID,
		"name":            req.Name,
		"owner_public_id": publicID,
	})
}

func (h *Handler) GetGroup(c *gin.Context) {
	id := c.Param("id")

	group, err := h.svc.GetGroupByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
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
		handleError(c, ErrInvalidInput)
		return
	}

	group, err := h.svc.GetGroupByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	if group.OwnerID != userID {
		handleError(c, ErrForbidden)
		return
	}

	if req.Name != "" {
		group.Name = req.Name
	}

	if err := h.svc.UpdateGroup(c.Request.Context(), group); err != nil {
		handleError(c, err)
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

	group, err := h.svc.GetGroupByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	if group.OwnerID != userID {
		handleError(c, ErrForbidden)
		return
	}

	if err := h.svc.DeleteGroup(c.Request.Context(), id); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *Handler) JoinGroup(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetInt64("user_id")

	if err := h.svc.AddMember(c.Request.Context(), id, userID, "member"); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully joined group"})
}

func (h *Handler) LeaveGroup(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetInt64("user_id")

	if err := h.svc.RemoveMember(c.Request.Context(), id, userID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully left group"})
}

func (h *Handler) KickMember(c *gin.Context) {
	groupID := c.Param("id")
	memberPublicID := c.Param("memberPublicId")
	ownerID := c.GetInt64("user_id")

	memberUser, err := h.svc.GetUserByPublicID(c.Request.Context(), memberPublicID)
	if err != nil {
		handleError(c, ErrNotFound)
		return
	}
	if memberUser == nil {
		handleError(c, ErrNotFound)
		return
	}

	if err := h.svc.KickMember(c.Request.Context(), groupID, memberUser.ID, ownerID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Member kicked successfully"})
}

func (s *Service) GetGroupByID(ctx context.Context, groupID string) (*Group, error) {
	group, err := s.groupStore.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, ErrNotFound
	}
	return group, nil
}

func (s *Service) GetGroupByName(ctx context.Context, name string) (*Group, error) {
	group, err := s.groupStore.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, ErrNotFound
	}
	return group, nil
}

func (s *Service) GetGroupsByOwner(ctx context.Context, ownerID int64) ([]*Group, error) {
	return s.groupStore.GetGroupsByOwner(ctx, ownerID)
}

func (s *Service) CreateGroup(ctx context.Context, name, description string, ownerID int64) (string, error) {
	tx, err := s.groupStore.DB().BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	now := time.Now().UnixMilli()
	groupID := s.GenerateGroupID()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO groups (group_id, name, description, owner_id, created_at, updated_at, is_deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		groupID, name, description, ownerID, now, now, 0,
	)
	if err != nil {
		return "", err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO group_members (group_id, uid, role, joined_at)
		VALUES ($1, $2, $3, $4)`,
		groupID, ownerID, "owner", now,
	)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	tx = nil
	return groupID, nil
}

func (s *Service) UpdateGroup(ctx context.Context, group *Group) error {
	group.UpdatedAt = time.Now().UnixMilli()
	_, err := s.groupStore.Update(ctx, group)
	return err
}

func (s *Service) DeleteGroup(ctx context.Context, groupID string) error {
	tx, err := s.groupStore.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	if _, err := s.groupMemStore.DeleteByGroupTx(tx, groupID); err != nil {
		return err
	}

	if _, err := s.groupStore.Delete(ctx, groupID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (s *Service) AddMember(ctx context.Context, groupID string, userID int64, role string) error {
	group, err := s.groupStore.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return ErrNotFound
	}

	member := &GroupMember{
		GroupID:  groupID,
		UID:      userID,
		Role:     role,
		JoinedAt: time.Now().UnixMilli(),
	}

	id, err := s.groupMemStore.Insert(ctx, member)
	if err != nil {
		return err
	}
	if id == 0 {
		return ErrConflict
	}

	goSafe(func() {
		_ = s.SendGroupInviteNotification(context.Background(), userID, groupID, group.Name, group.OwnerID)
	})

	return nil
}

func (s *Service) RemoveMember(ctx context.Context, groupID string, userID int64) error {
	isMember, err := s.groupMemStore.IsMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotMember
	}

	member, err := s.groupMemStore.Get(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if member.Role == "owner" {
		return ErrForbidden
	}

	_, err = s.groupMemStore.DeleteByGroupAndUser(ctx, groupID, userID)
	return err
}

func (s *Service) KickMember(ctx context.Context, groupID string, memberID, ownerID int64) error {
	group, err := s.groupStore.GetByID(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil {
		return ErrNotFound
	}
	if group.OwnerID != ownerID {
		return ErrForbidden
	}

	if memberID == ownerID {
		return ErrForbidden
	}

	kickedUser, err := s.userStore.GetByID(ctx, memberID)
	if err != nil {
		return err
	}
	if kickedUser == nil {
		return ErrNotFound
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

	isMember, err := s.groupMemStore.IsMember(ctx, groupID, memberID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotMember
	}

	_, err = s.groupMemStore.DeleteByGroupAndUserTx(tx, groupID, memberID)
	if err != nil {
		return err
	}

	convID := "g_" + groupID
	systemMsg := map[string]interface{}{
		"type":    "system",
		"content": "member_kicked",
		"data": map[string]interface{}{
			"kicked_public_id": kickedUser.PublicID,
		},
	}

	contentBytes, err := json.Marshal(systemMsg)
	if err != nil {
		return fmt.Errorf("marshal system message failed: %w", err)
	}

	_, err = s.sendSystemMessageTx(tx, convID, contentBytes)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	slog.Info("KickMember",
		"group_id", groupID,
		"member_public_id", kickedUser.PublicID,
		"owner_id", ownerID,
	)

	return nil
}

func (s *Service) IsUserInGroup(ctx context.Context, groupID string, userID int64) (bool, error) {
	return s.groupMemStore.IsMember(ctx, groupID, userID)
}

func (s *Service) GetGroupMemberCount(ctx context.Context, groupID string) (int, error) {
	return s.groupMemStore.CountByGroup(ctx, groupID)
}

func (s *Service) DeleteGroupMembersByUser(ctx context.Context, tx *sql.Tx, userID int64) error {
	_, err := s.groupMemStore.DeleteByUserTx(tx, userID)
	return err
}

func (s *Service) GenerateGroupID() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	length := s.cfg.GroupIDRandomLength
	if length <= 0 {
		length = 8
	}
	bytes := make([]byte, length)
	if _, err := crand.Read(bytes); err != nil {
		for i := range bytes {
			bytes[i] = byte(i)
		}
	}
	buf := make([]byte, 2+length)
	buf[0], buf[1] = 'g', '_'
	for i := 0; i < length; i++ {
		buf[2+i] = chars[int(bytes[i])%len(chars)]
	}
	return string(buf)
}

func (s *Service) IsValidGroupID(groupID string) bool {
	expectedLen := 2 + s.cfg.GroupIDRandomLength
	if s.cfg.GroupIDRandomLength <= 0 {
		expectedLen = 10
	}
	return len(groupID) == expectedLen && groupID[:2] == "g_"
}

func IsGroupConv(convID string) bool {
	return len(convID) >= 2 && convID[:2] == "g_"
}

func ExtractGroupID(convID string) string {
	if IsGroupConv(convID) {
		return convID[2:] // 去掉 "g_" 前缀
	}
	return ""
}
