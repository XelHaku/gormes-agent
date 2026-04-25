package plannerloop

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func docWithItem(item progress.Item) *progress.Progress {
	return &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {
				Name: "P",
				Subphases: map[string]progress.Subphase{
					"1.A": {Name: "S", Items: []progress.Item{item}},
				},
			},
		},
	}
}

func TestValidateHealthPreservation_IdenticalAccepted(t *testing.T) {
	h := &progress.RowHealth{AttemptCount: 3, ConsecutiveFailures: 1}
	before := docWithItem(progress.Item{Name: "x", Status: progress.StatusInProgress, Contract: "c", Health: h})
	after := docWithItem(progress.Item{Name: "x", Status: progress.StatusInProgress, Contract: "c", Health: h})
	if err := validateHealthPreservation(before, after); err != nil {
		t.Fatalf("expected accepted, got %v", err)
	}
}

func TestValidateHealthPreservation_ModifiedHealthRejected(t *testing.T) {
	before := docWithItem(progress.Item{Name: "x", Contract: "c", Health: &progress.RowHealth{AttemptCount: 3}})
	after := docWithItem(progress.Item{Name: "x", Contract: "c", Health: &progress.RowHealth{AttemptCount: 99}})
	if err := validateHealthPreservation(before, after); err == nil {
		t.Fatal("expected error when health was modified")
	}
}

func TestValidateHealthPreservation_DroppedHealthRejected(t *testing.T) {
	before := docWithItem(progress.Item{Name: "x", Contract: "c", Health: &progress.RowHealth{AttemptCount: 3}})
	after := docWithItem(progress.Item{Name: "x", Contract: "c", Health: nil})
	if err := validateHealthPreservation(before, after); err == nil {
		t.Fatal("expected error when health was dropped")
	}
}

func TestValidateHealthPreservation_DeletedRowAccepted(t *testing.T) {
	before := docWithItem(progress.Item{Name: "x", Contract: "c", Health: &progress.RowHealth{AttemptCount: 3}})
	after := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{"1.A": {Name: "S", Items: nil}}},
		},
	}
	if err := validateHealthPreservation(before, after); err != nil {
		t.Fatalf("deletion should be accepted, got %v", err)
	}
}

func TestValidateHealthPreservation_SplitRowAccepted(t *testing.T) {
	before := docWithItem(progress.Item{Name: "x", Contract: "umbrella", Health: &progress.RowHealth{AttemptCount: 3}})
	after := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "x-a", Contract: "split a"},
					{Name: "x-b", Contract: "split b"},
				}},
			}},
		},
	}
	if err := validateHealthPreservation(before, after); err != nil {
		t.Fatalf("split (rename) should be accepted, got %v", err)
	}
}

func TestValidateHealthPreservation_SpecChangedHealthPreservedAccepted(t *testing.T) {
	h := &progress.RowHealth{AttemptCount: 3}
	before := docWithItem(progress.Item{Name: "x", Contract: "old", Health: h})
	after := docWithItem(progress.Item{Name: "x", Contract: "NEW SPEC", Health: h})
	if err := validateHealthPreservation(before, after); err != nil {
		t.Fatalf("spec change with health preserved should be accepted, got %v", err)
	}
}
