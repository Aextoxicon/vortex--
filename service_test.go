package main

import (
	"context"
	"database/sql"
	"testing"

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
		pgContainer.Cleanup(t)
		t.Fatalf("failed to open database connection: %v", err)
	}

	if err := RunMigrations(db); err != nil {
		db.Close()
		pgContainer.Cleanup(t)
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
	)

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
	userID, err := svc.CreateUser(ctx, "sender", "Test1234!", "sender@example.com")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	user, err := svc.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	convID := "conv_test_123"
	content := "Hello, World!"

	result, err := svc.SendMessage(ctx, user, convID, content, "")
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

	requestID, autoAccepted, err := svc.SendFriendRequest(ctx, user1, user2, "")
	if err != nil {
		t.Fatalf("failed to send friend request: %v", err)
	}

	if requestID <= 0 {
		t.Errorf("expected positive request ID, got %d", requestID)
	}

	if !autoAccepted {
		t.Error("expected friend request to be auto-accepted")
	}
}

func TestService_SendMessage_Idempotency(t *testing.T) {
	svc, _, _ := setupTestService(t)

	ctx := context.Background()
	userID, err := svc.CreateUser(ctx, "sender", "Test1234!", "sender@example.com")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	user, err := svc.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	convID := "conv_test"
	content := "Hello"
	clientMsgID := "client_msg_123"

	// First send
	result1, err := svc.SendMessage(ctx, user, convID, content, clientMsgID)
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	// Second send with same clientMsgID (should be idempotent)
	result2, err := svc.SendMessage(ctx, user, convID, content, clientMsgID)
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
