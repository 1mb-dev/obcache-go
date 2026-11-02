package obcache

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/1mb-dev/obcache-go/v2/internal/entry"
	"github.com/1mb-dev/obcache-go/v2/internal/eviction"
	"github.com/1mb-dev/obcache-go/v2/internal/singleflight"
	"github.com/1mb-dev/obcache-go/v2/internal/store"
	"github.com/1mb-dev/obcache-go/v2/internal/store/memory"
	redisstore "github.com/1mb-dev/obcache-go/v2/internal/store/redis"
	"github.com/1mb-dev/obcache-go/v2/pkg/compression"
	"github.com/1mb-dev/obcache-go/v2/pkg/metrics"
)

func (c *Cache) hit(ctx context.Context, key string, value any) {
	c.stats.incHits()
	if c.hooks != nil {
		c.hooks.invokeOnHitWithCtx(ctx, key, value, nil)
	}
}

func (c *Cache) miss(ctx context.Context, key string) {
	c.stats.incMisses()
	if c.hooks != nil {
		c.hooks.invokeOnMissWithCtx(ctx, key, nil)
	}
}

// Cache is the main cache implementation with LRU and TTL support
type Cache struct {
	config *Config
	store  store.Store
	stats  *Stats
	hooks  *Hooks
	sf     *singleflight.Group[string, flight]

	// Compression
	compressor compression.Compressor

	// Metrics
	metricsExporter metrics.Exporter
	metricsLabels   metrics.Labels
	metricsStop     chan struct{}
	metricsWg       sync.WaitGroup

	closeOnce sync.Once
	closeErr  error

	// Function identity
	names   sync.Map // name -> nameBinding
	wrapSeq atomic.Uint64
}

// New creates a new Cache instance with the given configuration
func New(config *Config) (*Cache, error) {
	if config == nil {
		config = NewDefaultConfig()
	}

	// Create the appropriate store based on configuration
	var cacheStore store.Store
	var err error

	switch config.StoreType {
	case StoreTypeMemory:
		cacheStore, err = createMemoryStore(config)
	case StoreTypeRedis:
		cacheStore, err = createRedisStore(config)
	default:
		return nil, fmt.Errorf("unsupported store type: %v", config.StoreType)
	}

	if err != nil {
		return nil, err
	}

	cache := &Cache{
		config: config,
		store:  cacheStore,
		stats:  &Stats{},
		hooks:  config.Hooks,
		sf:     &singleflight.Group[string, flight]{},
	}

	// Initialize compression if configured
	if err := cache.initializeCompression(); err != nil {
		return nil, fmt.Errorf("failed to initialize compression: %w", err)
	}

	// Initialize metrics if configured
	if err := cache.initializeMetrics(); err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}

	// Set up store callbacks for statistics and hooks
	if lruStore, ok := cacheStore.(store.LRUStore); ok {
		lruStore.SetEvictCallback(func(key string, value any) {
			cache.stats.incEvictions()
			if cache.hooks != nil {
				// All memory stores now use StrategyStore which evicts based on capacity
				cache.hooks.invokeOnEvict(key, value, EvictReasonCapacity)
			}
		})
	}

	if ttlStore, ok := cacheStore.(store.TTLStore); ok {
		ttlStore.SetCleanupCallback(func(key string, value any) {
			cache.stats.incEvictions()
			if cache.hooks != nil {
				cache.hooks.invokeOnEvict(key, value, EvictReasonTTL)
			}
		})
	}

	return cache, nil
}

// NewSimple creates a simple cache with minimal configuration
// This is perfect for most use cases where you just need basic caching
func NewSimple(maxEntries int, defaultTTL time.Duration) (*Cache, error) {
	return New(NewSimpleConfig(maxEntries, defaultTTL))
}

// createMemoryStore creates a memory-based store
func createMemoryStore(config *Config) (store.Store, error) {
	// Determine eviction type (default to LRU if not specified)
	evictionType := config.EvictionType
	if evictionType == "" {
		evictionType = eviction.LRU
	}

	switch evictionType {
	case EvictionLRU, EvictionLFU, EvictionFIFO:
	default:
		return nil, fmt.Errorf("unknown eviction type %q: use EvictionLRU, EvictionLFU or EvictionFIFO", evictionType)
	}

	evictionConfig := eviction.Config{
		Type:     evictionType,
		Capacity: config.MaxEntries,
	}

	// Create store with or without cleanup interval
	if config.CleanupInterval > 0 {
		return memory.NewWithStrategyAndCleanup(evictionConfig, config.CleanupInterval)
	}
	return memory.NewWithStrategy(evictionConfig)
}

// A cache must not turn a hung Redis into a hung caller: without these, go-redis
// ignores ctx deadlines and retries for seconds on a silent connection.
const (
	redisDialTimeout = time.Second
	redisIOTimeout   = 250 * time.Millisecond
	redisMaxRetries  = 1
)

// createRedisStore creates a Redis-based store
func createRedisStore(config *Config) (store.Store, error) {
	if config.Redis == nil {
		return nil, fmt.Errorf("redis configuration is required when using StoreTypeRedis")
	}

	redisConfig := &redisstore.Config{
		DefaultTTL: config.DefaultTTL,
		KeyPrefix:  config.Redis.KeyPrefix,
		Context:    context.Background(),
	}

	// Use provided client or create a new one
	if config.Redis.Client != nil {
		redisConfig.Client = config.Redis.Client
	} else {
		// Create Redis client from connection parameters
		client := redis.NewClient(&redis.Options{
			Addr:                  config.Redis.Addr,
			Password:              config.Redis.Password,
			DB:                    config.Redis.DB,
			ContextTimeoutEnabled: true,
			DialTimeout:           redisDialTimeout,
			ReadTimeout:           redisIOTimeout,
			WriteTimeout:          redisIOTimeout,
			MaxRetries:            redisMaxRetries,
		})

		// Test the connection
		ctx := context.Background()
		if err := client.Ping(ctx).Err(); err != nil {
			return nil, fmt.Errorf("failed to connect to Redis: %w", err)
		}

		redisConfig.Client = client
		redisConfig.CloseClient = client.Close
	}

	return redisstore.New(redisConfig)
}

// Get retrieves a value from the cache by key
// For context-aware operations, use GetContext instead
func (c *Cache) Get(key string) (any, bool) {
	return c.GetContext(context.Background(), key)
}

// GetContext retrieves a value from the cache by key with context support
// The context can be used for cancellation, timeouts, and trace propagation
func (c *Cache) GetContext(ctx context.Context, key string) (any, bool) {
	start := time.Now()
	defer func() {
		c.recordCacheOperation(metrics.OperationGet, time.Since(start))
	}()

	entry, ok := c.store.Get(ctx, key)
	if !ok {
		c.miss(ctx, key)
		return nil, false
	}

	value, err := c.decompressValue(entry)
	if err != nil {
		c.miss(ctx, key)
		return nil, false
	}

	c.hit(ctx, key, value)
	return value, true
}

// Set stores a value in the cache with the specified key and TTL
// For context-aware operations, use SetContext instead
func (c *Cache) Set(key string, value any, ttl time.Duration) error {
	return c.SetContext(context.Background(), key, value, ttl)
}

// SetContext stores a value in the cache with context support
// The context can be used for cancellation, timeouts, and trace propagation
func (c *Cache) SetContext(ctx context.Context, key string, value any, ttl time.Duration) error {
	start := time.Now()
	defer func() {
		c.recordCacheOperation(metrics.OperationSet, time.Since(start))
	}()

	if ttl <= 0 {
		ttl = c.config.DefaultTTL
	}

	entry, err := c.createCompressedEntry(value, ttl)
	if err != nil {
		return fmt.Errorf("failed to create entry: %w", err)
	}

	return c.store.Set(ctx, key, entry)
}

// Put stores a value using the default TTL
func (c *Cache) Put(key string, value any) error {
	return c.Set(key, value, c.config.DefaultTTL)
}

// Delete removes a key from the cache
func (c *Cache) Delete(key string) error {
	ctx := context.Background()

	err := c.store.Delete(key)
	if err == nil {
		c.stats.incInvalidations()
		if c.hooks != nil {
			c.hooks.invokeOnInvalidateWithCtx(ctx, key, nil)
		}
	}

	return err
}

// Clear removes all entries from the cache
func (c *Cache) Clear() error {
	ctx := context.Background()

	keys := c.store.Keys()
	err := c.store.Clear()
	if err == nil {
		for _, key := range keys {
			c.stats.incInvalidations()
			if c.hooks != nil {
				c.hooks.invokeOnInvalidateWithCtx(ctx, key, nil)
			}
		}
	}

	return err
}

// Stats returns the current cache statistics
func (c *Cache) Stats() *Stats {
	c.updateKeyCount()
	return c.stats
}

// Keys returns all current cache keys
func (c *Cache) Keys() []string {
	return c.store.Keys()
}

// Len returns the current number of entries in the cache
func (c *Cache) Len() int {
	return c.store.Len()
}

// Has checks if a key exists in the cache
func (c *Cache) Has(key string) bool {
	entry, found := c.store.Get(context.Background(), key)
	return found && !entry.IsExpired()
}

// TTL returns the remaining TTL for a key
func (c *Cache) TTL(key string) (time.Duration, bool) {
	entry, ok := c.store.Get(context.Background(), key)
	if ok && !entry.IsExpired() {
		return entry.TTL(), true
	}
	return 0, false
}

// Close closes the cache and cleans up resources
func (c *Cache) Close() error {
	c.closeOnce.Do(func() {
		if c.metricsStop != nil {
			close(c.metricsStop)
			c.metricsWg.Wait()
		}
		if c.metricsExporter != nil {
			_ = c.metricsExporter.Close() // Ignore error on shutdown
		}
		c.closeErr = c.store.Close()
	})
	return c.closeErr
}

// Cleanup removes expired entries and returns count removed
func (c *Cache) Cleanup() int {
	if store, ok := c.store.(store.TTLStore); ok {
		return store.Cleanup()
	}
	return 0
}

// updateKeyCount updates the key count statistic
func (c *Cache) updateKeyCount() {
	count := int64(c.store.Len())
	c.stats.setKeyCount(count)
}

// serializes reports whether values pass through JSON on their way to the store.
func (c *Cache) serializes() bool {
	return c.config.StoreType == StoreTypeRedis || (c.config.Compression != nil && c.config.Compression.Enabled)
}

// createCompressedEntry creates a cache entry with compression if applicable
func (c *Cache) createCompressedEntry(value any, ttl time.Duration) (*entry.Entry, error) {
	var cacheEntry *entry.Entry
	if ttl > 0 {
		cacheEntry = entry.New(nil, ttl) // We'll set the value after compression
	} else {
		cacheEntry = entry.NewWithoutTTL(nil)
	}

	// Only try compression if it's enabled
	if c.config.Compression != nil && c.config.Compression.Enabled {
		// Serialize and compress the value
		compressed, isCompressed, err := compression.SerializeAndCompress(
			value,
			c.compressor,
			c.config.Compression.MinSize,
		)
		if err != nil {
			return nil, err
		}

		if isCompressed {
			// Store compressed data and metadata
			cacheEntry.Value = compressed

			// Calculate original size by serializing without compression
			serialized, _, serErr := compression.SerializeAndCompress(value, compression.NewNoOpCompressor(), 0)
			originalSize := len(serialized)
			if serErr != nil {
				// Fallback to approximate size if serialization fails
				originalSize = c.approximateSize(value)
			}

			cacheEntry.SetCompressionInfo(c.compressor.Name(), originalSize, len(compressed))
		} else {
			// Store uncompressed data
			cacheEntry.Value = compressed // This is actually the uncompressed serialized data
		}
	} else {
		// No compression, store value directly
		cacheEntry.Value = value
	}

	return cacheEntry, nil
}

// decompressValue decompresses a cached value if needed
func (c *Cache) decompressValue(entry *entry.Entry) (any, error) {
	// Check if compression was used during storage
	if c.config.Compression != nil && c.config.Compression.Enabled {
		// Value was stored with compression logic (might be compressed or serialized)
		data, ok := entry.Value.([]byte)
		if !ok {
			return nil, fmt.Errorf("serialized value is not []byte")
		}

		var result any
		err := compression.DecompressAndDeserialize(data, entry.IsCompressed, c.compressor, &result)
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize value: %w", err)
		}

		return result, nil
	}
	// No compression was configured, return value directly
	return entry.Value, nil
}

// approximateSize estimates the memory size of a value
func (c *Cache) approximateSize(value any) int {
	if value == nil {
		return 0
	}

	switch v := value.(type) {
	case string:
		return len(v)
	case []byte:
		return len(v)
	case int, int8, int16, int32, int64:
		return 8
	case uint, uint8, uint16, uint32, uint64:
		return 8
	case float32:
		return 4
	case float64:
		return 8
	case bool:
		return 1
	default:
		// For complex types, use reflection to estimate
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			return rv.Len() * 8 // Rough estimate
		case reflect.Map:
			return rv.Len() * 16 // Rough estimate for key-value pairs
		case reflect.Struct:
			return rv.NumField() * 8 // Rough estimate
		default:
			return 64 // Default fallback
		}
	}
}

// initializeCompression sets up compression if enabled
func (c *Cache) initializeCompression() error {
	if c.config.Compression == nil {
		c.config.Compression = compression.NewDefaultConfig()
	}

	compressor, err := compression.NewCompressor(c.config.Compression)
	if err != nil {
		return fmt.Errorf("failed to create compressor: %w", err)
	}

	c.compressor = compressor
	return nil
}

// initializeMetrics sets up metrics collection if enabled
func (c *Cache) initializeMetrics() error {
	if c.config.Metrics == nil || !c.config.Metrics.Enabled || c.config.Metrics.Exporter == nil {
		c.metricsExporter = metrics.NewNoOpExporter()
		return nil
	}

	c.metricsExporter = c.config.Metrics.Exporter

	// Prepare metrics labels with cache name
	c.metricsLabels = make(metrics.Labels)
	if c.config.Metrics.CacheName != "" {
		c.metricsLabels["cache_name"] = c.config.Metrics.CacheName
	} else {
		c.metricsLabels["cache_name"] = "default"
	}

	// Add any additional labels from config
	for k, v := range c.config.Metrics.Labels {
		c.metricsLabels[k] = v
	}

	// Start automatic stats reporting if interval is configured
	if c.config.Metrics.ReportingInterval > 0 {
		c.metricsStop = make(chan struct{})
		c.metricsWg.Add(1)
		go c.metricsReporter()
	}

	return nil
}

// metricsReporter periodically exports cache statistics
func (c *Cache) metricsReporter() {
	defer c.metricsWg.Done()

	ticker := time.NewTicker(c.config.Metrics.ReportingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.exportCurrentStats()
		case <-c.metricsStop:
			// Final stats export before shutting down
			c.exportCurrentStats()
			return
		}
	}
}

// exportCurrentStats exports the current statistics to metrics
func (c *Cache) exportCurrentStats() {
	c.updateKeyCount()
	if c.metricsExporter != nil {
		_ = c.metricsExporter.ExportStats(c.stats, c.metricsLabels) //nolint:errcheck // Error handling done at higher level
	}
}

// recordCacheOperation records a cache operation with timing for metrics
func (c *Cache) recordCacheOperation(operation metrics.Operation, duration time.Duration) {
	if c.metricsExporter != nil {
		_ = c.metricsExporter.RecordCacheOperation(operation, duration, c.metricsLabels) //nolint:errcheck // Error handling done at higher level
	}
}
