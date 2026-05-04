package main

import (
	"database/sql"
	"fmt"
	"time"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// ==================== helpers ====================

func scanUser(row *sql.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PwdHash, &u.Email, &u.PublicID, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func scanUsers(rows *sql.Rows) ([]*User, error) {
	var result []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PwdHash, &u.Email, &u.PublicID, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, &u)
	}
	return result, rows.Err()
}

func scanMessage(row *sql.Row) (*Message, error) {
	var m Message
	err := row.Scan(&m.MsgID, &m.ConvID, &m.FromUID, &m.Content, &m.Ts, &m.IsRecalled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &m, err
}

func scanMessages(rows *sql.Rows) ([]*Message, error) {
	var result []*Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.MsgID, &m.ConvID, &m.FromUID, &m.Content, &m.Ts, &m.IsRecalled); err != nil {
			return nil, err
		}
		result = append(result, &m)
	}
	return result, rows.Err()
}

func scanGroup(row *sql.Row) (*Group, error) {
	var g Group
	err := row.Scan(&g.GroupID, &g.Name, &g.Description, &g.OwnerID, &g.CreatedAt, &g.UpdatedAt, &g.IsDeleted)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &g, err
}

func scanGroups(rows *sql.Rows) ([]*Group, error) {
	var result []*Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.GroupID, &g.Name, &g.Description, &g.OwnerID, &g.CreatedAt, &g.UpdatedAt, &g.IsDeleted); err != nil {
			return nil, err
		}
		result = append(result, &g)
	}
	return result, rows.Err()
}

func scanGroupMember(row *sql.Row) (*GroupMember, error) {
	var m GroupMember
	err := row.Scan(&m.ID, &m.GroupID, &m.UID, &m.Role, &m.JoinedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &m, err
}

func scanFriendRequest(row *sql.Row) (*FriendRequest, error) {
	var r FriendRequest
	err := row.Scan(&r.ID, &r.FromUserID, &r.ToUserID, &r.Message, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &r, err
}

func scanFriendRequests(rows *sql.Rows) ([]*FriendRequest, error) {
	var result []*FriendRequest
	for rows.Next() {
		var r FriendRequest
		if err := rows.Scan(&r.ID, &r.FromUserID, &r.ToUserID, &r.Message, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, &r)
	}
	return result, rows.Err()
}

func scanIdGenState(row *sql.Row) (*IdGeneratorState, error) {
	var s IdGeneratorState
	err := row.Scan(&s.ID, &s.LastTs, &s.LastSeq)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

// ==================== UserStore ====================

type UserStore struct {
	*Store
}

func (s *UserStore) GetByID(id int64) (*User, error) {
	row := s.db.QueryRow(`SELECT id, username, pwd_hash, email, public_id, created_at, updated_at FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (s *UserStore) GetByUsername(username string) (*User, error) {
	row := s.db.QueryRow(`SELECT id, username, pwd_hash, email, public_id, created_at, updated_at FROM users WHERE username = $1`, username)
	return scanUser(row)
}

func (s *UserStore) GetByPublicID(publicID string) (*User, error) {
	row := s.db.QueryRow(`SELECT id, username, pwd_hash, email, public_id, created_at, updated_at FROM users WHERE public_id = $1`, publicID)
	return scanUser(row)
}

func (s *UserStore) Insert(user *User) (int64, error) {
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO users (username, pwd_hash, email, public_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		user.Username, user.PwdHash, user.Email, user.PublicID, user.CreatedAt, user.UpdatedAt,
	).Scan(&id)
	return id, err
}

func (s *UserStore) Update(user *User) (int64, error) {
	res, err := s.db.Exec(`
		UPDATE users SET username = $1, pwd_hash = $2, email = $3, public_id = $4, created_at = $5, updated_at = $6
		WHERE id = $7`,
		user.Username, user.PwdHash, user.Email, user.PublicID, user.CreatedAt, user.UpdatedAt, user.ID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *UserStore) Delete(id int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *UserStore) UsernameExists(username string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)`, username).Scan(&exists)
	return exists, err
}

func (s *UserStore) EmailExists(email string) (bool, error) {
	if email == "" {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists)
	return exists, err
}

// ==================== MessageStore ====================
//
// Uses Postgres declarative partitioning on ts (milliseconds).
// The parent partitioned table "messages" is created once.
// Daily partitions "messages_YYYYMMDD" are created as partition children.
// All CRUD queries go to the parent "messages" table — Postgres partition
// pruning routes to the correct daily partition automatically.

type MessageStore struct {
	*Store
}

// setupParentTable ensures the partitioned parent exists
func (s *MessageStore) setupParentTable() error {
	sql := `
		CREATE TABLE IF NOT EXISTS messages (
			msg_id BIGINT NOT NULL,
			conv_id TEXT NOT NULL,
			from_uid BIGINT NOT NULL,
			content TEXT NOT NULL,
			ts BIGINT NOT NULL,
			is_recalled INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (msg_id, ts)
		) PARTITION BY RANGE (ts)`
	_, err := s.db.Exec(sql)
	return err
}

func (s *MessageStore) IsValidTableName(tableName string) bool {
	if len(tableName) != 17 {
		return false
	}
	if tableName[:9] != "messages_" {
		return false
	}
	_, err := time.Parse("20060102", tableName[9:])
	return err == nil
}

func (s *MessageStore) GetMessage(tableName string, msgID int64) (*Message, error) {
	row := s.db.QueryRow(`
		SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
		FROM messages WHERE msg_id = $1`, msgID)
	return scanMessage(row)
}

func (s *MessageStore) GetConversationMessages(tableName string, convID string, limit, offset int) ([]*Message, error) {
	rows, err := s.db.Query(`
		SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
		FROM messages
		WHERE conv_id = $1
		ORDER BY ts DESC
		LIMIT $2 OFFSET $3`, convID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func (s *MessageStore) GetMessagesByUser(tableName string, userID int64, limit, offset int) ([]*Message, error) {
	rows, err := s.db.Query(`
		SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
		FROM messages
		WHERE from_uid = $1
		ORDER BY ts DESC
		LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func (s *MessageStore) InsertMessage(tableName string, msg *Message) (int64, error) {
	err := s.db.QueryRow(`
		INSERT INTO messages (msg_id, conv_id, from_uid, content, ts, is_recalled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING msg_id`,
		msg.MsgID, msg.ConvID, msg.FromUID, msg.Content, msg.Ts, msg.IsRecalled,
	).Scan(&msg.MsgID)
	return msg.MsgID, err
}

func (s *MessageStore) InsertMessagesBatch(tableName string, msgs []*Message) (int64, error) {
	if len(msgs) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO messages (msg_id, conv_id, from_uid, content, ts, is_recalled)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (msg_id) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	var count int64
	for _, msg := range msgs {
		res, err := stmt.Exec(msg.MsgID, msg.ConvID, msg.FromUID, msg.Content, msg.Ts, msg.IsRecalled)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		count += n
	}

	return count, tx.Commit()
}

func (s *MessageStore) UpdateMessage(tableName string, msg *Message) (int64, error) {
	res, err := s.db.Exec(`
		UPDATE messages SET conv_id = $1, from_uid = $2, content = $3, ts = $4, is_recalled = $5
		WHERE msg_id = $6`,
		msg.ConvID, msg.FromUID, msg.Content, msg.Ts, msg.IsRecalled, msg.MsgID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *MessageStore) DeleteMessage(tableName string, msgID int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM messages WHERE msg_id = $1`, msgID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *MessageStore) GetMessageCount(tableName string, convID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE conv_id = $1`, convID).Scan(&count)
	return count, err
}

func (s *MessageStore) MessageExists(tableName string, msgID int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM messages WHERE msg_id = $1)`, msgID).Scan(&exists)
	return exists, err
}

func (s *MessageStore) CreateMessageTable(tableName string) (int64, error) {
	if !s.IsValidTableName(tableName) {
		return 0, fmt.Errorf("invalid table name: %s", tableName)
	}
	// ensure parent partitioned table exists
	if err := s.setupParentTable(); err != nil {
		return 0, err
	}
	// create daily partition
	date, _ := time.Parse("20060102", tableName[9:])
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
	endOfDay := time.Date(date.Year(), date.Month(), date.Day()+1, 0, 0, 0, 0, time.UTC).UnixMilli()

	sql := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s PARTITION OF messages
		FOR VALUES FROM (%d) TO (%d)`,
		tableName, startOfDay, endOfDay)

	_, err := s.db.Exec(sql)
	if err != nil {
		return 0, err
	}

	// create indexes on the partition
	_, err = s.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_msg_conv_ts_%s ON %s (conv_id, ts DESC)`, tableName[9:], tableName))
	if err != nil {
		return 1, err
	}
	_, err = s.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_msg_conv_id_%s ON %s (conv_id, msg_id)`, tableName[9:], tableName))
	if err != nil {
		return 1, err
	}
	_, err = s.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_msg_from_uid_%s ON %s (from_uid)`, tableName[9:], tableName))
	if err != nil {
		return 1, err
	}
	_, err = s.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_msg_id_uid_%s ON %s (msg_id, from_uid)`, tableName[9:], tableName))
	return 1, err
}

func (s *MessageStore) DropMessageTable(tableName string) (int64, error) {
	if !s.IsValidTableName(tableName) {
		return 0, fmt.Errorf("invalid table name: %s", tableName)
	}
	res, err := s.db.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS %s`, tableName))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *MessageStore) GetMaxMessageID(tableName string) (int64, error) {
	var maxID sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(msg_id) FROM messages`).Scan(&maxID)
	if err != nil {
		return 0, err
	}
	if maxID.Valid {
		return maxID.Int64, nil
	}
	return 0, nil
}

func (s *MessageStore) GetMaxMessageIDsFromRecentTables(days int) ([]int64, error) {
	// for partitioned table, just get global max
	maxID, err := s.GetMaxMessageID("")
	if err != nil {
		return nil, err
	}
	if maxID > 0 {
		return []int64{maxID}, nil
	}
	return nil, nil
}

// ==================== GroupStore ====================

type GroupStore struct {
	*Store
}

func (s *GroupStore) GetByID(groupID string) (*Group, error) {
	row := s.db.QueryRow(`
		SELECT group_id, name, description, owner_id, created_at, updated_at, is_deleted
		FROM groups WHERE group_id = $1 AND is_deleted = 0`, groupID)
	return scanGroup(row)
}

func (s *GroupStore) GetByName(name string) (*Group, error) {
	row := s.db.QueryRow(`
		SELECT group_id, name, description, owner_id, created_at, updated_at, is_deleted
		FROM groups WHERE name = $1 AND is_deleted = 0`, name)
	return scanGroup(row)
}

func (s *GroupStore) GetGroupsByOwner(ownerID int64) ([]*Group, error) {
	rows, err := s.db.Query(`
		SELECT group_id, name, description, owner_id, created_at, updated_at, is_deleted
		FROM groups WHERE owner_id = $1 AND is_deleted = 0
		ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroups(rows)
}

func (s *GroupStore) Insert(group *Group) (int64, error) {
	_, err := s.db.Exec(`
		INSERT INTO groups (group_id, name, description, owner_id, created_at, updated_at, is_deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		group.GroupID, group.Name, group.Description, group.OwnerID, group.CreatedAt, group.UpdatedAt, group.IsDeleted,
	)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func (s *GroupStore) Update(group *Group) (int64, error) {
	res, err := s.db.Exec(`
		UPDATE groups SET name = $1, description = $2, owner_id = $3, created_at = $4, updated_at = $5, is_deleted = $6
		WHERE group_id = $7`,
		group.Name, group.Description, group.OwnerID, group.CreatedAt, group.UpdatedAt, group.IsDeleted, group.GroupID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *GroupStore) Delete(groupID string) (int64, error) {
	res, err := s.db.Exec(`UPDATE groups SET is_deleted = 1 WHERE group_id = $1`, groupID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *GroupStore) NameExists(name string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM groups WHERE name = $1 AND is_deleted = 0)`, name).Scan(&exists)
	return exists, err
}

// ==================== GroupMemberStore ====================

type GroupMemberStore struct {
	*Store
}

func (s *GroupMemberStore) Get(groupID string, uid int64) (*GroupMember, error) {
	row := s.db.QueryRow(`
		SELECT id, group_id, uid, role, joined_at
		FROM group_members WHERE group_id = $1 AND uid = $2`, groupID, uid)
	return scanGroupMember(row)
}

func (s *GroupMemberStore) Insert(member *GroupMember) (int64, error) {
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO group_members (group_id, uid, role, joined_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		member.GroupID, member.UID, member.Role, member.JoinedAt,
	).Scan(&id)
	return id, err
}

func (s *GroupMemberStore) DeleteByGroupAndUser(groupID string, uid int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM group_members WHERE group_id = $1 AND uid = $2`, groupID, uid)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *GroupMemberStore) DeleteByUser(uid int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM group_members WHERE uid = $1`, uid)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *GroupMemberStore) IsMember(groupID string, uid int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id = $1 AND uid = $2)`, groupID, uid).Scan(&exists)
	return exists, err
}

// ==================== FriendRequestStore ====================

type FriendRequestStore struct {
	*Store
}

func (s *FriendRequestStore) GetByID(id int64) (*FriendRequest, error) {
	row := s.db.QueryRow(`
		SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
		FROM friend_requests WHERE id = $1`, id)
	return scanFriendRequest(row)
}

func (s *FriendRequestStore) GetByUsers(fromUserID, toUserID int64) (*FriendRequest, error) {
	row := s.db.QueryRow(`
		SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
		FROM friend_requests WHERE from_user_id = $1 AND to_user_id = $2
		ORDER BY created_at DESC LIMIT 1`, fromUserID, toUserID)
	return scanFriendRequest(row)
}

func (s *FriendRequestStore) GetSentRequests(fromUserID int64) ([]*FriendRequest, error) {
	rows, err := s.db.Query(`
		SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
		FROM friend_requests WHERE from_user_id = $1
		ORDER BY created_at DESC`, fromUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFriendRequests(rows)
}

func (s *FriendRequestStore) GetReceivedRequests(toUserID int64) ([]*FriendRequest, error) {
	rows, err := s.db.Query(`
		SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
		FROM friend_requests WHERE to_user_id = $1
		ORDER BY created_at DESC`, toUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFriendRequests(rows)
}

func (s *FriendRequestStore) GetPendingRequests(userID int64) ([]*FriendRequest, error) {
	rows, err := s.db.Query(`
		SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
		FROM friend_requests WHERE (from_user_id = $1 OR to_user_id = $1) AND status = 'pending'
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFriendRequests(rows)
}

func (s *FriendRequestStore) Insert(req *FriendRequest) (int64, error) {
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO friend_requests (from_user_id, to_user_id, message, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		req.FromUserID, req.ToUserID, req.Message, req.Status, req.CreatedAt, req.UpdatedAt,
	).Scan(&id)
	return id, err
}

func (s *FriendRequestStore) Update(req *FriendRequest) (int64, error) {
	res, err := s.db.Exec(`
		UPDATE friend_requests SET from_user_id = $1, to_user_id = $2, message = $3, status = $4, created_at = $5, updated_at = $6
		WHERE id = $7`,
		req.FromUserID, req.ToUserID, req.Message, req.Status, req.CreatedAt, req.UpdatedAt, req.ID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *FriendRequestStore) Delete(id int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM friend_requests WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *FriendRequestStore) DeleteByUser(userID int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM friend_requests WHERE from_user_id = $1 OR to_user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ==================== ConversationParticipantStore ====================

type ConversationParticipantStore struct {
	*Store
}

func (s *ConversationParticipantStore) Exists(convID string, userID int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM conversation_participants WHERE conv_id = $1 AND user_id = $2)`,
		convID, userID,
	).Scan(&exists)
	return exists, err
}

func (s *ConversationParticipantStore) InsertBatch(participants []*ConversationParticipant) (int64, error) {
	if len(participants) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO conversation_participants (conv_id, user_id, join_ts)
		VALUES ($1, $2, $3)
		ON CONFLICT (conv_id, user_id) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	var count int64
	for _, p := range participants {
		res, err := stmt.Exec(p.ConvID, p.UserID, p.JoinTs)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		count += n
	}

	return count, tx.Commit()
}

// ==================== UserDeviceStore ====================

type UserDeviceStore struct {
	*Store
}

func (s *UserDeviceStore) TokenBelongsToUser(userID int64, deviceToken string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM user_devices WHERE user_id = $1 AND device_token = $2 AND is_active = 1)`,
		userID, deviceToken,
	).Scan(&exists)
	return exists, err
}

func (s *UserDeviceStore) ClearDeviceToken(userID int64, deviceToken string) (int64, error) {
	res, err := s.db.Exec(`
		UPDATE user_devices SET is_active = 0, updated_at = EXTRACT(EPOCH FROM now())::bigint
		WHERE user_id = $1 AND device_token = $2`, userID, deviceToken)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *UserDeviceStore) DeleteByUser(userID int64) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM user_devices WHERE user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ==================== IdGeneratorStateStore ====================

type IdGeneratorStateStore struct {
	*Store
}

func (s *IdGeneratorStateStore) GetFirst() (*IdGeneratorState, error) {
	row := s.db.QueryRow(`SELECT id, last_ts, last_seq FROM id_generator_state ORDER BY id LIMIT 1`)
	return scanIdGenState(row)
}

func (s *IdGeneratorStateStore) Insert(state *IdGeneratorState) (int64, error) {
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO id_generator_state (last_ts, last_seq)
		VALUES ($1, $2)
		RETURNING id`,
		state.LastTs, state.LastSeq,
	).Scan(&id)
	return id, err
}

func (s *IdGeneratorStateStore) Update(state *IdGeneratorState) (int64, error) {
	res, err := s.db.Exec(`
		UPDATE id_generator_state SET last_ts = $1, last_seq = $2
		WHERE id = $3`,
		state.LastTs, state.LastSeq, state.ID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
