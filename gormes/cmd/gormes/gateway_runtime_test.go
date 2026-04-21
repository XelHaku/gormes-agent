package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/config"
)

func testGatewayConfig() config.Config {
	return config.Config{
		Hermes: config.HermesCfg{
			Endpoint: "http://127.0.0.1:8642",
			Model:    "hermes-agent",
		},
		Input: config.InputCfg{
			MaxBytes: 200_000,
			MaxLines: 10_000,
		},
		Telegram: config.TelegramCfg{
			MemoryQueueCap:         8,
			ExtractorBatchSize:     1,
			ExtractorPollInterval:  10 * time.Second,
			RecallEnabled:          true,
			RecallWeightThreshold:  1.0,
			RecallMaxFacts:         10,
			RecallDepth:            2,
			RecallDecayHorizonDays: 180,
			SemanticEnabled:        false,
			QueryEmbedTimeout:      60 * time.Millisecond,
		},
	}
}

func TestOpenGatewayRuntime_AppliesResumeOverride(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	rt, err := openGatewayRuntime(testGatewayConfig(), gatewayRuntimeOptions{
		ChatKey:        "discord:chan-1",
		ResumeOverride: "sess-123",
		RecallEnabled:  true,
	}, slog.Default())
	if err != nil {
		t.Fatalf("openGatewayRuntime: %v", err)
	}
	defer rt.Close(context.Background())

	if rt.chatKey != "discord:chan-1" {
		t.Errorf("chatKey = %q, want discord:chan-1", rt.chatKey)
	}
	if rt.initialSID != "sess-123" {
		t.Errorf("initialSID = %q, want sess-123", rt.initialSID)
	}
}

func TestOpenGatewayRuntime_DisablesRecallWhenChatKeyEmpty(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	rt, err := openGatewayRuntime(testGatewayConfig(), gatewayRuntimeOptions{
		ChatKey:       "",
		RecallEnabled: true,
	}, slog.Default())
	if err != nil {
		t.Fatalf("openGatewayRuntime: %v", err)
	}
	defer rt.Close(context.Background())

	if rt.recallActive {
		t.Error("recallActive = true, want false when chatKey is empty")
	}
}
