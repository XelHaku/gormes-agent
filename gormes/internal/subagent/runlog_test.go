package subagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerAppendsRunLogRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subagents", "runs.jsonl")

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager(ManagerOpts{
		ParentCtx:  parentCtx,
		ParentID:   "parent_test",
		Depth:      0,
		Registry:   NewRegistry(),
		NewRunner:  func() Runner { return StubRunner{} },
		RunLogPath: path,
	})
	defer mgr.Close()

	sa, err := mgr.Spawn(context.Background(), SubagentConfig{Goal: "log me"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	result, err := sa.WaitForResult(context.Background())
	if err != nil {
		t.Fatalf("WaitForResult: %v", err)
	}
	if result.Status != StatusCompleted {
		t.Fatalf("Status: want %q, got %q", StatusCompleted, result.Status)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}

	var rec struct {
		ID         string       `json:"id"`
		ParentID   string       `json:"parent_id"`
		Goal       string       `json:"goal"`
		Status     ResultStatus `json:"status"`
		ExitReason string       `json:"exit_reason"`
		DurationMs int64        `json:"duration_ms"`
		FinishedAt time.Time    `json:"finished_at"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("Unmarshal run log: %v\nraw=%s", err, raw)
	}
	if rec.ID != result.ID {
		t.Fatalf("ID: want %q, got %q", result.ID, rec.ID)
	}
	if rec.ParentID != "parent_test" {
		t.Fatalf("ParentID: want %q, got %q", "parent_test", rec.ParentID)
	}
	if rec.Goal != "log me" {
		t.Fatalf("Goal: want %q, got %q", "log me", rec.Goal)
	}
	if rec.Status != StatusCompleted {
		t.Fatalf("Status: want %q, got %q", StatusCompleted, rec.Status)
	}
	if rec.ExitReason != "stub_runner_no_llm_yet" {
		t.Fatalf("ExitReason: want %q, got %q", "stub_runner_no_llm_yet", rec.ExitReason)
	}
	if rec.DurationMs < 0 {
		t.Fatalf("DurationMs: want >= 0, got %d", rec.DurationMs)
	}
	if rec.FinishedAt.IsZero() {
		t.Fatal("FinishedAt: want non-zero timestamp")
	}
}

func TestManagerRunLogFailureDoesNotBlockCompletion(t *testing.T) {
	dir := t.TempDir()

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewManager(ManagerOpts{
		ParentCtx:  parentCtx,
		ParentID:   "parent_test",
		Depth:      0,
		Registry:   NewRegistry(),
		NewRunner:  func() Runner { return StubRunner{} },
		RunLogPath: dir,
	})
	defer mgr.Close()

	sa, err := mgr.Spawn(context.Background(), SubagentConfig{Goal: "log failure still completes"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer waitCancel()

	result, err := sa.WaitForResult(waitCtx)
	if err != nil {
		t.Fatalf("WaitForResult: %v", err)
	}
	if result.Status != StatusCompleted {
		t.Fatalf("Status: want %q, got %q", StatusCompleted, result.Status)
	}
}
