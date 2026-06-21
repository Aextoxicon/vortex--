package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

func TestNewError(t *testing.T) {
	err := NewError("test_code", "test message")
	if err.Code != "test_code" {
		t.Errorf("NewError().Code = %q, want %q", err.Code, "test_code")
	}
	if err.Message != "test message" {
		t.Errorf("NewError().Message = %q, want %q", err.Message, "test message")
	}
	if err.Error() != "[test_code] test message" {
		t.Errorf("NewError().Error() = %q, want %q", err.Error(), "[test_code] test message")
	}
}

func TestAppError_Error(t *testing.T) {
	err := &AppError{Code: "not_found", Message: "Resource not found"}
	want := "[not_found] Resource not found"
	if got := err.Error(); got != want {
		t.Errorf("AppError.Error() = %q, want %q", got, want)
	}
}

func TestHandleError_AppError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"not_found", ErrNotFound, http.StatusNotFound, `{"error":"Resource not found"}`},
		{"forbidden", ErrForbidden, http.StatusForbidden, `{"error":"Access denied"}`},
		{"unauthorized", ErrUnauthorized, http.StatusUnauthorized, `{"error":"Unauthorized"}`},
		{"invalid_credentials", ErrInvalidCredentials, http.StatusUnauthorized, `{"error":"Invalid username or password"}`},
		{"conflict", ErrConflict, http.StatusConflict, `{"error":"Resource conflict"}`},
		{"already_exists", ErrAlreadyExists, http.StatusConflict, `{"error":"Resource already exists"}`},
		{"self_request", ErrSelfRequest, http.StatusConflict, `{"error":"Cannot request self"}`},
		{"rate_limit_exceeded", ErrRateLimitExceeded, http.StatusTooManyRequests, `{"error":"Rate limit exceeded"}`},
		{"invalid_input", ErrInvalidInput, http.StatusBadRequest, `{"error":"Invalid input"}`},
		{"invalid_type", ErrInvalidType, http.StatusBadRequest, `{"error":"Invalid type"}`},
		{"not_member", ErrNotMember, http.StatusBadRequest, `{"error":"Not a group member"}`},
		{"recall_window_expired", ErrRecallWindowExpired, http.StatusBadRequest, `{"error":"Message recall window expired"}`},
		{"weak_password", ErrWeakPassword, http.StatusBadRequest, `{"error":"Password must be 8-128 characters and contain uppercase, lowercase, number, and special character"}`},
		{"invalid_username", ErrInvalidUsername, http.StatusBadRequest, `{"error":"Username must be 3-20 characters and contain only letters, numbers, or common Asian characters"}`},
		{"invalid_email", ErrInvalidEmail, http.StatusBadRequest, `{"error":"Invalid email format"}`},
		{"invalid_group_name", ErrInvalidGroupName, http.StatusBadRequest, `{"error":"Group name must be 1-50 characters"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			handleError(c, tt.err)
			if w.Code != tt.wantStatus {
				t.Errorf("handleError() status = %d, want %d", w.Code, tt.wantStatus)
			}
			body := w.Body.String()
			if body != tt.wantBody+"\n" {
				t.Errorf("handleError() body = %q, want %q", body, tt.wantBody+"\n")
			}
		})
	}
}

func TestHandleError_UnknownAppError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	unknownErr := &AppError{Code: "unknown_code", Message: "Something unexpected"}
	handleError(c, unknownErr)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("handleError(unknown AppError) status = %d, want 500", w.Code)
	}
	if w.Body.String() != `{"error":"internal error"}`+"\n" {
		t.Errorf("handleError(unknown AppError) body = %q, want %q", w.Body.String(), `{"error":"internal error"}`+"\n")
	}
}

func TestHandleError_PQErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		pqCode     string
		wantStatus int
		wantBody   string
	}{
		{"unique violation", "23505", http.StatusConflict, `{"error":"Resource already exists"}`},
		{"foreign key violation", "23503", http.StatusBadRequest, `{"error":"Invalid reference"}`},
		{"not null violation", "23502", http.StatusBadRequest, `{"error":"Missing required field"}`},
		{"connection failure", "08006", http.StatusServiceUnavailable, `{"error":"Database connection failed"}`},
		{"unknown pq error", "XX000", http.StatusInternalServerError, `{"error":"Database error"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			handleError(c, &pq.Error{Code: pq.ErrorCode(tt.pqCode)})
			if w.Code != tt.wantStatus {
				t.Errorf("handleError(pq error code %s) status = %d, want %d", tt.pqCode, w.Code, tt.wantStatus)
			}
			if w.Body.String() != tt.wantBody+"\n" {
				t.Errorf("handleError(pq error code %s) body = %q, want %q", tt.pqCode, w.Body.String(), tt.wantBody+"\n")
			}
		})
	}
}

func TestHandleError_SQLErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"sql.ErrNoRows", sql.ErrNoRows, http.StatusNotFound, `{"error":"Resource not found"}`},
		{"sql.ErrConnDone", sql.ErrConnDone, http.StatusServiceUnavailable, `{"error":"Service unavailable"}`},
		{"sql.ErrTxDone", sql.ErrTxDone, http.StatusServiceUnavailable, `{"error":"Service unavailable"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			handleError(c, tt.err)
			if w.Code != tt.wantStatus {
				t.Errorf("handleError(%v) status = %d, want %d", tt.err, w.Code, tt.wantStatus)
			}
			if w.Body.String() != tt.wantBody+"\n" {
				t.Errorf("handleError(%v) body = %q, want %q", tt.err, w.Body.String(), tt.wantBody+"\n")
			}
		})
	}
}

func TestHandleError_UnexpectedError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	handleError(c, fmt.Errorf("something completely unexpected"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("handleError(unknown error) status = %d, want 500", w.Code)
	}
	if w.Body.String() != `{"error":"internal error"}`+"\n" {
		t.Errorf("handleError(unknown error) body = %q, want %q", w.Body.String(), `{"error":"internal error"}`+"\n")
	}
}

func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"pq unique violation", &pq.Error{Code: "23505"}, true},
		{"pq foreign key violation", &pq.Error{Code: "23503"}, false},
		{"pq not null violation", &pq.Error{Code: "23502"}, false},
		{"fmt.Errorf", fmt.Errorf("some error"), false},
		{"nil error", nil, false},
		{"errors.New", errors.New("generic"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUniqueViolation(tt.err); got != tt.want {
				t.Errorf("isUniqueViolation(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
