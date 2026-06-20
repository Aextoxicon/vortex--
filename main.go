// Vortex API
//
//	Vortex 即时通讯系统 API 文档
//
//	Schemes: http
//	Host: localhost:9178
//	BasePath: /api
//	Version: 1.0.0
//	Contact: API Support <support@example.com>
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
//	SecurityDefinitions:
//	bearerAuth:
//	  type: apiKey
//	  name: Authorization
//	  in: header
//	  description: JWT Token认证，格式: Bearer <token>
//
// swagger:meta
package main

import (
	"context"
	"database/sql"
	"fmt"
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

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// 这个后端有一部分是ai辅助的，所以有些代码可能会比较冗余或者不够优雅，甚至有些地方可能会有点蠢
// 不过好消息是这会经过一个人类审查并且在他认为重要的地方写上注释
// 可能这里面人类参与度最高的是jwt和id生成器的代码，其他主要就是基于ai的骨架修改，然后就是尝试着让变量名或者是函数名变得好理解，以及去掉一些重复的东西
// 当然我感觉这里可能还是处于鸟不拉屎的状态，为什么曾经有一段rust的版本？不我不要rust参与后端
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
	// 检查 --version 参数
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(GetVersion())
		return
	}

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
	store.SetEpochTime(idGen.GetEpochTime())

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
	jwtService.StartCleanup(10 * time.Minute)
	handler := NewHandler(svc, jwtService, cfg)

	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()
	setupRoutes(r, handler, jwtService, userStore, rateLimiter, cfg)

	// 我感觉除了gin可能我属性的http服务器就是express了，但是c#的也不错，可惜对cs产生了PTSD
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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down gracefully...")

	// 等待清理完成，最多 10 秒；若收到第二个信号则强制退出
	cleanupDone := make(chan struct{})
	go func() {
		worker.Stop()
		rateLimiter.Stop()
		jwtService.Stop()
		close(cleanupDone)
	}()

	// 第二个信号直接强制退出(当然我也可以再加一个信号？)
	go func() {
		secondSignal := make(chan os.Signal, 1)
		signal.Notify(secondSignal, syscall.SIGINT, syscall.SIGTERM)
		<-secondSignal
		slog.Warn("second signal received, forcing shutdown")
		os.Exit(1)
	}()

	select {
	case <-cleanupDone:
	case <-time.After(10 * time.Second):
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

		// 跳过健康检查和 Swagger 端点
		if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/ready" || strings.HasPrefix(c.Request.URL.Path, "/swagger/") {
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

	// Swagger
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	api := r.Group("/api")
	{
		api.POST("/auth/register", h.Register)
		api.POST("/auth/login", loginRateLimitMiddleware(rateLimiter, 15*time.Minute, 5), h.Login)
		// 我在想限流是不是加的有点少了，不过目前还没其他用户，就先这样吧，项目能不能活都不好说
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

			auth.GET("/conversations/count", h.GetConversationCount)
			auth.GET("/conversations/:convId/participants", h.GetConversationParticipants)
			auth.GET("/conversations/:convId/blocked/:userId", h.CheckBlocked)
			auth.GET("/messages/:msgId", h.GetMessageByID)
			auth.GET("/groups/:id/members/count", h.GetGroupMemberCount)
			auth.GET("/friends/requests/pending", h.GetPendingFriendRequests)
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
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.Next()
			return
		}
		username := req.Username
		if !rl.AllowRequestWithMaxFailures(username, interval, maxFailures) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, ErrorResponse{Error: ErrRateLimitExceeded.Message})
			return
		}
		c.Set("rate_limit_key", username)
		c.Next()
	}
}

// 与其这样我觉得我现在应该把精力多花一些在app上面，经过以往经验可能这个东西多半是被骂一顿然后被驳回，至少app做好了自己也能用
// 为什么注释这么啰嗦？真的会有人去看这一坨注释吗，感觉还是让codex这种东西一键解释更常见吧，毕竟不是什么高级的东西
