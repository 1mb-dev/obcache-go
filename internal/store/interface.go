package store

import (
	"github.com/1mb-dev/obcache-go/internal/entry"
)

// Store defines the interface for cache storage backends
// This abstraction allows for different implementations (memory, Redis, etc.)
type Store interface {
	// Get retrieves an entry by key
	// Returns the entry and true if found, nil and false if not found
	Get(key string) (*entry.Entry, bool)

	// Set stores an entry with the given key
	// Returns an error if the operation fails
	Set(key string, entry *entry.Entry) error

	// Delete removes an entry by key
	// Returns an error if the operation fails
	Delete(key string) error

	// Keys returns all keys currently in the store
	Keys() []string

	// Len returns the current number of entries in the store
	Len() int

	// Clear removes all entries from the store
	// Returns an error if the operation fails
	Clear() error

	// Close closes the store and cleans up resources
	// Should be called when the cache is no longer needed
	Close() error
}

// EvictCallback is called when an entry is evicted from the store
// This allows the cache to track evictions and invoke hooks
type EvictCallback func(key string, value any)

// LRUStore extends Store with LRU-specific functionality
type LRUStore interface {
	Store

	// SetEvictCallback sets a callback function that will be called
	// when entries are evicted due to LRU policy
	SetEvictCallback(callback EvictCallback)

	// Capacity returns the maximum number of entries the store can hold
	Capacity() int
}

// TTLStore extends Store with TTL cleanup functionality
type TTLStore interface {
	Store

	// Cleanup removes expired entries
	// Returns the number of entries removed
	Cleanup() int

	// SetCleanupCallback sets a callback function that will be called
	// when entries are removed during cleanup
	SetCleanupCallback(callback EvictCallback)
}
