package progress

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestSymmetricPreservation_AutoloopWritesPreserveVerdict verifies that
// autoloop's ApplyHealthUpdates does not erase Item.PlannerVerdict, which
// the planner owns. The preservation is structural via typed JSON round-trip.
func TestSymmetricPreservation_AutoloopWritesPreserveVerdict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	body := `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-1", "status": "planned", "contract": "do x",
             "health": {"attempt_count": 2, "consecutive_failures": 2},
             "planner_verdict": {"reshape_count": 1, "last_outcome": "still_failing"}}
          ]
        }
      }
    }
  }
}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Autoloop-side write: increment Health.AttemptCount.
	err := ApplyHealthUpdates(path, []HealthUpdate{{
		PhaseID: "1", SubphaseID: "1.A", ItemName: "row-1",
		Mutate: func(h *RowHealth) {
			h.AttemptCount = 3
			h.ConsecutiveFailures = 3
		},
	}})
	if err != nil {
		t.Fatalf("ApplyHealthUpdates: %v", err)
	}

	prog, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	row := &prog.Phases["1"].Subphases["1.A"].Items[0]
	if row.PlannerVerdict == nil {
		t.Fatal("PlannerVerdict was erased by autoloop's write")
	}
	if row.PlannerVerdict.ReshapeCount != 1 {
		t.Fatalf("PlannerVerdict.ReshapeCount = %d, want 1 (preserved)", row.PlannerVerdict.ReshapeCount)
	}
	if row.PlannerVerdict.LastOutcome != "still_failing" {
		t.Fatalf("PlannerVerdict.LastOutcome = %q, want still_failing (preserved)", row.PlannerVerdict.LastOutcome)
	}
	// The Health update did land:
	if row.Health.AttemptCount != 3 {
		t.Fatalf("Health.AttemptCount = %d, want 3", row.Health.AttemptCount)
	}
}

// TestSymmetricPreservation_PlannerWritesPreserveHealth verifies that a
// SaveProgress call with verdict-only changes preserves Health.
func TestSymmetricPreservation_PlannerWritesPreserveHealth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	body := `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-1", "status": "planned", "contract": "do x",
             "health": {"attempt_count": 2, "consecutive_failures": 2,
                        "quarantine": {"reason": "auto", "threshold": 3, "spec_hash": "abc"}}}
          ]
        }
      }
    }
  }
}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	prog, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	originalHealth := *prog.Phases["1"].Subphases["1.A"].Items[0].Health

	// Planner-side write: stamp PlannerVerdict (mimics StampVerdicts).
	prog.Phases["1"].Subphases["1.A"].Items[0].PlannerVerdict = &PlannerVerdict{
		ReshapeCount: 1,
		LastReshape:  "2026-04-24T12:00:00Z",
		LastOutcome:  "still_failing",
	}
	if err := SaveProgress(path, prog); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	// Reload and verify Health survived byte-equal.
	prog2, err := Load(path)
	if err != nil {
		t.Fatalf("Load 2: %v", err)
	}
	row := &prog2.Phases["1"].Subphases["1.A"].Items[0]
	if !reflect.DeepEqual(*row.Health, originalHealth) {
		t.Fatalf("Health was modified by planner's write\nbefore: %+v\nafter:  %+v", originalHealth, *row.Health)
	}
	if row.PlannerVerdict == nil || row.PlannerVerdict.ReshapeCount != 1 {
		t.Fatal("PlannerVerdict was not persisted")
	}
}

// TestSymmetricPreservation_BothBlocksRoundTrip combines both directions
// and asserts the spec hash is stable after a full round-trip with both
// blocks populated.
func TestSymmetricPreservation_BothBlocksRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	body := `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-1", "status": "planned", "contract": "do x", "blocked_by": ["dep-a"],
             "health": {"attempt_count": 1},
             "planner_verdict": {"needs_human": true, "reason": "auto", "reshape_count": 4}}
          ]
        }
      }
    }
  }
}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	prog, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	hashBefore := ItemSpecHash(&prog.Phases["1"].Subphases["1.A"].Items[0])

	if err := SaveProgress(path, prog); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}

	prog2, err := Load(path)
	if err != nil {
		t.Fatalf("Load 2: %v", err)
	}
	row := &prog2.Phases["1"].Subphases["1.A"].Items[0]
	hashAfter := ItemSpecHash(row)

	if hashBefore != hashAfter {
		t.Fatalf("spec hash changed across round-trip:\nbefore: %s\nafter:  %s", hashBefore, hashAfter)
	}
	if row.Health == nil || row.PlannerVerdict == nil {
		t.Fatal("one of the blocks went missing across round-trip")
	}
	if !row.PlannerVerdict.NeedsHuman {
		t.Fatal("PlannerVerdict.NeedsHuman flipped across round-trip")
	}
}

// TestSymmetricPreservation_FourBlocksRoundTrip verifies that all four typed
// blocks (Health, PlannerVerdict, Provenance, DriftState) survive both writers:
// planner-side SaveProgress AND autoloop-side ApplyHealthUpdates.
func TestSymmetricPreservation_FourBlocksRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	body := `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "drift_state": {"status": "owned", "origin_decision": "test"},
          "items": [
            {"name": "row-1", "status": "planned", "contract": "do x",
             "health": {"attempt_count": 1},
             "planner_verdict": {"reshape_count": 2, "last_outcome": "still_failing"},
             "provenance": {"origin_type": "gormes", "owned_since": "2026-04-25T00:00:00Z"}}
          ]
        }
      }
    }
  }
}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Round-trip via SaveProgress (planner side) — all 4 blocks survive.
	prog, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := SaveProgress(path, prog); err != nil {
		t.Fatalf("SaveProgress: %v", err)
	}
	prog2, _ := Load(path)
	row := &prog2.Phases["1"].Subphases["1.A"].Items[0]
	if row.Health == nil || row.PlannerVerdict == nil || row.Provenance == nil {
		t.Fatal("one of the row-level typed blocks went missing")
	}
	sub2 := prog2.Phases["1"].Subphases["1.A"]
	if sub2.DriftState == nil {
		t.Fatal("DriftState went missing on round-trip")
	}

	// Round-trip via ApplyHealthUpdates (autoloop side) — all 4 blocks survive.
	if err := ApplyHealthUpdates(path, []HealthUpdate{{
		PhaseID: "1", SubphaseID: "1.A", ItemName: "row-1",
		Mutate: func(h *RowHealth) {
			h.AttemptCount = 5
		},
	}}); err != nil {
		t.Fatalf("ApplyHealthUpdates: %v", err)
	}
	prog3, _ := Load(path)
	row3 := &prog3.Phases["1"].Subphases["1.A"].Items[0]
	if row3.PlannerVerdict == nil || row3.PlannerVerdict.ReshapeCount != 2 {
		t.Fatal("PlannerVerdict erased by autoloop write")
	}
	if row3.Provenance == nil || row3.Provenance.OriginType != "gormes" {
		t.Fatal("Provenance erased by autoloop write")
	}
	sub3 := prog3.Phases["1"].Subphases["1.A"]
	if sub3.DriftState == nil || sub3.DriftState.Status != "owned" {
		t.Fatal("DriftState erased by autoloop write")
	}
}
