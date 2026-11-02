// Package obcache memoizes Go functions, with TTL, eviction and stampede protection.
//
// # Function Wrapping
//
// Wrap returns a function with the same signature that caches its results.
// Concurrent callers for one key share one call.
//
//	cache, err := obcache.New(obcache.NewDefaultConfig())
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer cache.Close()
//
//	cachedUser := obcache.Wrap(cache, fetchUser, obcache.WithTTL(5*time.Minute))
//	user, err := cachedUser(123) // queries
//	user, err = cachedUser(123)  // cache hit
//
// A context.Context first parameter is passed through and left out of the key. An
// error return is not cached unless WithErrorCaching is set.
//
// # Direct Use
//
//	err := cache.Set("user:123", user, time.Hour)
//	value, found := cache.Get("user:123")
//
// # Invalidation
//
// Name the function with WithName, then delete by the key its arguments produce:
//
//	cachedUser := obcache.Wrap(cache, fetchUser, obcache.WithName("user"))
//	err := cache.Delete(cache.Key("user", 123))
//
// # Stores
//
// NewDefaultConfig selects the memory store, with LRU, LFU or FIFO eviction.
// NewRedisConfig and NewRedisConfigWithClient select Redis. WithCompression adds gzip
// or deflate to either. Prometheus metrics live in pkg/metrics, and importing this
// package alone does not link them.
//
// # Limits
//
// Memory: Get and Wrap return the stored value, not a copy. Do not mutate results.
//
// Side effects: a wrapped call that hits does not run the function. A wrapped function
// with no results runs once per key per TTL: use that to dedupe, or call the function
// directly if every call must run.
//
// Redis or compression: every value passes through encoding/json, at any size. Wrap
// decodes hits to the declared return type and panics at startup on return types that
// cannot round-trip: interfaces (including any and map[string]any), unexported fields,
// json:"-", func, chan, complex, one-way marshalers. For a dynamic shape, return
// json.RawMessage. A value JSON cannot encode (NaN, cycles) is not cached. JSON also
// loses invalid UTF-8, a time zone's name, and an empty slice or map under omitempty.
// Set and Get on those paths are unchecked: unexported fields are lost, and numbers
// return as float64, losing precision above 2^53.
//
// Redis names: every Wrap needs WithName or WithKeyFunc. A name is shared by every
// process on that Redis and key prefix, and binding is per Cache, so services sharing
// Redis need distinct KeyPrefix values. A field added to a return struct under a kept
// name reads as zero until TTL; bump the name. A field renamed or removed is a miss.
//
// Identity: a name binds one Wrap, and only a top-level function may Wrap under it
// again. Wrap once, at setup. For a closure or method value wrapped per request, use
// WithKeyFunc; unnamed, it never hits.
//
// Backends: refusals apply only on Redis and compression, so a program tested on memory
// can panic at its first start on Redis. Name every Wrap if any environment uses Redis,
// Wrap at startup, and test against the backend you ship.
//
// Context: ctx is not in the key. Anything that varies by user belongs in the arguments.
//
// Trust: anyone who can write to your Redis can forge any return value. Keys up to 64
// bytes hold arguments in clear; a hash above that is not secrecy. A custom key function
// must be injective and namespaced. MarshalText on an argument type must be injective.
//
// Errors: WithErrorCaching panics at Wrap on Redis or compression.
//
// Concurrency: stampede protection is per process. A function with no error return
// cannot honor a waiter's ctx. OnEvict does not fire on Redis.
//
// Outages: a Redis error is a miss, and Wrap drops a failed write; there is no error
// stat or hook. With Redis down, every call runs the function after waiting on Redis,
// about a second with a client obcache builds.
//
// Operations: a client you pass in needs ContextTimeoutEnabled and its own timeouts, and
// is the route to TLS. New pings a client it builds; pass a Client to skip that. Stats
// and Len scan the prefix on Redis. DebugHandler has no authentication.
package obcache
