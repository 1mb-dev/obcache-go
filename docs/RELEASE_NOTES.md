# obcache-go v1.0.0 Release Notes

🎉 **Initial stable release of obcache-go** - A high-performance, thread-safe caching library for Go.

## 🚀 What is obcache-go?

obcache-go is a powerful caching library designed for modern Go applications. It provides automatic function wrapping, multiple backend support, and production-ready features that make caching both simple and powerful.

## ✨ Key Features

### 🎯 **Function Wrapping (Primary Feature)**
```go
// Turn any function into a cached version
cachedFunc := obcache.Wrap(cache, expensiveFunction)
result, _ := cachedFunc(args...) // Automatic caching!
```

### 🏪 **Multiple Backends**
- **Memory**: High-performance in-memory caching
- **Redis**: Distributed caching for multi-instance deployments

### 🔧 **Advanced Features**
- **TTL Support**: Time-based expiration with automatic cleanup
- **LRU/LFU/FIFO Eviction**: Multiple eviction strategies
- **Compression**: Gzip/Deflate compression for large values
- **Thread Safety**: Optimized concurrent access
- **Statistics**: Hit rates, miss counts, performance metrics
- **Hooks**: Event callbacks for monitoring and debugging
- **Singleflight**: Prevents duplicate concurrent function calls

### 📊 **Monitoring & Observability**
- Prometheus metrics integration
- OpenTelemetry support
- Built-in statistics and hit rate tracking
- Event hooks for custom monitoring

## 🎯 **Why Choose obcache-go?**

1. **Simplicity**: One line to cache any function - `obcache.Wrap(cache, fn)`
2. **Performance**: Optimized for high-concurrency scenarios
3. **Flexibility**: Memory or Redis backends, multiple eviction strategies
4. **Production Ready**: Comprehensive testing, monitoring, and error handling
5. **Type Safety**: Full Go generics support for type-safe caching

## 📚 **Quick Start**

```go
package main

import (
    "fmt"
    "time"
    "github.com/1mb-dev/obcache-go/pkg/obcache"
)

func expensiveFunction(id int) (string, error) {
    time.Sleep(100 * time.Millisecond) // Simulate work
    return fmt.Sprintf("result-%d", id), nil
}

func main() {
    cache, _ := obcache.New(obcache.NewDefaultConfig())
    
    // Wrap function with caching
    cachedFunc := obcache.Wrap(cache, expensiveFunction)
    
    // First call: slow (cache miss)
    result1, _ := cachedFunc(123) 
    
    // Second call: fast (cache hit)  
    result2, _ := cachedFunc(123)
    
    fmt.Println(result1, result2) // Same result, much faster
}
```

## 📦 **Installation**

```bash
go get github.com/1mb-dev/obcache-go
```

Requires Go 1.21+ for generics support.

## 🎨 **Configuration Examples**

### Memory Cache
```go
config := obcache.NewDefaultConfig().
    WithMaxEntries(1000).
    WithDefaultTTL(30 * time.Minute)
cache, _ := obcache.New(config)
```

### Redis Cache
```go
config := obcache.NewRedisConfig("localhost:6379").
    WithRedis(&obcache.RedisConfig{KeyPrefix: "myapp:"})
cache, _ := obcache.New(config)
```

### With Compression
```go
config := obcache.NewDefaultConfig().
    WithCompression(&compression.Config{
        Enabled: true,
        Algorithm: compression.CompressorGzip,
        MinSize: 1000, // Only compress values > 1KB
    })
```

## 📈 **Performance**

- **Concurrent reads/writes**: Optimized locking strategy
- **Memory efficient**: LRU eviction prevents unbounded growth  
- **Fast key generation**: Efficient hashing for complex keys
- **Singleflight**: Eliminates duplicate work under high load

## 🧪 **Examples**

The release includes comprehensive examples:

- **[Basic Usage](examples/basic/main.go)** - Simple cache operations
- **[Redis Backend](examples/redis-cache/main.go)** - Distributed caching
- **[Compression](examples/compression/main.go)** - Value compression
- **[Metrics Integration](examples/metrics/main.go)** - Prometheus/OpenTelemetry
- **[Web Server](examples/gin-web-server/main.go)** - Real-world usage

## 🔄 **Migration from other libraries**

### From go-cache:
```go
// Before (go-cache)
import "github.com/patrickmn/go-cache"
c := cache.New(5*time.Minute, 10*time.Minute)

// After (obcache-go)  
import "github.com/1mb-dev/obcache-go/pkg/obcache"
cache, _ := obcache.New(obcache.NewDefaultConfig())
```

## 🎯 **Best Practices**

1. **Use function wrapping** - More convenient than manual cache operations
2. **Set appropriate TTLs** - Balance freshness vs performance
3. **Monitor hit rates** - Aim for >80% hit rate for good effectiveness
4. **Use Redis for distributed scenarios** - Essential for multi-instance apps
5. **Enable compression** - For large values (>1KB) to save memory

## 🔗 **Links**

- **Documentation**: [docs/README.md](docs/README.md)
- **Examples**: [examples/](examples/)
- **Go Reference**: https://pkg.go.dev/github.com/1mb-dev/obcache-go
- **Issues**: https://github.com/1mb-dev/obcache-go/issues

## 🙏 **Acknowledgments**

This library was built with production use cases in mind, incorporating lessons learned from scaling Go applications. Special thanks to the Go community for the excellent ecosystem of tools and libraries that made this possible.

---

**Ready to supercharge your Go applications with intelligent caching?**

```bash
go get github.com/1mb-dev/obcache-go
```

Happy caching! 🚀