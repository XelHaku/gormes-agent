package plannerloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/builderloop"
	"github.com/TrebuchetDynamics/gormes-agent/internal/plannertriggers"
	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// TestLifecycle_PlannerSelfHealingFullLoop walks one row through the full
// Phase C loop using real APIs (no LLM):
//
//	Run 1 (autoloop): row-x fails 3 times → quarantine → trigger emitted
//	Run 2 (planner, event): trigger consumed; row-x reshape; verdict.ReshapeCount=1
//	Run 3 (autoloop): stale-quarantine flagged; row-x attempted; fails again →
//	                  quarantine re-set; trigger emitted again
//	Run 4 (planner, event): L4 outcome=still_failing; verdict.ReshapeCount=2
//	Run 5 (autoloop): row-x fails again → quarantine re-set
//	Run 6 (planner, event): verdict.ReshapeCount=3 → NeedsHuman=true
//	Run 7 (autoloop): selection EXCLUDES row-x (NeedsHuman)
//
// Drives autoloop's row-health writes via progress.ApplyHealthUpdates
// (the same single-source-of-truth API that healthAccumulator.Flush goes
// through) and planner's StampVerdicts directly. The LLM is mocked: each
// "planner run" simulates a successful regen by mutating the row's
// contract (which advances ItemSpecHash) without dropping Health.
//
// This is the catch-all integration check that proves L1-L6 compose
// correctly. If any of the seven run boundaries diverges from expectation
// the failure message names which boundary failed and what diverged.
func TestLifecycle_PlannerSelfHealingFullLoop(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.json")
	plannerLedgerPath := filepath.Join(dir, "planner-runs.jsonl")
	autoloopLedgerPath := filepath.Join(dir, "autoloop-runs.jsonl")
	triggersPath := filepath.Join(dir, "triggers.jsonl")
	cursorPath := filepath.Join(dir, "triggers-cursor.json")

	writeLifecycleProgress(t, progressPath)

	const threshold = 3
	const escalationThreshold = 3

	// All test timestamps are deterministic. Each run advances by one hour
	// so Evaluate's window filter and ordering checks behave predictably.
	t0 := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	// ---- Run 1 (autoloop): row-x fails 3 times → quarantine ----
	r1Time := t0
	failRowX(t, progressPath, "R1", r1Time, threshold, 3, progress.FailureWorkerError)
	writeAutoloopFailures(t, autoloopLedgerPath, "R1", r1Time, "row-x", 3, "backend_failed")

	prog, _ := progress.Load(progressPath)
	rowX := findRow(t, prog, "row-x")
	if rowX.Health == nil || rowX.Health.ConsecutiveFailures != 3 {
		t.Fatalf("Run 1: expected CF=3, got Health=%+v", rowX.Health)
	}
	if rowX.Health.Quarantine == nil {
		t.Fatal("Run 1: expected Quarantine to be set after threshold")
	}
	originalHash := rowX.Health.Quarantine.SpecHash
	if originalHash == "" {
		t.Fatal("Run 1: Quarantine.SpecHash should be populated")
	}

	// Emit the quarantine_added trigger autoloop's flush would have written.
	if err := plannertriggers.AppendTriggerEvent(triggersPath, plannertriggers.TriggerEvent{
		TS:            r1Time.Format(time.RFC3339),
		Source:        "autoloop",
		Kind:          "quarantine_added",
		PhaseID:       "2",
		SubphaseID:    "2.B",
		ItemName:      "row-x",
		Reason:        rowX.Health.Quarantine.Reason,
		AutoloopRunID: "R1",
	}); err != nil {
		t.Fatalf("Run 1: append trigger: %v", err)
	}

	// ---- Run 2 (planner, event): consume trigger; reshape row-x; ReshapeCount=1 ----
	r2Time := t0.Add(1 * time.Hour)
	cursor, err := plannertriggers.LoadCursor(cursorPath)
	if err != nil {
		t.Fatalf("Run 2: load cursor: %v", err)
	}
	events, err := plannertriggers.ReadTriggersSinceCursor(triggersPath, cursor)
	if err != nil {
		t.Fatalf("Run 2: read triggers: %v", err)
	}
	if len(events) != 1 || events[0].Kind != "quarantine_added" || events[0].ItemName != "row-x" {
		t.Fatalf("Run 2: expected exactly one quarantine_added trigger for row-x, got %+v", events)
	}

	mutateContract(t, progressPath, "row-x", "do x — sharpened by planner v1")
	prog, _ = progress.Load(progressPath)
	rowsChanged := []RowChange{{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-x", Kind: "spec_changed"}}
	verdictChanges := StampVerdicts(prog, rowsChanged, nil, escalationThreshold, r2Time)
	if len(verdictChanges) != 1 || verdictChanges[0].Detail != "reshape_count incremented" {
		t.Fatalf("Run 2: expected one verdict_set increment, got %+v", verdictChanges)
	}
	if err := progress.SaveProgress(progressPath, prog); err != nil {
		t.Fatalf("Run 2: save: %v", err)
	}
	prog, _ = progress.Load(progressPath)
	rowX = findRow(t, prog, "row-x")
	if rowX.PlannerVerdict == nil || rowX.PlannerVerdict.ReshapeCount != 1 {
		t.Fatalf("Run 2: expected ReshapeCount=1, got %+v", rowX.PlannerVerdict)
	}
	if rowX.Health == nil || rowX.Health.Quarantine == nil {
		t.Fatal("Run 2: planner reshape must NOT clear Health.Quarantine (autoloop owns Health)")
	}
	if err := AppendLedgerEvent(plannerLedgerPath, LedgerEvent{
		TS:            r2Time.Format(time.RFC3339),
		RunID:         "P2",
		Trigger:       "event",
		TriggerEvents: []string{events[0].ID},
		Backend:       "codexu",
		Mode:          "safe",
		Status:        "ok",
		RowsChanged:   append(append([]RowChange(nil), rowsChanged...), verdictChanges...),
	}); err != nil {
		t.Fatalf("Run 2: append planner ledger: %v", err)
	}
	if err := plannertriggers.SaveCursor(cursorPath, plannertriggers.TriggerCursor{
		LastConsumedID: events[0].ID,
		LastReadAt:     r2Time.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("Run 2: save cursor: %v", err)
	}

	// Confirm spec hash advanced — selection must surface row-x as stale next run.
	postR2Hash := progress.ItemSpecHash(rowX)
	if postR2Hash == originalHash {
		t.Fatalf("Run 2: planner edit did not advance ItemSpecHash (was %q)", originalHash)
	}

	// ---- Run 3 (autoloop): stale-quarantine flagged; row-x fails again → quarantine re-set ----
	r3Time := t0.Add(2 * time.Hour)
	candidates, err := builderloop.NormalizeCandidates(progressPath, builderloop.CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("Run 3: NormalizeCandidates: %v", err)
	}
	var rowXCandidate builderloop.Candidate
	var foundStale bool
	for _, c := range candidates {
		if c.ItemName == "row-x" {
			rowXCandidate = c
			foundStale = true
			break
		}
	}
	if !foundStale {
		t.Fatal("Run 3: row-x should re-enter selection after planner reshape (StaleQuarantine path)")
	}
	if !rowXCandidate.StaleQuarantine {
		t.Fatal("Run 3: row-x should have StaleQuarantine=true after spec change")
	}
	// Apply stale-clear + 3 fresh failures to re-trigger quarantine.
	clearAndFailRowX(t, progressPath, "R3", r3Time, threshold, 3, progress.FailureWorkerError)
	writeAutoloopFailures(t, autoloopLedgerPath, "R3", r3Time, "row-x", 3, "backend_failed")

	prog, _ = progress.Load(progressPath)
	rowX = findRow(t, prog, "row-x")
	if rowX.Health.Quarantine == nil {
		t.Fatal("Run 3: expected Quarantine to be re-set after threshold failures")
	}
	if rowX.Health.ConsecutiveFailures != 3 {
		t.Fatalf("Run 3: expected CF=3, got %d", rowX.Health.ConsecutiveFailures)
	}

	if err := plannertriggers.AppendTriggerEvent(triggersPath, plannertriggers.TriggerEvent{
		TS:            r3Time.Format(time.RFC3339),
		Source:        "autoloop",
		Kind:          "quarantine_added",
		PhaseID:       "2",
		SubphaseID:    "2.B",
		ItemName:      "row-x",
		Reason:        rowX.Health.Quarantine.Reason,
		AutoloopRunID: "R3",
	}); err != nil {
		t.Fatalf("Run 3: append trigger: %v", err)
	}

	// ---- Run 4 (planner, event): outcome=still_failing; ReshapeCount=2 ----
	r4Time := t0.Add(3 * time.Hour)
	cursor, _ = plannertriggers.LoadCursor(cursorPath)
	events, err = plannertriggers.ReadTriggersSinceCursor(triggersPath, cursor)
	if err != nil {
		t.Fatalf("Run 4: read triggers: %v", err)
	}
	if len(events) != 1 || events[0].AutoloopRunID != "R3" {
		t.Fatalf("Run 4: expected exactly one new trigger from R3, got %+v", events)
	}

	outcomes, err := Evaluate(plannerLedgerPath, autoloopLedgerPath, 7*24*time.Hour, r4Time)
	if err != nil {
		t.Fatalf("Run 4: Evaluate: %v", err)
	}
	rowXOutcome := findOutcome(t, outcomes, "row-x")
	if rowXOutcome.Outcome != "still_failing" {
		t.Fatalf("Run 4: expected outcome=still_failing, got %q (runs=%d, lastFailure=%q)",
			rowXOutcome.Outcome, rowXOutcome.AutoloopRuns, rowXOutcome.LastFailure)
	}

	mutateContract(t, progressPath, "row-x", "do x — sharpened by planner v2")
	prog, _ = progress.Load(progressPath)
	rowsChanged = []RowChange{{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-x", Kind: "spec_changed"}}
	verdictChanges = StampVerdicts(prog, rowsChanged, outcomes, escalationThreshold, r4Time)
	if err := progress.SaveProgress(progressPath, prog); err != nil {
		t.Fatalf("Run 4: save: %v", err)
	}
	prog, _ = progress.Load(progressPath)
	rowX = findRow(t, prog, "row-x")
	if rowX.PlannerVerdict.ReshapeCount != 2 {
		t.Fatalf("Run 4: expected ReshapeCount=2, got %d", rowX.PlannerVerdict.ReshapeCount)
	}
	if rowX.PlannerVerdict.NeedsHuman {
		t.Fatalf("Run 4: NeedsHuman should NOT be set yet (ReshapeCount=2 < threshold=%d)", escalationThreshold)
	}
	if rowX.PlannerVerdict.LastOutcome != "still_failing" {
		t.Fatalf("Run 4: expected LastOutcome=still_failing, got %q", rowX.PlannerVerdict.LastOutcome)
	}
	if err := AppendLedgerEvent(plannerLedgerPath, LedgerEvent{
		TS:            r4Time.Format(time.RFC3339),
		RunID:         "P4",
		Trigger:       "event",
		TriggerEvents: []string{events[0].ID},
		Backend:       "codexu",
		Mode:          "safe",
		Status:        "ok",
		RowsChanged:   append(append([]RowChange(nil), rowsChanged...), verdictChanges...),
	}); err != nil {
		t.Fatalf("Run 4: append planner ledger: %v", err)
	}
	if err := plannertriggers.SaveCursor(cursorPath, plannertriggers.TriggerCursor{
		LastConsumedID: events[0].ID,
		LastReadAt:     r4Time.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("Run 4: save cursor: %v", err)
	}

	// ---- Run 5 (autoloop): stale-cleared again; row-x fails 3x; quarantine re-set ----
	r5Time := t0.Add(4 * time.Hour)
	candidates, err = builderloop.NormalizeCandidates(progressPath, builderloop.CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("Run 5: NormalizeCandidates: %v", err)
	}
	var sawStaleR5 bool
	for _, c := range candidates {
		if c.ItemName == "row-x" {
			if !c.StaleQuarantine {
				t.Fatal("Run 5: row-x should be StaleQuarantine after planner reshape v2")
			}
			sawStaleR5 = true
			break
		}
	}
	if !sawStaleR5 {
		t.Fatal("Run 5: row-x should re-enter selection after planner reshape v2")
	}

	clearAndFailRowX(t, progressPath, "R5", r5Time, threshold, 3, progress.FailureWorkerError)
	writeAutoloopFailures(t, autoloopLedgerPath, "R5", r5Time, "row-x", 3, "backend_failed")

	prog, _ = progress.Load(progressPath)
	rowX = findRow(t, prog, "row-x")
	if rowX.Health.Quarantine == nil {
		t.Fatal("Run 5: expected Quarantine to be re-set")
	}

	if err := plannertriggers.AppendTriggerEvent(triggersPath, plannertriggers.TriggerEvent{
		TS:            r5Time.Format(time.RFC3339),
		Source:        "autoloop",
		Kind:          "quarantine_added",
		PhaseID:       "2",
		SubphaseID:    "2.B",
		ItemName:      "row-x",
		Reason:        rowX.Health.Quarantine.Reason,
		AutoloopRunID: "R5",
	}); err != nil {
		t.Fatalf("Run 5: append trigger: %v", err)
	}

	// ---- Run 6 (planner, event): ReshapeCount=3 → NeedsHuman=true ----
	r6Time := t0.Add(5 * time.Hour)
	cursor, _ = plannertriggers.LoadCursor(cursorPath)
	events, err = plannertriggers.ReadTriggersSinceCursor(triggersPath, cursor)
	if err != nil {
		t.Fatalf("Run 6: read triggers: %v", err)
	}
	if len(events) != 1 || events[0].AutoloopRunID != "R5" {
		t.Fatalf("Run 6: expected exactly one new trigger from R5, got %+v", events)
	}

	outcomes, err = Evaluate(plannerLedgerPath, autoloopLedgerPath, 7*24*time.Hour, r6Time)
	if err != nil {
		t.Fatalf("Run 6: Evaluate: %v", err)
	}
	rowXOutcome = findOutcome(t, outcomes, "row-x")
	if rowXOutcome.Outcome != "still_failing" {
		t.Fatalf("Run 6: expected outcome=still_failing, got %q", rowXOutcome.Outcome)
	}

	mutateContract(t, progressPath, "row-x", "do x — sharpened by planner v3")
	prog, _ = progress.Load(progressPath)
	rowsChanged = []RowChange{{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-x", Kind: "spec_changed"}}
	verdictChanges = StampVerdicts(prog, rowsChanged, outcomes, escalationThreshold, r6Time)
	if err := progress.SaveProgress(progressPath, prog); err != nil {
		t.Fatalf("Run 6: save: %v", err)
	}
	prog, _ = progress.Load(progressPath)
	rowX = findRow(t, prog, "row-x")
	if rowX.PlannerVerdict.ReshapeCount != 3 {
		t.Fatalf("Run 6: expected ReshapeCount=3, got %d", rowX.PlannerVerdict.ReshapeCount)
	}
	if !rowX.PlannerVerdict.NeedsHuman {
		t.Fatalf("Run 6: expected NeedsHuman=true at ReshapeCount=%d, got verdict=%+v",
			rowX.PlannerVerdict.ReshapeCount, rowX.PlannerVerdict)
	}
	if rowX.PlannerVerdict.Reason == "" {
		t.Fatal("Run 6: NeedsHuman trigger should populate Reason")
	}
	// Confirm at least one verdict_set entry recorded the needs_human transition.
	var sawNeedsHuman bool
	for _, vc := range verdictChanges {
		if vc.Detail == "needs_human=true" {
			sawNeedsHuman = true
			break
		}
	}
	if !sawNeedsHuman {
		t.Fatalf("Run 6: expected verdict_set{detail=needs_human=true} in %+v", verdictChanges)
	}
	if err := AppendLedgerEvent(plannerLedgerPath, LedgerEvent{
		TS:            r6Time.Format(time.RFC3339),
		RunID:         "P6",
		Trigger:       "event",
		TriggerEvents: []string{events[0].ID},
		Backend:       "codexu",
		Mode:          "safe",
		Status:        "needs_human_set",
		RowsChanged:   append(append([]RowChange(nil), rowsChanged...), verdictChanges...),
	}); err != nil {
		t.Fatalf("Run 6: append planner ledger: %v", err)
	}
	if err := plannertriggers.SaveCursor(cursorPath, plannertriggers.TriggerCursor{
		LastConsumedID: events[0].ID,
		LastReadAt:     r6Time.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("Run 6: save cursor: %v", err)
	}

	// ---- Run 7 (autoloop): selection EXCLUDES row-x because PlannerVerdict.NeedsHuman=true ----
	candidates, err = builderloop.NormalizeCandidates(progressPath, builderloop.CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("Run 7: NormalizeCandidates: %v", err)
	}
	for _, c := range candidates {
		if c.ItemName == "row-x" {
			t.Fatalf("Run 7: row-x must be excluded by NeedsHuman skip; got candidate=%+v", c)
		}
	}
	// Sanity: row-y (the companion row) must still be selectable so we know
	// the filter is targeted, not a full-table block.
	var sawRowY bool
	for _, c := range candidates {
		if c.ItemName == "row-y" {
			sawRowY = true
			break
		}
	}
	if !sawRowY {
		t.Fatalf("Run 7: row-y should remain selectable (NeedsHuman skip should only block row-x); got %d candidates", len(candidates))
	}

	// Sanity: the IncludeNeedsHuman override surfaces row-x with the flag set.
	candidates, err = builderloop.NormalizeCandidates(progressPath, builderloop.CandidateOptions{
		ActiveFirst:        true,
		IncludeQuarantined: true,
		IncludeNeedsHuman:  true,
	})
	if err != nil {
		t.Fatalf("Run 7 override: NormalizeCandidates: %v", err)
	}
	var rowXOverride builderloop.Candidate
	var foundOverride bool
	for _, c := range candidates {
		if c.ItemName == "row-x" {
			rowXOverride = c
			foundOverride = true
			break
		}
	}
	if !foundOverride {
		t.Fatal("Run 7 override: row-x should surface when IncludeNeedsHuman=true")
	}
	if !rowXOverride.NeedsHumanFlag {
		t.Fatal("Run 7 override: surfaced row-x should carry NeedsHumanFlag=true")
	}
}

// writeLifecycleProgress seeds progress.json with two rows in subphase 2.B
// using ContractStatus=draft so they pass agentQueueCandidate.
func writeLifecycleProgress(t *testing.T, path string) {
	t.Helper()
	body := `{
  "version": "1",
  "phases": {
    "2": {
      "name": "P",
      "subphases": {
        "2.B": {
          "name": "S",
          "items": [
            {"name": "row-x", "status": "planned", "contract": "do x", "contract_status": "draft"},
            {"name": "row-y", "status": "planned", "contract": "do y", "contract_status": "draft"}
          ]
        }
      }
    }
  }
}
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write progress: %v", err)
	}
}

// failRowX records `n` failures against row-x and stamps quarantine when CF
// crosses threshold. Mirrors what healthAccumulator.Flush would do at the
// end of one autoloop run with `n` failed attempts on the same row.
func failRowX(t *testing.T, path, runID string, now time.Time, threshold, n int, cat progress.FailureCategory) {
	t.Helper()
	nowStr := now.UTC().Format(time.RFC3339)
	updates := []progress.HealthUpdate{{
		PhaseID:    "2",
		SubphaseID: "2.B",
		ItemName:   "row-x",
		Mutate: func(h *progress.RowHealth) {
			h.AttemptCount += n
			h.LastAttempt = nowStr
			h.LastFailure = &progress.FailureSummary{
				RunID:      runID,
				Category:   cat,
				Backend:    "codexu",
				StderrTail: "boom",
			}
			h.ConsecutiveFailures += n
			if h.Quarantine == nil && h.ConsecutiveFailures >= threshold {
				h.Quarantine = &progress.Quarantine{
					Reason:       "auto: lifecycle test",
					Since:        nowStr,
					AfterRunID:   runID,
					Threshold:    threshold,
					SpecHash:     progress.ItemSpecHash(loadRowSpec(t, path, "row-x")),
					LastCategory: cat,
				}
			}
		},
	}}
	if err := progress.ApplyHealthUpdates(path, updates); err != nil {
		t.Fatalf("ApplyHealthUpdates (failRowX %s): %v", runID, err)
	}
}

// clearAndFailRowX simulates one autoloop run that observed a stale
// quarantine on row-x: it clears the stale block, resets CF to 0, then
// records `n` fresh failures (re-quarantining if CF crosses threshold).
// Mirrors the staleClear arm of healthAccumulator.applyMutation.
func clearAndFailRowX(t *testing.T, path, runID string, now time.Time, threshold, n int, cat progress.FailureCategory) {
	t.Helper()
	nowStr := now.UTC().Format(time.RFC3339)
	updates := []progress.HealthUpdate{{
		PhaseID:    "2",
		SubphaseID: "2.B",
		ItemName:   "row-x",
		Mutate: func(h *progress.RowHealth) {
			// Stale-clear arm.
			h.Quarantine = nil
			h.ConsecutiveFailures = 0
			// Then apply the fresh failures.
			h.AttemptCount += n
			h.LastAttempt = nowStr
			h.LastFailure = &progress.FailureSummary{
				RunID:      runID,
				Category:   cat,
				Backend:    "codexu",
				StderrTail: "boom",
			}
			h.ConsecutiveFailures += n
			if h.Quarantine == nil && h.ConsecutiveFailures >= threshold {
				h.Quarantine = &progress.Quarantine{
					Reason:       "auto: lifecycle test",
					Since:        nowStr,
					AfterRunID:   runID,
					Threshold:    threshold,
					SpecHash:     progress.ItemSpecHash(loadRowSpec(t, path, "row-x")),
					LastCategory: cat,
				}
			}
		},
	}}
	if err := progress.ApplyHealthUpdates(path, updates); err != nil {
		t.Fatalf("ApplyHealthUpdates (clearAndFailRowX %s): %v", runID, err)
	}
}

// loadRowSpec returns a snapshot of the named row from progress.json. Used
// to compute the stable ItemSpecHash inside the Mutate callback.
func loadRowSpec(t *testing.T, path, name string) *progress.Item {
	t.Helper()
	prog, err := progress.Load(path)
	if err != nil {
		t.Fatalf("loadRowSpec: %v", err)
	}
	for _, ph := range prog.Phases {
		for _, sub := range ph.Subphases {
			for i := range sub.Items {
				if sub.Items[i].Name == name {
					it := sub.Items[i]
					return &it
				}
			}
		}
	}
	t.Fatalf("loadRowSpec: row %q not found", name)
	return nil
}

// mutateContract simulates a planner regen by editing the named row's
// contract on disk. Advances ItemSpecHash without touching Health, which is
// the core invariant the lifecycle relies on (stale-quarantine detection).
func mutateContract(t *testing.T, path, name, newContract string) {
	t.Helper()
	prog, err := progress.Load(path)
	if err != nil {
		t.Fatalf("mutateContract load: %v", err)
	}
	for phaseID, ph := range prog.Phases {
		for subID, sub := range ph.Subphases {
			for i := range sub.Items {
				if sub.Items[i].Name == name {
					sub.Items[i].Contract = newContract
					ph.Subphases[subID] = sub
					prog.Phases[phaseID] = ph
					if err := progress.SaveProgress(path, prog); err != nil {
						t.Fatalf("mutateContract save: %v", err)
					}
					return
				}
			}
		}
	}
	t.Fatalf("mutateContract: row %q not found", name)
}

// findRow locates the named row in a loaded progress doc.
func findRow(t *testing.T, prog *progress.Progress, name string) *progress.Item {
	t.Helper()
	for _, ph := range prog.Phases {
		for _, sub := range ph.Subphases {
			for i := range sub.Items {
				if sub.Items[i].Name == name {
					return &sub.Items[i]
				}
			}
		}
	}
	t.Fatalf("row %q not found in progress doc", name)
	return nil
}

// findOutcome returns the ReshapeOutcome for the named row, failing the
// test if Evaluate did not produce one.
func findOutcome(t *testing.T, outcomes []ReshapeOutcome, name string) ReshapeOutcome {
	t.Helper()
	for _, o := range outcomes {
		if o.ItemName == name {
			return o
		}
	}
	t.Fatalf("Evaluate produced no outcome for %q; got %+v", name, outcomes)
	return ReshapeOutcome{}
}

// writeAutoloopFailures appends `n` worker_failed events for one row to the
// autoloop runs.jsonl. Each event is timestamped one second apart starting
// at `base` so Evaluate's after-reshape filter sees them in append order.
// Uses the same json shape autoloop's appendRunLedgerEvent writes (see
// internal/builderloop/ledger.go).
func writeAutoloopFailures(t *testing.T, path, runID string, base time.Time, itemName string, n int, status string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir autoloop ledger dir: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open autoloop ledger: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := 0; i < n; i++ {
		ev := builderloop.LedgerEvent{
			TS:     base.Add(time.Duration(i) * time.Second),
			RunID:  runID,
			Event:  "worker_failed",
			Worker: i + 1,
			Task:   "2/2.B/" + itemName,
			Status: status,
		}
		if err := enc.Encode(ev); err != nil {
			t.Fatalf("encode autoloop event: %v", err)
		}
	}
}
