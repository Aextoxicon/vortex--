package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"vortex/test/testutil"

	_ "github.com/lib/pq"
)

func setupTestStore(t *testing.T) (*Store, *sql.DB, *testutil.PostgresContainer) {
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

	return &Store{db: db}, db, pgContainer
}

func TestUserStore_Insert(t *testing.T) {
	store, _, _ := setupTestStore(t)
	userStore := &UserStore{Store: store}
	ctx := context.Background()

	now := time.Now().UnixMilli()

	user := &User{
		Username:  "testuser",
		PwdHash:   "hashed_password",
		Email:     "test@example.com",
		PublicID:  "pub123",
		CreatedAt: now,
		UpdatedAt: now,
	}

	id, err := userStore.Insert(ctx, user)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}

	if id <= 0 {
		t.Errorf("expected positive user ID, got %d", id)
	}

	retrieved, err := userStore.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected user to be retrieved, got nil")
	}

	if retrieved.Username != user.Username {
		t.Errorf("expected username %s, got %s", user.Username, retrieved.Username)
	}

	if retrieved.Email != user.Email {
		t.Errorf("expected email %s, got %s", user.Email, retrieved.Email)
	}
}

func TestUserStore_GetByUsername(t *testing.T) {
	store, _, _ := setupTestStore(t)
	userStore := &UserStore{Store: store}
	ctx := context.Background()

	now := time.Now().UnixMilli()

	_, err := userStore.Insert(ctx, &User{
		Username:  "findme",
		PwdHash:   "hash",
		Email:     "find@example.com",
		PublicID:  "pub456",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	user, err := userStore.GetByUsername(ctx, "findme")
	if err != nil {
		t.Fatalf("failed to get user by username: %v", err)
	}

	if user == nil {
		t.Fatal("expected user to be found, got nil")
	}

	if user.Username != "findme" {
		t.Errorf("expected username findme, got %s", user.Username)
	}
}

func TestUserStore_GetByPublicID(t *testing.T) {
	store, _, _ := setupTestStore(t)
	userStore := &UserStore{Store: store}
	ctx := context.Background()

	now := time.Now().UnixMilli()

	_, err := userStore.Insert(ctx, &User{
		Username:  "publicuser",
		PwdHash:   "hash",
		Email:     "",
		PublicID:  "unique_public_id",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	user, err := userStore.GetByPublicID(ctx, "unique_public_id")
	if err != nil {
		t.Fatalf("failed to get user by public ID: %v", err)
	}

	if user == nil {
		t.Fatal("expected user to be found, got nil")
	}

	if user.PublicID != "unique_public_id" {
		t.Errorf("expected public ID unique_public_id, got %s", user.PublicID)
	}
}

func TestUserStore_Update(t *testing.T) {
	store, _, _ := setupTestStore(t)
	userStore := &UserStore{Store: store}
	ctx := context.Background()

	now := time.Now().UnixMilli()

	userID, err := userStore.Insert(ctx, &User{
		Username:  "updateuser",
		PwdHash:   "hash",
		Email:     "old@example.com",
		PublicID:  "pub789",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	updated := &User{
		ID:        userID,
		Username:  "updateduser",
		PwdHash:   "hash",
		Email:     "new@example.com",
		PublicID:  "pub789",
		CreatedAt: now,
		UpdatedAt: time.Now().UnixMilli(),
	}

	_, err = userStore.Update(ctx, updated)
	if err != nil {
		t.Fatalf("failed to update user: %v", err)
	}

	retrieved, err := userStore.GetByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get updated user: %v", err)
	}

	if retrieved.Username != "updateduser" {
		t.Errorf("expected username updateduser, got %s", retrieved.Username)
	}

	if retrieved.Email != "new@example.com" {
		t.Errorf("expected email new@example.com, got %s", retrieved.Email)
	}
}

func TestMessageStore_CreateMessageTable(t *testing.T) {
	store, _, _ := setupTestStore(t)
	msgStore := &MessageStore{Store: store}

	tableName, err := msgStore.CreateMessageTable("messages_20260101")
	if err != nil {
		t.Fatalf("failed to create message table: %v", err)
	}

	if tableName != 1 {
		t.Error("expected table creation to succeed")
	}
}

func TestMessageStore_InsertMessage(t *testing.T) {
	store, _, _ := setupTestStore(t)
	msgStore := &MessageStore{Store: store}
	ctx := context.Background()

	_, err := msgStore.CreateMessageTable("messages_20260102")
	if err != nil {
		t.Fatalf("failed to create message table: %v", err)
	}

	now := time.Now().UnixMilli()
	msg := &Message{
		ConvID:     "conv_123",
		FromUID:    1,
		Content:    "Hello, World!",
		Ts:         now,
		IsRecalled: 0,
	}

	msgID, err := msgStore.InsertMessage(ctx, "messages_20260102", msg)
	if err != nil {
		t.Fatalf("failed to insert message: %v", err)
	}

	if msgID <= 0 {
		t.Errorf("expected positive message ID, got %d", msgID)
	}
}

func TestMessageStore_GetConversationMessages(t *testing.T) {
	store, _, _ := setupTestStore(t)
	msgStore := &MessageStore{Store: store}
	ctx := context.Background()

	_, err := msgStore.CreateMessageTable("messages_20260103")
	if err != nil {
		t.Fatalf("failed to create message table: %v", err)
	}

	now := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		_, err := msgStore.InsertMessage(ctx, "messages_20260103", &Message{
			ConvID:     "conv_456",
			FromUID:    1,
			Content:    "Message " + string(rune('1'+i)),
			Ts:         now + int64(i)*1000,
			IsRecalled: 0,
		})
		if err != nil {
			t.Fatalf("failed to insert message: %v", err)
		}
	}

	messages, err := msgStore.GetConversationMessages(ctx, "messages_20260103", "conv_456", 10, 0)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}

	if len(messages) != 5 {
		t.Errorf("expected 5 messages, got %d", len(messages))
	}
}

func TestGroupStore_Insert(t *testing.T) {
	store, _, _ := setupTestStore(t)
	groupStore := &GroupStore{Store: store}
	ctx := context.Background()

	now := time.Now().UnixMilli()

	group := &Group{
		GroupID:     "grp_test123",
		Name:        "Test Group",
		Description: "A test group",
		OwnerID:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
		IsDeleted:   0,
	}

	_, err := groupStore.Insert(ctx, group)
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	retrieved, err := groupStore.GetByID(ctx, "grp_test123")
	if err != nil {
		t.Fatalf("failed to get group: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected group to be retrieved, got nil")
	}

	if retrieved.Name != "Test Group" {
		t.Errorf("expected group name Test Group, got %s", retrieved.Name)
	}
}

func TestGroupMemberStore_Insert(t *testing.T) {
	store, _, _ := setupTestStore(t)
	groupStore := &GroupStore{Store: store}
	groupMemStore := &GroupMemberStore{Store: store}
	ctx := context.Background()

	now := time.Now().UnixMilli()

	group := &Group{
		GroupID:     "grp_members",
		Name:        "Members Group",
		Description: "",
		OwnerID:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
		IsDeleted:   0,
	}

	_, err := groupStore.Insert(ctx, group)
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	member := &GroupMember{
		GroupID:  "grp_members",
		UID:      100,
		Role:     "member",
		JoinedAt: now,
	}

	id, err := groupMemStore.Insert(ctx, member)
	if err != nil {
		t.Fatalf("failed to insert group member: %v", err)
	}

	if id <= 0 {
		t.Errorf("expected positive member ID, got %d", id)
	}
}

func TestFriendRequestStore_Insert(t *testing.T) {
	store, _, _ := setupTestStore(t)
	friendStore := &FriendRequestStore{Store: store}
	ctx := context.Background()

	now := time.Now().UnixMilli()

	req := &FriendRequest{
		FromUserID: 1,
		ToUserID:   2,
		Message:    "Hi, let's be friends!",
		Status:     "pending",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	id, err := friendStore.Insert(ctx, req)
	if err != nil {
		t.Fatalf("failed to insert friend request: %v", err)
	}

	if id <= 0 {
		t.Errorf("expected positive request ID, got %d", id)
	}
}

func TestConversationParticipantStore_Exists(t *testing.T) {
	store, _, _ := setupTestStore(t)
	convPartStore := &ConversationParticipantStore{Store: store}
	ctx := context.Background()

	participant := &ConversationParticipant{
		ConvID:    "conv_test",
		UserID:    1,
		JoinTs:    time.Now().UnixMilli(),
		IsBlocked: 0,
	}

	_, err := store.db.ExecContext(ctx, `
		INSERT INTO conversation_participants (conv_id, user_id, join_ts, is_blocked)
		VALUES ($1, $2, $3, $4)`,
		participant.ConvID, participant.UserID, participant.JoinTs, participant.IsBlocked,
	)
	if err != nil {
		t.Fatalf("failed to insert participant: %v", err)
	}

	exists, err := convPartStore.Exists(ctx, "conv_test", 1)
	if err != nil {
		t.Fatalf("failed to check existence: %v", err)
	}

	if !exists {
		t.Error("expected participant to exist")
	}
}
