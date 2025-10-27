# Changelog

All notable changes to this project will be documented in this file.

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