package tuigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

// TestDialSSE_ConsumesFixtureFrames opens an SSE stream where the server
// emits two RenderFrame fixtures back-to-back and asserts the client
// surfaces them on its Frames() channel in order. Mirrors upstream
// hermes-agent/tui_gateway/server.py's gateway.ready + per-turn frame
// emission, but over the native Go SSE transport: the JSON payload is the
// kernel.RenderFrame snapshot rather than a JSON-RPC envelope, since the
// remote TUI consumes RenderFrames directly.
func TestDialSSE_ConsumesFixtureFrames(t *testing.T) {
	t.Parallel()

	f1 := kernel.RenderFrame{Phase: kernel.PhaseStreaming, Seq: 1, DraftText: "hello"}
	f2 := kernel.RenderFrame{Phase: kernel.PhaseIdle, Seq: 2}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("server: ResponseWriter is not a Flusher")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, f := range []kernel.RenderFrame{f1, f2} {
			data, err := json.Marshal(f)
			if err != nil {
				t.Errorf("server: marshal frame: %v", err)
				return
			}
			fmt.Fprintf(w, "event: frame\ndata: %s\n\n", data)
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := DialSSE(ctx, server.URL)
	if err != nil {
		t.Fatalf("DialSSE: %v", err)
	}
	defer client.Close()

	got := make([]kernel.RenderFrame, 0, 2)
	deadline := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case f, ok := <-client.Frames():
			if !ok {
				t.Fatalf("Frames channel closed early; got %d frames", len(got))
			}
			got = append(got, f)
		case <-deadline:
			t.Fatalf("timed out waiting for frames; got %d", len(got))
		}
	}
	if got[0].Seq != 1 || got[0].DraftText != "hello" || got[0].Phase != kernel.PhaseStreaming {
		t.Errorf("first frame = %+v; want seq=1 draft=hello phase=Streaming", got[0])
	}
	if got[1].Seq != 2 || got[1].Phase != kernel.PhaseIdle {
		t.Errorf("second frame = %+v; want seq=2 phase=Idle", got[1])
	}
}

// TestDialSSE_ReconnectsAfterTransportEOF: the first GET returns 200 and
// then EOFs the body; the client must reconnect and consume a frame from
// the second GET. Reconnects() reports at least one reconnect attempt.
func TestDialSSE_ReconnectsAfterTransportEOF(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("server: ResponseWriter is not a Flusher")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
		if n == 1 {
			// EOF the body to force reconnect.
			return
		}
		f := kernel.RenderFrame{Seq: 42, DraftText: "after-reconnect"}
		data, _ := json.Marshal(f)
		fmt.Fprintf(w, "event: frame\ndata: %s\n\n", data)
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := DialSSE(ctx, server.URL,
		WithReconnectBackoff(5*time.Millisecond, 50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("DialSSE: %v", err)
	}
	defer client.Close()

	select {
	case f, ok := <-client.Frames():
		if !ok {
			t.Fatalf("Frames channel closed before reconnect frame arrived")
		}
		if f.Seq != 42 || f.DraftText != "after-reconnect" {
			t.Errorf("post-reconnect frame = %+v; want seq=42 draft=after-reconnect", f)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for post-reconnect frame; hits=%d", hits.Load())
	}
	if r := client.Reconnects(); r < 1 {
		t.Errorf("Reconnects() = %d; want >= 1", r)
	}
}

// TestDialSSE_ContextCancellationCloses: cancelling the parent context
// terminates the run loop and closes the Frames channel, unblocking the
// consumer. This is the cleanup contract the TUI relies on at shutdown.
func TestDialSSE_ContextCancellationCloses(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client, err := DialSSE(ctx, server.URL)
	if err != nil {
		cancel()
		t.Fatalf("DialSSE: %v", err)
	}

	cancel()

	// Drain Frames; expect the channel to close within the deadline.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-client.Frames():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatalf("Frames channel did not close after ctx cancellation")
		}
	}
}

// TestDialSSE_RejectsNon200Status: a 500 response on the first GET must
// surface as an error from DialSSE itself; the caller has to know remote
// streaming is unavailable rather than silently retrying forever.
func TestDialSSE_RejectsNon200Status(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := DialSSE(ctx, server.URL)
	if err == nil {
		client.Close()
		t.Fatalf("DialSSE returned nil error on 500; want non-nil")
	}
}

// TestRemoteClient_PostPlatformEventSubmit proves PostPlatformEvent POSTs
// the SubmitEvent JSON to /platform-event and the gateway sees the same
// shape. The wire encoding matches the SubmitEvent struct's JSON tags so
// the GatewayMux on the other end can decode it without an envelope.
func TestRemoteClient_PostPlatformEventSubmit(t *testing.T) {
	t.Parallel()

	var got SubmitEvent
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/events":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		case r.Method == http.MethodPost && r.URL.Path == "/platform-event":
			calls.Add(1)
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &got); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"queued"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := DialSSE(ctx, server.URL, WithSessionID("sid-77"))
	if err != nil {
		t.Fatalf("DialSSE: %v", err)
	}
	defer client.Close()

	if err := client.PostPlatformEvent(ctx, SubmitEvent{
		Kind:      PlatformEventKindSubmit,
		SessionID: "sid-77",
		Text:      "hi",
	}); err != nil {
		t.Fatalf("PostPlatformEvent: %v", err)
	}
	deadline := time.After(2 * time.Second)
	for calls.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("submit POST never reached the server")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if got.Kind != PlatformEventKindSubmit || got.SessionID != "sid-77" || got.Text != "hi" {
		t.Errorf("server saw %+v; want kind=submit sid=sid-77 text=hi", got)
	}
}

// TestSSEClient_SubmitCancelResizeConvenienceHelpers exercises the
// session-scoped helpers: Submit/Cancel/Resize fold the WithSessionID()
// value into the wire payload so callers do not duplicate the id on every
// call. Each helper hits its dedicated endpoint instead of the catch-all
// /platform-event so the GatewayMux can dispatch with simple matchers.
func TestSSEClient_SubmitCancelResizeConvenienceHelpers(t *testing.T) {
	t.Parallel()

	var submitCalls, cancelCalls, resizeCalls atomic.Int32
	var lastSubmit SubmitEvent
	var lastCancel CancelEvent
	var lastResize ResizeEvent

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/events":
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		case r.Method == http.MethodPost && r.URL.Path == "/submit":
			submitCalls.Add(1)
			_ = json.Unmarshal(body, &lastSubmit)
		case r.Method == http.MethodPost && r.URL.Path == "/cancel":
			cancelCalls.Add(1)
			_ = json.Unmarshal(body, &lastCancel)
		case r.Method == http.MethodPost && r.URL.Path == "/resize":
			resizeCalls.Add(1)
			_ = json.Unmarshal(body, &lastResize)
		default:
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := DialSSE(ctx, server.URL, WithSessionID("sid-9"))
	if err != nil {
		t.Fatalf("DialSSE: %v", err)
	}
	defer client.Close()

	if err := client.Submit(ctx, "hello"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := client.Cancel(ctx); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := client.Resize(ctx, 132); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for submitCalls.Load() < 1 || cancelCalls.Load() < 1 || resizeCalls.Load() < 1 {
		select {
		case <-deadline:
			t.Fatalf("dispatch counts: submit=%d cancel=%d resize=%d",
				submitCalls.Load(), cancelCalls.Load(), resizeCalls.Load())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if lastSubmit.SessionID != "sid-9" || lastSubmit.Text != "hello" || lastSubmit.Kind != PlatformEventKindSubmit {
		t.Errorf("submit body = %+v; want kind=submit sid=sid-9 text=hello", lastSubmit)
	}
	if lastCancel.SessionID != "sid-9" || lastCancel.Kind != PlatformEventKindCancel {
		t.Errorf("cancel body = %+v; want kind=cancel sid=sid-9", lastCancel)
	}
	if lastResize.SessionID != "sid-9" || lastResize.Cols != 132 || lastResize.Kind != PlatformEventKindResize {
		t.Errorf("resize body = %+v; want kind=resize sid=sid-9 cols=132", lastResize)
	}
}
