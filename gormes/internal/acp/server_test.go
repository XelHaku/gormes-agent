package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

type fakeSessionFactory struct {
	mu       sync.Mutex
	sessions []*fakeSession
}

func (f *fakeSessionFactory) NewSession(_ context.Context, cwd string) (Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := &fakeSession{cwd: cwd}
	f.sessions = append(f.sessions, s)
	return s, nil
}

func (f *fakeSessionFactory) Session(t *testing.T, idx int) *fakeSession {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if idx >= len(f.sessions) {
		t.Fatalf("session[%d] not created; have %d sessions", idx, len(f.sessions))
	}
	return f.sessions[idx]
}

type fakeSession struct {
	cwd      string
	cancelMu sync.Mutex
	cancelFn func()
	promptFn func(context.Context, []ContentBlock, func(SessionUpdate)) (PromptResult, error)
}

func (s *fakeSession) Prompt(ctx context.Context, prompt []ContentBlock, send func(SessionUpdate)) (PromptResult, error) {
	if s.promptFn == nil {
		return PromptResult{}, fmt.Errorf("promptFn not configured")
	}
	return s.promptFn(ctx, prompt, send)
}

func (s *fakeSession) Cancel() {
	s.cancelMu.Lock()
	defer s.cancelMu.Unlock()
	if s.cancelFn != nil {
		s.cancelFn()
	}
}

func (s *fakeSession) Close() error { return nil }

func TestServer_InitializeNewSessionAndPrompt(t *testing.T) {
	factory := &fakeSessionFactory{}
	server := NewServer(Options{
		AgentInfo: Implementation{Name: "gormes", Title: "Gormes", Version: "dev"},
		NewSession: func(ctx context.Context, cwd string) (Session, error) {
			return factory.NewSession(ctx, cwd)
		},
		NewSessionID: func() string { return "sess-test" },
	})

	inR, inW := io.Pipe()
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- server.Serve(ctx, inR, &out)
	}()

	writeJSONLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": 1,
			"clientInfo": map[string]any{
				"name":    "test-client",
				"title":   "Test Client",
				"version": "1.0.0",
			},
		},
	})
	writeJSONLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "session/new",
		"params": map[string]any{
			"cwd": "/tmp/project",
		},
	})

	waitForSession(t, factory, 1)
	factory.Session(t, 0).promptFn = func(ctx context.Context, prompt []ContentBlock, send func(SessionUpdate)) (PromptResult, error) {
		if len(prompt) != 1 || prompt[0].Type != "text" || prompt[0].Text != "hello ACP" {
			t.Fatalf("prompt = %#v, want one text block", prompt)
		}
		send(SessionUpdate{
			SessionUpdate: "agent_message_chunk",
			Content:       TextContentBlock{Type: "text", Text: "hello"},
		})
		send(SessionUpdate{
			SessionUpdate: "agent_message_chunk",
			Content:       TextContentBlock{Type: "text", Text: " world"},
		})
		return PromptResult{StopReason: StopReasonEndTurn}, nil
	}

	writeJSONLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "session/prompt",
		"params": map[string]any{
			"sessionId": "sess-test",
			"prompt": []map[string]any{
				{
					"type": "text",
					"text": "hello ACP",
				},
			},
		},
	})
	if err := inW.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	msgs := decodeJSONLines(t, out.Bytes())
	if len(msgs) != 5 {
		t.Fatalf("message count = %d, want 5", len(msgs))
	}

	initResp := responseByID(t, msgs, float64(1))
	if got := initResp["protocolVersion"]; got != float64(1) {
		t.Fatalf("initialize protocolVersion = %v, want 1", got)
	}
	info := initResp["agentInfo"].(map[string]any)
	if got := info["name"]; got != "gormes" {
		t.Fatalf("initialize agentInfo.name = %v, want gormes", got)
	}

	newResp := responseByID(t, msgs, float64(2))
	if got := newResp["sessionId"]; got != "sess-test" {
		t.Fatalf("session/new sessionId = %v, want sess-test", got)
	}

	updates := notificationsByMethod(msgs, "session/update")
	if len(updates) != 2 {
		t.Fatalf("session/update notifications = %d, want 2", len(updates))
	}
	firstText := updateText(t, updates[0])
	secondText := updateText(t, updates[1])
	if firstText != "hello" || secondText != " world" {
		t.Fatalf("update texts = %q, %q, want %q + %q", firstText, secondText, "hello", " world")
	}

	promptResp := responseByID(t, msgs, float64(3))
	if got := promptResp["stopReason"]; got != string(StopReasonEndTurn) {
		t.Fatalf("session/prompt stopReason = %v, want %q", got, StopReasonEndTurn)
	}
}

func TestServer_SessionCancelStopsActivePrompt(t *testing.T) {
	factory := &fakeSessionFactory{}
	server := NewServer(Options{
		AgentInfo: Implementation{Name: "gormes", Title: "Gormes", Version: "dev"},
		NewSession: func(ctx context.Context, cwd string) (Session, error) {
			return factory.NewSession(ctx, cwd)
		},
		NewSessionID: func() string { return "sess-cancel" },
	})

	inR, inW := io.Pipe()
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- server.Serve(ctx, inR, &out)
	}()

	writeJSONLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{"protocolVersion": 1},
	})
	writeJSONLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "session/new",
		"params":  map[string]any{"cwd": "/tmp/project"},
	})

	waitForSession(t, factory, 1)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	factory.Session(t, 0).cancelFn = func() { close(cancelled) }
	factory.Session(t, 0).promptFn = func(ctx context.Context, prompt []ContentBlock, send func(SessionUpdate)) (PromptResult, error) {
		close(started)
		send(SessionUpdate{
			SessionUpdate: "agent_message_chunk",
			Content:       TextContentBlock{Type: "text", Text: "working"},
		})
		<-cancelled
		return PromptResult{StopReason: StopReasonCancelled}, nil
	}

	writeJSONLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "session/prompt",
		"params": map[string]any{
			"sessionId": "sess-cancel",
			"prompt": []map[string]any{
				{"type": "text", "text": "long run"},
			},
		},
	})

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not start before cancel")
	}

	writeJSONLine(t, inW, map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/cancel",
		"params":  map[string]any{"sessionId": "sess-cancel"},
	})
	if err := inW.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	msgs := decodeJSONLines(t, out.Bytes())
	promptResp := responseByID(t, msgs, float64(3))
	if got := promptResp["stopReason"]; got != string(StopReasonCancelled) {
		t.Fatalf("session/prompt stopReason = %v, want %q", got, StopReasonCancelled)
	}
}

func waitForSession(t *testing.T, factory *fakeSessionFactory, want int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		factory.mu.Lock()
		n := len(factory.sessions)
		factory.mu.Unlock()
		if n >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d sessions; have %d", want, n)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func writeJSONLine(t *testing.T, w *io.PipeWriter, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal(%T): %v", value, err)
	}
	if _, err := fmt.Fprintln(w, string(raw)); err != nil {
		t.Fatalf("write line: %v", err)
	}
}

func decodeJSONLines(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
	out := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var msg map[string]any
		if err := json.Unmarshal(line, &msg); err != nil {
			t.Fatalf("json.Unmarshal(%s): %v", line, err)
		}
		out = append(out, msg)
	}
	return out
}

func responseByID(t *testing.T, msgs []map[string]any, wantID any) map[string]any {
	t.Helper()
	for _, msg := range msgs {
		if msg["id"] == wantID {
			result, ok := msg["result"].(map[string]any)
			if !ok {
				t.Fatalf("message %v result missing map payload", wantID)
			}
			return result
		}
	}
	t.Fatalf("response id %v not found", wantID)
	return nil
}

func notificationsByMethod(msgs []map[string]any, method string) []map[string]any {
	out := make([]map[string]any, 0)
	for _, msg := range msgs {
		if msg["method"] == method {
			out = append(out, msg)
		}
	}
	return out
}

func updateText(t *testing.T, msg map[string]any) string {
	t.Helper()
	params := msg["params"].(map[string]any)
	update := params["update"].(map[string]any)
	content := update["content"].(map[string]any)
	text, _ := content["text"].(string)
	return text
}
