package consumer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/compression"
	"github.com/1mb-dev/obcache-go/v2/pkg/metrics"
	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
	"github.com/prometheus/client_golang/prometheus"
)

// One test per advertised feature. A README Features bullet with no test here
// fails readme_test.go.

func TestFeatureFunctionWrapping(t *testing.T) {
	c := memoryCache(t, obcache.NewDefaultConfig())
	var calls atomic.Int32
	w := obcache.Wrap(c, func(id int) (string, error) {
		calls.Add(1)
		return fmt.Sprintf("result-%d", id), nil
	}, opts("wrap")...)
	a, _ := w(123)
	b, _ := w(123)
	other, _ := w(124)
	if a != "result-123" || b != a || other != "result-124" || calls.Load() != 2 {
		t.Errorf("a=%q b=%q other=%q calls=%d", a, b, other, calls.Load())
	}
}

func TestFeatureTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := bubbleCache(t)
		_ = c.Set("short", 1, 10*time.Millisecond)
		_ = c.Set("long", 2, time.Hour)
		time.Sleep(20 * time.Millisecond)
		if _, ok := c.Get("short"); ok {
			t.Error("expired entry returned")
		}
		if _, ok := c.Get("long"); !ok {
			t.Error("live entry missing")
		}
	})
}

func TestFeatureEviction(t *testing.T) {
	cases := []struct {
		strategy string
		keep     string
		lose     string
	}{
		{"lru", "b", "a"},  // a read first, b read last
		{"lfu", "a", "b"},  // a read twice, b once
		{"fifo", "b", "a"}, // a written first
	}
	for _, tc := range cases {
		t.Run(tc.strategy, func(t *testing.T) {
			cfg := obcache.NewDefaultConfig().WithMaxEntries(2)
			switch tc.strategy {
			case "lru":
				cfg = cfg.WithEvictionType("lru")
			case "lfu":
				cfg = cfg.WithEvictionType("lfu")
			case "fifo":
				cfg = cfg.WithEvictionType("fifo")
			}
			c := memoryCache(t, cfg)
			_ = c.Set("a", 1, time.Hour)
			_ = c.Set("b", 2, time.Hour)
			c.Get("a")
			c.Get("a")
			c.Get("b")
			_ = c.Set("c", 3, time.Hour)
			if !c.Has(tc.keep) || c.Has(tc.lose) {
				t.Errorf("keys = %v, want %s kept and %s evicted", c.Keys(), tc.keep, tc.lose)
			}
		})
	}
}

func TestFeatureThreadSafety(t *testing.T) {
	c := memoryCache(t, obcache.NewDefaultConfig().WithMaxEntries(50))
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 500 {
				key := fmt.Sprintf("k%d", (g*i)%100)
				_ = c.Set(key, i, time.Hour)
				c.Get(key)
				if i%10 == 0 {
					_ = c.Delete(key)
				}
				c.Stats()
			}
		})
	}
	wg.Wait()
}

func TestFeatureStampedeProtection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := bubbleCache(t)
		var calls atomic.Int32
		w := obcache.Wrap(c, func(int) (string, error) {
			calls.Add(1)
			time.Sleep(50 * time.Millisecond)
			return "v", nil
		}, opts("stampede")...)
		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() { _, _ = w(1) })
		}
		wg.Wait()
		if n := calls.Load(); n != 1 {
			t.Errorf("100 concurrent callers ran the function %d times", n)
		}
	})
}

func TestFeatureRedisBackend(t *testing.T) {
	cfg, mr := miniConfig(t)
	cfg.Redis.KeyPrefix = "myapp:"
	c := memoryCache(t, cfg)
	if err := c.Set("k", "v", time.Minute); err != nil {
		t.Fatal(err)
	}
	if v, ok := c.Get("k"); !ok || v != "v" {
		t.Fatalf("Get = %v, %v", v, ok)
	}
	if ttl := mr.TTL("myapp:k"); ttl <= 0 || ttl > time.Minute {
		t.Errorf("stored TTL = %v", ttl)
	}
	mr.FastForward(2 * time.Minute)
	if _, ok := c.Get("k"); ok {
		t.Error("entry outlived its TTL")
	}
	_ = c.Set("gone", 1, time.Minute)
	_ = c.Delete("gone")
	if mr.Exists("myapp:gone") {
		t.Error("Delete left the key in Redis")
	}
}

func TestFeatureCompression(t *testing.T) {
	suppress(t, "Z1")
	for _, algo := range []compression.CompressorType{compression.CompressorGzip, compression.CompressorDeflate} {
		t.Run(string(algo), func(t *testing.T) {
			cfg, mr := miniConfig(t)
			c := memoryCache(t, cfg.WithCompression(readmeCompression(algo)))
			value := strings.Repeat("compressible ", 5000)
			if err := c.Set("k", value, time.Hour); err != nil {
				t.Fatal(err)
			}
			raw, err := mr.Get(mr.Keys()[0])
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) >= len(value)/2 {
				t.Errorf("stored %d bytes for a %d byte compressible value", len(raw), len(value))
			}
			if got, ok := c.Get("k"); !ok || got != value {
				t.Errorf("Get: ok=%v, value intact=%v", ok, got == value)
			}
		})
	}
}

func TestFeaturePrometheusMetrics(t *testing.T) {
	suppress(t, "P1")
	reg := prometheus.NewRegistry()
	exporter, err := metrics.NewPrometheusExporter(metrics.NewDefaultConfig(), &metrics.PrometheusConfig{Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	c := memoryCache(t, obcache.NewDefaultConfig().WithMetrics(&obcache.MetricsConfig{
		Enabled:           true,
		Exporter:          exporter,
		CacheName:         "feature",
		ReportingInterval: time.Millisecond,
	}))
	_ = c.Set("k", "v", time.Hour)
	for range 10 {
		c.Get("k")
	}
	time.Sleep(20 * time.Millisecond)
	if got := counter(t, reg, "obcache_hits_total"); got != 10 {
		t.Errorf("obcache_hits_total = %v after 10 hits", got)
	}
}

func TestFeatureStatistics(t *testing.T) {
	c := memoryCache(t, obcache.NewDefaultConfig())
	_ = c.Set("k", "v", time.Hour)
	c.Get("k")
	c.Get("k")
	c.Get("k")
	c.Get("absent")
	s := c.Stats()
	if s.Hits() != 3 || s.Misses() != 1 || s.HitRate() != 75 || s.KeyCount() != 1 {
		t.Errorf("hits=%d misses=%d rate=%v keys=%d", s.Hits(), s.Misses(), s.HitRate(), s.KeyCount())
	}
}

func TestFeatureHooks(t *testing.T) {
	type marker struct{}
	var hits, misses, evicts, invalidations atomic.Int32
	var sawCtx atomic.Bool
	hooks := obcache.NewHooks()
	hooks.AddOnHit(func(ctx context.Context, _ string, _ any) {
		hits.Add(1)
		sawCtx.Store(ctx.Value(marker{}) != nil)
	})
	hooks.AddOnMiss(func(context.Context, string) { misses.Add(1) })
	hooks.AddOnEvict(func(context.Context, string, any, obcache.EvictReason) { evicts.Add(1) })
	hooks.AddOnInvalidate(func(context.Context, string) { invalidations.Add(1) })

	c := memoryCache(t, obcache.NewDefaultConfig().WithMaxEntries(1).WithHooks(hooks))
	_ = c.Set("a", 1, time.Hour)
	c.GetContext(context.WithValue(context.Background(), marker{}, 1), "a")
	c.Get("absent")
	_ = c.Set("b", 2, time.Hour)
	_ = c.Delete("b")
	if hits.Load() != 1 || misses.Load() != 1 || evicts.Load() != 1 || invalidations.Load() != 1 {
		t.Errorf("hits=%d misses=%d evicts=%d invalidations=%d",
			hits.Load(), misses.Load(), evicts.Load(), invalidations.Load())
	}
	if !sawCtx.Load() {
		t.Error("hook did not receive the caller's context")
	}
}
