# Gormes Phase 3.A — Lattice Memory Foundation Design

**Status:** Approved 2026-04-20 · implementation plan pending
**Depends on:** Phase 2.C (bbolt session mapping) green on `main`

## Related Documents

- [`gormes/docs/ARCH_PLAN.md`](../../ARCH_PLAN.md) — executive roadmap. Phase 3 = "The Black Box (Memory) — SQLite + FTS5 + ontological graph". This spec delivers the SQLite + FTS5 foundation (the "concrete slab"); entity/relationship extraction is deferred to Phase 3.B.
- Phase 2.C — [`2026-04-19-gormes-phase2c-persistence-design.md`](2026-04-19-gormes-phase2c-persistence-design.md) — bbolt session mapping. **Untouched by this spec.** `internal/session` stays exactly as-is; the new `internal/memory` package is a sibling.
- Phase 2.B.1 — [`2026-04-19-gormes-phase2b-telegram.md`](2026-04-19-gormes-phase2b-telegram.md) — Telegram adapter + `cmd/gormes-telegram`. This spec extends the bot's startup wiring; the TUI is not touched.

---

## 1. Goal

`bin/gormes-telegram` persists every conversational turn to a local SQLite database with full-text search (FTS5) enabled — the foundation for future entity extraction (3.B) and recall tools (3.C). The kernel's 250 ms `StoreAckDeadline` is structurally preserved: every database operation happens on a background worker goroutine, never on the turn-loop.

## 2. Non-Goals

- **No entity or relationship extraction.** The `entities` / `relationships` tables, LLM-assisted extraction pipelines, and graph-traversal APIs are Phase 3.B.
- **No recall tools.** `search_past_conversations`, `recall_entity`, pre-turn context injection — all Phase 3.C.
- **No TUI persistence.** `cmd/gormes` keeps `NoopStore`. The 8.2 MB binary stays 8.2 MB. Adding SQLite to the TUI would bust the "sub-10 MB" promise on the project's public landing for zero user benefit (the TUI is a single-user local viewer; memory aspirations live in the bot where turns accumulate over weeks).
- **No Python-side changes.** Python's `SessionDB` remains the canonical transcript for the LLM pipeline. Gormes's SQLite store is a *local mirror* for Go-side search and future memory features. Two writers to the same logical data is fine because they write to *different* files — no split-brain.
- **No dependency-graph bloat of the kernel.** `internal/memory` must not be importable from `internal/kernel`. A new build-isolation test extends T12.

## 3. Scope

Exactly one new package (`internal/memory`) with a SQLite-backed implementation of the existing `store.Store` interface, plus a small async worker that serializes writes off the kernel's hot path, plus the kernel wiring to activate the reserved `FinalizeAssistantTurn` command kind. Plus the `cmd/gormes-telegram` injection point.

## 4. Engine Selection (measured)

Both candidates were built with `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"` on 2026-04-20. Each program imported the driver, created an FTS5 virtual table, inserted one row, and ran one `MATCH` query.

| Engine | Binary size | Notes |
|---|---:|---|
| `modernc.org/sqlite` (ccgo transpiled C-to-Go) | **5.9 MB** | Winner |
| `github.com/ncruces/go-sqlite3` (`wasm2go` translated WASM-to-Go) | 10.0 MB | ~70 % larger |

Both are functionally equivalent for our workload (turns + FTS5). Both support `CGO_ENABLED=0`. `modernc.org/sqlite` wins on binary size by ~4 MB, preserving Phase-3.B/C headroom.

**Engine locked: `modernc.org/sqlite` @ latest stable 1.x.**

## 5. Architecture at a Glance

```
                                   ┌────────────────────────────────────┐
                                   │   $XDG_DATA_HOME/gormes/memory.db  │
                                   │              (SQLite)              │
                                   └──────────────────┬─────────────────┘
                                                      │ db.Exec/Query
            ┌──────────────────────┐          ┌───────▼───────┐
            │  kernel.runTurn      │          │  worker       │
            │  (turn loop)         │  send→   │  goroutine    │
            │                      │  Put     │  (single      │
            │  store.Exec(cmd) →───┼──────────▶  owner of     │
            │  returns Ack in <1ms │  drop    │   *sql.DB)    │
            └──────────────────────┘  on-full └───────────────┘
                                                      │
                                                      │ on ctx.Done():
                                                      │   drain queue,
                                                      │   call db.Close()
                                                      ▼
                                              [graceful shutdown]
```

**Single-owner invariant preserved:** the kernel owns its turn state; the worker owns the `*sql.DB`. They communicate only through a bounded `chan Command`. Nothing mutable is shared.

## 6. Data Model / Schema

### 6.1 Tables

```sql
-- Baseline schema version tracking. Exactly one row.
CREATE TABLE IF NOT EXISTS schema_meta (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL
);
INSERT OR IGNORE INTO schema_meta(k, v) VALUES ('version', '3a');

-- Every turn: one row per user or assistant message.
CREATE TABLE IF NOT EXISTS turns (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id  TEXT    NOT NULL,
  role        TEXT    NOT NULL CHECK(role IN ('user','assistant')),
  content     TEXT    NOT NULL,
  ts_unix     INTEGER NOT NULL,
  meta_json   TEXT             -- optional; reserved for 3.B (extraction state)
);

CREATE INDEX IF NOT EXISTS idx_turns_session_ts
  ON turns(session_id, ts_unix);
```

### 6.2 FTS5 virtual table (auto-synced via triggers)

```sql
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
```

**Why content-backed FTS5:** SQLite maintains the FTS5 index inside the same transaction as the base-table INSERT. We get search correctness without hand-written dual-writes. Index maintenance cost is paid on the worker goroutine, not the kernel.

### 6.3 SQLite PRAGMAs (set at Open)

```sql
PRAGMA journal_mode = WAL;     -- durable + concurrent readers
PRAGMA synchronous = NORMAL;   -- acceptable durability for a local mirror
PRAGMA busy_timeout = 2000;    -- 2 s — the worker is the only writer
PRAGMA foreign_keys = ON;
```

### 6.4 File location

`$XDG_DATA_HOME/gormes/memory.db` (same `gormes/` directory as `sessions.db`).

- Mode `0600` on creation.
- Auto-created parent dir `gormes/` with mode `0700` (same as Phase 2.C).
- WAL and SHM sidecar files live alongside; cleaned up by SQLite on close.

## 7. Interface & Types

### 7.1 `internal/store` — unchanged API surface, augmented enum

The existing `store.Store` interface is already designed for this — commands flow in via `Exec`, Ack flows out. Phase 3.A keeps the interface shape and:

- **Keeps** `AppendUserTurn` and `FinalizeAssistantTurn` command kinds.
- **Deletes** the unused `AppendAssistantDraft` command kind (declared in Phase 1, never called — YAGNI).
- Adds nothing else to `Store`. Reads come later in Phase 3.C via a new sibling interface.

```go
// gormes/internal/store/store.go (edited)

type CommandKind int
const (
    AppendUserTurn CommandKind = iota
    FinalizeAssistantTurn
)
```

Breaking change: any test or caller that referenced `AppendAssistantDraft` must be updated. Search results confirm zero current callers, so it's pure cleanup.

### 7.2 `internal/memory` — new package

```go
package memory

import (
    "context"
    "database/sql"

    _ "modernc.org/sqlite"
    "github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
)

// SqliteStore is a fire-and-forget store.Store implementation backed by
// SQLite. Exec returns an Ack in <1 ms by enqueueing the command on an
// internal bounded channel; a background worker goroutine performs the
// actual SQL writes. On channel full, the write is DROPPED and logged
// as a WARN — the kernel's 250 ms deadline is never at risk.
type SqliteStore struct {
    db      *sql.DB
    queue   chan store.Command
    done    chan struct{}
    drops   atomic.Uint64
    log     *slog.Logger
}

// OpenSqlite opens/creates path and starts the worker goroutine.
// queueCap defaults to 1024 when zero.
func OpenSqlite(path string, queueCap int, log *slog.Logger) (*SqliteStore, error)

// Exec implements store.Store.
// Fast path: one channel send, then immediate Ack{TurnID: 0}. Returns
// an error ONLY on a nil receiver or a closed store. Drops are silent
// to the caller (logged, counted internally).
func (s *SqliteStore) Exec(ctx context.Context, cmd store.Command) (store.Ack, error)

// Close signals the worker to drain, waits up to ctx.Deadline for drain,
// flushes WAL, and closes the DB. Idempotent.
func (s *SqliteStore) Close(ctx context.Context) error

// Stats returns counters useful for tests and future metrics. Not part
// of store.Store — only consumers who hold a *SqliteStore can call it.
func (s *SqliteStore) Stats() Stats

type Stats struct {
    QueueLen    int
    QueueCap    int
    Drops       uint64
    Accepted    uint64
}
```

### 7.3 Command payload schema (internal contract)

Both `AppendUserTurn` and `FinalizeAssistantTurn` carry a JSON payload:

```json
{
  "session_id": "sess-2026-04-20_abc123",
  "content":    "the user's raw message or the assistant's final reply",
  "ts_unix":    1745100300
}
```

Built by the kernel in `runTurn`. The worker unmarshals and dispatches to one of two SQL statements:

- `AppendUserTurn`   → `INSERT INTO turns(session_id, role, content, ts_unix) VALUES(?, 'user', ?, ?)`
- `FinalizeAssistantTurn` → `INSERT INTO turns(session_id, role, content, ts_unix) VALUES(?, 'assistant', ?, ?)`

Both use `?`-bound parameters — no string concatenation into SQL — standard SQL-injection prevention.

## 8. Async Worker Model

### 8.1 Queue & drop semantics

```go
select {
case s.queue <- cmd:
    // enqueued
default:
    s.drops.Add(1)
    s.log.Warn("memory.SqliteStore: queue full, dropping command",
        "kind", cmd.Kind.String(),
        "queue_cap", cap(s.queue),
        "drops_total", s.drops.Load())
}
return store.Ack{}, nil
```

**Zero-Leak justification:** a dropped turn is a partial-history degradation — the user's conversation still succeeds against Python (Python's `SessionDB` is canonical). A blocked kernel is a service outage. We choose the former every time.

### 8.2 Worker loop

```go
func (s *SqliteStore) run() {
    defer close(s.done)
    for cmd := range s.queue {
        s.handleCommand(cmd)
    }
}
```

Single-owner invariant: **only this goroutine holds `*sql.DB`**. No sharing. `database/sql` itself is already goroutine-safe, but restricting to one writer simplifies transaction semantics and makes the SQLite busy-timeout practically unreachable.

### 8.3 Graceful shutdown

```go
func (s *SqliteStore) Close(ctx context.Context) error {
    s.closeOnce.Do(func() {
        close(s.queue)          // signal worker to drain + exit
    })
    select {
    case <-s.done:
        // worker drained cleanly
    case <-ctx.Done():
        s.log.Warn("memory.SqliteStore: shutdown deadline exceeded; DB may lose in-flight writes")
    }
    return s.db.Close()          // flush WAL, release file lock
}
```

Callers pass a budgeted context (`kernel.ShutdownBudget` = 2 s minus whatever the rest of shutdown has already consumed). In-flight SQL writes are ~single-digit ms each, so draining 1024 queued commands on a disk-of-average-speed takes well under the budget.

### 8.4 Queue sizing

- Default cap: **1024**. A typical user turn is one `AppendUserTurn` + one `FinalizeAssistantTurn` (two commands). Even at extreme burst (multiple rapid `/new` + fresh turns), the worker drains at ~1000 cmds/s on a modest SSD; 1024 of buffer gives >1 s of slack before drops begin.
- Configurable via `[telegram].memory_queue_cap` (TOML) with sensible default. Not env-overridable — a production config decision, not a secret.

## 9. Kernel Wiring

### 9.1 Activate `FinalizeAssistantTurn`

Current code (`kernel.go:183`) calls `store.Exec` only for `AppendUserTurn`, before streaming. Phase 3.A adds a second call **after** the assistant stream is committed to history:

```go
// Inside runTurn, AFTER the tool loop completes and the final assistant
// message has been appended to k.history. Pseudo-location: after the
// existing `k.history = append(k.history, hermes.Message{Role: "assistant", Content: k.draft})`
// near kernel.go:387 (subject to line-number drift).
{
    payload, _ := json.Marshal(map[string]any{
        "session_id": k.sessionID,
        "content":    k.draft,
        "ts_unix":    time.Now().Unix(),
    })
    storeCtx, storeCancel := context.WithTimeout(ctx, StoreAckDeadline)
    _, _ = k.store.Exec(storeCtx, store.Command{
        Kind:    store.FinalizeAssistantTurn,
        Payload: payload,
    })
    storeCancel()
}
```

The Ack is discarded (`_, _`) because Phase 3.A's `SqliteStore.Exec` never blocks and never errors on a live store. We still wrap in a 250 ms context for contract safety (if someone swaps in a synchronous store later, it gracefully times out).

### 9.2 Payload schema drift

The existing `AppendUserTurn` payload is `{"text": "..."}` (hardcoded in `kernel.go:182`). Phase 3.A tightens this to match §7.3 — `{"session_id", "content", "ts_unix"}` — so both commands share one schema. One-line diff in `kernel.go`.

**Backwards-compat note:** the shipping `NoopStore` discards the payload entirely, so changing the payload shape breaks nothing today. The kernel tests that exercise the store path pass `NoopStore` and never inspect payload content.

### 9.3 Keep `StoreAckDeadline = 250ms` unchanged

It remains the *safety contract* — any store impl that introduces synchronous latency beyond 250 ms trips `PhaseFailed`. `SqliteStore.Exec` cannot trip it (returns in microseconds), but `SlowStore` still can (existing test pins this). The constant is not semantically repurposed.

## 10. Dependency Injection

### 10.1 `cmd/gormes` (TUI) — unchanged

```go
// stays exactly as today
k := kernel.New(kernel.Config{...}, c, store.NewNoop(), tm, log)
```

No SQLite import. Binary stays 8.2 MB.

### 10.2 `cmd/gormes-telegram` (bot) — new wiring

```go
import "github.com/XelHaku/golang-hermes-agent/gormes/internal/memory"

memPath := filepath.Join(xdgDataHome(), "gormes", "memory.db")
sstore, err := memory.OpenSqlite(memPath, cfg.Telegram.MemoryQueueCap, slog.Default())
if err != nil {
    return fmt.Errorf("memory store: %w", err)
}
defer func() {
    shutdownCtx, cancel := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
    defer cancel()
    _ = sstore.Close(shutdownCtx)
}()

k := kernel.New(kernel.Config{...}, hc, sstore, tm, slog.Default())
```

An exported helper `config.MemoryDBPath()` is added (same shape as Phase 2.C's `SessionDBPath()`) so both binaries agree on the path without duplicating the XDG logic.

### 10.3 New config field

```go
// internal/config/config.go
type TelegramCfg struct {
    // ... existing fields ...
    MemoryQueueCap int `toml:"memory_queue_cap"` // default 1024 when zero
}
```

Defaulting logic in `defaults()`:

```go
Telegram: TelegramCfg{
    CoalesceMs:        1000,
    FirstRunDiscovery: true,
    MemoryQueueCap:    1024,
},
```

No env-var override. No CLI flag. Config-file or default only — queue sizing is operator knowledge, not a secret.

## 11. Error Handling

### 11.1 Open failures

`memory.OpenSqlite` failure modes:

| Condition | Behavior |
|---|---|
| Parent dir not writable | Return wrapped `fmt.Errorf("memory: ...")` → bot exits 1 |
| SQLite magic invalid (corrupt file) | Return wrapped error that includes `"corrupt"` substring → bot exits 1 with clear message |
| Schema migration fails | Return wrapped error → exit 1 |
| Out of disk at Open | Return wrapped error → exit 1 |

No sentinel-error export in 3.A — the bot simply exits on any Open failure. (Unlike `internal/session.ErrDBLocked`, SQLite does not *require* exclusive file locks in WAL mode; multiple processes can open the same DB file. If we later need to distinguish, we add sentinels in 3.B.)

### 11.2 Write failures (on the worker)

The worker's INSERT can fail transiently (disk full mid-run, readonly filesystem). Handling:

```go
if _, err := tx.Exec(sqlStmt, args...); err != nil {
    s.log.Warn("memory.SqliteStore: write failed",
        "kind", cmd.Kind.String(),
        "err", err)
    return // drop the command; next iteration
}
```

No retry loop. The command is lost (same impact class as a drop-on-full). Surfacing the error to the kernel would require a reverse channel that contradicts the fire-and-forget design.

### 11.3 Drop-on-full

Logged at `WARN` with a counter. An operator grepping logs for `"queue full"` gets the signal. Not escalated to the kernel.

### 11.4 Shutdown timeout

Logged at `WARN`. `db.Close()` still runs — SQLite's WAL checkpoint flushes on close. The durability window that's lost is bounded by `PRAGMA synchronous=NORMAL`'s semantics (last few ms of writes may be discarded). Acceptable for a local mirror.

## 12. Security

- **File modes:** `0600` on `memory.db`, `0600` on the `-wal` and `-shm` sidecars (SQLite creates them with the main file's mode). Parent dir `0700`.
- **Parameterized SQL only.** Every `Exec`/`Query` uses `?` placeholders. Never `fmt.Sprintf` into SQL. Repeat for `turns_fts`: FTS5 `MATCH` queries also parameterize the pattern — `WHERE content MATCH ?`.
- **No FTS5 untokenized auxiliary functions.** We do not expose `snippet()` / `highlight()` yet; those come in Phase 3.C alongside the recall tool.
- **The memory.db may leak private conversation content on disk.** Out of scope for Phase 3.A: at-rest encryption. Documented as a known limitation in the manifesto/why-gormes page (deferred; tracking under Phase 3.C's threat model).

## 13. Binary Budgets

Measured baseline (post-Phase 2.C, commit `bf61992d`):

| Binary | Size | Post-3.A projection | Budget |
|---|---:|---:|---:|
| `bin/gormes` | 8.2 MB | 8.2 MB (unchanged — NoopStore) | ≤ 10 MB ✓ |
| `bin/gormes-telegram` | 10.0 MB | **~15-16 MB** (+5.9 MB SQLite, minus ~0.4 MB dead-code elimination gains from dropping AppendAssistantDraft plumbing) | ≤ 20 MB ✓ |

T13 verification will measure the actual post-landing sizes. If `bin/gormes-telegram` exceeds 17 MB, we investigate — modernc stripping should deliver ~5.9 MB consistently.

## 14. Testing Strategy

### 14.1 Unit (no disk)

- `TestSqliteStore_ExecReturnsFast` — Open `:memory:` DB; measure Exec returns in <1 ms under race.
- `TestSqliteStore_DropsOnFullQueue` — Open with `queueCap=2`, fire a slow worker barrier (or direct queue inspection), send 10 commands, assert `Stats().Drops >= 8`.
- `TestSqliteStore_AcceptedCounterTracksWrites` — send N cmds, wait for drain, assert `Accepted == N` and `SELECT COUNT(*) FROM turns == N`.

### 14.2 FTS5 query tests

- `TestSqliteStore_FTS5MatchBasic` — insert 3 turns, `SELECT rowid FROM turns_fts WHERE turns_fts MATCH 'asparagus'` returns exactly the rows containing that word.
- `TestSqliteStore_FTS5MatchPhrase` — phrase query `"gormes telegram"` returns only turns with that bigram.
- `TestSqliteStore_FTS5UpdatesOnDelete` — insert, then `DELETE FROM turns`, assert FTS5 returns zero results.

### 14.3 Worker lifecycle

- `TestSqliteStore_CloseDrainsQueue` — enqueue 100 commands, Close with 5 s context, assert all 100 present in `turns`.
- `TestSqliteStore_CloseIdempotent` — double Close returns nil on second call.
- (A deadline-overrun test is not included: synthetically stalling the worker is either invasive — requires test-only hooks inside production code — or flaky. The fast-return test in §14.1 and the drain test above provide sufficient coverage of the happy + clean-shutdown paths. The deadline's protective role is exercised in the shutdown-budget test in `kernel_test.go:348` via `SlowStore`, which remains in place.)

### 14.4 Kernel integration

- `TestKernel_FinalizeAssistantTurnReachesStore` — introduce a new `store.RecordingStore` test double in `internal/store/` (alongside `NoopStore` + `SlowStore`) that captures every Command into a `[]Command` slice with a mutex. Submit a turn, assert both `AppendUserTurn` and `FinalizeAssistantTurn` commands are observed, with matching session_id and content after JSON unmarshal.
- Existing `kernel_test.go` tests that use `NoopStore` must continue passing — the schema change to the payload is cosmetic because `NoopStore` discards.
- Existing `SlowStore` test must continue tripping `StoreAckDeadline`.

### 14.5 Build isolation

- Extend `internal/buildisolation_test.go` with `TestKernelHasNoMemoryDep`: `go list -deps ./internal/kernel` must not contain `modernc.org/sqlite` or `/internal/memory`.
- Sanity-break verified (blank import in kernel → test fails loudly with offender list, then revert).
- Also: `TestTUIBinaryHasNoSqliteDep` — the TUI must not transitively import `modernc.org/sqlite`.

### 14.6 End-to-end

- `TestBot_TurnPersistsToSqlite` — scripted `hermes.MockClient`, real `OpenSqlite(t.TempDir())`, run one full turn via the bot's handleUpdate → kernel → worker, wait for `Stats().Accepted == 2`, assert `SELECT role, content FROM turns ORDER BY id` returns the expected user + assistant pair.

### 14.7 Full sweep

`go test -race ./... -count=1 -timeout 120s` green; `go vet ./...` clean.

## 15. Verification Checklist

- [ ] `go build` produces static binaries (`CGO_ENABLED=0`).
- [ ] `bin/gormes` ≤ 10 MB (unchanged).
- [ ] `bin/gormes-telegram` ≤ 20 MB (projected ~15-16 MB).
- [ ] `go list -deps ./internal/kernel` — no `modernc.org/sqlite`, no `/internal/memory`.
- [ ] `go list -deps ./cmd/gormes` — no `modernc.org/sqlite`, no `/internal/memory`.
- [ ] FTS5 `MATCH` returns inserted rows.
- [ ] Drop counter increments when queue is overfilled.
- [ ] `SqliteStore.Close(ctx)` drains within budget; writes survive restart.
- [ ] All tests in §14 green under `-race`.
- [ ] `go vet ./...` clean.

## 16. Manual Smoke Test (Phase 3.A close-out)

Requires Telegram bot token + running Python `api_server`.

```bash
# Terminal 1
API_SERVER_ENABLED=true hermes gateway start

# Terminal 2
export GORMES_TELEGRAM_TOKEN=<your:token>
export GORMES_TELEGRAM_CHAT_ID=<your:chat:id>
rm -f ~/.local/share/gormes/memory.db   # start clean
./bin/gormes-telegram &

# From Telegram, DM the bot a few messages.
#   "what is the capital of spain"
#   "and the capital of portugal"

kill %1

# Inspect with any SQLite CLI (or a Go debug script):
sqlite3 ~/.local/share/gormes/memory.db \
  "SELECT role, substr(content,1,40) FROM turns ORDER BY id"

# Expected: alternating user/assistant rows with the exact text.

sqlite3 ~/.local/share/gormes/memory.db \
  "SELECT rowid, substr(content,1,40) FROM turns_fts WHERE turns_fts MATCH 'capital'"

# Expected: rows for both user queries plus both assistant replies.
```

## 17. Out of Scope (explicit deferrals)

- **Entity / relationship extraction** — Phase 3.B. Will add `entities` + `relationships` tables and an LLM-assisted background extractor reading from `turns`.
- **Recall tools** — Phase 3.C. Exposes `search_past_conversations(query)`, `recall_entity(name)`, possibly pre-turn context injection.
- **At-rest encryption** — tracked under Phase 3.C threat model.
- **TUI memory** — no date; we'll revisit if there's user demand.
- **Cross-platform memory merging** — memory.db is per-host. Syncing across devices is a hypothetical Phase 4+ concern.
- **Telemetry on `Stats()`** — not plumbed to `internal/telemetry` in 3.A; available for tests and future wiring.

## 18. Rollout

- Ships as one PR, one commit series under `subagent-driven-development`.
- First boot on existing installs: no `memory.db` file → `OpenSqlite` creates it, schema migration runs once (ms-scale), binary starts serving. **Zero-regression launch.**
- No migration script required. Phase 3.B will add columns/tables via idempotent `CREATE TABLE IF NOT EXISTS` and bump `schema_meta.v` to `3b`.
- If a user wants to reset memory, `rm ~/.local/share/gormes/memory.db` is safe — it's a local mirror; Python's `SessionDB` is untouched.
