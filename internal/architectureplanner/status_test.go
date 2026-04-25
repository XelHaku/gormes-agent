package architectureplanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func TestSuggestedActionForCategory_TableDriven(t *testing.T) {
	cases := []struct {
		category string
		want     string
	}{
		{"report_validation_failed", "split into smaller rows or set contract_status=\"draft\""},
		{"worker_error", "investigate infrastructure (backend or worktree state)"},
		{"backend_degraded", "investigate infrastructure (backend or worktree state)"},
		{"progress_summary_failed", "manual contract review — autoloop preflight is failing"},
		{"timeout", "split into smaller rows; the work is too large for the worker budget"},
		{"", "manual review"},
		{"unknown_category", "manual review"},
	}
	for _, c := range cases {
		got := SuggestedActionForCategory(c.category)
		if !strings.Contains(got, c.want) {
			t.Errorf("SuggestedActionForCategory(%q) = %q, want substring %q", c.category, got, c.want)
		}
	}
}

func TestRenderStatus_IncludesOutcomesAndNeedsHuman(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "planner_state.json")
	plannerLedger := filepath.Join(dir, "state", "runs.jsonl")
	autoloopLedger := filepath.Join(dir, "autoloop", "runs.jsonl")
	progressPath := filepath.Join(dir, "progress.json")

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	// 1. planner_state.json metadata header.
	stateBody, err := json.Marshal(map[string]any{
		"last_run_utc":  "2026-04-25T11:00:00Z",
		"backend":       "codexu",
		"mode":          "safe",
		"progress_json": progressPath,
		"report_path":   filepath.Join(dir, "latest_planner_report.md"),
		"context_path":  filepath.Join(dir, "context.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, stateBody, 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Planner ledger: one reshape per row, three rows total. Combined with
	//    the autoloop ledger below this yields exactly one of each outcome
	//    bucket (unstuck, still_failing, no_attempts_yet).
	for _, rc := range []RowChange{
		{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-unstuck", Kind: "spec_changed"},
		{PhaseID: "2", SubphaseID: "2.C", ItemName: "row-still-failing", Kind: "spec_changed"},
		{PhaseID: "2", SubphaseID: "2.C", ItemName: "row-no-attempts", Kind: "spec_changed"},
	} {
		if err := AppendLedgerEvent(plannerLedger, LedgerEvent{
			TS:          now.Add(-2 * time.Hour).Format(time.RFC3339),
			RunID:       "planner-1",
			Status:      "ok",
			RowsChanged: []RowChange{rc},
		}); err != nil {
			t.Fatal(err)
		}
	}

	// 3. Autoloop ledger: row-unstuck got promoted; row-still-failing failed
	//    twice; row-no-attempts has no entries (stays in no_attempts_yet bucket).
	if err := os.MkdirAll(filepath.Dir(autoloopLedger), 0o755); err != nil {
		t.Fatal(err)
	}
	autoloopEvents := []map[string]any{
		{
			"ts":     now.Add(-1 * time.Hour).Format(time.RFC3339),
			"event":  "worker_promoted",
			"task":   "2/2.B/row-unstuck",
			"status": "promoted",
		},
		{
			"ts":     now.Add(-50 * time.Minute).Format(time.RFC3339),
			"event":  "worker_failed",
			"task":   "2/2.C/row-still-failing",
			"status": "failed",
		},
		{
			"ts":     now.Add(-30 * time.Minute).Format(time.RFC3339),
			"event":  "worker_failed",
			"task":   "2/2.C/row-still-failing",
			"status": "failed",
		},
	}
	for _, ev := range autoloopEvents {
		body, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		body = append(body, '\n')
		f, err := os.OpenFile(autoloopLedger, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(body); err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()
	}

	// 4. progress.json: two NeedsHuman rows. The first has Health.LastFailure
	//    set so its suggested-action mapping resolves to the
	//    report_validation_failed bucket; the second has no Health block so it
	//    falls through to "manual review".
	prog := &progress.Progress{
		Meta: progress.Meta{Version: "1"},
		Phases: map[string]progress.Phase{
			"2": {
				Name: "Phase Two",
				Subphases: map[string]progress.Subphase{
					"2.C": {
						Name: "Sub C",
						Items: []progress.Item{
							{
								Name:   "row-3",
								Status: progress.StatusPlanned,
								PlannerVerdict: &progress.PlannerVerdict{
									NeedsHuman:   true,
									Reason:       "auto: 4 reshapes without unsticking; last category report_validation_failed",
									Since:        "2026-04-23T14:00:00Z",
									ReshapeCount: 4,
								},
								Health: &progress.RowHealth{
									LastFailure: &progress.FailureSummary{
										RunID:    "run-1",
										Category: progress.FailureReportValidation,
									},
								},
							},
							{
								Name:   "row-4",
								Status: progress.StatusPlanned,
								PlannerVerdict: &progress.PlannerVerdict{
									NeedsHuman:   true,
									Reason:       "manually escalated",
									Since:        "2026-04-22T08:00:00Z",
									ReshapeCount: 1,
								},
							},
						},
					},
				},
			},
		},
	}
	if err := progress.SaveProgress(progressPath, prog); err != nil {
		t.Fatal(err)
	}

	out, err := RenderStatus(RenderStatusOptions{
		StatePath:          statePath,
		PlannerLedgerPath:  plannerLedger,
		AutoloopLedgerPath: autoloopLedger,
		ProgressJSONPath:   progressPath,
		EvaluationWindow:   7 * 24 * time.Hour,
		Now:                now,
	})
	if err != nil {
		t.Fatalf("RenderStatus: %v", err)
	}

	wantSubstrings := []string{
		// Metadata header survived the refactor.
		"Last run UTC: 2026-04-25T11:00:00Z",
		"Backend: codexu",
		"Mode: safe",
		// Outcome bucket section + counts.
		"Reshape outcomes (last 7d):",
		"unstuck: 1",
		"still failing: 1",
		"no attempts yet: 1",
		// NeedsHuman inventory header + count.
		"Rows needing human attention: 2",
		// row-3: full reason + reshape count + suggested action mapped from
		// FailureReportValidation.
		"2/2.C/row-3 — auto: 4 reshapes without unsticking",
		"reshape count: 4",
		"since: 2026-04-23T14:00:00Z",
		"split into smaller rows or set contract_status=\"draft\"",
		// row-4: no Health block -> "manual review" fallback.
		"2/2.C/row-4 — manually escalated",
		"manual review",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestRenderStatus_MissingFilesProducesEmptySections(t *testing.T) {
	dir := t.TempDir()
	out, err := RenderStatus(RenderStatusOptions{
		StatePath:          filepath.Join(dir, "missing_state.json"),
		PlannerLedgerPath:  filepath.Join(dir, "missing_planner.jsonl"),
		AutoloopLedgerPath: filepath.Join(dir, "missing_autoloop.jsonl"),
		ProgressJSONPath:   filepath.Join(dir, "missing_progress.json"),
		EvaluationWindow:   7 * 24 * time.Hour,
		Now:                time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("RenderStatus should soft-fail missing files: %v", err)
	}
	for _, want := range []string{
		"Last run UTC: unknown",
		"Backend: unknown",
		"unstuck: 0",
		"still failing: 0",
		"no attempts yet: 0",
		"Rows needing human attention: 0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing-files render missing %q\n--- output ---\n%s", want, out)
		}
	}
}
