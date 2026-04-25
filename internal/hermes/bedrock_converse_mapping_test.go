package hermes

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildBedrockConversePayload_GoldenMapsSharedProviderContract(t *testing.T) {
	payload, err := buildBedrockConversePayload(ChatRequest{
		Model:       "anthropic.claude-3-5-sonnet-20241022-v2:0",
		MaxTokens:   2048,
		Temperature: ptrFloat64(0.35),
		Messages: []Message{
			{Role: "system", Content: "Follow ops policy.", CacheControl: &CacheControl{Type: "default", TTL: "1h"}},
			{Role: "user", Content: "look up weather"},
			{
				Role:    "assistant",
				Content: "Checking the weather.",
				Reasoning: &ReasoningContent{
					Text:      "Need current weather.",
					Signature: "sig-bedrock",
				},
				ToolCalls: []ToolCall{{
					ID:        "toolu_weather",
					Name:      "get_weather",
					Arguments: json.RawMessage(`{"location":"Monterrey","unit":"f"}`),
				}},
			},
			{
				Role:         "tool",
				ToolCallID:   "toolu_weather",
				Name:         "get_weather",
				Content:      `{"temperature":"72F","condition":"sunny"}`,
				CacheControl: &CacheControl{Type: "default"},
			},
			{Role: "assistant", Content: ""},
		},
		Tools: []ToolDescriptor{{
			Name:        "get_weather",
			Description: "Returns current weather.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"location":{"type":"string","description":"City name"},"unit":{"type":"string","enum":["c","f"]}},"required":["location","unit"],"additionalProperties":false}`),
		}},
	})
	if err != nil {
		t.Fatalf("buildBedrockConversePayload() error = %v", err)
	}

	got := mustMarshalIndent(t, payload)
	want, err := os.ReadFile(filepath.Join("testdata", "bedrock_converse", "request_body.golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Bedrock Converse payload mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}

func mustMarshalIndent(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return append(raw, '\n')
}
