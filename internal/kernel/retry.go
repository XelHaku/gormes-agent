package kernel

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
)

// RetryBudget implements the Route-B reconnect schedule from spec §9.2:
// 1s, 2s, 4s, 8s, 16s with +/-20% jitter, then exhausted. Not goroutine-safe;
// the kernel holds one budget per turn on the Run goroutine.
type RetryBudget struct {
	attempt int
}

const maxRetryAttempts = 5
const maxProviderRetryAfter = 16 * time.Second

const (
	RetryDecisionScheduled     = "scheduled_backoff"
	RetryDecisionProviderHint  = "provider_retry_after"
	RetryDecisionBudgetExhaust = "budget_exhausted"
)

type RetryStatus struct {
	Schedule               []time.Duration
	MaxAttempts            int
	MaxProviderRetryAfter  time.Duration
	AttemptsUsed           int
	LastScheduledDelay     time.Duration
	LastDelay              time.Duration
	LastProviderRetryAfter time.Duration
	LastDecision           string
	LastErrorClass         string
	LastErrorKind          string
}

type RetryDelayDecision struct {
	Attempt            int
	ScheduledDelay     time.Duration
	Delay              time.Duration
	ProviderRetryAfter time.Duration
	Decision           string
}

// NewRetryBudget returns a fresh budget — 5 attempts remaining.
func NewRetryBudget() *RetryBudget { return &RetryBudget{} }

func NewRetryStatus() RetryStatus {
	return RetryStatus{
		Schedule:              RetrySchedule(),
		MaxAttempts:           maxRetryAttempts,
		MaxProviderRetryAfter: maxProviderRetryAfter,
	}
}

func RetrySchedule() []time.Duration {
	return []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
	}
}

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

// NextDelayFor advances the retry budget and prefers a provider Retry-After
// hint when the triggering error carries one. The hint is capped to the
// reconnect budget's maximum base delay so a provider cannot stall the kernel
// beyond the bounded Route-B recovery window.
func (b *RetryBudget) NextDelayFor(err error) time.Duration {
	return b.NextDelayDecision(err).Delay
}

// NextDelayDecision advances the retry budget and returns both the jittered
// schedule result and any provider hint decision made for status reporting.
func (b *RetryBudget) NextDelayDecision(err error) RetryDelayDecision {
	scheduled := b.NextDelay()
	decision := RetryDelayDecision{
		Attempt:        b.attempt,
		ScheduledDelay: scheduled,
		Delay:          scheduled,
		Decision:       RetryDecisionScheduled,
	}
	if scheduled < 0 {
		decision.Decision = RetryDecisionBudgetExhaust
		return decision
	}
	hint := providerRetryAfter(err)
	if hint <= 0 {
		return decision
	}
	if hint > maxProviderRetryAfter {
		hint = maxProviderRetryAfter
	}
	decision.ProviderRetryAfter = hint
	decision.Delay = hint
	decision.Decision = RetryDecisionProviderHint
	return decision
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

func providerRetryAfter(err error) time.Duration {
	var httpErr *hermes.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.RetryAfter
	}
	return 0
}

func retryStatusWithDecision(status RetryStatus, decision RetryDelayDecision, classification hermes.ProviderErrorClassification) RetryStatus {
	if len(status.Schedule) == 0 {
		status = NewRetryStatus()
	}
	status.AttemptsUsed = decision.Attempt
	status.LastScheduledDelay = decision.ScheduledDelay
	status.LastDelay = decision.Delay
	status.LastProviderRetryAfter = decision.ProviderRetryAfter
	status.LastDecision = decision.Decision
	status.LastErrorClass = classification.Class.String()
	status.LastErrorKind = classification.Kind.String()
	return status
}

func (s RetryStatus) snapshot() RetryStatus {
	s.Schedule = append([]time.Duration(nil), s.Schedule...)
	return s
}
