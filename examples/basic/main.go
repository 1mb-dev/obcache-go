package main

import (
	"fmt"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

func main() {
	// Create a cache with default configuration
	cache, err := obcache.New(obcache.NewDefaultConfig())
	if err != nil {
		panic(err)
	}

	// Example 1: Basic cache usage
	fmt.Println("=== Basic Cache Usage ===")
	if err := cache.Set("key1", "value1", 5*time.Minute); err != nil {
		fmt.Printf("Error setting cache: %v\n", err)
		return
	}

	if value, found := cache.Get("key1"); found {
		fmt.Printf("Found: %v\n", value)
	}

	// Example 2: Function wrapping - simple function
	fmt.Println("\n=== Function Wrapping ===")

	// Original expensive function
	expensiveComputation := func(n int) string {
		fmt.Printf("Computing for %d (this is expensive)...\n", n)
		time.Sleep(100 * time.Millisecond) // Simulate expensive operation
		return fmt.Sprintf("result-%d", n*2)
	}

	// Wrap the function with caching
	cachedComputation := obcache.Wrap(cache, expensiveComputation)

	// First call - will execute the function
	result1 := cachedComputation(5)
	fmt.Printf("Result 1: %s\n", result1)

	// Second call - will use cache
	result2 := cachedComputation(5)
	fmt.Printf("Result 2: %s\n", result2)

	// Different input - will execute the function again
	result3 := cachedComputation(10)
	fmt.Printf("Result 3: %s\n", result3)

	// Example 3: Function with error handling
	fmt.Println("\n=== Function with Error ===")

	riskyFunction := func(n int) (string, error) {
		if n < 0 {
			return "", fmt.Errorf("negative numbers not allowed")
		}
		fmt.Printf("Processing %d...\n", n)
		time.Sleep(50 * time.Millisecond)
		return fmt.Sprintf("processed-%d", n), nil
	}

	cachedRiskyFunction := obcache.Wrap(cache, riskyFunction)

	// Success case - will be cached
	if result, err := cachedRiskyFunction(42); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Success: %s\n", result)
	}

	// Same input - from cache
	if result, err := cachedRiskyFunction(42); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Cached: %s\n", result)
	}

	// Error case - not cached
	if result, err := cachedRiskyFunction(-1); err != nil {
		fmt.Printf("Error (not cached): %v\n", err)
	} else {
		fmt.Printf("Result: %s\n", result)
	}

	// Example 4: Custom configuration
	fmt.Println("\n=== Custom Configuration ===")

	customConfig := obcache.NewDefaultConfig().
		WithMaxEntries(100).
		WithDefaultTTL(1 * time.Second)

	customCache, err := obcache.New(customConfig)
	if err != nil {
		panic(err)
	}

	slowFunction := func(s string) string {
		fmt.Printf("Slow processing of '%s'...\n", s)
		time.Sleep(100 * time.Millisecond)
		return "processed-" + s
	}

	cachedSlowFunction := obcache.Wrap(customCache, slowFunction,
		obcache.WithTTL(500*time.Millisecond))

	// First call
	result := cachedSlowFunction("test")
	fmt.Printf("Result: %s\n", result)

	// Immediate second call - from cache
	result = cachedSlowFunction("test")
	fmt.Printf("Cached: %s\n", result)

	// Wait for TTL expiration
	fmt.Println("Waiting for TTL expiration...")
	time.Sleep(600 * time.Millisecond)

	// Third call - will recompute due to TTL expiration
	result = cachedSlowFunction("test")
	fmt.Printf("Recomputed: %s\n", result)

	// Example 5: Cache statistics
	fmt.Println("\n=== Cache Statistics ===")
	stats := cache.Stats()
	fmt.Printf("Cache Stats:\n")
	fmt.Printf("  Hits: %d\n", stats.Hits())
	fmt.Printf("  Misses: %d\n", stats.Misses())
	fmt.Printf("  Hit Rate: %.2f%%\n", stats.HitRate())
	fmt.Printf("  Keys: %d\n", stats.KeyCount())
	fmt.Printf("  Evictions: %d\n", stats.Evictions())
	fmt.Printf("  Invalidations: %d\n", stats.Invalidations())
}
