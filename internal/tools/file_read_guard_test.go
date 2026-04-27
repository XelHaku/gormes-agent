package tools

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileReadGuard_DedupStatusOutOfContent(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "notes.txt")
	diskContent := []byte("line one\nline two\n")
	if err := os.WriteFile(path, diskContent, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	guard := NewFileReadGuard(FileReadGuardOptions{WorkspaceRoot: workspace})

	first, err := guard.ReadFile(path)
	if err != nil {
		t.Fatalf("first ReadFile: %v", err)
	}
	if !bytes.Equal(first.Content, diskContent) {
		t.Fatalf("first content = %q, want exact disk bytes %q", first.Content, diskContent)
	}
	if first.DedupStatus != "" {
		t.Fatalf("first dedup status = %q, want empty", first.DedupStatus)
	}

	second, err := guard.ReadFile(path)
	if err != nil {
		t.Fatalf("second ReadFile: %v", err)
	}
	if !bytes.Equal(second.Content, diskContent) {
		t.Fatalf("second content = %q, want exact disk bytes %q", second.Content, diskContent)
	}
	if second.DedupStatus != FileReadDedupStatusUnchanged {
		t.Fatalf("second dedup status = %q, want %q", second.DedupStatus, FileReadDedupStatusUnchanged)
	}
	if bytes.Contains(second.Content, []byte(FileReadDedupStatusUnchanged)) {
		t.Fatalf("dedup status leaked into content: %q", second.Content)
	}
	if !hasFileReadEvidence(second, FileReadDedupStatusUnchanged) {
		t.Fatalf("second evidence = %+v, want %q", second.Evidence, FileReadDedupStatusUnchanged)
	}
}

func TestFileReadGuard_WriteInvalidatesCache(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "draft.txt")
	content := []byte("old content\n")

	guard := NewFileReadGuard(FileReadGuardOptions{WorkspaceRoot: workspace})
	readCalls := 0
	read := func(gotPath string) ([]byte, error) {
		readCalls++
		if gotPath != path {
			t.Fatalf("read path = %q, want %q", gotPath, path)
		}
		return append([]byte(nil), content...), nil
	}

	first, err := guard.ReadFileWith(path, read)
	if err != nil {
		t.Fatalf("first ReadFileWith: %v", err)
	}
	if !bytes.Equal(first.Content, []byte("old content\n")) {
		t.Fatalf("first content = %q, want old content", first.Content)
	}

	err = guard.WriteFile(path, []byte("new content\n"), func(gotPath string, newContent []byte) error {
		if gotPath != path {
			t.Fatalf("write path = %q, want %q", gotPath, path)
		}
		content = append([]byte(nil), newContent...)
		return nil
	})
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	second, err := guard.ReadFileWith(path, read)
	if err != nil {
		t.Fatalf("second ReadFileWith: %v", err)
	}
	if !bytes.Equal(second.Content, []byte("new content\n")) {
		t.Fatalf("second content = %q, want new content", second.Content)
	}
	if second.DedupStatus != "" {
		t.Fatalf("second dedup status = %q, want empty after invalidation", second.DedupStatus)
	}
	if readCalls != 2 {
		t.Fatalf("read calls = %d, want 2 so the stale cache was bypassed", readCalls)
	}
}

func TestFileReadGuard_PatchInvalidatesCache(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "module.go")
	content := []byte("package demo\n\nconst value = 1\n")

	guard := NewFileReadGuard(FileReadGuardOptions{WorkspaceRoot: workspace})
	readCalls := 0
	read := func(gotPath string) ([]byte, error) {
		readCalls++
		if gotPath != path {
			t.Fatalf("read path = %q, want %q", gotPath, path)
		}
		return append([]byte(nil), content...), nil
	}

	if _, err := guard.ReadFileWith(path, read); err != nil {
		t.Fatalf("initial ReadFileWith: %v", err)
	}

	err := guard.PatchFile(path, func(gotPath string) error {
		if gotPath != path {
			t.Fatalf("patch path = %q, want %q", gotPath, path)
		}
		content = []byte("package demo\n\nconst value = 2\n")
		return nil
	})
	if err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	result, err := guard.ReadFileWith(path, read)
	if err != nil {
		t.Fatalf("post-patch ReadFileWith: %v", err)
	}
	if !bytes.Equal(result.Content, []byte("package demo\n\nconst value = 2\n")) {
		t.Fatalf("post-patch content = %q, want patched content", result.Content)
	}
	if result.DedupStatus != "" {
		t.Fatalf("post-patch dedup status = %q, want empty after invalidation", result.DedupStatus)
	}
	if readCalls != 2 {
		t.Fatalf("read calls = %d, want 2 so the patch invalidated the cache", readCalls)
	}
}

func TestFileReadGuard_SmallWrapperDetected(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "wrapped.txt")
	content := []byte("wrapped read content\n")

	guard := NewFileReadGuard(FileReadGuardOptions{WorkspaceRoot: workspace})
	read := func(gotPath string) ([]byte, error) {
		if gotPath != path {
			t.Fatalf("read path = %q, want %q", gotPath, path)
		}
		return append([]byte(nil), content...), nil
	}
	readThroughWrapper := func() (FileReadResult, error) {
		return guard.ReadFileWith(path, read)
	}

	if _, err := readThroughWrapper(); err != nil {
		t.Fatalf("first wrapped read: %v", err)
	}
	second, err := readThroughWrapper()
	if err != nil {
		t.Fatalf("second wrapped read: %v", err)
	}
	if second.DedupStatus != FileReadDedupStatusUnchanged {
		t.Fatalf("wrapped read dedup status = %q, want %q", second.DedupStatus, FileReadDedupStatusUnchanged)
	}
	evidence, ok := findFileReadEvidence(second, FileReadDedupStatusUnchanged)
	if !ok {
		t.Fatalf("wrapped read evidence = %+v, want %q", second.Evidence, FileReadDedupStatusUnchanged)
	}
	if evidence.Message == "" {
		t.Fatal("wrapped read evidence message is empty")
	}

	wrappedStatus := "Note: " + evidence.Message + "\n\n(continuing.)"
	wrote := false
	err = guard.WriteFile(path, []byte(wrappedStatus), func(string, []byte) error {
		wrote = true
		return nil
	})
	if !errors.Is(err, ErrFileReadGuardStatusContent) {
		t.Fatalf("WriteFile error = %v, want %v", err, ErrFileReadGuardStatusContent)
	}
	if wrote {
		t.Fatal("WriteFile persisted a small wrapper around internal read status")
	}

	largeContent := []byte("# documentation\n\n" + evidence.Message + "\n\n" + strings.Repeat("legitimate content ", 80))
	err = guard.WriteFile(path, largeContent, func(string, []byte) error {
		wrote = true
		return nil
	})
	if err != nil {
		t.Fatalf("WriteFile with large quoted status: %v", err)
	}
}

func TestFileReadGuard_DegradedStatusOutOfContent(t *testing.T) {
	path := "degraded.txt"
	content := []byte("degraded content\n")
	read := func(string) ([]byte, error) {
		return append([]byte(nil), content...), nil
	}

	var unavailable *FileReadGuard
	unavailableResult, err := unavailable.ReadFileWith(path, read)
	if err != nil {
		t.Fatalf("nil guard ReadFileWith: %v", err)
	}
	if !bytes.Equal(unavailableResult.Content, content) {
		t.Fatalf("nil guard content = %q, want %q", unavailableResult.Content, content)
	}
	if unavailableResult.DedupStatus != FileReadStatusGuardUnavailable {
		t.Fatalf("nil guard status = %q, want %q", unavailableResult.DedupStatus, FileReadStatusGuardUnavailable)
	}
	if bytes.Contains(unavailableResult.Content, []byte(FileReadStatusGuardUnavailable)) {
		t.Fatalf("guard unavailable status leaked into content: %q", unavailableResult.Content)
	}

	disabled := &FileReadGuard{}
	disabledResult, err := disabled.ReadFileWith(path, read)
	if err != nil {
		t.Fatalf("disabled cache ReadFileWith: %v", err)
	}
	if !bytes.Equal(disabledResult.Content, content) {
		t.Fatalf("disabled cache content = %q, want %q", disabledResult.Content, content)
	}
	if disabledResult.DedupStatus != FileReadStatusDedupCacheDisabled {
		t.Fatalf("disabled cache status = %q, want %q", disabledResult.DedupStatus, FileReadStatusDedupCacheDisabled)
	}
	if bytes.Contains(disabledResult.Content, []byte(FileReadStatusDedupCacheDisabled)) {
		t.Fatalf("cache disabled status leaked into content: %q", disabledResult.Content)
	}
}

func hasFileReadEvidence(result FileReadResult, kind string) bool {
	_, ok := findFileReadEvidence(result, kind)
	return ok
}

func findFileReadEvidence(result FileReadResult, kind string) (FileReadEvidence, bool) {
	for _, evidence := range result.Evidence {
		if evidence.Kind == kind {
			return evidence, true
		}
	}
	return FileReadEvidence{}, false
}
