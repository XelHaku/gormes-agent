package repoctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncProgressUpdatesDocsDataAndSiteMirror(t *testing.T) {
	root := t.TempDir()
	docsData := filepath.Join(root, "docs", "data")
	archDir := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan")
	siteData := filepath.Join(root, "www.gormes.ai", "internal", "site", "data")
	for _, dir := range []string{docsData, archDir, siteData} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	progress := `{"meta":{"last_updated":"old"},"phases":{}}` + "\n"
	if err := os.WriteFile(filepath.Join(docsData, "progress.json"), []byte(progress), 0o644); err != nil {
		t.Fatal(err)
	}
	archProgress := `{"meta":{"last_updated":"arch"},"phases":{"1":{}}}` + "\n"
	if err := os.WriteFile(filepath.Join(archDir, "progress.json"), []byte(archProgress), 0o644); err != nil {
		t.Fatal(err)
	}

	err := SyncProgress(ProgressOptions{
		Root: root,
		Now:  func() time.Time { return time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("SyncProgress: %v", err)
	}

	var docs struct {
		Meta map[string]string `json:"meta"`
	}
	raw, err := os.ReadFile(filepath.Join(docsData, "progress.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &docs); err != nil {
		t.Fatal(err)
	}
	if docs.Meta["last_updated"] != "2026-04-24" {
		t.Fatalf("last_updated = %q", docs.Meta["last_updated"])
	}
	mirror, err := os.ReadFile(filepath.Join(siteData, "progress.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mirror) != archProgress {
		t.Fatalf("site mirror = %s", mirror)
	}
}

func TestSyncProgressSkipsMissingDocsProgress(t *testing.T) {
	root := t.TempDir()
	siteProgress := filepath.Join(root, "www.gormes.ai", "internal", "site", "data", "progress.json")
	if err := SyncProgress(ProgressOptions{Root: root}); err != nil {
		t.Fatalf("SyncProgress: %v", err)
	}
	if _, err := os.Stat(siteProgress); !os.IsNotExist(err) {
		t.Fatalf("site mirror was created: %v", err)
	}
}
