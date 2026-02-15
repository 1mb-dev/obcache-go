package obcache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/1mb-dev/obcache-go/internal/eviction"
)

func TestEvictionStrategies(t *testing.T) {
	testCases := []struct {
		name         string
		config       *Config
		evictionType string
	}{
		{
			name:         "LRU_Default",
			config:       NewDefaultConfig().WithMaxEntries(2),
			evictionType: "lru",
		},
		{
			name:         "LRU_Explicit",
			config:       NewDefaultConfig().WithMaxEntries(2).WithEvictionType(eviction.LRU),
			evictionType: "lru",
		},
		{
			name:         "LFU",
			config:       NewDefaultConfig().WithMaxEntries(2).WithEvictionType(eviction.LFU),
			evictionType: "lfu",
		},
		{
			name:         "FIFO",
			config:       NewDefaultConfig().WithMaxEntries(2).WithEvictionType(eviction.FIFO),
			evictionType: "fifo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cache, err := New(tc.config)
			if err != nil {
				t.Fatalf("Failed to create cache: %v", err)
			}
			defer func() { _ = cache.Close() }()

			testEvictionBehavior(t, cache, tc.evictionType)
		})
	}
}

func testEvictionBehavior(t *testing.T, cache *Cache, evictionType string) {
	// Add first two entries
	err1 := cache.Set("key1", "value1", time.Hour)
	err2 := cache.Set("key2", "value2", time.Hour)
	if err1 != nil || err2 != nil {
		t.Fatalf("Failed to set initial entries: %v, %v", err1, err2)
	}

	// Verify both entries exist
	_, found1 := cache.Get("key1")
	_, found2 := cache.Get("key2")
	if !found1 || !found2 {
		t.Fatal("Expected both entries to be found after initial set")
	}

	switch evictionType {
	case "lru":
		testLRUBehavior(t, cache)
	case "lfu":
		testLFUBehavior(t, cache)
	case "fifo":
		testFIFOBehavior(t, cache)
	}
}

func testLRUBehavior(t *testing.T, cache *Cache) {
	// Access key1 to make it more recently used
	cache.Get("key1")

	// Add third entry - should evict key2 (least recently used)
	err := cache.Set("key3", "value3", time.Hour)
	if err != nil {
		t.Fatalf("Failed to set third entry: %v", err)
	}

	// key2 should be evicted, key1 and key3 should remain
	_, found1 := cache.Get("key1")
	_, found2 := cache.Get("key2")
	_, found3 := cache.Get("key3")

	if !found1 {
		t.Error("Expected key1 to remain (most recently used)")
	}
	if found2 {
		t.Error("Expected key2 to be evicted (least recently used)")
	}
	if !found3 {
		t.Error("Expected key3 to be present")
	}
}

func testLFUBehavior(t *testing.T, cache *Cache) {
	// Access key1 multiple times to make it more frequently used
	cache.Get("key1")
	cache.Get("key1")
	cache.Get("key2") // key1 has frequency 3, key2 has frequency 2

	// Add third entry - should evict key2 (least frequently used)
	err := cache.Set("key3", "value3", time.Hour)
	if err != nil {
		t.Fatalf("Failed to set third entry: %v", err)
	}

	// key2 should be evicted, key1 and key3 should remain
	_, found1 := cache.Get("key1")
	_, found2 := cache.Get("key2")
	_, found3 := cache.Get("key3")

	if !found1 {
		t.Error("Expected key1 to remain (most frequently used)")
	}
	if found2 {
		t.Error("Expected key2 to be evicted (least frequently used)")
	}
	if !found3 {
		t.Error("Expected key3 to be present")
	}
}

func testFIFOBehavior(t *testing.T, cache *Cache) {
	// Access key1 (should not affect FIFO ordering)
	cache.Get("key1")

	// Add third entry - should evict key1 (first in, first out)
	err := cache.Set("key3", "value3", time.Hour)
	if err != nil {
		t.Fatalf("Failed to set third entry: %v", err)
	}

	// key1 should be evicted, key2 and key3 should remain
	_, found1 := cache.Get("key1")
	_, found2 := cache.Get("key2")
	_, found3 := cache.Get("key3")

	if found1 {
		t.Error("Expected key1 to be evicted (first in, first out)")
	}
	if !found2 {
		t.Error("Expected key2 to remain")
	}
	if !found3 {
		t.Error("Expected key3 to be present")
	}
}

func TestEvictionWithWrappedFunctions(t *testing.T) {
	config := NewDefaultConfig().WithMaxEntries(2).WithEvictionType(eviction.LFU)
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = cache.Close() }()

	callCount := 0
	expensiveFunc := func(x int) string {
		callCount++
		return fmt.Sprintf("result-%d", x)
	}

	wrapped := Wrap(cache, expensiveFunc)

	// Call function with different arguments
	result1 := wrapped(1)      // First call
	result2 := wrapped(2)      // Second call
	result1Again := wrapped(1) // Should hit cache, increase frequency for arg 1

	if callCount != 2 {
		t.Errorf("Expected 2 function calls, got %d", callCount)
	}

	if result1 != "result-1" || result2 != "result-2" || result1Again != "result-1" {
		t.Error("Unexpected results from wrapped function")
	}

	// Call with third argument - should evict arg 2 (less frequently used)
	result3 := wrapped(3)

	if callCount != 3 {
		t.Errorf("Expected 3 function calls after adding third arg, got %d", callCount)
	}

	// Call with arg 2 again - should trigger function call (was evicted)
	wrapped(2)

	if callCount != 4 {
		t.Errorf("Expected 4 function calls after re-calling evicted arg, got %d", callCount)
	}

	// Call with arg 1 again - should hit cache (was more frequently used)
	wrapped(1)

	if callCount != 4 {
		t.Errorf("Expected 4 function calls after re-calling frequent arg, got %d", callCount)
	}

	if result3 != "result-3" {
		t.Error("Unexpected result from third function call")
	}
}

func TestEvictionCallbacks(t *testing.T) {
	evictedKeys := make([]string, 0)
	evictedValues := make([]any, 0)

	hooks := NewHooks()
	hooks.AddOnEvict(func(_ context.Context, key string, value any, reason EvictReason) {
		t.Logf("Eviction callback called: key=%s, value=%v, reason=%v", key, value, reason)
		if reason == EvictReasonCapacity {
			evictedKeys = append(evictedKeys, key)
			evictedValues = append(evictedValues, value)
		}
	})

	config := NewDefaultConfig().
		WithMaxEntries(2).
		WithEvictionType(eviction.FIFO).
		WithHooks(hooks)

	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = cache.Close() }()

	t.Logf("Cache created with eviction type: %v", config.EvictionType)

	// Fill cache to capacity
	err1 := cache.Set("key1", "value1", time.Hour)
	err2 := cache.Set("key2", "value2", time.Hour)
	if err1 != nil || err2 != nil {
		t.Fatalf("Failed to set initial entries: %v, %v", err1, err2)
	}

	t.Logf("After setting key1 and key2, cache length: %d", cache.Len())

	// Add third entry to trigger eviction
	err3 := cache.Set("key3", "value3", time.Hour)
	if err3 != nil {
		t.Fatalf("Failed to set third entry: %v", err3)
	}

	t.Logf("After setting key3, cache length: %d", cache.Len())
	t.Logf("Evicted keys: %v", evictedKeys)

	// Verify eviction callback was called
	if len(evictedKeys) != 1 {
		t.Errorf("Expected 1 evicted key, got %d", len(evictedKeys))
	}

	if len(evictedKeys) > 0 && evictedKeys[0] != "key1" {
		t.Errorf("Expected key1 to be evicted (FIFO), got %s", evictedKeys[0])
	}

	if len(evictedValues) > 0 && evictedValues[0] != "value1" {
		t.Errorf("Expected evicted value1, got %v", evictedValues[0])
	}
}

func TestEvictionStrategiesWithCleanup(t *testing.T) {
	testCases := []struct {
		name     string
		strategy eviction.EvictionType
	}{
		{"LRU", eviction.LRU},
		{"LFU", eviction.LFU},
		{"FIFO", eviction.FIFO},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := NewDefaultConfig().
				WithMaxEntries(10).
				WithEvictionType(tc.strategy).
				WithCleanupInterval(10 * time.Millisecond)

			cache, err := New(config)
			if err != nil {
				t.Fatalf("Failed to create cache: %v", err)
			}
			defer func() { _ = cache.Close() }()

			// Add entry with short TTL
			err = cache.Set("shortlived", "value", 20*time.Millisecond)
			if err != nil {
				t.Fatalf("Failed to set entry: %v", err)
			}

			// Entry should exist initially
			_, found := cache.Get("shortlived")
			if !found {
				t.Error("Expected entry to be found initially")
			}

			// Wait for TTL to expire and cleanup to run
			time.Sleep(50 * time.Millisecond)

			// Entry should be cleaned up
			_, found = cache.Get("shortlived")
			if found {
				t.Error("Expected entry to be cleaned up after TTL expiry")
			}
		})
	}
}

func TestConfigurationMethods(t *testing.T) {
	config := NewDefaultConfig()

	// Test default
	if config.EvictionType != eviction.LRU {
		t.Errorf("Expected default eviction type to be LRU, got %s", config.EvictionType)
	}

	// Test WithEvictionType
	config = config.WithEvictionType(eviction.LFU)
	if config.EvictionType != eviction.LFU {
		t.Errorf("Expected eviction type to be LFU, got %s", config.EvictionType)
	}

	// Test convenience methods
	config = config.WithEvictionType(eviction.LRU)
	if config.EvictionType != eviction.LRU {
		t.Errorf("Expected eviction type to be LRU, got %s", config.EvictionType)
	}

	config = config.WithEvictionType(eviction.LFU)
	if config.EvictionType != eviction.LFU {
		t.Errorf("Expected eviction type to be LFU, got %s", config.EvictionType)
	}

	config = config.WithEvictionType(eviction.FIFO)
	if config.EvictionType != eviction.FIFO {
		t.Errorf("Expected eviction type to be FIFO, got %s", config.EvictionType)
	}
}
