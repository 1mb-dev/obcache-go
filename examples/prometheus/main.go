// Prometheus example: the obcache exporter, served by promhttp, with a small workload
// so /metrics has something to show.
package main

import (
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/metrics"
	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func loadProduct(id int) (string, error) {
	time.Sleep(10 * time.Millisecond) // Simulate a query
	return fmt.Sprintf("product-%d", id), nil
}

func main() {
	registry := prometheus.NewRegistry()
	exporter, err := metrics.NewPrometheusExporter(
		metrics.NewDefaultConfig().WithDetailedTimings(true),
		&metrics.PrometheusConfig{Registry: registry},
	)
	if err != nil {
		log.Fatalf("Failed to create exporter: %v", err)
	}

	cache, err := obcache.New(obcache.NewDefaultConfig().
		WithMaxEntries(50).
		WithDefaultTTL(time.Minute).
		WithMetrics(&obcache.MetricsConfig{
			Enabled:           true,
			Exporter:          exporter,
			CacheName:         "products",
			ReportingInterval: 5 * time.Second,
		}))
	if err != nil {
		log.Fatalf("Failed to create cache: %v", err)
	}
	defer func() {
		if err := cache.Close(); err != nil {
			log.Printf("Error closing cache: %v", err)
		}
	}()

	cachedProduct := obcache.Wrap(cache, loadProduct)
	go func() {
		for range time.Tick(20 * time.Millisecond) {
			if _, err := cachedProduct(rand.IntN(100)); err != nil { // #nosec G404 -- demo workload
				log.Printf("Lookup failed: %v", err)
			}
		}
	}()

	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	log.Println("Metrics on http://localhost:8080/metrics")
	server := &http.Server{Addr: ":8080", ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
