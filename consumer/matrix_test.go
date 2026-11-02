package consumer

import (
	"math"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

type Inner struct {
	Tags []string
	At   time.Time
}

type Embedded struct {
	Inner
	ID int64
}

type User struct {
	ID    int64
	Name  string
	Inner Inner
}

type Node struct {
	Next *Node
}

type Sparse struct {
	Tags []string `json:"tags,omitempty"`
}

type ID int64

type Label string

// shape is one return type and value class crossing every backend.
type shape struct {
	name string
	run  func(t *testing.T, c *obcache.Cache, name string)
}

func deep[R any](a, b R) bool { return reflect.DeepEqual(a, b) }

func cell[R any](name string, v R, eq func(a, b R) bool) shape {
	return shape{name: name, run: func(t *testing.T, c *obcache.Cache, key string) {
		var calls atomic.Int32
		w := obcache.Wrap(c, func(int) (R, error) {
			calls.Add(1)
			return v, nil
		}, opts(key)...)

		miss, err := w(1)
		if err != nil {
			t.Fatalf("miss: %v", err)
		}
		hit, err := w(1)
		if err != nil {
			t.Fatalf("hit: %v", err)
		}
		if !eq(miss, v) {
			t.Errorf("miss = %#v, want %#v", miss, v)
		}
		if !eq(hit, miss) {
			t.Errorf("hit = %#v, miss = %#v", hit, miss)
		}
		if n := calls.Load(); n != 1 {
			t.Errorf("underlying calls = %d, want 1", n)
		}
		stats := c.Stats()
		if stats.Hits() != 1 || stats.Misses() != 1 {
			t.Errorf("hits = %d, misses = %d, want 1 and 1", stats.Hits(), stats.Misses())
		}
	}}
}

func shapes() []shape {
	large := strings.Repeat("compressible ", 5000)
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	user := User{ID: math.MaxInt64 - 1, Name: "ann", Inner: Inner{Tags: []string{"a"}, At: stamp}}
	timeEq := func(a, b time.Time) bool { return a.Equal(b) }

	return []shape{
		cell("int/zero", 0, deep[int]),
		cell("int/max", math.MaxInt, deep[int]),
		cell("int/min", math.MinInt, deep[int]),
		cell("int64/zero", int64(0), deep[int64]),
		cell("int64/max", int64(math.MaxInt64), deep[int64]),
		cell("int64/min", int64(math.MinInt64), deep[int64]),
		cell("uint64/max", uint64(math.MaxUint64), deep[uint64]),
		cell("float32/max", float32(math.MaxFloat32), deep[float32]),
		cell("float64/zero", 0.0, deep[float64]),
		cell("float64/max", math.MaxFloat64, deep[float64]),
		cell("bool/false", false, deep[bool]),
		cell("bool/true", true, deep[bool]),
		cell("string/empty", "", deep[string]),
		cell("string/small", "plain", deep[string]),
		cell("string/under-minsize", strings.Repeat("x", 999), deep[string]),
		cell("string/large", large, deep[string]),
		cell("bytes/empty", []byte{}, deep[[]byte]),
		cell("bytes/nil", []byte(nil), deep[[]byte]),
		cell("bytes/large", []byte(large), deep[[]byte]),
		cell("struct/zero", User{}, deep[User]),
		cell("struct/full", user, deep[User]),
		cell("struct/nested", Embedded{Inner: Inner{Tags: []string{"x"}, At: stamp}, ID: 2}, deep[Embedded]),
		cell("pointer/nil", (*User)(nil), deep[*User]),
		cell("pointer/full", &user, deep[*User]),
		cell("slice/nil", []string(nil), deep[[]string]),
		cell("slice/empty", []string{}, deep[[]string]),
		cell("slice/full", []string{"a", "b"}, deep[[]string]),
		cell("slice/nil-element", []*User{nil, {ID: 1}}, deep[[]*User]),
		cell("array/full", [3]int{1, 2, 3}, deep[[3]int]),
		cell("map/nil", map[string]string(nil), deep[map[string]string]),
		cell("map/empty", map[string]string{}, deep[map[string]string]),
		cell("map/string", map[string]string{"k": "v"}, deep[map[string]string]),
		cell("map/int", map[int]string{1: "v"}, deep[map[int]string]),
		cell("named/int", ID(7), deep[ID]),
		cell("named/string", Label("l"), deep[Label]),
		cell("time/zero", time.Time{}, timeEq),
		cell("time/utc", stamp, timeEq),
		{name: "multi/nil-middle", run: func(t *testing.T, c *obcache.Cache, key string) {
			var calls atomic.Int32
			w := obcache.Wrap(c, func(int) (string, *User, int, error) {
				calls.Add(1)
				return "a", nil, 2, nil
			}, opts(key)...)
			_, _, _, _ = w(1)
			a, u, n, err := w(1)
			if a != "a" || u != nil || n != 2 || err != nil {
				t.Errorf("hit = %q, %v, %d, %v", a, u, n, err)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("underlying calls = %d, want 1", got)
			}
		}},
		{name: "no-error-return", run: func(t *testing.T, c *obcache.Cache, key string) {
			var calls atomic.Int32
			w := obcache.Wrap(c, func(int) int64 {
				calls.Add(1)
				return math.MaxInt64 - 1
			}, opts(key)...)
			miss := w(1)
			if hit := w(1); hit != miss {
				t.Errorf("hit = %d, miss = %d", hit, miss)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("underlying calls = %d, want 1", got)
			}
		}},
		{name: "no-results", run: func(t *testing.T, c *obcache.Cache, key string) {
			var calls atomic.Int32
			w := obcache.Wrap(c, func(int) { calls.Add(1) }, opts(key)...)
			w(1)
			w(1)
			w(2)
			if got := calls.Load(); got != 2 {
				t.Errorf("underlying calls = %d, want one per key", got)
			}
		}},
		{name: "error-only", run: func(t *testing.T, c *obcache.Cache, key string) {
			var calls atomic.Int32
			w := obcache.Wrap(c, func(int) error {
				calls.Add(1)
				return nil
			}, opts(key)...)
			if err := w(1); err != nil {
				t.Errorf("miss: %v", err)
			}
			if err := w(1); err != nil {
				t.Errorf("hit: %v", err)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("underlying calls = %d, want 1", got)
			}
		}},
	}
}

func TestMatrix(t *testing.T) {
	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			for _, s := range shapes() {
				t.Run(s.name, func(t *testing.T) {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("panic: %v", r)
						}
					}()
					s.run(t, b.new(t), s.name)
				})
			}
		})
	}
}

// TestPinnedLosses asserts what JSON cannot carry on serializing backends, so
// the documented limits cannot drift from behavior.
func TestPinnedLosses(t *testing.T) {
	for _, b := range backends() {
		if !b.serial {
			continue
		}
		t.Run(b.name, func(t *testing.T) {
			t.Run("invalid-utf8", func(t *testing.T) {
				w := obcache.Wrap(b.new(t), func(int) (string, error) { return "a\xffb", nil }, opts("utf8")...)
				_, _ = w(1)
				if hit, _ := w(1); hit != "a\uFFFDb" {
					t.Errorf("hit = %q", hit)
				}
			})
			t.Run("nan-never-cached", func(t *testing.T) {
				var calls atomic.Int32
				w := obcache.Wrap(b.new(t), func(int) (float64, error) {
					calls.Add(1)
					return math.NaN(), nil
				}, opts("nan")...)
				_, _ = w(1)
				_, _ = w(1)
				if n := calls.Load(); n != 2 {
					t.Errorf("underlying calls = %d, want 2", n)
				}
			})
			t.Run("cycle-never-cached", func(t *testing.T) {
				var calls atomic.Int32
				w := obcache.Wrap(b.new(t), func(int) (*Node, error) {
					calls.Add(1)
					n := &Node{}
					n.Next = n
					return n, nil
				}, opts("cycle")...)
				_, _ = w(1)
				_, _ = w(1)
				if n := calls.Load(); n != 2 {
					t.Errorf("underlying calls = %d, want 2", n)
				}
			})
			t.Run("zone-name-lost", func(t *testing.T) {
				at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("EST", -5*3600))
				w := obcache.Wrap(b.new(t), func(int) (time.Time, error) { return at, nil }, opts("zone")...)
				_, _ = w(1)
				hit, _ := w(1)
				if !hit.Equal(at) {
					t.Errorf("hit = %v, want %v", hit, at)
				}
				if name, _ := hit.Zone(); name == "EST" {
					t.Errorf("zone name survived; update the documented limit")
				}
			})
			t.Run("omitempty-empty-becomes-nil", func(t *testing.T) {
				w := obcache.Wrap(b.new(t), func(int) (Sparse, error) {
					return Sparse{Tags: []string{}}, nil
				}, opts("omitempty")...)
				_, _ = w(1)
				if hit, _ := w(1); hit.Tags != nil {
					t.Errorf("hit.Tags = %#v, want nil", hit.Tags)
				}
			})
		})
	}
}
