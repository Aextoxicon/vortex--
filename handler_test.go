package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// 一大坨handler测试，懒得写注释了，职责如同文件名，函数名称也是非常清晰
var testUserCounter int64

func uniqueUsername() string {
	n := atomic.AddInt64(&testUserCounter, 1)
	return fmt.Sprintf("testuser%d", n)
}

func setupTestHandler(t *testing.T) (*Handler, *Service, *JwtService) {
	t.Helper()

	svc, db, _ := setupTestService(t)

	cfg := &Config{
		NodeID:                               1,
		PublicIDLength:                       12,
		BCryptCost:                           10,
		JWTSecret:                            "test-secret-key-min-32-chars-long!!",
		JWTIssuer:                            "test-issuer",
		JWTExpiresMinutes:                    60,
		DefaultPageSize:                      20,
		MaxPageSize:                          100,
		EpochTime:                            1700000000000,
		SegmentDurationMs:                    3600000,
		SegmentSize:                          1000,
		MessageRecallWindowMs:                120000,
		WorkerTableCreateIntervalHours:       24,
		WorkerMaintenanceInitialDelayMinutes: 1,
		WorkerMaintenanceIntervalHours:       24,
		MessageRetentionDays:                 7,
	}

	jwtService := NewJwtService(db, cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTExpiresMinutes)
	handler := NewHandler(svc, jwtService, cfg)

	t.Cleanup(func() {
		jwtService.Stop()
	})

	return handler, svc, jwtService
}

func createTestUser(t *testing.T, svc *Service) *User {
	t.Helper()

	ctx := context.Background()
	username := uniqueUsername()
	user, err := svc.CreateUser(ctx, username, "Test1234!", username+"@example.com", "")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	return user
}

func generateToken(handler *Handler, user *User) string {
	token, err := handler.jwt.GenerateToken(user)
	if err != nil {
		panic(err)
	}
	return token
}

func authHeader(token string) string {
	return "Bearer " + token
}

func setupTestGin() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func TestHandler_HealthCheck(t *testing.T) {
	handler, _, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/health", handler.HealthCheck)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
}

func TestHandler_Register(t *testing.T) {
	handler, _, _ := setupTestHandler(t)
	r := setupTestGin()
	r.POST("/api/auth/register", handler.Register)

	username := uniqueUsername()

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantErr    string
	}{
		{
			name:       "success",
			body:       fmt.Sprintf(`{"username":"%s","password":"Test1234!","email":"%s@example.com"}`, username, username),
			wantStatus: http.StatusCreated,
		},
		{
			name:       "weak password",
			body:       `{"username":"weakuser","password":"123","email":"weak@example.com"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "Password must be 8-128 characters",
		},
		{
			name:       "invalid username",
			body:       `{"username":"ab","password":"Test1234!","email":"ab@example.com"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "Username must be 3-20 characters",
		},
		{
			name:       "missing fields",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    "Invalid input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/auth/register", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}

			if tt.wantErr != "" {
				var errResp ErrorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &errResp); err == nil {
					if !strings.Contains(errResp.Error, tt.wantErr) {
						t.Errorf("expected error containing %s, got %s", tt.wantErr, errResp.Error)
					}
				}
			}
		})
	}
}

func TestHandler_Register_Duplicate(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.POST("/api/auth/register", handler.Register)

	user := createTestUser(t, svc)
	body := fmt.Sprintf(`{"username":"%s","password":"Test1234!","email":"dup@example.com"}`, user.Username)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Login(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	rl := NewRateLimiter()
	defer rl.Stop()
	r.Use(func(c *gin.Context) {
		c.Set("rateLimiter", rl)
		c.Next()
	})
	r.POST("/api/auth/login", handler.Login)

	username := uniqueUsername()
	ctx := context.Background()
	_, err := svc.CreateUser(ctx, username, "Test1234!", username+"@example.com", "")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "success",
			body:       fmt.Sprintf(`{"username":"%s","password":"Test1234!"}`, username),
			wantStatus: http.StatusOK,
		},
		{
			name:       "wrong password",
			body:       fmt.Sprintf(`{"username":"%s","password":"wrongpass1!"}`, username),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "user not found",
			body:       `{"username":"nonexistent123","password":"Test1234!"}`,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/auth/login", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, w.Code)
			}
		})
	}
}

func TestHandler_GetMe(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/api/auth/me", jwtMiddleware(handler.jwt), handler.GetMe)

	user := createTestUser(t, svc)
	token := generateToken(handler, user)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/auth/me", nil)
	req.Header.Set("Authorization", authHeader(token))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	u, ok := resp["user"].(map[string]interface{})
	if !ok {
		t.Fatal("expected user object in response")
	}
	if u["username"] != user.Username {
		t.Errorf("expected username %s, got %v", user.Username, u["username"])
	}
}

func TestHandler_GetMe_Unauthorized(t *testing.T) {
	handler, _, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/api/auth/me", jwtMiddleware(handler.jwt), handler.GetMe)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/auth/me", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}

func TestHandler_SendMessage(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.POST("/api/messages/send", jwtMiddleware(handler.jwt), handler.SendMessage)

	ctx := context.Background()
	user1 := createTestUser(t, svc)
	user2 := createTestUser(t, svc)

	user1PublicID, err := svc.GetPublicIDByUserID(ctx, user1.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}
	user2PublicID, err := svc.GetPublicIDByUserID(ctx, user2.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	reqID, _, err := svc.SendFriendRequest(ctx, user1.ID, user2.ID, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}

	if reqID > 0 {
		if err := svc.AcceptFriendRequest(ctx, reqID, user2.ID); err != nil {
			t.Fatalf("failed to accept friend request: %v", err)
		}
	}

	convID := PrivateConvID(user1PublicID, user2PublicID)

	tableName := MessageTableNameByTs(time.Now().UnixMilli())
	err = svc.msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}

	token := generateToken(handler, user1)
	body := fmt.Sprintf(`{"conv_id":"%s","content":"Hello, World!"}`, convID)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/messages/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(token))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	msg, ok := resp["message"].(map[string]interface{})
	if !ok {
		t.Fatal("expected message object in response")
	}
	if msg["conv_id"] != convID {
		t.Errorf("expected conv_id %s, got %v", convID, msg["conv_id"])
	}
}

func TestHandler_CreateGroup(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.POST("/api/groups", jwtMiddleware(handler.jwt), handler.CreateGroup)

	user := createTestUser(t, svc)
	token := generateToken(handler, user)

	body := `{"name":"Test Group","description":"A test group"}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(token))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["group_id"] == "" {
		t.Error("expected group_id to be non-empty")
	}

	if resp["name"] != "Test Group" {
		t.Errorf("expected name 'Test Group', got %v", resp["name"])
	}
}

func TestHandler_GetGroup(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/api/groups/:id", jwtMiddleware(handler.jwt), handler.GetGroup)

	ctx := context.Background()
	user := createTestUser(t, svc)

	groupID, err := svc.CreateGroup(ctx, "Test Group", "", user.ID)
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	token := generateToken(handler, user)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/groups/"+groupID, nil)
	req.Header.Set("Authorization", authHeader(token))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["group_id"] != groupID {
		t.Errorf("expected group_id %s, got %v", groupID, resp["group_id"])
	}
}

func TestHandler_JoinAndLeaveGroup(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.POST("/api/groups/:id/join", jwtMiddleware(handler.jwt), handler.JoinGroup)
	r.POST("/api/groups/:id/leave", jwtMiddleware(handler.jwt), handler.LeaveGroup)

	ctx := context.Background()
	owner := createTestUser(t, svc)
	member := createTestUser(t, svc)

	groupID, err := svc.CreateGroup(ctx, "Join Group", "", owner.ID)
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	memberToken := generateToken(handler, member)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/"+groupID+"/join", nil)
	req.Header.Set("Authorization", authHeader(memberToken))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 on join, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/groups/"+groupID+"/leave", nil)
	req.Header.Set("Authorization", authHeader(memberToken))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 on leave, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_JoinGroup_NotFound(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.POST("/api/groups/:id/join", jwtMiddleware(handler.jwt), handler.JoinGroup)

	user := createTestUser(t, svc)
	token := generateToken(handler, user)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/groups/nonexistent/join", nil)
	req.Header.Set("Authorization", authHeader(token))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_FriendRequestFlow(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.POST("/api/friends/request/send/:targetPublicId", jwtMiddleware(handler.jwt), handler.SendFriendRequest)
	r.GET("/api/friends/requests", jwtMiddleware(handler.jwt), handler.GetFriendRequests)
	r.POST("/api/friends/request/:requestId/accept", jwtMiddleware(handler.jwt), handler.AcceptFriendRequest)

	ctx := context.Background()
	user1 := createTestUser(t, svc)
	user2 := createTestUser(t, svc)

	user2PublicID, err := svc.GetPublicIDByUserID(ctx, user2.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	token1 := generateToken(handler, user1)
	token2 := generateToken(handler, user2)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/friends/request/send/"+user2PublicID, nil)
	req.Header.Set("Authorization", authHeader(token1))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var sendResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &sendResp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/friends/requests", nil)
	req.Header.Set("Authorization", authHeader(token2))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var listResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	received, ok := listResp["received"].([]interface{})
	if !ok || len(received) == 0 {
		t.Error("expected received requests")
	}

	requestID := int64(sendResp["id"].(float64))

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/friends/request/"+fmt.Sprintf("%d", requestID)+"/accept", nil)
	req.Header.Set("Authorization", authHeader(token2))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_CancelFriendRequest(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.POST("/api/friends/request/send/:targetPublicId", jwtMiddleware(handler.jwt), handler.SendFriendRequest)
	r.DELETE("/api/friends/request/:requestId", jwtMiddleware(handler.jwt), handler.CancelFriendRequest)

	ctx := context.Background()
	user1 := createTestUser(t, svc)
	user2 := createTestUser(t, svc)

	user2PublicID, err := svc.GetPublicIDByUserID(ctx, user2.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	token1 := generateToken(handler, user1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/friends/request/send/"+user2PublicID, nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(token1))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var sendResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &sendResp)
	requestID := int64(sendResp["id"].(float64))

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/friends/request/"+fmt.Sprintf("%d", requestID), nil)
	req.Header.Set("Authorization", authHeader(token1))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_BlockUser(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.POST("/api/blocks/:targetPublicId", jwtMiddleware(handler.jwt), handler.BlockUser)
	r.DELETE("/api/blocks/:targetPublicId", jwtMiddleware(handler.jwt), handler.UnblockUser)

	ctx := context.Background()
	user1 := createTestUser(t, svc)
	user2 := createTestUser(t, svc)

	user2PublicID, err := svc.GetPublicIDByUserID(ctx, user2.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	token1 := generateToken(handler, user1)

	reqID, _, err := svc.SendFriendRequest(ctx, user1.ID, user2.ID, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}

	t.Logf("SendFriendRequest: reqID=%d", reqID)

	if reqID > 0 {
		if err := svc.AcceptFriendRequest(ctx, reqID, user2.ID); err != nil {
			t.Fatalf("failed to accept friend request: %v", err)
		}
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/blocks/"+user2PublicID, nil)
	req.Header.Set("Authorization", authHeader(token1))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 on block, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/blocks/"+user2PublicID, nil)
	req.Header.Set("Authorization", authHeader(token1))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 on unblock, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_RecallMessage(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.POST("/api/messages/recall/:msgId", jwtMiddleware(handler.jwt), handler.RecallMessage)

	ctx := context.Background()
	user1 := createTestUser(t, svc)
	user2 := createTestUser(t, svc)

	user1PublicID, err := svc.GetPublicIDByUserID(ctx, user1.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}
	user2PublicID, err := svc.GetPublicIDByUserID(ctx, user2.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	reqID, _, err := svc.SendFriendRequest(ctx, user1.ID, user2.ID, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}

	t.Logf("SendFriendRequest: reqID=%d", reqID)

	if reqID > 0 {
		if err := svc.AcceptFriendRequest(ctx, reqID, user2.ID); err != nil {
			t.Fatalf("failed to accept friend request: %v", err)
		}
	}

	convID := PrivateConvID(user1PublicID, user2PublicID)

	tableName := MessageTableNameByTs(time.Now().UnixMilli())
	err = svc.msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}

	result, err := svc.SendMessage(ctx, user1, convID, "Hello", "")
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	token1 := generateToken(handler, user1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/messages/recall/"+result.MsgID, nil)
	req.Header.Set("Authorization", authHeader(token1))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["message"] != "message recalled successfully" {
		t.Errorf("expected recall message, got %v", resp["message"])
	}
}

func TestHandler_GetConversations(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/api/conversations", jwtMiddleware(handler.jwt), handler.GetConversations)

	ctx := context.Background()
	user1 := createTestUser(t, svc)
	user2 := createTestUser(t, svc)

	user1PublicID, err := svc.GetPublicIDByUserID(ctx, user1.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}
	user2PublicID, err := svc.GetPublicIDByUserID(ctx, user2.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	reqID, autoAccepted, err := svc.SendFriendRequest(ctx, user1.ID, user2.ID, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}

	t.Logf("SendFriendRequest: reqID=%d, autoAccepted=%v", reqID, autoAccepted)

	if !autoAccepted {
		if reqID == 0 {
			t.Fatal("expected non-zero request ID")
		}
		if err := svc.AcceptFriendRequest(ctx, reqID, user2.ID); err != nil {
			t.Fatalf("failed to accept friend request: %v", err)
		}
	}

	convID := PrivateConvID(user1PublicID, user2PublicID)

	tableName := MessageTableNameByTs(time.Now().UnixMilli())
	err = svc.msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message table: %v", err)
	}

	_, err = svc.SendMessage(ctx, user1, convID, "Hello", "")
	if err != nil {
		t.Logf("warning: failed to send message: %v", err)
	}

	token1 := generateToken(handler, user1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/conversations", nil)
	req.Header.Set("Authorization", authHeader(token1))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	convs, ok := resp["conversations"].([]interface{})
	if !ok {
		t.Fatal("expected conversations array in response")
	}

	if len(convs) == 0 {
		t.Error("expected at least one conversation")
	}
}

func TestHandler_KickMember(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.DELETE("/api/groups/:id/members/:memberPublicId", jwtMiddleware(handler.jwt), handler.KickMember)

	ctx := context.Background()
	owner := createTestUser(t, svc)
	member := createTestUser(t, svc)

	memberPublicID, err := svc.GetPublicIDByUserID(ctx, member.ID)
	if err != nil {
		t.Fatalf("failed to get member public ID: %v", err)
	}

	groupID, err := svc.CreateGroup(ctx, "Kick Group", "", owner.ID)
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	err = svc.AddMember(ctx, groupID, member.ID, "member")
	if err != nil {
		t.Fatalf("failed to add member: %v", err)
	}

	ownerToken := generateToken(handler, owner)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/groups/"+groupID+"/members/"+memberPublicID, nil)
	req.Header.Set("Authorization", authHeader(ownerToken))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetGroup_NotFound(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/api/groups/:id", jwtMiddleware(handler.jwt), handler.GetGroup)

	user := createTestUser(t, svc)
	token := generateToken(handler, user)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/groups/nonexistent", nil)
	req.Header.Set("Authorization", authHeader(token))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetMessageByID(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/api/messages/:msgId", jwtMiddleware(handler.jwt), handler.GetMessageByID)

	ctx := context.Background()
	user1 := createTestUser(t, svc)
	user2 := createTestUser(t, svc)

	user1PublicID, err := svc.GetPublicIDByUserID(ctx, user1.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}
	user2PublicID, err := svc.GetPublicIDByUserID(ctx, user2.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	reqID, _, err := svc.SendFriendRequest(ctx, user1.ID, user2.ID, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}
	if reqID > 0 {
		if err := svc.AcceptFriendRequest(ctx, reqID, user2.ID); err != nil {
			t.Fatalf("failed to accept friend request: %v", err)
		}
	}

	convID := PrivateConvID(user1PublicID, user2PublicID)
	tableName := MessageTableNameByTs(time.Now().UnixMilli())
	err = svc.msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}

	result, err := svc.SendMessage(ctx, user1, convID, "Test message", "")
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	token := generateToken(handler, user1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/messages/"+result.MsgID, nil)
	req.Header.Set("Authorization", authHeader(token))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["content"] != "Test message" {
		t.Errorf("expected content 'Test message', got %v", resp["content"])
	}
}

func TestHandler_GetConversationCount(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/api/conversations/count", jwtMiddleware(handler.jwt), handler.GetConversationCount)

	ctx := context.Background()
	user1 := createTestUser(t, svc)
	user2 := createTestUser(t, svc)

	user1PublicID, err := svc.GetPublicIDByUserID(ctx, user1.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}
	user2PublicID, err := svc.GetPublicIDByUserID(ctx, user2.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	reqID, _, err := svc.SendFriendRequest(ctx, user1.ID, user2.ID, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}
	if reqID > 0 {
		if err := svc.AcceptFriendRequest(ctx, reqID, user2.ID); err != nil {
			t.Fatalf("failed to accept friend request: %v", err)
		}
	}

	convID := PrivateConvID(user1PublicID, user2PublicID)
	tableName := MessageTableNameByTs(time.Now().UnixMilli())
	err = svc.msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}

	_, err = svc.SendMessage(ctx, user1, convID, "Hello", "")
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	token := generateToken(handler, user1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/conversations/count", nil)
	req.Header.Set("Authorization", authHeader(token))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["user_id"] == nil {
		t.Error("expected user_id in response")
	}
	count, ok := resp["count"].(float64)
	if !ok {
		t.Fatal("expected count field")
	}
	if int(count) < 1 {
		t.Errorf("expected count >= 1, got %v", count)
	}
}

func TestHandler_GetConversationParticipants(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/api/conversations/:convId/participants", jwtMiddleware(handler.jwt), handler.GetConversationParticipants)

	ctx := context.Background()
	user1 := createTestUser(t, svc)
	user2 := createTestUser(t, svc)

	user1PublicID, err := svc.GetPublicIDByUserID(ctx, user1.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}
	user2PublicID, err := svc.GetPublicIDByUserID(ctx, user2.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	reqID, _, err := svc.SendFriendRequest(ctx, user1.ID, user2.ID, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}
	if reqID > 0 {
		if err := svc.AcceptFriendRequest(ctx, reqID, user2.ID); err != nil {
			t.Fatalf("failed to accept friend request: %v", err)
		}
	}

	convID := PrivateConvID(user1PublicID, user2PublicID)
	tableName := MessageTableNameByTs(time.Now().UnixMilli())
	err = svc.msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}

	_, err = svc.SendMessage(ctx, user1, convID, "Hello", "")
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	token := generateToken(handler, user1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/conversations/"+convID+"/participants", nil)
	req.Header.Set("Authorization", authHeader(token))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["conv_id"] != convID {
		t.Errorf("expected conv_id %s, got %v", convID, resp["conv_id"])
	}

	participants, ok := resp["participants"].([]interface{})
	if !ok {
		t.Fatal("expected participants array in response")
	}

	if len(participants) < 2 {
		t.Errorf("expected at least 2 participants, got %d", len(participants))
	}
}

func TestHandler_CheckBlocked(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/api/conversations/:convId/blocked/:userId", jwtMiddleware(handler.jwt), handler.CheckBlocked)

	ctx := context.Background()
	user1 := createTestUser(t, svc)
	user2 := createTestUser(t, svc)

	user1PublicID, err := svc.GetPublicIDByUserID(ctx, user1.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}
	user2PublicID, err := svc.GetPublicIDByUserID(ctx, user2.ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	convID := PrivateConvID(user1PublicID, user2PublicID)
	now := time.Now().UnixMilli()

	_, err = svc.convPartStore.DB().ExecContext(ctx, `
		INSERT INTO conversation_participants (conv_id, user_id, join_ts, is_blocked)
		VALUES ($1, $2, $3, 0), ($1, $4, $3, 0)`,
		convID, user1.ID, now, user2.ID,
	)
	if err != nil {
		t.Fatalf("failed to insert participants: %v", err)
	}

	token := generateToken(handler, user1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/conversations/"+convID+"/blocked/"+fmt.Sprintf("%d", user2.ID), nil)
	req.Header.Set("Authorization", authHeader(token))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["conv_id"] != convID {
		t.Errorf("expected conv_id %s, got %v", convID, resp["conv_id"])
	}

	isBlocked, ok := resp["is_blocked"].(bool)
	if !ok {
		t.Fatal("expected is_blocked boolean in response")
	}

	if isBlocked {
		t.Error("expected is_blocked to be false")
	}
}

func TestHandler_GetPendingFriendRequests(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/api/friends/requests/pending", jwtMiddleware(handler.jwt), handler.GetPendingFriendRequests)

	ctx := context.Background()
	user1 := createTestUser(t, svc)
	user2 := createTestUser(t, svc)

	_, _, err := svc.SendFriendRequest(ctx, user2.ID, user1.ID, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}

	token1 := generateToken(handler, user1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/friends/requests/pending", nil)
	req.Header.Set("Authorization", authHeader(token1))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	requests, ok := resp["requests"].([]interface{})
	if !ok {
		t.Fatal("expected requests array in response")
	}

	if len(requests) == 0 {
		t.Error("expected at least one pending request")
	}
}

func TestHandler_GetGroupMemberCount(t *testing.T) {
	handler, svc, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/api/groups/:id/members/count", jwtMiddleware(handler.jwt), handler.GetGroupMemberCount)

	ctx := context.Background()
	owner := createTestUser(t, svc)

	groupID, err := svc.CreateGroup(ctx, "Count Group", "", owner.ID)
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	member := createTestUser(t, svc)
	err = svc.AddMember(ctx, groupID, member.ID, "member")
	if err != nil {
		t.Fatalf("failed to add member: %v", err)
	}

	token := generateToken(handler, owner)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/groups/"+groupID+"/members/count", nil)
	req.Header.Set("Authorization", authHeader(token))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["group_id"] != groupID {
		t.Errorf("expected group_id %s, got %v", groupID, resp["group_id"])
	}

	count, ok := resp["count"].(float64)
	if !ok {
		t.Fatal("expected count field")
	}
	if int(count) != 2 {
		t.Errorf("expected count 2 (owner + member), got %v", count)
	}
}

func TestHandler_Metrics(t *testing.T) {
	handler, _, _ := setupTestHandler(t)
	r := setupTestGin()
	r.GET("/metrics", handler.Metrics)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["pid"] == nil {
		t.Error("expected pid in response")
	}

	if resp["threads"] == nil {
		t.Error("expected threads in response")
	}
}
