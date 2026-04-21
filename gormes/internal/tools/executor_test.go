package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestInProcessExecutorEchoTool(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&EchoTool{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	exec := NewInProcessToolExecutor(reg)

	ch, err := exec.Execute(context.Background(), ToolRequest{
		ToolName: "echo",
		Input:    json.RawMessage(`{"text":"hi"}`),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got []ToolEvent
	for ev := range ch {
		got = append(got, ev)
	}
	if len(got) != 3 {
		t.Fatalf("event count: want 3, got %d (%+v)", len(got), got)
	}
	if got[0].Type != "started" || got[1].Type != "output" || got[2].Type != "completed" {
		t.Errorf("event sequence: want started→output→completed, got %s→%s→%s", got[0].Type, got[1].Type, got[2].Type)
	}
	if !strings.Contains(string(got[1].Output), `"hi"`) {
		t.Errorf("output payload: want contains \"hi\", got %s", got[1].Output)
	}
}

func TestInProcessExecutorUnknownTool(t *testing.T) {
	reg := NewRegistry()
	exec := NewInProcessToolExecutor(reg)

	_, err := exec.Execute(context.Background(), ToolRequest{ToolName: "nope"})
	if err == nil {
		t.Fatal("Execute: want error, got nil")
	}
	if !errors.Is(err, ErrUnknownTool) {
		t.Errorf("err: want ErrUnknownTool, got %v", err)
	}
}

func TestInProcessExecutorToolErrorEmitsFailed(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&EchoTool{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	exec := NewInProcessToolExecutor(reg)

	ch, err := exec.Execute(context.Background(), ToolRequest{
		ToolName: "echo",
		Input:    json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("Execute (registration error path is separate): %v", err)
	}

	var got []ToolEvent
	for ev := range ch {
		got = append(got, ev)
	}
	if len(got) != 2 {
		t.Fatalf("event count: want 2 (started+failed), got %d (%+v)", len(got), got)
	}
	if got[0].Type != "started" || got[1].Type != "failed" {
		t.Errorf("event sequence: want started→failed, got %s→%s", got[0].Type, got[1].Type)
	}
	if got[1].Err == nil {
		t.Errorf("failed event Err: want non-nil")
	}
}
