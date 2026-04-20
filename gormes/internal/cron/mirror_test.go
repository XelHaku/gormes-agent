package cron

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/memory"
	"go.etcd.io/bbolt"
)

func newMirrorTestEnv(t *testing.T) (*Store, *RunStore, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "session.db")
	db, _ := bbolt.Open(dbPath, 0o600, nil)
	js, _ := NewStore(db)
	msPath := filepath.Join(t.TempDir(), "memory.db")
	ms, _ := memory.OpenSqlite(msPath, 0, nil)
	rs := NewRunStore(ms.DB())
	cleanup := func() {
		_ = ms.Close(context.Background())
		_ = db.Close()
	}
	return js, rs, cleanup
}

func TestMirror_WritesMarkdownWithJobsAndRuns(t *testing.T) {
	js, rs, cleanup := newMirrorTestEnv(t)
	defer cleanup()

	j := NewJob("morning", "0 8 * * *", "status prompt here")
	_ = js.Create(j)
	_ = rs.RecordRun(context.Background(), Run{
		JobID: j.ID, StartedAt: 1700000000, FinishedAt: 1700000005,
		PromptHash: "h", Status: "success", Delivered: true,
		OutputPreview: "morning report OK",
	})

	path := filepath.Join(t.TempDir(), "CRON.md")
	m := NewMirror(MirrorConfig{
		JobStore: js, RunStore: rs, Path: path, Interval: 50 * time.Millisecond,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	time.Sleep(120 * time.Millisecond)

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"# Gormes Cron",
		"morning",
		"0 8 * * *",
		"status prompt here",
		"morning report OK",
		"success",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("CRON.md missing %q — got:\n%s", want, s)
		}
	}
}

func TestMirror_AtomicWrite_NoPartialReadOnCrash(t *testing.T) {
	js, rs, cleanup := newMirrorTestEnv(t)
	defer cleanup()
	_ = js.Create(NewJob("j", "@daily", "p"))

	path := filepath.Join(t.TempDir(), "CRON.md")
	m := NewMirror(MirrorConfig{
		JobStore: js, RunStore: rs, Path: path, Interval: 10 * time.Millisecond,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	time.Sleep(40 * time.Millisecond)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Error("CRON.md is empty after mirror ticks")
	}
	dir := filepath.Dir(path)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestMirror_EmptyStoreProducesEmptyActiveSection(t *testing.T) {
	js, rs, cleanup := newMirrorTestEnv(t)
	defer cleanup()

	path := filepath.Join(t.TempDir(), "CRON.md")
	m := NewMirror(MirrorConfig{
		JobStore: js, RunStore: rs, Path: path, Interval: 10 * time.Millisecond,
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)
	time.Sleep(30 * time.Millisecond)

	body, _ := os.ReadFile(path)
	s := string(body)
	if !strings.Contains(s, "Active Jobs (0)") {
		t.Errorf("empty-store mirror should state Active Jobs (0) — got:\n%s", s)
	}
}
