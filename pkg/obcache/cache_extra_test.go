package obcache

import (
	"testing"
	"time"
)

func TestCacheKeys(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Initially should be empty
	keys := cache.Keys()
	if len(keys) != 0 {
		t.Fatalf("Expected 0 keys, got %d", len(keys))
	}

	// Add some entries
	_ = cache.Set("key1", "value1", time.Hour)
	_ = cache.Set("key2", "value2", time.Hour)
	_ = cache.Set("key3", "value3", time.Hour)

	keys = cache.Keys()
	if len(keys) != 3 {
		t.Fatalf("Expected 3 keys, got %d", len(keys))
	}

	// Check that all expected keys are present
	expectedKeys := map[string]bool{"key1": true, "key2": true, "key3": true}
	for _, key := range keys {
		if !expectedKeys[key] {
			t.Fatalf("Unexpected key: %s", key)
		}
		delete(expectedKeys, key)
	}

	if len(expectedKeys) != 0 {
		t.Fatal("Some expected keys were not returned")
	}
}

func TestCacheLen(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Initially should be empty
	if cache.Len() != 0 {
		t.Fatalf("Expected length 0, got %d", cache.Len())
	}

	// Add some entries
	_ = cache.Set("key1", "value1", time.Hour)
	if cache.Len() != 1 {
		t.Fatalf("Expected length 1, got %d", cache.Len())
	}

	_ = cache.Set("key2", "value2", time.Hour)
	if cache.Len() != 2 {
		t.Fatalf("Expected length 2, got %d", cache.Len())
	}

	// Remove an entry
	_ = cache.Delete("key1")
	if cache.Len() != 1 {
		t.Fatalf("Expected length 1 after deletion, got %d", cache.Len())
	}
}

func TestCacheHas(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Initially should not have any keys
	if cache.Has("nonexistent") {
		t.Fatal("Cache should not have nonexistent key")
	}

	// Add an entry
	_ = cache.Set("key1", "value1", time.Hour)

	if !cache.Has("key1") {
		t.Fatal("Cache should have key1")
	}

	if cache.Has("key2") {
		t.Fatal("Cache should not have key2")
	}

	// Test with expired entry - add and wait
	_ = cache.Set("expired", "value", time.Nanosecond) // Very short TTL
	time.Sleep(2 * time.Millisecond)                   // Wait for expiration
	if cache.Has("expired") {
		t.Fatal("Cache should not have expired key")
	}
}

func TestCacheTTLMethod(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Test missing key
	ttl, found := cache.TTL("missing")
	if found {
		t.Fatal("Should not find missing key")
	}
	if ttl != 0 {
		t.Fatal("TTL should be 0 for missing key")
	}

	// Add an entry with TTL
	originalTTL := time.Hour
	_ = cache.Set("key1", "value1", originalTTL)

	value, found := cache.Get("key1")
	if !found {
		t.Fatal("Should find existing key")
	}
	if value != "value1" {
		t.Fatalf("Expected value1, got %v", value)
	}

	ttl, found = cache.TTL("key1")
	if !found {
		t.Fatal("Should find TTL for existing key")
	}
	if ttl <= 0 || ttl > originalTTL {
		t.Fatalf("TTL should be positive and <= original TTL, got %v", ttl)
	}

	// Add an entry with default TTL via Put
	_ = cache.Put("default-ttl", "value-default-ttl") // Test setup

	value, found = cache.Get("default-ttl")
	if !found {
		t.Fatal("Should find key with default TTL")
	}
	if value != "value-default-ttl" {
		t.Fatalf("Expected value-default-ttl, got %v", value)
	}

	ttl, found = cache.TTL("default-ttl")
	if !found {
		t.Fatal("Should find TTL for default TTL key")
	}
	if ttl <= 0 || ttl > 5*time.Minute {
		t.Fatalf("TTL should be positive and <= 5 minutes for default TTL, got %v", ttl)
	}
}

func TestCacheClear(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add multiple entries
	_ = cache.Set("key1", "value1", time.Hour)
	_ = cache.Set("key2", "value2", time.Hour)
	_ = cache.Set("key3", "value3", time.Hour)

	if cache.Len() != 3 {
		t.Fatalf("Expected 3 entries before Clear, got %d", cache.Len())
	}

	// Clear all
	_ = cache.Clear() // Test cleanup

	if cache.Len() != 0 {
		t.Fatalf("Expected 0 entries after Clear, got %d", cache.Len())
	}

	// Verify none of the keys exist
	if cache.Has("key1") || cache.Has("key2") || cache.Has("key3") {
		t.Fatal("No keys should exist after Clear")
	}
}

func TestCacheClose(t *testing.T) {
	config := NewDefaultConfig().WithCleanupInterval(10 * time.Millisecond)
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add an entry
	_ = cache.Set("key1", "value1", time.Hour)

	// Close should not error
	_ = cache.Close() // Test cleanup

	// After close, operations should still work but cleanup stops
	_ = cache.Set("key2", "value2", time.Hour)
	if !cache.Has("key2") {
		t.Fatal("Cache should still work after Close")
	}
}

func TestCacheCleanup(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add entries with very short and long TTLs
	_ = cache.Set("expired", "value", time.Nanosecond) // Very short TTL
	_ = cache.Set("valid", "value", time.Hour)

	// Wait for the short TTL entry to expire
	time.Sleep(2 * time.Millisecond)

	initialLen := cache.Len()
	if initialLen < 1 || initialLen > 2 {
		t.Fatalf("Expected 1-2 entries before cleanup, got %d", initialLen)
	}

	// Run cleanup
	cache.Cleanup()

	// Only valid entry should remain
	if cache.Len() != 1 {
		t.Fatalf("Expected 1 entry after cleanup, got %d", cache.Len())
	}

	if !cache.Has("valid") {
		t.Fatal("Valid entry should remain after cleanup")
	}

	if cache.Has("expired") {
		t.Fatal("Expired entry should be removed after cleanup")
	}
}

func TestCacheClearWithHooks(t *testing.T) {
	invalidateCount := 0

	hooks := &Hooks{
		OnInvalidate: []OnInvalidateHook{
			func(_ string) {
				invalidateCount++
			},
		},
	}

	config := NewDefaultConfig().WithHooks(hooks)
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add multiple entries
	_ = cache.Set("key1", "value1", time.Hour)
	_ = cache.Set("key2", "value2", time.Hour)

	// Invalidate all should trigger hooks
	_ = cache.Clear() // Test cleanup

	// Should have called invalidate hook for each entry
	if invalidateCount != 2 {
		t.Fatalf("Expected 2 invalidate hook calls, got %d", invalidateCount)
	}
}
