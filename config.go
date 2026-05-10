package main

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	// 必要配置（必须通过环境变量设置）
	NodeID      int64
	Port        string
	DatabaseURL string
	JWTSecret   string

	// 可选配置（有合理的默认值，可通过环境变量覆盖）
	DBMaxOpenConns int
	DBMaxIdleConns int

	JWTIssuer             string
	JWTExpiresMinutes     int
	BCryptCost            int
	MessageRecallWindowMs int64

	EpochTime         int64
	SegmentDurationMs int64
	SegmentSize       int64

	MessageRetentionDays int
	DefaultPageSize      int
	MaxPageSize          int

	PublicIDLength      int
	GroupIDRandomLength int

	WorkerTableCreateIntervalHours       int
	WorkerMaintenanceInitialDelayMinutes int
	WorkerMaintenanceIntervalHours       int
}

// 可选配置的默认值（硬编码，需自定义时通过环境变量覆盖）
const (
	defaultJWTIssuer             = "vortex"
	defaultJWTExpiresMinutes     = 10080 // 7 天
	defaultBCryptCost            = 10
	defaultMessageRecallWindowMs = 120_000 // 2 分钟
	defaultEpochTime             = 1_767_225_600_000
	defaultSegmentDurationMs     = 10_000  // 10 秒
	defaultSegmentSize           = 1 << 17 // 128KB
	defaultMessageRetentionDays  = 7
	defaultPageSize              = 100
	defaultMaxPageSize           = 500
	defaultPublicIDLength        = 21
	defaultGroupIDRandomLength   = 8
	defaultWorkerCreateInterval  = 168 // 1 周
	defaultMaintenanceDelay      = 5
	defaultMaintenanceInterval   = 24
	defaultDBMaxOpenConns        = 50
	defaultDBMaxIdleConns        = 20
)

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func (cfg *Config) Validate() error {
	if cfg.NodeID < 0 || cfg.NodeID > vfMaxNodeID {
		return fmt.Errorf("NODE_ID must be between 0 and %d", vfMaxNodeID)
	}
	if cfg.BCryptCost < 10 || cfg.BCryptCost > 15 {
		return fmt.Errorf("BCRYPT_COST must be between 10 and 15")
	}
	if cfg.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if cfg.SegmentDurationMs <= 0 {
		return fmt.Errorf("SEGMENT_DURATION_MS must be positive")
	}
	if cfg.SegmentSize <= 0 {
		return fmt.Errorf("SEGMENT_SIZE must be positive")
	}
	if cfg.MessageRetentionDays <= 0 {
		return fmt.Errorf("MESSAGE_RETENTION_DAYS must be positive")
	}
	if cfg.DefaultPageSize <= 0 || cfg.MaxPageSize <= 0 {
		return fmt.Errorf("PAGE_SIZE must be positive")
	}
	if cfg.DefaultPageSize > cfg.MaxPageSize {
		return fmt.Errorf("DEFAULT_PAGE_SIZE cannot exceed MAX_PAGE_SIZE")
	}
	return nil
}

func LoadConfig() *Config {
	cfg := &Config{
		// 必要配置
		NodeID:      envInt64("NODE_ID", 0),
		Port:        envString("PORT", ":8080"),
		DatabaseURL: envString("DATABASE_URL", "postgres://localhost:5432/vortex?sslmode=disable"),
		JWTSecret:   os.Getenv("JWT_SECRET"),

		// 可选配置（使用默认值，可通过环境变量覆盖）
		DBMaxOpenConns:                       envInt("DB_MAX_OPEN_CONNS", defaultDBMaxOpenConns),
		DBMaxIdleConns:                       envInt("DB_MAX_IDLE_CONNS", defaultDBMaxIdleConns),
		JWTIssuer:                            envString("JWT_ISSUER", defaultJWTIssuer),
		JWTExpiresMinutes:                    envInt("JWT_EXPIRES_MINUTES", defaultJWTExpiresMinutes),
		BCryptCost:                           envInt("BCRYPT_COST", defaultBCryptCost),
		MessageRecallWindowMs:                envInt64("MESSAGE_RECALL_WINDOW_MS", defaultMessageRecallWindowMs),
		EpochTime:                            envInt64("EPOCH_TIME", defaultEpochTime),
		SegmentDurationMs:                    envInt64("ID_SEGMENT_DURATION_MS", defaultSegmentDurationMs),
		SegmentSize:                          envInt64("ID_SEGMENT_SIZE", defaultSegmentSize),
		MessageRetentionDays:                 envInt("MESSAGE_RETENTION_DAYS", defaultMessageRetentionDays),
		DefaultPageSize:                      envInt("DEFAULT_PAGE_SIZE", defaultPageSize),
		MaxPageSize:                          envInt("MAX_PAGE_SIZE", defaultMaxPageSize),
		PublicIDLength:                       envInt("PUBLIC_ID_LENGTH", defaultPublicIDLength),
		GroupIDRandomLength:                  envInt("GROUP_ID_RANDOM_LENGTH", defaultGroupIDRandomLength),
		WorkerTableCreateIntervalHours:       envInt("WORKER_TABLE_CREATE_INTERVAL_HOURS", defaultWorkerCreateInterval),
		WorkerMaintenanceInitialDelayMinutes: envInt("WORKER_MAINTENANCE_INITIAL_DELAY_MINUTES", defaultMaintenanceDelay),
		WorkerMaintenanceIntervalHours:       envInt("WORKER_MAINTENANCE_INTERVAL_HOURS", defaultMaintenanceInterval),
	}

	if cfg.Port != "" && cfg.Port[0] != ':' {
		cfg.Port = ":" + cfg.Port
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		if cfg.JWTSecret == "" {
			fmt.Fprintln(os.Stderr, "To generate a JWT secret, run one of:")
			fmt.Fprintln(os.Stderr, "  openssl rand -base64 32")
			fmt.Fprintln(os.Stderr, "  head -c 32 /dev/urandom | base64")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Then set the environment variable:")
			fmt.Fprintln(os.Stderr, "  export JWT_SECRET=\"<your-generated-secret>\"")
		}
		os.Exit(1)
	}

	return cfg
}
