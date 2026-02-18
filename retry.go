package dome

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// backoff calculates the next backoff interval given consecutive failures.
// It uses exponential backoff with jitter, capped at maxInterval.
func backoff(baseInterval, maxInterval time.Duration, consecutiveFailures int) time.Duration {
	if consecutiveFailures <= 0 {
		return baseInterval
	}

	// Exponential: base * 2^failures
	multiplier := math.Pow(2, float64(consecutiveFailures))
	interval := time.Duration(float64(baseInterval) * multiplier)

	if interval > maxInterval {
		interval = maxInterval
	}

	// Add jitter: +/- 25% of the interval
	jitter := time.Duration(float64(interval) * 0.25 * (rand.Float64()*2 - 1))
	interval += jitter

	// Never go below the base interval
	if interval < baseInterval {
		interval = baseInterval
	}

	return interval
}

// retryWithBackoff retries fn with exponential backoff until it succeeds or
// the context is canceled. It calls onRetry (if non-nil) before each retry
// with the current attempt number and next wait duration.
func retryWithBackoff(ctx context.Context, fn func(ctx context.Context) error, baseInterval, maxInterval time.Duration) error {
	var failures int
	for {
		err := fn(ctx)
		if err == nil {
			return nil
		}

		failures++
		wait := backoff(baseInterval, maxInterval, failures)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}
