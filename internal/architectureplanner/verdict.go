package architectureplanner

import (
	"fmt"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// DefaultEscalationThreshold is the number of consecutive reshapes after which
// a row is auto-marked NeedsHuman if its latest L4 outcome is still_failing.
// Sourced from PLANNER_ESCALATION_THRESHOLD; see Config.EscalationThreshold.
const DefaultEscalationThreshold = 3

// StampVerdicts applies deterministic PlannerVerdict updates to the after-doc
// in two passes. First, every row whose spec was reshaped this run gets its
// ReshapeCount incremented and LastReshape set. Second, rows whose latest L4
// evaluation outcome shows persistent failure past the escalation threshold
// are marked NeedsHuman.
//
// NeedsHuman is sticky: once true, this function never auto-unsets it, even
// if a later evaluation reports the row as unstuck. LastOutcome is still
// refreshed so operators can see the most recent signal alongside the sticky
// escalation reason.
//
// Returns the list of rows whose verdict materially changed for ledger
// emission as RowChange{Kind:"verdict_set"}. The function is idempotent on
// the verdict-stamp pass: a second call with no new rowsChanged and the same
// outcomes produces no additional verdict_set entries (NeedsHuman is sticky,
// so the threshold-trigger arm short-circuits).
func StampVerdicts(afterDoc *progress.Progress, rowsChanged []RowChange, outcomes []ReshapeOutcome, threshold int, now time.Time) []RowChange {
	if afterDoc == nil {
		return nil
	}
	if threshold <= 0 {
		threshold = DefaultEscalationThreshold
	}

	idx := indexItems(afterDoc)
	nowStr := now.UTC().Format(time.RFC3339)

	var changed []RowChange

	// Pass 1: increment ReshapeCount for every reshaped row this run.
	for _, rc := range rowsChanged {
		if rc.Kind != "spec_changed" {
			continue
		}
		key := itemKey{rc.PhaseID, rc.SubphaseID, rc.ItemName}
		item, ok := idx[key]
		if !ok {
			continue
		}
		if item.PlannerVerdict == nil {
			item.PlannerVerdict = &progress.PlannerVerdict{}
		}
		item.PlannerVerdict.ReshapeCount++
		item.PlannerVerdict.LastReshape = nowStr
		changed = append(changed, RowChange{
			PhaseID:    rc.PhaseID,
			SubphaseID: rc.SubphaseID,
			ItemName:   rc.ItemName,
			Kind:       "verdict_set",
			Detail:     "reshape_count incremented",
		})
	}

	// Pass 2: apply L4 outcome-driven updates. LastOutcome is always
	// refreshed (even when NeedsHuman is sticky) so the planner has a
	// current view of what autoloop did. Sticky semantic: NeedsHuman is
	// only ever set true here, never cleared.
	for _, oc := range outcomes {
		key := itemKey{oc.PhaseID, oc.SubphaseID, oc.ItemName}
		item, ok := idx[key]
		if !ok {
			continue
		}
		if item.PlannerVerdict == nil {
			item.PlannerVerdict = &progress.PlannerVerdict{}
		}
		v := item.PlannerVerdict
		v.LastOutcome = oc.Outcome

		if oc.Outcome == "still_failing" && !v.NeedsHuman && v.ReshapeCount >= threshold {
			v.NeedsHuman = true
			v.Reason = fmt.Sprintf("auto: %d reshapes without unsticking; last category %s", v.ReshapeCount, oc.LastFailure)
			v.Since = nowStr
			changed = append(changed, RowChange{
				PhaseID:    oc.PhaseID,
				SubphaseID: oc.SubphaseID,
				ItemName:   oc.ItemName,
				Kind:       "verdict_set",
				Detail:     "needs_human=true",
			})
		}
	}

	return changed
}
