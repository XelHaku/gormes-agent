package tuigateway

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

// fakeKernelHandle implements KernelHandle for the GatewayMux tests. It
// records every PlatformEvent it receives via Submit and exposes a render
// channel writers can push frames onto so the SSE stream test sees them.
type fakeKernelHandle struct {
	mu      sync.Mutex
	events  []kernel.PlatformEvent
	frames  chan kernel.RenderFrame
	submitErr error
}

func newFakeKernelHandle() *fakeKernelHandle {
	return &fakeKernelHandle{frames: make(chan kernel.RenderFrame, 8)}
}

func (k *fakeKernelHandle) Submit(e kernel.PlatformEvent) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.submitErr != nil {
		return k.submitErr
	}
	k.events = append(k.events, e)
	return nil
}

func (k *fakeKernelHandle) Render() <-chan kernel.RenderFrame { return k.frames }

func (k *fakeKernelHandle) Events() []kernel.PlatformEvent {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]kernel.PlatformEvent, len(k.events))
	copy(out, k.events)
	return out
}

// TestGatewayMux_SubmitDispatchesPlatformEvent verifies POST /submit decodes
// the JSON body into a kernel.PlatformEventSubmit with the supplied
// session_id and text. The kernel handle is the only state mutated by the
// dispatch path; the response is a JSON status envelope.
func TestGatewayMux_SubmitDispatchesPlatformEvent(t *testing.T) {
	t.Parallel()

	handle := newFakeKernelHandle()
	mux := NewGatewayMux(handle)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := `{"kind":"submit","session_id":"sid-1","text":"hello"}`
	resp, err := http.Post(server.URL+"/submit", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /submit: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}

	events := handle.Events()
	if len(events) != 1 {
		t.Fatalf("submit events = %d; want 1", len(events))
	}
	if events[0].Kind != kernel.PlatformEventSubmit {
		t.Errorf("event kind = %v; want PlatformEventSubmit", events[0].Kind)
	}
	if events[0].SessionID != "sid-1" || events[0].Text != "hello" {
		t.Errorf("event = %+v; want sid=sid-1 text=hello", events[0])
	}
}

// TestGatewayMux_CancelDispatchesPlatformEvent: POST /cancel forwards a
// PlatformEventCancel without text/payload.
func TestGatewayMux_CancelDispatchesPlatformEvent(t *testing.T) {
	t.Parallel()

	handle := newFakeKernelHandle()
	mux := NewGatewayMux(handle)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := `{"kind":"cancel","session_id":"sid-2"}`
	resp, err := http.Post(server.URL+"/cancel", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /cancel: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	events := handle.Events()
	if len(events) != 1 {
		t.Fatalf("cancel events = %d; want 1", len(events))
	}
	if events[0].Kind != kernel.PlatformEventCancel {
		t.Errorf("event kind = %v; want PlatformEventCancel", events[0].Kind)
	}
}

// TestGatewayMux_ResizeAcceptsCols: POST /resize returns 200 even though
// the kernel does not have a resize event (Bubble Tea reads cols locally).
// The handler must accept and acknowledge the payload so the remote TUI
// can complete its terminal.resize JSON-RPC analogue without errors.
func TestGatewayMux_ResizeAcceptsCols(t *testing.T) {
	t.Parallel()

	handle := newFakeKernelHandle()
	mux := NewGatewayMux(handle)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := `{"kind":"resize","session_id":"sid-3","cols":132}`
	resp, err := http.Post(server.URL+"/resize", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /resize: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if got := mux.LastResizeCols("sid-3"); got != 132 {
		t.Errorf("LastResizeCols(sid-3) = %d; want 132", got)
	}
}

// TestGatewayMux_PlatformEventGenericDispatch: the unified
// /platform-event endpoint dispatches based on the kind discriminator.
// Posting {kind:"submit"} routes through the same path as /submit.
func TestGatewayMux_PlatformEventGenericDispatch(t *testing.T) {
	t.Parallel()

	handle := newFakeKernelHandle()
	mux := NewGatewayMux(handle)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := `{"kind":"submit","session_id":"sid-4","text":"generic"}`
	resp, err := http.Post(server.URL+"/platform-event", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /platform-event: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	events := handle.Events()
	if len(events) != 1 {
		t.Fatalf("platform-event submit count = %d; want 1", len(events))
	}
	if events[0].Kind != kernel.PlatformEventSubmit || events[0].Text != "generic" || events[0].SessionID != "sid-4" {
		t.Errorf("event = %+v; want submit text=generic sid=sid-4", events[0])
	}
}

// TestGatewayMux_RejectsUnknownPlatformEventKind: an unrecognised kind
// returns 400 with a JSON-RPC-shaped error envelope (code/message keys)
// instead of silently no-oping. The kernel handle must NOT receive any
// event for an unknown kind.
func TestGatewayMux_RejectsUnknownPlatformEventKind(t *testing.T) {
	t.Parallel()

	handle := newFakeKernelHandle()
	mux := NewGatewayMux(handle)
	server := httptest.NewServer(mux)
	defer server.Close()

	body := `{"kind":"shutdown","session_id":"sid-5"}`
	resp, err := http.Post(server.URL+"/platform-event", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /platform-event: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", resp.StatusCode)
	}
	var env map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env["code"] == nil || env["message"] == nil {
		t.Errorf("error envelope = %+v; want code+message keys", env)
	}
	if got := handle.Events(); len(got) != 0 {
		t.Errorf("kernel events on unknown kind = %d; want 0", len(got))
	}
}

// TestGatewayMux_StreamsRenderFramesOverSSE: the GET /events endpoint
// streams kernel.RenderFrames as SSE frames so DialSSE on the other end
// can decode them. The fixture pushes one frame onto the kernel channel
// and asserts the body contains a JSON-encoded frame envelope.
func TestGatewayMux_StreamsRenderFramesOverSSE(t *testing.T) {
	t.Parallel()

	handle := newFakeKernelHandle()
	mux := NewGatewayMux(handle)
	server := httptest.NewServer(mux)
	defer server.Close()

	frame := kernel.RenderFrame{Phase: kernel.PhaseStreaming, Seq: 11, DraftText: "stream-me"}
	go func() {
		// Slight delay to ensure the server is ready before we push.
		time.Sleep(20 * time.Millisecond)
		handle.frames <- frame
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q; want text/event-stream", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var data, event string
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out reading SSE; data=%q event=%q", data, event)
		default:
		}
		if !scanner.Scan() {
			t.Fatalf("scanner returned EOF before frame; err=%v", scanner.Err())
		}
		line := scanner.Text()
		if line == "" {
			if event != "" || data != "" {
				break
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if data != "" {
				data += "\n"
			}
			data += strings.TrimPrefix(line, "data: ")
		}
	}
	if event != "frame" {
		t.Errorf("event = %q; want frame", event)
	}
	var got kernel.RenderFrame
	if err := json.Unmarshal([]byte(data), &got); err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	if got.Seq != 11 || got.DraftText != "stream-me" || got.Phase != kernel.PhaseStreaming {
		t.Errorf("frame = %+v; want seq=11 draft=stream-me phase=Streaming", got)
	}
}

// TestGatewayMux_RejectsNonJSONBody: malformed JSON on a dispatch endpoint
// returns 400 with a JSON-RPC parse-error envelope. No kernel events get
// emitted on a parse failure.
func TestGatewayMux_RejectsNonJSONBody(t *testing.T) {
	t.Parallel()

	handle := newFakeKernelHandle()
	mux := NewGatewayMux(handle)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Post(server.URL+"/submit", "application/json", strings.NewReader("not-json"))
	if err != nil {
		t.Fatalf("POST /submit: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", resp.StatusCode)
	}
	if got := handle.Events(); len(got) != 0 {
		t.Errorf("kernel events on parse error = %d; want 0", len(got))
	}
}
