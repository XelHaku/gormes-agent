package site

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"sort"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/progress"
)

//go:embed data/progress.json
var progressFS embed.FS

// loadEmbeddedProgress decodes the progress.json embedded at build time.
// If the embed is missing or invalid, the landing page falls back to an
// empty roadmap rather than refusing to build.
func loadEmbeddedProgress() *progress.Progress {
	b, err := progressFS.ReadFile("data/progress.json")
	if err != nil {
		return nil
	}
	var p progress.Progress
	if err := json.Unmarshal(b, &p); err != nil {
		return nil
	}
	return &p
}

// toneFor returns the CSS-class suffix used by .roadmap-status-<tone>.
//   - complete  -> "shipped"
//   - in_progress -> "progress"
//   - planned: phase "5" -> "later" (per design), otherwise "planned"
func toneFor(st progress.Status, phaseKey string) string {
	switch st {
	case progress.StatusComplete:
		return "shipped"
	case progress.StatusInProgress:
		return "progress"
	case progress.StatusPlanned:
		if phaseKey == "5" {
			return "later"
		}
		return "planned"
	}
	return "planned"
}

// itemIconFor maps item status to the glyph shown on the landing page.
func itemIconFor(st progress.Status) (icon, tone string) {
	switch st {
	case progress.StatusComplete:
		return "✓", "shipped"
	case progress.StatusInProgress:
		return "◌", "ongoing"
	default:
		return "⏳", "pending"
	}
}

// buildRoadmapPhases turns the progress.json model into the
// []RoadmapPhase slice consumed by the landing-page template.
func buildRoadmapPhases(p *progress.Progress) []RoadmapPhase {
	if p == nil {
		return nil
	}
	keys := sortPhaseKeys(p.Phases)
	out := make([]RoadmapPhase, 0, len(keys))
	for _, key := range keys {
		ph := p.Phases[key]
		items := buildItems(ph)
		total := len(ph.Subphases)
		complete := 0
		for _, sp := range ph.Subphases {
			if sp.DerivedStatus() == progress.StatusComplete {
				complete++
			}
		}
		out = append(out, RoadmapPhase{
			StatusLabel: statusLabelFor(ph.DerivedStatus(), complete, total),
			StatusTone:  toneFor(ph.DerivedStatus(), key),
			Title:       ph.Name,
			Items:       items,
		})
	}
	return out
}

func buildItems(ph progress.Phase) []RoadmapItem {
	subKeys := make([]string, 0, len(ph.Subphases))
	for k := range ph.Subphases {
		subKeys = append(subKeys, k)
	}
	sort.Strings(subKeys)

	items := make([]RoadmapItem, 0)
	for _, spKey := range subKeys {
		sp := ph.Subphases[spKey]
		icon, tone := itemIconFor(sp.DerivedStatus())
		items = append(items, RoadmapItem{
			Icon:  icon,
			Tone:  tone,
			Label: template.HTML(fmt.Sprintf("%s %s", spKey, sp.Name)),
		})
	}
	return items
}

func statusLabelFor(st progress.Status, complete, total int) string {
	switch st {
	case progress.StatusComplete:
		return fmt.Sprintf("SHIPPED · %d/%d", complete, total)
	case progress.StatusInProgress:
		return fmt.Sprintf("IN PROGRESS · %d/%d", complete, total)
	default:
		return fmt.Sprintf("PLANNED · 0/%d", total)
	}
}

func sortPhaseKeys(m map[string]progress.Phase) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// progressTrackerLabel returns the "N/M shipped" headline text shown
// above the roadmap. Falls back to an empty string if the embed fails.
func progressTrackerLabel() string {
	p := loadEmbeddedProgress()
	if p == nil {
		return ""
	}
	s := p.Stats()
	return fmt.Sprintf("%d/%d shipped", s.Subphases.Complete, s.Subphases.Total)
}
