package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestResponseStore_PersistsAndEvictsLeastRecentlyUsed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "responses.db")
	store, err := OpenResponseStore(path, 2)
	if err != nil {
		t.Fatalf("OpenResponseStore: %v", err)
	}
	store.now = steppedClock(time.Unix(100, 0), time.Second)

	if err := store.Put("resp_1", StoredResponse{Response: ResponseObject{ID: "resp_1", Object: "response", Status: "completed"}}); err != nil {
		t.Fatalf("put resp_1: %v", err)
	}
	if err := store.Put("resp_2", StoredResponse{Response: ResponseObject{ID: "resp_2", Object: "response", Status: "completed"}}); err != nil {
		t.Fatalf("put resp_2: %v", err)
	}
	if _, ok, err := store.Get("resp_1"); err != nil || !ok {
		t.Fatalf("touch resp_1 ok=%v err=%v", ok, err)
	}
	if err := store.Put("resp_3", StoredResponse{Response: ResponseObject{ID: "resp_3", Object: "response", Status: "completed"}}); err != nil {
		t.Fatalf("put resp_3: %v", err)
	}
	if _, ok, err := store.Get("resp_2"); err != nil || ok {
		t.Fatalf("resp_2 after LRU eviction ok=%v err=%v, want missing", ok, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reopened, err := OpenResponseStore(path, 2)
	if err != nil {
		t.Fatalf("reopen response store: %v", err)
	}
	defer reopened.Close()
	if got, ok, err := reopened.Get("resp_1"); err != nil || !ok || got.Response.ID != "resp_1" {
		t.Fatalf("reopened resp_1 = %+v ok=%v err=%v", got, ok, err)
	}
	if got, ok, err := reopened.Get("resp_3"); err != nil || !ok || got.Response.ID != "resp_3" {
		t.Fatalf("reopened resp_3 = %+v ok=%v err=%v", got, ok, err)
	}
	if n, err := reopened.Len(); err != nil || n != 2 {
		t.Fatalf("reopened Len = %d err=%v, want 2", n, err)
	}
}

func TestResponses_PreviousResponseIDChainsStoredToolHistoryAndDelete(t *testing.T) {
	loop := &fakeTurnLoop{result: TurnResult{
		Content:   "Files: README.md",
		SessionID: "sess-chain",
		Usage:     Usage{PromptTokens: 4, CompletionTokens: 3, TotalTokens: 7},
		Messages: []ChatMessage{
			{
				Role:    "assistant",
				Content: "I will inspect the files.",
				ToolCalls: []ToolCall{{
					ID:        "call_1",
					Name:      "terminal",
					Arguments: `{"command":"ls"}`,
				}},
			},
			{Role: "tool", ToolCallID: "call_1", Name: "terminal", Content: "README.md"},
			{Role: "assistant", Content: "Files: README.md"},
		},
	}}
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop, ResponseStore: NewResponseStore(10)})

	first := postJSON(t, srv.Handler(), "/v1/responses", map[string]any{
		"model":        "gormes-agent",
		"input":        "List files",
		"instructions": "Be concise.",
	}, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200; body=%s", first.Code, first.Body.String())
	}
	var firstBody ResponseObject
	if err := json.Unmarshal(first.Body.Bytes(), &firstBody); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if firstBody.ID == "" || firstBody.Object != "response" || firstBody.Status != "completed" {
		t.Fatalf("first response identity = %+v", firstBody)
	}
	if !hasOutputItem(firstBody.Output, "function_call", "terminal") ||
		!hasOutputItem(firstBody.Output, "function_call_output", "terminal") ||
		!hasOutputText(firstBody.Output, "Files: README.md") {
		t.Fatalf("first output missing tool call/result/final text: %+v", firstBody.Output)
	}

	get := getJSON(t, srv.Handler(), "/v1/responses/"+firstBody.ID, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body=%s", get.Code, get.Body.String())
	}

	loop.result = TurnResult{Content: "README contents", SessionID: "sess-chain"}
	second := postJSON(t, srv.Handler(), "/v1/responses", map[string]any{
		"model":                "gormes-agent",
		"input":                "Read it",
		"previous_response_id": firstBody.ID,
	}, nil)
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200; body=%s", second.Code, second.Body.String())
	}
	secondCall := loop.lastCall()
	if secondCall.SessionID != "sess-chain" {
		t.Fatalf("second SessionID = %q, want sess-chain", secondCall.SessionID)
	}
	if secondCall.SystemPrompt != "Be concise." {
		t.Fatalf("second SystemPrompt = %q, want inherited instructions", secondCall.SystemPrompt)
	}
	if !historyContainsToolExchange(secondCall.History, "call_1", "terminal", "README.md") {
		t.Fatalf("second history missing stored tool exchange: %+v", secondCall.History)
	}

	del := deleteJSON(t, srv.Handler(), "/v1/responses/"+firstBody.ID, nil)
	if del.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want 200; body=%s", del.Code, del.Body.String())
	}
	missing := getJSON(t, srv.Handler(), "/v1/responses/"+firstBody.ID, nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("GET after delete status = %d, want 404; body=%s", missing.Code, missing.Body.String())
	}
}

func TestResponses_ConversationNameChainsToLatestResponse(t *testing.T) {
	loop := &fakeTurnLoop{result: TurnResult{
		Content:   "First answer",
		SessionID: "sess-conversation",
		Messages:  []ChatMessage{{Role: "assistant", Content: "First answer"}},
	}}
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop, ResponseStore: NewResponseStore(10)})

	first := postJSON(t, srv.Handler(), "/v1/responses", map[string]any{
		"input":        "First question",
		"conversation": "project-alpha",
	}, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200; body=%s", first.Code, first.Body.String())
	}

	loop.result = TurnResult{Content: "Second answer", SessionID: "sess-conversation"}
	second := postJSON(t, srv.Handler(), "/v1/responses", map[string]any{
		"input":        "Second question",
		"conversation": "project-alpha",
	}, nil)
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200; body=%s", second.Code, second.Body.String())
	}
	call := loop.lastCall()
	if call.SessionID != "sess-conversation" {
		t.Fatalf("conversation SessionID = %q, want sess-conversation", call.SessionID)
	}
	if len(call.History) < 2 || call.History[0].Role != "user" || call.History[0].Content != "First question" ||
		call.History[1].Role != "assistant" || call.History[1].Content != "First answer" {
		t.Fatalf("conversation history = %+v, want first user/assistant turn", call.History)
	}
}

func TestResponses_PreviousResponseMissUsesErrorEnvelopeAndHealthStatus(t *testing.T) {
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: &fakeTurnLoop{}, ResponseStore: NewResponseStore(10)})

	rec := postJSON(t, srv.Handler(), "/v1/responses", map[string]any{
		"input":                "follow up",
		"previous_response_id": "resp_missing",
	}, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if body.Error.Code != "previous_response_not_found" {
		t.Fatalf("error code = %q, want previous_response_not_found", body.Error.Code)
	}

	health := getJSON(t, srv.Handler(), "/v1/health", nil)
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d; body=%s", health.Code, health.Body.String())
	}
	var status struct {
		Responses struct {
			StoreEnabled           bool `json:"store_enabled"`
			PreviousResponseMisses int  `json:"previous_response_misses"`
		} `json:"responses"`
	}
	if err := json.Unmarshal(health.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if !status.Responses.StoreEnabled || status.Responses.PreviousResponseMisses != 1 {
		t.Fatalf("responses status = %+v, want enabled with one previous miss", status.Responses)
	}
}

func TestRuns_StreamsLifecycleEventsFromNativeTurn(t *testing.T) {
	loop := &fakeTurnLoop{
		result:       TurnResult{Content: "Hello run", SessionID: "sess-run", Usage: Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}},
		streamTokens: []string{"Hello", " run"},
	}
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop, ResponseStore: NewResponseStore(10)})

	start := postJSON(t, srv.Handler(), "/v1/runs", map[string]any{"input": "hello"}, nil)
	if start.Code != http.StatusAccepted {
		t.Fatalf("POST /v1/runs status = %d, want 202; body=%s", start.Code, start.Body.String())
	}
	var started struct {
		RunID  string `json:"run_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode run start: %v", err)
	}
	if !strings.HasPrefix(started.RunID, "run_") || started.Status != "started" {
		t.Fatalf("run start = %+v", started)
	}

	events := getJSON(t, srv.Handler(), "/v1/runs/"+started.RunID+"/events", nil)
	if events.Code != http.StatusOK {
		t.Fatalf("GET run events status = %d, want 200; body=%s", events.Code, events.Body.String())
	}
	if got := events.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	body := events.Body.String()
	for _, want := range []string{`"event":"run.started"`, `"event":"message.delta"`, `"delta":"Hello"`, `"delta":" run"`, `"event":"run.completed"`, `"output":"Hello run"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("run events missing %s: %s", want, body)
		}
	}
	if got := loop.lastCall().SessionID; got != started.RunID {
		t.Fatalf("run turn SessionID = %q, want run id", got)
	}
}

func TestRuns_SweepsOrphanedRunStreams(t *testing.T) {
	loop := newBlockingRunLoop()
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop, ResponseStore: NewResponseStore(10), RunTTL: time.Minute})
	now := time.Unix(1_000, 0)
	srv.now = func() time.Time { return now }

	start := postJSON(t, srv.Handler(), "/v1/runs", map[string]any{"input": "wait"}, nil)
	if start.Code != http.StatusAccepted {
		t.Fatalf("POST /v1/runs status = %d, want 202; body=%s", start.Code, start.Body.String())
	}
	var started struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode run start: %v", err)
	}
	loop.waitStarted(t)

	now = now.Add(2 * time.Minute)
	if swept := srv.sweepOrphanedRuns(); swept != 1 {
		t.Fatalf("sweepOrphanedRuns = %d, want 1", swept)
	}
	missing := getJSON(t, srv.Handler(), "/v1/runs/"+started.RunID+"/events", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("events after orphan sweep status = %d, want 404; body=%s", missing.Code, missing.Body.String())
	}
	loop.release(TurnResult{Content: "late", SessionID: started.RunID})
}

type blockingRunLoop struct {
	mu      sync.Mutex
	calls   []TurnRequest
	started chan struct{}
	done    chan TurnResult
	once    sync.Once
}

func newBlockingRunLoop() *blockingRunLoop {
	return &blockingRunLoop{
		started: make(chan struct{}),
		done:    make(chan TurnResult, 1),
	}
}

func (b *blockingRunLoop) RunTurn(context.Context, TurnRequest) (TurnResult, error) {
	return TurnResult{}, errors.New("blockingRunLoop only supports StreamTurn")
}

func (b *blockingRunLoop) StreamTurn(ctx context.Context, req TurnRequest, _ StreamCallbacks) (TurnResult, error) {
	b.mu.Lock()
	b.calls = append(b.calls, req)
	b.mu.Unlock()
	b.once.Do(func() { close(b.started) })
	select {
	case <-ctx.Done():
		return TurnResult{}, ctx.Err()
	case result := <-b.done:
		return result, nil
	}
}

func (b *blockingRunLoop) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-b.started:
	case <-time.After(time.Second):
		t.Fatal("run did not start")
	}
}

func (b *blockingRunLoop) release(result TurnResult) {
	b.done <- result
}

func hasOutputItem(items []ResponseOutputItem, typ, name string) bool {
	for _, item := range items {
		if item.Type == typ && item.Name == name {
			return true
		}
	}
	return false
}

func hasOutputText(items []ResponseOutputItem, text string) bool {
	for _, item := range items {
		for _, part := range item.Content {
			if part.Text == text {
				return true
			}
		}
	}
	return false
}

func historyContainsToolExchange(history []ChatMessage, callID, name, output string) bool {
	var sawCall, sawResult bool
	for _, msg := range history {
		for _, call := range msg.ToolCalls {
			if call.ID == callID && call.Name == name {
				sawCall = true
			}
		}
		if msg.Role == "tool" && msg.ToolCallID == callID && msg.Name == name && msg.Content == output {
			sawResult = true
		}
	}
	return sawCall && sawResult
}

func steppedClock(start time.Time, step time.Duration) func() time.Time {
	var mu sync.Mutex
	next := start
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		out := next
		next = next.Add(step)
		return out
	}
}

func getJSON(t *testing.T, h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	h.ServeHTTP(rec, req)
	_, _ = io.Copy(io.Discard, rec.Result().Body)
	return rec
}

func deleteJSON(t *testing.T, h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, path, bytes.NewReader(nil))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	h.ServeHTTP(rec, req)
	_, _ = io.Copy(io.Discard, rec.Result().Body)
	return rec
}
