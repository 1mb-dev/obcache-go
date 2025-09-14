package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	
	"github.com/vnykmshr/obcache-go/pkg/obcache"
)

var (
	tracer = otel.Tracer("obcache-go-example")
	meter  = otel.Meter("obcache-go-example")
)

// TracedCache wraps obcache with OpenTelemetry observability
type TracedCache struct {
	cache *obcache.Cache
	
	// Metrics
	hitCounter     metric.Int64Counter
	missCounter    metric.Int64Counter
	evictCounter   metric.Int64Counter
	operationTimer metric.Float64Histogram
}

func NewTracedCache(ctx context.Context, config *obcache.Config) (*TracedCache, error) {
	// Initialize metrics
	hitCounter, err := meter.Int64Counter(
		"obcache_hits_total",
		metric.WithDescription("Total number of cache hits"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating hit counter: %w", err)
	}

	missCounter, err := meter.Int64Counter(
		"obcache_misses_total",
		metric.WithDescription("Total number of cache misses"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating miss counter: %w", err)
	}

	evictCounter, err := meter.Int64Counter(
		"obcache_evictions_total",
		metric.WithDescription("Total number of cache evictions"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, fmt.Errorf("creating evict counter: %w", err)
	}

	operationTimer, err := meter.Float64Histogram(
		"obcache_operation_duration_seconds",
		metric.WithDescription("Duration of cache operations"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1),
	)
	if err != nil {
		return nil, fmt.Errorf("creating operation timer: %w", err)
	}

	tc := &TracedCache{
		hitCounter:     hitCounter,
		missCounter:    missCounter,
		evictCounter:   evictCounter,
		operationTimer: operationTimer,
	}

	// Create hooks for automatic metric collection
	hooks := &obcache.Hooks{}
	hooks.AddOnHit(func(key string, value any) {
		tc.hitCounter.Add(context.Background(), 1,
			metric.WithAttributes(
				attribute.String("key_pattern", extractKeyPattern(key)),
			),
		)
	})

	hooks.AddOnMiss(func(key string) {
		tc.missCounter.Add(context.Background(), 1,
			metric.WithAttributes(
				attribute.String("key_pattern", extractKeyPattern(key)),
			),
		)
	})

	hooks.AddOnEvict(func(key string, value any, reason obcache.EvictReason) {
		tc.evictCounter.Add(context.Background(), 1,
			metric.WithAttributes(
				attribute.String("reason", reason.String()),
				attribute.String("key_pattern", extractKeyPattern(key)),
			),
		)
	})

	// Apply hooks and create cache
	config = config.WithHooks(hooks)
	cache, err := obcache.New(config)
	if err != nil {
		return nil, fmt.Errorf("creating cache: %w", err)
	}

	tc.cache = cache
	return tc, nil
}

func (tc *TracedCache) Get(ctx context.Context, key string) (any, bool) {
	ctx, span := tracer.Start(ctx, "cache.Get",
		trace.WithAttributes(
			attribute.String("cache.key", key),
			attribute.String("cache.operation", "get"),
		),
	)
	defer span.End()

	start := time.Now()
	value, found := tc.cache.Get(key)
	duration := time.Since(start).Seconds()

	// Record metrics
	status := "miss"
	if found {
		status = "hit"
	}

	tc.operationTimer.Record(ctx, duration,
		metric.WithAttributes(
			attribute.String("operation", "get"),
			attribute.String("status", status),
			attribute.String("key_pattern", extractKeyPattern(key)),
		),
	)

	// Add span attributes
	span.SetAttributes(
		attribute.Bool("cache.hit", found),
		attribute.String("cache.status", status),
		attribute.Float64("cache.duration_seconds", duration),
		attribute.String("cache.key_pattern", extractKeyPattern(key)),
	)

	if found {
		span.SetAttributes(attribute.String("cache.value_type", fmt.Sprintf("%T", value)))
	}

	return value, found
}

func (tc *TracedCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	ctx, span := tracer.Start(ctx, "cache.Set",
		trace.WithAttributes(
			attribute.String("cache.key", key),
			attribute.String("cache.operation", "set"),
			attribute.Int64("cache.ttl_seconds", int64(ttl.Seconds())),
			attribute.String("cache.value_type", fmt.Sprintf("%T", value)),
		),
	)
	defer span.End()

	start := time.Now()
	err := tc.cache.Set(key, value, ttl)
	duration := time.Since(start).Seconds()

	status := "success"
	if err != nil {
		status = "error"
		span.RecordError(err)
	}

	tc.operationTimer.Record(ctx, duration,
		metric.WithAttributes(
			attribute.String("operation", "set"),
			attribute.String("status", status),
			attribute.String("key_pattern", extractKeyPattern(key)),
		),
	)

	span.SetAttributes(
		attribute.String("cache.status", status),
		attribute.Float64("cache.duration_seconds", duration),
	)

	return err
}

func (tc *TracedCache) WrapFunction(ctx context.Context, key string, ttl time.Duration, fn func() (any, error)) (any, error) {
	ctx, span := tracer.Start(ctx, "cache.WrapFunction",
		trace.WithAttributes(
			attribute.String("cache.key", key),
			attribute.String("cache.operation", "wrap"),
		),
	)
	defer span.End()

	// Check cache first
	if value, found := tc.Get(ctx, key); found {
		span.SetAttributes(
			attribute.String("cache.source", "cache"),
			attribute.Bool("function.executed", false),
		)
		return value, nil
	}

	// Execute function
	span.SetAttributes(attribute.String("cache.source", "function"))
	
	_, funcSpan := tracer.Start(ctx, "wrapped_function")
	start := time.Now()
	
	value, err := fn()
	duration := time.Since(start)
	
	funcSpan.SetAttributes(
		attribute.Float64("function.duration_seconds", duration.Seconds()),
		attribute.Bool("function.error", err != nil),
	)
	
	if err != nil {
		funcSpan.RecordError(err)
	}
	funcSpan.End()

	span.SetAttributes(
		attribute.Bool("function.executed", true),
		attribute.Float64("function.duration_seconds", duration.Seconds()),
	)

	if err != nil {
		span.SetAttributes(attribute.Bool("function.error", true))
		return value, err
	}

	// Cache successful result
	if setErr := tc.Set(ctx, key, value, ttl); setErr != nil {
		span.SetAttributes(attribute.Bool("cache.set_error", true))
	}

	return value, nil
}

func (tc *TracedCache) Stats() *obcache.Stats {
	return tc.cache.Stats()
}

func extractKeyPattern(key string) string {
	// Extract pattern from key, e.g., "user:123" -> "user"
	for i, char := range key {
		if char == ':' {
			return key[:i]
		}
	}
	return "other"
}

func initOpenTelemetry(ctx context.Context) (func(), error) {
	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", "obcache-go-example"),
			attribute.String("service.version", "1.0.0"),
			attribute.String("deployment.environment", "development"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	// Set up trace exporter (stdout for demo)
	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating trace exporter: %w", err)
	}

	// Set up trace provider
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Set up metrics exporter (Prometheus)
	metricExporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("creating metric exporter: %w", err)
	}

	// Set up metric provider
	metricProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(metricExporter),
	)

	// Set global providers
	otel.SetTracerProvider(traceProvider)
	otel.SetMeterProvider(metricProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		if err := traceProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down trace provider: %v", err)
		}
		
		if err := metricProvider.Shutdown(ctx); err != nil {
			log.Printf("Error shutting down metric provider: %v", err)
		}
	}

	return cleanup, nil
}

func simulateWorkload(ctx context.Context, cache *TracedCache) {
	keys := []string{"user:1", "user:2", "user:3", "product:100", "product:200", "config:app"}
	
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Create a span for the operation
				opCtx, span := tracer.Start(ctx, "simulate_operation")
				
				switch rand.Intn(10) {
				case 0, 1, 2, 3, 4: // 50% get operations
					key := keys[rand.Intn(len(keys))]
					_, _ = cache.Get(opCtx, key)
					
				case 5, 6: // 20% set operations
					key := keys[rand.Intn(len(keys))]
					value := map[string]any{
						"data":      fmt.Sprintf("value_%d", rand.Intn(1000)),
						"timestamp": time.Now().Unix(),
					}
					_ = cache.Set(opCtx, key, value, time.Duration(rand.Intn(300))*time.Second)
					
				case 7: // 10% wrap operations
					key := keys[rand.Intn(len(keys))]
					_, _ = cache.WrapFunction(opCtx, key, time.Minute, func() (any, error) {
						// Simulate expensive operation
						time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
						return map[string]any{
							"computed": rand.Intn(1000),
							"timestamp": time.Now().Unix(),
						}, nil
					})
					
				case 8, 9: // 20% miss operations
					key := fmt.Sprintf("missing:%d", rand.Intn(100))
					_, _ = cache.Get(opCtx, key)
				}
				
				span.End()
			}
		}
	}()
}

func main() {
	ctx := context.Background()

	fmt.Println("🚀 Starting obcache-go OpenTelemetry integration example")

	// Initialize OpenTelemetry
	cleanup, err := initOpenTelemetry(ctx)
	if err != nil {
		log.Fatalf("Failed to initialize OpenTelemetry: %v", err)
	}
	defer cleanup()

	// Create cache with OpenTelemetry
	config := obcache.NewDefaultConfig().
		WithMaxEntries(50).
		WithDefaultTTL(2 * time.Minute).
		WithCleanupInterval(30 * time.Second)

	cache, err := NewTracedCache(ctx, config)
	if err != nil {
		log.Fatalf("Failed to create cache: %v", err)
	}

	fmt.Println("📦 Populating cache with initial data...")
	
	// Add some initial data with tracing
	_, rootSpan := tracer.Start(ctx, "initial_data_load")
	_ = cache.Set(ctx, "user:1", map[string]any{"name": "Alice", "role": "admin"}, time.Hour)
	_ = cache.Set(ctx, "user:2", map[string]any{"name": "Bob", "role": "user"}, time.Hour)  
	_ = cache.Set(ctx, "config:app", map[string]any{"version": "1.0.0", "debug": true}, 30*time.Minute)
	rootSpan.End()

	// Start workload simulation
	fmt.Println("🔄 Starting traced workload simulation...")
	workloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	simulateWorkload(workloadCtx, cache)

	// Set up HTTP endpoints
	http.Handle("/metrics", promhttp.Handler())
	
	http.HandleFunc("/cache/info", func(w http.ResponseWriter, r *http.Request) {
		_, span := tracer.Start(r.Context(), "cache_info_handler")
		defer span.End()
		
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
		
		span.SetAttributes(
			attribute.Int64("cache.hits", stats.Hits()),
			attribute.Int64("cache.misses", stats.Misses()),
			attribute.Float64("cache.hit_rate", stats.HitRate()),
		)
	})

	// Demonstration of traced operations
	http.HandleFunc("/demo/get", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			key = "demo:test"
		}
		
		value, found := cache.Get(r.Context(), key)
		
		w.Header().Set("Content-Type", "application/json")
		if found {
			fmt.Fprintf(w, `{"found": true, "value": %v}`, value)
		} else {
			fmt.Fprintf(w, `{"found": false}`)
		}
	})

	fmt.Println("\n📊 OpenTelemetry servers started on :2113")
	fmt.Println("Endpoints:")
	fmt.Println("  http://localhost:2113/metrics     - Prometheus metrics (via OpenTelemetry)")
	fmt.Println("  http://localhost:2113/cache/info  - Cache statistics JSON (traced)")
	fmt.Println("  http://localhost:2113/demo/get    - Demo traced GET operation")

	fmt.Println("\n🔍 Traces are being output to stdout")
	fmt.Println("📈 Metrics are exported via Prometheus format")
	fmt.Println("\n🛑 Press Ctrl+C to stop")

	// Print periodic stats
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-workloadCtx.Done():
				return
			case <-ticker.C:
				stats := cache.Stats()
				fmt.Printf("📊 Stats: Hits=%d, Misses=%d, Hit Rate=%.1f%%, Keys=%d\n",
					stats.Hits(), stats.Misses(), stats.HitRate(), stats.KeyCount())
			}
		}
	}()

	log.Fatal(http.ListenAndServe(":2113", nil))
}