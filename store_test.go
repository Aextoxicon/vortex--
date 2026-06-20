package main

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"vortex/test/testutil"

	_ "github.com/lib/pq"
)

func setupTestStore(t *testing.T) (*Store, *sql.DB, *testutil.PostgresContainer) {
	t.Helper()

	db, pgContainer := setupTestPG(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

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

func TestUserStore_GetByIDs(t *testing.T) {
	store, _, _ := setupTestStore(t)
	userStore := &UserStore{Store: store}
	ctx := context.Background()

	now := time.Now().UnixMilli()

	user1 := &User{
		Username:  "batchuser1",
		PwdHash:   "hash1",
		Email:     "batch1@example.com",
		PublicID:  "batch_pub_1",
		CreatedAt: now,
		UpdatedAt: now,
	}
	user2 := &User{
		Username:  "batchuser2",
		PwdHash:   "hash2",
		Email:     "batch2@example.com",
		PublicID:  "batch_pub_2",
		CreatedAt: now,
		UpdatedAt: now,
	}

	id1, err := userStore.Insert(ctx, user1)
	if err != nil {
		t.Fatalf("failed to insert user1: %v", err)
	}
	id2, err := userStore.Insert(ctx, user2)
	if err != nil {
		t.Fatalf("failed to insert user2: %v", err)
	}

	result, err := userStore.GetByIDs(ctx, []int64{id1, id2})
	if err != nil {
		t.Fatalf("failed to get users by IDs: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 users, got %d", len(result))
	}

	if u, ok := result[id1]; !ok {
		t.Error("expected user1 in result")
	} else if u.Username != "batchuser1" {
		t.Errorf("expected username batchuser1, got %s", u.Username)
	}

	if u, ok := result[id2]; !ok {
		t.Error("expected user2 in result")
	} else if u.Username != "batchuser2" {
		t.Errorf("expected username batchuser2, got %s", u.Username)
	}

	emptyResult, err := userStore.GetByIDs(ctx, []int64{})
	if err != nil {
		t.Fatalf("failed to get users by empty IDs: %v", err)
	}
	if len(emptyResult) != 0 {
		t.Errorf("expected empty result for empty IDs, got %d", len(emptyResult))
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

func TestMessageStore_EnsurePartition(t *testing.T) {
	store, _, _ := setupTestStore(t)
	msgStore := &MessageStore{Store: store}

	err := msgStore.EnsurePartition("messages_20260101")
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}
}

func TestMessageStore_InsertMessage(t *testing.T) {
	store, _, _ := setupTestStore(t)
	msgStore := &MessageStore{Store: store}
	ctx := context.Background()

	tableName := MessageTableNameByDate(time.Now())
	err := msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}

	now := time.Now().UnixMilli()
	msg := &Message{
		MsgID:      time.Now().UnixNano(),
		ConvID:     "conv_123",
		FromUID:    1,
		Content:    "Hello, World!",
		Ts:         now,
		IsRecalled: 0,
	}

	msgID, err := msgStore.InsertMessage(ctx, msgStore.DB(), msg)
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

	tableName := MessageTableNameByDate(time.Now())
	err := msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}

	now := time.Now().UnixMilli()
	for i := 0; i < 5; i++ {
		_, err := msgStore.InsertMessage(ctx, msgStore.DB(), &Message{
			MsgID:      time.Now().UnixNano() + int64(i),
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

	messages, err := msgStore.GetConversationMessages(ctx, "conv_456", 10, 0)
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

	_, err := groupStore.Insert(ctx, groupStore.DB(), group)
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
	userStore := &UserStore{Store: store}
	groupStore := &GroupStore{Store: store}
	groupMemStore := &GroupMemberStore{Store: store}
	ctx := context.Background()

	now := time.Now().UnixMilli()

	userID, err := userStore.Insert(ctx, &User{
		Username:  "groupmember",
		PwdHash:   "hash",
		PublicID:  "pub_groupmember",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	group := &Group{
		GroupID:     "grp_members",
		Name:        "Members Group",
		Description: "",
		OwnerID:     userID,
		CreatedAt:   now,
		UpdatedAt:   now,
		IsDeleted:   0,
	}

	_, err = groupStore.Insert(ctx, groupStore.DB(), group)
	if err != nil {
		t.Fatalf("failed to create group: %v", err)
	}

	member := &GroupMember{
		GroupID:  "grp_members",
		UID:      userID,
		Role:     "member",
		JoinedAt: now,
	}

	id, err := groupMemStore.Insert(ctx, groupMemStore.DB(), member)
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

	id, err := friendStore.Insert(ctx, friendStore.DB(), req)
	if err != nil {
		t.Fatalf("failed to insert friend request: %v", err)
	}

	if id <= 0 {
		t.Errorf("expected positive request ID, got %d", id)
	}
}

func TestConversationParticipantStore_Exists(t *testing.T) {
	store, _, _ := setupTestStore(t)
	userStore := &UserStore{Store: store}
	convPartStore := &ConversationParticipantStore{Store: store}
	ctx := context.Background()

	now := time.Now().UnixMilli()

	userID, err := userStore.Insert(ctx, &User{
		Username:  "convparticipant",
		PwdHash:   "hash",
		PublicID:  "pub_convparticipant",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	participant := &ConversationParticipant{
		ConvID:    "conv_test",
		UserID:    userID,
		JoinTs:    now,
		IsBlocked: 0,
	}

	_, err = store.db.ExecContext(ctx, `
		INSERT INTO conversation_participants (conv_id, user_id, join_ts, is_blocked)
		VALUES ($1, $2, $3, $4)`,
		participant.ConvID, participant.UserID, participant.JoinTs, participant.IsBlocked,
	)
	if err != nil {
		t.Fatalf("failed to insert participant: %v", err)
	}

	exists, err := convPartStore.Exists(ctx, convPartStore.DB(), "conv_test", userID)
	if err != nil {
		t.Fatalf("failed to check existence: %v", err)
	}

	if !exists {
		t.Error("expected participant to exist")
	}
}

func TestMessageStore_GetMessagesAfter(t *testing.T) {
	svc, _, _ := setupTestService(t)
	msgStore := svc.msgStore
	ctx := context.Background()

	tableName := MessageTableNameByDate(time.Now())
	err := msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}

	convID := "conv_get_after"
	now := time.Now().UnixMilli()

	// Insert 5 messages with ascending msg_id
	var msgIDs []int64
	for i := 0; i < 5; i++ {
		msgID := time.Now().UnixNano() + int64(i)
		_, err := msgStore.InsertMessage(ctx, msgStore.DB(), &Message{
			MsgID:      msgID,
			ConvID:     convID,
			FromUID:    1,
			Content:    "Message " + string(rune('A'+i)),
			Ts:         now + int64(i)*1000,
			IsRecalled: 0,
		})
		if err != nil {
			t.Fatalf("failed to insert message %d: %v", i, err)
		}
		msgIDs = append(msgIDs, msgID)
	}

	// Get messages after first message
	page, err := msgStore.GetMessagesAfter(ctx, convID, msgIDs[0], 10)
	if err != nil {
		t.Fatalf("failed to get messages after: %v", err)
	}

	if page == nil {
		t.Fatal("expected non-nil page")
	}

	if len(page.Messages) != 4 {
		t.Errorf("expected 4 messages after first, got %d", len(page.Messages))
	}

	if page.MaxMsgID != msgIDs[4] {
		t.Errorf("expected maxMsgID %d, got %d", msgIDs[4], page.MaxMsgID)
	}

	// Test with limit smaller than remaining
	page, err = msgStore.GetMessagesAfter(ctx, convID, msgIDs[0], 2)
	if err != nil {
		t.Fatalf("failed to get messages after with limit: %v", err)
	}

	if len(page.Messages) != 2 {
		t.Errorf("expected 2 messages with limit=2, got %d", len(page.Messages))
	}

	if page.MaxMsgID != msgIDs[2] {
		t.Errorf("expected maxMsgID %d, got %d", msgIDs[2], page.MaxMsgID)
	}
}

func TestMessageStore_GetUpdatedConversations(t *testing.T) {
	svc, _, _ := setupTestService(t)
	msgStore := svc.msgStore
	ctx := context.Background()

	tableName := MessageTableNameByDate(time.Now())
	err := msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}

	// Create a user to be a participant
	user, err := svc.CreateUser(ctx, uniqueUsername(), "Test1234!", uniqueUsername()+"@example.com", "")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	convID := "conv_updated"
	now := time.Now().UnixMilli()

	// Insert a message
	msgID := time.Now().UnixNano()
	_, err = msgStore.InsertMessage(ctx, msgStore.DB(), &Message{
		MsgID:      msgID,
		ConvID:     convID,
		FromUID:    user.ID,
		Content:    "Hello",
		Ts:         now,
		IsRecalled: 0,
	})
	if err != nil {
		t.Fatalf("failed to insert message: %v", err)
	}

	// Insert participant
	_, err = svc.convPartStore.DB().ExecContext(ctx, `
		INSERT INTO conversation_participants (conv_id, user_id, join_ts, is_blocked)
		VALUES ($1, $2, $3, 0)`,
		convID, user.ID, now,
	)
	if err != nil {
		t.Fatalf("failed to insert participant: %v", err)
	}

	// Query updated conversations with lastMsgID < msgID
	convIDs, err := msgStore.GetUpdatedConversations(ctx, user.ID, msgID-1)
	if err != nil {
		t.Fatalf("failed to get updated conversations: %v", err)
	}

	if len(convIDs) == 0 {
		t.Error("expected at least one updated conversation")
	}

	found := false
	for _, id := range convIDs {
		if id == convID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected convID %s in updated conversations, got %v", convID, convIDs)
	}

	// Query with lastMsgID > msgID should return empty
	convIDs, err = msgStore.GetUpdatedConversations(ctx, user.ID, msgID+1)
	if err != nil {
		t.Fatalf("failed to get updated conversations with high lastMsgID: %v", err)
	}

	if len(convIDs) != 0 {
		t.Errorf("expected empty result, got %v", convIDs)
	}
}

func TestConversationParticipantStore_CountConversations(t *testing.T) {
	svc, _, _ := setupTestService(t)
	convPartStore := svc.convPartStore
	ctx := context.Background()

	user, err := svc.CreateUser(ctx, uniqueUsername(), "Test1234!", uniqueUsername()+"@example.com", "")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	now := time.Now().UnixMilli()

	for i := 0; i < 3; i++ {
		convID := fmt.Sprintf("conv_count_%d", i)
		_, err := svc.convPartStore.DB().ExecContext(ctx, `
			INSERT INTO conversation_participants (conv_id, user_id, join_ts, is_blocked)
			VALUES ($1, $2, $3, 0)`,
			convID, user.ID, now+int64(i),
		)
		if err != nil {
			t.Fatalf("failed to insert participant %d: %v", i, err)
		}
	}

	count, err := convPartStore.CountConversations(ctx, user.ID)
	if err != nil {
		t.Fatalf("failed to count conversations: %v", err)
	}

	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}

	// Test with user that has no conversations
	count, err = convPartStore.CountConversations(ctx, -1)
	if err != nil {
		t.Fatalf("failed to count conversations for non-existent user: %v", err)
	}

	if count != 0 {
		t.Errorf("expected count 0 for non-existent user, got %d", count)
	}
}

func TestConversationParticipantStore_UpdateLastMsgCache(t *testing.T) {
	svc, db, _ := setupTestService(t)
	convPartStore := svc.convPartStore
	msgStore := svc.msgStore
	ctx := context.Background()

	tableName := MessageTableNameByDate(time.Now())
	err := msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}

	now := time.Now().UnixMilli()
	convID := "conv_cache_test"

	_, err = db.ExecContext(ctx, `
		INSERT INTO conversation_participants (conv_id, user_id, join_ts, is_blocked, last_msg_id, last_msg_content, last_msg_ts, last_msg_is_recalled)
		VALUES ($1, $2, $3, 0, 0, '', 0, 0)`,
		convID, 1, now,
	)
	if err != nil {
		t.Fatalf("failed to insert participant: %v", err)
	}

	msgID := time.Now().UnixNano()
	msg := &Message{
		MsgID:      msgID,
		ConvID:     convID,
		FromUID:    1,
		Content:    "Cache test message",
		Ts:         now,
		IsRecalled: 0,
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	defer tx.Rollback()

	err = convPartStore.UpdateLastMsgCache(tx, msg)
	if err != nil {
		t.Fatalf("failed to update last msg cache: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit tx: %v", err)
	}

	var lastMsgID int64
	var lastMsgContent string
	err = db.QueryRowContext(ctx, `
		SELECT last_msg_id, last_msg_content 
		FROM conversation_participants 
		WHERE conv_id = $1 AND user_id = $2`,
		convID, 1,
	).Scan(&lastMsgID, &lastMsgContent)
	if err != nil {
		t.Fatalf("failed to query cache: %v", err)
	}

	if lastMsgID != msgID {
		t.Errorf("expected last_msg_id %d, got %d", msgID, lastMsgID)
	}

	if lastMsgContent != "Cache test message" {
		t.Errorf("expected last_msg_content 'Cache test message', got '%s'", lastMsgContent)
	}
}

func TestConversationParticipantStore_UpdateLastMsgCacheOnRecall(t *testing.T) {
	svc, db, _ := setupTestService(t)
	convPartStore := svc.convPartStore
	ctx := context.Background()

	now := time.Now().UnixMilli()
	convID := "conv_recall_cache"
	recalledMsgID := int64(12345)

	_, err := db.ExecContext(ctx, `
		INSERT INTO conversation_participants (conv_id, user_id, join_ts, is_blocked, last_msg_id, last_msg_content, last_msg_ts, last_msg_is_recalled)
		VALUES ($1, $2, $3, 0, $4, 'original content', $5, 0)`,
		convID, 1, now, recalledMsgID, now,
	)
	if err != nil {
		t.Fatalf("failed to insert participant: %v", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	defer tx.Rollback()

	newContent := "[已撤回]"
	err = convPartStore.UpdateLastMsgCacheOnRecall(tx, convID, recalledMsgID, newContent, 1)
	if err != nil {
		t.Fatalf("failed to update last msg cache on recall: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit tx: %v", err)
	}

	var lastMsgContent string
	var lastMsgIsRecalled int
	err = db.QueryRowContext(ctx, `
		SELECT last_msg_content, last_msg_is_recalled 
		FROM conversation_participants 
		WHERE conv_id = $1 AND user_id = $2`,
		convID, 1,
	).Scan(&lastMsgContent, &lastMsgIsRecalled)
	if err != nil {
		t.Fatalf("failed to query cache: %v", err)
	}

	if lastMsgContent != newContent {
		t.Errorf("expected last_msg_content '%s', got '%s'", newContent, lastMsgContent)
	}

	if lastMsgIsRecalled != 1 {
		t.Errorf("expected last_msg_is_recalled 1, got %d", lastMsgIsRecalled)
	}

	// Verify update only applies to matching last_msg_id
	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx2: %v", err)
	}
	defer tx2.Rollback()

	err = convPartStore.UpdateLastMsgCacheOnRecall(tx2, "nonexistent_conv", recalledMsgID, newContent, 1)
	if err != nil {
		t.Fatalf("failed to update on recall for non-matching conv: %v", err)
	}

	var stillExists int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM conversation_participants 
		WHERE conv_id = $1 AND user_id = $2 AND last_msg_content = $3`,
		convID, 1, newContent,
	).Scan(&stillExists)
	if err != nil {
		t.Fatalf("failed to verify: %v", err)
	}
	if stillExists != 1 {
		t.Error("existing record should still be updated after non-matching update")
	}
}
