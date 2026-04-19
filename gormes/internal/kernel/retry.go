package kernel

import (
	"context"
	"math/rand"
	"time"
)

// RetryBudget implements the Route-B reconnect schedule from spec §9.2:
// 1s, 2s, 4s, 8s, 16s with +/-20% jitter, then exhausted. Not goroutine-safe;
// the kernel holds one budget per turn on the Run goroutine.
type RetryBudget struct {
	attempt int
}

const maxRetryAttempts = 5

// NewRetryBudget returns a fresh budget — 5 attempts remaining.
func NewRetryBudget() *RetryBudget { return &RetryBudget{} }

// NextDelay returns the jittered backoff for the next attempt, or -1 if the
// budget is exhausted. Advances the internal attempt counter on each call.
func (b *RetryBudget) NextDelay() time.Duration {
	if b.attempt >= maxRetryAttempts {
		return -1
	}
	b.attempt++
	base := time.Second << uint(b.attempt-1)
	jitter := rand.Float64()*0.4 - 0.2 // +/-0.2
	return time.Duration(float64(base) * (1.0 + jitter))
}

// Exhausted returns true if NextDelay has been called maxRetryAttempts times.
func (b *RetryBudget) Exhausted() bool {
	return b.attempt >= maxRetryAttempts
}

// Wait sleeps for d or returns early on ctx cancellation. Returns ctx.Err()
// on cancellation, nil on clean timer expiration.
func Wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
