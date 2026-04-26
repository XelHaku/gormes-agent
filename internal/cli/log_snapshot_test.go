package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotLogs_MissingFileSetsMissingTrue(t *testing.T) {
	dir := t.TempDir()
	roots := LogSnapshotRoots{
		LogPath:       filepath.Join(dir, "missing-main.log"),
		ToolAuditPath: filepath.Join(dir, "missing-tool.log"),
	}

	snap := SnapshotLogs(roots, SnapshotOpts{})

	if got := len(snap.Sections); got != 2 {
		t.Fatalf("len(Sections) = %d, want 2", got)
	}
	for _, s := range snap.Sections {
		if !s.Missing {
			t.Errorf("class %s: Missing = false, want true", s.Class)
		}
		if s.Truncated {
			t.Errorf("class %s: Truncated = true, want false", s.Class)
		}
		if s.Unreadable != "" {
			t.Errorf("class %s: Unreadable = %q, want \"\"", s.Class, s.Unreadable)
		}
	}
}

func TestSnapshotLogs_SmallFileSetsHeadOnlyTruncatedFalse(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.log")
	content := bytes.Repeat([]byte("a"), 100)
	if err := os.WriteFile(mainPath, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	snap := SnapshotLogs(LogSnapshotRoots{LogPath: mainPath}, SnapshotOpts{})

	if got := len(snap.Sections); got < 1 {
		t.Fatalf("len(Sections) = %d, want at least 1", got)
	}
	main := snap.Sections[0]
	if main.Class != LogClassMain {
		t.Fatalf("Class = %s, want %s", main.Class, LogClassMain)
	}
	if main.Truncated {
		t.Errorf("Truncated = true, want false")
	}
	if !bytes.Equal(main.Head, content) {
		t.Errorf("Head = %q, want %q", main.Head, content)
	}
	if main.Tail != nil {
		t.Errorf("Tail = %v, want nil", main.Tail)
	}
}

func TestSnapshotLogs_HeadAndTailTruncationSetsTruncatedTrue(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.log")
	content := bytes.Repeat([]byte("z"), 32)
	if err := os.WriteFile(mainPath, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	snap := SnapshotLogs(
		LogSnapshotRoots{LogPath: mainPath},
		SnapshotOpts{HeadBytes: 8, TailBytes: 4},
	)

	main := snap.Sections[0]
	if !main.Truncated {
		t.Errorf("Truncated = false, want true")
	}
	if got := len(main.Head); got != 8 {
		t.Errorf("len(Head) = %d, want 8", got)
	}
	if got := len(main.Tail); got != 4 {
		t.Errorf("len(Tail) = %d, want 4", got)
	}
}

func TestSnapshotLogs_AppliesRedactorPerLine(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.log")

	// 33 bytes: two Bearer lines plus one plain line, terminated with \n.
	headContent := "Bearer aaa\nBearer bbb\nplain text\n"
	// 11 bytes: one Bearer line, terminated with \n.
	tailContent := "Bearer ccc\n"
	// 50 bytes of filler keeps the middle bytes out of both head and tail.
	filler := strings.Repeat("x", 50)
	full := headContent + filler + tailContent
	if err := os.WriteFile(mainPath, []byte(full), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	snap := SnapshotLogs(
		LogSnapshotRoots{LogPath: mainPath},
		SnapshotOpts{HeadBytes: int64(len(headContent)), TailBytes: int64(len(tailContent))},
	)

	main := snap.Sections[0]
	if !main.Truncated {
		t.Fatalf("Truncated = false, want true (head+tail < file size)")
	}
	if main.Redacted != 3 {
		t.Errorf("Redacted = %d, want 3", main.Redacted)
	}
	if !bytes.Contains(main.Head, []byte("[REDACTED]")) {
		t.Errorf("Head missing [REDACTED]: %q", main.Head)
	}
	if !bytes.Contains(main.Tail, []byte("[REDACTED]")) {
		t.Errorf("Tail missing [REDACTED]: %q", main.Tail)
	}
}

func TestSnapshotLogs_UnreadableFileSetsUnreadableField(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root - mode 0000 is still readable")
	}

	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.log")
	if err := os.WriteFile(mainPath, []byte("hello"), 0o000); err != nil {
		t.Fatalf("write main: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(mainPath, 0o644) })

	toolPath := filepath.Join(dir, "tool.log")
	if err := os.WriteFile(toolPath, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write tool: %v", err)
	}

	snap := SnapshotLogs(
		LogSnapshotRoots{LogPath: mainPath, ToolAuditPath: toolPath},
		SnapshotOpts{},
	)

	if got := len(snap.Sections); got != 2 {
		t.Fatalf("len(Sections) = %d, want 2", got)
	}
	main := snap.Sections[0]
	tool := snap.Sections[1]

	if main.Class != LogClassMain {
		t.Fatalf("Sections[0].Class = %s, want %s", main.Class, LogClassMain)
	}
	if main.Unreadable == "" {
		t.Errorf("main Unreadable = \"\", want non-empty error message")
	}
	if main.Missing {
		t.Errorf("main Missing = true, want false (file exists, just unreadable)")
	}

	if tool.Class != LogClassToolAudit {
		t.Fatalf("Sections[1].Class = %s, want %s", tool.Class, LogClassToolAudit)
	}
	if tool.Unreadable != "" {
		t.Errorf("tool Unreadable = %q, want \"\"", tool.Unreadable)
	}
	if !bytes.Equal(tool.Head, []byte("ok")) {
		t.Errorf("tool Head = %q, want %q", tool.Head, "ok")
	}
}

func TestSnapshotLogs_BothFilesMissingReturnsTwoSections(t *testing.T) {
	snap := SnapshotLogs(LogSnapshotRoots{}, SnapshotOpts{})

	if got := len(snap.Sections); got != 2 {
		t.Fatalf("len(Sections) = %d, want 2", got)
	}
	if snap.Sections[0].Class != LogClassMain {
		t.Errorf("Sections[0].Class = %s, want %s", snap.Sections[0].Class, LogClassMain)
	}
	if snap.Sections[1].Class != LogClassToolAudit {
		t.Errorf("Sections[1].Class = %s, want %s", snap.Sections[1].Class, LogClassToolAudit)
	}
	for _, s := range snap.Sections {
		if !s.Missing {
			t.Errorf("class %s: Missing = false, want true", s.Class)
		}
	}
}

func TestSnapshotLogs_DefaultOptsAppliedWhenZero(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.log")
	// 200 KiB of filler with no newlines and no secret shapes — redaction is
	// a no-op so byte-length assertions stay exact after the split/join.
	content := bytes.Repeat([]byte("a"), 200*1024)
	if err := os.WriteFile(mainPath, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	snap := SnapshotLogs(LogSnapshotRoots{LogPath: mainPath}, SnapshotOpts{})

	main := snap.Sections[0]
	if !main.Truncated {
		t.Errorf("Truncated = false, want true")
	}
	if got := len(main.Head); got != 64*1024 {
		t.Errorf("len(Head) = %d, want %d", got, 64*1024)
	}
	if got := len(main.Tail); got != 16*1024 {
		t.Errorf("len(Tail) = %d, want %d", got, 16*1024)
	}
}
