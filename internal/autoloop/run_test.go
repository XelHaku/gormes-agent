package autoloop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDryRunSelectsCandidatesWithoutRunningBackend(t *testing.T) {
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
					"subphases": {
					"12.A": {
							"items": [
								{"name": "planned run candidate", "status": "planned", "contract": "run contract", "contract_status": "draft"}
							]
						}
					}
			}
		}
	}`)
	runner := &FakeRunner{}

	summary, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     t.TempDir(),
			ProgressJSON: progressPath,
			Backend:      "codexu",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: runner,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	wantSelected := []Candidate{
		{PhaseID: "12", SubphaseID: "12.A", ItemName: "planned run candidate", Status: "planned", Contract: "run contract", ContractStatus: "draft"},
	}
	if summary.Candidates != 1 {
		t.Fatalf("Candidates = %d, want 1", summary.Candidates)
	}
	if !reflect.DeepEqual(summary.Selected, wantSelected) {
		t.Fatalf("Selected = %#v, want %#v", summary.Selected, wantSelected)
	}
	if len(runner.Commands) != 0 {
		t.Fatalf("Commands length = %d, want 0", len(runner.Commands))
	}
}

func TestRunOnceMergesOpenPullRequestsBeforeSelectingWork(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)
	progressPath := writeProgressJSON(t, `{"phases": {}}`)
	runRoot := t.TempDir()
	runner := &FakeRunner{Results: []Result{
		{Stdout: "git@github.com:TrebuchetDynamics/gormes-agent.git\n"},
		{Stdout: `[{"number": 7, "title": "land worker", "isDraft": false, "mergeStateStatus": "CLEAN", "headRefName": "autoloop/run"}]`},
		{},
		{},
		{},
		{},
	}}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:              repoRoot,
			ProgressJSON:          progressPath,
			RunRoot:               runRoot,
			Backend:               "opencode",
			Mode:                  "safe",
			MaxAgents:             1,
			MergeOpenPullRequests: true,
		},
		Runner: runner,
		Now:    time.Date(2026, 4, 25, 7, 8, 2, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if got, want := runner.Commands[0], (Command{Name: "git", Args: []string{"remote", "get-url", "origin"}, Dir: repoRoot}); !reflect.DeepEqual(got, want) {
		t.Fatalf("first command = %#v, want remote command %#v", got, want)
	}
	if got, want := runner.Commands[1], (Command{Name: "gh", Args: []string{"pr", "list", "--repo", "TrebuchetDynamics/gormes-agent", "--state", "open", "--limit", "100", "--json", "number,title,isDraft,mergeStateStatus,headRefName,url"}, Dir: repoRoot}); !reflect.DeepEqual(got, want) {
		t.Fatalf("second command = %#v, want PR list command %#v", got, want)
	}
	if got, want := runner.Commands[2], (Command{Name: "gh", Args: []string{"pr", "merge", "7", "--repo", "TrebuchetDynamics/gormes-agent", "--merge", "--delete-branch", "--admin"}, Dir: repoRoot}); !reflect.DeepEqual(got, want) {
		t.Fatalf("third command = %#v, want PR merge command %#v", got, want)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	if got := events[1].Event + ":" + events[1].Status; got != "pr_intake_started:started" {
		t.Fatalf("second ledger event = %q, want pr_intake_started:started", got)
	}
}

func TestRunOnceUsesNanosecondSuffixForRapidRunIDs(t *testing.T) {
	progressPath := writeProgressJSON(t, `{"phases": {}}`)
	config := Config{
		RepoRoot:     t.TempDir(),
		ProgressJSON: progressPath,
		Backend:      "codexu",
		Mode:         "safe",
		MaxAgents:    8,
	}

	first, err := RunOnce(context.Background(), RunOptions{
		Config: config,
		DryRun: true,
		Now:    time.Date(2026, 4, 25, 7, 8, 2, 123, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunOnce() first error = %v", err)
	}
	second, err := RunOnce(context.Background(), RunOptions{
		Config: config,
		DryRun: true,
		Now:    time.Date(2026, 4, 25, 7, 8, 2, 456, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunOnce() second error = %v", err)
	}

	if got, want := first.RunID, "20260425T070802Z-000000123"; got != want {
		t.Fatalf("first RunID = %q, want %q", got, want)
	}
	if got, want := second.RunID, "20260425T070802Z-000000456"; got != want {
		t.Fatalf("second RunID = %q, want %q", got, want)
	}
	if first.RunID == second.RunID {
		t.Fatalf("RunID collision = %q, want distinct IDs for rapid runs", first.RunID)
	}
}

func TestRunOncePreservesSecondPrecisionRunIDWhenClockHasNoNanoseconds(t *testing.T) {
	progressPath := writeProgressJSON(t, `{"phases": {}}`)

	summary, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     t.TempDir(),
			ProgressJSON: progressPath,
			Backend:      "codexu",
			Mode:         "safe",
			MaxAgents:    8,
		},
		DryRun: true,
		Now:    time.Date(2026, 4, 25, 7, 8, 2, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if got, want := summary.RunID, "20260425T070802Z"; got != want {
		t.Fatalf("RunID = %q, want %q", got, want)
	}
}

func TestDryRunSkipsCandidatesAboveConfiguredMaxPhase(t *testing.T) {
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"3": {
					"subphases": {
					"3.E": {
							"items": [
								{"name": "phase 3 candidate", "status": "planned", "contract": "phase 3 contract", "contract_status": "draft"}
							]
						}
					}
			},
			"4": {
					"subphases": {
					"4.A": {
							"items": [
								{"name": "phase 4 active candidate", "status": "in_progress", "contract": "phase 4 contract"}
							]
						}
					}
			}
		}
	}`)

	summary, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     t.TempDir(),
			ProgressJSON: progressPath,
			Backend:      "codexu",
			Mode:         "safe",
			MaxAgents:    4,
			MaxPhase:     3,
		},
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	wantSelected := []Candidate{
		{PhaseID: "3", SubphaseID: "3.E", ItemName: "phase 3 candidate", Status: "planned", Contract: "phase 3 contract", ContractStatus: "draft"},
	}
	if summary.Candidates != 1 {
		t.Fatalf("Candidates = %d, want 1", summary.Candidates)
	}
	if !reflect.DeepEqual(summary.Selected, wantSelected) {
		t.Fatalf("Selected = %#v, want %#v", summary.Selected, wantSelected)
	}
}

func TestRunOnceExecutesOncePerSelectedCandidate(t *testing.T) {
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
							"items": [
								{"name": "active candidate", "status": "in_progress", "contract": "active contract"},
								{"name": "planned candidate", "status": "planned", "contract": "planned contract", "contract_status": "draft"},
								{"name": "deferred candidate", "status": "deferred"}
							]
						}
				}
			}
		}
	}`)
	repoRoot := t.TempDir()
	runner := &FakeRunner{
		Results: []Result{{}, {}},
	}

	summary, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     repoRoot,
			ProgressJSON: progressPath,
			Backend:      "opencode",
			Mode:         "safe",
			MaxAgents:    2,
		},
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if summary.Candidates != 2 {
		t.Fatalf("Candidates = %d, want 2", summary.Candidates)
	}
	if got, want := len(summary.Selected), 2; got != want {
		t.Fatalf("Selected length = %d, want %d", got, want)
	}

	wantCommands := []Command{
		{
			Name: "opencode",
			Args: []string{"run", "--no-interactive", BuildWorkerPrompt(Candidate{
				PhaseID:    "12",
				SubphaseID: "12.A",
				ItemName:   "active candidate",
				Status:     "in_progress",
				Contract:   "active contract",
			})},
			Dir: repoRoot,
		},
		{
			Name: "opencode",
			Args: []string{"run", "--no-interactive", BuildWorkerPrompt(Candidate{
				PhaseID:        "12",
				SubphaseID:     "12.A",
				ItemName:       "planned candidate",
				Status:         "planned",
				Contract:       "planned contract",
				ContractStatus: "draft",
			})},
			Dir: repoRoot,
		},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, wantCommands)
	}
}

func TestRunOnceLaunchesGitWorkersConcurrently(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{"name": "first parallel candidate", "status": "planned", "contract": "first contract", "contract_status": "draft"}
						]
					},
					"12.B": {
						"items": [
							{"name": "second parallel candidate", "status": "planned", "contract": "second contract", "contract_status": "draft"}
						]
					}
				}
			}
		}
	}`)
	runRoot := t.TempDir()
	started := make(chan string, 2)
	firstBackend := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	runner := runnerFunc(func(ctx context.Context, command Command) Result {
		switch command.Name {
		case "opencode":
			started <- filepath.Base(command.Dir)
			select {
			case firstBackend <- struct{}{}:
				select {
				case <-releaseFirst:
				case <-ctx.Done():
					return Result{Err: ctx.Err()}
				}
			default:
			}
			return Result{}
		default:
			return Result{}
		}
	})

	done := make(chan error, 1)
	go func() {
		_, err := RunOnce(context.Background(), RunOptions{
			Config: Config{
				RepoRoot:     repoRoot,
				ProgressJSON: progressPath,
				RunRoot:      runRoot,
				Backend:      "opencode",
				Mode:         "safe",
				MaxAgents:    2,
			},
			Runner: runner,
		})
		done <- err
	}()

	var seen []string
	seen = append(seen, <-started)
	select {
	case worker := <-started:
		seen = append(seen, worker)
		close(releaseFirst)
	case <-time.After(2 * time.Second):
		close(releaseFirst)
		if err := <-done; err != nil {
			t.Fatalf("RunOnce() error = %v", err)
		}
		t.Fatalf("started workers = %#v, want a second worker to start before the first backend returns", seen)
	}

	if err := <-done; err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if seen[0] == seen[1] {
		t.Fatalf("started workers = %#v, want distinct worker worktrees", seen)
	}
}

func TestRunOncePassesExecutionMetadataPromptToBackend(t *testing.T) {
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{
								"name": "prompted candidate",
								"status": "planned",
								"priority": "P0",
								"contract": "Provider-neutral transcript contract",
								"contract_status": "fixture_ready",
								"slice_size": "medium",
								"execution_owner": "provider",
								"trust_class": ["system"],
								"degraded_mode": "provider status reports missing fixtures",
								"fixture": "internal/hermes/testdata/provider_transcripts",
								"source_refs": ["docs/content/upstream-hermes/source-study.md"],
								"ready_when": ["fixtures replay"],
								"not_ready_when": ["live provider call required"],
								"acceptance": ["go test ./internal/hermes passes"],
								"write_scope": ["internal/hermes/"],
								"test_commands": ["go test ./internal/hermes -count=1"],
								"done_signal": ["provider transcript replay passes"],
								"unblocks": ["Bedrock adapter"],
								"note": "Use captured transcript fixtures."
							}
						]
					}
				}
			}
		}
	}`)
	runner := &FakeRunner{
		Results: []Result{{}},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     t.TempDir(),
			ProgressJSON: progressPath,
			Backend:      "codexu",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: runner,
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}

	if got, want := len(runner.Commands), 1; got != want {
		t.Fatalf("Commands length = %d, want %d", got, want)
	}
	args := runner.Commands[0].Args
	if len(args) == 0 {
		t.Fatal("Command args are empty, want backend flags plus prompt")
	}
	prompt := args[len(args)-1]
	for _, want := range []string{
		"Mission:",
		"Selected task:",
		"12 / 12.A / prompted candidate",
		"Current status: planned",
		"Priority: P0",
		"Execution owner: provider",
		"Slice size: medium",
		"Selection reason: P0 handoff",
		"Contract: Provider-neutral transcript contract",
		"Contract status: fixture_ready",
		"Fixture: internal/hermes/testdata/provider_transcripts",
		"Trust class:",
		"- system",
		"Ready when:",
		"- fixtures replay",
		"Not ready when:",
		"- live provider call required",
		"Blocked by:",
		"- (none declared)",
		"Unblocks:",
		"- Bedrock adapter",
		"Allowed write scope:",
		"- internal/hermes/",
		"Required test commands:",
		"- go test ./internal/hermes -count=1",
		"Done signal:",
		"- provider transcript replay passes",
		"Acceptance:",
		"- go test ./internal/hermes passes",
		"Source references:",
		"- docs/content/upstream-hermes/source-study.md",
		"Degraded mode: provider status reports missing fixtures",
		"Note: Use captured transcript fixtures.",
		"Requirements:",
		"- Keep changes scoped to the selected task and its allowed write scope.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want %q", prompt, want)
		}
	}
}

func TestBuildWorkerPromptRendersDependencyMetadata(t *testing.T) {
	prompt := BuildWorkerPrompt(Candidate{
		PhaseID:    "12",
		SubphaseID: "12.A",
		ItemName:   "blocked candidate",
		Status:     "planned",
		BlockedBy:  []string{"Hermes fixtures"},
	})

	for _, want := range []string{
		"Blocked by:",
		"- Hermes fixtures",
		"Unblocks:",
		"- (none declared)",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want %q", prompt, want)
		}
	}
}

func TestBuildWorkerPromptRendersEmptyNote(t *testing.T) {
	prompt := BuildWorkerPrompt(Candidate{
		PhaseID:    "12",
		SubphaseID: "12.A",
		ItemName:   "empty note candidate",
		Status:     "planned",
	})

	if !strings.Contains(prompt, "Note: -") {
		t.Fatalf("prompt = %q, want empty note rendered as dash", prompt)
	}
}

func TestRunOnceReturnsBackendRunnerError(t *testing.T) {
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
							"items": [
								{"name": "planned run candidate", "status": "planned", "contract": "run contract", "contract_status": "draft"}
							]
						}
					}
			}
		}
	}`)
	wantErr := errors.New("backend failed")
	runner := &FakeRunner{
		Results: []Result{{Err: wantErr}},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     t.TempDir(),
			ProgressJSON: progressPath,
			Backend:      "opencode",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: runner,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunOnce() error = %v, want %v", err, wantErr)
	}
}

func TestRunOnceIncludesBackendStderrInError(t *testing.T) {
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
							"items": [
								{"name": "planned run candidate", "status": "planned", "contract": "run contract", "contract_status": "draft"}
							]
						}
					}
			}
		}
	}`)
	wantErr := errors.New("exit status 1")
	runner := &FakeRunner{
		Results: []Result{{Err: wantErr, Stderr: "No prompt provided via stdin.\n"}},
	}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     t.TempDir(),
			ProgressJSON: progressPath,
			Backend:      "codexu",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: runner,
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunOnce() error = %v, want wrapped %v", err, wantErr)
	}
	if !strings.Contains(err.Error(), "No prompt provided via stdin.") {
		t.Fatalf("RunOnce() error = %q, want backend stderr", err)
	}
}

func TestRunOnceWritesLedgerEvents(t *testing.T) {
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{"name": "ledger candidate", "status": "planned", "contract": "ledger contract", "contract_status": "draft"}
						]
					}
				}
			}
		}
	}`)
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
	var got []string
	for _, event := range events {
		got = append(got, event.Event)
	}
	want := []string{"run_started", "worker_claimed", "worker_success", "run_completed", "health_updated"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ledger events = %#v, want %#v", got, want)
	}
	if events[1].Worker != 1 || events[1].Task != "12/12.A/ledger candidate" || events[1].Status != "claimed" {
		t.Fatalf("claim event = %#v, want worker/task/status detail", events[1])
	}
}

func TestRunOnceWritesWorkerFailedLedgerEventBeforeReturningBackendError(t *testing.T) {
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{"name": "failing ledger candidate", "status": "planned", "contract": "failing ledger contract", "contract_status": "draft"}
						]
					}
				}
			}
		}
	}`)
	runRoot := t.TempDir()
	wantErr := errors.New("backend failed")
	runner := &FakeRunner{Results: []Result{{Err: wantErr, Stderr: "backend stderr"}}}

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
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunOnce() error = %v, want wrapped %v", err, wantErr)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var got []string
	for _, event := range events {
		got = append(got, event.Event+":"+event.Status)
	}
	want := []string{"run_started:started", "worker_claimed:claimed", "worker_failed:backend_failed", "health_updated:ok"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ledger events = %#v, want %#v", got, want)
	}
	if detail := events[2].Detail; !strings.Contains(detail, "backend failed") || !strings.Contains(detail, "backend stderr") {
		t.Fatalf("backend failure detail = %q, want error and stderr", detail)
	}
}

func TestRunOnceRefusesDirtyRepositoryBeforeWorkerLaunch(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{"name": "dirty preflight", "status": "planned", "contract": "dirty contract", "contract_status": "draft"}
						]
					}
				}
			}
		}
	}`)
	if err := os.WriteFile(filepath.Join(repoRoot, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runRoot := t.TempDir()
	runner := &FakeRunner{Results: []Result{{}}}

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     repoRoot,
			ProgressJSON: progressPath,
			RunRoot:      runRoot,
			Backend:      "opencode",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: runner,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want dirty worktree error")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("RunOnce() error = %q, want dirty worktree context", err)
	}
	if len(runner.Commands) != 0 {
		t.Fatalf("Commands length = %d, want 0", len(runner.Commands))
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var got []string
	for _, event := range events {
		got = append(got, event.Event+":"+event.Status)
	}
	want := []string{"run_started:started", "run_failed:worktree_dirty"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ledger events = %#v, want %#v", got, want)
	}
}

func TestRunOnceAutoCommitsDirtyRepositoryBeforePreflight(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)
	progressPath := writeProgressJSON(t, `{"phases": {}}`)
	if err := os.WriteFile(filepath.Join(repoRoot, "conflict.txt"), []byte("base changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "new-cycle-file.txt"), []byte("cycle artifact\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runRoot := t.TempDir()

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:                repoRoot,
			ProgressJSON:            progressPath,
			RunRoot:                 runRoot,
			Backend:                 "opencode",
			Mode:                    "safe",
			MaxAgents:               1,
			AutoCommitDirtyWorktree: true,
		},
		Runner: &FakeRunner{},
		Now:    time.Date(2026, 4, 25, 7, 8, 2, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if status := gitStatusPorcelain(t, repoRoot); status != "" {
		t.Fatalf("git status = %q, want clean checkpointed worktree", status)
	}
	if subject := gitLogSubject(t, repoRoot); subject != "autoloop: checkpoint dirty worktree 20260425T070802Z" {
		t.Fatalf("last commit subject = %q", subject)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var got []string
	for _, event := range events {
		got = append(got, event.Event+":"+event.Status)
	}
	want := []string{
		"run_started:started",
		"worktree_checkpoint_started:started",
		"worktree_checkpoint_committed:committed",
		"run_completed:completed",
		"health_updated:ok",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ledger events = %#v, want %#v", got, want)
	}
	if events[2].Commit == "" {
		t.Fatalf("checkpoint commit sha is empty")
	}
}

func TestRunOnceFailsWhenWorkerLeavesDirtyWorktree(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{"name": "dirty worker", "status": "planned", "contract": "dirty worker contract", "contract_status": "draft"}
						]
					}
				}
			}
		}
	}`)
	runRoot := t.TempDir()
	runner := runnerFunc(func(_ context.Context, command Command) Result {
		if err := os.WriteFile(filepath.Join(command.Dir, "worker-dirty.go"), []byte("package dirty\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return Result{}
	})

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     repoRoot,
			ProgressJSON: progressPath,
			RunRoot:      runRoot,
			Backend:      "opencode",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: runner,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want dirty worktree error")
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("RunOnce() error = %q, want dirty worktree context", err)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var got []string
	for _, event := range events {
		got = append(got, event.Event+":"+event.Status)
	}
	want := []string{"run_started:started", "worker_claimed:claimed", "worker_failed:worktree_dirty", "health_updated:ok"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ledger events = %#v, want %#v", got, want)
	}
}

func TestRunOnceRunsWorkerInIsolatedGitWorktree(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{
								"name": "isolated worker",
								"status": "planned",
								"contract": "isolation contract",
								"contract_status": "draft",
								"write_scope": ["internal/channels/whatsapp/"]
							}
						]
					}
				}
			}
		}
	}`)
	runRoot := t.TempDir()
	var workerDir string
	runner := runnerFunc(func(_ context.Context, command Command) Result {
		workerDir = command.Dir
		if workerDir == repoRoot {
			return Result{}
		}
		outsidePath := filepath.Join(workerDir, "www.gormes.ai", "internal", "site", "content.go")
		if err := os.MkdirAll(filepath.Dir(outsidePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(outsidePath, []byte("package site\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return Result{}
	})

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     repoRoot,
			ProgressJSON: progressPath,
			RunRoot:      runRoot,
			Backend:      "opencode",
			Mode:         "safe",
			MaxAgents:    1,
			MaxPhase:     12,
		},
		Runner: runner,
		Now:    time.Date(2026, 4, 25, 1, 40, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want dirty worker worktree error")
	}
	if workerDir == "" {
		t.Fatal("worker command was not captured")
	}
	if workerDir == repoRoot {
		t.Fatalf("worker command Dir = repo root %q, want isolated worktree", repoRoot)
	}
	wantDir := filepath.Join(runRoot, "worktrees", "20260425T014000Z", "w1")
	if workerDir != wantDir {
		t.Fatalf("worker command Dir = %q, want %q", workerDir, wantDir)
	}
	if !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("RunOnce() error = %q, want dirty worktree context", err)
	}
	if status := gitStatusPorcelain(t, repoRoot); status != "" {
		t.Fatalf("base repo status = %q, want clean", status)
	}
	if current := mustGitCurrentBranch(t, repoRoot); current != "master" {
		t.Fatalf("base repo branch = %q, want master", current)
	}
}

func TestRunOnceFailsWhenWorkerCommitsOutsideWriteScope(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{
								"name": "scope leaking worker",
								"status": "planned",
								"contract": "scope contract",
								"contract_status": "draft",
								"write_scope": ["allowed/"]
							}
						]
					}
				}
			}
		}
	}`)
	runRoot := t.TempDir()
	runner := runnerFunc(func(_ context.Context, command Command) Result {
		switch command.Name {
		case "opencode":
			outsidePath := filepath.Join(command.Dir, "outside", "scope.txt")
			if err := os.MkdirAll(filepath.Dir(outsidePath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(outsidePath, []byte("scope leak\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runGitCommand(t, command.Dir, "add", "outside/scope.txt")
			runGitCommand(t, command.Dir, "commit", "-m", "scope leak")
			return Result{}
		case "git", "gh":
			return Result{}
		default:
			return Result{Err: ErrUnexpectedCommand}
		}
	})

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     repoRoot,
			ProgressJSON: progressPath,
			RunRoot:      runRoot,
			Backend:      "opencode",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: runner,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want write scope violation")
	}
	if !strings.Contains(err.Error(), "outside declared write scope") {
		t.Fatalf("RunOnce() error = %q, want write scope violation context", err)
	}
	if !strings.Contains(err.Error(), "outside/scope.txt") {
		t.Fatalf("RunOnce() error = %q, want changed path in error", err)
	}
	if current := mustGitCurrentBranch(t, repoRoot); current != "master" {
		t.Fatalf("current branch = %q, want master restored", current)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var got []string
	for _, event := range events {
		got = append(got, event.Event+":"+event.Status)
	}
	want := []string{"run_started:started", "worker_claimed:claimed", "worker_failed:write_scope_violation", "health_updated:ok"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ledger events = %#v, want %#v", got, want)
	}
}

func TestRunOnceFailsWhenWorkerLeavesWorkerBranch(t *testing.T) {
	repoRoot := t.TempDir()
	initCleanRepo(t, repoRoot)
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{"name": "branch escaping worker", "status": "planned", "contract": "branch contract", "contract_status": "draft"}
						]
					}
				}
			}
		}
	}`)
	runRoot := t.TempDir()
	runner := runnerFunc(func(_ context.Context, command Command) Result {
		if command.Name == "opencode" {
			return ExecRunner{}.Run(context.Background(), Command{
				Name: "git",
				Args: []string{"switch", "-c", "escaped-worker"},
				Dir:  command.Dir,
			})
		}
		return Result{Err: ErrUnexpectedCommand}
	})

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     repoRoot,
			ProgressJSON: progressPath,
			RunRoot:      runRoot,
			Backend:      "opencode",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: runner,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want branch changed error")
	}
	if !strings.Contains(err.Error(), "worker branch changed") {
		t.Fatalf("RunOnce() error = %q, want branch changed context", err)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var got []string
	for _, event := range events {
		got = append(got, event.Event+":"+event.Status)
	}
	want := []string{"run_started:started", "worker_claimed:claimed", "worker_failed:branch_changed", "health_updated:ok"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ledger events = %#v, want %#v", got, want)
	}
}

func TestRunOnceFailsWhenWorkerLeavesMergeConflicts(t *testing.T) {
	repoRoot := t.TempDir()
	initConflictingRepo(t, repoRoot)
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{"name": "conflicting worker", "status": "planned", "contract": "conflict contract", "contract_status": "draft"}
						]
					}
				}
			}
		}
	}`)
	runRoot := t.TempDir()
	runner := runnerFunc(func(_ context.Context, command Command) Result {
		cmd := exec.Command("git", "-C", command.Dir, "merge", "worker-branch")
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("git merge unexpectedly succeeded:\n%s", output)
		}
		return Result{}
	})

	_, err := RunOnce(context.Background(), RunOptions{
		Config: Config{
			RepoRoot:     repoRoot,
			ProgressJSON: progressPath,
			RunRoot:      runRoot,
			Backend:      "opencode",
			Mode:         "safe",
			MaxAgents:    1,
		},
		Runner: runner,
	})
	if err == nil {
		t.Fatal("RunOnce() error = nil, want unresolved merge error")
	}
	if !strings.Contains(err.Error(), "unresolved merge conflicts") {
		t.Fatalf("RunOnce() error = %q, want unresolved merge conflict context", err)
	}

	events := readLedgerEvents(t, filepath.Join(runRoot, "state", "runs.jsonl"))
	var got []string
	for _, event := range events {
		got = append(got, event.Event+":"+event.Status)
	}
	want := []string{"run_started:started", "worker_claimed:claimed", "worker_failed:worktree_unmerged", "health_updated:ok"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ledger events = %#v, want %#v", got, want)
	}
}

type runnerFunc func(context.Context, Command) Result

func (fn runnerFunc) Run(ctx context.Context, command Command) Result {
	return fn(ctx, command)
}

func initConflictingRepo(t *testing.T, repoRoot string) {
	t.Helper()

	initCleanRepo(t, repoRoot)
	runGitCommand(t, repoRoot, "checkout", "-b", "worker-branch")
	if err := os.WriteFile(filepath.Join(repoRoot, "conflict.txt"), []byte("worker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, repoRoot, "commit", "-am", "worker")
	runGitCommand(t, repoRoot, "checkout", "master")
	if err := os.WriteFile(filepath.Join(repoRoot, "conflict.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, repoRoot, "commit", "-am", "main")
}

func initCleanRepo(t *testing.T, repoRoot string) {
	t.Helper()

	runGitCommand(t, repoRoot, "init")
	runGitCommand(t, repoRoot, "config", "user.email", "test@example.com")
	runGitCommand(t, repoRoot, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repoRoot, "conflict.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, repoRoot, "add", "conflict.txt")
	runGitCommand(t, repoRoot, "commit", "-m", "base")
}

func runGitCommand(t *testing.T, repoRoot string, args ...string) {
	t.Helper()

	cmdArgs := append([]string{"-C", repoRoot}, args...)
	cmd := exec.Command("git", cmdArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func mustGitCurrentBranch(t *testing.T, repoRoot string) string {
	t.Helper()

	out, err := exec.Command("git", "-C", repoRoot, "branch", "--show-current").Output()
	if err != nil {
		t.Fatalf("git branch --show-current failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func gitStatusPorcelain(t *testing.T, repoRoot string) string {
	t.Helper()

	out, err := exec.Command("git", "-C", repoRoot, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status --porcelain failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func gitLogSubject(t *testing.T, repoRoot string) string {
	t.Helper()

	out, err := exec.Command("git", "-C", repoRoot, "log", "-1", "--pretty=%s").Output()
	if err != nil {
		t.Fatalf("git log -1 --pretty=%%s failed: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func readLedgerEvents(t *testing.T, path string) []LedgerEvent {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	var events []LedgerEvent
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var event LedgerEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("Unmarshal(%q) error = %v", line, err)
		}
		events = append(events, event)
	}
	return events
}
