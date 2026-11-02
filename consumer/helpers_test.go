// Package consumer exercises obcache from outside its module. The module path
// sits outside github.com/1mb-dev/obcache-go/v2, so importing internal/ fails to
// compile here; package obcache_test cannot give that guarantee.
package consumer

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/compression"
	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// suppress skips a test for a finding not yet fixed. The fixing commit deletes
// the call, and the helper itself goes before release so a leftover cannot pass.
func suppress(t *testing.T, id string) {
	t.Helper()
	t.Skip("finding " + id)
}

// opts is the single place every Wrap in this module gets its identity from.
func opts(name string) []obcache.WrapOption {
	return []obcache.WrapOption{obcache.WithName(name)}
}

// readmeCompression is the literal config README.md documents.
func readmeCompression(algo compression.CompressorType) *compression.Config {
	return &compression.Config{Enabled: true, Algorithm: algo, MinSize: 1000}
}

const redisGzip = "redis+gzip"

type backend struct {
	name   string
	serial bool // values pass through JSON
	redis  bool
	new    func(t *testing.T) *obcache.Cache
}

func memoryCache(t *testing.T, cfg *obcache.Config) *obcache.Cache {
	t.Helper()
	c, err := obcache.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func redisClient(t *testing.T, addr string) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func miniConfig(t *testing.T) (*obcache.Config, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	return obcache.NewRedisConfigWithClient(redisClient(t, mr.Addr())), mr
}

func backends() []backend {
	all := []backend{
		{name: "memory", new: func(t *testing.T) *obcache.Cache {
			return memoryCache(t, obcache.NewDefaultConfig())
		}},
		{name: "gzip", serial: true, new: func(t *testing.T) *obcache.Cache {
			return memoryCache(t, obcache.NewDefaultConfig().WithCompression(readmeCompression(compression.CompressorGzip)))
		}},
		{name: "deflate", serial: true, new: func(t *testing.T) *obcache.Cache {
			return memoryCache(t, obcache.NewDefaultConfig().WithCompression(readmeCompression(compression.CompressorDeflate)))
		}},
		{name: "redis", serial: true, redis: true, new: func(t *testing.T) *obcache.Cache {
			cfg, _ := miniConfig(t)
			return memoryCache(t, cfg)
		}},
		{name: redisGzip, serial: true, redis: true, new: func(t *testing.T) *obcache.Cache {
			cfg, _ := miniConfig(t)
			return memoryCache(t, cfg.WithCompression(readmeCompression(compression.CompressorGzip)))
		}},
	}
	if addr := os.Getenv("OBCACHE_TEST_REDIS"); addr != "" {
		all = append(all, backend{name: "real-redis", serial: true, redis: true, new: func(t *testing.T) *obcache.Cache {
			sum := sha256.Sum256([]byte(t.Name()))
			cfg := obcache.NewRedisConfigWithClient(redisClient(t, addr))
			cfg.Redis.KeyPrefix = "consumer-" + hex.EncodeToString(sum[:8]) + ":"
			c := memoryCache(t, cfg)
			// The prefix repeats across runs, and a server can outlive one.
			if err := c.Clear(); err != nil {
				t.Fatalf("Clear: %v", err)
			}
			return c
		}})
	}
	return all
}

// bounded fails the test if f does not return in time, so a hang reports as a
// failure instead of killing the binary at -timeout.
func bounded(t *testing.T, d time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case r := <-done:
		if r != nil {
			panic(r)
		}
	case <-time.After(d):
		t.Fatalf("%s: no result after %v", what, d)
	}
}

// gate is a TCP proxy that can stop forwarding, so a live connection goes silent
// the way a hung server does. Refusing connections would be the friendly outage.
type gate struct {
	addr string
	hung atomic.Bool
}

func newGate(t *testing.T, upstream string) *gate {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	g := &gate{addr: l.Addr().String()}
	go func() {
		for {
			down, err := l.Accept()
			if err != nil {
				return
			}
			up, err := net.Dial("tcp", upstream)
			if err != nil {
				_ = down.Close()
				return
			}
			t.Cleanup(func() { _ = down.Close(); _ = up.Close() })
			go g.pump(down, up)
			go g.pump(up, down)
		}
	}()
	return g
}

func (g *gate) pump(from, to net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		n, err := from.Read(buf)
		if err != nil {
			return
		}
		if g.hung.Load() {
			continue
		}
		if _, err := to.Write(buf[:n]); err != nil && err != io.ErrClosedPipe {
			return
		}
	}
}
