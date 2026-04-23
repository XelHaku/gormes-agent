package skills

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestHubSyncLocalCatalogsBuildsLockAndMirror(t *testing.T) {
	root := t.TempDir()
	writeSkillDoc(t, filepath.Join(root, ".hub", "catalogs", "official", "research", "arxiv", "SKILL.md"), "arxiv", "Search and retrieve arXiv papers", "Use arXiv APIs and summarize relevant papers.")
	writeSkillDoc(t, filepath.Join(root, ".hub", "catalogs", "clawhub", "devops", "docker-management", "SKILL.md"), "docker-management", "Manage Docker from the CLI", "Inspect images, containers, and logs.")

	hub := NewHub(root, 8*1024)
	lock, err := hub.SyncLocalCatalogs(context.Background(), time.Date(2026, 4, 23, 18, 2, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("SyncLocalCatalogs() error = %v", err)
	}

	if lock.Version != 1 {
		t.Fatalf("lock.Version = %d, want 1", lock.Version)
	}

	gotRefs := make([]string, 0, len(lock.Skills))
	for _, skill := range lock.Skills {
		gotRefs = append(gotRefs, skill.Ref)
		if _, err := os.Stat(skill.Path); err != nil {
			t.Fatalf("mirrored skill path %q: %v", skill.Path, err)
		}
	}
	wantRefs := []string{
		"clawhub/devops/docker-management",
		"official/research/arxiv",
	}
	if !reflect.DeepEqual(gotRefs, wantRefs) {
		t.Fatalf("lock refs = %#v, want %#v", gotRefs, wantRefs)
	}

	if _, err := os.Stat(filepath.Join(root, ".hub", "lock.json")); err != nil {
		t.Fatalf("lock.json missing: %v", err)
	}
}

func TestHubInstallCopiesMirroredSkillIntoActiveStore(t *testing.T) {
	root := t.TempDir()
	writeSkillDoc(t, filepath.Join(root, ".hub", "catalogs", "official", "research", "arxiv", "SKILL.md"), "arxiv", "Search and retrieve arXiv papers", "Use arXiv APIs and summarize relevant papers.")

	hub := NewHub(root, 8*1024)
	if _, err := hub.SyncLocalCatalogs(context.Background(), time.Date(2026, 4, 23, 18, 2, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SyncLocalCatalogs() error = %v", err)
	}

	meta, err := hub.Install("official/research/arxiv")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if meta.Ref != "official/research/arxiv" {
		t.Fatalf("meta.Ref = %q, want %q", meta.Ref, "official/research/arxiv")
	}

	activePath := filepath.Join(root, "active", "research", "arxiv", "SKILL.md")
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("installed skill missing from active store: %v", err)
	}

	snap, err := NewStore(root, 8*1024).SnapshotActive()
	if err != nil {
		t.Fatalf("SnapshotActive() error = %v", err)
	}
	if len(snap.Skills) != 1 || snap.Skills[0].Name != "arxiv" {
		t.Fatalf("active snapshot = %#v, want arxiv", snap.Skills)
	}
}
