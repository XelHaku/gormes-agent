package architectureplanner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// DefaultMaxRetries is the default number of follow-up LLM calls the planner
// will issue after validateHealthPreservation rejects an initial regen. Set
// to 0 to disable retries (pre-L3 single-attempt behavior). Configurable via
// PLANNER_MAX_RETRIES.
const DefaultMaxRetries = 2

// retryAttempt records one LLM call's lifecycle for ledger forensics. Stored
// on LedgerEvent.Attempts so operators can see exactly which attempt failed
// and which rows were dropped at each step without having to grep raw logs.
type retryAttempt struct {
	Index       int      `json:"index"`
	Status      string   `json:"status"` // "ok" | "validation_rejected" | "backend_failed"
	Detail      string   `json:"detail,omitempty"`
	DroppedRows []string `json:"dropped_rows,omitempty"`
}

// RetryFeedback formats a one-paragraph correction prompt for the LLM after
// validateHealthPreservation rejects a regen. Names the dropped rows
// explicitly and references the HARD rule by name so the LLM can connect the
// feedback to the rule it just violated.
//
// The rejection error is passed in for forensic context but currently the
// dropped-row enumeration carries the actionable signal; the error string is
// reserved for future expansion (e.g., naming the validator that fired).
func RetryFeedback(rejection error, beforeDoc, afterDoc *progress.Progress) string {
	_ = rejection // reserved for future use; current feedback derives from the doc diff
	dropped := extractDroppedRows(beforeDoc, afterDoc)
	var b strings.Builder
	b.WriteString("\n\nRETRY: Your previous output dropped or modified the `health` block on the\n")
	b.WriteString("following rows. Per the HEALTH BLOCK PRESERVATION (HARD RULE), you must\n")
	b.WriteString("reproduce every `health` block verbatim. Please regenerate the entire\n")
	b.WriteString("progress.json output, this time preserving these rows' health metadata:\n\n")
	for _, row := range dropped {
		fmt.Fprintf(&b, "- %s\n", row)
	}
	b.WriteString("\nThe original task and quarantine priorities still apply. Do NOT re-do the\n")
	b.WriteString("upstream sync analysis or implementation inventory — just produce a corrected\n")
	b.WriteString("progress.json with the health blocks restored.\n")
	return b.String()
}

// extractDroppedRows identifies rows whose Health block was dropped or
// modified between before and after. Reuses the indexItems and healthEqual
// helpers defined in run.go so the dropped-row detection stays in sync with
// validateHealthPreservation. Rows missing from the after-doc are NOT
// reported — they're intentional deletions, not dropped Health.
//
// Output is sorted by "phaseID/subphaseID/itemName" so feedback (and ledger
// forensics) are deterministic across runs and Go map iteration order.
func extractDroppedRows(beforeDoc, afterDoc *progress.Progress) []string {
	var out []string
	beforeIndex := indexItems(beforeDoc)
	afterIndex := indexItems(afterDoc)
	for key, beforeItem := range beforeIndex {
		afterItem, exists := afterIndex[key]
		if !exists {
			continue // intentional deletion is not a "dropped health"
		}
		if !healthEqual(beforeItem.Health, afterItem.Health) {
			out = append(out, fmt.Sprintf("%s/%s/%s", key.phaseID, key.subphaseID, key.itemName))
		}
	}
	sort.Strings(out)
	return out
}
