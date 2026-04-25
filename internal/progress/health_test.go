package progress

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRowHealth_RoundTrip(t *testing.T) {
	row := &RowHealth{
		AttemptCount:        5,
		ConsecutiveFailures: 3,
		LastAttempt:         "2026-04-24T12:00:00Z",
		LastSuccess:         "2026-04-23T08:00:00Z",
		LastFailure: &FailureSummary{
			RunID:      "20260424T120000Z-1234-001",
			Category:   FailureReportValidation,
			Backend:    "codexu",
			StderrTail: "tests failed",
		},
		BackendsTried: []string{"codexu", "claudeu"},
		Quarantine: &Quarantine{
			Reason:       "auto: 3 consecutive failures",
			Since:        "2026-04-24T12:05:00Z",
			AfterRunID:   "20260424T120500Z-1234-001",
			Threshold:    3,
			SpecHash:     "abc123",
			LastCategory: FailureReportValidation,
		},
	}

	data, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got RowHealth
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.AttemptCount != 5 {
		t.Fatalf("AttemptCount = %d, want 5", got.AttemptCount)
	}
	if got.Quarantine == nil || got.Quarantine.SpecHash != "abc123" {
		t.Fatalf("Quarantine.SpecHash mismatch: %+v", got.Quarantine)
	}
	if got.LastFailure == nil || got.LastFailure.Category != FailureReportValidation {
		t.Fatalf("LastFailure.Category mismatch: %+v", got.LastFailure)
	}
}

func TestRowHealth_OmitemptyKeepsZeroFieldsOut(t *testing.T) {
	row := &RowHealth{}
	data, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != "{}" {
		t.Fatalf("zero-value RowHealth should marshal to {}, got %s", data)
	}
}

func TestItem_HealthOmitemptyByDefault(t *testing.T) {
	item := &Item{Name: "x", Status: StatusPlanned}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "health") {
		t.Fatalf("Item with no health should not emit health key, got %s", data)
	}
}

func TestItemSpecHash_StableAcrossInvocations(t *testing.T) {
	item := &Item{
		Name:           "row-a",
		Status:         StatusInProgress,
		Contract:       "do the thing",
		ContractStatus: ContractStatusFixtureReady,
		BlockedBy:      []string{"row-b", "row-c"},
		WriteScope:     []string{"internal/foo/", "internal/bar/"},
		Fixture:        "internal/foo/foo_test.go",
	}
	a := ItemSpecHash(item)
	b := ItemSpecHash(item)
	if a != b {
		t.Fatalf("hash not deterministic: %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("expected sha256 hex (64 chars), got %d: %s", len(a), a)
	}
}

func TestItemSpecHash_IgnoresBlockedByOrdering(t *testing.T) {
	a := &Item{Name: "x", BlockedBy: []string{"a", "b", "c"}}
	b := &Item{Name: "x", BlockedBy: []string{"c", "b", "a"}}
	if ItemSpecHash(a) != ItemSpecHash(b) {
		t.Fatal("hash should be order-independent for BlockedBy")
	}
}

func TestItemSpecHash_IgnoresStatusAndName(t *testing.T) {
	a := &Item{Name: "row-a", Status: StatusPlanned, Contract: "x"}
	b := &Item{Name: "row-b", Status: StatusComplete, Contract: "x"}
	if ItemSpecHash(a) != ItemSpecHash(b) {
		t.Fatal("hash should ignore Name and Status")
	}
}

func TestItemSpecHash_ChangesWhenContractChanges(t *testing.T) {
	a := &Item{Name: "x", Contract: "old"}
	b := &Item{Name: "x", Contract: "new"}
	if ItemSpecHash(a) == ItemSpecHash(b) {
		t.Fatal("hash should change when Contract changes")
	}
}

func writeProgressJSON(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

const minimalProgress = `{
  "version": "1",
  "phases": {
    "1": {
      "name": "Phase One",
      "subphases": {
        "1.A": {
          "name": "Sub A",
          "items": [
            {"name": "item-1", "status": "planned"}
          ]
        }
      }
    }
  }
}
`

func TestApplyHealthUpdates_AddsHealthBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeProgressJSON(t, path, minimalProgress)

	err := ApplyHealthUpdates(path, []HealthUpdate{
		{
			PhaseID:    "1",
			SubphaseID: "1.A",
			ItemName:   "item-1",
			Mutate: func(h *RowHealth) {
				h.AttemptCount = 2
				h.ConsecutiveFailures = 2
				h.LastAttempt = "2026-04-24T12:00:00Z"
			},
		},
	})
	if err != nil {
		t.Fatalf("ApplyHealthUpdates: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(body), `"health":`) {
		t.Fatalf("expected health block in output, got:\n%s", body)
	}
	if !strings.Contains(string(body), `"attempt_count": 2`) {
		t.Fatalf("expected attempt_count=2, got:\n%s", body)
	}
}

func TestApplyHealthUpdates_PreservesOtherRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeProgressJSON(t, path, `{
  "version": "1",
  "phases": {
    "1": {
      "name": "Phase One",
      "subphases": {
        "1.A": {
          "name": "Sub A",
          "items": [
            {"name": "item-1", "status": "planned", "contract": "do A"},
            {"name": "item-2", "status": "in_progress", "contract": "do B"}
          ]
        }
      }
    }
  }
}
`)

	err := ApplyHealthUpdates(path, []HealthUpdate{
		{PhaseID: "1", SubphaseID: "1.A", ItemName: "item-2", Mutate: func(h *RowHealth) {
			h.AttemptCount = 1
		}},
	})
	if err != nil {
		t.Fatalf("ApplyHealthUpdates: %v", err)
	}

	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), `"contract": "do A"`) {
		t.Fatalf("item-1 contract dropped:\n%s", body)
	}
	if !strings.Contains(string(body), `"contract": "do B"`) {
		t.Fatalf("item-2 contract dropped:\n%s", body)
	}
}

func TestApplyHealthUpdates_UnknownRowReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	writeProgressJSON(t, path, minimalProgress)

	err := ApplyHealthUpdates(path, []HealthUpdate{
		{PhaseID: "9", SubphaseID: "9.Z", ItemName: "ghost", Mutate: func(h *RowHealth) {}},
	})
	if err == nil {
		t.Fatal("expected error when target row does not exist")
	}
}

func TestPlannerVerdict_RoundTrip(t *testing.T) {
	verdict := &PlannerVerdict{
		NeedsHuman:   true,
		Reason:       "auto: 3 reshapes without unsticking; last category report_validation_failed",
		Since:        "2026-04-24T12:00:00Z",
		ReshapeCount: 3,
		LastReshape:  "2026-04-24T11:00:00Z",
		LastOutcome:  "still_failing",
	}

	data, err := json.Marshal(verdict)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got PlannerVerdict
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.NeedsHuman {
		t.Fatal("NeedsHuman should round-trip true")
	}
	if got.ReshapeCount != 3 {
		t.Fatalf("ReshapeCount = %d, want 3", got.ReshapeCount)
	}
	if got.LastOutcome != "still_failing" {
		t.Fatalf("LastOutcome = %q, want still_failing", got.LastOutcome)
	}
}

func TestPlannerVerdict_OmitemptyKeepsZeroFieldsOut(t *testing.T) {
	v := &PlannerVerdict{}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != "{}" {
		t.Fatalf("zero-value PlannerVerdict should marshal to {}, got %s", data)
	}
}

func TestItem_PlannerVerdictOmitemptyByDefault(t *testing.T) {
	item := &Item{Name: "x", Status: StatusPlanned}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "planner_verdict") {
		t.Fatalf("Item with no verdict should not emit planner_verdict key, got %s", data)
	}
}
