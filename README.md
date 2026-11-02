# obcache-go

Memoize any Go function in one line, with TTL, eviction and stampede protection.

[![Go Reference](https://pkg.go.dev/badge/github.com/1mb-dev/obcache-go/v2/pkg/obcache.svg)](https://pkg.go.dev/github.com/1mb-dev/obcache-go/v2/pkg/obcache)
[![CI](https://github.com/1mb-dev/obcache-go/workflows/CI/badge.svg)](https://github.com/1mb-dev/obcache-go/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/1mb-dev/obcache-go.svg)](https://github.com/1mb-dev/obcache-go/blob/main/LICENSE)
[![Release](https://img.shields.io/github/release/1mb-dev/obcache-go.svg)](https://github.com/1mb-dev/obcache-go/releases)

## When to Use This

Use obcache-go when an expensive call, usually a query or an API request, should be cached without writing get, set and invalidate around it. Concurrent callers for the same key share one call.

**Use something else when:**
- You need maximum throughput or hit ratio under contention: [ristretto](https://github.com/dgraph-io/ristretto)
- You hold millions of entries and want them out of the garbage collector's way: [freecache](https://github.com/coocood/freecache)
- A small TTL map is enough: [go-cache](https://github.com/patrickmn/go-cache), with [singleflight](https://pkg.go.dev/golang.org/x/sync/singleflight) if you need stampede protection
- All your caching is shared across services: a plain Redis client

## Installation

Requires Go 1.26 or later.

```bash
go get github.com/1mb-dev/obcache-go/v2
```

## Quick Start

```go
package main

import (
	"fmt"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

func expensiveFunction(id int) (string, error) {
	time.Sleep(100 * time.Millisecond) // Simulate expensive work
	return fmt.Sprintf("result-%d", id), nil
}

func main() {
	cache, err := obcache.New(obcache.NewDefaultConfig())
	if err != nil {
		panic(err)
	}
	defer cache.Close()

	// Wrap once, at setup
	cachedFunc := obcache.Wrap(cache, expensiveFunction)

	result1, _ := cachedFunc(123) // slow: cache miss
	result2, _ := cachedFunc(123) // fast: cache hit

	fmt.Println(result1, result2)
}
```

## Configuration

### Direct Use and Eviction

The defaults are 1000 entries, a 5 minute TTL, and LRU eviction.

```go
package main

import (
	"fmt"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

func main() {
	config := obcache.NewDefaultConfig().
		WithMaxEntries(1000).
		WithDefaultTTL(30 * time.Minute).
		WithEvictionType(obcache.EvictionLFU) // or EvictionLRU, EvictionFIFO

	cache, err := obcache.New(config)
	if err != nil {
		panic(err)
	}
	defer cache.Close()

	_ = cache.Set("key", "value", time.Hour)
	if value, found := cache.Get("key"); found {
		fmt.Println("Found:", value)
	}
	_ = cache.Delete("key")

	fmt.Printf("Hit rate: %.1f%%\n", cache.Stats().HitRate())
}
```

### Redis

On Redis, every `Wrap` needs `WithName` or `WithKeyFunc`, because the name is what processes share. A return type must survive JSON, and anyone who can write to your Redis can forge results; see [Limits](#limits).

```go
package main

import (
	"fmt"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

type User struct {
	ID   int64
	Name string
}

func loadUser(id int64) (*User, error) {
	return &User{ID: id, Name: "Ann"}, nil
}

func main() {
	config := obcache.NewRedisConfig("localhost:6379").
		WithDefaultTTL(time.Hour)
	config.Redis.KeyPrefix = "myapp:"

	cache, err := obcache.New(config)
	if err != nil {
		panic(err)
	}
	defer cache.Close()

	cachedUser := obcache.Wrap(cache, loadUser, obcache.WithName("user"))

	user, _ := cachedUser(7)
	fmt.Println(user.Name)
}
```

### Compression

Compression stores large values smaller, for a shared Redis or a memory limit. Values under `MinSize` stay plain. With compression on, values pass through JSON in memory too, so the Redis limits apply.

```go
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/compression"
	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

func main() {
	config := obcache.NewDefaultConfig().
		WithCompression(&compression.Config{
			Enabled:   true,
			Algorithm: compression.CompressorGzip, // or CompressorDeflate
			MinSize:   1000,                       // Only compress values over 1KB
		})

	cache, err := obcache.New(config)
	if err != nil {
		panic(err)
	}
	defer cache.Close()

	_ = cache.Set("report", strings.Repeat("row\n", 1000), time.Hour)
	value, _ := cache.Get("report")
	fmt.Println(len(value.(string)))
}
```

### Prometheus

`pkg/metrics` holds the exporter. Importing only `pkg/obcache` does not link the Prometheus client. Counters reach Prometheus every `ReportingInterval`; with it unset, nothing is exported.

```go
package main

import (
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/metrics"
	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	exporter, err := metrics.NewPrometheusExporter(metrics.NewDefaultConfig(), &metrics.PrometheusConfig{
		Registry: prometheus.NewRegistry(), // serve this with promhttp
	})
	if err != nil {
		panic(err)
	}

	cache, err := obcache.New(obcache.NewDefaultConfig().WithMetrics(&obcache.MetricsConfig{
		Enabled:           true,
		Exporter:          exporter,
		CacheName:         "users",
		ReportingInterval: 15 * time.Second,
	}))
	if err != nil {
		panic(err)
	}
	defer cache.Close()
}
```

### Hooks

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

func main() {
	hooks := obcache.NewHooks()
	hooks.AddOnMiss(func(_ context.Context, _ string) { log.Print("cache miss") })
	hooks.AddOnEvict(func(_ context.Context, _ string, _ any, reason obcache.EvictReason) {
		log.Printf("evicted: %s", reason)
	})

	cache, err := obcache.New(obcache.NewDefaultConfig().WithHooks(hooks))
	if err != nil {
		panic(err)
	}
	defer cache.Close()

	cache.Get("absent")
	_ = cache.Set("key", "value", time.Hour)
}
```

## Invalidation

Name the function, then delete by the key its arguments produce. `Key` panics on a name no `Wrap` bound in this `Cache`, and on arguments that differ from the function's parameters, because either would make `Delete` a silent no-op. A process that only invalidates binds the name with one uncalled `Wrap` of a function with the same signature.

```go
package main

import (
	"fmt"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

func loadPrice(sku string) (int, error) {
	return 42, nil
}

func main() {
	cache, err := obcache.New(obcache.NewDefaultConfig())
	if err != nil {
		panic(err)
	}
	defer cache.Close()

	cachedPrice := obcache.Wrap(cache, loadPrice, obcache.WithName("price"))
	_, _ = cachedPrice("sku-1")

	// The price changed
	if err := cache.Delete(cache.Key("price", "sku-1")); err != nil {
		panic(err)
	}
	fmt.Println(cache.Has(cache.Key("price", "sku-1")))
}
```

## Features

- **Function wrapping** - Cache any function with `Wrap`, including context-first, multi-return, error-only and variadic functions
- **Stampede protection** - Concurrent callers for one key share one call, within a process
- **TTL** - Per-entry expiration, with a default for each cache
- **Eviction** - LRU, LFU or FIFO, for the memory store
- **Thread safety** - Safe for concurrent use
- **Redis backend** - A shared store, keyed under a prefix
- **Compression** - Gzip or deflate above a size threshold
- **Prometheus metrics** - An exporter in `pkg/metrics`
- **Statistics** - Hits, misses, hit rate and key count
- **Hooks** - Context-aware callbacks on hit, miss, eviction and invalidation

## Limits

- **Memory:** `Get` and `Wrap` return the stored value, not a copy. Do not mutate results.
- **Side effects:** a wrapped call that hits does not run the function. A wrapped function with no results runs once per key per TTL: use that to dedupe, or call the function directly if every call must run.
- **Redis or compression:** every value passes through `encoding/json`, at any size. `Wrap` decodes hits to the declared return type and panics at startup on return types that cannot round-trip: interfaces (including `any` and `map[string]any`), unexported fields, `json:"-"`, func, chan, complex, one-way marshalers. For a dynamic shape, return `json.RawMessage`. A value JSON cannot encode (NaN, cycles) is not cached. JSON also loses invalid UTF-8, a time zone's name, and an empty slice or map under `omitempty`.
- **`Set` and `Get` on those paths are unchecked:** unexported fields are lost, and numbers return as `float64`, losing precision above 2^53.
- **Redis names:** a name is shared by every process on that Redis and key prefix, and binding is per `Cache`, so services sharing Redis need distinct `KeyPrefix` values. A field added to a return struct under a kept name reads as zero until TTL; bump the name. A field renamed or removed is a miss.
- **Identity:** a name binds one `Wrap`, and only a top-level function may `Wrap` under it again. Wrap once, at setup. For a closure or method value wrapped per request, use `WithKeyFunc`; unnamed, it never hits. A closure or method value under `WithName` panics on its second `Wrap`.
- **Test the backend you ship:** refusals apply only on Redis and compression, so a program tested on memory can panic at its first start on Redis. Name every `Wrap` if any environment uses Redis, and `Wrap` at startup.
- **Context:** ctx is not in the key. Anything that varies by user belongs in the arguments.
- **Trust:** anyone who can write to your Redis can forge any return value. Keys up to 64 bytes hold arguments in clear; a hash above that is not secrecy. A custom key function must be injective and namespaced. `MarshalText` on an argument type must be injective.
- **Errors:** `WithErrorCaching` panics at `Wrap` on Redis or compression.
- **Concurrency:** stampede protection is per process. A function with no error return cannot honor a waiter's ctx. `OnEvict` does not fire on Redis.
- **Outages:** a Redis error is a miss, and `Wrap` drops a failed write; there is no error stat or hook. With Redis down, every call runs the function after waiting on Redis, about a second with a client obcache builds.
- **Operations:** a client you pass in needs `ContextTimeoutEnabled` and its own timeouts, and is the route to TLS. `New` pings a client it builds; pass a `Client` to skip that. `Stats` and `Len` scan the prefix on Redis. `DebugHandler` has no authentication.

## Examples

See [examples/](examples/) for complete programs:
- [Basic usage](examples/basic/main.go)
- [Redis caching](examples/redis-cache/main.go)
- [Compression](examples/compression/main.go)
- [Prometheus metrics](examples/prometheus/main.go)
- [Gin web server integration](examples/gin-web-server/main.go)

Upgrading from 2.0.x: see [CHANGELOG.md](CHANGELOG.md).

## License

MIT License - see [LICENSE](LICENSE) file.
