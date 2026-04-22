package transcript

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/memory"
)

func TestExportMarkdown_Golden(t *testing.T) {
	store := openTranscriptStore(t)
	defer store.Close(context.Background())

	mustInsertTurn(t, store.DB(), transcriptRow{
		SessionID: "sess-export",
		Role:      "user",
		Content:   "How do I inspect this repo?",
		TSUnix:    time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC).Unix(),
		ChatID:    "telegram:42",
	})

	meta, err := json.Marshal(map[string]any{
		"tool_calls": []map[string]any{
			{"id": "call_1", "name": "list_dir", "arguments": map[string]any{"path": "."}},
			{"id": "call_2", "name": "read_file", "arguments": map[string]any{"path": "README.md"}},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(meta): %v", err)
	}
	mustInsertTurn(t, store.DB(), transcriptRow{
		SessionID: "sess-export",
		Role:      "assistant",
		Content:   "I'll inspect the project files first.",
		TSUnix:    time.Date(2026, 4, 20, 10, 0, 5, 0, time.UTC).Unix(),
		ChatID:    "telegram:42",
		MetaJSON:  string(meta),
	})

	got, err := ExportMarkdown(context.Background(), store.DB(), "sess-export")
	if err != nil {
		t.Fatalf("ExportMarkdown: %v", err)
	}

	want := `# Session: sess-export

**Session ID:** ` + "`sess-export`" + `  
**Platform:** telegram  
**Created:** 2026-04-20 10:00:00 UTC  
**Messages:** 2

---

## Turn 1 - 2026-04-20 10:00:00 UTC

**User:** How do I inspect this repo?

---

## Turn 2 - 2026-04-20 10:00:05 UTC

**Agent:** I'll inspect the project files first.

**Tool Calls:**
- ` + "`list_dir`" + ` ` + "`{\"path\":\".\"}`" + `
- ` + "`read_file`" + ` ` + "`{\"path\":\"README.md\"}`" + `
`

	if got != want {
		t.Fatalf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestExportMarkdown_MissingSession(t *testing.T) {
	store := openTranscriptStore(t)
	defer store.Close(context.Background())

	_, err := ExportMarkdown(context.Background(), store.DB(), "missing-session")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("ExportMarkdown error = %v, want %v", err, ErrSessionNotFound)
	}
}

type transcriptRow struct {
	SessionID string
	Role      string
	Content   string
	TSUnix    int64
	ChatID    string
	MetaJSON  string
}

func openTranscriptStore(t *testing.T) *memory.SqliteStore {
	t.Helper()

	store, err := memory.OpenSqlite(t.TempDir()+"/memory.db", 8, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	return store
}

func mustInsertTurn(t *testing.T, db *sql.DB, row transcriptRow) {
	t.Helper()

	if _, err := db.Exec(
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id, meta_json)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		row.SessionID, row.Role, row.Content, row.TSUnix, row.ChatID, nullIfEmptyString(row.MetaJSON),
	); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
}

func nullIfEmptyString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
