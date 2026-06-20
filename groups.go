package main

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"math/rand"
	"net/http"
	"sync"
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

// CreateGroup 创建群组
// @Summary      创建群组
// @Description  创建一个新的群组，创建者自动成为群主
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        request  body  CreateGroupRequest  true  "创建群组请求"
// @Success      201  {object}  map[string]interface{}  "创建成功"
// @Failure      400  {object}  ErrorResponse  "输入错误"
// @Router       /api/groups [post]
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

// GetGroup 获取群组详情
// @Summary      获取群组详情
// @Description  根据群组 ID 或群名获取群组信息和成员列表
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id  path  string  true  "群组 ID"
// @Success      200  {object}  map[string]interface{}  "群组详情"
// @Failure      404  {object}  ErrorResponse  "群组不存在"
// @Router       /api/groups/{id} [get]
func (h *Handler) GetGroup(c *gin.Context) {
	id := c.Param("id")

	group, err := h.svc.GetGroupByID(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	ownerUser, err := h.svc.GetUserByID(c.Request.Context(), group.OwnerID)
	if err != nil {
		handleError(c, err)
		return
	}

	members, err := h.svc.groupMemStore.GetMembersByGroup(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}

	// 收集所有成员的 user_id（而非public_id，因为后者仅仅是给别人看的）
	memberIDs := make([]int64, len(members))
	for i, m := range members {
		memberIDs[i] = m.UID
	}

	usersMap, err := h.svc.GetUsersByIDs(c.Request.Context(), memberIDs)
	if err != nil {
		handleError(c, err)
		return
	}

	memberList := make([]gin.H, 0, len(members))
	for _, m := range members {
		user, ok := usersMap[m.UID]
		if !ok {
			continue
		}
		memberList = append(memberList, gin.H{
			"public_id": user.PublicID,
			"username":  user.Username,
			"role":      m.Role,
		})
	}

	createdAt := time.UnixMilli(group.CreatedAt).UTC().Format(time.RFC3339)

	c.JSON(http.StatusOK, gin.H{
		"group_id":    group.GroupID,
		"name":        group.Name,
		"description": group.Description,
		"owner_id":    ownerUser.PublicID,
		"members":     memberList,
		"created_at":  createdAt,
	})
}

// UpdateGroup 更新群组信息
// @Summary      更新群组
// @Description  更新指定群组的名称等信息（仅群主可操作）
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id      path  string              true  "群组 ID"
// @Param        request body  UpdateGroupRequest  true  "更新群组请求"
// @Success      200  {object}  map[string]interface{}  "更新成功"
// @Failure      400  {object}  ErrorResponse  "输入错误"
// @Failure      403  {object}  ErrorResponse  "无权限，仅群主可操作"
// @Failure      404  {object}  ErrorResponse  "群组不存在"
// @Router       /api/groups/{id} [put]
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

// DeleteGroup 删除群组
// @Summary      删除群组
// @Description  删除指定群组（仅群主可操作）
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id  path  string  true  "群组 ID"
// @Success      204  {object}  map[string]interface{}  "删除成功"
// @Failure      403  {object}  ErrorResponse  "无权限，仅群主可操作"
// @Failure      404  {object}  ErrorResponse  "群组不存在"
// @Router       /api/groups/{id} [delete]
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

// GetGroupMemberCount 获取群组成员数量
// @Summary      获取群组成员数量
// @Description  获取指定群组的成员数量
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id  path  string  true  "群组 ID"
// @Success      200  {object}  map[string]interface{}  "成员数量"
// @Router       /api/groups/{id}/members/count [get]
func (h *Handler) GetGroupMemberCount(c *gin.Context) {
	id := c.Param("id")
	count, err := h.svc.GetGroupMemberCount(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"group_id": id, "count": count})
}

// JoinGroup 加入群组
// @Summary      加入群组
// @Description  当前用户加入指定群组
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id  path  string  true  "群组 ID"
// @Success      200  {object}  SuccessResponse  "加入成功"
// @Failure      404  {object}  ErrorResponse  "群组不存在"
// @Router       /api/groups/{id}/join [post]
func (h *Handler) JoinGroup(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetInt64("user_id")

	if err := h.svc.AddMember(c.Request.Context(), id, userID, "member"); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully joined group"})
}

// LeaveGroup 退出群组
// @Summary      退出群组
// @Description  当前用户退出指定群组（群主不可退出）
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id  path  string  true  "群组 ID"
// @Success      200  {object}  SuccessResponse  "退出成功"
// @Failure      404  {object}  ErrorResponse  "群组不存在"
// @Router       /api/groups/{id}/leave [post]
func (h *Handler) LeaveGroup(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetInt64("user_id")

	if err := h.svc.RemoveMember(c.Request.Context(), id, userID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully left group"})
}

// KickMember 踢出群成员
// @Summary      踢出群成员
// @Description  群主将指定成员踢出群组
// @Tags         groups
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        id              path  string  true  "群组 ID"
// @Param        memberPublicId  path  string  true  "成员公钥 ID"
// @Success      200  {object}  SuccessResponse  "踢出成功"
// @Failure      403  {object}  ErrorResponse  "无权限"
// @Failure      404  {object}  ErrorResponse  "群组或成员不存在"
// @Router       /api/groups/{id}/members/{memberPublicId} [delete]
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
		return // 聊天记录就让他随波逐流得了
	}

	c.JSON(http.StatusOK, gin.H{"message": "Member kicked successfully"})
}

func (s *Service) GetGroupByID(ctx context.Context, id string) (*Group, error) {
	// 先按 groupID 查
	group, err := s.groupStore.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if group != nil {
		return group, nil
	}
	// 没找到，按群名查
	return checkFound(s.groupStore.GetByName(ctx, id))
}

func (s *Service) GetGroupsByOwner(ctx context.Context, ownerID int64) ([]*Group, error) {
	return s.groupStore.GetGroupsByOwner(ctx, ownerID)
}

// 这里面一部分原先是直接SQL，后面挪到store里面了
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

	group := &Group{
		GroupID:     groupID,
		Name:        name,
		Description: description,
		OwnerID:     ownerID,
		CreatedAt:   now,
		UpdatedAt:   now,
		IsDeleted:   0,
	}
	_, err = s.groupStore.Insert(context.Background(), tx, group)
	if err != nil {
		return "", err
	}

	member := &GroupMember{
		GroupID:  groupID,
		UID:      ownerID,
		Role:     "owner",
		JoinedAt: now,
	}
	_, err = s.groupMemStore.Insert(context.Background(), tx, member)
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

	if _, err := s.groupStore.Delete(context.Background(), tx, groupID); err != nil {
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

	id, err := s.groupMemStore.Insert(ctx, s.groupMemStore.DB(), member)
	if err != nil {
		return err
	}
	if id == 0 {
		return ErrConflict
	}

	goSafe(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.SendNotificationToUser(ctx, userID, NotifGroupInvite, NotifData{"group_id": groupID, "group_name": group.Name, "inviter_uid": group.OwnerID, "action": "invited"})
	})

	return nil
}

func (s *Service) RemoveMember(ctx context.Context, groupID string, userID int64) error {
	tx, err := s.groupMemStore.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	member, err := s.groupMemStore.Get(context.Background(), tx, groupID, userID)
	if err != nil {
		return err
	}
	if member == nil {
		return ErrNotMember
	}
	if member.Role == "owner" {
		return ErrForbidden
	}

	_, err = s.groupMemStore.DeleteByGroupAndUser(context.Background(), tx, groupID, userID)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
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

	isMember, err := s.groupMemStore.IsMember(context.Background(), tx, groupID, memberID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrNotMember
	}

	_, err = s.groupMemStore.DeleteByGroupAndUser(context.Background(), tx, groupID, memberID)
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

	_, err = s.sendSystemMessageTx(ctx, tx, convID, contentBytes)
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
	return s.groupMemStore.IsMember(ctx, s.groupMemStore.DB(), groupID, userID)
}

func (s *Service) GetGroupMemberCount(ctx context.Context, groupID string) (int, error) {
	return s.groupMemStore.CountByGroup(ctx, groupID)
}

var seededRand = rand.New(&lockedSource{src: rand.NewSource(time.Now().UnixNano())})

type lockedSource struct {
	mu sync.Mutex
	src rand.Source
}

func (s *lockedSource) Int63() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.src.Int63()
}

func (s *lockedSource) Seed(seed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.src.Seed(seed)
}

func (s *Service) GenerateGroupID() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	charsLen := big.NewInt(int64(len(chars)))
	length := s.cfg.GroupIDRandomLength
	if length <= 0 {
		length = 8
	}
	buf := make([]byte, 2+length)
	buf[0], buf[1] = 'g', '_'
	for i := 0; i < length; i++ {
		n, err := crand.Int(crand.Reader, charsLen)
		if err != nil {
			// 不可达：crypto/rand 几乎不可能失败
			// 作为保护，退回到 math/rand 带锁的种子随机
			n = big.NewInt(int64(seededRand.Intn(len(chars))))
		}
		buf[2+i] = chars[n.Int64()]
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
		return convID[2:] // rm "g_" prefix
	}
	return ""
}