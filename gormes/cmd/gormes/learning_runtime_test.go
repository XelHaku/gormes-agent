package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/learning"
)

func TestConfiguredLearningRuntimeWritesToDefaultPath(t *testing.T) {
	dataHome := t.TempDir()
	configHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	cfg, err := config.Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	runtime := configuredLearningRuntime(cfg)
	if runtime == nil {
		t.Fatal("configuredLearningRuntime() = nil, want runtime")
	}

	if _, err := runtime.RecordTurn(context.Background(), learning.Turn{
		SessionID:        "sess-runtime",
		UserMessage:      "inspect the failing gateway flow",
		AssistantMessage: "I traced it and verified the fix with two tool-backed checks.",
		ToolNames:        []string{"read_file", "run_tests"},
		TokensIn:         180,
		TokensOut:        150,
		Duration:         5 * time.Second,
		FinishedAt:       time.Date(2026, 4, 23, 19, 2, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("RecordTurn() error = %v", err)
	}

	path := filepath.Join(dataHome, "gormes", "learning", "complexity.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("learning signal log missing at %q: %v", path, err)
	}
}
