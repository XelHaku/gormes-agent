package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/audit"
	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
	"github.com/TrebuchetDynamics/gormes-agent/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/internal/tools"
	"github.com/spf13/cobra"
)

func TestOneshotSafety_ClarifyToolCallReturnsNoninteractiveAssumptionWithoutStdoutLeak(t *testing.T) {
	setupOneshotFlagTestEnv(t)

	mock := hermes.NewMockClient()
	mock.Script([]hermes.Event{{
		Kind:         hermes.EventDone,
		FinishReason: "tool_calls",
		ToolCalls: []hermes.ToolCall{{
			ID:        "call_clarify",
			Name:      "clarify",
			Arguments: json.RawMessage(`{"question":"Pick deployment region","choices":["us-east","eu-west"]}`),
		}},
	}}, "sess-clarify")
	scriptOneshotFinal(mock, "continued with assumption")

	cmd := newRootCommandWithRuntime(rootRuntime{
		runTUI: func(*cobra.Command, []string) error {
			t.Fatal("runTUI was called for oneshot")
			return nil
		},
		newOneshotClient: func(context.Context, config.Config, oneshotInvocation) (hermes.Client, error) {
			return mock, nil
		},
	})

	stdout, stderr, err := executeOneshotFlagCommand(cmd, "-z", "deploy?", "--model", "fixture-model")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if stdout != "continued with assumption\n" {
		t.Fatalf("stdout = %q, want final assistant content only", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty on successful oneshot", stderr)
	}
	for _, forbidden := range []string{"clarify_unavailable", "Pick deployment region", "call_clarify", "sess-clarify"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout leaked %q: %q", forbidden, stdout)
		}
	}

	requests := mock.Requests()
	if len(requests) != 2 {
		t.Fatalf("OpenStream requests = %d, want tool round plus final round", len(requests))
	}
	toolMsg := requireToolMessage(t, requests[1].Messages, "clarify")
	for _, want := range []string{
		`"status":"clarify_unavailable"`,
		`"noninteractive":true`,
		`"trust_class":"operator"`,
		"Pick deployment region",
		"best option",
	} {
		if !strings.Contains(toolMsg.Content, want) {
			t.Fatalf("clarify tool content missing %q\ncontent=%s", want, toolMsg.Content)
		}
	}

	records := readOneshotAuditRecords(t)
	if len(records) != 1 {
		t.Fatalf("audit record count = %d, want 1", len(records))
	}
	if records[0].Tool != "clarify" || records[0].Status != "clarify_unavailable" {
		t.Fatalf("audit[0] = tool:%q status:%q, want clarify/clarify_unavailable", records[0].Tool, records[0].Status)
	}
	if !strings.Contains(records[0].Error, "noninteractive") {
		t.Fatalf("audit error = %q, want noninteractive evidence", records[0].Error)
	}
}

func TestOneshotSafety_DangerousCommandBlockedAuditedAndToolNotExecuted(t *testing.T) {
	setupOneshotFlagTestEnv(t)

	mock := hermes.NewMockClient()
	mock.Script([]hermes.Event{{
		Kind:         hermes.EventDone,
		FinishReason: "tool_calls",
		ToolCalls: []hermes.ToolCall{{
			ID:        "call_exec",
			Name:      "execute_code",
			Arguments: json.RawMessage(`{"language":"sh","code":"rm -rf /tmp/gormes-oneshot-fixture"}`),
		}},
	}}, "sess-danger")
	scriptOneshotFinal(mock, "dangerous command was blocked")

	var executed bool
	reg := tools.NewRegistry()
	reg.MustRegister(&tools.MockTool{
		NameStr: "execute_code",
		ExecuteFn: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			executed = true
			return json.RawMessage(`{"executed":true}`), nil
		},
	})

	cmd := newRootCommandWithRuntime(rootRuntime{
		runTUI: func(*cobra.Command, []string) error {
			t.Fatal("runTUI was called for oneshot")
			return nil
		},
		newOneshotClient: func(context.Context, config.Config, oneshotInvocation) (hermes.Client, error) {
			return mock, nil
		},
		configureOneshotKernel: func(cfg *kernel.Config) {
			cfg.Tools = reg
		},
	})

	stdout, stderr, err := executeOneshotFlagCommand(cmd, "-z", "clean temp files", "--model", "fixture-model")
	if err != nil {
		t.Fatalf("Execute() error = %v\nstderr=%s\nstdout=%s", err, stderr, stdout)
	}
	if executed {
		t.Fatal("dangerous execute_code fake tool ran; want policy block before tool execution")
	}
	if stdout != "dangerous command was blocked\n" {
		t.Fatalf("stdout = %q, want final assistant content only", stdout)
	}
	for _, forbidden := range []string{"dangerous_command_blocked", "rm -rf", "call_exec", "sess-danger"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout leaked %q: %q", forbidden, stdout)
		}
	}

	requests := mock.Requests()
	if len(requests) != 2 {
		t.Fatalf("OpenStream requests = %d, want tool round plus final round", len(requests))
	}
	toolMsg := requireToolMessage(t, requests[1].Messages, "execute_code")
	for _, want := range []string{
		`"status":"dangerous_command_blocked"`,
		`"approval_mode":"default_block"`,
		`"noninteractive":true`,
		`"trust_class":"operator"`,
		"rm -rf /tmp/gormes-oneshot-fixture",
	} {
		if !strings.Contains(toolMsg.Content, want) {
			t.Fatalf("dangerous command tool content missing %q\ncontent=%s", want, toolMsg.Content)
		}
	}

	records := readOneshotAuditRecords(t)
	if len(records) != 1 {
		t.Fatalf("audit record count = %d, want 1", len(records))
	}
	if records[0].Tool != "execute_code" || records[0].Status != "dangerous_command_blocked" {
		t.Fatalf("audit[0] = tool:%q status:%q, want execute_code/dangerous_command_blocked", records[0].Tool, records[0].Status)
	}
	if !strings.Contains(records[0].Error, "approval") {
		t.Fatalf("audit error = %q, want approval evidence", records[0].Error)
	}
}

func TestOneshotSafety_ApprovalBypassPolicyIsOperatorLocal(t *testing.T) {
	for _, trustClass := range []kernel.TrustClass{
		kernel.TrustClassSystem,
		kernel.TrustClassGateway,
		kernel.TrustClassChildAgent,
	} {
		_, err := kernel.NewOneshotToolSafetyPolicy(kernel.OneshotToolSafetyOptions{
			TrustClass:     trustClass,
			ApprovalBypass: true,
		})
		if err == nil {
			t.Fatalf("approval bypass for trust class %q succeeded, want operator-local rejection", trustClass)
		}
	}

	operatorBypass, err := kernel.NewOneshotToolSafetyPolicy(kernel.OneshotToolSafetyOptions{
		TrustClass:     kernel.TrustClassOperator,
		ApprovalBypass: true,
	})
	if err != nil {
		t.Fatalf("operator-local approval bypass rejected: %v", err)
	}
	if operatorBypass.ApprovalMode() != "operator_local_bypass" {
		t.Fatalf("operator approval mode = %q, want operator_local_bypass", operatorBypass.ApprovalMode())
	}

	defaultPolicy, err := kernel.NewOneshotToolSafetyPolicy(kernel.OneshotToolSafetyOptions{
		TrustClass: kernel.TrustClassOperator,
	})
	if err != nil {
		t.Fatalf("default operator policy rejected: %v", err)
	}
	if defaultPolicy.ApprovalMode() != "default_block" {
		t.Fatalf("default approval mode = %q, want default_block", defaultPolicy.ApprovalMode())
	}
}

func scriptOneshotFinal(mock *hermes.MockClient, content string) {
	mock.Script([]hermes.Event{
		{Kind: hermes.EventToken, Token: content},
		{Kind: hermes.EventDone, FinishReason: "stop", TokensIn: 1, TokensOut: 1},
	}, "sess-final")
}

func requireToolMessage(t *testing.T, messages []hermes.Message, name string) hermes.Message {
	t.Helper()
	for _, msg := range messages {
		if msg.Role == "tool" && msg.Name == name {
			return msg
		}
	}
	t.Fatalf("tool message %q not found in %#v", name, messages)
	return hermes.Message{}
}

func readOneshotAuditRecords(t *testing.T) []audit.Record {
	t.Helper()
	raw, err := os.ReadFile(config.ToolAuditLogPath())
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", config.ToolAuditLogPath(), err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	out := make([]audit.Record, 0, len(lines))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var rec audit.Record
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("Unmarshal audit line %s: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}
