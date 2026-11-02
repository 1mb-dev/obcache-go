package singleflight

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestDoReturnsValueToLeader(t *testing.T) {
	var g Group[string, int]
	v, leader, err := g.Do(context.Background(), "k", func() int { return 7 })
	if v != 7 || !leader || err != nil {
		t.Errorf("Do = %d, %v, %v", v, leader, err)
	}
}

func TestDoSharesOneCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var g Group[string, int]
		var calls, leaders atomic.Int32
		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				v, leader, err := g.Do(context.Background(), "k", func() int {
					calls.Add(1)
					time.Sleep(10 * time.Millisecond)
					return 7
				})
				if v != 7 || err != nil {
					t.Errorf("Do = %d, %v", v, err)
				}
				if leader {
					leaders.Add(1)
				}
			})
		}
		wg.Wait()
		if calls.Load() != 1 || leaders.Load() != 1 {
			t.Errorf("calls = %d, leaders = %d", calls.Load(), leaders.Load())
		}
	})
}

func TestDoKeysAreIndependent(t *testing.T) {
	var g Group[string, string]
	a, _, _ := g.Do(context.Background(), "a", func() string { return "a" })
	b, _, _ := g.Do(context.Background(), "b", func() string { return "b" })
	if a != "a" || b != "b" {
		t.Errorf("a = %q, b = %q", a, b)
	}
}

func TestWaiterStopsOnContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var g Group[string, int]
		var wg sync.WaitGroup
		wg.Go(func() {
			_, _, _ = g.Do(context.Background(), "k", func() int {
				time.Sleep(time.Second)
				return 7
			})
		})
		time.Sleep(time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		if _, _, err := g.Do(ctx, "k", func() int { return 0 }); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("waiter err = %v", err)
		}
		wg.Wait()
	})
}

func TestLeaderPanicReleasesWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var g Group[string, int]
		var wg sync.WaitGroup
		wg.Go(func() {
			defer func() {
				if recover() == nil {
					t.Error("leader's panic did not reach the leader")
				}
			}()
			_, _, _ = g.Do(context.Background(), "k", func() int {
				time.Sleep(10 * time.Millisecond)
				panic("boom")
			})
		})
		time.Sleep(time.Millisecond)

		if _, _, err := g.Do(context.Background(), "k", func() int { return 0 }); !errors.Is(err, ErrAborted) {
			t.Errorf("waiter err = %v", err)
		}
		wg.Wait()
		if v, leader, err := g.Do(context.Background(), "k", func() int { return 7 }); v != 7 || !leader || err != nil {
			t.Errorf("next Do = %d, %v, %v", v, leader, err)
		}
	})
}

func TestLeaderGoexitReleasesWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var g Group[string, int]
		var wg sync.WaitGroup
		wg.Go(func() {
			_, _, _ = g.Do(context.Background(), "k", func() int {
				time.Sleep(10 * time.Millisecond)
				runtime.Goexit()
				return 0
			})
		})
		time.Sleep(time.Millisecond)

		if _, _, err := g.Do(context.Background(), "k", func() int { return 0 }); !errors.Is(err, ErrAborted) {
			t.Errorf("waiter err = %v", err)
		}
		wg.Wait()
	})
}
