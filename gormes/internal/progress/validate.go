package progress

import "fmt"

// Validate enforces schema invariants:
//   - meta.version is "2.0"
//   - every item.status is in {complete, in_progress, planned}
//   - every subphase has exactly one of items or an explicit status
//   - every explicit subphase status is in the allowed set
func Validate(p *Progress) error {
	if p.Meta.Version != "2.0" {
		return fmt.Errorf("progress: meta.version = %q, want %q", p.Meta.Version, "2.0")
	}
	for phKey, ph := range p.Phases {
		for spKey, sp := range ph.Subphases {
			hasItems := len(sp.Items) > 0
			hasStatus := sp.Status != ""
			if hasItems == hasStatus { // both true or both false
				return fmt.Errorf("progress: phase %s subphase %s must have exactly one of items or status", phKey, spKey)
			}
			if hasStatus && !validStatus(sp.Status) {
				return fmt.Errorf("progress: phase %s subphase %s: invalid status %q", phKey, spKey, sp.Status)
			}
			for i, it := range sp.Items {
				if !validStatus(it.Status) {
					return fmt.Errorf("progress: phase %s subphase %s item[%d] (%q): invalid status %q",
						phKey, spKey, i, it.Name, it.Status)
				}
			}
		}
	}
	return nil
}

func validStatus(s Status) bool {
	return s == StatusComplete || s == StatusInProgress || s == StatusPlanned
}
