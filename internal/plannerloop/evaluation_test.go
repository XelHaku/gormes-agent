package plannerloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvaluate_UnstuckRowDetected(t *testing.T) {
	dir := t.TempDir()
	plannerLedger := filepath.Join(dir, "planner.jsonl")
	autoloopLedger := filepath.Join(dir, "autoloop.jsonl")

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	reshapeTS := now.Add(-2 * time.Hour)

	// Planner reshaped row.
	if err := AppendLedgerEvent(plannerLedger, LedgerEvent{
		TS: reshapeTS.Format(time.RFC3339), RunID: "planner-1", Status: "ok",
		RowsChanged: []RowChange{{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-1", Kind: "spec_changed"}},
	}); err != nil {
		t.Fatal(err)
	}

	// Autoloop later promoted the same row.
	autoloopEvent := map[string]any{
		"ts":     now.Add(-1 * time.Hour).Format(time.RFC3339),
		"event":  "worker_promoted",
		"task":   "2/2.B/row-1",
		"status": "promoted",
	}
	appendLineJSON(t, autoloopLedger, autoloopEvent)

	outcomes, err := Evaluate(plannerLedger, autoloopLedger, 7*24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].Outcome != "unstuck" {
		t.Fatalf("expected unstuck, got %q", outcomes[0].Outcome)
	}
	if outcomes[0].LastSuccess == "" {
		t.Fatalf("expected LastSuccess to be set, got empty")
	}
}

func TestEvaluate_StillFailingDetected(t *testing.T) {
	dir := t.TempDir()
	plannerLedger := filepath.Join(dir, "planner.jsonl")
	autoloopLedger := filepath.Join(dir, "autoloop.jsonl")
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	reshapeTS := now.Add(-2 * time.Hour)

	if err := AppendLedgerEvent(plannerLedger, LedgerEvent{
		TS: reshapeTS.Format(time.RFC3339), RunID: "planner-1", Status: "ok",
		RowsChanged: []RowChange{{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-1", Kind: "spec_changed"}},
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		appendLineJSON(t, autoloopLedger, map[string]any{
			"ts":     now.Add(-time.Duration(60-i*10) * time.Minute).Format(time.RFC3339),
			"event":  "worker_failed",
			"task":   "2/2.B/row-1",
			"status": "failed",
		})
	}

	outcomes, err := Evaluate(plannerLedger, autoloopLedger, 7*24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].Outcome != "still_failing" {
		t.Fatalf("expected still_failing, got %q", outcomes[0].Outcome)
	}
	if outcomes[0].AutoloopRuns != 3 {
		t.Fatalf("expected AutoloopRuns=3, got %d", outcomes[0].AutoloopRuns)
	}
	if outcomes[0].LastFailure == "" {
		t.Fatalf("expected LastFailure to be set")
	}
}

func TestEvaluate_NoAttemptsYet(t *testing.T) {
	dir := t.TempDir()
	plannerLedger := filepath.Join(dir, "planner.jsonl")
	autoloopLedger := filepath.Join(dir, "autoloop.jsonl")
	now := time.Now().UTC()

	if err := AppendLedgerEvent(plannerLedger, LedgerEvent{
		TS: now.Add(-time.Hour).Format(time.RFC3339), RunID: "planner-1", Status: "ok",
		RowsChanged: []RowChange{{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-1", Kind: "spec_changed"}},
	}); err != nil {
		t.Fatal(err)
	}
	// No autoloop ledger entries written; the file simply does not exist.

	outcomes, err := Evaluate(plannerLedger, autoloopLedger, 7*24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].Outcome != "no_attempts_yet" {
		t.Fatalf("expected no_attempts_yet, got %q", outcomes[0].Outcome)
	}
}

// appendLineJSON appends one JSON-encoded map to an autoloop-style ledger.
// The autoloop ledger schema differs from the planner's; we use the same
// O_APPEND pattern for compatibility.
func appendLineJSON(t *testing.T, path string, obj map[string]any) {
	t.Helper()
	body, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(body); err != nil {
		t.Fatal(err)
	}
}
