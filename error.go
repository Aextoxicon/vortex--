package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type AppError struct {
	Code    string
	Message string
	Details interface{}
}

func (e *AppError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

var (
	ErrNotFound            = &AppError{Code: "not_found", Message: "Resource not found"}
	ErrForbidden           = &AppError{Code: "forbidden", Message: "Access denied"}
	ErrUnauthorized        = &AppError{Code: "unauthorized", Message: "Unauthorized"}
	ErrInvalidCredentials  = &AppError{Code: "invalid_credentials", Message: "Invalid username or password"}
	ErrRateLimitExceeded   = &AppError{Code: "rate_limit_exceeded", Message: "Rate limit exceeded"}
	ErrConflict            = &AppError{Code: "conflict", Message: "Resource conflict"}
	ErrNotMember           = &AppError{Code: "not_member", Message: "Not a group member"}
	ErrNotFriend           = &AppError{Code: "not_friend", Message: "Not friends yet, please send friend request first"}
	ErrInvalidInput        = &AppError{Code: "invalid_input", Message: "Invalid input"}
	ErrInvalidType         = &AppError{Code: "invalid_type", Message: "Invalid type"}
	ErrInvalidTargetID     = &AppError{Code: "invalid_target_id", Message: "Invalid target ID"}
	ErrSelfRequest         = &AppError{Code: "self_request", Message: "Cannot request self"}
	ErrNotPending          = &AppError{Code: "not_pending", Message: "Status is not pending"}
	ErrAlreadyExists       = &AppError{Code: "already_exists", Message: "Resource already exists"}
	ErrTokenGeneration     = &AppError{Code: "token_generation_failed", Message: "Token generation failed"}
	ErrGetRequestsFailed   = &AppError{Code: "get_requests_failed", Message: "Failed to get requests"}
	ErrInvalidDateFormat   = &AppError{Code: "invalid_date_format", Message: "Invalid date format, use YYYY-MM-DD"}
	ErrInvalidMsgID        = &AppError{Code: "invalid_msg_id", Message: "Invalid message ID"}
	ErrInternalServer      = &AppError{Code: "internal_server", Message: "Internal server error"}
	ErrRecallWindowExpired = &AppError{Code: "recall_window_expired", Message: "Message recall window expired"}
	ErrWeakPassword        = &AppError{Code: "weak_password", Message: "Password must be 8-16 characters and contain uppercase, lowercase, number, and special character"}
	ErrInvalidUsername     = &AppError{Code: "invalid_username", Message: "Username must be 3-20 characters and contain only letters, numbers, or common Asian characters"}
	ErrInvalidEmail        = &AppError{Code: "invalid_email", Message: "Invalid email format"}
	ErrInvalidGroupName    = &AppError{Code: "invalid_group_name", Message: "Group name must be 1-50 characters"}
)

func NewError(code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func handleError(c *gin.Context, err error) {
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
		case "invalid_input", "invalid_type", "invalid_target_id", "not_pending", "not_member", "invalid_date_format", "invalid_msg_id", "token_generation_failed", "get_requests_failed", "recall_window_expired", "weak_password", "invalid_username", "invalid_email", "invalid_group_name":
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: appErr.Message})
		default:
			slog.Error("unhandled app error", "code", appErr.Code, "message", appErr.Message, "details", appErr.Details)
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
		}
		return
	}

	if pqErr, ok := err.(*pq.Error); ok {
		slog.Error("database error", "code", pqErr.Code, "message", pqErr.Message, "detail", pqErr.Detail)
		switch pqErr.Code {
		case "23505":
			c.JSON(http.StatusConflict, ErrorResponse{Error: "Resource already exists"})
		case "23503":
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid reference"})
		case "23502":
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing required field"})
		case "08006":
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "Database connection failed"})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Database error"})
		}
		return
	}

	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Resource not found"})
		return
	}

	if errors.Is(err, sql.ErrConnDone) || errors.Is(err, sql.ErrTxDone) {
		slog.Error("connection or transaction error", "error", err)
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "Service unavailable"})
		return
	}

	slog.Error("unexpected error", "error", err)
	c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
}
