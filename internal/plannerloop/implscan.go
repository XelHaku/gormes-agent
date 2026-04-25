package plannerloop

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// ImplInventory is what ScanImplementation returns to the planner prompt:
// the set of impl-tree paths that are Gormes-original (no upstream analog),
// the subset modified within the lookback window, and the candidate list of
// subphases that are entirely Gormes-original (suggested for promotion to
// DriftState.Status="owned").
type ImplInventory struct {
	GormesOriginalPaths []string `json:"gormes_original_paths,omitempty"`
	RecentlyChanged     []string `json:"recently_changed,omitempty"`
	OwnedSubphases      []string `json:"owned_subphases,omitempty"`
}

// DefaultGormesOriginalPaths is the seed deny-list of paths considered
// Gormes-original (no upstream Hermes/GBrain/Honcho analog). Tunable via
// Config.GormesOriginalPaths and the PLANNER_GORMES_ORIGINAL_PATHS env var.
var DefaultGormesOriginalPaths = []string{
	"cmd/builder-loop/",
	"cmd/planner-loop/",
	"internal/builderloop/",
	"internal/plannerloop/",
	"internal/plannertriggers/",
	"internal/progress/health.go",
	"www.gormes.ai/internal/site/installers/",
}

// ScanImplementation walks repoRoot/cmd and repoRoot/internal, identifies
// paths matching any of gormesOriginalPaths (prefix match), and reports the
// subset modified within [now-lookback, now]. Used by L3 prompt to give the
// LLM a concrete "what's here that you don't need to research upstream for"
// inventory.
//
// Missing repoRoot returns empty inventory, no error (fresh checkout case).
func ScanImplementation(repoRoot string, gormesOriginalPaths []string, lookback time.Duration, now time.Time) (ImplInventory, error) {
	if len(gormesOriginalPaths) == 0 {
		gormesOriginalPaths = DefaultGormesOriginalPaths
	}

	var inv ImplInventory
	cutoff := now.Add(-lookback)

	for _, root := range []string{"cmd", "internal"} {
		base := filepath.Join(repoRoot, root)
		if _, err := os.Stat(base); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return inv, err
		}
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip unreadable entries
			}
			if d.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if !matchesAnyPrefix(rel, gormesOriginalPaths) {
				return nil
			}
			inv.GormesOriginalPaths = append(inv.GormesOriginalPaths, rel)

			info, infoErr := d.Info()
			if infoErr != nil {
				return nil
			}
			mt := info.ModTime()
			if !mt.Before(cutoff) && !mt.After(now) {
				inv.RecentlyChanged = append(inv.RecentlyChanged, rel)
			}
			return nil
		})
		if err != nil {
			return inv, err
		}
	}

	sort.Strings(inv.GormesOriginalPaths)
	sort.Strings(inv.RecentlyChanged)
	return inv, nil
}

func matchesAnyPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// computeOwnedSubphases walks the loaded progress doc and returns the set of
// subphase IDs whose every Item.WriteScope entry is entirely under one of the
// supplied Gormes-original prefixes. Subphases with zero items having
// WriteScope are NOT owned (cannot decide). When gormesOriginalPaths is
// empty/nil, the function falls back to DefaultGormesOriginalPaths so the
// caller sees the same "owned" determination as ScanImplementation.
//
// Returned IDs are subphase keys as they appear in progress.Phase.Subphases
// (e.g. "5.O", "5.P"), sorted ascending.
func computeOwnedSubphases(prog *progress.Progress, gormesOriginalPaths []string) []string {
	if prog == nil {
		return nil
	}
	if len(gormesOriginalPaths) == 0 {
		gormesOriginalPaths = DefaultGormesOriginalPaths
	}
	var owned []string
	for _, phase := range prog.Phases {
		for subID, sub := range phase.Subphases {
			anyScope := false
			allUnder := true
			for i := range sub.Items {
				it := &sub.Items[i]
				if len(it.WriteScope) == 0 {
					continue
				}
				anyScope = true
				for _, ws := range it.WriteScope {
					if !matchesAnyPrefix(ws, gormesOriginalPaths) {
						allUnder = false
						break
					}
				}
				if !allUnder {
					break
				}
			}
			if anyScope && allUnder {
				owned = append(owned, subID)
			}
		}
	}
	sort.Strings(owned)
	return owned
}
