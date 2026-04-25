package autoloop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
            {"name": "row-ok", "status": "planned", "contract": "do y", "contract_status": "draft"}
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

	runner := &FakeRunner{Results: []Result{{}, {}}}

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
