package kernel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/internal/telemetry"
)

func TestKernel_StreamInterruptSuppressesRetryAfterPlatformCancel(t *testing.T) {
	st := newInterruptRetryStore()
	releaseFirst := make(chan struct{})
	client := &interruptRetryClient{
		firstErr: &hermes.HTTPError{
			Status:     http.StatusServiceUnavailable,
			Body:       "transient stream setup failure",
			RetryAfter: 15 * time.Millisecond,
		},
		firstStarted: make(chan struct{}),
		releaseFirst: releaseFirst,
		streams: []hermes.Stream{&interruptRetryStream{events: []hermes.Event{
			{Kind: hermes.EventToken, Token: "unexpected"},
			{Kind: hermes.EventDone, FinishReason: "stop"},
		}}},
	}
	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, st, telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)

	initial := <-k.Render()
	if initial.Phase != PhaseIdle {
		t.Fatalf("initial phase = %v, want Idle", initial.Phase)
	}
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "stop before reconnect"}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-client.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first OpenStream attempt did not start")
	}
	if err := k.Submit(PlatformEvent{Kind: PlatformEventCancel}); err != nil {
		t.Fatal(err)
	}
	close(releaseFirst)

	final := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.Seq > initial.Seq
	}, time.Second)

	if got := client.Calls(); got != 1 {
		t.Fatalf("OpenStream calls = %d, want 1; cancel before retry must suppress fresh provider stream", got)
	}
	if len(final.History) != 1 || final.History[0].Role != "user" {
		t.Fatalf("history after interrupted retry = %#v, want only original user turn", final.History)
	}
	st.requireSingleSkip(t, "interrupted")
	st.requireNoAssistantFinalize(t)
}

func TestKernel_StreamInterruptSuppressesRetryWhenParentContextCancelsDuringBackoff(t *testing.T) {
	st := newInterruptRetryStore()
	client := &interruptRetryClient{
		firstErr: &hermes.HTTPError{
			Status:     http.StatusServiceUnavailable,
			Body:       "transient stream setup failure",
			RetryAfter: time.Hour,
		},
		streams: []hermes.Stream{&interruptRetryStream{events: []hermes.Event{
			{Kind: hermes.EventToken, Token: "unexpected"},
			{Kind: hermes.EventDone, FinishReason: "stop"},
		}}},
	}
	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, st, telemetry.New(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	go k.Run(ctx)

	initial := <-k.Render()
	if initial.Phase != PhaseIdle {
		t.Fatalf("initial phase = %v, want Idle", initial.Phase)
	}
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "parent cancel"}); err != nil {
		t.Fatal(err)
	}
	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseReconnecting
	}, time.Second)

	cancel()
	waitForRenderClosed(t, k.Render(), time.Second)

	if got := client.Calls(); got != 1 {
		t.Fatalf("OpenStream calls = %d, want 1; context cancellation during backoff must suppress retry", got)
	}
	st.requireSingleSkip(t, "interrupted")
}

func TestKernel_StreamInterruptSuppressesRetryNoCancelControlRecovers(t *testing.T) {
	st := newInterruptRetryStore()
	client := &interruptRetryClient{
		firstErr: &hermes.HTTPError{
			Status:     http.StatusServiceUnavailable,
			Body:       "transient stream setup failure",
			RetryAfter: time.Millisecond,
		},
		streams: []hermes.Stream{&interruptRetryStream{events: []hermes.Event{
			{Kind: hermes.EventToken, Token: "ok"},
			{Kind: hermes.EventDone, FinishReason: "stop"},
		}}},
	}
	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, client, st, telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go k.Run(ctx)

	initial := <-k.Render()
	if initial.Phase != PhaseIdle {
		t.Fatalf("initial phase = %v, want Idle", initial.Phase)
	}
	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "recover"}); err != nil {
		t.Fatal(err)
	}

	final := waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.Seq > initial.Seq
	}, time.Second)

	if got := client.Calls(); got != 2 {
		t.Fatalf("OpenStream calls = %d, want 2 for no-cancel transient recovery", got)
	}
	if len(final.History) == 0 || final.History[len(final.History)-1].Content != "ok" {
		t.Fatalf("final history = %#v, want assistant content ok", final.History)
	}
	if skips := st.Skips(); len(skips) != 0 {
		t.Fatalf("memory sync skips = %#v, want none for successful retry", skips)
	}
}

type interruptRetryClient struct {
	firstErr     error
	firstStarted chan struct{}
	releaseFirst <-chan struct{}

	mu               sync.Mutex
	calls            int
	streams          []hermes.Stream
	firstStartedOnce sync.Once
}

func (c *interruptRetryClient) OpenStream(ctx context.Context, _ hermes.ChatRequest) (hermes.Stream, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()

	if call == 1 {
		if c.firstStarted != nil {
			c.firstStartedOnce.Do(func() { close(c.firstStarted) })
		}
		if c.releaseFirst != nil {
			select {
			case <-c.releaseFirst:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return nil, c.firstErr
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.streams) == 0 {
		return &interruptRetryStream{}, nil
	}
	s := c.streams[0]
	c.streams = c.streams[1:]
	return s, nil
}

func (c *interruptRetryClient) OpenRunEvents(context.Context, string) (hermes.RunEventStream, error) {
	return nil, hermes.ErrRunEventsNotSupported
}

func (c *interruptRetryClient) Health(context.Context) error { return nil }

func (c *interruptRetryClient) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type interruptRetryStream struct {
	events []hermes.Event
	pos    int
}

func (s *interruptRetryStream) Recv(context.Context) (hermes.Event, error) {
	if s.pos >= len(s.events) {
		return hermes.Event{}, io.EOF
	}
	ev := s.events[s.pos]
	s.pos++
	return ev, nil
}

func (s *interruptRetryStream) SessionID() string { return "" }
func (s *interruptRetryStream) Close() error      { return nil }

type interruptRetryStore struct {
	*store.RecordingStore

	mu    sync.Mutex
	skips []memorySkipCall
}

type memorySkipCall struct {
	turnKey string
	reason  string
}

func newInterruptRetryStore() *interruptRetryStore {
	return &interruptRetryStore{RecordingStore: store.NewRecording()}
}

func (s *interruptRetryStore) SkipMemorySync(ctx context.Context, turnKey, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.skips = append(s.skips, memorySkipCall{turnKey: turnKey, reason: reason})
	return nil
}

func (s *interruptRetryStore) Skips() []memorySkipCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]memorySkipCall, len(s.skips))
	copy(out, s.skips)
	return out
}

func (s *interruptRetryStore) requireSingleSkip(t *testing.T, reason string) {
	t.Helper()
	skips := s.Skips()
	if len(skips) != 1 {
		t.Fatalf("memory sync skips = %#v, want one skip", skips)
	}
	if skips[0].turnKey == "" {
		t.Fatal("memory sync skip turnKey is empty")
	}
	if skips[0].reason != reason {
		t.Fatalf("memory sync skip reason = %q, want %q", skips[0].reason, reason)
	}
}

func (s *interruptRetryStore) requireNoAssistantFinalize(t *testing.T) {
	t.Helper()
	for _, cmd := range s.Commands() {
		if cmd.Kind != store.FinalizeAssistantTurn {
			continue
		}
		var payload struct {
			Content string `json:"content"`
		}
		if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
			t.Fatalf("FinalizeAssistantTurn payload: %v", err)
		}
		if payload.Content != "" {
			t.Fatalf("unexpected assistant finalize content %q after interrupted retry", payload.Content)
		}
	}
}

func waitForRenderClosed(t *testing.T, ch <-chan RenderFrame, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("timeout waiting for render channel to close")
		}
	}
}
