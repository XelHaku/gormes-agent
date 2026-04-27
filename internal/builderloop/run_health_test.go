package builderloop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

// writeNamedProgressJSON writes a progress.json that uses the canonical "name"
// item key (the format the live planner emits and the format internal/progress
// understands). The autoloop accumulator can only resolve rows in this format.
func writeNamedProgressJSON(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "progress.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// loadItem loads progress.json from path and returns the item identified by
// (phase, sub, name). Fails the test if the row is missing.
func loadItem(t *testing.T, path, phaseID, subID, itemName string) *progress.Item {
	t.Helper()

	prog, err := progress.Load(path)
	if err != nil {
		t.Fatalf("progress.Load: %v", err)
	}
	phase, ok := prog.Phases[phaseID]
	if !ok {
		t.Fatalf("phase %q not found", phaseID)
	}
	sub, ok := phase.Subphases[subID]
	if !ok {
		t.Fatalf("subphase %q not found", subID)
	}
	for i := range sub.Items {
		if sub.Items[i].Name == itemName {
			return &sub.Items[i]
		}
	}
	t.Fatalf("item %q not found in subphase %q", itemName, subID)
	return nil
}

const baseNamedProgress = `{
  "meta": {
    "version": "2.0",
    "last_updated": "2026-04-24",
    "links": {"github_readme": "", "landing_page": "", "docs_site": "", "source_code": ""}
  },
  "phases": {
    "12": {
      "name": "P12",
      "deliverable": "x",
      "subphases": {
        "12.A": {
          "name": "S",
          "items": [
            {"name": "row-1", "status": "planned", "contract": "do x", "contract_status": "draft"}
          ]
        }
      }
    }
  }
}
`

func TestRunOnce_QuarantineCarriesCurrentSpecHash(t *testing.T) {
	progressPath := writeNamedProgressJSON(t, baseNamedProgress)

	// Seed the row at ConsecutiveFailures=2 so one more failure trips the
	// quarantine threshold (default 3).
	if err := progress.ApplyHealthUpdates(progressPath, []progress.HealthUpdate{{
		PhaseID:    "12",
		SubphaseID: "12.A",
		ItemName:   "row-1",
		Mutate: func(h *progress.RowHealth) {
			h.ConsecutiveFailures = 2
			h.AttemptCount = 2
		},
	}}); err != nil {
		t.Fatalf("seed health: %v", err)
	}

	wantErr := errors.New("backend failed")
	runner := &FakeRunner{Results: []Result{{Err: wantErr, Stderr: "boom"}}}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:                t.TempDir(),
			ProgressJSON:            progressPath,
			RunRoot:                 t.TempDir(),
			Backend:                 "opencode",
			Mode:                    "safe",
			MaxAgents:               1,
			QuarantineThreshold:     3,
			BackendDegradeThreshold: 3,
		},
		Runner: runner,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunOnce() error = %v, want wrapped %v", err, wantErr)
	}

	item := loadItem(t, progressPath, "12", "12.A", "row-1")
	if item.Health == nil {
		t.Fatal("item.Health is nil after failed run")
	}
	if item.Health.ConsecutiveFailures != 3 {
		t.Fatalf("ConsecutiveFailures = %d, want 3", item.Health.ConsecutiveFailures)
	}
	if item.Health.Quarantine == nil {
		t.Fatal("expected Quarantine to be populated after threshold")
	}
	wantHash := progress.ItemSpecHash(item)
	if item.Health.Quarantine.SpecHash != wantHash {
		t.Fatalf("Quarantine.SpecHash = %q, want %q", item.Health.Quarantine.SpecHash, wantHash)
	}
	if item.Health.Quarantine.Threshold != 3 {
		t.Fatalf("Quarantine.Threshold = %d, want 3", item.Health.Quarantine.Threshold)
	}
}

func TestRunOnce_HealthUpdatedEventEmittedOnSuccess(t *testing.T) {
	progressPath := writeNamedProgressJSON(t, baseNamedProgress)
	runRoot := t.TempDir()
	runner := &FakeRunner{Results: []Result{{}}}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     t.TempDir(),
			ProgressJSON: progressPath,
			RunRoot:      runRoot,
			Backend:      "opencode",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	if !ledgerContainsEvent(events, "health_updated") {
		t.Fatalf("ledger did not contain health_updated event; got=%v", ledgerEventNames(events))
	}

	// Sanity: the row's LastSuccess should be set after a successful run.
	item := loadItem(t, progressPath, "12", "12.A", "row-1")
	if item.Health == nil || item.Health.LastSuccess == "" {
		t.Fatalf("item.Health.LastSuccess not set; got %+v", item.Health)
	}
}

func TestRunOnce_NoChangeWorkerCountsAsNoProgressAndSkipsPostVerify(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)

	progressPath := filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	if err := os.MkdirAll(filepath.Dir(progressPath), 0o755); err != nil {
		t.Fatalf("mkdir progress dir: %v", err)
	}
	if err := os.WriteFile(progressPath, []byte(`{
  "meta": {"version": "2.0", "last_updated": "2026-04-24",
    "links": {"github_readme": "", "landing_page": "", "docs_site": "", "source_code": ""}},
  "phases": {
    "12": {
      "name": "P12",
      "deliverable": "x",
      "subphases": {
        "12.A": {
          "name": "S",
          "items": [
            {
              "name": "stuck row",
              "status": "planned",
              "contract": "make a real code change",
              "contract_status": "draft",
              "write_scope": ["internal/goncho/"],
              "health": {"attempt_count": 2, "consecutive_failures": 2}
            }
          ]
        }
      }
    }
  }
}
`), 0o644); err != nil {
		t.Fatalf("write progress: %v", err)
	}
	runGitCommand(t, repoRoot, "add", ".")
	runGitCommand(t, repoRoot, "commit", "-m", "add progress")

	var postVerifyRuns int
	runner := runnerFunc(func(ctx context.Context, command Command) Result {
		switch command.Name {
		case "opencode":
			return Result{
				Stdout: strings.Repeat("backend progress noise\n", 80) + "worker stopped without edits api_key=sk-secret\n",
				Stderr: "no-change stderr clue token=secret-value\n",
			}
		case "sh":
			postVerifyRuns++
			return Result{}
		default:
			return (ExecRunner{}).Run(ctx, command)
		}
	})
	runRoot := t.TempDir()

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:                    repoRoot,
			ProgressJSON:                progressPath,
			RunRoot:                     runRoot,
			Backend:                     "opencode",
			Mode:                        "safe",
			MaxAgents:                   1,
			MaxPhase:                    12,
			PostPromotionVerifyCommands: []string{"go test ./... -count=1"},
			QuarantineThreshold:         3,
			BackendDegradeThreshold:     3,
		},
		Runner: runner,
		Now:    time.Date(2026, 4, 26, 7, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if postVerifyRuns != 0 {
		t.Fatalf("post-promotion verify commands ran %d times, want 0 for no-change worker", postVerifyRuns)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	if !ledgerContainsEvent(events, "worker_no_changes") {
		t.Fatalf("ledger missing worker_no_changes; got=%v", ledgerEventNames(events))
	}
	var noChanges *LedgerEvent
	for i := range events {
		if events[i].Event == "worker_no_changes" {
			noChanges = &events[i]
			break
		}
	}
	if noChanges == nil {
		t.Fatal("worker_no_changes event not found")
	}
	if !strings.Contains(noChanges.StdoutTail, "worker stopped without edits") {
		t.Fatalf("worker_no_changes stdout_tail missing backend evidence: %q", noChanges.StdoutTail)
	}
	if !strings.Contains(noChanges.StderrTail, "no-change stderr clue") {
		t.Fatalf("worker_no_changes stderr_tail missing backend evidence: %q", noChanges.StderrTail)
	}
	if strings.Contains(noChanges.StdoutTail, "sk-secret") || strings.Contains(noChanges.StderrTail, "secret-value") {
		t.Fatalf("worker_no_changes leaked secret output: stdout=%q stderr=%q", noChanges.StdoutTail, noChanges.StderrTail)
	}
	if noChanges.StdoutBytes == 0 || noChanges.StderrBytes == 0 {
		t.Fatalf("worker_no_changes missing byte counts: stdout=%d stderr=%d", noChanges.StdoutBytes, noChanges.StderrBytes)
	}
	if ledgerContainsEvent(events, "worker_success") {
		t.Fatalf("ledger contains worker_success for no-change worker; got=%v", ledgerEventNames(events))
	}
	if ledgerContainsEvent(events, "post_promotion_verify_started") {
		t.Fatalf("ledger contains post_promotion_verify_started for no-change worker; got=%v", ledgerEventNames(events))
	}
	if !ledgerContainsEvent(events, "run_completed") {
		t.Fatalf("ledger missing run_completed; got=%v", ledgerEventNames(events))
	}

	item := loadItem(t, progressPath, "12", "12.A", "stuck row")
	if item.Health == nil {
		t.Fatal("item.Health is nil after no-change run")
	}
	if item.Health.LastSuccess != "" {
		t.Fatalf("LastSuccess = %q, want empty for no-change worker", item.Health.LastSuccess)
	}
	if item.Health.ConsecutiveFailures != 3 {
		t.Fatalf("ConsecutiveFailures = %d, want 3", item.Health.ConsecutiveFailures)
	}
	if item.Health.LastFailure == nil || item.Health.LastFailure.Category != progress.FailureNoProgress {
		t.Fatalf("LastFailure = %+v, want no_progress category", item.Health.LastFailure)
	}
	if item.Health.Quarantine == nil {
		t.Fatal("Quarantine is nil, want no-progress row quarantined at threshold")
	}
	if item.Health.Quarantine.LastCategory != progress.FailureNoProgress {
		t.Fatalf("Quarantine.LastCategory = %q, want %q", item.Health.Quarantine.LastCategory, progress.FailureNoProgress)
	}
}

func TestRunOnce_PostPromotionVerifyFailureStopsBeforeRunHealth(t *testing.T) {
	progressPath := writeNamedProgressJSON(t, baseNamedProgress)
	runRoot := t.TempDir()
	verifyErr := errors.New("exit status 1")
	runner := &FakeRunner{Results: []Result{
		{},
		{Err: verifyErr, Stderr: "suite broke"},
	}}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:                    t.TempDir(),
			ProgressJSON:                progressPath,
			RunRoot:                     runRoot,
			Backend:                     "opencode",
			Mode:                        "safe",
			MaxAgents:                   1,
			MaxPhase:                    12,
			PostPromotionVerifyCommands: []string{"go test ./... -count=1"},
			PostPromotionRepairEnabled:  false,
			PostPromotionRepairAttempts: 0,
			QuarantineThreshold:         3,
			BackendDegradeThreshold:     3,
		},
		Runner: runner,
	})
	if !errors.Is(err, verifyErr) {
		t.Fatalf("RunOnce() error = %v, want wrapped %v", err, verifyErr)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	if !ledgerContainsEvent(events, "post_promotion_verify_failed") {
		t.Fatalf("ledger missing post_promotion_verify_failed; got=%v", ledgerEventNames(events))
	}
	if ledgerContainsEvent(events, "run_completed") {
		t.Fatalf("ledger should NOT contain run_completed after failed post-promotion verification; got=%v", ledgerEventNames(events))
	}
	if ledgerContainsEvent(events, "health_updated") {
		t.Fatalf("ledger should NOT contain health_updated after failed post-promotion verification; got=%v", ledgerEventNames(events))
	}

	item := loadItem(t, progressPath, "12", "12.A", "row-1")
	if item.Health != nil && item.Health.LastSuccess != "" {
		t.Fatalf("item.Health.LastSuccess = %q, want empty because health was not flushed", item.Health.LastSuccess)
	}
	if got, want := len(runner.Commands), 2; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
	if runner.Commands[1].Name != "sh" || !reflect.DeepEqual(runner.Commands[1].Args, []string{"-lc", "go test ./... -count=1"}) {
		t.Fatalf("verification command = %#v, want shell command", runner.Commands[1])
	}
}

func TestRunOnce_PostPromotionVerifyFailureRepairsBeforeRunHealth(t *testing.T) {
	progressPath := writeNamedProgressJSON(t, baseNamedProgress)
	runRoot := t.TempDir()
	verifyErr := errors.New("exit status 1")
	runner := &FakeRunner{Results: []Result{
		{},
		{Err: verifyErr, Stderr: "suite broke"},
		{},
		{},
	}}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:                    t.TempDir(),
			ProgressJSON:                progressPath,
			RunRoot:                     runRoot,
			Backend:                     "opencode",
			Mode:                        "safe",
			MaxAgents:                   1,
			MaxPhase:                    12,
			PostPromotionVerifyCommands: []string{"go test ./... -count=1"},
			PostPromotionRepairEnabled:  true,
			PostPromotionRepairAttempts: 1,
			QuarantineThreshold:         3,
			BackendDegradeThreshold:     3,
		},
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	got := ledgerEventPairsExcludingJobs(events)
	want := []string{
		"run_started:started",
		"worker_claimed:claimed",
		"worker_success:success",
		"post_promotion_verify_started:started",
		"post_promotion_verify_failed:failed",
		"post_promotion_repair_started:started",
		"post_promotion_repair_succeeded:ok",
		"post_promotion_verify_started:started",
		"post_promotion_verify_succeeded:ok",
		"run_completed:completed",
		"health_updated:ok",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ledger events = %#v, want %#v", got, want)
	}
	if _, ok := findJobFinished(events, "post_verify_command", "post_verify_failed"); !ok {
		t.Fatalf("ledger missing failed post_verify_command job_finished: %+v", events)
	}
	if _, ok := findJobFinished(events, "post_repair_backend", "ok"); !ok {
		t.Fatalf("ledger missing successful post_repair_backend job_finished: %+v", events)
	}
	if _, ok := findJobFinished(events, "post_verify_command", "ok"); !ok {
		t.Fatalf("ledger missing successful post_verify_command job_finished after repair: %+v", events)
	}

	item := loadItem(t, progressPath, "12", "12.A", "row-1")
	if item.Health == nil || item.Health.LastSuccess == "" {
		t.Fatalf("item.Health.LastSuccess not set after repaired verification; got %+v", item.Health)
	}
	if got, want := len(runner.Commands), 4; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
	if runner.Commands[2].Name != "opencode" {
		t.Fatalf("repair command = %#v, want opencode backend", runner.Commands[2])
	}
	if runner.Commands[3].Name != "sh" || !reflect.DeepEqual(runner.Commands[3].Args, []string{"-lc", "go test ./... -count=1"}) {
		t.Fatalf("second verification command = %#v, want shell command", runner.Commands[3])
	}
}

func TestRunOnce_CommitsRunHealthAfterPromotedWorker(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)

	progressPath := filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	if err := os.MkdirAll(filepath.Dir(progressPath), 0o755); err != nil {
		t.Fatalf("mkdir progress dir: %v", err)
	}
	if err := os.WriteFile(progressPath, []byte(`{
  "meta": {
    "version": "2.0",
    "last_updated": "2026-04-24",
    "links": {"github_readme": "", "landing_page": "", "docs_site": "", "source_code": ""}
  },
  "phases": {
    "3": {
      "name": "P3",
      "deliverable": "memory",
      "subphases": {
        "3.F": {
          "name": "Goncho",
          "items": [
            {
              "name": "health clean row",
              "status": "planned",
              "contract": "land worker and health",
              "contract_status": "draft",
              "write_scope": ["internal/goncho/"]
            }
          ]
        }
      }
    }
  }
}
`), 0o644); err != nil {
		t.Fatalf("write progress: %v", err)
	}
	runGitCommand(t, repoRoot, "add", ".")
	runGitCommand(t, repoRoot, "commit", "-m", "add progress")

	runner := runnerFunc(func(ctx context.Context, command Command) Result {
		switch command.Name {
		case "opencode":
			path := filepath.Join(command.Dir, "internal", "goncho", "health_clean.go")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("package goncho\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if result := (ExecRunner{}).Run(ctx, Command{Name: "git", Args: []string{"add", "."}, Dir: command.Dir}); result.Err != nil {
				t.Fatalf("git add: %v\n%s", result.Err, result.Stderr)
			}
			if result := (ExecRunner{}).Run(ctx, Command{Name: "git", Args: []string{"commit", "-m", "worker change"}, Dir: command.Dir}); result.Err != nil {
				t.Fatalf("git commit: %v\n%s", result.Err, result.Stderr)
			}
			return Result{}
		case "git":
			if len(command.Args) > 0 && command.Args[0] == "push" {
				return Result{Err: errors.New("offline push")}
			}
			return (ExecRunner{}).Run(ctx, command)
		default:
			return Result{}
		}
	})

	runRoot := t.TempDir()
	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     repoRoot,
			ProgressJSON: progressPath,
			RunRoot:      runRoot,
			Backend:      "opencode",
			Mode:         "safe",
			MaxAgents:    1,
			MaxPhase:     3,
		},
		Runner: runner,
		Now:    time.Date(2026, 4, 25, 4, 56, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if status := gitStatusPorcelain(t, repoRoot); status != "" {
		t.Fatalf("base repo status = %q, want clean after worker promotion and health update", status)
	}
	item := loadItem(t, progressPath, "3", "3.F", "health clean row")
	if item.Health == nil || item.Health.LastSuccess == "" {
		t.Fatalf("item.Health.LastSuccess not set after run; got %+v", item.Health)
	}
}

func TestRunOnce_PushesMainAfterCompletedRun(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)

	progressPath := filepath.Join(repoRoot, "docs", "content", "building-gormes", "architecture_plan", "progress.json")
	if err := os.MkdirAll(filepath.Dir(progressPath), 0o755); err != nil {
		t.Fatalf("mkdir progress dir: %v", err)
	}
	if err := os.WriteFile(progressPath, []byte(`{
  "meta": {
    "version": "2.0",
    "last_updated": "2026-04-24",
    "links": {"github_readme": "", "landing_page": "", "docs_site": "", "source_code": ""}
  },
  "phases": {
    "3": {
      "name": "P3",
      "deliverable": "memory",
      "subphases": {
        "3.F": {
          "name": "Goncho",
          "items": [
            {
              "name": "push clean row",
              "status": "planned",
              "contract": "land worker and push main",
              "contract_status": "draft",
              "write_scope": ["internal/goncho/"]
            }
          ]
        }
      }
    }
  }
}
`), 0o644); err != nil {
		t.Fatalf("write progress: %v", err)
	}
	runGitCommand(t, repoRoot, "add", ".")
	runGitCommand(t, repoRoot, "commit", "-m", "add progress")

	var mainPushes []Command
	runner := runnerFunc(func(ctx context.Context, command Command) Result {
		switch command.Name {
		case "opencode":
			path := filepath.Join(command.Dir, "internal", "goncho", "push_clean.go")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("package goncho\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if result := (ExecRunner{}).Run(ctx, Command{Name: "git", Args: []string{"add", "."}, Dir: command.Dir}); result.Err != nil {
				t.Fatalf("git add: %v\n%s", result.Err, result.Stderr)
			}
			if result := (ExecRunner{}).Run(ctx, Command{Name: "git", Args: []string{"commit", "-m", "worker change"}, Dir: command.Dir}); result.Err != nil {
				t.Fatalf("git commit: %v\n%s", result.Err, result.Stderr)
			}
			return Result{}
		case "git":
			if reflect.DeepEqual(command.Args, []string{"push", "origin", "HEAD:main"}) {
				mainPushes = append(mainPushes, command)
				return Result{}
			}
			return (ExecRunner{}).Run(ctx, command)
		default:
			return Result{}
		}
	})

	runRoot := t.TempDir()
	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:           repoRoot,
			ProgressJSON:       progressPath,
			RunRoot:            runRoot,
			Backend:            "opencode",
			Mode:               "safe",
			MaxAgents:          1,
			MaxPhase:           3,
			PromotionMode:      "cherry-pick",
			PushMainOnComplete: true,
		},
		Runner: runner,
		Now:    time.Date(2026, 4, 25, 5, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(mainPushes) != 1 {
		t.Fatalf("main pushes = %#v, want one git push origin HEAD:main", mainPushes)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	if _, ok := findLedgerEvent(events, "main_push_completed", "ok"); !ok {
		t.Fatalf("ledger missing main_push_completed: %+v", events)
	}
}

func TestRunOnce_PreflightFailureSoftSkipsAndContinues(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)
	progressPath := writeNamedProgressJSON(t, `{
  "meta": {"version": "2.0", "last_updated": "2026-04-24",
    "links": {"github_readme": "", "landing_page": "", "docs_site": "", "source_code": ""}},
  "phases": {
    "12": {
      "name": "P12", "deliverable": "x",
      "subphases": {
        "12.A": {
          "name": "SA",
          "items": [
            {"name": "row-skip", "status": "planned", "contract": "do x", "contract_status": "draft"}
          ]
        },
        "12.B": {
          "name": "SB",
          "items": [
            {"name": "row-ok", "status": "planned", "contract": "do y", "contract_status": "draft", "write_scope": ["soft-skip-success.txt"]}
          ]
        }
      }
    }
  }
}
`)

	runRoot := t.TempDir()
	now := time.Date(2026, 4, 25, 1, 40, 0, 0, time.UTC)
	runID := "20260425T014000Z"

	// Block the first worker's worktree path by pre-creating a regular file
	// where git wants to put a directory. mkdir of the parent works, but
	// `git worktree add` will fail because the target is a non-empty file.
	worktreesParent := filepath.Join(runRoot, "worktrees", runID)
	if err := os.MkdirAll(worktreesParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreesParent, "w1"), []byte("blocker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := runnerFunc(func(ctx context.Context, command Command) Result {
		if command.Name == "opencode" {
			if err := os.WriteFile(filepath.Join(command.Dir, "soft-skip-success.txt"), []byte("ok\n"), 0o644); err != nil {
				t.Fatalf("write worker output: %v", err)
			}
			runGitCommand(t, command.Dir, "add", "soft-skip-success.txt")
			runGitCommand(t, command.Dir, "commit", "-m", "worker success after soft skip")
		}
		return Result{}
	})

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:                repoRoot,
			ProgressJSON:            progressPath,
			RunRoot:                 runRoot,
			Backend:                 "opencode",
			Mode:                    "safe",
			MaxAgents:               2,
			QuarantineThreshold:     3,
			BackendDegradeThreshold: 3,
		},
		Runner: runner,
		Now:    now,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v, want nil (soft-skip should not fail run)", err)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	names := ledgerEventNames(events)
	if !ledgerContainsEvent(events, "candidate_skipped") {
		t.Fatalf("ledger missing candidate_skipped; got=%v", names)
	}
	if !ledgerContainsEvent(events, "worker_success") {
		t.Fatalf("ledger missing worker_success for the second candidate; got=%v", names)
	}
	if !ledgerContainsEvent(events, "run_completed") {
		t.Fatalf("ledger missing run_completed; got=%v", names)
	}

	// The skipped row should have its failure counted so future runs can
	// quarantine on repeat failures even when the row never reached a worker.
	skipped := loadItem(t, progressPath, "12", "12.A", "row-skip")
	if skipped.Health == nil || skipped.Health.ConsecutiveFailures != 1 {
		t.Fatalf("skip row health = %+v, want ConsecutiveFailures=1", skipped.Health)
	}
}

func TestRunOnce_BackendFallbackEmptyMeansNoOp(t *testing.T) {
	progressPath := writeNamedProgressJSON(t, baseNamedProgress)
	runRoot := t.TempDir()
	wantErr := errors.New("backend failed")
	runner := &FakeRunner{Results: []Result{{Err: wantErr}}}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:                t.TempDir(),
			ProgressJSON:            progressPath,
			RunRoot:                 runRoot,
			Backend:                 "opencode",
			Mode:                    "safe",
			MaxAgents:               1,
			BackendFallback:         nil, // empty chain: degrader is a no-op
			BackendDegradeThreshold: 1,   // would normally trigger immediately
		},
		Runner: runner,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunOnce() error = %v, want wrapped %v", err, wantErr)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	if ledgerContainsEvent(events, "backend_degraded") {
		t.Fatalf("backend_degraded should NOT appear with empty fallback; got=%v", ledgerEventNames(events))
	}
}

func TestRunOnce_BackendDegradedEventEmittedAfterThreshold(t *testing.T) {
	progressPath := writeNamedProgressJSON(t, baseNamedProgress)
	runRoot := t.TempDir()
	wantErr := errors.New("backend failed")
	runner := &FakeRunner{Results: []Result{{Err: wantErr, Stderr: "boom"}}}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:                t.TempDir(),
			ProgressJSON:            progressPath,
			RunRoot:                 runRoot,
			Backend:                 "opencode",
			Mode:                    "safe",
			MaxAgents:               1,
			BackendFallback:         []string{"opencode", "codexu"},
			BackendDegradeThreshold: 1, // single failure crosses threshold
		},
		Runner: runner,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunOnce() error = %v, want wrapped %v", err, wantErr)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var degraded *LedgerEvent
	for i := range events {
		if events[i].Event == "backend_degraded" {
			degraded = &events[i]
			break
		}
	}
	if degraded == nil {
		t.Fatalf("backend_degraded event missing; got=%v", ledgerEventNames(events))
	}
	if !strings.Contains(degraded.Detail, "from=opencode") || !strings.Contains(degraded.Detail, "to=codexu") {
		t.Fatalf("backend_degraded detail = %q, want from/to fields", degraded.Detail)
	}
}

func TestRunOnce_BackendUsageLimitDoesNotQuarantineRow(t *testing.T) {
	progressPath := writeNamedProgressJSON(t, baseNamedProgress)
	if err := progress.ApplyHealthUpdates(progressPath, []progress.HealthUpdate{{
		PhaseID:    "12",
		SubphaseID: "12.A",
		ItemName:   "row-1",
		Mutate: func(h *progress.RowHealth) {
			h.AttemptCount = 2
			h.ConsecutiveFailures = 2
		},
	}}); err != nil {
		t.Fatalf("seed health: %v", err)
	}

	runRoot := t.TempDir()
	wantErr := errors.New("exit status 1")
	usageLimit := "You've hit your usage limit. Reading additional input from stdin...\n"
	runner := &FakeRunner{Results: []Result{{Err: wantErr, Stdout: usageLimit}}}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:                t.TempDir(),
			ProgressJSON:            progressPath,
			RunRoot:                 runRoot,
			Backend:                 "opencode",
			Mode:                    "safe",
			MaxAgents:               1,
			QuarantineThreshold:     3,
			BackendFallback:         []string{"opencode", "codexu"},
			BackendDegradeThreshold: 1,
		},
		Runner: runner,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunOnce() error = %v, want wrapped %v", err, wantErr)
	}

	item := loadItem(t, progressPath, "12", "12.A", "row-1")
	if item.Health == nil {
		t.Fatal("item.Health is nil after seeded run")
	}
	if item.Health.AttemptCount != 2 {
		t.Fatalf("AttemptCount = %d, want unchanged 2", item.Health.AttemptCount)
	}
	if item.Health.ConsecutiveFailures != 2 {
		t.Fatalf("ConsecutiveFailures = %d, want unchanged 2", item.Health.ConsecutiveFailures)
	}
	if item.Health.Quarantine != nil {
		t.Fatalf("Quarantine = %+v, want nil for backend usage-limit outage", item.Health.Quarantine)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var failed *LedgerEvent
	var degraded *LedgerEvent
	for i := range events {
		switch events[i].Event {
		case "worker_failed":
			failed = &events[i]
		case "backend_degraded":
			degraded = &events[i]
		}
	}
	if failed == nil {
		t.Fatalf("worker_failed event missing; got=%v", ledgerEventNames(events))
	}
	if failed.Status != "backend_usage_limited" {
		t.Fatalf("worker_failed status = %q, want backend_usage_limited", failed.Status)
	}
	if !strings.Contains(failed.Detail, "You've hit your usage limit") {
		t.Fatalf("worker_failed detail = %q, want usage-limit detail", failed.Detail)
	}
	if degraded == nil {
		t.Fatalf("backend_degraded event missing; got=%v", ledgerEventNames(events))
	}
	if !strings.Contains(degraded.Detail, "from=opencode") || !strings.Contains(degraded.Detail, "to=codexu") {
		t.Fatalf("backend_degraded detail = %q, want from/to fields", degraded.Detail)
	}
}

func ledgerEventNames(events []LedgerEvent) []string {
	names := make([]string, 0, len(events))
	for _, ev := range events {
		names = append(names, ev.Event)
	}
	return names
}

func ledgerContainsEvent(events []LedgerEvent, name string) bool {
	for _, ev := range events {
		if ev.Event == name {
			return true
		}
	}
	return false
}

// TestRunOnce_HealthUpdateFailedEventOnFlushError verifies the spec contract
// that RunOnce must (a) emit a "health_update_failed" ledger event AND
// (b) propagate the flush error back to the caller when the run-end Flush
// fails. The failure is induced by chmod'ing the progress.json parent
// directory to read-only after the runner completes its worker invocation,
// so atomicWrite (CreateTemp + Rename) inside SaveProgress fails.
func TestRunOnce_HealthUpdateFailedEventOnFlushError(t *testing.T) {
	progressPath := writeNamedProgressJSON(t, baseNamedProgress)
	progressDir := filepath.Dir(progressPath)
	runRoot := t.TempDir()

	// Restore writability so t.TempDir's RemoveAll cleanup can succeed.
	t.Cleanup(func() {
		_ = os.Chmod(progressDir, 0o755)
	})

	// chmodRunner records a successful worker invocation, then locks the
	// progress.json parent directory before returning so the run-end Flush
	// fails on the next atomicWrite. The initial NormalizeCandidates load
	// has already happened by the time Run is called.
	runner := runnerFunc(func(_ context.Context, _ Command) Result {
		if err := os.Chmod(progressDir, 0o555); err != nil {
			t.Fatalf("chmod progress dir read-only: %v", err)
		}
		return Result{}
	})

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:                t.TempDir(),
			ProgressJSON:            progressPath,
			RunRoot:                 runRoot,
			Backend:                 "opencode",
			Mode:                    "safe",
			MaxAgents:               1,
			QuarantineThreshold:     3,
			BackendDegradeThreshold: 3,
		},
		Runner: runner,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want flush error")
	}
	if !strings.Contains(err.Error(), "flush health") {
		t.Fatalf("RunOnce() error = %q, want wrapped flush health error", err)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	if !ledgerContainsEvent(events, "health_update_failed") {
		t.Fatalf("ledger missing health_update_failed; got=%v", ledgerEventNames(events))
	}
	if ledgerContainsEvent(events, "health_updated") {
		t.Fatalf("ledger should NOT contain health_updated when flush failed; got=%v", ledgerEventNames(events))
	}
}
