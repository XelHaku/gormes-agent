package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/config"
	"github.com/TrebuchetDynamics/gormes-agent/internal/memory"
)

func TestMemoryStatusCommand_PrintsExtractorSummary(t *testing.T) {
	seedMemoryStatusDB(t)

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"memory", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstderr=%s", err, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"Extractor status",
		"worker_health: degraded",
		"queue_depth: 1",
		"dead_letters: 2",
		"dead_letter_summary:",
		"error=\"malformed JSON\" count=1",
		"error=\"upstream timeout\" count=1",
		"session_id=sess-3",
		"error=\"upstream timeout\"",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %q, want substring %q", out, want)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestMemoryStatusCommand_MissingDatabase(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dataHome, "config"))

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"memory", "status"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want missing-database error")
	}
	if !strings.Contains(err.Error(), "memory database not found") {
		t.Fatalf("error = %v, want missing database message", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestMemoryStatusCommand_PrintsGonchoQueueZeroState(t *testing.T) {
	seedMemoryStatusDB(t)

	cmd := newRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"memory", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\nstderr=%s", err, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"Goncho queue status (observability only; not synchronization)",
		"representation: total=0 pending=0 in_progress=0 completed=0",
		"summary: total=0 pending=0 in_progress=0 completed=0",
		"dream: total=0 pending=0 in_progress=0 completed=0",
		"goncho_queue: unavailable (zero tracked work units)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %q, want substring %q", out, want)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func seedMemoryStatusDB(t *testing.T) {
	t.Helper()

	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dataHome, "config"))

	store, err := memory.OpenSqlite(config.MemoryDBPath(), 8, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer store.Close(context.Background())

	now := time.Date(2026, 4, 22, 15, 4, 5, 0, time.UTC).Unix()
	_, err = store.DB().Exec(
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id, extracted, extraction_attempts, extraction_error, cron)
		 VALUES
		 ('sess-1', 'user', 'queued turn', ?, 'telegram:1', 0, 0, NULL, 0),
		 ('sess-2', 'user', 'dead letter one', ?, 'telegram:2', 2, 3, 'malformed JSON', 0),
		 ('sess-3', 'assistant', 'dead letter two', ?, 'discord:9', 2, 4, 'upstream timeout', 0)`,
		now, now+1, now+2,
	)
	if err != nil {
		t.Fatalf("seed turns: %v", err)
	}
}
