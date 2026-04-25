package hermes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildCodexResponsesPayloadOmitsTemperatureAsTransportRule(t *testing.T) {
	payload, err := buildCodexResponsesPayload(ChatRequest{
		Model:       "gpt-5-codex",
		MaxTokens:   5120,
		Temperature: ptrFloat64(0.3),
		Messages: []Message{
			{Role: "system", Content: "You are Gormes."},
			{Role: "user", Content: "Summarize this session for context compression."},
		},
		Tools: []ToolDescriptor{{
			Name:        "summarize_context",
			Description: "Produces a compact context summary.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string"}}}`),
		}},
	})
	if err != nil {
		t.Fatalf("buildCodexResponsesPayload() error = %v", err)
	}

	raw := mustMarshalIndent(t, payload)
	if jsonHasKey(t, raw, "temperature") {
		t.Fatalf("Codex Responses transport payload emitted temperature: %s", raw)
	}
}

func TestCodexResponsesTemperatureFixturesAvoidDeletedFlushDonorReferences(t *testing.T) {
	files := []string{
		"internal/hermes/unsupported_temperature_retry_test.go",
		"internal/hermes/codex_responses_temperature_test.go",
		"docs/content/building-gormes/architecture_plan/progress.json",
	}
	forbidden := []string{
		strings.Join([]string{"tests/run_agent/test", "flush", "memories", "codex.py"}, "_"),
		"TestUnsupportedTemperatureRetryCodexResponses" + "FlushPayloadNeverEmitsTemperature",
		"Codex Responses " + "flush-shaped payload",
		"Codex " + "flush guard",
		"memory-" + "flush fallback fixtures",
	}

	for _, file := range files {
		content := readRepositoryTextFile(t, file)
		for _, pattern := range forbidden {
			if strings.Contains(content, pattern) {
				t.Fatalf("%s still contains deleted flush donor language %q", file, pattern)
			}
		}
	}
}

func readRepositoryTextFile(t *testing.T, slashPath string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(slashPath)))
	if err != nil {
		t.Fatalf("read %s: %v", slashPath, err)
	}
	return string(raw)
}
