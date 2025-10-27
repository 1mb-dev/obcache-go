package memory

import (
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/vnykmshr/obcache-go/internal/entry"
	"github.com/vnykmshr/obcache-go/internal/store"
)

// Store implements an in-memory LRU cache with TTL support
type Store struct {
	cache           *lru.Cache[string, *entry.Entry]
	mutex           sync.RWMutex
	evictCallback   store.EvictCallback
	cleanupCallback store.EvictCallback
	cleanupTicker   *time.Ticker
	stopCleanup     chan struct{}
	capacity        int
}

// New creates a new memory store with the specified capacity
func New(capacity int) (*Store, error) {
	s := &Store{
		capacity:    capacity,
		stopCleanup: make(chan struct{}),
	}

	// Create cache with eviction callback
	cache, err := lru.NewWithEvict[string, *entry.Entry](capacity, func(key string, entry *entry.Entry) {
		if s.evictCallback != nil {
			s.evictCallback(key, entry.Value)
		}
	})
	if err != nil {
		return nil, err
	}

	s.cache = cache
	return s, nil
}

// NewWithCleanup creates a new memory store with automatic TTL cleanup
func NewWithCleanup(capacity int, cleanupInterval time.Duration) (*Store, error) {
	s, err := New(capacity)
	if err != nil {
		return nil, err
	}

	if cleanupInterval > 0 {
		s.startCleanup(cleanupInterval)
	}

	return s, nil
}

// Get retrieves an entry by key
func (s *Store) Get(key string) (*entry.Entry, bool) {
	s.mutex.RLock()
	entry, found := s.cache.Get(key)
	if !found {
		s.mutex.RUnlock()
		return nil, false
	}

	// Check if entry has expired
	if entry.IsExpired() {
		// Upgrade to write lock to remove expired entry synchronously
		s.mutex.RUnlock()
		s.mutex.Lock()

		// Double-check after acquiring write lock (race condition protection)
		entry, found = s.cache.Peek(key)
		if found && entry.IsExpired() {
			s.cache.Remove(key)
			// Call cleanup callback if set
			if s.cleanupCallback != nil {
				s.cleanupCallback(key, entry.Value)
			}
		}
		s.mutex.Unlock()
		return nil, false
	}

	// Touch the entry for LRU
	entry.Touch()
	s.mutex.RUnlock()
	return entry, true
}

// Set stores an entry with the given key
func (s *Store) Set(key string, entry *entry.Entry) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.cache.Add(key, entry)
	return nil
}

// Delete removes an entry by key
func (s *Store) Delete(key string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.cache.Remove(key)
	return nil
}

// Keys returns all keys currently in the store
func (s *Store) Keys() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	keys := s.cache.Keys()
	// Filter out expired keys
	validKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if entry, found := s.cache.Peek(key); found && !entry.IsExpired() {
			validKeys = append(validKeys, key)
		}
	}

	return validKeys
}

// Len returns the current number of entries in the store
func (s *Store) Len() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Count only non-expired entries
	count := 0
	keys := s.cache.Keys()
	for _, key := range keys {
		if entry, found := s.cache.Peek(key); found && !entry.IsExpired() {
			count++
		}
	}

	return count
}

// Clear removes all entries from the store
func (s *Store) Clear() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.cache.Purge()
	return nil
}

// Close closes the store and cleans up resources
func (s *Store) Close() error {
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
	}

	close(s.stopCleanup)
	return s.Clear()
}

// SetEvictCallback sets the callback for LRU evictions
func (s *Store) SetEvictCallback(callback store.EvictCallback) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.evictCallback = callback
}

// SetCleanupCallback sets the callback for TTL cleanup
func (s *Store) SetCleanupCallback(callback store.EvictCallback) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.cleanupCallback = callback
}

// Capacity returns the maximum number of entries the store can hold
func (s *Store) Capacity() int {
	return s.capacity
}

// Cleanup removes expired entries and returns the number of entries removed
func (s *Store) Cleanup() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	keys := s.cache.Keys()
	removed := 0

	for _, key := range keys {
		if entry, found := s.cache.Peek(key); found && entry.IsExpired() {
			s.cache.Remove(key)
			removed++

			if s.cleanupCallback != nil {
				s.cleanupCallback(key, entry.Value)
			}
		}
	}

	return removed
}

// startCleanup starts the automatic cleanup goroutine
func (s *Store) startCleanup(interval time.Duration) {
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

// Ensure Store implements the required interfaces
var (
	_ store.Store    = (*Store)(nil)
	_ store.LRUStore = (*Store)(nil)
	_ store.TTLStore = (*Store)(nil)
)
