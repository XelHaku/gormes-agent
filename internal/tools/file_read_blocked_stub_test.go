package tools

import (
	"bytes"
	"strings"
	"testing"
)

func TestFileReadRepeatedStubBlocked_RepeatedDedupStatus(t *testing.T) {
	guard := NewFileReadGuard(FileReadGuardOptions{})
	path := "notes.txt"
	stub := []byte(fileReadDedupStatusMessage)

	first := guard.GuardRepeatedReadStatus(FileReadResult{
		Path:    path,
		Content: stub,
	})
	if len(first.Content) != 0 {
		t.Fatalf("first status content = %q, want status separated from content", first.Content)
	}
	if first.DedupStatus != FileReadDedupStatusUnchanged {
		t.Fatalf("first status = %q, want %q", first.DedupStatus, FileReadDedupStatusUnchanged)
	}
	if !hasFileReadEvidence(first, FileReadDedupStatusUnchanged) {
		t.Fatalf("first evidence = %+v, want %q", first.Evidence, FileReadDedupStatusUnchanged)
	}
	if hasFileReadEvidence(first, FileReadStatusDedupStubBlocked) {
		t.Fatalf("first evidence = %+v, did not want blocked evidence", first.Evidence)
	}

	second := guard.GuardRepeatedReadStatus(FileReadResult{
		Path:    path,
		Content: stub,
	})
	if len(second.Content) != 0 {
		t.Fatalf("blocked status content = %q, want BLOCKED kept out of content", second.Content)
	}
	if second.DedupStatus != FileReadStatusDedupStubBlocked {
		t.Fatalf("second status = %q, want %q", second.DedupStatus, FileReadStatusDedupStubBlocked)
	}
	evidence, ok := findFileReadEvidence(second, FileReadStatusDedupStubBlocked)
	if !ok {
		t.Fatalf("second evidence = %+v, want %q", second.Evidence, FileReadStatusDedupStubBlocked)
	}
	if !strings.Contains(evidence.Message, "BLOCKED") {
		t.Fatalf("blocked evidence message = %q, want BLOCKED", evidence.Message)
	}
	if bytes.Contains(second.Content, []byte("BLOCKED")) || bytes.Contains(second.Content, stub) {
		t.Fatalf("blocked evidence leaked into content: %q", second.Content)
	}
}

func TestFileReadRepeatedStubBlocked_LegitimateContentPreserved(t *testing.T) {
	guard := NewFileReadGuard(FileReadGuardOptions{})
	path := "guide.md"
	content := []byte(
		"# Guide\n\n" +
			"This document mentions read_file status text as an example, not as tool output:\n\n" +
			fileReadDedupStatusMessage + "\n\n" +
			strings.Repeat("ordinary file bytes remain ordinary file bytes. ", 40),
	)

	first := guard.GuardRepeatedReadStatus(FileReadResult{
		Path:    path,
		Content: content,
	})
	if !bytes.Equal(first.Content, content) {
		t.Fatalf("first content = %q, want exact fixture bytes", first.Content)
	}
	if first.DedupStatus != "" {
		t.Fatalf("first status = %q, want empty for legitimate content", first.DedupStatus)
	}
	if hasFileReadEvidence(first, FileReadStatusDedupStubBlocked) {
		t.Fatalf("first evidence = %+v, did not want blocked evidence", first.Evidence)
	}

	second := guard.GuardRepeatedReadStatus(FileReadResult{
		Path:    path,
		Content: content,
	})
	if !bytes.Equal(second.Content, content) {
		t.Fatalf("second content = %q, want exact fixture bytes", second.Content)
	}
	if second.DedupStatus != "" {
		t.Fatalf("second status = %q, want empty for repeated legitimate content", second.DedupStatus)
	}
	if hasFileReadEvidence(second, FileReadStatusDedupStubBlocked) {
		t.Fatalf("second evidence = %+v, did not want blocked evidence", second.Evidence)
	}
}

func TestFileReadRepeatedStubBlocked_SingleStatusAllowed(t *testing.T) {
	guard := NewFileReadGuard(FileReadGuardOptions{})
	path := "notes.txt"

	got := guard.GuardRepeatedReadStatus(FileReadResult{
		Path:        path,
		DedupStatus: FileReadDedupStatusUnchanged,
		Evidence: []FileReadEvidence{{
			Kind:    FileReadDedupStatusUnchanged,
			Path:    path,
			Message: fileReadDedupStatusMessage,
		}},
	})

	if len(got.Content) != 0 {
		t.Fatalf("status content = %q, want status evidence only", got.Content)
	}
	if got.DedupStatus != FileReadDedupStatusUnchanged {
		t.Fatalf("status = %q, want %q", got.DedupStatus, FileReadDedupStatusUnchanged)
	}
	if hasFileReadEvidence(got, FileReadStatusDedupStubBlocked) {
		t.Fatalf("evidence = %+v, did not want blocked evidence", got.Evidence)
	}
	evidence, ok := findFileReadEvidence(got, FileReadDedupStatusUnchanged)
	if !ok {
		t.Fatalf("evidence = %+v, want %q", got.Evidence, FileReadDedupStatusUnchanged)
	}
	if evidence.Message != fileReadDedupStatusMessage {
		t.Fatalf("evidence message = %q, want %q", evidence.Message, fileReadDedupStatusMessage)
	}
}
