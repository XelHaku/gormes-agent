package architectureplanner

import (
	"errors"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func TestRetryFeedback_NamesAllDroppedRows(t *testing.T) {
	before := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Health: &progress.RowHealth{AttemptCount: 3}},
					{Name: "row-y", Health: &progress.RowHealth{AttemptCount: 2}},
				}},
			}},
		},
	}
	after := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Health: nil}, // dropped
					{Name: "row-y", Health: &progress.RowHealth{AttemptCount: 2}},
				}},
			}},
		},
	}
	feedback := RetryFeedback(errors.New("validation error"), before, after)
	if !strings.Contains(feedback, "1/1.A/row-x") {
		t.Fatalf("feedback should name dropped row, got:\n%s", feedback)
	}
	if !strings.Contains(feedback, "HEALTH BLOCK PRESERVATION") {
		t.Fatal("feedback missing HARD RULE reference")
	}
}

func TestExtractDroppedRows_FindsDroppedAndModified(t *testing.T) {
	before := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Health: &progress.RowHealth{AttemptCount: 3}},
				}},
			}},
		},
	}
	after := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Health: nil},
				}},
			}},
		},
	}
	dropped := extractDroppedRows(before, after)
	if len(dropped) != 1 || dropped[0] != "1/1.A/row-x" {
		t.Fatalf("expected 1/1.A/row-x, got %v", dropped)
	}
}

func TestExtractDroppedRows_DeletionIsNotDropped(t *testing.T) {
	before := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-x", Health: &progress.RowHealth{AttemptCount: 3}},
				}},
			}},
		},
	}
	// Row removed entirely from the after-doc — counts as intentional deletion,
	// NOT a dropped Health block.
	after := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: nil},
			}},
		},
	}
	if dropped := extractDroppedRows(before, after); len(dropped) != 0 {
		t.Fatalf("deletion should not be reported as dropped Health, got %v", dropped)
	}
}
