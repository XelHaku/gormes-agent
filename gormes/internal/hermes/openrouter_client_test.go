package hermes

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type openRouterTestRequest struct {
	Model    string                     `json:"model"`
	Messages []openRouterTestMessage    `json:"messages"`
	Stream   bool                       `json:"stream"`
	Tools    []openRouterToolDescriptor `json:"tools,omitempty"`
}

type openRouterTestMessage struct {
	Role       string                   `json:"role"`
	Content    string                   `json:"content,omitempty"`
	ToolCalls  []openRouterTestToolCall `json:"tool_calls,omitempty"`
	ToolCallID string                   `json:"tool_call_id,omitempty"`
	Name       string                   `json:"name,omitempty"`
}

type openRouterTestToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openRouterToolDescriptor struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

const openRouterEndTurnFixture = `data: {"id":"or_1","choices":[{"delta":{"content":"done"}}]}

data: {"id":"or_1","choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":21,"completion_tokens":7}}

data: [DONE]

`

const openRouterToolUseFixture = `data: {"id":"or_2","choices":[{"delta":{"reasoning":"need calculator","content":"Let me check. "}}]}

data: {"id":"or_2","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_or_1","type":"function","function":{"name":"calc","arguments":""}}]}}]}

data: {"id":"or_2","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"expression\":\"2+2\"}"}}]}}]}

data: {"id":"or_2","choices":[{"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":42,"completion_tokens":17}}

data: [DONE]

`

func TestNewClient_OpenRouterTranslatesCanonicalMessages(t *testing.T) {
	reqSeen := make(chan openRouterTestRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/chat/completions" {
			t.Fatalf("path = %s, want /api/v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want Bearer test-key", got)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("Accept = %q, want text/event-stream", got)
		}

		var body openRouterTestRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		reqSeen <- body

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		bw := bufio.NewWriter(w)
		fmt.Fprint(bw, openRouterEndTurnFixture)
		bw.Flush()
	}))
	defer srv.Close()

	c := NewClient("openrouter", srv.URL+"/api/v1", "test-key")
	s, err := c.OpenStream(context.Background(), ChatRequest{
		Model: "anthropic/claude-sonnet-4",
		Messages: []Message{
			{Role: "system", Content: "follow rules"},
			{Role: "system", Content: "use tools precisely"},
			{Role: "user", Content: "what is 2+2"},
			{
				Role:    "assistant",
				Content: "I'll calculate that.",
				ToolCalls: []ToolCall{{
					ID:        "call_or_1",
					Name:      "calc",
					Arguments: json.RawMessage(`{"expression":"2+2"}`),
				}},
			},
			{Role: "tool", ToolCallID: "call_or_1", Name: "calc", Content: "4"},
		},
		Tools: []ToolDescriptor{{
			Name:        "calc",
			Description: "calculator",
			Schema:      json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}},"required":["expression"]}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for {
		ev, err := s.Recv(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ev.Kind == EventDone {
			break
		}
	}

	body := <-reqSeen
	if body.Model != "anthropic/claude-sonnet-4" {
		t.Fatalf("model = %q, want anthropic/claude-sonnet-4", body.Model)
	}
	if !body.Stream {
		t.Fatal("stream = false, want true")
	}
	if len(body.Messages) != 5 {
		t.Fatalf("messages len = %d, want 5", len(body.Messages))
	}
	if body.Messages[0].Role != "system" || body.Messages[0].Content != "follow rules" {
		t.Fatalf("messages[0] = %+v", body.Messages[0])
	}
	if body.Messages[1].Role != "system" || body.Messages[1].Content != "use tools precisely" {
		t.Fatalf("messages[1] = %+v", body.Messages[1])
	}
	if body.Messages[2].Role != "user" || body.Messages[2].Content != "what is 2+2" {
		t.Fatalf("messages[2] = %+v", body.Messages[2])
	}
	if body.Messages[3].Role != "assistant" || body.Messages[3].Content != "I'll calculate that." {
		t.Fatalf("messages[3] = %+v", body.Messages[3])
	}
	if len(body.Messages[3].ToolCalls) != 1 {
		t.Fatalf("assistant tool_calls len = %d, want 1", len(body.Messages[3].ToolCalls))
	}
	call := body.Messages[3].ToolCalls[0]
	if call.ID != "call_or_1" || call.Type != "function" || call.Function.Name != "calc" {
		t.Fatalf("assistant tool_call = %+v", call)
	}
	if call.Function.Arguments != `{"expression":"2+2"}` {
		t.Fatalf("assistant tool_call arguments = %q, want JSON string", call.Function.Arguments)
	}
	if body.Messages[4].Role != "tool" || body.Messages[4].ToolCallID != "call_or_1" || body.Messages[4].Name != "calc" || body.Messages[4].Content != "4" {
		t.Fatalf("messages[4] = %+v", body.Messages[4])
	}
	if len(body.Tools) != 1 || body.Tools[0].Type != "function" || body.Tools[0].Function.Name != "calc" || body.Tools[0].Function.Description != "calculator" {
		t.Fatalf("tools = %+v", body.Tools)
	}
	if got := body.Tools[0].Function.Parameters["type"]; got != "object" {
		t.Fatalf("tool parameters type = %#v, want object", got)
	}
}

func TestNewClient_OpenRouterMapsReasoningAndToolUseEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		bw := bufio.NewWriter(w)
		fmt.Fprint(bw, openRouterToolUseFixture)
		bw.Flush()
	}))
	defer srv.Close()

	c := NewClient("openrouter", srv.URL+"/api/v1", "test-key")
	s, err := c.OpenStream(context.Background(), ChatRequest{
		Model:    "anthropic/claude-sonnet-4",
		Messages: []Message{{Role: "user", Content: "what is 2+2?"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var reasoning strings.Builder
	var tokens strings.Builder
	var final Event
	for {
		ev, err := s.Recv(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch ev.Kind {
		case EventReasoning:
			reasoning.WriteString(ev.Reasoning)
		case EventToken:
			tokens.WriteString(ev.Token)
		case EventDone:
			final = ev
			goto done
		}
	}

done:
	if reasoning.String() != "need calculator" {
		t.Fatalf("reasoning = %q, want need calculator", reasoning.String())
	}
	if tokens.String() != "Let me check. " {
		t.Fatalf("tokens = %q, want Let me check. ", tokens.String())
	}
	if final.FinishReason != "tool_calls" {
		t.Fatalf("finish_reason = %q, want tool_calls", final.FinishReason)
	}
	if final.TokensIn != 42 || final.TokensOut != 17 {
		t.Fatalf("usage = %d/%d, want 42/17", final.TokensIn, final.TokensOut)
	}
	if len(final.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(final.ToolCalls))
	}
	tc := final.ToolCalls[0]
	if tc.ID != "call_or_1" || tc.Name != "calc" {
		t.Fatalf("tool_call = %+v, want call_or_1/calc", tc)
	}
	if string(tc.Arguments) != `{"expression":"2+2"}` {
		t.Fatalf("tool_call arguments = %s, want JSON args", tc.Arguments)
	}
}
