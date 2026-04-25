package architectureplanner

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

func TestSummarizeAutoloopAuditAggregatesRecentLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	events := []autoloop.LedgerEvent{
		{TS: now.Add(-2 * time.Hour), Event: "run_started", Status: "started"},
		{TS: now.Add(-90 * time.Minute), Event: "worker_claimed", Task: "2/2.B.4/WhatsApp identity", Status: "claimed"},
		{TS: now.Add(-80 * time.Minute), Event: "worker_failed", Task: "2/2.B.4/WhatsApp identity", Status: "worktree_dirty"},
		{TS: now.Add(-70 * time.Minute), Event: "worker_claimed", Task: "3/3.E.7/Memory scope", Status: "claimed"},
		{TS: now.Add(-60 * time.Minute), Event: "worker_promoted", Task: "3/3.E.7/Memory scope", Status: "promoted"},
		{TS: now.Add(-50 * time.Minute), Event: "worker_success", Task: "3/3.E.7/Memory scope", Status: "success"},
		{TS: now.Add(-8 * 24 * time.Hour), Event: "worker_failed", Task: "2/2.B.3/Old", Status: "backend_failed"},
	}
	for _, event := range events {
		if err := autoloop.AppendLedgerEvent(path, event); err != nil {
			t.Fatalf("AppendLedgerEvent() error = %v", err)
		}
	}

	audit, err := SummarizeAutoloopAudit(path, 7*24*time.Hour, now)
	if err != nil {
		t.Fatalf("SummarizeAutoloopAudit() error = %v", err)
	}

	if audit.Runs != 1 || audit.Claimed != 2 || audit.Failed != 1 || audit.Promoted != 1 || audit.Succeeded != 1 {
		t.Fatalf("audit counts = runs:%d claimed:%d failed:%d promoted:%d succeeded:%d", audit.Runs, audit.Claimed, audit.Failed, audit.Promoted, audit.Succeeded)
	}
	if got := audit.FailStatusCounts["worktree_dirty"]; got != 1 {
		t.Fatalf("worktree_dirty count = %d, want 1", got)
	}
	if got := audit.ProductivityPercent(); got != 50 {
		t.Fatalf("ProductivityPercent() = %d, want 50", got)
	}
	if len(audit.ToxicSubphases) != 1 || audit.ToxicSubphases[0].SubphaseID != "2/2.B.4" {
		t.Fatalf("ToxicSubphases = %#v, want 2/2.B.4", audit.ToxicSubphases)
	}
	if len(audit.RecentFailedTasks) != 1 || audit.RecentFailedTasks[0].Status != "worktree_dirty" {
		t.Fatalf("RecentFailedTasks = %#v, want dirty failed task", audit.RecentFailedTasks)
	}
}

func TestSummarizeAutoloopAuditMissingLedgerIsEmpty(t *testing.T) {
	audit, err := SummarizeAutoloopAudit(filepath.Join(t.TempDir(), "missing.jsonl"), time.Hour, time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("SummarizeAutoloopAudit() error = %v", err)
	}
	if audit.Runs != 0 || audit.Claimed != 0 || audit.ProductivityPercent() != 0 {
		t.Fatalf("audit = %#v, want empty summary", audit)
	}
}
