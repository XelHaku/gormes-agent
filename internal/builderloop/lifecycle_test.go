package builderloop

import (
	"path/filepath"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// TestLifecycle_FailingRowQuarantinesThenPlannerRepairUnlocksIt walks one
// row through the full reactive-autoloop loop:
//
//	Run 1: row attempted, fails → ConsecutiveFailures=1, no quarantine
//	Run 2: row attempted, fails → ConsecutiveFailures=2, no quarantine
//	Run 3: row attempted, fails → ConsecutiveFailures=3, quarantine SET with current spec hash
//	Run 4: selection excludes quarantined row (only row-2 surfaces)
//	Planner edit: row-1's contract is changed (simulated by direct progress.json mutation),
//	              making the stored Quarantine.SpecHash stale.
//	Run 5: selection surfaces row-1 with StaleQuarantine=true; accumulator records both
//	       a stale-clear AND a success → quarantine cleared, CF=0, LastSuccess set.
//
// This test uses the real internal/progress and internal/builderloop APIs.
// It does NOT spawn workers or use a fake runner — instead it drives the
// healthAccumulator directly (which is the same API run.go uses), proving
// the per-layer pieces compose correctly.
func TestLifecycle_FailingRowQuarantinesThenPlannerRepairUnlocksIt(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, progressPath)

	// writeBaseProgress emits rows with status=planned and no contract_status,
	// which puts them in candidateBucketPlanned — below the agentQueueCandidate
	// cutoff (<= candidateBucketDraft). The lifecycle test exercises selection
	// (R4/R5), so promote both rows into the "draft" bucket so they're eligible.
	// This mirrors the fixture shape used in candidates_health_test.go.
	prog, err := progress.Load(progressPath)
	if err != nil {
		t.Fatalf("seed load: %v", err)
	}
	{
		phase := prog.Phases["2"]
		sub := phase.Subphases["2.B"]
		for i := range sub.Items {
			sub.Items[i].ContractStatus = progress.ContractStatusDraft
		}
		phase.Subphases["2.B"] = sub
		prog.Phases["2"] = phase
	}
	if err := progress.SaveProgress(progressPath, prog); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	const threshold = 3

	// hashOf is the SpecHashProvider that the run loop uses at flush time.
	// We define it as a closure that reloads progress.json so it sees the
	// current row state (including any planner edits that happened mid-test).
	hashOf := func(phaseID, subphaseID, itemName string) string {
		prog, err := progress.Load(progressPath)
		if err != nil {
			return ""
		}
		phase, ok := prog.Phases[phaseID]
		if !ok {
			return ""
		}
		sub, ok := phase.Subphases[subphaseID]
		if !ok {
			return ""
		}
		for i := range sub.Items {
			if sub.Items[i].Name == itemName {
				return progress.ItemSpecHash(&sub.Items[i])
			}
		}
		return ""
	}

	// ---- Run 1: row-1 fails once ----
	acc := newHealthAccumulator("R1", fixedNow(), threshold)
	acc.RecordFailure(candidateOf("2", "2.B", "row-1", "do x"), progress.FailureWorkerError, "codexu", "boom 1")
	if err := acc.Flush(progressPath, hashOf); err != nil {
		t.Fatalf("R1 flush: %v", err)
	}
	prog, _ = progress.Load(progressPath)
	row1 := &prog.Phases["2"].Subphases["2.B"].Items[0]
	if row1.Health == nil || row1.Health.ConsecutiveFailures != 1 {
		t.Fatalf("R1: expected CF=1, got Health=%+v", row1.Health)
	}
	if row1.Health.Quarantine != nil {
		t.Fatalf("R1: should not be quarantined yet, got %+v", row1.Health.Quarantine)
	}

	// ---- Run 2: row-1 fails again ----
	acc = newHealthAccumulator("R2", fixedNow(), threshold)
	acc.RecordFailure(candidateOf("2", "2.B", "row-1", "do x"), progress.FailureWorkerError, "codexu", "boom 2")
	if err := acc.Flush(progressPath, hashOf); err != nil {
		t.Fatalf("R2 flush: %v", err)
	}
	prog, _ = progress.Load(progressPath)
	row1 = &prog.Phases["2"].Subphases["2.B"].Items[0]
	if row1.Health.ConsecutiveFailures != 2 {
		t.Fatalf("R2: expected CF=2, got %d", row1.Health.ConsecutiveFailures)
	}
	if row1.Health.Quarantine != nil {
		t.Fatalf("R2: should not be quarantined yet at threshold-1, got %+v", row1.Health.Quarantine)
	}

	// ---- Run 3: row-1 fails again — threshold hit, quarantine triggers ----
	acc = newHealthAccumulator("R3", fixedNow(), threshold)
	acc.RecordFailure(candidateOf("2", "2.B", "row-1", "do x"), progress.FailureReportValidation, "codexu", "report parse failed")
	if err := acc.Flush(progressPath, hashOf); err != nil {
		t.Fatalf("R3 flush: %v", err)
	}
	prog, _ = progress.Load(progressPath)
	row1 = &prog.Phases["2"].Subphases["2.B"].Items[0]
	if row1.Health.ConsecutiveFailures != 3 {
		t.Fatalf("R3: expected CF=3, got %d", row1.Health.ConsecutiveFailures)
	}
	if row1.Health.Quarantine == nil {
		t.Fatal("R3: expected quarantine to be set after threshold")
	}
	if row1.Health.Quarantine.SpecHash == "" {
		t.Fatal("R3: Quarantine.SpecHash should be populated by hashOf")
	}
	originalHash := row1.Health.Quarantine.SpecHash

	// ---- Run 4: selection excludes row-1, only row-2 surfaces ----
	candidates, err := NormalizeCandidates(progressPath, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("R4 NormalizeCandidates: %v", err)
	}
	var sawRow1, sawRow2 bool
	for _, c := range candidates {
		if c.ItemName == "row-1" {
			sawRow1 = true
		}
		if c.ItemName == "row-2" {
			sawRow2 = true
		}
	}
	if sawRow1 {
		t.Fatal("R4: row-1 should be excluded by quarantine filter")
	}
	if !sawRow2 {
		t.Fatalf("R4: row-2 should still be selectable; got %d candidates", len(candidates))
	}

	// ---- Planner edit: change row-1's contract → spec hash will differ ----
	prog, _ = progress.Load(progressPath)
	phase2 := prog.Phases["2"]
	sub2B := phase2.Subphases["2.B"]
	sub2B.Items[0].Contract = "do x — sharpened by planner"
	phase2.Subphases["2.B"] = sub2B
	prog.Phases["2"] = phase2
	if err := progress.SaveProgress(progressPath, prog); err != nil {
		t.Fatalf("save planner edit: %v", err)
	}

	// Verify the spec hash actually changed (else the rest of the test is moot).
	prog, _ = progress.Load(progressPath)
	newHash := progress.ItemSpecHash(&prog.Phases["2"].Subphases["2.B"].Items[0])
	if newHash == originalHash {
		t.Fatalf("planner edit did not change spec hash; got %q both times", newHash)
	}

	// ---- Run 5: selection surfaces row-1 with StaleQuarantine flag ----
	candidates, err = NormalizeCandidates(progressPath, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("R5 NormalizeCandidates: %v", err)
	}
	var staleRow1 Candidate
	var foundStale bool
	for _, c := range candidates {
		if c.ItemName == "row-1" {
			staleRow1 = c
			foundStale = true
			break
		}
	}
	if !foundStale {
		t.Fatal("R5: row-1 should re-enter the candidate pool after spec change")
	}
	if !staleRow1.StaleQuarantine {
		t.Fatal("R5: row-1 should have StaleQuarantine=true after spec change")
	}

	// Run 5 (cont.): row-1 is attempted and succeeds → quarantine cleared, CF=0
	acc = newHealthAccumulator("R5", fixedNow(), threshold)
	acc.MarkStaleQuarantine(staleRow1)
	acc.RecordSuccess(candidateOf("2", "2.B", "row-1", "do x — sharpened by planner"))
	if err := acc.Flush(progressPath, hashOf); err != nil {
		t.Fatalf("R5 flush: %v", err)
	}

	prog, _ = progress.Load(progressPath)
	row1 = &prog.Phases["2"].Subphases["2.B"].Items[0]
	if row1.Health.Quarantine != nil {
		t.Fatalf("R5: quarantine should be cleared, got %+v", row1.Health.Quarantine)
	}
	if row1.Health.ConsecutiveFailures != 0 {
		t.Fatalf("R5: ConsecutiveFailures should reset to 0, got %d", row1.Health.ConsecutiveFailures)
	}
	if row1.Health.LastSuccess == "" {
		t.Fatal("R5: LastSuccess should be set after successful run")
	}
}
