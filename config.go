package main

import (
	crand "crypto/rand"
	"errors"
	"fmt"
	mathrand "math/rand"
	"os"
	"time"
)

// ==================== AppConfig ====================

var AppCfg = struct {
	NodeID int64

	Time struct {
		EpochTime            int64
		RateLimitWindowMs    int
		MessageRecallWindowMs int
	}

	IdGenerator struct {
		SegmentDurationMs int64
		SegmentSize       int64
	}

	Message struct {
		BatchSize         int
		BatchTimeoutMs    int
		BackpressureThreshold int
		QueryDays         int
		RetentionDays     int
	}
}{
	NodeID: 0,
	Time: struct {
		EpochTime            int64
		RateLimitWindowMs    int
		MessageRecallWindowMs int
	}{
		EpochTime:            1_767_225_600_000,
		RateLimitWindowMs:    1_000,
		MessageRecallWindowMs: 120_000,
	},
	IdGenerator: struct {
		SegmentDurationMs int64
		SegmentSize       int64
	}{
		SegmentDurationMs: 10_000,
		SegmentSize:       1 << 17,
	},
	Message: struct {
		BatchSize         int
		BatchTimeoutMs    int
		BackpressureThreshold int
		QueryDays         int
		RetentionDays     int
	}{
		BatchSize:         50,
		BatchTimeoutMs:    100,
		BackpressureThreshold: 1_000,
		QueryDays:         7,
		RetentionDays:     7,
	},
}

// ==================== Error ====================

type AppError struct {
	Code    string
	Message string
	Details interface{}
}

func (e *AppError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

var (
	ErrNotFound           = &AppError{Code: "not_found", Message: "Resource not found"}
	ErrForbidden          = &AppError{Code: "forbidden", Message: "Access denied"}
	ErrUnauthorized       = &AppError{Code: "unauthorized", Message: "Unauthorized"}
	ErrInvalidCredentials = &AppError{Code: "invalid_credentials", Message: "Invalid credentials"}
	ErrRateLimitExceeded  = &AppError{Code: "rate_limit_exceeded", Message: "Rate limit exceeded"}
	ErrConflict           = &AppError{Code: "conflict", Message: "Resource conflict"}
	ErrNotMember          = &AppError{Code: "not_member", Message: "Not a group member"}
	ErrInvalidInput       = &AppError{Code: "invalid_input", Message: "Invalid input"}
	ErrInvalidType        = &AppError{Code: "invalid_type", Message: "Invalid type"}
	ErrInvalidTargetID    = &AppError{Code: "invalid_target_id", Message: "Invalid target ID"}
	ErrSelfRequest        = &AppError{Code: "self_request", Message: "Cannot request self"}
	ErrNotPending         = &AppError{Code: "not_pending", Message: "Status is not pending"}
	ErrAlreadyExists      = &AppError{Code: "already_exists", Message: "Resource already exists"}
)

func NewError(code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// ==================== Models ====================

type User struct {
	ID        int64  `db:"id"`
	Username  string `db:"username"`
	PwdHash   string `db:"pwd_hash"`
	Email     string `db:"email"`
	PublicID  string `db:"public_id"`
	CreatedAt int64  `db:"created_at"`
	UpdatedAt int64  `db:"updated_at"`
}

type UserDevice struct {
	ID           int64  `db:"id"`
	UserID       int64  `db:"user_id"`
	DeviceToken  string `db:"device_token"`
	DeviceName   string `db:"device_name"`
	DeviceType   string `db:"device_type"`
	IPAddress    string `db:"ip_address"`
	LastActiveAt int64  `db:"last_active_at"`
	IsActive     int    `db:"is_active"`
	InsertedAt   int64  `db:"inserted_at"`
	UpdatedAt    int64  `db:"updated_at"`
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
	MsgID     int64  `db:"msg_id"`
	ConvID    string `db:"conv_id"`
	FromUID   int64  `db:"from_uid"`
	Content   string `db:"content"`
	Ts        int64  `db:"ts"`
	IsRecalled int   `db:"is_recalled"`
}

type ConversationParticipant struct {
	ConvID string `db:"conv_id"`
	UserID int64  `db:"user_id"`
	JoinTs int64  `db:"join_ts"`
}

type IdGeneratorState struct {
	ID     int64 `db:"id"`
	LastTs int64 `db:"last_ts"`
	LastSeq int64 `db:"last_seq"`
}

// ==================== Response types ====================

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// ==================== NanoId ====================

const nanoIDAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"

func GenerateNanoID(size int) string {
	if size <= 0 {
		size = 21
	}
	bytes := make([]byte, size)
	if _, err := crand.Read(bytes); err != nil {
		for i := range bytes {
			bytes[i] = nanoIDAlphabet[mathrand.Intn(len(nanoIDAlphabet))]
		}
	}
	for i, b := range bytes {
		bytes[i] = nanoIDAlphabet[int(b)%len(nanoIDAlphabet)]
	}
	return string(bytes)
}

// ==================== Schema ====================

var Schema = struct {
	Users                   struct{ Table, ID, Username, PwdHash, Email, PublicID, CreatedAt, UpdatedAt string }
	UserDevices             struct{ Table, ID, UserID, DeviceToken, DeviceName, DeviceType, IPAddress, LastActiveAt, IsActive, InsertedAt, UpdatedAt string }
	Groups                  struct{ Table, GroupID, OwnerID, Name, Description, CreatedAt, UpdatedAt, IsDeleted, Prefix string; RandomIdLength int }
	GroupMembers            struct{ Table, ID, GroupID, UID, Role, JoinedAt string }
	FriendRequests          struct{ Table, ID, FromUserID, ToUserID, Message, Status, CreatedAt, UpdatedAt string }
	Messages                struct{ TablePrefix, MsgID, ConvID, FromUID, Content, Ts, IsRecalled string }
	ConversationParticipants struct{ Table, ConvID, UserID, JoinTs string }
	IdGeneratorState        struct{ Table, ID, LastTs, LastSeq string }
	Conversations           struct{}
}{
	Users: struct{ Table, ID, Username, PwdHash, Email, PublicID, CreatedAt, UpdatedAt string }{
		Table: "users", ID: "id", Username: "username", PwdHash: "pwd_hash",
		Email: "email", PublicID: "public_id", CreatedAt: "created_at", UpdatedAt: "updated_at",
	},
	UserDevices: struct{ Table, ID, UserID, DeviceToken, DeviceName, DeviceType, IPAddress, LastActiveAt, IsActive, InsertedAt, UpdatedAt string }{
		Table: "user_devices", ID: "id", UserID: "user_id", DeviceToken: "device_token",
		DeviceName: "device_name", DeviceType: "device_type", IPAddress: "ip_address",
		LastActiveAt: "last_active_at", IsActive: "is_active", InsertedAt: "inserted_at", UpdatedAt: "updated_at",
	},
	Groups: struct{ Table, GroupID, OwnerID, Name, Description, CreatedAt, UpdatedAt, IsDeleted, Prefix string; RandomIdLength int }{
		Table: "groups", GroupID: "group_id", OwnerID: "owner_id", Name: "name",
		Description: "description", CreatedAt: "created_at", UpdatedAt: "updated_at",
		IsDeleted: "is_deleted", Prefix: "g_", RandomIdLength: 8,
	},
	GroupMembers: struct{ Table, ID, GroupID, UID, Role, JoinedAt string }{
		Table: "group_members", ID: "id", GroupID: "group_id", UID: "uid",
		Role: "role", JoinedAt: "joined_at",
	},
	FriendRequests: struct{ Table, ID, FromUserID, ToUserID, Message, Status, CreatedAt, UpdatedAt string }{
		Table: "friend_requests", ID: "id", FromUserID: "from_user_id", ToUserID: "to_user_id",
		Message: "message", Status: "status", CreatedAt: "created_at", UpdatedAt: "updated_at",
	},
	Messages: struct{ TablePrefix, MsgID, ConvID, FromUID, Content, Ts, IsRecalled string }{
		TablePrefix: "messages_", MsgID: "msg_id", ConvID: "conv_id", FromUID: "from_uid",
		Content: "content", Ts: "ts", IsRecalled: "is_recalled",
	},
	ConversationParticipants: struct{ Table, ConvID, UserID, JoinTs string }{
		Table: "conversation_participants", ConvID: "conv_id", UserID: "user_id", JoinTs: "join_ts",
	},
	IdGeneratorState: struct{ Table, ID, LastTs, LastSeq string }{
		Table: "id_generator_state", ID: "id", LastTs: "last_ts", LastSeq: "last_seq",
	},
}

func MessageTableNameByDate(date time.Time) string {
	return fmt.Sprintf("messages_%s", date.Format("20060102"))
}

func MessageTableNameByTs(ts int64) string {
	t := time.UnixMilli(ts)
	return MessageTableNameByDate(t)
}

func GenerateGroupID() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, 8)
	for i := range buf {
		buf[i] = chars[mathrand.Intn(len(chars))]
	}
	return "g_" + string(buf)
}

func IsValidGroupID(groupID string) bool {
	return len(groupID) == 11 && groupID[:2] == "g_"
}

func PrivateConvID(uid1, uid2 int64) string {
	if uid1 < uid2 {
		return fmt.Sprintf("p_%d_%d", uid1, uid2)
	}
	return fmt.Sprintf("p_%d_%d", uid2, uid1)
}

func IsPrivateConv(convID string) bool {
	return len(convID) > 0 && convID[0] == 'p'
}

func IsGroupConv(convID string) bool {
	return len(convID) >= 2 && convID[:2] == "g_"
}

func ParsePrivateConv(convID string) (int64, int64, error) {
	if !IsPrivateConv(convID) {
		return 0, 0, errors.New("not a private conversation")
	}
	var a, b int64
	if _, err := fmt.Sscanf(convID, "p_%d_%d", &a, &b); err != nil {
		return 0, 0, err
	}
	return a, b, nil
}

func CanAccessPrivateConv(convID string, uid int64) bool {
	a, b, err := ParsePrivateConv(convID)
	if err != nil {
		return false
	}
	return a == uid || b == uid
}

func ExtractGroupID(convID string) string {
	if IsGroupConv(convID) {
		return convID
	}
	return ""
}

type NotificationPayload struct {
	Type string                 `json:"type"`
	SubType string              `json:"sub_type"`
	Data  map[string]interface{} `json:"data"`
}

type ImageContent struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Text    string `json:"text"`
}

// ==================== Config ====================

type Config struct {
	NodeID      int64  `env:"NODE_ID"`
	DatabaseURL string `env:"DATABASE_URL"`
	JWTSecret   string `env:"JWT_SECRET"`
}

func LoadConfig() *Config {
	cfg := &Config{
		NodeID:      0,
		DatabaseURL: "postgres://localhost:5432/vortex?sslmode=disable",
		JWTSecret:   "dev_jwt_secret_key_for_development_only",
	}

	if v := os.Getenv("NODE_ID"); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &cfg.NodeID); err != nil || n != 1 {
			cfg.NodeID = 0
		}
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWTSecret = v
	}

	return cfg
}
