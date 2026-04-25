package architectureplanner

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/plannertriggers"
	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

type SourceRoot struct {
	Name      string   `json:"name"`
	Path      string   `json:"path"`
	Exists    bool     `json:"exists"`
	FileCount int      `json:"file_count"`
	Samples   []string `json:"samples,omitempty"`
}

type ContextBundle struct {
	GeneratedUTC            string                  `json:"generated_utc"`
	RepoRoot                string                  `json:"repo_root"`
	ProgressJSON            string                  `json:"progress_json"`
	ProgressStats           ProgressInfo            `json:"progress_stats"`
	SourceRoots             []SourceRoot            `json:"source_roots"`
	SyncResults             []RepoSyncResult        `json:"sync_results,omitempty"`
	ImplementationInventory ImplementationInventory `json:"implementation_inventory"`
	AutoloopAudit           AutoloopAudit           `json:"autoloop_audit"`
	// QuarantinedRows surfaces autoloop-quarantined progress.json items as a
	// call-to-action list for the planner (sorted most-attempted-then-oldest).
	// Capped by Config.PlannerQuarantineLimit. Empty when no rows are
	// quarantined or when progress.json could not be loaded.
	QuarantinedRows []QuarantinedRowContext `json:"quarantined_rows,omitempty"`
	// TriggerEvents lists the autoloop trigger events consumed by this
	// planner run. Populated from plannertriggers.ReadTriggersSinceCursor;
	// rendered into the prompt's "Recent Autoloop Signals" section so the
	// LLM sees which row state changes prompted this run. Empty when the
	// run was scheduled (no new events since the last cursor advance).
	TriggerEvents []plannertriggers.TriggerEvent `json:"trigger_events,omitempty"`
	// PreviousReshapes correlates rows the planner reshaped within the last
	// EvaluationWindow with what autoloop did to those rows afterwards. The
	// L4 self-evaluation surface lets the planner see whether its previous
	// reshape attempts unstuck the row, are still failing, or haven't been
	// retried yet. Sourced from Evaluate (evaluation.go); rendered into the
	// prompt's "Previous Reshape Outcomes" section. Empty when no rows
	// matching that window were reshaped or when both ledgers are empty.
	PreviousReshapes []ReshapeOutcome `json:"previous_reshapes,omitempty"`
}

// QuarantinedRowContext is the planner-side view of one autoloop-quarantined
// row. Sorted by (AttemptCount desc, QuarantinedSince asc) so the planner
// sees the most-attempted-then-oldest rows first. AuditCorroboration is
// reserved for cross-referencing AutoloopAudit but is currently always
// empty (the audit surface is subphase-level, not row-level).
type QuarantinedRowContext struct {
	PhaseID            string                   `json:"phase_id"`
	SubphaseID         string                   `json:"subphase_id"`
	ItemName           string                   `json:"item_name"`
	Contract           string                   `json:"contract,omitempty"`
	LastCategory       progress.FailureCategory `json:"last_category,omitempty"`
	AttemptCount       int                      `json:"attempt_count,omitempty"`
	BackendsTried      []string                 `json:"backends_tried,omitempty"`
	QuarantinedSince   string                   `json:"quarantined_since,omitempty"`
	SpecHash           string                   `json:"spec_hash,omitempty"`
	LastFailureExcerpt string                   `json:"last_failure_excerpt,omitempty"`
	AuditCorroboration string                   `json:"audit_corroboration,omitempty"`
}

type ProgressInfo struct {
	Items      int `json:"items"`
	Planned    int `json:"planned"`
	InProgress int `json:"in_progress"`
	Complete   int `json:"complete"`
}

type ImplementationInventory struct {
	Commands         []string   `json:"commands"`
	InternalPackages []string   `json:"internal_packages"`
	BuildingDocs     []string   `json:"building_docs"`
	LandingSite      SourceRoot `json:"landing_site"`
	HugoDocs         SourceRoot `json:"hugo_docs"`
}

func CollectContext(cfg Config, now time.Time) (ContextBundle, error) {
	prog, err := progress.Load(cfg.ProgressJSON)
	if err != nil {
		return ContextBundle{}, err
	}
	stats := prog.Stats()
	progressInfo := ProgressInfo{
		Items:      stats.Items.Total,
		Planned:    stats.Items.Planned,
		InProgress: stats.Items.InProgress,
		Complete:   stats.Items.Complete,
	}

	roots := cfg.SourceRoots()
	for i := range roots {
		if err := enrichSourceRoot(&roots[i]); err != nil {
			return ContextBundle{}, err
		}
	}

	inventory, err := collectImplementationInventory(cfg)
	if err != nil {
		return ContextBundle{}, err
	}

	audit, err := SummarizeAutoloopAudit(autoloopLedgerPath(cfg), 7*24*time.Hour, now)
	if err != nil {
		return ContextBundle{}, err
	}

	return ContextBundle{
		GeneratedUTC:            now.UTC().Format(time.RFC3339),
		RepoRoot:                cfg.RepoRoot,
		ProgressJSON:            cfg.ProgressJSON,
		ProgressStats:           progressInfo,
		SourceRoots:             roots,
		ImplementationInventory: inventory,
		AutoloopAudit:           audit,
		QuarantinedRows:         collectQuarantinedRows(prog, audit, cfg.PlannerQuarantineLimit),
	}, nil
}

// collectQuarantinedRows returns the quarantined items in prog sorted by
// (AttemptCount desc, QuarantinedSince asc), capped at limit. limit=0
// means unlimited. Items without a Health.Quarantine block are skipped.
// Stderr tails are capped at 1 KiB so the planner prompt stays bounded.
func collectQuarantinedRows(prog *progress.Progress, audit AutoloopAudit, limit int) []QuarantinedRowContext {
	out := []QuarantinedRowContext{}
	if prog == nil {
		return out
	}
	for phaseID, phase := range prog.Phases {
		for subID, sub := range phase.Subphases {
			for i := range sub.Items {
				it := &sub.Items[i]
				if it.Health == nil || it.Health.Quarantine == nil {
					continue
				}
				excerpt := ""
				if it.Health.LastFailure != nil {
					excerpt = capExcerpt(it.Health.LastFailure.StderrTail, 1024)
				}
				out = append(out, QuarantinedRowContext{
					PhaseID:            phaseID,
					SubphaseID:         subID,
					ItemName:           it.Name,
					Contract:           it.Contract,
					LastCategory:       it.Health.Quarantine.LastCategory,
					AttemptCount:       it.Health.AttemptCount,
					BackendsTried:      append([]string(nil), it.Health.BackendsTried...),
					QuarantinedSince:   it.Health.Quarantine.Since,
					SpecHash:           it.Health.Quarantine.SpecHash,
					LastFailureExcerpt: excerpt,
					AuditCorroboration: corroborateFromAudit(audit, phaseID, subID, it.Name),
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AttemptCount != out[j].AttemptCount {
			return out[i].AttemptCount > out[j].AttemptCount
		}
		return out[i].QuarantinedSince < out[j].QuarantinedSince
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// capExcerpt returns at most max trailing bytes of s. The tail is preferred
// over the head because failure stack traces are usually most diagnostic at
// the bottom (panic site / final assertion). Returns s unchanged when short.
func capExcerpt(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

// corroborateFromAudit would return a short note when SummarizeAutoloopAudit
// already flagged this row as toxic/hot. AutoloopAudit currently exposes
// subphase-level aggregates, not row-level, so this returns "" today.
// Future work can scan audit.RecentFailedTasks for a matching task key.
func corroborateFromAudit(audit AutoloopAudit, phaseID, subphaseID, itemName string) string {
	_ = audit
	_ = phaseID
	_ = subphaseID
	_ = itemName
	return ""
}

func autoloopLedgerPath(cfg Config) string {
	if cfg.AutoloopRunRoot == "" {
		return ""
	}
	return filepath.Join(cfg.AutoloopRunRoot, "state", "runs.jsonl")
}

func writeContext(path string, bundle ContextBundle) error {
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func enrichSourceRoot(root *SourceRoot) error {
	info, err := os.Stat(root.Path)
	if err != nil {
		if os.IsNotExist(err) {
			root.Exists = false
			return nil
		}
		return err
	}
	if !info.IsDir() {
		root.Exists = false
		return nil
	}

	root.Exists = true
	var samples []string
	err = filepath.WalkDir(root.Path, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".codex", ".worktrees", "node_modules", "dist", "build":
				if path != root.Path {
					return filepath.SkipDir
				}
			}
			return nil
		}
		root.FileCount++
		if len(samples) < 12 && sampleFile(path) {
			rel, err := filepath.Rel(root.Path, path)
			if err != nil {
				return err
			}
			samples = append(samples, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(samples)
	root.Samples = samples
	return nil
}

func sampleFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".css", ".go", ".html", ".js", ".json", ".md", ".py", ".tmpl", ".toml", ".ts", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func collectImplementationInventory(cfg Config) (ImplementationInventory, error) {
	landingSite := SourceRoot{Name: "www.gormes.ai", Path: filepath.Join(cfg.RepoRoot, "www.gormes.ai")}
	if err := enrichSourceRoot(&landingSite); err != nil {
		return ImplementationInventory{}, err
	}

	hugoDocs := SourceRoot{Name: "Hugo docs", Path: filepath.Join(cfg.RepoRoot, "docs")}
	if err := enrichSourceRoot(&hugoDocs); err != nil {
		return ImplementationInventory{}, err
	}

	return ImplementationInventory{
		Commands:         collectImmediateDirs(filepath.Join(cfg.RepoRoot, "cmd")),
		InternalPackages: collectImmediateDirs(filepath.Join(cfg.RepoRoot, "internal")),
		BuildingDocs:     collectImmediateFiles(filepath.Join(cfg.RepoRoot, "docs", "content", "building-gormes"), ".md"),
		LandingSite:      landingSite,
		HugoDocs:         hugoDocs,
	}, nil
}

func collectImmediateDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	sort.Strings(dirs)
	return dirs
}

func collectImmediateFiles(root, ext string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ext {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files
}
