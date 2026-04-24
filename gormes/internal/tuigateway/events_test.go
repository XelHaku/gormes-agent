package tuigateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

// TestEventHandler_ForwardsSubmit is the round-trip proof that a remote
// Bubble Tea client can reach the kernel: PostPlatformEvent serialises a
// Submit event, NewEventHandler decodes it, and the injected sink sees a
// kernel.PlatformEvent that preserves Kind, Text, SessionID, and CronJobID.
func TestEventHandler_ForwardsSubmit(t *testing.T) {
	t.Parallel()

	got := make(chan kernel.PlatformEvent, 1)
	sink := func(e kernel.PlatformEvent) error {
		got <- e
		return nil
	}

	srv := httptest.NewServer(NewEventHandler(sink))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	want := kernel.PlatformEvent{
		Kind:      kernel.PlatformEventSubmit,
		Text:      "hello gateway",
		SessionID: "sess-xyz",
		CronJobID: "cron-123",
	}
	if err := PostPlatformEvent(ctx, srv.URL, want); err != nil {
		t.Fatalf("PostPlatformEvent: %v", err)
	}
	select {
	case e := <-got:
		if e.Kind != want.Kind {
			t.Errorf("Kind = %v, want %v", e.Kind, want.Kind)
		}
		if e.Text != want.Text {
			t.Errorf("Text = %q, want %q", e.Text, want.Text)
		}
		if e.SessionID != want.SessionID {
			t.Errorf("SessionID = %q, want %q", e.SessionID, want.SessionID)
		}
		if e.CronJobID != want.CronJobID {
			t.Errorf("CronJobID = %q, want %q", e.CronJobID, want.CronJobID)
		}
	case <-time.After(time.Second):
		t.Fatal("sink did not receive event")
	}
}

// TestEventHandler_ForwardsCancel proves the Cancel kind survives the wire
// mapping. Cancel carries no payload but must still reach the sink with the
// correct PlatformEventKind so the kernel aborts an in-flight turn.
func TestEventHandler_ForwardsCancel(t *testing.T) {
	t.Parallel()

	got := make(chan kernel.PlatformEvent, 1)
	sink := func(e kernel.PlatformEvent) error {
		got <- e
		return nil
	}
	srv := httptest.NewServer(NewEventHandler(sink))
	defer srv.Close()

	if err := PostPlatformEvent(context.Background(), srv.URL,
		kernel.PlatformEvent{Kind: kernel.PlatformEventCancel}); err != nil {
		t.Fatalf("PostPlatformEvent: %v", err)
	}
	select {
	case e := <-got:
		if e.Kind != kernel.PlatformEventCancel {
			t.Fatalf("Kind = %v, want PlatformEventCancel", e.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("sink did not receive cancel event")
	}
}

// TestEventHandler_ForwardsReset proves PlatformEventResetSession round-trips.
// Reset is the third mode a Bubble Tea client needs so "new chat" works
// against a remote kernel.
func TestEventHandler_ForwardsReset(t *testing.T) {
	t.Parallel()

	got := make(chan kernel.PlatformEvent, 1)
	sink := func(e kernel.PlatformEvent) error {
		got <- e
		return nil
	}
	srv := httptest.NewServer(NewEventHandler(sink))
	defer srv.Close()

	if err := PostPlatformEvent(context.Background(), srv.URL,
		kernel.PlatformEvent{Kind: kernel.PlatformEventResetSession}); err != nil {
		t.Fatalf("PostPlatformEvent: %v", err)
	}
	select {
	case e := <-got:
		if e.Kind != kernel.PlatformEventResetSession {
			t.Fatalf("Kind = %v, want PlatformEventResetSession", e.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("sink did not receive reset event")
	}
}

// TestEventHandler_RejectsUnknownKind proves a bad `kind` string returns 400
// and the sink is NOT invoked. Keeping the handler defensive is important
// because the upstream surface is public-internet-facing once the gateway
// is deployed.
func TestEventHandler_RejectsUnknownKind(t *testing.T) {
	t.Parallel()

	var invocations int32
	sink := func(kernel.PlatformEvent) error {
		atomic.AddInt32(&invocations, 1)
		return nil
	}
	h := NewEventHandler(sink)

	req := httptest.NewRequest(http.MethodPost, "/events",
		strings.NewReader(`{"kind":"bogus"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if got := atomic.LoadInt32(&invocations); got != 0 {
		t.Fatalf("sink invoked %d times; expected 0 for bad kind", got)
	}
}

// TestEventHandler_RejectsNonPost proves only POST is allowed. A GET (or any
// other verb) should return 405 without touching the sink — this keeps the
// method surface small and the handler easy to mount behind a router that
// might try to probe /events with OPTIONS or HEAD.
func TestEventHandler_RejectsNonPost(t *testing.T) {
	t.Parallel()

	var invocations int32
	sink := func(kernel.PlatformEvent) error {
		atomic.AddInt32(&invocations, 1)
		return nil
	}
	h := NewEventHandler(sink)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := atomic.LoadInt32(&invocations); got != 0 {
		t.Fatalf("sink invoked %d times; expected 0 for GET", got)
	}
}

// TestEventHandler_SinkErrorSurfaces proves that a sink error (e.g., the
// kernel's ErrEventMailboxFull) is reported to the client with a non-2xx
// status so retry logic can kick in — silently swallowing backpressure
// would desync the remote TUI from the kernel.
func TestEventHandler_SinkErrorSurfaces(t *testing.T) {
	t.Parallel()

	sink := func(kernel.PlatformEvent) error { return kernel.ErrEventMailboxFull }
	srv := httptest.NewServer(NewEventHandler(sink))
	defer srv.Close()

	err := PostPlatformEvent(context.Background(), srv.URL,
		kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: "x"})
	if err == nil {
		t.Fatal("PostPlatformEvent: want non-nil error on sink backpressure")
	}
}

// TestPostPlatformEvent_RejectsQuit proves Quit is not a wire-transmittable
// event kind — the documented disconnect signal is the SSE request-context
// cancel. The client must reject Quit locally rather than hit the network.
func TestPostPlatformEvent_RejectsQuit(t *testing.T) {
	t.Parallel()

	// Unreachable URL on purpose: the client must return an error before
	// attempting any network I/O.
	err := PostPlatformEvent(context.Background(), "http://127.0.0.1:1",
		kernel.PlatformEvent{Kind: kernel.PlatformEventQuit})
	if err == nil {
		t.Fatal("PostPlatformEvent(Quit): want non-nil error, got nil")
	}
	if !strings.Contains(err.Error(), "quit") && !strings.Contains(err.Error(), "Quit") {
		t.Fatalf("error %q should mention quit", err.Error())
	}
}
