// Package singleflight suppresses duplicate concurrent calls per key.
package singleflight

import (
	"context"
	"errors"
	"sync"
)

// ErrAborted reports that the leader panicked or called runtime.Goexit, so no
// result exists. Callers retry; there is nothing to share.
var ErrAborted = errors.New("singleflight: leader aborted")

// Group suppresses duplicate concurrent calls per key. The zero value is ready.
type Group[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]*call[V]
}

type call[V any] struct {
	done    chan struct{}
	val     V
	aborted bool
}

// Do runs fn once per key among concurrent callers. The leader runs fn on its
// own goroutine, so a panic in fn reaches the leader unchanged, with its stack.
// A waiter stops waiting when ctx is done.
func (g *Group[K, V]) Do(ctx context.Context, key K, fn func() V) (v V, leader bool, err error) {
	g.mu.Lock()
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		select {
		case <-c.done:
			if c.aborted {
				return v, false, ErrAborted
			}
			return c.val, false, nil
		case <-ctx.Done():
			return v, false, ctx.Err()
		}
	}
	if g.m == nil {
		g.m = make(map[K]*call[V])
	}
	c := &call[V]{done: make(chan struct{}), aborted: true}
	g.m[key] = c
	g.mu.Unlock()

	// aborted starts true and is cleared only after fn returns, so this also
	// releases waiters on panic and Goexit, which leave no return to observe.
	defer func() {
		g.mu.Lock()
		delete(g.m, key)
		g.mu.Unlock()
		close(c.done)
	}()
	c.val = fn()
	c.aborted = false
	return c.val, true, nil
}
