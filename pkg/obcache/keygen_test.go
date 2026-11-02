package obcache

import (
	"encoding"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaultKeyFunc(t *testing.T) {

	testCases := []struct {
		name     string
		args     []any
		expected string
	}{
		{
			name:     "single int",
			args:     []any{42},
			expected: "t3:inti42;",
		},
		{
			name:     "single string",
			args:     []any{"hello"},
			expected: "t6:strings5:hello",
		},
		{
			name:     "multiple args",
			args:     []any{"user", 123, true},
			expected: "t6:strings4:usert3:inti123;t4:boolbtrue",
		},
		{
			name:     "empty args",
			args:     []any{},
			expected: "no-args",
		},
		{
			name:     "nil arg",
			args:     []any{nil},
			expected: "n",
		},
		{
			name:     "mixed types",
			args:     []any{"str", 42, 3.14, true, nil},
			expected: "t6:strings3:strt3:inti42;t7:float64f3.14;t4:boolbtruen",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := DefaultKeyFunc(tc.args)
			if result != tc.expected {
				t.Fatalf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

func TestSimpleKeyFunc(t *testing.T) {

	testCases := []struct {
		name     string
		args     []any
		expected string
	}{
		{
			name:     "single int",
			args:     []any{42},
			expected: "42",
		},
		{
			name:     "single string",
			args:     []any{"hello"},
			expected: "hello",
		},
		{
			name:     "multiple args",
			args:     []any{"user", 123},
			expected: "user:123",
		},
		{
			name:     "empty args",
			args:     []any{},
			expected: "no-args",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := SimpleKeyFunc(tc.args)
			if result != tc.expected {
				t.Fatalf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

func TestDefaultKeyFuncComplexTypes(t *testing.T) {

	// Test struct
	type TestStruct struct {
		Name string
		Age  int
	}

	args := []any{TestStruct{Name: "John", Age: 30}}
	key := DefaultKeyFunc(args)
	// The actual format will depend on the implementation, let's just check it's not empty
	if key == "" {
		t.Fatal("Expected non-empty key for struct")
	}

	// Test slice
	args = []any{[]int{1, 2, 3}}
	key = DefaultKeyFunc(args)
	if key == "" {
		t.Fatal("Expected non-empty key for slice")
	}

	// Test map
	args = []any{map[string]int{"a": 1, "b": 2}}
	key = DefaultKeyFunc(args)
	// Map should produce a non-empty key
	if key == "" {
		t.Fatal("Expected non-empty key for map")
	}
}

func TestKeyFuncLongKeys(t *testing.T) {

	// Create a very long string
	longString := strings.Repeat("a", 300) // Longer than maxKeyLength (256)
	args := []any{longString}
	key := DefaultKeyFunc(args)

	// Should be hashed (64 characters for SHA256 hex)
	if len(key) != 64 {
		t.Fatalf("Expected hashed key length 64, got %d", len(key))
	}

	// Should be consistent
	key2 := DefaultKeyFunc(args)
	if key != key2 {
		t.Fatal("Long key hashing should be consistent")
	}
}

func TestKeyFuncConsistency(t *testing.T) {

	args := []any{"test", 123, true}

	// Should produce the same key for the same inputs
	key1 := DefaultKeyFunc(args)
	key2 := DefaultKeyFunc(args)

	if key1 != key2 {
		t.Fatalf("Key function should be consistent, got '%s' and '%s'", key1, key2)
	}
}

func TestKeyFuncDifferentArgsProduceDifferentKeys(t *testing.T) {

	testCases := []struct {
		args1 []any
		args2 []any
		name  string
	}{
		{
			args1: []any{"test", 123},
			args2: []any{"test", 124},
			name:  "different numbers",
		},
		{
			args1: []any{"test", 123},
			args2: []any{"test2", 123},
			name:  "different strings",
		},
		{
			args1: []any{"test", 123},
			args2: []any{123, "test"},
			name:  "different order",
		},
		{
			args1: []any{"test"},
			args2: []any{"test", nil},
			name:  "different arg count",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key1 := DefaultKeyFunc(tc.args1)
			key2 := DefaultKeyFunc(tc.args2)

			if key1 == key2 {
				t.Fatalf("Different args should produce different keys, both got '%s'", key1)
			}
		})
	}
}

func TestKeyFuncWithPointers(t *testing.T) {

	// Test that pointers are dereferenced
	value := 42
	ptr := &value

	args1 := []any{value}
	args2 := []any{ptr}

	key1 := DefaultKeyFunc(args1)
	key2 := DefaultKeyFunc(args2)

	// Keys should be different because one is int and one is *int
	if key1 == key2 {
		t.Fatalf("Pointer and value should produce different keys, both got '%s'", key1)
	}
}

func TestKeyFuncSpecialCharacters(t *testing.T) {

	// Test strings with special characters
	specialStrings := []string{
		"hello:world",
		"test\nwith\nnewlines",
		"unicode: 你好",
		"symbols: !@#$%^&*()",
		"",
	}

	// Each should produce a different key
	keys := make(map[string]bool)
	for _, str := range specialStrings {
		key := DefaultKeyFunc([]any{str})
		if keys[key] {
			t.Fatalf("Duplicate key generated for special string: '%s'", str)
		}
		keys[key] = true
	}
}

func TestKeyFuncIntegrationWithCache(t *testing.T) {
	// Test that custom key functions work with the cache
	customKeyFunc := func(_ []any) string {
		// Always return the same key regardless of args
		return "constant-key"
	}

	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	fn1 := func(x int) int { return x * 2 }
	fn2 := func(x int) int { return x * 3 }

	wrapped1 := Wrap(cache, fn1, WithKeyFunc(customKeyFunc))
	wrapped2 := Wrap(cache, fn2, WithKeyFunc(customKeyFunc))

	// First call
	result1 := wrapped1(5)
	if result1 != 10 {
		t.Fatalf("Expected 10, got %d", result1)
	}

	// Second call with different function but same key should return cached result
	result2 := wrapped2(7) // Would be 21 if executed, but should return cached 10
	if result2 != 10 {
		t.Fatalf("Expected cached result 10, got %d", result2)
	}
}

func BenchmarkDefaultKeyFunc(b *testing.B) {
	args := []any{"user", 12345, "active", true}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DefaultKeyFunc(args)
	}
}

func BenchmarkSimpleKeyFunc(b *testing.B) {
	args := []any{"user", 12345, "active", true}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SimpleKeyFunc(args)
	}
}

func BenchmarkDefaultKeyFuncLongKey(b *testing.B) {
	longString := strings.Repeat("x", 300)
	args := []any{longString}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DefaultKeyFunc(args)
	}
}

type embeddedTime struct {
	time.Time
	Tenant string
}

func TestDefaultKeyFuncWalksPromotedMarshalers(t *testing.T) {
	at := time.Unix(1000, 0)
	a := DefaultKeyFunc([]any{embeddedTime{at, "alice"}})
	b := DefaultKeyFunc([]any{embeddedTime{at, "bob"}})
	if a == b {
		t.Error("embedded time.Time hid the Tenant field from the key")
	}
}

type marshalerField struct {
	M encoding.TextMarshaler
}

func TestDefaultKeyFuncNilMarshalerField(t *testing.T) {
	if a, b := DefaultKeyFunc([]any{marshalerField{}}), DefaultKeyFunc([]any{marshalerField{time.Unix(1, 0)}}); a == b {
		t.Error("nil and set interface fields shared a key")
	}
}

func TestDefaultKeyFuncPanicsWithNoEncoding(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic on an argument with no encoding")
		}
	}()
	DefaultKeyFunc([]any{make(chan int)})
}

func TestEncodeArgsLargeValuesHash(t *testing.T) {
	large := make([]byte, 3*keySpill)
	other := make([]byte, 3*keySpill)
	other[len(other)-1] = 1
	a, err := encodeArgs([]any{large})
	if err != nil {
		t.Fatal(err)
	}
	b, err := encodeArgs([]any{other})
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) != 64 {
		t.Errorf("a = %q, b = %q", a, b)
	}
	if s, _ := encodeArgs([]any{strings.Repeat("x", 3*keySpill)}); len(s) != 64 {
		t.Errorf("large string key = %q", s)
	}
}

// Keys must stay distinct whatever the strings contain, so the generator favors
// the encoding's own delimiters.
func TestEncodeArgsIsInjective(t *testing.T) {
	alphabet := []string{"a", "b", "|", ":", ",", ";", "s", "t", "i", "n", "(", ")", "[", "{", "1", "0", "6", "\x00"}
	r := rand.New(rand.NewPCG(1, 2))
	word := func() string {
		var b strings.Builder
		for range r.IntN(5) {
			b.WriteString(alphabet[r.IntN(len(alphabet))])
		}
		return b.String()
	}
	build := func() []any {
		switch r.IntN(5) {
		case 0:
			return []any{word(), word()}
		case 1:
			return []any{word()}
		case 2:
			return []any{[]string{word(), word()}}
		case 3:
			return []any{map[string]string{word(): word(), word(): word()}}
		}
		return []any{[]string{word()}, []string{word()}}
	}

	seen := make(map[string][]any)
	for range 200_000 {
		args := build()
		key, err := encodeArgs(args)
		if err != nil {
			t.Fatal(err)
		}
		if prior, ok := seen[key]; ok && !reflect.DeepEqual(prior, args) {
			t.Fatalf("key %q shared by %#v and %#v", key, prior, args)
		}
		seen[key] = args
	}
}

func BenchmarkDefaultKeyFuncLargeBytes(b *testing.B) {
	args := []any{make([]byte, 1<<20)}
	b.ReportAllocs()
	for b.Loop() {
		_ = DefaultKeyFunc(args)
	}
}
