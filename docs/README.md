# Documentation

## Configuration Reference

### Memory Cache

```go
config := obcache.NewDefaultConfig().
    WithMaxEntries(1000).              // Max entries before eviction (default: 1000)
    WithDefaultTTL(30 * time.Minute).  // Default TTL (default: 5 minutes)
    WithCleanupInterval(time.Minute).  // Cleanup frequency (default: 1 minute)
    WithEvictionType(obcache.LRU)      // LRU, LFU, or FIFO (default: LRU)
```

### Redis Cache

```go
// Simple Redis setup
config := obcache.NewRedisConfig("localhost:6379").
    WithRedis(&obcache.RedisConfig{
        KeyPrefix: "myapp:",
        Password:  "secret",
        DB:       0,
    })

// With existing Redis client
client := redis.NewClient(&redis.Options{...})
config := obcache.NewRedisConfigWithClient(client).
    WithRedis(&obcache.RedisConfig{KeyPrefix: "app:"})
```

### Compression

```go
config := obcache.NewDefaultConfig().
    WithCompression(&compression.Config{
        Enabled:   true,
        Algorithm: compression.CompressorGzip, // or CompressorDeflate
        MinSize:   1000,                       // Only compress values > 1KB
        Level:     6,                          // Compression level (1-9)
    })
```

### Hooks and Monitoring

```go
config := obcache.NewDefaultConfig().
    WithHooks(&obcache.Hooks{
        OnHit: []obcache.OnHitHook{
            func(key string, value any) {
                log.Printf("Cache hit: %s", key)
            },
        },
        OnMiss: []obcache.OnMissHook{
            func(key string) {
                metrics.IncrementMisses()
            },
        },
    }).
    WithMetrics(&obcache.MetricsConfig{
        Enabled:   true,
        Exporter:  prometheusExporter,
        CacheName: "user-cache",
    })
```

## Function Wrapping Options

```go
// Custom TTL
wrapped := obcache.Wrap(cache, fn, obcache.WithTTL(time.Hour))

// Custom key generation
wrapped := obcache.Wrap(cache, fn, 
    obcache.WithKeyFunc(func(args []any) string {
        return fmt.Sprintf("custom:%v", args[0])
    }),
)

// Disable caching (useful for testing)
wrapped := obcache.Wrap(cache, fn, obcache.WithoutCache())

// Cache errors with shorter TTL
wrapped := obcache.Wrap(cache, fn, 
    obcache.WithErrorCaching(true),
    obcache.WithErrorTTL(30 * time.Second),
)
```

## Migration from go-cache

Replace this:
```go
// go-cache
import "github.com/patrickmn/go-cache"
c := cache.New(5*time.Minute, 10*time.Minute)
c.Set("key", "value", cache.DefaultExpiration)
value, found := c.Get("key")
```

With this:
```go
// obcache-go
import "github.com/1mb-dev/obcache-go/pkg/obcache"
cache, _ := obcache.New(obcache.NewDefaultConfig().WithDefaultTTL(5*time.Minute))
cache.Set("key", "value", time.Hour)
value, found := cache.Get("key")
```

## Best Practices

1. **Use function wrapping** - It's more convenient and handles edge cases
2. **Set appropriate TTLs** - Short for changing data, long for stable data  
3. **Monitor hit rates** - Aim for >80% for good cache effectiveness
4. **Use Redis for distributed** - Required when scaling across multiple instances
5. **Enable compression** - For large values (>1KB) to save memory
6. **Handle cache misses** - Always have fallback logic for missing data

## API Reference

### Cache Methods

```go
// Basic operations
cache.Set(key string, value any, ttl time.Duration) error
cache.Get(key string) (any, bool)
cache.Delete(key string) error
cache.Clear() error
cache.Close() error

// Utility methods
cache.Has(key string) bool
cache.Keys() []string
cache.Len() int
cache.Stats() *Stats

// Function wrapping
obcache.Wrap(cache, function, options...)
```

### Stats

```go
stats := cache.Stats()
stats.Hits() int64        // Cache hits
stats.Misses() int64      // Cache misses  
stats.HitRate() float64   // Hit rate percentage
stats.Evictions() int64   // Number of evicted entries
stats.KeyCount() int64    // Current number of keys
```