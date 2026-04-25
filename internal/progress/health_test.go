package progress

import (
	"encoding/json"
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
