package subagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDelegateToolMetadata(t *testing.T) {
	tool := NewDelegateTool(nil)
	if tool.Name() != "delegate_task" {
		t.Errorf("Name: want %q, got %q", "delegate_task", tool.Name())
	}
	if tool.Description() == "" {
		t.Errorf("Description: want non-empty")
	}
	if tool.Timeout() != 0 {
		t.Errorf("Timeout: want 0 (governed by subagent timeout), got %v", tool.Timeout())
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Errorf("Schema: invalid JSON: %v", err)
	}
}

func TestDelegateToolExecuteHappyPath(t *testing.T) {
	mgr, _, cancel := newStubManager(t, 0)
	defer cancel()
	defer mgr.Close()

	tool := NewDelegateTool(mgr)
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"research X","context":"channels only"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output JSON: %v", err)
	}
	if got["status"] != "completed" {
		t.Errorf("status: want %q, got %v", "completed", got["status"])
	}
	if got["summary"] != "research X" {
		t.Errorf("summary: want %q, got %v", "research X", got["summary"])
	}
	if got["exit_reason"] != "stub_runner_no_llm_yet" {
		t.Errorf("exit_reason: want %q, got %v", "stub_runner_no_llm_yet", got["exit_reason"])
	}
	id, _ := got["id"].(string)
	if !strings.HasPrefix(id, "sa_") {
		t.Errorf("id: want %q-prefixed, got %v", "sa_", got["id"])
	}
}

func TestDelegateToolMissingGoal(t *testing.T) {
	mgr, _, cancel := newStubManager(t, 0)
	defer cancel()
	defer mgr.Close()

	tool := NewDelegateTool(mgr)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Errorf("Execute: want error for missing goal, got nil")
	}
}

func TestDelegateToolInvalidArgs(t *testing.T) {
	mgr, _, cancel := newStubManager(t, 0)
	defer cancel()
	defer mgr.Close()

	tool := NewDelegateTool(mgr)
	_, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Errorf("Execute: want error for invalid JSON, got nil")
	}
}

func TestDelegateToolToolsetsParsing(t *testing.T) {
	mgr, _, cancel := newStubManager(t, 0)
	defer cancel()
	defer mgr.Close()

	tool := NewDelegateTool(mgr)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"x","toolsets":"a,b , c"}`))
	if err != nil {
		t.Errorf("Execute with toolsets: %v", err)
	}
}
