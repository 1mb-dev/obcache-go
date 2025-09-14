package obcache

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

const testKeyConst = "test-key"

func TestCacheWithRedisStore(t *testing.T) {
	// Create a Redis client
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // Use DB 15 for testing to avoid conflicts
	})

	// Test Redis connection - skip if not available
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available, skipping Redis integration test: %v", err)
	}

	// Clean up test data before starting
	client.FlushDB(ctx)

	// Create cache with Redis backend
	config := NewDefaultConfig().
		WithRedis(&RedisConfig{
			Client:    client,
			KeyPrefix: "test:cache:",
		}).
		WithDefaultTTL(30 * time.Minute)

	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create Redis cache: %v", err)
	}
	defer func() { _ = cache.Close() }()

	// Test basic operations
	testKey := testKeyConst
	testValue := "test-value"

	// Test Set
	err = cache.Set(testKey, testValue, time.Hour)
	if err != nil {
		t.Fatalf("Failed to set cache entry: %v", err)
	}

	// Test Get
	value, found := cache.Get(testKey)
	if !found {
		t.Fatal("Expected to find cached value")
	}
	if value != testValue {
		t.Fatalf("Expected value '%s', got '%v'", testValue, value)
	}

	// Test cache statistics
	stats := cache.Stats()
	if stats.Hits() != 1 {
		t.Fatalf("Expected 1 hit, got %d", stats.Hits())
	}
	if stats.Misses() != 0 {
		t.Fatalf("Expected 0 misses, got %d", stats.Misses())
	}

	// Test Delete
	err = cache.Delete(testKey)
	if err != nil {
		t.Fatalf("Failed to delete cache entry: %v", err)
	}

	_, found = cache.Get(testKey)
	if found {
		t.Fatal("Expected entry to be deleted")
	}

	// Test miss after invalidation
	stats = cache.Stats()
	if stats.Misses() != 1 {
		t.Fatalf("Expected 1 miss after invalidation, got %d", stats.Misses())
	}
}

func TestCacheWithRedisStoreAndHooks(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
	}

	client.FlushDB(ctx)

	// Set up hooks to track events
	hitCalled := false
	missCalled := false
	invalidateCalled := false

	hooks := &Hooks{}
	hooks.AddOnHit(func(key string, value any) {
		hitCalled = true
	})
	hooks.AddOnMiss(func(key string) {
		missCalled = true
	})
	hooks.AddOnInvalidate(func(key string) {
		invalidateCalled = true
	})

	config := NewDefaultConfig().
		WithRedis(&RedisConfig{
			Client:    client,
			KeyPrefix: "hooks:test:",
		}).
		WithHooks(hooks)

	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create Redis cache with hooks: %v", err)
	}
	defer func() { _ = cache.Close() }()

	testKey := "hooks-test"

	// Test miss hook
	_, _ = cache.Get(testKey)
	if !missCalled {
		t.Fatal("Expected miss hook to be called")
	}

	// Test hit hook
	_ = cache.Set(testKey, "value", time.Hour) // Test setup
	_, _ = cache.Get(testKey)
	if !hitCalled {
		t.Fatal("Expected hit hook to be called")
	}

	// Test invalidate hook
	_ = cache.Delete(testKey) // Test operation
	if !invalidateCalled {
		t.Fatal("Expected invalidate hook to be called")
	}
}

func TestWrappedFunctionWithRedisStore(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
	}

	client.FlushDB(ctx)

	config := NewDefaultConfig().
		WithRedis(&RedisConfig{
			Client:    client,
			KeyPrefix: "wrap:test:",
		})

	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create Redis cache: %v", err)
	}
	defer func() { _ = cache.Close() }()

	callCount := 0
	// Note: Using float64 to avoid JSON serialization type conversion issues
	// Integer types get converted to float64 during JSON round-trip
	expensiveFunc := func(x float64) float64 {
		callCount++
		return x * 2
	}

	wrappedFunc := Wrap(cache, expensiveFunc)

	// First call should execute function
	result1 := wrappedFunc(5.0)
	if result1 != 10.0 {
		t.Fatalf("Expected 10.0, got %f", result1)
	}
	if callCount != 1 {
		t.Fatalf("Expected function to be called once, got %d", callCount)
	}

	// Second call should use cache
	result2 := wrappedFunc(5.0)
	if result2 != 10.0 {
		t.Fatalf("Expected 10.0, got %f", result2)
	}
	if callCount != 1 {
		t.Fatalf("Expected function to still be called once, got %d", callCount)
	}

	// Different argument should call function again
	result3 := wrappedFunc(7.0)
	if result3 != 14.0 {
		t.Fatalf("Expected 14.0, got %f", result3)
	}
	if callCount != 2 {
		t.Fatalf("Expected function to be called twice, got %d", callCount)
	}
}

func TestRedisConfigBuilders(t *testing.T) {
	// Test NewRedisConfig
	config1 := NewRedisConfig("localhost:6379")
	if config1.StoreType != StoreTypeRedis {
		t.Fatal("Expected StoreType to be Redis")
	}
	if config1.Redis.Addr != "localhost:6379" {
		t.Fatalf("Expected Redis addr 'localhost:6379', got '%s'", config1.Redis.Addr)
	}
	if config1.Redis.KeyPrefix != "obcache:" {
		t.Fatalf("Expected default key prefix 'obcache:', got '%s'", config1.Redis.KeyPrefix)
	}

	// Test configuration chaining
	config2 := NewDefaultConfig().
		WithRedis(&RedisConfig{
			Addr:      "redis:6379",
			Password:  "password",
			DB:        2,
			KeyPrefix: "myapp:",
		})

	if config2.StoreType != StoreTypeRedis {
		t.Fatal("Expected StoreType to be Redis after WithRedis")
	}
	if config2.Redis.Addr != "redis:6379" {
		t.Fatalf("Expected Redis addr 'redis:6379', got '%s'", config2.Redis.Addr)
	}
	if config2.Redis.Password != "password" {
		t.Fatalf("Expected Redis password 'password', got '%s'", config2.Redis.Password)
	}
	if config2.Redis.DB != 2 {
		t.Fatalf("Expected Redis DB 2, got %d", config2.Redis.DB)
	}
	if config2.Redis.KeyPrefix != "myapp:" {
		t.Fatalf("Expected Redis key prefix 'myapp:', got '%s'", config2.Redis.KeyPrefix)
	}
}
