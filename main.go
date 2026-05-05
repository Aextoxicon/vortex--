package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

func main() {
	cfg := LoadConfig()

	db, err := initDB(cfg)
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	store := NewStore(db)
	rateLimiter := NewRateLimiter()
	rateLimiter.StartCleanup(time.Minute, 5*time.Minute)

	userStore := &UserStore{Store: store}
	msgStore := &MessageStore{Store: store}
	groupStore := &GroupStore{Store: store}
	groupMemStore := &GroupMemberStore{Store: store}
	friendStore := &FriendRequestStore{Store: store}
	convPartStore := &ConversationParticipantStore{Store: store}
	idGenStateStore := &IdGeneratorStateStore{Store: store}

	idGen := NewIdGenerator(idGenStateStore, msgStore, cfg.NodeID)
	idGen.Init()

	svc := NewService(
		userStore, msgStore, groupStore, groupMemStore,
		friendStore, convPartStore,
		idGenStateStore, idGen,
	)

	jwtService := NewJwtService(db, cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTExpiresMinutes)
	jwtService.StartCleanup(30 * time.Minute)
	handler := NewHandler(svc, jwtService)

	r := gin.Default()
	setupRoutes(r, handler, jwtService, userStore, rateLimiter, cfg)

	srv := &http.Server{Addr: cfg.Port, Handler: r}

	worker := NewWorker(svc, msgStore)
	worker.Start()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("failed to start server: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server forced to shutdown: %v", err)
	}

	rateLimiter.Stop()
	worker.Stop()
}

func setupRoutes(r *gin.Engine, h *Handler, jwtService *JwtService, us *UserStore, rateLimiter *RateLimiter, cfg *Config) {
	api := r.Group("/api")
	{
		api.POST("/auth/register", h.Register)
		api.POST("/auth/login", h.Login)

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

			auth.POST("/groups", h.CreateGroup)
			auth.GET("/groups/:id", h.GetGroup)
			auth.PUT("/groups/:id", h.UpdateGroup)
			auth.DELETE("/groups/:id", h.DeleteGroup)
			auth.POST("/groups/:id/join", h.JoinGroup)
			auth.POST("/groups/:id/leave", h.LeaveGroup)

			auth.POST("/friends/request/:targetPublicId", h.SendFriendRequest)
			auth.GET("/friends/requests", h.GetFriendRequests)
			auth.POST("/friends/request/:requestId/accept", h.AcceptFriendRequest)
			auth.POST("/friends/request/:requestId/reject", h.RejectFriendRequest)
			auth.DELETE("/friends/request/:requestId", h.CancelFriendRequest)
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


