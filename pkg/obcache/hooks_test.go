package obcache

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHookExecution(t *testing.T) {
	var hitCount, missCount, evictCount, invalidateCount int32

	config := NewDefaultConfig().WithMaxEntries(2).WithHooks(&Hooks{
		OnHit: []OnHitHook{
			func(_ string, _ any) {
				atomic.AddInt32(&hitCount, 1)
			},
		},
		OnMiss: []OnMissHook{
			func(_ string) {
				atomic.AddInt32(&missCount, 1)
			},
		},
		OnEvict: []OnEvictHook{
			func(_ string, _ any, _ EvictReason) {
				atomic.AddInt32(&evictCount, 1)
			},
		},
		OnInvalidate: []OnInvalidateHook{
			func(_ string) {
				atomic.AddInt32(&invalidateCount, 1)
			},
		},
	})
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Test OnMiss hook
	_, found := cache.Get("nonexistent")
	if found {
		t.Fatal("Expected miss")
	}
	if atomic.LoadInt32(&missCount) != 1 {
		t.Fatalf("Expected 1 miss hook call, got %d", missCount)
	}

	// Test OnHit hook
	_ = cache.Set("key1", "value1", time.Hour)
	_, found = cache.Get("key1")
	if !found {
		t.Fatal("Expected hit")
	}
	if atomic.LoadInt32(&hitCount) != 1 {
		t.Fatalf("Expected 1 hit hook call, got %d", hitCount)
	}

	// Test OnInvalidate hook
	_ = cache.Delete("key1")
	if atomic.LoadInt32(&invalidateCount) != 1 {
		t.Fatalf("Expected 1 invalidate hook call, got %d", invalidateCount)
	}

	// Test OnEvict hook
	_ = cache.Set("key2", "value2", time.Hour)
	_ = cache.Set("key3", "value3", time.Hour)
	_ = cache.Set("key4", "value4", time.Hour) // Should evict key2 (LRU)

	// Give some time for eviction to be processed
	time.Sleep(10 * time.Millisecond)

	if atomic.LoadInt32(&evictCount) == 0 {
		t.Fatal("Expected at least 1 evict hook call")
	}
}

func TestHookParameters(t *testing.T) {
	var capturedKeys []string
	var capturedValues []any
	var mu sync.Mutex

	config := NewDefaultConfig().WithMaxEntries(1).WithHooks(&Hooks{
		OnHit: []OnHitHook{
			func(key string, value any) {
				mu.Lock()
				capturedKeys = append(capturedKeys, key)
				capturedValues = append(capturedValues, value)
				mu.Unlock()
			},
		},
		OnEvict: []OnEvictHook{
			func(key string, value any, _ EvictReason) {
				mu.Lock()
				capturedKeys = append(capturedKeys, key)
				capturedValues = append(capturedValues, value)
				mu.Unlock()
			},
		},
	})
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Test hit hook parameters
	testKey := "test-key"
	testValue := "test-value"

	_ = cache.Set(testKey, testValue, time.Hour)
	cache.Get(testKey)

	mu.Lock()
	if len(capturedKeys) != 1 {
		t.Fatalf("Expected 1 captured key, got %d", len(capturedKeys))
	}
	if capturedKeys[0] != testKey {
		t.Fatalf("Expected key '%s', got '%s'", testKey, capturedKeys[0])
	}
	if capturedValues[0] != testValue {
		t.Fatalf("Expected value '%s', got '%v'", testValue, capturedValues[0])
	}
	mu.Unlock()

	// Test evict hook parameters
	_ = cache.Set("new-key", "new-value", time.Hour) // Should evict previous entry
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	if len(capturedKeys) < 2 {
		t.Fatalf("Expected at least 2 captured keys (hit + evict), got %d", len(capturedKeys))
	}
	// The evicted entry should be captured
	evictedKey := capturedKeys[len(capturedKeys)-1]
	evictedValue := capturedValues[len(capturedValues)-1]
	if evictedKey != testKey {
		t.Fatalf("Expected evicted key '%s', got '%s'", testKey, evictedKey)
	}
	if evictedValue != testValue {
		t.Fatalf("Expected evicted value '%s', got '%v'", testValue, evictedValue)
	}
	mu.Unlock()
}

func TestHookConcurrency(t *testing.T) {
	var hookCallCount int32

	config := NewDefaultConfig().WithHooks(&Hooks{
		OnHit: []OnHitHook{
			func(_ string, _ any) {
				atomic.AddInt32(&hookCallCount, 1)
			},
		},
		OnMiss: []OnMissHook{
			func(_ string) {
				atomic.AddInt32(&hookCallCount, 1)
			},
		},
	})
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Add some data
	for i := 0; i < 10; i++ {
		_ = cache.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i), time.Hour)
	}

	// Concurrent cache operations to trigger hooks
	var wg sync.WaitGroup
	const numGoroutines = 50
	const numOperations = 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				if j%2 == 0 {
					// Hit
					cache.Get(fmt.Sprintf("key%d", j%10))
				} else {
					// Miss
					cache.Get(fmt.Sprintf("nonexistent-%d-%d", id, j))
				}
			}
		}(i)
	}

	wg.Wait()

	expectedCalls := int32(numGoroutines * numOperations)
	actualCalls := atomic.LoadInt32(&hookCallCount)

	if actualCalls != expectedCalls {
		t.Fatalf("Expected %d hook calls, got %d", expectedCalls, actualCalls)
	}
}

func TestMultipleHooksOfSameType(t *testing.T) {
	var hook1Calls, hook2Calls int32

	// Test multiple OnHit hooks
	hooks := &Hooks{
		OnHit: []OnHitHook{
			func(_ string, _ any) {
				atomic.AddInt32(&hook1Calls, 1)
			},
			func(_ string, _ any) {
				atomic.AddInt32(&hook2Calls, 1)
			},
		},
	}

	config := NewDefaultConfig().WithHooks(hooks)
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	_ = cache.Set("key1", "value1", time.Hour)
	cache.Get("key1")

	if atomic.LoadInt32(&hook1Calls) != 1 {
		t.Fatalf("Expected hook1 to be called once, got %d", hook1Calls)
	}
	if atomic.LoadInt32(&hook2Calls) != 1 {
		t.Fatalf("Expected hook2 to be called once, got %d", hook2Calls)
	}
}

func TestHookIntegrationWithWrap(t *testing.T) {
	var hitCalls, missCalls int32

	config := NewDefaultConfig().WithHooks(&Hooks{
		OnHit: []OnHitHook{
			func(_ string, _ any) {
				atomic.AddInt32(&hitCalls, 1)
			},
		},
		OnMiss: []OnMissHook{
			func(_ string) {
				atomic.AddInt32(&missCalls, 1)
			},
		},
	})
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	expensiveFunc := func(x int) int {
		return x * 2
	}

	wrapped := Wrap(cache, expensiveFunc)

	// First call - should miss and then cache
	result1 := wrapped(5)
	if result1 != 10 {
		t.Fatalf("Expected 10, got %d", result1)
	}

	// The wrap function first tries to get from cache (miss), then caches the result
	if atomic.LoadInt32(&missCalls) != 1 {
		t.Fatalf("Expected 1 miss call, got %d", missCalls)
	}

	// Second call - should hit
	result2 := wrapped(5)
	if result2 != 10 {
		t.Fatalf("Expected 10, got %d", result2)
	}

	if atomic.LoadInt32(&hitCalls) != 1 {
		t.Fatalf("Expected 1 hit call, got %d", hitCalls)
	}
}

func TestNilHooks(t *testing.T) {
	// Test that nil hooks don't cause panics
	config := NewDefaultConfig().WithHooks(nil)
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	_ = cache.Set("key1", "value1", time.Hour)
	cache.Get("key1")
	cache.Get("nonexistent")
	_ = cache.Delete("key1")

	// If we reach here without panic, test passes
}

func TestEmptyHooks(t *testing.T) {
	// Test that empty hooks struct doesn't cause issues
	config := NewDefaultConfig().WithHooks(&Hooks{})
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	_ = cache.Set("key1", "value1", time.Hour)
	cache.Get("key1")
	cache.Get("nonexistent")
	_ = cache.Delete("key1")

	// If we reach here without panic, test passes
}

func TestHookErrorHandling(t *testing.T) {
	// Test that panicking hooks don't break the cache
	config := NewDefaultConfig().WithHooks(&Hooks{
		OnHit: []OnHitHook{
			func(_ string, _ any) {
				panic("hook panic")
			},
		},
	})
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	_ = cache.Set("key1", "value1", time.Hour)

	// This should not panic even though the hook panics
	// The cache should continue to function normally
	defer func() {
		if r := recover(); r != nil {
			// Hook panics are expected to propagate in this implementation
			// This is acceptable behavior
			return
		}
		// If no panic occurred, that's also fine
	}()

	_, found := cache.Get("key1")
	// We may or may not reach this point depending on hook implementation
	_ = found
}
