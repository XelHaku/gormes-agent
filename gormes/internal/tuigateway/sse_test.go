package tuigateway

import (
	"context"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

// TestSSEHandler_EmitsEventStreamHeaders proves the handler advertises an
// SSE response so a browser/Bubble Tea client knows to switch to the
// streaming reader (rather than waiting for a buffered response).
func TestSSEHandler_EmitsEventStreamHeaders(t *testing.T) {
	frames := make(chan kernel.RenderFrame, 1)
	close(frames) // immediately closed: handler should write headers + exit

	h := NewSSEHandler(frames)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/events", nil)

	h.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if got := res.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := res.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}
}

// TestSSEHandler_StreamsFramesEndToEnd writes a couple of frames into the
// kernel-side channel, runs ConsumeSSE against the handler, and asserts
// the same frames pop out the consumer side in order.
func TestSSEHandler_StreamsFramesEndToEnd(t *testing.T) {
	frames := make(chan kernel.RenderFrame, 2)
	frames <- kernel.RenderFrame{
		Seq:       1,
		Phase:     kernel.PhaseStreaming,
		DraftText: "hello",
		Model:     "hermes-agent",
		SessionID: "sess-abc",
		History: []hermes.Message{
			{Role: "user", Content: "hi"},
		},
		Telemetry: telemetry.Snapshot{
			Model:        "hermes-agent",
			TokensPerSec: 12.5,
		},
	}
	frames <- kernel.RenderFrame{
		Seq:   2,
		Phase: kernel.PhaseIdle,
		Model: "hermes-agent",
	}
	close(frames)

	srv := httptest.NewServer(NewSSEHandler(frames))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := DialSSE(ctx, srv.URL)
	if err != nil {
		t.Fatalf("DialSSE: %v", err)
	}

	got := drainFrames(t, out, 2, time.Second)
	if len(got) != 2 {
		t.Fatalf("got %d frames, want 2: %#v", len(got), got)
	}
	if got[0].Seq != 1 || got[0].DraftText != "hello" || got[0].Model != "hermes-agent" {
		t.Errorf("frame0 = %+v", got[0])
	}
	if !reflect.DeepEqual(got[0].History, []hermes.Message{{Role: "user", Content: "hi"}}) {
		t.Errorf("frame0.History = %#v", got[0].History)
	}
	if got[1].Seq != 2 || got[1].Phase != kernel.PhaseIdle {
		t.Errorf("frame1 = %+v", got[1])
	}
}

// TestSSEHandler_StopsOnContextCancel proves the handler exits its inner
// loop when the request context is cancelled — i.e. a disconnecting client
// does NOT leak the kernel-side frame consumer goroutine.
func TestSSEHandler_StopsOnContextCancel(t *testing.T) {
	frames := make(chan kernel.RenderFrame, 1)
	frames <- kernel.RenderFrame{Seq: 1, Phase: kernel.PhaseIdle}

	srv := httptest.NewServer(NewSSEHandler(frames))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	out, err := DialSSE(ctx, srv.URL)
	if err != nil {
		t.Fatalf("DialSSE: %v", err)
	}

	// Drain the first frame so we know the stream is live, then cancel.
	select {
	case <-out:
	case <-time.After(time.Second):
		t.Fatal("first frame did not arrive")
	}
	cancel()

	// After cancel, the consumer channel must close within a generous
	// window. A leak would manifest as a timer expiry here.
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case _, ok := <-out:
			if !ok {
				return // closed — pass
			}
			// Drain any in-flight frames; loop until close.
		case <-deadline.C:
			t.Fatal("consumer channel did not close after request context cancel")
		}
	}
}

// TestConsumeSSE_DecodesRawStream pinpoints the wire format by feeding a
// hand-rolled SSE stream into the consumer. This isolates the parser from
// the handler so a future format change can't quietly drift.
func TestConsumeSSE_DecodesRawStream(t *testing.T) {
	body := strings.Join([]string{
		`event: frame`,
		`data: {"Seq":1,"Phase":2,"DraftText":"d1"}`,
		``,
		`: keepalive comment is ignored`,
		``,
		`event: frame`,
		`data: {"Seq":2,"Phase":0}`,
		``,
		`event: end`,
		`data: {}`,
		``,
		``,
	}, "\n")

	out := ConsumeSSE(context.Background(), strings.NewReader(body))
	got := drainFrames(t, out, 2, time.Second)
	if len(got) != 2 || got[0].Seq != 1 || got[0].DraftText != "d1" || got[1].Seq != 2 {
		t.Fatalf("decoded = %#v", got)
	}
}

func drainFrames(t *testing.T, out <-chan kernel.RenderFrame, n int, perFrameTimeout time.Duration) []kernel.RenderFrame {
	t.Helper()
	var frames []kernel.RenderFrame
	for i := 0; i < n; i++ {
		select {
		case f, ok := <-out:
			if !ok {
				return frames
			}
			frames = append(frames, f)
		case <-time.After(perFrameTimeout):
			t.Fatalf("timeout waiting for frame %d (got %d)", i, len(frames))
		}
	}
	return frames
}
