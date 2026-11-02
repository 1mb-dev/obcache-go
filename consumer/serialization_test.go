package consumer

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/compression"
	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
	"github.com/alicebob/miniredis/v2"
)

const fresh = "fresh"

const stamp = `"created_at":"2026-01-01T00:00:00Z","last_access":"2026-01-01T00:00:00Z"`

func gzipBase64(t *testing.T, payload []byte) string {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// Bytes a v2.0.2 pod wrote under an explicit prefix. New keys differ, but a
// WithKeyFunc key does not, so these reach the decoder.
func TestGoldenV2Entries(t *testing.T) {
	fixtures := map[string]string{
		"cached error": `{"value":{"Err":{}},` + stamp + `}`,
		"null value":   `{"value":null,` + stamp + `}`,
		"gzip base64":  `{"value":"` + gzipBase64(t, []byte(`{"ID":1}`)) + `",` + stamp + `}`,
		"plain value":  `{"value":{"ID":1},` + stamp + `}`,
	}
	for name, raw := range fixtures {
		t.Run(name, func(t *testing.T) {
			cfg, mr := miniConfig(t)
			cfg.Redis.KeyPrefix = "myapp:"
			c := memoryCache(t, cfg)
			if err := mr.Set("myapp:k", raw); err != nil {
				t.Fatal(err)
			}

			if _, ok := c.Get("k"); ok {
				t.Error("an entry with no format read as a hit")
			}
			if !mr.Exists("myapp:k") {
				t.Error("reading an old entry deleted it from under an older pod")
			}

			var calls atomic.Int32
			w := obcache.Wrap(c, func(int) (Inner, error) {
				calls.Add(1)
				return Inner{Tags: []string{fresh}}, nil
			}, obcache.WithKeyFunc(func([]any) string { return "k" }))
			got, err := w(1)
			if err != nil || len(got.Tags) != 1 || calls.Load() != 1 {
				t.Errorf("got %+v, %v, calls=%d: an old entry passed as a value", got, err, calls.Load())
			}
		})
	}
}

func TestFindingDM1(t *testing.T) {
	cases := map[string]string{
		"wrong type":        `"a string"`,
		"unknown field":     `{"ID":1,"Gone":true}`,
		"null for a struct": `null`,
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, mr := miniConfig(t)
			c := memoryCache(t, cfg)
			var calls atomic.Int32
			w := obcache.Wrap(c, func(int) (Embedded, error) { calls.Add(1); return Embedded{ID: 2}, nil }, opts("n")...)
			key := "obcache2:" + c.Key("n", 1)
			if err := mr.Set(key, `{"value":`+value+`,`+stamp+`,"format":"json"}`); err != nil {
				t.Fatal(err)
			}

			got, err := w(1)
			if err != nil || got.ID != 2 || calls.Load() != 1 {
				t.Errorf("got %+v, %v, calls=%d", got, err, calls.Load())
			}
			s := c.Stats()
			if s.DecodeFailures() != 1 || s.Hits() != 0 || s.Misses() != 1 {
				t.Errorf("decodeFailures=%d hits=%d misses=%d", s.DecodeFailures(), s.Hits(), s.Misses())
			}
		})
	}
}

// A rolling change of compression settings puts both kinds of entry on one Redis.
func TestCompressionFlip(t *testing.T) {
	value := strings.Repeat("compressible ", 500)
	write := func(t *testing.T, mr *miniredis.Miniredis, cfg *compression.Config) {
		t.Helper()
		base := obcache.NewRedisConfigWithClient(redisClient(t, mr.Addr()))
		if cfg != nil {
			base = base.WithCompression(cfg)
		}
		w := obcache.Wrap(memoryCache(t, base), func(int) (string, error) { return value, nil }, opts("v")...)
		if _, err := w(1); err != nil {
			t.Fatal(err)
		}
	}
	read := func(t *testing.T, mr *miniredis.Miniredis, cfg *compression.Config) (hits int64, got string) {
		t.Helper()
		base := obcache.NewRedisConfigWithClient(redisClient(t, mr.Addr()))
		if cfg != nil {
			base = base.WithCompression(cfg)
		}
		c := memoryCache(t, base)
		w := obcache.Wrap(c, func(int) (string, error) { return value, nil }, opts("v")...)
		got, err := w(1)
		if err != nil {
			t.Fatal(err)
		}
		return c.Stats().Hits(), got
	}

	t.Run("compressed entry on a cache without compression is a miss", func(t *testing.T) {
		mr := miniredis.RunT(t)
		write(t, mr, readmeCompression(compression.CompressorGzip))
		if hits, got := read(t, mr, nil); hits != 0 || got != value {
			t.Errorf("hits=%d, value intact=%v", hits, got == value)
		}
	})
	t.Run("entry from another compressor is a miss", func(t *testing.T) {
		mr := miniredis.RunT(t)
		write(t, mr, readmeCompression(compression.CompressorGzip))
		if hits, got := read(t, mr, readmeCompression(compression.CompressorDeflate)); hits != 0 || got != value {
			t.Errorf("hits=%d, value intact=%v", hits, got == value)
		}
	})
	t.Run("plain entry on a compressing cache reads correctly", func(t *testing.T) {
		mr := miniredis.RunT(t)
		write(t, mr, nil)
		if _, got := read(t, mr, readmeCompression(compression.CompressorGzip)); got != value {
			t.Errorf("value intact = %v", got == value)
		}
	})
}

func TestDecompressionIsBounded(t *testing.T) {
	cfg, mr := miniConfig(t)
	c := memoryCache(t, cfg.WithCompression(readmeCompression(compression.CompressorGzip)))
	var calls atomic.Int32
	w := obcache.Wrap(c, func(int) (string, error) { calls.Add(1); return fresh, nil }, opts("bomb")...)

	// Valid JSON once expanded, so only the size limit can refuse it.
	payload := append(append([]byte{'"'}, bytes.Repeat([]byte{'0'}, compression.MaxDecompressedSize+1024)...), '"')
	encoded := `{"value":"` + gzipBase64(t, payload) + `",` + stamp + `,"format":"gzip"}`
	if err := mr.Set("obcache2:"+c.Key("bomb", 1), encoded); err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 2<<20 {
		t.Fatalf("fixture is %d bytes; it should be small", len(encoded))
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got, err := w(1)
	runtime.ReadMemStats(&after)

	if got != fresh || err != nil || calls.Load() != 1 {
		t.Errorf("got %d bytes, %v, calls=%d", len(got), err, calls.Load())
	}
	if failures := c.Stats().DecodeFailures(); failures != 1 {
		t.Errorf("decode failures = %d", failures)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 512<<20 {
		t.Errorf("one bad entry allocated %d MiB", grew>>20)
	}
}

// Hooks see what the caller receives: never stored bytes.
func TestOnHitValue(t *testing.T) {
	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			var seen atomic.Value
			hooks := obcache.NewHooks()
			hooks.AddOnHit(func(_ context.Context, _ string, value any) { seen.Store(fmt.Sprintf("%T", value)) })

			c := withHooks(t, b, hooks)
			w := obcache.Wrap(c, func(int) (User, error) { return User{ID: 1}, nil }, opts("user")...)
			_, _ = w(1)
			_, _ = w(1)
			if got := seen.Load(); got != "consumer.User" {
				t.Errorf("Wrap hit: hook saw %v", got)
			}

			multi := obcache.Wrap(c, func(int) (string, int, error) { return "a", 1, nil }, opts("multi")...)
			_, _, _ = multi(1)
			_, _, _ = multi(1)
			if got := seen.Load(); got != "[]interface {}" {
				t.Errorf("multi-return hit: hook saw %v", got)
			}

			_ = c.Set("k", "v", time.Hour)
			c.Get("k")
			if got := seen.Load(); got != "string" {
				t.Errorf("Get hit: hook saw %v", got)
			}
		})
	}
}

func withHooks(t *testing.T, b backend, hooks *obcache.Hooks) *obcache.Cache {
	t.Helper()
	var cfg *obcache.Config
	switch {
	case b.redis:
		cfg, _ = miniConfig(t)
	default:
		cfg = obcache.NewDefaultConfig()
	}
	if strings.Contains(b.name, "gzip") {
		cfg = cfg.WithCompression(readmeCompression(compression.CompressorGzip))
	}
	if b.name == "deflate" {
		cfg = cfg.WithCompression(readmeCompression(compression.CompressorDeflate))
	}
	return memoryCache(t, cfg.WithHooks(hooks))
}

// json.RawMessage is the documented way out for a dynamic shape on Redis.
func TestRawMessageRoundTrip(t *testing.T) {
	cfg, _ := miniConfig(t)
	c := memoryCache(t, cfg)
	var calls atomic.Int32
	w := obcache.Wrap(c, func(int) (json.RawMessage, error) {
		calls.Add(1)
		return json.RawMessage(`{"any":"shape"}`), nil
	}, opts("raw")...)
	miss, _ := w(1)
	hit, err := w(1)
	if err != nil || !reflect.DeepEqual(miss, hit) || calls.Load() != 1 {
		t.Errorf("miss=%s hit=%s err=%v calls=%d", miss, hit, err, calls.Load())
	}
}
