package metrics

import (
	"errors"
	"testing"
	"time"
)

// Mock Stats implementation for testing
type mockStats struct {
	hits          int64
	misses        int64
	evictions     int64
	invalidations int64
	keyCount      int64
	inFlight      int64
	hitRate       float64
}

func (m *mockStats) Hits() int64          { return m.hits }
func (m *mockStats) Misses() int64        { return m.misses }
func (m *mockStats) Evictions() int64     { return m.evictions }
func (m *mockStats) Invalidations() int64 { return m.invalidations }
func (m *mockStats) KeyCount() int64      { return m.keyCount }
func (m *mockStats) InFlight() int64      { return m.inFlight }
func (m *mockStats) HitRate() float64     { return m.hitRate }

// Mock Exporter for testing MultiExporter
type mockExporter struct {
	exportStatsCallCount int
	recordOpCallCount    int
	incrCounterCallCount int
	recordHistoCallCount int
	setGaugeCallCount    int
	closeCallCount       int
	shouldError          bool
	lastOperation        Operation
	lastDuration         time.Duration
	lastLabels           Labels
}

func newMockExporter() *mockExporter {
	return &mockExporter{}
}

func (m *mockExporter) ExportStats(stats Stats, labels Labels) error {
	m.exportStatsCallCount++
	m.lastLabels = labels
	if m.shouldError {
		return errors.New("mock error")
	}
	return nil
}

func (m *mockExporter) RecordCacheOperation(operation Operation, duration time.Duration, labels Labels) error {
	m.recordOpCallCount++
	m.lastOperation = operation
	m.lastDuration = duration
	m.lastLabels = labels
	if m.shouldError {
		return errors.New("mock error")
	}
	return nil
}

func (m *mockExporter) IncrementCounter(name string, labels Labels) error {
	m.incrCounterCallCount++
	m.lastLabels = labels
	if m.shouldError {
		return errors.New("mock error")
	}
	return nil
}

func (m *mockExporter) RecordHistogram(name string, value float64, labels Labels) error {
	m.recordHistoCallCount++
	m.lastLabels = labels
	if m.shouldError {
		return errors.New("mock error")
	}
	return nil
}

func (m *mockExporter) SetGauge(name string, value float64, labels Labels) error {
	m.setGaugeCallCount++
	m.lastLabels = labels
	if m.shouldError {
		return errors.New("mock error")
	}
	return nil
}

func (m *mockExporter) Close() error {
	m.closeCallCount++
	if m.shouldError {
		return errors.New("mock error")
	}
	return nil
}

func TestNewDefaultConfig(t *testing.T) {
	config := NewDefaultConfig()

	if !config.Enabled {
		t.Error("Expected Enabled to be true by default")
	}
	if config.Namespace != "obcache" {
		t.Errorf("Expected namespace 'obcache', got %s", config.Namespace)
	}
	if config.Labels == nil {
		t.Error("Expected Labels to be initialized")
	}
	if config.ReportingInterval != 30*time.Second {
		t.Errorf("Expected ReportingInterval 30s, got %v", config.ReportingInterval)
	}
	if config.IncludeDetailedTimings {
		t.Error("Expected IncludeDetailedTimings to be false")
	}
	if config.IncludeKeyValueSizes {
		t.Error("Expected IncludeKeyValueSizes to be false")
	}
}

func TestConfigBuilder(t *testing.T) {
	labels := Labels{"env": "test", "service": "cache"}

	config := NewDefaultConfig().
		WithNamespace("myapp").
		WithLabels(labels).
		WithReportingInterval(60 * time.Second).
		WithDetailedTimings(true).
		WithKeyValueSizes(true)

	if config.Namespace != "myapp" {
		t.Errorf("Expected namespace 'myapp', got %s", config.Namespace)
	}
	if config.Labels["env"] != "test" {
		t.Errorf("Expected label env=test, got %s", config.Labels["env"])
	}
	if config.Labels["service"] != "cache" {
		t.Errorf("Expected label service=cache, got %s", config.Labels["service"])
	}
	if config.ReportingInterval != 60*time.Second {
		t.Errorf("Expected ReportingInterval 60s, got %v", config.ReportingInterval)
	}
	if !config.IncludeDetailedTimings {
		t.Error("Expected IncludeDetailedTimings to be true")
	}
	if !config.IncludeKeyValueSizes {
		t.Error("Expected IncludeKeyValueSizes to be true")
	}
}

func TestDefaultMetricNames(t *testing.T) {
	names := DefaultMetricNames()

	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{"CacheHitsTotal", names.CacheHitsTotal, "obcache_hits_total"},
		{"CacheMissesTotal", names.CacheMissesTotal, "obcache_misses_total"},
		{"CacheEvictionsTotal", names.CacheEvictionsTotal, "obcache_evictions_total"},
		{"CacheInvalidationsTotal", names.CacheInvalidationsTotal, "obcache_invalidations_total"},
		{"CacheOperationsTotal", names.CacheOperationsTotal, "obcache_operations_total"},
		{"CacheErrorsTotal", names.CacheErrorsTotal, "obcache_errors_total"},
		{"CacheOperationDuration", names.CacheOperationDuration, "obcache_operation_duration_seconds"},
		{"CacheKeySize", names.CacheKeySize, "obcache_key_size_bytes"},
		{"CacheValueSize", names.CacheValueSize, "obcache_value_size_bytes"},
		{"CacheKeysCount", names.CacheKeysCount, "obcache_keys_count"},
		{"CacheInFlightRequests", names.CacheInFlightRequests, "obcache_inflight_requests"},
		{"CacheHitRate", names.CacheHitRate, "obcache_hit_rate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("Expected %s to be %s, got %s", tt.name, tt.expected, tt.value)
			}
		})
	}
}

func TestNoOpExporter(t *testing.T) {
	exporter := NewNoOpExporter()

	stats := &mockStats{
		hits:    100,
		misses:  20,
		hitRate: 83.33,
	}
	labels := Labels{"test": "value"}

	// All operations should succeed and do nothing
	if err := exporter.ExportStats(stats, labels); err != nil {
		t.Errorf("ExportStats should not error: %v", err)
	}

	if err := exporter.RecordCacheOperation(OperationGet, time.Millisecond, labels); err != nil {
		t.Errorf("RecordCacheOperation should not error: %v", err)
	}

	if err := exporter.IncrementCounter("test", labels); err != nil {
		t.Errorf("IncrementCounter should not error: %v", err)
	}

	if err := exporter.RecordHistogram("test", 1.5, labels); err != nil {
		t.Errorf("RecordHistogram should not error: %v", err)
	}

	if err := exporter.SetGauge("test", 42.0, labels); err != nil {
		t.Errorf("SetGauge should not error: %v", err)
	}

	if err := exporter.Close(); err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestMultiExporter(t *testing.T) {
	mock1 := newMockExporter()
	mock2 := newMockExporter()

	multi := NewMultiExporter(mock1, mock2)

	stats := &mockStats{
		hits:    100,
		misses:  20,
		hitRate: 83.33,
	}
	labels := Labels{"env": "test"}

	t.Run("ExportStats calls all exporters", func(t *testing.T) {
		err := multi.ExportStats(stats, labels)
		if err != nil {
			t.Fatalf("ExportStats failed: %v", err)
		}

		if mock1.exportStatsCallCount != 1 {
			t.Errorf("Expected mock1 to be called once, got %d", mock1.exportStatsCallCount)
		}
		if mock2.exportStatsCallCount != 1 {
			t.Errorf("Expected mock2 to be called once, got %d", mock2.exportStatsCallCount)
		}
	})

	t.Run("RecordCacheOperation calls all exporters", func(t *testing.T) {
		duration := 5 * time.Millisecond
		err := multi.RecordCacheOperation(OperationGet, duration, labels)
		if err != nil {
			t.Fatalf("RecordCacheOperation failed: %v", err)
		}

		if mock1.recordOpCallCount != 1 {
			t.Errorf("Expected mock1 to be called once, got %d", mock1.recordOpCallCount)
		}
		if mock2.recordOpCallCount != 1 {
			t.Errorf("Expected mock2 to be called once, got %d", mock2.recordOpCallCount)
		}
		if mock1.lastOperation != OperationGet {
			t.Errorf("Expected operation GET, got %s", mock1.lastOperation)
		}
		if mock1.lastDuration != duration {
			t.Errorf("Expected duration %v, got %v", duration, mock1.lastDuration)
		}
	})

	t.Run("IncrementCounter calls all exporters", func(t *testing.T) {
		err := multi.IncrementCounter("test_counter", labels)
		if err != nil {
			t.Fatalf("IncrementCounter failed: %v", err)
		}

		if mock1.incrCounterCallCount != 1 {
			t.Errorf("Expected mock1 to be called once, got %d", mock1.incrCounterCallCount)
		}
		if mock2.incrCounterCallCount != 1 {
			t.Errorf("Expected mock2 to be called once, got %d", mock2.incrCounterCallCount)
		}
	})

	t.Run("RecordHistogram calls all exporters", func(t *testing.T) {
		err := multi.RecordHistogram("test_histogram", 12.34, labels)
		if err != nil {
			t.Fatalf("RecordHistogram failed: %v", err)
		}

		if mock1.recordHistoCallCount != 1 {
			t.Errorf("Expected mock1 to be called once, got %d", mock1.recordHistoCallCount)
		}
		if mock2.recordHistoCallCount != 1 {
			t.Errorf("Expected mock2 to be called once, got %d", mock2.recordHistoCallCount)
		}
	})

	t.Run("SetGauge calls all exporters", func(t *testing.T) {
		err := multi.SetGauge("test_gauge", 99.9, labels)
		if err != nil {
			t.Fatalf("SetGauge failed: %v", err)
		}

		if mock1.setGaugeCallCount != 1 {
			t.Errorf("Expected mock1 to be called once, got %d", mock1.setGaugeCallCount)
		}
		if mock2.setGaugeCallCount != 1 {
			t.Errorf("Expected mock2 to be called once, got %d", mock2.setGaugeCallCount)
		}
	})

	t.Run("Close calls all exporters", func(t *testing.T) {
		err := multi.Close()
		if err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		if mock1.closeCallCount != 1 {
			t.Errorf("Expected mock1 to be called once, got %d", mock1.closeCallCount)
		}
		if mock2.closeCallCount != 1 {
			t.Errorf("Expected mock2 to be called once, got %d", mock2.closeCallCount)
		}
	})
}

func TestMultiExporterError(t *testing.T) {
	mock1 := newMockExporter()
	mock2 := newMockExporter()
	mock2.shouldError = true

	multi := NewMultiExporter(mock1, mock2)

	stats := &mockStats{hits: 100}
	labels := Labels{"env": "test"}

	// Should return error from second exporter
	err := multi.ExportStats(stats, labels)
	if err == nil {
		t.Error("Expected error from multi-exporter")
	}

	// First exporter should still have been called
	if mock1.exportStatsCallCount != 1 {
		t.Errorf("Expected mock1 to be called before error, got %d", mock1.exportStatsCallCount)
	}
}

func TestOperationConstants(t *testing.T) {
	operations := []Operation{
		OperationGet,
		OperationSet,
		OperationDelete,
		OperationInvalidate,
		OperationEvict,
		OperationCleanup,
		OperationFunctionCall,
	}

	for _, op := range operations {
		if string(op) == "" {
			t.Errorf("Operation %v should not be empty string", op)
		}
	}
}

func TestResultConstants(t *testing.T) {
	results := []Result{
		ResultHit,
		ResultMiss,
		ResultError,
	}

	for _, res := range results {
		if string(res) == "" {
			t.Errorf("Result %v should not be empty string", res)
		}
	}
}

func TestInterfaceImplementation(t *testing.T) {
	// Ensure all types implement the Exporter interface
	var _ Exporter = (*MultiExporter)(nil)
	var _ Exporter = (*NoOpExporter)(nil)
	var _ Exporter = (*mockExporter)(nil)

	// Ensure mockStats implements Stats interface
	var _ Stats = (*mockStats)(nil)
}

func TestLabelsType(t *testing.T) {
	labels := Labels{
		"env":     "production",
		"service": "cache",
		"version": "1.0",
	}

	if len(labels) != 3 {
		t.Errorf("Expected 3 labels, got %d", len(labels))
	}

	if labels["env"] != "production" {
		t.Errorf("Expected env=production, got %s", labels["env"])
	}
}
