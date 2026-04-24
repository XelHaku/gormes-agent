package repoctl

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecordBenchmarkUpdatesBinaryMetrics(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, make([]byte, 2*1024*1024), 0o755); err != nil {
		t.Fatal(err)
	}
	benchPath := filepath.Join(root, "benchmarks.json")
	if err := os.WriteFile(benchPath, []byte(`{"binary":{"size_mb":"old"},"history":[]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RecordBenchmark(BenchmarkOptions{
		Root:      root,
		Binary:    bin,
		Now:       func() time.Time { return time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC) },
		GitCommit: func(string) (string, error) { return "abc123", nil },
	})
	if err != nil {
		t.Fatalf("RecordBenchmark: %v", err)
	}

	var got struct {
		Binary struct {
			SizeBytes int64  `json:"size_bytes"`
			SizeMB    string `json:"size_mb"`
			Commit    string `json:"commit"`
			Date      string `json:"date"`
		} `json:"binary"`
		History []struct {
			SizeBytes int64  `json:"size_bytes"`
			SizeMB    string `json:"size_mb"`
			Commit    string `json:"commit"`
			Date      string `json:"date"`
		} `json:"history"`
	}
	raw, err := os.ReadFile(benchPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("benchmarks.json is invalid JSON: %v\n%s", err, raw)
	}
	if got.Binary.SizeBytes != 2*1024*1024 {
		t.Fatalf("size_bytes = %d", got.Binary.SizeBytes)
	}
	if got.Binary.SizeMB != "2.0" {
		t.Fatalf("size_mb = %q", got.Binary.SizeMB)
	}
	if got.Binary.Commit != "abc123" || got.Binary.Date != "2026-04-24" {
		t.Fatalf("binary metadata = %+v", got.Binary)
	}
	if len(got.History) != 1 || got.History[0].SizeMB != "2.0" {
		t.Fatalf("history = %+v", got.History)
	}
}

func TestRecordBenchmarkSkipsMissingBinary(t *testing.T) {
	root := t.TempDir()
	err := RecordBenchmark(BenchmarkOptions{
		Root:      root,
		Binary:    filepath.Join(root, "bin", "gormes"),
		Now:       time.Now,
		GitCommit: func(string) (string, error) { return "unused", nil },
	})
	if err != nil {
		t.Fatalf("RecordBenchmark missing binary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "benchmarks.json")); !os.IsNotExist(err) {
		t.Fatalf("benchmarks.json created for missing binary: %v", err)
	}
}

func TestRecordBenchmarkPreservesRepoStyleMetadata(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, make([]byte, 3*1024*1024), 0o755); err != nil {
		t.Fatal(err)
	}
	benchPath := filepath.Join(root, "benchmarks.json")
	if err := os.WriteFile(benchPath, []byte(`{
  "binary": {
    "name": "gormes",
    "path": "bin/gormes",
    "size_bytes": 17019042,
    "size_mb": "16.2",
    "build_flags": "CGO_ENABLED=0 -trimpath -ldflags=\"-s -w\"",
    "linker": "static",
    "stripped": true,
    "go_version": "1.25+",
    "last_measured": "2026-04-21"
  },
  "properties": {
    "cgo": false,
    "dependencies": "zero (no dynamic library deps)",
    "platforms": [
      "linux/amd64",
      "linux/arm64"
    ]
  },
  "history": [
    {
      "date": "2026-04-21",
      "size_mb": 16.2,
      "commit": "5e36abb0",
      "phase": "Phase 2 - The Gateway"
    }
  ]
}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RecordBenchmark(BenchmarkOptions{
		Root:      root,
		Binary:    bin,
		Now:       func() time.Time { return time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC) },
		GitCommit: func(string) (string, error) { return "def456", nil },
	})
	if err != nil {
		t.Fatalf("RecordBenchmark: %v", err)
	}

	var got map[string]any
	raw, err := os.ReadFile(benchPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("benchmarks.json is invalid JSON: %v\n%s", err, raw)
	}

	binary := got["binary"].(map[string]any)
	if binary["name"] != "gormes" || binary["path"] != "bin/gormes" || binary["build_flags"] == nil {
		t.Fatalf("binary metadata was not preserved: %+v", binary)
	}
	if binary["linker"] != "static" || binary["stripped"] != true || binary["go_version"] != "1.25+" {
		t.Fatalf("binary build metadata was not preserved: %+v", binary)
	}
	if binary["size_bytes"] != float64(3*1024*1024) || binary["size_mb"] != "3.0" {
		t.Fatalf("binary size fields were not updated: %+v", binary)
	}
	if binary["commit"] != "def456" || binary["last_measured"] != "2026-04-24" {
		t.Fatalf("binary measurement metadata was not updated: %+v", binary)
	}

	properties := got["properties"].(map[string]any)
	if properties["dependencies"] != "zero (no dynamic library deps)" {
		t.Fatalf("properties were not preserved: %+v", properties)
	}

	history := got["history"].([]any)
	if len(history) != 2 {
		t.Fatalf("history length = %d", len(history))
	}
	first := history[0].(map[string]any)
	if first["size_mb"] != 16.2 || first["phase"] != "Phase 2 - The Gateway" {
		t.Fatalf("existing history entry was not preserved: %+v", first)
	}
	last := history[1].(map[string]any)
	if last["size_bytes"] != float64(3*1024*1024) || last["size_mb"] != "3.0" || last["commit"] != "def456" || last["date"] != "2026-04-24" {
		t.Fatalf("new history entry = %+v", last)
	}
}

func TestRecordBenchmarkDefaultGitCommit(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "-c", "user.email=test@example.com", "-c", "user.name=Test User", "commit", "-m", "initial")
	expected := string(runGit(t, root, "rev-parse", "--short", "HEAD"))

	bin := filepath.Join(root, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := RecordBenchmark(BenchmarkOptions{
		Root:   root,
		Binary: bin,
		Now:    func() time.Time { return time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("RecordBenchmark: %v", err)
	}

	var got struct {
		Binary struct {
			Commit string `json:"commit"`
		} `json:"binary"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "benchmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Binary.Commit != strings.TrimSpace(expected) {
		t.Fatalf("commit = %q, want %q", got.Binary.Commit, strings.TrimSpace(expected))
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
