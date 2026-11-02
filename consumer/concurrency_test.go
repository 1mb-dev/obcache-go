package consumer

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

// bubbleCache runs inside synctest: the cleanup ticker would otherwise keep the
// bubble from ever going idle.
func bubbleCache(t *testing.T) *obcache.Cache {
	t.Helper()
	c, err := obcache.New(obcache.NewDefaultConfig().WithCleanupInterval(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestFindingB2(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := bubbleCache(t)
		var calls atomic.Int32
		w := obcache.Wrap(c, func(int) (string, error) {
			if calls.Add(1) == 1 {
				time.Sleep(10 * time.Millisecond)
				panic("first call fails")
			}
			return "ok", nil
		}, opts("panics")...)

		var wg sync.WaitGroup
		var panicked atomic.Int32
		results := make([]string, 5)
		for i := range results {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if recover() != nil {
						panicked.Add(1)
					}
				}()
				v, err := w(1)
				if err != nil {
					t.Errorf("waiter %d: %v", i, err)
				}
				results[i] = v
			}()
			time.Sleep(time.Millisecond)
		}
		wg.Wait()

		if n := panicked.Load(); n != 1 {
			t.Errorf("%d callers panicked, want the leader only", n)
		}
		zero := 0
		for _, v := range results {
			if v == "" {
				zero++
			}
		}
		if zero != 1 {
			t.Errorf("%d callers have no value, want the leader only: %q", zero, results)
		}
		if v, err := w(1); v != "ok" || err != nil {
			t.Errorf("next call = %q, %v", v, err)
		}
	})
}

func TestFindingGX1(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := bubbleCache(t)
		var calls atomic.Int32
		w := obcache.Wrap(c, func(int) (string, error) {
			if calls.Add(1) == 1 {
				time.Sleep(10 * time.Millisecond)
				runtime.Goexit()
			}
			return "ok", nil
		}, opts("goexit")...)

		var wg sync.WaitGroup
		wg.Go(func() { _, _ = w(1) })
		time.Sleep(time.Millisecond)
		if v, err := w(1); v != "ok" || err != nil {
			t.Errorf("waiter = %q, %v", v, err)
		}
		wg.Wait()
	})
}

// Callers behind a function that always panics each get a panic, never a zero
// value, and the calls stay bounded by the number of callers.
func TestAlwaysPanickingFunction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := bubbleCache(t)
		var calls, panicked atomic.Int32
		w := obcache.Wrap(c, func(int) (string, error) {
			calls.Add(1)
			time.Sleep(time.Millisecond)
			panic("always")
		}, opts("always")...)

		const callers = 8
		var wg sync.WaitGroup
		for range callers {
			wg.Go(func() {
				defer func() {
					if recover() != nil {
						panicked.Add(1)
					}
				}()
				_, _ = w(1)
				t.Error("a caller returned without a panic")
			})
		}
		wg.Wait()
		if panicked.Load() != callers {
			t.Errorf("panicked = %d, want %d", panicked.Load(), callers)
		}
		if n := calls.Load(); n > callers {
			t.Errorf("underlying calls = %d, want at most %d", n, callers)
		}
	})
}

func TestFindingB3(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := bubbleCache(t)
		var calls atomic.Int32
		w := obcache.Wrap(c, func(int) (string, error) {
			calls.Add(1)
			time.Sleep(30 * time.Millisecond)
			return "v", nil
		}, opts("hot")...)

		var wg sync.WaitGroup
		for range 50 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = w(1)
			}()
		}
		wg.Wait()
		_, _ = w(1)
		if n := calls.Load(); n != 1 {
			t.Errorf("underlying calls = %d, want 1", n)
		}
	})
}

func TestFindingC1(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := bubbleCache(t)
		w := obcache.Wrap(c, func(ctx context.Context, _ int) (string, error) {
			select {
			case <-time.After(200 * time.Millisecond):
				return "v", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}, opts("cancel")...)

		leader, cancel := context.WithCancel(context.Background())
		defer cancel()
		var wg sync.WaitGroup
		wg.Go(func() { _, _ = w(leader, 1) })
		time.Sleep(10 * time.Millisecond)
		var got string
		var err error
		wg.Go(func() { got, err = w(context.Background(), 1) })
		time.Sleep(10 * time.Millisecond)
		cancel()
		wg.Wait()
		if got != "v" || err != nil {
			t.Errorf("live waiter = %q, %v", got, err)
		}
	})
}

func TestFindingC2(t *testing.T) {
	c := memoryCache(t, obcache.NewDefaultConfig())
	o := append(opts("poison"), obcache.WithErrorCaching())
	w := obcache.Wrap(c, func(ctx context.Context, _ int) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return "ok", nil
	}, o...)

	dead, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = w(dead, 1)
	if v, err := w(context.Background(), 1); v != "ok" || err != nil {
		t.Errorf("live caller = %q, %v", v, err)
	}
}

// A leader whose deadline passes during a slow call still caches the result, or a
// slow origin is never cached at all.
func TestSlowOriginStillCached(t *testing.T) {
	cfg, mr := miniConfig(t)
	c := memoryCache(t, cfg)
	var calls atomic.Int32
	w := obcache.Wrap(c, func(context.Context, int) (string, error) {
		calls.Add(1)
		time.Sleep(100 * time.Millisecond)
		return "v", nil
	}, opts("slow")...)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if v, err := w(ctx, 1); v != "v" || err != nil {
		t.Fatalf("slow call = %q, %v", v, err)
	}
	if v, err := w(context.Background(), 1); v != "v" || err != nil {
		t.Fatalf("second call = %q, %v", v, err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("underlying calls = %d, want 1", n)
	}
	if n := len(mr.Keys()); n != 1 {
		t.Errorf("stored keys = %d, want 1", n)
	}
}

type codedError struct{}

func (*codedError) Error() string { return "coded" }

// ctx.Err() does not fit a *codedError result, so this waiter cannot leave early.
func TestWaiterWithConcreteErrorTypeKeepsWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := bubbleCache(t)
		w := obcache.Wrap(c, func(context.Context, int) (string, *codedError) {
			time.Sleep(time.Second)
			return "v", nil
		}, opts("coded")...)

		var wg sync.WaitGroup
		wg.Go(func() { _, _ = w(context.Background(), 1) })
		time.Sleep(time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		if v, err := w(ctx, 1); v != "v" || err != nil {
			t.Errorf("waiter = %q, %v", v, err)
		}
		wg.Wait()
	})
}

func TestFindingCTX1(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := bubbleCache(t)
		w := obcache.Wrap(c, func(context.Context, int) (string, error) {
			time.Sleep(time.Second)
			return "v", nil
		}, opts("wait")...)

		var wg sync.WaitGroup
		wg.Go(func() { _, _ = w(context.Background(), 1) })
		time.Sleep(time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		if _, err := w(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("waiter err = %v", err)
		}
		wg.Wait()
	})
}

// WithKeyFunc can give two functions the same key. Sharing a flight must then fall
// back to a call of its own, never hand over the other function's results.
func TestSharedKeyAcrossFunctionTypes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := bubbleCache(t)
		same := obcache.WithKeyFunc(func(args []any) string { return "same" })
		users := obcache.Wrap(c, func(int) (*User, error) {
			time.Sleep(10 * time.Millisecond)
			return &User{ID: 1}, nil
		}, same)
		names := obcache.Wrap(c, func(int) (*Inner, error) { return &Inner{Tags: []string{"x"}}, nil }, same)

		var wg sync.WaitGroup
		wg.Go(func() { _, _ = users(1) })
		time.Sleep(time.Millisecond)
		if got, err := names(1); err != nil || got == nil || len(got.Tags) != 1 {
			t.Errorf("names = %+v, %v", got, err)
		}
		wg.Wait()
	})
}
