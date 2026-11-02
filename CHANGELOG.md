# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Fixed

- Redis: concurrent `Set` with `Keys`, `Len` or `Stats` no longer deadlocks
- Redis: a cache hit no longer writes the entry back, which could restore a key another instance deleted
- Redis: `GetContext`, `SetContext` and wrapped calls honor context deadlines, and a client obcache builds uses short timeouts, so a hung server no longer stalls callers for seconds
- A write to Redis or a hook no longer runs under a cache-wide lock: `Set` issues one command instead of scanning every key, and hooks may call back into the cache
- `Close` can be called more than once
- A wrapped call's result is cached after a concurrent miss, and after the caller's deadline passes
- A wrapped function that panics no longer hangs later calls for that key; waiters retry instead of inheriting the failure
- A cancelled caller no longer fails other callers waiting on the same key, and with `WithErrorCaching` its cancellation is not cached
- A waiter stops waiting when its own context ends
- `Wrap` handles a nil value in an interface return, functions returning only `error`, and variadic functions
- Default cache keys encode every argument in full: `time.Time` values, long slices, large maps and structs with many fields no longer share keys, and `[16]byte` arguments no longer panic
- Wrapped functions no longer share entries by default: two functions on one cache could return each other's values, or panic on differing return types
- `New` returns an error for an unknown eviction type instead of silently using LRU
- Redis: `Close` no longer deletes shared entries, and it closes a client obcache built
- Redis: key listing and `Clear` use SCAN with an escaped prefix, so a prefix containing a glob character cannot reach another application's keys
- `Wrap` over Redis or compression decodes a hit into the declared return type, so integers, structs, pointers, slices and multi-return functions no longer panic on a hit, and large integers stay exact
- Redis with compression now hits: entries record their format, and an entry that cannot be read is a miss rather than a wrong value
- A compression config with `Level` unset now compresses; level 0 used to store the value uncompressed, larger than plain
- Decompression is limited to `compression.MaxDecompressedSize`, so a small entry in a shared Redis cannot exhaust memory
- Prometheus counters report true totals; each export used to add the running total again, so counters grew with every reporting interval

### Security

- Default cache keys could collide across arguments, so a call could return another caller's cached value, and a caller-controlled string could reach such an entry in some shapes. Keys are now unambiguous. Affects v2.0.2 and `github.com/vnykmshr/obcache-go` v1.0.0 to v1.1.0, which retract cannot reach. Upgrade to v2.1.0.

### Deprecated

- `SimpleKeyFunc` is not injective; use `DefaultKeyFunc`

### Added

- `WithName` gives a wrapped function a stable identity, and `Cache.Key` returns the key its entry is stored under, for use with `Delete`
- `EvictionLRU`, `EvictionLFU` and `EvictionFIFO` select an eviction strategy without importing an internal package
- `Stats.DecodeFailures` counts entries that could not be read back

### Changed

- `ValidateWrappableFunction` now reports what `Wrap` refuses: it accepts functions with no results and multi-return functions without an error, and rejects parameter types the default key cannot encode
- `Wrap` panics on a function with no results: caching it ran it once per key and silently skipped its effects after. `ValidateWrappableFunction` now reports what `Wrap` refuses, and accepts multi-return functions without an error
- Redis: the default key prefix is `obcache2:`, so a v2.0.x instance and a v2.1.0 instance do not read or wipe each other's entries during a rollout
- On Redis or with compression, `Wrap` panics at startup on return types that cannot round-trip through JSON, and on `WithErrorCaching`. On Redis it also needs `WithName` or `WithKeyFunc`. Each message names the fix
- `New` returns an error for a Redis config with neither `Addr` nor `Client`; it used to dial localhost. `NewRedisConfig(addr).WithRedis(...)` hits this, because `WithRedis` replaces the config
- `DebugHandler` no longer serves keys or values by default, because it has no authentication; `WithDebugValues` restores them
- Importing `pkg/obcache` no longer links the Prometheus client; `pkg/metrics` keeps the exporter, with the same exported API. `%T` and `reflect.PkgPath` of the shared metric types now show `internal/metricsapi`
- Prometheus: `obcache_evictions_total` uses `reason="unspecified"`, because exported totals do not separate capacity from TTL evictions

## [2.0.0] - 2025-10-27

### Major Simplification & Breaking Changes

This release significantly simplifies the codebase by removing over-engineered abstractions and consolidating implementations. **Net reduction: 3,057 lines of code (-92%).**

**⚠️ BREAKING CHANGES:**

### Memory Store Consolidation

**Changed:**
- Removed dual memory store implementations
- Unified on `StrategyStore` for all eviction strategies (LRU/LFU/FIFO)
- `Strategy.Add()` signature changed to return evicted entry:
  ```go
  // Before
  Add(key string, entry *entry.Entry) (evictKey string, evicted bool)

  // After
  Add(key string, entry *entry.Entry) (evictKey string, evictedEntry *entry.Entry, evicted bool)
  ```

**Impact:**
- Only affects custom Strategy implementations (internal API)
- Eviction callbacks now receive actual evicted values instead of placeholders
- Better observability for eviction events

### Metrics System Changes

**Removed:**
- OpenTelemetry support completely removed
- `pkg/metrics/opentelemetry.go` deleted (351 lines)
- All OpenTelemetry dependencies removed from go.mod

**Kept:**
- Prometheus metrics exporter (fully functional)
- `metrics.Exporter` interface for custom implementations
- All existing Prometheus integrations work unchanged

**Migration Path:**
```go
// If you were using OpenTelemetry, switch to Prometheus:
promConfig := &metrics.PrometheusConfig{
    Registry: prometheus.DefaultRegisterer,
}
exporter, _ := metrics.NewPrometheusExporter(metricsConfig, promConfig)

// Or implement custom exporter:
type CustomExporter struct{}
func (e *CustomExporter) ExportStats(stats metrics.Stats, labels metrics.Labels) error {
    // Your implementation
}
```

### Examples Consolidation

**Removed Examples (6):**
- `advanced` - Used deprecated hooks API
- `batch-processing` - Too specific/niche
- `debug` - Utility, not core feature
- `echo-web-server` - Redundant with Gin
- `opentelemetry` - OpenTelemetry removed
- `metrics` - Used OpenTelemetry

**Kept & Updated (5 core examples):**
- ✅ `basic` - Getting started
- ✅ `compression` - Data compression feature
- ✅ `redis-cache` - Redis backend integration
- ✅ `prometheus` - Metrics with Prometheus
- ✅ `gin-web-server` - Web framework integration

All examples updated to use context-aware hooks API.

### Improvements

**Code Quality:**
- Removed 3,057 lines of redundant code
- Single, well-tested memory store implementation
- Cleaner dependency tree (no OpenTelemetry deps)
- Better eviction callback semantics
- More focused, maintainable examples

**Documentation:**
- Updated all package documentation
- Fixed outdated API examples
- Added eviction strategy documentation
- Clarified metrics integration options

**Performance:**
- Zero performance regressions
- Eviction callbacks more efficient (no goroutine spawn)
- Reduced memory allocations in hot paths

### Testing

- ✅ All tests pass with `-race` detector
- ✅ Zero test regressions
- ✅ All 5 examples build successfully
- Coverage maintained at ~45%

### Migration Guide

**For Strategy Implementers:**
If you implemented a custom eviction strategy, update the `Add()` method:
```go
func (s *CustomStrategy) Add(key string, entry *entry.Entry) (string, *entry.Entry, bool) {
    // Capture evicted entry before deletion
    if needsEviction {
        evictedEntry := s.data[evictKey]
        delete(s.data, evictKey)
        return evictKey, evictedEntry, true
    }
    return "", nil, false
}
```

**For OpenTelemetry Users:**
Switch to Prometheus or implement the `metrics.Exporter` interface for your preferred backend.

**For Example Users:**
Review the 5 core examples - they demonstrate all key features with updated APIs.

---

## [1.1.0] - 2025-10-27

### New Features

**Context Propagation Support:**
- Add `GetContext()` and `SetContext()` methods for context-aware cache operations
- Enable proper timeout and cancellation propagation throughout the cache layer
- Support distributed tracing through context in cache operations
- Propagate context through function wrapping (`Wrap`) for better observability
- Maintain full backward compatibility with existing `Get()`/`Set()` methods

### Bug Fixes

**Critical:**
- Fix goroutine leak in memory store expired entry cleanup
  - Replace unbounded goroutine spawning with synchronous lock upgrade pattern
  - Implement double-check locking for race condition safety
  - Prevents resource exhaustion under high load with many expired entries

### Dependency Updates

**Major Updates:**
- Upgrade Redis client: `v9.6.3` → `v9.16.0` (10 minor versions)
  - Includes maintenance notification support, trace filtering improvements
  - Bug fixes and performance enhancements across 10 releases
- All transitive dependencies updated to Go 1.23-compatible versions

### CI/CD Improvements

**Quality Gates:**
- Add example build validation to prevent broken documentation
- Implement coverage threshold enforcement (45% minimum)
- Improve CI configuration for better reliability
- Add automated example compilation checks

**Technical Details:**
- All changes tested with full test suite (100+ tests)
- Race detector validated on all concurrent operations
- Zero breaking changes - fully backward compatible

## [1.0.3] - 2025-09-16

### Code Quality & Production Readiness

**Improvements:**
- Pre-production code cleanup and standardization
- Remove development artifacts (12MB binary cleanup)
- Consolidate test constants into centralized `testutil.go`
- Standardize code formatting across all example files
- Fix unused imports and resolve all lint issues
- Improve struct field alignment and code consistency

**Technical Enhancements:**
- Add centralized test constants: `TestTTL`, `TestSlowOperation`, `TestMetricsReportInterval`
- Standardize time duration usage in tests and examples
- Clean dependency management and `go.sum` optimization
- Ensure consistent indentation and professional code style

**Quality Assurance:**
- All tests pass with clean build pipeline
- Production-ready codebase with improved maintainability
- Code now reads as unified, professionally developed library
- Ready for enterprise deployment

## [1.0.0] - 2025-08-27

### Initial Release

High-performance, thread-safe caching library for Go.

**Core Features:**
- Function wrapping with `obcache.Wrap()`
- TTL support with automatic cleanup
- LRU/LFU/FIFO eviction strategies
- Thread-safe operations
- Redis backend support
- Compression (gzip/deflate)
- Statistics and monitoring hooks

**API:**
- `cache.Get(key)` / `cache.Set(key, value, ttl)`
- `obcache.Wrap(cache, function, options...)`
- Memory and Redis backends
- Prometheus metrics

See [README.md](README.md) for usage examples.