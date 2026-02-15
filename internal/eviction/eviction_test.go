package eviction

import (
	"testing"

	"github.com/1mb-dev/obcache-go/v2/internal/entry"
)

// Helper function to create a test entry
func createTestEntry(value string) *entry.Entry {
	return entry.NewWithoutTTL(value)
}

func TestLRUStrategy(t *testing.T) {
	strategy := NewLRUStrategy(2)

	// Test basic operations
	t.Run("BasicOperations", func(t *testing.T) {
		// Add first entry
		evictKey, _, evicted := strategy.Add("key1", createTestEntry("value1"))
		if evicted {
			t.Errorf("Expected no eviction, but got eviction of key: %s", evictKey)
		}

		// Add second entry
		evictKey, _, evicted = strategy.Add("key2", createTestEntry("value2"))
		if evicted {
			t.Errorf("Expected no eviction, but got eviction of key: %s", evictKey)
		}

		if strategy.Len() != 2 {
			t.Errorf("Expected length 2, got %d", strategy.Len())
		}

		// Add third entry (should evict)
		_, _, evicted = strategy.Add("key3", createTestEntry("value3"))
		if !evicted {
			t.Error("Expected eviction when exceeding capacity")
		}

		if strategy.Len() != 2 {
			t.Errorf("Expected length 2 after eviction, got %d", strategy.Len())
		}
	})

	t.Run("GetAndContains", func(t *testing.T) {
		strategy.Clear()
		_, _, _ = strategy.Add("key1", createTestEntry("value1"))

		entry, found := strategy.Get("key1")
		if !found {
			t.Error("Expected to find key1")
		}
		if entry.Value != "value1" {
			t.Errorf("Expected value1, got %v", entry.Value)
		}

		if !strategy.Contains("key1") {
			t.Error("Expected Contains to return true for key1")
		}

		if strategy.Contains("nonexistent") {
			t.Error("Expected Contains to return false for nonexistent key")
		}
	})

	t.Run("RemoveAndClear", func(t *testing.T) {
		strategy.Clear()
		_, _, _ = strategy.Add("key1", createTestEntry("value1"))
		_, _, _ = strategy.Add("key2", createTestEntry("value2"))

		removed := strategy.Remove("key1")
		if !removed {
			t.Error("Expected Remove to return true")
		}

		if strategy.Len() != 1 {
			t.Errorf("Expected length 1 after removal, got %d", strategy.Len())
		}

		strategy.Clear()
		if strategy.Len() != 0 {
			t.Errorf("Expected length 0 after clear, got %d", strategy.Len())
		}
	})
}

func TestLFUStrategy(t *testing.T) {
	strategy := NewLFUStrategy(2)

	t.Run("FrequencyBasedEviction", func(t *testing.T) {
		// Add two entries
		_, _, _ = strategy.Add("key1", createTestEntry("value1"))
		_, _, _ = strategy.Add("key2", createTestEntry("value2"))

		// Access key1 multiple times to increase its frequency
		strategy.Get("key1")
		strategy.Get("key1")
		strategy.Get("key2") // key2 has frequency 2, key1 has frequency 3

		// Add third entry - should evict key2 (least frequent)
		_, _, evicted := strategy.Add("key3", createTestEntry("value3"))
		if !evicted {
			t.Error("Expected eviction when exceeding capacity")
		}

		// Verify key1 is still present and key2 was evicted
		if strategy.Contains("key2") {
			t.Error("Expected key2 to be evicted")
		}
		if !strategy.Contains("key1") {
			t.Error("Expected key1 to remain")
		}
		if !strategy.Contains("key3") {
			t.Error("Expected key3 to be present")
		}
	})

	t.Run("PeekDoesNotUpdateFrequency", func(t *testing.T) {
		strategy.Clear()
		_, _, _ = strategy.Add("key1", createTestEntry("value1"))

		// Peek should not affect frequency
		entry, found := strategy.Peek("key1")
		if !found {
			t.Error("Expected to find key1 with Peek")
		}
		if entry.Value != "value1" {
			t.Errorf("Expected value1, got %v", entry.Value)
		}

		// Add more entries to test that peek didn't change frequency
		_, _, _ = strategy.Add("key2", createTestEntry("value2"))
		strategy.Get("key2") // Make key2 more frequent than key1

		_, _, evicted := strategy.Add("key3", createTestEntry("value3"))
		if !evicted {
			t.Error("Expected eviction when exceeding capacity")
		}

		// key1 should be evicted since it has lower frequency (peek doesn't count)
		if strategy.Contains("key1") {
			t.Error("Expected key1 to be evicted due to low frequency")
		}
	})
}

func TestFIFOStrategy(t *testing.T) {
	strategy := NewFIFOStrategy(2)

	t.Run("FIFOEviction", func(t *testing.T) {
		// Add two entries
		_, _, _ = strategy.Add("key1", createTestEntry("value1"))
		_, _, _ = strategy.Add("key2", createTestEntry("value2"))

		// Access key1 (should not change eviction order in FIFO)
		strategy.Get("key1")

		// Add third entry - should evict key1 (first in)
		_, _, evicted := strategy.Add("key3", createTestEntry("value3"))
		if !evicted {
			t.Error("Expected eviction when exceeding capacity")
		}

		// Verify key1 was evicted (first in, first out)
		if strategy.Contains("key1") {
			t.Error("Expected key1 to be evicted (FIFO)")
		}
		if !strategy.Contains("key2") {
			t.Error("Expected key2 to remain")
		}
		if !strategy.Contains("key3") {
			t.Error("Expected key3 to be present")
		}
	})

	t.Run("InsertionOrder", func(t *testing.T) {
		strategy.Clear()
		_, _, _ = strategy.Add("third", createTestEntry("3"))
		_, _, _ = strategy.Add("first", createTestEntry("1"))
		_, _, _ = strategy.Add("second", createTestEntry("2"))

		keys := strategy.Keys()
		expectedOrder := []string{"first", "second"} // third was evicted
		if len(keys) != len(expectedOrder) {
			t.Errorf("Expected %d keys, got %d", len(expectedOrder), len(keys))
		}

		for i, key := range keys {
			if key != expectedOrder[i] {
				t.Errorf("Expected key %s at position %d, got %s", expectedOrder[i], i, key)
			}
		}
	})
}

func TestStrategyFactory(t *testing.T) {
	testCases := []struct {
		name         string
		evictionType EvictionType
		capacity     int
	}{
		{"LRU", LRU, 10},
		{"LFU", LFU, 10},
		{"FIFO", FIFO, 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := Config{
				Type:     tc.evictionType,
				Capacity: tc.capacity,
			}

			strategy := NewStrategy(config)
			if strategy == nil {
				t.Fatalf("Expected strategy to be created, got nil")
			}

			if strategy.Capacity() != tc.capacity {
				t.Errorf("Expected capacity %d, got %d", tc.capacity, strategy.Capacity())
			}

			// Test basic operations
			evictKey, _, evicted := strategy.Add("test", createTestEntry("value"))
			if evicted {
				t.Errorf("Expected no eviction for first entry, got: %s", evictKey)
			}

			entry, found := strategy.Get("test")
			if !found {
				t.Error("Expected to find test key")
			}
			if entry.Value != "value" {
				t.Errorf("Expected value 'value', got %v", entry.Value)
			}
		})
	}
}

func TestDefaultStrategy(t *testing.T) {
	// Test that unknown eviction type defaults to LRU
	config := Config{
		Type:     "unknown",
		Capacity: 5,
	}

	strategy := NewStrategy(config)
	if strategy == nil {
		t.Fatal("Expected default strategy to be created")
	}

	// Should behave like LRU (this is a basic check)
	_, _, _ = strategy.Add("key1", createTestEntry("value1"))
	if !strategy.Contains("key1") {
		t.Error("Expected default strategy to work like LRU")
	}
}

func TestCapacityLimits(t *testing.T) {
	testCases := []struct {
		name     string
		strategy Strategy
	}{
		{"LRU", NewLRUStrategy(1)},
		{"LFU", NewLFUStrategy(1)},
		{"FIFO", NewFIFOStrategy(1)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Add first entry
			evictKey, _, evicted := tc.strategy.Add("key1", createTestEntry("value1"))
			if evicted {
				t.Errorf("Expected no eviction for first entry, got: %s", evictKey)
			}

			// Add second entry - should evict first
			_, _, evicted = tc.strategy.Add("key2", createTestEntry("value2"))
			if !evicted {
				t.Error("Expected eviction when capacity is 1")
			}

			if tc.strategy.Len() != 1 {
				t.Errorf("Expected length 1, got %d", tc.strategy.Len())
			}

			if !tc.strategy.Contains("key2") {
				t.Error("Expected key2 to be present")
			}
		})
	}
}
