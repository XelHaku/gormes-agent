package autoloop

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDryRunSelectsCandidatesWithoutRunningBackend(t *testing.T) {
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"12": {
				"subphases": {
					"12.A": {
						"items": [
							{"item_name": "planned run candidate", "status": "planned"}
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
		{PhaseID: "12", SubphaseID: "12.A", ItemName: "planned run candidate", Status: "planned"},
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

func TestDryRunSkipsCandidatesAboveConfiguredMaxPhase(t *testing.T) {
	progressPath := writeProgressJSON(t, `{
		"phases": {
			"3": {
				"subphases": {
					"3.E": {
						"items": [
							{"item_name": "phase 3 candidate", "status": "planned"}
						]
					}
				}
			},
			"4": {
				"subphases": {
					"4.A": {
						"items": [
							{"item_name": "phase 4 active candidate", "status": "in_progress"}
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
		{PhaseID: "3", SubphaseID: "3.E", ItemName: "phase 3 candidate", Status: "planned"},
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
							{"item_name": "active candidate", "status": "in_progress"},
							{"item_name": "planned candidate", "status": "planned"},
							{"item_name": "deferred candidate", "status": "deferred"}
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

	if summary.Candidates != 3 {
		t.Fatalf("Candidates = %d, want 3", summary.Candidates)
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
			})},
			Dir: repoRoot,
		},
		{
			Name: "opencode",
			Args: []string{"run", "--no-interactive", BuildWorkerPrompt(Candidate{
				PhaseID:    "12",
				SubphaseID: "12.A",
				ItemName:   "planned candidate",
				Status:     "planned",
			})},
			Dir: repoRoot,
		},
	}
	if !reflect.DeepEqual(runner.Commands, wantCommands) {
		t.Fatalf("Commands = %#v, want %#v", runner.Commands, wantCommands)
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
								"item_name": "prompted candidate",
								"status": "planned",
								"priority": "P0",
								"contract": "Provider-neutral transcript contract",
								"contract_status": "draft",
								"slice_size": "medium",
								"execution_owner": "provider",
								"trust_class": ["system"],
								"degraded_mode": "provider status reports missing fixtures",
								"fixture": "captured provider transcript fixture",
								"source_refs": ["docs/content/upstream-hermes/source-study.md"],
								"ready_when": ["provider transcript fixtures are captured"],
								"not_ready_when": ["live provider calls are required"],
								"acceptance": ["provider transcript replay passes"],
								"write_scope": ["internal/hermes/"],
								"test_commands": ["go test ./internal/hermes -count=1"],
								"done_signal": ["provider transcript replay passes"],
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
		"Contract: Provider-neutral transcript contract",
		"Trust class:",
		"- system",
		"Allowed write scope:",
		"- internal/hermes/",
		"Required test commands:",
		"- go test ./internal/hermes -count=1",
		"Done signal:",
		"- provider transcript replay passes",
		"Source references:",
		"- docs/content/upstream-hermes/source-study.md",
		"Degraded mode: provider status reports missing fixtures",
		"Note: Use captured transcript fixtures.",
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
							{"item_name": "planned run candidate", "status": "planned"}
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
							{"item_name": "planned run candidate", "status": "planned"}
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
