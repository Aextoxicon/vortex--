package main

import (
	"database/sql"
	"log/slog"
)

func RunMigrations(db *sql.DB) error {
	migrations := []func(*sql.DB) error{
		createUsersTable,
		createGroupsTable,
		createGroupMembersTable,
		createFriendRequestsTable,
		createConversationParticipantsTable,
		createIdGeneratorStateTable,
		createMessageIdempotencyTable,
		createMessagesParentTable,
		createJwtBlacklistTable,
		addIsBlockedColumn,
		addEpochTimeColumn,
	}

	for _, m := range migrations {
		if err := m(db); err != nil {
			return err
		}
	}

	slog.Info("migrations completed successfully")
	return nil
}

func createUsersTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id BIGSERIAL PRIMARY KEY,
			username TEXT NOT NULL,
			pwd_hash TEXT NOT NULL,
			email TEXT UNIQUE,
			public_id TEXT NOT NULL UNIQUE,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_users_username ON users (username);
		CREATE INDEX IF NOT EXISTS idx_users_public_id ON users (public_id);
	`)
	return err
}

func createGroupsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS groups (
			group_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			owner_id BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			is_deleted INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_groups_owner_id ON groups (owner_id);
	`)
	return err
}

func createGroupMembersTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS group_members (
			id BIGSERIAL PRIMARY KEY,
			group_id TEXT NOT NULL,
			uid BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			role TEXT NOT NULL,
			joined_at BIGINT NOT NULL,
			UNIQUE (group_id, uid)
		);
		CREATE INDEX IF NOT EXISTS idx_group_members_uid ON group_members (uid);
	`)
	return err
}

func createFriendRequestsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS friend_requests (
			id BIGSERIAL PRIMARY KEY,
			from_user_id BIGINT NOT NULL,
			to_user_id BIGINT NOT NULL,
			message TEXT,
			status TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_friend_requests_pending_unique ON friend_requests (from_user_id, to_user_id) WHERE status = 'pending';
		CREATE INDEX IF NOT EXISTS idx_friend_requests_from_user_id ON friend_requests (from_user_id);
		CREATE INDEX IF NOT EXISTS idx_friend_requests_to_user_id ON friend_requests (to_user_id);
	`)
	return err
}

func createConversationParticipantsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS conversation_participants (
			conv_id TEXT NOT NULL,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			join_ts BIGINT NOT NULL,
			is_blocked INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (conv_id, user_id)
		);
		CREATE INDEX IF NOT EXISTS idx_conversation_participants_user_id ON conversation_participants (user_id);
	`)
	return err
}

func createIdGeneratorStateTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS id_generator_state (
			id BIGSERIAL PRIMARY KEY,
			last_ts BIGINT NOT NULL
		)
	`)
	return err
}

func createMessageIdempotencyTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS message_idempotency (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL,
			client_msg_id TEXT NOT NULL,
			msg_id BIGINT NOT NULL DEFAULT 0,
			conv_id TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL,
			UNIQUE (user_id, client_msg_id)
		);
		CREATE INDEX IF NOT EXISTS idx_message_idempotency_user_id ON message_idempotency (user_id);
		CREATE INDEX IF NOT EXISTS idx_message_idempotency_created_at ON message_idempotency (created_at);
	`)
	return err
}

func createMessagesParentTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			msg_id BIGINT NOT NULL,
			conv_id TEXT NOT NULL,
			from_uid BIGINT NOT NULL,
			content TEXT NOT NULL,
			ts BIGINT NOT NULL,
			is_recalled INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (msg_id, ts)
		) PARTITION BY RANGE (ts)
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_messages_conv_ts ON messages (conv_id, ts DESC, msg_id DESC)
	`)
	return err
}

func createJwtBlacklistTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS jwt_blacklist (
			jti TEXT PRIMARY KEY,
			expires_at BIGINT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_jwt_blacklist_expires_at ON jwt_blacklist (expires_at);
	`)
	return err
}

func addIsBlockedColumn(db *sql.DB) error {
	_, err := db.Exec(`
		ALTER TABLE conversation_participants
		ADD COLUMN IF NOT EXISTS is_blocked INTEGER NOT NULL DEFAULT 0
	`)
	return err
}

func addEpochTimeColumn(db *sql.DB) error {
	_, err := db.Exec(`
		ALTER TABLE id_generator_state
		ADD COLUMN IF NOT EXISTS epoch_time BIGINT NOT NULL DEFAULT 0
	`)
	return err
}
