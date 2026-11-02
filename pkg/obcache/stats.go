package obcache

import (
	"sync/atomic"
)

// Stats holds cache performance statistics
type Stats struct {
	// Hits is the number of cache hits
	hits int64

	// Misses is the number of cache misses
	misses int64

	// Evictions is the number of evicted entries
	evictions int64

	// Invalidations is the number of manually invalidated entries
	invalidations int64

	// KeyCount is the current number of keys in the cache
	keyCount int64

	// InFlight is the number of requests currently being processed (singleflight)
	inFlight int64

	// decodeFailures is the number of stored entries that could not be read back
	decodeFailures int64
}

// DecodeFailures returns how many stored entries could not be read back. Each one
// counts as a miss, so a rising count with a steady hit rate means entries are being
// recomputed: check for a changed return type under a kept name.
func (s *Stats) DecodeFailures() int64 {
	return atomic.LoadInt64(&s.decodeFailures)
}

func (s *Stats) incDecodeFailures() {
	atomic.AddInt64(&s.decodeFailures, 1)
}

// Hits returns the number of cache hits
func (s *Stats) Hits() int64 {
	return atomic.LoadInt64(&s.hits)
}

// Misses returns the number of cache misses
func (s *Stats) Misses() int64 {
	return atomic.LoadInt64(&s.misses)
}

// Evictions returns the number of evicted entries
func (s *Stats) Evictions() int64 {
	return atomic.LoadInt64(&s.evictions)
}

// Invalidations returns the number of manually invalidated entries
func (s *Stats) Invalidations() int64 {
	return atomic.LoadInt64(&s.invalidations)
}

// KeyCount returns the current number of keys in the cache
func (s *Stats) KeyCount() int64 {
	return atomic.LoadInt64(&s.keyCount)
}

// InFlight returns the number of requests currently in flight
func (s *Stats) InFlight() int64 {
	return atomic.LoadInt64(&s.inFlight)
}

// HitRate returns the cache hit rate as a percentage (0-100)
func (s *Stats) HitRate() float64 {
	hits := s.Hits()
	misses := s.Misses()
	total := hits + misses

	if total == 0 {
		return 0
	}

	return float64(hits) / float64(total) * 100
}

// Total returns the total number of cache requests (hits + misses)
func (s *Stats) Total() int64 {
	return s.Hits() + s.Misses()
}

// Reset resets all statistics to zero
func (s *Stats) Reset() {
	atomic.StoreInt64(&s.hits, 0)
	atomic.StoreInt64(&s.misses, 0)
	atomic.StoreInt64(&s.evictions, 0)
	atomic.StoreInt64(&s.invalidations, 0)
	atomic.StoreInt64(&s.keyCount, 0)
	atomic.StoreInt64(&s.inFlight, 0)
	atomic.StoreInt64(&s.decodeFailures, 0)
}

// Internal methods for updating stats (not exported)

func (s *Stats) incHits() {
	atomic.AddInt64(&s.hits, 1)
}

func (s *Stats) incMisses() {
	atomic.AddInt64(&s.misses, 1)
}

func (s *Stats) incEvictions() {
	atomic.AddInt64(&s.evictions, 1)
}

func (s *Stats) incInvalidations() {
	atomic.AddInt64(&s.invalidations, 1)
}

func (s *Stats) setKeyCount(count int64) {
	atomic.StoreInt64(&s.keyCount, count)
}

func (s *Stats) incInFlight() {
	atomic.AddInt64(&s.inFlight, 1)
}

func (s *Stats) decInFlight() {
	atomic.AddInt64(&s.inFlight, -1)
}
