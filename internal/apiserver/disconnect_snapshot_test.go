package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
)

var errSimulatedDisconnect = errors.New("simulated client disconnect")

func TestResponses_StreamConnectionResetStoresIncompleteSnapshotAndCancelsTurn(t *testing.T) {
	store := NewResponseStore(10)
	loop := &disconnectSnapshotLoop{
		streamTokens: []string{"partial output"},
		streamResult: TurnResult{
			Content:   "partial output",
			SessionID: "sess-disconnect",
			Usage:     Usage{PromptTokens: 2, CompletionTokens: 1, TotalTokens: 3},
		},
		runResult: TurnResult{Content: "unexpected non-stream response", SessionID: "sess-disconnect"},
	}
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop, ResponseStore: store})

	rec := &disconnectingResponseWriter{header: make(http.Header), failAtWrite: 2}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", jsonBody(t, map[string]any{
		"model":        "gormes-agent",
		"input":        "will disconnect",
		"instructions": "keep context",
		"stream":       true,
		"store":        true,
	}))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)

	if !loop.streamContextCancelled() {
		t.Fatal("stream turn context was not cancelled after client disconnect")
	}
	id, stored := onlyStoredResponse(t, store)
	if !strings.HasPrefix(id, "resp_") {
		t.Fatalf("stored response id = %q, want resp_ prefix", id)
	}
	if stored.Response.Status != "incomplete" {
		t.Fatalf("stored status = %q, want incomplete; response=%+v", stored.Response.Status, stored.Response)
	}
	if got := responseOutputText(stored.Response.Output); got != "partial output" {
		t.Fatalf("stored output text = %q, want partial output", got)
	}
	if stored.SessionID != "sess-disconnect" {
		t.Fatalf("stored SessionID = %q, want sess-disconnect", stored.SessionID)
	}
	if !historyHasMessage(stored.ConversationHistory, "assistant", "partial output") {
		t.Fatalf("conversation history missing partial assistant text: %+v", stored.ConversationHistory)
	}
}

func TestResponses_StreamRequestCancellationStoresIncompleteSnapshot(t *testing.T) {
	store := NewResponseStore(10)
	loop := &cancelledSnapshotLoop{token: "partial before cancellation"}
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop, ResponseStore: store})

	ctx, cancel := context.WithCancel(context.Background())
	rec := &cancelingResponseWriter{header: make(http.Header), cancelAtWrite: 2, cancel: cancel}
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", jsonBody(t, map[string]any{
		"input":  "will be cancelled",
		"stream": true,
		"store":  true,
	})).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)

	if !loop.streamContextCancelled() {
		t.Fatal("stream turn context was not cancelled by request cancellation")
	}
	_, stored := onlyStoredResponse(t, store)
	if stored.Response.Status != "incomplete" {
		t.Fatalf("stored status = %q, want incomplete; response=%+v", stored.Response.Status, stored.Response)
	}
	if got := responseOutputText(stored.Response.Output); got != "partial before cancellation" {
		t.Fatalf("stored output text = %q, want partial before cancellation", got)
	}
	if !historyHasMessage(stored.ConversationHistory, "assistant", "partial before cancellation") {
		t.Fatalf("conversation history missing partial assistant text: %+v", stored.ConversationHistory)
	}
}

func TestResponses_PreviousResponseIDContinuesFromIncompleteSnapshot(t *testing.T) {
	store := NewResponseStore(10)
	loop := &disconnectSnapshotLoop{
		streamTokens: []string{"draft answer"},
		streamResult: TurnResult{Content: "draft answer", SessionID: "sess-chain"},
		runResult:    TurnResult{Content: "continued answer", SessionID: "sess-chain"},
	}
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop, ResponseStore: store})

	rec := &disconnectingResponseWriter{header: make(http.Header), failAtWrite: 2}
	firstReq := httptest.NewRequest(http.MethodPost, "/v1/responses", jsonBody(t, map[string]any{
		"input":        "start",
		"instructions": "remember drafts",
		"stream":       true,
		"store":        true,
	}))
	firstReq.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, firstReq)

	incompleteID, stored := storedResponseByStatus(t, store, "incomplete")
	if got := responseOutputText(stored.Response.Output); got != "draft answer" {
		t.Fatalf("incomplete output text = %q, want draft answer", got)
	}

	second := postJSON(t, srv.Handler(), "/v1/responses", map[string]any{
		"input":                "continue",
		"previous_response_id": incompleteID,
	}, nil)
	if second.Code != http.StatusOK {
		t.Fatalf("follow-up status = %d, want 200; body=%s", second.Code, second.Body.String())
	}

	call := loop.lastRunCall()
	if call.SessionID != "sess-chain" {
		t.Fatalf("follow-up SessionID = %q, want sess-chain", call.SessionID)
	}
	if call.SystemPrompt != "remember drafts" {
		t.Fatalf("follow-up SystemPrompt = %q, want stored instructions", call.SystemPrompt)
	}
	if !historyHasMessage(call.History, "assistant", "draft answer") {
		t.Fatalf("follow-up history lost incomplete assistant text: %+v", call.History)
	}
}

func TestKernelTurnLoop_RequestCancellationSubmitsKernelCancel(t *testing.T) {
	submitter := newCancelRecordingKernelSubmitter()
	loop := NewKernelTurnLoop(submitter)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		_, err := loop.StreamTurn(ctx, TurnRequest{UserMessage: "stream until cancelled"}, StreamCallbacks{
			OnToken: func(string) error { return nil },
		})
		errCh <- err
	}()

	submitter.waitForKind(t, kernel.PlatformEventSubmit)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("StreamTurn error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("StreamTurn did not return after request cancellation")
	}
	submitter.requireSawKind(t, kernel.PlatformEventCancel)
}

type disconnectSnapshotLoop struct {
	mu           sync.Mutex
	streamTokens []string
	streamResult TurnResult
	runResult    TurnResult
	streamCalls  []TurnRequest
	runCalls     []TurnRequest
	cancelled    bool
}

func (l *disconnectSnapshotLoop) RunTurn(_ context.Context, req TurnRequest) (TurnResult, error) {
	l.mu.Lock()
	l.runCalls = append(l.runCalls, req)
	l.mu.Unlock()
	return l.runResult, nil
}

func (l *disconnectSnapshotLoop) StreamTurn(ctx context.Context, req TurnRequest, cb StreamCallbacks) (TurnResult, error) {
	l.mu.Lock()
	l.streamCalls = append(l.streamCalls, req)
	l.mu.Unlock()
	for _, token := range l.streamTokens {
		if err := cb.OnToken(token); err != nil {
			if ctx.Err() != nil {
				l.mu.Lock()
				l.cancelled = true
				l.mu.Unlock()
			}
			return l.streamResult, err
		}
	}
	return l.streamResult, nil
}

func (l *disconnectSnapshotLoop) streamContextCancelled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cancelled
}

func (l *disconnectSnapshotLoop) lastRunCall() TurnRequest {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.runCalls[len(l.runCalls)-1]
}

type cancelledSnapshotLoop struct {
	mu        sync.Mutex
	token     string
	cancelled bool
}

func (l *cancelledSnapshotLoop) RunTurn(context.Context, TurnRequest) (TurnResult, error) {
	return TurnResult{Content: "unexpected non-stream response"}, nil
}

func (l *cancelledSnapshotLoop) StreamTurn(ctx context.Context, req TurnRequest, cb StreamCallbacks) (TurnResult, error) {
	if err := cb.OnToken(l.token); err != nil {
		return TurnResult{}, err
	}
	<-ctx.Done()
	l.mu.Lock()
	l.cancelled = true
	l.mu.Unlock()
	return TurnResult{Content: l.token, SessionID: req.SessionID}, ctx.Err()
}

func (l *cancelledSnapshotLoop) streamContextCancelled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cancelled
}

type cancelRecordingKernelSubmitter struct {
	mu     sync.Mutex
	events []kernel.PlatformEvent
	notify chan kernel.PlatformEventKind
	render chan kernel.RenderFrame
}

func newCancelRecordingKernelSubmitter() *cancelRecordingKernelSubmitter {
	return &cancelRecordingKernelSubmitter{
		notify: make(chan kernel.PlatformEventKind, 8),
		render: make(chan kernel.RenderFrame),
	}
}

func (s *cancelRecordingKernelSubmitter) Submit(e kernel.PlatformEvent) error {
	s.mu.Lock()
	s.events = append(s.events, e)
	s.mu.Unlock()
	s.notify <- e.Kind
	return nil
}

func (s *cancelRecordingKernelSubmitter) Render() <-chan kernel.RenderFrame {
	return s.render
}

func (s *cancelRecordingKernelSubmitter) waitForKind(t *testing.T, kind kernel.PlatformEventKind) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case got := <-s.notify:
			if got == kind {
				return
			}
		case <-deadline:
			t.Fatalf("timeout waiting for kernel event kind %d", kind)
		}
	}
}

func (s *cancelRecordingKernelSubmitter) requireSawKind(t *testing.T, kind kernel.PlatformEventKind) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range s.events {
		if event.Kind == kind {
			return
		}
	}
	t.Fatalf("kernel events = %+v, want kind %d", s.events, kind)
}

type disconnectingResponseWriter struct {
	header      http.Header
	body        bytes.Buffer
	code        int
	writes      int
	failAtWrite int
}

func (w *disconnectingResponseWriter) Header() http.Header { return w.header }

func (w *disconnectingResponseWriter) WriteHeader(code int) { w.code = code }

func (w *disconnectingResponseWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.failAtWrite > 0 && w.writes >= w.failAtWrite {
		return 0, errSimulatedDisconnect
	}
	return w.body.Write(p)
}

func (w *disconnectingResponseWriter) Flush() {}

type cancelingResponseWriter struct {
	header        http.Header
	body          bytes.Buffer
	code          int
	writes        int
	cancelAtWrite int
	cancel        context.CancelFunc
}

func (w *cancelingResponseWriter) Header() http.Header { return w.header }

func (w *cancelingResponseWriter) WriteHeader(code int) { w.code = code }

func (w *cancelingResponseWriter) Write(p []byte) (int, error) {
	w.writes++
	n, err := w.body.Write(p)
	if w.cancelAtWrite > 0 && w.writes >= w.cancelAtWrite && w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	return n, err
}

func (w *cancelingResponseWriter) Flush() {}

func jsonBody(t *testing.T, body map[string]any) *bytes.Reader {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal JSON body: %v", err)
	}
	return bytes.NewReader(raw)
}

func onlyStoredResponse(t *testing.T, store *ResponseStore) (string, StoredResponse) {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.mem) != 1 {
		t.Fatalf("stored response count = %d, want 1", len(store.mem))
	}
	for id, rec := range store.mem {
		return id, rec.Data
	}
	t.Fatal("unreachable")
	return "", StoredResponse{}
}

func storedResponseByStatus(t *testing.T, store *ResponseStore, status string) (string, StoredResponse) {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	for id, rec := range store.mem {
		if rec.Data.Response.Status == status {
			return id, rec.Data
		}
	}
	t.Fatalf("no stored response with status %q in %+v", status, store.mem)
	return "", StoredResponse{}
}

func responseOutputText(items []ResponseOutputItem) string {
	var b strings.Builder
	for _, item := range items {
		if item.Type != "message" {
			continue
		}
		for _, part := range item.Content {
			if part.Type == "output_text" {
				b.WriteString(part.Text)
			}
		}
	}
	return b.String()
}

func historyHasMessage(history []ChatMessage, role, content string) bool {
	for _, msg := range history {
		if msg.Role == role && msg.Content == content {
			return true
		}
	}
	return false
}
