package main

import (
	"database/sql"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

func main() {
	cfg := LoadConfig()

	db, err := initDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to init database: %v", err)
	}
	defer db.Close()

	store := NewStore(db)
	rateLimiter := NewRateLimiter()

	userStore := &UserStore{Store: store}
	msgStore := &MessageStore{Store: store}
	groupStore := &GroupStore{Store: store}
	groupMemStore := &GroupMemberStore{Store: store}
	friendStore := &FriendRequestStore{Store: store}
	convPartStore := &ConversationParticipantStore{Store: store}
	deviceStore := &UserDeviceStore{Store: store}
	idGenStateStore := &IdGeneratorStateStore{Store: store}

	idGen := NewIdGenerator(idGenStateStore, msgStore, cfg.NodeID)
	idGen.Init()

	svc := NewService(
		userStore, msgStore, groupStore, groupMemStore,
		friendStore, convPartStore, deviceStore,
		idGenStateStore, idGen, rateLimiter,
	)

	jwtService := NewJwtService(cfg.JWTSecret, "vortex")
	handler := NewHandler(svc, jwtService)

	r := gin.Default()
	setupRoutes(r, handler, jwtService, userStore, deviceStore, cfg)

	worker := NewWorker(svc, msgStore)
	worker.Start()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := r.Run(":8080"); err != nil {
			log.Fatalf("failed to start server: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")
	worker.Stop()
}

func setupRoutes(r *gin.Engine, h *Handler, jwtService *JwtService, us *UserStore, ds *UserDeviceStore, cfg *Config) {
	r.Use(corsMiddleware(cfg))

	api := r.Group("/api")
	{
		api.POST("/auth/register", h.Register)
		api.POST("/auth/login", h.Login)

		auth := api.Group("")
		auth.Use(jwtMiddleware(jwtService))
		auth.Use(deviceTokenMiddleware(us, ds))
		{
			auth.GET("/auth/me", h.GetMe)
			auth.POST("/auth/logout", h.Logout)
			auth.PUT("/auth/:publicId", h.UpdateUser)
			auth.DELETE("/auth/:publicId", h.DeleteUser)

			auth.POST("/messages/send", h.SendMessage)
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

func initDB(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(20)

	return db, nil
}

func corsMiddleware(cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Device-Token")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func jwtMiddleware(jwtService *JwtService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(401, ErrorResponse{Error: "unauthorized"})
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := jwtService.ValidateToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(401, ErrorResponse{Error: "unauthorized"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("public_id", claims.PublicID)
		c.Set("username", claims.Username)
		c.Next()
	}
}

func deviceTokenMiddleware(userStore *UserStore, deviceStore *UserDeviceStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetInt64("user_id")
		deviceToken := c.GetHeader("X-Device-Token")

		if userID == 0 || deviceToken == "" {
			c.Next()
			return
		}

		user, err := userStore.GetByID(userID)
		if err != nil || user == nil {
			c.AbortWithStatusJSON(401, ErrorResponse{Error: "unauthorized"})
			return
		}

		valid, err := deviceStore.TokenBelongsToUser(userID, deviceToken)
		if err != nil || !valid {
			c.AbortWithStatusJSON(401, ErrorResponse{Error: "invalid device token"})
			return
		}

		c.Next()
	}
}
