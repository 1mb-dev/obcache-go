package obcache

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestConfigDefaults(t *testing.T) {
	config := NewDefaultConfig()

	if config.MaxEntries != 1000 {
		t.Fatalf("Expected MaxEntries 1000, got %d", config.MaxEntries)
	}
	if config.DefaultTTL != 5*time.Minute {
		t.Fatalf("Expected DefaultTTL 5m, got %v", config.DefaultTTL)
	}
	if config.CleanupInterval != time.Minute {
		t.Fatalf("Expected CleanupInterval 1m, got %v", config.CleanupInterval)
	}
	if config.KeyGenFunc != nil {
		t.Fatal("Expected KeyGenFunc to be nil by default")
	}
	if config.Hooks == nil {
		t.Fatal("Expected Hooks to be non-nil")
	}
}

func TestWithMaxEntries(t *testing.T) {
	config := NewDefaultConfig().WithMaxEntries(500)

	if config.MaxEntries != 500 {
		t.Fatalf("Expected MaxEntries 500, got %d", config.MaxEntries)
	}
}

func TestWithDefaultTTL(t *testing.T) {
	ttl := 2 * time.Hour
	config := NewDefaultConfig().WithDefaultTTL(ttl)

	if config.DefaultTTL != ttl {
		t.Fatalf("Expected DefaultTTL %v, got %v", ttl, config.DefaultTTL)
	}
}

func TestWithCleanupInterval(t *testing.T) {
	interval := 30 * time.Second
	config := NewDefaultConfig().WithCleanupInterval(interval)

	if config.CleanupInterval != interval {
		t.Fatalf("Expected CleanupInterval %v, got %v", interval, config.CleanupInterval)
	}
}

func TestWithKeyGenFunc(t *testing.T) {
	customKeyFunc := func(_ []any) string {
		const customKey = "custom-key"
		return customKey
	}
	config := NewDefaultConfig().WithKeyGenFunc(customKeyFunc)

	if config.KeyGenFunc == nil {
		t.Fatal("Expected KeyGenFunc to be set")
	}

	// Test that the function works
	key := config.KeyGenFunc([]any{"test"})
	if key != "custom-key" {
		t.Fatalf("Expected 'custom-key', got '%s'", key)
	}
}

func TestWithHooks(t *testing.T) {
	hooks := NewHooks()
	hooks.AddOnHit(func(_ context.Context, _ string, _ any) {})

	config := NewDefaultConfig().WithHooks(hooks)

	if config.Hooks != hooks {
		t.Fatal("Expected Hooks to be set")
	}
}

func TestConfigBuilder(t *testing.T) {
	// Test building config with multiple options
	customKeyFunc := func(_ []any) string {
		return testKeyConst
	}

	hooks := NewHooks()
	hooks.AddOnHit(func(_ context.Context, _ string, _ any) {})

	config := NewDefaultConfig().
		WithMaxEntries(200).
		WithDefaultTTL(30 * time.Minute).
		WithCleanupInterval(2 * time.Minute).
		WithKeyGenFunc(customKeyFunc).
		WithHooks(hooks)

	// Verify all fields are set correctly
	if config.MaxEntries != 200 {
		t.Fatalf("Expected MaxEntries 200, got %d", config.MaxEntries)
	}
	if config.DefaultTTL != 30*time.Minute {
		t.Fatalf("Expected DefaultTTL 30m, got %v", config.DefaultTTL)
	}
	if config.CleanupInterval != 2*time.Minute {
		t.Fatalf("Expected CleanupInterval 2m, got %v", config.CleanupInterval)
	}
	if config.KeyGenFunc == nil {
		t.Fatal("Expected KeyGenFunc to be set")
	}
	if config.Hooks != hooks {
		t.Fatal("Expected Hooks to be set")
	}

	// Test custom key function
	key := config.KeyGenFunc([]any{"anything"})
	if key != "test-key" {
		t.Fatalf("Expected 'test-key', got '%s'", key)
	}
}

func TestNewCacheWithConfig(t *testing.T) {
	// Test that New() properly applies config options
	config := NewDefaultConfig().WithMaxEntries(50).WithDefaultTTL(10 * time.Minute)
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Verify the cache was created with the right config
	// We can test this indirectly by checking behavior

	// Fill the cache beyond the max entries to test eviction
	for i := 0; i < 60; i++ { // More than the 50 limit
		_ = cache.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i), time.Hour) // Test setup
	}

	stats := cache.Stats()

	// Should have evictions due to the 50-entry limit
	if stats.Evictions() == 0 {
		t.Fatal("Expected some evictions due to MaxEntries limit")
	}

	// Key count should be around the max entries limit
	if stats.KeyCount() > 50 {
		t.Fatalf("Expected key count <= 50, got %d", stats.KeyCount())
	}
}

func TestConfigCopy(t *testing.T) {
	// Test that config modifications don't affect existing caches
	originalKeyFunc := func(_ []any) string {
		return "original"
	}

	config1 := NewDefaultConfig().WithKeyGenFunc(originalKeyFunc)
	cache1, err := New(config1)
	if err != nil {
		t.Fatalf("Failed to create cache1: %v", err)
	}

	// Create another cache with different config
	newKeyFunc := func(_ []any) string {
		return "new"
	}

	config2 := NewDefaultConfig().WithKeyGenFunc(newKeyFunc)
	cache2, err := New(config2)
	if err != nil {
		t.Fatalf("Failed to create cache2: %v", err)
	}

	// Both caches should work independently
	// We can test this indirectly through wrapped functions

	fn := func(x int) int { return x * 2 }

	wrapped1 := Wrap(cache1, fn)
	wrapped2 := Wrap(cache2, fn)

	// Both should work (if they share config, one might interfere with the other)
	result1 := wrapped1(5)
	result2 := wrapped2(5)

	if result1 != 10 || result2 != 10 {
		t.Fatalf("Expected both to return 10, got %d and %d", result1, result2)
	}
}
