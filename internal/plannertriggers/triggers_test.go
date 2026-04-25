package plannertriggers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendTriggerEvent_GeneratesIDIfEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triggers.jsonl")
	if err := AppendTriggerEvent(path, TriggerEvent{Kind: "quarantine_added"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(body) == 0 || body[len(body)-1] != '\n' {
		t.Fatalf("expected trailing newline, got %q", body)
	}
	var ev TriggerEvent
	if err := json.Unmarshal(body[:len(body)-1], &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.ID == "" {
		t.Fatal("AppendTriggerEvent should generate an ID when empty")
	}
	if ev.TS == "" {
		t.Fatal("AppendTriggerEvent should generate a TS when empty")
	}
	if ev.Source != "builder-loop" {
		t.Fatalf("Source = %q, want builder-loop default", ev.Source)
	}
}

func TestAppendTriggerEvent_PreservesExplicitFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triggers.jsonl")
	in := TriggerEvent{
		ID:     "explicit-id",
		TS:     "2026-04-24T12:00:00Z",
		Source: "manual",
		Kind:   "manual",
	}
	if err := AppendTriggerEvent(path, in); err != nil {
		t.Fatalf("Append: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var ev TriggerEvent
	if err := json.Unmarshal(body[:len(body)-1], &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.ID != "explicit-id" {
		t.Fatalf("ID overwritten: %q", ev.ID)
	}
	if ev.TS != "2026-04-24T12:00:00Z" {
		t.Fatalf("TS overwritten: %q", ev.TS)
	}
	if ev.Source != "manual" {
		t.Fatalf("Source overwritten: %q", ev.Source)
	}
}

func TestAppendTriggerEvent_CreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "triggers.jsonl")
	if err := AppendTriggerEvent(path, TriggerEvent{Kind: "quarantine_added"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at nested path, got: %v", err)
	}
}

func TestReadTriggersSinceCursor_EmptyCursorReturnsAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triggers.jsonl")
	for i := 0; i < 3; i++ {
		if err := AppendTriggerEvent(path, TriggerEvent{Kind: "quarantine_added", PhaseID: "p", ItemName: "i"}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	events, err := ReadTriggersSinceCursor(path, TriggerCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3, got %d", len(events))
	}
}

func TestReadTriggersSinceCursor_AdvancesPastCursor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triggers.jsonl")
	for i := 0; i < 5; i++ {
		if err := AppendTriggerEvent(path, TriggerEvent{Kind: "quarantine_added"}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	all, err := ReadTriggersSinceCursor(path, TriggerCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5 baseline events, got %d", len(all))
	}
	cursor := TriggerCursor{LastConsumedID: all[2].ID}
	events, err := ReadTriggersSinceCursor(path, cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events past cursor, got %d", len(events))
	}
	if events[0].ID != all[3].ID || events[1].ID != all[4].ID {
		t.Fatalf("unexpected events past cursor: %+v", events)
	}
}

func TestReadTriggersSinceCursor_StaleCursorReturnsAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triggers.jsonl")
	for i := 0; i < 2; i++ {
		if err := AppendTriggerEvent(path, TriggerEvent{Kind: "quarantine_added"}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	events, err := ReadTriggersSinceCursor(path, TriggerCursor{LastConsumedID: "no-such-id"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events with stale cursor, got %d", len(events))
	}
}

func TestReadTriggersSinceCursor_MissingFileReturnsNil(t *testing.T) {
	dir := t.TempDir()
	events, err := ReadTriggersSinceCursor(filepath.Join(dir, "missing.jsonl"), TriggerCursor{})
	if err != nil {
		t.Fatalf("ReadTriggersSinceCursor missing file should not error: %v", err)
	}
	if events != nil {
		t.Fatalf("expected nil events for missing file, got %d", len(events))
	}
}

func TestReadTriggersSinceCursor_SkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "triggers.jsonl")
	if err := AppendTriggerEvent(path, TriggerEvent{Kind: "quarantine_added"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := f.WriteString("garbage not json\n"); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	f.Close()
	if err := AppendTriggerEvent(path, TriggerEvent{Kind: "quarantine_stale_cleared"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	events, err := ReadTriggersSinceCursor(path, TriggerCursor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 valid events (garbage skipped), got %d", len(events))
	}
}

func TestSaveCursor_AtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cursor.json")
	cursor := TriggerCursor{LastConsumedID: "abc", LastReadAt: time.Now().UTC().Format(time.RFC3339)}
	if err := SaveCursor(path, cursor); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCursor(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastConsumedID != "abc" {
		t.Fatalf("LastConsumedID = %q", got.LastConsumedID)
	}
	if got.LastReadAt != cursor.LastReadAt {
		t.Fatalf("LastReadAt = %q, want %q", got.LastReadAt, cursor.LastReadAt)
	}
}

func TestSaveCursor_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cursor.json")
	if err := SaveCursor(path, TriggerCursor{LastConsumedID: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveCursor(path, TriggerCursor{LastConsumedID: "second"}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCursor(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastConsumedID != "second" {
		t.Fatalf("LastConsumedID = %q, want second", got.LastConsumedID)
	}
}

func TestLoadCursor_MissingFileReturnsZeroValue(t *testing.T) {
	dir := t.TempDir()
	cursor, err := LoadCursor(filepath.Join(dir, "nonexistent.json"))
	if err != nil {
		t.Fatalf("LoadCursor missing file should not error, got: %v", err)
	}
	if cursor.LastConsumedID != "" || cursor.LastReadAt != "" {
		t.Fatalf("expected zero-value cursor, got %+v", cursor)
	}
}
