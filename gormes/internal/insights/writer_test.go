package insights

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsageWriterAppendsOneJSONLRecordPerDailyRollupInCallOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	writer := NewUsageWriter(path)

	first := DailyRollup{
		Date:             "2026-04-22",
		SessionCount:     2,
		TotalTokensIn:    190,
		TotalTokensOut:   75,
		EstimatedCostUSD: 0.034,
		ModelBreakdown: map[string]int{
			"claude-opus": 1,
			"gpt-4":       1,
		},
	}
	second := DailyRollup{
		Date:             "2026-04-23",
		SessionCount:     1,
		TotalTokensIn:    50,
		TotalTokensOut:   20,
		EstimatedCostUSD: 0.009,
		ModelBreakdown: map[string]int{
			"gpt-4": 1,
		},
	}

	if err := writer.Record(first); err != nil {
		t.Fatalf("Record(first) error = %v", err)
	}
	if err := writer.Record(second); err != nil {
		t.Fatalf("Record(second) error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(lines))
	}

	var gotFirst, gotSecond DailyRollup
	if err := json.Unmarshal([]byte(lines[0]), &gotFirst); err != nil {
		t.Fatalf("Unmarshal(first) error = %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &gotSecond); err != nil {
		t.Fatalf("Unmarshal(second) error = %v", err)
	}
	if gotFirst.Date != "2026-04-22" {
		t.Fatalf("first date = %q, want 2026-04-22", gotFirst.Date)
	}
	if gotSecond.Date != "2026-04-23" {
		t.Fatalf("second date = %q, want 2026-04-23", gotSecond.Date)
	}
	if gotFirst.TotalTokensIn != 190 || gotSecond.TotalTokensOut != 20 {
		t.Fatalf("unexpected decoded records: first=%+v second=%+v", gotFirst, gotSecond)
	}
}

func TestUsageWriterNormalizesSchemaFriendlyDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.jsonl")
	writer := NewUsageWriter(path)

	if err := writer.Record(DailyRollup{
		Date:             "2026-04-24",
		SessionCount:     -3,
		TotalTokensIn:    -10,
		TotalTokensOut:   -20,
		EstimatedCostUSD: 0.01234567,
		ModelBreakdown:   nil,
	}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	var got DailyRollup
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.SessionCount != 0 {
		t.Fatalf("SessionCount = %d, want 0", got.SessionCount)
	}
	if got.TotalTokensIn != 0 || got.TotalTokensOut != 0 {
		t.Fatalf("token totals = (%d, %d), want (0, 0)", got.TotalTokensIn, got.TotalTokensOut)
	}
	if got.EstimatedCostUSD != 0.012346 {
		t.Fatalf("EstimatedCostUSD = %v, want 0.012346", got.EstimatedCostUSD)
	}
	if got.ModelBreakdown == nil {
		t.Fatal("ModelBreakdown = nil, want empty object/map")
	}
	if len(got.ModelBreakdown) != 0 {
		t.Fatalf("ModelBreakdown len = %d, want 0", len(got.ModelBreakdown))
	}
}
