package learning

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectorLeavesSimpleTurnBelowLearningThreshold(t *testing.T) {
	detector := NewDetector(Config{})

	signal := detector.Evaluate(Turn{
		SessionID:        "sess-simple",
		UserMessage:      "hi",
		AssistantMessage: "hello",
		TokensIn:         12,
		TokensOut:        9,
		Duration:         2 * time.Second,
		FinishedAt:       time.Date(2026, 4, 23, 19, 0, 0, 0, time.UTC),
	})

	if signal.WorthLearning {
		t.Fatal("WorthLearning = true, want false for a trivial turn")
	}
	if signal.Score != 0 {
		t.Fatalf("Score = %d, want 0", signal.Score)
	}
	if len(signal.Reasons) != 0 {
		t.Fatalf("Reasons = %#v, want none", signal.Reasons)
	}
	if signal.Metrics.ToolCallCount != 0 {
		t.Fatalf("ToolCallCount = %d, want 0", signal.Metrics.ToolCallCount)
	}
}

func TestRuntimeAppendsJSONLForLearnWorthyTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learning", "complexity.jsonl")
	runtime := NewRuntime(path, Config{})

	signal, err := runtime.RecordTurn(context.Background(), Turn{
		SessionID:        "sess-complex",
		UserMessage:      "Trace the failure, inspect the CLI wiring, and fix the restart path before rerunning the tests.",
		AssistantMessage: "I traced the failure through the kernel, used the tools twice, and landed the guard after verifying the fix.",
		ToolNames:        []string{"read_file", "run_tests"},
		TokensIn:         260,
		TokensOut:        220,
		Duration:         8 * time.Second,
		FinishedAt:       time.Date(2026, 4, 23, 19, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordTurn() error = %v", err)
	}
	if !signal.WorthLearning {
		t.Fatal("WorthLearning = false, want true for a tool-heavy turn")
	}
	if signal.Score < signal.Threshold {
		t.Fatalf("Score = %d, Threshold = %d, want score at or above threshold", signal.Score, signal.Threshold)
	}
	if signal.Metrics.ToolCallCount != 2 {
		t.Fatalf("ToolCallCount = %d, want 2", signal.Metrics.ToolCallCount)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("line count = %d, want 1", len(lines))
	}

	var persisted Signal
	if err := json.Unmarshal(lines[0], &persisted); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}
	if persisted.SessionID != "sess-complex" {
		t.Fatalf("SessionID = %q, want %q", persisted.SessionID, "sess-complex")
	}
	if !persisted.WorthLearning {
		t.Fatal("persisted WorthLearning = false, want true")
	}
	if !containsReason(persisted.Reasons, "tool_calls") {
		t.Fatalf("Reasons = %#v, want tool_calls", persisted.Reasons)
	}
	if !containsReason(persisted.Reasons, "multi_tool_calls") {
		t.Fatalf("Reasons = %#v, want multi_tool_calls", persisted.Reasons)
	}
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
