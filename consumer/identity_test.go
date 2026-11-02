package consumer

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

func TestFindingB1(t *testing.T) {
	c := memoryCache(t, obcache.NewDefaultConfig())
	users := obcache.Wrap(c, lookupUser)
	orders := obcache.Wrap(c, lookupOrder)
	if a, b := users(1), orders(1); a != "user" || b != "order" {
		t.Errorf("users = %q, orders = %q", a, b)
	}
	counts := obcache.Wrap(c, lookupCount)
	if n := counts(1); n != 3 {
		t.Errorf("counts = %d", n)
	}
}

func lookupUser(int) string  { return "user" }
func lookupOrder(int) string { return "order" }
func lookupCount(int) int    { return 3 }

type shard struct{ id string }

func (s shard) Get(int) string { return s.id }

func generic[T any](int) string {
	var zero T
	return fmt.Sprintf("%T", zero)
}

func factory(label string) func(int) string {
	return func(int) string { return label }
}

func TestIdentityWithoutNames(t *testing.T) {
	c := memoryCache(t, obcache.NewDefaultConfig())

	t.Run("closures", func(t *testing.T) {
		if a, b := obcache.Wrap(c, factory("users"))(1), obcache.Wrap(c, factory("orders"))(1); a != "users" || b != "orders" {
			t.Errorf("got %q, %q", a, b)
		}
	})
	t.Run("method values", func(t *testing.T) {
		if a, b := obcache.Wrap(c, shard{"s1"}.Get)(1), obcache.Wrap(c, shard{"s2"}.Get)(1); a != "s1" || b != "s2" {
			t.Errorf("got %q, %q", a, b)
		}
	})
	t.Run("generics", func(t *testing.T) {
		if a, b := obcache.Wrap(c, generic[*int])(1), obcache.Wrap(c, generic[*string])(1); a == b {
			t.Errorf("instantiations shared a result: %q", a)
		}
	})
	t.Run("already wrapped", func(t *testing.T) {
		a := obcache.Wrap(c, obcache.Wrap(c, lookupUser))
		b := obcache.Wrap(c, obcache.Wrap(c, lookupOrder))
		if x, y := a(1), b(1); x != "user" || y != "order" {
			t.Errorf("got %q, %q", x, y)
		}
	})
	t.Run("top-level rewrap shares entries", func(t *testing.T) {
		fresh := memoryCache(t, obcache.NewDefaultConfig())
		obcache.Wrap(fresh, lookupUser)(1)
		obcache.Wrap(fresh, lookupUser)(1)
		if hits := fresh.Stats().Hits(); hits != 1 {
			t.Errorf("hits = %d, want 1", hits)
		}
	})
}

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), want) {
			t.Errorf("panic = %v, want it to mention %q", r, want)
		}
	}()
	f()
}

func TestWithNameBinding(t *testing.T) {
	t.Run("one name, one closure, from one call site", func(t *testing.T) {
		c := memoryCache(t, obcache.NewDefaultConfig())
		mustPanic(t, "already bound", func() {
			for _, tenant := range []string{"a", "b"} {
				obcache.Wrap(c, factory(tenant), obcache.WithName("loader"))
			}
		})
	})
	t.Run("one name, one method value", func(t *testing.T) {
		c := memoryCache(t, obcache.NewDefaultConfig())
		mustPanic(t, "already bound", func() {
			for _, id := range []string{"a", "b"} {
				obcache.Wrap(c, shard{id}.Get, obcache.WithName("loader"))
			}
		})
	})
	t.Run("one name, two functions", func(t *testing.T) {
		c := memoryCache(t, obcache.NewDefaultConfig())
		obcache.Wrap(c, lookupUser, obcache.WithName("n"))
		mustPanic(t, "already bound", func() { obcache.Wrap(c, lookupOrder, obcache.WithName("n")) })
	})
	t.Run("top-level function may rewrap", func(t *testing.T) {
		c := memoryCache(t, obcache.NewDefaultConfig())
		obcache.Wrap(c, lookupUser, obcache.WithName("n"))(1)
		obcache.Wrap(c, lookupUser, obcache.WithName("n"))(1)
		if hits := c.Stats().Hits(); hits != 1 {
			t.Errorf("hits = %d, want 1", hits)
		}
	})
	t.Run("a name cannot claim a derived identity", func(t *testing.T) {
		c := memoryCache(t, obcache.NewDefaultConfig())
		obcache.Wrap(c, lookupUser)(1)
		mustPanic(t, "already bound", func() {
			obcache.Wrap(c, lookupOrder, obcache.WithName("consumer.lookupUser"))
		})
	})
	t.Run("a name cannot forge a key", func(t *testing.T) {
		c := memoryCache(t, obcache.NewDefaultConfig())
		for _, name := range []string{"a|b", "#1"} {
			mustPanic(t, "must not contain", func() { obcache.Wrap(c, lookupUser, obcache.WithName(name)) })
		}
	})
	t.Run("WithName and WithKeyFunc conflict", func(t *testing.T) {
		c := memoryCache(t, obcache.NewDefaultConfig())
		mustPanic(t, "mutually exclusive", func() {
			obcache.Wrap(c, lookupUser, obcache.WithName("n"),
				obcache.WithKeyFunc(func([]any) string { return "k" }))
		})
	})
	t.Run("WithKeyFunc is the per-request remedy", func(t *testing.T) {
		c := memoryCache(t, obcache.NewDefaultConfig())
		for range 2 {
			w := obcache.Wrap(c, factory("x"), obcache.WithKeyFunc(func(args []any) string {
				return fmt.Sprint("req|", args[0])
			}))
			w(1)
		}
		if hits := c.Stats().Hits(); hits != 1 {
			t.Errorf("hits = %d, want 1", hits)
		}
	})
	t.Run("WithoutCache binds a given name", func(t *testing.T) {
		c := memoryCache(t, obcache.NewDefaultConfig())
		obcache.Wrap(c, lookupUser, obcache.WithName("n"), obcache.WithoutCache())
		if key := c.Key("n", 1); key == "" {
			t.Error("empty key")
		}
	})
}

type accountID int64

func TestFindingKEY1(t *testing.T) {
	for _, b := range backends() {
		if b.name == redisGzip {
			continue
		}
		t.Run(b.name, func(t *testing.T) {
			c := b.new(t)
			var calls atomic.Int32
			byID := obcache.Wrap(c, func(int64) (string, error) { calls.Add(1); return "v", nil }, opts("by-id")...)
			byAccount := obcache.Wrap(c, func(context.Context, accountID) (string, error) { calls.Add(1); return "v", nil }, opts("by-account")...)

			_, _ = byID(7)
			_, _ = byID(7)
			if n := calls.Swap(0); n != 1 {
				t.Fatalf("calls before Delete = %d", n)
			}
			if err := c.Delete(c.Key("by-id", int64(7))); err != nil {
				t.Fatal(err)
			}
			_, _ = byID(7)
			if n := calls.Swap(0); n != 1 {
				t.Errorf("calls after Delete = %d, want a recompute", n)
			}

			_, _ = byAccount(context.Background(), 9)
			_, _ = byAccount(context.Background(), 9)
			calls.Store(0)
			if err := c.Delete(c.Key("by-account", accountID(9))); err != nil {
				t.Fatal(err)
			}
			_, _ = byAccount(context.Background(), 9)
			if n := calls.Load(); n != 1 {
				t.Errorf("calls after Delete = %d, want a recompute", n)
			}
		})
	}
}

func TestKeyRefusesWhatWouldNotMatch(t *testing.T) {
	c := memoryCache(t, obcache.NewDefaultConfig())
	obcache.Wrap(c, func(int64) string { return "v" }, obcache.WithName("n"))
	mustPanic(t, "no Wrap named", func() { c.Key("typo", int64(1)) })
	mustPanic(t, "got 2 args", func() { c.Key("n", int64(1), int64(2)) })
	mustPanic(t, "takes int64", func() { c.Key("n", 1) })
}

// A value with no key encoding can come from a request, so it runs uncached rather
// than panicking, including when DefaultKeyFunc is passed as the key function.
func TestArgumentWithNoEncodingRunsUncached(t *testing.T) {
	farFuture := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	run := func(t *testing.T, c *obcache.Cache, options ...obcache.WrapOption) {
		t.Helper()
		var calls atomic.Int32
		w := obcache.Wrap(c, func(time.Time) string { calls.Add(1); return "v" }, options...)
		for range 2 {
			if v := w(farFuture); v != "v" {
				t.Errorf("v = %q", v)
			}
		}
		if n := calls.Load(); n != 2 {
			t.Errorf("underlying calls = %d, want 2", n)
		}
	}
	t.Run("default key", func(t *testing.T) {
		run(t, memoryCache(t, obcache.NewDefaultConfig()))
	})
	t.Run("WithKeyFunc(DefaultKeyFunc)", func(t *testing.T) {
		run(t, memoryCache(t, obcache.NewDefaultConfig()), obcache.WithKeyFunc(obcache.DefaultKeyFunc))
	})
	t.Run("WithKeyGenFunc(DefaultKeyFunc)", func(t *testing.T) {
		run(t, memoryCache(t, obcache.NewDefaultConfig().WithKeyGenFunc(obcache.DefaultKeyFunc)))
	})
	t.Run("a key function's own panic is not hidden", func(t *testing.T) {
		c := memoryCache(t, obcache.NewDefaultConfig())
		w := obcache.Wrap(c, lookupUser, obcache.WithKeyFunc(func([]any) string { panic("bug in key func") }))
		mustPanic(t, "bug in key func", func() { w(1) })
	})
}

type hiddenState struct{ n int }

func TestRefusedWrapLeavesNoName(t *testing.T) {
	c := memoryCache(t, obcache.NewDefaultConfig())
	mustPanic(t, "unexported field", func() {
		obcache.Wrap(c, func(hiddenState) string { return "" }, obcache.WithName("report"))
	})
	mustPanic(t, "no Wrap named", func() { c.Key("report", hiddenState{n: 1}) })
	obcache.Wrap(c, func(int) string { return "" }, obcache.WithName("report"))
}
