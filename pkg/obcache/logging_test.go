package obcache

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"
)

// TestLogger implements Logger interface for testing
type TestLogger struct {
	logs   []LogEntry
	fields []Field
}

type LogEntry struct {
	Level   string
	Message string
	Fields  []Field
}

func NewTestLogger() *TestLogger {
	return &TestLogger{
		logs: make([]LogEntry, 0),
	}
}

func (tl *TestLogger) Debug(msg string, fields ...Field) {
	tl.log("DEBUG", msg, fields...)
}

func (tl *TestLogger) Info(msg string, fields ...Field) {
	tl.log("INFO", msg, fields...)
}

func (tl *TestLogger) Warn(msg string, fields ...Field) {
	tl.log("WARN", msg, fields...)
}

func (tl *TestLogger) Error(msg string, fields ...Field) {
	tl.log("ERROR", msg, fields...)
}

func (tl *TestLogger) With(fields ...Field) Logger {
	newFields := make([]Field, len(tl.fields)+len(fields))
	copy(newFields, tl.fields)
	copy(newFields[len(tl.fields):], fields)

	return &TestLogger{
		logs:   tl.logs,
		fields: newFields,
	}
}

func (tl *TestLogger) log(level, msg string, fields ...Field) {
	allFields := make([]Field, len(tl.fields)+len(fields))
	copy(allFields, tl.fields)
	copy(allFields[len(tl.fields):], fields)

	tl.logs = append(tl.logs, LogEntry{
		Level:   level,
		Message: msg,
		Fields:  allFields,
	})
}

func (tl *TestLogger) GetLogs() []LogEntry {
	return tl.logs
}

func (tl *TestLogger) GetLogsWithLevel(level string) []LogEntry {
	var filtered []LogEntry
	for _, entry := range tl.logs {
		if entry.Level == level {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (tl *TestLogger) HasLogWithMessage(message string) bool {
	for _, entry := range tl.logs {
		if strings.Contains(entry.Message, message) {
			return true
		}
	}
	return false
}

func (tl *TestLogger) HasLogWithField(key string, value interface{}) bool {
	for _, entry := range tl.logs {
		for _, field := range entry.Fields {
			if field.Key == key && field.Value == value {
				return true
			}
		}
	}
	return false
}

func (tl *TestLogger) Clear() {
	tl.logs = make([]LogEntry, 0)
}

func TestLogLevel(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LogLevelDebug, "DEBUG"},
		{LogLevelInfo, "INFO"},
		{LogLevelWarn, "WARN"},
		{LogLevelError, "ERROR"},
		{LogLevelNone, "NONE"},
		{LogLevel(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.level.String() != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.level.String())
			}
		})
	}
}

func TestDefaultLogger(t *testing.T) {
	// Capture output for testing
	var buf bytes.Buffer
	logger := &DefaultLogger{
		level:  LogLevelInfo,
		logger: log.New(&buf, "", log.LstdFlags),
		fields: make([]Field, 0),
	}

	// Test that debug messages are filtered out
	logger.Debug("debug message")
	if buf.Len() > 0 {
		t.Error("Debug message should be filtered out at INFO level")
	}

	// Test that info messages are logged
	buf.Reset()
	logger.Info("info message", F("key", "value"))
	output := buf.String()
	if !strings.Contains(output, "INFO") || !strings.Contains(output, "info message") {
		t.Errorf("Expected INFO message, got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Expected field in output, got: %s", output)
	}
}

func TestDefaultLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := &DefaultLogger{
		level:  LogLevelInfo,
		logger: log.New(&buf, "", 0), // No timestamp for easier testing
		fields: []Field{F("service", "test")},
	}

	logger.Info("test message", F("request_id", "123"))
	output := buf.String()

	if !strings.Contains(output, "service=test") {
		t.Errorf("Expected base field in output, got: %s", output)
	}
	if !strings.Contains(output, "request_id=123") {
		t.Errorf("Expected additional field in output, got: %s", output)
	}
}

func TestNoOpLogger(t *testing.T) {
	logger := NewNoOpLogger()

	// These should not panic and should do nothing
	logger.Debug("test")
	logger.Info("test")
	logger.Warn("test")
	logger.Error("test")

	newLogger := logger.With(F("key", "value"))
	if newLogger != logger {
		t.Error("NoOpLogger.With should return the same instance")
	}
}

func TestLoggingHooksCreation(t *testing.T) {
	testLogger := NewTestLogger()
	config := &LoggingConfig{
		Logger:           testLogger,
		LogCacheHits:     true,
		LogCacheMisses:   true,
		LogEvictions:     true,
		LogInvalidations: true,
	}

	hooks := CreateLoggingHooks(config)

	// Verify hooks were created
	if len(hooks.OnHitCtxPriority) == 0 {
		t.Error("Expected hit logging hook to be created")
	}
	if len(hooks.OnMissCtxPriority) == 0 {
		t.Error("Expected miss logging hook to be created")
	}
	if len(hooks.OnEvictCtxPriority) == 0 {
		t.Error("Expected eviction logging hook to be created")
	}
	if len(hooks.OnInvalidateCtxPriority) == 0 {
		t.Error("Expected invalidation logging hook to be created")
	}
}

func TestLoggingHooksIntegration(t *testing.T) {
	testLogger := NewTestLogger()
	config := &LoggingConfig{
		Logger:           testLogger,
		LogCacheHits:     true,
		LogCacheMisses:   true,
		LogInvalidations: true,
		IncludeValues:    true,
		MaxValueLength:   50,
	}

	hooks := CreateLoggingHooks(config)
	cache, err := New(NewDefaultConfig().WithHooks(hooks))
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Test cache hit logging
	_ = cache.Set("test-key", "test-value", time.Hour)
	_, _ = cache.Get("test-key")

	if !testLogger.HasLogWithMessage("Cache hit") {
		t.Error("Expected cache hit to be logged")
	}
	if !testLogger.HasLogWithField("key", "test-key") {
		t.Error("Expected key field in hit log")
	}
	if !testLogger.HasLogWithField("event", "cache_hit") {
		t.Error("Expected event field in hit log")
	}

	// Test cache miss logging
	testLogger.Clear()
	_, _ = cache.Get("missing-key")

	if !testLogger.HasLogWithMessage("Cache miss") {
		t.Error("Expected cache miss to be logged")
	}
	if !testLogger.HasLogWithField("key", "missing-key") {
		t.Error("Expected key field in miss log")
	}

	// Test invalidation logging
	testLogger.Clear()
	_ = cache.Delete("test-key")

	if !testLogger.HasLogWithMessage("Cache invalidation") {
		t.Error("Expected cache invalidation to be logged")
	}
}

func TestLoggingHookBuilder(t *testing.T) {
	// Test fluent builder interface
	hooks := NewLoggingHookBuilder().
		WithLevel(LogLevelInfo).
		EnableAllLogging().
		IncludeValues(100).
		EnableSlowOperationLogging(50 * time.Millisecond).
		Build()

	// Verify hooks were created
	if len(hooks.OnHitCtxPriority) == 0 {
		t.Error("Builder should create hit hooks")
	}
	if len(hooks.OnMissCtxPriority) == 0 {
		t.Error("Builder should create miss hooks")
	}
	if len(hooks.OnEvictCtxPriority) == 0 {
		t.Error("Builder should create eviction hooks")
	}
	if len(hooks.OnInvalidateCtxPriority) == 0 {
		t.Error("Builder should create invalidation hooks")
	}
}

func TestLoggingBuilderSelectiveEnabling(t *testing.T) {
	hooks := NewLoggingHookBuilder().
		WithLevel(LogLevelInfo).
		EnableHitLogging().
		EnableMissLogging().
		Build()

	// Should have hit and miss hooks only
	if len(hooks.OnHitCtxPriority) == 0 {
		t.Error("Builder should create hit hooks")
	}
	if len(hooks.OnMissCtxPriority) == 0 {
		t.Error("Builder should create miss hooks")
	}
	if len(hooks.OnEvictCtxPriority) != 0 {
		t.Error("Builder should not create eviction hooks when not enabled")
	}
	if len(hooks.OnInvalidateCtxPriority) != 0 {
		t.Error("Builder should not create invalidation hooks when not enabled")
	}
}

func TestValueTruncation(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		maxLength int
		expected  string
	}{
		{
			name:      "short value",
			value:     "short",
			maxLength: 10,
			expected:  "short",
		},
		{
			name:      "exact length",
			value:     "exactly10!",
			maxLength: 10,
			expected:  "exactly10!",
		},
		{
			name:      "too long",
			value:     "this value is too long",
			maxLength: 10,
			expected:  "this va...",
		},
		{
			name:      "very short limit",
			value:     "hello",
			maxLength: 3,
			expected:  "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateValue(tt.value, tt.maxLength)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestLoggingConfigDefaults(t *testing.T) {
	config := NewDefaultLoggingConfig(LogLevelInfo)

	if config.Logger == nil {
		t.Error("Default config should have a logger")
	}
	if !config.LogCacheHits {
		t.Error("Default config should enable hit logging")
	}
	if !config.LogCacheMisses {
		t.Error("Default config should enable miss logging")
	}
	if !config.LogEvictions {
		t.Error("Default config should enable eviction logging")
	}
	if !config.LogInvalidations {
		t.Error("Default config should enable invalidation logging")
	}
	if config.SlowOpThreshold != 100*time.Millisecond {
		t.Error("Default slow operation threshold should be 100ms")
	}
	if config.MaxValueLength != 100 {
		t.Error("Default max value length should be 100")
	}
}

func TestLoggingWithEviction(t *testing.T) {
	testLogger := NewTestLogger()
	hooks := CreateLoggingHooks(&LoggingConfig{
		Logger:       testLogger,
		LogEvictions: true,
	})

	// Create small cache to force evictions
	config := NewDefaultConfig().WithMaxEntries(2).WithHooks(hooks)
	cache, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	// Fill cache beyond capacity
	_ = cache.Set("key1", "value1", time.Hour)
	_ = cache.Set("key2", "value2", time.Hour)
	_ = cache.Set("key3", "value3", time.Hour) // Should evict key1

	// Wait a bit for eviction hook to be called
	time.Sleep(10 * time.Millisecond)

	if !testLogger.HasLogWithMessage("Cache eviction") {
		t.Error("Expected cache eviction to be logged")
	}
	if !testLogger.HasLogWithField("event", "cache_evict") {
		t.Error("Expected event field in eviction log")
	}
}

func TestLoggingNilConfig(t *testing.T) {
	// Should not panic with nil config
	hooks := CreateLoggingHooks(nil)

	if hooks == nil {
		t.Error("Should return empty hooks, not nil")
		return // Prevent nil dereference
	}

	// Should have no hooks
	if len(hooks.OnHitCtxPriority) != 0 || len(hooks.OnMissCtxPriority) != 0 {
		t.Error("Nil config should result in no hooks")
	}
}

func TestCustomLoggerIntegration(t *testing.T) {
	testLogger := NewTestLogger().With(F("component", "cache")).(*TestLogger)

	hooks := NewLoggingHookBuilder().
		WithLogger(testLogger).
		EnableHitLogging().
		Build()

	cache, _ := New(NewDefaultConfig().WithHooks(hooks))

	_ = cache.Set("test", "value", time.Hour)
	_, _ = cache.Get("test")

	if !testLogger.HasLogWithField("component", "cache") {
		t.Error("Expected logger with fields to maintain context")
	}
}

func BenchmarkLoggingOverhead(b *testing.B) {
	// Benchmark with no logging
	b.Run("NoLogging", func(b *testing.B) {
		cache, _ := New(NewDefaultConfig())

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("key-%d", i%100)
			_ = cache.Set(key, i, time.Hour)
			_, _ = cache.Get(key)
		}
	})

	// Benchmark with full logging enabled
	b.Run("WithLogging", func(b *testing.B) {
		hooks := NewLoggingHookBuilder().
			WithLevel(LogLevelInfo).
			EnableAllLogging().
			Build()
		cache, _ := New(NewDefaultConfig().WithHooks(hooks))

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("key-%d", i%100)
			_ = cache.Set(key, i, time.Hour)
			_, _ = cache.Get(key)
		}
	})

	// Benchmark with NoOp logger (should be minimal overhead)
	b.Run("WithNoOpLogger", func(b *testing.B) {
		hooks := NewLoggingHookBuilder().
			WithLogger(NewNoOpLogger()).
			EnableAllLogging().
			Build()
		cache, _ := New(NewDefaultConfig().WithHooks(hooks))

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("key-%d", i%100)
			_ = cache.Set(key, i, time.Hour)
			_, _ = cache.Get(key)
		}
	})
}
