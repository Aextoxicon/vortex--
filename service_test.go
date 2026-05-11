package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"vortex/test/testutil"

	_ "github.com/lib/pq"
)

func setupTestService(t *testing.T) (*Service, *sql.DB, *testutil.PostgresContainer) {
	t.Helper()

	ctx := context.Background()
	pgContainer, err := testutil.NewPostgresContainer(ctx)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	db, err := sql.Open("postgres", pgContainer.ConnectionString)
	if err != nil {
		t.Fatalf("failed to open database connection: %v", err)
	}

	if err := RunMigrations(db); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		pgContainer.Cleanup(t)
	})

	store := NewStore(db)
	userStore := &UserStore{Store: store}
	msgStore := &MessageStore{Store: store}
	groupStore := &GroupStore{Store: store}
	groupMemStore := &GroupMemberStore{Store: store}
	friendStore := &FriendRequestStore{Store: store}
	convPartStore := &ConversationParticipantStore{Store: store}
	idGenStateStore := &IdGeneratorStateStore{Store: store}
	idempotencyStore := &MessageIdempotencyStore{Store: store}

	cfg := &Config{
		NodeID:         1,
		PublicIDLength: 12,
		BCryptCost:     10,
	}

	idGen := NewIdGenerator(cfg, idGenStateStore, msgStore, cfg.NodeID)
	idGen.Init()

	svc := NewService(
		cfg,
		userStore, msgStore, groupStore, groupMemStore,
		friendStore, convPartStore,
		idGenStateStore, idempotencyStore, idGen,
		nil,
	)

	// 创建当前日期和下周的消息表
	now := time.Now().UTC()
	for offset := 0; offset < 8; offset++ {
		date := now.AddDate(0, 0, offset)
		tableName := MessageTableNameByDate(date)
		if _, err := msgStore.CreateMessageTable(tableName); err != nil {
			t.Fatalf("failed to create message table %s: %v", tableName, err)
		}
	}

	return svc, db, pgContainer
}

func TestService_CreateUser(t *testing.T) {
	svc, _, _ := setupTestService(t)

	ctx := context.Background()
	username := "testuser"
	password := "Test1234!"
	email := "test@example.com"

	userID, err := svc.CreateUser(ctx, username, password, email)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if userID <= 0 {
		t.Errorf("expected positive user ID, got %d", userID)
	}

	user, err := svc.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	if user == nil {
		t.Fatal("expected user to exist")
	}

	if user.Username != username {
		t.Errorf("expected username %s, got %s", username, user.Username)
	}

	if user.Email != email {
		t.Errorf("expected email %s, got %s", email, user.Email)
	}
}

func TestService_AuthenticateUser(t *testing.T) {
	svc, _, _ := setupTestService(t)

	ctx := context.Background()
	username := "authuser"
	password := "Test1234!"
	email := "auth@example.com"

	_, err := svc.CreateUser(ctx, username, password, email)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	user, err := svc.userStore.GetByUsername(ctx, username)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	if user == nil {
		t.Fatal("expected user to exist")
	}
}

func TestService_SendMessage(t *testing.T) {
	svc, _, _ := setupTestService(t)

	ctx := context.Background()
	user1ID, err := svc.CreateUser(ctx, "sender", "Test1234!", "sender@example.com")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	user2ID, err := svc.CreateUser(ctx, "receiver", "Test1234!", "receiver@example.com")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	user1, err := svc.GetUserByID(ctx, user1ID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	publicID1, err := svc.GetPublicIDByUserID(ctx, user1ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	publicID2, err := svc.GetPublicIDByUserID(ctx, user2ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	convID := PrivateConvID(publicID1, publicID2)
	content := "Hello, World!"

	_, _, err = svc.SendFriendRequest(ctx, user1ID, user2ID, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}

	result, err := svc.SendMessage(ctx, user1, convID, content, "")
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	if result == nil {
		t.Fatal("expected message result, got nil")
	}

	if result.ConvID != convID {
		t.Errorf("expected conv_id %s, got %s", convID, result.ConvID)
	}

	if result.Content != content {
		t.Errorf("expected content %s, got %s", content, result.Content)
	}
}

func TestService_CreateGroup(t *testing.T) {
	svc, _, _ := setupTestService(t)

	ctx := context.Background()
	userID, err := svc.CreateUser(ctx, "groupowner", "Test1234!", "owner@example.com")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	groupName := "Test Group"
	description := "A test group"

	groupID, err := svc.CreateGroup(ctx, groupName, description, userID)
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	if groupID == "" {
		t.Error("expected group ID, got empty string")
	}

	group, err := svc.GetGroupByID(ctx, groupID)
	if err != nil {
		t.Fatalf("failed to get group: %v", err)
	}

	if group == nil {
		t.Fatal("expected group to exist")
	}

	if group.Name != groupName {
		t.Errorf("expected group name %s, got %s", groupName, group.Name)
	}

	if group.OwnerID != userID {
		t.Errorf("expected owner ID %d, got %d", userID, group.OwnerID)
	}
}

func TestService_SendFriendRequest(t *testing.T) {
	svc, _, _ := setupTestService(t)

	ctx := context.Background()
	user1, err := svc.CreateUser(ctx, "user1", "Test1234!", "user1@example.com")
	if err != nil {
		t.Fatalf("failed to create user1: %v", err)
	}

	user2, err := svc.CreateUser(ctx, "user2", "Test1234!", "user2@example.com")
	if err != nil {
		t.Fatalf("failed to create user2: %v", err)
	}

	// 先让 user2 向 user1 发送请求
	_, _, err = svc.SendFriendRequest(ctx, user2, user1, "")
	if err != nil {
		t.Fatalf("failed to send friend request from user2: %v", err)
	}

	// 然后 user1 向 user2 发送请求，应该自动接受
	requestID, autoAccepted, err := svc.SendFriendRequest(ctx, user1, user2, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}

	if !autoAccepted {
		t.Error("expected friend request to be auto-accepted")
	}

	if requestID != 0 {
		t.Errorf("expected requestID to be 0 for auto-accepted, got %d", requestID)
	}
}

func TestService_SendMessage_Idempotency(t *testing.T) {
	svc, _, _ := setupTestService(t)

	ctx := context.Background()
	user1ID, err := svc.CreateUser(ctx, "sender", "Test1234!", "sender@example.com")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	user2ID, err := svc.CreateUser(ctx, "receiver", "Test1234!", "receiver@example.com")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	user1, err := svc.GetUserByID(ctx, user1ID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	publicID1, err := svc.GetPublicIDByUserID(ctx, user1ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	publicID2, err := svc.GetPublicIDByUserID(ctx, user2ID)
	if err != nil {
		t.Fatalf("failed to get public ID: %v", err)
	}

	convID := PrivateConvID(publicID1, publicID2)
	content := "Hello"
	clientMsgID := "client_msg_123"

	_, _, err = svc.SendFriendRequest(ctx, user1ID, user2ID, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}

	// First send
	result1, err := svc.SendMessage(ctx, user1, convID, content, clientMsgID)
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// Second send with same clientMsgID (should be idempotent)
	result2, err := svc.SendMessage(ctx, user1, convID, content, clientMsgID)
	if err != nil {
		t.Fatalf("failed to send duplicate message: %v", err)
	}

	// Verify idempotency
	if !result2.Duplicate {
		t.Error("expected duplicate message to be detected")
	}

	if result1.MsgID != result2.MsgID {
		t.Errorf("expected same msg_id, got %s vs %s", result1.MsgID, result2.MsgID)
	}
}
