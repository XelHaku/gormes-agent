package autoloop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMergeOpenPullRequestsMergesEveryNonDraftPROneByOne(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, ".git"))
	runRoot := t.TempDir()
	runner := &FakeRunner{Results: []Result{
		{Stdout: "git@github.com:TrebuchetDynamics/gormes-agent.git\n"},
		{Stdout: `[
			{"number": 3, "title": "third", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/third"},
			{"number": 1, "title": "first", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/first"},
			{"number": 2, "title": "draft", "isDraft": true, "mergeStateStatus": "CLEAN", "headRefName": "feature/draft"}
		]`},
		{},
		{},
		{},
		{},
	}}

	summary, err := MergeOpenPullRequests(context.Background(), PullRequestIntakeOptions{
		Runner:   runner,
		RepoRoot: repoRoot,
		RunRoot:  runRoot,
		RunID:    "run-1",
	})
	if err != nil {
		t.Fatalf("MergeOpenPullRequests() error = %v", err)
	}

	if summary.Listed != 3 || summary.Merged != 2 || summary.Failed != 0 || summary.Skipped != 1 {
		t.Fatalf("summary = %+v, want listed=3 merged=2 failed=0 skipped=1", summary)
	}

	wantCommands := []Command{
		{Name: "git", Args: []string{"remote", "get-url", "origin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "list", "--repo", "TrebuchetDynamics/gormes-agent", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "1", "--repo", "TrebuchetDynamics/gormes-agent", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "3", "--repo", "TrebuchetDynamics/gormes-agent", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "git", Args: []string{"fetch", "origin", "main"}, Dir: repoRoot},
		{Name: "git", Args: []string{"merge", "--ff-only", "FETCH_HEAD"}, Dir: repoRoot},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, wantCommands)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var got []string
	for _, event := range events {
		got = append(got, event.Event+":"+event.Status)
	}
	wantEvents := []string{
		"pr_intake_started:started",
		"pr_intake_skipped:draft",
		"pr_intake_merged:merged",
		"pr_intake_merged:merged",
		"pr_intake_synced:fast_forward",
		"pr_intake_completed:completed",
	}
	if !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("ledger events = %#v, want %#v", got, wantEvents)
	}
}

func TestMergeOpenPullRequestsClosesDirtyPullRequestsByDefault(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, ".git"))
	runRoot := t.TempDir()
	runner := &FakeRunner{Results: []Result{
		{Stdout: "git@github.com:TrebuchetDynamics/gormes-agent.git\n"},
		{Stdout: `[
			{"number": 7, "title": "ready", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/ready"},
			{"number": 4, "title": "conflicted", "isDraft": false, "mergeStateStatus": "DIRTY", "headRefName": "feature/conflicted"},
			{"number": 5, "title": "draft", "isDraft": true, "mergeStateStatus": "DIRTY", "headRefName": "feature/draft"}
		]`},
		{},
		{},
		{},
		{},
	}}

	summary, err := MergeOpenPullRequests(context.Background(), PullRequestIntakeOptions{
		Runner:   runner,
		RepoRoot: repoRoot,
		RunRoot:  runRoot,
		RunID:    "run-1",
	})
	if err != nil {
		t.Fatalf("MergeOpenPullRequests() error = %v", err)
	}
	if summary.Listed != 3 || summary.Merged != 1 || summary.Closed != 1 || summary.Failed != 0 || summary.Skipped != 1 {
		t.Fatalf("summary = %+v, want listed=3 merged=1 closed=1 failed=0 skipped=1", summary)
	}

	wantCommands := []Command{
		{Name: "git", Args: []string{"remote", "get-url", "origin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "list", "--repo", "TrebuchetDynamics/gormes-agent", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "close", "4", "--repo", "TrebuchetDynamics/gormes-agent", "--delete-branch", "--comment", "Closed by Gormes PR intake: mergeStateStatus=DIRTY means this PR has merge conflicts and cannot be merged automatically at cycle start."}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "7", "--repo", "TrebuchetDynamics/gormes-agent", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "git", Args: []string{"fetch", "origin", "main"}, Dir: repoRoot},
		{Name: "git", Args: []string{"merge", "--ff-only", "FETCH_HEAD"}, Dir: repoRoot},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, wantCommands)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var got []string
	for _, event := range events {
		got = append(got, event.Event+":"+event.Status)
	}
	wantEvents := []string{
		"pr_intake_started:started",
		"pr_intake_closed:dirty",
		"pr_intake_skipped:draft",
		"pr_intake_merged:merged",
		"pr_intake_synced:fast_forward",
		"pr_intake_completed:completed",
	}
	if !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("ledger events = %#v, want %#v", got, wantEvents)
	}
}

func TestMergeOpenPullRequestsCanSkipDirtyPullRequests(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, ".git"))
	runRoot := t.TempDir()
	runner := &FakeRunner{Results: []Result{
		{Stdout: "git@github.com:TrebuchetDynamics/gormes-agent.git\n"},
		{Stdout: `[{"number": 4, "title": "conflicted", "isDraft": false, "mergeStateStatus": "DIRTY", "headRefName": "feature/conflicted"}]`},
	}}

	summary, err := MergeOpenPullRequests(context.Background(), PullRequestIntakeOptions{
		Runner:         runner,
		RepoRoot:       repoRoot,
		RunRoot:        runRoot,
		RunID:          "run-1",
		ConflictAction: PRConflictActionSkip,
	})
	if err != nil {
		t.Fatalf("MergeOpenPullRequests() error = %v", err)
	}
	if summary.Listed != 1 || summary.Merged != 0 || summary.Closed != 0 || summary.Failed != 0 || summary.Skipped != 1 {
		t.Fatalf("summary = %+v, want listed=1 merged=0 closed=0 failed=0 skipped=1", summary)
	}

	wantCommands := []Command{
		{Name: "git", Args: []string{"remote", "get-url", "origin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "list", "--repo", "TrebuchetDynamics/gormes-agent", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"}, Dir: repoRoot},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, wantCommands)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var got []string
	for _, event := range events {
		got = append(got, event.Event+":"+event.Status)
	}
	wantEvents := []string{
		"pr_intake_started:started",
		"pr_intake_skipped:dirty",
		"pr_intake_completed:completed",
	}
	if !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("ledger events = %#v, want %#v", got, wantEvents)
	}
}

func TestMergeOpenPullRequestsContinuesAfterMergeFailures(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, ".git"))
	runRoot := t.TempDir()
	wantErr := errors.New("merge failed")
	runner := &FakeRunner{Results: []Result{
		{Stdout: "https://github.com/TrebuchetDynamics/gormes-agent.git\n"},
		{Stdout: `[
			{"number": 1, "title": "first", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/first"},
			{"number": 2, "title": "second", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/second"},
			{"number": 3, "title": "third", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/third"}
		]`},
		{Err: wantErr, Stderr: "branch protection"},
		{},
		{Err: wantErr, Stderr: "merge conflicts"},
		{},
		{},
	}}

	summary, err := MergeOpenPullRequests(context.Background(), PullRequestIntakeOptions{
		Runner:   runner,
		RepoRoot: repoRoot,
		RunRoot:  runRoot,
		RunID:    "run-1",
	})
	if err != nil {
		t.Fatalf("MergeOpenPullRequests() error = %v", err)
	}
	if summary.Listed != 3 || summary.Merged != 1 || summary.Failed != 2 || summary.Skipped != 0 {
		t.Fatalf("summary = %+v, want listed=3 merged=1 failed=2 skipped=0", summary)
	}

	wantCommands := []Command{
		{Name: "git", Args: []string{"remote", "get-url", "origin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "list", "--repo", "TrebuchetDynamics/gormes-agent", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "1", "--repo", "TrebuchetDynamics/gormes-agent", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "2", "--repo", "TrebuchetDynamics/gormes-agent", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "3", "--repo", "TrebuchetDynamics/gormes-agent", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "git", Args: []string{"fetch", "origin", "main"}, Dir: repoRoot},
		{Name: "git", Args: []string{"merge", "--ff-only", "FETCH_HEAD"}, Dir: repoRoot},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, wantCommands)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var got []string
	for _, event := range events {
		got = append(got, event.Event+":"+event.Status)
	}
	wantEvents := []string{
		"pr_intake_started:started",
		"pr_intake_failed:merge_failed",
		"pr_intake_merged:merged",
		"pr_intake_failed:merge_failed",
		"pr_intake_synced:fast_forward",
		"pr_intake_completed:completed",
	}
	if !reflect.DeepEqual(got, wantEvents) {
		t.Fatalf("ledger events = %#v, want %#v", got, wantEvents)
	}
}

func TestMergeOpenPullRequestsAttemptsAllMergesBeforePullFailure(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, ".git"))
	runRoot := t.TempDir()
	fastForwardErr := errors.New("fast-forward failed")
	wantErr := errors.New("merge failed")
	runner := &FakeRunner{Results: []Result{
		{Stdout: "git@github.com:TrebuchetDynamics/gormes-agent.git\n"},
		{Stdout: `[
			{"number": 1, "title": "first", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/first"},
			{"number": 2, "title": "second", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/second"}
		]`},
		{},
		{},
		{},
		{Err: fastForwardErr, Stderr: "Not possible to fast-forward"},
		{Err: wantErr, Stderr: "merge conflicts"},
		{},
	}}

	summary, err := MergeOpenPullRequests(context.Background(), PullRequestIntakeOptions{
		Runner:   runner,
		RepoRoot: repoRoot,
		RunRoot:  runRoot,
		RunID:    "run-1",
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("MergeOpenPullRequests() error = %v, want wrapped %v", err, wantErr)
	}
	if summary.Listed != 2 || summary.Merged != 2 || summary.Failed != 0 || summary.Skipped != 0 {
		t.Fatalf("summary = %+v, want listed=2 merged=2 failed=0 skipped=0", summary)
	}

	wantCommands := []Command{
		{Name: "git", Args: []string{"remote", "get-url", "origin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "list", "--repo", "TrebuchetDynamics/gormes-agent", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "1", "--repo", "TrebuchetDynamics/gormes-agent", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "2", "--repo", "TrebuchetDynamics/gormes-agent", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "git", Args: []string{"fetch", "origin", "main"}, Dir: repoRoot},
		{Name: "git", Args: []string{"merge", "--ff-only", "FETCH_HEAD"}, Dir: repoRoot},
		{Name: "git", Args: []string{"merge", "--no-edit", "FETCH_HEAD"}, Dir: repoRoot},
		{Name: "git", Args: []string{"merge", "--abort"}, Dir: repoRoot},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestMergeOpenPullRequestsFallsBackToNonFastForwardSync(t *testing.T) {
	repoRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(repoRoot, ".git"))
	runRoot := t.TempDir()
	runner := &FakeRunner{Results: []Result{
		{Stdout: "git@github.com:TrebuchetDynamics/gormes-agent.git\n"},
		{Stdout: `[{"number": 9, "title": "rescue", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/rescue"}]`},
		{},
		{},
		{Err: errors.New("not ff"), Stderr: "Not possible to fast-forward"},
		{},
	}}

	summary, err := MergeOpenPullRequests(context.Background(), PullRequestIntakeOptions{
		Runner:   runner,
		RepoRoot: repoRoot,
		RunRoot:  runRoot,
		RunID:    "run-1",
	})
	if err != nil {
		t.Fatalf("MergeOpenPullRequests() error = %v", err)
	}
	if summary.Listed != 1 || summary.Merged != 1 || summary.Failed != 0 || summary.Skipped != 0 {
		t.Fatalf("summary = %+v, want listed=1 merged=1 failed=0 skipped=0", summary)
	}

	wantCommands := []Command{
		{Name: "git", Args: []string{"remote", "get-url", "origin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "list", "--repo", "TrebuchetDynamics/gormes-agent", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "9", "--repo", "TrebuchetDynamics/gormes-agent", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "git", Args: []string{"fetch", "origin", "main"}, Dir: repoRoot},
		{Name: "git", Args: []string{"merge", "--ff-only", "FETCH_HEAD"}, Dir: repoRoot},
		{Name: "git", Args: []string{"merge", "--no-edit", "FETCH_HEAD"}, Dir: repoRoot},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, wantCommands)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	if got := events[len(events)-2].Event + ":" + events[len(events)-2].Status; got != "pr_intake_synced:merged" {
		t.Fatalf("sync ledger event = %q, want pr_intake_synced:merged", got)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", path, err)
	}
}

func TestMergeOpenPullRequestsSkipsNonGitRepositories(t *testing.T) {
	runner := &FakeRunner{}

	summary, err := MergeOpenPullRequests(context.Background(), PullRequestIntakeOptions{
		Runner:   runner,
		RepoRoot: t.TempDir(),
		RunRoot:  t.TempDir(),
		RunID:    "run-1",
	})
	if err != nil {
		t.Fatalf("MergeOpenPullRequests() error = %v", err)
	}
	if summary.Listed != 0 || summary.Merged != 0 || summary.Skipped != 0 {
		t.Fatalf("summary = %+v, want zero summary", summary)
	}
	if len(runner.Commands) != 0 {
		t.Fatalf("Commands length = %d, want 0", len(runner.Commands))
	}
}
