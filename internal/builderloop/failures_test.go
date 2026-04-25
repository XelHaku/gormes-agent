package builderloop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFailureRecordWriteIncrementsCountAndTailsStderr(t *testing.T) {
	root := t.TempDir()
	stderrPath := filepath.Join(root, "stderr.log")

	var lines []string
	for i := 1; i <= 45; i++ {
		lines = append(lines, fmt.Sprintf("line %02d", i))
	}
	if err := os.WriteFile(stderrPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := WriteFailureRecord(root, "task-a", 1, "red failed", stderrPath, []string{"first"}); err != nil {
		t.Fatalf("WriteFailureRecord() first error = %v", err)
	}
	if err := WriteFailureRecord(root, "task-a", 2, "green failed", stderrPath, []string{"second", "third"}); err != nil {
		t.Fatalf("WriteFailureRecord() second error = %v", err)
	}

	got, err := ReadFailureRecord(root, "task-a")
	if err != nil {
		t.Fatalf("ReadFailureRecord() error = %v", err)
	}

	if got.Count != 2 {
		t.Fatalf("Count = %d, want 2", got.Count)
	}
	if got.LastRC != 2 {
		t.Fatalf("LastRC = %d, want 2", got.LastRC)
	}
	if got.LastReason != "green failed" {
		t.Fatalf("LastReason = %q, want green failed", got.LastReason)
	}
	if !reflect.DeepEqual(got.LastFinalErrors, []string{"second", "third"}) {
		t.Fatalf("LastFinalErrors = %#v, want second and third", got.LastFinalErrors)
	}

	tailLines := strings.Split(got.LastStderrTail, "\n")
	if len(tailLines) != 40 {
		t.Fatalf("stderr tail has %d lines, want 40", len(tailLines))
	}
	if tailLines[0] != "line 06" || tailLines[39] != "line 45" {
		t.Fatalf("stderr tail bounds = %q ... %q, want line 06 ... line 45", tailLines[0], tailLines[39])
	}

	path := filepath.Join(root, "task-failures", "task-a.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatal("failure record JSON missing trailing newline")
	}
	if !json.Valid(raw) {
		t.Fatal("failure record JSON is invalid")
	}
	if !strings.Contains(string(raw), "\n  \"count\": 2,") {
		t.Fatalf("failure record JSON is not indented: %q", string(raw))
	}
}

func TestFailureRecordWriteWithEmptyStderrPathUsesEmptyTail(t *testing.T) {
	root := t.TempDir()

	if err := WriteFailureRecord(root, "task-empty-stderr", 3, "no stderr", "", []string{"final"}); err != nil {
		t.Fatalf("WriteFailureRecord() error = %v", err)
	}

	got, err := ReadFailureRecord(root, "task-empty-stderr")
	if err != nil {
		t.Fatalf("ReadFailureRecord() error = %v", err)
	}
	if got.Count != 1 {
		t.Fatalf("Count = %d, want 1", got.Count)
	}
	if got.LastStderrTail != "" {
		t.Fatalf("LastStderrTail = %q, want empty", got.LastStderrTail)
	}
}

func TestFailureRecordWriteWithMissingStderrPathUsesEmptyTail(t *testing.T) {
	root := t.TempDir()
	stderrPath := filepath.Join(root, "missing-stderr.log")

	if err := WriteFailureRecord(root, "task-missing-stderr", 4, "missing stderr", stderrPath, nil); err != nil {
		t.Fatalf("WriteFailureRecord() error = %v", err)
	}

	got, err := ReadFailureRecord(root, "task-missing-stderr")
	if err != nil {
		t.Fatalf("ReadFailureRecord() error = %v", err)
	}
	if got.Count != 1 {
		t.Fatalf("Count = %d, want 1", got.Count)
	}
	if got.LastStderrTail != "" {
		t.Fatalf("LastStderrTail = %q, want empty", got.LastStderrTail)
	}
}

func TestFailureRecordWriteRecoversCorruptExistingRecord(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "task-failures", "task-corrupt.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := WriteFailureRecord(root, "task-corrupt", 5, "recovered", "", nil); err != nil {
		t.Fatalf("WriteFailureRecord() error = %v", err)
	}

	got, err := ReadFailureRecord(root, "task-corrupt")
	if err != nil {
		t.Fatalf("ReadFailureRecord() error = %v", err)
	}
	if got.Count != 1 {
		t.Fatalf("Count = %d, want 1", got.Count)
	}
	if got.LastRC != 5 {
		t.Fatalf("LastRC = %d, want 5", got.LastRC)
	}
}

func TestFailureRecordWriteRecoversEmptyExistingRecord(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "task-failures", "task-empty.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := WriteFailureRecord(root, "task-empty", 6, "recovered empty", "", nil); err != nil {
		t.Fatalf("WriteFailureRecord() error = %v", err)
	}

	got, err := ReadFailureRecord(root, "task-empty")
	if err != nil {
		t.Fatalf("ReadFailureRecord() error = %v", err)
	}
	if got.Count != 1 {
		t.Fatalf("Count = %d, want 1", got.Count)
	}
	if got.LastRC != 6 {
		t.Fatalf("LastRC = %d, want 6", got.LastRC)
	}
}
