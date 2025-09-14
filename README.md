# obcache-go

High-performance, thread-safe caching library for Go with automatic function wrapping and TTL support.

[![Go Reference](https://pkg.go.dev/badge/github.com/vnykmshr/obcache-go.svg)](https://pkg.go.dev/github.com/vnykmshr/obcache-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/vnykmshr/obcache-go)](https://goreportcard.com/report/github.com/vnykmshr/obcache-go)
[![CI](https://github.com/vnykmshr/obcache-go/workflows/CI/badge.svg)](https://github.com/vnykmshr/obcache-go/actions/workflows/ci.yml)
[![Security](https://github.com/vnykmshr/obcache-go/workflows/Security/badge.svg)](https://github.com/vnykmshr/obcache-go/actions/workflows/security.yml)
[![codecov](https://codecov.io/gh/vnykmshr/obcache-go/branch/main/graph/badge.svg)](https://codecov.io/gh/vnykmshr/obcache-go)
[![License](https://img.shields.io/github/license/vnykmshr/obcache-go.svg)](https://github.com/vnykmshr/obcache-go/blob/main/LICENSE)
[![Release](https://img.shields.io/github/release/vnykmshr/obcache-go.svg)](https://github.com/vnykmshr/obcache-go/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/vnykmshr/obcache-go)](https://github.com/vnykmshr/obcache-go/blob/main/go.mod)

## Installation

```bash
go get github.com/vnykmshr/obcache-go
```

## Quick Start

### Function Wrapping (Recommended)

```go
package main

import (
    "fmt"
    "time"
    "github.com/vnykmshr/obcache-go/pkg/obcache"
)

func expensiveFunction(id int) (string, error) {
    time.Sleep(100 * time.Millisecond) // Simulate expensive work
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

### Basic Operations

```go
cache, _ := obcache.New(obcache.NewDefaultConfig())

// Set with TTL
cache.Set("key", "value", time.Hour)

// Get value
if value, found := cache.Get("key"); found {
    fmt.Println("Found:", value)
}

// Delete
cache.Delete("key")

// Stats
stats := cache.Stats()
fmt.Printf("Hit rate: %.1f%%\n", stats.HitRate())
```

## Configuration

### Memory Cache

```go
config := obcache.NewDefaultConfig().
    WithMaxEntries(1000).
    WithDefaultTTL(30 * time.Minute)

cache, _ := obcache.New(config)
```

### Redis Backend

```go
config := obcache.NewRedisConfig("localhost:6379").
    WithRedis(&obcache.RedisConfig{
        KeyPrefix: "myapp:",
    }).
    WithDefaultTTL(time.Hour)

cache, _ := obcache.New(config)
```

### Compression

```go
config := obcache.NewDefaultConfig().
    WithCompression(&compression.Config{
        Enabled:   true,
        Algorithm: compression.CompressorGzip,
        MinSize:   1000, // Only compress values > 1KB
    })
```

## Features

- **Function wrapping** - Automatically cache expensive function calls
- **TTL support** - Time-based expiration 
- **LRU eviction** - Automatic cleanup of old entries
- **Thread safe** - Concurrent access support
- **Redis backend** - Distributed caching
- **Compression** - Automatic value compression (gzip/deflate)
- **Statistics** - Hit rates, miss counts, etc.
- **Hooks** - Event callbacks for cache operations

## Examples

See [examples/](examples/) for complete examples:
- [Basic usage](examples/basic/main.go)
- [Redis caching](examples/redis-cache/main.go) 
- [Compression](examples/compression/main.go)

## License

MIT License - see [LICENSE](LICENSE) file.