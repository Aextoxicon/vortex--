package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// 我不知道你是不是从这里开始看，不过我认为一开始定义好所有struct不是什么坏习惯，但是这一坨代码真的会有人去看吗？这个项目也没什么吸引人的
type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email"`
	Bio      string `json:"bio"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type UpdateUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Bio      string `json:"bio"`
}

// 这个我需要特意说明一下，用户名允许中文和下划线，长度限制在3-20个字符之间，邮箱则必须符合标准格式且长度不超过100个字符，群组名称允许中文、英文、数字、空格、下划线和连字符，长度限制在1-50个字符之间。密码必须至少8个字符，且包含大小写字母、数字和特殊字符
// 这些规则虽然看起来有点严格，但我觉得对于一个聊天应用来说是合理的，可以有效防止一些奇奇怪怪的破事情，反正go改改再编译成本又不大
// 其实我在考虑会不会有人往里面塞一些Unicode字符，比如说表情包之类的，但为了保险起见，我还是限制一下吧，但是如果要改，就像上句话一样
// 什么你觉得这一段像入机生成的而不是一个真正的人写出来的？呜呜呜我好难过...
var (
	usernameRegex  = regexp.MustCompile(`^[a-zA-Z0-9_\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}]{3,20}$`)
	emailRegex     = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	groupNameRegex = regexp.MustCompile(`^[\w\s\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}_-]{1,50}$`)
)

func validatePassword(password string) bool {
	if len(password) < 8 || len(password) > 128 {
		return false
	}

	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSpecial := false

	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '!' || r == '@' || r == '#' || r == '$' || r == '%' || r == '^' || r == '&' || r == '*' || r == '(' || r == ')':
			hasSpecial = true
		}
	}

	return hasLower && hasUpper && hasDigit && hasSpecial
}

func validateUsername(username string) bool {
	return usernameRegex.MatchString(username)
}

func validateEmail(email string) bool {
	if email == "" {
		return true
	}
	return emailRegex.MatchString(email) && len(email) <= 100 // md这年头我就不信有人的Email地址能超过100个字符
}

func validateGroupName(name string) bool {
	return groupNameRegex.MatchString(name)
}

// Register 用户注册
// @Summary      用户注册
// @Description  使用用户名和密码创建新账号，目前暂时没打算折腾头像
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      RegisterRequest  true  "注册信息"
// @Success      201   {object}  map[string]interface{} "注册成功"
// @Failure      400   {object}  ErrorResponse    "请求参数无效"
// @Failure      409   {object}  ErrorResponse    "用户名或邮箱已存在"
// @Router       /api/auth/register [post]
func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, ErrInvalidInput)
		return
	}

	if !validateUsername(req.Username) {
		handleError(c, ErrInvalidUsername)
		return
	}

	if !validateEmail(req.Email) {
		handleError(c, ErrInvalidEmail)
		return
	}

	if !validatePassword(req.Password) {
		handleError(c, ErrWeakPassword)
		return
	}

	if req.Bio != "" && len(req.Bio) > 150 {
		handleError(c, ErrInvalidInput)
		return
	}

	user, err := h.svc.CreateUser(c.Request.Context(), req.Username, req.Password, req.Email, req.Bio)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"user": gin.H{
			"public_id": user.PublicID,
			"username":  user.Username,
			"email":     user.Email,
			"bio":       user.Bio,
		},
	})
}

// Login 用户登录
// @Summary      用户登录，一样东西很少
// @Description  使用用户名和密码登录，返回 JWT Token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      LoginRequest   true  "登录信息"
// @Success      200   {object}  map[string]interface{} "登录成功"
// @Failure      400   {object}  ErrorResponse   "请求参数无效"
// @Failure      401   {object}  ErrorResponse   "用户名或密码错误"
// @Failure      429   {object}  ErrorResponse   "登录失败次数过多"
// @Router       /api/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, ErrInvalidInput)
		return
	}

	rateLimiter := c.MustGet("rateLimiter").(*RateLimiter)
	username := req.Username

	user, err := h.svc.GetUserByUsername(c.Request.Context(), req.Username)
	if err != nil {
		rateLimiter.RecordFailure(username)
		handleError(c, ErrInvalidCredentials)
		return
	}

	if err := h.svc.ValidateCredentials(user, req.Password); err != nil {
		rateLimiter.RecordFailure(username)
		handleError(c, ErrInvalidCredentials)
		return
	}

	token, err := h.jwt.GenerateToken(user)
	if err != nil {
		handleError(c, ErrTokenGeneration)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user": gin.H{
			"username":  user.Username,
			"email":     user.Email,
			"public_id": user.PublicID,
			"bio":       user.Bio,
		},
	})
}

// GetMe 获取当前用户信息
// @Summary      获取当前用户信息
// @Description  获取当前登录用户的详细信息
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Success      200  {object}  map[string]interface{} "成功"
// @Failure      401  {object}  ErrorResponse         "未授权"
// @Router       /api/auth/me [get]
func (h *Handler) GetMe(c *gin.Context) {
	userID := c.GetInt64("user_id")
	user, err := h.svc.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"username":  user.Username,
			"email":     user.Email,
			"public_id": user.PublicID,
			"bio":       user.Bio,
		},
	})
}

// UpdateUser 更新用户信息
// @Summary      更新用户信息
// @Description  更新当前登录用户的用户名、邮箱或个人简介
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        publicId  path  string              true   "用户公开ID"
// @Param        body      body  UpdateUserRequest   true   "更新信息"
// @Success      200       {object}  map[string]interface{} "更新成功"
// @Failure      400       {object}  ErrorResponse    "请求参数无效"
// @Failure      401       {object}  ErrorResponse    "未授权"
// @Failure      403       {object}  ErrorResponse    "无权限"
// @Router       /api/auth/{publicId} [put]
func (h *Handler) UpdateUser(c *gin.Context) {
	publicID := c.Param("publicId")
	currentPublicID := c.GetString("public_id")
	if currentPublicID != publicID {
		handleError(c, ErrForbidden)
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, ErrInvalidInput)
		return
	}

	if req.Username != "" && !validateUsername(req.Username) {
		handleError(c, ErrInvalidUsername)
		return
	}

	if req.Email != "" && !validateEmail(req.Email) {
		handleError(c, ErrInvalidEmail)
		return
	}

	if req.Bio != "" && len(req.Bio) > 150 {
		handleError(c, ErrInvalidInput)
		return
	}

	user, err := h.svc.GetUserByPublicID(c.Request.Context(), publicID)
	if err != nil {
		handleError(c, err)
		return
	}

	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Bio != "" {
		user.Bio = req.Bio
	}

	if err := h.svc.UpdateUser(c.Request.Context(), user); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"public_id": user.PublicID,
		"username":  user.Username,
		"email":     user.Email,
		"bio":       user.Bio,
	})
}

// DeleteUser 删除用户
// @Summary      删除用户
// @Description  删除当前登录用户的账号及相关数据，但是聊天记录我打算是随着自动清理清理掉，主要数据还是在客户端懒得管
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Param        publicId  path  string  true  "用户公开ID"
// @Success      200  {object}  SuccessResponse  "删除成功"
// @Failure      401  {object}  ErrorResponse    "未授权"
// @Failure      403  {object}  ErrorResponse    "无权限"
// @Router       /api/auth/{publicId} [delete]
func (h *Handler) DeleteUser(c *gin.Context) {
	publicID := c.Param("publicId")
	currentPublicID := c.GetString("public_id")
	if currentPublicID != publicID {
		handleError(c, ErrForbidden)
		return
	}

	user, err := h.svc.GetUserByPublicID(c.Request.Context(), publicID)
	if err != nil {
		handleError(c, err)
		return
	}

	if err := h.svc.DeleteUser(c.Request.Context(), user.ID); err != nil {
		handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted successfully"})
}

// Logout 用户登出
// @Summary      用户登出
// @Description  将当前 JWT Token 加入黑名单，实现登出
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     bearerAuth
// @Success      200  {object}  SuccessResponse  "登出成功"
// @Failure      401  {object}  ErrorResponse    "未授权"
// @Router       /api/auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	tokenStr := c.GetHeader("Authorization")
	if len(tokenStr) > 7 {
		tokenStr = tokenStr[7:]
		claims, err := h.jwt.ValidateToken(tokenStr)
		if err == nil && claims.ExpiresAt != nil {
			expiresAt := claims.ExpiresAt.Time.UnixMilli()
			_ = h.jwt.BlacklistToken(claims.ID, expiresAt)
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Logged out successfully"})
}

func (s *Service) GetUserByID(ctx context.Context, userID int64) (*User, error) {
	return checkFound(s.userStore.GetByID(ctx, userID))
}

func (s *Service) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return checkFound(s.userStore.GetByUsername(ctx, username))
}

// 这个publicID是暴露给外部的，内部还是自增ID，只能说最早是考虑的某些安全性，虽然说我也懒得去想到底有没有必要去做，但是来都来了
func (s *Service) GetUserByPublicID(ctx context.Context, publicID string) (*User, error) {
	return checkFound(s.userStore.GetByPublicID(ctx, publicID))
}

func (s *Service) GetUsersByIDs(ctx context.Context, userIDs []int64) (map[int64]*User, error) {
	if len(userIDs) == 0 {
		return make(map[int64]*User), nil
	}
	return s.userStore.GetByIDs(ctx, userIDs)
}

func (s *Service) GetPublicIDByUserID(ctx context.Context, userID int64) (string, error) {
	user, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", ErrNotFound
	}
	return user.PublicID, nil
}

// bio是才加上不到24h，不确定到底有没有必要
func (s *Service) CreateUser(ctx context.Context, username, password, email, bio string) (*User, error) {
	exists, err := s.userStore.UsernameExists(ctx, username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrConflict
	}

	if email != "" {
		emailExists, err := s.userStore.EmailExists(ctx, email)
		if err != nil {
			return nil, err
		}
		if emailExists {
			return nil, ErrConflict
		}
	}

	pwdHash, err := bcrypt.GenerateFromPassword([]byte(password), s.cfg.BCryptCost)
	if err != nil {
		return nil, fmt.Errorf("password hashing failed: %w", err)
	}

	publicID := GenerateNanoID(s.cfg.PublicIDLength)

	now := time.Now().UnixMilli()
	user := &User{
		Username:  username,
		PwdHash:   string(pwdHash),
		Email:     email,
		Bio:       bio,
		PublicID:  publicID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	result, err := s.userStore.Insert(ctx, user)
	if err != nil {
		return nil, err
	}

	user.ID = result
	return user, nil
}

func (s *Service) UpdateUser(ctx context.Context, user *User) error {
	user.UpdatedAt = time.Now().UnixMilli()
	_, err := s.userStore.Update(ctx, user)
	return err
}

func (s *Service) DeleteUser(ctx context.Context, userID int64) error {
	user, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrNotFound
	}

	groups, err := s.groupStore.GetGroupsByOwner(ctx, userID)
	if err != nil {
		return err
	}
	for _, g := range groups {
		if err := s.DeleteGroup(ctx, g.GroupID); err != nil {
			return fmt.Errorf("failed to delete group %s: %w", g.GroupID, err)
		}
	}

	tx, err := s.userStore.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	if _, err := s.groupMemStore.DeleteByUser(context.Background(), tx, userID); err != nil {
		return err
	}

	if _, err := s.convPartStore.DeleteByUserTx(tx, userID); err != nil {
		return err
	}

	if _, err := s.friendStore.DeleteByUser(context.Background(), tx, userID); err != nil {
		return err
	}
	if _, err := s.userStore.Delete(context.Background(), tx, userID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (s *Service) ValidateCredentials(user *User, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(user.PwdHash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}