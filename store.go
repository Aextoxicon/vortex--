package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SQLExecutor 统一 *sql.DB 和 *sql.Tx 的查询接口
type SQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type Store struct {
	db        *sql.DB
	epochTime int64
}

func NewStore(db *sql.DB, epochTime int64) *Store {
	return &Store{db: db, epochTime: epochTime}
}

func (s *Store) SetEpochTime(epochTime int64) {
	s.epochTime = epochTime
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
	Bio       string `db:"bio"`
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
	ConvID    string `db:"conv_id"`
	UserID    int64  `db:"user_id"`
	JoinTs    int64  `db:"join_ts"`
	IsBlocked int    `db:"is_blocked"`
}

type IdGeneratorState struct {
	ID        int64 `db:"id"`
	LastTs    int64 `db:"last_ts"`
	EpochTime int64 `db:"epoch_time"`
}

type MessageIdempotency struct {
	ID          int64  `db:"id"`
	UserID      int64  `db:"user_id"`
	ClientMsgID string `db:"client_msg_id"`
	MsgID       int64  `db:"msg_id"`
	ConvID      string `db:"conv_id"`
	CreatedAt   int64  `db:"created_at"`
}

// ==================== 泛型 scan 辅助 ====================

// Scanner 兼容 *sql.Row 和 *sql.Rows
type Scanner interface {
	Scan(dest ...any) error
}

// scanOne 泛型单行扫描：自动将 sql.ErrNoRows 转为 (nil, nil)
func scanOne[T any](row *sql.Row, scanFn func(Scanner) (*T, error)) (*T, error) {
	item, err := scanFn(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

// scanMany 泛型多行扫描：自动处理 rows.Next() 循环、defer rows.Close()、rows.Err()
func scanMany[T any](rows *sql.Rows, scanFn func(Scanner) (*T, error)) ([]*T, error) {
	defer rows.Close()
	var result []*T
	for rows.Next() {
		item, err := scanFn(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ==================== 实体字段映射函数 ====================

func scanUserFields(s Scanner) (*User, error) {
	var u User
	err := s.Scan(&u.ID, &u.Username, &u.PwdHash, &u.Email, &u.PublicID, &u.Bio, &u.CreatedAt, &u.UpdatedAt)
	return &u, err
}

func scanMessageFields(s Scanner) (*Message, error) {
	var m Message
	err := s.Scan(&m.MsgID, &m.ConvID, &m.FromUID, &m.Content, &m.Ts, &m.IsRecalled)
	return &m, err
}

func scanGroupFields(s Scanner) (*Group, error) {
	var g Group
	err := s.Scan(&g.GroupID, &g.Name, &g.Description, &g.OwnerID, &g.CreatedAt, &g.UpdatedAt, &g.IsDeleted)
	return &g, err
}

func scanGroupMemberFields(s Scanner) (*GroupMember, error) {
	var m GroupMember
	err := s.Scan(&m.ID, &m.GroupID, &m.UID, &m.Role, &m.JoinedAt)
	return &m, err
}

func scanFriendRequestFields(s Scanner) (*FriendRequest, error) {
	var r FriendRequest
	err := s.Scan(&r.ID, &r.FromUserID, &r.ToUserID, &r.Message, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	return &r, err
}

func scanIdGenStateFields(s Scanner) (*IdGeneratorState, error) {
	var st IdGeneratorState
	err := s.Scan(&st.ID, &st.LastTs, &st.EpochTime)
	return &st, err
}

// ==================== 旧扫描函数（保留签名兼容，委托给泛型） ====================

func scanUser(row *sql.Row) (*User, error) {
	return scanOne(row, scanUserFields)
}

func scanMessage(row *sql.Row) (*Message, error) {
	return scanOne(row, scanMessageFields)
}

func scanMessages(rows *sql.Rows) ([]*Message, error) {
	return scanMany(rows, scanMessageFields)
}

func scanGroup(row *sql.Row) (*Group, error) {
	return scanOne(row, scanGroupFields)
}

func scanGroups(rows *sql.Rows) ([]*Group, error) {
	return scanMany(rows, scanGroupFields)
}

func scanGroupMember(row *sql.Row) (*GroupMember, error) {
	return scanOne(row, scanGroupMemberFields)
}

func scanFriendRequest(row *sql.Row) (*FriendRequest, error) {
	return scanOne(row, scanFriendRequestFields)
}

func scanFriendRequests(rows *sql.Rows) ([]*FriendRequest, error) {
	return scanMany(rows, scanFriendRequestFields)
}

func scanIdGenState(row *sql.Row) (*IdGeneratorState, error) {
	return scanOne(row, scanIdGenStateFields)
}

// ==================== UserStore ====================

type UserStore struct {
	*Store
}

func (s *UserStore) GetByIDs(ctx context.Context, ids []int64) (map[int64]*User, error) {
	if len(ids) == 0 {
		return make(map[int64]*User), nil
	}

	query := `SELECT id, username, pwd_hash, email, public_id, bio, created_at, updated_at FROM users WHERE id = ANY($1)`
	rows, err := s.db.QueryContext(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]*User)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PwdHash, &u.Email, &u.PublicID, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		result[u.ID] = &u
	}
	return result, rows.Err()
}

func (s *UserStore) GetByID(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, pwd_hash, email, public_id, bio, created_at, updated_at FROM users WHERE id = $1`, id)
	return scanUser(row)
}

func (s *UserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, pwd_hash, email, public_id, bio, created_at, updated_at FROM users WHERE username = $1`, username)
	return scanUser(row)
}

func (s *UserStore) GetByPublicID(ctx context.Context, publicID string) (*User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, pwd_hash, email, public_id, bio, created_at, updated_at FROM users WHERE public_id = $1`, publicID)
	return scanUser(row)
}

func (s *UserStore) Insert(ctx context.Context, user *User) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO users (username, pwd_hash, email, public_id, bio, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		user.Username, user.PwdHash, user.Email, user.PublicID, user.Bio, user.CreatedAt, user.UpdatedAt,
	).Scan(&id)
	return id, err
}

func (s *UserStore) Update(ctx context.Context, user *User) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE users SET username = $1, email = $2, bio = $3, updated_at = $4
		WHERE id = $5`,
		user.Username, user.Email, user.Bio, user.UpdatedAt, user.ID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ==================== MessageIdempotencyStore ====================

type MessageIdempotencyStore struct {
	*Store
}

func (s *MessageIdempotencyStore) CheckAndInsert(ctx context.Context, userID int64, clientMsgID string) (isDuplicate bool, existingMsgID int64, err error) {
	var existingID int64
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO message_idempotency (user_id, client_msg_id, msg_id, conv_id, created_at)
		VALUES ($1, $2, 0, '', $3)
		ON CONFLICT (user_id, client_msg_id) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING msg_id`,
		userID, clientMsgID, time.Now().UnixMilli(),
	).Scan(&existingID)
	if err != nil {
		return false, 0, err
	}
	if existingID > 0 {
		return true, existingID, nil
	}
	return false, 0, nil
}

func (s *MessageIdempotencyStore) UpdateMsgID(ctx context.Context, exec SQLExecutor, userID int64, clientMsgID string, msgID int64, convID string) error {
	_, err := exec.ExecContext(ctx, `
		UPDATE message_idempotency 
		SET msg_id = $1, conv_id = $2 
		WHERE user_id = $3 AND client_msg_id = $4`,
		msgID, convID, userID, clientMsgID,
	)
	return err
}

func (s *UserStore) Delete(ctx context.Context, exec SQLExecutor, id int64) (int64, error) {
	res, err := exec.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *UserStore) UsernameExists(ctx context.Context, username string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)`, username).Scan(&exists)
	return exists, err
}

func (s *UserStore) EmailExists(ctx context.Context, email string) (bool, error) {
	if email == "" {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists)
	return exists, err
}

// ==================== MessageStore ====================
//
// Uses Postgres declarative partitioning on ts (milliseconds).
// The parent partitioned table "messages" is created once.
// Daily partitions "messages_YYYYMMDD" are created as partition children.
// CRUD queries go directly to the parent "messages" table;
// PostgreSQL automatically routes to the correct partition.

type MessageStore struct {
	*Store
}

func (s *MessageStore) GetMessage(ctx context.Context, msgID int64) (*Message, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
		FROM messages WHERE msg_id = $1`, msgID)
	return scanMessage(row)
}

func (s *MessageStore) GetConversationMessages(ctx context.Context, convID string, limit, offset int) ([]*Message, error) {
	rows, err := s.db.QueryContext(ctx, `
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

func (s *MessageStore) InsertMessage(ctx context.Context, exec SQLExecutor, msg *Message) (int64, error) {
	err := exec.QueryRowContext(ctx, `
		INSERT INTO messages (msg_id, conv_id, from_uid, content, ts, is_recalled)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING msg_id`,
		msg.MsgID, msg.ConvID, msg.FromUID, msg.Content, msg.Ts, msg.IsRecalled,
	).Scan(&msg.MsgID)
	return msg.MsgID, err
}

func (s *MessageStore) UpdateMessage(ctx context.Context, exec SQLExecutor, msg *Message) (int64, error) {
	res, err := exec.ExecContext(ctx, `
		UPDATE messages SET conv_id = $1, from_uid = $2, content = $3, ts = $4, is_recalled = $5
		WHERE msg_id = $6`,
		msg.ConvID, msg.FromUID, msg.Content, msg.Ts, msg.IsRecalled, msg.MsgID,
	)
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

func (s *MessageStore) GetConversationMessagesByRange(ctx context.Context, convID string, startTs, endTs int64, limit int, lastMsgId int64) (*MessagePage, error) {
	queryLimit := limit + 1

	var rows *sql.Rows
	var err error

	if lastMsgId > 0 {
		rows, err = s.db.QueryContext(ctx, `
			SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
			FROM messages
			WHERE conv_id = $1 AND ts >= $2 AND ts < $3 AND msg_id > $4
			ORDER BY ts ASC, msg_id ASC
			LIMIT $5`,
			convID, startTs, endTs, lastMsgId, queryLimit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
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

func (s *MessageStore) EnsurePartition(tableName string) error {
	date, err := time.Parse("20060102", tableName[9:])
	if err != nil {
		return err
	}
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC).UnixMilli() - s.epochTime
	endOfDay := time.Date(date.Year(), date.Month(), date.Day()+1, 0, 0, 0, 0, time.UTC).UnixMilli() - s.epochTime

	var quoted string
	err = s.db.QueryRow("SELECT quote_ident($1)", tableName).Scan(&quoted)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s PARTITION OF messages
		FOR VALUES FROM (%d) TO (%d)`,
		quoted, startOfDay, endOfDay))
	if err != nil {
		return err
	}

	_, err = s.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_msg_conv_ts_%s ON %s (conv_id, ts DESC)`, tableName[9:], quoted))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_msg_from_uid_%s ON %s (from_uid)`, tableName[9:], quoted))
	if err != nil {
		return err
	}
	return nil
}

func (s *MessageStore) GetMaxMessageID() (int64, error) {
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

func (s *MessageStore) GetMessagesAfter(ctx context.Context, convID string, lastMsgID int64, limit int) (*MessagePage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT msg_id, conv_id, from_uid, content, ts, is_recalled
		FROM messages
		WHERE conv_id = $1 AND msg_id > $2
		ORDER BY msg_id ASC
		LIMIT $3`, convID, lastMsgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}

	hasMore := len(messages) > limit-1
	if hasMore {
		messages = messages[:limit-1]
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

func (s *MessageStore) GetUpdatedConversations(ctx context.Context, userID int64, lastMsgID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT conv_id
		FROM messages
		WHERE msg_id > $1
		AND conv_id IN (
			SELECT conv_id FROM conversation_participants WHERE user_id = $2
		)`, lastMsgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convIDs []string
	for rows.Next() {
		var convID string
		if err := rows.Scan(&convID); err != nil {
			return nil, err
		}
		convIDs = append(convIDs, convID)
	}
	return convIDs, rows.Err()
}

// ==================== GroupStore ====================

type GroupStore struct {
	*Store
}

func (s *GroupStore) GetByID(ctx context.Context, groupID string) (*Group, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT group_id, name, description, owner_id, created_at, updated_at, is_deleted
		FROM groups WHERE group_id = $1 AND is_deleted = 0`, groupID)
	return scanGroup(row)
}

func (s *GroupStore) GetByName(ctx context.Context, name string) (*Group, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT group_id, name, description, owner_id, created_at, updated_at, is_deleted
		FROM groups WHERE name = $1 AND is_deleted = 0`, name)
	return scanGroup(row)
}

func (s *GroupStore) GetGroupsByOwner(ctx context.Context, ownerID int64) ([]*Group, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT group_id, name, description, owner_id, created_at, updated_at, is_deleted
		FROM groups WHERE owner_id = $1 AND is_deleted = 0
		ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroups(rows)
}

func (s *GroupStore) Insert(ctx context.Context, exec SQLExecutor, group *Group) (int64, error) {
	_, err := exec.ExecContext(ctx, `
		INSERT INTO groups (group_id, name, description, owner_id, created_at, updated_at, is_deleted)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		group.GroupID, group.Name, group.Description, group.OwnerID, group.CreatedAt, group.UpdatedAt, group.IsDeleted,
	)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func (s *GroupStore) Delete(ctx context.Context, exec SQLExecutor, groupID string) (int64, error) {
	res, err := exec.ExecContext(ctx, `UPDATE groups SET is_deleted = 1 WHERE group_id = $1`, groupID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *GroupStore) Update(ctx context.Context, group *Group) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE groups SET name = $1, description = $2, owner_id = $3, updated_at = $4
		WHERE group_id = $5`,
		group.Name, group.Description, group.OwnerID, group.UpdatedAt, group.GroupID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ==================== GroupMemberStore ====================

type GroupMemberStore struct {
	*Store
}

func (s *GroupMemberStore) Get(ctx context.Context, exec SQLExecutor, groupID string, uid int64) (*GroupMember, error) {
	row := exec.QueryRowContext(ctx, `
		SELECT id, group_id, uid, role, joined_at
		FROM group_members WHERE group_id = $1 AND uid = $2`, groupID, uid)
	return scanGroupMember(row)
}

func (s *GroupMemberStore) Insert(ctx context.Context, exec SQLExecutor, member *GroupMember) (int64, error) {
	var id int64
	err := exec.QueryRowContext(ctx, `
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

func (s *GroupMemberStore) DeleteByGroupAndUser(ctx context.Context, exec SQLExecutor, groupID string, uid int64) (int64, error) {
	res, err := exec.ExecContext(ctx, `DELETE FROM group_members WHERE group_id = $1 AND uid = $2`, groupID, uid)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *GroupMemberStore) DeleteByUser(ctx context.Context, exec SQLExecutor, uid int64) (int64, error) {
	res, err := exec.ExecContext(ctx, `DELETE FROM group_members WHERE uid = $1`, uid)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *GroupMemberStore) CountByGroup(ctx context.Context, groupID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) 
		FROM group_members 
		WHERE group_id = $1`,
		groupID,
	).Scan(&count)
	return count, err
}

func (s *GroupMemberStore) DeleteByGroupTx(tx *sql.Tx, groupID string) (int64, error) {
	res, err := tx.Exec(`DELETE FROM group_members WHERE group_id = $1`, groupID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *GroupMemberStore) IsMember(ctx context.Context, exec SQLExecutor, groupID string, uid int64) (bool, error) {
	var exists bool
	err := exec.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id = $1 AND uid = $2)`, groupID, uid).Scan(&exists)
	return exists, err
}

func (s *GroupMemberStore) GetMembersByGroup(ctx context.Context, groupID string) ([]*GroupMember, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, group_id, uid, role, joined_at
		FROM group_members WHERE group_id = $1`, groupID)
	if err != nil {
		return nil, err
	}
	return scanMany(rows, scanGroupMemberFields)
}

// ==================== FriendRequestStore ====================

type FriendRequestStore struct {
	*Store
}

func (s *FriendRequestStore) GetSentRequests(ctx context.Context, fromUserID int64) ([]*FriendRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
		FROM friend_requests WHERE from_user_id = $1
		ORDER BY created_at DESC`, fromUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFriendRequests(rows)
}

func (s *FriendRequestStore) GetReceivedRequests(ctx context.Context, toUserID int64) ([]*FriendRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
		FROM friend_requests WHERE to_user_id = $1
		ORDER BY created_at DESC`, toUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFriendRequests(rows)
}

func (s *FriendRequestStore) GetPendingRequests(ctx context.Context, userID int64) ([]*FriendRequest, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, from_user_id, to_user_id, message, status, created_at, updated_at
		FROM friend_requests WHERE (from_user_id = $1 OR to_user_id = $1) AND status = 'pending'
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFriendRequests(rows)
}

func (s *FriendRequestStore) AreFriends(ctx context.Context, userID1, userID2 int64) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT 1 FROM friend_requests
			WHERE ((from_user_id = $1 AND to_user_id = $2)
				OR (from_user_id = $2 AND to_user_id = $1))
			AND status = 'accepted'
			LIMIT 1
		) t`, userID1, userID2).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *FriendRequestStore) HasPendingRequests(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM friend_requests
			WHERE (from_user_id = $1 OR to_user_id = $1) AND status = 'pending'
		)`,
		userID,
	).Scan(&exists)
	return exists, err
}

func (s *FriendRequestStore) Insert(ctx context.Context, exec SQLExecutor, req *FriendRequest) (int64, error) {
	var id int64
	err := exec.QueryRowContext(ctx, `
		INSERT INTO friend_requests (from_user_id, to_user_id, message, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		req.FromUserID, req.ToUserID, req.Message, req.Status, req.CreatedAt, req.UpdatedAt,
	).Scan(&id)
	return id, err
}

func (s *FriendRequestStore) DeleteByUser(ctx context.Context, exec SQLExecutor, userID int64) (int64, error) {
	res, err := exec.ExecContext(ctx, `DELETE FROM friend_requests WHERE from_user_id = $1 OR to_user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// updateRequestStatusTx 通用 UPDATE friend_requests status 辅助
// status/whereCond/returningCol 均为受控常量（非用户输入），构造 SQL 安全
func (s *FriendRequestStore) updateRequestStatusTx(tx *sql.Tx, status, returningCol, whereCond string, whereArgs ...any) (int64, error) {
	now := time.Now().UnixMilli()
	query := fmt.Sprintf(
		`UPDATE friend_requests SET status = '%s', updated_at = $1 WHERE %s AND status = 'pending' RETURNING %s`,
		status, whereCond, returningCol,
	)
	var id int64
	err := tx.QueryRow(query, append([]any{now}, whereArgs...)...).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

func (s *FriendRequestStore) AcceptPendingTx(tx *sql.Tx, fromUserID, toUserID int64) (int64, error) {
	return s.updateRequestStatusTx(tx, "accepted", "id", "from_user_id = $2 AND to_user_id = $3", fromUserID, toUserID)
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

	fromUserID, err = s.updateRequestStatusTx(tx, "accepted", "from_user_id", "id = $2 AND to_user_id = $3", requestID, userID)
	if err != nil {
		return 0, err
	}
	if fromUserID == 0 {
		return -1, nil
	}
	return fromUserID, nil
}

func (s *FriendRequestStore) RejectTx(tx *sql.Tx, requestID, userID int64) (bool, error) {
	id, err := s.updateRequestStatusTx(tx, "rejected", "id", "id = $2 AND to_user_id = $3", requestID, userID)
	return id > 0, err
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

func (s *ConversationParticipantStore) Exists(ctx context.Context, exec SQLExecutor, convID string, userID int64) (bool, error) {
	var exists bool
	err := exec.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM conversation_participants WHERE conv_id = $1 AND user_id = $2)`,
		convID, userID,
	).Scan(&exists)
	return exists, err
}

func (s *ConversationParticipantStore) DeleteByUserTx(tx *sql.Tx, userID int64) (int64, error) {
	res, err := tx.Exec(`DELETE FROM conversation_participants WHERE user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *ConversationParticipantStore) InsertBatchTx(tx *sql.Tx, participants []*ConversationParticipant) (int64, error) {
	if len(participants) == 0 {
		return 0, nil
	}

	var sb strings.Builder
	sb.WriteString(`INSERT INTO conversation_participants (conv_id, user_id, join_ts, is_blocked) VALUES `)

	args := make([]interface{}, 0, len(participants)*4)
	for i, p := range participants {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d)",
			i*4+1, i*4+2, i*4+3, i*4+4))
		args = append(args, p.ConvID, p.UserID, p.JoinTs, p.IsBlocked)
	}
	sb.WriteString(" ON CONFLICT (conv_id, user_id) DO NOTHING")

	res, err := tx.Exec(sb.String(), args...)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func (s *ConversationParticipantStore) SetBlocked(ctx context.Context, convID string, userID int64, blocked bool) error {
	blockedInt := 0
	if blocked {
		blockedInt = 1
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE conversation_participants 
		SET is_blocked = $1 
		WHERE conv_id = $2 AND user_id = $3`,
		blockedInt, convID, userID,
	)
	return err
}

func (s *ConversationParticipantStore) IsAnyBlocked(ctx context.Context, convID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM conversation_participants 
			WHERE conv_id = $1 AND is_blocked = 1
		)`, convID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *ConversationParticipantStore) IsBlocked(ctx context.Context, convID string, userID int64) (bool, error) {
	var isBlocked int
	err := s.db.QueryRowContext(ctx, `
		SELECT is_blocked 
		FROM conversation_participants 
		WHERE conv_id = $1 AND user_id = $2`,
		convID, userID,
	).Scan(&isBlocked)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return isBlocked == 1, nil
}

func (s *ConversationParticipantStore) UpdateLastMsgCacheOnRecall(tx *sql.Tx, convID string, recalledMsgID int64, newContent string, isRecalled int) error {
	_, err := tx.Exec(`
        UPDATE conversation_participants 
        SET last_msg_content = $1, last_msg_is_recalled = $2
        WHERE conv_id = $3 AND last_msg_id = $4`,
		newContent, isRecalled, convID, recalledMsgID)
	return err
}

func (s *ConversationParticipantStore) UpdateLastMsgCache(tx *sql.Tx, msg *Message) error {
	_, err := tx.Exec(`
        UPDATE conversation_participants 
        SET last_msg_id = $1, last_msg_from_uid = $2, last_msg_content = $3, last_msg_ts = $4, last_msg_is_recalled = $5
        WHERE conv_id = $6`,
		msg.MsgID, msg.FromUID, msg.Content, msg.Ts, msg.IsRecalled, msg.ConvID)
	return err
}

func (s *ConversationParticipantStore) GetParticipants(ctx context.Context, convID string) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id 
		FROM conversation_participants 
		WHERE conv_id = $1`,
		convID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		participants = append(participants, userID)
	}
	return participants, rows.Err()
}

func (s *ConversationParticipantStore) CountConversations(ctx context.Context, userID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversation_participants WHERE user_id = $1`, userID).Scan(&count)
	return count, err
}

type ConversationListItem struct {
	ConvID     string  `db:"conv_id"`
	Type       string  `db:"type"`
	TargetUID  *int64  `db:"target_uid"`
	GroupID    *string `db:"group_id"`
	LastMsgID  *int64  `db:"last_msg_id"`
	FromUID    *int64  `db:"from_uid"`
	Content    *string `db:"content"`
	LastMsgTs  *int64  `db:"last_msg_ts"`
	IsRecalled *int    `db:"is_recalled"`
}

func (s *ConversationParticipantStore) GetConversationList(ctx context.Context, userID int64, limit, offset int) ([]*ConversationListItem, error) {
	query := `
		WITH user_conversations AS (
			SELECT cp.conv_id
			FROM conversation_participants cp
			WHERE cp.user_id = $1
		),
		last_messages AS (
			SELECT DISTINCT ON (m.conv_id)
				m.conv_id,
				m.msg_id,
				m.from_uid,
				m.content,
				m.ts,
				m.is_recalled
			FROM messages m
			WHERE m.conv_id IN (SELECT conv_id FROM user_conversations)
			ORDER BY m.conv_id, m.ts DESC, m.msg_id DESC
		)
		SELECT 
			uc.conv_id,
			CASE 
				WHEN uc.conv_id LIKE 'p_%' THEN 'private'
				WHEN uc.conv_id LIKE 'g_%' THEN 'group'
			END as type,
			CASE 
				WHEN uc.conv_id LIKE 'p_%' THEN (
					SELECT p.user_id 
					FROM conversation_participants p 
					WHERE p.conv_id = uc.conv_id AND p.user_id != $1
					LIMIT 1
				)
				ELSE NULL
			END as target_uid,
			CASE 
				WHEN uc.conv_id LIKE 'g_%' THEN (
					SELECT g.group_id 
					FROM groups g 
					WHERE g.group_id = SUBSTRING(uc.conv_id FROM 2)
				)
				ELSE NULL
			END as group_id,
			lm.msg_id as last_msg_id,
			lm.from_uid,
			lm.content,
			lm.ts as last_msg_ts,
			lm.is_recalled
		FROM user_conversations uc
		LEFT JOIN last_messages lm ON uc.conv_id = lm.conv_id
		ORDER BY lm.ts DESC NULLS LAST
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []*ConversationListItem
	for rows.Next() {
		var conv ConversationListItem
		if err := rows.Scan(
			&conv.ConvID,
			&conv.Type,
			&conv.TargetUID,
			&conv.GroupID,
			&conv.LastMsgID,
			&conv.FromUID,
			&conv.Content,
			&conv.LastMsgTs,
			&conv.IsRecalled,
		); err != nil {
			return nil, err
		}
		conversations = append(conversations, &conv)
	}
	return conversations, rows.Err()
}

// ==================== IdGeneratorStateStore ====================

type IdGeneratorStateStore struct {
	*Store
}

func (s *IdGeneratorStateStore) GetFirst(ctx context.Context) (*IdGeneratorState, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, last_ts, epoch_time FROM id_generator_state ORDER BY id LIMIT 1`)
	return scanIdGenState(row)
}

func (s *IdGeneratorStateStore) GetFirstForUpdate(tx *sql.Tx) (*IdGeneratorState, error) {
	row := tx.QueryRow(`SELECT id, last_ts, epoch_time FROM id_generator_state ORDER BY id LIMIT 1 FOR UPDATE`)
	return scanIdGenState(row)
}

func (s *IdGeneratorStateStore) Insert(state *IdGeneratorState) (int64, error) {
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO id_generator_state (last_ts, epoch_time)
		VALUES ($1, $2)
		RETURNING id`,
		state.LastTs, state.EpochTime,
	).Scan(&id)
	return id, err
}

func (s *IdGeneratorStateStore) InsertWithTx(tx *sql.Tx, state *IdGeneratorState) (int64, error) {
	var id int64
	err := tx.QueryRow(`
		INSERT INTO id_generator_state (last_ts, epoch_time)
		VALUES ($1, $2)
		RETURNING id`,
		state.LastTs, state.EpochTime,
	).Scan(&id)
	return id, err
}

func (s *IdGeneratorStateStore) UpdateWithTx(tx *sql.Tx, state *IdGeneratorState) (int64, error) {
	res, err := tx.Exec(`
		UPDATE id_generator_state SET last_ts = $1, epoch_time = $2
		WHERE id = $3`,
		state.LastTs, state.EpochTime, state.ID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
