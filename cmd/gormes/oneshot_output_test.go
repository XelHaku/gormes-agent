package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/spf13/cobra"
)

func TestOneshotFinalOutput_PrintsOnlyFinalAssistantContent(t *testing.T) {
	setupOneshotFlagTestEnv(t)

	mock := hermes.NewMockClient()
	mock.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: "final "},
		{Kind: hermes.EventToken, Token: "answer"},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 3, TokensOut: 2},
	}, "sess-must-not-enter-stdout")

	var clientCalls int
	cmd := newRootCommandWithRuntime(rootRuntime{
		runTUI: func(*cobra.Command, []string) error {
			t.Fatal("runTUI was called for oneshot")
			return nil
		},
		newOneshotClient: func(_ context.Context, cfg config.Config, invocation oneshotInvocation) (hermes.Client, error) {
			clientCalls++
			if cfg.Hermes.Endpoint == "" {
				t.Fatal("oneshot client factory received empty endpoint")
			}
			if invocation.Prompt != "hi" {
				t.Fatalf("invocation prompt = %q, want hi", invocation.Prompt)
			}
			if invocation.Inference.Model != "fixture-model" {
				t.Fatalf("invocation model = %q, want fixture-model", invocation.Inference.Model)
			}
			return mock, nil
		},
	})

	stdout, stderr, err := executeOneshotFlagCommand(cmd, "-z", "hi", "--model", "fixture-model")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if clientCalls != 1 {
		t.Fatalf("oneshot client factory calls = %d, want 1", clientCalls)
	}
	if stdout != "final answer\n" {
		t.Fatalf("stdout = %q, want exactly final assistant content plus newline", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty on successful oneshot", stderr)
	}
	for _, forbidden := range []string{
		"sess-must-not-enter-stdout",
		"Idle",
		"Connecting",
		"Streaming",
		"api_server",
		"gateway",
		"tool",
		"session",
	} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout leaked %q: %q", forbidden, stdout)
		}
	}

	requests := mock.Requests()
	if len(requests) != 1 {
		t.Fatalf("OpenStream requests = %d, want one native kernel turn", len(requests))
	}
	req := requests[0]
	if req.Model != "fixture-model" {
		t.Fatalf("ChatRequest.Model = %q, want fixture-model", req.Model)
	}
	if !req.Stream {
		t.Fatal("ChatRequest.Stream = false, want true")
	}
	if len(req.Tools) != 0 {
		t.Fatalf("ChatRequest.Tools length = %d, want no tools in output-boundary fixture", len(req.Tools))
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" || req.Messages[0].Content != "hi" {
		t.Fatalf("ChatRequest.Messages = %#v, want one user prompt", req.Messages)
	}
}

func TestOneshotFinalOutput_SetupFailureUsesStderrAndNonzeroExit(t *testing.T) {
	setupOneshotFlagTestEnv(t)

	cmd := newRootCommandWithRuntime(rootRuntime{
		runTUI: func(*cobra.Command, []string) error {
			t.Fatal("runTUI was called for oneshot setup failure")
			return nil
		},
		newOneshotClient: func(context.Context, config.Config, oneshotInvocation) (hermes.Client, error) {
			return nil, errors.New("fixture provider unavailable")
		},
	})

	stdout, stderr, err := executeOneshotFlagCommand(cmd, "-z", "hi", "--model", "fixture-model")
	if err == nil {
		t.Fatalf("Execute() error = nil, want setup failure\nstdout=%s\nstderr=%s", stdout, stderr)
	}
	if code := exitCodeFromError(err); code != 1 {
		t.Fatalf("exit code = %d, want 1 (err=%v)", code, err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty on setup failure", stdout)
	}
	for _, want := range []string{
		"gormes -z: provider setup failed",
		"fixture provider unavailable",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q\nstderr=%s", want, stderr)
		}
	}
	for _, forbidden := range []string{"api_server", "gateway", "session_id", "sess-"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout leaked %q on setup failure: %q", forbidden, stdout)
		}
	}
}
