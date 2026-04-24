package repoctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type BenchmarkOptions struct {
	Root      string
	Binary    string
	Now       func() time.Time
	GitCommit func(string) (string, error)
}

type benchmarkFile struct {
	Binary  benchmarkEntry   `json:"binary"`
	History []benchmarkEntry `json:"history"`
}

type benchmarkEntry struct {
	SizeBytes int64  `json:"size_bytes"`
	SizeMB    string `json:"size_mb"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
}

func RecordBenchmark(opts BenchmarkOptions) error {
	if opts.Root == "" {
		opts.Root = "."
	}
	if opts.Binary == "" {
		opts.Binary = filepath.Join(opts.Root, "bin", "gormes")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.GitCommit == nil {
		opts.GitCommit = gitCommit
	}

	info, err := os.Stat(opts.Binary)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}

	benchPath := filepath.Join(opts.Root, "benchmarks.json")
	var bench benchmarkFile
	if raw, err := os.ReadFile(benchPath); err == nil {
		if err := json.Unmarshal(raw, &bench); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	commit, err := opts.GitCommit(opts.Root)
	if err != nil {
		return err
	}

	entry := benchmarkEntry{
		SizeBytes: info.Size(),
		SizeMB:    fmt.Sprintf("%.1f", float64(info.Size())/1048576),
		Commit:    commit,
		Date:      opts.Now().UTC().Format("2006-01-02"),
	}
	bench.Binary = entry
	bench.History = append(bench.History, entry)

	raw, err := json.MarshalIndent(bench, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(benchPath, raw, 0o644)
}

func gitCommit(root string) (string, error) {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
