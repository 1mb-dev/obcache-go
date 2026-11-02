package singleflight

import (
	"context"
	"strconv"
	"testing"
)

func BenchmarkDo(b *testing.B) {
	var g Group[string, int]
	ctx := context.Background()
	for b.Loop() {
		_, _, _ = g.Do(ctx, "k", func() int { return 1 })
	}
}

func BenchmarkDoConcurrent(b *testing.B) {
	var g Group[string, int]
	ctx := context.Background()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = g.Do(ctx, "k", func() int { return 1 })
		}
	})
}

func BenchmarkDoDifferentKeys(b *testing.B) {
	var g Group[string, int]
	ctx := context.Background()
	i := 0
	for b.Loop() {
		i++
		_, _, _ = g.Do(ctx, strconv.Itoa(i%100), func() int { return 1 })
	}
}
