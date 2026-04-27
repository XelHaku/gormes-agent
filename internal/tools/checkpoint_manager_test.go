package tools

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCheckpointManagerPrunesOrphanShadowRepos(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 4, 27, 5, 0, 0, 0, time.UTC)

	activeWorkdir := filepath.Join(t.TempDir(), "active-project")
	if err := os.MkdirAll(activeWorkdir, 0o755); err != nil {
		t.Fatalf("mkdir active workdir: %v", err)
	}
	activeShadow := seedCheckpointShadowRepo(t, root, "active-shadow", activeWorkdir, now)
	orphanShadow := seedCheckpointShadowRepo(t, root, "orphan-shadow", filepath.Join(t.TempDir(), "deleted-project"), now)

	mgr, err := NewCheckpointManager(CheckpointManagerOptions{
		Root: root,
		Now:  func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewCheckpointManager: %v", err)
	}

	if _, err := os.Stat(activeShadow); err != nil {
		t.Fatalf("active shadow was pruned: %v", err)
	}
	if _, err := os.Stat(orphanShadow); !os.IsNotExist(err) {
		t.Fatalf("orphan shadow still exists; stat err = %v", err)
	}

	evidence := findCheckpointEvidence(t, mgr.Status(), "orphan_shadow_repo")
	if evidence.Count != 1 {
		t.Fatalf("orphan evidence count = %d, want 1", evidence.Count)
	}
	if !slices.Contains(evidence.Paths, "orphan-shadow") {
		t.Fatalf("orphan evidence paths = %v, want orphan-shadow", evidence.Paths)
	}
	for _, p := range evidence.Paths {
		if filepath.IsAbs(p) || filepath.Clean(p) != p {
			t.Fatalf("evidence path %q is not clean relative path", p)
		}
	}
}

func TestCheckpointManagerPrunesStaleShadowRepos(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 4, 27, 5, 0, 0, 0, time.UTC)

	freshWorkdir := filepath.Join(t.TempDir(), "fresh-project")
	staleWorkdir := filepath.Join(t.TempDir(), "stale-project")
	for _, dir := range []string{freshWorkdir, staleWorkdir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir workdir %q: %v", dir, err)
		}
	}

	freshShadow := seedCheckpointShadowRepo(t, root, "fresh-shadow", freshWorkdir, now.Add(-1*time.Hour))
	staleShadow := seedCheckpointShadowRepo(t, root, "stale-shadow", staleWorkdir, now.Add(-72*time.Hour))

	mgr, err := NewCheckpointManager(CheckpointManagerOptions{
		Root:      root,
		Now:       func() time.Time { return now },
		ShadowTTL: 48 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewCheckpointManager: %v", err)
	}

	if _, err := os.Stat(freshShadow); err != nil {
		t.Fatalf("fresh active shadow was pruned: %v", err)
	}
	if _, err := os.Stat(staleShadow); !os.IsNotExist(err) {
		t.Fatalf("stale shadow still exists; stat err = %v", err)
	}

	evidence := findCheckpointEvidence(t, mgr.Status(), "stale_shadow_repo")
	if evidence.Count != 1 {
		t.Fatalf("stale evidence count = %d, want 1", evidence.Count)
	}
	if !slices.Contains(evidence.Paths, "stale-shadow") {
		t.Fatalf("stale evidence paths = %v, want stale-shadow", evidence.Paths)
	}
}

func TestCheckpointManagerDryRunReportsCandidates(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 4, 27, 5, 0, 0, 0, time.UTC)

	staleWorkdir := filepath.Join(t.TempDir(), "stale-project")
	if err := os.MkdirAll(staleWorkdir, 0o755); err != nil {
		t.Fatalf("mkdir stale workdir: %v", err)
	}
	orphanShadow := seedCheckpointShadowRepo(t, root, "orphan-shadow", filepath.Join(t.TempDir(), "deleted-project"), now)
	staleShadow := seedCheckpointShadowRepo(t, root, "stale-shadow", staleWorkdir, now.Add(-72*time.Hour))

	mgr, err := NewCheckpointManager(CheckpointManagerOptions{
		Root:      root,
		Now:       func() time.Time { return now },
		ShadowTTL: 48 * time.Hour,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("NewCheckpointManager: %v", err)
	}

	for _, path := range []string{orphanShadow, staleShadow} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("dry-run removed %q: %v", path, err)
		}
	}

	orphanEvidence := findCheckpointEvidence(t, mgr.Status(), "orphan_shadow_repo")
	staleEvidence := findCheckpointEvidence(t, mgr.Status(), "stale_shadow_repo")
	if orphanEvidence.Count != 1 || !slices.Contains(orphanEvidence.Paths, "orphan-shadow") {
		t.Fatalf("orphan evidence = %+v, want one orphan-shadow", orphanEvidence)
	}
	if staleEvidence.Count != 1 || !slices.Contains(staleEvidence.Paths, "stale-shadow") {
		t.Fatalf("stale evidence = %+v, want one stale-shadow", staleEvidence)
	}
}

func TestCheckpointManagerStatusEvidenceRedactsCheckpointRoot(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "data", "gormes", "checkpoints")
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		t.Fatalf("mkdir checkpoint parent: %v", err)
	}
	if err := os.WriteFile(root, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write checkpoint root file: %v", err)
	}

	mgr, err := NewCheckpointManager(CheckpointManagerOptions{Root: root})
	if err != nil {
		t.Fatalf("NewCheckpointManager: %v", err)
	}

	evidence := findCheckpointEvidence(t, mgr.Status(), "shadow_gc_unavailable")
	if evidence.Count != 1 {
		t.Fatalf("unavailable evidence count = %d, want 1", evidence.Count)
	}
	if len(evidence.Paths) != 0 {
		t.Fatalf("unavailable evidence paths = %v, want none", evidence.Paths)
	}
	if evidence.Error == "" {
		t.Fatal("unavailable evidence error is empty")
	}
	if strings.Contains(evidence.Error, home) || strings.Contains(evidence.Error, root) {
		t.Fatalf("unavailable evidence leaks absolute checkpoint root: %q", evidence.Error)
	}
}

func seedCheckpointShadowRepo(t *testing.T, root, name, workdir string, mtime time.Time) string {
	t.Helper()

	shadow := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(shadow, "objects"), 0o755); err != nil {
		t.Fatalf("mkdir shadow repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shadow, "GORMES_WORKDIR"), []byte(workdir+"\n"), 0o644); err != nil {
		t.Fatalf("write GORMES_WORKDIR: %v", err)
	}
	if !mtime.IsZero() {
		if err := filepath.WalkDir(shadow, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			return os.Chtimes(path, mtime, mtime)
		}); err != nil {
			t.Fatalf("chtimes shadow repo: %v", err)
		}
	}
	return shadow
}

func findCheckpointEvidence(t *testing.T, status CheckpointStatus, kind string) CheckpointEvidence {
	t.Helper()
	for _, evidence := range status.Evidence {
		if evidence.Kind == kind {
			return evidence
		}
	}
	t.Fatalf("status evidence missing %q: %+v", kind, status.Evidence)
	return CheckpointEvidence{}
}
