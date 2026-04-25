package builderloop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func writeBaseProgress(t *testing.T, path string) {
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
            {"name": "row-1", "status": "planned", "contract": "do x"},
            {"name": "row-2", "status": "planned", "contract": "do y"}
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
		t.Fatalf("write: %v", err)
	}
}

func fixedNow() func() time.Time {
	t := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

func candidateOf(phase, sub, item, contract string) Candidate {
	return Candidate{
		PhaseID:    phase,
		SubphaseID: sub,
		ItemName:   item,
		Contract:   contract,
	}
}

func TestHealthAccumulator_RecordSuccessSetsLastSuccessAndResetsCounter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, path)

	acc := newHealthAccumulator("run-A", fixedNow(), 3)
	acc.RecordSuccess(candidateOf("2", "2.B", "row-1", "do x"))
	if err := acc.Flush(path, nil); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	prog, err := progress.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	row := prog.Phases["2"].Subphases["2.B"].Items[0]
	if row.Health == nil {
		t.Fatal("row.Health should be set")
	}
	if row.Health.LastSuccess != "2026-04-24T12:00:00Z" {
		t.Fatalf("LastSuccess = %q", row.Health.LastSuccess)
	}
	if row.Health.ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures should be 0, got %d", row.Health.ConsecutiveFailures)
	}
	if row.Health.Quarantine != nil {
		t.Fatalf("Quarantine should be nil after success, got %+v", row.Health.Quarantine)
	}
}

func TestHealthAccumulator_RecordFailureIncrementsConsecutive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, path)

	acc := newHealthAccumulator("run-A", fixedNow(), 3)
	acc.RecordFailure(candidateOf("2", "2.B", "row-1", "do x"), progress.FailureWorkerError, "codexu", "boom")
	if err := acc.Flush(path, nil); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	prog, _ := progress.Load(path)
	row := prog.Phases["2"].Subphases["2.B"].Items[0]
	if row.Health.ConsecutiveFailures != 1 {
		t.Fatalf("ConsecutiveFailures = %d, want 1", row.Health.ConsecutiveFailures)
	}
	if row.Health.AttemptCount != 1 {
		t.Fatalf("AttemptCount = %d, want 1", row.Health.AttemptCount)
	}
	if row.Health.Quarantine != nil {
		t.Fatalf("should not quarantine on first failure, got %+v", row.Health.Quarantine)
	}
}

func TestHealthAccumulator_QuarantinesAfterThresholdConsecutiveFailures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, path)

	// Pre-load existing health: 2 consecutive failures already.
	if err := progress.ApplyHealthUpdates(path, []progress.HealthUpdate{{
		PhaseID:    "2",
		SubphaseID: "2.B",
		ItemName:   "row-1",
		Mutate: func(h *progress.RowHealth) {
			h.AttemptCount = 2
			h.ConsecutiveFailures = 2
		},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	acc := newHealthAccumulator("run-A", fixedNow(), 3)
	acc.RecordFailure(candidateOf("2", "2.B", "row-1", "do x"), progress.FailureReportValidation, "codexu", "report parse failed")
	if err := acc.Flush(path, nil); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	prog, _ := progress.Load(path)
	row := prog.Phases["2"].Subphases["2.B"].Items[0]
	if row.Health.ConsecutiveFailures != 3 {
		t.Fatalf("ConsecutiveFailures = %d, want 3", row.Health.ConsecutiveFailures)
	}
	if row.Health.Quarantine == nil {
		t.Fatal("expected Quarantine to be set after threshold")
	}
	if row.Health.Quarantine.LastCategory != progress.FailureReportValidation {
		t.Fatalf("Quarantine.LastCategory = %q", row.Health.Quarantine.LastCategory)
	}
	if row.Health.Quarantine.SpecHash != "" {
		t.Fatalf("Quarantine.SpecHash should be empty when hashOf=nil, got %q", row.Health.Quarantine.SpecHash)
	}
}

func TestHealthAccumulator_SuccessAfterFailuresClearsQuarantine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, path)

	// Pre-quarantine the row.
	if err := progress.ApplyHealthUpdates(path, []progress.HealthUpdate{{
		PhaseID:    "2",
		SubphaseID: "2.B",
		ItemName:   "row-1",
		Mutate: func(h *progress.RowHealth) {
			h.ConsecutiveFailures = 3
			h.Quarantine = &progress.Quarantine{Reason: "auto", Threshold: 3, SpecHash: "abc"}
		},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	acc := newHealthAccumulator("run-A", fixedNow(), 3)
	acc.RecordSuccess(candidateOf("2", "2.B", "row-1", "do x"))
	if err := acc.Flush(path, nil); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	prog, _ := progress.Load(path)
	row := prog.Phases["2"].Subphases["2.B"].Items[0]
	if row.Health.Quarantine != nil {
		t.Fatalf("Quarantine should be cleared after success, got %+v", row.Health.Quarantine)
	}
	if row.Health.ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures should reset on success, got %d", row.Health.ConsecutiveFailures)
	}
}

func TestFlush_NewQuarantineEmitsTrigger(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, progressPath)

	// Pre-load row at CF=2; one more failure trips quarantine.
	if err := progress.ApplyHealthUpdates(progressPath, []progress.HealthUpdate{{
		PhaseID:    "2",
		SubphaseID: "2.B",
		ItemName:   "row-1",
		Mutate: func(h *progress.RowHealth) {
			h.AttemptCount = 2
			h.ConsecutiveFailures = 2
		},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	acc := newHealthAccumulator("R1", fixedNow(), 3)
	acc.RecordFailure(candidateOf("2", "2.B", "row-1", "do x"), progress.FailureWorkerError, "codexu", "boom")
	events, err := acc.FlushWithTriggers(progressPath, nil)
	if err != nil {
		t.Fatalf("FlushWithTriggers: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 trigger event, got %d", len(events))
	}
	if events[0].Kind != "quarantine_added" {
		t.Fatalf("event kind = %q, want quarantine_added", events[0].Kind)
	}
	if events[0].ItemName != "row-1" {
		t.Fatalf("event.ItemName = %q", events[0].ItemName)
	}
	if events[0].PhaseID != "2" || events[0].SubphaseID != "2.B" {
		t.Fatalf("event coords = %s/%s, want 2/2.B", events[0].PhaseID, events[0].SubphaseID)
	}
	if events[0].AutoloopRunID != "R1" {
		t.Fatalf("event.AutoloopRunID = %q, want R1", events[0].AutoloopRunID)
	}
	if events[0].Reason == "" {
		t.Fatalf("expected non-empty Reason from quarantine, got empty")
	}
}

func TestFlush_PureFailureBelowThresholdEmitsNoTrigger(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, progressPath)

	acc := newHealthAccumulator("R1", fixedNow(), 3)
	acc.RecordFailure(candidateOf("2", "2.B", "row-1", "do x"), progress.FailureWorkerError, "codexu", "boom")
	events, err := acc.FlushWithTriggers(progressPath, nil)
	if err != nil {
		t.Fatalf("FlushWithTriggers: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no trigger events for sub-threshold failure, got %d", len(events))
	}
}

func TestFlush_StaleClearEmitsTrigger(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, progressPath)

	if err := progress.ApplyHealthUpdates(progressPath, []progress.HealthUpdate{{
		PhaseID:    "2",
		SubphaseID: "2.B",
		ItemName:   "row-1",
		Mutate: func(h *progress.RowHealth) {
			h.ConsecutiveFailures = 5
			h.Quarantine = &progress.Quarantine{Reason: "auto", Threshold: 3, SpecHash: "stale"}
		},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	acc := newHealthAccumulator("R1", fixedNow(), 3)
	acc.MarkStaleQuarantine(candidateOf("2", "2.B", "row-1", "do x"))
	events, err := acc.FlushWithTriggers(progressPath, nil)
	if err != nil {
		t.Fatalf("FlushWithTriggers: %v", err)
	}
	if len(events) != 1 || events[0].Kind != "quarantine_stale_cleared" {
		t.Fatalf("expected 1 quarantine_stale_cleared event, got %+v", events)
	}
}

func TestFlush_NoTriggersWhenNoRows(t *testing.T) {
	dir := t.TempDir()
	progressPath := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, progressPath)

	acc := newHealthAccumulator("R1", fixedNow(), 3)
	events, err := acc.FlushWithTriggers(progressPath, nil)
	if err != nil {
		t.Fatalf("FlushWithTriggers: %v", err)
	}
	if events != nil {
		t.Fatalf("expected nil events with empty accumulator, got %d", len(events))
	}
}

func TestHealthAccumulator_StaleQuarantineClearsAndResetsCounter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeBaseProgress(t, path)

	// Quarantine row-1 with a SpecHash that does NOT match its current spec.
	if err := progress.ApplyHealthUpdates(path, []progress.HealthUpdate{{
		PhaseID:    "2",
		SubphaseID: "2.B",
		ItemName:   "row-1",
		Mutate: func(h *progress.RowHealth) {
			h.ConsecutiveFailures = 5
			h.Quarantine = &progress.Quarantine{Reason: "auto", Threshold: 3, SpecHash: "stale-hash"}
		},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	acc := newHealthAccumulator("run-A", fixedNow(), 3)
	acc.MarkStaleQuarantine(candidateOf("2", "2.B", "row-1", "do x"))
	if err := acc.Flush(path, nil); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	prog, _ := progress.Load(path)
	row := prog.Phases["2"].Subphases["2.B"].Items[0]
	if row.Health.Quarantine != nil {
		t.Fatalf("Quarantine should be cleared after stale-quarantine signal, got %+v", row.Health.Quarantine)
	}
	if row.Health.ConsecutiveFailures != 0 {
		t.Fatalf("ConsecutiveFailures should reset on stale-clear, got %d", row.Health.ConsecutiveFailures)
	}
}
