package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

type Config struct {
	NodeID   int64
	Port     string
	DatabaseURL    string
	DBMaxOpenConns int
	DBMaxIdleConns int

	JWTSecret         string
	JWTIssuer         string
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
		NodeID:                                envInt64("NODE_ID", 0),
		Port:                                  envString("PORT", ":8080"),
		DatabaseURL:                           envString("DATABASE_URL", "postgres://localhost:5432/vortex?sslmode=disable"),
		DBMaxOpenConns:                        envInt("DB_MAX_OPEN_CONNS", 50),
		DBMaxIdleConns:                        envInt("DB_MAX_IDLE_CONNS", 20),
		JWTSecret:                             envString("JWT_SECRET", "dev_jwt_secret_key_for_development_only"),
		JWTIssuer:                             envString("JWT_ISSUER", "vortex"),
		JWTExpiresMinutes:                     envInt("JWT_EXPIRES_MINUTES", 10080),
		BCryptCost:                            envInt("BCRYPT_COST", 10),
		MessageRecallWindowMs:                 envInt64("MESSAGE_RECALL_WINDOW_MS", 120_000),
		EpochTime:                             envInt64("EPOCH_TIME", 1_767_225_600_000),
		SegmentDurationMs:                     envInt64("ID_SEGMENT_DURATION_MS", 10_000),
		SegmentSize:                           envInt64("ID_SEGMENT_SIZE", 1<<17),
		MessageRetentionDays:                  envInt("MESSAGE_RETENTION_DAYS", 7),
		DefaultPageSize:                       envInt("DEFAULT_PAGE_SIZE", 100),
		MaxPageSize:                           envInt("MAX_PAGE_SIZE", 500),
		PublicIDLength:                        envInt("PUBLIC_ID_LENGTH", 21),
		GroupIDRandomLength:                   envInt("GROUP_ID_RANDOM_LENGTH", 8),
		WorkerTableCreateIntervalHours:        envInt("WORKER_TABLE_CREATE_INTERVAL_HOURS", 168),
		WorkerMaintenanceInitialDelayMinutes:  envInt("WORKER_MAINTENANCE_INITIAL_DELAY_MINUTES", 5),
		WorkerMaintenanceIntervalHours:        envInt("WORKER_MAINTENANCE_INTERVAL_HOURS", 24),
	}

	if cfg.Port != "" && cfg.Port[0] != ':' {
		cfg.Port = ":" + cfg.Port
	}

	if cfg.JWTSecret == "dev_jwt_secret_key_for_development_only" {
		slog.Warn("using default JWT secret. Set JWT_SECRET environment variable for production")
	}

	if err := cfg.Validate(); err != nil {
		slog.Error("invalid config", "error", err)
		os.Exit(1)
	}

	return cfg
}
