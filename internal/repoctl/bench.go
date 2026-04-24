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
	bench := map[string]any{}
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

	date := opts.Now().UTC().Format("2006-01-02")
	sizeMB := fmt.Sprintf("%.1f", float64(info.Size())/1048576)

	binary, _ := bench["binary"].(map[string]any)
	if binary == nil {
		binary = map[string]any{}
	}
	binary["size_bytes"] = info.Size()
	binary["size_mb"] = sizeMB
	binary["commit"] = commit
	if _, ok := binary["last_measured"]; ok {
		binary["last_measured"] = date
	}
	if _, ok := binary["date"]; ok {
		binary["date"] = date
	}
	if _, hasLastMeasured := binary["last_measured"]; !hasLastMeasured {
		if _, hasDate := binary["date"]; !hasDate {
			binary["date"] = date
		}
	}
	bench["binary"] = binary

	entry := map[string]any{
		"size_bytes": info.Size(),
		"size_mb":    sizeMB,
		"commit":     commit,
		"date":       date,
	}
	history, _ := bench["history"].([]any)
	bench["history"] = append(history, entry)

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
