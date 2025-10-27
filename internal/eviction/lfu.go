package eviction

import (
	"sync"

	"github.com/vnykmshr/obcache-go/internal/entry"
)

// LFUStrategy implements the LFU (Least Frequently Used) eviction strategy
type LFUStrategy struct {
	data        map[string]*entry.Entry
	frequencies map[string]int
	capacity    int
	mutex       sync.RWMutex
}

// NewLFUStrategy creates a new LFU eviction strategy
func NewLFUStrategy(capacity int) *LFUStrategy {
	return &LFUStrategy{
		data:        make(map[string]*entry.Entry),
		frequencies: make(map[string]int),
		capacity:    capacity,
	}
}

// Add adds an entry to the LFU tracker
func (l *LFUStrategy) Add(key string, entry *entry.Entry) (string, *entry.Entry, bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// If key already exists, update it
	if _, exists := l.data[key]; exists {
		l.data[key] = entry
		l.frequencies[key]++
		return "", nil, false
	}

	// If we're at capacity, evict the least frequently used item
	if len(l.data) >= l.capacity {
		evictKey := l.findLFU()
		if evictKey != "" {
			evictedEntry := l.data[evictKey]
			delete(l.data, evictKey)
			delete(l.frequencies, evictKey)
			l.data[key] = entry
			l.frequencies[key] = 1
			return evictKey, evictedEntry, true
		}
	}

	// Add new entry
	l.data[key] = entry
	l.frequencies[key] = 1
	return "", nil, false
}

// Get retrieves an entry and increments its frequency
func (l *LFUStrategy) Get(key string) (*entry.Entry, bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	entry, found := l.data[key]
	if found {
		l.frequencies[key]++
	}
	return entry, found
}

// Remove removes an entry from the LFU tracker
func (l *LFUStrategy) Remove(key string) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if _, exists := l.data[key]; exists {
		delete(l.data, key)
		delete(l.frequencies, key)
		return true
	}
	return false
}

// Contains checks if a key exists in the LFU tracker
func (l *LFUStrategy) Contains(key string) bool {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	_, exists := l.data[key]
	return exists
}

// Keys returns all keys currently tracked by the LFU strategy
func (l *LFUStrategy) Keys() []string {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	keys := make([]string, 0, len(l.data))
	for key := range l.data {
		keys = append(keys, key)
	}
	return keys
}

// Len returns the number of entries currently tracked
func (l *LFUStrategy) Len() int {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	return len(l.data)
}

// Clear removes all entries from the LFU tracker
func (l *LFUStrategy) Clear() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.data = make(map[string]*entry.Entry)
	l.frequencies = make(map[string]int)
}

// Capacity returns the maximum number of entries this strategy can hold
func (l *LFUStrategy) Capacity() int {
	return l.capacity
}

// Peek retrieves an entry without updating its frequency
func (l *LFUStrategy) Peek(key string) (*entry.Entry, bool) {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	entry, found := l.data[key]
	return entry, found
}

// findLFU finds the key with the lowest frequency (internal method, assumes lock is held)
func (l *LFUStrategy) findLFU() string {
	if len(l.data) == 0 {
		return ""
	}

	var lfuKey string
	minFreq := -1

	for key, freq := range l.frequencies {
		if minFreq == -1 || freq < minFreq {
			minFreq = freq
			lfuKey = key
		}
	}

	return lfuKey
}
