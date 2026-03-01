# obcache-go

High-performance, thread-safe caching library for Go with automatic function wrapping and TTL support.

[![Go Reference](https://pkg.go.dev/badge/github.com/1mb-dev/obcache-go.svg)](https://pkg.go.dev/github.com/1mb-dev/obcache-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/1mb-dev/obcache-go)](https://goreportcard.com/report/github.com/1mb-dev/obcache-go)
[![CI](https://github.com/1mb-dev/obcache-go/workflows/CI/badge.svg)](https://github.com/1mb-dev/obcache-go/actions/workflows/ci.yml)
[![Security](https://github.com/1mb-dev/obcache-go/workflows/Security/badge.svg)](https://github.com/1mb-dev/obcache-go/actions/workflows/security.yml)
[![codecov](https://codecov.io/gh/1mb-dev/obcache-go/branch/main/graph/badge.svg)](https://codecov.io/gh/1mb-dev/obcache-go)
[![License](https://img.shields.io/github/license/1mb-dev/obcache-go.svg)](https://github.com/1mb-dev/obcache-go/blob/main/LICENSE)
[![Release](https://img.shields.io/github/release/1mb-dev/obcache-go.svg)](https://github.com/1mb-dev/obcache-go/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/1mb-dev/obcache-go)](https://github.com/1mb-dev/obcache-go/blob/main/go.mod)

## When to Use This

Use obcache-go when you want to cache expensive function results with minimal boilerplate -- wrap the function, get caching for free.

**Choose obcache-go over [patrickmn/go-cache](https://github.com/patrickmn/go-cache) when:**
- You want automatic function wrapping (`obcache.Wrap`) instead of manual get/set/invalidate
- You need multiple eviction strategies (LRU, LFU, FIFO) -- go-cache only does TTL expiration
- You need a Redis backend for distributed caching alongside in-memory
- You want built-in Prometheus metrics and compression

**Choose patrickmn/go-cache instead when:**
- You need a simple TTL cache with minimal API surface
- You don't need function wrapping, eviction strategies, or Redis

**Choose Redis directly when:**
- All your caching is distributed and you don't need an in-memory layer
- You need cache sharing across multiple services

## Installation

```bash
go get github.com/1mb-dev/obcache-go
```

## Quick Start

### Function Wrapping (Recommended)

```go
package main

import (
    "fmt"
    "time"
    "github.com/1mb-dev/obcache-go/pkg/obcache"
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
    WithDefaultTTL(time.Hour)

// Customize Redis key prefix
config.Redis.KeyPrefix = "myapp:"

cache, _ := obcache.New(config)
```

### Eviction Strategies

```go
import "github.com/1mb-dev/obcache-go/internal/eviction"

// LRU (Least Recently Used) - Default
config := obcache.NewDefaultConfig().
    WithMaxEntries(1000).
    WithEvictionType(eviction.LRU)

// LFU (Least Frequently Used)
config := obcache.NewDefaultConfig().
    WithMaxEntries(1000).
    WithEvictionType(eviction.LFU)

// FIFO (First In, First Out)
config := obcache.NewDefaultConfig().
    WithMaxEntries(1000).
    WithEvictionType(eviction.FIFO)
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
- **Multiple eviction strategies** - LRU, LFU, and FIFO support
- **Thread safe** - Concurrent access support
- **Redis backend** - Distributed caching
- **Compression** - Automatic value compression (gzip/deflate)
- **Prometheus metrics** - Built-in metrics exporter
- **Statistics** - Hit rates, miss counts, etc.
- **Context-aware hooks** - Event callbacks for cache operations

## Examples

See [examples/](examples/) for complete examples:
- [Basic usage](examples/basic/main.go)
- [Redis caching](examples/redis-cache/main.go)
- [Compression](examples/compression/main.go)
- [Prometheus metrics](examples/prometheus/main.go)
- [Gin web server integration](examples/gin-web-server/main.go)

## License

MIT License - see [LICENSE](LICENSE) file.