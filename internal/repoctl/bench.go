package repoctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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
	} else if errors.Is(err, os.ErrNotExist) {
		bench = defaultBenchmarkSkeleton()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	commit, err := opts.GitCommit(opts.Root)
	if err != nil {
		return err
	}

	date := opts.Now().Format("2006-01-02")
	sizeMB := fmt.Sprintf("%.1f", float64(info.Size())/1048576)
	historySizeMB, err := strconv.ParseFloat(sizeMB, 64)
	if err != nil {
		return err
	}

	binary, _ := bench["binary"].(map[string]any)
	if binary == nil {
		binary = map[string]any{}
	}
	binary["size_bytes"] = info.Size()
	binary["size_mb"] = sizeMB
	binary["commit"] = commit
	binary["last_measured"] = date
	bench["binary"] = binary

	history, _ := bench["history"].([]any)
	if len(history) == 0 || historyDate(history[0]) != date {
		entry := map[string]any{
			"date":       date,
			"size_bytes": info.Size(),
			"size_mb":    historySizeMB,
			"commit":     commit,
			"phase":      currentPhase(opts.Root, history),
		}
		history = append([]any{entry}, history...)
	}
	bench["history"] = history

	raw, err := json.MarshalIndent(bench, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(benchPath, raw, 0o644); err != nil {
		return err
	}

	docsBenchPath := filepath.Join(opts.Root, "docs", "data", "benchmarks.json")
	if err := os.MkdirAll(filepath.Dir(docsBenchPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(docsBenchPath, raw, 0o644)
}

func gitCommit(root string) (string, error) {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func defaultBenchmarkSkeleton() map[string]any {
	return map[string]any{
		"binary": map[string]any{
			"name":        "gormes",
			"path":        "bin/gormes",
			"build_flags": `CGO_ENABLED=0 -trimpath -ldflags="-s -w"`,
			"linker":      "static",
			"stripped":    true,
			"go_version":  "1.25+",
		},
		"properties": map[string]any{
			"cgo":          false,
			"dependencies": "zero (no dynamic library deps)",
			"platforms": []string{
				"linux/amd64",
				"linux/arm64",
				"darwin/amd64",
				"darwin/arm64",
			},
		},
		"history": []any{},
	}
}

func historyDate(entry any) string {
	fields, ok := entry.(map[string]any)
	if !ok {
		return ""
	}
	date, _ := fields["date"].(string)
	return date
}

func currentPhase(root string, history []any) string {
	if phase := phaseFromArchPlan(filepath.Join(root, "docs", "ARCH_PLAN.md")); phase != "" {
		return phase
	}
	if phase := phaseFromProgress(filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json")); phase != "" {
		return phase
	}
	for _, entry := range history {
		fields, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		phase, _ := fields["phase"].(string)
		if phase != "" {
			return phase
		}
	}
	return "unknown"
}

func phaseFromArchPlan(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "## Phase") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "## Phase"))
		if rest == "" {
			continue
		}
		return "Phase " + rest
	}
	return ""
}

func phaseFromProgress(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var progress struct {
		Phases map[string]struct {
			Name      string                          `json:"name"`
			Status    string                          `json:"status"`
			Subphases map[string]progressSubphaseJSON `json:"subphases"`
		} `json:"phases"`
	}
	if err := json.Unmarshal(raw, &progress); err != nil {
		return ""
	}

	keys := make([]string, 0, len(progress.Phases))
	for key := range progress.Phases {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return phaseKeyLess(keys[i], keys[j])
	})

	for _, key := range keys {
		phase := progress.Phases[key]
		if phase.Name == "" {
			continue
		}
		if phaseIncomplete(phase.Status, phase.Subphases) {
			return phase.Name
		}
	}
	for i := len(keys) - 1; i >= 0; i-- {
		if name := progress.Phases[keys[i]].Name; name != "" {
			return name
		}
	}
	return ""
}

type progressSubphaseJSON struct {
	Status string `json:"status"`
	Items  []struct {
		Status string `json:"status"`
	} `json:"items"`
}

func phaseIncomplete(status string, subphases map[string]progressSubphaseJSON) bool {
	if len(subphases) == 0 {
		return status != "complete"
	}
	for _, subphase := range subphases {
		if len(subphase.Items) == 0 {
			if subphase.Status != "complete" {
				return true
			}
			continue
		}
		for _, item := range subphase.Items {
			if item.Status != "complete" {
				return true
			}
		}
	}
	return false
}

func phaseKeyLess(left, right string) bool {
	leftInt, leftErr := parsePhaseKey(left)
	rightInt, rightErr := parsePhaseKey(right)
	if leftErr == nil && rightErr == nil {
		return leftInt < rightInt
	}
	if leftErr == nil {
		return true
	}
	if rightErr == nil {
		return false
	}
	return left < right
}

func parsePhaseKey(key string) (int, error) {
	var n int
	_, err := fmt.Sscanf(key, "%d", &n)
	return n, err
}
