package acp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/tools"
)

func TestKernelSession_PromptStreamsMockClientResponse(t *testing.T) {
	mock := hermes.NewMockClient()
	mock.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "hello"},
		{Kind: hermes.EventToken, Token: " ACP"},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 3, TokensOut: 2},
	}, "provider-session")

	factory := NewKernelSessionFactory(KernelSessionFactoryOptions{
		Model: "gormes-test",
		ClientFactory: func() hermes.Client {
			return mock
		},
		RegistryFactory: func(_ hermes.Client) *tools.Registry {
			return tools.NewRegistry()
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := factory.NewSession(ctx, "/tmp/project")
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	defer session.Close()

	var chunks []string
	result, err := session.Prompt(ctx, []ContentBlock{{Type: "text", Text: "Say hi"}}, func(update SessionUpdate) {
		chunks = append(chunks, update.Content.Text)
	})
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	if result.StopReason != StopReasonEndTurn {
		t.Fatalf("stop reason = %q, want %q", result.StopReason, StopReasonEndTurn)
	}
	if got := strings.Join(chunks, ""); got != "hello ACP" {
		t.Fatalf("streamed text = %q, want %q", got, "hello ACP")
	}
}
