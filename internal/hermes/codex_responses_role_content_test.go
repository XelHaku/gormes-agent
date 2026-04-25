package hermes

import (
	"encoding/json"
	"testing"
)

func TestCodexResponsesRoleContent_UserTextPartsUseInputTextAndImages(t *testing.T) {
	payload, err := buildCodexResponsesPayload(ChatRequest{
		Model: "gpt-5-codex",
		Messages: []Message{{
			Role: "user",
			ContentParts: []MessageContentPart{
				{Type: "text", Text: "plain text"},
				{Type: "input_text", Text: "already input"},
				{Type: "output_text", Text: "assistant-shaped source"},
				{Type: "image_url", ImageURL: "data:image/png;base64,user", Detail: "high"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("buildCodexResponsesPayload() error = %v", err)
	}

	content := requireCodexResponsesMessageContent(t, payload.Input[0], "user")
	requireCodexResponsesContentLen(t, content, 4)
	requireCodexResponsesTextPart(t, content[0], "input_text", "plain text")
	requireCodexResponsesTextPart(t, content[1], "input_text", "already input")
	requireCodexResponsesTextPart(t, content[2], "input_text", "assistant-shaped source")
	if content[3].Type != "input_image" || content[3].ImageURL != "data:image/png;base64,user" || content[3].Detail != "high" {
		t.Fatalf("content[3] = %+v, want input_image with URL and detail", content[3])
	}
}

func TestCodexResponsesRoleContent_AssistantTextPartsUseOutputText(t *testing.T) {
	payload, err := buildCodexResponsesPayload(ChatRequest{
		Model: "gpt-5-codex",
		Messages: []Message{{
			Role: "assistant",
			ContentParts: []MessageContentPart{
				{Type: "text", Text: "plain assistant text"},
				{Type: "input_text", Text: "stored input-style assistant text"},
				{Type: "output_text", Text: "already output"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("buildCodexResponsesPayload() error = %v", err)
	}

	content := requireCodexResponsesMessageContent(t, payload.Input[0], "assistant")
	requireCodexResponsesContentLen(t, content, 3)
	requireCodexResponsesTextPart(t, content[0], "output_text", "plain assistant text")
	requireCodexResponsesTextPart(t, content[1], "output_text", "stored input-style assistant text")
	requireCodexResponsesTextPart(t, content[2], "output_text", "already output")
}

func TestCodexResponsesRoleContent_RoundTripPreservesRoleTypesBeforeToolReplay(t *testing.T) {
	payload, err := buildCodexResponsesPayload(ChatRequest{
		Model: "gpt-5-codex",
		Messages: []Message{
			{
				Role:         "user",
				ContentParts: []MessageContentPart{{Type: "text", Text: "first turn"}},
			},
			{
				Role: "assistant",
				ContentParts: []MessageContentPart{
					{Type: "input_text", Text: "assistant replay"},
				},
				ToolCalls: []ToolCall{{
					ID:        "call_lookup|fc_lookup",
					Name:      "lookup",
					Arguments: json.RawMessage(`{"query":"state"}`),
				}},
			},
			{
				Role:         "user",
				ContentParts: []MessageContentPart{{Type: "output_text", Text: "follow up"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildCodexResponsesPayload() error = %v", err)
	}
	if len(payload.Input) != 4 {
		t.Fatalf("payload input len = %d, want user/assistant/function_call/user", len(payload.Input))
	}

	firstUser := requireCodexResponsesMessageContent(t, payload.Input[0], "user")
	requireCodexResponsesContentLen(t, firstUser, 1)
	requireCodexResponsesTextPart(t, firstUser[0], "input_text", "first turn")

	assistant := requireCodexResponsesMessageContent(t, payload.Input[1], "assistant")
	requireCodexResponsesContentLen(t, assistant, 1)
	requireCodexResponsesTextPart(t, assistant[0], "output_text", "assistant replay")

	call, ok := payload.Input[2].(codexResponsesFunctionCallItem)
	if !ok {
		t.Fatalf("payload.Input[2] = %T, want codexResponsesFunctionCallItem", payload.Input[2])
	}
	if call.CallID != "call_lookup" || call.Name != "lookup" || call.Arguments != `{"query":"state"}` {
		t.Fatalf("function call = %+v, want lookup replay", call)
	}

	secondUser := requireCodexResponsesMessageContent(t, payload.Input[3], "user")
	requireCodexResponsesContentLen(t, secondUser, 1)
	requireCodexResponsesTextPart(t, secondUser[0], "input_text", "follow up")
}

func requireCodexResponsesMessageContent(t *testing.T, item any, role string) []codexResponsesContentPart {
	t.Helper()
	msg, ok := item.(codexResponsesMessageItem)
	if !ok {
		t.Fatalf("message item = %T, want codexResponsesMessageItem", item)
	}
	if msg.Role != role {
		t.Fatalf("message role = %q, want %q", msg.Role, role)
	}
	content, ok := msg.Content.([]codexResponsesContentPart)
	if !ok {
		t.Fatalf("message content = %T, want []codexResponsesContentPart", msg.Content)
	}
	return content
}

func requireCodexResponsesContentLen(t *testing.T, content []codexResponsesContentPart, want int) {
	t.Helper()
	if len(content) != want {
		t.Fatalf("content len = %d, want %d: %+v", len(content), want, content)
	}
}

func requireCodexResponsesTextPart(t *testing.T, part codexResponsesContentPart, partType, text string) {
	t.Helper()
	if part.Type != partType || part.Text != text {
		t.Fatalf("content part = %+v, want type %q text %q", part, partType, text)
	}
}
