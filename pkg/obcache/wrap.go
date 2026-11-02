package obcache

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	metrics "github.com/1mb-dev/obcache-go/v2/internal/metricsapi"
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

	// Name is the wrapped function's stable identity within the cache
	Name string

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

// WithName names the wrapped function: its entries are keyed "name|args". A name
// binds one Wrap, and only a top-level function may Wrap under it again, so a
// closure or method value is wrapped once, at setup. For a per-request Wrap, use
// WithKeyFunc. A name must not contain '|' or '#'.
func WithName(name string) WrapOption {
	return func(opts *WrapOptions) {
		opts.Name = name
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

	// Refusals run before bindKey, which binds a name only once nothing can refuse.
	err := cache.checkWrap(fnType, opts)
	var keyOf keyBinding
	if err == nil {
		keyOf, err = cache.bindKey(fnValue, fnType, opts)
	}
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

type nameBinding struct {
	pc uintptr
	t  reflect.Type
}

// bindKey resolves the function's identity once, at Wrap time. Failure is loud or
// cold, never a read of another function's entries.
func (c *Cache) bindKey(fnValue reflect.Value, fnType reflect.Type, opts *WrapOptions) (keyBinding, error) {
	if opts.KeyFunc != nil {
		if opts.Name != "" {
			return nil, errors.New("WithName and WithKeyFunc are mutually exclusive")
		}
		return func(args []any) (string, bool) { return callKeyFunc(opts.KeyFunc, args) }, nil
	}

	// Checks come before identity binds a name, so a refused Wrap leaves no name.
	if !opts.DisableCache && c.config.KeyGenFunc == nil {
		if err := checkParams(fnType); err != nil {
			return nil, err
		}
	}
	name, err := c.identity(fnValue, fnType, opts)
	if err != nil {
		return nil, err
	}
	if opts.DisableCache {
		return nil, nil // nothing is keyed or stored
	}
	return func(args []any) (string, bool) { return c.keyFor(name, args) }, nil
}

// identity returns the name a wrapped function keys under, binding it if it is
// stable. A name that cannot be trusted across Wraps gets a fresh id, so its
// entries go cold rather than cross.
func (c *Cache) identity(fnValue reflect.Value, fnType reflect.Type, opts *WrapOptions) (string, error) {
	pc := fnValue.Pointer()
	fnName := runtime.FuncForPC(pc).Name()
	name := opts.Name

	if name == "" {
		if opts.DisableCache {
			return "", nil
		}
		if c.config.StoreType == StoreTypeRedis {
			return "", errors.New("a Redis cache needs a stable function name: add WithName or WithKeyFunc")
		}
		if !isTopLevel(fnName) {
			return fmt.Sprintf("#%d", c.wrapSeq.Add(1)), nil
		}
		name = fnName
	} else if strings.ContainsAny(name, "|#") {
		return "", fmt.Errorf("name %q must not contain '|' or '#'", name)
	}

	binding := nameBinding{pc, fnType}
	if prev, loaded := c.names.LoadOrStore(name, binding); loaded && (prev != binding || !isTopLevel(fnName)) {
		return "", fmt.Errorf("name %q is already bound: Wrap once at setup, or use WithKeyFunc", name)
	}
	return name, nil
}

// isTopLevel reports whether a runtime function name is pkg.Func. Closures, method
// values, method expressions and generics all carry a suffix, and reflect.MakeFunc
// results report a reflect name, so anything else falls to a per-Wrap id.
func isTopLevel(name string) bool {
	base := name[strings.LastIndexByte(name, '/')+1:]
	return strings.Count(base, ".") == 1 && !strings.ContainsAny(base, "[-(") && !strings.HasPrefix(base, "reflect.")
}

func (c *Cache) keyFor(name string, args []any) (string, bool) {
	keyFunc := c.config.KeyGenFunc
	if keyFunc == nil {
		keyFunc = DefaultKeyFunc
	}
	key, ok := callKeyFunc(keyFunc, args)
	if !ok {
		return "", false
	}
	return name + "|" + key, true
}

// Key returns the key a Wrap with WithName(name) stores args under, for use with
// Delete. It panics on a name no Wrap bound in this Cache and on args that differ
// from the wrapped function's parameters, because either would make Delete a
// silent no-op. A process that only invalidates binds the name with one uncalled
// Wrap of a function with the same signature.
func (c *Cache) Key(name string, args ...any) string {
	bound, ok := c.names.Load(name)
	if !ok {
		panic(fmt.Sprintf("obcache.Key: no Wrap named %q in this Cache; Wrap a function with this signature under WithName(%q)", name, name))
	}
	if err := checkKeyArgs(bound.(nameBinding).t, args); err != nil {
		panic(fmt.Sprintf("obcache.Key(%q): %v", name, err))
	}
	key, ok := c.keyFor(name, args)
	if !ok {
		panic(fmt.Sprintf("obcache.Key(%q): args have no key encoding", name))
	}
	return key
}

func checkKeyArgs(fnType reflect.Type, args []any) error {
	offset := 0
	if fnType.NumIn() > 0 && fnType.In(0) == contextType {
		offset = 1
	}
	if len(args) != fnType.NumIn()-offset {
		return fmt.Errorf("got %d args, the wrapped function takes %d", len(args), fnType.NumIn()-offset)
	}
	for i, arg := range args {
		want := fnType.In(i + offset)
		if want.Kind() == reflect.Interface {
			continue
		}
		if got := reflect.TypeOf(arg); got != want {
			return fmt.Errorf("arg %d is %v, the wrapped function takes %v", i, got, want)
		}
	}
	return nil
}

// checkWrap refuses at Wrap what would misbehave on a later cache hit.
func (c *Cache) checkWrap(fnType reflect.Type, opts *WrapOptions) error {
	if !c.serializes() || opts.DisableCache {
		return nil
	}
	if opts.CacheErrors {
		return errors.New("WithErrorCaching needs the memory store without compression: errors do not survive serialization")
	}
	for i := range valueCount(fnType) {
		if err := checkReturn(fnType.Out(i)); err != nil {
			return fmt.Errorf("return %d: %w", i, err)
		}
	}
	return nil
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
		if !hasErr && ctx.Err() != nil {
			// With no error to say so, a result computed under cancellation may be
			// partial. It goes to the leader only.
			return flight{results: results, leaderCtxDone: true}
		}
		store(storedValue(results, valueCount(fnType)), opts.TTL)
		return flight{results: results}
	}

	// Reporting a wait timeout needs a result that can hold ctx.Err(). A function
	// returning a concrete error type has none, so its waiters keep waiting.
	waitCtx := ctx
	if !hasErr || !errorType.AssignableTo(fnType.Out(fnType.NumOut()-1)) {
		waitCtx = context.Background()
	}

	cache.stats.incInFlight()
	defer cache.stats.decInFlight()

	for aborts := 0; ; aborts++ {
		// A retry is the same call: its misses were counted the first time.
		if results, found := cache.lookup(ctx, key, fnType, hasErr, aborts == 0); found {
			return results
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
		case !leader && !fits(f.results, fnType):
			// Another function's flight under the same key, possible with WithKeyFunc.
			continue
		}
		return f.results
	}
}

// fits reports whether shared results can be returned from a function of fnType.
func fits(results []reflect.Value, fnType reflect.Type) bool {
	if len(results) != fnType.NumOut() {
		return false
	}
	for i, r := range results {
		if !r.Type().AssignableTo(fnType.Out(i)) {
			return false
		}
	}
	return true
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

// lookup returns a wrapped function's cached results. Anything that does not read
// back as this function's results is a miss, counted before it is reported, so the
// hit rate cannot hide an entry that recomputes on every call.
func (c *Cache) lookup(ctx context.Context, key string, fnType reflect.Type, hasErr, countMiss bool) ([]reflect.Value, bool) {
	start := time.Now()
	defer func() { c.recordCacheOperation(metrics.OperationGet, time.Since(start)) }()

	e, ok := c.store.Get(ctx, key)
	if !ok {
		if countMiss {
			c.miss(ctx, key)
		}
		return nil, false
	}

	var results []reflect.Value
	raw, serialized, err := c.encoded(e)
	switch {
	case err != nil:
		ok = false
	case serialized:
		results, ok = decodeResults(raw, fnType, hasErr)
	default:
		results, ok = cachedResults(e.Value, fnType, hasErr)
	}
	if !ok {
		if countMiss {
			c.stats.incDecodeFailures()
			c.miss(ctx, key)
		}
		return nil, false
	}

	// Hooks see what the caller is about to receive, in the form a miss stores.
	// Building it allocates, so only when a hook will read it.
	var hooked any
	if c.hooks != nil && c.hooks.hasOnHit() {
		hooked = e.Value
		if _, isErr := e.Value.(cachedError); !isErr {
			hooked = storedValue(results, valueCount(fnType))
		}
	}
	c.hit(ctx, key, hooked)
	return results, true
}

// decodeResults decodes stored JSON straight into the declared result types. Going
// through any would turn every number into a float64 first.
func decodeResults(raw []byte, fnType reflect.Type, hasErr bool) ([]reflect.Value, bool) {
	results := emptyResults(fnType, hasErr)
	n := valueCount(fnType)
	switch n {
	case 0:
		return results, true
	case 1:
		return results, decodeResult(raw, fnType.Out(0), results, 0)
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil || len(parts) != n {
		return nil, false
	}
	for i := range n {
		if !decodeResult(parts[i], fnType.Out(i), results, i) {
			return nil, false
		}
	}
	return results, true
}

// decodeResult is strict because JSON decoding is lenient: unknown fields and a
// null for a value type both decode without error, to a zero value that would
// pass as a hit.
func decodeResult(raw []byte, t reflect.Type, results []reflect.Value, i int) bool {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		switch t.Kind() {
		case reflect.Pointer, reflect.Map, reflect.Slice:
		default:
			return false
		}
	}
	p := reflect.New(t)
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(p.Interface()) != nil {
		return false
	}
	results[i] = p.Elem()
	return true
}

func emptyResults(fnType reflect.Type, hasErr bool) []reflect.Value {
	results := make([]reflect.Value, fnType.NumOut())
	if hasErr {
		results[len(results)-1] = reflect.Zero(fnType.Out(len(results) - 1))
	}
	return results
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

	results := emptyResults(fnType, hasErr)
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

// WrapSimple wraps a function without an error return.
//
// Deprecated: use Wrap.
func WrapSimple[T any, R any](cache *Cache, fn func(T) R, options ...WrapOption) func(T) R {
	wrapped := Wrap(cache, fn, options...)
	return wrapped
}

// WrapWithError wraps a function returning (R, error).
//
// Deprecated: use Wrap.
func WrapWithError[T any, R any](cache *Cache, fn func(T) (R, error), options ...WrapOption) func(T) (R, error) {
	wrapped := Wrap(cache, fn, options...)
	return wrapped
}

// WrapFunc0 wraps a function with no arguments.
//
// Deprecated: use Wrap.
func WrapFunc0[R any](cache *Cache, fn func() R, options ...WrapOption) func() R {
	return Wrap(cache, fn, options...)
}

// WrapFunc1 wraps a function with one argument.
//
// Deprecated: use Wrap.
func WrapFunc1[T any, R any](cache *Cache, fn func(T) R, options ...WrapOption) func(T) R {
	return Wrap(cache, fn, options...)
}

// WrapFunc2 wraps a function with two arguments.
//
// Deprecated: use Wrap.
func WrapFunc2[T1, T2, R any](cache *Cache, fn func(T1, T2) R, options ...WrapOption) func(T1, T2) R {
	return Wrap(cache, fn, options...)
}

// WrapFunc0WithError wraps a function with no arguments that returns an error.
//
// Deprecated: use Wrap.
func WrapFunc0WithError[R any](cache *Cache, fn func() (R, error), options ...WrapOption) func() (R, error) {
	return Wrap(cache, fn, options...)
}

// WrapFunc1WithError wraps a function with one argument that returns an error.
//
// Deprecated: use Wrap.
func WrapFunc1WithError[T, R any](cache *Cache, fn func(T) (R, error), options ...WrapOption) func(T) (R, error) {
	return Wrap(cache, fn, options...)
}

// WrapFunc2WithError wraps a function with two arguments that returns an error.
//
// Deprecated: use Wrap.
func WrapFunc2WithError[T1, T2, R any](cache *Cache, fn func(T1, T2) (R, error), options ...WrapOption) func(T1, T2) (R, error) {
	return Wrap(cache, fn, options...)
}

// ValidateWrappableFunction reports what Wrap would refuse about fn with any store.
// Refusals that depend on the store, such as return types on Redis, need a Cache.
func ValidateWrappableFunction(fn any) error {
	fnType := reflect.TypeOf(fn)
	if fnType == nil || fnType.Kind() != reflect.Func {
		return fmt.Errorf("not a function: %T", fn)
	}
	return checkParams(fnType)
}
