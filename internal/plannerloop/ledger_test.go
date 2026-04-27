package plannerloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLedgerEvent_RoundTrip(t *testing.T) {
	event := LedgerEvent{
		TS:            "2026-04-25T10:00:00Z",
		RunID:         "20260425T100000Z",
		Trigger:       "event",
		TriggerEvents: []string{"trig-1", "trig-2"},
		Backend:       "codexu",
		Mode:          "safe",
		Status:        "ok",
		BeforeStats:   ProgressStats{Shipped: 10, Planned: 50, Quarantined: 2},
		AfterStats:    ProgressStats{Shipped: 11, Planned: 49, Quarantined: 1},
		RowsChanged: []RowChange{
			{PhaseID: "2", SubphaseID: "2.B", ItemName: "row-1", Kind: "spec_changed"},
		},
		Keywords: []string{"honcho"},
	}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got LedgerEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.RunID != "20260425T100000Z" || got.Trigger != "event" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if len(got.RowsChanged) != 1 || got.RowsChanged[0].Kind != "spec_changed" {
		t.Fatalf("RowsChanged round-trip failed: %+v", got.RowsChanged)
	}
}

func TestAppendLedgerEvent_AppendsOneJSONLineAndIsParseable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	for i := 0; i < 3; i++ {
		err := AppendLedgerEvent(path, LedgerEvent{
			TS:     time.Date(2026, 4, 25, 10, i, 0, 0, time.UTC).Format(time.RFC3339),
			RunID:  "run-" + string(rune('A'+i)),
			Status: "ok",
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	body, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), body)
	}
	for i, line := range lines {
		var event LedgerEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("line %d not parseable JSON: %v\n%s", i, err, line)
		}
	}
}

func TestAppendLedgerEvent_AppendsAtomicallyAcrossWriters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	const N = 8
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = AppendLedgerEvent(path, LedgerEvent{
				TS:     time.Now().UTC().Format(time.RFC3339Nano),
				RunID:  "run-" + string(rune('A'+idx)),
				Status: "ok",
			})
		}(i)
	}
	wg.Wait()
	events, err := LoadLedger(path)
	if err != nil {
		t.Fatalf("LoadLedger: %v", err)
	}
	if len(events) != N {
		t.Fatalf("got %d events, want %d", len(events), N)
	}
}

func TestAppendLedgerEvent_QuarantinesTrailingPartialLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	if err := os.WriteFile(path, []byte(`{"ts`), 0o644); err != nil {
		t.Fatalf("write partial ledger: %v", err)
	}

	if err := AppendLedgerEvent(path, LedgerEvent{
		TS:     "2026-04-27T03:00:00Z",
		RunID:  "after-partial",
		Status: "ok",
	}); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("ledger lines = %d, want only valid event:\n%s", len(lines), body)
	}
	var event LedgerEvent
	if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
		t.Fatalf("ledger line is not valid event JSON: %v\n%s", err, lines[0])
	}
	if event.RunID != "after-partial" {
		t.Fatalf("RunID = %q, want after-partial", event.RunID)
	}
	corruptBody, err := os.ReadFile(path + ".corrupt")
	if err != nil {
		t.Fatalf("read corrupt sidecar: %v", err)
	}
	if !strings.Contains(string(corruptBody), `{"ts`) || !strings.Contains(string(corruptBody), "trailing_partial_line") {
		t.Fatalf("corrupt sidecar missing partial evidence:\n%s", corruptBody)
	}
}

func TestLoadLedger_SkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	good1 := `{"ts":"2026-04-25T10:00:00Z","run_id":"a","status":"ok"}`
	bad := `{this is not json`
	good2 := `{"ts":"2026-04-25T10:01:00Z","run_id":"b","status":"ok"}`
	if err := os.WriteFile(path, []byte(good1+"\n"+bad+"\n"+good2+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	events, err := LoadLedger(path)
	if err != nil {
		t.Fatalf("LoadLedger: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 good events, got %d", len(events))
	}
}

func TestLoadLedgerWindow_BoundsByTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs.jsonl")
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	for i := -10; i <= 0; i++ {
		_ = AppendLedgerEvent(path, LedgerEvent{
			TS:     now.Add(time.Duration(i) * 24 * time.Hour).Format(time.RFC3339),
			RunID:  "run",
			Status: "ok",
		})
	}
	events, err := LoadLedgerWindow(path, 7*24*time.Hour, now)
	if err != nil {
		t.Fatalf("LoadLedgerWindow: %v", err)
	}
	// Window includes events from 7 days ago to now → 8 events (-7..0).
	if len(events) != 8 {
		t.Fatalf("expected 8 events in 7-day window, got %d", len(events))
	}
}
