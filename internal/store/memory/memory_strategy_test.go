package memory

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/1mb-dev/obcache-go/v2/internal/entry"
	"github.com/1mb-dev/obcache-go/v2/internal/eviction"
)

func TestExpiredReadKeepsAFreshEntry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, err := NewWithStrategy(eviction.Config{Type: eviction.LRU, Capacity: 10})
		if err != nil {
			t.Fatal(err)
		}
		var evicted int
		s.SetCleanupCallback(func(string, any) { evicted++ })
		ctx := context.Background()

		_ = s.Set(ctx, "k", entry.New("old", time.Millisecond))
		time.Sleep(2 * time.Millisecond)
		if _, ok := s.Get(ctx, "k"); ok {
			t.Fatal("expired entry returned")
		}
		_ = s.Set(ctx, "k", entry.New("new", time.Hour))
		synctest.Wait()

		if got, ok := s.Get(ctx, "k"); !ok || got.Value != "new" {
			t.Errorf("fresh entry gone: %v, %v", got, ok)
		}
		if evicted != 0 {
			t.Errorf("evictions = %d for an entry that was replaced, not expired", evicted)
		}
	})
}
