package architectureplanner

import (
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/plannertriggers"
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

func TestBuildPrompt_RecentAutoloopSignalsSectionRendered(t *testing.T) {
	bundle := ContextBundle{
		TriggerEvents: []plannertriggers.TriggerEvent{
			{
				ID:         "evt-1",
				Kind:       "quarantine_added",
				PhaseID:    "2",
				SubphaseID: "2.B",
				ItemName:   "row-x",
				Reason:     "3rd consecutive failure",
			},
			{
				ID:         "evt-2",
				Kind:       "quarantine_stale_cleared",
				PhaseID:    "3",
				SubphaseID: "3.A",
				ItemName:   "row-y",
				Reason:     "spec hash changed",
			},
		},
	}
	prompt := BuildPrompt(bundle, nil)
	wants := []string{
		"## Recent Autoloop Signals (Since Last Planner Run)",
		"2/2.B/row-x — quarantine_added — 3rd consecutive failure",
		"3/3.A/row-y — quarantine_stale_cleared — spec hash changed",
	}
	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("BuildPrompt missing %q\nprompt:\n%s", want, prompt)
		}
	}
}

func TestBuildPrompt_RecentAutoloopSignalsOmittedWhenEmpty(t *testing.T) {
	bundle := ContextBundle{}
	prompt := BuildPrompt(bundle, nil)
	if strings.Contains(prompt, "Recent Autoloop Signals") {
		t.Fatal("Recent Autoloop Signals section should be omitted when no events")
	}
}

func TestBuildPrompt_SelfEvaluationClauseAlwaysPresent(t *testing.T) {
	// SELF-EVALUATION (SOFT RULE) is unconditional, like the HARD/SOFT
	// quarantine clauses. The data section beneath it appears only when
	// PreviousReshapes is non-empty.
	bundle := ContextBundle{}
	prompt := BuildPrompt(bundle, nil)
	if !strings.Contains(prompt, "SELF-EVALUATION (SOFT RULE)") {
		t.Fatalf("BuildPrompt must include SELF-EVALUATION clause unconditionally\nprompt:\n%s", prompt)
	}
	// The DATA section header (a markdown ## with the "Last 7 Days"
	// qualifier) should be omitted when no reshapes are present. The
	// SOFT clause itself mentions "Previous Reshape Outcomes" by name to
	// tell the LLM what the missing section means, so we look for the
	// concrete header instead.
	if strings.Contains(prompt, "## Previous Reshape Outcomes (Last 7 Days)") {
		t.Fatal("Previous Reshape Outcomes data section should be omitted when no reshapes")
	}
}

func TestBuildPrompt_PreviousReshapesSectionRendersAllBuckets(t *testing.T) {
	bundle := ContextBundle{
		PreviousReshapes: []ReshapeOutcome{
			{
				PhaseID: "2", SubphaseID: "2.B", ItemName: "row-unstuck",
				ReshapedAt: "2026-04-24T12:00:00Z", ReshapedBy: "planner-1",
				Outcome: "unstuck", LastSuccess: "2026-04-24T13:00:00Z",
			},
			{
				PhaseID: "3", SubphaseID: "3.A", ItemName: "row-stuck",
				ReshapedAt: "2026-04-24T12:00:00Z", ReshapedBy: "planner-1",
				Outcome: "still_failing", AutoloopRuns: 4, LastFailure: "report_validation_failed",
			},
			{
				PhaseID: "4", SubphaseID: "4.A", ItemName: "row-untouched",
				ReshapedAt: "2026-04-24T12:00:00Z", ReshapedBy: "planner-1",
				Outcome: "no_attempts_yet",
			},
		},
	}
	prompt := BuildPrompt(bundle, nil)
	wants := []string{
		"## Previous Reshape Outcomes (Last 7 Days)",
		"UNSTUCK (1):",
		"STILL FAILING (1):",
		"NO ATTEMPTS YET (1):",
		"row-unstuck",
		"row-stuck",
		"row-untouched",
		"autoloop attempted 4 times",
		"report_validation_failed",
	}
	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("BuildPrompt missing %q\nprompt:\n%s", want, prompt)
		}
	}
}
