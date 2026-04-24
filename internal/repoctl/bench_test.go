package repoctl

import (
	"encoding/json"
	"os"
	"path/filepath"
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
