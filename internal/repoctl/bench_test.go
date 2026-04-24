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
			SizeBytes    int64  `json:"size_bytes"`
			SizeMB       string `json:"size_mb"`
			Commit       string `json:"commit"`
			LastMeasured string `json:"last_measured"`
		} `json:"binary"`
		History []struct {
			SizeBytes int64   `json:"size_bytes"`
			SizeMB    float64 `json:"size_mb"`
			Commit    string  `json:"commit"`
			Date      string  `json:"date"`
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
	if got.Binary.Commit != "abc123" || got.Binary.LastMeasured != "2026-04-24" {
		t.Fatalf("binary metadata = %+v", got.Binary)
	}
	if len(got.History) != 1 || got.History[0].SizeMB != 2.0 {
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

func TestRecordBenchmarkCreatesLegacySkeletonWhenBenchmarksMissing(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, make([]byte, 4*1024*1024), 0o755); err != nil {
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
			Name         string `json:"name"`
			Path         string `json:"path"`
			SizeBytes    int64  `json:"size_bytes"`
			SizeMB       string `json:"size_mb"`
			BuildFlags   string `json:"build_flags"`
			Linker       string `json:"linker"`
			Stripped     bool   `json:"stripped"`
			GoVersion    string `json:"go_version"`
			LastMeasured string `json:"last_measured"`
			Commit       string `json:"commit"`
		} `json:"binary"`
		Properties struct {
			CGO          bool     `json:"cgo"`
			Dependencies string   `json:"dependencies"`
			Platforms    []string `json:"platforms"`
		} `json:"properties"`
		History []struct {
			Date      string  `json:"date"`
			SizeBytes int64   `json:"size_bytes"`
			SizeMB    float64 `json:"size_mb"`
			Commit    string  `json:"commit"`
			Phase     string  `json:"phase"`
		} `json:"history"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "benchmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("benchmarks.json is invalid JSON: %v\n%s", err, raw)
	}

	if got.Binary.Name != "gormes" || got.Binary.Path != "bin/gormes" {
		t.Fatalf("binary identity = %+v", got.Binary)
	}
	if got.Binary.BuildFlags != `CGO_ENABLED=0 -trimpath -ldflags="-s -w"` || got.Binary.Linker != "static" || !got.Binary.Stripped || got.Binary.GoVersion != "1.25+" {
		t.Fatalf("binary build metadata = %+v", got.Binary)
	}
	if got.Binary.SizeBytes != 4*1024*1024 || got.Binary.SizeMB != "4.0" || got.Binary.LastMeasured != "2026-04-24" || got.Binary.Commit != "abc123" {
		t.Fatalf("binary measured metadata = %+v", got.Binary)
	}
	if got.Properties.CGO || got.Properties.Dependencies != "zero (no dynamic library deps)" {
		t.Fatalf("properties = %+v", got.Properties)
	}
	wantPlatforms := []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"}
	if strings.Join(got.Properties.Platforms, ",") != strings.Join(wantPlatforms, ",") {
		t.Fatalf("platforms = %v", got.Properties.Platforms)
	}
	if len(got.History) != 1 || got.History[0].Date != "2026-04-24" || got.History[0].SizeBytes != 4*1024*1024 || got.History[0].SizeMB != 4.0 || got.History[0].Commit != "abc123" || got.History[0].Phase != "unknown" {
		t.Fatalf("history = %+v", got.History)
	}

	docsRaw, err := os.ReadFile(filepath.Join(root, "docs", "data", "benchmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(docsRaw) != string(raw) {
		t.Fatalf("docs/data/benchmarks.json did not match root benchmarks.json")
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
	if first["size_bytes"] != float64(3*1024*1024) || first["size_mb"] != 3.0 || first["commit"] != "def456" || first["date"] != "2026-04-24" || first["phase"] != "Phase 2 - The Gateway" {
		t.Fatalf("new history entry = %+v", first)
	}
	second := history[1].(map[string]any)
	if second["size_mb"] != 16.2 || second["phase"] != "Phase 2 - The Gateway" {
		t.Fatalf("existing history entry was not preserved after new entry: %+v", second)
	}
}

func TestRecordBenchmarkCopiesBenchmarksToDocsData(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
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

	rootBench, err := os.ReadFile(filepath.Join(root, "benchmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	docsBench, err := os.ReadFile(filepath.Join(root, "docs", "data", "benchmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(docsBench) != string(rootBench) {
		t.Fatalf("docs/data/benchmarks.json did not match root benchmarks.json")
	}
}

func TestRecordBenchmarkAvoidsDuplicateSameDayHistory(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	benchPath := filepath.Join(root, "benchmarks.json")
	if err := os.WriteFile(benchPath, []byte(`{"binary":{},"history":[{"date":"2026-04-24","size_mb":1.0,"phase":"Phase 1"}]}`+"\n"), 0o644); err != nil {
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
		History []map[string]any `json:"history"`
	}
	raw, err := os.ReadFile(benchPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.History) != 1 {
		t.Fatalf("history length = %d, want 1: %+v", len(got.History), got.History)
	}
}

func TestRecordBenchmarkUsesLocalDateFromNow(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	location := time.FixedZone("late-west", -8*60*60)
	now := time.Date(2026, 4, 23, 23, 30, 0, 0, location)

	err := RecordBenchmark(BenchmarkOptions{
		Root:      root,
		Binary:    bin,
		Now:       func() time.Time { return now },
		GitCommit: func(string) (string, error) { return "abc123", nil },
	})
	if err != nil {
		t.Fatalf("RecordBenchmark: %v", err)
	}

	var got struct {
		Binary struct {
			LastMeasured string `json:"last_measured"`
		} `json:"binary"`
		History []struct {
			Date string `json:"date"`
		} `json:"history"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "benchmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Binary.LastMeasured != "2026-04-23" {
		t.Fatalf("last_measured = %q, want local date 2026-04-23", got.Binary.LastMeasured)
	}
	if len(got.History) == 0 || got.History[0].Date != "2026-04-23" {
		t.Fatalf("history = %+v, want local date 2026-04-23", got.History)
	}
}

func TestRecordBenchmarkInfersPhaseFromProgressJSON(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "benchmarks.json"), []byte(`{"binary":{},"history":[]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "docs", "ARCH_PLAN.md"), "# Stub\n\nNo real phase marker.\n")
	writeTestFile(t, filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json"), `{
  "phases": {
    "1": {
      "name": "Phase 1 - Complete",
      "subphases": {
        "1.A": {"items": [{"status": "complete"}]}
      }
    },
    "2": {
      "name": "Phase 2 - In Progress",
      "subphases": {
        "2.A": {"items": [{"status": "complete"}, {"status": "planned"}]}
      }
    }
  }
}
`)

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
		History []struct {
			Phase string `json:"phase"`
		} `json:"history"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "benchmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.History) == 0 || got.History[0].Phase != "Phase 2 - In Progress" {
		t.Fatalf("history = %+v", got.History)
	}
}

func TestRecordBenchmarkUsesLastProgressPhaseWhenAllComplete(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json"), `{
  "phases": {
    "1": {
      "name": "Phase 1 - Complete",
      "subphases": {
        "1.A": {"items": [{"status": "complete"}]}
      }
    },
    "2": {
      "name": "Phase 2 - Also Complete",
      "subphases": {
        "2.A": {"items": [{"status": "complete"}]}
      }
    }
  }
}
`)

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
		History []struct {
			Phase string `json:"phase"`
		} `json:"history"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "benchmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.History) == 0 || got.History[0].Phase != "Phase 2 - Also Complete" {
		t.Fatalf("history = %+v", got.History)
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

func writeTestFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
