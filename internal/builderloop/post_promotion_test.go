package builderloop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTruncateLedgerDetail_PreservesTail verifies that long detail strings
// keep the END (where `go test ./...` prints --- FAIL summaries) instead of
// the head (which is just a list of passing packages). The pre-improvement
// truncation kept the first 2000 bytes and dropped the actual failure.
func TestTruncateLedgerDetail_PreservesTail(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("ok  \tgithub.com/Trebuchet/package/")
		b.WriteString(strings.Repeat("p", 12))
		b.WriteString("\t0.05s\n")
	}
	b.WriteString("--- FAIL: TestThatActuallyBroke (0.04s)\n")
	b.WriteString("    foo_test.go:42: expected 1, got 2\n")
	b.WriteString("FAIL\nFAIL\tgithub.com/Trebuchet/package/foo\t0.04s\n")
	b.WriteString("FAIL\n")
	full := b.String()
	if len(full) <= 2000 {
		t.Fatalf("synth input too short to exercise truncation (%d bytes)", len(full))
	}

	got := truncateLedgerDetail(full)
	if !strings.Contains(got, "--- FAIL: TestThatActuallyBroke") {
		t.Fatalf("truncated detail missing FAIL marker (tail must be preserved):\n%s", got)
	}
	if !strings.Contains(got, "FAIL\tgithub.com/Trebuchet/package/foo") {
		t.Fatalf("truncated detail missing FAIL summary line:\n%s", got)
	}
	if !strings.Contains(got, "bytes elided") {
		t.Fatalf("truncated detail missing elision marker:\n%s", got)
	}
	if len(got) > 2200 {
		t.Fatalf("truncated detail too long: %d bytes", len(got))
	}
}

func TestTruncateLedgerDetail_ShortStringPassesThrough(t *testing.T) {
	short := "command failed: exit 1"
	if got := truncateLedgerDetail(short); got != short {
		t.Fatalf("short string was truncated: got %q, want %q", got, short)
	}
}

func TestPostPromotionCommandEnvDisablesCompanionsForVerification(t *testing.T) {
	cfg := Config{
		ProgressJSON: "/repo/progress.json",
		RunRoot:      "/repo/.codex/builder-loop",
		Backend:      "codexu",
		Mode:         "full",
		MaxAgents:    8,
		MaxPhase:     4,
	}

	env := postPromotionCommandEnv(cfg)

	for _, want := range []string{
		"REPO_ROOT=",
		"DISABLE_COMPANIONS=1",
		"COMPANION_ON_IDLE=1",
		"COMPANION_PLANNER_CMD=:",
		"COMPANION_DOC_IMPROVER_CMD=:",
		"COMPANION_LANDINGPAGE_CMD=:",
		"INTEGRATION_BRANCH=",
		"FAIL_FAST_ON_WORKER_FAILURE=",
		"PAUSE_ON_RUN_FAILURE=",
		"SKIP_COMPANIONS_ON_RUN_FAILURE=",
		"PHASE_FLOOR=",
		"PHASE_PRIORITY_BOOST=",
		"PHASE_SKIP_SUBPHASES=",
		"MAX_RETRIES=",
		"CANDIDATE_LOW_WATERMARK=",
		"MIN_MEM_PER_WORKER_MB=",
	} {
		var found bool
		for _, got := range env {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("postPromotionCommandEnv() = %#v, want %q", env, want)
		}
	}
}

func TestPostPromotionVerificationOverridesInheritedCompanionEnv(t *testing.T) {
	t.Setenv("DISABLE_COMPANIONS", "0")
	dir := t.TempDir()
	cfg := Config{
		RepoRoot:                    dir,
		RunRoot:                     filepath.Join(dir, "runroot"),
		PostPromotionVerifyCommands: []string{`test "$DISABLE_COMPANIONS" = 1`},
	}

	err := runPostPromotionVerification(context.Background(), cfg, ExecRunner{}, "env-run", 1)
	if err != nil {
		t.Fatalf("verification should force DISABLE_COMPANIONS=1 over inherited env: %v", err)
	}
}

func TestRunPostPromotionVerificationLogsCommandJobs(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot: dir,
		RunRoot:  filepath.Join(dir, "runroot"),
		PostPromotionVerifyCommands: []string{
			"go test ./internal/hermes -count=1",
			"go run ./cmd/builder-loop progress validate",
		},
	}
	runner := runnerFunc(func(_ context.Context, _ Command) Result {
		return Result{Stdout: "ok\n"}
	})

	if err := runPostPromotionVerification(context.Background(), cfg, runner, "run-verify", 2); err != nil {
		t.Fatalf("runPostPromotionVerification() error = %v", err)
	}

	events := readLedgerEvents(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	var finished []LedgerEvent
	for _, event := range events {
		if event.Event == "job_finished" {
			finished = append(finished, event)
		}
	}
	if len(finished) != 2 {
		t.Fatalf("job_finished events = %d, want 2\nall events: %+v", len(finished), events)
	}
	for i, event := range finished {
		if event.JobKind != "post_verify_command" {
			t.Fatalf("event %d JobKind = %q, want post_verify_command", i, event.JobKind)
		}
		if event.Attempt != 2 {
			t.Fatalf("event %d Attempt = %d, want 2", i, event.Attempt)
		}
		if event.Command != cfg.PostPromotionVerifyCommands[i] {
			t.Fatalf("event %d Command = %q, want %q", i, event.Command, cfg.PostPromotionVerifyCommands[i])
		}
		if event.Status != "ok" {
			t.Fatalf("event %d Status = %q, want ok", i, event.Status)
		}
	}
}

// TestRunPostPromotionVerification_RunsAllCommandsAndCollectsFailures
// verifies that the verify gate does NOT abort on the first failed command.
// Recent ledger evidence shows verify aborts at command 1/5, so the operator
// (and the repair agent) never see whether commands 2-5 had additional
// problems. With the all-commands fix, a single verify failure event
// reports every broken command in one detail.
func TestRunPostPromotionVerification_RunsAllCommandsAndCollectsFailures(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot: dir,
		RunRoot:  filepath.Join(dir, "runroot"),
		PostPromotionVerifyCommands: []string{
			"true",
			"false",
			"true",
			"false",
			"true",
		},
	}

	calls := 0
	runner := runnerFunc(func(_ context.Context, cmd Command) Result {
		calls++
		// Simulate the shell-c failure for `false` commands.
		if strings.Contains(strings.Join(cmd.Args, " "), "false") {
			return Result{Err: errors.New("exit status 1"), Stderr: "boom"}
		}
		return Result{}
	})

	err := runPostPromotionVerification(context.Background(), cfg, runner, "test-run", 1)
	if err == nil {
		t.Fatal("expected verification error when 2 commands fail")
	}
	if calls != 5 {
		t.Fatalf("runner called %d times, want 5 (all commands must run)", calls)
	}

	// Read the ledger; assert ONE post_promotion_verify_failed event with a
	// detail that names BOTH failed commands.
	ledgerPath := filepath.Join(cfg.RunRoot, "state", "runs.jsonl")
	body := readLedgerLines(t, ledgerPath)
	var failures int
	var failedDetail string
	for _, line := range body {
		if strings.Contains(line, "post_promotion_verify_failed") {
			failures++
			failedDetail = line
		}
	}
	if failures != 1 {
		t.Fatalf("expected exactly 1 failure ledger event, got %d", failures)
	}
	if !strings.Contains(failedDetail, "command=2/5") {
		t.Fatalf("failure detail missing command 2/5 reference:\n%s", failedDetail)
	}
	if !strings.Contains(failedDetail, "command=4/5") {
		t.Fatalf("failure detail missing command 4/5 reference (all-failures collected):\n%s", failedDetail)
	}
}

// TestRunPrePromotionVerify_DisabledByDefaultIsNoop confirms that an empty
// PrePromotionVerifyCommands does not run any commands and emits no ledger
// events. This preserves the existing post-promotion-only behavior for
// installs that have not opted in.
func TestRunPrePromotionVerify_DisabledByDefaultIsNoop(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot: dir,
		RunRoot:  filepath.Join(dir, "runroot"),
	}
	calls := 0
	runner := runnerFunc(func(_ context.Context, _ Command) Result {
		calls++
		return Result{}
	})
	worker := workerRun{ID: 1, Task: "phase/sub/item", Branch: "autoloop/test/w1", RepoRoot: filepath.Join(dir, "worktree-1")}

	if err := runPrePromotionVerify(context.Background(), cfg, runner, "run-A", worker, 1); err != nil {
		t.Fatalf("disabled gate should not error: %v", err)
	}
	if calls != 0 {
		t.Fatalf("disabled gate must not run commands; got %d calls", calls)
	}
	if got := readLedgerLines(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl")); len(got) != 0 {
		t.Fatalf("disabled gate must not emit ledger events; got %d:\n%s", len(got), strings.Join(got, "\n"))
	}
}

// TestRunPrePromotionVerify_RunsInWorkerWorktree checks that the verify
// commands' Dir is set to worker.RepoRoot, NOT cfg.RepoRoot. This is the
// load-bearing distinction that keeps main from being briefly broken.
func TestRunPrePromotionVerify_RunsInWorkerWorktree(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot:                   filepath.Join(dir, "main"),
		RunRoot:                    filepath.Join(dir, "runroot"),
		PrePromotionVerifyCommands: []string{"echo ok"},
	}
	worktreePath := filepath.Join(dir, "worktree-1")
	worker := workerRun{ID: 1, Task: "phase/sub/item", Branch: "autoloop/test/w1", RepoRoot: worktreePath}

	var seenDir string
	runner := runnerFunc(func(_ context.Context, cmd Command) Result {
		seenDir = cmd.Dir
		return Result{}
	})

	if err := runPrePromotionVerify(context.Background(), cfg, runner, "run-A", worker, 1); err != nil {
		t.Fatalf("verify should pass: %v", err)
	}
	if seenDir != worktreePath {
		t.Fatalf("Command.Dir = %q, want worker.RepoRoot %q (gate must run in worktree, not main)", seenDir, worktreePath)
	}
	events := readLedgerEvents(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	job, ok := findJobFinished(events, "pre_verify_command", "ok")
	if !ok {
		t.Fatalf("ledger missing pre_verify_command job_finished: %+v", events)
	}
	if job.Worker != worker.ID || job.Task != worker.Task || job.Branch != worker.Branch || job.Dir != worktreePath {
		t.Fatalf("pre-verify job identity = %+v, want worker/task/branch/dir", job)
	}
}

// TestRunPrePromotionVerify_FailureEmitsWorkerFailedAndPreventsPromotion is
// the headline behavior: a verify failure aborts the worker as a
// pre_promotion_verify_failed worker_failed event AND runs every command so
// the operator sees the full failure set in one ledger entry. The caller
// (finishWorker) bails on the returned error before promoteWorkerCommit.
func TestRunPrePromotionVerify_FailureEmitsWorkerFailedAndPreventsPromotion(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot:                   filepath.Join(dir, "main"),
		RunRoot:                    filepath.Join(dir, "runroot"),
		PrePromotionVerifyCommands: []string{"true", "false", "false"},
	}
	worker := workerRun{ID: 7, Task: "2/2.B/test-row", Branch: "autoloop/run-A/w7", RepoRoot: filepath.Join(dir, "worktree-7")}

	calls := 0
	runner := runnerFunc(func(_ context.Context, cmd Command) Result {
		calls++
		if strings.Contains(strings.Join(cmd.Args, " "), "false") {
			return Result{Err: errors.New("exit status 1"), Stderr: "boom"}
		}
		return Result{}
	})

	err := runPrePromotionVerify(context.Background(), cfg, runner, "run-A", worker, 1)
	if err == nil {
		t.Fatal("verify failure must propagate as error")
	}
	if calls != 3 {
		t.Fatalf("all commands must run regardless of order; got %d calls, want 3", calls)
	}

	body := readLedgerLines(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	var startedSeen, failedSeen bool
	var failedDetail string
	for _, line := range body {
		if strings.Contains(line, `"event":"pre_promotion_verify_started"`) {
			startedSeen = true
		}
		if strings.Contains(line, `"event":"worker_failed"`) && strings.Contains(line, `"status":"pre_promotion_verify_failed"`) {
			failedSeen = true
			failedDetail = line
		}
	}
	if !startedSeen {
		t.Errorf("pre_promotion_verify_started event missing")
	}
	if !failedSeen {
		t.Fatalf("worker_failed/pre_promotion_verify_failed event missing:\n%s", strings.Join(body, "\n"))
	}
	for _, want := range []string{`"worker":7`, `"task":"2/2.B/test-row"`, "command=2/3", "command=3/3"} {
		if !strings.Contains(failedDetail, want) {
			t.Errorf("worker_failed event missing %q\n%s", want, failedDetail)
		}
	}
}

// TestRunPrePromotionGate_RepairFixesFailingVerify exercises the
// verify→repair→verify orchestration. The runner closure simulates an LLM
// repair: the first verify fails, the backend command "fixes" things by
// flipping a counter so subsequent verify commands pass.
func TestRunPrePromotionGate_RepairFixesFailingVerify(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot:                   filepath.Join(dir, "main"),
		RunRoot:                    filepath.Join(dir, "runroot"),
		Backend:                    "codexu",
		Mode:                       "safe",
		PrePromotionVerifyCommands: []string{"go test ./..."},
		PrePromotionRepairEnabled:  true,
		PrePromotionRepairAttempts: 1,
	}
	worktreePath := filepath.Join(dir, "worktree-1")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatal(err)
	}
	worker := workerRun{ID: 4, Task: "2/2.B/test", Branch: "autoloop/test/w4", RepoRoot: worktreePath}

	repairRan := false
	verifyCalls := 0
	runner := runnerFunc(func(_ context.Context, cmd Command) Result {
		// The backend repair command is the codexu invocation; the verify
		// commands run via "sh -lc ...". Distinguish by Name.
		if cmd.Name == "sh" {
			verifyCalls++
			// First verify fails; verifies after repair pass.
			if !repairRan {
				return Result{Err: errors.New("exit status 1"), Stderr: "boom"}
			}
			return Result{}
		}
		// Backend repair invocation. Mark repair as run.
		repairRan = true
		return Result{}
	})

	if err := runPrePromotionGate(context.Background(), cfg, runner, "run-A", worker); err != nil {
		t.Fatalf("gate should pass after repair: %v", err)
	}
	if !repairRan {
		t.Fatal("repair was never invoked despite verify failure")
	}
	if verifyCalls != 2 {
		t.Fatalf("expected 2 verify calls (initial + post-repair), got %d", verifyCalls)
	}

	body := readLedgerLines(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	wantEvents := []string{
		`"event":"pre_promotion_verify_started"`,
		`"event":"worker_failed"`,
		`"status":"pre_promotion_verify_failed"`,
		`"event":"pre_promotion_repair_started"`,
		`"event":"pre_promotion_repair_succeeded"`,
		`"event":"pre_promotion_verify_succeeded"`,
	}
	for _, want := range wantEvents {
		var found bool
		for _, line := range body {
			if strings.Contains(line, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ledger missing %q\nfull ledger:\n%s", want, strings.Join(body, "\n"))
		}
	}
	events := readLedgerEvents(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if _, ok := findJobFinished(events, "pre_verify_command", "pre_verify_failed"); !ok {
		t.Fatalf("ledger missing failed pre_verify_command job_finished: %+v", events)
	}
	if _, ok := findJobFinished(events, "pre_repair_backend", "ok"); !ok {
		t.Fatalf("ledger missing successful pre_repair_backend job_finished: %+v", events)
	}
	if _, ok := findJobFinished(events, "pre_verify_command", "ok"); !ok {
		t.Fatalf("ledger missing successful pre_verify_command job_finished after repair: %+v", events)
	}
}

// TestRunPrePromotionGate_RepairDisabledShortCircuits verifies that when
// the operator opts out of repair (PRE_PROMOTION_REPAIR=0), a verify
// failure is terminal — no repair is invoked and the gate returns the
// original verify error.
func TestRunPrePromotionGate_RepairDisabledShortCircuits(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot:                   filepath.Join(dir, "main"),
		RunRoot:                    filepath.Join(dir, "runroot"),
		PrePromotionVerifyCommands: []string{"false"},
		PrePromotionRepairEnabled:  false,
		PrePromotionRepairAttempts: 1,
	}
	worker := workerRun{ID: 1, Task: "phase/sub/item", Branch: "br", RepoRoot: filepath.Join(dir, "wt")}

	backendInvoked := false
	runner := runnerFunc(func(_ context.Context, cmd Command) Result {
		if cmd.Name != "sh" {
			backendInvoked = true
		}
		return Result{Err: errors.New("exit status 1")}
	})

	err := runPrePromotionGate(context.Background(), cfg, runner, "run-A", worker)
	if err == nil {
		t.Fatal("expected verify failure to propagate when repair disabled")
	}
	if backendInvoked {
		t.Fatal("backend should NOT be invoked when PrePromotionRepairEnabled=false")
	}
}

// TestRunPrePromotionGate_RepairAttemptsExhausted verifies the loop bound:
// when verify keeps failing across attempts, the gate eventually gives up
// with the final verify error (and still no main-side modifications).
func TestRunPrePromotionGate_RepairAttemptsExhausted(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot:                   filepath.Join(dir, "main"),
		RunRoot:                    filepath.Join(dir, "runroot"),
		Backend:                    "codexu",
		Mode:                       "safe",
		PrePromotionVerifyCommands: []string{"false"},
		PrePromotionRepairEnabled:  true,
		PrePromotionRepairAttempts: 2,
	}
	worktreePath := filepath.Join(dir, "wt")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatal(err)
	}
	worker := workerRun{ID: 2, Task: "task", Branch: "br", RepoRoot: worktreePath}

	repairCalls := 0
	verifyCalls := 0
	runner := runnerFunc(func(_ context.Context, cmd Command) Result {
		if cmd.Name == "sh" {
			verifyCalls++
			return Result{Err: errors.New("exit status 1"), Stderr: "still broken"}
		}
		repairCalls++
		return Result{} // repair always claims success but verify keeps failing
	})

	if err := runPrePromotionGate(context.Background(), cfg, runner, "run-A", worker); err == nil {
		t.Fatal("expected gate to fail after exhausting repair attempts")
	}
	if repairCalls != 2 {
		t.Fatalf("expected 2 repair attempts, got %d", repairCalls)
	}
	if verifyCalls != 3 {
		t.Fatalf("expected 3 verify calls (1 initial + 2 post-repair), got %d", verifyCalls)
	}
}

func TestRunRowEvaluator_RunsOnlyTestCommands(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot: filepath.Join(dir, "main"),
		RunRoot:  filepath.Join(dir, "runroot"),
	}
	worker := workerRun{
		ID:       5,
		Task:     "4/4.A/replay",
		Branch:   "autoloop/test/w5",
		RepoRoot: filepath.Join(dir, "worktree-5"),
		Candidate: Candidate{
			TestCommands: []string{"echo row-test"},
			Acceptance:   []string{"reasoning replay is visible in the transcript"},
		},
	}

	var commands []Command
	runner := runnerFunc(func(_ context.Context, cmd Command) Result {
		commands = append(commands, cmd)
		return Result{}
	})

	if err := runRowEvaluator(context.Background(), cfg, runner, "run-A", worker); err != nil {
		t.Fatalf("row evaluator should pass: %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("runner called %d times, want 1 test command only", len(commands))
	}
	gotCommand := strings.Join(commands[0].Args, " ")
	if !strings.Contains(gotCommand, "echo row-test") {
		t.Fatalf("runner command = %q, want row test command", gotCommand)
	}
	if strings.Contains(gotCommand, "reasoning replay") {
		t.Fatalf("runner command includes acceptance prose as shell: %q", gotCommand)
	}
	if commands[0].Dir != worker.RepoRoot {
		t.Fatalf("row evaluator Dir = %q, want worker repo %q", commands[0].Dir, worker.RepoRoot)
	}

	body := readLedgerLines(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	var started, succeeded bool
	for _, line := range body {
		if strings.Contains(line, `"event":"row_evaluation_started"`) && strings.Contains(line, "commands=1") {
			started = true
		}
		if strings.Contains(line, `"event":"row_evaluation_succeeded"`) && strings.Contains(line, "commands=1") {
			succeeded = true
		}
	}
	if !started || !succeeded {
		t.Fatalf("row evaluator ledger missing start/success commands=1:\n%s", strings.Join(body, "\n"))
	}
	events := readLedgerEvents(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	job, ok := findJobFinished(events, "row_eval_command", "ok")
	if !ok {
		t.Fatalf("ledger missing row_eval_command job_finished: %+v", events)
	}
	if job.Worker != worker.ID || job.Task != worker.Task || job.Branch != worker.Branch || job.Command != "echo row-test" {
		t.Fatalf("row evaluation job identity = %+v, want worker/task/branch/command", job)
	}
}

func TestFinishWorkerUsesPostRepairHeadForPromotion(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)
	baseCommit, err := gitHeadSha(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	baseBranch := mustGitCurrentBranch(t, repoRoot)
	workerDir := filepath.Join(t.TempDir(), "worker")
	workerBranch := "autoloop/test/w6"
	runGitCommand(t, repoRoot, "worktree", "add", "-b", workerBranch, workerDir)

	writeAndCommitTestFile(t, workerDir, "allowed/initial.txt", "initial\n", "initial worker change")
	initialCommit, err := gitHeadSha(workerDir)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	cfg := Config{
		RepoRoot:                   repoRoot,
		RunRoot:                    filepath.Join(dir, "runroot"),
		Backend:                    "opencode",
		Mode:                       "safe",
		PrePromotionVerifyCommands: []string{"go test ./..."},
		PrePromotionRepairEnabled:  true,
		PrePromotionRepairAttempts: 1,
	}
	worker := workerRun{
		ID:           6,
		Task:         "4/4.A/replay",
		Branch:       workerBranch,
		RepoRoot:     workerDir,
		WorktreePath: workerDir,
		BaseCommit:   baseCommit,
		Candidate: Candidate{
			WriteScope: []string{"allowed/"},
		},
	}

	verifyCalls := 0
	var promotedCommit string
	var repairedCommit string
	runner := runnerFunc(func(_ context.Context, cmd Command) Result {
		switch cmd.Name {
		case "sh":
			verifyCalls++
			if verifyCalls == 1 {
				return Result{Err: errors.New("exit status 1"), Stderr: "first verify failed"}
			}
			return Result{}
		case "opencode":
			writeAndCommitTestFile(t, cmd.Dir, "allowed/repair.txt", "repair\n", "repair worker change")
			var err error
			repairedCommit, err = gitHeadSha(cmd.Dir)
			if err != nil {
				t.Fatal(err)
			}
			return Result{}
		case "git":
			if len(cmd.Args) >= 3 && cmd.Args[0] == "cherry-pick" {
				promotedCommit = cmd.Args[len(cmd.Args)-1]
				return Result{}
			}
			return Result{}
		case "gh":
			return Result{}
		default:
			return Result{Err: ErrUnexpectedCommand}
		}
	})

	if err := finishWorker(context.Background(), cfg, runner, "opencode", "run-A", baseBranch, true, worker); err != nil {
		t.Fatalf("finishWorker should pass: %v", err)
	}
	if repairedCommit == initialCommit {
		t.Fatal("test setup failed: repair did not advance worker head")
	}
	if promotedCommit != repairedCommit {
		t.Fatalf("promoted commit = %s, want repaired head %s (initial was %s)", promotedCommit, repairedCommit, initialCommit)
	}
}

func TestFinishWorkerRechecksWriteScopeAfterPrePromotionRepair(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)
	baseCommit, err := gitHeadSha(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	baseBranch := mustGitCurrentBranch(t, repoRoot)
	workerDir := filepath.Join(t.TempDir(), "worker")
	workerBranch := "autoloop/test/w7"
	runGitCommand(t, repoRoot, "worktree", "add", "-b", workerBranch, workerDir)

	writeAndCommitTestFile(t, workerDir, "allowed/initial.txt", "initial\n", "initial worker change")

	dir := t.TempDir()
	cfg := Config{
		RepoRoot:                   repoRoot,
		RunRoot:                    filepath.Join(dir, "runroot"),
		Backend:                    "opencode",
		Mode:                       "safe",
		PrePromotionVerifyCommands: []string{"go test ./..."},
		PrePromotionRepairEnabled:  true,
		PrePromotionRepairAttempts: 1,
	}
	worker := workerRun{
		ID:           7,
		Task:         "4/4.A/replay",
		Branch:       workerBranch,
		RepoRoot:     workerDir,
		WorktreePath: workerDir,
		BaseCommit:   baseCommit,
		Candidate: Candidate{
			WriteScope: []string{"allowed/"},
		},
	}

	verifyCalls := 0
	promoteCalls := 0
	runner := runnerFunc(func(_ context.Context, cmd Command) Result {
		switch cmd.Name {
		case "sh":
			verifyCalls++
			if verifyCalls == 1 {
				return Result{Err: errors.New("exit status 1"), Stderr: "first verify failed"}
			}
			return Result{}
		case "opencode":
			writeAndCommitTestFile(t, cmd.Dir, "internal/memory/interrupted_sync_test.go", "package memory\n", "repair outside row scope")
			return Result{}
		case "git":
			if len(cmd.Args) >= 3 && cmd.Args[0] == "cherry-pick" {
				promoteCalls++
				return Result{}
			}
			return Result{}
		case "gh":
			return Result{}
		default:
			return Result{Err: ErrUnexpectedCommand}
		}
	})

	err = finishWorker(context.Background(), cfg, runner, "opencode", "run-A", baseBranch, true, worker)
	if err == nil {
		t.Fatal("finishWorker error = nil, want write scope violation after repair")
	}
	if !strings.Contains(err.Error(), "internal/memory/interrupted_sync_test.go") {
		t.Fatalf("finishWorker error = %q, want repaired out-of-scope path", err)
	}
	if promoteCalls != 0 {
		t.Fatalf("promote called %d times, want 0 after repair scope violation", promoteCalls)
	}

	events := readLedgerEvents(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	var failed LedgerEvent
	for _, event := range events {
		if event.Event == "worker_failed" && event.Status == "write_scope_violation" {
			failed = event
			break
		}
	}
	if failed.Event == "" {
		t.Fatalf("missing worker_failed/write_scope_violation event:\n%#v", events)
	}
	if !strings.Contains(failed.Detail, "internal/memory/interrupted_sync_test.go") {
		t.Fatalf("scope violation detail = %q, want repaired path", failed.Detail)
	}
}

func TestBuildPrePromotionRepairPrompt_NamesBranchAndCommands(t *testing.T) {
	worker := workerRun{Branch: "autoloop/run-A/w4", Task: "2/2.B/sample"}
	prompt := BuildPrePromotionRepairPrompt(
		[]string{"go test ./...", "go vet ./..."},
		worker,
		errors.New("simulated verify failure"),
	)
	for _, want := range []string{
		"autoloop/run-A/w4",
		"2/2.B/sample",
		"go test ./...",
		"go vet ./...",
		"simulated verify failure",
		"NOT yet on main",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q\n%s", want, prompt)
		}
	}
}

func TestRunPostPromotionVerification_AllCommandsPassEmitsSuccess(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		RepoRoot:                    dir,
		RunRoot:                     filepath.Join(dir, "runroot"),
		PostPromotionVerifyCommands: []string{"true", "true"},
	}
	runner := runnerFunc(func(_ context.Context, _ Command) Result { return Result{} })

	if err := runPostPromotionVerification(context.Background(), cfg, runner, "ok-run", 1); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	ledgerPath := filepath.Join(cfg.RunRoot, "state", "runs.jsonl")
	body := readLedgerLines(t, ledgerPath)
	var succeeded bool
	for _, line := range body {
		if strings.Contains(line, "post_promotion_verify_succeeded") {
			succeeded = true
		}
	}
	if !succeeded {
		t.Fatalf("expected post_promotion_verify_succeeded event in ledger:\n%s", strings.Join(body, "\n"))
	}
}

func writeAndCommitTestFile(t *testing.T, repoRoot string, rel string, body string, message string) {
	t.Helper()

	path := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, repoRoot, "add", rel)
	runGitCommand(t, repoRoot, "commit", "-m", message)
}

// readLedgerLines reads runs.jsonl as one line per event, returning a
// slice. Returns nil on missing file (treats as empty).
func readLedgerLines(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read ledger %s: %v", path, err)
	}
	if len(body) == 0 {
		return nil
	}
	return strings.Split(strings.TrimRight(string(body), "\n"), "\n")
}
