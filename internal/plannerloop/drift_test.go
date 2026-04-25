package plannerloop

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func TestDiffSubphaseStates_RecordsForwardTransitions(t *testing.T) {
	before := progressWithDrift(map[string]string{
		"2.B": "porting",
		"5.O": "converged",
	})
	after := progressWithDrift(map[string]string{
		"2.B": "converged",
		"5.O": "owned",
	})
	after.Phases["1"].Subphases["2.B"].DriftState.OriginDecision = "matches upstream"
	after.Phases["1"].Subphases["5.O"].DriftState.OriginDecision = "Gormes-owned planner surface"

	got := diffSubphaseStates(before, after)
	if len(got) != 2 {
		t.Fatalf("diffSubphaseStates() len = %d, want 2: %+v", len(got), got)
	}
	if got[0].SubphaseID != "1.2.B" || got[0].From != "porting" || got[0].To != "converged" || got[0].Reason != "matches upstream" {
		t.Fatalf("first promotion = %+v", got[0])
	}
	if got[1].SubphaseID != "1.5.O" || got[1].From != "converged" || got[1].To != "owned" {
		t.Fatalf("second promotion = %+v", got[1])
	}
}

func TestDiffSubphaseStates_IgnoresBackwardOrUnchangedTransitions(t *testing.T) {
	before := progressWithDrift(map[string]string{
		"2.B": "owned",
		"5.O": "converged",
	})
	after := progressWithDrift(map[string]string{
		"2.B": "porting",
		"5.O": "converged",
	})

	if got := diffSubphaseStates(before, after); len(got) != 0 {
		t.Fatalf("diffSubphaseStates() = %+v, want no promotions", got)
	}
}

func progressWithDrift(states map[string]string) *progress.Progress {
	subphases := map[string]progress.Subphase{}
	for subphaseID, status := range states {
		subphases[subphaseID] = progress.Subphase{
			Name: subphaseID,
			DriftState: &progress.DriftState{
				Status: status,
			},
		}
	}
	return &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {
				Name:      "P",
				Subphases: subphases,
			},
		},
	}
}
