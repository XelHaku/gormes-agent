package hermes

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicOpenStream_MapsConversationAndCacheControl(t *testing.T) {
	type capturedRequest struct {
		Model     string           `json:"model"`
		MaxTokens int              `json:"max_tokens"`
		Stream    bool             `json:"stream"`
		System    any              `json:"system"`
		Messages  []map[string]any `json:"messages"`
		Tools     []map[string]any `json:"tools"`
	}

	var got capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/messages")
		}
		if got := r.Header.Get("x-api-key"); got != "sk-ant-api-test" {
			t.Fatalf("x-api-key = %q, want %q", got, "sk-ant-api-test")
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicVersion {
			t.Fatalf("anthropic-version = %q, want %q", got, anthropicVersion)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("Accept = %q, want text/event-stream", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	client := NewAnthropicClient(srv.URL, "sk-ant-api-test")
	stream, err := client.OpenStream(context.Background(), ChatRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 2048,
		Messages: []Message{
			{Role: "system", Content: "cached system", CacheControl: &CacheControl{Type: "ephemeral"}},
			{Role: "user", Content: "look up weather"},
			{
				Role:    "assistant",
				Content: "Calling a tool",
				ToolCalls: []ToolCall{{
					ID:        "toolu_1",
					Name:      "get_weather",
					Arguments: json.RawMessage(`{"location":"Monterrey"}`),
				}},
			},
			{Role: "tool", ToolCallID: "toolu_1", Name: "get_weather", Content: "72F and sunny", CacheControl: &CacheControl{Type: "ephemeral"}},
		},
		Tools: []ToolDescriptor{{
			Name:        "get_weather",
			Description: "Returns the weather",
			Schema:      json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
		}},
	})
	if err != nil {
		t.Fatalf("OpenStream() error = %v", err)
	}
	defer stream.Close()

	if _, err := stream.Recv(context.Background()); err != io.EOF {
		t.Fatalf("Recv() err = %v, want EOF", err)
	}

	if got.Model != "claude-sonnet-4-5-20250929" {
		t.Fatalf("model = %q, want claude-sonnet-4-5-20250929", got.Model)
	}
	if got.MaxTokens != 2048 {
		t.Fatalf("max_tokens = %d, want 2048", got.MaxTokens)
	}
	if !got.Stream {
		t.Fatal("stream = false, want true")
	}
	systemBlocks, ok := got.System.([]any)
	if !ok || len(systemBlocks) != 1 {
		t.Fatalf("system = %#v, want one cached content block", got.System)
	}
	systemBlock, _ := systemBlocks[0].(map[string]any)
	if systemBlock["text"] != "cached system" {
		t.Fatalf("system text = %#v, want %q", systemBlock["text"], "cached system")
	}
	cacheControl, _ := systemBlock["cache_control"].(map[string]any)
	if cacheControl["type"] != "ephemeral" {
		t.Fatalf("system cache_control = %#v, want ephemeral", systemBlock["cache_control"])
	}
	if len(got.Messages) != 3 {
		t.Fatalf("messages len = %d, want 3 after tool result continuation mapping", len(got.Messages))
	}
	assistantBlocks, _ := got.Messages[1]["content"].([]any)
	if len(assistantBlocks) != 2 {
		t.Fatalf("assistant blocks len = %d, want 2", len(assistantBlocks))
	}
	toolUse, _ := assistantBlocks[1].(map[string]any)
	if toolUse["type"] != "tool_use" || toolUse["name"] != "get_weather" || toolUse["id"] != "toolu_1" {
		t.Fatalf("assistant tool_use = %#v, want id/name/type preserved", toolUse)
	}
	input, _ := toolUse["input"].(map[string]any)
	if input["location"] != "Monterrey" {
		t.Fatalf("tool_use input = %#v, want location Monterrey", input)
	}
	toolResultBlocks, _ := got.Messages[2]["content"].([]any)
	if len(toolResultBlocks) != 1 {
		t.Fatalf("tool result blocks len = %d, want 1", len(toolResultBlocks))
	}
	toolResult, _ := toolResultBlocks[0].(map[string]any)
	if toolResult["type"] != "tool_result" || toolResult["tool_use_id"] != "toolu_1" {
		t.Fatalf("tool_result = %#v, want linked tool result", toolResult)
	}
	toolResultCache, _ := toolResult["cache_control"].(map[string]any)
	if toolResultCache["type"] != "ephemeral" {
		t.Fatalf("tool_result cache_control = %#v, want ephemeral", toolResult["cache_control"])
	}
	if len(got.Tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(got.Tools))
	}
	if got.Tools[0]["name"] != "get_weather" {
		t.Fatalf("tools[0].name = %#v, want get_weather", got.Tools[0]["name"])
	}
	if got.Tools[0]["input_schema"] == nil {
		t.Fatalf("tools[0].input_schema = nil, want schema passthrough")
	}
}

const anthropicToolUseFixture = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5-20250929","content":[],"usage":{"input_tokens":11}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Need a tool."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Checking weather. "}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"One moment."}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: content_block_start
data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"location\":\"Mon"}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"terrey\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":2}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":23}}

event: message_stop
data: {"type":"message_stop"}

`

func TestAnthropicStream_AccumulatesToolUseDeltasAndMapsStopReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		bw := bufio.NewWriter(w)
		fmt.Fprint(bw, anthropicToolUseFixture)
		bw.Flush()
	}))
	defer srv.Close()

	client := NewAnthropicClient(srv.URL, "sk-ant-api-test")
	stream, err := client.OpenStream(context.Background(), ChatRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 256,
		Messages:  []Message{{Role: "user", Content: "weather in Monterrey"}},
		Tools: []ToolDescriptor{{
			Name:        "get_weather",
			Description: "Returns current weather.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
		}},
	})
	if err != nil {
		t.Fatalf("OpenStream() error = %v", err)
	}
	defer stream.Close()

	var got []Event
	for {
		ev, err := stream.Recv(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv() error = %v", err)
		}
		got = append(got, ev)
	}

	if len(got) != 4 {
		t.Fatalf("event count = %d, want 4 (reasoning, token, token, done)", len(got))
	}
	if got[0].Kind != EventReasoning || got[0].Reasoning != "Need a tool." {
		t.Fatalf("got[0] = %+v, want reasoning delta", got[0])
	}
	if got[1].Kind != EventToken || got[1].Token != "Checking weather. " {
		t.Fatalf("got[1] = %+v, want first text delta", got[1])
	}
	if got[2].Kind != EventToken || got[2].Token != "One moment." {
		t.Fatalf("got[2] = %+v, want second text delta", got[2])
	}
	final := got[3]
	if final.Kind != EventDone {
		t.Fatalf("final kind = %v, want EventDone", final.Kind)
	}
	if final.FinishReason != "tool_calls" {
		t.Fatalf("FinishReason = %q, want %q", final.FinishReason, "tool_calls")
	}
	if final.TokensIn != 11 || final.TokensOut != 23 {
		t.Fatalf("usage = %d/%d, want 11/23", final.TokensIn, final.TokensOut)
	}
	if len(final.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(final.ToolCalls))
	}
	tc := final.ToolCalls[0]
	if tc.ID != "toolu_1" || tc.Name != "get_weather" {
		t.Fatalf("tool call = %+v, want toolu_1/get_weather", tc)
	}
	if strings.TrimSpace(string(tc.Arguments)) != `{"location":"Monterrey"}` {
		t.Fatalf("tool args = %s, want Monterrey payload", tc.Arguments)
	}
}

func TestAnthropicOpenStream_MapsRateLimitErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
	}))
	defer srv.Close()

	client := NewAnthropicClient(srv.URL, "sk-ant-api-test")
	_, err := client.OpenStream(context.Background(), ChatRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 128,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("OpenStream() err = nil, want rate limit error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error type = %T, want *HTTPError", err)
	}
	if httpErr.Status != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", httpErr.Status)
	}
	if !strings.Contains(httpErr.Body, "slow down") {
		t.Fatalf("body = %q, want slow down", httpErr.Body)
	}
	if got := Classify(err); got != ClassRetryable {
		t.Fatalf("Classify(err) = %q, want %q", got, ClassRetryable)
	}
}
