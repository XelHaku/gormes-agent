package autoloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/internal/progress"
)

func writeHealthProgress(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestNormalizeCandidates_NoHealthBehavesLikeBaseline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeHealthProgress(t, path, `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-a", "status": "planned", "contract": "do a", "contract_status": "draft"},
            {"name": "row-b", "status": "planned", "contract": "do b", "contract_status": "draft"}
          ]
        }
      }
    }
  }
}
`)

	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("NormalizeCandidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2", len(got))
	}
}

func TestNormalizeCandidates_QuarantineFiltersByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeHealthProgress(t, path, `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-a", "status": "planned", "contract": "do a", "contract_status": "draft"},
            {"name": "row-b", "status": "planned", "contract": "do b", "contract_status": "draft"}
          ]
        }
      }
    }
  }
}
`)

	// Quarantine row-a with the CURRENT spec hash so it's not stale.
	prog, err := progress.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	currentHash := progress.ItemSpecHash(&prog.Phases["1"].Subphases["1.A"].Items[0])
	if err := progress.ApplyHealthUpdates(path, []progress.HealthUpdate{{
		PhaseID: "1", SubphaseID: "1.A", ItemName: "row-a",
		Mutate: func(h *progress.RowHealth) {
			h.ConsecutiveFailures = 3
			h.Quarantine = &progress.Quarantine{Threshold: 3, SpecHash: currentHash}
		},
	}}); err != nil {
		t.Fatalf("seed quarantine: %v", err)
	}

	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("NormalizeCandidates: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 (row-b), got %d", len(got))
	}
	if got[0].ItemName != "row-b" {
		t.Fatalf("got %q, want row-b", got[0].ItemName)
	}
}

func TestNormalizeCandidates_IncludeQuarantinedReturnsAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeHealthProgress(t, path, `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-a", "status": "planned", "contract": "do a", "contract_status": "draft"},
            {"name": "row-b", "status": "planned", "contract": "do b", "contract_status": "draft"}
          ]
        }
      }
    }
  }
}
`)

	prog, _ := progress.Load(path)
	currentHash := progress.ItemSpecHash(&prog.Phases["1"].Subphases["1.A"].Items[0])
	if err := progress.ApplyHealthUpdates(path, []progress.HealthUpdate{{
		PhaseID: "1", SubphaseID: "1.A", ItemName: "row-a",
		Mutate: func(h *progress.RowHealth) {
			h.ConsecutiveFailures = 3
			h.Quarantine = &progress.Quarantine{Threshold: 3, SpecHash: currentHash}
		},
	}}); err != nil {
		t.Fatalf("seed quarantine: %v", err)
	}

	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true, IncludeQuarantined: true})
	if err != nil {
		t.Fatalf("NormalizeCandidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected both, got %d", len(got))
	}
}

func TestNormalizeCandidates_StaleQuarantineFlagsAndIncludes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeHealthProgress(t, path, `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-a", "status": "planned", "contract": "do a", "contract_status": "draft"}
          ]
        }
      }
    }
  }
}
`)

	// Quarantine with a SpecHash that does NOT match the current spec.
	if err := progress.ApplyHealthUpdates(path, []progress.HealthUpdate{{
		PhaseID: "1", SubphaseID: "1.A", ItemName: "row-a",
		Mutate: func(h *progress.RowHealth) {
			h.ConsecutiveFailures = 5
			h.Quarantine = &progress.Quarantine{Threshold: 3, SpecHash: "completely-stale-hash"}
		},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("NormalizeCandidates: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected stale quarantine to surface candidate, got %d", len(got))
	}
	if !got[0].StaleQuarantine {
		t.Fatal("StaleQuarantine flag should be true")
	}
}

func TestNormalizeCandidates_NeedsHumanSkippedByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeHealthProgress(t, path, `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-a", "status": "planned", "contract": "do a", "contract_status": "draft",
             "planner_verdict": {"needs_human": true, "reason": "auto", "since": "2026-04-25T10:00:00Z"}},
            {"name": "row-b", "status": "planned", "contract": "do b", "contract_status": "draft"}
          ]
        }
      }
    }
  }
}
`)
	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ItemName != "row-b" {
		t.Fatalf("expected only row-b, got %+v", got)
	}
}

func TestNormalizeCandidates_IncludeNeedsHumanSurfacesAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeHealthProgress(t, path, `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-a", "status": "planned", "contract": "do a", "contract_status": "draft",
             "planner_verdict": {"needs_human": true}}
          ]
        }
      }
    }
  }
}
`)
	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true, IncludeNeedsHuman: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(got))
	}
	if !got[0].NeedsHumanFlag {
		t.Fatal("NeedsHumanFlag should be true")
	}
	if reason := got[0].SelectionReason(); !strings.Contains(reason, "needs_human_visible") {
		t.Fatalf("SelectionReason missing needs_human_visible; got: %s", reason)
	}
}

func TestFailurePenalty_TableDriven(t *testing.T) {
	cases := []struct {
		consecutive int
		want        int
	}{
		{0, 0},
		{1, 5},
		{2, 20},
		{3, 45},
		{10, 45},
	}
	for _, c := range cases {
		got := failurePenalty(c.consecutive)
		if got != c.want {
			t.Errorf("failurePenalty(%d) = %d, want %d", c.consecutive, got, c.want)
		}
	}
}

func TestNormalizeCandidates_PenaltyDemotesAndAnnotates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeHealthProgress(t, path, `{
  "version": "1",
  "phases": {
    "1": {
      "name": "P",
      "subphases": {
        "1.A": {
          "name": "S",
          "items": [
            {"name": "row-a", "status": "planned", "contract": "do a", "contract_status": "draft"},
            {"name": "row-b", "status": "planned", "contract": "do b", "contract_status": "draft"}
          ]
        }
      }
    }
  }
}
`)

	// Seed row-a with 2 consecutive failures and 1 backend tried.
	// Penalty math: failurePenalty(2) + 2*len([]) = 20 + 2 = 22 (with 1 backend).
	if err := progress.ApplyHealthUpdates(path, []progress.HealthUpdate{{
		PhaseID: "1", SubphaseID: "1.A", ItemName: "row-a",
		Mutate: func(h *progress.RowHealth) {
			h.ConsecutiveFailures = 2
			h.BackendsTried = []string{"codexu"}
		},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := NormalizeCandidates(path, CandidateOptions{ActiveFirst: true})
	if err != nil {
		t.Fatalf("NormalizeCandidates: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(got))
	}

	// row-b (no penalty) should sort BEFORE row-a (penalized).
	if got[0].ItemName != "row-b" {
		t.Fatalf("unpenalized row should sort first; got order [%s, %s]", got[0].ItemName, got[1].ItemName)
	}

	// row-a's PenaltyApplied must be populated: failurePenalty(2)=20 + 2*1=2 → 22.
	const wantPenalty = 22
	if got[1].PenaltyApplied != wantPenalty {
		t.Fatalf("row-a PenaltyApplied = %d, want %d", got[1].PenaltyApplied, wantPenalty)
	}

	// SelectionReason must surface the penalty annotation.
	reason := got[1].SelectionReason()
	if !strings.Contains(reason, "penalty=22") {
		t.Fatalf("row-a SelectionReason missing penalty=22; got: %s", reason)
	}
}
