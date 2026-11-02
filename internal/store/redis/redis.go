package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/1mb-dev/obcache-go/v2/internal/entry"
	"github.com/1mb-dev/obcache-go/v2/internal/store"
)

const scanBatch = 1000

var globReplacer = strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`, `]`, `\]`)

// Store implements a Redis-backed cache store
type Store struct {
	client          redis.Cmdable
	keyPrefix       string
	defaultTTL      time.Duration
	evictCallback   store.EvictCallback
	cleanupCallback store.EvictCallback
	ctx             context.Context
	closeClient     func() error
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

	// CloseClient releases a client the store owns. Nil for a caller's client.
	CloseClient func() error
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
		keyPrefix = "obcache2:"
	}

	s := &Store{
		client:      config.Client,
		keyPrefix:   keyPrefix,
		defaultTTL:  config.DefaultTTL,
		ctx:         ctx,
		closeClient: config.CloseClient,
	}

	return s, nil
}

// Get retrieves an entry by key
func (s *Store) Get(ctx context.Context, key string) (*entry.Entry, bool) {
	redisKey := s.buildKey(key)
	result := s.client.Get(ctx, redisKey)
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
		s.client.Del(ctx, redisKey)
		return nil, false
	}

	// Check if entry has expired
	if entry.IsExpired() {
		// Remove expired entry
		s.client.Del(ctx, redisKey)

		// Call cleanup callback if set
		if s.cleanupCallback != nil {
			go s.cleanupCallback(key, entry.Value)
		}
		return nil, false
	}

	return entry, true
}

// Set stores an entry with the given key
func (s *Store) Set(ctx context.Context, key string, entry *entry.Entry) error {
	redisKey := s.buildKey(key)
	return s.saveEntryToRedis(ctx, redisKey, entry)
}

// Delete removes an entry by key
func (s *Store) Delete(key string) error {
	redisKey := s.buildKey(key)
	return s.client.Del(s.ctx, redisKey).Err()
}

// Keys returns all keys currently in the store
func (s *Store) Keys() []string {
	redisKeys, err := s.scan()
	if err != nil {
		return []string{}
	}

	cacheKeys := make([]string, 0, len(redisKeys))
	for _, redisKey := range redisKeys {
		if cacheKey := s.extractKey(redisKey); cacheKey != "" {
			cacheKeys = append(cacheKeys, cacheKey)
		}
	}

	return cacheKeys
}

// scan lists this store's keys. The prefix is escaped so it cannot act as a
// pattern, and results are filtered because SCAN MATCH is the server's reading of
// it. A key under a longer prefix that starts with this one still belongs here.
func (s *Store) scan() ([]string, error) {
	var keys []string
	iter := s.client.Scan(s.ctx, 0, escapeGlob(s.keyPrefix)+"*", scanBatch).Iterator()
	for iter.Next(s.ctx) {
		if strings.HasPrefix(iter.Val(), s.keyPrefix) {
			keys = append(keys, iter.Val())
		}
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	// SCAN may return a key more than once.
	slices.Sort(keys)
	return slices.Compact(keys), nil
}

func escapeGlob(s string) string {
	return globReplacer.Replace(s)
}

// Len returns the current number of entries in the store
func (s *Store) Len() int {
	return len(s.Keys())
}

// Clear removes all entries from the store
func (s *Store) Clear() error {
	keys, err := s.scan()
	if err != nil {
		return err
	}

	for chunk := range slices.Chunk(keys, scanBatch) {
		if err := s.client.Del(s.ctx, chunk...).Err(); err != nil {
			return err
		}
	}

	return nil
}

// Close releases a client the store owns. Entries stay: other instances share them.
func (s *Store) Close() error {
	if s.closeClient != nil {
		return s.closeClient()
	}
	return nil
}

// SetEvictCallback sets the callback for evictions (not applicable for Redis)
func (s *Store) SetEvictCallback(callback store.EvictCallback) {
	s.evictCallback = callback
}

// SetCleanupCallback sets the callback for TTL cleanup
func (s *Store) SetCleanupCallback(callback store.EvictCallback) {
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
func (s *Store) saveEntryToRedis(ctx context.Context, redisKey string, e *entry.Entry) error {
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
			return s.client.Del(ctx, redisKey).Err()
		}
		redisTTL = remaining
	} else if s.defaultTTL > 0 {
		// Use default TTL if no expiry set
		redisTTL = s.defaultTTL
	}

	if redisTTL > 0 {
		return s.client.SetEx(ctx, redisKey, string(data), redisTTL).Err()
	}
	return s.client.Set(ctx, redisKey, string(data), 0).Err()
}

// Ensure Store implements the required interfaces
var (
	_ store.Store    = (*Store)(nil)
	_ store.TTLStore = (*Store)(nil)
)
