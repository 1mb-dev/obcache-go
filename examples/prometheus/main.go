package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vnykmshr/obcache-go/pkg/obcache"
)

// Prometheus metrics
var (
	cacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "obcache_hits_total",
		Help: "The total number of cache hits",
	})

	cacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "obcache_misses_total",
		Help: "The total number of cache misses",
	})

	cacheEvictions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "obcache_evictions_total",
		Help: "The total number of cache evictions by reason",
	}, []string{"reason"})

	cacheInvalidations = promauto.NewCounter(prometheus.CounterOpts{
		Name: "obcache_invalidations_total",
		Help: "The total number of cache invalidations",
	})

	cacheHitRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "obcache_hit_rate_percent",
		Help: "Current cache hit rate as percentage",
	})

	cacheKeyCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "obcache_keys_current",
		Help: "Current number of keys in cache",
	})

	cacheOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "obcache_operation_duration_seconds",
			Help:    "Time spent on cache operations",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		},
		[]string{"operation", "status"},
	)
)

// PrometheusCache wraps obcache with Prometheus metrics
type PrometheusCache struct {
	cache *obcache.Cache
}

func NewPrometheusCache(config *obcache.Config) (*PrometheusCache, error) {
	// Create hooks for real-time metric collection
	hooks := &obcache.Hooks{}
	hooks.AddOnHit(func(ctx context.Context, key string, value any) {
		cacheHits.Inc()
	})
	hooks.AddOnMiss(func(ctx context.Context, key string) {
		cacheMisses.Inc()
	})
	hooks.AddOnEvict(func(ctx context.Context, key string, value any, reason obcache.EvictReason) {
		cacheEvictions.WithLabelValues(reason.String()).Inc()
	})
	hooks.AddOnInvalidate(func(ctx context.Context, key string) {
		cacheInvalidations.Inc()
	})

	// Apply hooks to config
	config = config.WithHooks(hooks)

	cache, err := obcache.New(config)
	if err != nil {
		return nil, err
	}

	pc := &PrometheusCache{cache: cache}

	// Start periodic metrics update
	go pc.updateMetrics()

	return pc, nil
}

func (pc *PrometheusCache) Get(key string) (any, bool) {
	start := time.Now()
	value, found := pc.cache.Get(key)
	duration := time.Since(start).Seconds()

	status := "miss"
	if found {
		status = "hit"
	}

	cacheOperationDuration.WithLabelValues("get", status).Observe(duration)
	return value, found
}

func (pc *PrometheusCache) Set(key string, value any, ttl time.Duration) error {
	start := time.Now()
	err := pc.cache.Set(key, value, ttl)
	duration := time.Since(start).Seconds()

	status := "success"
	if err != nil {
		status = "error"
	}

	cacheOperationDuration.WithLabelValues("set", status).Observe(duration)
	return err
}

func (pc *PrometheusCache) Invalidate(key string) error {
	start := time.Now()
	err := pc.cache.Delete(key)
	duration := time.Since(start).Seconds()

	status := "success"
	if err != nil {
		status = "error"
	}

	cacheOperationDuration.WithLabelValues("invalidate", status).Observe(duration)
	return err
}

func (pc *PrometheusCache) Stats() *obcache.Stats {
	return pc.cache.Stats()
}

func (pc *PrometheusCache) updateMetrics() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := pc.cache.Stats()
		cacheHitRate.Set(stats.HitRate())
		cacheKeyCount.Set(float64(stats.KeyCount()))
	}
}

// Simulate application workload
func simulateWorkload(cache *PrometheusCache) {
	keys := []string{"user:1", "user:2", "user:3", "product:100", "product:200", "config:app"}

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			// Random operations
			switch rand.Intn(10) {
			case 0, 1, 2, 3, 4, 5: // 60% get operations
				key := keys[rand.Intn(len(keys))]
				_, _ = cache.Get(key)
			case 6, 7: // 20% set operations
				key := keys[rand.Intn(len(keys))]
				value := map[string]any{
					"data":      fmt.Sprintf("value_%d", rand.Intn(1000)),
					"timestamp": time.Now().Unix(),
				}
				_ = cache.Set(key, value, time.Duration(rand.Intn(300))*time.Second)
			case 8: // 10% invalidate operations
				key := keys[rand.Intn(len(keys))]
				_ = cache.Invalidate(key)
			case 9: // 10% miss operations (non-existent keys)
				key := fmt.Sprintf("missing:%d", rand.Intn(100))
				_, _ = cache.Get(key)
			}
		}
	}()
}

func main() {
	fmt.Println("🚀 Starting obcache-go Prometheus integration example")

	// Create cache with Prometheus metrics
	config := obcache.NewDefaultConfig().
		WithMaxEntries(100).
		WithDefaultTTL(5 * time.Minute).
		WithCleanupInterval(time.Minute)

	cache, err := NewPrometheusCache(config)
	if err != nil {
		log.Fatalf("Failed to create cache: %v", err)
	}

	// Add some initial data
	fmt.Println("📦 Populating cache with initial data...")
	_ = cache.Set("user:1", map[string]any{"name": "Alice", "role": "admin"}, time.Hour)
	_ = cache.Set("user:2", map[string]any{"name": "Bob", "role": "user"}, time.Hour)
	_ = cache.Set("config:app", map[string]any{"version": "1.0.0", "debug": true}, 30*time.Minute)

	// Start workload simulation
	fmt.Println("🔄 Starting workload simulation...")
	simulateWorkload(cache)

	// Start metrics server
	http.Handle("/metrics", promhttp.Handler())

	// Add custom cache info endpoint
	http.HandleFunc("/cache/info", func(w http.ResponseWriter, r *http.Request) {
		stats := cache.Stats()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
  "hits": %d,
  "misses": %d,
  "hit_rate": %.2f,
  "key_count": %d,
  "evictions": %d,
  "invalidations": %d
}`, stats.Hits(), stats.Misses(), stats.HitRate(), stats.KeyCount(), stats.Evictions(), stats.Invalidations())
	})

	fmt.Println("\n📊 Prometheus metrics server started on :2112")
	fmt.Println("Endpoints:")
	fmt.Println("  http://localhost:2112/metrics    - Prometheus metrics")
	fmt.Println("  http://localhost:2112/cache/info - Cache statistics JSON")

	fmt.Println("\n📈 Example Prometheus queries:")
	fmt.Println("  # Hit rate: obcache_hit_rate_percent")
	fmt.Println("  # Request rate: rate(obcache_hits_total[1m]) + rate(obcache_misses_total[1m])")
	fmt.Println("  # Eviction rate: rate(obcache_evictions_total[1m])")
	fmt.Println("  # Operation latency: obcache_operation_duration_seconds")

	fmt.Println("\n🛑 Press Ctrl+C to stop")

	// Print periodic stats
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			stats := cache.Stats()
			fmt.Printf("📊 Stats: Hits=%d, Misses=%d, Hit Rate=%.1f%%, Keys=%d\n",
				stats.Hits(), stats.Misses(), stats.HitRate(), stats.KeyCount())
		}
	}()

	log.Fatal(http.ListenAndServe(":2112", nil))
}
