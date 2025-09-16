# Changelog

All notable changes to this project will be documented in this file.

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
- Prometheus/OpenTelemetry metrics

See [README.md](README.md) for usage examples.