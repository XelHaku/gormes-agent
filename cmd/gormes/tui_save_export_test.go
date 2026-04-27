package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
	"github.com/TrebuchetDynamics/gormes-agent/internal/transcript"
)

func TestTUISaveExportHelper_WritesUnderXDGDataHome(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(root, "xdg-data")
	hermesHome := filepath.Join(root, "hermes-home")
	cwd := filepath.Join(root, "cwd")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd): %v", err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("Chdir(cwd): %v", err)
	}
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg-config"))
	t.Setenv("HERMES_HOME", hermesHome)
	seedTUISaveTranscriptDB(t, "sess-xdg", "discord:chan-7")

	path, err := newTUISaveExportFunc()(context.Background(), "sess-xdg")
	if err != nil {
		t.Fatalf("SessionExportFunc: %v", err)
	}

	wantPath := filepath.Join(dataHome, "gormes", "sessions", "exports", "sess-xdg.md")
	if path != wantPath {
		t.Fatalf("export path = %q, want %q", path, wantPath)
	}
	assertPathNotUnder(t, path, hermesHome)
	assertPathNotUnder(t, path, cwd)
	assertNoFile(t, filepath.Join(hermesHome, "gormes", "sessions", "exports", "sess-xdg.md"))
	assertNoFile(t, filepath.Join(cwd, "gormes", "sessions", "exports", "sess-xdg.md"))

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(export): %v", err)
	}
	text := string(body)
	for _, want := range []string{"# Session: sess-xdg", "**Platform:** discord", "**Tool Calls:**", "`list_dir`"} {
		if !strings.Contains(text, want) {
			t.Fatalf("export body missing %q:\n%s", want, text)
		}
	}
}

func TestTUISaveExportHelper_CollisionSafeName(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(root, "xdg-data")
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg-config"))
	t.Setenv("HERMES_HOME", filepath.Join(root, "hermes-home"))
	seedTUISaveTranscriptDB(t, "sess-collision", "telegram:42")

	exportDir := filepath.Join(dataHome, "gormes", "sessions", "exports")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(exportDir): %v", err)
	}
	occupied := filepath.Join(exportDir, "sess-collision.md")
	const sentinel = "unrelated operator note"
	if err := os.WriteFile(occupied, []byte(sentinel), 0o644); err != nil {
		t.Fatalf("seed occupied export: %v", err)
	}

	path, err := newTUISaveExportFunc()(context.Background(), "sess-collision")
	if err != nil {
		t.Fatalf("SessionExportFunc: %v", err)
	}

	if path == occupied {
		t.Fatalf("export path reused occupied file %q", occupied)
	}
	if filepath.Dir(path) != exportDir {
		t.Fatalf("export dir = %q, want %q", filepath.Dir(path), exportDir)
	}
	if filepath.Ext(path) != ".md" {
		t.Fatalf("export path = %q, want markdown extension", path)
	}
	kept, err := os.ReadFile(occupied)
	if err != nil {
		t.Fatalf("ReadFile(occupied): %v", err)
	}
	if string(kept) != sentinel {
		t.Fatalf("occupied export was overwritten: got %q, want %q", kept, sentinel)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(export): %v", err)
	}
	if !strings.Contains(string(body), "# Session: sess-collision") {
		t.Fatalf("export body = %q, want transcript markdown", string(body))
	}
}

func TestTUISaveExportHelper_PropagatesExportErrors(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "xdg-data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg-config"))
	t.Setenv("HERMES_HOME", filepath.Join(root, "hermes-home"))
	seedTUISaveTranscriptDB(t, "other-session", "telegram:42")

	path, err := newTUISaveExportFunc()(context.Background(), "missing-session")
	if !errors.Is(err, transcript.ErrSessionNotFound) {
		t.Fatalf("SessionExportFunc error = %v, want %v", err, transcript.ErrSessionNotFound)
	}
	if path != "" {
		t.Fatalf("SessionExportFunc path = %q, want empty on export failure", path)
	}
}

func seedTUISaveTranscriptDB(t *testing.T, sessionID, chatID string) {
	t.Helper()

	store, err := memory.OpenSqlite(config.MemoryDBPath(), 8, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer store.Close(context.Background())

	meta := `{"tool_calls":[{"id":"call_1","name":"list_dir","arguments":{"path":"."}}]}`
	if _, err := store.DB().Exec(
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id, meta_json)
		 VALUES (?, 'user', 'hello from tui', ?, ?, NULL),
		        (?, 'assistant', 'saved from transcript store', ?, ?, ?)`,
		sessionID, time.Date(2026, 4, 22, 10, 0, 0, 0, time.UTC).Unix(), chatID,
		sessionID, time.Date(2026, 4, 22, 10, 0, 4, 0, time.UTC).Unix(), chatID, meta,
	); err != nil {
		t.Fatalf("seed turns: %v", err)
	}
}

func assertPathNotUnder(t *testing.T, path, root string) {
	t.Helper()

	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("Rel(%q, %q): %v", root, path, err)
	}
	if rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..") {
		t.Fatalf("path %q is under forbidden root %q", path, root)
	}
}

func assertNoFile(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(%q) err = %v, want not exist", path, err)
	}
}
