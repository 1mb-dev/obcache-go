# Changelog

All notable changes to this project will be documented in this file.

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
- Prometheus/OpenTelemetry metrics

See [README.md](README.md) for usage examples.