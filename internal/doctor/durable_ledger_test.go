package doctor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/subagent"
)

func TestCheckDurableLedgerReportsAppendOnlyRunLogDegradedMode(t *testing.T) {
	result := CheckDurableLedger(context.Background(), nil, "/tmp/gormes-subagents.jsonl")

	if result.Status != StatusWarn {
		t.Fatalf("Status = %v, want WARN", result.Status)
	}
	if !strings.Contains(result.Summary, "append-only run logs") {
		t.Fatalf("Summary = %q, want append-only run logs", result.Summary)
	}
	if !strings.Contains(result.Summary, "restart/replay unavailable") {
		t.Fatalf("Summary = %q, want restart/replay unavailable", result.Summary)
	}
}

func TestCheckDurableLedgerReportsReplayAvailable(t *testing.T) {
	ms, err := memory.OpenSqlite(filepath.Join(t.TempDir(), "ledger.db"), 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer ms.Close(context.Background())
	ledger, err := subagent.NewDurableLedger(ms.DB())
	if err != nil {
		t.Fatalf("NewDurableLedger: %v", err)
	}

	if _, err := ledger.Submit(context.Background(), subagent.DurableJobSubmission{
		ID:   "job-1",
		Kind: subagent.WorkKindCronJob,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	result := CheckDurableLedger(context.Background(), ledger, "")

	if result.Status != StatusPass {
		t.Fatalf("Status = %v, want PASS: %+v", result.Status, result)
	}
	if !strings.Contains(result.Summary, "restart/replay available") {
		t.Fatalf("Summary = %q, want restart/replay available", result.Summary)
	}
}
