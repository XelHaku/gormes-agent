package subagent

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestDelegateToolCandidateDraftFailureIsNonFatal(t *testing.T) {
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

	drafter := &recordingCandidateDrafter{err: context.DeadlineExceeded}
	tool := NewDelegateTool(mgr, drafter)
	out, err := tool.Execute(context.Background(), json.RawMessage(`{"goal":"review patch","draft_candidate_slug":"review-patch"}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output JSON: %v", err)
	}
	if got["status"] != "completed" {
		t.Fatalf("status = %v, want completed", got["status"])
	}
	if got["candidate_error"] == nil {
		t.Fatal("candidate_error = nil, want surfaced drafting failure")
	}
}
