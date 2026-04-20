# Gormes Phase 3.A — Lattice Memory Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `bin/gormes-telegram` persists every conversational turn to a SQLite database with FTS5 search enabled, without ever blocking the kernel's 250 ms `StoreAckDeadline`.

**Architecture:** New `internal/memory` package with `SqliteStore` — a fire-and-forget `store.Store` implementation. `Exec` enqueues `Command`s on a bounded channel and returns in microseconds; a single-owner background goroutine performs all SQL writes. On queue full: log + drop. TUI keeps `NoopStore` (binary stays 8.2 MB); only the bot pays the ~5.9 MB SQLite tax (projected ~15-16 MB under 20 MB budget).

**Tech Stack:** Go 1.22+, `modernc.org/sqlite` (pure-Go, zero CGO), `database/sql`, SQLite FTS5 content-backed virtual table with auto-sync triggers.

**Spec:** [`gormes/docs/superpowers/specs/2026-04-20-gormes-phase3a-memory-design.md`](../specs/2026-04-20-gormes-phase3a-memory-design.md)

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `gormes/go.mod` / `gormes/go.sum` | Modify | Add `modernc.org/sqlite` |
| `gormes/internal/store/store.go` | Modify | Remove `AppendAssistantDraft` enum value + String() case; tighten comment |
| `gormes/internal/store/recording.go` | Create | `RecordingStore` test double — captures commands for kernel tests |
| `gormes/internal/store/recording_test.go` | Create | RecordingStore unit tests |
| `gormes/internal/memory/memory.go` | Create | Package doc, `SqliteStore` type, `OpenSqlite`, `Stats`, `Close` |
| `gormes/internal/memory/schema.go` | Create | Schema DDL + `migrate()` helper |
| `gormes/internal/memory/worker.go` | Create | Worker goroutine + `handleCommand` dispatcher |
| `gormes/internal/memory/memory_test.go` | Create | Unit tests (open/close, fast-return, drop-on-full, accepted counter) |
| `gormes/internal/memory/fts5_test.go` | Create | FTS5 `MATCH` behaviour tests (basic, phrase, delete-sync) |
| `gormes/internal/memory/shutdown_test.go` | Create | Drain + idempotent Close tests |
| `gormes/internal/kernel/kernel.go` | Modify | Update `AppendUserTurn` payload shape; activate `FinalizeAssistantTurn` after stream finalization |
| `gormes/internal/kernel/finalize_store_test.go` | Create | `TestKernel_FinalizeAssistantTurnReachesStore` via `RecordingStore` |
| `gormes/internal/config/config.go` | Modify | `TelegramCfg.MemoryQueueCap` + `MemoryDBPath()` export |
| `gormes/internal/config/config_test.go` | Modify | Append `TestLoad_MemoryQueueCapDefault`, `TestMemoryDBPath_HonorsXDG` |
| `gormes/cmd/gormes-telegram/main.go` | Modify | `OpenSqlite` + `defer Close(ctx)` + pass to `kernel.New` |
| `gormes/internal/telegram/bot_test.go` | Modify | Append `TestBot_TurnPersistsToSqlite` end-to-end |
| `gormes/internal/buildisolation_test.go` | Modify | Add `TestKernelHasNoMemoryDep`, `TestTUIBinaryHasNoSqliteDep` |

---

## Task 1: Add `modernc.org/sqlite` dependency

**Files:**
- Modify: `gormes/go.mod`
- Modify: `gormes/go.sum`

- [ ] **Step 1: Add the dep**

```bash
cd gormes
go get modernc.org/sqlite@latest
```

Expected: `go.mod` gains `modernc.org/sqlite v1.X.Y` in `require` (as indirect since no code imports it yet); `go.sum` gains hashes.

- [ ] **Step 2: Verify module graph compiles**

```bash
cd gormes
go build ./...
```

Expected: clean build. No code imports `modernc.org/sqlite` yet; DCE keeps it out of every existing binary.

- [ ] **Step 3: Confirm pre-existing binary sizes unchanged (DCE still working)**

```bash
cd gormes
make build
ls -lh bin/gormes
```

Expected: `bin/gormes` stays ~8.2 MB (pre-3.A size from commit `bf61992d`). If it jumps >100 KB, STOP — something accidentally started importing SQLite.

- [ ] **Step 4: Commit (from repo root, one level above `gormes/`)**

```bash
git add gormes/go.mod gormes/go.sum
git commit -m "$(cat <<'EOF'
build(gormes): add modernc.org/sqlite dependency

Preparatory commit for Phase 3.A Lattice Memory Foundation.
modernc is pure Go (CGO_ENABLED=0 compatible), ~5.9 MB
stripped — measured cheaper than ncruces/go-sqlite3's
wasm2go (~10 MB) on the same FTS5 smoke build on 2026-04-20.

No code imports it yet; dead-code elimination keeps all
existing binaries at their current sizes until Task 10
wires cmd/gormes-telegram.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Remove `AppendAssistantDraft` + add `RecordingStore`

**Files:**
- Modify: `gormes/internal/store/store.go`
- Create: `gormes/internal/store/recording.go`
- Create: `gormes/internal/store/recording_test.go`

- [ ] **Step 1: Confirm zero runtime callers of `AppendAssistantDraft`**

```bash
cd gormes
grep -rn "AppendAssistantDraft" --include="*.go" .
```

Expected output: exactly two matches, both in `internal/store/store.go` (the `const` and the `String()` switch case). If any other file matches, STOP — there's an unexpected caller that must be migrated first.

- [ ] **Step 2: Write the `RecordingStore` failing test FIRST**

Create `gormes/internal/store/recording_test.go`:

```go
package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRecordingStore_CapturesCommands(t *testing.T) {
	r := NewRecording()

	ctx := context.Background()
	_, err := r.Exec(ctx, Command{Kind: AppendUserTurn, Payload: json.RawMessage(`{"x":1}`)})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	_, err = r.Exec(ctx, Command{Kind: FinalizeAssistantTurn, Payload: json.RawMessage(`{"x":2}`)})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	got := r.Commands()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Kind != AppendUserTurn {
		t.Errorf("got[0].Kind = %v, want AppendUserTurn", got[0].Kind)
	}
	if got[1].Kind != FinalizeAssistantTurn {
		t.Errorf("got[1].Kind = %v, want FinalizeAssistantTurn", got[1].Kind)
	}
}

func TestRecordingStore_ConcurrentSafe(t *testing.T) {
	r := NewRecording()
	ctx := context.Background()
	done := make(chan struct{}, 50)
	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = r.Exec(ctx, Command{Kind: AppendUserTurn})
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
	if got := len(r.Commands()); got != 50 {
		t.Errorf("len(Commands) = %d, want 50", got)
	}
}

func TestRecordingStore_CtxCancelHonored(t *testing.T) {
	r := NewRecording()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Exec(ctx, Command{Kind: AppendUserTurn}); err == nil {
		t.Error("Exec on canceled ctx should return ctx.Err(), got nil")
	}
}
```

- [ ] **Step 3: Run — expect FAIL**

```bash
cd gormes
go test ./internal/store/... 2>&1 | head -5
```

Expected: `undefined: NewRecording` (or build error about missing type).

- [ ] **Step 4: Remove `AppendAssistantDraft` from `store.go`**

Open `gormes/internal/store/store.go`. Edit the `const` block and the `String()` switch:

```go
type CommandKind int

const (
	AppendUserTurn CommandKind = iota
	FinalizeAssistantTurn
)

func (c CommandKind) String() string {
	switch c {
	case AppendUserTurn:
		return "append_user_turn"
	case FinalizeAssistantTurn:
		return "finalize_assistant_turn"
	}
	return "unknown"
}
```

(Delete the `AppendAssistantDraft` line and its `"append_assistant_draft"` case.)

Also update the package comment at the top of `store.go` if it explicitly lists command kinds — replace the list with `AppendUserTurn, FinalizeAssistantTurn` only.

- [ ] **Step 5: Write `recording.go`**

Create `gormes/internal/store/recording.go`:

```go
package store

import (
	"context"
	"sync"
)

// Compile-time interface check.
var _ Store = (*RecordingStore)(nil)

// RecordingStore is a test double that captures every Command passed to
// Exec. Safe for concurrent use. Use Commands() to read a snapshot.
type RecordingStore struct {
	mu   sync.Mutex
	cmds []Command
}

// NewRecording constructs an empty RecordingStore.
func NewRecording() *RecordingStore { return &RecordingStore{} }

func (r *RecordingStore) Exec(ctx context.Context, cmd Command) (Ack, error) {
	if err := ctx.Err(); err != nil {
		return Ack{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmds = append(r.cmds, cmd)
	return Ack{}, nil
}

// Commands returns a snapshot slice — safe to iterate while other goroutines
// continue recording.
func (r *RecordingStore) Commands() []Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Command, len(r.cmds))
	copy(out, r.cmds)
	return out
}
```

- [ ] **Step 6: Run the full store suite + vet**

```bash
cd gormes
go test -race ./internal/store/... -v
go vet ./...
```

Expected: existing `NoopStore` / `SlowStore` tests still PASS + 3 new `RecordingStore` tests PASS. Vet clean.

- [ ] **Step 7: Full module sweep**

```bash
cd gormes
go test -race ./... -count=1 -timeout 120s
```

Expected: all green. Removing `AppendAssistantDraft` is a no-op for every caller (there were none).

- [ ] **Step 8: Commit (from repo root)**

```bash
git add gormes/internal/store/store.go \
        gormes/internal/store/recording.go \
        gormes/internal/store/recording_test.go
git commit -m "$(cat <<'EOF'
refactor(gormes/store): drop AppendAssistantDraft + add RecordingStore

AppendAssistantDraft was declared in Phase 1 but never called.
YAGNI cleanup — removes the enum value and its String() case.
Grep confirms zero runtime callers before removal.

RecordingStore is a test double that captures every Command
into an internal slice. Used by Task 8 to assert the kernel
now fires both AppendUserTurn and FinalizeAssistantTurn.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `internal/memory` — package + schema + OpenSqlite skeleton

**Files:**
- Create: `gormes/internal/memory/memory.go`
- Create: `gormes/internal/memory/schema.go`
- Create: `gormes/internal/memory/memory_test.go`

- [ ] **Step 1: Write failing test FIRST**

Create `gormes/internal/memory/memory_test.go`:

```go
package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSqlite_CreatesSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer s.Close(context.Background())

	// Confirm the turns table exists by running a no-op SELECT.
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM turns").Scan(&n); err != nil {
		t.Errorf("turns table missing: %v", err)
	}
	if n != 0 {
		t.Errorf("turns count at startup = %d, want 0", n)
	}

	// Confirm the FTS5 virtual table exists.
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
	if v != "3a" {
		t.Errorf("schema version = %q, want %q", v, "3a")
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
```

- [ ] **Step 2: Run — expect FAIL (package missing)**

```bash
cd gormes
go test ./internal/memory/... 2>&1 | head -5
```

Expected: `no Go files` or build error.

- [ ] **Step 3: Write `schema.go`**

Create `gormes/internal/memory/schema.go`:

```go
package memory

// schemaVersion is the string stored in schema_meta.v. Bump on every
// incompatible migration; Phase 3.B will introduce "3b" alongside.
const schemaVersion = "3a"

// schemaDDL is applied idempotently on every OpenSqlite. CREATE IF NOT
// EXISTS everywhere so re-open on an existing DB is a cheap no-op.
const schemaDDL = `
CREATE TABLE IF NOT EXISTS schema_meta (
	k TEXT PRIMARY KEY,
	v TEXT NOT NULL
);

INSERT OR IGNORE INTO schema_meta(k, v) VALUES ('version', '` + schemaVersion + `');

CREATE TABLE IF NOT EXISTS turns (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id  TEXT    NOT NULL,
	role        TEXT    NOT NULL CHECK(role IN ('user','assistant')),
	content     TEXT    NOT NULL,
	ts_unix     INTEGER NOT NULL,
	meta_json   TEXT
);

CREATE INDEX IF NOT EXISTS idx_turns_session_ts
	ON turns(session_id, ts_unix);

CREATE VIRTUAL TABLE IF NOT EXISTS turns_fts USING fts5(
	content,
	content='turns',
	content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS turns_ai AFTER INSERT ON turns BEGIN
	INSERT INTO turns_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS turns_ad AFTER DELETE ON turns BEGIN
	INSERT INTO turns_fts(turns_fts, rowid, content) VALUES('delete', old.id, old.content);
END;

CREATE TRIGGER IF NOT EXISTS turns_au AFTER UPDATE ON turns BEGIN
	INSERT INTO turns_fts(turns_fts, rowid, content) VALUES('delete', old.id, old.content);
	INSERT INTO turns_fts(rowid, content) VALUES (new.id, new.content);
END;
`
```

- [ ] **Step 4: Write `memory.go` (skeleton: open/close + Stats; no worker yet)**

Create `gormes/internal/memory/memory.go`:

```go
// Package memory is the SQLite-backed Phase-3.A Lattice Foundation.
// It implements store.Store with fire-and-forget semantics: Exec returns
// an Ack in microseconds after enqueueing a Command on a bounded channel;
// a single-owner background worker performs all SQL I/O. On queue-full:
// log + drop. See gormes/docs/superpowers/specs/2026-04-20-gormes-phase3a-memory-design.md.
package memory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	_ "modernc.org/sqlite"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
)

// defaultQueueCap is used when OpenSqlite receives queueCap <= 0.
const defaultQueueCap = 1024

// Stats exposes counters for tests and future telemetry. Not part of
// store.Store — consumers must hold a concrete *SqliteStore to call Stats.
type Stats struct {
	QueueLen int
	QueueCap int
	Drops    uint64
	Accepted uint64
}

// SqliteStore is a fire-and-forget store.Store backed by SQLite + FTS5.
type SqliteStore struct {
	db    *sql.DB
	queue chan store.Command
	done  chan struct{}
	log   *slog.Logger

	drops    atomic.Uint64
	accepted atomic.Uint64

	closeOnce sync.Once
}

// Compile-time interface check.
var _ store.Store = (*SqliteStore)(nil)

// OpenSqlite opens/creates the SQLite file at path, applies the schema,
// and starts the background worker goroutine. queueCap <= 0 falls back
// to defaultQueueCap. log == nil falls back to slog.Default().
func OpenSqlite(path string, queueCap int, log *slog.Logger) (*SqliteStore, error) {
	if log == nil {
		log = slog.Default()
	}
	if queueCap <= 0 {
		queueCap = defaultQueueCap
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("memory: create parent dir for %s: %w", path, err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("memory: open %s: %w", path, err)
	}
	// database/sql with modernc works best with a single writer connection.
	db.SetMaxOpenConns(1)

	if err := applyPragmas(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory: pragmas: %w", err)
	}
	if _, err := db.Exec(schemaDDL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory: apply schema: %w", err)
	}

	s := &SqliteStore{
		db:    db,
		queue: make(chan store.Command, queueCap),
		done:  make(chan struct{}),
		log:   log,
	}
	go s.run() // Task 5 will provide run()
	return s, nil
}

func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 2000",
		"PRAGMA foreign_keys = ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

// Exec is Task 4's scope.
func (s *SqliteStore) Exec(ctx context.Context, cmd store.Command) (store.Ack, error) {
	// Task 4 fills this in.
	return store.Ack{}, nil
}

// Stats returns a snapshot of worker counters.
func (s *SqliteStore) Stats() Stats {
	return Stats{
		QueueLen: len(s.queue),
		QueueCap: cap(s.queue),
		Drops:    s.drops.Load(),
		Accepted: s.accepted.Load(),
	}
}

// Close is Task 7's scope — for Task 3 just close DB and be done.
func (s *SqliteStore) Close(ctx context.Context) error {
	var err error
	s.closeOnce.Do(func() {
		// Task 7 replaces this with worker-drain-then-close.
		close(s.queue)
		<-s.done
		err = s.db.Close()
	})
	return err
}

// run is Task 5's scope — for Task 3 it just ranges over the queue and
// drops everything so tests can Close cleanly.
func (s *SqliteStore) run() {
	defer close(s.done)
	for range s.queue {
		// Task 5 replaces with real handling.
	}
}
```

- [ ] **Step 5: Run — expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -v
```

Expected: all 4 schema/open tests PASS.

Also run `go vet ./...` — must be clean.

- [ ] **Step 6: Commit (from repo root)**

```bash
git add gormes/internal/memory/memory.go \
        gormes/internal/memory/schema.go \
        gormes/internal/memory/memory_test.go \
        gormes/go.mod gormes/go.sum
git commit -m "$(cat <<'EOF'
feat(gormes/memory): package scaffold + schema + OpenSqlite

New internal/memory package. OpenSqlite:
  - Auto-creates parent dir (0700)
  - Opens SQLite at path (modernc.org/sqlite)
  - Applies WAL + synchronous=NORMAL + busy_timeout pragmas
  - Installs the 3a schema (turns + turns_fts + triggers +
    schema_meta) idempotently via CREATE IF NOT EXISTS

Exec and the real worker are stubs to be filled in by Tasks
4 and 5; this commit is the pure infrastructure foundation.

Compile-time check: SqliteStore satisfies store.Store.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Note: `go.mod` / `go.sum` are included because adding the `modernc.org/sqlite` import flips it from indirect to direct.

---

## Task 4: `SqliteStore.Exec` — fire-and-forget + drop-on-full

**Files:**
- Modify: `gormes/internal/memory/memory.go`
- Modify: `gormes/internal/memory/memory_test.go`

- [ ] **Step 1: Write failing tests (append to memory_test.go)**

Append to `gormes/internal/memory/memory_test.go`:

```go
import (
	// ... existing imports ...
	"encoding/json"
	"time"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
)

// (If the existing imports already have some of these, don't duplicate; just
// add whatever's missing.)

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
	// 10 ms is generous — real return should be sub-ms. Under race-detector
	// CI this still has headroom.
	if elapsed > 10*time.Millisecond {
		t.Errorf("Exec took %v, want well under 10 ms", elapsed)
	}
}

func TestSqliteStore_ExecDropsOnFullQueue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	// Use a tiny queue; keep the worker paused by not reading from the
	// channel. Since Task 3's run() drops everything silently we need a
	// different approach: build with queueCap that is small AND fire Execs
	// fast enough that the worker can't keep up. But Task 3's run() drops
	// instantly — so this test relies on Task 5 landing. For Task 4 we
	// assert the DROP COUNTER behavior by pre-filling the queue directly
	// before the worker gets a chance to drain it.
	//
	// Strategy: use queueCap=2, Exec 10 times back-to-back, assert that
	// Drops > 0 and Accepted + Drops == 10 (allowing for timing — the
	// worker may drain between sends).

	s, _ := OpenSqlite(path, 2, nil)
	defer s.Close(context.Background())

	// Temporarily pause the worker by grabbing a DB-level lock using a
	// long read. Skip this complexity: just fire 1000 commands at queueCap=2.
	// With Task 3's drop-only worker, Accepted stays 0 and Drops counts the
	// overflows.
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
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd gormes
go test -race ./internal/memory/... -run "ExecReturnsFast|ExecDropsOnFullQueue|ExecHonorsCtxCancel" -v
```

Expected: `ExecDropsOnFullQueue` fails because Task 3's `Exec` always returns `Ack{}, nil` without enqueueing. Drops counter stays 0.

- [ ] **Step 3: Replace `Exec` body in `memory.go`**

In `gormes/internal/memory/memory.go`, replace the stub `Exec` with:

```go
// Exec enqueues cmd on the worker queue. Returns an Ack in microseconds.
// On queue full: increments Drops counter, logs a WARN, and returns
// Ack{} — the caller cannot tell the difference. This is the deliberate
// Zero-Leak design: a dropped turn is acceptable degradation; a blocked
// kernel is not.
func (s *SqliteStore) Exec(ctx context.Context, cmd store.Command) (store.Ack, error) {
	if err := ctx.Err(); err != nil {
		return store.Ack{}, err
	}
	select {
	case s.queue <- cmd:
		s.accepted.Add(1)
	default:
		s.drops.Add(1)
		s.log.Warn("memory: queue full, dropping command",
			"kind", cmd.Kind.String(),
			"queue_cap", cap(s.queue),
			"drops_total", s.drops.Load())
	}
	return store.Ack{}, nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -v
```

Expected: all memory tests PASS (including the new three).

Also `go vet ./...` — clean.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/memory/memory.go \
        gormes/internal/memory/memory_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): SqliteStore.Exec — fire-and-forget + drop-on-full

Exec enqueues on the cap-N queue or increments the drops
counter if the queue is full. Returns Ack in microseconds;
the kernel never sees disk I/O in the hot path.

Tests pin three invariants:
  - Exec returns well under the 250 ms StoreAckDeadline
  - Drops + Accepted = total calls (no command evaporates)
  - Canceled ctx returns ctx.Err() immediately

Worker run() still stubs — Task 5 activates real SQL writes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Worker goroutine — real SQL writes

**Files:**
- Create: `gormes/internal/memory/worker.go`
- Modify: `gormes/internal/memory/memory.go` (remove the stub `run()`)
- Modify: `gormes/internal/memory/memory_test.go`

- [ ] **Step 1: Write failing test (append to memory_test.go)**

Append:

```go
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

	// Drain: the worker is async. Wait until Accepted > 0 AND the row is
	// visible on disk (worker commits synchronously inside run loop).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.Stats().Accepted > 0 {
			break
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
	for time.Now().Before(deadline) && s.Stats().Accepted < 1 {
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
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd gormes
go test -race ./internal/memory/... -run Worker -v
```

Expected: tests time out waiting for rows because Task 3's `run()` silently drops.

- [ ] **Step 3: Delete the stub `run()` from `memory.go`**

Remove the `func (s *SqliteStore) run()` definition from `memory.go`. The real implementation lives in `worker.go` next.

- [ ] **Step 4: Write `worker.go`**

Create `gormes/internal/memory/worker.go`:

```go
package memory

import (
	"context"
	"encoding/json"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
)

// turnPayload is the shared JSON schema for AppendUserTurn and
// FinalizeAssistantTurn. See spec §7.3.
type turnPayload struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	TsUnix    int64  `json:"ts_unix"`
}

// run is the worker loop. Exactly one goroutine owns s.db.
func (s *SqliteStore) run() {
	defer close(s.done)
	for cmd := range s.queue {
		s.handleCommand(cmd)
	}
}

func (s *SqliteStore) handleCommand(cmd store.Command) {
	var p turnPayload
	if err := json.Unmarshal(cmd.Payload, &p); err != nil {
		s.log.Warn("memory: malformed payload, dropping",
			"kind", cmd.Kind.String(), "err", err)
		return
	}
	if p.SessionID == "" || p.Content == "" {
		s.log.Warn("memory: empty session_id or content, dropping",
			"kind", cmd.Kind.String())
		return
	}
	var role string
	switch cmd.Kind {
	case store.AppendUserTurn:
		role = "user"
	case store.FinalizeAssistantTurn:
		role = "assistant"
	default:
		s.log.Warn("memory: unknown command kind, dropping", "kind", cmd.Kind.String())
		return
	}
	_, err := s.db.ExecContext(context.Background(),
		"INSERT INTO turns(session_id, role, content, ts_unix) VALUES(?, ?, ?, ?)",
		p.SessionID, role, p.Content, p.TsUnix)
	if err != nil {
		s.log.Warn("memory: INSERT failed", "kind", cmd.Kind.String(), "err", err)
	}
}
```

- [ ] **Step 5: Run — expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -v
```

Expected: all worker tests PASS. Existing tests still PASS.

- [ ] **Step 6: Commit (from repo root)**

```bash
git add gormes/internal/memory/memory.go gormes/internal/memory/worker.go gormes/internal/memory/memory_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): worker goroutine writes turns to SQLite

Real worker loop now unmarshals the shared turnPayload
({session_id, content, ts_unix}) and INSERTs into turns
with role derived from Command.Kind:
  - AppendUserTurn       -> role 'user'
  - FinalizeAssistantTurn -> role 'assistant'

Malformed payloads are logged and dropped — the worker never
panics on bad input. An unknown command kind is also dropped
(defensive: today the two kinds cover everything).

The FTS5 triggers installed in Task 3's schema keep turns_fts
auto-synced on every INSERT; Task 6 pins MATCH behavior.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: FTS5 `MATCH` behaviour tests

**Files:**
- Create: `gormes/internal/memory/fts5_test.go`

- [ ] **Step 1: Write the tests**

Create `gormes/internal/memory/fts5_test.go`:

```go
package memory

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
)

func insertTurn(t *testing.T, s *SqliteStore, sid, content string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{
		"session_id": sid, "content": content, "ts_unix": time.Now().Unix(),
	})
	_, _ = s.Exec(context.Background(), store.Command{
		Kind: store.AppendUserTurn, Payload: payload,
	})
}

func waitForAccepted(t *testing.T, s *SqliteStore, want int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if int(s.Stats().Accepted) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for Accepted >= %d; got %d", want, s.Stats().Accepted)
}

func TestFTS5_MatchBasic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	insertTurn(t, s, "s1", "I remember the word asparagus")
	insertTurn(t, s, "s1", "and the capital of portugal is Lisbon")
	insertTurn(t, s, "s2", "banana")
	waitForAccepted(t, s, 3, 2*time.Second)

	rows, err := s.db.Query(`SELECT rowid FROM turns_fts WHERE turns_fts MATCH ?`, "asparagus")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	if len(ids) != 1 {
		t.Errorf("MATCH asparagus returned %d rows, want 1", len(ids))
	}
}

func TestFTS5_MatchPhrase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	insertTurn(t, s, "s1", "gormes telegram binary is awesome")
	insertTurn(t, s, "s1", "telegram works; gormes works")
	insertTurn(t, s, "s1", "unrelated message about coffee")
	waitForAccepted(t, s, 3, 2*time.Second)

	// Phrase query: "gormes telegram" must appear as a bigram.
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM turns_fts WHERE turns_fts MATCH ?`,
		`"gormes telegram"`,
	).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf(`MATCH "gormes telegram" returned %d rows, want 1`, n)
	}
}

func TestFTS5_DeleteTriggerUpdatesIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	insertTurn(t, s, "s1", "ephemeral memory candidate")
	waitForAccepted(t, s, 1, 2*time.Second)

	var n int
	_ = s.db.QueryRow(
		`SELECT COUNT(*) FROM turns_fts WHERE turns_fts MATCH ?`, "ephemeral",
	).Scan(&n)
	if n != 1 {
		t.Fatalf("precondition: expected 1 FTS hit, got %d", n)
	}

	if _, err := s.db.Exec("DELETE FROM turns"); err != nil {
		t.Fatal(err)
	}

	_ = s.db.QueryRow(
		`SELECT COUNT(*) FROM turns_fts WHERE turns_fts MATCH ?`, "ephemeral",
	).Scan(&n)
	if n != 0 {
		t.Errorf("after DELETE, FTS hit count = %d, want 0", n)
	}
}
```

- [ ] **Step 2: Run — expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run FTS5 -v
```

Expected: all 3 FTS5 tests PASS. The triggers installed in Task 3's schema make this work without any production-code changes in Task 6.

- [ ] **Step 3: Commit (from repo root)**

```bash
git add gormes/internal/memory/fts5_test.go
git commit -m "$(cat <<'EOF'
test(gormes/memory): FTS5 MATCH + trigger sync

Three tests pin the FTS5 contract:
  - Basic word MATCH returns only the row containing the term
  - Phrase MATCH ("gormes telegram") requires a bigram, not
    just both tokens
  - DELETE FROM turns triggers purge the FTS index

All three rely on the triggers installed in Task 3's schema;
no production-code changes required for this commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Graceful shutdown — drain queue + idempotent Close

**Files:**
- Modify: `gormes/internal/memory/memory.go`
- Create: `gormes/internal/memory/shutdown_test.go`

- [ ] **Step 1: Write failing tests**

Create `gormes/internal/memory/shutdown_test.go`:

```go
package memory

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
)

func TestClose_DrainsQueue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 4096, nil)

	const N = 100
	for i := 0; i < N; i++ {
		payload, _ := json.Marshal(map[string]any{
			"session_id": "s", "content": "msg", "ts_unix": int64(i),
		})
		_, _ = s.Exec(context.Background(), store.Command{
			Kind: store.AppendUserTurn, Payload: payload,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// DB is closed inside Close; reopen to verify.
	s2, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close(context.Background())

	var n int
	if err := s2.db.QueryRow("SELECT COUNT(*) FROM turns").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != N {
		t.Errorf("persisted turns = %d, want %d", n, N)
	}
}

func TestClose_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)

	ctx := context.Background()
	if err := s.Close(ctx); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := s.Close(ctx); err != nil {
		t.Errorf("second Close should be no-op, got %v", err)
	}
}

func TestClose_HonorsDeadline(t *testing.T) {
	// With a zero-deadline context, Close should return promptly even if
	// the worker has not finished. The DB still gets db.Close()'d so WAL
	// checkpoint runs; no panic, no leak.
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 4096, nil)

	// Pre-fill the queue.
	for i := 0; i < 500; i++ {
		payload, _ := json.Marshal(map[string]any{
			"session_id": "s", "content": "x", "ts_unix": int64(i),
		})
		_, _ = s.Exec(context.Background(), store.Command{
			Kind: store.AppendUserTurn, Payload: payload,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	// Close should return very soon (we don't strictly require a bound,
	// but it must NOT hang). Timeout the test harness if it does.
	done := make(chan error, 1)
	go func() { done <- s.Close(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close on tiny deadline returned err = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close hung despite ctx deadline")
	}
}
```

- [ ] **Step 2: Run — expect PASS for Drains/Idempotent, FAIL for HonorsDeadline**

```bash
cd gormes
go test -race ./internal/memory/... -run "Close_" -v
```

Task 3's stub `Close` already drains (channel close → for-range exits after drain) and is idempotent via `sync.Once`. So Drains + Idempotent likely PASS.

Expected: `TestClose_HonorsDeadline` HANGS or fails, because the current Close unconditionally waits for `<-s.done`.

- [ ] **Step 3: Replace `Close` in `memory.go`**

Replace the existing `Close` with:

```go
// Close signals the worker to drain, waits up to ctx deadline for drain,
// then closes the underlying *sql.DB (which flushes WAL). Idempotent —
// subsequent calls return nil.
func (s *SqliteStore) Close(ctx context.Context) error {
	var closeErr error
	s.closeOnce.Do(func() {
		close(s.queue) // signal worker to exit after draining
		select {
		case <-s.done:
			// drained cleanly
		case <-ctx.Done():
			s.log.Warn("memory: shutdown deadline exceeded; in-flight writes may be lost",
				"queue_len", len(s.queue))
		}
		closeErr = s.db.Close()
	})
	return closeErr
}
```

Note: the `sync.Once`-guarded `close(s.queue)` means a second Close does not double-close the channel (which would panic).

- [ ] **Step 4: Run — expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -v
```

Expected: all memory tests PASS (including the three shutdown tests and every earlier memory test).

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/memory/memory.go gormes/internal/memory/shutdown_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): graceful shutdown with ctx-deadline honouring

Close now races worker-drain vs. ctx.Done(). On happy path
the worker ranges over the closed queue and exits; on budget
exceeded Close logs WARN and proceeds to db.Close() anyway
so WAL is flushed regardless.

sync.Once guards the channel close so a caller racing two
Close() calls cannot cause a double-close panic.

Three new tests:
  - Drains 100 queued commands to disk, reopens, verifies all 100
  - Close is idempotent (second call returns nil)
  - tiny-deadline Close does not hang (the 5 s harness timeout
    is the safety net; real Close returns in <100 ms)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Kernel — activate `FinalizeAssistantTurn` + tighten payload

**Files:**
- Modify: `gormes/internal/kernel/kernel.go`
- Create: `gormes/internal/kernel/finalize_store_test.go`

- [ ] **Step 1: Write failing test**

Create `gormes/internal/kernel/finalize_store_test.go`:

```go
package kernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/telemetry"
)

// TestKernel_FinalizeAssistantTurnReachesStore proves the kernel fires
// both AppendUserTurn (pre-stream) and FinalizeAssistantTurn (post-stream)
// on every successful turn, with matching session_id and content.
func TestKernel_FinalizeAssistantTurnReachesStore(t *testing.T) {
	rec := store.NewRecording()

	mc := hermes.NewMockClient()
	reply := "hello back"
	events := make([]hermes.Event, 0, len(reply)+1)
	for _, ch := range reply {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop"})
	mc.Script(events, "sess-finalize-test")

	k := New(Config{
		Model:     "hermes-agent",
		Endpoint:  "http://mock",
		Admission: Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, mc, rec, telemetry.New(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()

	if err := k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "hi"}); err != nil {
		t.Fatal(err)
	}

	waitForFrameMatching(t, k.Render(), func(f RenderFrame) bool {
		return f.Phase == PhaseIdle && f.SessionID != ""
	}, 3*time.Second)

	cmds := rec.Commands()
	if len(cmds) < 2 {
		t.Fatalf("len(cmds) = %d, want >= 2 (AppendUserTurn + FinalizeAssistantTurn)", len(cmds))
	}

	// First command must be AppendUserTurn with the user's text.
	if cmds[0].Kind != store.AppendUserTurn {
		t.Errorf("cmds[0].Kind = %v, want AppendUserTurn", cmds[0].Kind)
	}
	var p1 struct {
		SessionID string `json:"session_id"`
		Content   string `json:"content"`
		TsUnix    int64  `json:"ts_unix"`
	}
	if err := json.Unmarshal(cmds[0].Payload, &p1); err != nil {
		t.Fatalf("cmds[0] payload: %v", err)
	}
	if p1.Content != "hi" {
		t.Errorf("AppendUserTurn content = %q, want %q", p1.Content, "hi")
	}
	if p1.TsUnix == 0 {
		t.Errorf("AppendUserTurn ts_unix is zero")
	}

	// A later command must be FinalizeAssistantTurn with the assistant's reply.
	var foundFinalize bool
	for _, c := range cmds[1:] {
		if c.Kind != store.FinalizeAssistantTurn {
			continue
		}
		var p2 struct {
			SessionID string `json:"session_id"`
			Content   string `json:"content"`
			TsUnix    int64  `json:"ts_unix"`
		}
		if err := json.Unmarshal(c.Payload, &p2); err != nil {
			t.Fatalf("FinalizeAssistantTurn payload: %v", err)
		}
		if !strings.Contains(p2.Content, "hello back") {
			t.Errorf("FinalizeAssistantTurn content = %q, want contains 'hello back'", p2.Content)
		}
		if p2.SessionID != "sess-finalize-test" {
			t.Errorf("FinalizeAssistantTurn session_id = %q", p2.SessionID)
		}
		foundFinalize = true
		break
	}
	if !foundFinalize {
		t.Errorf("no FinalizeAssistantTurn command captured; got kinds = %v", kinds(cmds))
	}
}

func kinds(cmds []store.Command) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = c.Kind.String()
	}
	return out
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd gormes
go test ./internal/kernel/... -run TestKernel_FinalizeAssistantTurnReachesStore -v 2>&1 | head -20
```

Expected: test FAILS — the AppendUserTurn payload today is `{"text":"..."}` (not `{"session_id","content","ts_unix"}`), AND no FinalizeAssistantTurn call exists at all.

- [ ] **Step 3: Update `AppendUserTurn` payload shape in `kernel.go`**

Open `gormes/internal/kernel/kernel.go`. Find the block around line 180-183 that currently looks like:

```go
// 2. Persist user turn with hard 250ms ack deadline (spec §7.8 store row).
storeCtx, storeCancel := context.WithTimeout(ctx, StoreAckDeadline)
payload := []byte(fmt.Sprintf(`{"text":%q}`, text))
_, err := k.store.Exec(storeCtx, store.Command{Kind: store.AppendUserTurn, Payload: payload})
storeCancel()
```

Replace with:

```go
// 2. Persist user turn with hard 250ms ack deadline (spec §7.8 store row).
//    Phase 3.A: payload shape matches the shared turnPayload used by both
//    AppendUserTurn and FinalizeAssistantTurn.
storeCtx, storeCancel := context.WithTimeout(ctx, StoreAckDeadline)
userPayload, _ := json.Marshal(map[string]any{
	"session_id": k.sessionID,
	"content":    text,
	"ts_unix":    time.Now().Unix(),
})
_, err := k.store.Exec(storeCtx, store.Command{Kind: store.AppendUserTurn, Payload: userPayload})
storeCancel()
```

If `encoding/json` is not already imported, add it to the import block of `kernel.go`.

- [ ] **Step 4: Add `FinalizeAssistantTurn` call after stream finalization**

Find the block around line 385-392 in `kernel.go` that appends the final assistant message to history and emits the idle frame. It looks like:

```go
	k.history = append(k.history, hermes.Message{Role: "assistant", Content: k.draft})
```

followed further down by:

```go
	k.phase = PhaseIdle
	k.lastError = ""
	k.emitFrame("idle")
```

Insert the FinalizeAssistantTurn call **between** the history-append and the idle-emit — immediately after the history append, before the phase transition:

```go
k.history = append(k.history, hermes.Message{Role: "assistant", Content: k.draft})

// Phase 3.A: finalize in the memory store. Fire-and-forget — the worker
// handles I/O off the hot path. 250ms context bound kept as a safety net
// in case someone injects a synchronous store in the future.
{
	finalPayload, _ := json.Marshal(map[string]any{
		"session_id": k.sessionID,
		"content":    k.draft,
		"ts_unix":    time.Now().Unix(),
	})
	finalCtx, finalCancel := context.WithTimeout(ctx, StoreAckDeadline)
	_, _ = k.store.Exec(finalCtx, store.Command{Kind: store.FinalizeAssistantTurn, Payload: finalPayload})
	finalCancel()
}
```

**IMPORTANT:** the FinalizeAssistantTurn write must happen ONLY on the happy finalization path — NOT on the cancellation path (line ~385 `k.emitFrame("cancelled")`) and NOT on the failure path (line ~364 `k.emitFrame("stream error")`). We record completed turns only.

If the codebase's actual layout has been restructured since this plan was drafted, find the existing `k.history = append(k.history, hermes.Message{Role: "assistant", ...})` line and insert the finalize block immediately after it — that line uniquely marks the "assistant turn succeeded" waypoint.

- [ ] **Step 5: Run — expect PASS**

```bash
cd gormes
go test -race ./internal/kernel/... -timeout 90s
```

Expected: the new `TestKernel_FinalizeAssistantTurnReachesStore` PASSes, and every pre-existing kernel test still passes. In particular, the `SlowStore` / 250ms-deadline test must remain green — the new Exec call uses the same `StoreAckDeadline` bound.

Also `go vet ./...` — clean.

- [ ] **Step 6: Full module sweep**

```bash
cd gormes
go test -race ./... -count=1 -timeout 120s
```

Expected: all green.

- [ ] **Step 7: Commit (from repo root)**

```bash
git add gormes/internal/kernel/kernel.go gormes/internal/kernel/finalize_store_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/kernel): activate FinalizeAssistantTurn + tighten payload

AppendUserTurn's payload shape moves from the minimal
{"text": "..."} used since Phase 1 to the shared Phase-3.A
shape {"session_id","content","ts_unix"}. NoopStore discards
the payload, so no other tests care.

A new FinalizeAssistantTurn call fires immediately after the
successful history-append at the end of runTurn — NOT on the
cancelled or failed paths. The write is fire-and-forget
wrapped in a 250ms ctx bound (vestigial safety net; real
SqliteStore.Exec returns in microseconds).

TestKernel_FinalizeAssistantTurnReachesStore uses the new
store.RecordingStore test double to assert both commands
arrive in order with matching session_id and content.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Config — `MemoryQueueCap` + `MemoryDBPath()`

**Files:**
- Modify: `gormes/internal/config/config.go`
- Modify: `gormes/internal/config/config_test.go`

- [ ] **Step 1: Write failing tests (append)**

Append to `gormes/internal/config/config_test.go`:

```go
func TestLoad_MemoryQueueCapDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telegram.MemoryQueueCap != 1024 {
		t.Errorf("MemoryQueueCap default = %d, want 1024", cfg.Telegram.MemoryQueueCap)
	}
}

func TestMemoryDBPath_HonorsXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/gormes-test-memxdg")
	got := MemoryDBPath()
	want := "/tmp/gormes-test-memxdg/gormes/memory.db"
	if got != want {
		t.Errorf("MemoryDBPath() = %q, want %q", got, want)
	}
}

func TestMemoryDBPath_DefaultsToHomeLocalShare(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	home, _ := os.UserHomeDir()
	got := MemoryDBPath()
	want := filepath.Join(home, ".local", "share", "gormes", "memory.db")
	if got != want {
		t.Errorf("MemoryDBPath() default = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd gormes
go test ./internal/config/... -run "MemoryQueueCap|MemoryDBPath" 2>&1 | head -10
```

Expected: `unknown field MemoryQueueCap in struct`, `undefined: MemoryDBPath`.

- [ ] **Step 3: Extend `config.go`**

1. Add `MemoryQueueCap` to `TelegramCfg`:

```go
type TelegramCfg struct {
	BotToken          string `toml:"bot_token"`
	AllowedChatID     int64  `toml:"allowed_chat_id"`
	CoalesceMs        int    `toml:"coalesce_ms"`
	FirstRunDiscovery bool   `toml:"first_run_discovery"`
	// MemoryQueueCap (Phase 3.A): async worker queue capacity in
	// cmd/gormes-telegram's SqliteStore. Defaults to 1024.
	MemoryQueueCap    int    `toml:"memory_queue_cap"`
}
```

2. Extend `defaults()`:

```go
Telegram: TelegramCfg{
	CoalesceMs:        1000,
	FirstRunDiscovery: true,
	MemoryQueueCap:    1024,
},
```

3. Add `MemoryDBPath()` near `SessionDBPath()`:

```go
// MemoryDBPath returns the default location of the Phase-3.A SQLite
// memory database. Honors XDG_DATA_HOME; falls back to
// ~/.local/share/gormes/memory.db.
func MemoryDBPath() string {
	return filepath.Join(xdgDataHome(), "gormes", "memory.db")
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
cd gormes
go test -race ./internal/config/... -v
go vet ./...
```

Expected: all 3 new tests PASS, existing config tests still PASS.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/config/config.go gormes/internal/config/config_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/config): MemoryQueueCap + MemoryDBPath()

MemoryQueueCap is the [telegram] TOML knob for the Phase-3.A
SqliteStore's worker queue capacity. Default 1024 — generous
enough that drops only fire under multi-second disk stalls.
No env override (operator-level tuning).

MemoryDBPath() is exported for symmetry with LogPath() and
SessionDBPath(), honouring XDG_DATA_HOME with the same
~/.local/share/gormes/ fallback.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Wire `cmd/gormes-telegram` to `SqliteStore`

**Files:**
- Modify: `gormes/cmd/gormes-telegram/main.go`

- [ ] **Step 1: Edit `main.go`**

Open `gormes/cmd/gormes-telegram/main.go`. Two changes:

1. Add the `internal/memory` import to the import block:

```go
"github.com/XelHaku/golang-hermes-agent/gormes/internal/memory"
```

2. After the existing `defer smap.Close()` line (Phase 2.C's session map) and BEFORE `hc := hermes.NewHTTPClient(...)`, insert:

```go
// Phase 3.A — open the SQLite memory store; worker starts immediately.
mstore, err := memory.OpenSqlite(config.MemoryDBPath(), cfg.Telegram.MemoryQueueCap, slog.Default())
if err != nil {
	return fmt.Errorf("memory store: %w", err)
}
defer func() {
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
	defer cancelShutdown()
	if err := mstore.Close(shutdownCtx); err != nil {
		slog.Warn("memory store close", "err", err)
	}
}()
```

3. Change the `kernel.New(...)` argument from `store.NewNoop()` to `mstore`. The surrounding call becomes:

```go
k := kernel.New(kernel.Config{
	Model:             cfg.Hermes.Model,
	Endpoint:          cfg.Hermes.Endpoint,
	Admission:         kernel.Admission{MaxBytes: cfg.Input.MaxBytes, MaxLines: cfg.Input.MaxLines},
	Tools:             reg,
	MaxToolIterations: 10,
	MaxToolDuration:   30 * time.Second,
	InitialSessionID:  initialSID,
}, hc, mstore, tm, slog.Default())
```

4. Remove the now-unused `store` import from the import block (grep `store.NewNoop` first — if no other use remains, delete the line `"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"`).

5. Update the startup log line to include the memory db path:

```go
slog.Info("gormes-telegram starting",
	"endpoint", cfg.Hermes.Endpoint,
	"allowed_chat_id", cfg.Telegram.AllowedChatID,
	"discovery", cfg.Telegram.FirstRunDiscovery,
	"sessions_db", config.SessionDBPath(),
	"memory_db", config.MemoryDBPath())
```

- [ ] **Step 2: Build + vet + size check**

```bash
cd gormes
go build ./...
go vet ./...
go build -o bin/gormes-telegram ./cmd/gormes-telegram
ls -lh bin/gormes-telegram
```

Expected: build succeeds; `bin/gormes-telegram` size ≤ 17 MB (target ~15-16 MB per spec §13). If >17 MB, STOP and report.

Also confirm TUI size unchanged:

```bash
cd gormes
make build
ls -lh bin/gormes
```

Expected: `bin/gormes` stays 8.2 MB. If it grew, `cmd/gormes` accidentally imported memory.

- [ ] **Step 3: Bot startup smoke**

```bash
cd gormes
./bin/gormes-telegram 2>&1 | head -3
```

Expected: exits 1 with `"no Telegram bot token — ..."` (unchanged from Phase 2.B.1). The memory.db is NOT created on this path because the token check fires before `OpenSqlite`.

Now actually trigger Open:

```bash
cd gormes
export XDG_DATA_HOME=/tmp/gormes-3a-smoke-$$
GORMES_TELEGRAM_TOKEN=fake:token GORMES_TELEGRAM_CHAT_ID=99 \
  timeout 2 ./bin/gormes-telegram 2>&1 | tail -5 || true
ls -la $XDG_DATA_HOME/gormes/
rm -rf $XDG_DATA_HOME
```

Expected: at least `memory.db` exists (possibly `memory.db-wal` / `-shm` too) with mode 0600.

- [ ] **Step 4: Full module sweep**

```bash
cd gormes
go test -race ./... -count=1 -timeout 120s
```

Expected: all green. (No test reached into cmd/gormes-telegram; tests cover internal/memory, internal/kernel, etc.)

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/cmd/gormes-telegram/main.go
git commit -m "$(cat <<'EOF'
feat(gormes/cmd/telegram): wire SqliteStore as kernel store

cmd/gormes-telegram now constructs memory.SqliteStore from
config.MemoryDBPath() + cfg.Telegram.MemoryQueueCap and passes
it to kernel.New. Shutdown budget wraps Close in a
kernel.ShutdownBudget-deadlined context to honour the
fire-and-forget drain contract.

NoopStore is no longer imported by the bot; only the TUI
still uses it. Build isolation locks this in (Task 12).

Startup log line includes the memory.db path so operators
can verify which file is being written.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: End-to-end — `TestBot_TurnPersistsToSqlite`

**Files:**
- Modify: `gormes/internal/telegram/bot_test.go`

- [ ] **Step 1: Write failing test (append)**

Append to `gormes/internal/telegram/bot_test.go`:

```go
// Imports may already include most of what's needed. Add if missing:
//   "path/filepath"
//   "github.com/XelHaku/golang-hermes-agent/gormes/internal/memory"

// TestBot_TurnPersistsToSqlite proves the full Phase 3.A flow:
// Telegram message -> kernel -> SqliteStore -> turns table. Uses a real
// (on-disk) memory.db in t.TempDir() + mock Telegram client + scripted
// hermes.MockClient. No network, no api_server.
func TestBot_TurnPersistsToSqlite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	mstore, err := memory.OpenSqlite(dbPath, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer mstore.Close(context.Background())

	mc := newMockClient()

	hmc := hermes.NewMockClient()
	reply := "pong"
	events := make([]hermes.Event, 0, len(reply)+1)
	for _, ch := range reply {
		events = append(events, hermes.Event{Kind: hermes.EventToken, Token: string(ch), TokensOut: 1})
	}
	events = append(events, hermes.Event{Kind: hermes.EventDone, FinishReason: "stop"})
	hmc.Script(events, "sess-phase3a-e2e")

	k := kernel.New(kernel.Config{
		Model: "hermes-agent", Endpoint: "http://mock",
		Admission: kernel.Admission{MaxBytes: 200_000, MaxLines: 10_000},
	}, hmc, mstore, telemetry.New(), nil)

	b := New(Config{AllowedChatID: 42, CoalesceMs: 100}, mc, k, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	go k.Run(ctx)
	<-k.Render()
	go func() { _ = b.Run(ctx) }()

	mc.pushTextUpdate(42, "ping")

	// Wait for the full turn: AppendUserTurn + FinalizeAssistantTurn
	// -> worker drains -> 2 rows in turns.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if int(mstore.Stats().Accepted) >= 2 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	// Read rows with a fresh sql.DB handle via mstore's accessor.
	// Use mstore's own db — it's unexported but the package allows in-test
	// access via an exported helper if needed; for e2e we re-open with a
	// READ-ONLY handle to avoid racing the worker.
	cancel()
	mc.closeUpdates()
	time.Sleep(150 * time.Millisecond) // let worker drain

	// Close and reopen for a clean read.
	if err := mstore.Close(context.Background()); err != nil {
		t.Fatalf("mstore close: %v", err)
	}

	verify, err := memory.OpenSqlite(dbPath, 0, nil)
	if err != nil {
		t.Fatalf("verify open: %v", err)
	}
	defer verify.Close(context.Background())

	type turn struct {
		Role, Content string
	}
	var turns []turn
	rows, err := verify.DB().Query("SELECT role, content FROM turns ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var t turn
		if err := rows.Scan(&t.Role, &t.Content); err != nil {
			rows.Close()
			break
		}
		turns = append(turns, t)
	}
	rows.Close()

	if len(turns) < 2 {
		t.Fatalf("len(turns) = %d, want >= 2", len(turns))
	}
	if turns[0].Role != "user" || turns[0].Content != "ping" {
		t.Errorf("turns[0] = %+v, want {user, ping}", turns[0])
	}
	var foundAsst bool
	for _, tt := range turns {
		if tt.Role == "assistant" && strings.Contains(tt.Content, "pong") {
			foundAsst = true
			break
		}
	}
	if !foundAsst {
		t.Errorf("no assistant turn with 'pong'; rows = %+v", turns)
	}
}
```

- [ ] **Step 2: Add `DB()` accessor to `memory.SqliteStore`**

The test needs read-only access to the underlying `*sql.DB` for verification. Add this to `gormes/internal/memory/memory.go` near `Stats`:

```go
// DB returns the underlying *sql.DB handle. Exposed for read-only test
// verification; production callers should not depend on this.
func (s *SqliteStore) DB() *sql.DB { return s.db }
```

- [ ] **Step 3: Run — expect PASS**

```bash
cd gormes
go test -race ./internal/telegram/... -run TestBot_TurnPersistsToSqlite -v -timeout 30s
```

Expected: PASS.

Also run the full telegram suite:

```bash
cd gormes
go test -race ./internal/telegram/... -count=1 -timeout 60s
```

Expected: all previous telegram tests still PASS.

- [ ] **Step 4: Commit (from repo root)**

```bash
git add gormes/internal/memory/memory.go gormes/internal/telegram/bot_test.go
git commit -m "$(cat <<'EOF'
test(gormes/telegram): Phase-3.A end-to-end SQLite persistence

TestBot_TurnPersistsToSqlite exercises the full flow:
  Telegram update -> bot.handleUpdate -> kernel.Submit
    -> kernel turn loop
    -> SqliteStore.Exec (AppendUserTurn + FinalizeAssistantTurn)
    -> worker inserts two rows into turns

Verification closes the write handle and reopens read-only
before SELECTing (avoids racing the worker).

Exposes memory.SqliteStore.DB() for read-only test access;
production code must not depend on it (commented as such).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Build-isolation — kernel + TUI must not import memory/sqlite

**Files:**
- Modify: `gormes/internal/buildisolation_test.go`

- [ ] **Step 1: Append two tests**

Append to `gormes/internal/buildisolation_test.go`:

```go
// TestKernelHasNoMemoryDep guards the Phase 3.A boundary: internal/kernel
// must never transitively import internal/memory or modernc.org/sqlite.
// If either appears in the kernel's dep graph, persistence has leaked into
// the turn loop and the 250ms StoreAckDeadline is structurally at risk.
func TestKernelHasNoMemoryDep(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./internal/kernel")
	cmd.Dir = ".."
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out.String())
	}

	for _, d := range strings.Split(out.String(), "\n") {
		if strings.Contains(d, "modernc.org/sqlite") ||
			strings.Contains(d, "/internal/memory") {
			t.Errorf("internal/kernel transitively depends on %q — Phase 3.A isolation violated", d)
		}
	}
}

// TestTUIBinaryHasNoSqliteDep guards the Operational Moat: cmd/gormes
// (the TUI) must never transitively depend on modernc.org/sqlite or on
// the internal/memory package. If either appears, the TUI's <10 MB
// binary budget is at risk and the dual-store architecture is breached.
func TestTUIBinaryHasNoSqliteDep(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "./cmd/gormes")
	cmd.Dir = ".."
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out.String())
	}

	for _, d := range strings.Split(out.String(), "\n") {
		if strings.Contains(d, "modernc.org/sqlite") ||
			strings.Contains(d, "/internal/memory") {
			t.Errorf("cmd/gormes transitively depends on %q — Dual-Store Architecture violated", d)
		}
	}
}
```

No new imports needed — `bytes`, `os/exec`, `strings`, `testing` are already imported by earlier isolation tests.

- [ ] **Step 2: Run**

```bash
cd gormes
go test -race ./internal/ -run "TestKernelHasNoMemoryDep|TestTUIBinaryHasNoSqliteDep" -v
```

Expected: both PASS.

- [ ] **Step 3: Sanity-break (kernel)**

Temporarily add to the TOP of `gormes/internal/kernel/kernel.go`'s import block:

```go
_ "github.com/XelHaku/golang-hermes-agent/gormes/internal/memory"
```

Re-run the kernel test:

```bash
cd gormes
go test ./internal/ -run TestKernelHasNoMemoryDep -v 2>&1 | tail -10
```

Expected: FAIL, naming both `modernc.org/sqlite` and `/internal/memory` as offenders.

**Revert the import.** Re-run — PASS.

Confirm revert:

```bash
cd gormes
git diff --stat internal/kernel/kernel.go
```

Expected: empty.

- [ ] **Step 4: Sanity-break (TUI)**

Temporarily add to `gormes/cmd/gormes/main.go`'s import block:

```go
_ "github.com/XelHaku/golang-hermes-agent/gormes/internal/memory"
```

Re-run:

```bash
cd gormes
go test ./internal/ -run TestTUIBinaryHasNoSqliteDep -v 2>&1 | tail -10
```

Expected: FAIL, naming the offenders.

**Revert.** Confirm `git diff --stat cmd/gormes/main.go` is empty.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/buildisolation_test.go
git commit -m "$(cat <<'EOF'
test(gormes/internal): forbid memory/sqlite in kernel + TUI deps

Two new build-isolation tests:
  - TestKernelHasNoMemoryDep: internal/kernel must never
    transitively import internal/memory or modernc.org/sqlite.
  - TestTUIBinaryHasNoSqliteDep: cmd/gormes must never carry
    SQLite into its static binary.

Both tests shell out to `go list -deps` and grep for offender
substrings — same pattern as the Phase 2.B.1 Telegram-isolation
test and the Phase 2.C session-isolation test.

Sanity-broken both assertions before committing: adding a blank
session import to each guarded package produced failing output
naming the offenders, then reverted.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Verification sweep

**Files:** no changes — verification only.

- [ ] **Step 1: Full test sweep**

```bash
cd gormes
go test -race ./... -count=1 -timeout 180s
go vet ./...
```

Expected: all packages PASS; vet clean.

- [ ] **Step 2: Build both binaries + size check**

```bash
cd gormes
make build
go build -o bin/gormes-telegram ./cmd/gormes-telegram
ls -lh bin/
```

Expected:
- `bin/gormes` ≤ **10 MB** (target 8.2 MB unchanged)
- `bin/gormes-telegram` ≤ **20 MB** (target ~15-16 MB with modernc)

If either overshoots, STOP and report sizes. Do NOT proceed.

- [ ] **Step 3: Build-isolation grep**

```bash
cd gormes
echo "---TUI (must be OK)---"
(go list -deps ./cmd/gormes | grep -E "modernc|internal/memory") && echo "VIOLATION" || echo "OK"
echo "---Kernel (must be OK)---"
(go list -deps ./internal/kernel | grep -E "modernc|internal/memory") && echo "VIOLATION" || echo "OK"
echo "---Bot (must include memory)---"
go list -deps ./cmd/gormes-telegram | grep -E "modernc|internal/memory" | head -3
```

Expected: first two lines print `OK`; third line prints at least `modernc.org/sqlite` AND `.../internal/memory`.

- [ ] **Step 4: Offline doctor still works**

```bash
cd gormes
./bin/gormes doctor --offline
```

Expected: `[PASS] Toolbox: 3 tools registered (echo, now, rand_int)`.

- [ ] **Step 5: Bot startup validation unchanged**

```bash
cd gormes
./bin/gormes-telegram 2>&1 | head -3
```

Expected: `gormes-telegram: no Telegram bot token — ...` exit 1.

- [ ] **Step 6: memory.db smoke test**

```bash
cd gormes
export XDG_DATA_HOME=/tmp/gormes-3a-v13-$$
GORMES_TELEGRAM_TOKEN=fake:token GORMES_TELEGRAM_CHAT_ID=99 \
  timeout 2 ./bin/gormes-telegram 2>&1 | tail -3 || true
ls -la $XDG_DATA_HOME/gormes/
echo "---perms---"
stat -c "%a %n" $XDG_DATA_HOME/gormes/memory.db 2>&1
rm -rf $XDG_DATA_HOME
```

Expected: `memory.db` (and `-wal` / `-shm` sidecars) exist with mode `0600`. The timeout kills the process cleanly; Close flushes WAL.

- [ ] **Step 7: FTS5 works against a real file**

```bash
cd gormes
export XDG_DATA_HOME=/tmp/gormes-3a-fts5-$$
mkdir -p $XDG_DATA_HOME/gormes
# Use a go-file one-liner to write a row then query via FTS5.
cat > /tmp/fts5_probe.go <<'GO'
package main
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/memory"
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
	"encoding/json"
)
func main() {
	path := filepath.Join(os.Getenv("XDG_DATA_HOME"), "gormes", "memory.db")
	s, err := memory.OpenSqlite(path, 0, nil)
	if err != nil { panic(err) }
	defer s.Close(context.Background())
	payload, _ := json.Marshal(map[string]any{"session_id":"s","content":"the word is asparagus","ts_unix":1})
	s.Exec(context.Background(), store.Command{Kind: store.AppendUserTurn, Payload: payload})
	// Wait for worker to drain.
	for i := 0; i < 100 && s.Stats().Accepted == 0; i++ {
		// spin
	}
	var n int
	s.DB().QueryRow(`SELECT COUNT(*) FROM turns_fts WHERE turns_fts MATCH ?`, "asparagus").Scan(&n)
	fmt.Println("fts5_match_count:", n)
}
GO
go run /tmp/fts5_probe.go
rm -rf $XDG_DATA_HOME /tmp/fts5_probe.go
```

Expected: `fts5_match_count: 1`.

- [ ] **Step 8: Live manual smoke test (optional)**

See spec §16 for the `sqlite3` CLI inspection commands. Skip unless a live Telegram bot is available.

- [ ] **Step 9: No commit**

If any check failed, STOP and report the failing command + output.

---

## Appendix: Self-Review

**Spec coverage:**

| Spec § | Task(s) |
|---|---|
| §1 Goal | All tasks |
| §2 Non-goals | Enforced by scope — no extraction/graph/recall tasks |
| §3 Scope | Tasks 2–12 |
| §4 Engine selection | Task 1 (locked modernc at dep add) |
| §5 Architecture | Tasks 3 (skeleton), 4 (Exec), 5 (worker) |
| §6 Data model / schema | Task 3 (schema.go) |
| §7 Interface & types | Task 2 (store cleanup + RecordingStore), 3 (SqliteStore type) |
| §8 Async worker | Tasks 4 (Exec drop-on-full), 5 (worker run loop) |
| §9 Kernel wiring | Task 8 |
| §10 DI | Task 10 (bot wires SqliteStore), Task 9 (config path) |
| §11 Error handling | Distributed: Task 3 (Open failures), Task 4 (drop-on-full), Task 5 (malformed payloads), Task 7 (shutdown timeout) |
| §12 Security | Task 3 (file modes), Task 5 (parameterized SQL via ? in INSERT), Task 6 (parameterized MATCH in FTS5 tests) |
| §13 Binary budgets | Task 10 (measured), Task 13 (verified) |
| §14 Testing strategy | Every task has tests; build-isolation in Task 12 |
| §15 Verification checklist | Task 13 |
| §16 Manual smoke | Task 13 Step 8 |
| §17 Out of scope | No tasks (correctly) |
| §18 Rollout | Structural — every task is a separate commit |

**Placeholder scan:** no `TBD` / `TODO` / `fill in` / `add appropriate error handling` / `similar to Task N`. Task 8's "if the codebase's layout drifted..." note is guidance, not a placeholder: it tells the implementer exactly what anchor to search for.

**Type consistency:**

- `store.CommandKind` — enum reduced to {`AppendUserTurn`, `FinalizeAssistantTurn`} in Task 2, consistent thereafter.
- `store.RecordingStore`, `store.NewRecording`, `(*RecordingStore).Commands()` — declared Task 2, consumed Task 8. Consistent.
- `memory.SqliteStore` (type), `memory.OpenSqlite(path, queueCap, log)` (constructor), `.Exec`, `.Close(ctx)`, `.Stats()`, `.DB()` — declared Tasks 3/4/5/7/11, consumed Tasks 10/11. Consistent.
- `memory.turnPayload` (worker.go) — JSON shape matches kernel's outbound payload (Task 8).
- `config.TelegramCfg.MemoryQueueCap`, `config.MemoryDBPath()` — declared Task 9, consumed Task 10.
- Schema names: `turns`, `turns_fts`, `schema_meta` — one source of truth in `schema.go` (Task 3).
- Error messages: all wrap with `fmt.Errorf("memory: ..., %w", err)` — consistent across memory.go and worker.go.

**Execution order:** linear dependency chain — each task's tests compile against symbols introduced in an earlier task. Recommended sequence: **T1 → T2 → T3 → T4 → T5 → T6 → T7 → T8 → T9 → T10 → T11 → T12 → T13**.

**Checkpoint suggestions:** halt after T5 (worker actually writing) for a sanity check on the hot-path / off-path separation, and after T8 (kernel wiring) before T10 pulls the trigger in `cmd/gormes-telegram`.

**Scope:** one cohesive Phase-3.A plan — dep, schema, async worker, kernel activation, bot wiring, isolation, verification. No spillover into 3.B (extraction) or 3.C (recall tools). Every task produces a commit that leaves the module green under `go test -race ./...`.
