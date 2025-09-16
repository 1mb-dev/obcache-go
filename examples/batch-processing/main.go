// Package main demonstrates using obcache-go for batch processing workloads
// where caching can dramatically improve performance by avoiding redundant computations.
//
// This example shows:
// - Data transformation pipeline with caching
// - Batch processing with cache warm-up
// - Different eviction strategies for different data types
// - Progress monitoring and statistics
// - Error handling in batch operations
//
// Run with: go run main.go
package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vnykmshr/obcache-go/pkg/obcache"
)

// DataRecord represents a data record to be processed
type DataRecord struct {
	ID        int               `json:"id"`
	Category  string            `json:"category"`
	Value     float64           `json:"value"`
	Metadata  map[string]string `json:"metadata"`
	Timestamp time.Time         `json:"timestamp"`
}

// ProcessedResult represents the result of processing a data record
type ProcessedResult struct {
	RecordID         int       `json:"record_id"`
	TransformedValue float64   `json:"transformed_value"`
	CategoryScore    float64   `json:"category_score"`
	Risk             string    `json:"risk"`
	ProcessedAt      time.Time `json:"processed_at"`
	CacheHit         bool      `json:"cache_hit"`
}

// BatchProcessor handles batch processing with intelligent caching
type BatchProcessor struct {
	// Different caches for different types of computations
	transformCache *obcache.Cache // For expensive mathematical transformations
	categoryCache  *obcache.Cache // For category-based computations
	riskCache      *obcache.Cache // For risk assessment computations

	// Cached functions
	transformValue         func(float64, string) (float64, error)
	calculateCategoryScore func(string) (float64, error)
	assessRisk             func(float64, float64) (string, error)

	// Metrics
	totalProcessed int64
	cacheHits      int64
	cacheMisses    int64
	errors         int64
}

func NewBatchProcessor() (*BatchProcessor, error) {
	// Transform cache: LRU for recent computations (mathematical transformations repeat often)
	transformConfig := obcache.NewDefaultConfig().
		WithMaxEntries(10000).
		WithLRUEviction().
		WithDefaultTTL(1 * time.Hour).
		WithCleanupInterval(5 * time.Minute)

	transformCache, err := obcache.New(transformConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create transform cache: %w", err)
	}

	// Category cache: LFU for category computations (popular categories accessed frequently)
	categoryConfig := obcache.NewDefaultConfig().
		WithMaxEntries(1000).
		WithLFUEviction().
		WithDefaultTTL(2 * time.Hour)

	categoryCache, err := obcache.New(categoryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create category cache: %w", err)
	}

	// Risk cache: FIFO for risk assessments (risk calculations change over time)
	riskConfig := obcache.NewDefaultConfig().
		WithMaxEntries(5000).
		WithFIFOEviction().
		WithDefaultTTL(30 * time.Minute)

	riskCache, err := obcache.New(riskConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create risk cache: %w", err)
	}

	processor := &BatchProcessor{
		transformCache: transformCache,
		categoryCache:  categoryCache,
		riskCache:      riskCache,
	}

	// Set up cache hooks for metrics
	processor.setupCacheHooks()

	// Create cached functions
	processor.transformValue = obcache.Wrap(transformCache, processor.expensiveTransform,
		obcache.WithTTL(1*time.Hour),
		obcache.WithKeyFunc(func(args []any) string {
			return fmt.Sprintf("transform:%.6f:%s", args[0].(float64), args[1].(string))
		}))

	processor.calculateCategoryScore = obcache.Wrap(categoryCache, processor.expensiveCategoryComputation,
		obcache.WithTTL(2*time.Hour),
		obcache.WithKeyFunc(func(args []any) string {
			return fmt.Sprintf("category:%s", args[0].(string))
		}))

	processor.assessRisk = obcache.Wrap(riskCache, processor.expensiveRiskAssessment,
		obcache.WithTTL(30*time.Minute),
		obcache.WithKeyFunc(func(args []any) string {
			return fmt.Sprintf("risk:%.6f:%.6f", args[0].(float64), args[1].(float64))
		}))

	return processor, nil
}

func (bp *BatchProcessor) setupCacheHooks() {
	// Transform cache hooks
	transformHooks := &obcache.Hooks{}
	transformHooks.AddOnHit(func(key string, value any) {
		atomic.AddInt64(&bp.cacheHits, 1)
		log.Printf("[TRANSFORM] Cache HIT: %s", key)
	})
	transformHooks.AddOnMiss(func(key string) {
		atomic.AddInt64(&bp.cacheMisses, 1)
		log.Printf("[TRANSFORM] Cache MISS: %s", key)
	})

	// Category cache hooks
	categoryHooks := &obcache.Hooks{}
	categoryHooks.AddOnHit(func(key string, value any) {
		atomic.AddInt64(&bp.cacheHits, 1)
		log.Printf("[CATEGORY] Cache HIT: %s", key)
	})
	categoryHooks.AddOnMiss(func(key string) {
		atomic.AddInt64(&bp.cacheMisses, 1)
		log.Printf("[CATEGORY] Cache MISS: %s", key)
	})

	// Risk cache hooks
	riskHooks := &obcache.Hooks{}
	riskHooks.AddOnHit(func(key string, value any) {
		atomic.AddInt64(&bp.cacheHits, 1)
		log.Printf("[RISK] Cache HIT: %s", key)
	})
	riskHooks.AddOnMiss(func(key string) {
		atomic.AddInt64(&bp.cacheMisses, 1)
		log.Printf("[RISK] Cache MISS: %s", key)
	})
}

// Expensive computation functions (simulated)

func (bp *BatchProcessor) expensiveTransform(value float64, category string) (float64, error) {
	// Simulate expensive mathematical computation
	time.Sleep(10 * time.Millisecond)

	// Complex transformation based on category
	var multiplier float64
	switch category {
	case "premium":
		multiplier = 2.5
	case "standard":
		multiplier = 1.5
	case "basic":
		multiplier = 1.0
	default:
		return 0, fmt.Errorf("unknown category: %s", category)
	}

	// Apply complex mathematical operations
	result := math.Pow(value*multiplier, 1.2) + math.Log(value+1)*multiplier

	log.Printf("[COMPUTE] Expensive transform: %.2f (%s) -> %.2f", value, category, result)
	return result, nil
}

func (bp *BatchProcessor) expensiveCategoryComputation(category string) (float64, error) {
	// Simulate database query or complex computation for category scoring
	time.Sleep(25 * time.Millisecond)

	scores := map[string]float64{
		"premium":  0.95,
		"standard": 0.75,
		"basic":    0.50,
		"trial":    0.25,
	}

	score, exists := scores[category]
	if !exists {
		score = 0.1 // Default low score for unknown categories
	}

	log.Printf("[COMPUTE] Category score: %s -> %.2f", category, score)
	return score, nil
}

func (bp *BatchProcessor) expensiveRiskAssessment(transformedValue, categoryScore float64) (string, error) {
	// Simulate complex risk analysis
	time.Sleep(15 * time.Millisecond)

	riskScore := (transformedValue * 0.6) + (categoryScore * 100 * 0.4)

	var risk string
	switch {
	case riskScore > 150:
		risk = "HIGH"
	case riskScore > 80:
		risk = "MEDIUM"
	case riskScore > 30:
		risk = "LOW"
	default:
		risk = "MINIMAL"
	}

	log.Printf("[COMPUTE] Risk assessment: %.2f -> %s", riskScore, risk)
	return risk, nil
}

// Process a single record
func (bp *BatchProcessor) ProcessRecord(record *DataRecord) (*ProcessedResult, error) {
	startTime := time.Now()

	// Step 1: Transform the value
	transformedValue, err := bp.transformValue(record.Value, record.Category)
	if err != nil {
		atomic.AddInt64(&bp.errors, 1)
		return nil, fmt.Errorf("transform failed for record %d: %w", record.ID, err)
	}

	// Step 2: Calculate category score
	categoryScore, err := bp.calculateCategoryScore(record.Category)
	if err != nil {
		atomic.AddInt64(&bp.errors, 1)
		return nil, fmt.Errorf("category scoring failed for record %d: %w", record.ID, err)
	}

	// Step 3: Assess risk
	risk, err := bp.assessRisk(transformedValue, categoryScore)
	if err != nil {
		atomic.AddInt64(&bp.errors, 1)
		return nil, fmt.Errorf("risk assessment failed for record %d: %w", record.ID, err)
	}

	atomic.AddInt64(&bp.totalProcessed, 1)

	// Determine if any step hit cache (simplified heuristic)
	cacheHit := time.Since(startTime) < 30*time.Millisecond

	return &ProcessedResult{
		RecordID:         record.ID,
		TransformedValue: transformedValue,
		CategoryScore:    categoryScore,
		Risk:             risk,
		ProcessedAt:      time.Now(),
		CacheHit:         cacheHit,
	}, nil
}

// Process a batch of records concurrently
func (bp *BatchProcessor) ProcessBatch(records []*DataRecord, concurrency int) ([]*ProcessedResult, error) {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	jobs := make(chan *DataRecord, len(records))
	results := make(chan *ProcessedResult, len(records))
	errors := make(chan error, len(records))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for record := range jobs {
				result, err := bp.ProcessRecord(record)
				if err != nil {
					log.Printf("Worker %d error processing record %d: %v", workerID, record.ID, err)
					errors <- err
					continue
				}
				results <- result
			}
		}(i)
	}

	// Send jobs
	go func() {
		for _, record := range records {
			jobs <- record
		}
		close(jobs)
	}()

	// Wait for workers to finish
	go func() {
		wg.Wait()
		close(results)
		close(errors)
	}()

	// Collect results
	var processedResults []*ProcessedResult
	var processingErrors []error

	for result := range results {
		processedResults = append(processedResults, result)
	}

	for err := range errors {
		processingErrors = append(processingErrors, err)
	}

	if len(processingErrors) > 0 {
		log.Printf("Batch processing completed with %d errors out of %d records",
			len(processingErrors), len(records))
		// Return partial results with first error
		return processedResults, processingErrors[0]
	}

	return processedResults, nil
}

// WarmupCache pre-populates cache with common computations
func (bp *BatchProcessor) WarmupCache(ctx context.Context) error {
	log.Println("Starting cache warmup...")

	// Common categories for pre-computation
	categories := []string{"premium", "standard", "basic", "trial"}

	// Warm up category cache
	for _, category := range categories {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, err := bp.calculateCategoryScore(category)
			if err != nil {
				log.Printf("Warmup error for category %s: %v", category, err)
			}
		}
	}

	// Warm up transform cache with common values
	commonValues := []float64{10.0, 25.0, 50.0, 100.0, 250.0, 500.0}
	for _, value := range commonValues {
		for _, category := range categories {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				_, err := bp.transformValue(value, category)
				if err != nil {
					log.Printf("Warmup error for transform %.2f,%s: %v", value, category, err)
				}
			}
		}
	}

	log.Println("Cache warmup completed")
	return nil
}

func (bp *BatchProcessor) GetStats() map[string]interface{} {
	transformStats := bp.transformCache.Stats()
	categoryStats := bp.categoryCache.Stats()
	riskStats := bp.riskCache.Stats()

	return map[string]interface{}{
		"processing": map[string]interface{}{
			"total_processed":    atomic.LoadInt64(&bp.totalProcessed),
			"total_cache_hits":   atomic.LoadInt64(&bp.cacheHits),
			"total_cache_misses": atomic.LoadInt64(&bp.cacheMisses),
			"total_errors":       atomic.LoadInt64(&bp.errors),
		},
		"caches": map[string]interface{}{
			"transform": map[string]interface{}{
				"hits":     transformStats.Hits(),
				"misses":   transformStats.Misses(),
				"hit_rate": fmt.Sprintf("%.2f%%", transformStats.HitRate()),
				"keys":     transformStats.KeyCount(),
				"capacity": bp.transformCache.Capacity(),
			},
			"category": map[string]interface{}{
				"hits":     categoryStats.Hits(),
				"misses":   categoryStats.Misses(),
				"hit_rate": fmt.Sprintf("%.2f%%", categoryStats.HitRate()),
				"keys":     categoryStats.KeyCount(),
				"capacity": bp.categoryCache.Capacity(),
			},
			"risk": map[string]interface{}{
				"hits":     riskStats.Hits(),
				"misses":   riskStats.Misses(),
				"hit_rate": fmt.Sprintf("%.2f%%", riskStats.HitRate()),
				"keys":     riskStats.KeyCount(),
				"capacity": bp.riskCache.Capacity(),
			},
		},
	}
}

func (bp *BatchProcessor) Close() error {
	if err := bp.transformCache.Close(); err != nil {
		return err
	}
	if err := bp.categoryCache.Close(); err != nil {
		return err
	}
	return bp.riskCache.Close()
}

// Generate sample data for testing
func generateSampleData(count int) []*DataRecord {
	categories := []string{"premium", "standard", "basic", "trial"}
	records := make([]*DataRecord, count)

	for i := 0; i < count; i++ {
		records[i] = &DataRecord{
			ID:       i + 1,
			Category: categories[i%len(categories)],
			Value:    float64(10 + (i*13)%500), // Semi-random values
			Metadata: map[string]string{
				"source":  "batch_job",
				"version": "1.0",
			},
			Timestamp: time.Now().Add(-time.Duration(i) * time.Second),
		}
	}

	return records
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Parse command line arguments
	recordCount := 1000
	concurrency := runtime.NumCPU()

	if len(os.Args) > 1 {
		if _, err := fmt.Sscanf(os.Args[1], "%d", &recordCount); err != nil {
			log.Printf("Invalid record count, using default: %d", recordCount)
		}
	}

	if len(os.Args) > 2 {
		if _, err := fmt.Sscanf(os.Args[2], "%d", &concurrency); err != nil {
			log.Printf("Invalid concurrency, using default: %d", concurrency)
		}
	}

	log.Printf("Starting batch processor with %d records, %d workers", recordCount, concurrency)

	// Create processor
	processor, err := NewBatchProcessor()
	if err != nil {
		log.Fatalf("Failed to create processor: %v", err)
	}
	defer func() {
		if err := processor.Close(); err != nil {
			log.Printf("Error closing processor: %v", err)
		}
	}()

	// Warm up cache
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := processor.WarmupCache(ctx); err != nil {
		log.Printf("Cache warmup failed: %v", err)
	}

	// Generate sample data
	log.Printf("Generating %d sample records...", recordCount)
	records := generateSampleData(recordCount)

	// Process first batch (cold cache)
	log.Println("Processing first batch (cold cache)...")
	startTime := time.Now()

	results1, err := processor.ProcessBatch(records[:recordCount/2], concurrency)
	if err != nil {
		log.Printf("First batch processing error: %v", err)
	}

	firstBatchDuration := time.Since(startTime)
	log.Printf("First batch: %d records processed in %v (%.2f records/sec)",
		len(results1), firstBatchDuration, float64(len(results1))/firstBatchDuration.Seconds())

	// Process second batch (warm cache - same data to demonstrate cache effectiveness)
	log.Println("Processing second batch (warm cache, same data)...")
	startTime = time.Now()

	results2, err := processor.ProcessBatch(records[:recordCount/2], concurrency)
	if err != nil {
		log.Printf("Second batch processing error: %v", err)
	}

	secondBatchDuration := time.Since(startTime)
	log.Printf("Second batch: %d records processed in %v (%.2f records/sec)",
		len(results2), secondBatchDuration, float64(len(results2))/secondBatchDuration.Seconds())

	// Process third batch (mixed cache - some new data)
	log.Println("Processing third batch (mixed cache, new + existing data)...")
	startTime = time.Now()

	results3, err := processor.ProcessBatch(records[recordCount/4:recordCount*3/4], concurrency)
	if err != nil {
		log.Printf("Third batch processing error: %v", err)
	}

	thirdBatchDuration := time.Since(startTime)
	log.Printf("Third batch: %d records processed in %v (%.2f records/sec)",
		len(results3), thirdBatchDuration, float64(len(results3))/thirdBatchDuration.Seconds())

	// Print final statistics
	log.Println("\n=== FINAL STATISTICS ===")
	stats := processor.GetStats()

	// Print processing stats
	if processingStats, ok := stats["processing"].(map[string]interface{}); ok {
		fmt.Printf("Total Processed: %d\n", processingStats["total_processed"])
		fmt.Printf("Cache Hits: %d\n", processingStats["total_cache_hits"])
		fmt.Printf("Cache Misses: %d\n", processingStats["total_cache_misses"])
		fmt.Printf("Errors: %d\n", processingStats["total_errors"])

		hits := processingStats["total_cache_hits"].(int64)
		misses := processingStats["total_cache_misses"].(int64)
		if hits+misses > 0 {
			overallHitRate := float64(hits) / float64(hits+misses) * 100
			fmt.Printf("Overall Cache Hit Rate: %.2f%%\n", overallHitRate)
		}
	}

	// Print cache-specific stats
	if cacheStats, ok := stats["caches"].(map[string]interface{}); ok {
		for cacheName, cacheData := range cacheStats {
			if cache, ok := cacheData.(map[string]interface{}); ok {
				fmt.Printf("\n%s Cache:\n", strings.ToUpper(cacheName[:1])+cacheName[1:])
				fmt.Printf("  Hit Rate: %s\n", cache["hit_rate"])
				fmt.Printf("  Keys: %d/%d\n", cache["keys"], cache["capacity"])
			}
		}
	}

	// Performance analysis
	fmt.Printf("\nPerformance Analysis:\n")
	fmt.Printf("First batch (cold):  %.2f records/sec\n", float64(len(results1))/firstBatchDuration.Seconds())
	fmt.Printf("Second batch (warm): %.2f records/sec\n", float64(len(results2))/secondBatchDuration.Seconds())
	fmt.Printf("Third batch (mixed): %.2f records/sec\n", float64(len(results3))/thirdBatchDuration.Seconds())

	if secondBatchDuration > 0 && firstBatchDuration > 0 {
		improvement := (firstBatchDuration.Seconds() - secondBatchDuration.Seconds()) / firstBatchDuration.Seconds() * 100
		fmt.Printf("Cache effectiveness: %.1f%% speed improvement\n", improvement)
	}

	log.Println("Batch processing completed successfully!")
}
