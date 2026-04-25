package architectureplanner

import (
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func TestStampVerdicts_IncrementsReshapeCount(t *testing.T) {
	doc := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Contract: "c"},
				}},
			}},
		},
	}
	rowsChanged := []RowChange{{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Kind: "spec_changed"}}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	StampVerdicts(doc, rowsChanged, nil, 3, now)
	row := &doc.Phases["1"].Subphases["1.A"].Items[0]
	if row.PlannerVerdict == nil || row.PlannerVerdict.ReshapeCount != 1 {
		t.Fatalf("ReshapeCount expected 1, got %+v", row.PlannerVerdict)
	}
	if row.PlannerVerdict.LastReshape != now.Format(time.RFC3339) {
		t.Fatal("LastReshape should be set to now")
	}
}

func TestStampVerdicts_SetsNeedsHumanWhenThresholdReachedAndStillFailing(t *testing.T) {
	doc := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Contract: "c", PlannerVerdict: &progress.PlannerVerdict{ReshapeCount: 2}},
				}},
			}},
		},
	}
	rowsChanged := []RowChange{{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Kind: "spec_changed"}}
	outcomes := []ReshapeOutcome{
		{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Outcome: "still_failing", LastFailure: "report_validation_failed"},
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	StampVerdicts(doc, rowsChanged, outcomes, 3, now)
	row := &doc.Phases["1"].Subphases["1.A"].Items[0]
	if !row.PlannerVerdict.NeedsHuman {
		t.Fatal("NeedsHuman should be set after threshold")
	}
	if row.PlannerVerdict.Reason == "" {
		t.Fatal("Reason should be set")
	}
}

func TestStampVerdicts_NeedsHumanIsSticky(t *testing.T) {
	doc := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Contract: "c", PlannerVerdict: &progress.PlannerVerdict{
						NeedsHuman: true, Reason: "original reason", ReshapeCount: 5,
					}},
				}},
			}},
		},
	}
	outcomes := []ReshapeOutcome{
		{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Outcome: "unstuck", LastSuccess: "now"},
	}
	StampVerdicts(doc, nil, outcomes, 3, time.Now())
	row := &doc.Phases["1"].Subphases["1.A"].Items[0]
	if !row.PlannerVerdict.NeedsHuman {
		t.Fatal("NeedsHuman must remain true (sticky)")
	}
	if row.PlannerVerdict.Reason != "original reason" {
		t.Fatal("Reason should not be overwritten")
	}
	if row.PlannerVerdict.LastOutcome != "unstuck" {
		t.Fatal("LastOutcome should be updated even when NeedsHuman is sticky")
	}
}

func TestStampVerdicts_DoesNotSetNeedsHumanIfUnstuck(t *testing.T) {
	doc := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Contract: "c", PlannerVerdict: &progress.PlannerVerdict{ReshapeCount: 10}},
				}},
			}},
		},
	}
	outcomes := []ReshapeOutcome{
		{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Outcome: "unstuck"},
	}
	StampVerdicts(doc, nil, outcomes, 3, time.Now())
	row := &doc.Phases["1"].Subphases["1.A"].Items[0]
	if row.PlannerVerdict.NeedsHuman {
		t.Fatal("unstuck row should NOT trigger NeedsHuman regardless of ReshapeCount")
	}
}

func TestStampVerdicts_ReturnsVerdictChangesForLedger(t *testing.T) {
	doc := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Contract: "c"},
				}},
			}},
		},
	}
	rowsChanged := []RowChange{{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Kind: "spec_changed"}}
	changes := StampVerdicts(doc, rowsChanged, nil, 3, time.Now())
	if len(changes) != 1 || changes[0].Kind != "verdict_set" {
		t.Fatalf("expected one verdict_set change, got %+v", changes)
	}
}

// TestStampVerdicts_IdempotentOnVerdictPass guards the sticky semantic on the
// second call: once NeedsHuman is true and ReshapeCount has been incremented,
// re-running with the SAME outcomes (and no new rowsChanged) must not
// re-trigger the NeedsHuman arm or otherwise emit verdict_set entries.
// The LastOutcome refresh is a no-op when the value is unchanged.
func TestStampVerdicts_IdempotentOnVerdictPass(t *testing.T) {
	doc := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Contract: "c", PlannerVerdict: &progress.PlannerVerdict{
						NeedsHuman:   true,
						Reason:       "auto: 3 reshapes without unsticking; last category report_validation_failed",
						ReshapeCount: 3,
						LastOutcome:  "still_failing",
					}},
				}},
			}},
		},
	}
	outcomes := []ReshapeOutcome{
		{PhaseID: "1", SubphaseID: "1.A", ItemName: "row-x", Outcome: "still_failing", LastFailure: "report_validation_failed"},
	}
	changes := StampVerdicts(doc, nil, outcomes, 3, time.Now())
	if len(changes) != 0 {
		t.Fatalf("expected zero verdict_set changes on idempotent call, got %+v", changes)
	}
	row := &doc.Phases["1"].Subphases["1.A"].Items[0]
	if row.PlannerVerdict.ReshapeCount != 3 {
		t.Fatalf("ReshapeCount mutated on idempotent call: got %d, want 3", row.PlannerVerdict.ReshapeCount)
	}
	if row.PlannerVerdict.Reason != "auto: 3 reshapes without unsticking; last category report_validation_failed" {
		t.Fatalf("Reason mutated on idempotent call: %q", row.PlannerVerdict.Reason)
	}
}
