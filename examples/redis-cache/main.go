package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
	"github.com/redis/go-redis/v9"
)

// User represents a simple user struct
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Simulate an expensive database operation
func getUserFromDatabase(userID int) (*User, error) {
	// Simulate database latency
	time.Sleep(100 * time.Millisecond)

	// Simulate database query
	user := &User{
		ID:    userID,
		Name:  fmt.Sprintf("User %d", userID),
		Email: fmt.Sprintf("user%d@example.com", userID),
	}

	return user, nil
}

func main() {
	fmt.Println("🚀 obcache-go Redis Adapter Example")
	fmt.Println("===================================")

	// Example 1: Basic Redis cache usage
	fmt.Println("\n1. Basic Redis Cache Operations")
	basicRedisExample()

	// Example 2: Function wrapping with Redis cache
	fmt.Println("\n2. Function Wrapping with Redis Cache")
	functionWrappingExample()

	// Example 3: Distributed caching scenario
	fmt.Println("\n3. Distributed Caching with Custom Client")
	distributedCachingExample()
}

func basicRedisExample() {
	// Create a Redis cache with simple address
	config := obcache.NewRedisConfig("localhost:6379").
		WithRedis(&obcache.RedisConfig{
			KeyPrefix: "example:",
		}).
		WithDefaultTTL(30 * time.Minute)

	cache, err := obcache.New(config)
	if err != nil {
		log.Printf("Failed to create Redis cache: %v", err)
		fmt.Println("⚠️  Make sure Redis is running on localhost:6379")
		return
	}
	defer func() {
		if err := cache.Close(); err != nil {
			log.Printf("Error closing cache: %v", err)
		}
	}()

	// Store a user in cache
	user := &User{ID: 1, Name: "Alice", Email: "alice@example.com"}
	err = cache.Set("user:1", user, time.Hour)
	if err != nil {
		log.Printf("Failed to set cache entry: %v", err)
		return
	}

	// Retrieve user from cache
	if cachedUser, found := cache.Get("user:1"); found {
		fmt.Printf("✅ Found cached user: %+v\n", cachedUser)
	} else {
		fmt.Println("❌ User not found in cache")
	}

	// Show cache statistics
	stats := cache.Stats()
	fmt.Printf("📊 Cache Stats - Hits: %d, Misses: %d, Hit Rate: %.1f%%\n",
		stats.Hits(), stats.Misses(), stats.HitRate())
}

func functionWrappingExample() {
	// Create Redis cache configuration
	config := obcache.NewRedisConfig("localhost:6379").
		WithRedis(&obcache.RedisConfig{
			KeyPrefix: "func:",
		}).
		WithDefaultTTL(15 * time.Minute)

	cache, err := obcache.New(config)
	if err != nil {
		log.Printf("Failed to create Redis cache: %v", err)
		fmt.Println("⚠️  Make sure Redis is running on localhost:6379")
		return
	}
	defer func() {
		if err := cache.Close(); err != nil {
			log.Printf("Error closing cache: %v", err)
		}
	}()

	// Wrap the expensive function with caching
	cachedGetUser := obcache.Wrap(cache, getUserFromDatabase,
		obcache.WithTTL(5*time.Minute))

	// First call - will execute the function and cache the result
	start := time.Now()
	user1, err := cachedGetUser(123)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}
	duration1 := time.Since(start)
	fmt.Printf("🔄 First call (cache miss): %+v (took %v)\n", user1, duration1)

	// Second call - will return from cache
	start = time.Now()
	user2, err := cachedGetUser(123)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}
	duration2 := time.Since(start)
	fmt.Printf("⚡ Second call (cache hit): %+v (took %v)\n", user2, duration2)

	// Show performance improvement
	fmt.Printf("🚀 Speed improvement: %.1fx faster\n", float64(duration1)/float64(duration2))

	// Show cache statistics
	stats := cache.Stats()
	fmt.Printf("📊 Cache Stats - Hits: %d, Misses: %d, Hit Rate: %.1f%%\n",
		stats.Hits(), stats.Misses(), stats.HitRate())
}

func distributedCachingExample() {
	// Create a Redis client with custom configuration for distributed scenario
	client := redis.NewClient(&redis.Options{
		Addr:         "localhost:6379",
		Password:     "", // No password
		DB:           1,  // Use database 1 for this example
		PoolSize:     10,
		MinIdleConns: 5,
		MaxRetries:   3,
	})

	// Test the connection
	if err := client.Ping(context.Background()).Err(); err != nil {
		log.Printf("Failed to connect to Redis: %v", err)
		fmt.Println("⚠️  Make sure Redis is running on localhost:6379")
		return
	}

	// Create cache configuration with the custom client
	config := obcache.NewRedisConfigWithClient(client).
		WithRedis(&obcache.RedisConfig{
			KeyPrefix: "distributed:",
		}).
		WithDefaultTTL(1 * time.Hour)

	cache, err := obcache.New(config)
	if err != nil {
		log.Printf("Failed to create distributed cache: %v", err)
		return
	}
	defer func() {
		if err := cache.Close(); err != nil {
			log.Printf("Error closing cache: %v", err)
		}
	}()

	// Simulate distributed caching scenario
	fmt.Println("🌐 Simulating distributed caching...")

	// Store some data that could be shared across multiple application instances
	sessionData := map[string]interface{}{
		"user_id":    42,
		"username":   "distributed_user",
		"session_id": "sess_123456789",
		"expires_at": time.Now().Add(2 * time.Hour),
	}

	err = cache.Set("session:sess_123456789", sessionData, 2*time.Hour)
	if err != nil {
		log.Printf("Failed to store session: %v", err)
		return
	}

	// Retrieve session data (could be from any application instance)
	if session, found := cache.Get("session:sess_123456789"); found {
		fmt.Printf("✅ Retrieved session data: %+v\n", session)
	}

	// Show all keys with our prefix
	keys := cache.Keys()
	fmt.Printf("🔑 Found %d keys in distributed cache: %v\n", len(keys), keys)

	fmt.Println("✨ Distributed caching example completed!")
}
