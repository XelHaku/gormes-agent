package hermes

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestBuildCodexResponsesPayload_ConvertsChatInputToolsAndCallIDs(t *testing.T) {
	payload, err := buildCodexResponsesPayload(ChatRequest{
		Model:     "gpt-5-codex",
		MaxTokens: 2048,
		Messages: []Message{
			{Role: "system", Content: "You are Gormes."},
			{Role: "user", Content: "Plain text request."},
			{
				Role: "user",
				ContentParts: []MessageContentPart{
					{Type: "text", Text: "Inspect this screenshot."},
					{Type: "image_url", ImageURL: "data:image/png;base64,abc123", Detail: "high"},
				},
			},
			{
				Role:    "assistant",
				Content: "Checking status.",
				ToolCalls: []ToolCall{{
					Name:      "lookup",
					Arguments: json.RawMessage(`{"query":"status"}`),
				}},
			},
			{Role: "tool", ToolCallID: "call_existing|fc_existing", Content: `{"ok":true}`},
		},
		Tools: []ToolDescriptor{{
			Name:        "lookup",
			Description: "Looks up fixture status.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`),
		}},
	})
	if err != nil {
		t.Fatalf("buildCodexResponsesPayload() error = %v", err)
	}

	got := mustMarshalIndent(t, payload)
	want := []byte(`{
  "model": "gpt-5-codex",
  "instructions": "You are Gormes.",
  "input": [
    {
      "role": "user",
      "content": "Plain text request."
    },
    {
      "role": "user",
      "content": [
        {
          "type": "input_text",
          "text": "Inspect this screenshot."
        },
        {
          "type": "input_image",
          "image_url": "data:image/png;base64,abc123",
          "detail": "high"
        }
      ]
    },
    {
      "role": "assistant",
      "content": "Checking status."
    },
    {
      "type": "function_call",
      "call_id": "call_7685ce46427f",
      "name": "lookup",
      "arguments": "{\"query\":\"status\"}"
    },
    {
      "type": "function_call_output",
      "call_id": "call_existing",
      "output": "{\"ok\":true}"
    }
  ],
  "tools": [
    {
      "type": "function",
      "name": "lookup",
      "description": "Looks up fixture status.",
      "strict": false,
      "parameters": {
        "type": "object",
        "properties": {
          "query": {
            "type": "string"
          }
        },
        "required": [
          "query"
        ],
        "additionalProperties": false
      }
    }
  ],
  "store": false,
  "max_output_tokens": 2048,
  "tool_choice": "auto",
  "parallel_tool_calls": true
}
`)
	if !bytes.Equal(got, want) {
		t.Fatalf("Codex Responses payload mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestNormalizeCodexResponsesResponse_MapsOutputItemsUsageAndToolCalls(t *testing.T) {
	got, err := normalizeCodexResponsesResponse(codexResponsesResponse{
		Status: "completed",
		Output: []codexResponsesOutputItem{
			{
				Type:             "reasoning",
				ID:               "rs_1",
				EncryptedContent: "enc_opaque",
				Summary: []codexResponsesOutputContent{
					{Type: "summary_text", Text: "Need status lookup."},
				},
			},
			{
				Type:   "message",
				Status: "completed",
				Content: []codexResponsesOutputContent{
					{Type: "output_text", Text: "Checking status."},
				},
			},
			{
				Type:      "function_call",
				ID:        "fc_lookup",
				CallID:    "call_lookup",
				Name:      "lookup",
				Arguments: json.RawMessage(`{"query":"status"}`),
			},
		},
		Usage: codexResponsesUsage{
			InputTokens:  21,
			OutputTokens: 8,
			TotalTokens:  29,
		},
	})
	if err != nil {
		t.Fatalf("normalizeCodexResponsesResponse() error = %v", err)
	}

	if got.Message.Role != "assistant" || got.Message.Content != "Checking status." {
		t.Fatalf("message = %+v, want assistant content", got.Message)
	}
	if got.Message.Reasoning == nil || got.Message.Reasoning.Text != "Need status lookup." {
		t.Fatalf("message reasoning = %+v, want summary text", got.Message.Reasoning)
	}
	if len(got.Message.ToolCalls) != 1 {
		t.Fatalf("message tool calls len = %d, want 1", len(got.Message.ToolCalls))
	}
	call := got.Message.ToolCalls[0]
	if call.ID != "call_lookup" || call.Name != "lookup" || string(call.Arguments) != `{"query":"status"}` {
		t.Fatalf("message tool call = %+v, want lookup call", call)
	}
	if got.Usage.InputTokens != 21 || got.Usage.OutputTokens != 8 || got.Usage.TotalTokens != 29 {
		t.Fatalf("usage = %+v, want 21/8/29", got.Usage)
	}

	if len(got.Events) != 3 {
		t.Fatalf("events len = %d, want reasoning/token/done: %+v", len(got.Events), got.Events)
	}
	if got.Events[0].Kind != EventReasoning || got.Events[0].Reasoning != "Need status lookup." {
		t.Fatalf("event[0] = %+v, want reasoning event", got.Events[0])
	}
	if got.Events[1].Kind != EventToken || got.Events[1].Token != "Checking status." {
		t.Fatalf("event[1] = %+v, want token event", got.Events[1])
	}
	final := got.Events[2]
	if final.Kind != EventDone || final.FinishReason != "tool_calls" || final.TokensIn != 21 || final.TokensOut != 8 {
		t.Fatalf("final event = %+v, want tool_calls with usage", final)
	}
	if len(final.ToolCalls) != 1 || final.ToolCalls[0].ID != "call_lookup" {
		t.Fatalf("final tool calls = %+v, want call_lookup", final.ToolCalls)
	}
}
