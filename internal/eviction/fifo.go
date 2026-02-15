package eviction

import (
	"sync"

	"github.com/1mb-dev/obcache-go/internal/entry"
)

// FIFOStrategy implements the FIFO (First In, First Out) eviction strategy
type FIFOStrategy struct {
	data     map[string]*entry.Entry
	order    []string // Keys in insertion order
	capacity int
	mutex    sync.RWMutex
}

// NewFIFOStrategy creates a new FIFO eviction strategy
func NewFIFOStrategy(capacity int) *FIFOStrategy {
	return &FIFOStrategy{
		data:     make(map[string]*entry.Entry),
		order:    make([]string, 0, capacity),
		capacity: capacity,
	}
}

// Add adds an entry to the FIFO tracker
func (f *FIFOStrategy) Add(key string, entry *entry.Entry) (string, *entry.Entry, bool) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	// If key already exists, update it without changing order
	if _, exists := f.data[key]; exists {
		f.data[key] = entry
		return "", nil, false
	}

	// If we're at capacity, evict the first item (oldest)
	if len(f.data) >= f.capacity && f.capacity > 0 {
		evictKey := f.order[0]
		evictedEntry := f.data[evictKey]
		f.order = f.order[1:] // Remove first element
		delete(f.data, evictKey)

		f.data[key] = entry
		f.order = append(f.order, key)
		return evictKey, evictedEntry, true
	}

	// Add new entry
	f.data[key] = entry
	f.order = append(f.order, key)
	return "", nil, false
}

// Get retrieves an entry (no ordering change in FIFO)
func (f *FIFOStrategy) Get(key string) (*entry.Entry, bool) {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	entry, found := f.data[key]
	return entry, found
}

// Remove removes an entry from the FIFO tracker
func (f *FIFOStrategy) Remove(key string) bool {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	if _, exists := f.data[key]; exists {
		delete(f.data, key)

		// Remove from order slice
		for i, k := range f.order {
			if k == key {
				f.order = append(f.order[:i], f.order[i+1:]...)
				break
			}
		}
		return true
	}
	return false
}

// Contains checks if a key exists in the FIFO tracker
func (f *FIFOStrategy) Contains(key string) bool {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	_, exists := f.data[key]
	return exists
}

// Keys returns all keys currently tracked by the FIFO strategy
func (f *FIFOStrategy) Keys() []string {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	// Return a copy of the order slice to maintain insertion order
	keys := make([]string, len(f.order))
	copy(keys, f.order)
	return keys
}

// Len returns the number of entries currently tracked
func (f *FIFOStrategy) Len() int {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	return len(f.data)
}

// Clear removes all entries from the FIFO tracker
func (f *FIFOStrategy) Clear() {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.data = make(map[string]*entry.Entry)
	f.order = f.order[:0] // Clear slice but keep capacity
}

// Capacity returns the maximum number of entries this strategy can hold
func (f *FIFOStrategy) Capacity() int {
	return f.capacity
}

// Peek retrieves an entry without any side effects
func (f *FIFOStrategy) Peek(key string) (*entry.Entry, bool) {
	f.mutex.RLock()
	defer f.mutex.RUnlock()

	entry, found := f.data[key]
	return entry, found
}
