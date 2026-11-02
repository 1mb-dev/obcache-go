package consumer

import (
	"context"
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
	suppress(t, "B2")
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

func TestFindingB3(t *testing.T) {
	suppress(t, "B3")
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
	suppress(t, "C1")
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
	suppress(t, "C2")
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
