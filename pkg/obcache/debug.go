package obcache

import (
	"encoding/json"
	"net/http"
	"time"
)

const expiredTTL = "expired"

// DebugResponse represents the JSON response structure for debug endpoints
type DebugResponse struct {
	Stats *DebugStats `json:"stats"`
	Keys  []DebugKey  `json:"keys,omitempty"`
}

// DebugStats represents cache statistics in the debug response
type DebugStats struct {
	Hits          int64        `json:"hits"`
	Misses        int64        `json:"misses"`
	Evictions     int64        `json:"evictions"`
	Invalidations int64        `json:"invalidations"`
	KeyCount      int64        `json:"keyCount"`
	InFlight      int64        `json:"inFlight"`
	HitRate       float64      `json:"hitRate"`
	Total         int64        `json:"total"`
	Config        *DebugConfig `json:"config"`
}

// DebugConfig represents cache configuration in the debug response
type DebugConfig struct {
	MaxEntries      int           `json:"maxEntries"`
	DefaultTTL      time.Duration `json:"defaultTTL"`
	CleanupInterval time.Duration `json:"cleanupInterval"`
}

// DebugKey represents a cache key with its metadata
type DebugKey struct {
	Key       string     `json:"key"`
	Value     any        `json:"value,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	Age       string     `json:"age"`
	TTL       string     `json:"ttl,omitempty"`
}

// DebugHandler returns an HTTP handler that provides cache debug information
// The handler supports the following endpoints:
//   - GET /stats - Returns only cache statistics (no keys)
//   - GET /keys - Returns statistics and all cache keys with metadata
//   - GET / - Returns statistics and all cache keys with metadata (same as /keys)
func (c *Cache) DebugHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		var response DebugResponse
		includeKeys := r.URL.Path == "/" || r.URL.Path == "/keys"

		// Collect statistics
		response.Stats = &DebugStats{
			Hits:          c.stats.Hits(),
			Misses:        c.stats.Misses(),
			Evictions:     c.stats.Evictions(),
			Invalidations: c.stats.Invalidations(),
			KeyCount:      c.stats.KeyCount(),
			InFlight:      c.stats.InFlight(),
			HitRate:       c.stats.HitRate(),
			Total:         c.stats.Total(),
			Config: &DebugConfig{
				MaxEntries:      c.config.MaxEntries,
				DefaultTTL:      c.config.DefaultTTL,
				CleanupInterval: c.config.CleanupInterval,
			},
		}

		// Collect keys if requested
		if includeKeys {
			c.mu.RLock()
			keys := c.store.Keys()
			response.Keys = make([]DebugKey, 0, len(keys))

			for _, key := range keys {
				if entry, found := c.store.Get(r.Context(), key); found {
					debugKey := DebugKey{
						Key:       key,
						Value:     entry.Value,
						ExpiresAt: entry.ExpiresAt,
						CreatedAt: entry.CreatedAt,
						Age:       formatDuration(entry.Age()),
					}

					if entry.HasExpiry() {
						ttl := entry.TTL()
						if ttl > 0 {
							debugKey.TTL = formatDuration(ttl)
						} else {
							debugKey.TTL = expiredTTL
						}
					}

					response.Keys = append(response.Keys, debugKey)
				}
			}
			c.mu.RUnlock()
		}

		// Write JSON response
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError)
		}
	})
}

// NewDebugServer creates a new HTTP server with cache debug endpoints
// The server serves on the following routes:
//   - GET /stats - Cache statistics only
//   - GET /keys - Cache statistics and keys
//   - GET / - Cache statistics and keys (default)
func (c *Cache) NewDebugServer(addr string) *http.Server {
	mux := http.NewServeMux()
	handler := c.DebugHandler()

	mux.Handle("/stats", handler)
	mux.Handle("/keys", handler)
	mux.Handle("/", handler)

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
}

// formatDuration formats a duration in a human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Microsecond {
		return d.String()
	}
	if d < time.Millisecond {
		return d.Truncate(time.Microsecond).String()
	}
	if d < time.Second {
		return d.Truncate(time.Millisecond).String()
	}
	if d < time.Minute {
		return d.Truncate(time.Second).String()
	}
	if d < time.Hour {
		return d.Truncate(time.Minute).String()
	}
	return d.Truncate(time.Hour).String()
}
