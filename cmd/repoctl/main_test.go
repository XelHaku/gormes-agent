package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := run([]string{"nope"}); err == nil {
		t.Fatal("run returned nil for unknown command")
	}
}

func TestRunBenchmarkRecordUpdatesBenchmarks(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "-c", "user.email=test@example.com", "-c", "user.name=Test User", "commit", "-m", "initial")

	bin := filepath.Join(root, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, make([]byte, 1024*1024), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "benchmarks.json"), []byte(`{"binary":{"name":"gormes"},"history":[]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"benchmark", "record"}); err != nil {
		t.Fatalf("run: %v", err)
	}

	var got struct {
		Binary struct {
			Name      string `json:"name"`
			SizeMB    string `json:"size_mb"`
			SizeBytes int64  `json:"size_bytes"`
			Commit    string `json:"commit"`
		} `json:"binary"`
		History []map[string]any `json:"history"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "benchmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Binary.Name != "gormes" || got.Binary.SizeMB != "1.0" || got.Binary.SizeBytes != 1024*1024 || got.Binary.Commit == "" {
		t.Fatalf("binary = %+v", got.Binary)
	}
	if len(got.History) != 1 {
		t.Fatalf("history = %+v", got.History)
	}
}

func TestRunProgressSyncUpdatesMirror(t *testing.T) {
	root := t.TempDir()
	docsData := filepath.Join(root, "docs", "data")
	archDir := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan")
	siteProgress := filepath.Join(root, "www.gormes.ai", "internal", "site", "data", "progress.json")
	for _, dir := range []string{docsData, archDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(docsData, "progress.json"), []byte(`{"meta":{},"phases":{}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	archProgress := `{"meta":{"last_updated":"arch"},"phases":{"1":{}}}` + "\n"
	if err := os.WriteFile(filepath.Join(archDir, "progress.json"), []byte(archProgress), 0o644); err != nil {
		t.Fatal(err)
	}

	withTempCwd(t, root)
	if err := run([]string{"progress", "sync"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	raw, err := os.ReadFile(siteProgress)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != archProgress {
		t.Fatalf("site mirror = %s", raw)
	}
}

func TestRunReadmeUpdateUpdatesReadme(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "benchmarks.json"), []byte(`{"binary":{"size_mb":"16.2"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(root, "README.md")
	if err := os.WriteFile(readme, []byte("Binary size: ~99.9 MB\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	withTempCwd(t, root)
	if err := run([]string{"readme", "update"}); err != nil {
		t.Fatalf("run: %v", err)
	}
	raw, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "~16.2 MB") {
		t.Fatalf("README not updated:\n%s", raw)
	}
}

func runGit(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

func withTempCwd(t *testing.T, dir string) {
	t.Helper()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
}
