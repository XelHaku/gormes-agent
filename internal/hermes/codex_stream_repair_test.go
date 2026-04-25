package hermes

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNormalizeCodexResponsesResponse_RejectsLeakedToolCallTextBeforeHistory(t *testing.T) {
	got, err := normalizeCodexResponsesResponse(codexResponsesResponse{
		Status: "completed",
		Output: []codexResponsesOutputItem{{
			Type:   "message",
			Role:   "assistant",
			Status: "completed",
			Content: []codexResponsesOutputContent{{
				Type: "output_text",
				Text: "assistant to=functions.exec_command {\"cmd\":\"pwd\"}",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("normalizeCodexResponsesResponse() error = %v", err)
	}

	if got.Message.Content != "" {
		t.Fatalf("assistant content = %q, want leaked tool-call text removed before history", got.Message.Content)
	}
	if got.FinishReason != "incomplete" {
		t.Fatalf("finish reason = %q, want incomplete repair path", got.FinishReason)
	}
	requireDiagnostic(t, got.Diagnostics, "leaked tool-call text")
	for _, event := range got.Events {
		if event.Kind == EventToken && strings.Contains(event.Token, "to=functions.exec_command") {
			t.Fatalf("token event leaked tool-call text into parent history: %+v", event)
		}
	}
}

func TestNormalizeCodexResponsesResponse_RepairsEmptyOutputFromStreamedOutputText(t *testing.T) {
	got, err := normalizeCodexResponsesResponse(codexResponsesResponse{
		Status:     "completed",
		OutputText: " streamed final answer ",
	})
	if err != nil {
		t.Fatalf("normalizeCodexResponsesResponse() error = %v", err)
	}

	if got.Message.Content != "streamed final answer" {
		t.Fatalf("assistant content = %q, want streamed output_text backfill", got.Message.Content)
	}
	if got.FinishReason != "stop" {
		t.Fatalf("finish reason = %q, want stop", got.FinishReason)
	}
	requireDiagnostic(t, got.Diagnostics, "empty response.output")
	if len(got.Events) != 2 || got.Events[0].Kind != EventToken || got.Events[0].Token != "streamed final answer" {
		t.Fatalf("events = %+v, want token plus done from output_text backfill", got.Events)
	}
}

func TestNormalizeCodexResponsesResponse_RepairsToolCallArgumentsAgainstSchema(t *testing.T) {
	got, err := normalizeCodexResponsesResponseWithTools(codexResponsesResponse{
		Status: "completed",
		Output: []codexResponsesOutputItem{{
			Type:      "function_call",
			ID:        "fc_echo",
			CallID:    "call_echo",
			Name:      "echo",
			Arguments: json.RawMessage(`{"text":"hi",`),
		}},
	}, []ToolDescriptor{echoToolDescriptor})
	if err != nil {
		t.Fatalf("normalizeCodexResponsesResponseWithTools() error = %v", err)
	}
	if got.FinishReason != "tool_calls" || len(got.Message.ToolCalls) != 1 {
		t.Fatalf("normalized = %+v, want one repaired tool call", got)
	}
	var args map[string]string
	if err := json.Unmarshal(got.Message.ToolCalls[0].Arguments, &args); err != nil {
		t.Fatalf("tool-call arguments are invalid JSON after repair: %v: %s", err, got.Message.ToolCalls[0].Arguments)
	}
	if args["text"] != "hi" {
		t.Fatalf("tool-call arguments = %s, want text=hi", got.Message.ToolCalls[0].Arguments)
	}
}

func TestNormalizeCodexResponsesResponse_RejectsMissingRequiredToolCallArgument(t *testing.T) {
	_, err := normalizeCodexResponsesResponseWithTools(codexResponsesResponse{
		Status: "completed",
		Output: []codexResponsesOutputItem{{
			Type:      "function_call",
			ID:        "fc_echo",
			CallID:    "call_echo",
			Name:      "echo",
			Arguments: json.RawMessage(`None`),
		}},
	}, []ToolDescriptor{echoToolDescriptor})
	if err == nil {
		t.Fatal("normalizeCodexResponsesResponseWithTools() error = nil, want missing-required repair error")
	}
	var repairErr *ToolCallRepairError
	if !errors.As(err, &repairErr) {
		t.Fatalf("error = %T %v, want ToolCallRepairError", err, err)
	}
	if !strings.Contains(repairErr.Error(), `missing required argument "text"`) {
		t.Fatalf("repair error = %q, want missing required text", repairErr.Error())
	}
}

func TestNormalizeCodexResponsesResponse_RejectsUnsupportedOutputItems(t *testing.T) {
	_, err := normalizeCodexResponsesResponse(codexResponsesResponse{
		Status: "completed",
		Output: []codexResponsesOutputItem{{
			Type:   "web_search_call",
			Status: "completed",
		}},
	})
	if err == nil {
		t.Fatal("normalizeCodexResponsesResponse() error = nil, want unsupported item error")
	}
	if !strings.Contains(err.Error(), `unsupported Codex Responses output item "web_search_call"`) {
		t.Fatalf("error = %q, want unsupported Codex Responses output item", err.Error())
	}
}

func requireDiagnostic(t *testing.T, diagnostics []string, want string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic, want) {
			return
		}
	}
	t.Fatalf("diagnostics = %v, want entry containing %q", diagnostics, want)
}
