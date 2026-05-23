package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// 必要配置（必须通过环境变量设置）
	NodeID    int64
	JWTSecret string

	// 可选配置（有合理的默认值，可通过环境变量覆盖）
	Port           string
	DatabaseURL    string
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

	// S3 配置（可选，为空则不启用文件存储）
	S3URL string
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
		fmt.Fprintf(os.Stderr, "Error: invalid value for %s: %s\n", key, v)
		os.Exit(1)
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
		fmt.Fprintf(os.Stderr, "Error: invalid value for %s: %s\n", key, v)
		os.Exit(1)
	}
	return fallback
}

func generateJWTSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

func promptInteractiveJWT() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("JWT_SECRET is not configured.")
	fmt.Println("Options:")
	fmt.Println("  1. Auto-generate a new JWT secret (recommended)")
	fmt.Println("  2. Exit and configure manually")
	fmt.Print("Choose [1-2]: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	switch input {
	case "1":
		secret, err := generateJWTSecret()
		if err != nil {
			return "", fmt.Errorf("failed to generate JWT secret: %w", err)
		}
		fmt.Printf("\nGenerated JWT secret: %s\n", secret)
		fmt.Println("Please save this secret securely!")
		fmt.Printf("You can set it with: export JWT_SECRET=\"%s\"\n\n", secret)
		return secret, nil
	case "2":
		fmt.Println("Exiting. Please set JWT_SECRET environment variable and restart.")
		os.Exit(0)
		return "", nil
	default:
		fmt.Println("Invalid option. Exiting.")
		os.Exit(1)
		return "", nil
	}
}

func promptInteractiveS3() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("S3_URL is not configured.")
	fmt.Println("Options:")
	fmt.Println("  1. Enable S3 storage (enter connection string)")
	fmt.Println("  2. Skip S3 configuration (file upload will be disabled)")
	fmt.Println("  3. Exit and configure manually")
	fmt.Print("Choose [1-3]: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	switch input {
	case "1":
		fmt.Print("\nEnter S3 connection string (s3://bucket?endpoint=...&region=...&access_key=...&secret_key=...): ")
		inputURL, _ := reader.ReadString('\n')
		inputURL = strings.TrimSpace(inputURL)
		if inputURL == "" {
			fmt.Println("Empty URL. S3 will be disabled.")
			return "", nil
		}
		return inputURL, nil
	case "2":
		fmt.Println("S3 storage disabled. File upload will be disabled.")
		return "", nil
	case "3":
		fmt.Println("Exiting. Please set S3_URL environment variable and restart.")
		os.Exit(0)
		return "", nil
	default:
		fmt.Println("Invalid option. S3 will be disabled.")
		return "", nil
	}
}

func promptInteractiveDatabase() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("DATABASE_URL is not configured.")
	fmt.Println("Options:")
	fmt.Println("  1. Use default PostgreSQL connection (localhost:5432/vortex)")
	fmt.Println("  2. Enter custom connection string")
	fmt.Println("  3. Exit and configure manually")
	fmt.Print("Choose [1-3]: ")

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	switch input {
	case "1":
		fmt.Println("Using default: postgres://localhost:5432/vortex?sslmode=disable")
		return "postgres://localhost:5432/vortex?sslmode=disable", nil
	case "2":
		fmt.Print("\nEnter database connection string: ")
		inputURL, _ := reader.ReadString('\n')
		inputURL = strings.TrimSpace(inputURL)
		if inputURL == "" {
			return "", fmt.Errorf("empty database URL")
		}
		return inputURL, nil
	case "3":
		fmt.Println("Exiting. Please set DATABASE_URL environment variable and restart.")
		os.Exit(0)
		return "", nil
	default:
		fmt.Println("Invalid option. Exiting.")
		os.Exit(1)
		return "", nil
	}
}

type S3Config struct {
	Bucket    string
	Region    string
	Endpoint  string
	AccessKey string
	SecretKey string
}

func ParseS3URL(s3URL string) (*S3Config, error) {
	if s3URL == "" {
		return nil, fmt.Errorf("empty S3 URL")
	}

	u, err := url.Parse(s3URL)
	if err != nil {
		return nil, fmt.Errorf("invalid S3 URL: %w", err)
	}

	if u.Scheme != "s3" {
		return nil, fmt.Errorf("invalid S3 URL scheme: %s (expected s3://)", u.Scheme)
	}

	bucket := u.Host
	if bucket == "" {
		return nil, fmt.Errorf("missing bucket in S3 URL")
	}

	query := u.Query()
	cfg := &S3Config{
		Bucket:    bucket,
		Region:    query.Get("region"),
		Endpoint:  query.Get("endpoint"),
		AccessKey: query.Get("access_key"),
		SecretKey: query.Get("secret_key"),
	}

	if u.User != nil {
		cfg.AccessKey = u.User.Username()
		cfg.SecretKey, _ = u.User.Password()
	}

	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	return cfg, nil
}

func (cfg *Config) Validate() error {
	if cfg.NodeID < 0 {
		return fmt.Errorf("NODE_ID is required (must be between 0 and %d)", vfMaxNodeID)
	}
	if cfg.NodeID > vfMaxNodeID {
		return fmt.Errorf("NODE_ID must be between 0 and %d", vfMaxNodeID)
	}
	if cfg.BCryptCost < 10 || cfg.BCryptCost > 15 {
		return fmt.Errorf("BCRYPT_COST must be between 10 and 15")
	}
	if cfg.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if cfg.Port == "" {
		return fmt.Errorf("PORT is required")
	}
	portStr := strings.TrimPrefix(cfg.Port, ":")
	if portNum, err := strconv.Atoi(portStr); err != nil || portNum < 1 || portNum > 65535 {
		return fmt.Errorf("PORT must be a valid port number (1-65535), got: %s", portStr)
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
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
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		var err error
		jwtSecret, err = promptInteractiveJWT()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		var err error
		databaseURL, err = promptInteractiveDatabase()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	s3URL := os.Getenv("S3_URL")
	if s3URL == "" {
		var err error
		s3URL, err = promptInteractiveS3()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	cfg := &Config{
		// 必要配置
		NodeID:      envInt64("NODE_ID", -1),
		Port:        envString("PORT", ":8080"),
		DatabaseURL: databaseURL,
		JWTSecret:   jwtSecret,

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

		// S3 配置
		S3URL: s3URL,
	}

	// 自动添加端口号冒号前缀
	if cfg.Port != "" && !strings.HasPrefix(cfg.Port, ":") {
		cfg.Port = ":" + cfg.Port
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		os.Exit(1)
	}

	return cfg
}
