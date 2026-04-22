package subagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type scriptedRunner struct {
	result *SubagentResult
}

func (r scriptedRunner) Run(ctx context.Context, cfg SubagentConfig, events chan<- SubagentEvent) *SubagentResult {
	select {
	case events <- SubagentEvent{Type: EventStarted, Message: cfg.Goal}:
	case <-ctx.Done():
		return &SubagentResult{Status: StatusInterrupted, ExitReason: "ctx_cancelled"}
	}
	select {
	case events <- SubagentEvent{Type: EventCompleted, Message: "scripted"}:
	case <-ctx.Done():
		return &SubagentResult{Status: StatusInterrupted, ExitReason: "ctx_cancelled"}
	}
	if r.result != nil {
		return r.result
	}
	return &SubagentResult{Status: StatusCompleted, Summary: cfg.Goal, ExitReason: "scripted"}
}

type recordingCandidateDrafter struct {
	calls int
	got   CandidateDraftRequest
	err   error
}

func (r *recordingCandidateDrafter) DraftCandidate(_ context.Context, req CandidateDraftRequest) (string, error) {
	r.calls++
	r.got = req
	if r.err != nil {
		return "", r.err
	}
	return "cand_123", nil
}

func TestDelegateToolMetadata(t *testing.T) {
	tool := NewDelegateTool(nil, nil)
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

	tool := NewDelegateTool(mgr, nil)
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

func TestDelegateToolUsesManagerDefaultTimeout(t *testing.T) {
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager(ManagerOpts{
		ParentCtx:            parentCtx,
		ParentID:             "parent_test",
		Depth:                0,
		Registry:             NewRegistry(),
		NewRunner:            func() Runner { return blockingRunner{} },
		DefaultTimeout:       25 * time.Millisecond,
		DefaultMaxIterations: DefaultMaxIterations,
	})
	defer mgr.Close()

	tool := NewDelegateTool(mgr, nil)
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"timed child"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output JSON: %v", err)
	}
	if got["status"] != "interrupted" {
		t.Fatalf("status: want %q, got %v", "interrupted", got["status"])
	}
	if got["exit_reason"] != "timeout" {
		t.Fatalf("exit_reason: want %q, got %v", "timeout", got["exit_reason"])
	}
}

func TestDelegateToolIncludesTypedToolCallAuditInOutput(t *testing.T) {
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager(ManagerOpts{
		ParentCtx: parentCtx,
		ParentID:  "parent_test",
		Depth:     0,
		Registry:  NewRegistry(),
		NewRunner: func() Runner {
			return scriptedRunner{result: &SubagentResult{
				Status:     StatusCompleted,
				Summary:    "tool audited",
				ExitReason: "scripted",
				ToolCalls: []ToolCallInfo{{
					Name:       "echo",
					ArgsBytes:  27,
					ResultSize: 27,
					Status:     "completed",
				}},
			}}
		},
		DefaultTimeout:       time.Second,
		DefaultMaxIterations: DefaultMaxIterations,
	})
	defer mgr.Close()

	tool := NewDelegateTool(mgr, nil)
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"research X"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got struct {
		ToolCalls []ToolCallInfo `json:"tool_calls"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output JSON: %v", err)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("tool_calls len = %d, want 1", len(got.ToolCalls))
	}
	if got.ToolCalls[0].Name != "echo" {
		t.Fatalf("tool_calls[0].Name = %q, want %q", got.ToolCalls[0].Name, "echo")
	}
	if got.ToolCalls[0].Status != "completed" {
		t.Fatalf("tool_calls[0].Status = %q, want %q", got.ToolCalls[0].Status, "completed")
	}
}

func TestDelegateToolMissingGoal(t *testing.T) {
	mgr, _, cancel := newStubManager(t, 0)
	defer cancel()
	defer mgr.Close()

	tool := NewDelegateTool(mgr, nil)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Errorf("Execute: want error for missing goal, got nil")
	}
}

func TestDelegateToolInvalidArgs(t *testing.T) {
	mgr, _, cancel := newStubManager(t, 0)
	defer cancel()
	defer mgr.Close()

	tool := NewDelegateTool(mgr, nil)
	_, err := tool.Execute(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Errorf("Execute: want error for invalid JSON, got nil")
	}
}

func TestDelegateToolToolsetsParsing(t *testing.T) {
	mgr, _, cancel := newStubManager(t, 0)
	defer cancel()
	defer mgr.Close()

	tool := NewDelegateTool(mgr, nil)
	_, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"x","toolsets":"a,b , c"}`))
	if err != nil {
		t.Errorf("Execute with toolsets: %v", err)
	}
}

func TestDelegateToolDraftsCandidateOnSuccessfulRun(t *testing.T) {
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager(ManagerOpts{
		ParentCtx: parentCtx,
		ParentID:  "parent_test",
		Depth:     0,
		Registry:  NewRegistry(),
		NewRunner: func() Runner {
			return scriptedRunner{result: &SubagentResult{
				Status:     StatusCompleted,
				Summary:    "review completed successfully",
				ExitReason: "scripted",
				ToolCalls:  []ToolCallInfo{{Name: "echo", Status: "ok"}},
			}}
		},
		DefaultTimeout:       time.Second,
		DefaultMaxIterations: DefaultMaxIterations,
	})
	defer mgr.Close()

	drafter := &recordingCandidateDrafter{}
	tool := NewDelegateTool(mgr, drafter)
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"review patch","draft_candidate_slug":"review-patch"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if drafter.calls != 1 {
		t.Fatalf("drafter calls = %d, want 1", drafter.calls)
	}
	if drafter.got.Slug != "review-patch" {
		t.Fatalf("drafter slug = %q, want %q", drafter.got.Slug, "review-patch")
	}
	if drafter.got.Summary != "review completed successfully" {
		t.Fatalf("drafter summary = %q", drafter.got.Summary)
	}
	if len(drafter.got.ToolNames) != 1 || drafter.got.ToolNames[0] != "echo" {
		t.Fatalf("drafter tool names = %#v, want [echo]", drafter.got.ToolNames)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output JSON: %v", err)
	}
	if got["candidate_id"] != "cand_123" {
		t.Fatalf("candidate_id = %v, want cand_123", got["candidate_id"])
	}
}

func TestDelegateToolSkipsCandidateDraftWithoutToolsUnlessOverridden(t *testing.T) {
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager(ManagerOpts{
		ParentCtx: parentCtx,
		ParentID:  "parent_test",
		Depth:     0,
		Registry:  NewRegistry(),
		NewRunner: func() Runner {
			return scriptedRunner{result: &SubagentResult{
				Status:     StatusCompleted,
				Summary:    "review completed successfully",
				ExitReason: "scripted",
			}}
		},
		DefaultTimeout:       time.Second,
		DefaultMaxIterations: DefaultMaxIterations,
	})
	defer mgr.Close()

	drafter := &recordingCandidateDrafter{}
	tool := NewDelegateTool(mgr, drafter)
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"review patch","draft_candidate_slug":"review-patch"}`)); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if drafter.calls != 0 {
		t.Fatalf("drafter calls = %d, want 0", drafter.calls)
	}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"review patch","draft_candidate_slug":"review-patch","allow_no_tool_draft":true}`)); err != nil {
		t.Fatalf("Execute with override: %v", err)
	}
	if drafter.calls != 1 {
		t.Fatalf("drafter calls after override = %d, want 1", drafter.calls)
	}
}

func TestDelegateToolSurfacesBlockedToolRequest(t *testing.T) {
	mgr, _, cancel := newStubManager(t, 0)
	defer cancel()
	defer mgr.Close()

	tool := NewDelegateTool(mgr, nil)
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"research X","toolsets":"delegate_task"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output JSON: %v", err)
	}
	if got["status"] != "failed" {
		t.Fatalf("status: want %q, got %v", "failed", got["status"])
	}
	if got["exit_reason"] != "blocked_tool_request" {
		t.Fatalf("exit_reason: want %q, got %v", "blocked_tool_request", got["exit_reason"])
	}
	if got["error"] == "" {
		t.Fatal("error: want non-empty blocked-tool message")
	}
}
