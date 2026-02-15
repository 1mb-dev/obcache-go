package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/1mb-dev/obcache-go/v2/internal/entry"
	"github.com/1mb-dev/obcache-go/v2/internal/store"
)

// Store implements a Redis-backed cache store
type Store struct {
	client          redis.Cmdable
	keyPrefix       string
	defaultTTL      time.Duration
	evictCallback   store.EvictCallback
	cleanupCallback store.EvictCallback
	mu              sync.RWMutex
	ctx             context.Context
}

// Config holds Redis store configuration
type Config struct {
	// Client is the Redis client to use
	Client redis.Cmdable

	// KeyPrefix is prepended to all cache keys to avoid conflicts
	KeyPrefix string

	// DefaultTTL is the default TTL for entries without explicit expiration
	DefaultTTL time.Duration

	// Context for Redis operations
	Context context.Context
}

// SerializedEntry represents an entry as stored in Redis
type SerializedEntry struct {
	Value      json.RawMessage `json:"value"`
	CreatedAt  time.Time       `json:"created_at"`
	ExpiresAt  *time.Time      `json:"expires_at,omitempty"`
	LastAccess time.Time       `json:"last_access"`
}

// New creates a new Redis store with the given configuration
func New(config *Config) (*Store, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("redis client is required")
	}

	ctx := config.Context
	if ctx == nil {
		ctx = context.Background()
	}

	keyPrefix := config.KeyPrefix
	if keyPrefix == "" {
		keyPrefix = "obcache:"
	}

	s := &Store{
		client:     config.Client,
		keyPrefix:  keyPrefix,
		defaultTTL: config.DefaultTTL,
		ctx:        ctx,
	}

	return s, nil
}

// Get retrieves an entry by key
func (s *Store) Get(key string) (*entry.Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	redisKey := s.buildKey(key)
	result := s.client.Get(s.ctx, redisKey)
	if result.Err() != nil {
		if result.Err() == redis.Nil {
			return nil, false // Key not found
		}
		return nil, false // Other Redis errors treated as miss
	}

	data, err := result.Result()
	if err != nil {
		return nil, false
	}

	// Deserialize the entry
	entry, err := s.deserializeEntry([]byte(data))
	if err != nil {
		// If deserialization fails, remove the corrupted key
		s.client.Del(s.ctx, redisKey)
		return nil, false
	}

	// Check if entry has expired
	if entry.IsExpired() {
		// Remove expired entry
		s.client.Del(s.ctx, redisKey)

		// Call cleanup callback if set
		if s.cleanupCallback != nil {
			go s.cleanupCallback(key, entry.Value)
		}
		return nil, false
	}

	// Update last access time and save back to Redis
	entry.Touch()
	_ = s.saveEntryToRedis(redisKey, entry)

	return entry, true
}

// Set stores an entry with the given key
func (s *Store) Set(key string, entry *entry.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	redisKey := s.buildKey(key)
	return s.saveEntryToRedis(redisKey, entry)
}

// Delete removes an entry by key
func (s *Store) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	redisKey := s.buildKey(key)
	return s.client.Del(s.ctx, redisKey).Err()
}

// Keys returns all keys currently in the store
func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pattern := s.buildKey("*")
	result := s.client.Keys(s.ctx, pattern)
	if result.Err() != nil {
		return []string{}
	}

	redisKeys, err := result.Result()
	if err != nil {
		return []string{}
	}

	// Convert Redis keys back to cache keys and filter expired entries
	cacheKeys := make([]string, 0, len(redisKeys))
	for _, redisKey := range redisKeys {
		cacheKey := s.extractKey(redisKey)
		if cacheKey == "" {
			continue
		}

		// Check if the entry is valid (not expired)
		if _, found := s.Get(cacheKey); found {
			cacheKeys = append(cacheKeys, cacheKey)
		}
	}

	return cacheKeys
}

// Len returns the current number of entries in the store
func (s *Store) Len() int {
	return len(s.Keys())
}

// Clear removes all entries from the store
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pattern := s.buildKey("*")
	result := s.client.Keys(s.ctx, pattern)
	if result.Err() != nil {
		return result.Err()
	}

	keys, err := result.Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return s.client.Del(s.ctx, keys...).Err()
	}

	return nil
}

// Close closes the store and cleans up resources
func (s *Store) Close() error {
	// Redis client cleanup is handled externally
	// We just clear our data
	return s.Clear()
}

// SetEvictCallback sets the callback for evictions (not applicable for Redis)
func (s *Store) SetEvictCallback(callback store.EvictCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictCallback = callback
}

// SetCleanupCallback sets the callback for TTL cleanup
func (s *Store) SetCleanupCallback(callback store.EvictCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupCallback = callback
}

// Cleanup removes expired entries (Redis handles TTL automatically)
// This method is provided for interface compatibility but is less useful with Redis
func (s *Store) Cleanup() int {
	// Since Redis handles TTL automatically, we don't need to do explicit cleanup.
	// Return 0 as no manual cleanup is needed.
	return 0
}

// buildKey creates a Redis key with the configured prefix
func (s *Store) buildKey(key string) string {
	return s.keyPrefix + key
}

// extractKey extracts the cache key from a Redis key
func (s *Store) extractKey(redisKey string) string {
	if !strings.HasPrefix(redisKey, s.keyPrefix) {
		return ""
	}
	return strings.TrimPrefix(redisKey, s.keyPrefix)
}

// serializeEntry converts an entry to JSON for Redis storage
func (s *Store) serializeEntry(e *entry.Entry) ([]byte, error) {
	valueBytes, err := json.Marshal(e.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal entry value: %w", err)
	}

	serialized := SerializedEntry{
		Value:      valueBytes,
		CreatedAt:  e.CreatedAt,
		LastAccess: e.AccessedAt,
	}

	if e.HasExpiry() {
		serialized.ExpiresAt = e.ExpiresAt
	}

	return json.Marshal(serialized)
}

// deserializeEntry converts JSON data back to an entry
func (s *Store) deserializeEntry(data []byte) (*entry.Entry, error) {
	var serialized SerializedEntry
	if err := json.Unmarshal(data, &serialized); err != nil {
		return nil, fmt.Errorf("failed to unmarshal serialized entry: %w", err)
	}

	var value any
	if err := json.Unmarshal(serialized.Value, &value); err != nil {
		return nil, fmt.Errorf("failed to unmarshal entry value: %w", err)
	}

	// Create a new entry with current time, then manually set the fields
	var e *entry.Entry
	if serialized.ExpiresAt != nil {
		ttl := serialized.ExpiresAt.Sub(serialized.CreatedAt)
		e = entry.New(value, ttl)
	} else {
		e = entry.NewWithoutTTL(value)
	}

	// Manually restore the timestamps by direct field access
	// Note: This requires the Entry fields to be exported
	e.CreatedAt = serialized.CreatedAt
	e.AccessedAt = serialized.LastAccess
	if serialized.ExpiresAt != nil {
		e.ExpiresAt = serialized.ExpiresAt
	}

	return e, nil
}

// saveEntryToRedis saves an entry to Redis with appropriate TTL
func (s *Store) saveEntryToRedis(redisKey string, e *entry.Entry) error {
	data, err := s.serializeEntry(e)
	if err != nil {
		return err
	}

	// Calculate TTL for Redis
	var redisTTL time.Duration
	if e.HasExpiry() {
		remaining := e.TTL()
		if remaining <= 0 {
			// Entry has already expired
			return s.client.Del(s.ctx, redisKey).Err()
		}
		redisTTL = remaining
	} else if s.defaultTTL > 0 {
		// Use default TTL if no expiry set
		redisTTL = s.defaultTTL
	}

	if redisTTL > 0 {
		return s.client.SetEx(s.ctx, redisKey, string(data), redisTTL).Err()
	}
	return s.client.Set(s.ctx, redisKey, string(data), 0).Err()
}

// Ensure Store implements the required interfaces
var (
	_ store.Store    = (*Store)(nil)
	_ store.TTLStore = (*Store)(nil)
)
