package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/store"
)

func TestOpenSqlite_CreatesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer s.Close(context.Background())

	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM turns").Scan(&n); err != nil {
		t.Errorf("turns table missing: %v", err)
	}
	if n != 0 {
		t.Errorf("turns count at startup = %d, want 0", n)
	}

	if err := s.db.QueryRow("SELECT COUNT(*) FROM turns_fts").Scan(&n); err != nil {
		t.Errorf("turns_fts virtual table missing: %v", err)
	}
}

func TestOpenSqlite_SchemaMetaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var v string
	err := s.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if err != nil {
		t.Fatalf("schema_meta missing: %v", err)
	}
	if v != schemaVersion {
		t.Errorf("schema version = %q, want %q", v, schemaVersion)
	}
}

func TestOpenSqlite_AutoCreatesParentDir(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "newsubdir")
	path := filepath.Join(parent, "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite (missing parent dir): %v", err)
	}
	defer s.Close(context.Background())

	info, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("parent dir should exist: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("parent dir perm = %o, want 0700", perm)
	}
}

func TestOpenSqlite_SetsWALMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var mode string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

func TestSqliteStore_ExecReturnsFast(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	start := time.Now()
	_, err := s.Exec(context.Background(), store.Command{
		Kind:    store.AppendUserTurn,
		Payload: json.RawMessage(`{"session_id":"s","content":"hi","ts_unix":1}`),
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	// 10 ms is generous — real return should be sub-ms. Under the race
	// detector this still has headroom.
	if elapsed > 10*time.Millisecond {
		t.Errorf("Exec took %v, want well under 10 ms", elapsed)
	}
}

func TestSqliteStore_ExecDropsOnFullQueue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")

	// With Task 3's silent-drain run(), commands that land in the queue
	// are drained without writes. With queueCap=2 the worker can only
	// buffer 2 at a time; firing 1000 Execs back-to-back MUST overflow
	// the queue. Drops > 0 and Drops + Accepted == 1000.

	s, _ := OpenSqlite(path, 2, nil)
	defer s.Close(context.Background())

	for i := 0; i < 1000; i++ {
		_, _ = s.Exec(context.Background(), store.Command{
			Kind:    store.AppendUserTurn,
			Payload: json.RawMessage(`{}`),
		})
	}

	st := s.Stats()
	if st.Drops == 0 {
		t.Errorf("expected some Drops after 1000 Execs into queueCap=2, got 0")
	}
	if st.Drops+st.Accepted != 1000 {
		t.Errorf("Accepted (%d) + Drops (%d) = %d, want 1000",
			st.Accepted, st.Drops, st.Drops+st.Accepted)
	}
}

func TestSqliteStore_ExecHonorsCtxCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Exec(ctx, store.Command{Kind: store.AppendUserTurn})
	if err == nil {
		t.Error("Exec with canceled ctx should return ctx.Err()")
	}
}

func TestSqliteStore_WorkerPersistsAppendUserTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	payload, _ := json.Marshal(map[string]any{
		"session_id": "sess-abc",
		"content":    "hello from user",
		"ts_unix":    1745000000,
	})
	_, _ = s.Exec(context.Background(), store.Command{
		Kind:    store.AppendUserTurn,
		Payload: payload,
	})

	// Worker is async. Wait until Accepted > 0 AND the row is visible.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.Stats().Accepted > 0 {
			// Also wait for the row to appear on disk.
			var n int
			_ = s.db.QueryRow("SELECT COUNT(*) FROM turns").Scan(&n)
			if n > 0 {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}

	var (
		sessionID, role, content string
		ts                       int64
	)
	row := s.db.QueryRow("SELECT session_id, role, content, ts_unix FROM turns LIMIT 1")
	if err := row.Scan(&sessionID, &role, &content, &ts); err != nil {
		t.Fatalf("scan: %v (Accepted=%d)", err, s.Stats().Accepted)
	}
	if sessionID != "sess-abc" {
		t.Errorf("session_id = %q", sessionID)
	}
	if role != "user" {
		t.Errorf("role = %q, want user", role)
	}
	if content != "hello from user" {
		t.Errorf("content = %q", content)
	}
	if ts != 1745000000 {
		t.Errorf("ts = %d", ts)
	}
}

func TestSqliteStore_WorkerPersistsFinalizeAssistantTurn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	payload, _ := json.Marshal(map[string]any{
		"session_id": "sess-abc",
		"content":    "hello from assistant",
		"ts_unix":    1745000001,
	})
	_, _ = s.Exec(context.Background(), store.Command{
		Kind:    store.FinalizeAssistantTurn,
		Payload: payload,
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = s.db.QueryRow("SELECT COUNT(*) FROM turns").Scan(&n)
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	var role string
	if err := s.db.QueryRow("SELECT role FROM turns LIMIT 1").Scan(&role); err != nil {
		t.Fatal(err)
	}
	if role != "assistant" {
		t.Errorf("role = %q, want assistant", role)
	}
}

func TestSqliteStore_WorkerHandlesMalformedPayload(t *testing.T) {
	// Bad JSON should be logged + dropped, not crash the worker.
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	_, _ = s.Exec(context.Background(), store.Command{
		Kind:    store.AppendUserTurn,
		Payload: []byte("not json"),
	})

	// Follow-up valid command must still succeed.
	good, _ := json.Marshal(map[string]any{
		"session_id": "s", "content": "ok", "ts_unix": 1,
	})
	_, _ = s.Exec(context.Background(), store.Command{
		Kind: store.AppendUserTurn, Payload: good,
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = s.db.QueryRow("SELECT COUNT(*) FROM turns").Scan(&n)
		if n == 1 {
			return // good: one row, malformed was dropped
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error("worker never wrote the follow-up valid command (or crashed)")
}

func TestAppendUserTurn_WritesCronColumnsWhenProvided(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 4, nil)
	defer s.Close(context.Background())

	payload := []byte(`{
		"session_id": "cron:job-1:1700000000",
		"content":    "hello from cron",
		"ts_unix":    1700000000,
		"chat_id":    "telegram:42",
		"cron":       1,
		"cron_job_id":"job-1"
	}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := s.Exec(ctx, store.Command{Kind: store.AppendUserTurn, Payload: payload}); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	// The worker is async; poll until the row lands.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM turns WHERE cron = 1 AND cron_job_id = 'job-1'`).Scan(&n)
		if n == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("turn with cron=1 was not persisted within 2s")
}

func TestAppendUserTurn_NoncronTurnLeavesColumnsAtDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 4, nil)
	defer s.Close(context.Background())

	payload := []byte(`{"session_id":"s","content":"hi","ts_unix":1,"chat_id":"c"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = s.Exec(ctx, store.Command{Kind: store.AppendUserTurn, Payload: payload})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var cron int
		var cjid sql.NullString
		err := s.db.QueryRow(`SELECT cron, cron_job_id FROM turns WHERE content = 'hi'`).Scan(&cron, &cjid)
		if err == nil {
			if cron != 0 {
				t.Errorf("default cron = %d, want 0", cron)
			}
			if cjid.Valid {
				t.Errorf("default cron_job_id = %q, want NULL", cjid.String)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("non-cron turn was not persisted within 2s")
}
