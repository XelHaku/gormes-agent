package kernel

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/internal/telemetry"
)

func TestRetryBudget_NextDelay_ExponentialWithJitter(t *testing.T) {
	b := NewRetryBudget()
	base := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
	}
	for i, want := range base {
		got := b.NextDelay()
		low := time.Duration(float64(want) * 0.8)
		high := time.Duration(float64(want) * 1.2)
		if got < low || got > high {
			t.Errorf("attempt %d: delay = %v, want within +/-20%% of %v", i+1, got, want)
		}
	}
	if got := b.NextDelay(); got != -1 {
		t.Errorf("attempt 6: delay = %v, want -1 (budget exhausted)", got)
	}
}

func TestRetryBudget_Exhausted(t *testing.T) {
	b := NewRetryBudget()
	for i := 0; i < 5; i++ {
		_ = b.NextDelay()
	}
	if !b.Exhausted() {
		t.Error("Exhausted should be true after 5 attempts")
	}
}

func TestRetryBudget_WaitRespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := Wait(ctx, 1*time.Hour)
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("Wait blocked %v on cancelled ctx; must return immediately", d)
	}
}

func TestRetryBudget_NextDelayForUsesCappedRetryAfterHint(t *testing.T) {
	b := NewRetryBudget()
	err := &hermes.HTTPError{Status: http.StatusTooManyRequests, RetryAfter: time.Hour}

	if got := b.NextDelayFor(err); got != 16*time.Second {
		t.Fatalf("NextDelayFor = %v, want capped 16s provider hint", got)
	}
}

func TestRetryBudget_NextDelayForFallsBackToJitteredSchedule(t *testing.T) {
	b := NewRetryBudget()

	got := b.NextDelayFor(&hermes.HTTPError{Status: http.StatusTooManyRequests})
	if got < 800*time.Millisecond || got > 1200*time.Millisecond {
		t.Fatalf("NextDelayFor = %v, want first jittered schedule delay", got)
	}
}

func TestKernel_OpenStreamRetryUsesProviderRetryAfterHint(t *testing.T) {
	client := &retryAfterClient{
		firstErr: &hermes.HTTPError{
			Status:     http.StatusTooManyRequests,
			Body:       "slow down",
			RetryAfter: time.Millisecond,
		},
	}
	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)

	initial := <-k.Render()
	if initial.Phase != PhaseIdle {
		t.Fatalf("initial phase = %v, want Idle", initial.Phase)
	}
	start := time.Now()
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	final := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.Seq > initial.Seq
	}, 300*time.Millisecond)
	if time.Since(start) > 250*time.Millisecond {
		t.Fatalf("retry took %v; provider RetryAfter hint was not preferred over jittered backoff", time.Since(start))
	}
	if client.calls != 2 {
		t.Fatalf("OpenStream calls = %d, want 2", client.calls)
	}
	if len(final.History) == 0 || final.History[len(final.History)-1].Content != "ok" {
		t.Fatalf("final history = %+v, want assistant content ok", final.History)
	}
}

func TestKernel_InitialFrameExposesProviderCapabilitiesAndRetrySchedule(t *testing.T) {
	client := &statusClient{status: hermes.ProviderStatus{
		Provider: "fixture-provider",
		Runtime:  "fixture-runtime",
		Capabilities: hermes.ProviderCapabilities{
			PromptCache:     hermes.CapabilityStatus{Available: false, Reason: "fixture cache disabled"},
			RateGuard:       hermes.CapabilityStatus{Available: false, Reason: "fixture rate guard unavailable"},
			BudgetTelemetry: hermes.CapabilityStatus{Available: false, Reason: "fixture budget telemetry unavailable"},
		},
	}}
	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go k.Run(ctx)

	frame := <-k.Render()
	if frame.ProviderStatus.Provider != "fixture-provider" {
		t.Fatalf("ProviderStatus.Provider = %q, want fixture-provider", frame.ProviderStatus.Provider)
	}
	if frame.ProviderStatus.Capabilities.PromptCache.Reason != "fixture cache disabled" {
		t.Fatalf("PromptCache.Reason = %q, want fixture cache disabled", frame.ProviderStatus.Capabilities.PromptCache.Reason)
	}
	wantSchedule := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
	}
	if !reflect.DeepEqual(frame.RetryStatus.Schedule, wantSchedule) {
		t.Fatalf("RetryStatus.Schedule = %v, want %v", frame.RetryStatus.Schedule, wantSchedule)
	}
	if frame.RetryStatus.MaxProviderRetryAfter != 16*time.Second {
		t.Fatalf("RetryStatus.MaxProviderRetryAfter = %v, want 16s", frame.RetryStatus.MaxProviderRetryAfter)
	}
}

func TestKernel_ReconnectingFrameReportsProviderRetryAfterDecision(t *testing.T) {
	client := &retryAfterClient{
		firstErr: &hermes.HTTPError{
			Status:     http.StatusTooManyRequests,
			Body:       `{"error":{"message":"slow down","code":"rate_limit"}}`,
			RetryAfter: 25 * time.Millisecond,
		},
	}
	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, store.NewNoop(), telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go k.Run(ctx)

	initial := <-k.Render()
	if initial.Phase != PhaseIdle {
		t.Fatalf("initial phase = %v, want Idle", initial.Phase)
	}
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	reconnecting := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseReconnecting && f.RetryStatus.LastDecision == RetryDecisionProviderHint
	}, 300*time.Millisecond)

	if reconnecting.RetryStatus.AttemptsUsed != 1 {
		t.Fatalf("AttemptsUsed = %d, want 1", reconnecting.RetryStatus.AttemptsUsed)
	}
	if reconnecting.RetryStatus.LastProviderRetryAfter != 25*time.Millisecond {
		t.Fatalf("LastProviderRetryAfter = %v, want 25ms", reconnecting.RetryStatus.LastProviderRetryAfter)
	}
	if reconnecting.RetryStatus.LastDelay != 25*time.Millisecond {
		t.Fatalf("LastDelay = %v, want provider hint 25ms", reconnecting.RetryStatus.LastDelay)
	}
	if reconnecting.RetryStatus.LastScheduledDelay < 800*time.Millisecond ||
		reconnecting.RetryStatus.LastScheduledDelay > 1200*time.Millisecond {
		t.Fatalf("LastScheduledDelay = %v, want first jittered backoff envelope", reconnecting.RetryStatus.LastScheduledDelay)
	}
	if reconnecting.RetryStatus.LastErrorKind != hermes.ProviderErrorRateLimit.String() {
		t.Fatalf("LastErrorKind = %q, want %q", reconnecting.RetryStatus.LastErrorKind, hermes.ProviderErrorRateLimit)
	}
	if reconnecting.RetryStatus.LastErrorClass != hermes.ClassRetryable.String() {
		t.Fatalf("LastErrorClass = %q, want retryable", reconnecting.RetryStatus.LastErrorClass)
	}
}

type retryAfterClient struct {
	firstErr error
	calls    int
}

func (c *retryAfterClient) OpenStream(context.Context, hermes.ChatRequest) (hermes.Stream, error) {
	c.calls++
	if c.calls == 1 {
		return nil, c.firstErr
	}
	return &retryAfterStream{
		events: []hermes.Event{
			{Kind: hermes.EventToken, Token: "ok"},
			{Kind: hermes.EventDone, FinishReason: "stop"},
		},
	}, nil
}

func (c *retryAfterClient) OpenRunEvents(context.Context, string) (hermes.RunEventStream, error) {
	return nil, hermes.ErrRunEventsNotSupported
}

func (c *retryAfterClient) Health(context.Context) error { return nil }

type statusClient struct {
	status hermes.ProviderStatus
}

func (c *statusClient) OpenStream(context.Context, hermes.ChatRequest) (hermes.Stream, error) {
	return nil, io.EOF
}

func (c *statusClient) OpenRunEvents(context.Context, string) (hermes.RunEventStream, error) {
	return nil, hermes.ErrRunEventsNotSupported
}

func (c *statusClient) Health(context.Context) error { return nil }

func (c *statusClient) ProviderStatus() hermes.ProviderStatus { return c.status }

type retryAfterStream struct {
	events []hermes.Event
	pos    int
}

func (s *retryAfterStream) Recv(context.Context) (hermes.Event, error) {
	if s.pos >= len(s.events) {
		return hermes.Event{}, io.EOF
	}
	ev := s.events[s.pos]
	s.pos++
	return ev, nil
}

func (s *retryAfterStream) SessionID() string { return "" }
func (s *retryAfterStream) Close() error      { return nil }
