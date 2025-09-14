package obcache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCacheBasicOperations(t *testing.T) {
	config := NewDefaultConfig().WithMaxEntries(100).WithDefaultTTL(time.Hour)
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Test Set and Get
	const testKey = "test-key"
	key := testKey
	const testValue = "test-value"
	value := testValue

	err = cache.Set(key, value, time.Hour)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrieved, found := cache.Get(key)
	if !found {
		t.Fatal("Expected to find key")
	}
	if retrieved != value {
		t.Fatalf("Expected %v, got %v", value, retrieved)
	}

	// Test cache hit stats
	stats := cache.Stats()
	if stats.Hits() != 1 {
		t.Fatalf("Expected 1 hit, got %d", stats.Hits())
	}
	if stats.KeyCount() != 1 {
		t.Fatalf("Expected 1 key, got %d", stats.KeyCount())
	}
}

func TestCacheMiss(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	_, found := cache.Get("nonexistent")
	if found {
		t.Fatal("Expected not to find nonexistent key")
	}

	stats := cache.Stats()
	if stats.Misses() != 1 {
		t.Fatalf("Expected 1 miss, got %d", stats.Misses())
	}
}

func TestCacheDelete(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	const testKey = "test-key"
	key := testKey
	_ = cache.Set(key, "value", time.Hour) // Test setup

	// Verify it exists
	_, found := cache.Get(key)
	if !found {
		t.Fatal("Expected to find key before invalidation")
	}

	// Delete
	err = cache.Delete(key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	_, found = cache.Get(key)
	if found {
		t.Fatal("Expected key to be gone after deletion")
	}

	stats := cache.Stats()
	if stats.Invalidations() != 1 {
		t.Fatalf("Expected 1 invalidation, got %d", stats.Invalidations())
	}
}

func TestCachePut(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	const testKey = "test-key"
	key := testKey
	const warmedValue = "warmed-value"
	value := warmedValue

	// Put
	err = cache.Put(key, value)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify the value is cached
	retrievedValue, found := cache.Get(key)
	if !found {
		t.Fatal("Expected key to be found after put")
	}
	if retrievedValue != value {
		t.Fatalf("Expected %v, got %v", value, retrievedValue)
	}
}

func TestCacheSetWithTTL(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	const testKey = "test-key"
	key := testKey
	const warmedValue = "warmed-value"
	value := warmedValue
	customTTL := 30 * time.Minute

	// Set with custom TTL
	err = cache.Set(key, value, customTTL)
	if err != nil {
		t.Fatalf("Set (warmup with TTL) failed: %v", err)
	}

	// Verify the value is cached
	retrievedValue, found := cache.Get(key)
	if !found {
		t.Fatal("Expected key to be found after put")
	}
	if retrievedValue != value {
		t.Fatalf("Expected %v, got %v", value, retrievedValue)
	}
}

func TestCacheTTL(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	const testKey = "test-key"
	key := testKey
	shortTTL := 10 * time.Millisecond

	_ = cache.Set(key, "value", shortTTL) // Test setup

	// Should exist immediately
	_, found := cache.Get(key)
	if !found {
		t.Fatal("Expected key to exist immediately")
	}

	// Wait for expiration
	time.Sleep(shortTTL + 5*time.Millisecond)

	// Should be expired
	_, found = cache.Get(key)
	if found {
		t.Fatal("Expected key to be expired")
	}
}

func TestCacheEviction(t *testing.T) {
	// Create cache with small capacity
	config := NewDefaultConfig().WithMaxEntries(2)
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add entries to fill cache
	_ = cache.Set("key1", "value1", time.Hour)
	_ = cache.Set("key2", "value2", time.Hour)

	// Add third entry to trigger eviction
	_ = cache.Set("key3", "value3", time.Hour)

	// key1 should be evicted (LRU)
	_, found := cache.Get("key1")
	if found {
		t.Fatal("Expected key1 to be evicted")
	}

	// key2 and key3 should exist
	_, found = cache.Get("key2")
	if !found {
		t.Fatal("Expected key2 to exist")
	}
	_, found = cache.Get("key3")
	if !found {
		t.Fatal("Expected key3 to exist")
	}

	stats := cache.Stats()
	if stats.Evictions() == 0 {
		t.Fatal("Expected at least one eviction")
	}
}

func TestCacheConcurrency(t *testing.T) {
	config := NewDefaultConfig().WithMaxEntries(1000)
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 100

	// Concurrent writes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				value := fmt.Sprintf("value-%d-%d", id, j)
				_ = cache.Set(key, value, time.Hour) // Test setup
			}
		}(i)
	}

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				cache.Get(key)
			}
		}(i)
	}

	wg.Wait()

	// Verify stats are consistent
	stats := cache.Stats()
	total := stats.Hits() + stats.Misses()
	if total == 0 {
		t.Fatal("Expected some cache operations")
	}
}

func TestCacheReset(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add some data and operations
	_ = cache.Set("key1", "value1", time.Hour)
	_ = cache.Set("key2", "value2", time.Hour)
	cache.Get("key1")
	cache.Get("nonexistent")
	_ = cache.Delete("key2") // Test operation

	stats := cache.Stats()
	if stats.Total() == 0 {
		t.Fatal("Expected some operations before reset")
	}

	// Reset stats
	stats.Reset()

	// Verify all stats are zero
	if stats.Hits() != 0 {
		t.Fatalf("Expected 0 hits after reset, got %d", stats.Hits())
	}
	if stats.Misses() != 0 {
		t.Fatalf("Expected 0 misses after reset, got %d", stats.Misses())
	}
	if stats.Evictions() != 0 {
		t.Fatalf("Expected 0 evictions after reset, got %d", stats.Evictions())
	}
	if stats.Invalidations() != 0 {
		t.Fatalf("Expected 0 invalidations after reset, got %d", stats.Invalidations())
	}
}

// TestError is a simple error type for testing
type TestError struct {
	msg string
}

func (e *TestError) Error() string {
	return e.msg
}
