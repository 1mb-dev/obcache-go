// Package obcache provides a high-performance, thread-safe, in-memory cache with TTL support,
// multiple eviction strategies (LRU/LFU/FIFO), function memoization, and hooks for observability.
//
// # Overview
//
// obcache is designed for high-throughput applications requiring fast, reliable caching
// with comprehensive observability and flexible configuration options. It supports both
// direct cache operations and transparent function memoization through its Wrap functionality.
//
// # Key Features
//
//   - Thread-safe concurrent access with minimal lock contention
//   - Time-to-live (TTL) expiration with automatic cleanup
//   - Multiple eviction strategies: LRU, LFU, and FIFO
//   - Function memoization with customizable key generation
//   - Context-aware hooks for monitoring cache operations
//   - Built-in statistics and performance monitoring
//   - Redis backend support for distributed caching
//   - Compression support for large values (gzip/deflate)
//   - Prometheus metrics integration
//   - Singleflight pattern to prevent cache stampedes
//
// # Basic Usage
//
// Create a cache and perform basic operations:
//
//	cache, err := obcache.New(obcache.NewDefaultConfig())
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Store a value with 1-hour TTL
//	err = cache.Set("user:123", userData, time.Hour)
//	if err != nil {
//	    log.Printf("Failed to set cache: %v", err)
//	}
//
//	// Retrieve a value
//	value, found := cache.Get("user:123")
//	if found {
//	    user := value.(UserData)
//	    fmt.Printf("Found user: %+v\n", user)
//	}
//
//	// Check statistics
//	stats := cache.Stats()
//	fmt.Printf("Hit rate: %.2f%%\n", stats.HitRate())
//
// # Function Memoization
//
// Cache expensive function calls automatically:
//
//	// Original expensive function
//	func fetchUser(userID int) (*User, error) {
//	    // Expensive database query
//	    return queryDatabase(userID)
//	}
//
//	// Wrap with caching
//	cache, _ := obcache.New(obcache.NewDefaultConfig())
//	cachedFetchUser := obcache.Wrap(cache, fetchUser, obcache.WithTTL(5*time.Minute))
//
//	// Use exactly like the original function - caching is transparent
//	user1, err := cachedFetchUser(123)  // Database query
//	user2, err := cachedFetchUser(123)  // Cache hit
//
// # Configuration
//
// Customize cache behavior with fluent configuration:
//
//	config := obcache.NewDefaultConfig().
//	    WithMaxEntries(10000).
//	    WithDefaultTTL(30*time.Minute).
//	    WithCleanupInterval(5*time.Minute).
//	    WithEvictionType(eviction.LFU)  // Use LFU instead of default LRU
//
//	cache, err := obcache.New(config)
//
// # Eviction Strategies
//
// Choose the eviction strategy that best fits your use case:
//
//	import "github.com/1mb-dev/obcache-go/internal/eviction"
//
//	// LRU (Least Recently Used) - Default
//	// Evicts items that haven't been accessed recently
//	config := obcache.NewDefaultConfig().WithEvictionType(eviction.LRU)
//
//	// LFU (Least Frequently Used)
//	// Evicts items with the lowest access count
//	config := obcache.NewDefaultConfig().WithEvictionType(eviction.LFU)
//
//	// FIFO (First In, First Out)
//	// Evicts oldest items regardless of access patterns
//	config := obcache.NewDefaultConfig().WithEvictionType(eviction.FIFO)
//
// # Context-Aware Hooks
//
// Monitor cache operations with context-aware hooks:
//
//	hooks := &obcache.Hooks{}
//
//	// Hook on cache hits
//	hooks.AddOnHit(func(ctx context.Context, key string, value any) {
//	    log.Printf("Cache hit: %s", key)
//	    metrics.IncrementCounter("cache.hits")
//	})
//
//	// Hook on cache misses
//	hooks.AddOnMiss(func(ctx context.Context, key string) {
//	    log.Printf("Cache miss: %s", key)
//	    metrics.IncrementCounter("cache.misses")
//	})
//
//	// Hook on evictions
//	hooks.AddOnEvict(func(ctx context.Context, key string, value any, reason obcache.EvictReason) {
//	    log.Printf("Evicted: %s (reason: %s)", key, reason)
//	})
//
//	// Hook on manual invalidations
//	hooks.AddOnInvalidate(func(ctx context.Context, key string) {
//	    log.Printf("Invalidated: %s", key)
//	})
//
//	cache, _ := obcache.New(obcache.NewDefaultConfig().WithHooks(hooks))
//
// # Context Propagation
//
// Use context-aware methods for timeouts and tracing:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
//	defer cancel()
//
//	// Set with context
//	err := cache.SetContext(ctx, "key", "value", time.Hour)
//
//	// Get with context
//	value, found := cache.GetContext(ctx, "key")
//
// # Redis Backend
//
// Use Redis for distributed caching:
//
//	config := obcache.NewRedisConfig("localhost:6379").
//	    WithDefaultTTL(time.Hour)
//
//	// Customize Redis key prefix
//	config.Redis.KeyPrefix = "myapp:"
//
//	cache, err := obcache.New(config)
//	// All operations now use Redis instead of local memory
//
// # Compression
//
// Enable compression for large values:
//
//	import "github.com/1mb-dev/obcache-go/pkg/compression"
//
//	config := obcache.NewDefaultConfig().
//	    WithCompression(&compression.Config{
//	        Enabled:   true,
//	        Algorithm: compression.CompressorGzip,
//	        MinSize:   1024, // Only compress values > 1KB
//	    })
//
//	cache, err := obcache.New(config)
//
// # Metrics Integration
//
// Export metrics to Prometheus:
//
//	import (
//	    "github.com/1mb-dev/obcache-go/pkg/metrics"
//	    "github.com/prometheus/client_golang/prometheus"
//	)
//
//	// Create Prometheus exporter
//	promConfig := &metrics.PrometheusConfig{
//	    Registry: prometheus.DefaultRegisterer,
//	}
//	metricsConfig := metrics.NewDefaultConfig()
//	exporter, _ := metrics.NewPrometheusExporter(metricsConfig, promConfig)
//
//	// Configure cache with metrics
//	config := obcache.NewDefaultConfig().
//	    WithMetrics(&obcache.MetricsConfig{
//	        Exporter:  exporter,
//	        Enabled:   true,
//	        CacheName: "user-cache",
//	    })
//
//	cache, _ := obcache.New(config)
//
// You can also implement custom exporters by implementing the metrics.Exporter interface.
//
// # Performance Considerations
//
//   - Use appropriate cache sizes based on available memory
//   - Set reasonable TTL values to balance freshness with performance
//   - Consider using Redis backend for multi-instance deployments
//   - Enable compression for large values to reduce memory usage
//   - Use hooks judiciously to avoid performance overhead
//   - Monitor hit rates and adjust cache policies accordingly
//   - Choose eviction strategy based on access patterns:
//     * LRU: Good for temporal locality (recently used data)
//     * LFU: Good for frequency patterns (popular items)
//     * FIFO: Simple, predictable, good for time-series data
//
// # Thread Safety
//
// All cache operations are thread-safe and can be called concurrently from multiple
// goroutines without additional synchronization. The cache uses fine-grained locking
// and atomic operations to minimize contention.
//
// # Error Handling
//
// The cache is designed to degrade gracefully:
//   - Set operations may fail due to capacity or backend issues
//   - Get operations never fail - they return (nil, false) for missing/error cases
//   - Hook execution errors are logged but don't affect cache operations
//   - Backend connectivity issues fall back to cache misses where possible
//
// # Best Practices
//
//   - Use meaningful cache keys with consistent naming patterns
//   - Set appropriate TTL values based on data freshness requirements
//   - Monitor cache performance using built-in statistics
//   - Use function wrapping for transparent caching of expensive operations
//   - Implement proper error handling for critical cache operations
//   - Use hooks for observability and debugging, not business logic
//   - Test cache behavior under various load conditions
//   - Choose the right eviction strategy for your access patterns
//
// # Examples
//
// See the examples directory for complete, runnable examples including:
//   - Basic cache usage patterns
//   - Redis integration
//   - Prometheus metrics collection
//   - Compression usage
//   - Web framework integration (Gin)
//
// For more detailed documentation and examples, visit:
// https://github.com/1mb-dev/obcache-go
package obcache
