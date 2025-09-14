package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/vnykmshr/obcache-go/pkg/obcache"
)

// Context key type for type safety
type contextKey string

const requestIDKey contextKey = "requestID"

// User represents a user from the database
type User struct {
	ID       int       `json:"id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	LastSeen time.Time `json:"last_seen"`
}

// UserService simulates a database service
type UserService struct {
	// Simulate database call delay
	mu    sync.Mutex
	calls int
}

func (s *UserService) GetUser(id int) (*User, error) {
	s.mu.Lock()
	s.calls++
	callNum := s.calls
	s.mu.Unlock()

	fmt.Printf("📞 Database call #%d for user ID %d\n", callNum, id)

	// Simulate database delay
	time.Sleep(100 * time.Millisecond)

	// Simulate occasional errors
	if id == 999 {
		return nil, fmt.Errorf("user %d not found", id)
	}

	return &User{
		ID:       id,
		Name:     fmt.Sprintf("User %d", id),
		Email:    fmt.Sprintf("user%d@example.com", id),
		LastSeen: time.Now().Add(-time.Duration(id) * time.Hour),
	}, nil
}

func (s *UserService) GetUsersByRole(role string) ([]User, error) {
	s.mu.Lock()
	s.calls++
	callNum := s.calls
	s.mu.Unlock()

	fmt.Printf("📞 Database call #%d for role '%s'\n", callNum, role)
	time.Sleep(200 * time.Millisecond) // Longer for complex query

	users := make([]User, 3) // Return 3 users for any role
	for i := 0; i < 3; i++ {
		users[i] = User{
			ID:    i + 1,
			Name:  fmt.Sprintf("%s User %d", role, i+1),
			Email: fmt.Sprintf("%s%d@example.com", role, i+1),
		}
	}

	return users, nil
}

// GetUserWithContext is a context-aware version that demonstrates new hook features
func (s *UserService) GetUserWithContext(ctx context.Context, id int) (*User, error) {
	s.mu.Lock()
	s.calls++
	callNum := s.calls
	s.mu.Unlock()

	// Extract request metadata from context
	const unknownRequestID = "unknown"
	requestID := unknownRequestID
	if rid, ok := ctx.Value(requestIDKey).(string); ok {
		requestID = rid
	}

	fmt.Printf("📞 Database call #%d for user ID %d (request: %s)\n", callNum, id, requestID)
	time.Sleep(100 * time.Millisecond)

	if id == 999 {
		return nil, fmt.Errorf("user %d not found", id)
	}

	return &User{
		ID:       id,
		Name:     fmt.Sprintf("User %d", id),
		Email:    fmt.Sprintf("user%d@example.com", id),
		LastSeen: time.Now().Add(-time.Duration(id) * time.Hour),
	}, nil
}

func main() {
	fmt.Println("🚀 Advanced obcache-go Example")
	fmt.Println("============================")

	// Create service
	userService := &UserService{}

	// Example 1: Cache with hooks for monitoring
	fmt.Println("\n📊 Example 1: Cache with Monitoring Hooks")

	hooks := &obcache.Hooks{
		OnHit: []obcache.OnHitHook{
			func(key string, value any) {
				fmt.Printf("✅ Cache HIT: %s\n", key)
			},
		},
		OnMiss: []obcache.OnMissHook{
			func(key string) {
				fmt.Printf("❌ Cache MISS: %s\n", key)
			},
		},
		OnEvict: []obcache.OnEvictHook{
			func(key string, value any, reason obcache.EvictReason) {
				fmt.Printf("🗑️  Cache EVICT: %s (reason: %v)\n", key, reason)
			},
		},
		// New context-aware hooks with function arguments
		OnHitCtx: []obcache.OnHitHookCtx{
			func(ctx context.Context, key string, value any, args []any) {
				requestID := "unknown"
				if rid, ok := ctx.Value(requestIDKey).(string); ok {
					requestID = rid
				}
				fmt.Printf("🎯 Context-aware HIT: %s (request: %s, args: %v)\n", key, requestID, args)
			},
		},
		OnMissCtx: []obcache.OnMissHookCtx{
			func(ctx context.Context, key string, args []any) {
				requestID := "unknown"
				if rid, ok := ctx.Value(requestIDKey).(string); ok {
					requestID = rid
				}
				fmt.Printf("🎯 Context-aware MISS: %s (request: %s, args: %v)\n", key, requestID, args)
			},
		},
	}

	config := obcache.NewDefaultConfig().
		WithMaxEntries(3).               // Small cache for demo
		WithDefaultTTL(2 * time.Second). // Short TTL for demo
		WithCleanupInterval(500 * time.Millisecond).
		WithHooks(hooks)

	cache, err := obcache.New(config)
	if err != nil {
		panic(err)
	}

	// Wrap the user service methods
	cachedGetUser := obcache.Wrap(cache, userService.GetUser)
	cachedGetUsersByRole := obcache.Wrap(cache, userService.GetUsersByRole,
		obcache.WithTTL(5*time.Second), // Longer TTL for role queries
	)

	// Test single user caching
	fmt.Println("\n--- Single User Queries ---")

	// First call - cache miss
	user1, err := cachedGetUser(1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("👤 User: %s (%s)\n", user1.Name, user1.Email)

	// Second call - cache hit
	user1Again, _ := cachedGetUser(1)
	fmt.Printf("👤 User: %s (%s)\n", user1Again.Name, user1Again.Email)

	// Different user - cache miss
	user2, _ := cachedGetUser(2)
	fmt.Printf("👤 User: %s (%s)\n", user2.Name, user2.Email)

	// Example 1.5: Context-aware wrapped functions
	fmt.Println("\n🎯 Example 1.5: Context-aware Functions with Enhanced Hooks")

	// Wrap the context-aware method
	cachedGetUserWithContext := obcache.Wrap(cache, userService.GetUserWithContext)

	// Create contexts with request metadata
	ctx1 := context.WithValue(context.Background(), requestIDKey, "req-001")
	ctx2 := context.WithValue(context.Background(), requestIDKey, "req-002")

	// First call with context - cache miss
	ctxUser1, err := cachedGetUserWithContext(ctx1, 10)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("👤 Context User: %s (%s)\n", ctxUser1.Name, ctxUser1.Email)

	// Second call with same context and args - cache hit
	ctxUser1Again, _ := cachedGetUserWithContext(ctx1, 10)
	fmt.Printf("👤 Context User: %s (%s)\n", ctxUser1Again.Name, ctxUser1Again.Email)

	// Third call with different context but same args - cache hit (context in hooks only)
	ctxUser1Diff, _ := cachedGetUserWithContext(ctx2, 10)
	fmt.Printf("👤 Context User: %s (%s)\n", ctxUser1Diff.Name, ctxUser1Diff.Email)

	// Test role-based queries
	fmt.Println("\n--- Role-based Queries ---")

	// First role query - cache miss
	admins1, _ := cachedGetUsersByRole("admin")
	fmt.Printf("👥 Found %d admins\n", len(admins1))

	// Same role query - cache hit
	admins2, _ := cachedGetUsersByRole("admin")
	fmt.Printf("👥 Found %d admins (cached)\n", len(admins2))

	// Different role - cache miss
	users, _ := cachedGetUsersByRole("user")
	fmt.Printf("👥 Found %d users\n", len(users))

	// Example 2: Demonstrate singleflight behavior
	fmt.Println("\n⚡ Example 2: Singleflight Demonstration")

	slowService := &UserService{}

	// Create a new cache for this demo
	singleflightCache, _ := obcache.New(obcache.NewDefaultConfig())
	cachedSlowGetUser := obcache.Wrap(singleflightCache, slowService.GetUser)

	// Launch multiple concurrent requests for the same user
	var wg sync.WaitGroup
	startTime := time.Now()

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			start := time.Now()
			user, err := cachedSlowGetUser(100) // All requesting same user
			duration := time.Since(start)

			if err != nil {
				fmt.Printf("⚠️  Worker %d: Error - %v\n", workerID, err)
			} else {
				fmt.Printf("✅ Worker %d: Got user %s in %v\n",
					workerID, user.Name, duration)
			}
		}(i)
	}

	wg.Wait()
	totalTime := time.Since(startTime)
	fmt.Printf("🎯 Total time for 5 concurrent requests: %v\n", totalTime)
	fmt.Printf("💡 Notice: Only 1 database call made due to singleflight!\n")

	// Example 3: Error handling (errors are not cached)
	fmt.Println("\n🚨 Example 3: Error Handling")

	errorCache, _ := obcache.New(obcache.NewDefaultConfig())
	cachedGetUserWithErrors := obcache.Wrap(errorCache, userService.GetUser)

	// Try to get a non-existent user (will error)
	fmt.Println("Attempting to get user 999 (this will fail)...")
	_, err = cachedGetUserWithErrors(999)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	}

	// Try again - should call function again since errors aren't cached
	fmt.Println("Attempting user 999 again...")
	_, err = cachedGetUserWithErrors(999)
	if err != nil {
		fmt.Printf("❌ Error: %v (called function again)\n", err)
	}

	// Example 4: Cache statistics and monitoring
	fmt.Println("\n📈 Example 4: Cache Statistics")

	stats := cache.Stats()
	fmt.Printf("Cache Performance:\n")
	fmt.Printf("  📊 Total Requests: %d\n", stats.Total())
	fmt.Printf("  ✅ Hits: %d\n", stats.Hits())
	fmt.Printf("  ❌ Misses: %d\n", stats.Misses())
	fmt.Printf("  🎯 Hit Rate: %.1f%%\n", stats.HitRate())
	fmt.Printf("  🗑️  Evictions: %d\n", stats.Evictions())
	fmt.Printf("  🔢 Current Keys: %d\n", stats.KeyCount())
	fmt.Printf("  ⏳ In-Flight Requests: %d\n", stats.InFlight())

	// Example 5: TTL demonstration
	fmt.Println("\n⏰ Example 5: TTL Expiration")

	ttlCache, _ := obcache.New(obcache.NewDefaultConfig())
	shortTTLFunc := obcache.Wrap(ttlCache, userService.GetUser,
		obcache.WithTTL(1*time.Second), // Very short TTL
	)

	// First call
	fmt.Println("Getting user with short TTL...")
	user, _ := shortTTLFunc(50)
	fmt.Printf("📝 Got: %s\n", user.Name)

	// Immediate second call - should hit cache
	user, _ = shortTTLFunc(50)
	fmt.Printf("📝 Got: %s (from cache)\n", user.Name)

	// Wait for TTL expiration
	fmt.Println("⏳ Waiting for TTL expiration...")
	time.Sleep(1200 * time.Millisecond)

	// Call after TTL - should execute function again
	user, _ = shortTTLFunc(50)
	fmt.Printf("📝 Got: %s (recomputed after TTL)\n", user.Name)

	fmt.Println("\n🎉 Advanced example completed!")

	// Final stats
	finalStats := cache.Stats()
	fmt.Printf("\n📊 Final Cache Stats: %d hits, %d misses (%.1f%% hit rate)\n",
		finalStats.Hits(), finalStats.Misses(), finalStats.HitRate())
}
