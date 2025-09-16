package obcache

import "time"

// Test and example constants for consistent usage across the codebase.
// These constants help maintain consistency in tests and examples.
const (
	// TestTTL is the standard TTL used in test cases
	TestTTL = time.Hour

	// TestShortTTL is used for tests that need quick expiration
	TestShortTTL = 10 * time.Millisecond

	// TestSlowOperation simulates slow operations in benchmarks
	TestSlowOperation = 100 * time.Millisecond

	// TestMetricsReportInterval for fast metrics reporting in tests
	TestMetricsReportInterval = 30 * time.Millisecond

	// ExampleTTL for documentation examples
	ExampleTTL = 30 * time.Minute

	// ExampleShortTTL for quick examples
	ExampleShortTTL = 10 * time.Minute
)
