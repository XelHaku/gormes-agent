package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/internal/telemetry"
)

type fakeTurnLoop struct {
	mu           sync.Mutex
	calls        []TurnRequest
	result       TurnResult
	err          error
	streamTokens []string
	streamErr    error
}

func (f *fakeTurnLoop) RunTurn(_ context.Context, req TurnRequest) (TurnResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()
	if f.err != nil {
		return TurnResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeTurnLoop) StreamTurn(_ context.Context, req TurnRequest, cb StreamCallbacks) (TurnResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()
	for _, token := range f.streamTokens {
		if err := cb.OnToken(token); err != nil {
			return TurnResult{}, err
		}
	}
	if f.streamErr != nil {
		return TurnResult{}, f.streamErr
	}
	if f.err != nil {
		return TurnResult{}, f.err
	}
	return f.result, nil
}

func (f *fakeTurnLoop) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeTurnLoop) lastCall() TurnRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

func TestChatCompletions_RequiresBearerAuthAndUsesOpenAIErrorEnvelope(t *testing.T) {
	loop := &fakeTurnLoop{}
	srv := NewServer(Config{APIKey: "sk-secret", ModelName: "gormes-agent", Loop: loop})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gormes-agent","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	var body map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error envelope JSON: %v", err)
	}
	if body["error"]["code"] != "invalid_api_key" {
		t.Fatalf("error.code = %v, want invalid_api_key", body["error"]["code"])
	}
	if loop.callCount() != 0 {
		t.Fatalf("turn loop calls = %d, want 0 for auth failure", loop.callCount())
	}
}

func TestChatCompletions_RejectsOversizeBodyBeforeTurnLoop(t *testing.T) {
	loop := &fakeTurnLoop{}
	srv := NewServer(Config{MaxBodyBytes: 64, Loop: loop})
	body := `{"model":"gormes-agent","messages":[{"role":"user","content":"` + strings.Repeat("x", 128) + `"}]}`

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("error envelope JSON: %v", err)
	}
	if got.Error.Code != "body_too_large" {
		t.Fatalf("error.code = %q, want body_too_large", got.Error.Code)
	}
	if loop.callCount() != 0 {
		t.Fatalf("turn loop calls = %d, want 0 for body limit failure", loop.callCount())
	}
}

func TestChatCompletions_NormalizesContentPartsForGatewayUserMessage(t *testing.T) {
	loop := &fakeTurnLoop{result: TurnResult{Content: "ok", SessionID: "sess-normalized"}}
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop})

	body := map[string]any{
		"model": "gormes-agent",
		"messages": []any{
			map[string]any{"role": "system", "content": []any{
				map[string]any{"type": "text", "text": "speak plainly"},
			}},
			map[string]any{"role": "user", "content": "first question"},
			map[string]any{"role": "assistant", "content": "first answer"},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "inspect"},
				map[string]any{"type": "input_text", "text": "the repo"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.test/img.png"}},
				"literal tail",
			}},
		},
	}
	rec := postJSON(t, srv.Handler(), "/v1/chat/completions", body, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	call := loop.lastCall()
	if call.SystemPrompt != "speak plainly" {
		t.Fatalf("SystemPrompt = %q, want speak plainly", call.SystemPrompt)
	}
	if call.UserMessage != "inspect\nthe repo\nliteral tail" {
		t.Fatalf("UserMessage = %q", call.UserMessage)
	}
	if len(call.History) != 2 {
		t.Fatalf("len(History) = %d, want 2", len(call.History))
	}
	if call.History[0].Role != "user" || call.History[0].Content != "first question" {
		t.Fatalf("History[0] = %+v", call.History[0])
	}
	if call.History[1].Role != "assistant" || call.History[1].Content != "first answer" {
		t.Fatalf("History[1] = %+v", call.History[1])
	}
}

func TestChatCompletions_ContentNormalizationFailureDoesNotStartTurn(t *testing.T) {
	loop := &fakeTurnLoop{}
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop})

	rec := postJSON(t, srv.Handler(), "/v1/chat/completions", map[string]any{
		"model": "gormes-agent",
		"messages": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_file", "file_id": "file_123"},
			}},
		},
	}, nil)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Error struct {
			Code  string `json:"code"`
			Param string `json:"param"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("error envelope JSON: %v", err)
	}
	if got.Error.Code != "unsupported_content_type" {
		t.Fatalf("error.code = %q, want unsupported_content_type", got.Error.Code)
	}
	if got.Error.Param != "messages[0].content" {
		t.Fatalf("error.param = %q, want messages[0].content", got.Error.Param)
	}
	if loop.callCount() != 0 {
		t.Fatalf("turn loop calls = %d, want 0 for content-normalization failure", loop.callCount())
	}
}

func TestChatCompletions_NonStreamingUsesNativeKernelAndReturnsSessionHeader(t *testing.T) {
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "Hello"},
		{Kind: hermes.EventToken, Token: " from kernel"},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 3, TokensOut: 2},
	}, "sess-native-1")
	k := kernel.New(kernel.Config{
		Model:     "gormes-agent",
		Endpoint:  "http://mock",
		Admission: kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, store.NewNoop(), telemetry.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = k.Run(ctx) }()

	srv := NewServer(Config{ModelName: "gormes-agent", Loop: NewKernelTurnLoop(k)})
	rec := postJSON(t, srv.Handler(), "/v1/chat/completions", map[string]any{
		"model":    "gormes-agent",
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Hermes-Session-Id"); got != "sess-native-1" {
		t.Fatalf("X-Hermes-Session-Id = %q, want sess-native-1", got)
	}
	var got struct {
		Object  string `json:"object"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response JSON: %v", err)
	}
	if got.Object != "chat.completion" {
		t.Fatalf("object = %q, want chat.completion", got.Object)
	}
	if got.Choices[0].Message.Role != "assistant" || got.Choices[0].Message.Content != "Hello from kernel" {
		t.Fatalf("message = %+v", got.Choices[0].Message)
	}
	if got.Choices[0].FinishReason != "stop" {
		t.Fatalf("finish_reason = %q, want stop", got.Choices[0].FinishReason)
	}
	if got.Usage.PromptTokens != 3 || got.Usage.CompletionTokens != 2 || got.Usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v, want 3/2/5", got.Usage)
	}
}

func TestChatCompletions_SessionHeaderContinuesNativeKernelSession(t *testing.T) {
	mc := hermes.NewMockClient()
	mc.Script([]hermes.Event{{Kind: hermes.EventToken, Token: "one"}, {Kind: hermes.EventDone, FinishReason: "stop"}}, "sess-shared")
	mc.Script([]hermes.Event{{Kind: hermes.EventToken, Token: "two"}, {Kind: hermes.EventDone, FinishReason: "stop"}}, "sess-shared")
	k := kernel.New(kernel.Config{
		Model:     "gormes-agent",
		Endpoint:  "http://mock",
		Admission: kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, store.NewNoop(), telemetry.New(), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = k.Run(ctx) }()

	srv := NewServer(Config{ModelName: "gormes-agent", Loop: NewKernelTurnLoop(k)})
	first := postJSON(t, srv.Handler(), "/v1/chat/completions", map[string]any{
		"model":    "gormes-agent",
		"messages": []any{map[string]any{"role": "user", "content": "first"}},
	}, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200; body=%s", first.Code, first.Body.String())
	}
	second := postJSON(t, srv.Handler(), "/v1/chat/completions", map[string]any{
		"model":    "gormes-agent",
		"messages": []any{map[string]any{"role": "user", "content": "second"}},
	}, map[string]string{"X-Hermes-Session-Id": "sess-shared"})
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200; body=%s", second.Code, second.Body.String())
	}

	requests := mc.Requests()
	if len(requests) != 2 {
		t.Fatalf("mock client request count = %d, want 2", len(requests))
	}
	if requests[1].SessionID != "sess-shared" {
		t.Fatalf("second kernel request SessionID = %q, want sess-shared", requests[1].SessionID)
	}
	if got := second.Header().Get("X-Hermes-Session-Id"); got != "sess-shared" {
		t.Fatalf("second X-Hermes-Session-Id = %q, want sess-shared", got)
	}
}

func TestChatCompletions_NonStreamingTurnFailureUsesOpenAIErrorEnvelope(t *testing.T) {
	loop := &fakeTurnLoop{err: errors.New("provider failed")}
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop})
	rec := postJSON(t, srv.Handler(), "/v1/chat/completions", map[string]any{
		"model":    "gormes-agent",
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	}, nil)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Error struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("error envelope JSON: %v", err)
	}
	if got.Error.Type != "server_error" || got.Error.Code != "turn_failed" {
		t.Fatalf("error = %+v, want server_error/turn_failed", got.Error)
	}
	if loop.callCount() != 1 {
		t.Fatalf("turn loop calls = %d, want 1 for provider failure", loop.callCount())
	}
}

func TestChatCompletions_StreamingReturnsOpenAIChunksAndSessionHeader(t *testing.T) {
	loop := &fakeTurnLoop{
		result:       TurnResult{Content: "Hello stream", SessionID: "sess-stream"},
		streamTokens: []string{"Hello", " stream"},
	}
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop})
	rec := postJSON(t, srv.Handler(), "/v1/chat/completions", map[string]any{
		"model":    "gormes-agent",
		"stream":   true,
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	}, map[string]string{"X-Hermes-Session-Id": "sess-stream"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := rec.Header().Get("X-Hermes-Session-Id"); got != "sess-stream" {
		t.Fatalf("X-Hermes-Session-Id = %q, want sess-stream", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"object":"chat.completion.chunk"`) {
		t.Fatalf("SSE body missing chat completion chunk: %s", body)
	}
	if !strings.Contains(body, `"role":"assistant"`) {
		t.Fatalf("SSE body missing assistant role chunk: %s", body)
	}
	if !strings.Contains(body, `"content":"Hello"`) || !strings.Contains(body, `"content":" stream"`) {
		t.Fatalf("SSE body missing streamed content chunks: %s", body)
	}
	if !strings.Contains(body, `"finish_reason":"stop"`) || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("SSE body missing finish chunk or DONE sentinel: %s", body)
	}
}

func TestChatCompletions_StreamingFailureUsesSSEErrorEnvelope(t *testing.T) {
	loop := &fakeTurnLoop{streamErr: errors.New("provider stream failed")}
	srv := NewServer(Config{ModelName: "gormes-agent", Loop: loop})
	rec := postJSON(t, srv.Handler(), "/v1/chat/completions", map[string]any{
		"model":    "gormes-agent",
		"stream":   true,
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	}, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want streaming 200 with error event; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Fatalf("SSE body missing error event: %s", body)
	}
	if !strings.Contains(body, `"code":"stream_failed"`) || !strings.Contains(body, "provider stream failed") {
		t.Fatalf("SSE body missing OpenAI error envelope: %s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("SSE body missing DONE sentinel after error: %s", body)
	}
}

func TestHealthDoesNotRequireAuth(t *testing.T) {
	srv := NewServer(Config{APIKey: "sk-secret", ModelName: "gormes-agent", Loop: &fakeTurnLoop{}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("health JSON: %v", err)
	}
	if got.Status != "ok" {
		t.Fatalf("status = %q, want ok", got.Status)
	}
}

func postJSON(t *testing.T, h http.Handler, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	h.ServeHTTP(rec, req)
	_, _ = io.Copy(io.Discard, rec.Result().Body)
	return rec
}

func waitForKernelIdle(t *testing.T, frames <-chan kernel.RenderFrame) kernel.RenderFrame {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case f := <-frames:
			if f.Phase == kernel.PhaseIdle && f.Seq > 1 {
				return f
			}
		case <-deadline:
			t.Fatal("timeout waiting for kernel idle")
		}
	}
}
