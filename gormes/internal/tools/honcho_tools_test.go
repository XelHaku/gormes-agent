package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/goncho"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/memory"
)

func TestHonchoTools_RegisterExpectedNames(t *testing.T) {
	reg, _, cleanup := newTestHonchoRegistry(t)
	defer cleanup()

	for _, name := range []string{
		"honcho_profile",
		"honcho_search",
		"honcho_context",
		"honcho_reasoning",
		"honcho_conclude",
	} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("%s not registered", name)
		}
	}
}

func TestHonchoProfileTool_UsesService(t *testing.T) {
	reg, svc, cleanup := newTestHonchoRegistry(t)
	defer cleanup()

	ctx := context.Background()
	if err := svc.SetProfile(ctx, "telegram:6586915095", []string{"Blind", "Prefers exact outputs"}); err != nil {
		t.Fatal(err)
	}

	exec := NewInProcessToolExecutor(reg)
	ch, err := exec.Execute(ctx, ToolRequest{
		ToolName: "honcho_profile",
		Input:    json.RawMessage(`{"peer":"telegram:6586915095"}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	var outputs []ToolEvent
	for ev := range ch {
		outputs = append(outputs, ev)
	}
	if len(outputs) != 3 {
		t.Fatalf("event count = %d, want 3", len(outputs))
	}
	if !strings.Contains(string(outputs[1].Output), `"Prefers exact outputs"`) {
		t.Fatalf("profile output = %s", outputs[1].Output)
	}
}

func TestHonchoReasoningTool_ReturnsDeterministicAnswer(t *testing.T) {
	reg, svc, cleanup := newTestHonchoRegistry(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := svc.Conclude(ctx, goncho.ConcludeParams{
		Peer:       "telegram:6586915095",
		Conclusion: "The user prefers exact evidence-first reports.",
		SessionKey: "telegram:6586915095",
	}); err != nil {
		t.Fatal(err)
	}

	exec := NewInProcessToolExecutor(reg)
	ch, err := exec.Execute(ctx, ToolRequest{
		ToolName: "honcho_reasoning",
		Input: json.RawMessage(`{
			"peer":"telegram:6586915095",
			"query":"How should I answer?",
			"reasoning_level":"low",
			"session_key":"telegram:6586915095"
		}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	var outputs []ToolEvent
	for ev := range ch {
		outputs = append(outputs, ev)
	}
	if len(outputs) != 3 {
		t.Fatalf("event count = %d, want 3", len(outputs))
	}
	if !strings.Contains(string(outputs[1].Output), `"answer"`) {
		t.Fatalf("reasoning output = %s", outputs[1].Output)
	}
	if !strings.Contains(string(outputs[1].Output), `exact evidence-first reports`) {
		t.Fatalf("reasoning output missing conclusion: %s", outputs[1].Output)
	}
}

func newTestHonchoRegistry(t *testing.T) (*Registry, *goncho.Service, func()) {
	t.Helper()

	store, err := memory.OpenSqlite(t.TempDir()+"/memory.db", 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}

	reg := NewRegistry()
	svc := goncho.NewService(store.DB(), goncho.Config{
		WorkspaceID:    "default",
		ObserverPeerID: "gormes",
		RecentMessages: 4,
	}, nil)
	RegisterHonchoTools(reg, svc)

	return reg, svc, func() {
		if err := store.Close(context.Background()); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}
