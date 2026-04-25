package plannerloop

import (
	"log"
	"sort"
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// diffSubphaseStates walks both progress docs by (phaseID, subphaseID) and
// records DriftState.Status transitions. Forward transitions
// (porting->converged, porting->owned, converged->owned) are returned as
// DriftPromotion entries. Backward transitions are logged via the standard
// log package as a warning but NOT emitted: the planner runtime never
// demotes, so a backward edge means a human edited progress.json directly
// and the operator should see the warning rather than have the ledger
// silently swallow the change. Subphases present in only one doc, or where
// the status did not change, contribute nothing.
//
// Subphases without a DriftState block default to "porting" — matching the
// L3 PROVENANCE / DRIFT STATE prompt clause's documented default and the
// status surface bucket counter (renderDriftStateBuckets). This means
// `nil -> &{Status:"porting"}` is a no-op (correctly), while
// `nil -> &{Status:"converged"}` IS recorded as a porting->converged
// promotion (the planner stamped the row's first DriftState directly into
// the converged bucket).
//
// Returns nil when either doc is nil (no diff possible) or no transitions
// occurred. Output is sorted by SubphaseID for deterministic ledger output.
func diffSubphaseStates(before, after *progress.Progress) []DriftPromotion {
	if before == nil || after == nil {
		return nil
	}

	var promotions []DriftPromotion
	for phaseID, afterPhase := range after.Phases {
		beforePhase, ok := before.Phases[phaseID]
		if !ok {
			continue
		}
		for subphaseID, afterSubphase := range afterPhase.Subphases {
			beforeSubphase, ok := beforePhase.Subphases[subphaseID]
			if !ok {
				continue
			}
			id := phaseID + "." + subphaseID
			from := driftStatusOrPorting(beforeSubphase)
			to := driftStatusOrPorting(afterSubphase)
			if from == to {
				continue
			}
			if !isForwardDriftTransition(from, to) {
				// Backward (or unknown) transition: log but do not emit.
				// Humans demote via direct edit; planner runtime never
				// demotes. Surfacing the warning lets operators spot
				// unexpected manual edits without polluting the ledger.
				log.Printf("planner: drift_state backward transition on %s: %s -> %s (not emitted)",
					id, from, to)
				continue
			}
			reason := ""
			if afterSubphase.DriftState != nil {
				reason = strings.TrimSpace(afterSubphase.DriftState.OriginDecision)
			}
			promotions = append(promotions, DriftPromotion{
				SubphaseID: id,
				From:       from,
				To:         to,
				Reason:     reason,
			})
		}
	}

	sort.Slice(promotions, func(i, j int) bool {
		if promotions[i].SubphaseID != promotions[j].SubphaseID {
			return promotions[i].SubphaseID < promotions[j].SubphaseID
		}
		if promotions[i].From != promotions[j].From {
			return promotions[i].From < promotions[j].From
		}
		return promotions[i].To < promotions[j].To
	})
	return promotions
}

func isForwardDriftTransition(from, to string) bool {
	fromRank, fromOK := driftStatusRank(from)
	toRank, toOK := driftStatusRank(to)
	return fromOK && toOK && toRank > fromRank
}

func driftStatusRank(status string) (int, bool) {
	switch status {
	case "porting":
		return 0, true
	case "converged":
		return 1, true
	case "owned":
		return 2, true
	default:
		return 0, false
	}
}

// driftStatusOrPorting returns the DriftState.Status for a subphase, or
// "porting" when DriftState is unset. Centralised so the bucket count in
// the status surface and the diff helper agree on the default semantics
// the L3 prompt clause documents.
func driftStatusOrPorting(sub progress.Subphase) string {
	if sub.DriftState == nil || strings.TrimSpace(sub.DriftState.Status) == "" {
		return "porting"
	}
	return sub.DriftState.Status
}
