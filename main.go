package main

import (
	"context"
	"database/sql"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

const (
	RequestTimeout     = 30 * time.Second
	MaxRequestBodySize = 1 << 20
	MaxMessageDays     = 7
	MaxConversations   = 100
)

func init() {
	godotenv.Load()

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(slog.New(handler))
}

func main() {
	PrintStartupInfo()
	cfg := LoadConfig()

	slog.Info("configuration loaded",
		"node_id", cfg.NodeID,
		"port", cfg.Port,
		"s3_enabled", cfg.S3URL != "",
	)

	db, err := initDB(cfg)
	if err != nil {
		slog.Error("failed to init database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	store := NewStore(db, cfg.EpochTime)
	rateLimiter := NewRateLimiter()
	// 错峰启动：RateLimiter 清理间隔 1 分钟，随机延迟 0-30 秒启动
	rateLimiter.StartCleanup(time.Minute+time.Duration(rand.Intn(30))*time.Second, 5*time.Minute)

	userStore := &UserStore{Store: store}
	msgStore := &MessageStore{Store: store}
	groupStore := &GroupStore{Store: store}
	groupMemStore := &GroupMemberStore{Store: store}
	friendStore := &FriendRequestStore{Store: store}
	convPartStore := &ConversationParticipantStore{Store: store}
	idGenStateStore := &IdGeneratorStateStore{Store: store}
	idempotencyStore := &MessageIdempotencyStore{Store: store}

	idGen := NewIdGenerator(cfg, idGenStateStore, msgStore, cfg.NodeID)
	idGen.Init()

	var s3Service *S3Service
	if cfg.S3URL != "" {
		s3Cfg, err := ParseS3URL(cfg.S3URL)
		if err != nil {
			slog.Error("failed to parse S3 URL", "error", err)
			os.Exit(1)
		}
		s3Service, err = NewS3Service(context.Background(), s3Cfg.Bucket, s3Cfg.Region, s3Cfg.Endpoint, s3Cfg.AccessKey, s3Cfg.SecretKey)
		if err != nil {
			slog.Error("failed to init S3 service", "error", err)
			os.Exit(1)
		}
	}

	svc := NewService(
		cfg,
		userStore, msgStore, groupStore, groupMemStore,
		friendStore, convPartStore,
		idGenStateStore, idempotencyStore, idGen,
		s3Service,
	)

	jwtService := NewJwtService(db, cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTExpiresMinutes)
	jwtService.StartCleanup(30 * time.Minute)
	handler := NewHandler(svc, jwtService, cfg)

	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()
	setupRoutes(r, handler, jwtService, userStore, rateLimiter, cfg)

	srv := &http.Server{
		Addr:         cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	worker := NewWorker(cfg, svc, msgStore)
	// 在启动 Worker 之前，先同步创建分区表，确保立即可用
	// 添加重试机制，避免临时性错误（如网络抖动）导致启动失败
	var createErr error
	for i := 0; i < 3; i++ {
		createErr = worker.CreateTablesFromTodayToSundayWithError()
		if createErr == nil {
			break
		}
		if i < 2 {
			slog.Warn("failed to create partition tables, retrying...", "error", createErr, "attempt", i+1)
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}
	if createErr != nil {
		slog.Error("failed to create initial partition tables after retries", "error", createErr)
		os.Exit(1)
	}
	worker.Start()

	quit := make(chan os.Signal, 2)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	go func() {
		<-quit
		<-quit
		slog.Warn("forced exit, skipping graceful shutdown")
		os.Exit(1)
	}()

	<-quit
	slog.Info("shutting down...")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()

	done := make(chan struct{})
	go func() {
		worker.Stop()
		rateLimiter.Stop()
		jwtService.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-stopCtx.Done():
		slog.Warn("cleanup timed out, proceeding with server shutdown")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}
}

func setupRoutes(r *gin.Engine, h *Handler, jwtService *JwtService, us *UserStore, rateLimiter *RateLimiter, cfg *Config) {
	r.Use(timeoutMiddleware(RequestTimeout))

	r.Use(func(c *gin.Context) {
		start := time.Now()
		c.Next()

		// 跳过健康检查端点
		if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/ready" {
			return
		}

		latency := time.Since(start)
		status := c.Writer.Status()

		// 只记录慢请求 (>500ms) 或错误请求 (>=400) 或特定路径
		if latency > 500*time.Millisecond || status >= 400 {
			slog.Info("slow/error request",
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"status", status,
				"latency_ms", latency.Milliseconds(),
				"ip", c.ClientIP(),
			)
		}
	})

	r.Use(func(c *gin.Context) {
		c.Set("rateLimiter", rateLimiter)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxRequestBodySize)
		c.Next()
	})

	r.GET("/health", h.HealthCheck)
	r.GET("/ready", h.ReadinessCheck)
	r.GET("/metrics", h.Metrics)

	api := r.Group("/api")
	{
		api.POST("/auth/register", h.Register)
		api.POST("/auth/login", loginRateLimitMiddleware(rateLimiter, 15*time.Minute, 5), h.Login)

		auth := api.Group("")
		auth.Use(jwtMiddleware(jwtService))
		{
			auth.GET("/auth/me", h.GetMe)
			auth.POST("/auth/logout", h.Logout)
			auth.PUT("/auth/:publicId", h.UpdateUser)
			auth.DELETE("/auth/:publicId", h.DeleteUser)

			auth.POST("/messages/send", rateLimitMiddleware(rateLimiter, time.Second), h.SendMessage)
			auth.GET("/messages", h.GetMessages)
			auth.POST("/messages/recall/:msgId", h.RecallMessage)
			auth.GET("/check", rateLimitMiddleware(rateLimiter, 3*time.Second), h.CheckNewMessages)
			auth.GET("/conversations", h.GetConversations)
			auth.POST("/blocks/:targetPublicId", h.BlockUser)
			auth.DELETE("/blocks/:targetPublicId", h.UnblockUser)

			auth.POST("/groups", h.CreateGroup)
			auth.GET("/groups/:id", h.GetGroup)
			auth.PUT("/groups/:id", h.UpdateGroup)
			auth.DELETE("/groups/:id", h.DeleteGroup)
			auth.POST("/groups/:id/join", h.JoinGroup)
			auth.POST("/groups/:id/leave", h.LeaveGroup)
			auth.DELETE("/groups/:id/members/:memberPublicId", h.KickMember)

			auth.POST("/friends/request/send/:targetPublicId", h.SendFriendRequest)
			auth.GET("/friends/requests", h.GetFriendRequests)
			auth.POST("/friends/request/:requestId/accept", h.AcceptFriendRequest)
			auth.POST("/friends/request/:requestId/reject", h.RejectFriendRequest)
			auth.DELETE("/friends/request/:requestId", h.CancelFriendRequest)

			auth.POST("/files/presign", h.GetPresignURL)
		}
	}
}

func initDB(cfg *Config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)

	return db, nil
}

func jwtMiddleware(jwtService *JwtService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: ErrUnauthorized.Message})
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := jwtService.ValidateToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: ErrUnauthorized.Message})
			return
		}

		if jwtService.IsBlacklisted(claims.ID) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: ErrUnauthorized.Message})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("public_id", claims.PublicID)
		c.Set("username", claims.Username)
		c.Next()
	}
}

func rateLimitMiddleware(rl *RateLimiter, interval time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		publicID := c.GetString("public_id")
		if !rl.AllowRequestWithInterval(publicID, interval) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, ErrorResponse{Error: ErrRateLimitExceeded.Message})
			return
		}
		c.Next()
	}
}

func loginRateLimitMiddleware(rl *RateLimiter, interval time.Duration, maxFailures int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !rl.AllowRequestWithMaxFailures(ip, interval, maxFailures) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, ErrorResponse{Error: ErrRateLimitExceeded.Message})
			return
		}
		c.Set("rate_limit_key", ip)
		c.Next()
	}
}
