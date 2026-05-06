package main

import (
	"fmt"
	"net/http"
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

func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleError(c, ErrInvalidInput)
		return
	}

	userID, err := h.svc.CreateUser(req.Username, req.Password, req.Email)
	if err != nil {
		handleError(c, err)
		return
	}

	user, err := h.svc.GetUserByID(userID)
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

	user, err := h.svc.GetUserByUsername(req.Username)
	if err != nil {
		handleError(c, ErrInvalidCredentials)
		return
	}

	if err := h.svc.ValidateCredentials(user, req.Password); err != nil {
		handleError(c, ErrInvalidCredentials)
		return
	}

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
	user, err := h.svc.GetUserByID(userID)
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

	user, err := h.svc.GetUserByPublicID(publicID)
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

	if err := h.svc.UpdateUser(user); err != nil {
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

	user, err := h.svc.GetUserByPublicID(publicID)
	if err != nil {
		handleError(c, err)
		return
	}

	if err := h.svc.DeleteUser(user.ID); err != nil {
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

func (s *Service) GetUserByID(userID int64) (*User, error) {
	user, err := s.userStore.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}

func (s *Service) GetUserByUsername(username string) (*User, error) {
	user, err := s.userStore.GetByUsername(username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}

func (s *Service) GetUserByPublicID(publicID string) (*User, error) {
	user, err := s.userStore.GetByPublicID(publicID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound
	}
	return user, nil
}

func (s *Service) CreateUser(username, password, email string) (int64, error) {
	exists, err := s.userStore.UsernameExists(username)
	if err != nil {
		return 0, err
	}
	if exists {
		return 0, ErrConflict
	}

	if email != "" {
		emailExists, err := s.userStore.EmailExists(email)
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

	result, err := s.userStore.Insert(user)
	if err != nil {
		return 0, err
	}

	return result, nil
}

func (s *Service) UpdateUser(user *User) error {
	user.UpdatedAt = time.Now().UnixMilli()
	_, err := s.userStore.Update(user)
	return err
}

func (s *Service) DeleteUser(userID int64) error {
	user, err := s.GetUserByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrNotFound
	}

	groups, err := s.groupStore.GetGroupsByOwner(userID)
	if err != nil {
		return err
	}
	for _, g := range groups {
		if err := s.DeleteGroup(g.GroupID); err != nil {
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

	if err := s.DeleteFriendRequestsByUser(tx, userID); err != nil {
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
