package contextengine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCompressor_GetStatusToolReflectsUpdatedBudgets(t *testing.T) {
	c := NewCompressor(Config{
		ContextLength:    200_000,
		ThresholdPercent: 0.50,
		TargetRatio:      0.20,
	})

	c.UpdateFromResponse(Usage{PromptTokens: 90_000, CompletionTokens: 2_500})
	c.RecordCompression(100_000, 70_000)
	c.UpdateModelContext(256_000)

	out, err := c.HandleToolCall("get_status", nil)
	if err != nil {
		t.Fatalf("HandleToolCall(get_status) error = %v", err)
	}

	var got struct {
		LastPromptTokens     int `json:"last_prompt_tokens"`
		LastCompletionTokens int `json:"last_completion_tokens"`
		LastTotalTokens      int `json:"last_total_tokens"`
		ThresholdTokens      int `json:"threshold_tokens"`
		ContextLength        int `json:"context_length"`
		CompressionCount     int `json:"compression_count"`
		TailTokenBudget      int `json:"tail_token_budget"`
		MaxSummaryTokens     int `json:"max_summary_tokens"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("json.Unmarshal(status) error = %v", err)
	}

	if got.LastPromptTokens != 90_000 {
		t.Fatalf("last_prompt_tokens = %d, want %d", got.LastPromptTokens, 90_000)
	}
	if got.LastCompletionTokens != 2_500 {
		t.Fatalf("last_completion_tokens = %d, want %d", got.LastCompletionTokens, 2_500)
	}
	if got.LastTotalTokens != 92_500 {
		t.Fatalf("last_total_tokens = %d, want %d", got.LastTotalTokens, 92_500)
	}
	if got.ContextLength != 256_000 {
		t.Fatalf("context_length = %d, want %d", got.ContextLength, 256_000)
	}
	if got.ThresholdTokens != 128_000 {
		t.Fatalf("threshold_tokens = %d, want %d", got.ThresholdTokens, 128_000)
	}
	if got.CompressionCount != 1 {
		t.Fatalf("compression_count = %d, want %d", got.CompressionCount, 1)
	}
	if got.TailTokenBudget != 25_600 {
		t.Fatalf("tail_token_budget = %d, want %d", got.TailTokenBudget, 25_600)
	}
	if got.MaxSummaryTokens != 12_000 {
		t.Fatalf("max_summary_tokens = %d, want %d", got.MaxSummaryTokens, 12_000)
	}
}

func TestCompressor_HandleToolCallUnknownToolReturnsErrorJSON(t *testing.T) {
	c := NewCompressor(Config{ContextLength: 128_000})

	out, err := c.HandleToolCall("missing_tool", nil)
	if err != nil {
		t.Fatalf("HandleToolCall(missing_tool) error = %v", err)
	}
	if !strings.Contains(string(out), `"error":"unknown tool: \"missing_tool\""`) {
		t.Fatalf("HandleToolCall(missing_tool) = %s, want unknown-tool JSON", out)
	}
}
