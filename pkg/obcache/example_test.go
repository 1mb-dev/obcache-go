package obcache_test

import (
	"fmt"
	"time"

	"github.com/1mb-dev/obcache-go/v2/pkg/compression"
	"github.com/1mb-dev/obcache-go/v2/pkg/obcache"
)

func square(n int) (int, error) {
	fmt.Println("computing", n)
	return n * n, nil
}

func ExampleWrap() {
	cache, err := obcache.New(obcache.NewDefaultConfig())
	if err != nil {
		panic(err)
	}
	defer func() { _ = cache.Close() }()

	cached := obcache.Wrap(cache, square)
	a, _ := cached(4)
	b, _ := cached(4)
	fmt.Println(a, b)
	// Output:
	// computing 4
	// 16 16
}

func ExampleCache_Key() {
	cache, err := obcache.New(obcache.NewDefaultConfig())
	if err != nil {
		panic(err)
	}
	defer func() { _ = cache.Close() }()

	cached := obcache.Wrap(cache, square, obcache.WithName("square"))
	_, _ = cached(4)

	if err := cache.Delete(cache.Key("square", 4)); err != nil {
		panic(err)
	}
	_, _ = cached(4)
	// Output:
	// computing 4
	// computing 4
}

// Memory returns the stored value, not a copy.
func Example_memoryResultsAreShared() {
	cache, err := obcache.New(obcache.NewDefaultConfig())
	if err != nil {
		panic(err)
	}
	defer func() { _ = cache.Close() }()

	cached := obcache.Wrap(cache, func(int) ([]int, error) { return []int{1, 2}, nil })
	first, _ := cached(1)
	first[0] = 99
	second, _ := cached(1)
	fmt.Println(second)
	// Output:
	// [99 2]
}

// With compression, Wrap keeps the declared type; Get has none, so JSON decides.
func Example_compressionGetReturnsJSONTypes() {
	cache, err := obcache.New(obcache.NewDefaultConfig().WithCompression(&compression.Config{
		Enabled:   true,
		Algorithm: compression.CompressorGzip,
		MinSize:   1000,
	}))
	if err != nil {
		panic(err)
	}
	defer func() { _ = cache.Close() }()

	if err := cache.Set("n", 7, time.Hour); err != nil {
		panic(err)
	}
	value, _ := cache.Get("n")
	fmt.Printf("%T\n", value)
	// Output:
	// float64
}
