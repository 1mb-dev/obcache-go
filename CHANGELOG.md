# Changelog

All notable changes to this project will be documented in this file.

## [2.1.0] - 2026-11-02

Install with `go get github.com/1mb-dev/obcache-go/v2`. Upgrading from 2.0.x: read Behavior Changes first.

### Security

- Default cache keys could collide across functions and arguments, so a call could return another caller's cached value, and a caller-controlled string could reach such an entry in some shapes. Affects v2.0.2 and `github.com/vnykmshr/obcache-go` v1.0.0 to v1.1.0, which retract cannot reach. Upgrade to v2.1.0.
- `DebugHandler` served keys and plaintext values with no authentication; see Behavior Changes.
- v2.0.0 to v2.0.2 are retracted.

### Behavior Changes

Some 2.0.x code now panics at `Wrap` or errors at `New`. Each message names the fix. Tests on the memory store will not catch the Redis and compression changes: construct your production config in a test, and `Wrap` at startup.

**All backends**

- The default key format changed, and default keys now include the function's identity, so wrapped functions no longer share entries. `Delete(DefaultKeyFunc(args))` and `Delete(myKeyGen(args))` silently stop matching a wrapped entry; use `Cache.Key`
- `Wrap` panics on func, chan and unexported-field parameters unless a custom key function is set, because the default key cannot encode them. This includes `func(*sql.DB, int)`, which worked with one database; use `WithKeyFunc`
- `DefaultKeyFunc` panics on arguments with no encoding when called directly. Passed to `WithKeyFunc` or `WithKeyGenFunc`, such a call runs uncached
- `Cache.Key` panics on an unbound name or mismatched arguments
- A closure or method value under `WithName` panics on its second `Wrap`, which can be at runtime if `Wrap` runs per request; use `WithKeyFunc` there. Without a name, closures and method values wrapped per call go cold and leave entries behind
- `New` returns an error for an unknown eviction type instead of silently using LRU
- `ValidateWrappableFunction` now reports what `Wrap` refuses: it accepts functions with no results and multi-return functions without an error, and rejects parameter types the default key cannot encode
- A function returning only `error` is now wrapped as one: nil is cached, and a non-nil error is not, unless asked. This covers a sole pointer-typed error return
- `DebugHandler` and `NewDebugServer` no longer serve keys or values by default, because they have no authentication; `WithDebugValues` restores them

**Redis or compression**

- `Wrap` panics at startup on return types that cannot round-trip through JSON: interfaces, including `any`, `[]any` and `map[string]any`, which ran on 2.0.x; unexported fields; `json:"-"`; func; chan; complex; one-way marshalers. Return a concrete type or `json.RawMessage`
- `WithErrorCaching` panics at `Wrap`
- Compression now compresses, so it costs CPU it did not before: `Level` unset used to mean no compression
- The stored entry gained a `format` field. An entry without one is a miss and is left in place

**Redis**

- Every `Wrap` needs `WithName` or `WithKeyFunc`. A per-request `Wrap` migrates to `WithKeyFunc`, not `WithName`
- `NewRedisConfig(addr).WithRedis(...)` now errors at `New`, because `WithRedis` replaces the config and it used to dial localhost. Assign `config.Redis.KeyPrefix` instead
- The default key prefix is `obcache2:`, so v2.0.x and v2.1.0 instances do not read or wipe each other's entries. With an explicit `KeyPrefix` they share a namespace, and a 2.0.x instance's `Close` wipes it: change `KeyPrefix` in the same deploy, or drain old instances first. Either way, 2.0.x entries are cold
- `Close` no longer clears Redis
- Entries written with no TTL under an old prefix never expire; remove them with `SCAN` and `UNLINK`
- Rolling back: code using `WithName` or `Cache.Key` does not compile against 2.0.x

**Prometheus**

- Counters drop to true values, so dashboards and alert thresholds shift
- `obcache_evictions_total` uses `reason="unspecified"`, because exported totals do not separate capacity from TTL evictions
- `WithKeyValueSizes` has no effect
- `pkg/obcache` no longer imports `pkg/metrics`, so importing it alone does not link Prometheus. `pkg/metrics` keeps every exported name; `%T` and `reflect.PkgPath` of the shared types now show `internal/metricsapi`

### Fixed

- Memory store: reading an expired entry no longer removes a fresh entry written under the same key a moment later
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
- Redis: `Close` no longer deletes shared entries, and it closes a client obcache built
- Redis: key listing and `Clear` use SCAN with an escaped prefix, so a prefix containing a glob character cannot reach another application's keys
- `Wrap` over Redis or compression decodes a hit into the declared return type, so integers, structs, pointers, slices and multi-return functions no longer panic on a hit, and large integers stay exact
- Redis with compression now hits: entries record their format, and an entry that cannot be read is a miss rather than a wrong value
- A compression config with `Level` unset now compresses; level 0 used to store the value uncompressed, larger than plain
- Decompression is limited to `compression.MaxDecompressedSize`, so a small entry in a shared Redis cannot exhaust memory
- Prometheus counters report true totals; each export used to add the running total again, so counters grew with every reporting interval

### Added

- `WithName` gives a wrapped function a stable identity, and `Cache.Key` returns the key its entry is stored under, for use with `Delete`
- `EvictionLRU`, `EvictionLFU` and `EvictionFIFO` select an eviction strategy without importing an internal package
- `Stats.DecodeFailures` counts entries that could not be read back

### Deprecated

- `SimpleKeyFunc` is not injective; use `DefaultKeyFunc`
- `metrics.WithKeyValueSizes`, `IncludeKeyValueSizes`, `SizeBuckets` and the `CacheOperationsTotal`, `CacheErrorsTotal`, `CacheKeySize` and `CacheValueSize` names: these metrics were registered but never written, and are no longer registered
- `WrapSimple`, `WrapWithError`, `WrapFunc0`, `WrapFunc1`, `WrapFunc2` and their `WithError` forms: `Wrap` covers them. The `TestTTL` and `ExampleTTL` constants: define your own

## [2.0.2] - 2026-02-15

### Fixed

- Module path gains `/v2`. This is the first version importable as `github.com/1mb-dev/obcache-go/v2`

## [2.0.1] - 2026-02-15

### Changed

- Repository moved to `github.com/1mb-dev/obcache-go`. The module path had no `/v2`, so this version cannot be imported as v2

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