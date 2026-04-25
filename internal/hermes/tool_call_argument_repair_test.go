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

var echoToolDescriptor = ToolDescriptor{
	Name:        "echo",
	Description: "echo text",
	Schema:      json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
}

func TestStream_ToolCallArgumentsRepairDeterministicAgainstAdvertisedSchema(t *testing.T) {
	final, err := runToolCallRepairStream(t, []ToolDescriptor{echoToolDescriptor}, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_echo","type":"function","function":{"name":"echo","arguments":"{\"text\":\"hi\","}}]}}]}

data: {"choices":[{"finish_reason":"tool_calls"}]}

data: [DONE]

`)
	if err != nil {
		t.Fatalf("stream returned error: %v", err)
	}
	if len(final.ToolCalls) != 1 {
		t.Fatalf("tool calls len = %d, want 1", len(final.ToolCalls))
	}
	call := final.ToolCalls[0]
	if call.Name != "echo" || call.ID != "call_echo" {
		t.Fatalf("tool call = %+v, want call_echo/echo", call)
	}
	var got map[string]string
	if err := json.Unmarshal(call.Arguments, &got); err != nil {
		t.Fatalf("repaired arguments are invalid JSON: %v: %s", err, call.Arguments)
	}
	if got["text"] != "hi" {
		t.Fatalf("repaired arguments = %s, want text=hi", call.Arguments)
	}
}

func TestStream_ToolCallArgumentsRejectImpossibleRepairBeforeExecution(t *testing.T) {
	_, err := runToolCallRepairStream(t, []ToolDescriptor{echoToolDescriptor}, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_echo","type":"function","function":{"name":"echo","arguments":"{\"text\":"}}]}}]}

data: {"choices":[{"finish_reason":"tool_calls"}]}

data: [DONE]

`)
	if err == nil {
		t.Fatal("stream error = nil, want tool-call repair error")
	}
	var repairErr *ToolCallRepairError
	if !errors.As(err, &repairErr) {
		t.Fatalf("stream error = %T %v, want ToolCallRepairError", err, err)
	}
	if repairErr.ToolName != "echo" || repairErr.ToolCallID != "call_echo" {
		t.Fatalf("repair error = %+v, want call_echo/echo", repairErr)
	}
}

func TestStream_ToolCallArgumentsRejectUnavailableTool(t *testing.T) {
	_, err := runToolCallRepairStream(t, []ToolDescriptor{echoToolDescriptor}, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_missing","type":"function","function":{"name":"missing","arguments":"{}"}}]}}]}

data: {"choices":[{"finish_reason":"tool_calls"}]}

data: [DONE]

`)
	if err == nil {
		t.Fatal("stream error = nil, want unavailable-tool repair error")
	}
	var repairErr *ToolCallRepairError
	if !errors.As(err, &repairErr) {
		t.Fatalf("stream error = %T %v, want ToolCallRepairError", err, err)
	}
	if !strings.Contains(repairErr.Error(), "not advertised") {
		t.Fatalf("repair error = %q, want not advertised", repairErr.Error())
	}
}

func TestStream_ToolCallArgumentsRejectMissingRequiredAfterRepair(t *testing.T) {
	_, err := runToolCallRepairStream(t, []ToolDescriptor{echoToolDescriptor}, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_echo","type":"function","function":{"name":"echo","arguments":"None"}}]}}]}

data: {"choices":[{"finish_reason":"tool_calls"}]}

data: [DONE]

`)
	if err == nil {
		t.Fatal("stream error = nil, want missing-required repair error")
	}
	if !strings.Contains(err.Error(), `missing required argument "text"`) {
		t.Fatalf("stream error = %q, want missing required text", err.Error())
	}
}

func TestSanitizeToolDescriptorsUsesProviderSafeSchemasWithoutMutatingInput(t *testing.T) {
	descriptors := []ToolDescriptor{{
		Name:        "read_file",
		Description: "read a file",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": ["string", "null"]},
				"metadata": "object"
			},
			"required": ["path", "missing"]
		}`),
	}}
	original := append(json.RawMessage(nil), descriptors[0].Schema...)

	sanitized := SanitizeToolDescriptors(descriptors)

	if string(descriptors[0].Schema) != string(original) {
		t.Fatalf("SanitizeToolDescriptors mutated input schema:\n got %s\nwant %s", descriptors[0].Schema, original)
	}
	var schema struct {
		Type       string `json:"type"`
		Required   []string
		Properties map[string]struct {
			Type       string         `json:"type"`
			Nullable   bool           `json:"nullable,omitempty"`
			Properties map[string]any `json:"properties,omitempty"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(sanitized[0].Schema, &schema); err != nil {
		t.Fatalf("sanitized schema invalid JSON: %v: %s", err, sanitized[0].Schema)
	}
	if schema.Type != "object" {
		t.Fatalf("top-level type = %q, want object", schema.Type)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "path" {
		t.Fatalf("required = %v, want [path]", schema.Required)
	}
	if got := schema.Properties["path"]; got.Type != "string" || !got.Nullable {
		t.Fatalf("path schema = %+v, want nullable string", got)
	}
	if got := schema.Properties["metadata"]; got.Type != "object" || got.Properties == nil {
		t.Fatalf("metadata schema = %+v, want object with properties", got)
	}
}

func runToolCallRepairStream(t *testing.T, tools []ToolDescriptor, fixture string) (Event, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		bw := bufio.NewWriter(w)
		fmt.Fprint(bw, fixture)
		bw.Flush()
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL, "")
	s, err := c.OpenStream(context.Background(), ChatRequest{
		Model:    "x",
		Messages: []Message{{Role: "user", Content: "echo hi"}},
		Tools:    tools,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for {
		e, err := s.Recv(context.Background())
		if err == io.EOF {
			return Event{}, io.EOF
		}
		if err != nil {
			return Event{}, err
		}
		if e.Kind == EventDone {
			return e, nil
		}
	}
}
