package audit

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJSONLWriterAppendsStableSchemaInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools", "audit.jsonl")
	writer := NewJSONLWriter(path)

	first := Record{
		Timestamp:       time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC),
		Source:          "kernel",
		SessionID:       "sess_123",
		Tool:            "echo",
		Args:            json.RawMessage(`{"text":"hi"}`),
		DurationMs:      14,
		Status:          "completed",
		ResultSizeBytes: 13,
		Error:           "",
	}
	second := Record{
		Timestamp:       time.Date(2026, 4, 22, 10, 0, 1, 0, time.UTC),
		Source:          "delegate_task",
		AgentID:         "sa_456",
		Tool:            "web_search",
		Args:            json.RawMessage(`{"query":"gormes"}`),
		DurationMs:      29,
		Status:          "failed",
		ResultSizeBytes: 0,
		Error:           "synthetic failure",
	}

	if err := writer.Record(first); err != nil {
		t.Fatalf("Record(first): %v", err)
	}
	if err := writer.Record(second); err != nil {
		t.Fatalf("Record(second): %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}

	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2\nraw=%s", len(lines), raw)
	}

	var gotFirst, gotSecond Record
	if err := json.Unmarshal(lines[0], &gotFirst); err != nil {
		t.Fatalf("Unmarshal(first): %v\nline=%s", err, lines[0])
	}
	if err := json.Unmarshal(lines[1], &gotSecond); err != nil {
		t.Fatalf("Unmarshal(second): %v\nline=%s", err, lines[1])
	}

	if gotFirst.Timestamp != first.Timestamp {
		t.Fatalf("first timestamp = %v, want %v", gotFirst.Timestamp, first.Timestamp)
	}
	if gotFirst.Source != "kernel" {
		t.Fatalf("first source = %q, want %q", gotFirst.Source, "kernel")
	}
	if gotFirst.SessionID != "sess_123" {
		t.Fatalf("first session_id = %q, want %q", gotFirst.SessionID, "sess_123")
	}
	if gotFirst.Tool != "echo" {
		t.Fatalf("first tool = %q, want %q", gotFirst.Tool, "echo")
	}
	if string(gotFirst.Args) != `{"text":"hi"}` {
		t.Fatalf("first args = %s, want %s", gotFirst.Args, `{"text":"hi"}`)
	}
	if gotFirst.DurationMs != 14 {
		t.Fatalf("first duration_ms = %d, want 14", gotFirst.DurationMs)
	}
	if gotFirst.Status != "completed" {
		t.Fatalf("first status = %q, want %q", gotFirst.Status, "completed")
	}
	if gotFirst.ResultSizeBytes != 13 {
		t.Fatalf("first result_size_bytes = %d, want 13", gotFirst.ResultSizeBytes)
	}
	if gotFirst.Error != "" {
		t.Fatalf("first error = %q, want empty", gotFirst.Error)
	}

	if gotSecond.Timestamp != second.Timestamp {
		t.Fatalf("second timestamp = %v, want %v", gotSecond.Timestamp, second.Timestamp)
	}
	if gotSecond.Source != "delegate_task" {
		t.Fatalf("second source = %q, want %q", gotSecond.Source, "delegate_task")
	}
	if gotSecond.AgentID != "sa_456" {
		t.Fatalf("second agent_id = %q, want %q", gotSecond.AgentID, "sa_456")
	}
	if gotSecond.Tool != "web_search" {
		t.Fatalf("second tool = %q, want %q", gotSecond.Tool, "web_search")
	}
	if string(gotSecond.Args) != `{"query":"gormes"}` {
		t.Fatalf("second args = %s, want %s", gotSecond.Args, `{"query":"gormes"}`)
	}
	if gotSecond.DurationMs != 29 {
		t.Fatalf("second duration_ms = %d, want 29", gotSecond.DurationMs)
	}
	if gotSecond.Status != "failed" {
		t.Fatalf("second status = %q, want %q", gotSecond.Status, "failed")
	}
	if gotSecond.ResultSizeBytes != 0 {
		t.Fatalf("second result_size_bytes = %d, want 0", gotSecond.ResultSizeBytes)
	}
	if gotSecond.Error != "synthetic failure" {
		t.Fatalf("second error = %q, want %q", gotSecond.Error, "synthetic failure")
	}
}
