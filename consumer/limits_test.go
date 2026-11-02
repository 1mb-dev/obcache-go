package consumer

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
	"github.com/alicebob/miniredis/v2"
)

// Each test pins a line of the documented limits, so the text cannot outlive the
// behavior. Passing means the limit still holds; update the docs if one fails.

func TestLimitGetReturnsJSONTypesOnRedis(t *testing.T) {
	cfg, _ := miniConfig(t)
	c := memoryCache(t, cfg)
	_ = c.Set("n", int64(math.MaxInt64-1), time.Hour)
	value, ok := c.Get("n")
	if !ok {
		t.Fatal("miss")
	}
	f, isFloat := value.(float64)
	if !isFloat {
		t.Fatalf("Get returned %T", value)
	}
	if int64(f) == math.MaxInt64-1 {
		t.Error("precision survived; update the documented limit")
	}
}

func TestLimitSetDropsUnexportedFields(t *testing.T) {
	cfg, _ := miniConfig(t)
	c := memoryCache(t, cfg)
	if err := c.Set("k", withPrivate{ID: 1, id: 2}, time.Hour); err != nil {
		t.Fatal(err)
	}
	value, _ := c.Get("k")
	if m, ok := value.(map[string]any); !ok || len(m) != 1 {
		t.Errorf("Get returned %#v", value)
	}
}

// A name means the same function to every process on a prefix: distinct KeyPrefix
// values are what separate services.
func TestLimitNameIsSharedAcrossCaches(t *testing.T) {
	cfg, mr := miniConfig(t)
	serviceA := memoryCache(t, cfg)
	serviceB := memoryCache(t, obcache.NewRedisConfigWithClient(redisClient(t, mr.Addr())))
	a := obcache.Wrap(serviceA, func(int) string { return "from-a" }, opts("user")...)
	b := obcache.Wrap(serviceB, func(int) string { return "from-b" }, opts("user")...)

	_ = a(1)
	if got := b(1); got != "from-a" {
		t.Errorf("b = %q: binding reached across Caches; update the documented limit", got)
	}

	separated := obcache.NewRedisConfigWithClient(redisClient(t, mr.Addr()))
	separated.Redis.KeyPrefix = "service-c:"
	c := obcache.Wrap(memoryCache(t, separated), func(int) string { return "from-c" }, opts("user")...)
	if got := c(1); got != "from-c" {
		t.Errorf("a distinct prefix did not separate services: %q", got)
	}
}

func TestLimitFieldChangeUnderAKeptName(t *testing.T) {
	type before struct {
		ID   int
		Gone string
	}
	type after struct {
		ID    int
		Added string
	}
	cfg, mr := miniConfig(t)
	old := obcache.Wrap(memoryCache(t, cfg), func(int) (before, error) { return before{ID: 1, Gone: "x"}, nil }, opts("v")...)
	_, _ = old(1)
	if len(mr.Keys()) != 1 {
		t.Fatalf("keys = %v", mr.Keys())
	}

	var calls atomic.Int32
	w := obcache.Wrap(memoryCache(t, cfg), func(int) (after, error) {
		calls.Add(1)
		return after{ID: 2, Added: "y"}, nil
	}, opts("v")...)
	if got, _ := w(1); got.ID != 2 || calls.Load() != 1 {
		t.Errorf("a removed field read as a hit: %+v, calls=%d", got, calls.Load())
	}
}

// The opposite direction has no signal: JSON has no entry for the new field.
func TestLimitAddedFieldReadsAsZero(t *testing.T) {
	type before struct{ ID int }
	type after struct {
		ID    int
		Added string
	}
	cfg, _ := miniConfig(t)
	w1 := obcache.Wrap(memoryCache(t, cfg), func(int) (before, error) { return before{ID: 1}, nil }, opts("v")...)
	_, _ = w1(1)

	var calls atomic.Int32
	w2 := obcache.Wrap(memoryCache(t, cfg), func(int) (after, error) {
		calls.Add(1)
		return after{ID: 2, Added: "y"}, nil
	}, opts("v")...)
	if got, _ := w2(1); got.ID != 1 || got.Added != "" || calls.Load() != 0 {
		t.Errorf("got %+v, calls=%d: update the documented limit", got, calls.Load())
	}
}

func TestLimitContextIsNotInTheKey(t *testing.T) {
	type userKey struct{}
	c := memoryCache(t, obcache.NewDefaultConfig())
	w := obcache.Wrap(c, func(ctx context.Context) string { return ctx.Value(userKey{}).(string) })
	alice := context.WithValue(context.Background(), userKey{}, "alice")
	mallory := context.WithValue(context.Background(), userKey{}, "mallory")
	_ = w(alice)
	if got := w(mallory); got != "alice" {
		t.Errorf("got %q: ctx reached the key; update the documented limit", got)
	}
}

func TestLimitStampedeProtectionIsPerProcess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		fn := func(int) string {
			calls.Add(1)
			time.Sleep(10 * time.Millisecond)
			return "v"
		}
		// Two Caches stand in for two processes on one Redis.
		a := obcache.Wrap(bubbleCache(t), fn, opts("n")...)
		b := obcache.Wrap(bubbleCache(t), fn, opts("n")...)
		var wg sync.WaitGroup
		wg.Go(func() { _ = a(1) })
		wg.Go(func() { _ = b(1) })
		wg.Wait()
		if n := calls.Load(); n != 2 {
			t.Errorf("calls = %d, want one per process", n)
		}
	})
}

func TestLimitWaiterWithoutErrorReturnKeepsWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := bubbleCache(t)
		w := obcache.Wrap(c, func(context.Context, int) string {
			time.Sleep(time.Second)
			return "v"
		}, opts("n")...)

		var wg sync.WaitGroup
		wg.Go(func() { _ = w(context.Background(), 1) })
		time.Sleep(time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		if got := w(ctx, 1); got != "v" {
			t.Errorf("waiter = %q", got)
		}
		wg.Wait()
	})
}

func TestLimitOnEvictDoesNotFireOnRedis(t *testing.T) {
	var evicts atomic.Int32
	hooks := obcache.NewHooks()
	hooks.AddOnEvict(func(context.Context, string, any, obcache.EvictReason) { evicts.Add(1) })
	cfg, mr := miniConfig(t)
	c := memoryCache(t, cfg.WithHooks(hooks))
	_ = c.Set("k", "v", time.Minute)
	mr.FastForward(2 * time.Minute)
	c.Get("k")
	if n := evicts.Load(); n != 0 {
		t.Errorf("OnEvict fired %d times; update the documented limit", n)
	}
}

func TestLimitRedisOutageIsColdCache(t *testing.T) {
	mr := miniredis.RunT(t)
	c := memoryCache(t, obcache.NewRedisConfig(mr.Addr()))
	var calls atomic.Int32
	w := obcache.Wrap(c, func(id int) (int, error) {
		calls.Add(1)
		return id, nil
	}, opts("outage")...)
	_, _ = w(1)

	mr.Close()
	for range 2 {
		if got, err := w(1); got != 1 || err != nil {
			t.Fatalf("got %d, %v: an outage reached the caller; update the documented limit", got, err)
		}
	}
	if n, misses := calls.Load(), c.Stats().Misses(); n != 3 || misses != 3 {
		t.Errorf("calls=%d misses=%d: an outage is not a counted miss; update the documented limit", n, misses)
	}
}
