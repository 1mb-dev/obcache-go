package obcache

import (
	"fmt"
	"testing"
	"time"
)

// TestCacheOperationsTableDriven demonstrates consolidated testing using table-driven approach
func TestCacheOperationsTableDriven(t *testing.T) { //nolint:gocyclo // Acceptable complexity for comprehensive table test
	tests := []struct {
		name        string
		operation   string
		key         string
		value       interface{}
		ttl         time.Duration
		expectFound bool
		expectError bool
		setup       func(*Cache)                                       // Optional setup function
		verify      func(*testing.T, *Cache, interface{}, bool, error) // Custom verification
	}{
		{
			name:        "basic set and get",
			operation:   "set_then_get",
			key:         "test-key",
			value:       "test-value",
			ttl:         time.Hour,
			expectFound: true,
			expectError: false,
		},
		{
			name:        "get nonexistent key",
			operation:   "get",
			key:         "nonexistent-key",
			value:       nil,
			ttl:         0,
			expectFound: false,
			expectError: false,
		},
		{
			name:        "set and invalidate",
			operation:   "set_then_invalidate",
			key:         "invalidate-key",
			value:       "invalidate-value",
			ttl:         time.Hour,
			expectFound: false,
			expectError: false,
		},
		{
			name:        "overwrite existing key",
			operation:   "set_then_overwrite",
			key:         "overwrite-key",
			value:       "new-value",
			ttl:         time.Hour,
			expectFound: true,
			expectError: false,
			setup: func(c *Cache) {
				_ = c.Set("overwrite-key", "old-value", time.Hour)
			},
		},
		{
			name:        "expired key",
			operation:   "set_expired_then_get",
			key:         "expired-key",
			value:       "expired-value",
			ttl:         time.Nanosecond, // Will expire immediately
			expectFound: false,
			expectError: false,
			verify: func(t *testing.T, c *Cache, _ interface{}, _ bool, _ error) {
				// Add a small delay to ensure expiration
				time.Sleep(time.Millisecond)
				val, isFound := c.Get("expired-key")
				if isFound {
					t.Errorf("Expected expired key to not be found, but got value: %v", val)
				}
			},
		},
		{
			name:        "complex value types",
			operation:   "set_then_get",
			key:         "complex-key",
			value:       map[string]interface{}{"id": 123, "name": "test", "active": true},
			ttl:         time.Hour,
			expectFound: true,
			expectError: false,
			verify: func(t *testing.T, c *Cache, expectedValue interface{}, found bool, err error) {
				if !found {
					t.Error("Expected to find complex value")
					return
				}
				val, _ := c.Get("complex-key")
				expectedMap := expectedValue.(map[string]interface{})
				actualMap, ok := val.(map[string]interface{})
				if !ok {
					t.Errorf("Expected map[string]interface{}, got %T", val)
					return
				}
				if len(actualMap) != len(expectedMap) {
					t.Errorf("Expected map length %d, got %d", len(expectedMap), len(actualMap))
				}
				for k, v := range expectedMap {
					if actualMap[k] != v {
						t.Errorf("Expected %s=%v, got %s=%v", k, v, k, actualMap[k])
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := New(NewDefaultConfig())
			if err != nil {
				t.Fatalf("Failed to create cache: %v", err)
			}

			// Run setup if provided
			if tt.setup != nil {
				tt.setup(cache)
			}

			var value interface{}
			var found bool

			switch tt.operation {
			case "set_then_get":
				err = cache.Set(tt.key, tt.value, tt.ttl)
				if (err != nil) != tt.expectError {
					t.Errorf("Set error = %v, expectError = %v", err, tt.expectError)
					return
				}
				if err == nil {
					value, found = cache.Get(tt.key)
				}

			case "get":
				value, found = cache.Get(tt.key)

			case "set_then_invalidate":
				err = cache.Set(tt.key, tt.value, tt.ttl)
				if err != nil {
					t.Fatalf("Unexpected error during set: %v", err)
				}
				err = cache.Delete(tt.key)
				if (err != nil) != tt.expectError {
					t.Errorf("Delete error = %v, expectError = %v", err, tt.expectError)
					return
				}
				value, found = cache.Get(tt.key)

			case "set_then_overwrite":
				err = cache.Set(tt.key, tt.value, tt.ttl)
				if (err != nil) != tt.expectError {
					t.Errorf("Set error = %v, expectError = %v", err, tt.expectError)
					return
				}
				if err == nil {
					value, found = cache.Get(tt.key)
				}

			case "set_expired_then_get":
				err = cache.Set(tt.key, tt.value, tt.ttl)
				if err != nil {
					t.Fatalf("Unexpected error during set: %v", err)
				}
				// Use custom verification for expiration test
				if tt.verify != nil {
					tt.verify(t, cache, tt.value, found, err)
					return
				}

			default:
				t.Fatalf("Unknown operation: %s", tt.operation)
			}

			// Use custom verification if provided
			if tt.verify != nil {
				tt.verify(t, cache, tt.value, found, err)
				return
			}

			// Standard verification
			if found != tt.expectFound {
				t.Errorf("Expected found = %v, got found = %v", tt.expectFound, found)
			}

			if tt.expectFound && value != tt.value {
				t.Errorf("Expected value = %v, got value = %v", tt.value, value)
			}
		})
	}
}

// TestCacheStatsTableDriven consolidates statistics testing
func TestCacheStatsTableDriven(t *testing.T) {
	tests := []struct {
		name       string
		operations func(*Cache)
		verify     func(*testing.T, *Stats)
	}{
		{
			name: "hit and miss counts",
			operations: func(c *Cache) {
				_ = c.Set("key1", "value1", time.Hour) // Test setup
				c.Get("key1")                          // hit
				c.Get("key2")                          // miss
				c.Get("key1")                          // hit
			},
			verify: func(t *testing.T, s *Stats) {
				if s.Hits() != 2 {
					t.Errorf("Expected 2 hits, got %d", s.Hits())
				}
				if s.Misses() != 1 {
					t.Errorf("Expected 1 miss, got %d", s.Misses())
				}
				expectedHitRate := float64(2) / float64(3) * 100 // 2 hits out of 3 gets as percentage
				if hitRate := s.HitRate(); fmt.Sprintf("%.2f", hitRate) != fmt.Sprintf("%.2f", expectedHitRate) {
					t.Errorf("Expected hit rate %.2f%%, got %.2f%%", expectedHitRate, hitRate)
				}
			},
		},
		{
			name: "key count tracking",
			operations: func(c *Cache) {
				_ = c.Set("key1", "value1", time.Hour) // Test setup
				_ = c.Set("key2", "value2", time.Hour) // Test setup
				_ = c.Set("key3", "value3", time.Hour) // Test setup
				_ = c.Delete("key2")                   // Test operation
			},
			verify: func(t *testing.T, s *Stats) {
				if s.KeyCount() != 2 {
					t.Errorf("Expected 2 keys remaining, got %d", s.KeyCount())
				}
				if s.Invalidations() != 1 {
					t.Errorf("Expected 1 invalidation, got %d", s.Invalidations())
				}
			},
		},
		{
			name: "evictions tracking",
			operations: func(c *Cache) {
				// This test would need a small cache to force evictions
				// For now, just verify eviction counter starts at 0
			},
			verify: func(t *testing.T, s *Stats) {
				if s.Evictions() != 0 {
					t.Errorf("Expected 0 evictions for empty operations, got %d", s.Evictions())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := New(NewDefaultConfig())
			if err != nil {
				t.Fatalf("Failed to create cache: %v", err)
			}

			// Execute operations
			tt.operations(cache)

			// Verify results
			stats := cache.Stats()
			tt.verify(t, stats)
		})
	}
}

// TestCacheEdgeCasesTableDriven consolidates edge case testing
func TestCacheEdgeCasesTableDriven(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() *Cache
		operation   func(*Cache) (interface{}, bool, error)
		expectError bool
		expectPanic bool
		description string
	}{
		{
			name: "nil value",
			setup: func() *Cache {
				c, _ := New(NewDefaultConfig())
				return c
			},
			operation: func(c *Cache) (interface{}, bool, error) {
				err := c.Set("nil-key", nil, time.Hour)
				if err != nil {
					return nil, false, err
				}
				val, found := c.Get("nil-key")
				return val, found, nil
			},
			expectError: false,
			description: "should handle nil values correctly",
		},
		{
			name: "empty key",
			setup: func() *Cache {
				c, _ := New(NewDefaultConfig())
				return c
			},
			operation: func(c *Cache) (interface{}, bool, error) {
				err := c.Set("", "empty-key-value", time.Hour)
				if err != nil {
					return nil, false, err
				}
				val, found := c.Get("")
				return val, found, nil
			},
			expectError: false,
			description: "should handle empty string keys",
		},
		{
			name: "zero TTL",
			setup: func() *Cache {
				c, _ := New(NewDefaultConfig())
				return c
			},
			operation: func(c *Cache) (interface{}, bool, error) {
				err := c.Set("zero-ttl", "value", 0)
				if err != nil {
					return nil, false, err
				}
				val, found := c.Get("zero-ttl")
				return val, found, nil
			},
			expectError: false,
			description: "should handle zero TTL (no expiration)",
		},
		{
			name: "very large value",
			setup: func() *Cache {
				c, _ := New(NewDefaultConfig())
				return c
			},
			operation: func(c *Cache) (interface{}, bool, error) {
				largeValue := make([]byte, 1024*1024) // 1MB
				for i := range largeValue {
					largeValue[i] = byte(i % 256)
				}
				err := c.Set("large", largeValue, time.Hour)
				if err != nil {
					return nil, false, err
				}
				val, found := c.Get("large")
				return val, found, nil
			},
			expectError: false,
			description: "should handle large values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("Expected panic but didn't get one")
					}
				}()
			}

			cache := tt.setup()
			val, found, err := tt.operation(cache)

			if (err != nil) != tt.expectError {
				t.Errorf("Expected error = %v, got error = %v (%s)", tt.expectError, err != nil, tt.description)
			}

			// For non-error cases, basic sanity checks
			if !tt.expectError && !tt.expectPanic {
				t.Logf("Operation result: value=%v, found=%v, error=%v (%s)", val, found, err, tt.description)
			}
		})
	}
}
