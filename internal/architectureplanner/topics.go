package architectureplanner

import (
	"strings"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// itemMatchesKeywords returns true if any keyword (case-insensitive
// substring) matches the item's name, contract, source_refs, write_scope,
// fixture, or any of the parent subphase/phase names.
func itemMatchesKeywords(item *progress.Item, phaseName, subphaseName string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		needle := strings.ToLower(kw)
		if strings.Contains(strings.ToLower(item.Name), needle) ||
			strings.Contains(strings.ToLower(item.Contract), needle) ||
			strings.Contains(strings.ToLower(item.Fixture), needle) ||
			strings.Contains(strings.ToLower(phaseName), needle) ||
			strings.Contains(strings.ToLower(subphaseName), needle) {
			return true
		}
		for _, ref := range item.SourceRefs {
			if strings.Contains(strings.ToLower(ref), needle) {
				return true
			}
		}
		for _, scope := range item.WriteScope {
			if strings.Contains(strings.ToLower(scope), needle) {
				return true
			}
		}
	}
	return false
}

// matchedRow identifies one item in a Progress doc with its parent IDs so
// callers can re-look-up the underlying row in the original tree.
type matchedRow struct {
	PhaseID    string
	SubphaseID string
	Item       *progress.Item
}

// matchKeywordsInDoc returns the subset of items in prog that match any of
// the keywords. Empty keywords returns all items.
func matchKeywordsInDoc(prog *progress.Progress, keywords []string) []matchedRow {
	var out []matchedRow
	if prog == nil {
		return out
	}
	for phaseID, phase := range prog.Phases {
		for subphaseID, sub := range phase.Subphases {
			for i := range sub.Items {
				it := &sub.Items[i]
				if itemMatchesKeywords(it, phase.Name, sub.Name, keywords) {
					out = append(out, matchedRow{PhaseID: phaseID, SubphaseID: subphaseID, Item: it})
				}
			}
		}
	}
	return out
}

// FilterContextByKeywords narrows the bundle's row-level slices
// (QuarantinedRows, PreviousReshapes if present) to only rows matching ANY
// of the keywords. Empty keywords returns the bundle unchanged.
// AutoloopAudit and SourceRoots are intentionally NOT narrowed: the audit
// is aggregate-level, and source roots are ground truth for orientation.
func FilterContextByKeywords(bundle ContextBundle, keywords []string) ContextBundle {
	if len(keywords) == 0 {
		return bundle
	}

	matchesAny := func(haystacks ...string) bool {
		for _, kw := range keywords {
			needle := strings.ToLower(kw)
			for _, h := range haystacks {
				if strings.Contains(strings.ToLower(h), needle) {
					return true
				}
			}
		}
		return false
	}

	narrowed := bundle
	if len(bundle.QuarantinedRows) > 0 {
		filtered := []QuarantinedRowContext{}
		for _, r := range bundle.QuarantinedRows {
			if matchesAny(r.ItemName, r.Contract) {
				filtered = append(filtered, r)
			}
		}
		narrowed.QuarantinedRows = filtered
	}
	// PreviousReshapes is added in Task 10 (L4); when present, narrow it too.
	// At Task 5 time the field doesn't exist yet — the FilterContextByKeywords
	// body will gain a matching block when Task 10 lands.
	return narrowed
}
