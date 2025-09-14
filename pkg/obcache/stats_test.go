package obcache

import (
	"sync"
	"testing"
	"time"
)

func TestStatsInitialState(t *testing.T) {
	stats := &Stats{}

	if hits := stats.Hits(); hits != 0 {
		t.Fatalf("Expected 0 initial hits, got %d", hits)
	}
	if misses := stats.Misses(); misses != 0 {
		t.Fatalf("Expected 0 initial misses, got %d", misses)
	}
	if evictions := stats.Evictions(); evictions != 0 {
		t.Fatalf("Expected 0 initial evictions, got %d", evictions)
	}
	if invalidations := stats.Invalidations(); invalidations != 0 {
		t.Fatalf("Expected 0 initial invalidations, got %d", invalidations)
	}
	if keyCount := stats.KeyCount(); keyCount != 0 {
		t.Fatalf("Expected 0 initial key count, got %d", keyCount)
	}
	if inFlight := stats.InFlight(); inFlight != 0 {
		t.Fatalf("Expected 0 initial in-flight, got %d", inFlight)
	}
	if total := stats.Total(); total != 0 {
		t.Fatalf("Expected 0 initial total, got %d", total)
	}
	if hitRate := stats.HitRate(); hitRate != 0 {
		t.Fatalf("Expected 0 initial hit rate, got %f", hitRate)
	}
}

func TestStatsIncrement(t *testing.T) {
	stats := &Stats{}

	// Test hit increment
	stats.incHits()
	if hits := stats.Hits(); hits != 1 {
		t.Fatalf("Expected 1 hit after increment, got %d", hits)
	}

	// Test miss increment
	stats.incMisses()
	if misses := stats.Misses(); misses != 1 {
		t.Fatalf("Expected 1 miss after increment, got %d", misses)
	}

	// Test eviction increment
	stats.incEvictions()
	if evictions := stats.Evictions(); evictions != 1 {
		t.Fatalf("Expected 1 eviction after increment, got %d", evictions)
	}

	// Test invalidation increment
	stats.incInvalidations()
	if invalidations := stats.Invalidations(); invalidations != 1 {
		t.Fatalf("Expected 1 invalidation after increment, got %d", invalidations)
	}

	// Test in-flight increment and decrement
	stats.incInFlight()
	if inFlight := stats.InFlight(); inFlight != 1 {
		t.Fatalf("Expected 1 in-flight after increment, got %d", inFlight)
	}

	stats.decInFlight()
	if inFlight := stats.InFlight(); inFlight != 0 {
		t.Fatalf("Expected 0 in-flight after decrement, got %d", inFlight)
	}

	// Test key count set
	stats.setKeyCount(5)
	if keyCount := stats.KeyCount(); keyCount != 5 {
		t.Fatalf("Expected 5 key count after set, got %d", keyCount)
	}
}

func TestStatsCalculations(t *testing.T) {
	stats := &Stats{}

	// Add some hits and misses
	stats.incHits()
	stats.incHits()
	stats.incHits()
	stats.incMisses()

	// Test total
	if total := stats.Total(); total != 4 {
		t.Fatalf("Expected 4 total requests, got %d", total)
	}

	// Test hit rate (3 hits out of 4 total = 75%)
	expectedHitRate := 75.0
	if hitRate := stats.HitRate(); hitRate != expectedHitRate {
		t.Fatalf("Expected hit rate %.1f, got %.1f", expectedHitRate, hitRate)
	}
}

func TestStatsHitRateEdgeCases(t *testing.T) {
	stats := &Stats{}

	// Test hit rate with no requests
	if hitRate := stats.HitRate(); hitRate != 0 {
		t.Fatalf("Expected 0 hit rate with no requests, got %f", hitRate)
	}

	// Test hit rate with only hits
	stats.incHits()
	stats.incHits()
	if hitRate := stats.HitRate(); hitRate != 100.0 {
		t.Fatalf("Expected 100%% hit rate with only hits, got %f", hitRate)
	}

	// Test hit rate with only misses
	stats2 := &Stats{}
	stats2.incMisses()
	stats2.incMisses()
	if hitRate := stats2.HitRate(); hitRate != 0.0 {
		t.Fatalf("Expected 0%% hit rate with only misses, got %f", hitRate)
	}
}

func TestStatsReset(t *testing.T) {
	stats := &Stats{}

	// Add some data
	stats.incHits()
	stats.incMisses()
	stats.incEvictions()
	stats.incInvalidations()
	stats.incInFlight()
	stats.setKeyCount(10)

	// Verify data exists
	if total := stats.Total(); total == 0 {
		t.Fatal("Expected some data before reset")
	}

	// Reset
	stats.Reset()

	// Verify all values are zero
	if hits := stats.Hits(); hits != 0 {
		t.Fatalf("Expected 0 hits after reset, got %d", hits)
	}
	if misses := stats.Misses(); misses != 0 {
		t.Fatalf("Expected 0 misses after reset, got %d", misses)
	}
	if evictions := stats.Evictions(); evictions != 0 {
		t.Fatalf("Expected 0 evictions after reset, got %d", evictions)
	}
	if invalidations := stats.Invalidations(); invalidations != 0 {
		t.Fatalf("Expected 0 invalidations after reset, got %d", invalidations)
	}
	if keyCount := stats.KeyCount(); keyCount != 0 {
		t.Fatalf("Expected 0 key count after reset, got %d", keyCount)
	}
	if inFlight := stats.InFlight(); inFlight != 0 {
		t.Fatalf("Expected 0 in-flight after reset, got %d", inFlight)
	}
}

func TestStatsConcurrency(t *testing.T) {
	stats := &Stats{}

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 1000

	// Concurrent increments
	wg.Add(numGoroutines * 4) // 4 types of operations

	// Hits
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				stats.incHits()
			}
		}()
	}

	// Misses
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				stats.incMisses()
			}
		}()
	}

	// Evictions
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				stats.incEvictions()
			}
		}()
	}

	// Invalidations
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				stats.incInvalidations()
			}
		}()
	}

	wg.Wait()

	// Verify final counts
	expectedCount := int64(numGoroutines * numOperations)
	if hits := stats.Hits(); hits != expectedCount {
		t.Fatalf("Expected %d hits, got %d", expectedCount, hits)
	}
	if misses := stats.Misses(); misses != expectedCount {
		t.Fatalf("Expected %d misses, got %d", expectedCount, misses)
	}
	if evictions := stats.Evictions(); evictions != expectedCount {
		t.Fatalf("Expected %d evictions, got %d", expectedCount, evictions)
	}
	if invalidations := stats.Invalidations(); invalidations != expectedCount {
		t.Fatalf("Expected %d invalidations, got %d", expectedCount, invalidations)
	}
}

func TestStatsInFlightConcurrency(t *testing.T) {
	stats := &Stats{}

	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrent increment/decrement of in-flight counter
	wg.Add(numGoroutines * 2)

	// Increment
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			stats.incInFlight()
		}()
	}

	// Decrement
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			stats.decInFlight()
		}()
	}

	wg.Wait()

	// Should balance out to zero
	if inFlight := stats.InFlight(); inFlight != 0 {
		t.Fatalf("Expected 0 in-flight after balanced inc/dec, got %d", inFlight)
	}
}

func TestStatsIntegrationWithCache(t *testing.T) {
	config := NewDefaultConfig().WithMaxEntries(2)
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Test various operations and verify stats

	// Miss
	_, found := cache.Get("key1")
	if found {
		t.Fatal("Expected miss")
	}
	stats := cache.Stats()
	if stats.Misses() != 1 {
		t.Fatalf("Expected 1 miss, got %d", stats.Misses())
	}

	// Set and Hit
	_ = cache.Set("key1", "value1", time.Hour)
	_, found = cache.Get("key1")
	if !found {
		t.Fatal("Expected hit")
	}
	stats = cache.Stats()
	if stats.Hits() != 1 {
		t.Fatalf("Expected 1 hit, got %d", stats.Hits())
	}
	if stats.KeyCount() != 1 {
		t.Fatalf("Expected 1 key, got %d", stats.KeyCount())
	}

	// Delete
	_ = cache.Delete("key1")
	stats = cache.Stats()
	if stats.Invalidations() != 1 {
		t.Fatalf("Expected 1 invalidation, got %d", stats.Invalidations())
	}

	// Test eviction by filling cache
	_ = cache.Set("key1", "value1", time.Hour)
	_ = cache.Set("key2", "value2", time.Hour)
	_ = cache.Set("key3", "value3", time.Hour) // Should evict key1

	stats = cache.Stats()
	if stats.Evictions() == 0 {
		t.Fatal("Expected at least 1 eviction")
	}
}
