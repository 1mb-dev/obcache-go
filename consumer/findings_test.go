package consumer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/compression"
	"github.com/1mb-dev/obcache-go/v2/pkg/metrics"
	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/prometheus/client_golang/prometheus"
)

const fallback = "fallback"

func TestFindingB4(t *testing.T) {
	suppress(t, "B4")
	for _, b := range backends() {
		if !b.serial || b.name == redisGzip {
			continue
		}
		t.Run(b.name, func(t *testing.T) {
			c := b.new(t)
			var calls atomic.Int32
			small := obcache.Wrap(c, func(int) (int, error) { calls.Add(1); return 7, nil }, opts("small")...)
			_, _ = small(1)
			if hit, err := small(1); hit != 7 || err != nil {
				t.Errorf("int hit = %d, %v", hit, err)
			}
			structured := obcache.Wrap(c, func(int) (User, error) { calls.Add(1); return User{ID: 1}, nil }, opts("struct")...)
			_, _ = structured(1)
			if hit, err := structured(1); hit.ID != 1 || err != nil {
				t.Errorf("struct hit = %+v, %v", hit, err)
			}
			if n := calls.Load(); n != 2 {
				t.Errorf("underlying calls = %d, want one per function", n)
			}
		})
	}
}

func TestFindingB5(t *testing.T) {
	c := memoryCache(t, obcache.NewDefaultConfig())
	a := obcache.Wrap(c, func(int) (any, error) { return nil, nil }, opts("any")...)
	s := obcache.Wrap(c, func(int) (fmt.Stringer, error) { return nil, nil }, opts("stringer")...)
	e := obcache.Wrap(c, func(int) (error, error) { return nil, nil }, opts("error")...)
	for range 2 {
		if v, err := a(1); v != nil || err != nil {
			t.Errorf("any = %v, %v", v, err)
		}
		if v, err := s(1); v != nil || err != nil {
			t.Errorf("Stringer = %v, %v", v, err)
		}
		if v, err := e(1); v != nil || err != nil {
			t.Errorf("error = %v, %v", v, err)
		}
	}
}

func TestFindingB6(t *testing.T) {
	suppress(t, "B6")
	for _, b := range backends() {
		if !b.serial {
			continue
		}
		t.Run(b.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "WithErrorCaching") {
					t.Errorf("panic = %v, want a Wrap-time refusal naming WithErrorCaching", r)
				}
			}()
			o := append(opts("errors"), obcache.WithErrorCaching())
			obcache.Wrap(b.new(t), func(int) (string, error) { return "", errors.New("x") }, o...)
		})
	}
}

func TestFindingB7(t *testing.T) {
	c, err := obcache.New(obcache.NewDefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestFindingB8(t *testing.T) {
	if _, err := obcache.New(obcache.NewDefaultConfig().WithEvictionType("bogus")); err == nil {
		t.Error("New accepted an unknown eviction type")
	}
}

func TestFindingSH1(t *testing.T) {
	c := memoryCache(t, obcache.NewDefaultConfig())

	t.Run("variadic", func(t *testing.T) {
		w := obcache.Wrap(c, func(xs ...int) (int, error) { return len(xs), nil }, opts("variadic")...)
		if v, err := w(1, 2); v != 2 || err != nil {
			t.Errorf("w(1, 2) = %d, %v", v, err)
		}
		if v, err := w(1, 2); v != 2 || err != nil {
			t.Errorf("hit = %d, %v", v, err)
		}
		if v, _ := w(1, 2, 3); v != 3 {
			t.Errorf("w(1, 2, 3) = %d", v)
		}
	})
	t.Run("error-only-nil-cached", func(t *testing.T) {
		var calls atomic.Int32
		w := obcache.Wrap(c, func(int) error { calls.Add(1); return nil }, opts("nil")...)
		if err := w(1); err != nil {
			t.Error(err)
		}
		if err := w(1); err != nil {
			t.Error(err)
		}
		if n := calls.Load(); n != 1 {
			t.Errorf("calls = %d, want 1", n)
		}
	})
	t.Run("error-only-failure-not-cached", func(t *testing.T) {
		var calls atomic.Int32
		w := obcache.Wrap(c, func(int) error { calls.Add(1); return errors.New("x") }, opts("fail")...)
		_ = w(1)
		_ = w(1)
		if n := calls.Load(); n != 2 {
			t.Errorf("calls = %d, want 2", n)
		}
	})
}

func TestFindingH1(t *testing.T) {
	t.Run("OnHit-calls-Delete", func(t *testing.T) {
		hooks := obcache.NewHooks()
		var c *obcache.Cache
		hooks.AddOnHit(func(_ context.Context, key string, _ any) { _ = c.Delete(key) })
		c = memoryCache(t, obcache.NewDefaultConfig().WithHooks(hooks))
		_ = c.Set("k", "v", time.Hour)
		bounded(t, 2*time.Second, "Get", func() { c.Get("k") })
	})
	t.Run("OnHit-panic-leaves-cache-usable", func(t *testing.T) {
		hooks := obcache.NewHooks()
		hooks.AddOnHit(func(context.Context, string, any) { panic("hook") })
		c := memoryCache(t, obcache.NewDefaultConfig().WithHooks(hooks))
		_ = c.Set("k", "v", time.Hour)
		func() {
			defer func() { _ = recover() }()
			c.Get("k")
		}()
		bounded(t, 2*time.Second, "Set", func() { _ = c.Set("k2", "v", time.Hour) })
	})
	t.Run("OnEvict-calls-Len-on-capacity", func(t *testing.T) {
		hooks := obcache.NewHooks()
		var c *obcache.Cache
		hooks.AddOnEvict(func(context.Context, string, any, obcache.EvictReason) { _ = c.Len() })
		c = memoryCache(t, obcache.NewDefaultConfig().WithMaxEntries(1).WithHooks(hooks))
		bounded(t, 2*time.Second, "Set", func() {
			_ = c.Set("a", 1, time.Hour)
			_ = c.Set("b", 2, time.Hour)
		})
	})
	t.Run("OnEvict-calls-Len-on-Cleanup", func(t *testing.T) {
		hooks := obcache.NewHooks()
		var c *obcache.Cache
		hooks.AddOnEvict(func(context.Context, string, any, obcache.EvictReason) { _ = c.Len() })
		c = memoryCache(t, obcache.NewDefaultConfig().WithCleanupInterval(0).WithHooks(hooks))
		_ = c.Set("a", 1, time.Millisecond)
		time.Sleep(5 * time.Millisecond)
		bounded(t, 2*time.Second, "Cleanup", func() { c.Cleanup() })
	})
}

func TestFindingR1(t *testing.T) {
	cfg, mr := miniConfig(t)
	c := memoryCache(t, cfg)
	for i := range 100 {
		_ = c.Set(fmt.Sprintf("k%d", i), i, time.Hour)
	}
	before := mr.CommandCount()
	_ = c.Set("one-more", 1, time.Hour)
	if n := mr.CommandCount() - before; n != 1 {
		t.Errorf("one Set issued %d commands with 100 keys present", n)
	}
}

func TestFindingR2(t *testing.T) {
	mr := miniredis.RunT(t)
	g := newGate(t, mr.Addr())
	c := memoryCache(t, obcache.NewRedisConfig(g.addr))
	withCtx := obcache.Wrap(c, func(context.Context, int) (string, error) { return fallback, nil }, opts("ctx")...)
	noCtx := obcache.Wrap(c, func(int) (string, error) { return fallback, nil }, opts("noctx")...)
	if v, err := noCtx(0); v != fallback || err != nil {
		t.Fatalf("healthy call = %q, %v", v, err)
	}
	g.hung.Store(true)

	bounded(t, 30*time.Second, "calls against a hung server", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		if v, err := withCtx(ctx, 1); v != fallback || err != nil {
			t.Errorf("ctx call = %q, %v", v, err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("ctx call took %v with a 50ms deadline", d)
		}
		start = time.Now()
		if v, err := noCtx(1); v != fallback || err != nil {
			t.Errorf("ctx-less call = %q, %v", v, err)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("ctx-less call took %v", d)
		}
	})
}

func TestFindingR3(t *testing.T) {
	cfg, mr := miniConfig(t)
	c := memoryCache(t, cfg)
	if err := c.Set("k", "v", time.Hour); err != nil {
		t.Fatal(err)
	}
	stored := mr.Keys()[0]

	// A second pod's Delete lands in the window between the read and any
	// write-back, which makes the resurrection deterministic.
	var armed, wrote atomic.Bool
	mr.Server().SetPreHook(func(_ *server.Peer, cmd string, _ ...string) bool {
		if armed.Load() && strings.HasPrefix(cmd, "SET") {
			wrote.Store(true)
			mr.Del(stored)
		}
		return false
	})
	armed.Store(true)
	before := mr.CommandCount()
	if _, ok := c.Get("k"); !ok {
		t.Fatal("Get missed")
	}
	if n := mr.CommandCount() - before; n != 1 {
		t.Errorf("a hit issued %d commands, want 1", n)
	}
	if wrote.Load() && mr.Exists(stored) {
		t.Error("a hit restored a key another pod deleted")
	}
}

func TestFindingF1(t *testing.T) {
	suppress(t, "F1")
	cfg, mr := miniConfig(t)
	pod := memoryCache(t, cfg)
	other := memoryCache(t, obcache.NewRedisConfigWithClient(redisClient(t, mr.Addr())))
	_ = pod.Set("k", "v", time.Hour)
	if err := pod.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := other.Get("k"); !ok {
		t.Error("Close on one instance removed shared data")
	}
}

func TestFindingF2(t *testing.T) {
	suppress(t, "F2")
	cfg, _ := miniConfig(t)
	c := memoryCache(t, cfg.WithCompression(readmeCompression(compression.CompressorGzip)))
	var calls atomic.Int32
	w := obcache.Wrap(c, func(int) (string, error) {
		calls.Add(1)
		return strings.Repeat("x", 5000), nil
	}, opts("large")...)
	for range 5 {
		_, _ = w(1)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("5 calls ran the function %d times", n)
	}
}

func TestFindingF3(t *testing.T) {
	suppress(t, "F3")
	_, err := obcache.New(obcache.NewRedisConfig("127.0.0.1:1").WithRedis(&obcache.RedisConfig{KeyPrefix: "x:"}))
	if err == nil {
		t.Fatal("New accepted a Redis config with no target")
	}
	if !strings.Contains(err.Error(), "Addr") || strings.Contains(err.Error(), "dial") {
		t.Errorf("err = %v, want a config error naming Addr, with no dial", err)
	}
}

func TestFindingF6(t *testing.T) {
	cfg, _ := miniConfig(t)
	c := memoryCache(t, cfg)
	bounded(t, 20*time.Second, "concurrent Set with Keys, Len and Stats", func() {
		done := make(chan struct{})
		var wg sync.WaitGroup
		for _, read := range []func(){func() { c.Keys() }, func() { c.Len() }, func() { c.Stats() }} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-done:
						return
					default:
						read()
					}
				}
			}()
		}
		for i := range 200 {
			_ = c.Set(fmt.Sprintf("k%d", i%10), i, time.Hour)
		}
		close(done)
		wg.Wait()
	})
}

func TestFindingP1(t *testing.T) {
	suppress(t, "P1")
	c := memoryCache(t, obcache.NewDefaultConfig())
	reg := prometheus.NewRegistry()
	exporter, err := metrics.NewPrometheusExporter(metrics.NewDefaultConfig(), &metrics.PrometheusConfig{Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	_ = c.Set("k", "v", time.Hour)
	labels := metrics.Labels{"cache_name": "p1"}
	for range 10 {
		c.Get("k")
	}
	for range 5 {
		if err := exporter.ExportStats(c.Stats(), labels); err != nil {
			t.Fatal(err)
		}
	}
	if got := counter(t, reg, "obcache_hits_total"); got != 10 {
		t.Errorf("obcache_hits_total = %v after 10 hits", got)
	}
}

func counter(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		var sum float64
		for _, m := range f.GetMetric() {
			sum += m.GetCounter().GetValue()
		}
		return sum
	}
	t.Fatalf("metric %s not found", name)
	return 0
}

func TestFindingZ1(t *testing.T) {
	suppress(t, "Z1")
	storedSize := func(t *testing.T, cfg *obcache.Config, mr *miniredis.Miniredis, value string) int {
		t.Helper()
		c := memoryCache(t, cfg)
		if err := c.Set("k", value, time.Hour); err != nil {
			t.Fatal(err)
		}
		raw, err := mr.Get(mr.Keys()[0])
		if err != nil {
			t.Fatal(err)
		}
		return len(raw)
	}
	for _, algo := range []compression.CompressorType{compression.CompressorGzip, compression.CompressorDeflate} {
		t.Run(string(algo), func(t *testing.T) {
			compressible := strings.Repeat("compressible ", 5000)
			plainCfg, plainMr := miniConfig(t)
			zipCfg, zipMr := miniConfig(t)
			plain := storedSize(t, plainCfg, plainMr, compressible)
			zipped := storedSize(t, zipCfg.WithCompression(readmeCompression(algo)), zipMr, compressible)
			if zipped >= plain/2 {
				t.Errorf("compressible value stored %d bytes, plain is %d", zipped, plain)
			}

			small := strings.Repeat("x", 900)
			plainCfg, plainMr = miniConfig(t)
			zipCfg, zipMr = miniConfig(t)
			plain = storedSize(t, plainCfg, plainMr, small)
			zipped = storedSize(t, zipCfg.WithCompression(readmeCompression(algo)), zipMr, small)
			if zipped > plain+plain/10 {
				t.Errorf("value under MinSize stored %d bytes, plain is %d", zipped, plain)
			}
		})
	}
}

type elevenFields struct {
	A, B, C, D, E, F, G, H, I, J int
	K                            int
}

func TestFindingK1(t *testing.T) {
	t.Run("time.Time", func(t *testing.T) {
		w := obcache.Wrap(memoryCache(t, obcache.NewDefaultConfig()), func(at time.Time) int64 { return at.Unix() })
		if a, b := w(time.Unix(1000, 0)), w(time.Unix(2000, 0)); a != 1000 || b != 2000 {
			t.Errorf("got %d, %d", a, b)
		}
	})
	t.Run("long-slice", func(t *testing.T) {
		w := obcache.Wrap(memoryCache(t, obcache.NewDefaultConfig()), func(s []int) int { return s[5] })
		x, y := make([]int, 20), make([]int, 20)
		y[5] = 7
		if a, b := w(x), w(y); a != 0 || b != 7 {
			t.Errorf("got %d, %d", a, b)
		}
	})
	t.Run("large-map", func(t *testing.T) {
		w := obcache.Wrap(memoryCache(t, obcache.NewDefaultConfig()), func(m map[string]int) int { return m["f"] })
		x := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6}
		y := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 9}
		if a, b := w(x), w(y); a != 6 || b != 9 {
			t.Errorf("got %d, %d", a, b)
		}
	})
	t.Run("struct-field-eleven", func(t *testing.T) {
		w := obcache.Wrap(memoryCache(t, obcache.NewDefaultConfig()), func(v elevenFields) int { return v.K })
		if a, b := w(elevenFields{K: 1}), w(elevenFields{K: 2}); a != 1 || b != 2 {
			t.Errorf("got %d, %d", a, b)
		}
	})
	t.Run("byte-array", func(t *testing.T) {
		w := obcache.Wrap(memoryCache(t, obcache.NewDefaultConfig()), func(u [16]byte) byte { return u[0] })
		if a, b := w([16]byte{1}), w([16]byte{2}); a != 1 || b != 2 {
			t.Errorf("got %d, %d", a, b)
		}
	})
}

func TestFindingK2(t *testing.T) {
	var calls atomic.Int32
	w := obcache.Wrap(memoryCache(t, obcache.NewDefaultConfig()), func(a, b string) string {
		calls.Add(1)
		return a + "/" + b
	})
	if x, y := w("a|1:s:b", "c"), w("a", "b|1:s:c"); x == y {
		t.Errorf("distinct arguments shared a result: %q", x)
	}
	s := obcache.Wrap(memoryCache(t, obcache.NewDefaultConfig()), func(v []string) int { return len(v) })
	if x, y := s([]string{"a,s:b"}), s([]string{"a", "b"}); x == y {
		t.Errorf("distinct slices shared a result: %d", x)
	}
}
