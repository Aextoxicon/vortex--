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

var (
	usernameRegex  = regexp.MustCompile(`^[a-zA-Z0-9_\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}]{3,20}$`)
	emailRegex     = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	groupNameRegex = regexp.MustCompile(`^[\w\s\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}_-]{1,50}$`)
)

func validatePassword(password string) bool {
	if len(password) < 8 || len(password) > 16 {
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
	return emailRegex.MatchString(email) && len(email) <= 100
}

func validateGroupName(name string) bool {
	return groupNameRegex.MatchString(name)
}

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

	userID, err := h.svc.CreateUser(c.Request.Context(), req.Username, req.Password, req.Email)
	if err != nil {
		handleError(c, err)
		return
	}

	user, err := h.svc.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		handleError(c, err)
		return
	}

	token, err := h.jwt.GenerateToken(user)
	if err != nil {
		handleError(c, ErrTokenGeneration)
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
		handleError(c, ErrInvalidInput)
		return
	}

	user, err := h.svc.GetUserByUsername(c.Request.Context(), req.Username)
	if err != nil {
		rateLimiter := c.MustGet("rateLimiter").(*RateLimiter)
		ip := c.ClientIP()
		rateLimiter.RecordFailure(ip)
		handleError(c, ErrInvalidCredentials)
		return
	}

	if err := h.svc.ValidateCredentials(user, req.Password); err != nil {
		rateLimiter := c.MustGet("rateLimiter").(*RateLimiter)
		ip := c.ClientIP()
		rateLimiter.RecordFailure(ip)
		handleError(c, ErrInvalidCredentials)
		return
	}

	rateLimiter := c.MustGet("rateLimiter").(*RateLimiter)
	ip := c.ClientIP()
	rateLimiter.ResetFailure(ip)

	token, err := h.jwt.GenerateToken(user)
	if err != nil {
		handleError(c, ErrTokenGeneration)
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
		},
	})
}

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

	if err := h.svc.UpdateUser(c.Request.Context(), user); err != nil {
		handleError(c, err)
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

	c.JSON(http.StatusNoContent, nil)
}

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
	user, err := s.userStore.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}

func (s *Service) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	user, err := s.userStore.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}

func (s *Service) GetUserByPublicID(ctx context.Context, publicID string) (*User, error) {
	user, err := s.userStore.GetByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
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

func (s *Service) CreateUser(ctx context.Context, username, password, email string) (int64, error) {
	exists, err := s.userStore.UsernameExists(ctx, username)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, ErrConflict
	}

	if email != "" {
		emailExists, err := s.userStore.EmailExists(ctx, email)
		if err != nil {
			return 0, err
		}
		if emailExists {
			return 0, ErrConflict
		}
	}

	pwdHash, err := bcrypt.GenerateFromPassword([]byte(password), s.cfg.BCryptCost)
	if err != nil {
		return 0, fmt.Errorf("password hashing failed: %w", err)
	}

	publicID := GenerateNanoID(s.cfg.PublicIDLength)

	now := time.Now().UnixMilli()
	user := &User{
		Username:  username,
		PwdHash:   string(pwdHash),
		Email:     email,
		PublicID:  publicID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	result, err := s.userStore.Insert(ctx, user)
	if err != nil {
		return 0, err
	}

	return result, nil
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

	tx, err := s.userStore.DB().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	if _, err := s.groupMemStore.DeleteByUserTx(tx, userID); err != nil {
		return err
	}

	if _, err := s.convPartStore.DeleteByUserTx(tx, userID); err != nil {
		return err
	}

	if err := s.DeleteFriendRequestsByUser(ctx, tx, userID); err != nil {
		return err
	}
	if _, err := s.userStore.DeleteTx(tx, userID); err != nil {
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
