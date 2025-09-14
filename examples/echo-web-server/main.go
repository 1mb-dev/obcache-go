// Package main demonstrates using obcache-go with Echo web framework
// for building high-performance APIs with intelligent caching.
//
// This example shows:
// - Multi-layer caching strategy (L1: memory, L2: Redis)
// - Different eviction strategies for different data types
// - Real-world API patterns with caching
// - Middleware integration
// - Error handling and fallback strategies
//
// Run with: go run main.go
// Test with: curl http://localhost:1323/api/products/1
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/vnykmshr/obcache-go/pkg/obcache"
)

// Product represents a product in our catalog
type Product struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Category    string  `json:"category"`
	InStock     bool    `json:"in_stock"`
	UpdatedAt   string  `json:"updated_at"`
}

// ProductAnalytics represents product analytics data
type ProductAnalytics struct {
	ProductID    int     `json:"product_id"`
	ViewCount    int     `json:"view_count"`
	PurchaseCount int     `json:"purchase_count"`
	Rating       float64 `json:"rating"`
	LastViewed   string  `json:"last_viewed"`
}

// ProductService simulates a database service
type ProductService struct {
	products map[int]*Product
	analytics map[int]*ProductAnalytics
}

func NewProductService() *ProductService {
	products := map[int]*Product{
		1: {ID: 1, Name: "Laptop Pro", Description: "High-performance laptop", Price: 1299.99, Category: "Electronics", InStock: true},
		2: {ID: 2, Name: "Coffee Mug", Description: "Ceramic coffee mug", Price: 12.99, Category: "Kitchen", InStock: true},
		3: {ID: 3, Name: "Book: Go Programming", Description: "Learn Go programming", Price: 39.99, Category: "Books", InStock: false},
		4: {ID: 4, Name: "Wireless Headphones", Description: "Premium wireless headphones", Price: 199.99, Category: "Electronics", InStock: true},
	}

	analytics := map[int]*ProductAnalytics{
		1: {ProductID: 1, ViewCount: 1250, PurchaseCount: 85, Rating: 4.7},
		2: {ProductID: 2, ViewCount: 892, PurchaseCount: 203, Rating: 4.3},
		3: {ProductID: 3, ViewCount: 445, PurchaseCount: 67, Rating: 4.9},
		4: {ProductID: 4, ViewCount: 2100, PurchaseCount: 156, Rating: 4.5},
	}

	return &ProductService{
		products:  products,
		analytics: analytics,
	}
}

func (s *ProductService) GetProduct(id int) (*Product, error) {
	// Simulate database latency
	time.Sleep(80 * time.Millisecond)
	
	product, exists := s.products[id]
	if !exists {
		return nil, fmt.Errorf("product not found: %d", id)
	}
	
	// Simulate dynamic updates
	productCopy := *product
	productCopy.UpdatedAt = time.Now().Format(time.RFC3339)
	
	log.Printf("Database query executed for product ID: %d", id)
	return &productCopy, nil
}

func (s *ProductService) GetProductAnalytics(id int) (*ProductAnalytics, error) {
	// Simulate expensive analytics computation
	time.Sleep(200 * time.Millisecond)
	
	analytics, exists := s.analytics[id]
	if !exists {
		return nil, fmt.Errorf("analytics not found for product: %d", id)
	}
	
	// Simulate view count increment
	analyticsCopy := *analytics
	analyticsCopy.ViewCount++
	analyticsCopy.LastViewed = time.Now().Format(time.RFC3339)
	
	log.Printf("Analytics computation executed for product ID: %d", id)
	return &analyticsCopy, nil
}

func (s *ProductService) GetProductsByCategory(category string) ([]*Product, error) {
	// Simulate complex query
	time.Sleep(150 * time.Millisecond)
	
	var results []*Product
	for _, product := range s.products {
		if product.Category == category {
			productCopy := *product
			productCopy.UpdatedAt = time.Now().Format(time.RFC3339)
			results = append(results, &productCopy)
		}
	}
	
	log.Printf("Category query executed for: %s", category)
	return results, nil
}

// CacheManager manages multiple cache instances with different strategies
type CacheManager struct {
	// L1 Cache: Memory-based, fast access for hot data
	l1Cache *obcache.Cache
	
	// L2 Cache: Redis-based, shared across instances, larger capacity
	l2Cache *obcache.Cache
	
	// Analytics Cache: LFU eviction for frequently accessed analytics
	analyticsCache *obcache.Cache
	
	productService *ProductService
	
	// Cached functions
	getProduct           func(int) (*Product, error)
	getProductAnalytics  func(int) (*ProductAnalytics, error)
	getProductsByCategory func(string) ([]*Product, error)
}

func NewCacheManager() (*CacheManager, error) {
	productService := NewProductService()
	
	// L1 Cache: Memory-based with LRU eviction for recent data
	l1Config := obcache.NewDefaultConfig().
		WithMaxEntries(500).
		WithLRUEviction().
		WithDefaultTTL(2 * time.Minute)
	
	l1Cache, err := obcache.New(l1Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create L1 cache: %w", err)
	}

	// L2 Cache: Redis or memory based on environment
	var l2Cache *obcache.Cache
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		l2Config := obcache.NewRedisConfig(redisURL).
			WithRedisKeyPrefix("api:products:").
			WithDefaultTTL(15 * time.Minute)
		l2Cache, err = obcache.New(l2Config)
	} else {
		l2Config := obcache.NewDefaultConfig().
			WithMaxEntries(2000).
			WithLRUEviction().
			WithDefaultTTL(15 * time.Minute)
		l2Cache, err = obcache.New(l2Config)
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to create L2 cache: %w", err)
	}

	// Analytics Cache: LFU eviction for frequently accessed analytics
	analyticsConfig := obcache.NewDefaultConfig().
		WithMaxEntries(1000).
		WithLFUEviction(). // Use LFU for analytics as popular products are accessed frequently
		WithDefaultTTL(30 * time.Minute)
	
	analyticsCache, err := obcache.New(analyticsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create analytics cache: %w", err)
	}

	// Set up cache hooks for monitoring
	setupCacheHooks(l1Cache, "L1")
	setupCacheHooks(l2Cache, "L2") 
	setupCacheHooks(analyticsCache, "Analytics")

	manager := &CacheManager{
		l1Cache:        l1Cache,
		l2Cache:        l2Cache,
		analyticsCache: analyticsCache,
		productService: productService,
	}

	// Create cached functions with different strategies
	manager.getProduct = obcache.Wrap(l1Cache, productService.GetProduct,
		obcache.WithTTL(2*time.Minute),
		obcache.WithKeyFunc(func(args []any) string {
			return fmt.Sprintf("product:%d", args[0].(int))
		}))

	manager.getProductAnalytics = obcache.Wrap(analyticsCache, productService.GetProductAnalytics,
		obcache.WithTTL(30*time.Minute),
		obcache.WithKeyFunc(func(args []any) string {
			return fmt.Sprintf("analytics:%d", args[0].(int))
		}))

	manager.getProductsByCategory = obcache.Wrap(l2Cache, productService.GetProductsByCategory,
		obcache.WithTTL(10*time.Minute),
		obcache.WithKeyFunc(func(args []any) string {
			return fmt.Sprintf("category:%s", args[0].(string))
		}))

	return manager, nil
}

func setupCacheHooks(cache *obcache.Cache, name string) {
	hooks := &obcache.Hooks{}
	hooks.AddOnHit(func(key string, value any) {
		log.Printf("[%s] Cache HIT: %s", name, key)
	})
	hooks.AddOnMiss(func(key string) {
		log.Printf("[%s] Cache MISS: %s", name, key)
	})
	hooks.AddOnEvict(func(key string, value any, reason obcache.EvictReason) {
		log.Printf("[%s] Cache EVICT: %s (reason: %s)", name, key, reason)
	})
}

func (cm *CacheManager) Close() error {
	if err := cm.l1Cache.Close(); err != nil {
		return err
	}
	if err := cm.l2Cache.Close(); err != nil {
		return err
	}
	return cm.analyticsCache.Close()
}

// API Server
type APIServer struct {
	cacheManager *CacheManager
	echo         *echo.Echo
}

func NewAPIServer() (*APIServer, error) {
	cacheManager, err := NewCacheManager()
	if err != nil {
		return nil, err
	}

	e := echo.New()
	
	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	server := &APIServer{
		cacheManager: cacheManager,
		echo:         e,
	}
	
	server.setupRoutes()
	return server, nil
}

func (s *APIServer) setupRoutes() {
	// Add cache timing middleware
	s.echo.Use(s.cacheTimingMiddleware)
	
	api := s.echo.Group("/api")
	
	// Product endpoints
	api.GET("/products/:id", s.getProduct)
	api.GET("/products/:id/analytics", s.getProductAnalytics)
	api.GET("/categories/:category/products", s.getProductsByCategory)
	
	// Cache management
	api.GET("/cache/stats", s.getCacheStats)
	api.DELETE("/cache/:cache_name/clear", s.clearCache)
	api.DELETE("/products/:id/cache", s.invalidateProductCache)
	
	// Health check
	s.echo.GET("/health", s.healthCheck)
}

func (s *APIServer) cacheTimingMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		
		// Add timing context
		c.Set("request_start", start)
		
		err := next(c)
		
		duration := time.Since(start)
		c.Response().Header().Set("X-Response-Time", duration.String())
		
		// Determine if request was likely served from cache
		cached := duration < 50*time.Millisecond
		c.Response().Header().Set("X-Cache-Hit", strconv.FormatBool(cached))
		
		return err
	}
}

func (s *APIServer) getProduct(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid product ID")
	}

	product, err := s.cacheManager.getProduct(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"product": product,
		"cached": true, // This will be overridden by middleware if needed
	})
}

func (s *APIServer) getProductAnalytics(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid product ID")
	}

	analytics, err := s.cacheManager.getProductAnalytics(id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"analytics": analytics,
	})
}

func (s *APIServer) getProductsByCategory(c echo.Context) error {
	category := c.Param("category")
	if category == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Category is required")
	}

	products, err := s.cacheManager.getProductsByCategory(category)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"category": category,
		"products": products,
		"count":    len(products),
	})
}

func (s *APIServer) getCacheStats(c echo.Context) error {
	l1Stats := s.cacheManager.l1Cache.Stats()
	l2Stats := s.cacheManager.l2Cache.Stats()
	analyticsStats := s.cacheManager.analyticsCache.Stats()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"l1_cache": map[string]interface{}{
			"hits":      l1Stats.Hits(),
			"misses":    l1Stats.Misses(),
			"hit_rate":  fmt.Sprintf("%.2f%%", l1Stats.HitRate()),
			"keys":      l1Stats.KeyCount(),
			"capacity":  s.cacheManager.l1Cache.Capacity(),
		},
		"l2_cache": map[string]interface{}{
			"hits":      l2Stats.Hits(),
			"misses":    l2Stats.Misses(),
			"hit_rate":  fmt.Sprintf("%.2f%%", l2Stats.HitRate()),
			"keys":      l2Stats.KeyCount(),
			"capacity":  s.cacheManager.l2Cache.Capacity(),
		},
		"analytics_cache": map[string]interface{}{
			"hits":      analyticsStats.Hits(),
			"misses":    analyticsStats.Misses(),
			"hit_rate":  fmt.Sprintf("%.2f%%", analyticsStats.HitRate()),
			"keys":      analyticsStats.KeyCount(),
			"capacity":  s.cacheManager.analyticsCache.Capacity(),
		},
	})
}

func (s *APIServer) clearCache(c echo.Context) error {
	cacheName := c.Param("cache_name")
	
	var cache *obcache.Cache
	switch cacheName {
	case "l1":
		cache = s.cacheManager.l1Cache
	case "l2":
		cache = s.cacheManager.l2Cache
	case "analytics":
		cache = s.cacheManager.analyticsCache
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid cache name. Use: l1, l2, or analytics")
	}

	if err := cache.Clear(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to clear cache")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Cache '%s' cleared successfully", cacheName),
	})
}

func (s *APIServer) invalidateProductCache(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid product ID")
	}

	// Invalidate from all relevant caches
	productKey := fmt.Sprintf("product:%d", id)
	analyticsKey := fmt.Sprintf("analytics:%d", id)

	if err := s.cacheManager.l1Cache.Delete(productKey); err != nil {
		log.Printf("Error deleting from L1 cache: %v", err)
	}
	if err := s.cacheManager.l2Cache.Delete(productKey); err != nil {
		log.Printf("Error deleting from L2 cache: %v", err)
	}
	if err := s.cacheManager.analyticsCache.Delete(analyticsKey); err != nil {
		log.Printf("Error deleting from analytics cache: %v", err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Cache invalidated for product %d", id),
	})
}

func (s *APIServer) healthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"caches":    "operational",
	})
}

func (s *APIServer) Start(address string) error {
	defer s.cacheManager.Close()
	return s.echo.Start(address)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	server, err := NewAPIServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "1323"
	}

	log.Printf("Starting Echo server on port %s", port)
	log.Printf("Test endpoints:")
	log.Printf("  GET  http://localhost:%s/api/products/1", port)
	log.Printf("  GET  http://localhost:%s/api/products/1/analytics", port)
	log.Printf("  GET  http://localhost:%s/api/categories/Electronics/products", port)
	log.Printf("  GET  http://localhost:%s/api/cache/stats", port)

	if err := server.Start(":" + port); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}