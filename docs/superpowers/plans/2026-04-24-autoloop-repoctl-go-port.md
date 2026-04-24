# Autoloop + Repoctl Go Port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace production shell automation with typed Go tools named `repoctl` and `autoloop`, while retaining legacy shell only as temporary parity fixtures or tiny wrappers.

**Status:** `repoctl` is cut over. `cmd/autoloop` now provides Go wrappers, CLI
commands, and typed primitives, but full `autoloop run` runtime parity remains
staged follow-up work. Long-form legacy orchestrator shell is vendored under
`testdata/legacy-shell`; repoctl/orchestrator entrypoints under `scripts/` are
compatibility wrappers while the three companion scripts remain live shell.

**Architecture:** `repoctl` owns deterministic repo maintenance commands and should land first. `autoloop` owns self-development orchestration, preserving the current shell contract through typed config, injectable command execution, fixture parity tests, and staged cutover. Long legacy shell is moved under vendored parity fixtures before production scripts are replaced.

**Tech Stack:** Go 1.25+, standard library, existing `internal/progress`, Cobra only where needed, git/agent/systemd external commands behind injectable runners, Markdown docs tests.

---

## File Structure

Implementation note: the repoctl side has been cut over at this structure.
`cmd/repoctl` and `internal/repoctl` own repo maintenance. `cmd/autoloop` and
`internal/autoloop` own the Go CLI surface plus typed autoloop primitives; full
legacy runtime parity remains staged. `testdata/legacy-shell` retains parity
fixtures. Repoctl/orchestrator shell entrypoints under `scripts/` are wrappers,
but `scripts/gormes-architecture-planner-tasks-manager.sh`,
`scripts/documentation-improver.sh`, and `scripts/landingpage-improver.sh`
remain live shell outside this cutover.

Create these files:

```text
cmd/repoctl/main.go
cmd/repoctl/main_test.go
internal/repoctl/bench.go
internal/repoctl/bench_test.go
internal/repoctl/progress.go
internal/repoctl/progress_test.go
internal/repoctl/readme.go
internal/repoctl/readme_test.go
internal/repoctl/compat.go
internal/repoctl/compat_test.go
internal/autoloop/config.go
internal/autoloop/config_test.go
internal/autoloop/runner.go
internal/autoloop/runner_test.go
internal/autoloop/backend.go
internal/autoloop/backend_test.go
internal/autoloop/candidates.go
internal/autoloop/candidates_test.go
internal/autoloop/claims.go
internal/autoloop/claims_test.go
internal/autoloop/failures.go
internal/autoloop/failures_test.go
internal/autoloop/report.go
internal/autoloop/report_test.go
internal/autoloop/worktree.go
internal/autoloop/worktree_test.go
internal/autoloop/promote.go
internal/autoloop/promote_test.go
internal/autoloop/companions.go
internal/autoloop/companions_test.go
internal/autoloop/ledger.go
internal/autoloop/ledger_test.go
internal/autoloop/audit.go
internal/autoloop/audit_test.go
internal/autoloop/service.go
internal/autoloop/service_test.go
internal/autoloop/run.go
internal/autoloop/run_test.go
cmd/autoloop/main.go
cmd/autoloop/main_test.go
```

Modify these files:

```text
.gitattributes
Makefile
scripts/record-benchmark.sh
scripts/record-progress.sh
scripts/update-readme.sh
scripts/check-go1.22-compat.sh
scripts/gormes-auto-codexu-orchestrator.sh
scripts/orchestrator/audit.sh
scripts/orchestrator/daily-digest.sh
scripts/orchestrator/install-service.sh
scripts/orchestrator/install-audit.sh
scripts/orchestrator/disable-legacy-timers.sh
scripts/orchestrator/systemd/gormes-orchestrator.service.in
scripts/orchestrator/systemd/gormes-orchestrator-audit.service.in
docs/superpowers/specs/2026-04-24-autoloop-repoctl-go-port-design.md
docs/superpowers/plans/2026-04-24-autoloop-repoctl-go-port.md
```

Move these files during the parity-harness task:

```text
scripts/gormes-auto-codexu-orchestrator.sh -> testdata/legacy-shell/scripts/gormes-auto-codexu-orchestrator.sh
scripts/orchestrator/lib/*.sh -> testdata/legacy-shell/scripts/orchestrator/lib/*.sh
scripts/orchestrator/tests/fixtures/** -> testdata/legacy-shell/scripts/orchestrator/tests/fixtures/**
```

Do not edit unrelated dirty files in the working tree. Before each task, run:

```bash
git status -sb --untracked-files=normal
```

If files outside the task are dirty, stage only the paths listed in that task.

---

### Task 1: Add Repoctl Benchmark Command

**Files:**
- Create: `internal/repoctl/bench.go`
- Create: `internal/repoctl/bench_test.go`
- Create: `cmd/repoctl/main.go`
- Create: `cmd/repoctl/main_test.go`

- [ ] **Step 1: Write the failing benchmark update test**

Create `internal/repoctl/bench_test.go`:

```go
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
```

- [ ] **Step 2: Run benchmark test to verify it fails**

Run:

```bash
go test ./internal/repoctl -run TestRecordBenchmark -count=1
```

Expected: fail because `internal/repoctl` and `RecordBenchmark` do not exist.

- [ ] **Step 3: Add benchmark implementation**

Create `internal/repoctl/bench.go`:

```go
package repoctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type BenchmarkOptions struct {
	Root      string
	Binary    string
	Now       func() time.Time
	GitCommit func(root string) (string, error)
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
		return errors.New("repo root is required")
	}
	if opts.Binary == "" {
		opts.Binary = filepath.Join(opts.Root, "bin", "gormes")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.GitCommit == nil {
		opts.GitCommit = currentGitCommit
	}

	info, err := os.Stat(opts.Binary)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "record-benchmark: binary not found at %s; skipping\n", opts.Binary)
			return nil
		}
		return err
	}

	benchPath := filepath.Join(opts.Root, "benchmarks.json")
	var data benchmarkFile
	if raw, err := os.ReadFile(benchPath); err == nil {
		if err := json.Unmarshal(raw, &data); err != nil {
			return fmt.Errorf("parse %s: %w", benchPath, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	commit, err := opts.GitCommit(opts.Root)
	if err != nil {
		return err
	}
	entry := benchmarkEntry{
		SizeBytes: info.Size(),
		SizeMB:    fmt.Sprintf("%.1f", float64(info.Size())/1048576.0),
		Commit:    commit,
		Date:      opts.Now().UTC().Format(time.DateOnly),
	}
	data.Binary = entry
	data.History = append(data.History, entry)

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(benchPath, raw, 0o644)
}

func currentGitCommit(root string) (string, error) {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return string(trimSpaceBytes(out)), nil
}

func trimSpaceBytes(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r' || b[len(b)-1] == '\t' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	for len(b) > 0 && (b[0] == '\n' || b[0] == '\r' || b[0] == '\t' || b[0] == ' ') {
		b = b[1:]
	}
	return b
}
```

- [ ] **Step 4: Add minimal `repoctl` CLI**

Create `cmd/repoctl/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/TrebuchetDynamics/gormes-agent/internal/repoctl"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 2 && args[0] == "benchmark" && args[1] == "record" {
		root, err := os.Getwd()
		if err != nil {
			return err
		}
		return repoctl.RecordBenchmark(repoctl.BenchmarkOptions{Root: root})
	}
	return fmt.Errorf("usage: repoctl benchmark record")
}
```

Create `cmd/repoctl/main_test.go`:

```go
package main

import "testing"

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := run([]string{"nope"}); err == nil {
		t.Fatal("run returned nil for unknown command")
	}
}
```

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./internal/repoctl ./cmd/repoctl -count=1
```

Expected: pass.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/repoctl/bench.go internal/repoctl/bench_test.go cmd/repoctl/main.go cmd/repoctl/main_test.go
git commit -m "feat(repoctl): record binary benchmarks"
```

---

### Task 2: Add Repoctl Progress And README Commands

**Files:**
- Create: `internal/repoctl/progress.go`
- Create: `internal/repoctl/progress_test.go`
- Create: `internal/repoctl/readme.go`
- Create: `internal/repoctl/readme_test.go`
- Modify: `cmd/repoctl/main.go`

- [ ] **Step 1: Write failing progress sync test**

Create `internal/repoctl/progress_test.go`:

```go
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
```

- [ ] **Step 2: Write failing README update test**

Create `internal/repoctl/readme_test.go`:

```go
package repoctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateReadmeSizeFromBenchmark(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "benchmarks.json"), []byte(`{"binary":{"size_mb":"16.2"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(root, "README.md")
	if err := os.WriteFile(readme, []byte("Binary size: ~99.9 MB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateReadme(ReadmeOptions{Root: root}); err != nil {
		t.Fatalf("UpdateReadme: %v", err)
	}
	raw, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "~16.2 MB") {
		t.Fatalf("README not updated:\n%s", raw)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
go test ./internal/repoctl -run 'TestSyncProgress|TestUpdateReadme' -count=1
```

Expected: fail because `SyncProgress`, `ProgressOptions`, `UpdateReadme`, and `ReadmeOptions` do not exist.

- [ ] **Step 4: Add progress implementation**

Create `internal/repoctl/progress.go`:

```go
package repoctl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type ProgressOptions struct {
	Root string
	Now  func() time.Time
}

func SyncProgress(opts ProgressOptions) error {
	if opts.Root == "" {
		return fmt.Errorf("repo root is required")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	docsProgress := filepath.Join(opts.Root, "docs", "data", "progress.json")
	raw, err := os.ReadFile(docsProgress)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stdout, "record-progress: progress.json not found; skipping")
			return nil
		}
		return err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	meta, _ := data["meta"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		data["meta"] = meta
	}
	meta["last_updated"] = opts.Now().UTC().Format(time.DateOnly)
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := os.WriteFile(docsProgress, out, 0o644); err != nil {
		return err
	}
	archProgress := filepath.Join(opts.Root, "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	siteProgress := filepath.Join(opts.Root, "www.gormes.ai", "internal", "site", "data", "progress.json")
	archRaw, err := os.ReadFile(archProgress)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(siteProgress), 0o755); err != nil {
		return err
	}
	return os.WriteFile(siteProgress, archRaw, 0o644)
}
```

- [ ] **Step 5: Add README implementation**

Create `internal/repoctl/readme.go`:

```go
package repoctl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type ReadmeOptions struct {
	Root string
}

func UpdateReadme(opts ReadmeOptions) error {
	if opts.Root == "" {
		return fmt.Errorf("repo root is required")
	}
	benchPath := filepath.Join(opts.Root, "benchmarks.json")
	raw, err := os.ReadFile(benchPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "update-readme: benchmarks.json not found; skipping")
			return nil
		}
		return err
	}
	var data struct {
		Binary struct {
			SizeMB string `json:"size_mb"`
		} `json:"binary"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	if data.Binary.SizeMB == "" {
		return fmt.Errorf("benchmarks.json missing binary.size_mb")
	}
	readmePath := filepath.Join(opts.Root, "README.md")
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`~[0-9.]+ MB`)
	updated := re.ReplaceAllString(string(readme), "~"+data.Binary.SizeMB+" MB")
	return os.WriteFile(readmePath, []byte(updated), 0o644)
}
```

- [ ] **Step 6: Wire repoctl subcommands**

Modify `cmd/repoctl/main.go` so `run` contains:

```go
func run(args []string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if len(args) == 2 && args[0] == "benchmark" && args[1] == "record" {
		return repoctl.RecordBenchmark(repoctl.BenchmarkOptions{Root: root})
	}
	if len(args) == 2 && args[0] == "progress" && args[1] == "sync" {
		return repoctl.SyncProgress(repoctl.ProgressOptions{Root: root})
	}
	if len(args) == 2 && args[0] == "readme" && args[1] == "update" {
		return repoctl.UpdateReadme(repoctl.ReadmeOptions{Root: root})
	}
	return fmt.Errorf("usage: repoctl benchmark record | progress sync | readme update")
}
```

- [ ] **Step 7: Run tests**

Run:

```bash
go test ./internal/repoctl ./cmd/repoctl -count=1
```

Expected: pass.

- [ ] **Step 8: Commit**

Run:

```bash
git add internal/repoctl/progress.go internal/repoctl/progress_test.go internal/repoctl/readme.go internal/repoctl/readme_test.go cmd/repoctl/main.go
git commit -m "feat(repoctl): sync progress and README metadata"
```

---

### Task 3: Add Repoctl Go 1.22 Compatibility Check

**Files:**
- Create: `internal/repoctl/compat.go`
- Create: `internal/repoctl/compat_test.go`
- Modify: `cmd/repoctl/main.go`

- [ ] **Step 1: Write failing compatibility decision tests**

Create `internal/repoctl/compat_test.go`:

```go
package repoctl

import (
	"context"
	"strings"
	"testing"
)

func TestGo122CompatUsesDockerWhenAvailable(t *testing.T) {
	var calls [][]string
	runner := RunnerFunc(func(_ context.Context, name string, args ...string) CommandResult {
		calls = append(calls, append([]string{name}, args...))
		if name == "docker" && len(args) == 1 && args[0] == "--version" {
			return CommandResult{Stdout: "Docker version 1\n"}
		}
		return CommandResult{Stdout: "ok\n"}
	})
	var out strings.Builder
	err := CheckGo122(context.Background(), Go122Options{
		Root:   "/repo",
		Runner: runner,
		Stdout: &out,
	})
	if err != nil {
		t.Fatalf("CheckGo122: %v", err)
	}
	joined := callsToString(calls)
	if !strings.Contains(joined, "docker --version") || !strings.Contains(joined, "docker run --rm") {
		t.Fatalf("calls = %s", joined)
	}
	if !strings.Contains(out.String(), "Go 1.22 builds cleanly") {
		t.Fatalf("stdout = %s", out.String())
	}
}

func TestGo122CompatFallsBackToDownloadedToolchain(t *testing.T) {
	var calls [][]string
	runner := RunnerFunc(func(_ context.Context, name string, args ...string) CommandResult {
		calls = append(calls, append([]string{name}, args...))
		if name == "docker" {
			return CommandResult{Err: errExit(127)}
		}
		return CommandResult{Stdout: "ok\n"}
	})
	var out strings.Builder
	err := CheckGo122(context.Background(), Go122Options{
		Root:   "/repo",
		Runner: runner,
		Stdout: &out,
	})
	if err != nil {
		t.Fatalf("CheckGo122: %v", err)
	}
	joined := callsToString(calls)
	if !strings.Contains(joined, "go install golang.org/dl/go1.22.10@latest") {
		t.Fatalf("calls = %s", joined)
	}
	if !strings.Contains(joined, "go1.22.10 download") || !strings.Contains(joined, "go1.22.10 build ./cmd/gormes") {
		t.Fatalf("calls = %s", joined)
	}
}

func callsToString(calls [][]string) string {
	var b strings.Builder
	for _, call := range calls {
		b.WriteString(strings.Join(call, " "))
		b.WriteByte('\n')
	}
	return b.String()
}

type exitErr int

func (e exitErr) Error() string { return "exit" }

func errExit(code int) error { return exitErr(code) }
```

- [ ] **Step 2: Run compatibility tests to verify they fail**

Run:

```bash
go test ./internal/repoctl -run TestGo122Compat -count=1
```

Expected: fail because `CheckGo122`, `Go122Options`, `RunnerFunc`, and `CommandResult` do not exist.

- [ ] **Step 3: Add shared command runner and compatibility implementation**

Create `internal/repoctl/compat.go`:

```go
package repoctl

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

type CommandResult struct {
	Stdout string
	Stderr string
	Err    error
}

type Runner interface {
	Run(context.Context, string, ...string) CommandResult
}

type RunnerFunc func(context.Context, string, ...string) CommandResult

func (f RunnerFunc) Run(ctx context.Context, name string, args ...string) CommandResult {
	return f(ctx, name, args...)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	if err == nil {
		return CommandResult{Stdout: string(out)}
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return CommandResult{Stdout: string(out), Stderr: string(ee.Stderr), Err: err}
	}
	return CommandResult{Stdout: string(out), Err: err}
}

type Go122Options struct {
	Root   string
	Runner Runner
	Stdout io.Writer
}

func CheckGo122(ctx context.Context, opts Go122Options) error {
	if opts.Root == "" {
		return fmt.Errorf("repo root is required")
	}
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}

	var build CommandResult
	if res := opts.Runner.Run(ctx, "docker", "--version"); res.Err == nil {
		build = opts.Runner.Run(ctx, "docker", "run", "--rm", "-v", opts.Root+":/src", "-w", "/src", "golang:1.22-alpine", "go", "build", "./cmd/gormes")
	} else {
		if res := opts.Runner.Run(ctx, "go", "install", "golang.org/dl/go1.22.10@latest"); res.Err != nil {
			return fmt.Errorf("install go1.22.10 fallback: %w", res.Err)
		}
		if res := opts.Runner.Run(ctx, "go1.22.10", "download"); res.Err != nil {
			return fmt.Errorf("download go1.22.10 fallback: %w", res.Err)
		}
		build = opts.Runner.Run(ctx, "go1.22.10", "build", "./cmd/gormes")
	}

	if build.Err != nil {
		fmt.Fprintln(opts.Stdout, "=== Decision data for 'Portability vs. Progress' ===")
		fmt.Fprintln(opts.Stdout, "Go 1.22 build failed")
		if build.Stdout != "" {
			fmt.Fprint(opts.Stdout, build.Stdout)
		}
		if build.Stderr != "" {
			fmt.Fprint(opts.Stdout, build.Stderr)
		}
		return build.Err
	}
	fmt.Fprintln(opts.Stdout, "=== Decision data for 'Portability vs. Progress' ===")
	fmt.Fprintln(opts.Stdout, "  Go 1.22 builds cleanly; no action needed")
	return nil
}
```

- [ ] **Step 4: Wire repoctl compat subcommand**

Modify `cmd/repoctl/main.go` to import `context` and add this branch before the usage error:

```go
if len(args) == 2 && args[0] == "compat" && args[1] == "go122" {
	return repoctl.CheckGo122(context.Background(), repoctl.Go122Options{Root: root})
}
```

Update the usage string to:

```go
return fmt.Errorf("usage: repoctl benchmark record | progress sync | readme update | compat go122")
```

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./internal/repoctl ./cmd/repoctl -count=1
```

Expected: pass.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/repoctl/compat.go internal/repoctl/compat_test.go cmd/repoctl/main.go
git commit -m "feat(repoctl): add Go 1.22 compatibility check"
```

---

### Task 4: Switch Makefile To Repoctl

**Files:**
- Modify: `Makefile`
- Modify: `scripts/record-benchmark.sh`
- Modify: `scripts/record-progress.sh`
- Modify: `scripts/update-readme.sh`
- Modify: `scripts/check-go1.22-compat.sh`

- [ ] **Step 1: Write failing Makefile path test**

Create `internal/repoctl/makefile_test.go`:

```go
package repoctl

import (
	"os"
	"strings"
	"testing"
)

func TestMakefileUsesRepoctlInsteadOfMaintenanceShell(t *testing.T) {
	raw, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, reject := range []string{
		"bash scripts/record-benchmark.sh",
		"bash scripts/record-progress.sh",
		"bash scripts/update-readme.sh",
	} {
		if strings.Contains(text, reject) {
			t.Fatalf("Makefile still contains %q", reject)
		}
	}
	for _, want := range []string{
		"go run ./cmd/repoctl benchmark record",
		"go run ./cmd/repoctl progress sync",
		"go run ./cmd/repoctl readme update",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Makefile missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/repoctl -run TestMakefileUsesRepoctl -count=1
```

Expected: fail while the Makefile still calls shell scripts.

- [ ] **Step 3: Replace Makefile shell calls**

Modify the Makefile helper blocks to:

```make
define record-benchmark
	@echo "Recording benchmark..."
	@go run ./cmd/repoctl benchmark record
endef

define update-readme
	@echo "Updating README.md..."
	@go run ./cmd/repoctl readme update
endef

define record-progress
	@echo "Updating progress..."
	@go run ./cmd/repoctl progress sync
endef
```

- [ ] **Step 4: Replace maintenance scripts with tiny wrappers**

Replace `scripts/record-benchmark.sh` with:

```sh
#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$REPO_ROOT"
exec go run ./cmd/repoctl benchmark record "$@"
```

Replace `scripts/record-progress.sh` with:

```sh
#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$REPO_ROOT"
exec go run ./cmd/repoctl progress sync "$@"
```

Replace `scripts/update-readme.sh` with:

```sh
#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$REPO_ROOT"
exec go run ./cmd/repoctl readme update "$@"
```

Replace `scripts/check-go1.22-compat.sh` with:

```sh
#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$REPO_ROOT"
exec go run ./cmd/repoctl compat go122 "$@"
```

- [ ] **Step 5: Run tests and Makefile target**

Run:

```bash
go test ./internal/repoctl ./cmd/repoctl -count=1
make validate-progress
```

Expected: tests pass and `make validate-progress` still validates `progress.json`.

- [ ] **Step 6: Commit**

Run:

```bash
git add Makefile scripts/record-benchmark.sh scripts/record-progress.sh scripts/update-readme.sh scripts/check-go1.22-compat.sh internal/repoctl/makefile_test.go
git commit -m "chore(repoctl): route maintenance scripts through Go"
```

---

### Task 5: Add Autoloop Config And Runner Foundations

**Files:**
- Create: `internal/autoloop/config.go`
- Create: `internal/autoloop/config_test.go`
- Create: `internal/autoloop/runner.go`
- Create: `internal/autoloop/runner_test.go`

- [ ] **Step 1: Write failing config defaults test**

Create `internal/autoloop/config_test.go`:

```go
package autoloop

import (
	"path/filepath"
	"testing"
)

func TestConfigFromEnvDefaultsToRepoRootPaths(t *testing.T) {
	root := t.TempDir()
	cfg, err := ConfigFromEnv(root, map[string]string{})
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.RepoRoot != root {
		t.Fatalf("RepoRoot = %q", cfg.RepoRoot)
	}
	if cfg.ProgressJSON != filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json") {
		t.Fatalf("ProgressJSON = %q", cfg.ProgressJSON)
	}
	if cfg.RunRoot != filepath.Join(root, ".codex", "orchestrator") {
		t.Fatalf("RunRoot = %q", cfg.RunRoot)
	}
	if cfg.Backend != "codexu" || cfg.Mode != "safe" || cfg.MaxAgents != 4 {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestConfigFromEnvReadsOverrides(t *testing.T) {
	root := t.TempDir()
	cfg, err := ConfigFromEnv(root, map[string]string{
		"RUN_ROOT":   "/tmp/run",
		"BACKEND":    "claudeu",
		"MODE":       "full",
		"MAX_AGENTS": "7",
	})
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.RunRoot != "/tmp/run" || cfg.Backend != "claudeu" || cfg.Mode != "full" || cfg.MaxAgents != 7 {
		t.Fatalf("cfg = %+v", cfg)
	}
}
```

- [ ] **Step 2: Write failing runner test**

Create `internal/autoloop/runner_test.go`:

```go
package autoloop

import (
	"context"
	"strings"
	"testing"
)

func TestFakeRunnerCapturesCommand(t *testing.T) {
	r := &FakeRunner{Results: []Result{{Stdout: "ok"}}}
	res := r.Run(context.Background(), Command{
		Name: "git",
		Args: []string{"status", "--short"},
		Dir:  "/repo",
		Env:  []string{"A=B"},
	})
	if res.Stdout != "ok" || res.Err != nil {
		t.Fatalf("result = %+v", res)
	}
	if len(r.Commands) != 1 {
		t.Fatalf("commands = %+v", r.Commands)
	}
	if got := strings.Join(append([]string{r.Commands[0].Name}, r.Commands[0].Args...), " "); got != "git status --short" {
		t.Fatalf("command = %q", got)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
go test ./internal/autoloop -run 'TestConfigFromEnv|TestFakeRunner' -count=1
```

Expected: fail because `internal/autoloop` does not exist.

- [ ] **Step 4: Add config implementation**

Create `internal/autoloop/config.go`:

```go
package autoloop

import (
	"fmt"
	"path/filepath"
	"strconv"
)

type Config struct {
	RepoRoot     string
	ProgressJSON string
	RunRoot      string
	Backend      string
	Mode         string
	MaxAgents    int
}

func ConfigFromEnv(repoRoot string, env map[string]string) (Config, error) {
	if repoRoot == "" {
		return Config{}, fmt.Errorf("repo root is required")
	}
	cfg := Config{
		RepoRoot:      repoRoot,
		ProgressJSON: filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json"),
		RunRoot:      filepath.Join(repoRoot, ".codex", "orchestrator"),
		Backend:      "codexu",
		Mode:         "safe",
		MaxAgents:    4,
	}
	if v := env["PROGRESS_JSON"]; v != "" {
		cfg.ProgressJSON = v
	}
	if v := env["RUN_ROOT"]; v != "" {
		cfg.RunRoot = v
	}
	if v := env["BACKEND"]; v != "" {
		cfg.Backend = v
	}
	if v := env["MODE"]; v != "" {
		cfg.Mode = v
	}
	if v := env["MAX_AGENTS"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("invalid MAX_AGENTS=%q", v)
		}
		cfg.MaxAgents = n
	}
	return cfg, nil
}
```

- [ ] **Step 5: Add runner implementation**

Create `internal/autoloop/runner.go`:

```go
package autoloop

import (
	"bytes"
	"context"
	"os/exec"
)

type Command struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

type Result struct {
	Stdout string
	Stderr string
	Err    error
}

type Runner interface {
	Run(context.Context, Command) Result
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, c Command) Result {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = c.Env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
}

type FakeRunner struct {
	Commands []Command
	Results  []Result
}

func (r *FakeRunner) Run(_ context.Context, c Command) Result {
	r.Commands = append(r.Commands, c)
	if len(r.Results) == 0 {
		return Result{}
	}
	res := r.Results[0]
	r.Results = r.Results[1:]
	return res
}
```

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./internal/autoloop -run 'TestConfigFromEnv|TestFakeRunner' -count=1
```

Expected: pass.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/autoloop/config.go internal/autoloop/config_test.go internal/autoloop/runner.go internal/autoloop/runner_test.go
git commit -m "feat(autoloop): add config and command runner foundations"
```

---

### Task 6: Add Backend Command Construction

**Files:**
- Create: `internal/autoloop/backend.go`
- Create: `internal/autoloop/backend_test.go`

- [ ] **Step 1: Write failing backend tests**

Create `internal/autoloop/backend_test.go`:

```go
package autoloop

import (
	"reflect"
	"testing"
)

func TestBuildBackendCommandCodexuSafe(t *testing.T) {
	got, err := BuildBackendCommand("codexu", "safe")
	if err != nil {
		t.Fatalf("BuildBackendCommand: %v", err)
	}
	want := []string{"codexu", "exec", "--json", "-m", "gpt-5.5", "-c", "approval_policy=never", "--sandbox", "workspace-write"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildBackendCommandCodexuFull(t *testing.T) {
	got, err := BuildBackendCommand("codexu", "full")
	if err != nil {
		t.Fatalf("BuildBackendCommand: %v", err)
	}
	want := []string{"codexu", "exec", "--json", "-m", "gpt-5.5", "-c", "approval_policy=never", "--sandbox", "danger-full-access"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestBuildBackendCommandClaudeuUsesShimShape(t *testing.T) {
	got, err := BuildBackendCommand("claudeu", "safe")
	if err != nil {
		t.Fatalf("BuildBackendCommand: %v", err)
	}
	if got[0] != "claudeu" || got[1] != "exec" || got[2] != "--json" {
		t.Fatalf("got %#v", got)
	}
}

func TestBuildBackendCommandRejectsInvalidMode(t *testing.T) {
	if _, err := BuildBackendCommand("codexu", "unknown"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/autoloop -run TestBuildBackendCommand -count=1
```

Expected: fail because `BuildBackendCommand` does not exist.

- [ ] **Step 3: Add backend implementation**

Create `internal/autoloop/backend.go`:

```go
package autoloop

import "fmt"

func BuildBackendCommand(backend, mode string) ([]string, error) {
	if backend == "" {
		backend = "codexu"
	}
	if mode == "" {
		mode = "safe"
	}
	var sandbox string
	switch mode {
	case "safe", "unattended":
		sandbox = "workspace-write"
	case "full":
		sandbox = "danger-full-access"
	default:
		return nil, fmt.Errorf("invalid MODE=%s", mode)
	}
	switch backend {
	case "codexu", "claudeu":
		return []string{backend, "exec", "--json", "-m", "gpt-5.5", "-c", "approval_policy=never", "--sandbox", sandbox}, nil
	case "opencode":
		return []string{"opencode", "run", "--no-interactive"}, nil
	default:
		return nil, fmt.Errorf("invalid BACKEND=%s", backend)
	}
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/autoloop -run TestBuildBackendCommand -count=1
```

Expected: pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/autoloop/backend.go internal/autoloop/backend_test.go
git commit -m "feat(autoloop): add backend command adapter"
```

---

### Task 7: Add Candidate Normalization

**Files:**
- Create: `internal/autoloop/candidates.go`
- Create: `internal/autoloop/candidates_test.go`

- [ ] **Step 1: Write failing candidate tests**

Create `internal/autoloop/candidates_test.go`:

```go
package autoloop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeCandidatesSkipsCompleteAndSortsActiveFirst(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "progress.json")
	raw := `{
	  "phases": {
	    "2": {"subphases": {"B": {"items": [
	      {"name": "done", "status": "complete"},
	      {"name": "active", "status": "in_progress"}
	    ]}}},
	    "1": {"subphases": {"A": {"items": [
	      {"name": "planned", "status": "planned"}
	    ]}}}
	  }
	}` + "\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("NormalizeCandidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, got %#v", len(got), got)
	}
	if got[0].ItemName != "active" || got[1].ItemName != "planned" {
		t.Fatalf("got %#v", got)
	}
}

func TestNormalizeCandidatesPriorityBoostWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.json")
	raw := `{"phases":{"3":{"subphases":{"E.7":{"items":[{"name":"boosted","status":"planned"}]}}},"2":{"subphases":{"A":{"items":[{"name":"normal","status":"in_progress"}]}}}}}` + "\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true, PriorityBoost: []string{"3.E.7"}})
	if err != nil {
		t.Fatalf("NormalizeCandidates: %v", err)
	}
	if got[0].ItemName != "boosted" {
		t.Fatalf("got %#v", got)
	}
}
```

- [ ] **Step 2: Run candidate tests to verify they fail**

Run:

```bash
go test ./internal/autoloop -run TestNormalizeCandidates -count=1
```

Expected: fail because `NormalizeCandidates` does not exist.

- [ ] **Step 3: Add candidate implementation**

Create `internal/autoloop/candidates.go`:

```go
package autoloop

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type CandidateOptions struct {
	ActiveFirst   bool
	PriorityBoost []string
}

type Candidate struct {
	PhaseID    string
	SubphaseID string
	ItemName   string
	Status     string
}

func NormalizeCandidates(path string, opts CandidateOptions) ([]Candidate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Phases map[string]struct {
			Subphases map[string]struct {
				Items []struct {
					Name     string `json:"name"`
					ItemName string `json:"item_name"`
					Title    string `json:"title"`
					ID       string `json:"id"`
					Status   string `json:"status"`
				} `json:"items"`
			} `json:"subphases"`
			SubPhases map[string]struct {
				Items []struct {
					Name     string `json:"name"`
					ItemName string `json:"item_name"`
					Title    string `json:"title"`
					ID       string `json:"id"`
					Status   string `json:"status"`
				} `json:"items"`
			} `json:"sub_phases"`
		} `json:"phases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	var out []Candidate
	for phaseID, phase := range doc.Phases {
		subphases := phase.Subphases
		if len(subphases) == 0 {
			subphases = phase.SubPhases
		}
		for subphaseID, sub := range subphases {
			for _, item := range sub.Items {
				name := firstNonEmpty(item.ItemName, item.Name, item.Title, item.ID)
				status := strings.ToLower(item.Status)
				if name == "" || status == "complete" {
					continue
				}
				out = append(out, Candidate{PhaseID: phaseID, SubphaseID: subphaseID, ItemName: name, Status: status})
			}
		}
	}
	boost := map[string]bool{}
	for _, b := range opts.PriorityBoost {
		boost[strings.ToLower(strings.TrimSpace(b))] = true
	}
	sort.Slice(out, func(i, j int) bool {
		ai := candidateRank(out[i], opts.ActiveFirst, boost)
		aj := candidateRank(out[j], opts.ActiveFirst, boost)
		if ai != aj {
			return ai < aj
		}
		return fmt.Sprintf("%s/%s/%s", out[i].PhaseID, out[i].SubphaseID, out[i].ItemName) < fmt.Sprintf("%s/%s/%s", out[j].PhaseID, out[j].SubphaseID, out[j].ItemName)
	})
	return out, nil
}

func candidateRank(c Candidate, activeFirst bool, boost map[string]bool) int {
	if boost[strings.ToLower(c.PhaseID+"."+c.SubphaseID)] {
		return 0
	}
	if !activeFirst {
		return 1
	}
	switch c.Status {
	case "in_progress":
		return 1
	case "planned":
		return 2
	default:
		return 3
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
go test ./internal/autoloop -run TestNormalizeCandidates -count=1
```

Expected: pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/autoloop/candidates.go internal/autoloop/candidates_test.go
git commit -m "feat(autoloop): normalize progress candidates"
```

---

### Task 8: Add Claims, Failure Records, And Report Parsing

**Files:**
- Create: `internal/autoloop/claims.go`
- Create: `internal/autoloop/claims_test.go`
- Create: `internal/autoloop/failures.go`
- Create: `internal/autoloop/failures_test.go`
- Create: `internal/autoloop/report.go`
- Create: `internal/autoloop/report_test.go`

- [ ] **Step 1: Write failing claim test**

Create `internal/autoloop/claims_test.go`:

```go
package autoloop

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupStaleLocksRemovesExpiredClaim(t *testing.T) {
	dir := t.TempDir()
	lockDir := filepath.Join(dir, "locks")
	if err := os.MkdirAll(filepath.Join(lockDir, "task.lock"), 0o755); err != nil {
		t.Fatal(err)
	}
	claim := filepath.Join(lockDir, "task.lock.claim.json")
	raw := `{"pid":999999,"claimed_at_epoch":1}` + "\n"
	if err := os.WriteFile(claim, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CleanupStaleLocks(lockDir, time.Hour, func() time.Time { return time.Unix(7200, 0) }); err != nil {
		t.Fatalf("CleanupStaleLocks: %v", err)
	}
	if _, err := os.Stat(filepath.Join(lockDir, "task.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock dir still exists: %v", err)
	}
}
```

- [ ] **Step 2: Write failing failure record test**

Create `internal/autoloop/failures_test.go`:

```go
package autoloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFailureRecordWriteIncrementsCountAndTailsStderr(t *testing.T) {
	dir := t.TempDir()
	stderrPath := filepath.Join(dir, "stderr.log")
	if err := os.WriteFile(stderrPath, []byte(strings.Repeat("x\n", 50)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFailureRecord(dir, "slug", 2, "test failed", stderrPath, []string{"red failed"}); err != nil {
		t.Fatalf("WriteFailureRecord: %v", err)
	}
	if err := WriteFailureRecord(dir, "slug", 3, "test failed again", stderrPath, nil); err != nil {
		t.Fatalf("WriteFailureRecord second: %v", err)
	}
	rec, err := ReadFailureRecord(dir, "slug")
	if err != nil {
		t.Fatalf("ReadFailureRecord: %v", err)
	}
	if rec.Count != 2 || rec.LastRC != 3 || rec.LastReason != "test failed again" {
		t.Fatalf("record = %+v", rec)
	}
	if got := strings.Count(rec.LastStderrTail, "\n"); got > 40 {
		t.Fatalf("stderr tail line count = %d", got)
	}
}
```

- [ ] **Step 3: Write failing report parser test**

Create `internal/autoloop/report_test.go`:

```go
package autoloop

import "testing"

func TestParseFinalReportRequiresAcceptanceAndCommit(t *testing.T) {
	report := `# Final

Commit: abc123

Acceptance:
- RED: exit 1
- GREEN: exit 0
- go test ./...: exit 0
`
	got, err := ParseFinalReport(report)
	if err != nil {
		t.Fatalf("ParseFinalReport: %v", err)
	}
	if got.Commit != "abc123" || len(got.Acceptance) != 3 {
		t.Fatalf("got = %+v", got)
	}
}

func TestParseFinalReportRejectsMissingRed(t *testing.T) {
	report := `Commit: abc123

Acceptance:
- GREEN: exit 0
`
	if _, err := ParseFinalReport(report); err == nil {
		t.Fatal("expected missing RED error")
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run:

```bash
go test ./internal/autoloop -run 'TestCleanupStaleLocks|TestFailureRecord|TestParseFinalReport' -count=1
```

Expected: fail because claims, failures, and report parser functions do not exist.

- [ ] **Step 5: Add claims implementation**

Create `internal/autoloop/claims.go`:

```go
package autoloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type claimFile struct {
	PID            int   `json:"pid"`
	ClaimedAtEpoch int64 `json:"claimed_at_epoch"`
}

func CleanupStaleLocks(lockRoot string, ttl time.Duration, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}
	entries, err := filepath.Glob(filepath.Join(lockRoot, "*.lock"))
	if err != nil {
		return err
	}
	for _, lock := range entries {
		claimPath := lock + ".claim.json"
		raw, err := os.ReadFile(claimPath)
		if err != nil {
			_ = os.RemoveAll(lock)
			_ = os.Remove(claimPath)
			continue
		}
		var claim claimFile
		if err := json.Unmarshal(raw, &claim); err != nil || claim.PID <= 0 {
			_ = os.RemoveAll(lock)
			_ = os.Remove(claimPath)
			continue
		}
		live := processLive(claim.PID)
		expired := claim.ClaimedAtEpoch > 0 && now().Unix()-claim.ClaimedAtEpoch > int64(ttl.Seconds())
		if !live || expired {
			_ = os.RemoveAll(lock)
			_ = os.Remove(claimPath)
		}
	}
	return nil
}

func processLive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || strings.Contains(err.Error(), "operation not permitted") || strconv.Itoa(pid) == strconv.Itoa(os.Getpid())
}
```

- [ ] **Step 6: Add failure record implementation**

Create `internal/autoloop/failures.go`:

```go
package autoloop

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type FailureRecord struct {
	Count           int      `json:"count"`
	LastRC          int      `json:"last_rc"`
	LastReason      string   `json:"last_reason"`
	LastStderrTail  string   `json:"last_stderr_tail"`
	LastFinalErrors []string `json:"last_final_errors"`
}

func failureRecordPath(root, slug string) string {
	return filepath.Join(root, "task-failures", slug+".json")
}

func ReadFailureRecord(root, slug string) (FailureRecord, error) {
	raw, err := os.ReadFile(failureRecordPath(root, slug))
	if err != nil {
		return FailureRecord{}, err
	}
	var rec FailureRecord
	return rec, json.Unmarshal(raw, &rec)
}

func WriteFailureRecord(root, slug string, rc int, reason, stderrPath string, finalErrors []string) error {
	rec, err := ReadFailureRecord(root, slug)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	rec.Count++
	rec.LastRC = rc
	rec.LastReason = reason
	rec.LastStderrTail = tailFile(stderrPath, 40)
	rec.LastFinalErrors = append([]string(nil), finalErrors...)
	path := failureRecordPath(root, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}

func tailFile(path string, maxLines int) string {
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > maxLines {
			lines = lines[1:]
		}
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 7: Add report parser implementation**

Create `internal/autoloop/report.go`:

```go
package autoloop

import (
	"fmt"
	"regexp"
	"strings"
)

type FinalReport struct {
	Commit     string
	Acceptance []string
}

var commitRE = regexp.MustCompile(`(?m)^Commit:\s*([0-9a-fA-F]+)\s*$`)

func ParseFinalReport(text string) (FinalReport, error) {
	m := commitRE.FindStringSubmatch(text)
	if len(m) != 2 {
		return FinalReport{}, fmt.Errorf("final report missing commit")
	}
	lines := strings.Split(text, "\n")
	var acceptance []string
	inAcceptance := false
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "acceptance:") {
			inAcceptance = true
			continue
		}
		if inAcceptance && strings.HasPrefix(strings.TrimSpace(line), "-") {
			acceptance = append(acceptance, strings.TrimSpace(line))
		}
	}
	if len(acceptance) == 0 {
		return FinalReport{}, fmt.Errorf("final report missing acceptance")
	}
	hasRed := false
	hasGreen := false
	for _, line := range acceptance {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "red") && strings.Contains(lower, "exit 1") {
			hasRed = true
		}
		if strings.Contains(lower, "green") && strings.Contains(lower, "exit 0") {
			hasGreen = true
		}
	}
	if !hasRed {
		return FinalReport{}, fmt.Errorf("final report missing failing RED evidence")
	}
	if !hasGreen {
		return FinalReport{}, fmt.Errorf("final report missing passing GREEN evidence")
	}
	return FinalReport{Commit: m[1], Acceptance: acceptance}, nil
}
```

- [ ] **Step 8: Run tests**

Run:

```bash
go test ./internal/autoloop -run 'TestCleanupStaleLocks|TestFailureRecord|TestParseFinalReport' -count=1
```

Expected: pass.

- [ ] **Step 9: Commit**

Run:

```bash
git add internal/autoloop/claims.go internal/autoloop/claims_test.go internal/autoloop/failures.go internal/autoloop/failures_test.go internal/autoloop/report.go internal/autoloop/report_test.go
git commit -m "feat(autoloop): add claims failures and report parsing"
```

---

### Task 9: Add Worktree And Promotion Primitives

**Files:**
- Create: `internal/autoloop/worktree.go`
- Create: `internal/autoloop/worktree_test.go`
- Create: `internal/autoloop/promote.go`
- Create: `internal/autoloop/promote_test.go`

- [ ] **Step 1: Write failing worktree path tests**

Create `internal/autoloop/worktree_test.go`:

```go
package autoloop

import "testing"

func TestWorkerBranchName(t *testing.T) {
	if got := WorkerBranchName("run-1", 3); got != "codexu/run-1/worker3" {
		t.Fatalf("branch = %q", got)
	}
}

func TestWorkerRepoRootHonorsSubdir(t *testing.T) {
	if got := WorkerRepoRoot("/tmp/wt/worker1", "."); got != "/tmp/wt/worker1" {
		t.Fatalf("root = %q", got)
	}
	if got := WorkerRepoRoot("/tmp/wt/worker1", "gormes"); got != "/tmp/wt/worker1/gormes" {
		t.Fatalf("root = %q", got)
	}
}
```

- [ ] **Step 2: Write failing promotion fallback test**

Create `internal/autoloop/promote_test.go`:

```go
package autoloop

import (
	"context"
	"strings"
	"testing"
)

func TestPromoteFallsBackToCherryPickWhenGHFails(t *testing.T) {
	r := &FakeRunner{Results: []Result{
		{Err: assertErr("push failed")},
		{},
	}}
	err := PromoteWorker(context.Background(), PromoteOptions{
		Runner:        r,
		RepoRoot:      "/repo",
		WorkerBranch:  "codexu/run/worker1",
		WorkerCommit:  "abc123",
		PromotionMode: "pr",
	})
	if err != nil {
		t.Fatalf("PromoteWorker: %v", err)
	}
	joined := commandsString(r.Commands)
	if !strings.Contains(joined, "git push origin codexu/run/worker1") {
		t.Fatalf("commands = %s", joined)
	}
	if !strings.Contains(joined, "git cherry-pick -Xtheirs abc123") {
		t.Fatalf("commands = %s", joined)
	}
}

func commandsString(commands []Command) string {
	var b strings.Builder
	for _, c := range commands {
		b.WriteString(c.Name)
		for _, a := range c.Args {
			b.WriteByte(' ')
			b.WriteString(a)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
go test ./internal/autoloop -run 'TestWorker|TestPromoteFallsBack' -count=1
```

Expected: fail because worktree and promotion functions do not exist.

- [ ] **Step 4: Add worktree implementation**

Create `internal/autoloop/worktree.go`:

```go
package autoloop

import "path/filepath"

func WorkerBranchName(runID string, workerID int) string {
	return "codexu/" + runID + "/worker" + itoa(workerID)
}

func WorkerRepoRoot(worktreeRoot, repoSubdir string) string {
	if repoSubdir == "" || repoSubdir == "." {
		return worktreeRoot
	}
	return filepath.Join(worktreeRoot, repoSubdir)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
```

- [ ] **Step 5: Add promotion implementation**

Create `internal/autoloop/promote.go`:

```go
package autoloop

import (
	"context"
	"fmt"
)

type PromoteOptions struct {
	Runner        Runner
	RepoRoot      string
	WorkerBranch  string
	WorkerCommit  string
	PromotionMode string
}

func PromoteWorker(ctx context.Context, opts PromoteOptions) error {
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}
	if opts.PromotionMode == "" {
		opts.PromotionMode = "pr"
	}
	if opts.RepoRoot == "" || opts.WorkerBranch == "" || opts.WorkerCommit == "" {
		return fmt.Errorf("repo root, worker branch, and worker commit are required")
	}
	if opts.PromotionMode == "cherry-pick" {
		return cherryPick(ctx, opts)
	}
	res := opts.Runner.Run(ctx, Command{Name: "git", Args: []string{"push", "origin", opts.WorkerBranch}, Dir: opts.RepoRoot})
	if res.Err != nil {
		return cherryPick(ctx, opts)
	}
	res = opts.Runner.Run(ctx, Command{Name: "gh", Args: []string{"pr", "create", "--fill", "--head", opts.WorkerBranch}, Dir: opts.RepoRoot})
	if res.Err != nil {
		return cherryPick(ctx, opts)
	}
	return nil
}

func cherryPick(ctx context.Context, opts PromoteOptions) error {
	res := opts.Runner.Run(ctx, Command{Name: "git", Args: []string{"cherry-pick", "-Xtheirs", opts.WorkerCommit}, Dir: opts.RepoRoot})
	return res.Err
}
```

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./internal/autoloop -run 'TestWorker|TestPromoteFallsBack' -count=1
```

Expected: pass.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/autoloop/worktree.go internal/autoloop/worktree_test.go internal/autoloop/promote.go internal/autoloop/promote_test.go
git commit -m "feat(autoloop): add worktree and promotion primitives"
```

---

### Task 10: Add Companion Scheduling And Ledger

**Files:**
- Create: `internal/autoloop/companions.go`
- Create: `internal/autoloop/companions_test.go`
- Create: `internal/autoloop/ledger.go`
- Create: `internal/autoloop/ledger_test.go`

- [ ] **Step 1: Write failing companion scheduler test**

Create `internal/autoloop/companions_test.go`:

```go
package autoloop

import (
	"testing"
	"time"
)

func TestCompanionDuePlannerOnCadence(t *testing.T) {
	state := CompanionState{LastCycle: 2, LastEpoch: 100}
	decision := CompanionDue(CompanionOptions{
		Name:           "planner",
		CurrentCycle:   6,
		EveryNCycles:   4,
		Now:            time.Unix(200, 0),
		LoopSleep:      time.Second,
		ExternalRecent: false,
	}, state)
	if !decision.Run {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestCompanionSkipsWhenDisabled(t *testing.T) {
	decision := CompanionDue(CompanionOptions{Name: "planner", Disabled: true}, CompanionState{})
	if decision.Run {
		t.Fatalf("decision = %+v", decision)
	}
}
```

- [ ] **Step 2: Write failing ledger test**

Create `internal/autoloop/ledger_test.go`:

```go
package autoloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendLedgerEventWritesJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	err := AppendLedgerEvent(path, LedgerEvent{
		TS:     time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC),
		Event:  "worker_claimed",
		Worker: 2,
		Task:   "3.E.7",
	})
	if err != nil {
		t.Fatalf("AppendLedgerEvent: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got LedgerEvent
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("invalid jsonl: %v\n%s", err, raw)
	}
	if got.Event != "worker_claimed" || got.Worker != 2 || got.Task != "3.E.7" {
		t.Fatalf("got = %+v", got)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
go test ./internal/autoloop -run 'TestCompanion|TestAppendLedger' -count=1
```

Expected: fail because companion and ledger functions do not exist.

- [ ] **Step 4: Add companion implementation**

Create `internal/autoloop/companions.go`:

```go
package autoloop

import "time"

type CompanionState struct {
	LastCycle int
	LastEpoch int64
}

type CompanionOptions struct {
	Name           string
	CurrentCycle   int
	EveryNCycles   int
	EveryDuration  time.Duration
	Now            time.Time
	LoopSleep      time.Duration
	ExternalRecent bool
	Disabled       bool
}

type CompanionDecision struct {
	Run    bool
	Reason string
}

func CompanionDue(opts CompanionOptions, state CompanionState) CompanionDecision {
	if opts.Disabled {
		return CompanionDecision{Reason: "disabled"}
	}
	if opts.ExternalRecent {
		return CompanionDecision{Reason: "external scheduler ran recently"}
	}
	if opts.EveryNCycles > 0 && opts.CurrentCycle-state.LastCycle >= opts.EveryNCycles {
		return CompanionDecision{Run: true, Reason: "cycle cadence reached"}
	}
	if opts.EveryDuration > 0 && opts.Now.Unix()-state.LastEpoch >= int64(opts.EveryDuration.Seconds()) {
		return CompanionDecision{Run: true, Reason: "time cadence reached"}
	}
	return CompanionDecision{Reason: "not due"}
}
```

- [ ] **Step 5: Add ledger implementation**

Create `internal/autoloop/ledger.go`:

```go
package autoloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type LedgerEvent struct {
	TS     time.Time `json:"ts"`
	Event  string    `json:"event"`
	Worker int       `json:"worker,omitempty"`
	Task   string    `json:"task,omitempty"`
	Status string    `json:"status,omitempty"`
}

func AppendLedgerEvent(path string, event LedgerEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(raw)
	return err
}
```

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./internal/autoloop -run 'TestCompanion|TestAppendLedger' -count=1
```

Expected: pass.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/autoloop/companions.go internal/autoloop/companions_test.go internal/autoloop/ledger.go internal/autoloop/ledger_test.go
git commit -m "feat(autoloop): add companion cadence and ledger events"
```

---

### Task 11: Add Audit, Digest, And Service Rendering

**Files:**
- Create: `internal/autoloop/audit.go`
- Create: `internal/autoloop/audit_test.go`
- Create: `internal/autoloop/service.go`
- Create: `internal/autoloop/service_test.go`

- [ ] **Step 1: Write failing audit test**

Create `internal/autoloop/audit_test.go`:

```go
package autoloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDigestLedgerCountsLastEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	raw := `{"ts":"2026-04-24T10:00:00Z","event":"run_started"}
{"ts":"2026-04-24T10:01:00Z","event":"worker_claimed"}
{"ts":"2026-04-24T10:02:00Z","event":"worker_success"}
{"ts":"2026-04-24T10:03:00Z","event":"worker_promoted"}
`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := DigestLedger(path)
	if err != nil {
		t.Fatalf("DigestLedger: %v", err)
	}
	if !strings.Contains(digest, "runs: 1") || !strings.Contains(digest, "promoted: 1") {
		t.Fatalf("digest = %s", digest)
	}
}
```

- [ ] **Step 2: Write failing service rendering test**

Create `internal/autoloop/service_test.go`:

```go
package autoloop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderServiceUnitInjectsPaths(t *testing.T) {
	unit := RenderServiceUnit(ServiceUnitOptions{
		AutoloopPath: "/repo/bin/autoloop",
		WorkDir:      "/repo",
	})
	if !strings.Contains(unit, "ExecStart=/repo/bin/autoloop run") {
		t.Fatalf("unit = %s", unit)
	}
	if !strings.Contains(unit, "WorkingDirectory=/repo") {
		t.Fatalf("unit = %s", unit)
	}
}

func TestInstallServiceWritesUnitAndReloadsSystemd(t *testing.T) {
	dir := t.TempDir()
	r := &FakeRunner{}
	err := InstallService(context.Background(), ServiceInstallOptions{
		Runner:       r,
		UnitDir:      dir,
		UnitName:     "gormes-orchestrator.service",
		AutoloopPath: "/repo/bin/autoloop",
		WorkDir:      "/repo",
		AutoStart:    true,
		Force:        true,
	})
	if err != nil {
		t.Fatalf("InstallService: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "gormes-orchestrator.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "ExecStart=/repo/bin/autoloop run") {
		t.Fatalf("unit = %s", raw)
	}
	joined := commandsString(r.Commands)
	if !strings.Contains(joined, "systemctl --user daemon-reload") {
		t.Fatalf("commands = %s", joined)
	}
	if !strings.Contains(joined, "systemctl --user enable --now gormes-orchestrator.service") {
		t.Fatalf("commands = %s", joined)
	}
}

func TestDisableLegacyTimersRunsExpectedSystemctlCalls(t *testing.T) {
	r := &FakeRunner{}
	err := DisableLegacyTimers(context.Background(), r)
	if err != nil {
		t.Fatalf("DisableLegacyTimers: %v", err)
	}
	joined := commandsString(r.Commands)
	for _, want := range []string{
		"systemctl --user disable --now gormes-architecture-planner-tasks-manager.timer",
		"systemctl --user disable --now gormes-architectureplanneragent.timer",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("commands missing %q:\n%s", want, joined)
		}
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
go test ./internal/autoloop -run 'TestDigestLedger|TestRenderServiceUnit|TestInstallService|TestDisableLegacyTimers' -count=1
```

Expected: fail because audit and service functions do not exist.

- [ ] **Step 4: Add audit implementation**

Create `internal/autoloop/audit.go`:

```go
package autoloop

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func DigestLedger(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	counts := map[string]int{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var ev LedgerEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			return "", err
		}
		counts[ev.Event]++
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "runs: %d\n", counts["run_started"])
	fmt.Fprintf(&b, "claimed: %d\n", counts["worker_claimed"])
	fmt.Fprintf(&b, "success: %d\n", counts["worker_success"])
	fmt.Fprintf(&b, "promoted: %d\n", counts["worker_promoted"])
	return b.String(), nil
}
```

- [ ] **Step 5: Add service rendering implementation**

Create `internal/autoloop/service.go`:

```go
package autoloop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type ServiceUnitOptions struct {
	AutoloopPath string
	WorkDir      string
}

func RenderServiceUnit(opts ServiceUnitOptions) string {
	return fmt.Sprintf(`[Unit]
Description=Gormes autoloop

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s run
Restart=on-failure

[Install]
WantedBy=default.target
`, opts.WorkDir, opts.AutoloopPath)
}

type ServiceInstallOptions struct {
	Runner       Runner
	UnitDir      string
	UnitName     string
	AutoloopPath string
	WorkDir      string
	AutoStart    bool
	Force        bool
}

func InstallService(ctx context.Context, opts ServiceInstallOptions) error {
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}
	if opts.UnitDir == "" || opts.UnitName == "" {
		return fmt.Errorf("unit dir and unit name are required")
	}
	if err := os.MkdirAll(opts.UnitDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(opts.UnitDir, opts.UnitName)
	if !opts.Force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("unit already exists: %s", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	body := RenderServiceUnit(ServiceUnitOptions{AutoloopPath: opts.AutoloopPath, WorkDir: opts.WorkDir})
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	if res := opts.Runner.Run(ctx, Command{Name: "systemctl", Args: []string{"--user", "daemon-reload"}}); res.Err != nil {
		return res.Err
	}
	if opts.AutoStart {
		if res := opts.Runner.Run(ctx, Command{Name: "systemctl", Args: []string{"--user", "enable", "--now", opts.UnitName}}); res.Err != nil {
			return res.Err
		}
	}
	return nil
}

func DisableLegacyTimers(ctx context.Context, runner Runner) error {
	if runner == nil {
		runner = ExecRunner{}
	}
	for _, unit := range []string{
		"gormes-architecture-planner-tasks-manager.timer",
		"gormes-architectureplanneragent.timer",
	} {
		if res := runner.Run(ctx, Command{Name: "systemctl", Args: []string{"--user", "disable", "--now", unit}}); res.Err != nil {
			return res.Err
		}
	}
	return nil
}
```

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./internal/autoloop -run 'TestDigestLedger|TestRenderServiceUnit|TestInstallService|TestDisableLegacyTimers' -count=1
```

Expected: pass.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/autoloop/audit.go internal/autoloop/audit_test.go internal/autoloop/service.go internal/autoloop/service_test.go
git commit -m "feat(autoloop): add audit digest and service rendering"
```

---

### Task 12: Add Autoloop Run Skeleton And CLI

**Files:**
- Create: `internal/autoloop/run.go`
- Create: `internal/autoloop/run_test.go`
- Create: `cmd/autoloop/main.go`
- Create: `cmd/autoloop/main_test.go`

- [ ] **Step 1: Write failing run skeleton test**

Create `internal/autoloop/run_test.go`:

```go
package autoloop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDryRunSelectsCandidatesWithoutRunningBackend(t *testing.T) {
	root := t.TempDir()
	progress := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan")
	if err := os.MkdirAll(progress, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"phases":{"1":{"subphases":{"A":{"items":[{"name":"first","status":"planned"}]}}}}}` + "\n"
	if err := os.WriteFile(filepath.Join(progress, "progress.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &FakeRunner{}
	summary, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:      root,
			ProgressJSON: filepath.Join(progress, "progress.json"),
			RunRoot:      filepath.Join(root, ".codex", "orchestrator"),
			Backend:      "codexu",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: r,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if summary.Candidates != 1 || summary.Selected[0].ItemName != "first" {
		t.Fatalf("summary = %+v", summary)
	}
	if len(r.Commands) != 0 {
		t.Fatalf("dry-run executed commands: %+v", r.Commands)
	}
}
```

- [ ] **Step 2: Write failing CLI test**

Create `cmd/autoloop/main_test.go`:

```go
package main

import "testing"

func TestRunRejectsUnknownCommand(t *testing.T) {
	if err := run([]string{"unknown"}); err == nil {
		t.Fatal("expected usage error")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
go test ./internal/autoloop ./cmd/autoloop -run 'TestDryRun|TestRunRejects' -count=1
```

Expected: fail because `RunOnce` and `cmd/autoloop` do not exist.

- [ ] **Step 4: Add run skeleton**

Create `internal/autoloop/run.go`:

```go
package autoloop

import "context"

type RunOptions struct {
	Config Config
	Runner Runner
	DryRun bool
}

type RunSummary struct {
	Candidates int
	Selected   []Candidate
}

func RunOnce(ctx context.Context, opts RunOptions) (RunSummary, error) {
	candidates, err := NormalizeCandidates(opts.Config.ProgressJSON, CandidateOptions{ActiveFirst: true})
	if err != nil {
		return RunSummary{}, err
	}
	selected := candidates
	if opts.Config.MaxAgents > 0 && len(selected) > opts.Config.MaxAgents {
		selected = selected[:opts.Config.MaxAgents]
	}
	if opts.DryRun {
		return RunSummary{Candidates: len(candidates), Selected: selected}, nil
	}
	if opts.Runner == nil {
		opts.Runner = ExecRunner{}
	}
	argv, err := BuildBackendCommand(opts.Config.Backend, opts.Config.Mode)
	if err != nil {
		return RunSummary{}, err
	}
	for range selected {
		res := opts.Runner.Run(ctx, Command{Name: argv[0], Args: argv[1:], Dir: opts.Config.RepoRoot})
		if res.Err != nil {
			return RunSummary{}, res.Err
		}
	}
	return RunSummary{Candidates: len(candidates), Selected: selected}, nil
}
```

- [ ] **Step 5: Add CLI**

Create `cmd/autoloop/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/TrebuchetDynamics/gormes-agent/internal/autoloop"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	env := map[string]string{}
	for _, key := range []string{"RUN_ROOT", "BACKEND", "MODE", "MAX_AGENTS"} {
		env[key] = os.Getenv(key)
	}
	cfg, err := autoloop.ConfigFromEnv(root, env)
	if err != nil {
		return err
	}
	if len(args) == 1 && args[0] == "run" {
		_, err := autoloop.RunOnce(context.Background(), autoloop.RunOptions{Config: cfg})
		return err
	}
	if len(args) == 2 && args[0] == "run" && args[1] == "--dry-run" {
		summary, err := autoloop.RunOnce(context.Background(), autoloop.RunOptions{Config: cfg, DryRun: true})
		if err != nil {
			return err
		}
		fmt.Printf("candidates=%d selected=%d\n", summary.Candidates, len(summary.Selected))
		return nil
	}
	if len(args) == 1 && args[0] == "digest" {
		out, err := autoloop.DigestLedger(cfg.RunRoot + "/state/runs.jsonl")
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	}
	return fmt.Errorf("usage: autoloop run [--dry-run] | digest")
}
```

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./internal/autoloop ./cmd/autoloop -count=1
```

Expected: pass.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/autoloop/run.go internal/autoloop/run_test.go cmd/autoloop/main.go cmd/autoloop/main_test.go
git commit -m "feat(autoloop): add run skeleton and CLI"
```

---

### Task 13: Move Legacy Shell To Vendored Parity Fixtures

**Files:**
- Modify: `.gitattributes`
- Move: `scripts/gormes-auto-codexu-orchestrator.sh`
- Move: `scripts/orchestrator/lib/*.sh`
- Move: `scripts/orchestrator/tests/fixtures/**`
- Create: `scripts/gormes-auto-codexu-orchestrator.sh`

- [ ] **Step 1: Write failing linguist fixture test**

Create `internal/autoloop/legacy_fixture_test.go`:

```go
package autoloop

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyShellMarkedVendored(t *testing.T) {
	raw, err := os.ReadFile("../../.gitattributes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "testdata/legacy-shell/** linguist-vendored") {
		t.Fatalf(".gitattributes missing legacy-shell vendored rule")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/autoloop -run TestLegacyShellMarkedVendored -count=1
```

Expected: fail if `.gitattributes` has no vendored legacy-shell rule.

- [ ] **Step 3: Move frozen shell sources**

Run:

```bash
mkdir -p testdata/legacy-shell/scripts/orchestrator
mkdir -p testdata/legacy-shell/scripts/orchestrator/tests
git mv scripts/gormes-auto-codexu-orchestrator.sh testdata/legacy-shell/scripts/gormes-auto-codexu-orchestrator.sh
git mv scripts/orchestrator/lib testdata/legacy-shell/scripts/orchestrator/lib
git mv scripts/orchestrator/tests/fixtures testdata/legacy-shell/scripts/orchestrator/tests/fixtures
```

- [ ] **Step 4: Add `.gitattributes` rule**

Append to `.gitattributes`:

```text
testdata/legacy-shell/** linguist-vendored
```

- [ ] **Step 5: Add temporary autoloop wrapper**

Create `scripts/gormes-auto-codexu-orchestrator.sh`:

```sh
#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$REPO_ROOT"
exec go run ./cmd/autoloop run "$@"
```

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./internal/autoloop -run TestLegacyShellMarkedVendored -count=1
go test ./internal/autoloop ./cmd/autoloop -count=1
```

Expected: pass.

- [ ] **Step 7: Commit**

Run:

```bash
git add .gitattributes scripts/gormes-auto-codexu-orchestrator.sh testdata/legacy-shell/scripts/gormes-auto-codexu-orchestrator.sh testdata/legacy-shell/scripts/orchestrator/lib internal/autoloop/legacy_fixture_test.go
git add testdata/legacy-shell/scripts/orchestrator/tests/fixtures
git commit -m "chore(autoloop): vendor legacy shell parity fixtures"
```

---

### Task 14: Replace Remaining Orchestrator Shell Entrypoints With Go Wrappers

**Files:**
- Modify: `scripts/orchestrator/audit.sh`
- Modify: `scripts/orchestrator/daily-digest.sh`
- Modify: `scripts/orchestrator/install-service.sh`
- Modify: `scripts/orchestrator/install-audit.sh`
- Modify: `scripts/orchestrator/disable-legacy-timers.sh`
- Modify: `cmd/autoloop/main.go`

- [ ] **Step 1: Add CLI branches for audit and service commands**

Modify `cmd/autoloop/main.go` to add these branches before the usage error:

```go
if len(args) == 1 && args[0] == "audit" {
	out, err := autoloop.DigestLedger(cfg.RunRoot + "/state/runs.jsonl")
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}
if len(args) >= 2 && args[0] == "service" && args[1] == "install" {
	return autoloop.InstallService(context.Background(), autoloop.ServiceInstallOptions{
		UnitDir:      os.Getenv("XDG_CONFIG_HOME") + "/systemd/user",
		UnitName:     "gormes-orchestrator.service",
		AutoloopPath: "autoloop",
		WorkDir:      root,
		AutoStart:    true,
		Force:        contains(args, "--force"),
	})
}
if len(args) >= 2 && args[0] == "service" && args[1] == "install-audit" {
	return autoloop.InstallService(context.Background(), autoloop.ServiceInstallOptions{
		UnitDir:      os.Getenv("XDG_CONFIG_HOME") + "/systemd/user",
		UnitName:     "gormes-orchestrator-audit.service",
		AutoloopPath: "autoloop",
		WorkDir:      root,
		AutoStart:    true,
		Force:        contains(args, "--force"),
	})
}
if len(args) == 3 && args[0] == "service" && args[1] == "disable" && args[2] == "legacy-timers" {
	return autoloop.DisableLegacyTimers(context.Background(), autoloop.ExecRunner{})
}
```

Update the usage error to:

```go
return fmt.Errorf("usage: autoloop run [--dry-run] | audit | digest | service install | service install-audit | service disable legacy-timers")
```

Add this helper in `cmd/autoloop/main.go`:

```go
func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Replace shell entrypoints with wrappers**

Replace `scripts/orchestrator/audit.sh` with:

```sh
#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
cd "$REPO_ROOT"
exec go run ./cmd/autoloop audit "$@"
```

Replace `scripts/orchestrator/daily-digest.sh` with:

```sh
#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
cd "$REPO_ROOT"
exec go run ./cmd/autoloop digest "$@"
```

Replace `scripts/orchestrator/install-service.sh` with:

```sh
#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
cd "$REPO_ROOT"
exec go run ./cmd/autoloop service install "$@"
```

Replace `scripts/orchestrator/install-audit.sh` with:

```sh
#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
cd "$REPO_ROOT"
exec go run ./cmd/autoloop service install-audit "$@"
```

Replace `scripts/orchestrator/disable-legacy-timers.sh` with:

```sh
#!/usr/bin/env sh
set -eu
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
cd "$REPO_ROOT"
exec go run ./cmd/autoloop service disable legacy-timers "$@"
```

- [ ] **Step 3: Run tests**

Run:

```bash
go test ./internal/autoloop ./cmd/autoloop -count=1
```

Expected: pass.

- [ ] **Step 4: Commit**

Run:

```bash
git add cmd/autoloop/main.go scripts/orchestrator/audit.sh scripts/orchestrator/daily-digest.sh scripts/orchestrator/install-service.sh scripts/orchestrator/install-audit.sh scripts/orchestrator/disable-legacy-timers.sh
git commit -m "chore(autoloop): wrap orchestrator entrypoints with Go"
```

---

### Task 15: Final Verification And Docs Update

Cutover note: this task documents the completed repoctl cutover and staged
autoloop cutover rather than changing automation behavior. Verification covers
`repoctl`, `autoloop` packages/CLI, docs tests, `cmd/gormes`,
language-shape scan, and whitespace checks.

**Files:**
- Modify: `docs/superpowers/specs/2026-04-24-autoloop-repoctl-go-port-design.md`
- Modify: `docs/superpowers/plans/2026-04-24-autoloop-repoctl-go-port.md`
- Modify: `scripts/orchestrator/README.md`
- Modify: `scripts/orchestrator/FROZEN.md`

- [ ] **Step 1: Update orchestrator docs**

In `scripts/orchestrator/README.md`, replace the opening paragraph with:

```markdown
# Autoloop Internals

The orchestrator wrapper and CLI implementation now live in Go under
`cmd/autoloop` and `internal/autoloop`. This directory contains transitional
wrappers, systemd templates, and historical notes for the old shell
entrypoints; full `autoloop run` runtime parity remains staged.
```

In `scripts/orchestrator/FROZEN.md`, add this freeze exception:

```markdown
## Active freeze exception

The user-approved Autoloop + Repoctl Go port may replace frozen shell files with
Go implementations, vendored parity fixtures, or tiny wrappers. Production
behavior changes still require parity tests.
```

- [ ] **Step 2: Run language-shape scan**

Run:

```bash
git ls-files '*.sh' '*.bash' '*.bats' | xargs -r wc -l | tail -1
git ls-files '*.sh' '*.bash' '*.bats'
```

Expected: repoctl/orchestrator entrypoints are wrappers or test harnesses, long legacy shell is under `testdata/legacy-shell/`, and the companion scripts remain live shell pending a later port.

- [ ] **Step 3: Run full targeted verification**

Run:

```bash
go test ./internal/repoctl ./cmd/repoctl ./internal/autoloop ./cmd/autoloop -count=1
go test ./docs -count=1
go test ./cmd/gormes -count=1
git diff --check
```

Expected: all commands pass.

- [ ] **Step 4: Commit docs and final polish**

Run:

```bash
git add docs/superpowers/specs/2026-04-24-autoloop-repoctl-go-port-design.md docs/superpowers/plans/2026-04-24-autoloop-repoctl-go-port.md scripts/orchestrator/README.md scripts/orchestrator/FROZEN.md
git commit -m "docs(autoloop): document Go automation cutover"
```

- [ ] **Step 5: Final status check**

Run:

```bash
git status -sb --untracked-files=normal
git log --oneline -5
```

Expected: branch contains the repoctl/autoloop port commits, with no untracked implementation artifacts.
