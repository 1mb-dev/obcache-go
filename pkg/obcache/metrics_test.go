package obcache

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/vnykmshr/obcache-go/pkg/metrics"
)

// MockExporter for testing metrics integration
type MockExporter struct {
	mu sync.RWMutex

	// Captured data
	statsExported    []metrics.Stats
	operationsLogged []mockOperation
	counters         map[string]int64
	histograms       map[string][]float64
	gauges           map[string]float64
	labels           []metrics.Labels

	// Control behavior
	exportStatsError bool
	recordOpError    bool
	closed           bool
}

type mockOperation struct {
	operation metrics.Operation
	duration  time.Duration
	labels    metrics.Labels
}

func NewMockExporter() *MockExporter {
	return &MockExporter{
		counters:   make(map[string]int64),
		histograms: make(map[string][]float64),
		gauges:     make(map[string]float64),
	}
}

func (m *MockExporter) ExportStats(stats metrics.Stats, labels metrics.Labels) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.exportStatsError {
		return fmt.Errorf("mock export stats error")
	}

	m.statsExported = append(m.statsExported, stats)
	m.labels = append(m.labels, labels)
	return nil
}

func (m *MockExporter) RecordCacheOperation(operation metrics.Operation, duration time.Duration, labels metrics.Labels) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.recordOpError {
		return fmt.Errorf("mock record operation error")
	}

	m.operationsLogged = append(m.operationsLogged, mockOperation{
		operation: operation,
		duration:  duration,
		labels:    labels,
	})
	return nil
}

func (m *MockExporter) IncrementCounter(name string, labels metrics.Labels) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := name + m.labelsKey(labels)
	m.counters[key]++
	return nil
}

func (m *MockExporter) RecordHistogram(name string, value float64, labels metrics.Labels) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := name + m.labelsKey(labels)
	m.histograms[key] = append(m.histograms[key], value)
	return nil
}

func (m *MockExporter) SetGauge(name string, value float64, labels metrics.Labels) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := name + m.labelsKey(labels)
	m.gauges[key] = value
	return nil
}

func (m *MockExporter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	return nil
}

func (m *MockExporter) labelsKey(labels metrics.Labels) string {
	result := ""
	for k, v := range labels {
		result += k + "=" + v + ","
	}
	return result
}

// Helper methods for test assertions
func (m *MockExporter) GetStatsExportCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.statsExported)
}

func (m *MockExporter) GetOperationCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.operationsLogged)
}

func (m *MockExporter) GetLastStats() metrics.Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.statsExported) == 0 {
		return nil
	}
	return m.statsExported[len(m.statsExported)-1]
}

func (m *MockExporter) HasOperation(op metrics.Operation) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, operation := range m.operationsLogged {
		if operation.operation == op {
			return true
		}
	}
	return false
}

func (m *MockExporter) GetCounterValue(name string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for key, value := range m.counters {
		if key == name || key[:len(name)] == name {
			return value
		}
	}
	return 0
}

func (m *MockExporter) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

func TestMetricsIntegration(t *testing.T) {
	mockExporter := NewMockExporter()

	config := NewDefaultConfig().WithMetrics(&MetricsConfig{
		Exporter:  mockExporter,
		Enabled:   true,
		CacheName: "test-cache",
		Labels:    make(metrics.Labels),
	})
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache with metrics: %v", err)
	}
	defer func() { _ = cache.Close() }()

	// Perform cache operations
	_ = cache.Set("key1", "value1", time.Hour)
	_, _ = cache.Get("key1") // hit
	_, _ = cache.Get("key2") // miss
	_ = cache.Set("key2", "value2", time.Hour)
	_ = cache.Delete("key1")

	// Give metrics exporter time to record operations
	time.Sleep(TestSlowOperation)

	// Verify operations were recorded
	if !mockExporter.HasOperation(metrics.OperationSet) {
		t.Error("Expected Set operation to be recorded")
	}
	if !mockExporter.HasOperation(metrics.OperationGet) {
		t.Error("Expected Get operation to be recorded")
	}

	// Check that we have multiple operations logged
	if mockExporter.GetOperationCount() < 4 {
		t.Errorf("Expected at least 4 operations, got %d", mockExporter.GetOperationCount())
	}
}

func TestMetricsPeriodicReporting(t *testing.T) {
	mockExporter := NewMockExporter()

	config := NewDefaultConfig().WithMetrics(&MetricsConfig{
		Exporter:  mockExporter,
		Enabled:   true,
		CacheName: "test-cache",
		Labels:    make(metrics.Labels),
	})
	config.Metrics.ReportingInterval = TestMetricsReportInterval // Fast interval for testing

	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache with metrics: %v", err)
	}
	defer func() { _ = cache.Close() }()

	// Perform some operations
	_ = cache.Set("key1", "value1", time.Hour)
	_, _ = cache.Get("key1")

	// Wait for at least 2 reporting intervals
	time.Sleep(120 * time.Millisecond)

	// Should have exported stats at least once
	if mockExporter.GetStatsExportCount() == 0 {
		t.Error("Expected stats to be exported periodically")
	}

	// Verify the stats content
	lastStats := mockExporter.GetLastStats()
	if lastStats == nil {
		t.Fatal("No stats were exported")
	}

	if lastStats.Hits() != 1 {
		t.Errorf("Expected 1 hit in exported stats, got %d", lastStats.Hits())
	}
	if lastStats.KeyCount() != 1 {
		t.Errorf("Expected 1 key in exported stats, got %d", lastStats.KeyCount())
	}
}

func TestMetricsWithLabels(t *testing.T) {
	mockExporter := NewMockExporter()

	labels := metrics.Labels{
		"environment": "test",
		"version":     "1.0.0",
	}

	config := NewDefaultConfig().WithMetrics(&MetricsConfig{
		Exporter:  mockExporter,
		Enabled:   true,
		CacheName: "labeled-cache",
		Labels:    labels,
	})

	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache with labeled metrics: %v", err)
	}
	defer func() { _ = cache.Close() }()

	// Perform operation
	_ = cache.Set("key1", "value1", time.Hour)

	// Wait for operation to be recorded
	time.Sleep(50 * time.Millisecond)

	// Force export stats
	cache.exportCurrentStats()

	// Verify labels were applied
	if mockExporter.GetStatsExportCount() == 0 {
		t.Fatal("Expected stats to be exported")
	}

	// Verify that operations were logged (labels are passed to each operation)
	if mockExporter.GetOperationCount() == 0 {
		t.Error("Expected operations to be logged with labels")
	}
}

func TestMetricsDisabled(t *testing.T) {
	// Create cache without metrics configuration
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = cache.Close() }()

	// Perform operations
	_ = cache.Set("key1", "value1", time.Hour)
	_, _ = cache.Get("key1")

	// Should not panic or cause issues
	// This tests that the no-op metrics path works correctly
}

func TestMetricsErrorHandling(t *testing.T) {
	mockExporter := NewMockExporter()
	mockExporter.exportStatsError = true // Simulate error

	config := NewDefaultConfig().WithMetrics(&MetricsConfig{
		Exporter:  mockExporter,
		Enabled:   true,
		CacheName: "error-test",
		Labels:    make(metrics.Labels),
	})
	config.Metrics.ReportingInterval = TestMetricsReportInterval

	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = cache.Close() }()

	// Perform operations - should not panic even with export errors
	_ = cache.Set("key1", "value1", time.Hour)
	_, _ = cache.Get("key1")

	time.Sleep(TestSlowOperation) // Let metrics reporter run

	// Cache should still function normally despite metrics errors
	value, found := cache.Get("key1")
	if !found {
		t.Error("Expected key to be found despite metrics errors")
	}
	if value != "value1" {
		t.Errorf("Expected value1, got %v", value)
	}
}

func TestMetricsCleanup(t *testing.T) {
	mockExporter := NewMockExporter()

	config := NewDefaultConfig().WithMetrics(&MetricsConfig{
		Exporter:  mockExporter,
		Enabled:   true,
		CacheName: "cleanup-test",
		Labels:    make(metrics.Labels),
	})
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Close cache and verify metrics cleanup
	_ = cache.Close() // Test cleanup

	if !mockExporter.IsClosed() {
		t.Error("Expected metrics exporter to be closed")
	}
}

func TestNoOpExporter(t *testing.T) {
	noOpExporter := metrics.NewNoOpExporter()

	// Should not error on any operations
	err := noOpExporter.ExportStats(nil, nil)
	if err != nil {
		t.Errorf("NoOpExporter.ExportStats should not error, got: %v", err)
	}

	err = noOpExporter.RecordCacheOperation(metrics.OperationGet, time.Millisecond, nil)
	if err != nil {
		t.Errorf("NoOpExporter.RecordCacheOperation should not error, got: %v", err)
	}

	err = noOpExporter.IncrementCounter("test", nil)
	if err != nil {
		t.Errorf("NoOpExporter.IncrementCounter should not error, got: %v", err)
	}

	err = noOpExporter.RecordHistogram("test", 1.0, nil)
	if err != nil {
		t.Errorf("NoOpExporter.RecordHistogram should not error, got: %v", err)
	}

	err = noOpExporter.SetGauge("test", 1.0, nil)
	if err != nil {
		t.Errorf("NoOpExporter.SetGauge should not error, got: %v", err)
	}

	err = noOpExporter.Close()
	if err != nil {
		t.Errorf("NoOpExporter.Close should not error, got: %v", err)
	}
}

func TestMultiExporter(t *testing.T) {
	mock1 := NewMockExporter()
	mock2 := NewMockExporter()

	multiExporter := metrics.NewMultiExporter(mock1, mock2)

	config := NewDefaultConfig().WithMetrics(&MetricsConfig{
		Exporter:  multiExporter,
		Enabled:   true,
		CacheName: "multi-test",
		Labels:    make(metrics.Labels),
	})
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache with multi-exporter: %v", err)
	}

	// Perform operations
	_ = cache.Set("key1", "value1", time.Hour)
	_, _ = cache.Get("key1")

	// Wait for operations to be recorded
	time.Sleep(50 * time.Millisecond)

	// Both exporters should have recorded operations
	if mock1.GetOperationCount() == 0 {
		t.Error("Expected operations to be recorded in first exporter")
	}
	if mock2.GetOperationCount() == 0 {
		t.Error("Expected operations to be recorded in second exporter")
	}

	// Operation counts should be equal
	if mock1.GetOperationCount() != mock2.GetOperationCount() {
		t.Errorf("Expected equal operation counts, got %d and %d",
			mock1.GetOperationCount(), mock2.GetOperationCount())
	}

	// Test close - only call once, not in defer and explicitly
	_ = cache.Close() // Test cleanup
	if !mock1.IsClosed() || !mock2.IsClosed() {
		t.Error("Expected both exporters to be closed")
	}
}

func TestMetricsConfiguration(t *testing.T) {
	// Test default configuration
	config := metrics.NewDefaultConfig()

	if !config.Enabled {
		t.Error("Default config should be enabled")
	}
	if config.Namespace != "obcache" {
		t.Errorf("Expected namespace 'obcache', got '%s'", config.Namespace)
	}
	if config.ReportingInterval != 30*time.Second {
		t.Errorf("Expected 30s reporting interval, got %v", config.ReportingInterval)
	}

	// Test fluent configuration
	customConfig := metrics.NewDefaultConfig().
		WithNamespace("custom").
		WithLabels(metrics.Labels{"env": "test"}).
		WithReportingInterval(10 * time.Second).
		WithDetailedTimings(true).
		WithKeyValueSizes(true)

	if customConfig.Namespace != "custom" {
		t.Errorf("Expected namespace 'custom', got '%s'", customConfig.Namespace)
	}
	if customConfig.Labels["env"] != "test" {
		t.Errorf("Expected label env=test, got %v", customConfig.Labels)
	}
	if customConfig.ReportingInterval != 10*time.Second {
		t.Errorf("Expected 10s reporting interval, got %v", customConfig.ReportingInterval)
	}
	if !customConfig.IncludeDetailedTimings {
		t.Error("Expected detailed timings to be enabled")
	}
	if !customConfig.IncludeKeyValueSizes {
		t.Error("Expected key/value sizes to be enabled")
	}
}

func TestMetricsStatsInterface(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer func() { _ = cache.Close() }()

	// Perform operations to generate stats
	_ = cache.Set("key1", "value1", time.Hour)
	_ = cache.Set("key2", "value2", time.Hour)
	_, _ = cache.Get("key1") // hit
	_, _ = cache.Get("key3") // miss
	_ = cache.Delete("key2")

	stats := cache.Stats()

	// Test that stats implements the metrics.Stats interface
	var metricsStats metrics.Stats = stats

	if metricsStats.Hits() != 1 {
		t.Errorf("Expected 1 hit, got %d", metricsStats.Hits())
	}
	if metricsStats.Misses() != 1 {
		t.Errorf("Expected 1 miss, got %d", metricsStats.Misses())
	}
	if metricsStats.Invalidations() != 1 {
		t.Errorf("Expected 1 invalidation, got %d", metricsStats.Invalidations())
	}
	if metricsStats.KeyCount() != 1 { // key1 remains after invalidation
		t.Errorf("Expected 1 key, got %d", metricsStats.KeyCount())
	}
	if metricsStats.HitRate() != 50.0 {
		t.Errorf("Expected 50%% hit rate, got %.2f", metricsStats.HitRate())
	}
}

func BenchmarkMetricsOverhead(b *testing.B) {
	// Benchmark without metrics
	b.Run("NoMetrics", func(b *testing.B) {
		cache, _ := New(NewDefaultConfig())
		defer func() { _ = cache.Close() }()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("key-%d", i%100)
			_ = cache.Set(key, i, time.Hour)
			_, _ = cache.Get(key)
		}
	})

	// Benchmark with metrics enabled
	b.Run("WithMetrics", func(b *testing.B) {
		mockExporter := NewMockExporter()
		config := NewDefaultConfig().WithMetrics(&MetricsConfig{
			Exporter:  mockExporter,
			Enabled:   true,
			CacheName: "bench-cache",
			Labels:    make(metrics.Labels),
		})
		cache, _ := New(config)
		defer func() { _ = cache.Close() }()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("key-%d", i%100)
			_ = cache.Set(key, i, time.Hour)
			_, _ = cache.Get(key)
		}
	})

	// Benchmark with NoOp exporter (should have minimal overhead)
	b.Run("WithNoOpMetrics", func(b *testing.B) {
		config := NewDefaultConfig().WithMetrics(&MetricsConfig{
			Exporter:  metrics.NewNoOpExporter(),
			Enabled:   true,
			CacheName: "bench-cache",
			Labels:    make(metrics.Labels),
		})
		cache, _ := New(config)
		defer func() { _ = cache.Close() }()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("key-%d", i%100)
			_ = cache.Set(key, i, time.Hour)
			_, _ = cache.Get(key)
		}
	})
}
