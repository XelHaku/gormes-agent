package plannerloop

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/cmdrunner"
	"github.com/TrebuchetDynamics/gormes-agent/internal/plannertriggers"
)

func TestRunDryRunCollectsContextWithoutBackend(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	runner := &cmdrunner.FakeRunner{}

	summary, err := RunOnce(context.Background(), RunOptions{
		Config: mustConfig(t, repoRoot),
		Runner: runner,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if summary.Backend != "codexu" {
		t.Fatalf("Backend = %q, want codexu", summary.Backend)
	}
	if len(summary.SourceRoots) != 8 {
		t.Fatalf("SourceRoots length = %d, want 8: %#v", len(summary.SourceRoots), summary.SourceRoots)
	}
	if summary.ProgressItems != 2 {
		t.Fatalf("ProgressItems = %d, want 2", summary.ProgressItems)
	}
	if len(runner.Commands) != 0 {
		t.Fatalf("Commands length = %d, want 0", len(runner.Commands))
	}
	if _, err := os.Stat(filepath.Join(summary.RunRoot, "context.json")); err != nil {
		t.Fatalf("context.json missing after dry-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(summary.RunRoot, "latest_prompt.txt")); err != nil {
		t.Fatalf("latest_prompt.txt missing after dry-run: %v", err)
	}
}

func TestRunOnceRejectsConcurrentRunInSameRunRoot(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	lock, err := acquirePlannerRunLock(cfg.RunRoot, time.Now().UTC())
	if err != nil {
		t.Fatalf("acquirePlannerRunLock() error = %v", err)
	}
	defer lock.release()

	_, err = RunOnce(context.Background(), RunOptions{
		Config: cfg,
		Runner: &cmdrunner.FakeRunner{},
		DryRun: true,
	})
	if !errors.Is(err, ErrPlannerRunInProgress) {
		t.Fatalf("RunOnce() error = %v, want ErrPlannerRunInProgress", err)
	}
}

func TestRunOnceSendsPlannerPromptToBackendAndWritesArtifacts(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{
			{Stdout: "Already up to date.\n"},
			{Stdout: "Updating abc123..def456\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: `{"type":"thread.started","thread_id":"thread-arch-1"}` + "\n"},
		},
	}

	summary, err := RunOnce(context.Background(), RunOptions{
		Config:         mustConfig(t, repoRoot),
		Runner:         runner,
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if got, want := len(runner.Commands), 4; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
	command := runner.Commands[3]
	if command.Name != "codexu" {
		t.Fatalf("Command.Name = %q, want codexu", command.Name)
	}
	prompt := command.Args[len(command.Args)-1]
	for _, want := range []string{
		"Gormes Architecture Planner Loop",
		"hermes-agent",
		"gbrain",
		"upstream-hermes",
		"upstream-gbrain",
		"building-gormes",
		"www.gormes.ai",
		"Hugo docs",
		"landing page",
		"docs/hugo.toml",
		"goncho",
		"progress.json",
		"only long-term prompt agent",
		"Sync results:",
		"gbrain: pull",
		"Updating abc123..def456",
		"Synchronize progress.json with the current Gormes implementation",
		"Synchronize landing page, Hugo docs, generated pages, and progress.json",
		"Autoloop workers should not have to search or guess",
		"source_refs",
		"write_scope",
		"test_commands",
		"ready_when",
		"not_ready_when",
		"done_signal",
		"acceptance",
		"Do not implement runtime feature code",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if _, err := os.Stat(filepath.Join(summary.RunRoot, "latest_planner_report.md")); err != nil {
		t.Fatalf("latest_planner_report.md missing: %v", err)
	}

	stateData, err := os.ReadFile(filepath.Join(summary.RunRoot, "planner_state.json"))
	if err != nil {
		t.Fatalf("ReadFile(planner_state.json) error = %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatalf("planner_state.json parse error = %v", err)
	}
	if state["backend"] != "codexu" {
		t.Fatalf("state backend = %#v, want codexu", state["backend"])
	}
	contextData, err := os.ReadFile(filepath.Join(summary.RunRoot, "context.json"))
	if err != nil {
		t.Fatalf("ReadFile(context.json) error = %v", err)
	}
	for _, want := range []string{
		`"sync_results"`,
		`"output": "Updating abc123..def456"`,
		`"implementation_inventory"`,
		`"landing_site"`,
		`"hugo_docs"`,
	} {
		if !strings.Contains(string(contextData), want) {
			t.Fatalf("context.json missing %q:\n%s", want, contextData)
		}
	}
}

func TestRunOnceClearsStaleRawReportBeforeBackend(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	if err := os.MkdirAll(cfg.RunRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(run root) error = %v", err)
	}
	rawReportPath := filepath.Join(cfg.RunRoot, "latest_planner_report.raw.md")
	if err := os.WriteFile(rawReportPath, []byte("stale backend report\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale raw report) error = %v", err)
	}
	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "fresh planner stdout\n"},
		},
	}

	summary, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	reportRaw, err := os.ReadFile(summary.ReportPath)
	if err != nil {
		t.Fatalf("ReadFile(report) error = %v", err)
	}
	report := string(reportRaw)
	if strings.Contains(report, "stale backend report") {
		t.Fatalf("report reused stale raw backend output:\n%s", report)
	}
	if !strings.Contains(report, "fresh planner stdout") {
		t.Fatalf("report missing fresh backend stdout:\n%s", report)
	}
}

func TestRunOnceMergesOpenPullRequestsBeforeSyncAndPlannerPrompt(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	cfg := mustConfig(t, repoRoot)
	cfg.MergeOpenPullRequests = true
	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{
			{Stdout: "git@github.com:TrebuchetDynamics/gormes-agent.git\n"},
			{Stdout: `[{"number": 11, "title": "planner fix", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "planner/fix"}]`},
			{},
			{},
			{},
			{},
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "planner ran ok\n"},
		},
	}

	if _, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	}); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	wantPrefix := []cmdrunner.Command{
		{Name: "git", Args: []string{"remote", "get-url", "origin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "list", "--repo", "TrebuchetDynamics/gormes-agent", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "11", "--repo", "TrebuchetDynamics/gormes-agent", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "git", Args: []string{"fetch", "origin", "main"}, Dir: repoRoot},
		{Name: "git", Args: []string{"merge", "--ff-only", "FETCH_HEAD"}, Dir: repoRoot},
	}
	if got := runner.Commands[:5]; !reflect.DeepEqual(got, wantPrefix) {
		t.Fatalf("command prefix = %#v, want %#v", got, wantPrefix)
	}
}

func TestRunOnceConsumesPlannerTriggersAndRecordsEventLedger(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	trigger := plannertriggers.TriggerEvent{
		ID:            "trigger-1",
		TS:            "2026-04-25T07:00:00Z",
		Source:        "autoloop",
		Kind:          "quarantine_added",
		PhaseID:       "5",
		SubphaseID:    "5.Q",
		ItemName:      "Responses API store",
		Reason:        "auto quarantine",
		AutoloopRunID: "run-1",
	}
	if err := plannertriggers.AppendTriggerEvent(cfg.PlannerTriggersPath, trigger); err != nil {
		t.Fatalf("AppendTriggerEvent() error = %v", err)
	}
	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "planner ran ok\n"},
		},
	}

	summary, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	promptRaw, err := os.ReadFile(summary.PromptPath)
	if err != nil {
		t.Fatalf("ReadFile(prompt) error = %v", err)
	}
	prompt := string(promptRaw)
	for _, want := range []string{
		"Recent Autoloop Signals",
		"5/5.Q/Responses API store",
		"quarantine_added",
		"auto quarantine",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1: %#v", len(events), events)
	}
	if events[0].Trigger != "event" {
		t.Fatalf("Trigger = %q, want event", events[0].Trigger)
	}
	if got := events[0].TriggerEvents; len(got) != 1 || got[0] != "trigger-1" {
		t.Fatalf("TriggerEvents = %#v, want [trigger-1]", got)
	}

	cursor, err := plannertriggers.LoadCursor(cfg.TriggersCursorPath)
	if err != nil {
		t.Fatalf("LoadCursor() error = %v", err)
	}
	if cursor.LastConsumedID != "trigger-1" {
		t.Fatalf("LastConsumedID = %q, want trigger-1", cursor.LastConsumedID)
	}
}

func TestRunDryRunDoesNotAdvanceTriggerCursor(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	if err := plannertriggers.AppendTriggerEvent(cfg.PlannerTriggersPath, plannertriggers.TriggerEvent{
		ID:       "trigger-1",
		Kind:     "quarantine_added",
		PhaseID:  "5",
		ItemName: "dry run row",
	}); err != nil {
		t.Fatalf("AppendTriggerEvent() error = %v", err)
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: cfg,
		Runner: &cmdrunner.FakeRunner{},
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if _, err := os.Stat(cfg.TriggersCursorPath); !os.IsNotExist(err) {
		t.Fatalf("trigger cursor stat error = %v, want missing cursor after dry-run", err)
	}
}

func TestRunOnceReturnsBackendErrorWithOutput(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	wantErr := os.ErrPermission
	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{{}, {}, {}, {Err: wantErr, Stderr: "backend denied\n"}},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config:         mustConfig(t, repoRoot),
		Runner:         runner,
		SkipValidation: true,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "backend denied") {
		t.Fatalf("RunOnce() error = %q, want backend stderr", err)
	}
}

func TestPlannerFailureDetailIncludesProcessErrorWithStdout(t *testing.T) {
	detail := plannerFailureDetail(cmdrunner.Result{
		Stdout: "Reading additional input from stdin...\n",
		Err:    errors.New("signal: killed"),
	})

	if !strings.Contains(detail, "signal: killed") {
		t.Fatalf("detail = %q, want process error", detail)
	}
	if !strings.Contains(detail, "Reading additional input from stdin") {
		t.Fatalf("detail = %q, want backend stdout", detail)
	}
}

func TestRunOnceClassifiesKilledBackendWithoutBlamingStdinWait(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Err: errors.New("signal: killed"), Stdout: "Reading additional input from stdin...\n"},
		},
	}
	cfg := mustConfig(t, repoRoot)

	_, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want backend failure")
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1: %#v", len(events), events)
	}
	if events[0].Status != "backend_failed" {
		t.Fatalf("Status = %q, want backend_failed terminal status", events[0].Status)
	}
	if events[0].Event != "backend_killed" {
		t.Fatalf("Event = %q, want backend_killed classification", events[0].Event)
	}
	if !strings.Contains(events[0].Detail, "backend_killed") {
		t.Fatalf("Detail = %q, want backend_killed prefix", events[0].Detail)
	}
	if !strings.Contains(events[0].Detail, "Reading additional input from stdin") {
		t.Fatalf("Detail = %q, want preserved stdin output", events[0].Detail)
	}
}

func TestRunOnceAppliesBackendTimeoutAndRecordsErrDetail(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	cfg.BackendTimeout = time.Minute
	runner := &deadlineCapturingRunner{}

	_, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want backend timeout error")
	}
	if !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("RunOnce() error = %q, want deadline detail", err)
	}
	if !runner.backendHadDeadline {
		t.Fatal("backend command did not receive a context deadline")
	}
	remaining := time.Until(runner.backendDeadline)
	if remaining <= 0 || remaining > cfg.BackendTimeout {
		t.Fatalf("backend deadline remaining = %s, want within %s", remaining, cfg.BackendTimeout)
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(events))
	}
	if events[0].Status != "backend_failed" {
		t.Fatalf("Status = %q, want backend_failed", events[0].Status)
	}
	if !strings.Contains(events[0].Detail, context.DeadlineExceeded.Error()) {
		t.Fatalf("Detail = %q, want deadline detail", events[0].Detail)
	}
	if len(events[0].Attempts) != 1 || !strings.Contains(events[0].Attempts[0].Detail, context.DeadlineExceeded.Error()) {
		t.Fatalf("Attempts = %#v, want deadline detail", events[0].Attempts)
	}
}

func TestRunOnceRunsValidationAfterBackend(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{{}, {}, {}, {}, {}, {}, {}, {}, {}},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: mustConfig(t, repoRoot),
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if got, want := len(runner.Commands), 9; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
	wantArgs := [][]string{
		{"run", "./cmd/builder-loop", "progress", "write"},
		{"run", "./cmd/builder-loop", "progress", "validate"},
		{"test", "./internal/progress", "-count=1"},
		{"test", "./docs", "-count=1"},
		{"test", "./...", "-count=1"},
	}
	for i, want := range wantArgs {
		command := runner.Commands[i+4]
		if command.Name != "go" {
			t.Fatalf("validation command %d name = %q, want go", i, command.Name)
		}
		if strings.Join(command.Args, " ") != strings.Join(want, " ") {
			t.Fatalf("validation command %d args = %#v, want %#v", i, command.Args, want)
		}
	}
	if got, want := runner.Commands[8].Dir, filepath.Join(repoRoot, "www.gormes.ai"); got != want {
		t.Fatalf("landing validation dir = %q, want %q", got, want)
	}
	docsEnv := strings.Join(runner.Commands[7].Env, "\n")
	if !strings.Contains(docsEnv, filepath.Join(home, "go", "bin")) {
		t.Fatalf("docs validation env = %#v, want HOME/go/bin on PATH", runner.Commands[7].Env)
	}
}

func TestRunOnce_AppendsLedgerEventOnValidationCommandFailure(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	validationErr := errors.New("signal: killed")
	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "planner ran ok\n"},
			{Err: validationErr, Stdout: "progress write started\n"},
		},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: cfg,
		Runner: runner,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want validation command failure")
	}
	if !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("RunOnce() error = %q, want validation error detail", err)
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1: %#v", len(events), events)
	}
	ev := events[0]
	if ev.Status != "validation_failed" {
		t.Fatalf("Status = %q, want validation_failed", ev.Status)
	}
	if !strings.Contains(ev.Detail, "signal: killed") || !strings.Contains(ev.Detail, "progress write started") {
		t.Fatalf("Detail = %q, want process error and command output", ev.Detail)
	}
	if len(ev.Attempts) != 1 || ev.Attempts[0].Status != "ok" {
		t.Fatalf("Attempts = %#v, want backend attempt recorded as ok", ev.Attempts)
	}
}

func TestWriteReportCondensesJSONEventStreamToLastAgentMessage(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "latest_planner_report.md")
	rawPath := filepath.Join(dir, "latest_planner_report.raw.md")
	result := cmdrunner.Result{Stdout: strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-1"}`,
		`{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"first draft"}}`,
		`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","aggregated_output":"large command transcript"}}`,
		`{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"final planner report"}}`,
	}, "\n")}
	bundle := ContextBundle{
		RepoRoot:     "/repo",
		ProgressJSON: "/repo/progress.json",
		ProgressStats: ProgressInfo{
			Items: 1,
		},
	}

	if err := writeReport(reportPath, rawPath, result, bundle, time.Date(2026, 4, 25, 23, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("writeReport() error = %v", err)
	}

	reportRaw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("ReadFile(report) error = %v", err)
	}
	report := string(reportRaw)
	if !strings.Contains(report, "final planner report") {
		t.Fatalf("report missing final agent message:\n%s", report)
	}
	if strings.Contains(report, "large command transcript") || strings.Contains(report, "thread.started") {
		t.Fatalf("report included JSON event stream instead of final agent text:\n%s", report)
	}
}

// mutatingRunner wraps a FakeRunner so the test can mutate progress.json
// when the planner backend command (codexu/claudeu) is dispatched. This
// mirrors what a real LLM backend does — emit the report and rewrite
// progress.json in one shot — so the ledger wire-in sees a real before/after
// delta to record. The mutator is invoked once on the FIRST backend
// invocation; later backend calls (e.g. retries in future tasks) fall
// through to the wrapped FakeRunner unchanged.
type mutatingRunner struct {
	inner   *cmdrunner.FakeRunner
	mutate  func(t *testing.T) // performed before returning the backend result
	t       *testing.T
	mutated bool
}

func (r *mutatingRunner) Run(ctx context.Context, command cmdrunner.Command) cmdrunner.Result {
	res := r.inner.Run(ctx, command)
	if !r.mutated && (command.Name == "codexu" || command.Name == "claudeu") {
		r.mutated = true
		if r.mutate != nil {
			r.mutate(r.t)
		}
	}
	return res
}

func TestRunOnce_AppendsLedgerEventOnSuccess(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	progressPath := cfg.ProgressJSON

	runner := &mutatingRunner{
		t: t,
		inner: &cmdrunner.FakeRunner{
			Results: []cmdrunner.Result{
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "planner ran ok\n"},
			},
		},
		mutate: func(t *testing.T) {
			// Add a brand-new row and flip an existing row's status to
			// in_progress. The Health blocks on the original rows are
			// preserved (they were never set in the fixture, so this is
			// trivially true), so validateHealthPreservation passes and
			// the run records status="ok".
			writeFile(t, progressPath, `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "complete"},
            {"name": "Goncho task", "status": "in_progress"},
            {"name": "Brand new task", "status": "planned"}
          ]
        }
      }
    }
  }
}`)
		},
	}

	summary, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1: %#v", len(events), events)
	}
	ev := events[0]
	if ev.Status != "ok" {
		t.Fatalf("Status = %q, want ok", ev.Status)
	}
	if ev.RunID != summary.RunID || ev.RunID == "" {
		t.Fatalf("RunID = %q, want %q (non-empty)", ev.RunID, summary.RunID)
	}
	if ev.Trigger != "scheduled" {
		t.Fatalf("Trigger = %q, want scheduled", ev.Trigger)
	}
	if ev.Backend != "codexu" {
		t.Fatalf("Backend = %q, want codexu", ev.Backend)
	}
	// Before doc had 2 rows (1 planned, 1 in_progress); after has 3 rows
	// (1 complete, 1 in_progress, 1 planned). Exactly one added row plus
	// one spec_changed (Gateway task flipped status — but status isn't in
	// ItemSpecHash, so it doesn't show up). Net: one "added" row only.
	if got, want := len(ev.RowsChanged), 1; got != want {
		t.Fatalf("RowsChanged length = %d, want %d: %#v", got, want, ev.RowsChanged)
	}
	if ev.RowsChanged[0].Kind != "added" || ev.RowsChanged[0].ItemName != "Brand new task" {
		t.Fatalf("RowsChanged[0] = %#v, want added/Brand new task", ev.RowsChanged[0])
	}
	if ev.BeforeStats.Planned != 1 || ev.BeforeStats.InProgress != 1 {
		t.Fatalf("BeforeStats = %#v, want Planned=1 InProgress=1", ev.BeforeStats)
	}
	if ev.AfterStats.Shipped != 1 || ev.AfterStats.InProgress != 1 || ev.AfterStats.Planned != 1 {
		t.Fatalf("AfterStats = %#v, want Shipped=1 InProgress=1 Planned=1", ev.AfterStats)
	}
}

func TestRunOnce_AppendsLedgerEventOnValidationReject(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	cfg.MaxRetries = 0
	progressPath := cfg.ProgressJSON

	// Seed a Health block on an existing row so the planner regen has
	// something to drop. The fixture writes raw JSON so we re-write it
	// here with the Health block included.
	writeFile(t, progressPath, `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned", "health": {"attempt_count": 3, "consecutive_failures": 1}},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
      }
    }
  }
}`)

	runner := &mutatingRunner{
		t: t,
		inner: &cmdrunner.FakeRunner{
			Results: []cmdrunner.Result{
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "planner ran ok\n"},
			},
		},
		mutate: func(t *testing.T) {
			// Drop the Health block from "Gateway task" — this MUST be
			// rejected by validateHealthPreservation.
			writeFile(t, progressPath, `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned"},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
      }
    }
  }
}`)
		},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want validation rejection")
	}
	if !strings.Contains(err.Error(), "regeneration rejected") {
		t.Fatalf("RunOnce() error = %q, want regeneration rejected", err)
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1: %#v", len(events), events)
	}
	if events[0].Status != "validation_rejected" {
		t.Fatalf("Status = %q, want validation_rejected", events[0].Status)
	}
	if events[0].Detail == "" {
		t.Fatalf("Detail = empty, want validation error message")
	}
}

func TestRunOnce_RejectsRuntimeSourceEdits(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	runtimePath := filepath.Join(repoRoot, "internal", "builderloop", "run.go")
	writeFile(t, runtimePath, "package builderloop\n")

	runner := &mutatingRunner{
		t: t,
		inner: &cmdrunner.FakeRunner{
			Results: []cmdrunner.Result{
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "planner ran ok\n"},
			},
		},
		mutate: func(t *testing.T) {
			writeFile(t, runtimePath, "package builderloop\n\nfunc offContract() {}\n")
		},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want runtime source rejection")
	}
	if !strings.Contains(err.Error(), "runtime source files") {
		t.Fatalf("RunOnce() error = %q, want runtime source rejection", err)
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1: %#v", len(events), events)
	}
	if events[0].Status != "validation_rejected" {
		t.Fatalf("Status = %q, want validation_rejected", events[0].Status)
	}
	if !strings.Contains(events[0].Detail, "internal/builderloop/run.go") {
		t.Fatalf("Detail = %q, want runtime path", events[0].Detail)
	}
	if len(events[0].Attempts) != 1 || events[0].Attempts[0].Status != "validation_rejected" {
		t.Fatalf("Attempts = %#v, want one validation_rejected attempt", events[0].Attempts)
	}
}

func TestRunOnce_RejectsDirtyRuntimeSourcesBeforeBackend(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	runtimePath := filepath.Join(repoRoot, "internal", "plannerloop", "topics_test.go")
	writeFile(t, runtimePath, "package plannerloop\n")
	initPlannerGitRepo(t, repoRoot)
	writeFile(t, runtimePath, "package plannerloop\n\nfunc dirtyRuntimeFixture() {}\n")

	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "planner ran ok\n"},
		},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want dirty runtime source preflight rejection")
	}
	if !strings.Contains(err.Error(), "runtime source preflight") {
		t.Fatalf("RunOnce() error = %q, want preflight rejection", err)
	}
	if !strings.Contains(err.Error(), "internal/plannerloop/topics_test.go") {
		t.Fatalf("RunOnce() error = %q, want dirty runtime path", err)
	}
	if got := len(runner.Commands); got != 3 {
		t.Fatalf("runner commands = %d, want only 3 sync commands before backend", got)
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1: %#v", len(events), events)
	}
	if events[0].Status != "validation_rejected" {
		t.Fatalf("Status = %q, want validation_rejected", events[0].Status)
	}
	if !strings.Contains(events[0].Detail, "internal/plannerloop/topics_test.go") {
		t.Fatalf("Detail = %q, want dirty runtime path", events[0].Detail)
	}
}

func TestRunOnce_LedgerWriteFailureIsSoftFail(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)

	// Pre-create a regular file at the path where the ledger directory
	// would be created. AppendLedgerEvent calls os.MkdirAll on the parent
	// directory; on a path that already exists as a non-directory,
	// MkdirAll returns ENOTDIR. This deterministically forces the soft-
	// fail path without relying on chmod (which is racy under sudo
	// or root-owned test runners).
	ledgerStateDir := filepath.Join(cfg.RunRoot, "state")
	if err := os.MkdirAll(cfg.RunRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(RunRoot) error = %v", err)
	}
	if err := os.WriteFile(ledgerStateDir, []byte("blocker\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(state) error = %v", err)
	}

	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "planner ran ok\n"},
		},
	}

	// Capture log output so we can assert the soft-fail message was
	// emitted without polluting test stderr.
	var logBuf strings.Builder
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&logBuf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	_, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v, want nil (soft-fail)", err)
	}
	if !strings.Contains(logBuf.String(), "planner: append ledger failed") {
		t.Fatalf("log output missing soft-fail message: %q", logBuf.String())
	}

	// The "ledger" path is now a regular file (the blocker we wrote),
	// not the JSONL we expected, so loadLedger via os.Stat shows a file
	// that is not a JSONL (or it's the blocker itself, depending on
	// where the failure occurred). Either way, no ledger entries should
	// have been recorded.
	if info, err := os.Stat(filepath.Join(cfg.RunRoot, "state")); err == nil && info.IsDir() {
		t.Fatalf("state/ unexpectedly became a directory; soft-fail path did not exercise MkdirAll failure")
	}
}

// mustReadLedger reads and decodes runs.jsonl, failing the test if the file
// is missing or unparsable. Caller asserts on the returned events.
func mustReadLedger(t *testing.T, path string) []LedgerEvent {
	t.Helper()
	events, err := LoadLedger(path)
	if err != nil {
		t.Fatalf("LoadLedger(%s) error = %v", path, err)
	}
	return events
}

func mustConfig(t *testing.T, repoRoot string) Config {
	t.Helper()

	cfg, err := ConfigFromEnv(repoRoot, MapEnv(map[string]string{
		"RUN_ROOT":                 filepath.Join(repoRoot, ".codex", "architecture-planner-test"),
		"MERGE_OPEN_PULL_REQUESTS": "0",
	}))
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	return cfg
}

func writePlannerFixture(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "content", "building-gormes", "architecture_plan", "progress.json"), `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned"},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
      }
    }
  }
}`)
	for _, path := range []string{
		filepath.Join(root, "..", "hermes-agent", ".git", "HEAD"),
		filepath.Join(root, "..", "hermes-agent", "README.md"),
		filepath.Join(root, "..", "gbrain", ".git", "HEAD"),
		filepath.Join(root, "..", "gbrain", "README.md"),
		filepath.Join(root, "..", "honcho", ".git", "HEAD"),
		filepath.Join(root, "..", "honcho", "README.md"),
		filepath.Join(root, "docs", "content", "upstream-hermes", "_index.md"),
		filepath.Join(root, "docs", "content", "upstream-gbrain", "_index.md"),
		filepath.Join(root, "docs", "content", "building-gormes", "_index.md"),
		filepath.Join(root, "docs", "hugo.toml"),
		filepath.Join(root, "docs", "layouts", "index.html"),
		filepath.Join(root, "docs", "static", "site.css"),
		filepath.Join(root, "www.gormes.ai", "README.md"),
		filepath.Join(root, "www.gormes.ai", "internal", "site", "server.go"),
		filepath.Join(root, "www.gormes.ai", "tests", "home.spec.mjs"),
	} {
		writeFile(t, path, "# fixture\n")
	}
	return root
}

func initPlannerGitRepo(t *testing.T, repoRoot string) {
	t.Helper()
	runPlannerGitCommand(t, repoRoot, "init")
	runPlannerGitCommand(t, repoRoot, "config", "user.email", "planner-test@example.com")
	runPlannerGitCommand(t, repoRoot, "config", "user.name", "Planner Test")
	runPlannerGitCommand(t, repoRoot, "add", ".")
	runPlannerGitCommand(t, repoRoot, "commit", "-m", "initial fixture")
}

func runPlannerGitCommand(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func TestRunOnce_TriggerEventsThreadIntoPrompt(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)

	// Seed two trigger events at the path the planner reads.
	if err := plannertriggers.AppendTriggerEvent(cfg.PlannerTriggersPath, plannertriggers.TriggerEvent{
		Kind:       "quarantine_added",
		PhaseID:    "2",
		SubphaseID: "2.A",
		ItemName:   "Gateway task",
		Reason:     "3rd failure in worker_error",
	}); err != nil {
		t.Fatalf("AppendTriggerEvent error = %v", err)
	}
	if err := plannertriggers.AppendTriggerEvent(cfg.PlannerTriggersPath, plannertriggers.TriggerEvent{
		Kind:       "quarantine_stale_cleared",
		PhaseID:    "2",
		SubphaseID: "2.A",
		ItemName:   "Goncho task",
		Reason:     "spec hash changed",
	}); err != nil {
		t.Fatalf("AppendTriggerEvent error = %v", err)
	}

	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "planner ran ok\n"},
		},
	}

	summary, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	promptBody, err := os.ReadFile(filepath.Join(summary.RunRoot, "latest_prompt.txt"))
	if err != nil {
		t.Fatalf("ReadFile(latest_prompt.txt) error = %v", err)
	}
	wants := []string{
		"## Recent Autoloop Signals (Since Last Planner Run)",
		"2/2.A/Gateway task — quarantine_added — 3rd failure in worker_error",
		"2/2.A/Goncho task — quarantine_stale_cleared — spec hash changed",
	}
	for _, want := range wants {
		if !strings.Contains(string(promptBody), want) {
			t.Fatalf("prompt missing %q:\n%s", want, promptBody)
		}
	}

	// Trigger=event in the ledger; trigger_events lists both IDs in
	// append order.
	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1: %#v", len(events), events)
	}
	if events[0].Trigger != "event" {
		t.Fatalf("Trigger = %q, want event", events[0].Trigger)
	}
	if len(events[0].TriggerEvents) != 2 {
		t.Fatalf("TriggerEvents length = %d, want 2: %#v", len(events[0].TriggerEvents), events[0].TriggerEvents)
	}

	// Cursor advances to the last event ID after a successful run.
	cursor, err := plannertriggers.LoadCursor(cfg.TriggersCursorPath)
	if err != nil {
		t.Fatalf("LoadCursor error = %v", err)
	}
	if cursor.LastConsumedID != events[0].TriggerEvents[1] {
		t.Fatalf("cursor.LastConsumedID = %q, want last event ID %q",
			cursor.LastConsumedID, events[0].TriggerEvents[1])
	}
}

func TestRunOnce_TriggerImplChangeFromEnv(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	// No plannertriggers events queued — the impl-change reason wins
	// over the default "scheduled" label per the Phase D priority order:
	// event > impl_change > scheduled.
	cfg.TriggerReason = "impl_change"

	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "planner ran ok\n"},
		},
	}

	if _, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	}); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(events))
	}
	if events[0].Trigger != "impl_change" {
		t.Fatalf("Trigger = %q, want impl_change", events[0].Trigger)
	}
	if len(events[0].TriggerEvents) != 0 {
		t.Fatalf("TriggerEvents = %#v, want empty (impl_change comes from env, not the trigger ledger)", events[0].TriggerEvents)
	}
}

func TestRunOnce_NoTriggerEventsKeepsScheduledTrigger(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)

	runner := &cmdrunner.FakeRunner{
		Results: []cmdrunner.Result{
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "Already up to date.\n"},
			{Stdout: "planner ran ok\n"},
		},
	}

	if _, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	}); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(events))
	}
	if events[0].Trigger != "scheduled" {
		t.Fatalf("Trigger = %q, want scheduled", events[0].Trigger)
	}
	if len(events[0].TriggerEvents) != 0 {
		t.Fatalf("TriggerEvents = %#v, want empty", events[0].TriggerEvents)
	}

	// No events queued -> no cursor file written either.
	if _, err := os.Stat(cfg.TriggersCursorPath); !os.IsNotExist(err) {
		t.Fatalf("expected no cursor file when no events; stat err = %v", err)
	}
}

func TestRunOnce_CursorAdvancesEvenOnValidationReject(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	cfg.MaxRetries = 0
	progressPath := cfg.ProgressJSON

	// Seed a Health block so the planner regen has something to drop.
	writeFile(t, progressPath, `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned", "health": {"attempt_count": 3, "consecutive_failures": 1}},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
      }
    }
  }
}`)

	// Seed one trigger event so the cursor has something to advance to.
	if err := plannertriggers.AppendTriggerEvent(cfg.PlannerTriggersPath, plannertriggers.TriggerEvent{
		Kind:       "quarantine_added",
		PhaseID:    "2",
		SubphaseID: "2.A",
		ItemName:   "Gateway task",
		Reason:     "validation seed",
	}); err != nil {
		t.Fatalf("AppendTriggerEvent error = %v", err)
	}
	seeded, err := plannertriggers.ReadTriggersSinceCursor(cfg.PlannerTriggersPath, plannertriggers.TriggerCursor{})
	if err != nil {
		t.Fatalf("ReadTriggersSinceCursor error = %v", err)
	}
	if len(seeded) != 1 {
		t.Fatalf("expected 1 seeded trigger event, got %d", len(seeded))
	}
	wantCursorID := seeded[0].ID

	// Mutator drops the Health block on Gateway task, forcing the planner
	// to reject the regeneration.
	runner := &mutatingRunner{
		t: t,
		inner: &cmdrunner.FakeRunner{
			Results: []cmdrunner.Result{
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "planner ran ok\n"},
			},
		},
		mutate: func(t *testing.T) {
			writeFile(t, progressPath, `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned"},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
      }
    }
  }
}`)
		},
	}

	if _, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	}); err == nil {
		t.Fatal("RunOnce() error = nil, want validation rejection")
	}

	// Even though the run failed, the deferred cursor save must have
	// fired — the trigger events represent state transitions, not work
	// to retry.
	cursor, err := plannertriggers.LoadCursor(cfg.TriggersCursorPath)
	if err != nil {
		t.Fatalf("LoadCursor error = %v", err)
	}
	if cursor.LastConsumedID != wantCursorID {
		t.Fatalf("cursor.LastConsumedID = %q, want %q (cursor must advance on validation reject)",
			cursor.LastConsumedID, wantCursorID)
	}

	// Ledger entry records trigger=event with the consumed event ID.
	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(events))
	}
	if events[0].Status != "validation_rejected" {
		t.Fatalf("Status = %q, want validation_rejected", events[0].Status)
	}
	if events[0].Trigger != "event" {
		t.Fatalf("Trigger = %q, want event", events[0].Trigger)
	}
	if len(events[0].TriggerEvents) != 1 || events[0].TriggerEvents[0] != wantCursorID {
		t.Fatalf("TriggerEvents = %#v, want [%s]", events[0].TriggerEvents, wantCursorID)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

// perAttemptRunner is a test runner that wires distinct per-backend-call
// behaviors. Each backend invocation (codexu/claudeu) increments a counter
// and dispatches to the corresponding entry in attemptMutators. Used to
// simulate the L3 retry-with-feedback loop where attempt 1 drops health,
// attempt 2 (or 3) restores it. Captures every prompt sent so tests can
// assert the retry feedback is appended on subsequent calls. Non-backend
// commands fall through to the wrapped FakeRunner unchanged.
type perAttemptRunner struct {
	inner            *cmdrunner.FakeRunner
	attemptMutators  []func(t *testing.T)
	t                *testing.T
	backendCallCount int
	prompts          []string
}

type deadlineCapturingRunner struct {
	commands           []cmdrunner.Command
	backendHadDeadline bool
	backendDeadline    time.Time
}

func (r *deadlineCapturingRunner) Run(ctx context.Context, command cmdrunner.Command) cmdrunner.Result {
	r.commands = append(r.commands, command)
	if command.Name != "codexu" && command.Name != "claudeu" {
		return cmdrunner.Result{}
	}
	deadline, ok := ctx.Deadline()
	r.backendHadDeadline = ok
	r.backendDeadline = deadline
	return cmdrunner.Result{Err: context.DeadlineExceeded}
}

func (r *perAttemptRunner) Run(ctx context.Context, command cmdrunner.Command) cmdrunner.Result {
	res := r.inner.Run(ctx, command)
	if command.Name == "codexu" || command.Name == "claudeu" {
		// Capture the prompt (last arg) so tests can verify the retry
		// feedback is appended to subsequent attempts.
		if len(command.Args) > 0 {
			r.prompts = append(r.prompts, command.Args[len(command.Args)-1])
		}
		idx := r.backendCallCount
		r.backendCallCount++
		if idx < len(r.attemptMutators) && r.attemptMutators[idx] != nil {
			r.attemptMutators[idx](r.t)
		}
	}
	return res
}

func TestRunOnce_RetryRecoversAfterFirstAttemptDropsHealth(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	cfg.MaxRetries = 2 // explicit; default is 2 anyway
	progressPath := cfg.ProgressJSON

	// Seed a Health block on Gateway task so a drop has something to
	// detect.
	withHealth := `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned", "health": {"attempt_count": 3, "consecutive_failures": 1}},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
      }
    }
  }
}`
	writeFile(t, progressPath, withHealth)

	withoutHealth := `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned"},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
      }
    }
  }
}`

	// Attempt 0 drops the health block (rejection); attempt 1 restores it
	// (acceptance). Expect status="ok" and 2 attempts in the ledger.
	runner := &perAttemptRunner{
		t: t,
		inner: &cmdrunner.FakeRunner{
			Results: []cmdrunner.Result{
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "planner attempt 0\n"},
				{Stdout: "planner attempt 1\n"},
			},
		},
		attemptMutators: []func(t *testing.T){
			func(t *testing.T) { writeFile(t, progressPath, withoutHealth) },
			func(t *testing.T) { writeFile(t, progressPath, withHealth) },
		},
	}

	if _, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	}); err != nil {
		t.Fatalf("RunOnce() error = %v, want recovery on retry", err)
	}

	if got := runner.backendCallCount; got != 2 {
		t.Fatalf("backendCallCount = %d, want 2 (initial + 1 retry)", got)
	}
	if len(runner.prompts) != 2 {
		t.Fatalf("len(prompts) = %d, want 2", len(runner.prompts))
	}
	if !strings.Contains(runner.prompts[1], "HEALTH BLOCK PRESERVATION") {
		t.Fatalf("retry prompt missing HARD RULE reference:\n%s", runner.prompts[1])
	}
	if !strings.Contains(runner.prompts[1], "2/2.A/Gateway task") {
		t.Fatalf("retry prompt missing dropped row name:\n%s", runner.prompts[1])
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Status != "ok" {
		t.Fatalf("Status = %q, want ok (recovered after retry)", ev.Status)
	}
	if len(ev.Attempts) != 2 {
		t.Fatalf("Attempts length = %d, want 2: %#v", len(ev.Attempts), ev.Attempts)
	}
	if ev.Attempts[0].Status != "validation_rejected" {
		t.Fatalf("Attempts[0].Status = %q, want validation_rejected", ev.Attempts[0].Status)
	}
	if len(ev.Attempts[0].DroppedRows) != 1 || ev.Attempts[0].DroppedRows[0] != "2/2.A/Gateway task" {
		t.Fatalf("Attempts[0].DroppedRows = %#v, want [2/2.A/Gateway task]", ev.Attempts[0].DroppedRows)
	}
	if ev.Attempts[1].Status != "ok" {
		t.Fatalf("Attempts[1].Status = %q, want ok", ev.Attempts[1].Status)
	}
	if ev.RetryAttempt != 1 {
		t.Fatalf("RetryAttempt = %d, want 1 (final attempt index)", ev.RetryAttempt)
	}
}

func TestRunOnce_RetryExhaustionStillRejectsWithFullForensics(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	cfg.MaxRetries = 2 // up to 3 total backend invocations
	progressPath := cfg.ProgressJSON

	withHealth := `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned", "health": {"attempt_count": 3, "consecutive_failures": 1}},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
      }
    }
  }
}`
	writeFile(t, progressPath, withHealth)

	withoutHealth := `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned"},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
      }
    }
  }
}`

	dropper := func(t *testing.T) { writeFile(t, progressPath, withoutHealth) }
	// Seed health back before each LLM attempt so beforeDoc remains stable
	// across the run; in real life the LLM stomps the file each run, so we
	// emulate by re-seeding before the next attempt's mutator drops it.
	runner := &perAttemptRunner{
		t: t,
		inner: &cmdrunner.FakeRunner{
			Results: []cmdrunner.Result{
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "attempt 0\n"},
				{Stdout: "attempt 1\n"},
				{Stdout: "attempt 2\n"},
			},
		},
		attemptMutators: []func(t *testing.T){dropper, dropper, dropper},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want regeneration rejected after retry exhaustion")
	}
	if !strings.Contains(err.Error(), "regeneration rejected") {
		t.Fatalf("RunOnce() error = %q, want regeneration rejected", err)
	}
	if got, want := runner.backendCallCount, 3; got != want {
		t.Fatalf("backendCallCount = %d, want %d (1 initial + 2 retries)", got, want)
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Status != "validation_rejected" {
		t.Fatalf("Status = %q, want validation_rejected", ev.Status)
	}
	if len(ev.Attempts) != 3 {
		t.Fatalf("Attempts length = %d, want 3 (initial + 2 retries)", len(ev.Attempts))
	}
	for i, a := range ev.Attempts {
		if a.Index != i {
			t.Fatalf("Attempts[%d].Index = %d, want %d", i, a.Index, i)
		}
		if a.Status != "validation_rejected" {
			t.Fatalf("Attempts[%d].Status = %q, want validation_rejected", i, a.Status)
		}
	}
	if ev.RetryAttempt != 2 {
		t.Fatalf("RetryAttempt = %d, want 2 (final attempt index)", ev.RetryAttempt)
	}
}

func TestRunOnce_BackendFailureDoesNotRetry(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	cfg.MaxRetries = 5 // even with retries available, backend errors short-circuit

	wantErr := os.ErrPermission
	runner := &perAttemptRunner{
		t: t,
		inner: &cmdrunner.FakeRunner{
			Results: []cmdrunner.Result{
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Err: wantErr, Stderr: "backend denied\n"},
			},
		},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want backend failure")
	}
	if !strings.Contains(err.Error(), "backend denied") {
		t.Fatalf("RunOnce() error = %q, want backend stderr", err)
	}
	if got := runner.backendCallCount; got != 1 {
		t.Fatalf("backendCallCount = %d, want 1 (backend failures must NOT retry)", got)
	}

	events := mustReadLedger(t, filepath.Join(cfg.RunRoot, "state", "runs.jsonl"))
	if len(events) != 1 {
		t.Fatalf("ledger entries = %d, want 1", len(events))
	}
	if events[0].Status != "backend_failed" {
		t.Fatalf("Status = %q, want backend_failed", events[0].Status)
	}
	if len(events[0].Attempts) != 1 || events[0].Attempts[0].Status != "backend_failed" {
		t.Fatalf("Attempts = %#v, want one entry with backend_failed", events[0].Attempts)
	}
}

func TestRunOnce_MaxRetriesZeroSkipsRetryLoop(t *testing.T) {
	repoRoot := writePlannerFixture(t)
	cfg := mustConfig(t, repoRoot)
	cfg.MaxRetries = 0 // disable retries — pre-L3 behavior
	progressPath := cfg.ProgressJSON

	writeFile(t, progressPath, `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned", "health": {"attempt_count": 3}},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
      }
    }
  }
}`)

	runner := &perAttemptRunner{
		t: t,
		inner: &cmdrunner.FakeRunner{
			Results: []cmdrunner.Result{
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "Already up to date.\n"},
				{Stdout: "attempt 0\n"},
			},
		},
		attemptMutators: []func(t *testing.T){
			func(t *testing.T) {
				writeFile(t, progressPath, `{
  "phases": {
    "2": {
      "name": "Gateway",
      "subphases": {
        "2.A": {
          "items": [
            {"name": "Gateway task", "status": "planned"},
            {"name": "Goncho task", "status": "in_progress"}
          ]
        }
      }
    }
  }
}`)
			},
		},
	}

	if _, err := RunOnce(context.Background(), RunOptions{
		Config:         cfg,
		Runner:         runner,
		SkipValidation: true,
	}); err == nil {
		t.Fatal("RunOnce() error = nil, want validation rejection without retry")
	}
	if got := runner.backendCallCount; got != 1 {
		t.Fatalf("backendCallCount = %d, want 1 (MaxRetries=0 disables retries)", got)
	}
}
