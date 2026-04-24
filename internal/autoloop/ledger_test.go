package autoloop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendLedgerEventWritesJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "ledger.jsonl")
	event := LedgerEvent{
		TS:     time.Unix(123, 0).UTC(),
		Event:  "claim",
		Worker: 2,
		Task:   "task-10",
		Status: "started",
	}

	if err := AppendLedgerEvent(path, event); err != nil {
		t.Fatalf("AppendLedgerEvent() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatal("ledger event missing trailing newline")
	}

	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("ledger line count = %d, want 1", len(lines))
	}

	var got LedgerEvent
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !got.TS.Equal(event.TS) {
		t.Fatalf("TS = %v, want %v", got.TS, event.TS)
	}
	if got.Event != event.Event {
		t.Fatalf("Event = %q, want %q", got.Event, event.Event)
	}
	if got.Worker != event.Worker {
		t.Fatalf("Worker = %d, want %d", got.Worker, event.Worker)
	}
	if got.Task != event.Task {
		t.Fatalf("Task = %q, want %q", got.Task, event.Task)
	}
	if got.Status != event.Status {
		t.Fatalf("Status = %q, want %q", got.Status, event.Status)
	}
}

func TestAppendLedgerEventAppendsTwoJSONLRecordsInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "ledger.jsonl")
	events := []LedgerEvent{
		{
			TS:     time.Unix(123, 0).UTC(),
			Event:  "claim",
			Worker: 1,
			Task:   "task-10",
			Status: "started",
		},
		{
			TS:     time.Unix(124, 0).UTC(),
			Event:  "claim",
			Worker: 1,
			Task:   "task-10",
			Status: "finished",
		},
	}

	for _, event := range events {
		if err := AppendLedgerEvent(path, event); err != nil {
			t.Fatalf("AppendLedgerEvent() error = %v", err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatal("ledger events missing trailing newline")
	}

	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("ledger line count = %d, want 2", len(lines))
	}

	for i, line := range lines {
		var got LedgerEvent
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("Unmarshal(line %d) error = %v", i, err)
		}
		want := events[i]
		if !got.TS.Equal(want.TS) {
			t.Fatalf("line %d TS = %v, want %v", i, got.TS, want.TS)
		}
		if got.Event != want.Event {
			t.Fatalf("line %d Event = %q, want %q", i, got.Event, want.Event)
		}
		if got.Worker != want.Worker {
			t.Fatalf("line %d Worker = %d, want %d", i, got.Worker, want.Worker)
		}
		if got.Task != want.Task {
			t.Fatalf("line %d Task = %q, want %q", i, got.Task, want.Task)
		}
		if got.Status != want.Status {
			t.Fatalf("line %d Status = %q, want %q", i, got.Status, want.Status)
		}
	}
}
