// Package main demonstrates using obcache-go with Gin web framework
// for caching expensive operations like database queries and API calls.
//
// This example shows:
// - User profile caching with Redis backend
// - API response caching with TTL
// - Cache statistics monitoring
// - Graceful error handling
//
// Run with: go run main.go
// Test with: curl http://localhost:8080/user/123
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

// User represents a user profile
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	LastSeen string `json:"last_seen"`
}

// UserService simulates a database service
type UserService struct {
	// In real application, this would be your database connection
	simulatedDB map[int]*User
}

func NewUserService() *UserService {
	// Simulate some users in the "database"
	users := map[int]*User{
		123: {ID: 123, Username: "john_doe", Email: "john@example.com", Role: "user", LastSeen: "2025-01-15"},
		456: {ID: 456, Username: "jane_admin", Email: "jane@example.com", Role: "admin", LastSeen: "2025-01-16"},
		789: {ID: 789, Username: "bob_user", Email: "bob@example.com", Role: "user", LastSeen: "2025-01-14"},
	}
	return &UserService{simulatedDB: users}
}

func (s *UserService) GetUserByID(userID int) (*User, error) {
	// Simulate database latency
	time.Sleep(100 * time.Millisecond)

	user, exists := s.simulatedDB[userID]
	if !exists {
		return nil, fmt.Errorf("user not found: %d", userID)
	}

	// Simulate updating last seen (in real app, this might be a separate call)
	userCopy := *user
	userCopy.LastSeen = time.Now().Format("2006-01-02 15:04:05")

	log.Printf("Database query executed for user ID: %d", userID)
	return &userCopy, nil
}

// CachedUserService wraps UserService with caching
type CachedUserService struct {
	userService *UserService
	cache       *obcache.Cache
	getUserFunc func(int) (*User, error)
}

func NewCachedUserService() (*CachedUserService, error) {
	userService := NewUserService()

	// Configure cache based on environment
	var cache *obcache.Cache
	var err error

	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		// Production: Use Redis backend
		config := obcache.NewRedisConfig(redisURL).
			WithDefaultTTL(10 * time.Minute)
		config.Redis.KeyPrefix = "user_cache:" // Set custom key prefix
		cache, err = obcache.New(config)
	} else {
		// Development: Use memory backend
		config := obcache.NewDefaultConfig().
			WithMaxEntries(1000).
			WithDefaultTTL(5 * time.Minute).
			WithCleanupInterval(time.Minute)
		cache, err = obcache.New(config)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	// Add cache monitoring hooks
	hooks := &obcache.Hooks{}
	hooks.AddOnHit(func(ctx context.Context, key string, value any) {
		log.Printf("Cache HIT for key: %s", key)
	})
	hooks.AddOnMiss(func(ctx context.Context, key string) {
		log.Printf("Cache MISS for key: %s", key)
	})
	hooks.AddOnEvict(func(ctx context.Context, key string, value any, reason obcache.EvictReason) {
		log.Printf("Cache EVICT: key=%s, reason=%s", key, reason)
	})

	// Create cached function with error caching disabled for user queries
	// (we don't want to cache "user not found" errors for too long)
	cachedGetUser := obcache.Wrap(cache, userService.GetUserByID,
		obcache.WithTTL(5*time.Minute),
		obcache.WithKeyFunc(func(args []any) string {
			return fmt.Sprintf("user:%d", args[0].(int))
		}))

	return &CachedUserService{
		userService: userService,
		cache:       cache,
		getUserFunc: cachedGetUser,
	}, nil
}

func (s *CachedUserService) GetUser(userID int) (*User, error) {
	return s.getUserFunc(userID)
}

func (s *CachedUserService) GetCache() *obcache.Cache {
	return s.cache
}

func (s *CachedUserService) Close() error {
	return s.cache.Close()
}

// WebServer handles HTTP requests
type WebServer struct {
	userService *CachedUserService
	router      *gin.Engine
}

func NewWebServer() (*WebServer, error) {
	userService, err := NewCachedUserService()
	if err != nil {
		return nil, err
	}

	router := gin.Default()

	server := &WebServer{
		userService: userService,
		router:      router,
	}

	server.setupRoutes()
	return server, nil
}

func (s *WebServer) setupRoutes() {
	// User endpoints
	s.router.GET("/user/:id", s.getUser)
	s.router.DELETE("/user/:id/cache", s.invalidateUserCache)

	// Cache management endpoints
	s.router.GET("/cache/stats", s.getCacheStats)
	s.router.DELETE("/cache/clear", s.clearCache)

	// Health check
	s.router.GET("/health", s.healthCheck)
}

func (s *WebServer) getUser(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	// Add timing for demonstration
	start := time.Now()

	user, err := s.userService.GetUser(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	duration := time.Since(start)

	c.Header("X-Cache-Duration", duration.String())
	c.JSON(http.StatusOK, gin.H{
		"user":     user,
		"cached":   duration < 50*time.Millisecond, // Rough heuristic
		"duration": duration.String(),
	})
}

func (s *WebServer) invalidateUserCache(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	key := fmt.Sprintf("user:%d", userID)
	err = s.userService.GetCache().Delete(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to invalidate cache",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Cache invalidated for user %d", userID),
	})
}

func (s *WebServer) getCacheStats(c *gin.Context) {
	stats := s.userService.GetCache().Stats()

	c.JSON(http.StatusOK, gin.H{
		"stats": gin.H{
			"hits":      stats.Hits(),
			"misses":    stats.Misses(),
			"hit_rate":  fmt.Sprintf("%.2f%%", stats.HitRate()),
			"evictions": stats.Evictions(),
			"key_count": stats.KeyCount(),
		},
	})
}

func (s *WebServer) clearCache(c *gin.Context) {
	err := s.userService.GetCache().Clear()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to clear cache",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Cache cleared successfully",
	})
}

func (s *WebServer) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"cache":  "operational",
	})
}

func (s *WebServer) Run(addr string) error {
	defer func() {
		if err := s.userService.Close(); err != nil {
			log.Printf("Error closing user service: %v", err)
		}
	}()
	return s.router.Run(addr)
}

func main() {
	// Configure logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	server, err := NewWebServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	log.Printf("Test endpoints:")
	log.Printf("  GET  http://localhost:%s/user/123", port)
	log.Printf("  GET  http://localhost:%s/cache/stats", port)
	log.Printf("  DELETE http://localhost:%s/user/123/cache", port)

	if err := server.Run(":" + port); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
