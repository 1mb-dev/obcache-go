package memory

import (
	"sync"
	"time"

	"github.com/vnykmshr/obcache-go/internal/entry"
	"github.com/vnykmshr/obcache-go/internal/eviction"
	"github.com/vnykmshr/obcache-go/internal/store"
)

// StrategyStore implements an in-memory cache with pluggable eviction strategies
type StrategyStore struct {
	strategy        eviction.Strategy
	mutex           sync.RWMutex
	evictCallback   store.EvictCallback
	cleanupCallback store.EvictCallback
	cleanupTicker   *time.Ticker
	stopCleanup     chan struct{}
}

// NewWithStrategy creates a new memory store with the specified eviction strategy
func NewWithStrategy(config eviction.Config) (*StrategyStore, error) {
	strategy := eviction.NewStrategy(config)

	s := &StrategyStore{
		strategy:    strategy,
		stopCleanup: make(chan struct{}),
	}

	return s, nil
}

// NewWithStrategyAndCleanup creates a new memory store with eviction strategy and automatic TTL cleanup
func NewWithStrategyAndCleanup(config eviction.Config, cleanupInterval time.Duration) (*StrategyStore, error) {
	s, err := NewWithStrategy(config)
	if err != nil {
		return nil, err
	}

	if cleanupInterval > 0 {
		s.startCleanup(cleanupInterval)
	}

	return s, nil
}

// Get retrieves an entry by key
func (s *StrategyStore) Get(key string) (*entry.Entry, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	entry, found := s.strategy.Get(key)
	if !found {
		return nil, false
	}

	// Check if entry has expired
	if entry.IsExpired() {
		// Remove expired entry (do this in a separate goroutine to avoid deadlock)
		go func() {
			s.mutex.Lock()
			s.strategy.Remove(key)
			s.mutex.Unlock()

			if s.cleanupCallback != nil {
				s.cleanupCallback(key, entry.Value)
			}
		}()
		return nil, false
	}

	// Touch the entry for TTL tracking
	entry.Touch()
	return entry, true
}

// Set stores an entry with the given key
func (s *StrategyStore) Set(key string, entry *entry.Entry) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	evictedKey, evictedEntry, wasEvicted := s.strategy.Add(key, entry)

	// Call eviction callback if an entry was evicted
	if wasEvicted && s.evictCallback != nil && evictedKey != "" && evictedEntry != nil {
		s.evictCallback(evictedKey, evictedEntry.Value)
	}

	return nil
}

// Delete removes an entry by key
func (s *StrategyStore) Delete(key string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.strategy.Remove(key)
	return nil
}

// Keys returns all keys currently in the store
func (s *StrategyStore) Keys() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	keys := s.strategy.Keys()
	// Filter out expired keys
	validKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if entry, found := s.strategy.Peek(key); found && !entry.IsExpired() {
			validKeys = append(validKeys, key)
		}
	}

	return validKeys
}

// Len returns the current number of entries in the store
func (s *StrategyStore) Len() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Count only non-expired entries
	count := 0
	keys := s.strategy.Keys()
	for _, key := range keys {
		if entry, found := s.strategy.Peek(key); found && !entry.IsExpired() {
			count++
		}
	}

	return count
}

// Clear removes all entries from the store
func (s *StrategyStore) Clear() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.strategy.Clear()
	return nil
}

// Close closes the store and cleans up resources
func (s *StrategyStore) Close() error {
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}

	close(s.stopCleanup)
	return s.Clear()
}

// SetEvictCallback sets the callback for evictions
func (s *StrategyStore) SetEvictCallback(callback store.EvictCallback) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.evictCallback = callback
}

// SetCleanupCallback sets the callback for TTL cleanup
func (s *StrategyStore) SetCleanupCallback(callback store.EvictCallback) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.cleanupCallback = callback
}

// Capacity returns the maximum number of entries the store can hold
func (s *StrategyStore) Capacity() int {
	return s.strategy.Capacity()
}

// Cleanup removes expired entries and returns the number of entries removed
func (s *StrategyStore) Cleanup() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	keys := s.strategy.Keys()
	removed := 0

	for _, key := range keys {
		if entry, found := s.strategy.Peek(key); found && entry.IsExpired() {
			s.strategy.Remove(key)
			removed++

			if s.cleanupCallback != nil {
				s.cleanupCallback(key, entry.Value)
			}
		}
	}

	return removed
}

// startCleanup starts the automatic cleanup goroutine
func (s *StrategyStore) startCleanup(interval time.Duration) {
	s.cleanupTicker = time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-s.cleanupTicker.C:
				s.Cleanup()
			case <-s.stopCleanup:
				return
			}
		}
	}()
}

// GetEvictionType returns the eviction strategy type (convenience method for debugging)
func (s *StrategyStore) GetEvictionType() string {
	switch s.strategy.(type) {
	case *eviction.LRUStrategy:
		return string(eviction.LRU)
	case *eviction.LFUStrategy:
		return string(eviction.LFU)
	case *eviction.FIFOStrategy:
		return string(eviction.FIFO)
	default:
		return "unknown"
	}
}

// Ensure StrategyStore implements the required interfaces
var (
	_ store.Store    = (*StrategyStore)(nil)
	_ store.LRUStore = (*StrategyStore)(nil)
	_ store.TTLStore = (*StrategyStore)(nil)
)
