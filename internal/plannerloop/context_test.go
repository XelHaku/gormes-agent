package plannerloop

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func itemWithQuarantine(name string, attempts int, since string) progress.Item {
	return progress.Item{
		Name:     name,
		Contract: "do " + name,
		Health: &progress.RowHealth{
			AttemptCount: attempts,
			Quarantine: &progress.Quarantine{
				Since:        since,
				Threshold:    3,
				SpecHash:     "hash-" + name,
				LastCategory: progress.FailureWorkerError,
			},
		},
	}
}

func progressWithItems(items ...progress.Item) *progress.Progress {
	return &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: items},
			}},
		},
	}
}

func TestCollectQuarantinedRows_SortsByAttemptCountThenSince(t *testing.T) {
	prog := progressWithItems(
		itemWithQuarantine("a", 2, "2026-04-24T10:00:00Z"), // fewer attempts, older
		itemWithQuarantine("b", 5, "2026-04-24T12:00:00Z"), // most attempts
		itemWithQuarantine("c", 5, "2026-04-24T11:00:00Z"), // tied attempts, older — should come before b
	)
	rows := collectQuarantinedRows(prog, AutoloopAudit{}, 0)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].ItemName != "c" {
		t.Errorf("rows[0] = %q, want c (5 attempts, older)", rows[0].ItemName)
	}
	if rows[1].ItemName != "b" {
		t.Errorf("rows[1] = %q, want b (5 attempts, newer)", rows[1].ItemName)
	}
	if rows[2].ItemName != "a" {
		t.Errorf("rows[2] = %q, want a (2 attempts)", rows[2].ItemName)
	}
}

func TestCollectQuarantinedRows_HonorsLimit(t *testing.T) {
	items := make([]progress.Item, 0, 10)
	for i := 0; i < 10; i++ {
		items = append(items, itemWithQuarantine(string(rune('a'+i)), 1, "2026-04-24T12:00:00Z"))
	}
	rows := collectQuarantinedRows(progressWithItems(items...), AutoloopAudit{}, 5)
	if len(rows) != 5 {
		t.Fatalf("expected limit=5, got %d", len(rows))
	}
}

func TestCollectQuarantinedRows_ExcludesNonQuarantined(t *testing.T) {
	prog := progressWithItems(
		itemWithQuarantine("a", 3, "2026-04-24T12:00:00Z"),
		progress.Item{Name: "b", Contract: "do b"},                                               // no Health
		progress.Item{Name: "c", Contract: "do c", Health: &progress.RowHealth{AttemptCount: 1}}, // no Quarantine
	)
	rows := collectQuarantinedRows(prog, AutoloopAudit{}, 0)
	if len(rows) != 1 {
		t.Fatalf("expected only quarantined row, got %d", len(rows))
	}
	if rows[0].ItemName != "a" {
		t.Errorf("rows[0] = %q, want a", rows[0].ItemName)
	}
}
