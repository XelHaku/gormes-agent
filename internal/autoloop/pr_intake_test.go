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
		{Stdout: `[
			{"number": 3, "title": "third", "isDraft": false, "mergeStateStatus": "DIRTY", "headRefName": "feature/third"},
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
		{Name: "gh", Args: []string{"pr", "list", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "1", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "3", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "git", Args: []string{"pull", "--ff-only"}, Dir: repoRoot},
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
		{Stdout: `[
			{"number": 1, "title": "first", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/first"},
			{"number": 2, "title": "second", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/second"},
			{"number": 3, "title": "third", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/third"}
		]`},
		{Err: wantErr, Stderr: "branch protection"},
		{},
		{Err: wantErr, Stderr: "merge conflicts"},
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
		{Name: "gh", Args: []string{"pr", "list", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "1", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "2", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "3", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "git", Args: []string{"pull", "--ff-only"}, Dir: repoRoot},
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
	wantErr := errors.New("pull failed")
	runner := &FakeRunner{Results: []Result{
		{Stdout: `[
			{"number": 1, "title": "first", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/first"},
			{"number": 2, "title": "second", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "feature/second"}
		]`},
		{},
		{},
		{Err: wantErr, Stderr: "Not possible to fast-forward"},
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
		{Name: "gh", Args: []string{"pr", "list", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "1", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "gh", Args: []string{"pr", "merge", "2", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot},
		{Name: "git", Args: []string{"pull", "--ff-only"}, Dir: repoRoot},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, wantCommands)
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
