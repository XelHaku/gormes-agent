package tools

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const checkpointWorkdirMarker = "GORMES_WORKDIR"

// CheckpointManagerOptions configures the startup shadow-repo GC contract.
type CheckpointManagerOptions struct {
	Root      string
	Now       func() time.Time
	ShadowTTL time.Duration
	DryRun    bool
}

// CheckpointManager owns the read model for Gormes checkpoint rollback state.
type CheckpointManager struct {
	root   string
	now    func() time.Time
	ttl    time.Duration
	dryRun bool
	status CheckpointStatus
}

// CheckpointStatus reports degraded-mode evidence from checkpoint startup GC.
type CheckpointStatus struct {
	Evidence []CheckpointEvidence
}

// CheckpointEvidence names a cleanup condition and the affected shadow repos.
type CheckpointEvidence struct {
	Kind  string
	Count int
	Paths []string
	Error string
}

// NewCheckpointManager performs deterministic startup cleanup before callers
// can depend on rollback state.
func NewCheckpointManager(opts CheckpointManagerOptions) (*CheckpointManager, error) {
	if opts.Root == "" {
		opts.Root = DefaultCheckpointRoot()
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	mgr := &CheckpointManager{
		root:   opts.Root,
		now:    opts.Now,
		ttl:    opts.ShadowTTL,
		dryRun: opts.DryRun,
	}
	mgr.runStartupGC()
	return mgr, nil
}

// DefaultCheckpointRoot returns Gormes' XDG-owned rollback directory.
func DefaultCheckpointRoot() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "gormes", "checkpoints")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "gormes", "checkpoints")
}

// Status returns a copy of the checkpoint read model.
func (m *CheckpointManager) Status() CheckpointStatus {
	out := m.status
	out.Evidence = append([]CheckpointEvidence(nil), m.status.Evidence...)
	for i := range out.Evidence {
		out.Evidence[i].Paths = append([]string(nil), out.Evidence[i].Paths...)
	}
	return out
}

func (m *CheckpointManager) runStartupGC() {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		m.addEvidence("shadow_gc_unavailable", nil, err)
		return
	}

	var orphanPaths []string
	var stalePaths []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		shadow := filepath.Join(m.root, entry.Name())
		if _, err := os.Stat(filepath.Join(shadow, "HEAD")); err != nil {
			continue
		}
		workdir, ok := readCheckpointWorkdir(shadow)
		if !ok || !pathExists(workdir) {
			orphanPaths = append(orphanPaths, entry.Name())
			if !m.dryRun {
				_ = os.RemoveAll(shadow)
			}
			continue
		}
		if m.ttl > 0 {
			newest, ok := newestCheckpointMTime(shadow)
			if ok && m.now().Sub(newest) > m.ttl {
				stalePaths = append(stalePaths, entry.Name())
				if !m.dryRun {
					_ = os.RemoveAll(shadow)
				}
			}
		}
	}
	m.addEvidence("orphan_shadow_repo", orphanPaths, nil)
	m.addEvidence("stale_shadow_repo", stalePaths, nil)
}

func readCheckpointWorkdir(shadow string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(shadow, checkpointWorkdirMarker))
	if err != nil {
		return "", false
	}
	workdir := strings.TrimSpace(string(raw))
	return workdir, workdir != ""
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func newestCheckpointMTime(shadow string) (time.Time, bool) {
	var newest time.Time
	err := filepath.WalkDir(shadow, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest, err == nil && !newest.IsZero()
}

func (m *CheckpointManager) addEvidence(kind string, paths []string, err error) {
	if len(paths) == 0 && err == nil {
		return
	}
	sort.Strings(paths)
	evidence := CheckpointEvidence{
		Kind:  kind,
		Count: len(paths),
		Paths: append([]string(nil), paths...),
	}
	if err != nil {
		evidence.Count = 1
		evidence.Error = "checkpoint root unavailable"
	}
	m.status.Evidence = append(m.status.Evidence, evidence)
}
