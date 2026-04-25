package architectureplanner

import (
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func TestBuildPrompt_IncludesHealthClauses(t *testing.T) {
	bundle := ContextBundle{
		QuarantinedRows: []QuarantinedRowContext{
			{
				PhaseID:      "2",
				SubphaseID:   "2.B",
				ItemName:     "row-x",
				Contract:     "do thing",
				LastCategory: progress.FailureWorkerError,
				AttemptCount: 4,
			},
		},
	}
	prompt := BuildPrompt(bundle, nil)
	wants := []string{
		"HEALTH BLOCK PRESERVATION (HARD RULE)",
		"QUARANTINE PRIORITY (SOFT RULE)",
		"row-x", // call-to-action surfaces the row
	}
	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("BuildPrompt missing %q\nprompt:\n%s", want, prompt)
		}
	}
}

func TestBuildPrompt_NoQuarantinedRowsOmitsCallToAction(t *testing.T) {
	bundle := ContextBundle{}
	prompt := BuildPrompt(bundle, nil)
	// Hard rule and soft rule still appear (they're rule clauses, not data).
	if !strings.Contains(prompt, "HEALTH BLOCK PRESERVATION") {
		t.Fatal("prompt missing health preservation clause when no quarantined rows")
	}
	// But the call-to-action section should NOT appear when there are zero rows.
	if strings.Contains(prompt, "Quarantined Rows (Top Priority for Repair)") {
		t.Fatal("call-to-action section should be omitted when zero quarantined rows")
	}
}

func TestBuildPrompt_TopicalClauseAppearsWithKeywords(t *testing.T) {
	bundle := ContextBundle{}
	prompt := BuildPrompt(bundle, []string{"honcho", "memory"})
	if !strings.Contains(prompt, "TOPICAL FOCUS") {
		t.Fatal("topical clause missing when keywords present")
	}
	if !strings.Contains(prompt, `"honcho"`) || !strings.Contains(prompt, `"memory"`) {
		t.Fatalf("topical clause should name keywords; got:\n%s", prompt)
	}
}

func TestBuildPrompt_NoTopicalClauseWithoutKeywords(t *testing.T) {
	bundle := ContextBundle{}
	prompt := BuildPrompt(bundle, nil)
	if strings.Contains(prompt, "TOPICAL FOCUS") {
		t.Fatal("topical clause should be omitted when no keywords")
	}
}
