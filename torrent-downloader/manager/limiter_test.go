package manager

import (
	"testing"
	"time"
)

func TestLimiter_NoLimit(t *testing.T) {
	limiter := NewLimiter(0)

	start := time.Now()
	limiter.Wait(1000) // Should not block
	duration := time.Since(start)

	if duration > 10*time.Millisecond {
		t.Errorf("Expected no delay, but got %v", duration)
	}
}

func TestLimiter_WithLimit(t *testing.T) {
	// Limit to 1000 bytes per second
	limiter := NewLimiter(1000)

	start := time.Now()
	// Send 500 bytes, shouldn't block much if we are within the first second,
	// but the wait function logic might sleep if we exceed the allowed bytes for the elapsed time fraction.
	// We'll test a simpler scenario: wait on a small amount.
	limiter.Wait(100)
	duration := time.Since(start)

	// Wait should be relatively quick
	if duration > 200*time.Millisecond {
		t.Errorf("Unexpectedly long delay: %v", duration)
	}
}
