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

func sortedKeys(m map[string]Phase) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
