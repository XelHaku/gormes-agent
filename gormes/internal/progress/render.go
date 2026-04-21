package progress

import (
	"fmt"
	"sort"
	"strings"
)

// statusIcon maps derived status to the glyph shown in markdown tables.
func statusIcon(s Status) string {
	switch s {
	case StatusComplete:
		return "✅"
	case StatusInProgress:
		return "🔨"
	default:
		return "⏳"
	}
}

// RenderReadmeRollup returns the 6-row phase table inserted into the
// README's `## Architecture` section between the PROGRESS markers.
func RenderReadmeRollup(p *Progress) string {
	var b strings.Builder
	b.WriteString("| Phase | Status | Shipped |\n")
	b.WriteString("|-------|--------|---------|\n")
	for _, key := range sortedKeys(p.Phases) {
		ph := p.Phases[key]
		total := len(ph.Subphases)
		complete := 0
		for _, sp := range ph.Subphases {
			if sp.DerivedStatus() == StatusComplete {
				complete++
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %d/%d subphases |\n",
			ph.Name, statusIcon(ph.DerivedStatus()), complete, total)
	}
	return b.String()
}

// sortedKeys returns the phase keys in lexicographic order. Phase keys
// are expected to be single characters ("1".."9"); a key ≥ "10" would
// sort before "2" under lex ordering. Revisit if the roadmap ever grows
// past 9 phases.
func sortedKeys(m map[string]Phase) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// RenderDocsChecklist returns the full item-level checklist embedded
// in _index.md between the PROGRESS markers. Emits:
//   - an **Overall** stats line
//   - a phase-level table matching the README rollup
//   - a per-subphase section with - [x] / - [ ] checkboxes
func RenderDocsChecklist(p *Progress) string {
	s := p.Stats()
	var b strings.Builder

	fmt.Fprintf(&b, "**Overall:** %d/%d subphases shipped · %d in progress · %d planned\n\n",
		s.Subphases.Complete, s.Subphases.Total, s.Subphases.InProgress, s.Subphases.Planned)

	b.WriteString(RenderReadmeRollup(p))
	b.WriteString("\n---\n\n")

	for _, key := range sortedKeys(p.Phases) {
		ph := p.Phases[key]
		fmt.Fprintf(&b, "## %s %s\n\n", ph.Name, statusIcon(ph.DerivedStatus()))
		if ph.Deliverable != "" {
			fmt.Fprintf(&b, "*%s*\n\n", ph.Deliverable)
		}
		for _, spKey := range sortedSubKeys(ph.Subphases) {
			sp := ph.Subphases[spKey]
			fmt.Fprintf(&b, "### %s — %s %s\n\n", spKey, sp.Name, statusIcon(sp.DerivedStatus()))
			if len(sp.Items) == 0 {
				fmt.Fprintf(&b, "*(no item breakdown — tracked at subphase level: %s)*\n\n", sp.Status)
				continue
			}
			for _, it := range sp.Items {
				box := "[ ]"
				if it.Status == StatusComplete {
					box = "[x]"
				}
				fmt.Fprintf(&b, "- %s %s\n", box, it.Name)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

// sortedSubKeys returns the subphase keys in lexicographic order.
// Keys like "2.B.1", "2.B.2" sort correctly under lex. The single-digit
// caveat on sortedKeys applies here at the sub-level (e.g. "3.E.10"
// would precede "3.E.2"), which is not currently encountered.
func sortedSubKeys(m map[string]Subphase) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
