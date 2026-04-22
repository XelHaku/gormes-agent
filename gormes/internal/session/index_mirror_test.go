package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionIndexMirror_WriteCreatesStableOrderedYAML(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	m, err := OpenBolt(dbPath)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	defer m.Close()

	ctx := context.Background()
	if err := m.Put(ctx, "telegram:42", "sess-telegram"); err != nil {
		t.Fatalf("Put telegram: %v", err)
	}
	if err := m.Put(ctx, "discord:chan-9", "sess-discord"); err != nil {
		t.Fatalf("Put discord: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "sessions", "index.yaml")
	mirror := NewSessionIndexMirror(m, outPath)
	mirror.now = func() time.Time {
		return time.Date(2026, 4, 22, 12, 34, 56, 0, time.UTC)
	}

	if err := mirror.Write(); err != nil {
		t.Fatalf("Write: %v", err)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", outPath, err)
	}

	want := "" +
		"# Auto-generated session index\n" +
		"# This file is a read-only mirror of sessions.db for operator auditability\n" +
		"sessions:\n" +
		"  discord:chan-9: sess-discord\n" +
		"  telegram:42: sess-telegram\n" +
		"updated_at: 2026-04-22T12:34:56Z\n"
	if string(raw) != want {
		t.Fatalf("mirror YAML =\n%s\nwant:\n%s", raw, want)
	}
}

func TestSessionIndexMirror_WriteIsReadOnlyForSessionState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	m, err := OpenBolt(dbPath)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	defer m.Close()

	ctx := context.Background()
	if err := m.Put(ctx, "telegram:42", "sess-telegram"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "sessions", "index.yaml")
	mirror := NewSessionIndexMirror(m, outPath)
	if err := mirror.Write(); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := m.Get(ctx, "telegram:42")
	if err != nil {
		t.Fatalf("Get after Write: %v", err)
	}
	if got != "sess-telegram" {
		t.Fatalf("Get after Write = %q, want %q", got, "sess-telegram")
	}
}

func TestSessionIndexMirror_WriteReplacesFileWithoutLeavingTempFiles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	m, err := OpenBolt(dbPath)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	defer m.Close()

	ctx := context.Background()
	if err := m.Put(ctx, "telegram:42", "sess-telegram"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	outDir := filepath.Join(t.TempDir(), "sessions")
	outPath := filepath.Join(outDir, "index.yaml")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(outPath, []byte("stale: true\n"), 0o644); err != nil {
		t.Fatalf("WriteFile stale: %v", err)
	}

	mirror := NewSessionIndexMirror(m, outPath)
	mirror.now = func() time.Time { return time.Date(2026, 4, 22, 13, 0, 0, 0, time.UTC) }

	if err := mirror.Write(); err != nil {
		t.Fatalf("Write: %v", err)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", outPath, err)
	}
	if strings.Contains(string(raw), "stale: true") {
		t.Fatalf("index.yaml still contains stale content:\n%s", raw)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", outDir, err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("unexpected temp file left behind: %s", entry.Name())
		}
	}
}

func TestSessionIndexMirror_StartRefreshSkipsRewriteWhenSnapshotUnchanged(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sessions.db")
	m, err := OpenBolt(dbPath)
	if err != nil {
		t.Fatalf("OpenBolt: %v", err)
	}
	defer m.Close()

	ctx := context.Background()
	if err := m.Put(ctx, "telegram:42", "sess-telegram"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "sessions", "index.yaml")
	mirror := NewSessionIndexMirror(m, outPath)
	times := []time.Time{
		time.Date(2026, 4, 22, 14, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 22, 14, 1, 0, 0, time.UTC),
	}
	var nowCalls atomic.Int32
	mirror.now = func() time.Time {
		i := int(nowCalls.Add(1) - 1)
		if i >= len(times) {
			return times[len(times)-1]
		}
		return times[i]
	}

	refresh := mirror.StartRefresh(10*time.Millisecond, nil)
	defer refresh.Stop()

	waitForMirrorText(t, outPath, "updated_at: 2026-04-22T14:00:00Z")
	first, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", outPath, err)
	}

	time.Sleep(40 * time.Millisecond)

	second, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile second(%q): %v", outPath, err)
	}
	if string(second) != string(first) {
		t.Fatalf("unchanged snapshot rewrote index.yaml:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func waitForMirrorText(t *testing.T, path, want string) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(raw), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	raw, _ := os.ReadFile(path)
	t.Fatalf("mirror %q never contained %q; last content:\n%s", path, want, raw)
}
