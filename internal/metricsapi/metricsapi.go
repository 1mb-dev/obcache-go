// Package metricsapi holds the types pkg/obcache reports metrics through. It exists
// so importing pkg/obcache does not link the Prometheus client: pkg/metrics needs
// Prometheus in its exported API, so pkg/obcache must not import pkg/metrics.
package metricsapi

import "time"

// Exporter defines the interface for cache metrics exporters
// This abstraction allows supporting multiple observability systems
type Exporter interface {
	// ExportStats exports the current cache statistics
	ExportStats(stats Stats, labels Labels) error

	// RecordCacheOperation records individual cache operations with timing
	RecordCacheOperation(operation Operation, duration time.Duration, labels Labels) error

	// IncrementCounter increments a named counter with labels
	IncrementCounter(name string, labels Labels) error

	// RecordHistogram records a value in a named histogram
	RecordHistogram(name string, value float64, labels Labels) error

	// SetGauge sets a gauge value
	SetGauge(name string, value float64, labels Labels) error

	// Close shuts down the exporter and flushes any pending metrics
	Close() error
}

// Labels represents key-value pairs for metric labels/tags
type Labels map[string]string

// Stats interface defines the cache statistics that can be exported
// This allows the metrics package to work with any stats implementation
type Stats interface {
	Hits() int64
	Misses() int64
	Evictions() int64
	Invalidations() int64
	KeyCount() int64
	InFlight() int64
	HitRate() float64
}

// Operation represents different cache operations for metrics
type Operation string

const (
	// Cache operations
	OperationGet        Operation = "get"
	OperationSet        Operation = "set"
	OperationDelete     Operation = "delete"
	OperationInvalidate Operation = "invalidate"
	OperationEvict      Operation = "evict"
	OperationCleanup    Operation = "cleanup"

	// Function wrapping operations
	OperationFunctionCall Operation = "function_call"
)

// NoOpExporter provides a no-op implementation for when metrics are disabled
type NoOpExporter struct{}

// NewNoOpExporter creates a no-op exporter
func NewNoOpExporter() *NoOpExporter {
	return &NoOpExporter{}
}

// ExportStats does nothing
func (n *NoOpExporter) ExportStats(Stats, Labels) error { return nil }

// RecordCacheOperation does nothing
func (n *NoOpExporter) RecordCacheOperation(Operation, time.Duration, Labels) error { return nil }

// IncrementCounter does nothing
func (n *NoOpExporter) IncrementCounter(string, Labels) error { return nil }

// RecordHistogram does nothing
func (n *NoOpExporter) RecordHistogram(string, float64, Labels) error { return nil }

// SetGauge does nothing
func (n *NoOpExporter) SetGauge(string, float64, Labels) error { return nil }

// Close does nothing
func (n *NoOpExporter) Close() error { return nil }
