package progress

import (
	"errors"
	"fmt"
	"sort"
)

// Validate enforces schema invariants. All violations are reported,
// not just the first — authors should be able to see the full list
// and fix them in one pass. Phase and subphase keys are iterated in
// sorted order so output is deterministic.
func Validate(p *Progress) error {
	if p.Meta.Version != "2.0" {
		return fmt.Errorf("progress: meta.version = %q, want %q", p.Meta.Version, "2.0")
	}
	var errs []error
	for _, phKey := range sortedPhaseKeys(p.Phases) {
		ph := p.Phases[phKey]
		for _, spKey := range sortedSubphaseKeys(ph.Subphases) {
			sp := ph.Subphases[spKey]
			hasItems := len(sp.Items) > 0
			hasStatus := sp.Status != ""
			if hasItems == hasStatus {
				errs = append(errs, fmt.Errorf("progress: phase %s subphase %s must have exactly one of items or status", phKey, spKey))
				continue // further item-level checks would just add noise
			}
			if hasStatus && !validStatus(sp.Status) {
				errs = append(errs, fmt.Errorf("progress: phase %s subphase %s: invalid status %q", phKey, spKey, sp.Status))
			}
			for i, it := range sp.Items {
				if !validStatus(it.Status) {
					errs = append(errs, fmt.Errorf("progress: phase %s subphase %s item[%d] (%q): invalid status %q",
						phKey, spKey, i, it.Name, it.Status))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func validStatus(s Status) bool {
	return s == StatusComplete || s == StatusInProgress || s == StatusPlanned
}

func sortedPhaseKeys(m map[string]Phase) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSubphaseKeys(m map[string]Subphase) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
