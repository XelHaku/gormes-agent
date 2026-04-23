package kernel

import (
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/contextengine"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/telemetry"
)

func TestKernel_BuildChatRequestTrimsOldestTurnsWhenThresholdExceeded(t *testing.T) {
	k := New(Config{
		Model:         "hermes-agent",
		Endpoint:      "http://mock",
		Admission:     Admission{MaxBytes: 200_000, MaxLines: 10_000},
		ContextEngine: contextengine.NewCompressor(contextengine.Config{ContextLength: 128_000}),
	}, hermes.NewMockClient(), store.NewNoop(), telemetry.New(), nil)

	k.history = []hermes.Message{
		{Role: "user", Content: strings.Repeat("old-user-", 7_500)},
		{Role: "assistant", Content: strings.Repeat("old-assistant-", 4_500)},
		{Role: "user", Content: "current question"},
	}

	req := k.buildChatRequest([]hermes.Message{{Role: "system", Content: "system guardrails"}})
	if len(req.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2 (system + current user)", len(req.Messages))
	}
	if got := req.Messages[0]; got.Role != "system" || got.Content != "system guardrails" {
		t.Fatalf("Messages[0] = %+v, want system guardrails", got)
	}
	if got := req.Messages[1]; got.Role != "user" || got.Content != "current question" {
		t.Fatalf("Messages[1] = %+v, want current user only", got)
	}
}
