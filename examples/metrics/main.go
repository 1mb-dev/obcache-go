package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"go.opentelemetry.io/otel"

	"github.com/vnykmshr/obcache-go/pkg/metrics"
	"github.com/vnykmshr/obcache-go/pkg/obcache"
)

// User represents a simple user struct for demonstration
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Simulate an expensive function that we want to cache and monitor
func fetchUserData(userID int) (*User, error) {
	// Simulate database latency
	time.Sleep(50 * time.Millisecond)

	user := &User{
		ID:    userID,
		Name:  fmt.Sprintf("User %d", userID),
		Email: fmt.Sprintf("user%d@example.com", userID),
	}

	return user, nil
}

func main() {
	fmt.Println("🚀 obcache-go Metrics Integration Example")
	fmt.Println("=========================================")

	// Example 1: Prometheus metrics
	fmt.Println("\n1. Setting up Prometheus Metrics")
	prometheusExample()

	// Example 2: OpenTelemetry metrics
	fmt.Println("\n2. Setting up OpenTelemetry Metrics")
	opentelemetryExample()

	// Example 3: Multiple exporters
	fmt.Println("\n3. Using Multiple Metrics Exporters")
	multiExporterExample()

	fmt.Println("\n✨ All metrics examples completed!")
	fmt.Println("📊 Check http://localhost:8080/metrics for Prometheus metrics")
}

func prometheusExample() {
	// Create a custom Prometheus registry to avoid conflicts
	registry := prometheus.NewRegistry()

	// Create Prometheus metrics configuration
	prometheusConfig := &metrics.PrometheusConfig{
		Registry: registry,
		DefaultLabels: prometheus.Labels{
			"service": "cache-demo",
			"version": "1.0",
		},
	}

	// Create metrics config
	metricsConfig := metrics.NewDefaultConfig().
		WithNamespace("demo").
		WithDetailedTimings(true).
		WithKeyValueSizes(false) // Disable for performance

	// Create Prometheus exporter
	prometheusExporter, err := metrics.NewPrometheusExporter(metricsConfig, prometheusConfig)
	if err != nil {
		log.Fatalf("Failed to create Prometheus exporter: %v", err)
	}

	// Create cache with Prometheus metrics
	cacheConfig := obcache.NewDefaultConfig().
		WithDefaultTTL(5 * time.Minute).
		WithMetrics(&obcache.MetricsConfig{
			Exporter:  prometheusExporter,
			Enabled:   true,
			CacheName: "user-cache",
		})

	cache, err := obcache.New(cacheConfig)
	if err != nil {
		log.Fatalf("Failed to create cache: %v", err)
	}
	defer func() {
		if err := cache.Close(); err != nil {
			log.Printf("Error closing cache: %v", err)
		}
	}()

	// Wrap function with caching
	cachedFetchUser := obcache.Wrap(cache, fetchUserData)

	// Generate some cache activity for metrics
	fmt.Println("📈 Generating cache activity for Prometheus metrics...")
	for i := 1; i <= 10; i++ {
		userID := (i % 3) + 1 // This will cause some cache hits

		start := time.Now()
		user, err := cachedFetchUser(userID)
		duration := time.Since(start)

		if err != nil {
			log.Printf("Error fetching user: %v", err)
			continue
		}

		fmt.Printf("  User %d: %s (took %v)\n", user.ID, user.Name, duration)
	}

	// Show cache statistics
	stats := cache.Stats()
	fmt.Printf("📊 Cache Stats - Hits: %d, Misses: %d, Hit Rate: %.1f%%\n",
		stats.Hits(), stats.Misses(), stats.HitRate())

	// Start Prometheus HTTP server in background
	go func() {
		http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
		fmt.Println("📊 Prometheus metrics server started on :8080/metrics")
		server := &http.Server{
			Addr:         ":8080",
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		if err := server.ListenAndServe(); err != nil {
			log.Printf("Prometheus server error: %v", err)
		}
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)
}

func opentelemetryExample() {
	// Create OpenTelemetry meter
	// In a real application, you would set up a proper meter provider
	meterProvider := otel.GetMeterProvider()
	meter := meterProvider.Meter("cache-demo")

	// Create OpenTelemetry exporter
	otelConfig := &metrics.OpenTelemetryConfig{
		Meter:   meter,
		Context: context.Background(),
	}

	metricsConfig := metrics.NewDefaultConfig().
		WithNamespace("demo_otel").
		WithDetailedTimings(true).
		WithLabels(metrics.Labels{
			"service":     "cache-demo",
			"environment": "development",
		})

	otelExporter, err := metrics.NewOpenTelemetryExporter(metricsConfig, otelConfig)
	if err != nil {
		log.Fatalf("Failed to create OpenTelemetry exporter: %v", err)
	}

	// Create cache with OpenTelemetry metrics
	cacheConfig := obcache.NewDefaultConfig().
		WithDefaultTTL(10 * time.Minute).
		WithMetrics(&obcache.MetricsConfig{
			Exporter:  otelExporter,
			Enabled:   true,
			CacheName: "product-cache",
		})

	cache, err := obcache.New(cacheConfig)
	if err != nil {
		log.Fatalf("Failed to create cache with OTel metrics: %v", err)
	}
	defer func() {
		if err := cache.Close(); err != nil {
			log.Printf("Error closing cache: %v", err)
		}
	}()

	// Simulate cache operations
	fmt.Println("📊 Generating cache activity for OpenTelemetry metrics...")

	// Set some values
	for i := 1; i <= 5; i++ {
		if err := cache.Set(fmt.Sprintf("product:%d", i), map[string]any{
			"id":    i,
			"name":  fmt.Sprintf("Product %d", i),
			"price": float64(i) * 10.99,
		}, time.Hour); err != nil {
			log.Printf("Error setting cache: %v", err)
		}
	}

	// Get some values (mix of hits and misses)
	for i := 1; i <= 8; i++ {
		key := fmt.Sprintf("product:%d", i)

		if value, found := cache.Get(key); found {
			product := value.(map[string]any)
			fmt.Printf("  Found %s: $%.2f\n", product["name"], product["price"])
		} else {
			fmt.Printf("  Product %d not found in cache\n", i)
		}
	}

	// Wait a bit for automatic metrics reporting
	fmt.Println("⏱️  Waiting for automatic metrics reporting...")
	time.Sleep(6 * time.Second)

	stats := cache.Stats()
	fmt.Printf("📊 OTel Cache Stats - Hits: %d, Misses: %d, Hit Rate: %.1f%%\n",
		stats.Hits(), stats.Misses(), stats.HitRate())
}

func multiExporterExample() {
	// Create both Prometheus and OpenTelemetry exporters
	registry := prometheus.NewRegistry()

	promConfig := &metrics.PrometheusConfig{
		Registry:      registry,
		DefaultLabels: prometheus.Labels{"exporter": "multi"},
	}

	metricsConfig := metrics.NewDefaultConfig()
	promExporter, _ := metrics.NewPrometheusExporter(metricsConfig, promConfig)

	// Create a simple OpenTelemetry exporter (in real use, you'd configure a proper provider)
	meter := otel.GetMeterProvider().Meter("multi-demo")
	otelConfig := &metrics.OpenTelemetryConfig{
		Meter:   meter,
		Context: context.Background(),
	}
	otelExporter, _ := metrics.NewOpenTelemetryExporter(metricsConfig, otelConfig)

	// Create multi-exporter
	multiExporter := metrics.NewMultiExporter(promExporter, otelExporter)

	// Create cache with multi-exporter
	cacheConfig := obcache.NewDefaultConfig().
		WithMetrics(&obcache.MetricsConfig{
			Exporter:  multiExporter,
			Enabled:   true,
			CacheName: "multi-cache",
			Labels: metrics.Labels{
				"component": "multi-example",
				"tier":      "cache",
			},
		})

	cache, err := obcache.New(cacheConfig)
	if err != nil {
		log.Fatalf("Failed to create multi-metrics cache: %v", err)
	}
	defer func() {
		if err := cache.Close(); err != nil {
			log.Printf("Error closing cache: %v", err)
		}
	}()

	fmt.Println("📊 Generating activity for multiple exporters...")

	// Generate cache activity
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("item:%d", i%5) // Create overlapping keys for hits
		value := fmt.Sprintf("Item %d Data", i)

		// Set every 4th item
		if i%4 == 0 {
			if err := cache.Set(key, value, 30*time.Second); err != nil {
				log.Printf("Error setting cache: %v", err)
			}
		}

		// Try to get every item
		if val, found := cache.Get(key); found {
			fmt.Printf("  ✓ Cache hit for %s: %s\n", key, val)
		} else {
			fmt.Printf("  ✗ Cache miss for %s\n", key)
		}
	}

	// Final stats
	stats := cache.Stats()
	fmt.Printf("📊 Multi-exporter Stats - Hits: %d, Misses: %d, Hit Rate: %.1f%%\n",
		stats.Hits(), stats.Misses(), stats.HitRate())

	fmt.Println("✅ Metrics exported to both Prometheus and OpenTelemetry!")
}
