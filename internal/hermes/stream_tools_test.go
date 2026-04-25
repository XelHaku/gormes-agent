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

const sseToolCallsFixture = `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"echo","arguments":""}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"tex"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"t\":\"hi\"}"}}]}}]}

data: {"choices":[{"finish_reason":"tool_calls"}]}

data: [DONE]

`

func TestStream_ToolCallDeltasAccumulate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		bw := bufio.NewWriter(w)
		fmt.Fprint(bw, sseToolCallsFixture)
		bw.Flush()
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL, "")
	s, err := c.OpenStream(context.Background(), ChatRequest{
		Model:    "x",
		Messages: []Message{{Role: "user", Content: "echo hi"}},
		Tools: []ToolDescriptor{{
			Name:        "echo",
			Description: "echo text",
			Schema:      json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var final Event
	for {
		e, err := s.Recv(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if e.Kind == EventDone {
			final = e
			break
		}
	}

	if final.FinishReason != "tool_calls" {
		t.Fatalf("FinishReason = %q, want tool_calls", final.FinishReason)
	}
	if len(final.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(final.ToolCalls))
	}
	tc := final.ToolCalls[0]
	if tc.ID != "call_abc" {
		t.Errorf("ID = %q", tc.ID)
	}
	if tc.Name != "echo" {
		t.Errorf("Name = %q", tc.Name)
	}
	if !strings.Contains(string(tc.Arguments), `"hi"`) {
		t.Errorf("Arguments = %s, want to contain \"hi\"", tc.Arguments)
	}
}
