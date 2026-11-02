package obcache

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/1mb-dev/obcache-go/v2/internal/singleflight"
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
	opts := &WrapOptions{TTL: cache.config.DefaultTTL}

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

	keyOf, err := cache.bindKey(fnType, opts)
	if err != nil {
		panic("obcache.Wrap: " + err.Error())
	}

	// Create the wrapper function
	wrapper := reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		return executeWrappedFunction(cache, fnValue, fnType, opts, keyOf, args)
	})

	return wrapper.Interface().(T)
}

// writeBudget bounds the store write after a call. The computed value outlives its
// caller's deadline, and waiters sit behind this write, so it gets its own limit.
const writeBudget = 250 * time.Millisecond

// maxAbortRetries is how many aborted leaders a caller waits behind before it
// calls the function itself.
const maxAbortRetries = 2

// flight is what a singleflight leader shares with its waiters.
type flight struct {
	results       []reflect.Value
	leaderCtxDone bool
}

// keyBinding turns a call's arguments into a key, or reports that they have none.
type keyBinding func(args []any) (string, bool)

// bindKey resolves how a wrapped function keys its calls, once, at Wrap time.
func (c *Cache) bindKey(fnType reflect.Type, opts *WrapOptions) (keyBinding, error) {
	if opts.KeyFunc != nil {
		return func(args []any) (string, bool) { return opts.KeyFunc(args), true }, nil
	}
	if custom := c.config.KeyGenFunc; custom != nil {
		return func(args []any) (string, bool) { return custom(args), true }, nil
	}
	if opts.DisableCache {
		return nil, nil
	}
	if err := checkParams(fnType); err != nil {
		return nil, err
	}
	return func(args []any) (string, bool) {
		key, err := encodeArgs(args)
		return key, err == nil
	}, nil
}

// executeWrappedFunction handles the core wrapping logic
func executeWrappedFunction(cache *Cache, fnValue reflect.Value, fnType reflect.Type, opts *WrapOptions, keyOf keyBinding, args []reflect.Value) []reflect.Value {
	// If caching is disabled, call original function directly
	if opts.DisableCache {
		return call(fnValue, fnType, args)
	}

	ctx, keyArgs := extractContextAndArgs(fnType, args)
	key, ok := keyOf(keyArgs)
	if !ok {
		return call(fnValue, fnType, args) // a value with no encoding runs uncached
	}

	hasErr := hasErrorReturn(fnType)

	store := func(value any, ttl time.Duration) {
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeBudget)
		defer cancel()
		_ = cache.SetContext(writeCtx, key, value, ttl)
	}

	// The write happens here so it runs exactly once, whether or not anyone is waiting.
	compute := func() flight {
		results := call(fnValue, fnType, args)
		if hasErr && !results[len(results)-1].IsNil() {
			if ctx.Err() != nil {
				// The leader's own cancellation, whatever the function wrapped it in.
				return flight{results: results, leaderCtxDone: true}
			}
			if opts.CacheErrors {
				errorTTL := opts.ErrorTTL
				if errorTTL == 0 {
					errorTTL = opts.TTL
				}
				store(cachedError{Err: results[len(results)-1].Interface().(error)}, errorTTL)
			}
			return flight{results: results}
		}
		store(storedValue(results, valueCount(fnType)), opts.TTL)
		return flight{results: results}
	}

	waitCtx := ctx
	if !hasErr {
		waitCtx = context.Background() // no error slot to report a wait timeout through
	}

	cache.stats.incInFlight()
	defer cache.stats.decInFlight()

	for aborts := 0; ; aborts++ {
		if cachedValue, found := cache.GetContext(ctx, key); found {
			if results, ok := cachedResults(cachedValue, fnType, hasErr); ok {
				return results
			}
		}
		if aborts == maxAbortRetries {
			// Queueing behind leaders that keep failing is worse than no cache.
			return compute().results
		}
		f, leader, err := cache.sf.Do(waitCtx, key, compute)
		switch {
		case errors.Is(err, singleflight.ErrAborted):
			continue
		case err != nil:
			return createErrorReturn(fnType, err)
		case f.leaderCtxDone && !leader && ctx.Err() == nil:
			continue
		}
		return f.results
	}
}

func call(fnValue reflect.Value, fnType reflect.Type, args []reflect.Value) []reflect.Value {
	if fnType.IsVariadic() {
		return fnValue.CallSlice(args)
	}
	return fnValue.Call(args)
}

// extractContextAndArgs extracts context and key args from function arguments
func extractContextAndArgs(fnType reflect.Type, args []reflect.Value) (context.Context, []any) {
	ctx := context.Background()
	var keyArgs []any

	// Detect context.Context as first parameter
	if len(args) > 0 && fnType.In(0) == contextType {
		// A nil context is possible: the parameter is an interface.
		if c, ok := args[0].Interface().(context.Context); ok {
			ctx = c
		}
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

var (
	errorType   = reflect.TypeFor[error]()
	contextType = reflect.TypeFor[context.Context]()
)

// hasErrorReturn reports whether the last result is an error. A struct error type
// has no "no error" value, so it counts as an ordinary result.
func hasErrorReturn(fnType reflect.Type) bool {
	n := fnType.NumOut()
	if n == 0 {
		return false
	}
	last := fnType.Out(n - 1)
	return last.Implements(errorType) && (last.Kind() == reflect.Interface || last.Kind() == reflect.Pointer)
}

// valueCount is the number of results excluding a trailing error.
func valueCount(fnType reflect.Type) int {
	if hasErrorReturn(fnType) {
		return fnType.NumOut() - 1
	}
	return fnType.NumOut()
}

// storedValue is the form results take in the store and in hooks.
func storedValue(results []reflect.Value, n int) any {
	switch n {
	case 0:
		return nil
	case 1:
		return results[0].Interface()
	}
	values := make([]any, n)
	for i := range n {
		values[i] = results[i].Interface()
	}
	return values
}

// cachedResults rebuilds a call's results from a stored value. A value that does
// not fit the function's results is a miss, never a panic.
func cachedResults(v any, fnType reflect.Type, hasErr bool) ([]reflect.Value, bool) {
	if ce, isErr := v.(cachedError); isErr {
		if !hasErr {
			return nil, false
		}
		return createErrorReturn(fnType, ce.Err), true
	}

	results := make([]reflect.Value, fnType.NumOut())
	if hasErr {
		results[len(results)-1] = reflect.Zero(fnType.Out(len(results) - 1))
	}
	n := valueCount(fnType)
	if n == 0 {
		return results, true
	}
	values := []any{v}
	if n > 1 {
		var ok bool
		if values, ok = v.([]any); !ok || len(values) != n {
			return nil, false
		}
	}
	for i := range n {
		t := fnType.Out(i)
		if values[i] == nil {
			switch t.Kind() {
			case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
				results[i] = reflect.Zero(t)
				continue
			}
			return nil, false
		}
		rv := reflect.ValueOf(values[i])
		if !rv.Type().AssignableTo(t) {
			return nil, false
		}
		results[i] = rv.Convert(t)
	}
	return results, true
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
// It sees no cache, so it cannot report refusals that depend on the store
func ValidateWrappableFunction(fn any) error {
	fnType := reflect.TypeOf(fn)

	if fnType.Kind() != reflect.Func {
		return fmt.Errorf("not a function: %T", fn)
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
