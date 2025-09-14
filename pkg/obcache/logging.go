package obcache

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// LogLevel defines the severity level for logging
type LogLevel int

const (
	// LogLevelDebug enables all log messages including detailed debugging
	LogLevelDebug LogLevel = iota

	// LogLevelInfo enables informational messages and above
	LogLevelInfo

	// LogLevelWarn enables warning messages and above
	LogLevelWarn

	// LogLevelError enables only error messages
	LogLevelError

	// LogLevelNone disables all logging
	LogLevelNone
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	case LogLevelNone:
		return "NONE"
	default:
		return "UNKNOWN"
	}
}

// Logger defines the interface for cache logging
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	With(fields ...Field) Logger
}

// Field represents a key-value pair for structured logging
type Field struct {
	Key   string
	Value interface{}
}

// F is a convenience function to create a logging field
func F(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// DefaultLogger implements Logger interface using Go's standard log package
type DefaultLogger struct {
	level  LogLevel
	logger *log.Logger
	fields []Field
}

// NewDefaultLogger creates a new logger with the specified level
func NewDefaultLogger(level LogLevel) *DefaultLogger {
	return &DefaultLogger{
		level:  level,
		logger: log.New(os.Stdout, "[OBCACHE] ", log.LstdFlags|log.Lmicroseconds),
		fields: make([]Field, 0),
	}
}

// Debug logs a debug message
func (dl *DefaultLogger) Debug(msg string, fields ...Field) {
	if dl.level <= LogLevelDebug {
		dl.log("DEBUG", msg, fields...)
	}
}

// Info logs an info message
func (dl *DefaultLogger) Info(msg string, fields ...Field) {
	if dl.level <= LogLevelInfo {
		dl.log("INFO", msg, fields...)
	}
}

// Warn logs a warning message
func (dl *DefaultLogger) Warn(msg string, fields ...Field) {
	if dl.level <= LogLevelWarn {
		dl.log("WARN", msg, fields...)
	}
}

// Error logs an error message
func (dl *DefaultLogger) Error(msg string, fields ...Field) {
	if dl.level <= LogLevelError {
		dl.log("ERROR", msg, fields...)
	}
}

// With creates a new logger with additional fields
func (dl *DefaultLogger) With(fields ...Field) Logger {
	newFields := make([]Field, len(dl.fields)+len(fields))
	copy(newFields, dl.fields)
	copy(newFields[len(dl.fields):], fields)

	return &DefaultLogger{
		level:  dl.level,
		logger: dl.logger,
		fields: newFields,
	}
}

func (dl *DefaultLogger) log(level, msg string, fields ...Field) {
	allFields := make([]Field, len(dl.fields)+len(fields))
	copy(allFields, dl.fields)
	copy(allFields[len(dl.fields):], fields)

	var fieldStrings []string
	for _, field := range allFields {
		fieldStrings = append(fieldStrings, fmt.Sprintf("%s=%v", field.Key, field.Value))
	}

	var logMsg string
	if len(fieldStrings) > 0 {
		logMsg = fmt.Sprintf("[%s] %s | %s", level, msg, strings.Join(fieldStrings, " "))
	} else {
		logMsg = fmt.Sprintf("[%s] %s", level, msg)
	}

	dl.logger.Println(logMsg)
}

// NoOpLogger is a logger that does nothing - useful for disabling logging
type NoOpLogger struct{}

// NewNoOpLogger creates a logger that discards all messages
func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}

func (nol *NoOpLogger) Debug(string, ...Field) {}
func (nol *NoOpLogger) Info(string, ...Field)  {}
func (nol *NoOpLogger) Warn(string, ...Field)  {}
func (nol *NoOpLogger) Error(string, ...Field) {}
func (nol *NoOpLogger) With(...Field) Logger   { return nol }

// LoggingConfig defines configuration for cache logging
type LoggingConfig struct {
	Logger Logger

	// LogCacheHits enables logging of cache hit events
	LogCacheHits bool

	// LogCacheMisses enables logging of cache miss events
	LogCacheMisses bool

	// LogEvictions enables logging of cache eviction events
	LogEvictions bool

	// LogInvalidations enables logging of cache invalidation events
	LogInvalidations bool

	// LogSlowOperations enables logging of operations that exceed the threshold
	LogSlowOperations bool
	SlowOpThreshold   time.Duration

	// IncludeValues determines whether to include actual cache values in logs (may be verbose)
	IncludeValues bool

	// MaxValueLength limits the length of values included in logs
	MaxValueLength int
}

// NewDefaultLoggingConfig creates a logging configuration with sensible defaults
func NewDefaultLoggingConfig(level LogLevel) *LoggingConfig {
	return &LoggingConfig{
		Logger:            NewDefaultLogger(level),
		LogCacheHits:      true,
		LogCacheMisses:    true,
		LogEvictions:      true,
		LogInvalidations:  true,
		LogSlowOperations: true,
		SlowOpThreshold:   100 * time.Millisecond,
		IncludeValues:     false,
		MaxValueLength:    100,
	}
}

// CreateLoggingHooks creates a set of hooks that implement cache event logging
func CreateLoggingHooks(config *LoggingConfig) *Hooks { //nolint:gocyclo // Acceptable complexity for configuration
	if config == nil || config.Logger == nil {
		return &Hooks{}
	}

	hooks := &Hooks{}
	logger := config.Logger

	// Add hit logging hooks with high priority (should run after metrics, before other logging)
	if config.LogCacheHits {
		hooks.AddOnHitCtxWithPriority(func(ctx context.Context, key string, value any, args []any) {
			fields := []Field{F("key", key), F("event", "cache_hit")}

			if config.IncludeValues {
				valueStr := truncateValue(fmt.Sprintf("%v", value), config.MaxValueLength)
				fields = append(fields, F("value", valueStr))
			}

			if len(args) > 0 {
				fields = append(fields, F("args_count", len(args)))
			}

			// Add context fields if available
			if ctx != nil {
				// Try both string keys (for compatibility) and typed keys
				if requestID := ctx.Value("request_id"); requestID != nil {
					fields = append(fields, F("request_id", requestID))
				}
				if userID := ctx.Value("user_id"); userID != nil {
					fields = append(fields, F("user_id", userID))
				}
			}

			logger.Debug("Cache hit", fields...)
		}, HookPriorityMedium)
	}

	// Add miss logging hooks
	if config.LogCacheMisses {
		hooks.AddOnMissCtxWithPriority(func(ctx context.Context, key string, args []any) {
			fields := []Field{F("key", key), F("event", "cache_miss")}

			if len(args) > 0 {
				fields = append(fields, F("args_count", len(args)))
			}

			// Add context fields if available
			if ctx != nil {
				if requestID := ctx.Value("request_id"); requestID != nil {
					fields = append(fields, F("request_id", requestID))
				}
			}

			logger.Info("Cache miss", fields...)
		}, HookPriorityMedium)
	}

	// Add eviction logging hooks
	if config.LogEvictions {
		hooks.AddOnEvictCtxWithPriority(func(_ context.Context, key string, value any, reason EvictReason, _ []any) {
			fields := []Field{
				F("key", key),
				F("event", "cache_evict"),
				F("reason", reason.String()),
			}

			if config.IncludeValues {
				valueStr := truncateValue(fmt.Sprintf("%v", value), config.MaxValueLength)
				fields = append(fields, F("value", valueStr))
			}

			logger.Info("Cache eviction", fields...)
		}, HookPriorityMedium)
	}

	// Add invalidation logging hooks
	if config.LogInvalidations {
		hooks.AddOnInvalidateCtxWithPriority(func(ctx context.Context, key string, args []any) {
			fields := []Field{F("key", key), F("event", "cache_invalidate")}

			// Add context fields if available
			if ctx != nil {
				if requestID := ctx.Value("request_id"); requestID != nil {
					fields = append(fields, F("request_id", requestID))
				}
			}

			logger.Info("Cache invalidation", fields...)
		}, HookPriorityMedium)
	}

	return hooks
}

// CreateSlowOperationLoggingHooks creates hooks that log slow cache operations
func CreateSlowOperationLoggingHooks(config *LoggingConfig) *Hooks {
	if config == nil || config.Logger == nil || !config.LogSlowOperations {
		return &Hooks{}
	}

	hooks := &Hooks{}

	// Track operation start times using context
	// type timingKey struct{} // unused for now

	// Pre-operation hook to record start time (would need to be integrated with cache operations)
	// For now, we'll create a utility function that users can call

	return hooks
}

// LoggingHookBuilder provides a fluent interface for creating logging hooks
type LoggingHookBuilder struct {
	config *LoggingConfig
}

// NewLoggingHookBuilder creates a new logging hook builder
func NewLoggingHookBuilder() *LoggingHookBuilder {
	return &LoggingHookBuilder{
		config: &LoggingConfig{
			Logger:          NewNoOpLogger(), // Default to no-op
			SlowOpThreshold: 100 * time.Millisecond,
			MaxValueLength:  100,
		},
	}
}

// WithLogger sets the logger to use
func (lhb *LoggingHookBuilder) WithLogger(logger Logger) *LoggingHookBuilder {
	lhb.config.Logger = logger
	return lhb
}

// WithLevel sets the logging level (creates a default logger)
func (lhb *LoggingHookBuilder) WithLevel(level LogLevel) *LoggingHookBuilder {
	lhb.config.Logger = NewDefaultLogger(level)
	return lhb
}

// EnableHitLogging enables cache hit logging
func (lhb *LoggingHookBuilder) EnableHitLogging() *LoggingHookBuilder {
	lhb.config.LogCacheHits = true
	return lhb
}

// EnableMissLogging enables cache miss logging
func (lhb *LoggingHookBuilder) EnableMissLogging() *LoggingHookBuilder {
	lhb.config.LogCacheMisses = true
	return lhb
}

// EnableEvictionLogging enables cache eviction logging
func (lhb *LoggingHookBuilder) EnableEvictionLogging() *LoggingHookBuilder {
	lhb.config.LogEvictions = true
	return lhb
}

// EnableInvalidationLogging enables cache invalidation logging
func (lhb *LoggingHookBuilder) EnableInvalidationLogging() *LoggingHookBuilder {
	lhb.config.LogInvalidations = true
	return lhb
}

// EnableAllLogging enables all types of cache event logging
func (lhb *LoggingHookBuilder) EnableAllLogging() *LoggingHookBuilder {
	lhb.config.LogCacheHits = true
	lhb.config.LogCacheMisses = true
	lhb.config.LogEvictions = true
	lhb.config.LogInvalidations = true
	return lhb
}

// EnableSlowOperationLogging enables logging of slow operations
func (lhb *LoggingHookBuilder) EnableSlowOperationLogging(threshold time.Duration) *LoggingHookBuilder {
	lhb.config.LogSlowOperations = true
	lhb.config.SlowOpThreshold = threshold
	return lhb
}

// IncludeValues enables including cache values in logs
func (lhb *LoggingHookBuilder) IncludeValues(maxLength int) *LoggingHookBuilder {
	lhb.config.IncludeValues = true
	lhb.config.MaxValueLength = maxLength
	return lhb
}

// Build creates the hooks configured by this builder
func (lhb *LoggingHookBuilder) Build() *Hooks {
	return CreateLoggingHooks(lhb.config)
}

// Helper functions

func truncateValue(value string, maxLength int) string {
	if len(value) <= maxLength {
		return value
	}
	return value[:maxLength-3] + "..."
}

// Example usage functions for documentation

// ExampleBasicLogging demonstrates basic cache event logging
func ExampleBasicLogging() {
	// Create a logger that logs INFO level and above
	loggingHooks := NewLoggingHookBuilder().
		WithLevel(LogLevelInfo).
		EnableAllLogging().
		Build()

	// Create cache with logging hooks
	config := NewDefaultConfig().WithHooks(loggingHooks)
	cache, err := New(config)
	if err != nil {
		panic(err) // Example code - handle appropriately in real usage
	}

	// These operations will now be logged
	if err := cache.Set("user:123", "John Doe", time.Hour); err != nil {
		log.Printf("Error setting cache: %v", err)
	}
	_, _ = cache.Get("user:123") // Will log cache hit
	_, _ = cache.Get("user:999") // Will log cache miss
	if err := cache.Delete("user:123"); err != nil {
		log.Printf("Error deleting from cache: %v", err)
	} // Will log deletion
}

// ExampleAdvancedLogging demonstrates advanced logging configuration
func ExampleAdvancedLogging() {
	// Create custom logger configuration
	customLogger := NewDefaultLogger(LogLevelDebug).
		With(F("service", "user-api"), F("version", "1.0.0"))

	loggingHooks := NewLoggingHookBuilder().
		WithLogger(customLogger).
		EnableAllLogging().
		IncludeValues(50).
		EnableSlowOperationLogging(50 * time.Millisecond).
		Build()

	cache, err := New(NewDefaultConfig().WithHooks(loggingHooks))
	if err != nil {
		panic(err) // Example code - handle appropriately in real usage
	}

	// Operations will be logged with service context
	if err := cache.Set("config:feature_flags", map[string]bool{"new_ui": true}, time.Hour); err != nil {
		log.Printf("Error setting feature flags: %v", err)
	}
	_, _ = cache.Get("config:feature_flags")
}

// ExampleConditionalLogging demonstrates conditional logging based on keys or context
func ExampleConditionalLogging() {
	logger := NewDefaultLogger(LogLevelInfo)
	hooks := &Hooks{}

	// Only log cache misses for critical keys
	hooks.AddOnMissCtxIf(func(_ context.Context, key string, _ []any) {
		logger.Error("Critical cache miss", F("key", key))
	}, KeyPrefixCondition("critical:"))

	// Log detailed debug info for specific users in development
	hooks.AddOnHitCtxIf(func(_ context.Context, key string, value any, _ []any) {
		logger.Debug("Debug cache hit", F("key", key), F("value", value))
	}, AndCondition(
		KeyPrefixCondition("user:"),
		ContextValueCondition("env", "development"),
	))

	cache, err := New(NewDefaultConfig().WithHooks(hooks))
	if err != nil {
		panic(err) // Example code - handle appropriately in real usage
	}

	// Only critical misses and dev user hits will be logged
	_, _ = cache.Get("critical:payment_config") // Will log if miss
	_, _ = cache.Get("user:123")                // Will log detailed info if in dev environment
}
