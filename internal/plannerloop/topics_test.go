package plannerloop

import (
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func TestMatchKeywords_EmptyKeywordsMatchesAll(t *testing.T) {
	prog := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-a", Contract: "do a"},
					{Name: "row-b", Contract: "do b"},
				}},
			}},
		},
	}
	matched := matchKeywordsInDoc(prog, nil)
	if len(matched) != 2 {
		t.Fatalf("expected all 2 rows, got %d", len(matched))
	}
}

func TestMatchKeywords_SubstringMatchesItemName(t *testing.T) {
	prog := docOneItem(progress.Item{Name: "honcho-client", Contract: "x"})
	matched := matchKeywordsInDoc(prog, []string{"honcho"})
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
}

func TestMatchKeywords_MatchesContract(t *testing.T) {
	prog := docOneItem(progress.Item{Name: "row-x", Contract: "Wire Honcho client"})
	matched := matchKeywordsInDoc(prog, []string{"honcho"})
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
}

func TestMatchKeywords_MatchesSourceRefs(t *testing.T) {
	prog := docOneItem(progress.Item{
		Name:       "row-x",
		Contract:   "x",
		SourceRefs: []string{"../honcho/api.py"},
	})
	matched := matchKeywordsInDoc(prog, []string{"honcho"})
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
}

func TestMatchKeywords_MatchesSubphaseName(t *testing.T) {
	prog := &progress.Progress{
		Phases: map[string]progress.Phase{
			"3": {Name: "Memory", Subphases: map[string]progress.Subphase{
				"3.A": {Name: "Honcho integration", Items: []progress.Item{
					{Name: "row-1", Contract: "x"},
					{Name: "row-2", Contract: "y"},
				}},
			}},
		},
	}
	matched := matchKeywordsInDoc(prog, []string{"honcho"})
	if len(matched) != 2 {
		t.Fatalf("subphase name match should bring all items; got %d", len(matched))
	}
}

func TestMatchKeywords_OrSemanticsAcrossKeywords(t *testing.T) {
	prog := &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{
					{Name: "row-honcho", Contract: "x"},
					{Name: "row-memory", Contract: "y"},
					{Name: "row-other", Contract: "z"},
				}},
			}},
		},
	}
	matched := matchKeywordsInDoc(prog, []string{"honcho", "memory"})
	if len(matched) != 2 {
		t.Fatalf("OR across keywords should match 2, got %d", len(matched))
	}
}

func TestMatchKeywords_CaseInsensitive(t *testing.T) {
	prog := docOneItem(progress.Item{Name: "row-x", Contract: "Wire Honcho"})
	matched := matchKeywordsInDoc(prog, []string{"HONCHO"})
	if len(matched) != 1 {
		t.Fatalf("case-insensitive match expected; got %d", len(matched))
	}
}

func TestFilterContextByKeywords_NarrowsBundleSelectively(t *testing.T) {
	bundle := ContextBundle{
		QuarantinedRows: []QuarantinedRowContext{
			{ItemName: "honcho-row", Contract: "x"},
			{ItemName: "other-row", Contract: "y"},
		},
		ImplInventory: ImplInventory{
			GormesOriginalPaths: []string{"internal/plannerloop/run.go", "internal/gateway/server.go"},
			RecentlyChanged:     []string{"cmd/builder-loop/main.go", "cmd/gormes/main.go"},
			OwnedSubphases:      []string{"5.O", "2.B"},
		},
		AutoloopAudit: AutoloopAudit{}, // would be aggregate-only
	}
	narrowed := FilterContextByKeywords(bundle, []string{"honcho", "builder-loop", "5.O"})
	if len(narrowed.QuarantinedRows) != 1 || narrowed.QuarantinedRows[0].ItemName != "honcho-row" {
		t.Fatalf("QuarantinedRows narrowing failed: %+v", narrowed.QuarantinedRows)
	}
	if len(narrowed.ImplInventory.GormesOriginalPaths) != 0 {
		t.Fatalf("GormesOriginalPaths narrowing failed: %+v", narrowed.ImplInventory.GormesOriginalPaths)
	}
	if len(narrowed.ImplInventory.RecentlyChanged) != 1 || narrowed.ImplInventory.RecentlyChanged[0] != "cmd/builder-loop/main.go" {
		t.Fatalf("RecentlyChanged narrowing failed: %+v", narrowed.ImplInventory.RecentlyChanged)
	}
	if len(narrowed.ImplInventory.OwnedSubphases) != 1 || narrowed.ImplInventory.OwnedSubphases[0] != "5.O" {
		t.Fatalf("OwnedSubphases narrowing failed: %+v", narrowed.ImplInventory.OwnedSubphases)
	}
	// AutoloopAudit must remain intact (aggregate, not row-level).
}

// docOneItem is a small builder used by topics tests.
func docOneItem(item progress.Item) *progress.Progress {
	return &progress.Progress{
		Phases: map[string]progress.Phase{
			"1": {Name: "P", Subphases: map[string]progress.Subphase{
				"1.A": {Name: "S", Items: []progress.Item{item}},
			}},
		},
	}
}
