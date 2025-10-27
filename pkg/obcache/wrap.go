package obcache

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

// cachedError represents an error that has been cached
type cachedError struct {
	Err error
}

// WrapOptions holds configuration options for function wrapping
type WrapOptions struct {
	// TTL overrides the default TTL for this wrapped function
	TTL time.Duration

	// KeyFunc overrides the default key generation function
	KeyFunc KeyGenFunc

	// DisableCache disables caching for this function (useful for testing)
	DisableCache bool

	// CacheErrors controls whether errors should be cached
	// When true, errors will be cached with the specified ErrorTTL
	CacheErrors bool

	// ErrorTTL is the TTL for cached errors (defaults to TTL if not set)
	ErrorTTL time.Duration
}

// WrapOption is a function that configures WrapOptions
type WrapOption func(*WrapOptions)

// WithTTL sets a custom TTL for the wrapped function
func WithTTL(ttl time.Duration) WrapOption {
	return func(opts *WrapOptions) {
		opts.TTL = ttl
	}
}

// WithKeyFunc sets a custom key generation function for the wrapped function
func WithKeyFunc(keyFunc KeyGenFunc) WrapOption {
	return func(opts *WrapOptions) {
		opts.KeyFunc = keyFunc
	}
}

// WithoutCache disables caching for the wrapped function
func WithoutCache() WrapOption {
	return func(opts *WrapOptions) {
		opts.DisableCache = true
	}
}

// WithErrorCaching enables caching of errors with the same TTL as successful results
func WithErrorCaching() WrapOption {
	return func(opts *WrapOptions) {
		opts.CacheErrors = true
	}
}

// WithErrorTTL enables error caching with a specific TTL
func WithErrorTTL(ttl time.Duration) WrapOption {
	return func(opts *WrapOptions) {
		opts.CacheErrors = true
		opts.ErrorTTL = ttl
	}
}

// Wrap wraps any function with caching using Go generics
// T must be a function type
func Wrap[T any](cache *Cache, fn T, options ...WrapOption) T {
	opts := &WrapOptions{
		TTL:     cache.config.DefaultTTL,
		KeyFunc: cache.getKeyGenFunc(),
	}

	for _, opt := range options {
		opt(opts)
	}

	return wrapFunction(cache, fn, opts)
}

// wrapFunction performs the actual function wrapping using reflection
func wrapFunction[T any](cache *Cache, fn T, opts *WrapOptions) T {
	fnValue := reflect.ValueOf(fn)
	fnType := fnValue.Type()

	// Validate that T is actually a function
	if fnType.Kind() != reflect.Func {
		panic("obcache.Wrap: argument must be a function")
	}

	// Create the wrapper function
	wrapper := reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		return executeWrappedFunction(cache, fnValue, fnType, opts, args)
	})

	return wrapper.Interface().(T)
}

// executeWrappedFunction handles the core wrapping logic
func executeWrappedFunction(cache *Cache, fnValue reflect.Value, fnType reflect.Type, opts *WrapOptions, args []reflect.Value) []reflect.Value {
	ctx, keyArgs := extractContextAndArgs(fnType, args)
	key := opts.KeyFunc(keyArgs)

	// If caching is disabled, call original function directly
	if opts.DisableCache {
		return fnValue.Call(args)
	}

	hasErrorReturn := hasErrorReturn(fnType)

	// Try to get from cache first using context
	if cachedValue, found := cache.GetContext(ctx, key); found {
		return convertCachedValue(cachedValue, fnType, hasErrorReturn)
	}

	return executeFunctionWithSingleflight(cache, ctx, fnValue, fnType, opts, args, key, hasErrorReturn)
}

// extractContextAndArgs extracts context and key args from function arguments
func extractContextAndArgs(fnType reflect.Type, args []reflect.Value) (context.Context, []any) {
	ctx := context.Background()
	var keyArgs []any

	// Detect context.Context as first parameter
	if len(args) > 0 && fnType.In(0).String() == "context.Context" {
		// First parameter is context.Context
		ctx = args[0].Interface().(context.Context)
		// Use remaining args for key generation
		keyArgs = make([]any, len(args)-1)
		for i := 1; i < len(args); i++ {
			keyArgs[i-1] = args[i].Interface()
		}
	} else {
		// No context parameter, use all args for key generation
		keyArgs = make([]any, len(args))
		for i, arg := range args {
			keyArgs[i] = arg.Interface()
		}
	}

	return ctx, keyArgs
}

// hasErrorReturn checks if function returns error as last parameter
func hasErrorReturn(fnType reflect.Type) bool {
	return fnType.NumOut() >= 2 &&
		fnType.Out(fnType.NumOut()-1).Implements(reflect.TypeOf((*error)(nil)).Elem())
}

// executeFunctionWithSingleflight executes the function with singleflight pattern
func executeFunctionWithSingleflight(cache *Cache, ctx context.Context, fnValue reflect.Value, fnType reflect.Type, opts *WrapOptions, args []reflect.Value, key string, hasErrorReturn bool) []reflect.Value {
	// Use singleflight to prevent duplicate calls
	compute := func() (any, error) {
		results := fnValue.Call(args)
		return processResults(results, hasErrorReturn)
	}

	// Execute with singleflight
	cache.stats.incInFlight()
	defer cache.stats.decInFlight()

	value, err, shared := cache.sf.Do(key, compute)

	if err != nil {
		// Cache errors if enabled
		if opts.CacheErrors && !shared {
			errorTTL := opts.ErrorTTL
			if errorTTL == 0 {
				errorTTL = opts.TTL
			}
			_ = cache.SetContext(ctx, key, cachedError{Err: err}, errorTTL) // Cache error with context
		}
		// Return the error in the function's expected format
		return createErrorReturn(fnType, err)
	}

	// Store in cache if this wasn't a shared call
	if !shared {
		_ = cache.SetContext(ctx, key, value, opts.TTL) // Cache result with context
	}

	// Convert the result back to the expected format
	return convertComputedValue(value, fnType, hasErrorReturn)
}

// processResults processes function results for caching
func processResults(results []reflect.Value, hasErrorReturn bool) (any, error) {
	if hasErrorReturn {
		return processResultsWithError(results)
	}
	return processResultsWithoutError(results)
}

// processResultsWithError handles results from functions that return error
func processResultsWithError(results []reflect.Value) (any, error) {
	// Handle (T, error) return pattern
	errResult := results[len(results)-1]
	if !errResult.IsNil() {
		// Return error for conditional caching (handled in caller)
		return nil, errResult.Interface().(error)
	}

	// Cache the successful result (all values except error)
	if len(results) == 2 {
		// Single value + error
		return results[0].Interface(), nil
	}
	// Multiple values + error
	values := make([]any, len(results)-1)
	for i := 0; i < len(results)-1; i++ {
		values[i] = results[i].Interface()
	}
	return values, nil
}

// processResultsWithoutError handles results from functions without error return
func processResultsWithoutError(results []reflect.Value) (any, error) {
	if len(results) == 1 {
		return results[0].Interface(), nil
	}
	// Multiple return values
	values := make([]any, len(results))
	for i, result := range results {
		values[i] = result.Interface()
	}
	return values, nil
}

// convertCachedValue converts a cached value back to the expected return format
func convertCachedValue(cachedValue any, fnType reflect.Type, hasErrorReturn bool) []reflect.Value {
	// Check if this is a cached error
	if ce, ok := cachedValue.(cachedError); ok {
		return createErrorReturn(fnType, ce.Err)
	}

	numOut := fnType.NumOut()
	results := make([]reflect.Value, numOut)

	if hasErrorReturn {
		// Set error to nil
		results[numOut-1] = reflect.Zero(fnType.Out(numOut - 1))

		if numOut == 2 {
			// Single value + error
			results[0] = reflect.ValueOf(cachedValue)
		} else {
			// Multiple values + error
			values := cachedValue.([]any)
			for i := 0; i < numOut-1; i++ {
				results[i] = reflect.ValueOf(values[i])
			}
		}
	} else {
		// No error return
		if numOut == 1 {
			results[0] = reflect.ValueOf(cachedValue)
		} else {
			values := cachedValue.([]any)
			for i, value := range values {
				results[i] = reflect.ValueOf(value)
			}
		}
	}

	return results
}

// convertComputedValue converts a computed value to the expected return format
func convertComputedValue(value any, fnType reflect.Type, hasErrorReturn bool) []reflect.Value {
	numOut := fnType.NumOut()
	results := make([]reflect.Value, numOut)

	if hasErrorReturn {
		// Set error to nil (since we only cache successful results)
		results[numOut-1] = reflect.Zero(fnType.Out(numOut - 1))

		if numOut == 2 {
			// Single value + error
			results[0] = reflect.ValueOf(value)
		} else {
			// Multiple values + error
			values := value.([]any)
			for i := 0; i < numOut-1; i++ {
				results[i] = reflect.ValueOf(values[i])
			}
		}
	} else {
		// No error return
		if numOut == 1 {
			results[0] = reflect.ValueOf(value)
		} else {
			values := value.([]any)
			for i, value := range values {
				results[i] = reflect.ValueOf(value)
			}
		}
	}

	return results
}

// createErrorReturn creates a return value slice with the given error
func createErrorReturn(fnType reflect.Type, err error) []reflect.Value {
	numOut := fnType.NumOut()
	results := make([]reflect.Value, numOut)

	// Set all non-error returns to zero values
	for i := 0; i < numOut-1; i++ {
		results[i] = reflect.Zero(fnType.Out(i))
	}

	// Set the error
	results[numOut-1] = reflect.ValueOf(err)

	return results
}

// WrapSimple is a convenience function for wrapping simple functions without error returns
// This is a specialized version that's easier to use for simple cases
func WrapSimple[T any, R any](cache *Cache, fn func(T) R, options ...WrapOption) func(T) R {
	wrapped := Wrap(cache, fn, options...)
	return wrapped
}

// WrapWithError is a convenience function for wrapping functions that return (T, error)
func WrapWithError[T any, R any](cache *Cache, fn func(T) (R, error), options ...WrapOption) func(T) (R, error) {
	wrapped := Wrap(cache, fn, options...)
	return wrapped
}

// Example wrapper functions for common patterns

// WrapFunc0 wraps a function with no arguments
func WrapFunc0[R any](cache *Cache, fn func() R, options ...WrapOption) func() R {
	return Wrap(cache, fn, options...)
}

// WrapFunc1 wraps a function with one argument
func WrapFunc1[T any, R any](cache *Cache, fn func(T) R, options ...WrapOption) func(T) R {
	return Wrap(cache, fn, options...)
}

// WrapFunc2 wraps a function with two arguments
func WrapFunc2[T1, T2, R any](cache *Cache, fn func(T1, T2) R, options ...WrapOption) func(T1, T2) R {
	return Wrap(cache, fn, options...)
}

// WrapFunc0WithError wraps a function with no arguments that returns an error
func WrapFunc0WithError[R any](cache *Cache, fn func() (R, error), options ...WrapOption) func() (R, error) {
	return Wrap(cache, fn, options...)
}

// WrapFunc1WithError wraps a function with one argument that returns an error
func WrapFunc1WithError[T, R any](cache *Cache, fn func(T) (R, error), options ...WrapOption) func(T) (R, error) {
	return Wrap(cache, fn, options...)
}

// WrapFunc2WithError wraps a function with two arguments that returns an error
func WrapFunc2WithError[T1, T2, R any](cache *Cache, fn func(T1, T2) (R, error), options ...WrapOption) func(T1, T2) (R, error) {
	return Wrap(cache, fn, options...)
}

// ValidateWrappableFunction checks if a function can be wrapped
// This is useful for providing better error messages at runtime
func ValidateWrappableFunction(fn any) error {
	fnType := reflect.TypeOf(fn)

	if fnType.Kind() != reflect.Func {
		return fmt.Errorf("not a function: %T", fn)
	}

	// Check if function is variadic (not currently supported)
	if fnType.IsVariadic() {
		return fmt.Errorf("variadic functions are not currently supported")
	}

	// Validate return types
	numOut := fnType.NumOut()
	if numOut == 0 {
		return fmt.Errorf("functions with no return values cannot be cached")
	}

	// If there are multiple returns, the last one should be error
	if numOut > 1 {
		lastOut := fnType.Out(numOut - 1)
		if !lastOut.Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			return fmt.Errorf("multi-return functions must have error as the last return value")
		}
	}

	return nil
}
