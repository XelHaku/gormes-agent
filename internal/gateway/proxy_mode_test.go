package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

type capturedProxyRequest struct {
	Path          string
	Authorization string
	SessionID     string
	Messages      []map[string]any
	Stream        bool
}

func TestProxySubmitter_ForwardsSessionHeaderAndFiltersUnsafeHistory(t *testing.T) {
	requests := make(chan capturedProxyRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]any `json:"messages"`
			Stream   bool             `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		requests <- capturedProxyRequest{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			SessionID:     r.Header.Get("X-Hermes-Session-Id"),
			Messages:      body.Messages,
			Stream:        body.Stream,
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Hermes-Session-Id", r.Header.Get("X-Hermes-Session-Id"))
		fmt.Fprint(w, `data: {"choices":[{"delta":{"role":"assistant"}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"Hello"}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":" world"}}]}`+"\n\n")
		fmt.Fprint(w, `data: {"choices":[{"finish_reason":"stop","delta":{}}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	proxy, err := NewProxySubmitter(ProxySubmitterConfig{
		BaseURL: srv.URL + "/",
		APIKey:  "secret-key",
		Model:   "gormes-agent",
		History: []hermes.Message{
			{Role: "user", Content: "previous user"},
			{Role: "assistant", ToolCalls: []hermes.ToolCall{{ID: "call_1", Name: "search"}}},
			{Role: "tool", Content: "tool result", ToolCallID: "call_1", Name: "search"},
			{Role: "assistant", Content: "  "},
			{Role: "assistant", Content: "previous assistant", ToolCalls: []hermes.ToolCall{{ID: "call_2", Name: "ignored"}}},
		},
	})
	if err != nil {
		t.Fatalf("NewProxySubmitter: %v", err)
	}

	err = proxy.Submit(kernel.PlatformEvent{
		Kind:           kernel.PlatformEventSubmit,
		Text:           "tell me more",
		SessionID:      "sess-abc",
		SessionContext: "## Current Session Context\nplatform: matrix",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	var req capturedProxyRequest
	select {
	case req = <-requests:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("proxy request not received")
	}
	if req.Path != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions", req.Path)
	}
	if req.Authorization != "Bearer secret-key" {
		t.Fatalf("Authorization = %q, want bearer key", req.Authorization)
	}
	if req.SessionID != "sess-abc" {
		t.Fatalf("X-Hermes-Session-Id = %q, want sess-abc", req.SessionID)
	}
	if !req.Stream {
		t.Fatal("stream = false, want true")
	}

	want := []map[string]string{
		{"role": "system", "content": "## Current Session Context\nplatform: matrix"},
		{"role": "user", "content": "previous user"},
		{"role": "assistant", "content": "previous assistant"},
		{"role": "user", "content": "tell me more"},
	}
	if len(req.Messages) != len(want) {
		t.Fatalf("messages len = %d, want %d: %#v", len(req.Messages), len(want), req.Messages)
	}
	for i, msg := range req.Messages {
		if msg["role"] != want[i]["role"] || msg["content"] != want[i]["content"] {
			t.Fatalf("messages[%d] = %#v, want role/content %#v", i, msg, want[i])
		}
		if _, ok := msg["tool_calls"]; ok {
			t.Fatalf("messages[%d] forwarded tool_calls: %#v", i, msg)
		}
		if _, ok := msg["tool_call_id"]; ok {
			t.Fatalf("messages[%d] forwarded tool_call_id: %#v", i, msg)
		}
	}

	final := readProxyTerminalFrame(t, proxy.Render())
	if final.Phase != kernel.PhaseIdle {
		t.Fatalf("terminal phase = %v, want idle", final.Phase)
	}
	if final.SessionID != "sess-abc" {
		t.Fatalf("terminal SessionID = %q, want sess-abc", final.SessionID)
	}
	if got := final.History[len(final.History)-1].Content; got != "Hello world" {
		t.Fatalf("final assistant = %q, want streamed content", got)
	}
}

func TestProxySubmitter_StaleGenerationReportsDegradedOutput(t *testing.T) {
	requestSeen := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestSeen)
		<-release
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Hermes-Session-Id", r.Header.Get("X-Hermes-Session-Id"))
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"stale answer"}}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	store := NewRuntimeStatusStore(t.TempDir() + "/gateway_state.json")
	proxy, err := NewProxySubmitter(ProxySubmitterConfig{
		BaseURL:       srv.URL,
		Model:         "gormes-agent",
		RuntimeStatus: store,
	})
	if err != nil {
		t.Fatalf("NewProxySubmitter: %v", err)
	}
	if err := proxy.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: "hi", SessionID: "sess-stale"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-requestSeen:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("proxy request not received")
	}
	if err := proxy.ResetSession(); err != nil {
		t.Fatalf("ResetSession: %v", err)
	}
	close(release)

	final := readProxyTerminalFrame(t, proxy.Render())
	if final.Phase != kernel.PhaseFailed {
		t.Fatalf("terminal phase = %v, want failed", final.Phase)
	}
	if !strings.Contains(final.LastError, "stale generation") {
		t.Fatalf("LastError = %q, want stale generation degradation", final.LastError)
	}
	for _, msg := range final.History {
		if strings.Contains(msg.Content, "stale answer") {
			t.Fatalf("stale remote content was accepted into history: %#v", final.History)
		}
	}
	assertProxyStatus(t, store, "degraded", "stale generation")
}

func TestProxySubmitter_RemoteErrorsReturnVisibleDegradedOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized: invalid API key", http.StatusUnauthorized)
	}))
	defer srv.Close()

	store := NewRuntimeStatusStore(t.TempDir() + "/gateway_state.json")
	proxy, err := NewProxySubmitter(ProxySubmitterConfig{
		BaseURL:       srv.URL,
		Model:         "gormes-agent",
		RuntimeStatus: store,
	})
	if err != nil {
		t.Fatalf("NewProxySubmitter: %v", err)
	}
	if err := proxy.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: "hi", SessionID: "sess-auth"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	final := readProxyTerminalFrame(t, proxy.Render())
	if final.Phase != kernel.PhaseFailed {
		t.Fatalf("terminal phase = %v, want failed", final.Phase)
	}
	if !strings.Contains(final.LastError, "missing proxy credentials") {
		t.Fatalf("LastError = %q, want missing proxy credentials degradation", final.LastError)
	}
	assertProxyStatus(t, store, "degraded", "missing proxy credentials")
}

func TestProxySubmitter_UnreachableProxyReportsDegradedOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "unused")
	}))
	baseURL := srv.URL
	srv.Close()

	store := NewRuntimeStatusStore(t.TempDir() + "/gateway_state.json")
	proxy, err := NewProxySubmitter(ProxySubmitterConfig{
		BaseURL:       baseURL,
		Model:         "gormes-agent",
		APIKey:        "secret",
		RuntimeStatus: store,
	})
	if err != nil {
		t.Fatalf("NewProxySubmitter: %v", err)
	}
	if err := proxy.Submit(kernel.PlatformEvent{Kind: kernel.PlatformEventSubmit, Text: "hi", SessionID: "sess-down"}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	final := readProxyTerminalFrame(t, proxy.Render())
	if final.Phase != kernel.PhaseFailed {
		t.Fatalf("terminal phase = %v, want failed", final.Phase)
	}
	if !strings.Contains(final.LastError, "proxy unreachable") {
		t.Fatalf("LastError = %q, want proxy unreachable degradation", final.LastError)
	}
	assertProxyStatus(t, store, "degraded", "proxy unreachable")
}

func readProxyTerminalFrame(t *testing.T, frames <-chan kernel.RenderFrame) kernel.RenderFrame {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case f := <-frames:
			if f.Phase == kernel.PhaseIdle || f.Phase == kernel.PhaseFailed || f.Phase == kernel.PhaseCancelling {
				return f
			}
		case <-timeout:
			t.Fatal("timed out waiting for proxy terminal frame")
		}
	}
}

func assertProxyStatus(t *testing.T, store *RuntimeStatusStore, wantState, wantMessage string) {
	t.Helper()
	status, err := store.ReadRuntimeStatus(context.Background())
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status.Proxy.State != wantState {
		t.Fatalf("proxy status = %q, want %q", status.Proxy.State, wantState)
	}
	if !strings.Contains(status.Proxy.ErrorMessage, wantMessage) {
		t.Fatalf("proxy error = %q, want %q", status.Proxy.ErrorMessage, wantMessage)
	}
}
