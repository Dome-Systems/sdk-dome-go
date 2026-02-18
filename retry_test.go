package dome

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackoff_NoFailures(t *testing.T) {
	got := backoff(time.Second, 5*time.Minute, 0)
	if got != time.Second {
		t.Errorf("backoff(0 failures) = %v, want %v", got, time.Second)
	}
}

func TestBackoff_Exponential(t *testing.T) {
	base := 100 * time.Millisecond
	max := 5 * time.Minute

	for failures := 1; failures <= 5; failures++ {
		got := backoff(base, max, failures)
		// With jitter, the value varies. Check it's within bounds.
		if got < base {
			t.Errorf("backoff(%d failures) = %v, less than base %v", failures, got, base)
		}
		if got > max {
			t.Errorf("backoff(%d failures) = %v, exceeds max %v", failures, got, max)
		}
	}
}

func TestBackoff_CapsAtMax(t *testing.T) {
	base := time.Second
	max := 10 * time.Second

	// With 20 failures, the raw exponential would be huge, but should cap at max.
	for i := 0; i < 100; i++ {
		got := backoff(base, max, 20)
		if got > max+max/4 { // allow for +25% jitter on the max
			t.Errorf("backoff(20 failures) = %v, exceeds max+jitter %v", got, max+max/4)
		}
		if got < base {
			t.Errorf("backoff(20 failures) = %v, less than base %v", got, base)
		}
	}
}

func TestRetryWithBackoff_SucceedsImmediately(t *testing.T) {
	var calls atomic.Int32
	err := retryWithBackoff(context.Background(), func(_ context.Context) error {
		calls.Add(1)
		return nil
	}, time.Millisecond, time.Second)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

func TestRetryWithBackoff_RetriesOnFailure(t *testing.T) {
	var calls atomic.Int32
	err := retryWithBackoff(context.Background(), func(_ context.Context) error {
		n := calls.Add(1)
		if n < 3 {
			return errors.New("not yet")
		}
		return nil
	}, time.Millisecond, 10*time.Millisecond)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}

func TestRetryWithBackoff_StopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var calls atomic.Int32
	go func() {
		// Cancel after a few retries.
		for calls.Load() < 2 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	err := retryWithBackoff(ctx, func(_ context.Context) error {
		calls.Add(1)
		return errors.New("always fail")
	}, time.Millisecond, 10*time.Millisecond)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}
