package obcache

import (
	"testing"
)

func TestWrapFunc0WithError(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	const expectedResult = "result"
	callCount := 0
	fn := func() (string, error) {
		callCount++
		return expectedResult, nil
	}

	wrappedFn := WrapFunc0WithError(cache, fn)

	// First call
	result1, err1 := wrappedFn()
	if err1 != nil {
		t.Fatalf("Unexpected error: %v", err1)
	}
	if result1 != "result" {
		t.Fatalf("Expected 'result', got %s", result1)
	}
	if callCount != 1 {
		t.Fatalf("Expected 1 function call, got %d", callCount)
	}

	// Second call should be cached
	result2, err2 := wrappedFn()
	if err2 != nil {
		t.Fatalf("Unexpected error: %v", err2)
	}
	if result2 != "result" {
		t.Fatalf("Expected 'result', got %s", result2)
	}
	if callCount != 1 {
		t.Fatalf("Expected 1 function call (cached), got %d", callCount)
	}
}

func TestWrapFunc1WithError(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	callCount := 0
	fn := func(x int) (int, error) {
		callCount++
		return x * 2, nil
	}

	wrappedFn := WrapFunc1WithError(cache, fn)

	// First call
	result1, err1 := wrappedFn(5)
	if err1 != nil {
		t.Fatalf("Unexpected error: %v", err1)
	}
	if result1 != 10 {
		t.Fatalf("Expected 10, got %d", result1)
	}
	if callCount != 1 {
		t.Fatalf("Expected 1 function call, got %d", callCount)
	}

	// Second call with same argument should be cached
	result2, err2 := wrappedFn(5)
	if err2 != nil {
		t.Fatalf("Unexpected error: %v", err2)
	}
	if result2 != 10 {
		t.Fatalf("Expected 10, got %d", result2)
	}
	if callCount != 1 {
		t.Fatalf("Expected 1 function call (cached), got %d", callCount)
	}

	// Third call with different argument should call function
	result3, err3 := wrappedFn(3)
	if err3 != nil {
		t.Fatalf("Unexpected error: %v", err3)
	}
	if result3 != 6 {
		t.Fatalf("Expected 6, got %d", result3)
	}
	if callCount != 2 {
		t.Fatalf("Expected 2 function calls, got %d", callCount)
	}
}

func TestWrapFunc2WithError(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	callCount := 0
	fn := func(x int, y string) (string, error) {
		callCount++
		return y + "-" + string(rune('0'+x)), nil
	}

	wrappedFn := WrapFunc2WithError(cache, fn)

	// First call
	result1, err1 := wrappedFn(1, "test")
	if err1 != nil {
		t.Fatalf("Unexpected error: %v", err1)
	}
	expected := "test-1"
	if result1 != expected {
		t.Fatalf("Expected '%s', got '%s'", expected, result1)
	}
	if callCount != 1 {
		t.Fatalf("Expected 1 function call, got %d", callCount)
	}

	// Second call with same arguments should be cached
	result2, err2 := wrappedFn(1, "test")
	if err2 != nil {
		t.Fatalf("Unexpected error: %v", err2)
	}
	if result2 != expected {
		t.Fatalf("Expected '%s', got '%s'", expected, result2)
	}
	if callCount != 1 {
		t.Fatalf("Expected 1 function call (cached), got %d", callCount)
	}
}

func TestWrapFunc0WithErrorError(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	expectedErr := &testError{msg: "test error"}
	fn := func() (string, error) {
		return "", expectedErr
	}

	wrappedFn := WrapFunc0WithError(cache, fn)

	// Error should be returned and not cached
	result, err := wrappedFn()
	if err != expectedErr {
		t.Fatalf("Expected specific error, got %v", err)
	}
	if result != "" {
		t.Fatalf("Expected empty result on error, got %s", result)
	}
}

func TestWrapFunc1WithErrorError(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	expectedErr := &testError{msg: "test error"}
	fn := func(x int) (int, error) {
		if x < 0 {
			return 0, expectedErr
		}
		return x * 2, nil
	}

	wrappedFn := WrapFunc1WithError(cache, fn)

	// Error case
	result, err := wrappedFn(-1)
	if err != expectedErr {
		t.Fatalf("Expected specific error, got %v", err)
	}
	if result != 0 {
		t.Fatalf("Expected 0 result on error, got %d", result)
	}

	// Success case should work normally
	result, err = wrappedFn(5)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != 10 {
		t.Fatalf("Expected 10, got %d", result)
	}
}

func TestWrapFunc2WithErrorError(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	expectedErr := &testError{msg: "test error"}
	fn := func(x int, y string) (string, error) {
		if x < 0 {
			return "", expectedErr
		}
		return y + "-" + string(rune('0'+x)), nil
	}

	wrappedFn := WrapFunc2WithError(cache, fn)

	// Error case
	result, err := wrappedFn(-1, "test")
	if err != expectedErr {
		t.Fatalf("Expected specific error, got %v", err)
	}
	if result != "" {
		t.Fatalf("Expected empty result on error, got %s", result)
	}
}

func TestWrapFuncWithErrorCaching(t *testing.T) {
	cache, err := New(NewDefaultConfig())
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}

	callCount := 0
	fn := func(x int) (int, error) {
		callCount++
		if x < 0 {
			return 0, &testError{msg: "negative"}
		}
		return x * 2, nil
	}

	wrappedFn := WrapFunc1WithError(cache, fn)

	// Success case - should be cached
	result1, err1 := wrappedFn(5)
	if err1 != nil {
		t.Fatalf("Unexpected error: %v", err1)
	}
	if result1 != 10 {
		t.Fatalf("Expected 10, got %d", result1)
	}
	if callCount != 1 {
		t.Fatalf("Expected 1 call, got %d", callCount)
	}

	// Same success case - should use cache
	result2, err2 := wrappedFn(5)
	if err2 != nil {
		t.Fatalf("Unexpected error: %v", err2)
	}
	if result2 != 10 {
		t.Fatalf("Expected 10, got %d", result2)
	}
	if callCount != 1 {
		t.Fatalf("Expected 1 call (cached), got %d", callCount)
	}

	// Error case - should not be cached (called each time)
	_, err3 := wrappedFn(-1)
	if err3 == nil {
		t.Fatal("Expected error")
	}
	if callCount != 2 {
		t.Fatalf("Expected 2 calls after error, got %d", callCount)
	}

	// Same error case - should call function again (errors not cached)
	_, err4 := wrappedFn(-1)
	if err4 == nil {
		t.Fatal("Expected error")
	}
	if callCount != 3 {
		t.Fatalf("Expected 3 calls after second error, got %d", callCount)
	}
}

// testError is a custom error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
