package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *sql.DB {
	return s.db
}

type User struct {
	ID        int64  `db:"id"`
	Username  string `db:"username"`
	PwdHash   string `db:"pwd_hash"`
	Email     string `db:"email"`
	PublicID  string `db:"public_id"`
	CreatedAt int64  `db:"created_at"`
	UpdatedAt int64  `db:"updated_at"`
}

type Group struct {
	GroupID     string `db:"group_id"`
	Name        string `db:"name"`
	Description string `db:"description"`
	OwnerID     int64  `db:"owner_id"`
	CreatedAt   int64  `db:"created_at"`
	UpdatedAt   int64  `db:"updated_at"`
	IsDeleted   int    `db:"is_deleted"`
}

type GroupMember struct {
	ID       int64  `db:"id"`
	GroupID  string `db:"group_id"`
	UID      int64  `db:"uid"`
	Role     string `db:"role"`
	JoinedAt int64  `db:"joined_at"`
}

type FriendRequest struct {
	ID         int64  `db:"id"`
	FromUserID int64  `db:"from_user_id"`
	ToUserID   int64  `db:"to_user_id"`
	Message    string `db:"message"`
	Status     string `db:"status"`
	CreatedAt  int64  `db:"created_at"`
	UpdatedAt  int64  `db:"updated_at"`
}

type Message struct {
	MsgID      int64  `db:"msg_id"`
	ConvID     string `db:"conv_id"`
	FromUID    int64  `db:"from_uid"`
	Content    string `db:"content"`
	Ts         int64  `db:"ts"`
	IsRecalled int    `db:"is_recalled"`
}

type ConversationParticipant struct {
	ConvID string `db:"conv_id"`
	UserID int64  `db:"user_id"`
	JoinTs int64  `db:"join_ts"`
}

type IdGeneratorState struct {
	ID     int64 `db:"id"`
	LastTs int64 `db:"last_ts"`
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
	err := row.Scan(&s.ID, &s.LastTs)
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

func (s *UserStore) DeleteTx(tx *sql.Tx, id int64) (int64, error) {
	res, err := tx.Exec(`DELETE FROM users WHERE id = $1`, id)
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
// CRUD queries go to the specific partition (e.g. messages_20260505)
// for optimal performance without partition pruning overhead.

type MessageStore struct {
	*Store
}

func (s *MessageStore) tableName(from string) string {
	if s.IsValidTableName(from) {
		return from
	}
	return "messages"
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

func (s *MessageStore) quoteTableName(tableName string) (string, error) {
	tbl := s.tableName(tableName)
	if !s.IsValidTableName(tbl) {
		return "", fmt.Errorf("invalid table name: %s", tbl)
	}
	var quoted string
	err := s.db.QueryRow("SELECT quote_ident($1)", tbl).Scan(&quoted)
	if err != nil {
		return "", fmt.Errorf("quote table name failed: %w", err)
	}
	return quoted, nil
}

func (s *MessageStore) GetMessage(tableName string, msgID int64) (*Message, error) {
	quoted, err := s.quoteTableName(tableName)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(fmt.Sprintf(`
		SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
		FROM %s WHERE msg_id = $1`, quoted), msgID)
	return scanMessage(row)
}

func (s *MessageStore) GetConversationMessages(tableName string, convID string, limit, offset int) ([]*Message, error) {
	quoted, err := s.quoteTableName(tableName)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
		FROM %s
		WHERE conv_id = $1
		ORDER BY ts DESC
		LIMIT $2 OFFSET $3`, quoted), convID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func (s *MessageStore) GetMessagesByUser(tableName string, userID int64, limit, offset int) ([]*Message, error) {
	quoted, err := s.quoteTableName(tableName)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
		FROM %s
		WHERE from_uid = $1
		ORDER BY ts DESC
		LIMIT $2 OFFSET $3`, quoted), userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func (s *MessageStore) InsertMessage(tableName string, msg *Message) (int64, error) {
	quoted, err := s.quoteTableName(tableName)
	if err != nil {
		return 0, err
	}
	err = s.db.QueryRow(fmt.Sprintf(`
		INSERT INTO %s (msg_id, conv_id, from_uid, content, ts, is_recalled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING msg_id`, quoted),
		msg.MsgID, msg.ConvID, msg.FromUID, msg.Content, msg.Ts, msg.IsRecalled,
	).Scan(&msg.MsgID)
	return msg.MsgID, err
}

func (s *MessageStore) InsertMessageTx(tx *sql.Tx, tableName string, msg *Message) (int64, error) {
	quoted, err := s.quoteTableName(tableName)
	if err != nil {
		return 0, err
	}
	err = tx.QueryRow(fmt.Sprintf(`
		INSERT INTO %s (msg_id, conv_id, from_uid, content, ts, is_recalled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING msg_id`, quoted),
		msg.MsgID, msg.ConvID, msg.FromUID, msg.Content, msg.Ts, msg.IsRecalled,
	).Scan(&msg.MsgID)
	return msg.MsgID, err
}

func (s *MessageStore) InsertMessagesBatch(tableName string, msgs []*Message) (int64, error) {
	if len(msgs) == 0 {
		return 0, nil
	}

	quoted, err := s.quoteTableName(tableName)
	if err != nil {
		return 0, err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("INSERT INTO %s (msg_id, conv_id, from_uid, content, ts, is_recalled) VALUES ", quoted))

	args := make([]interface{}, 0, len(msgs)*6)
	for i, msg := range msgs {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d)",
			i*6+1, i*6+2, i*6+3, i*6+4, i*6+5, i*6+6))
		args = append(args, msg.MsgID, msg.ConvID, msg.FromUID, msg.Content, msg.Ts, msg.IsRecalled)
	}
	sb.WriteString(" ON CONFLICT (msg_id) DO NOTHING")

	res, err := s.db.Exec(sb.String(), args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *MessageStore) UpdateMessage(tableName string, msg *Message) (int64, error) {
	quoted, err := s.quoteTableName(tableName)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(fmt.Sprintf(`
		UPDATE %s SET conv_id = $1, from_uid = $2, content = $3, ts = $4, is_recalled = $5
		WHERE msg_id = $6`, quoted),
		msg.ConvID, msg.FromUID, msg.Content, msg.Ts, msg.IsRecalled, msg.MsgID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *MessageStore) DeleteMessage(tableName string, msgID int64) (int64, error) {
	quoted, err := s.quoteTableName(tableName)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(fmt.Sprintf(`DELETE FROM %s WHERE msg_id = $1`, quoted), msgID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type MessagePage struct {
	Messages []*Message
	HasMore  bool
	MaxMsgID int64
}

func (s *MessageStore) GetConversationMessagesByRange(convID string, startTs, endTs int64, limit int, lastMsgId int64) (*MessagePage, error) {
	queryLimit := limit + 1

	var rows *sql.Rows
	var err error

	if lastMsgId > 0 {
		rows, err = s.db.Query(`
			SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
			FROM messages
			WHERE conv_id = $1 AND ts >= $2 AND ts < $3 AND msg_id > $4
			ORDER BY ts ASC, msg_id ASC
			LIMIT $5`,
			convID, startTs, endTs, lastMsgId, queryLimit)
	} else {
		rows, err = s.db.Query(`
			SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
			FROM messages
			WHERE conv_id = $1 AND ts >= $2 AND ts < $3
			ORDER BY ts ASC, msg_id ASC
			LIMIT $4`,
			convID, startTs, endTs, queryLimit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}

	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	var maxMsgID int64
	if len(messages) > 0 {
		maxMsgID = messages[len(messages)-1].MsgID
	}

	return &MessagePage{
		Messages: messages,
		HasMore:  hasMore,
		MaxMsgID: maxMsgID,
	}, nil
}

func (s *MessageStore) GetMessageCount(tableName string, convID string) (int, error) {
	quoted, err := s.quoteTableName(tableName)
	if err != nil {
		return 0, err
	}
	var count int
	err = s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE conv_id = $1`, quoted), convID).Scan(&count)
	return count, err
}

func (s *MessageStore) MessageExists(tableName string, msgID int64) (bool, error) {
	quoted, err := s.quoteTableName(tableName)
	if err != nil {
		return false, err
	}
	var exists bool
	err = s.db.QueryRow(fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE msg_id = $1)`, quoted), msgID).Scan(&exists)
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

func (s *MessageStore) HasNewMessagesAfter(userID int64, lastMsgID int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM messages
			WHERE msg_id > $1
			AND conv_id IN (
				SELECT conv_id FROM conversation_participants WHERE user_id = $2
			)
		)`,
		lastMsgID, userID,
	).Scan(&exists)
	return exists, err
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
		ON CONFLICT (group_id, uid) DO NOTHING
		RETURNING id`,
		member.GroupID, member.UID, member.Role, member.JoinedAt,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
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

func (s *GroupMemberStore) DeleteByUserTx(tx *sql.Tx, uid int64) (int64, error) {
	res, err := tx.Exec(`DELETE FROM group_members WHERE uid = $1`, uid)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *GroupMemberStore) DeleteByGroupTx(tx *sql.Tx, groupID string) (int64, error) {
	res, err := tx.Exec(`DELETE FROM group_members WHERE group_id = $1`, groupID)
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

func (s *FriendRequestStore) HasPendingRequests(userID int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM friend_requests
			WHERE (from_user_id = $1 OR to_user_id = $1) AND status = 'pending'
		)`,
		userID,
	).Scan(&exists)
	return exists, err
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

func (s *FriendRequestStore) DeleteByUserTx(tx *sql.Tx, userID int64) (int64, error) {
	res, err := tx.Exec(`DELETE FROM friend_requests WHERE from_user_id = $1 OR to_user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *FriendRequestStore) AcceptPendingTx(tx *sql.Tx, fromUserID, toUserID int64) (int64, error) {
	var id int64
	now := time.Now().UnixMilli()
	err := tx.QueryRow(`
		UPDATE friend_requests
		SET status = 'accepted', updated_at = $1
		WHERE from_user_id = $2 AND to_user_id = $3 AND status = 'pending'
		RETURNING id`,
		now, fromUserID, toUserID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func (s *FriendRequestStore) AcceptByIDTx(tx *sql.Tx, requestID, userID int64) (fromUserID int64, err error) {
	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM friend_requests WHERE id = $1)`, requestID).Scan(&exists)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}

	now := time.Now().UnixMilli()
	err = tx.QueryRow(`
		UPDATE friend_requests
		SET status = 'accepted', updated_at = $1
		WHERE id = $2 AND to_user_id = $3 AND status = 'pending'
		RETURNING from_user_id`,
		now, requestID, userID,
	).Scan(&fromUserID)
	if err == sql.ErrNoRows {
		return -1, nil
	}
	return fromUserID, err
}

func (s *FriendRequestStore) RejectTx(tx *sql.Tx, requestID, userID int64) (bool, error) {
	now := time.Now().UnixMilli()
	var id int64
	err := tx.QueryRow(`
		UPDATE friend_requests
		SET status = 'rejected', updated_at = $1
		WHERE id = $2 AND to_user_id = $3 AND status = 'pending'
		RETURNING id`,
		now, requestID, userID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return true, err
}

func (s *FriendRequestStore) CancelTx(tx *sql.Tx, requestID, userID int64) (bool, error) {
	var id int64
	err := tx.QueryRow(`
		DELETE FROM friend_requests
		WHERE id = $1 AND from_user_id = $2 AND status = 'pending'
		RETURNING id`,
		requestID, userID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return true, err
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

func (s *ConversationParticipantStore) ExistsTx(tx *sql.Tx, convID string, userID int64) (bool, error) {
	var exists bool
	err := tx.QueryRow(`
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
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

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

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	tx = nil
	return count, nil
}

func (s *ConversationParticipantStore) InsertBatchTx(tx *sql.Tx, participants []*ConversationParticipant) (int64, error) {
	if len(participants) == 0 {
		return 0, nil
	}

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

	return count, nil
}

// ==================== IdGeneratorStateStore ====================

type IdGeneratorStateStore struct {
	*Store
}

func (s *IdGeneratorStateStore) GetFirst() (*IdGeneratorState, error) {
	row := s.db.QueryRow(`SELECT id, last_ts FROM id_generator_state ORDER BY id LIMIT 1`)
	return scanIdGenState(row)
}

func (s *IdGeneratorStateStore) GetFirstForUpdate(tx *sql.Tx) (*IdGeneratorState, error) {
	row := tx.QueryRow(`SELECT id, last_ts FROM id_generator_state ORDER BY id LIMIT 1 FOR UPDATE`)
	return scanIdGenState(row)
}

func (s *IdGeneratorStateStore) Insert(state *IdGeneratorState) (int64, error) {
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO id_generator_state (last_ts)
		VALUES ($1)
		RETURNING id`,
		state.LastTs,
	).Scan(&id)
	return id, err
}

func (s *IdGeneratorStateStore) Update(state *IdGeneratorState) (int64, error) {
	res, err := s.db.Exec(`
		UPDATE id_generator_state SET last_ts = $1
		WHERE id = $2`,
		state.LastTs, state.ID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *IdGeneratorStateStore) InsertWithTx(tx *sql.Tx, state *IdGeneratorState) (int64, error) {
	var id int64
	err := tx.QueryRow(`
		INSERT INTO id_generator_state (last_ts)
		VALUES ($1)
		RETURNING id`,
		state.LastTs,
	).Scan(&id)
	return id, err
}

func (s *IdGeneratorStateStore) UpdateWithTx(tx *sql.Tx, state *IdGeneratorState) (int64, error) {
	res, err := tx.Exec(`
		UPDATE id_generator_state SET last_ts = $1
		WHERE id = $2`,
		state.LastTs, state.ID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
