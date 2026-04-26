package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/builderloop"
	"github.com/TrebuchetDynamics/gormes-agent/internal/cmdrunner"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	deps := defaultDeps()
	err := run(context.Background(), deps, []string{"unknown"})
	if err == nil {
		t.Fatal("run() error = nil, want error")
	}
	if !errors.Is(err, errParse) {
		t.Fatalf("run() error = %v, want errors.Is(errParse) = true", err)
	}
	for _, want := range []string{
		"usage: builder-loop",
		"progress validate",
		"progress write",
		"repo benchmark record",
		"repo readme update",
		"audit",
		"service install",
		"service install-audit",
		"service disable legacy-timers",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run() error = %q, want %q", err, want)
		}
	}
}

func TestResolveRepoRootPrefersFlag(t *testing.T) {
	dir := t.TempDir()
	args, root, err := resolveRepoRoot([]string{"run", "--repo-root", dir, "--dry-run"})
	if err != nil {
		t.Fatalf("resolveRepoRoot() error = %v", err)
	}
	if root != dir {
		t.Fatalf("root = %q, want %q", root, dir)
	}
	if got, want := args, []string{"run", "--dry-run"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestResolveRepoRootFallsBackToEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REPO_ROOT", dir)
	args, root, err := resolveRepoRoot([]string{"run"})
	if err != nil {
		t.Fatalf("resolveRepoRoot() error = %v", err)
	}
	if root != dir {
		t.Fatalf("root = %q, want %q", root, dir)
	}
	if got, want := args, []string{"run"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
}

func TestResolveRepoRootMissingValue(t *testing.T) {
	if _, _, err := resolveRepoRoot([]string{"--repo-root"}); !errors.Is(err, errParse) {
		t.Fatalf("err = %v, want errParse", err)
	}
}

func TestWriteDigestOutputRefusesClobber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "digest.txt")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeDigestOutput(path, "new", false); err == nil {
		t.Fatal("writeDigestOutput() error = nil, want clobber refusal")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want 'already exists'", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "existing" {
		t.Fatalf("file overwritten unexpectedly: %q", got)
	}
}

func TestClassifyExitCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{name: "parse", err: fmt.Errorf("%w: nope", errParse), want: exitParseError},
		{name: "verify-failed", err: fmt.Errorf("wrap: %w", builderloop.ErrPostPromotionVerifyFailed), want: exitVerifyFailed},
		{name: "deadline", err: context.DeadlineExceeded, want: exitBackendTimeout},
		{name: "canceled", err: context.Canceled, want: exitBackendTimeout},
		{name: "other", err: errors.New("something else"), want: exitInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyExit(tc.err); got != tc.want {
				t.Fatalf("classifyExit(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

func TestParseFormatDefaultsToText(t *testing.T) {
	got, err := parseFormat(nil, "doctor")
	if err != nil {
		t.Fatalf("parseFormat(nil) error = %v", err)
	}
	if got != "text" {
		t.Fatalf("parseFormat(nil) = %q, want text", got)
	}
}

func TestParseFormatRejectsInvalid(t *testing.T) {
	if _, err := parseFormat([]string{"--format", "yaml"}, "doctor"); !errors.Is(err, errParse) {
		t.Fatalf("err = %v, want errParse for yaml", err)
	}
	if _, err := parseFormat([]string{"--format"}, "doctor"); !errors.Is(err, errParse) {
		t.Fatalf("err = %v, want errParse for missing value", err)
	}
}

func TestParseFormatAcceptsJSON(t *testing.T) {
	got, err := parseFormat([]string{"--format", "json"}, "doctor")
	if err != nil {
		t.Fatalf("parseFormat(json) error = %v", err)
	}
	if got != "json" {
		t.Fatalf("got = %q, want json", got)
	}
}

func TestLatestLedgerEventTimeFindsLatest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	body := strings.Join([]string{
		`{"ts":"2026-04-25T12:00:00Z","event":"run_started"}`,
		`{"ts":"2026-04-25T12:01:00Z","event":"health_updated"}`,
		`{"ts":"2026-04-25T12:05:00Z","event":"health_updated"}`,
		`{"ts":"2026-04-25T12:10:00Z","event":"run_started"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := latestLedgerEventTime(path, "health_updated")
	if err != nil {
		t.Fatalf("latestLedgerEventTime() error = %v", err)
	}
	want, _ := time.Parse(time.RFC3339, "2026-04-25T12:05:00Z")
	if !got.Equal(want) {
		t.Fatalf("got = %v, want %v", got, want)
	}
}

func TestLatestLedgerEventTimeMissingFile(t *testing.T) {
	_, err := latestLedgerEventTime(filepath.Join(t.TempDir(), "nope.jsonl"), "health_updated")
	if !os.IsNotExist(err) {
		t.Fatalf("err = %v, want os.IsNotExist", err)
	}
}

func TestStaleWorkerClaimWarningsFindsUnfinishedClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	if err := builderloop.AppendLedgerEvent(path, builderloop.LedgerEvent{
		TS:     now.Add(-2 * time.Hour),
		RunID:  "run-1",
		Event:  "worker_claimed",
		Worker: 1,
		Task:   "5/5.Q/Native TUI bundle independence check",
		Branch: "builder-loop/run-1/w1/native-tui",
		Status: "claimed",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}

	warnings, err := staleWorkerClaimWarnings(path, now, time.Hour)
	if err != nil {
		t.Fatalf("staleWorkerClaimWarnings() error = %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one stale claim warning", warnings)
	}
	for _, want := range []string{
		"doctor: warning: builder-loop worker claim stale for 2h0m0s",
		"run=run-1",
		"worker=1",
		"task=5/5.Q/Native TUI bundle independence check",
		"branch=builder-loop/run-1/w1/native-tui",
	} {
		if !strings.Contains(warnings[0], want) {
			t.Fatalf("warning = %q, want %q", warnings[0], want)
		}
	}
}

func TestStaleWorkerClaimWarningsIgnoresFinishedClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	claim := builderloop.LedgerEvent{
		TS:     now.Add(-2 * time.Hour),
		RunID:  "run-1",
		Event:  "worker_claimed",
		Worker: 1,
		Task:   "task",
		Branch: "branch",
		Status: "claimed",
	}
	if err := builderloop.AppendLedgerEvent(path, claim); err != nil {
		t.Fatalf("AppendLedgerEvent(claim) error = %v", err)
	}
	claim.TS = now.Add(-90 * time.Minute)
	claim.Event = "worker_failed"
	claim.Status = "backend_failed"
	if err := builderloop.AppendLedgerEvent(path, claim); err != nil {
		t.Fatalf("AppendLedgerEvent(worker_failed) error = %v", err)
	}

	warnings, err := staleWorkerClaimWarnings(path, now, time.Hour)
	if err != nil {
		t.Fatalf("staleWorkerClaimWarnings() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none for finished claim", warnings)
	}
}

func TestDriftWarningStaleEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	stale := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	body := fmt.Sprintf(`{"ts":%q,"event":"health_updated"}`+"\n", stale)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := driftWarning("builder-loop", path, "health_updated", time.Hour)
	if !strings.Contains(msg, "may be stalled") {
		t.Fatalf("driftWarning() = %q, want stall warning", msg)
	}
}

func TestDriftWarningFreshEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	fresh := time.Now().Add(-30 * time.Second).UTC().Format(time.RFC3339)
	body := fmt.Sprintf(`{"ts":%q,"event":"health_updated"}`+"\n", fresh)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if msg := driftWarning("builder-loop", path, "health_updated", time.Hour); msg != "" {
		t.Fatalf("driftWarning() = %q, want no warning", msg)
	}
}

func TestDriftWarningMissingLedger(t *testing.T) {
	if msg := driftWarning("builder-loop", filepath.Join(t.TempDir(), "absent.jsonl"), "health_updated", time.Hour); msg != "" {
		t.Fatalf("driftWarning() = %q, want empty (no history is not a stall)", msg)
	}
}

func TestWriteDigestOutputForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "digest.txt")
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeDigestOutput(path, "new", true); err != nil {
		t.Fatalf("writeDigestOutput(force=true) error = %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Fatalf("file = %q, want %q", got, "new")
	}
}

func TestProgressValidateValidatesCanonicalProgress(t *testing.T) {
	repoRoot := t.TempDir()
	writeMinimalProgressRepo(t, repoRoot)
	withTempCwd(t, repoRoot)

	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout

	if err := run(context.Background(), deps, []string{"progress", "validate"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "progress: validated 1 phases") {
		t.Fatalf("stdout = %q, want validation summary", stdout.String())
	}
}

func TestProgressWriteRegeneratesDocsAndSiteProgress(t *testing.T) {
	repoRoot := t.TempDir()
	writeMinimalProgressRepo(t, repoRoot)
	withTempCwd(t, repoRoot)

	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout

	if err := run(context.Background(), deps, []string{"progress", "write"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	indexRaw, err := os.ReadFile(filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "_index.md"))
	if err != nil {
		t.Fatalf("ReadFile(_index.md) error = %v", err)
	}
	if !strings.Contains(string(indexRaw), "Phase 1 — Test Phase") || !strings.Contains(string(indexRaw), "- [x] First shipped item") {
		t.Fatalf("_index.md was not regenerated with progress content:\n%s", indexRaw)
	}

	siteRaw, err := os.ReadFile(filepath.Join(repoRoot, "www.gormes.ai", "internal", "site", "data", "progress.json"))
	if err != nil {
		t.Fatalf("ReadFile(site progress) error = %v", err)
	}
	sourceRaw, err := os.ReadFile(filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json"))
	if err != nil {
		t.Fatalf("ReadFile(source progress) error = %v", err)
	}
	if string(siteRaw) != string(sourceRaw) {
		t.Fatalf("site progress mirror mismatch")
	}
	if !strings.Contains(stdout.String(), "progress: _index.md regenerated") {
		t.Fatalf("stdout = %q, want generation summary", stdout.String())
	}
}

func TestRepoBenchmarkRecordUpdatesBenchmarks(t *testing.T) {
	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "-c", "user.email=test@example.com", "-c", "user.name=Test User", "commit", "-m", "initial")

	bin := filepath.Join(repoRoot, "bin", "gormes")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, make([]byte, 1024*1024), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "benchmarks.json"), []byte(`{"binary":{"name":"gormes"},"history":[]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withTempCwd(t, repoRoot)

	deps := defaultDeps()
	if err := run(context.Background(), deps, []string{"repo", "benchmark", "record"}); err != nil {
		t.Fatalf("run() error = %v", err)
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
	raw, err := os.ReadFile(filepath.Join(repoRoot, "benchmarks.json"))
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

func TestRepoReadmeUpdateUpdatesReadme(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "benchmarks.json"), []byte(`{"binary":{"size_mb":"16.2"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(readme, []byte("Binary size: ~99.9 MB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withTempCwd(t, repoRoot)

	deps := defaultDeps()
	if err := run(context.Background(), deps, []string{"repo", "readme", "update"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	raw, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "~16.2 MB") {
		t.Fatalf("README not updated:\n%s", raw)
	}
}

func TestRunCommandDryRunPrintsSummary(t *testing.T) {
	repoRoot := t.TempDir()
	progressPath := filepath.Join(repoRoot, "progress.json")
	if err := os.WriteFile(progressPath, []byte(`{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{
								"item_name": "planned CLI candidate",
								"status": "planned",
								"priority": "P0",
								"contract": "CLI execution contract",
								"contract_status": "draft",
								"slice_size": "small",
								"execution_owner": "orchestrator",
								"ready_when": ["dry-run fixture exists"],
								"write_scope": ["cmd/builder-loop/"],
								"test_commands": ["go test ./cmd/builder-loop -count=1"],
								"done_signal": ["dry-run output names metadata"]
							}
						]
					}
				}
			}
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PROGRESS_JSON", progressPath)
	t.Setenv("RUN_ROOT", filepath.Join(repoRoot, "runs"))
	t.Setenv("BACKEND", "opencode")
	t.Setenv("MODE", "safe")
	t.Setenv("MAX_AGENTS", "1")
	t.Setenv("MAX_PHASE", "12")

	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"run", "--dry-run"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"candidates: 1",
		"selected: 1",
		"planned CLI candidate",
		"owner=orchestrator",
		"size=small",
		"reason=P0 handoff",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
}

func TestRunCommandDryRunExplainsMaxPhaseFilteredQueue(t *testing.T) {
	repoRoot := t.TempDir()
	progressPath := filepath.Join(repoRoot, "progress.json")
	if err := os.WriteFile(progressPath, []byte(`{
		"phases": {
			"5": {
				"subphases": {
					"5.Q": {
						"items": [
							{
								"item_name": "phase 5 candidate",
								"status": "planned",
								"contract": "phase 5 contract",
								"contract_status": "draft",
								"slice_size": "small",
								"execution_owner": "gateway",
								"ready_when": ["fixture exists"],
								"write_scope": ["internal/tui/"],
								"test_commands": ["go test ./internal/tui -count=1"],
								"done_signal": ["fixture proves behavior"]
							}
						]
					}
				}
			}
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PROGRESS_JSON", progressPath)
	t.Setenv("RUN_ROOT", filepath.Join(repoRoot, "runs"))
	t.Setenv("BACKEND", "opencode")
	t.Setenv("MODE", "safe")
	t.Setenv("MAX_AGENTS", "1")
	t.Setenv("MAX_PHASE", "3")

	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout
	withTempCwd(t, repoRoot)

	if err := run(context.Background(), deps, []string{"run", "--dry-run"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"candidates: 0",
		"selected: 0",
		"max_phase_filtered: 1",
		"next_max_phase: 5",
		"hint: rerun with MAX_PHASE=5 to include the next queued phase",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
}

func TestRunCommandDryRunDefaultIncludesPhaseFourQueue(t *testing.T) {
	repoRoot := t.TempDir()
	progressPath := filepath.Join(repoRoot, "progress.json")
	if err := os.WriteFile(progressPath, []byte(`{
		"phases": {
			"4": {
				"subphases": {
					"4.A": {
						"items": [
							{
								"item_name": "phase 4 candidate",
								"status": "planned",
								"contract": "phase 4 contract",
								"contract_status": "draft",
								"slice_size": "small",
								"execution_owner": "provider",
								"ready_when": ["fixture exists"],
								"write_scope": ["internal/provider/"],
								"test_commands": ["go test ./internal/provider -count=1"],
								"done_signal": ["fixture proves behavior"]
							}
						]
					}
				}
			}
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PROGRESS_JSON", progressPath)
	t.Setenv("RUN_ROOT", filepath.Join(repoRoot, "runs"))
	t.Setenv("BACKEND", "opencode")
	t.Setenv("MODE", "safe")
	t.Setenv("MAX_AGENTS", "1")
	t.Setenv("MAX_PHASE", "")

	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout
	withTempCwd(t, repoRoot)

	if err := run(context.Background(), deps, []string{"run", "--dry-run"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"candidates: 1",
		"selected: 1",
		"phase 4 candidate",
		"owner=provider",
		"size=small",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
}

func TestRunCommandBackendFlagSetsBackend(t *testing.T) {
	repoRoot := t.TempDir()
	progressPath := filepath.Join(repoRoot, "progress.json")
	if err := os.WriteFile(progressPath, []byte(`{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{"item_name": "backend flag candidate", "status": "planned", "contract": "backend flag contract", "contract_status": "draft"}
						]
					}
				}
			}
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PROGRESS_JSON", progressPath)
	t.Setenv("RUN_ROOT", filepath.Join(repoRoot, "runs"))
	t.Setenv("BACKEND", "codexu")
	t.Setenv("MODE", "safe")
	t.Setenv("MAX_AGENTS", "1")
	t.Setenv("MAX_PHASE", "12")

	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"run", "--dry-run", "--backend", "opencode"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "backend flag candidate") {
		t.Fatalf("stdout = %q, want dry-run summary with backend flag accepted", stdout.String())
	}
}

type fakeAutoloopRuntime struct {
	builderCalls     int
	plannerCalls     int
	checkpointCalls  int
	sleepCalls       int
	events           []string
	builderErr       error
	plannerErr       error
	checkpointErr    error
	cancelAfterSleep context.CancelFunc
}

func (f *fakeAutoloopRuntime) runtime() autoloopRuntime {
	return autoloopRuntime{
		runBuilder: func(_ context.Context, _ builderloop.Config, dryRun bool) (builderloop.RunSummary, error) {
			f.builderCalls++
			f.events = append(f.events, fmt.Sprintf("builder:%v", dryRun))
			if f.builderErr != nil {
				return builderloop.RunSummary{}, f.builderErr
			}
			return builderloop.RunSummary{
				Candidates: 1,
				Selected:   []builderloop.Candidate{{PhaseID: "1", SubphaseID: "1.A", ItemName: "loop candidate", Status: "planned"}},
			}, nil
		},
		runPlanner: func(_ context.Context) error {
			f.plannerCalls++
			f.events = append(f.events, "planner")
			return f.plannerErr
		},
		checkpoint: func(_ context.Context, _ builderloop.Config) error {
			f.checkpointCalls++
			f.events = append(f.events, "checkpoint")
			return f.checkpointErr
		},
		sleep: func(ctx context.Context, d time.Duration) error {
			f.sleepCalls++
			f.events = append(f.events, "sleep:"+d.String())
			if f.cancelAfterSleep != nil {
				f.cancelAfterSleep()
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		},
	}
}

func TestRunAutoloopLoopRunsPlannerAfterBuilder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeAutoloopRuntime{cancelAfterSleep: cancel}
	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout

	err := runAutoloopWithRuntime(ctx, deps, builderloop.Config{}, runOptions{loop: true}, time.Second, fake.runtime())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAutoloopWithRuntime() error = %v, want context.Canceled", err)
	}
	wantEvents := []string{"builder:false", "planner", "checkpoint", "sleep:1s"}
	if !reflect.DeepEqual(fake.events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", fake.events, wantEvents)
	}
	if fake.builderCalls != 1 || fake.plannerCalls != 1 || fake.checkpointCalls != 1 || fake.sleepCalls != 1 {
		t.Fatalf("calls builder=%d planner=%d checkpoint=%d sleep=%d, want 1/1/1/1", fake.builderCalls, fake.plannerCalls, fake.checkpointCalls, fake.sleepCalls)
	}
	if !strings.Contains(stdout.String(), "loop candidate") {
		t.Fatalf("stdout = %q, want builder summary", stdout.String())
	}
}

func TestRunAutoloopLoopContinuesOnPlannerFailure(t *testing.T) {
	wantErr := errors.New("planner failed")
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeAutoloopRuntime{plannerErr: wantErr, cancelAfterSleep: cancel}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout
	deps.stderr = &stderr

	err := runAutoloopWithRuntime(ctx, deps, builderloop.Config{}, runOptions{loop: true}, time.Second, fake.runtime())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAutoloopWithRuntime() error = %v, want context.Canceled", err)
	}
	wantEvents := []string{"builder:false", "planner", "checkpoint", "sleep:1s"}
	if !reflect.DeepEqual(fake.events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", fake.events, wantEvents)
	}
	if fake.checkpointCalls != 1 || fake.sleepCalls != 1 {
		t.Fatalf("checkpointCalls=%d sleepCalls=%d, want 1/1 after planner failure", fake.checkpointCalls, fake.sleepCalls)
	}
	if !strings.Contains(stderr.String(), "planner failed") {
		t.Fatalf("stderr = %q, want planner failure evidence", stderr.String())
	}
}

func TestRunAutoloopLoopRunsPlannerAfterBuilderFailure(t *testing.T) {
	wantErr := errors.New("builder failed")
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeAutoloopRuntime{builderErr: wantErr, cancelAfterSleep: cancel}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout
	deps.stderr = &stderr

	err := runAutoloopWithRuntime(ctx, deps, builderloop.Config{}, runOptions{loop: true}, time.Second, fake.runtime())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAutoloopWithRuntime() error = %v, want context.Canceled", err)
	}
	wantEvents := []string{"builder:false", "planner", "checkpoint", "sleep:1s"}
	if !reflect.DeepEqual(fake.events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", fake.events, wantEvents)
	}
	if fake.builderCalls != 1 || fake.plannerCalls != 1 || fake.checkpointCalls != 1 || fake.sleepCalls != 1 {
		t.Fatalf("calls builder=%d planner=%d checkpoint=%d sleep=%d, want 1/1/1/1", fake.builderCalls, fake.plannerCalls, fake.checkpointCalls, fake.sleepCalls)
	}
	if !strings.Contains(stderr.String(), "builder failed") {
		t.Fatalf("stderr = %q, want builder failure evidence", stderr.String())
	}
}

func TestRunAutoloopLoopContinuesOnCheckpointFailure(t *testing.T) {
	wantErr := errors.New("checkpoint failed")
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeAutoloopRuntime{checkpointErr: wantErr, cancelAfterSleep: cancel}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout
	deps.stderr = &stderr

	err := runAutoloopWithRuntime(ctx, deps, builderloop.Config{}, runOptions{loop: true}, time.Second, fake.runtime())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAutoloopWithRuntime() error = %v, want context.Canceled", err)
	}
	wantEvents := []string{"builder:false", "planner", "checkpoint", "sleep:1s"}
	if !reflect.DeepEqual(fake.events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", fake.events, wantEvents)
	}
	if !strings.Contains(stderr.String(), "checkpoint failed") {
		t.Fatalf("stderr = %q, want checkpoint failure evidence", stderr.String())
	}
}

func TestRunAutoloopOneShotDoesNotRunPlanner(t *testing.T) {
	fake := &fakeAutoloopRuntime{}
	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout

	if err := runAutoloopWithRuntime(context.Background(), deps, builderloop.Config{}, runOptions{}, time.Second, fake.runtime()); err != nil {
		t.Fatalf("runAutoloopWithRuntime() error = %v", err)
	}
	wantEvents := []string{"builder:false"}
	if !reflect.DeepEqual(fake.events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", fake.events, wantEvents)
	}
	if fake.plannerCalls != 0 {
		t.Fatalf("plannerCalls = %d, want 0 for one-shot", fake.plannerCalls)
	}
}

func TestDefaultAutoloopRuntimeRunsPlannerCommandInRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{}}}
	deps := defaultDeps()
	deps.runner = runner

	runtime := defaultAutoloopRuntime(deps, repoRoot)
	if err := runtime.runPlanner(context.Background()); err != nil {
		t.Fatalf("runPlanner() error = %v", err)
	}

	want := []cmdrunner.Command{{
		Name: "go",
		Args: []string{"run", "./cmd/planner-loop", "run"},
		Dir:  repoRoot,
	}}
	if !reflect.DeepEqual(runner.Commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, want)
	}
}

func TestDefaultAutoloopRuntimePlannerFailureIncludesCommand(t *testing.T) {
	repoRoot := t.TempDir()
	wantErr := errors.New("exit 1")
	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{Err: wantErr, Stderr: "planner stderr\n"}}}
	deps := defaultDeps()
	deps.runner = runner

	runtime := defaultAutoloopRuntime(deps, repoRoot)
	err := runtime.runPlanner(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("runPlanner() error = %v, want %v", err, wantErr)
	}
	for _, want := range []string{"go run ./cmd/planner-loop run", "planner stderr"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err, want)
		}
	}
}

func TestBuilderLoopSleepDefault(t *testing.T) {
	got, err := builderLoopSleep(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("builderLoopSleep(default) error = %v", err)
	}
	if got != 30*time.Second {
		t.Fatalf("builderLoopSleep(default) = %v, want 30s", got)
	}
}

func TestBuilderLoopSleepFromEnv(t *testing.T) {
	got, err := builderLoopSleep(func(key string) (string, bool) {
		if key == "BUILDER_LOOP_SLEEP" {
			return "2m", true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("builderLoopSleep(2m) error = %v", err)
	}
	if got != 2*time.Minute {
		t.Fatalf("builderLoopSleep(2m) = %v, want 2m", got)
	}
}

func TestBuilderLoopSleepRejectsInvalid(t *testing.T) {
	_, err := builderLoopSleep(func(key string) (string, bool) {
		if key == "BUILDER_LOOP_SLEEP" {
			return "soon", true
		}
		return "", false
	})
	if !errors.Is(err, errParse) {
		t.Fatalf("builderLoopSleep(invalid) error = %v, want errParse", err)
	}
}

func TestParseRunOptions_BackendFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "codexu", args: []string{"--backend", "codexu"}, want: "codexu"},
		{name: "claudeu", args: []string{"--backend", "claudeu"}, want: "claudeu"},
		{name: "opencode", args: []string{"--backend", "opencode"}, want: "opencode"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, err := parseRunOptions(tc.args)
			if err != nil {
				t.Fatalf("parseRunOptions(%v) error = %v", tc.args, err)
			}
			if opts.backend != tc.want {
				t.Fatalf("backend = %q, want %q", opts.backend, tc.want)
			}
		})
	}
}

func TestParseRunOptions_LoopFlag(t *testing.T) {
	opts, err := parseRunOptions([]string{"--loop"})
	if err != nil {
		t.Fatalf("parseRunOptions(--loop) error = %v", err)
	}
	if !opts.loop {
		t.Fatalf("loop = false, want true")
	}
}

func TestParseRunOptions_RejectsLoopDryRunCombination(t *testing.T) {
	_, err := parseRunOptions([]string{"--loop", "--dry-run"})
	if !errors.Is(err, errParse) {
		t.Fatalf("parseRunOptions(--loop --dry-run) error = %v, want errParse", err)
	}
	if err == nil || !strings.Contains(err.Error(), "--loop cannot be combined with --dry-run") {
		t.Fatalf("error = %v, want loop/dry-run message", err)
	}
}

func TestParseRunOptions_BackendFlagRequiresValue(t *testing.T) {
	if _, err := parseRunOptions([]string{"--backend"}); err == nil {
		t.Fatal("parseRunOptions(--backend with no value) error = nil, want error")
	}
}

func TestParseRunOptions_BackendFlagRejectsUnsupported(t *testing.T) {
	if _, err := parseRunOptions([]string{"--backend", "ollama"}); err == nil {
		t.Fatal("parseRunOptions(--backend ollama) error = nil, want error")
	}
}

func TestRunCommandHelpPrintsUsage(t *testing.T) {
	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout

	if err := run(context.Background(), deps, []string{"run", "--help"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "usage: builder-loop run") {
		t.Fatalf("stdout = %q, want scoped run usage", stdout.String())
	}
	// Per-subcommand help should NOT dump the full top-level surface.
	if strings.Contains(stdout.String(), "service disable legacy-timers") {
		t.Fatalf("stdout = %q, expected scoped help only", stdout.String())
	}
}

func TestSubcommandHelpPrintsScopedUsage(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "digest", args: []string{"digest", "--help"}, want: "usage: builder-loop digest"},
		{name: "audit", args: []string{"audit", "-h"}, want: "usage: builder-loop audit"},
		{name: "progress", args: []string{"progress", "--help"}, want: "usage: builder-loop progress"},
		{name: "progress validate", args: []string{"progress", "validate", "--help"}, want: "usage: builder-loop progress validate"},
		{name: "service install", args: []string{"service", "install", "--help"}, want: "usage: builder-loop service install"},
		{name: "service disable legacy-timers", args: []string{"service", "disable", "legacy-timers", "--help"}, want: "usage: builder-loop service disable legacy-timers"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			deps := defaultDeps()
			deps.stdout = &stdout
			withTempCwd(t, t.TempDir())

			if err := run(context.Background(), deps, tc.args); err != nil {
				t.Fatalf("run() error = %v", err)
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("stdout = %q, want substring %q", stdout.String(), tc.want)
			}
		})
	}
}

func TestConfigFromEnvFlowsThroughOSLookup(t *testing.T) {
	want := filepath.Join(t.TempDir(), "triggers.jsonl")
	t.Setenv("PLANNER_TRIGGERS_PATH", want)

	cfg, err := builderloop.ConfigFromEnv(t.TempDir(), os.LookupEnv)
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	if cfg.PlannerTriggersPath != want {
		t.Fatalf("PlannerTriggersPath = %q, want %q", cfg.PlannerTriggersPath, want)
	}
}

func TestDigestUsesConfiguredRunRoot(t *testing.T) {
	repoRoot := t.TempDir()
	runRoot := filepath.Join(repoRoot, "custom-runs")
	ledgerPath := filepath.Join(runRoot, "state", "runs.jsonl")
	if err := builderloop.AppendLedgerEvent(ledgerPath, builderloop.LedgerEvent{
		TS:    time.Now().UTC(),
		Event: "run_started",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}
	t.Setenv("RUN_ROOT", runRoot)
	t.Setenv("AUDIT_DIR", filepath.Join(repoRoot, "audit"))

	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"digest"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "runs: 1") {
		t.Fatalf("stdout = %q, want digest from configured RUN_ROOT", stdout.String())
	}
}

func TestDigestOutputWritesFile(t *testing.T) {
	repoRoot := t.TempDir()
	runRoot := filepath.Join(repoRoot, "custom-runs")
	ledgerPath := filepath.Join(runRoot, "state", "runs.jsonl")
	if err := builderloop.AppendLedgerEvent(ledgerPath, builderloop.LedgerEvent{
		TS:    time.Now().UTC(),
		Event: "run_started",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}
	t.Setenv("RUN_ROOT", runRoot)
	outputPath := filepath.Join(repoRoot, "digest.md")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	deps := defaultDeps()
	if err := run(context.Background(), deps, []string{"digest", "--output", outputPath}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(raw), "runs: 1") {
		t.Fatalf("digest output = %q, want digest", raw)
	}
}

func TestAuditUsesConfiguredRunRoot(t *testing.T) {
	repoRoot := t.TempDir()
	runRoot := filepath.Join(repoRoot, "custom-runs")
	ledgerPath := filepath.Join(runRoot, "state", "runs.jsonl")
	if err := builderloop.AppendLedgerEvent(ledgerPath, builderloop.LedgerEvent{
		TS:    time.Now().UTC(),
		Event: "run_started",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}
	t.Setenv("RUN_ROOT", runRoot)
	t.Setenv("AUDIT_DIR", filepath.Join(repoRoot, "audit"))

	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"audit"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "run_started=1") {
		t.Fatalf("stdout = %q, want audit digest from configured RUN_ROOT", stdout.String())
	}
}

func TestAuditCreatesReportArtifacts(t *testing.T) {
	repoRoot := t.TempDir()
	runRoot := filepath.Join(repoRoot, "custom-runs")
	auditDir := filepath.Join(repoRoot, "audit")
	ledgerPath := filepath.Join(runRoot, "state", "runs.jsonl")
	if err := builderloop.AppendLedgerEvent(ledgerPath, builderloop.LedgerEvent{
		TS:    time.Now().UTC(),
		Event: "run_started",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}
	t.Setenv("RUN_ROOT", runRoot)
	t.Setenv("AUDIT_DIR", auditDir)

	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"audit"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "gormes-orchestrator-audit @") {
		t.Fatalf("stdout = %q, want audit summary", stdout.String())
	}
	for _, name := range []string{"cursor", "report.ndjson", "report.csv"} {
		if _, err := os.Stat(filepath.Join(auditDir, name)); err != nil {
			t.Fatalf("Stat(%s) error = %v", name, err)
		}
	}
}

func TestServiceInstallWritesUnitUnderXDGConfigHome(t *testing.T) {
	repoRoot := t.TempDir()
	xdgConfigHome := filepath.Join(repoRoot, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	t.Setenv("FORCE", "")
	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{}, {}}}
	deps := defaultDeps()
	deps.runner = runner

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"service", "install"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	unitPath := filepath.Join(xdgConfigHome, "systemd", "user", "gormes-orchestrator.service")
	unit, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(unit), "WorkingDirectory="+repoRoot) {
		t.Fatalf("unit = %q, want workdir %q", unit, repoRoot)
	}
	wantExec := "ExecStart=" + filepath.Join(repoRoot, "scripts", "gormes-auto-codexu-orchestrator.sh") + " run --loop"
	if !strings.Contains(string(unit), wantExec) {
		t.Fatalf("unit = %q, want stable wrapper exec %q", unit, wantExec)
	}
	if strings.Contains(string(unit), "go-build") {
		t.Fatalf("unit = %q, want no temporary go-build path", unit)
	}

	wantCommands := []cmdrunner.Command{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
		{Name: "systemctl", Args: []string{"--user", "enable", "--now", "gormes-orchestrator.service"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestServiceInstallAuditUsesAuditUnitName(t *testing.T) {
	repoRoot := t.TempDir()
	xdgConfigHome := filepath.Join(repoRoot, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{}, {}}}
	deps := defaultDeps()
	deps.runner = runner

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"service", "install-audit"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(xdgConfigHome, "systemd", "user", "gormes-orchestrator-audit.service")); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(xdgConfigHome, "systemd", "user", "gormes-orchestrator-audit.timer")); err != nil {
		t.Fatalf("Stat(timer) error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(xdgConfigHome, "systemd", "user", "gormes-orchestrator-audit.service"))
	if err != nil {
		t.Fatalf("ReadFile(service) error = %v", err)
	}
	wantExec := "ExecStart=" + filepath.Join(repoRoot, "scripts", "orchestrator", "audit.sh")
	if !strings.Contains(string(raw), wantExec) {
		t.Fatalf("service unit = %q, want stable audit wrapper exec %q", raw, wantExec)
	}
	if strings.Contains(string(raw), "go-build") || strings.Contains(string(raw), " run") {
		t.Fatalf("service unit = %q, want no temporary go-build path and no run arg", raw)
	}
	wantEnable := cmdrunner.Command{Name: "systemctl", Args: []string{"--user", "enable", "--now", "gormes-orchestrator-audit.timer"}}
	if got := runner.Commands[len(runner.Commands)-1]; !reflect.DeepEqual(got, wantEnable) {
		t.Fatalf("last command = %#v, want %#v", got, wantEnable)
	}
}

func TestServiceInstallWatchdogUsesWatchdogUnitName(t *testing.T) {
	repoRoot := t.TempDir()
	xdgConfigHome := filepath.Join(repoRoot, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{}, {}}}
	deps := defaultDeps()
	deps.runner = runner

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"service", "install-watchdog"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	servicePath := filepath.Join(xdgConfigHome, "systemd", "user", "gormes-orchestrator-watchdog.service")
	timerPath := filepath.Join(xdgConfigHome, "systemd", "user", "gormes-orchestrator-watchdog.timer")
	if _, err := os.Stat(servicePath); err != nil {
		t.Fatalf("Stat(service) error = %v", err)
	}
	if _, err := os.Stat(timerPath); err != nil {
		t.Fatalf("Stat(timer) error = %v", err)
	}
	raw, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("ReadFile(service) error = %v", err)
	}
	wantExec := "ExecStart=" + filepath.Join(repoRoot, "scripts", "orchestrator", "watchdog.sh")
	if !strings.Contains(string(raw), wantExec) {
		t.Fatalf("service unit = %q, want stable watchdog wrapper exec %q", raw, wantExec)
	}
	if strings.Contains(string(raw), "go-build") || strings.Contains(string(raw), " run") {
		t.Fatalf("service unit = %q, want no temporary go-build path and no run arg", raw)
	}
	wantEnable := cmdrunner.Command{Name: "systemctl", Args: []string{"--user", "enable", "--now", "gormes-orchestrator-watchdog.timer"}}
	if got := runner.Commands[len(runner.Commands)-1]; !reflect.DeepEqual(got, wantEnable) {
		t.Fatalf("last command = %#v, want %#v", got, wantEnable)
	}
}

func TestServiceInstallHonorsAutoStartZero(t *testing.T) {
	repoRoot := t.TempDir()
	xdgConfigHome := filepath.Join(repoRoot, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	t.Setenv("AUTO_START", "0")
	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{}}}
	deps := defaultDeps()
	deps.runner = runner

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"service", "install"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	wantCommands := []cmdrunner.Command{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestServiceInstallAuditHonorsAutoStartZero(t *testing.T) {
	repoRoot := t.TempDir()
	xdgConfigHome := filepath.Join(repoRoot, "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	t.Setenv("AUTO_START", "0")
	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{}}}
	deps := defaultDeps()
	deps.runner = runner

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"service", "install-audit"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	wantCommands := []cmdrunner.Command{
		{Name: "systemctl", Args: []string{"--user", "daemon-reload"}},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestServiceDisableLegacyTimersUsesRunner(t *testing.T) {
	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{}, {}, {}, {}, {}, {}}}
	deps := defaultDeps()
	deps.runner = runner

	if err := run(context.Background(), deps, []string{"service", "disable", "legacy-timers"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	wantCommands := []cmdrunner.Command{
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architecture-planner-tasks-manager.timer"}},
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architectureplanneragent.timer"}},
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architecture-planner.timer"}},
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architecture-planner.path"}},
		{Name: "systemctl", Args: []string{"--user", "disable", "--now", "gormes-architecture-planner-impl.path"}},
	}
	if len(runner.Commands) != 6 || !reflect.DeepEqual(runner.Commands[:5], wantCommands) {
		t.Fatalf("commands = %#v, want systemd disables plus cron cleanup", runner.Commands)
	}
	cronCommand := runner.Commands[5]
	if cronCommand.Name != "sh" || len(cronCommand.Args) != 2 || !strings.Contains(cronCommand.Args[1], "landingpage-improver\\.sh") {
		t.Fatalf("cron cleanup command = %#v, want sh cleanup for legacy cron entries", cronCommand)
	}
}

func TestServiceInstallUsesHomeWhenXDGConfigHomeEmpty(t *testing.T) {
	repoRoot := t.TempDir()
	home := filepath.Join(repoRoot, "home")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	runner := &cmdrunner.FakeRunner{Results: []cmdrunner.Result{{}, {}}}
	deps := defaultDeps()
	deps.runner = runner

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"service", "install"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", "gormes-orchestrator.service")); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
}

func TestDigestIgnoresRunOnlyEnvValidation(t *testing.T) {
	repoRoot := t.TempDir()
	runRoot := filepath.Join(repoRoot, "custom-runs")
	ledgerPath := filepath.Join(runRoot, "state", "runs.jsonl")
	if err := builderloop.AppendLedgerEvent(ledgerPath, builderloop.LedgerEvent{
		TS:    time.Unix(1, 0).UTC(),
		Event: "run_started",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}
	t.Setenv("RUN_ROOT", runRoot)
	t.Setenv("MAX_AGENTS", "bad")

	var stdout bytes.Buffer
	deps := defaultDeps()
	deps.stdout = &stdout

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	if err := run(context.Background(), deps, []string{"digest"}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if !strings.Contains(stdout.String(), "runs: 1") {
		t.Fatalf("stdout = %q, want digest from configured RUN_ROOT", stdout.String())
	}
}

func writeMinimalProgressRepo(t *testing.T, root string) {
	t.Helper()

	progressPath := filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	progressJSON := `{
  "meta": {
    "version": "2.0",
    "last_updated": "2026-04-24"
  },
  "phases": {
    "1": {
      "name": "Phase 1 — Test Phase",
      "deliverable": "Test deliverable",
      "subphases": {
        "1.A": {
          "name": "Test Subphase",
          "items": [
            {"name": "First shipped item", "status": "complete"}
          ]
        }
      }
    }
  }
}
`
	if err := os.MkdirAll(filepath.Dir(progressPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(progressPath, []byte(progressJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	markers := map[string]string{
		"README.md": "readme-rollup",
		"docs/content/building-gormes/architecture_plan/_index.md":          "docs-full-checklist",
		"docs/content/building-gormes/contract-readiness.md":                "contract-readiness",
		"docs/content/building-gormes/builder-loop/builder-loop-handoff.md": "builder-loop-handoff",
		"docs/content/building-gormes/builder-loop/agent-queue.md":          "agent-queue",
		"docs/content/building-gormes/builder-loop/next-slices.md":          "next-slices",
		"docs/content/building-gormes/builder-loop/blocked-slices.md":       "blocked-slices",
		"docs/content/building-gormes/builder-loop/umbrella-cleanup.md":     "umbrella-cleanup",
		"docs/content/building-gormes/builder-loop/progress-schema.md":      "progress-schema",
	}
	for rel, kind := range markers {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "before\n<!-- PROGRESS:START kind=" + kind + " -->\nstale\n<!-- PROGRESS:END -->\nafter\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "www.gormes.ai", "internal", "site", "data"), 0o755); err != nil {
		t.Fatal(err)
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
