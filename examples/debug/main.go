package main

import (
	"fmt"
	"log"
	"time"

	"github.com/vnykmshr/obcache-go/pkg/obcache"
)

func main() {
	// Create cache with custom configuration
	config := obcache.NewDefaultConfig().
		WithMaxEntries(100).
		WithDefaultTTL(5 * time.Minute).
		WithCleanupInterval(time.Minute)

	cache, err := obcache.New(config)
	if err != nil {
		log.Fatalf("Failed to create cache: %v", err)
	}
	defer func() {
		if err := cache.Close(); err != nil {
			log.Printf("Error closing cache: %v", err)
		}
	}()

	// Populate cache with some data
	fmt.Println("Populating cache with test data...")
	_ = cache.Set("user:1", map[string]any{
		"name":  "Alice",
		"email": "alice@example.com",
	}, time.Hour)

	_ = cache.Set("user:2", map[string]any{
		"name":  "Bob",
		"email": "bob@example.com",
	}, 30*time.Minute)

	_ = cache.Set("config:app", map[string]any{
		"version": "1.0.0",
		"debug":   true,
	}, 10*time.Minute)

	// Warmup some entries (uses DefaultTTL)
	_ = cache.Warmup("session:abc123", "authenticated")
	_ = cache.Warmup("feature:flags", []string{"new-ui", "analytics"})

	// Generate some cache activity for statistics
	_, _ = cache.Get("user:1")       // hit
	_, _ = cache.Get("user:2")       // hit
	_, _ = cache.Get("missing")      // miss
	_, _ = cache.Get("also-missing") // miss

	// Create debug server
	server := cache.NewDebugServer(":8080")

	fmt.Println("\n🚀 Cache debug server started on http://localhost:8080")
	fmt.Println("\nAvailable endpoints:")
	fmt.Println("  GET http://localhost:8080/stats - Cache statistics only")
	fmt.Println("  GET http://localhost:8080/keys  - Statistics + all keys with metadata")
	fmt.Println("  GET http://localhost:8080/      - Same as /keys (default)")

	fmt.Println("\nExample responses:")
	fmt.Println("📊 Stats endpoint will show:")
	fmt.Println(`  {
    "stats": {
      "hits": 2,
      "misses": 2, 
      "hitRate": 50.0,
      "keyCount": 5,
      "config": {
        "maxEntries": 100,
        "defaultTTL": "5m0s",
        "cleanupInterval": "1m0s"
      }
    }
  }`)

	fmt.Println("\n🔑 Keys endpoint will additionally show:")
	fmt.Println(`  {
    "keys": [
      {
        "key": "user:1",
        "value": {"name": "Alice", "email": "alice@example.com"},
        "expiresAt": "2024-01-01T15:00:00Z",
        "createdAt": "2024-01-01T14:00:00Z", 
        "age": "5m30s",
        "ttl": "54m30s"
      }
    ]
  }`)

	fmt.Println("\n⏹️  Press Ctrl+C to stop the server")

	// Start server
	log.Fatal(server.ListenAndServe())
}
