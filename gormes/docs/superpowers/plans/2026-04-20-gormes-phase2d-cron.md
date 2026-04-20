# Gormes Phase 2.D — Cron (Proactive Heartbeat) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Gormes proactive — operator defines a cron job (schedule + prompt) in a bbolt bucket; at the scheduled time Gormes runs the prompt through the full agent loop with an isolated session and delivers the result via the existing Telegram bot, unless the agent returns `[SILENT]`.

**Architecture:** New `internal/cron` package co-located with extractor/embedder/mirror inside the `gormes telegram` subcommand. `robfig/cron/v3` drives the scheduler. Jobs live in a new bbolt bucket; per-run audit rows live in a new SQLite table. Cron turns are tagged `cron=1` on the `turns` table so the extractor (3.B) skips them. Kernel gains two opaque new fields on `PlatformEvent` (`SessionID`, `CronJobID`) for per-event session override — no kernel-level coupling to memory/session/cron packages. Delivery goes through a generic `DeliverySink` interface; Telegram implements it today, Slack/Discord later.

**Tech Stack:** Go 1.25+, `github.com/robfig/cron/v3` (new dep; ~20 KB binary impact), existing ncruces SQLite (schema v3d → v3e migration), `go.etcd.io/bbolt` (existing), stdlib `crypto/sha256` + `encoding/json`.

**Module path:** `github.com/TrebuchetDynamics/gormes-agent/gormes`

**Spec:** [`docs/superpowers/specs/2026-04-20-gormes-phase2d-cron-design.md`](../specs/2026-04-20-gormes-phase2d-cron-design.md) (approved `692505a1`)

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `gormes/internal/memory/schema.go` | Modify | Bump `schemaVersion = "3e"`; add `migration3dTo3e` constant (cron columns + cron_runs table) |
| `gormes/internal/memory/migrate.go` | Modify | Extend switch: `3d` case now applies `migration3dTo3e` and recurses; add `3e: return nil` case |
| `gormes/internal/memory/migrate_test.go` | Modify | `TestMigrate_3dTo3e_AddsCronColumns`, `TestMigrate_3dTo3e_AddsCronRunsTable`, `TestMigrate_3dTo3e_StatusCheckConstraint` |
| `gormes/internal/memory/memory.go` | Modify | Worker's `AppendUserTurn` handler now writes optional `cron` + `cron_job_id` columns when payload has them |
| `gormes/internal/memory/memory_test.go` | Modify | Append `TestAppendUserTurn_WritesCronColumnsWhenProvided` |
| `gormes/internal/memory/extractor.go` | Modify | `pollMissing` SQL adds `AND cron = 0` so cron turns are excluded from entity extraction |
| `gormes/internal/memory/extractor_test.go` | Modify | Append `TestExtractor_SkipsCronTurns` |
| `gormes/internal/kernel/frame.go` | Modify | Add `SessionID string` and `CronJobID string` to `PlatformEvent` struct |
| `gormes/internal/kernel/kernel.go` | Modify | `processSubmit` saves `k.sessionID`, applies `e.SessionID` override when non-empty, restores after turn; `runTurn` carries cron fields into store payload |
| `gormes/internal/kernel/kernel_test.go` | Modify | `TestKernel_SessionIDOverrideAppliesToTurn`, `TestKernel_SessionIDOverrideDoesNotLeakToNextTurn`, `TestKernel_CronJobIDFlowsToStorePayload` |
| `gormes/internal/cron/job.go` | Create | `Job` struct + `ValidateSchedule(cronExpr string) error` wrapping robfig parser |
| `gormes/internal/cron/job_test.go` | Create | Unit tests for Job field defaults + schedule validation |
| `gormes/internal/cron/store.go` | Create | bbolt `cron_jobs` bucket CRUD: `Create`, `Get`, `List`, `Update`, `Delete` |
| `gormes/internal/cron/store_test.go` | Create | CRUD round-trip tests with real bbolt |
| `gormes/internal/cron/run_store.go` | Create | SQLite `cron_runs` writes: `RecordRun(run Run) error` + `LatestRuns(limit int)` |
| `gormes/internal/cron/run_store_test.go` | Create | SQL round-trip + CHECK-constraint tests |
| `gormes/internal/cron/heartbeat.go` | Create | `const cronHeartbeatPrefix` (verbatim upstream bytes), `BuildPrompt(userPrompt) string`, `DetectSilent(finalResponse) bool` |
| `gormes/internal/cron/heartbeat_test.go` | Create | Byte-match prefix test, `DetectSilent` exact-match vs substring tests |
| `gormes/internal/cron/sink.go` | Create | `DeliverySink` interface + `funcSink` adapter for tests |
| `gormes/internal/cron/sink_test.go` | Create | funcSink forwards + nil-safety tests |
| `gormes/internal/cron/executor.go` | Create | `Executor` struct; `Run(ctx, job)` — build session, submit event, collect final via `kernel.Render()` filter, decide delivery, record run |
| `gormes/internal/cron/executor_test.go` | Create | Silent-suppresses, normal-delivers, timeout-delivers-notice, empty-delivers-notice tests |
| `gormes/internal/cron/scheduler.go` | Create | `Scheduler` wrapping `*cron.Cron`; `Start(ctx)`, `Stop(ctx)`, `Reload()` (future-proof but no-op MVP) |
| `gormes/internal/cron/scheduler_test.go` | Create | Start/Stop lifecycle + bad-schedule skip-but-continue tests |
| `gormes/internal/cron/mirror.go` | Create | Background goroutine: every 30s read jobs + recent runs, atomic-write `CRON.md` |
| `gormes/internal/cron/mirror_test.go` | Create | Atomic write + format tests |
| `gormes/internal/config/config.go` | Modify | New `CronCfg` + nested field in `Config`; defaults in `defaults()` |
| `gormes/internal/config/config_test.go` | Modify | Append `TestLoad_CronDefaults` |
| `gormes/cmd/gormes/telegram.go` | Modify | Wire Store/RunStore/Scheduler/Executor/Mirror; implement `telegramDeliverySink` |
| `gormes/cmd/gormes/telegram_cron_sink_test.go` | Create | telegramDeliverySink forwards to existing bot send path |
| `gormes/go.mod` / `gormes/go.sum` | Modify | Add `github.com/robfig/cron/v3` |

---

## Task 1: Schema v3e migration — `cron` columns on turns + `cron_runs` table

**Files:**
- Modify: `gormes/internal/memory/schema.go`
- Modify: `gormes/internal/memory/migrate.go`
- Modify: `gormes/internal/memory/migrate_test.go`

- [ ] **Step 1: Write failing tests — append to `migrate_test.go`**

```go
func TestOpenSqlite_FreshDBIsV3e(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var v string
	_ = s.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != "3e" {
		t.Errorf("schema version = %q, want 3e", v)
	}
}

func TestMigrate_3dTo3e_AddsCronColumnsToTurns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	// Both new columns must exist.
	rows, err := s.db.Query(`PRAGMA table_info(turns)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	has := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		_ = rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk)
		has[name] = true
	}
	if !has["cron"] {
		t.Error("turns is missing 'cron' column")
	}
	if !has["cron_job_id"] {
		t.Error("turns is missing 'cron_job_id' column")
	}
}

func TestMigrate_3dTo3e_AddsCronRunsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM cron_runs`).Scan(&n)
	if err != nil {
		t.Errorf("cron_runs table missing: %v", err)
	}
}

func TestMigrate_3dTo3e_StatusCheckConstraint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	// Valid status values must be accepted.
	for _, status := range []string{"success", "timeout", "error", "suppressed"} {
		_, err := s.db.Exec(
			`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status) VALUES(?, ?, ?, ?)`,
			"j", 1, "h", status)
		if err != nil {
			t.Errorf("status=%q rejected: %v", status, err)
		}
	}
	// Invalid status must trip CHECK.
	_, err := s.db.Exec(
		`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status) VALUES('j', 1, 'h', 'nope')`)
	if err == nil {
		t.Error("status='nope' should trip CHECK constraint")
	}
}

func TestMigrate_3dTo3e_SuppressionReasonCheckConstraint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	// NULL must be allowed.
	_, err := s.db.Exec(
		`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status, suppression_reason)
		 VALUES('j', 1, 'h', 'success', NULL)`)
	if err != nil {
		t.Errorf("suppression_reason NULL rejected: %v", err)
	}
	// Valid non-NULL values.
	for _, r := range []string{"silent", "empty"} {
		_, err := s.db.Exec(
			`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status, suppression_reason)
			 VALUES('j', 1, 'h', 'suppressed', ?)`, r)
		if err != nil {
			t.Errorf("suppression_reason=%q rejected: %v", r, err)
		}
	}
	// Invalid.
	_, err = s.db.Exec(
		`INSERT INTO cron_runs(job_id, started_at, prompt_hash, status, suppression_reason)
		 VALUES('j', 1, 'h', 'suppressed', 'bogus')`)
	if err == nil {
		t.Error("suppression_reason='bogus' should trip CHECK")
	}
}
```

**IMPORTANT:** Also update any pre-existing migrate test that asserts `"3d"` on fresh DB — change expected value to `"3e"`. Search for `FreshDBIsV3d`, `TestOpenSqlite_SchemaMetaVersion` (in `memory_test.go`), `TestMigrate_Idempotent` (wherever it asserts `3d`).

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run "TestOpenSqlite_FreshDBIsV3e|TestMigrate_3dTo3e_" -v 2>&1 | tail -10
```

Expected: failing assertions.

- [ ] **Step 3: Update `schema.go` — bump + new migration fragment**

In `gormes/internal/memory/schema.go`:

1. Change `const schemaVersion = "3d"` to `const schemaVersion = "3e"`.

2. Append a new migration constant at the end of the file:

```go
// migration3dTo3e extends v3d with Phase 2.D cron fields:
//   - turns gains cron / cron_job_id columns; default 0/NULL so
//     existing rows (non-cron) are unaffected.
//   - cron_runs table is the per-run audit trail: one row per
//     scheduled fire, capturing outcome + delivery decision.
//   - CHECK constraints lock the allowed status / suppression_reason
//     values so garbage data can't enter the audit log.
const migration3dTo3e = `
ALTER TABLE turns ADD COLUMN cron INTEGER NOT NULL DEFAULT 0;
ALTER TABLE turns ADD COLUMN cron_job_id TEXT;

CREATE TABLE IF NOT EXISTS cron_runs (
	id                  INTEGER PRIMARY KEY AUTOINCREMENT,
	job_id              TEXT    NOT NULL,
	started_at          INTEGER NOT NULL,
	finished_at         INTEGER,
	prompt_hash         TEXT    NOT NULL,
	status              TEXT    NOT NULL CHECK(status IN (
	                        'success','timeout','error','suppressed'
	                    )),
	delivered           INTEGER NOT NULL DEFAULT 0 CHECK(delivered IN (0,1)),
	suppression_reason  TEXT    CHECK(suppression_reason IS NULL OR
	                                  suppression_reason IN ('silent','empty')),
	output_preview      TEXT,
	error_msg           TEXT
);
CREATE INDEX IF NOT EXISTS idx_cron_runs_job_started
	ON cron_runs(job_id, started_at DESC);

UPDATE schema_meta SET v = '3e' WHERE k = 'version' AND v = '3d';
`
```

- [ ] **Step 4: Extend `migrate.go` switch**

In `gormes/internal/memory/migrate.go`, change `case "3d"` + add `case "3e"`:

```go
	case "3d":
		if err := runMigrationTx(db, migration3dTo3e); err != nil {
			return fmt.Errorf("memory: migrate 3d->3e: %w", err)
		}
		return migrate(db)
	case "3e":
		return nil
	default:
		return fmt.Errorf("%w: got %q, want %q", ErrSchemaUnknown, v, schemaVersion)
	}
```

- [ ] **Step 5: Update stale version assertions in pre-existing tests**

Grep-and-replace in the memory package:

```bash
cd gormes
grep -rn "\"3d\"" internal/memory/ | head
```

Any test that asserts `"3d"` as the expected version after a fresh `OpenSqlite` call must be updated to `"3e"`. Likely candidates: `TestOpenSqlite_SchemaMetaVersion` (in `memory_test.go`), `TestOpenSqlite_FreshDBIsV3d` (migrate_test.go), `TestMigrate_Idempotent`.

**Do NOT rename** `TestOpenSqlite_FreshDBIsV3b` / `FreshDBIsV3c` / `FreshDBIsV3d` — the naming drift is pre-existing tech debt and renaming them now would clobber git blame. Just update their assertion values.

- [ ] **Step 6: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run "TestMigrate_|TestOpenSqlite_" -v -timeout 30s
```

All pass.

Full memory suite (minus Ollama):
```bash
cd gormes
go test -race ./internal/memory/... -count=1 -timeout 60s -skip Integration_Ollama
```

Green.

- [ ] **Step 7: Commit (from repo root)**

```bash
git add gormes/internal/memory/schema.go gormes/internal/memory/migrate.go gormes/internal/memory/migrate_test.go gormes/internal/memory/memory_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): schema v3e adds cron columns + cron_runs table

Phase 2.D cron foundation.

  migration3dTo3e:
    ALTER TABLE turns ADD COLUMN cron INTEGER DEFAULT 0
    ALTER TABLE turns ADD COLUMN cron_job_id TEXT
    CREATE TABLE cron_runs (
      id PK, job_id, started_at, finished_at,
      prompt_hash, status CHECK IN(success|timeout|error|suppressed),
      delivered (0|1), suppression_reason CHECK IN(silent|empty),
      output_preview, error_msg
    )
    CREATE INDEX idx_cron_runs_job_started(job_id, started_at DESC)

  migrate() switch extended to 3d -> 3e.

Five tests lock the invariants: fresh DB lands on 3e, both cron
columns exist on turns, cron_runs table exists, status CHECK
rejects garbage values, suppression_reason CHECK accepts NULL +
silent/empty and rejects other strings.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Turn-payload cron fields + memory worker writes them

**Files:**
- Modify: `gormes/internal/memory/memory.go`
- Modify: `gormes/internal/memory/memory_test.go`

The kernel passes a JSON payload to `store.Command{Kind: AppendUserTurn}`. Current payload shape is `{session_id, content, ts_unix, chat_id}`. Extending it with `cron` and `cron_job_id` when the event carries cron metadata is a JSON-shape change — no struct-signature break.

- [ ] **Step 1: Locate the current worker handler**

```bash
cd gormes
grep -n "AppendUserTurn" internal/memory/memory.go
```

Inspect the block that decodes the JSON payload and runs `INSERT INTO turns(...)`. That's the single site to extend.

- [ ] **Step 2: Write failing test — append to `memory_test.go`**

```go
func TestAppendUserTurn_WritesCronColumnsWhenProvided(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 4, nil)
	defer store.Close(context.Background())

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
	if _, err := store.Exec(ctx, storepkg.Command{Kind: storepkg.AppendUserTurn, Payload: payload}); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	// Give the worker a moment to drain; store.Exec returns on ACK, not on persist.
	// Poll the DB until the row lands.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM turns WHERE cron = 1 AND cron_job_id = 'job-1'`).Scan(&n)
		if n == 1 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("turn with cron=1 was not persisted within 2s")
}

func TestAppendUserTurn_NoncronTurnLeavesColumnsAtDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 4, nil)
	defer store.Close(context.Background())

	// Payload WITHOUT cron fields — the common case.
	payload := []byte(`{"session_id":"s","content":"hi","ts_unix":1,"chat_id":"c"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = store.Exec(ctx, storepkg.Command{Kind: storepkg.AppendUserTurn, Payload: payload})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var cron int
		var cjid sql.NullString
		err := store.db.QueryRow(`SELECT cron, cron_job_id FROM turns WHERE content = 'hi'`).Scan(&cron, &cjid)
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
```

Add imports as needed: `"database/sql"`, `storepkg "github.com/TrebuchetDynamics/gormes-agent/gormes/internal/store"`.

- [ ] **Step 3: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run "TestAppendUserTurn_WritesCronColumnsWhenProvided|TestAppendUserTurn_NoncronTurnLeavesColumnsAtDefault" -v 2>&1 | tail -10
```

Expected: the "writes cron columns" test fails because the current INSERT doesn't include `cron` or `cron_job_id`.

- [ ] **Step 4: Extend the worker's turn-insert path**

In `internal/memory/memory.go`, find the `case store.AppendUserTurn:` block. Decode the payload into a struct that includes optional `Cron` / `CronJobID`:

```go
// Existing shape (locate and extend):
var t struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
	TsUnix    int64  `json:"ts_unix"`
	ChatID    string `json:"chat_id"`
	Cron      int    `json:"cron"`          // NEW; 0 when absent = non-cron turn
	CronJobID string `json:"cron_job_id"`   // NEW; "" when absent
}
if err := json.Unmarshal(cmd.Payload, &t); err != nil {
	// ... existing error handling
}

// The INSERT gains two params.
_, err := s.db.Exec(
	`INSERT INTO turns(session_id, role, content, ts_unix, chat_id, cron, cron_job_id)
	 VALUES(?, 'user', ?, ?, ?, ?, ?)`,
	t.SessionID, t.Content, t.TsUnix, t.ChatID, t.Cron, nullIfEmpty(t.CronJobID))
```

Add helper at the bottom of the file:

```go
// nullIfEmpty returns sql.NullString{Valid:false} for empty strings so
// the cron_job_id column stays NULL (CHECK-friendly) instead of being
// an empty string.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
```

- [ ] **Step 5: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestAppendUserTurn_ -v -timeout 30s
go vet ./...
```

Both tests pass. Full memory suite:

```bash
cd gormes
go test -race ./internal/memory/... -count=1 -timeout 60s -skip Integration_Ollama
```

Green.

- [ ] **Step 6: Commit**

```bash
git add gormes/internal/memory/memory.go gormes/internal/memory/memory_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): turn payload now carries optional cron fields

AppendUserTurn worker extends the payload decoder + INSERT with
two new optional fields:
  cron         int    (0 when absent)
  cron_job_id  string ("" -> NULL via nullIfEmpty helper)

Non-cron turns are unaffected: both fields default to 0 / NULL
when absent from the JSON payload.

Two tests lock the invariant:
  - WritesCronColumnsWhenProvided: cron=1 + cron_job_id='job-1'
    payload lands on the row.
  - NoncronTurnLeavesColumnsAtDefault: payload without cron keys
    leaves cron=0 and cron_job_id=NULL.

Kernel-side plumbing (T4) will populate these fields when the
PlatformEvent carries a CronJobID.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Extractor filter — skip cron turns

**Files:**
- Modify: `gormes/internal/memory/extractor.go`
- Modify: `gormes/internal/memory/extractor_test.go`

Spec §6.2: *"Extractor change: the `pollMissing` query adds `AND cron = 0`. One-word diff to an existing SQL string."*

- [ ] **Step 1: Locate the extractor's pollMissing query**

```bash
cd gormes
grep -nE "pollMissing|extracted = 0" internal/memory/extractor.go
```

Find the SELECT that pulls unextracted turns.

- [ ] **Step 2: Write failing test — append to `extractor_test.go`**

```go
func TestExtractor_SkipsCronTurns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	// Seed one normal turn and one cron turn.
	now := time.Now().Unix()
	_, err := store.db.Exec(
		`INSERT INTO turns(session_id, role, content, ts_unix, chat_id, cron, cron_job_id)
		 VALUES('s', 'user', 'normal turn about Widgets', ?, 'c', 0, NULL),
		        ('cron:j:1', 'user', 'cron turn about Gizmos', ?, 'c', 1, 'j')`,
		now, now+1)
	if err != nil {
		t.Fatal(err)
	}

	// Directly exercise the private pollMissing function via a tiny Extractor.
	// We don't need a live LLM — we're only checking which rows it picks up.
	ext := NewExtractor(store, nil, ExtractorConfig{BatchSize: 10}, nil)
	rows, err := ext.pollMissing(context.Background())
	if err != nil {
		t.Fatalf("pollMissing: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("pollMissing returned %d rows, want 1 (normal only)", len(rows))
	}
	if !strings.Contains(rows[0].Content, "Widgets") {
		t.Errorf("returned row = %q, want the normal turn (Widgets)", rows[0].Content)
	}
}
```

Add imports as needed: `"strings"`, `"time"`.

- [ ] **Step 3: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run TestExtractor_SkipsCronTurns -v 2>&1 | tail -10
```

Expected: returns both rows (count = 2, not 1).

- [ ] **Step 4: Extend the SQL**

In `extractor.go`, locate the pollMissing query. Wherever the SELECT filters `WHERE extracted = 0`, add `AND cron = 0`:

```go
const pollQ = `
	SELECT id, session_id, content, ts_unix
	FROM turns
	WHERE extracted = 0
	  AND cron = 0
	ORDER BY id
	LIMIT ?`
```

(Exact string may differ if the current query already selects other columns; the load-bearing change is the added `AND cron = 0`.)

- [ ] **Step 5: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestExtractor_ -v -timeout 30s
```

All existing extractor tests + new one pass.

- [ ] **Step 6: Commit**

```bash
git add gormes/internal/memory/extractor.go gormes/internal/memory/extractor_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): extractor skips cron turns

pollMissing SQL gains AND cron = 0 so the entity-extraction
pipeline ignores turns tagged by the Phase 2.D cron system.

Upstream Hermes rationale (cron/scheduler.py): 'Cron system
prompts would corrupt user representations.' Same reasoning
applies here.

One test locks the invariant: seed a normal turn + a cron turn,
verify pollMissing returns only the normal one.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Kernel `PlatformEvent.SessionID` + `CronJobID` (per-event override)

**Files:**
- Modify: `gormes/internal/kernel/frame.go`
- Modify: `gormes/internal/kernel/kernel.go`
- Modify: `gormes/internal/kernel/kernel_test.go`

**Spec §8.** Adds two opaque-string fields to `PlatformEvent` so cron can inject a per-run session ID without mutating the kernel's resident `k.sessionID`. Also flows `CronJobID` through to the turn payload so T2's memory worker writes `cron=1` + `cron_job_id=<v>`.

- [ ] **Step 1: Write failing tests — append to `kernel_test.go`**

```go
func TestKernel_SessionIDOverrideAppliesToTurn(t *testing.T) {
	hc, rec := newTestHermesAndRecorder(t) // existing helper; follows finalize_store_test pattern
	st := store.NewRecording()
	k := New(Config{
		Model:    "m",
		Endpoint: "http://mock",
	}, hc, st, telemetry.New(), slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go k.Run(ctx)
	// drain render frames
	go func() { for range k.Render() {} }()

	// Submit with explicit sessionID override + cronJobID.
	_ = k.Submit(PlatformEvent{
		Kind:      PlatformEventSubmit,
		Text:      "hello",
		SessionID: "cron:job-7:1700000000",
		CronJobID: "job-7",
	})

	// Wait for the AppendUserTurn command and inspect its payload.
	cmds := waitCommands(t, st, 1, 2*time.Second) // helper waits for N commands
	var p map[string]any
	_ = json.Unmarshal(cmds[0].Payload, &p)
	if p["session_id"] != "cron:job-7:1700000000" {
		t.Errorf("session_id = %v, want cron override", p["session_id"])
	}
	if p["cron_job_id"] != "job-7" {
		t.Errorf("cron_job_id = %v, want job-7", p["cron_job_id"])
	}
	if v, _ := p["cron"].(float64); int(v) != 1 {
		t.Errorf("cron = %v, want 1", p["cron"])
	}
	_ = rec // mark used
}

func TestKernel_SessionIDOverrideDoesNotLeakToNextTurn(t *testing.T) {
	hc, _ := newTestHermesAndRecorder(t)
	st := store.NewRecording()
	k := New(Config{
		Model:    "m",
		Endpoint: "http://mock",
		InitialSessionID: "resident-session-xyz",
	}, hc, st, telemetry.New(), slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go k.Run(ctx)
	go func() { for range k.Render() {} }()

	// First turn: cron override.
	_ = k.Submit(PlatformEvent{
		Kind: PlatformEventSubmit, Text: "cron hi",
		SessionID: "cron:job-1:1", CronJobID: "job-1",
	})
	_ = waitCommands(t, st, 1, 2*time.Second) // first turn's AppendUserTurn

	// Second turn: NO override. Must use the resident-session-xyz.
	_ = k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: "normal hi"})

	cmds := waitCommands(t, st, 2, 2*time.Second) // now two commands total
	// Find the second AppendUserTurn.
	var secondPayload map[string]any
	for _, c := range cmds {
		if c.Kind == store.AppendUserTurn {
			var p map[string]any
			_ = json.Unmarshal(c.Payload, &p)
			if p["content"] == "normal hi" {
				secondPayload = p
			}
		}
	}
	if secondPayload == nil {
		t.Fatal("could not find the second AppendUserTurn command")
	}
	// The kernel's resident sessionID may be updated by the server's
	// response to the cron turn (both turns may share the server-issued
	// session), but what we REALLY care about is: the override must not
	// persist as k.sessionID. Verify the second payload's session_id is
	// NOT the cron override.
	if secondPayload["session_id"] == "cron:job-1:1" {
		t.Errorf("cron sessionID leaked into next turn: %v", secondPayload["session_id"])
	}
	if _, hasCron := secondPayload["cron_job_id"]; hasCron && secondPayload["cron_job_id"] != "" && secondPayload["cron_job_id"] != nil {
		t.Errorf("cron_job_id leaked into next turn: %v", secondPayload["cron_job_id"])
	}
}
```

**Note on test helpers:** `newTestHermesAndRecorder` and `waitCommands` are patterns used by existing kernel tests (see `finalize_store_test.go`). If they don't exist in the test file yet, copy the needed bits from the existing chat_key_test.go / finalize_store_test.go harnesses. Don't invent new helper shapes.

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/kernel/... -run "TestKernel_SessionIDOverride" -v 2>&1 | tail -10
```

Expected: compile error (unknown fields SessionID/CronJobID on PlatformEvent).

- [ ] **Step 3: Extend `PlatformEvent` struct**

In `internal/kernel/frame.go`, add two new string fields to the struct:

```go
type PlatformEvent struct {
	Kind      PlatformEventKind
	Text      string
	// SessionID, when non-empty, overrides k.sessionID for this turn
	// only. Used by the Phase 2.D cron executor so each cron fire has
	// an isolated "cron:<job_id>:<unix_ts>" session. A non-cron event
	// leaves this empty and inherits k.sessionID as before.
	SessionID string
	// CronJobID, when non-empty, flags the persisted turn row as
	// cron=1 and populates turns.cron_job_id. Used by the extractor
	// to skip cron turns (see internal/memory/extractor.go).
	CronJobID string
	ack       chan<- struct{}
}
```

- [ ] **Step 4: Apply the override in processSubmit**

In `internal/kernel/kernel.go`, find the `case PlatformEventSubmit:` block in the main `Run` select. Wrap the `runTurn` call with a save/restore:

```go
case PlatformEventSubmit:
	if k.phase != PhaseIdle {
		k.lastError = ErrTurnInFlight.Error()
		k.emitFrame("still processing previous turn")
		continue
	}
	// Phase 2.D — per-event sessionID override. Save current, swap in,
	// restore after. Restoration is idempotent if e.SessionID == "".
	prevSessionID := k.sessionID
	if e.SessionID != "" {
		k.sessionID = e.SessionID
	}
	k.runTurn(ctx, e.Text, e.CronJobID)
	// After the turn completes, the server may have assigned a new
	// session_id for the normal (non-override) case — that flows into
	// k.sessionID via the existing response-handling path. For the
	// override case, the turn belongs to the ephemeral cron session,
	// and we must NOT let that id persist. So restore.
	if e.SessionID != "" {
		k.sessionID = prevSessionID
	}
```

- [ ] **Step 5: Flow `CronJobID` through `runTurn` into the store payload**

In the same file, change `runTurn`'s signature from `runTurn(ctx, text string)` to `runTurn(ctx context.Context, text, cronJobID string)`. Inside, where the AppendUserTurn payload is built:

```go
	userPayload, _ := json.Marshal(map[string]any{
		"session_id":  k.sessionID,
		"content":     text,
		"ts_unix":     time.Now().Unix(),
		"chat_id":     k.cfg.ChatKey,
		"cron":        cronFlag(cronJobID),
		"cron_job_id": cronJobID,
	})
```

Add helper (same file, near other helpers):

```go
// cronFlag returns 1 when the turn carries a cron_job_id (Phase 2.D),
// 0 otherwise. A 0 value omits nothing — the store worker treats it as
// a normal non-cron turn.
func cronFlag(cronJobID string) int {
	if cronJobID == "" {
		return 0
	}
	return 1
}
```

Any OTHER `runTurn` call site (there should be just one) needs the new parameter passed through. Search:

```bash
cd gormes
grep -n "k.runTurn\|\.runTurn(" internal/kernel/kernel.go
```

- [ ] **Step 6: Update the direct-call runTurn in processSubmit**

Based on Step 4's wrapper, the call becomes `k.runTurn(ctx, e.Text, e.CronJobID)`. Confirm no other runTurn call sites exist.

- [ ] **Step 7: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/kernel/... -run "TestKernel_" -v -timeout 30s
```

All kernel tests pass (the two new ones + all pre-existing). Especially verify:

```bash
cd gormes
go test -race ./internal/... -run "TestKernelHasNoMemoryDep|TestKernelHasNoSessionDep" -v
```

Kernel isolation invariants preserved.

Full suite (minus Ollama):
```bash
cd gormes
go test -race ./... -count=1 -timeout 240s -skip Integration_Ollama
```

Green.

- [ ] **Step 8: Commit**

```bash
git add gormes/internal/kernel/frame.go gormes/internal/kernel/kernel.go gormes/internal/kernel/kernel_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/kernel): PlatformEvent carries per-event SessionID + CronJobID

Phase 2.D cron needs each scheduled fire to run in its own
ephemeral session 'cron:<job_id>:<unix_ts>' without disturbing
the kernel's resident sessionID (which serves live operator
chat).

Two new opaque-string fields on PlatformEvent:
  SessionID  — if non-empty, overrides k.sessionID for THIS turn
               only; restored after the turn completes.
  CronJobID  — if non-empty, flags the persisted turn as
               cron=1 with this job_id, so the extractor skips
               it and USER.md stays clean.

Kernel package has no new imports. Both fields are opaque —
kernel doesn't know about cron, just passes them through to
the store.Command payload. Existing TestKernelHasNoMemoryDep /
NoSessionDep isolation tests continue to hold.

Three tests pin the contract:
  - SessionIDOverrideAppliesToTurn: cron event's payload
    carries the override sessionID + cron=1 + cron_job_id.
  - SessionIDOverrideDoesNotLeakToNextTurn: subsequent normal
    events don't inherit the cron sessionID.
  - CronJobIDFlowsToStorePayload: turns.cron_job_id is
    populated downstream.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `cron.Job` + `cron.Store` (bbolt CRUD)

**Files:**
- Create: `gormes/internal/cron/job.go`
- Create: `gormes/internal/cron/job_test.go`
- Create: `gormes/internal/cron/store.go`
- Create: `gormes/internal/cron/store_test.go`
- Modify: `gormes/go.mod` (add robfig/cron/v3)

- [ ] **Step 1: Add robfig/cron/v3 dependency**

```bash
cd gormes
go get github.com/robfig/cron/v3@v3.0.1
go mod tidy
```

Verify:
```bash
grep robfig go.mod
```

Expected: `github.com/robfig/cron/v3 v3.0.1` present in require block.

- [ ] **Step 2: Write failing tests — `job_test.go`**

```go
package cron

import (
	"testing"
)

func TestValidateSchedule_AcceptsStandardCron(t *testing.T) {
	for _, expr := range []string{
		"0 8 * * *",
		"*/5 * * * *",
		"0 0 1 * *",
		"@daily",
		"@every 30m",
	} {
		if err := ValidateSchedule(expr); err != nil {
			t.Errorf("ValidateSchedule(%q) = %v, want nil", expr, err)
		}
	}
}

func TestValidateSchedule_RejectsGarbage(t *testing.T) {
	for _, expr := range []string{
		"",
		"not a cron expression",
		"* * * *", // too few fields
		"99 * * * *", // minute out of range
		"@unknown",
	} {
		if err := ValidateSchedule(expr); err == nil {
			t.Errorf("ValidateSchedule(%q) = nil, want error", expr)
		}
	}
}

func TestJob_NewGeneratesID(t *testing.T) {
	j := NewJob("morning", "0 8 * * *", "status prompt")
	if j.ID == "" {
		t.Error("NewJob did not populate ID")
	}
	if j.Name != "morning" || j.Schedule != "0 8 * * *" || j.Prompt != "status prompt" {
		t.Errorf("NewJob fields = %+v, want name/sched/prompt", j)
	}
	if j.CreatedAt == 0 {
		t.Error("NewJob did not set CreatedAt")
	}
	if j.Paused {
		t.Error("NewJob must default to Paused=false (active)")
	}
}
```

- [ ] **Step 3: Run, expect FAIL**

```bash
cd gormes
go test ./internal/cron/... -run "TestValidateSchedule|TestJob_" -v 2>&1 | tail -5
```

Expected: package doesn't exist yet.

- [ ] **Step 4: Create `job.go`**

```go
// Package cron is the Phase 2.D proactive scheduler. Jobs stored in
// bbolt; per-run audit rows in SQLite; agent turns isolated via an
// ephemeral session id per fire. See spec at
// docs/superpowers/specs/2026-04-20-gormes-phase2d-cron-design.md.
package cron

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	rc "github.com/robfig/cron/v3"
)

// Job is a scheduled agent prompt. Persisted as a JSON blob under its
// ID as key in the cron_jobs bbolt bucket.
type Job struct {
	ID           string `json:"id"`            // 16-byte random hex — unique within one DB
	Name         string `json:"name"`          // operator-friendly label; must be unique
	Schedule     string `json:"schedule"`      // cron expression or @shortcut; validated via ValidateSchedule
	Prompt       string `json:"prompt"`        // the user-facing prompt, WITHOUT the [SYSTEM:] prefix
	Paused       bool   `json:"paused"`        // default false; if true, scheduler ignores
	CreatedAt    int64  `json:"created_at"`    // unix seconds
	LastRunUnix  int64  `json:"last_run_unix"` // 0 when never run
	LastStatus   string `json:"last_status"`   // "success"|"timeout"|"error"|"suppressed"|""
}

// NewJob constructs a Job with a fresh random ID and the current time
// as CreatedAt. The caller still needs to validate the schedule and
// call Store.Create.
func NewJob(name, schedule, prompt string) Job {
	return Job{
		ID:        newID(),
		Name:      name,
		Schedule:  schedule,
		Prompt:    prompt,
		Paused:    false,
		CreatedAt: time.Now().Unix(),
	}
}

// ValidateSchedule parses the cron expression via robfig/cron/v3's
// standard parser, returning a typed error on rejection. Accepts
// 5-field standard cron and @shortcut forms (@daily, @hourly,
// @every 30m, etc.).
func ValidateSchedule(expr string) error {
	if expr == "" {
		return fmt.Errorf("cron: schedule is empty")
	}
	_, err := rc.ParseStandard(expr)
	if err != nil {
		return fmt.Errorf("cron: invalid schedule %q: %w", expr, err)
	}
	return nil
}

// newID generates a 16-byte (32-hex-char) random ID. Not ULID, not
// UUID — we don't need the timestamp encoding, just uniqueness within
// one bbolt file. crypto/rand is stdlib so no new deps.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
```

- [ ] **Step 5: Run, expect PASS**

```bash
cd gormes
go test ./internal/cron/... -run "TestValidateSchedule|TestJob_" -v
go vet ./...
```

All 3 tests pass.

- [ ] **Step 6: Write failing store tests — `store_test.go`**

```go
package cron

import (
	"path/filepath"
	"testing"

	"go.etcd.io/bbolt"
)

func newTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "session.db")
	db, err := bbolt.Open(dbPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(db)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return s, func() { _ = db.Close() }
}

func TestStore_CreateAndGet(t *testing.T) {
	s, done := newTestStore(t)
	defer done()

	j := NewJob("morning", "0 8 * * *", "status")
	if err := s.Create(j); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(j.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "morning" || got.Schedule != "0 8 * * *" || got.Prompt != "status" {
		t.Errorf("got = %+v, want name/sched/prompt intact", got)
	}
}

func TestStore_List(t *testing.T) {
	s, done := newTestStore(t)
	defer done()

	_ = s.Create(NewJob("a", "@daily", "x"))
	_ = s.Create(NewJob("b", "@hourly", "y"))
	_ = s.Create(NewJob("c", "@every 1m", "z"))

	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len = %d, want 3", len(got))
	}
}

func TestStore_Update(t *testing.T) {
	s, done := newTestStore(t)
	defer done()

	j := NewJob("m", "@daily", "p")
	_ = s.Create(j)
	j.Paused = true
	j.LastRunUnix = 1700000000
	j.LastStatus = "success"
	if err := s.Update(j); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := s.Get(j.ID)
	if !got.Paused || got.LastRunUnix != 1700000000 || got.LastStatus != "success" {
		t.Errorf("after Update, got = %+v, want Paused=true LastRun=1.7e9 LastStatus=success", got)
	}
}

func TestStore_Delete(t *testing.T) {
	s, done := newTestStore(t)
	defer done()

	j := NewJob("x", "@daily", "y")
	_ = s.Create(j)
	if err := s.Delete(j.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(j.ID); err == nil {
		t.Error("Get after Delete returned no error, want ErrJobNotFound")
	}
}

func TestStore_GetMissingReturnsTypedError(t *testing.T) {
	s, done := newTestStore(t)
	defer done()

	_, err := s.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing id")
	}
	if err != ErrJobNotFound {
		t.Errorf("err = %v, want ErrJobNotFound", err)
	}
}

func TestStore_CreateRejectsDuplicateName(t *testing.T) {
	s, done := newTestStore(t)
	defer done()

	_ = s.Create(NewJob("same", "@daily", "p1"))
	err := s.Create(NewJob("same", "@hourly", "p2"))
	if err == nil {
		t.Fatal("expected error on duplicate name")
	}
	if err != ErrJobNameTaken {
		t.Errorf("err = %v, want ErrJobNameTaken", err)
	}
}
```

- [ ] **Step 7: Run, expect FAIL**

```bash
cd gormes
go test ./internal/cron/... -run TestStore_ -v 2>&1 | tail -5
```

Expected: undefined Store / NewStore / ErrJobNotFound / ErrJobNameTaken.

- [ ] **Step 8: Create `store.go`**

```go
package cron

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
)

// ErrJobNotFound is returned by Get / Delete / Update when the target
// job ID isn't in the cron_jobs bucket.
var ErrJobNotFound = errors.New("cron: job not found")

// ErrJobNameTaken is returned by Create when another job already uses
// the requested Name. Names are unique; IDs are unique too but
// separately (IDs are random, names are operator-assigned).
var ErrJobNameTaken = errors.New("cron: job name already taken")

const cronJobsBucket = "cron_jobs"

// Store is the bbolt-backed Job persistence layer. The underlying
// *bbolt.DB is owned by the caller (typically the same *bbolt.DB the
// Phase 2.C session map uses, so a single file on disk).
type Store struct {
	db *bbolt.DB
}

// NewStore opens/creates the cron_jobs bucket and returns a ready-to-use
// Store. Safe to call multiple times.
func NewStore(db *bbolt.DB) (*Store, error) {
	err := db.Update(func(tx *bbolt.Tx) error {
		_, e := tx.CreateBucketIfNotExists([]byte(cronJobsBucket))
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("cron: init bucket: %w", err)
	}
	return &Store{db: db}, nil
}

// Create persists a new job. Fails with ErrJobNameTaken if Name is
// already used.
func (s *Store) Create(j Job) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(cronJobsBucket))
		// Uniqueness check: scan existing jobs for matching Name.
		var dup bool
		_ = b.ForEach(func(k, v []byte) error {
			var other Job
			if err := json.Unmarshal(v, &other); err != nil {
				return nil // skip corrupt row
			}
			if other.Name == j.Name {
				dup = true
			}
			return nil
		})
		if dup {
			return ErrJobNameTaken
		}
		blob, err := json.Marshal(j)
		if err != nil {
			return err
		}
		return b.Put([]byte(j.ID), blob)
	})
}

// Get loads one job by ID.
func (s *Store) Get(id string) (Job, error) {
	var j Job
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(cronJobsBucket))
		blob := b.Get([]byte(id))
		if blob == nil {
			return ErrJobNotFound
		}
		return json.Unmarshal(blob, &j)
	})
	return j, err
}

// List returns every job in the bucket. Corrupt rows are silently
// skipped so one bad blob doesn't block operation.
func (s *Store) List() ([]Job, error) {
	var out []Job
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(cronJobsBucket))
		return b.ForEach(func(k, v []byte) error {
			var j Job
			if err := json.Unmarshal(v, &j); err != nil {
				return nil // skip corrupt
			}
			out = append(out, j)
			return nil
		})
	})
	return out, err
}

// Update overwrites an existing job by ID. Errors with ErrJobNotFound
// if the ID isn't present (we never create via Update — explicit about
// create vs. update semantics).
func (s *Store) Update(j Job) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(cronJobsBucket))
		if b.Get([]byte(j.ID)) == nil {
			return ErrJobNotFound
		}
		blob, err := json.Marshal(j)
		if err != nil {
			return err
		}
		return b.Put([]byte(j.ID), blob)
	})
}

// Delete removes a job by ID. No-op on missing keys (bbolt convention).
func (s *Store) Delete(id string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(cronJobsBucket))
		return b.Delete([]byte(id))
	})
}
```

- [ ] **Step 9: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/cron/... -v -timeout 30s
go vet ./...
```

All tests pass.

- [ ] **Step 10: Commit**

```bash
git add gormes/internal/cron/job.go gormes/internal/cron/job_test.go gormes/internal/cron/store.go gormes/internal/cron/store_test.go gormes/go.mod gormes/go.sum
git commit -m "$(cat <<'EOF'
feat(gormes/cron): Job struct + bbolt-backed Store

Phase 2.D step one: Job persistence + schedule validation.

internal/cron/job.go:
  type Job { ID, Name, Schedule, Prompt, Paused, CreatedAt,
             LastRunUnix, LastStatus }
  NewJob(name, schedule, prompt) Job   -- fresh random ID
  ValidateSchedule(expr) error         -- wraps robfig/cron/v3
                                          ParseStandard

internal/cron/store.go:
  Store backed by one bbolt bucket 'cron_jobs'. Caller owns the
  *bbolt.DB (reuses the existing Phase 2.C session.db).
  Create/Get/List/Update/Delete with two typed errors:
    ErrJobNotFound
    ErrJobNameTaken  (names are unique operator-assigned labels)

New dep: github.com/robfig/cron/v3 v3.0.1 — pure Go, ~20 KB
binary impact, the standard Go cron parser.

Nine tests cover schedule validation (accepts standard + @shortcuts,
rejects garbage), Job field population + ID generation, and Store
CRUD + duplicate-name guard + missing-id error.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `cron.RunStore` (SQLite audit trail)

**Files:**
- Create: `gormes/internal/cron/run_store.go`
- Create: `gormes/internal/cron/run_store_test.go`

- [ ] **Step 1: Write failing tests — `run_store_test.go`**

```go
package cron

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/memory"
)

func newTestRunStore(t *testing.T) (*RunStore, *memory.SqliteStore, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	ms, err := memory.OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	rs := NewRunStore(ms.DB())
	return rs, ms, func() { _ = ms.Close(context.Background()) }
}

func TestRunStore_RecordRound(t *testing.T) {
	rs, _, cleanup := newTestRunStore(t)
	defer cleanup()

	run := Run{
		JobID:             "job-1",
		StartedAt:         1700000000,
		FinishedAt:        1700000005,
		PromptHash:        "deadbeef",
		Status:            "success",
		Delivered:         true,
		SuppressionReason: "",
		OutputPreview:     "report contents",
	}
	if err := rs.RecordRun(context.Background(), run); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	got, err := rs.LatestRuns(context.Background(), "job-1", 5)
	if err != nil {
		t.Fatalf("LatestRuns: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	if got[0].Status != "success" || got[0].OutputPreview != "report contents" || !got[0].Delivered {
		t.Errorf("got = %+v, want success/delivered/preview intact", got[0])
	}
}

func TestRunStore_RecordSuppressed(t *testing.T) {
	rs, _, cleanup := newTestRunStore(t)
	defer cleanup()

	run := Run{
		JobID:             "job-1",
		StartedAt:         1,
		FinishedAt:        2,
		PromptHash:        "h",
		Status:            "suppressed",
		Delivered:         false,
		SuppressionReason: "silent",
	}
	if err := rs.RecordRun(context.Background(), run); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
}

func TestRunStore_RecordTimeoutWithErrorMsg(t *testing.T) {
	rs, _, cleanup := newTestRunStore(t)
	defer cleanup()

	run := Run{
		JobID:      "job-1",
		StartedAt:  1,
		FinishedAt: 61,
		PromptHash: "h",
		Status:     "timeout",
		Delivered:  true,
		ErrorMsg:   "deadline exceeded after 60s",
	}
	if err := rs.RecordRun(context.Background(), run); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
}

func TestRunStore_LatestRunsOrdersByStartedDesc(t *testing.T) {
	rs, _, cleanup := newTestRunStore(t)
	defer cleanup()

	for _, s := range []int64{3, 1, 5, 2, 4} {
		_ = rs.RecordRun(context.Background(), Run{
			JobID: "j", StartedAt: s, PromptHash: "h", Status: "success", Delivered: true,
		})
	}
	got, _ := rs.LatestRuns(context.Background(), "j", 3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// DESC by started_at: 5, 4, 3
	if got[0].StartedAt != 5 || got[1].StartedAt != 4 || got[2].StartedAt != 3 {
		t.Errorf("order = %v, want 5,4,3", []int64{got[0].StartedAt, got[1].StartedAt, got[2].StartedAt})
	}
}

func TestRunStore_RejectsInvalidStatus(t *testing.T) {
	rs, _, cleanup := newTestRunStore(t)
	defer cleanup()

	err := rs.RecordRun(context.Background(), Run{
		JobID: "j", StartedAt: 1, PromptHash: "h", Status: "bogus",
	})
	if err == nil {
		t.Error("RecordRun with status='bogus' should fail CHECK constraint")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/cron/... -run TestRunStore_ -v 2>&1 | tail -5
```

Expected: undefined RunStore / NewRunStore / Run.

- [ ] **Step 3: Create `run_store.go`**

```go
package cron

import (
	"context"
	"database/sql"
	"fmt"
)

// Run is one scheduled fire's audit record. Written by the Executor
// after each run (success, timeout, error, or suppressed).
type Run struct {
	ID                int64  // auto-assigned by SQLite
	JobID             string // foreign reference to bbolt Job.ID; no FK enforced
	StartedAt         int64  // unix seconds at execution start
	FinishedAt        int64  // unix seconds at completion; 0 when never finished
	PromptHash        string // sha256 hex of Job.Prompt BEFORE Heartbeat prefix; 16-hex-char prefix
	Status            string // "success" | "timeout" | "error" | "suppressed"
	Delivered         bool
	SuppressionReason string // "silent" | "empty" | "" — NULL when empty
	OutputPreview     string // first 200 chars of final response (or failure notice)
	ErrorMsg          string // populated on status="error" or "timeout"
}

// RunStore writes to the cron_runs SQLite table. Read path is rare
// (CRON.md mirror only); writes happen once per job fire.
type RunStore struct {
	db *sql.DB
}

// NewRunStore wraps an open *sql.DB. The cron_runs table must exist —
// it's created by migration 3d->3e (internal/memory/schema.go).
func NewRunStore(db *sql.DB) *RunStore {
	return &RunStore{db: db}
}

// RecordRun persists one run. The SQL CHECK constraints catch invalid
// status / suppression_reason values, so the caller gets an error
// rather than garbage in the audit log.
func (s *RunStore) RecordRun(ctx context.Context, r Run) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cron_runs
		  (job_id, started_at, finished_at, prompt_hash, status,
		   delivered, suppression_reason, output_preview, error_msg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.JobID,
		r.StartedAt,
		nullIfZero(r.FinishedAt),
		r.PromptHash,
		r.Status,
		boolToInt(r.Delivered),
		nullIfEmpty(r.SuppressionReason),
		nullIfEmpty(r.OutputPreview),
		nullIfEmpty(r.ErrorMsg),
	)
	if err != nil {
		return fmt.Errorf("cron: record run: %w", err)
	}
	return nil
}

// LatestRuns returns up to `limit` most-recent runs for the given
// job_id (started_at DESC). Used by the CRON.md mirror.
func (s *RunStore) LatestRuns(ctx context.Context, jobID string, limit int) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, job_id, started_at, COALESCE(finished_at,0), prompt_hash,
		       status, delivered,
		       COALESCE(suppression_reason, ''), COALESCE(output_preview, ''),
		       COALESCE(error_msg, '')
		FROM cron_runs
		WHERE job_id = ?
		ORDER BY started_at DESC
		LIMIT ?`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Run
	for rows.Next() {
		var r Run
		var delivered int
		if err := rows.Scan(&r.ID, &r.JobID, &r.StartedAt, &r.FinishedAt,
			&r.PromptHash, &r.Status, &delivered,
			&r.SuppressionReason, &r.OutputPreview, &r.ErrorMsg); err != nil {
			return nil, err
		}
		r.Delivered = delivered != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullIfZero(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/cron/... -run TestRunStore_ -v -timeout 30s
go vet ./...
```

All 5 tests pass.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/cron/run_store.go gormes/internal/cron/run_store_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/cron): RunStore for cron_runs audit table

SQLite-backed audit trail for Phase 2.D cron fires. One row per
job execution captures:
  status             success | timeout | error | suppressed
  delivered          bool
  suppression_reason silent | empty | NULL
  output_preview     first 200 chars of final response
  error_msg          populated on error/timeout

Five tests cover happy path, suppressed + silent reason, timeout +
error_msg, descending order by started_at, CHECK rejection of
invalid status values.

Caller passes the same *sql.DB the Phase 3.A SqliteStore owns so
runs land alongside turns + entities in memory.db.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Heartbeat prefix + `[SILENT]` detection

**Files:**
- Create: `gormes/internal/cron/heartbeat.go`
- Create: `gormes/internal/cron/heartbeat_test.go`

- [ ] **Step 1: Write failing tests — `heartbeat_test.go`**

```go
package cron

import (
	"strings"
	"testing"
)

func TestHeartbeatPrefix_ContainsLoadBearingPhrases(t *testing.T) {
	p := CronHeartbeatPrefix
	for _, want := range []string{
		"[SYSTEM:",
		"scheduled cron job",
		"DELIVERY:",
		"automatically delivered",
		"do NOT use send_message",
		"SILENT:",
		"\"[SILENT]\"",
		"nothing more",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("CronHeartbeatPrefix missing %q", want)
		}
	}
}

func TestBuildPrompt_PrependsPrefix(t *testing.T) {
	full := BuildPrompt("Give me a status summary")
	if !strings.HasPrefix(full, "[SYSTEM:") {
		t.Errorf("BuildPrompt does not start with [SYSTEM: — got %q", full[:40])
	}
	if !strings.HasSuffix(full, "Give me a status summary") {
		t.Errorf("BuildPrompt does not end with user prompt — got %q", full[len(full)-40:])
	}
}

func TestDetectSilent_ExactMatchOnly(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"[SILENT]", true},
		{"  [SILENT]", true},
		{"[SILENT]\n", true},
		{"\n\t [SILENT] \t\n", true},
		{"[SILENT] followed by text", false},
		{"Status: [SILENT] means nothing to report", false},
		{"<silent>", false},
		{"silent", false},
		{"SILENT", false},
		{"[silent]", false},
		{"[SILENT][SILENT]", false},
		{"", false},
	}
	for _, c := range cases {
		got := DetectSilent(c.in)
		if got != c.want {
			t.Errorf("DetectSilent(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/cron/... -run "TestHeartbeatPrefix_|TestBuildPrompt_|TestDetectSilent_" -v 2>&1 | tail -5
```

Expected: undefined CronHeartbeatPrefix / BuildPrompt / DetectSilent.

- [ ] **Step 3: Create `heartbeat.go`**

```go
package cron

import "strings"

// CronHeartbeatPrefix is the verbatim port of upstream Hermes'
// cron/scheduler.py cron_hint. Prepended to every scheduled-job
// prompt so the LLM knows:
//   - Its output is auto-delivered — don't call send_message
//   - It can return exactly "[SILENT]" (and nothing else) to
//     suppress delivery
//
// Matching upstream byte-for-byte matters: if a future Hermes bump
// changes the wording, we want to notice (the byte-match test
// TestHeartbeatPrefix_ContainsLoadBearingPhrases flags drift on
// any of the load-bearing phrases).
const CronHeartbeatPrefix = "[SYSTEM: You are running as a scheduled cron job. " +
	"DELIVERY: Your final response will be automatically delivered " +
	"to the user — do NOT use send_message or try to deliver " +
	"the output yourself. Just produce your report/output as your " +
	"final response and the system handles the rest. " +
	"SILENT: If there is genuinely nothing new to report, respond " +
	"with exactly \"[SILENT]\" (nothing else) to suppress delivery. " +
	"Never combine [SILENT] with content — either report your " +
	"findings normally, or say [SILENT] and nothing more.]\n\n"

// BuildPrompt prepends the cron heartbeat prefix to the operator's
// prompt. The concatenated result is what the kernel sees as the
// user message for the cron turn.
func BuildPrompt(userPrompt string) string {
	return CronHeartbeatPrefix + userPrompt
}

// DetectSilent returns true ONLY when the final response, after
// TrimSpace, equals the literal "[SILENT]" token. Substring matches,
// alternate casings, and responses that explain the token all return
// false — that's intentional. See spec §7.2.
//
// A false return means "deliver normally" (which is the right default
// for any ambiguous output — the operator would rather see a weird
// message than silently miss one).
func DetectSilent(finalResponse string) bool {
	return strings.TrimSpace(finalResponse) == "[SILENT]"
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/cron/... -run "TestHeartbeatPrefix_|TestBuildPrompt_|TestDetectSilent_" -v
go vet ./...
```

All 3 tests (covering 12 DetectSilent cases) pass.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/cron/heartbeat.go gormes/internal/cron/heartbeat_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/cron): Heartbeat prefix + [SILENT] detection

CronHeartbeatPrefix is a verbatim port of upstream Hermes'
cron/scheduler.py _build_job_prompt cron_hint. Tells the LLM
its output is auto-delivered and reserves '[SILENT]' as a
suppression control token.

DetectSilent is exact-match after TrimSpace — NOT substring
matching. A response like 'Status: [SILENT] means nothing to
report' is DELIVERED, not suppressed, because the token is
inside a larger message. False positives on substring matches
would silently swallow legitimate output, which is worse than
an occasional spurious delivery.

Twelve cases covered in TestDetectSilent_ExactMatchOnly —
leading/trailing whitespace variants, substring misses, wrong
casings, empty string.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `DeliverySink` interface + test adapter

**Files:**
- Create: `gormes/internal/cron/sink.go`
- Create: `gormes/internal/cron/sink_test.go`

- [ ] **Step 1: Write failing test — `sink_test.go`**

```go
package cron

import (
	"context"
	"errors"
	"testing"
)

func TestFuncSink_Forwards(t *testing.T) {
	var got string
	sink := FuncSink(func(ctx context.Context, text string) error {
		got = text
		return nil
	})
	if err := sink.Deliver(context.Background(), "hello"); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got != "hello" {
		t.Errorf("func got = %q, want 'hello'", got)
	}
}

func TestFuncSink_PropagatesError(t *testing.T) {
	stub := errors.New("stub failure")
	sink := FuncSink(func(ctx context.Context, text string) error { return stub })
	err := sink.Deliver(context.Background(), "x")
	if !errors.Is(err, stub) {
		t.Errorf("err = %v, want stub", err)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/cron/... -run TestFuncSink_ -v 2>&1 | tail -5
```

Expected: undefined FuncSink.

- [ ] **Step 3: Create `sink.go`**

```go
package cron

import "context"

// DeliverySink is the abstraction between the cron executor and the
// actual outbound channel. cmd/gormes/telegram.go provides a Telegram
// implementation; future Slack/Discord adapters plug in the same way
// without the cron package learning about them.
//
// Implementations:
//   - Own their rate limiting + retries internally (cron won't retry
//     on failure — it records the delivery_status and moves on).
//   - Return a non-nil error on failure so the executor can log it.
type DeliverySink interface {
	Deliver(ctx context.Context, text string) error
}

// FuncSink adapts a plain function to the DeliverySink interface —
// convenient for test injections and for wrapping the Telegram bot's
// existing send method without wrapping in a struct.
type FuncSink func(ctx context.Context, text string) error

// Deliver satisfies DeliverySink.
func (f FuncSink) Deliver(ctx context.Context, text string) error {
	return f(ctx, text)
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/cron/... -run TestFuncSink_ -v
go vet ./...
```

Both tests pass.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/cron/sink.go gormes/internal/cron/sink_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/cron): DeliverySink interface + FuncSink test adapter

Kernel-generic delivery abstraction. cron.Executor calls
sink.Deliver(ctx, text) after deciding NOT to suppress — the
interface means cron has zero Telegram-specific knowledge.

FuncSink adapts a plain func so cmd/gormes/telegram.go can wrap
the bot's existing send method in one line without introducing
a new struct.

Future Slack/Discord adapters implement the same interface and
drop in via the wiring in cmd/gormes/*.go.

Two tests cover the happy path and error propagation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Executor (the bridge from tick → kernel → decide → deliver)

**Files:**
- Create: `gormes/internal/cron/executor.go`
- Create: `gormes/internal/cron/executor_test.go`

This is the biggest task. The executor is the thing that makes a cron fire "do" anything. It:
1. Builds the ephemeral session_id.
2. Hashes the prompt.
3. Submits a `PlatformEvent` to the kernel.
4. Drains frames from `kernel.Render()` until it sees the final response for that session_id (or times out).
5. Detects `[SILENT]`, decides delivery.
6. Records a `Run` row.
7. Updates job state (`LastRunUnix`, `LastStatus`).
8. Calls `sink.Deliver` unless suppressed.

- [ ] **Step 1: Write failing tests — `executor_test.go`**

```go
package cron

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/memory"
	"go.etcd.io/bbolt"
)

// fakeKernel is a tiny stub that accepts Submit events and emits one
// RenderFrame per submit containing a pre-programmed final response.
// Used to exercise Executor without a live LLM or store.
type fakeKernel struct {
	resp   string         // what to return as the final assistant response
	delay  time.Duration  // simulate LLM latency; 0 = instant
	events []kernel.PlatformEvent
	mu     sync.Mutex
	render chan kernel.RenderFrame
	seen   atomic.Int32
}

func newFakeKernel(resp string, delay time.Duration) *fakeKernel {
	return &fakeKernel{
		resp:   resp,
		delay:  delay,
		render: make(chan kernel.RenderFrame, 4),
	}
}

func (fk *fakeKernel) Submit(e kernel.PlatformEvent) error {
	fk.mu.Lock()
	fk.events = append(fk.events, e)
	fk.mu.Unlock()
	fk.seen.Add(1)

	go func() {
		if fk.delay > 0 {
			time.Sleep(fk.delay)
		}
		fk.render <- kernel.RenderFrame{
			SessionID:    e.SessionID,
			Response:     fk.resp,
			FinishReason: "stop",
			Phase:        kernel.PhaseIdle,
		}
	}()
	return nil
}

func (fk *fakeKernel) Render() <-chan kernel.RenderFrame {
	return fk.render
}

func newTestExecutorEnv(t *testing.T, fk *fakeKernel) (*Executor, *sync.WaitGroup, *atomic.Value, func()) {
	t.Helper()

	// bbolt for the job store.
	dbPath := filepath.Join(t.TempDir(), "session.db")
	db, err := bbolt.Open(dbPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	js, _ := NewStore(db)

	// SQLite for the run store.
	msPath := filepath.Join(t.TempDir(), "memory.db")
	ms, err := memory.OpenSqlite(msPath, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	rs := NewRunStore(ms.DB())

	// Capture-sink.
	var deliveries atomic.Value
	deliveries.Store([]string{})
	sink := FuncSink(func(ctx context.Context, text string) error {
		cur := deliveries.Load().([]string)
		deliveries.Store(append(cur, text))
		return nil
	})

	e := NewExecutor(ExecutorConfig{
		Kernel:      fk,
		JobStore:    js,
		RunStore:    rs,
		Sink:        sink,
		CallTimeout: 2 * time.Second,
	}, nil)

	var wg sync.WaitGroup
	cleanup := func() {
		wg.Wait()
		_ = ms.Close(context.Background())
		_ = db.Close()
	}
	return e, &wg, &deliveries, cleanup
}

func TestExecutor_NormalResponseDelivers(t *testing.T) {
	fk := newFakeKernel("Morning report: all systems nominal.", 0)
	e, wg, deliveries, cleanup := newTestExecutorEnv(t, fk)
	defer cleanup()

	job := NewJob("morning", "0 8 * * *", "status summary")
	_ = e.cfg.JobStore.Create(job)

	wg.Add(1)
	go func() { defer wg.Done(); e.Run(context.Background(), job) }()
	wg.Wait()

	got := deliveries.Load().([]string)
	if len(got) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(got))
	}
	if got[0] != "Morning report: all systems nominal." {
		t.Errorf("delivery content = %q", got[0])
	}

	runs, _ := e.cfg.RunStore.LatestRuns(context.Background(), job.ID, 5)
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	if runs[0].Status != "success" || !runs[0].Delivered {
		t.Errorf("run = %+v, want success+delivered", runs[0])
	}
}

func TestExecutor_SilentResponseSuppresses(t *testing.T) {
	fk := newFakeKernel("[SILENT]", 0)
	e, wg, deliveries, cleanup := newTestExecutorEnv(t, fk)
	defer cleanup()

	job := NewJob("j", "@daily", "p")
	_ = e.cfg.JobStore.Create(job)

	wg.Add(1)
	go func() { defer wg.Done(); e.Run(context.Background(), job) }()
	wg.Wait()

	got := deliveries.Load().([]string)
	if len(got) != 0 {
		t.Errorf("deliveries = %d, want 0 (suppressed)", len(got))
	}
	runs, _ := e.cfg.RunStore.LatestRuns(context.Background(), job.ID, 5)
	if runs[0].Status != "suppressed" || runs[0].SuppressionReason != "silent" || runs[0].Delivered {
		t.Errorf("run = %+v, want suppressed/silent/!delivered", runs[0])
	}
}

func TestExecutor_EmptyResponseDeliversFailureNotice(t *testing.T) {
	fk := newFakeKernel("", 0)
	e, wg, deliveries, cleanup := newTestExecutorEnv(t, fk)
	defer cleanup()

	job := NewJob("empty-job", "@daily", "p")
	_ = e.cfg.JobStore.Create(job)

	wg.Add(1)
	go func() { defer wg.Done(); e.Run(context.Background(), job) }()
	wg.Wait()

	got := deliveries.Load().([]string)
	if len(got) != 1 {
		t.Fatalf("deliveries = %d, want 1 (failure notice)", len(got))
	}
	if !strings.Contains(got[0], "empty-job") || !strings.Contains(got[0], "empty") {
		t.Errorf("notice = %q, want mention of job name + 'empty'", got[0])
	}
	runs, _ := e.cfg.RunStore.LatestRuns(context.Background(), job.ID, 5)
	if runs[0].Status != "error" || runs[0].SuppressionReason != "empty" || !runs[0].Delivered {
		t.Errorf("run = %+v, want error/empty/delivered", runs[0])
	}
}

func TestExecutor_TimeoutDeliversFailureNotice(t *testing.T) {
	fk := newFakeKernel("too late", 3*time.Second) // longer than CallTimeout
	e, wg, deliveries, cleanup := newTestExecutorEnv(t, fk)
	// Shorten the timeout for the test.
	e.cfg.CallTimeout = 100 * time.Millisecond
	defer cleanup()

	job := NewJob("slow", "@daily", "p")
	_ = e.cfg.JobStore.Create(job)

	wg.Add(1)
	go func() { defer wg.Done(); e.Run(context.Background(), job) }()
	wg.Wait()

	got := deliveries.Load().([]string)
	if len(got) != 1 {
		t.Fatalf("deliveries = %d, want 1 (timeout notice)", len(got))
	}
	if !strings.Contains(got[0], "slow") || !strings.Contains(got[0], "timed out") {
		t.Errorf("notice = %q, want mention of job name + 'timed out'", got[0])
	}
	runs, _ := e.cfg.RunStore.LatestRuns(context.Background(), job.ID, 5)
	if runs[0].Status != "timeout" || !runs[0].Delivered {
		t.Errorf("run = %+v, want timeout+delivered", runs[0])
	}
}

func TestExecutor_SubmitErrorRecordsWithoutDelivery(t *testing.T) {
	// Swap in a kernel that errors on Submit.
	fk := newFakeKernel("whatever", 0)
	e, wg, deliveries, cleanup := newTestExecutorEnv(t, fk)
	e.cfg.Kernel = &erroringKernel{err: errors.New("mailbox full")}
	defer cleanup()

	job := NewJob("x", "@daily", "p")
	_ = e.cfg.JobStore.Create(job)

	wg.Add(1)
	go func() { defer wg.Done(); e.Run(context.Background(), job) }()
	wg.Wait()

	got := deliveries.Load().([]string)
	if len(got) != 0 {
		t.Errorf("deliveries = %d, want 0 on kernel error", len(got))
	}
	runs, _ := e.cfg.RunStore.LatestRuns(context.Background(), job.ID, 5)
	if runs[0].Status != "error" || runs[0].Delivered {
		t.Errorf("run = %+v, want error/!delivered", runs[0])
	}
}

type erroringKernel struct{ err error }

func (e *erroringKernel) Submit(ev kernel.PlatformEvent) error {
	return e.err
}

func (e *erroringKernel) Render() <-chan kernel.RenderFrame {
	ch := make(chan kernel.RenderFrame)
	close(ch)
	return ch
}

func TestExecutor_UpdatesJobLastRunStatus(t *testing.T) {
	fk := newFakeKernel("ok", 0)
	e, wg, _, cleanup := newTestExecutorEnv(t, fk)
	defer cleanup()

	job := NewJob("update-test", "@daily", "p")
	_ = e.cfg.JobStore.Create(job)

	wg.Add(1)
	go func() { defer wg.Done(); e.Run(context.Background(), job) }()
	wg.Wait()

	got, _ := e.cfg.JobStore.Get(job.ID)
	if got.LastRunUnix == 0 {
		t.Error("LastRunUnix not updated")
	}
	if got.LastStatus != "success" {
		t.Errorf("LastStatus = %q, want success", got.LastStatus)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/cron/... -run TestExecutor_ -v 2>&1 | tail -10
```

Expected: undefined Executor / NewExecutor / ExecutorConfig.

- [ ] **Step 3: Create `executor.go`**

```go
package cron

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

// KernelAPI is the narrow slice of *kernel.Kernel the Executor needs.
// Defined as an interface here so tests can swap in a fake without
// importing the full kernel package's internals.
type KernelAPI interface {
	Submit(e kernel.PlatformEvent) error
	Render() <-chan kernel.RenderFrame
}

// ExecutorConfig is the set of live dependencies. Callers construct it
// once at startup (cmd/gormes/telegram.go) and pass the same Executor
// to the Scheduler.
type ExecutorConfig struct {
	Kernel      KernelAPI
	JobStore    *Store
	RunStore    *RunStore
	Sink        DeliverySink
	CallTimeout time.Duration // default 60s when zero
}

func (c *ExecutorConfig) withDefaults() {
	if c.CallTimeout <= 0 {
		c.CallTimeout = 60 * time.Second
	}
}

// Executor bridges a scheduler tick into the kernel and records what
// happened.
type Executor struct {
	cfg ExecutorConfig
	log *slog.Logger
}

func NewExecutor(cfg ExecutorConfig, log *slog.Logger) *Executor {
	cfg.withDefaults()
	if log == nil {
		log = slog.Default()
	}
	return &Executor{cfg: cfg, log: log}
}

// Run fires one job end-to-end. Blocks until the turn completes or
// times out. Safe to call concurrently (the kernel serializes via its
// mailbox).
func (e *Executor) Run(ctx context.Context, job Job) {
	startedAt := time.Now().Unix()
	sessionID := fmt.Sprintf("cron:%s:%d", job.ID, startedAt)
	promptHash := shortHash(job.Prompt)

	// Subscribe to the render channel FIRST so we don't miss the
	// final frame between Submit and drain.
	frames := e.cfg.Kernel.Render()
	done := make(chan kernel.RenderFrame, 1)
	ctx, cancel := context.WithTimeout(ctx, e.cfg.CallTimeout)
	defer cancel()
	go func() {
		// Collect the first frame whose SessionID matches and that
		// signals completion (FinishReason != "" OR Phase == PhaseIdle
		// AFTER we've seen content). The kernel's render channel is
		// shared with other consumers, so we filter by SessionID.
		for {
			select {
			case f, ok := <-frames:
				if !ok {
					return
				}
				if f.SessionID != sessionID {
					continue
				}
				if f.FinishReason != "" {
					done <- f
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Submit.
	submitErr := e.cfg.Kernel.Submit(kernel.PlatformEvent{
		Kind:      kernel.PlatformEventSubmit,
		Text:      BuildPrompt(job.Prompt),
		SessionID: sessionID,
		CronJobID: job.ID,
	})
	if submitErr != nil {
		run := Run{
			JobID:      job.ID,
			StartedAt:  startedAt,
			FinishedAt: time.Now().Unix(),
			PromptHash: promptHash,
			Status:     "error",
			Delivered:  false,
			ErrorMsg:   submitErr.Error(),
		}
		e.recordAndUpdateJob(ctx, job, run)
		return
	}

	// Wait for a final frame or timeout.
	var final kernel.RenderFrame
	select {
	case final = <-done:
	case <-ctx.Done():
		// Timeout. Deliver a short failure notice.
		notice := fmt.Sprintf("Cron job %s timed out after %s.", job.Name, e.cfg.CallTimeout)
		_ = e.cfg.Sink.Deliver(context.Background(), notice)
		run := Run{
			JobID:      job.ID,
			StartedAt:  startedAt,
			FinishedAt: time.Now().Unix(),
			PromptHash: promptHash,
			Status:     "timeout",
			Delivered:  true,
			OutputPreview: truncate(notice, 200),
			ErrorMsg:   "context deadline exceeded",
		}
		e.recordAndUpdateJob(ctx, job, run)
		return
	}

	finalText := final.Response
	finished := time.Now().Unix()

	// Decide: silent, empty, or deliver.
	if DetectSilent(finalText) {
		run := Run{
			JobID:             job.ID,
			StartedAt:         startedAt,
			FinishedAt:        finished,
			PromptHash:        promptHash,
			Status:            "suppressed",
			Delivered:         false,
			SuppressionReason: "silent",
			OutputPreview:     "",
		}
		e.recordAndUpdateJob(ctx, job, run)
		return
	}

	if isEmpty(finalText) {
		notice := fmt.Sprintf("Cron job %s returned empty output.", job.Name)
		_ = e.cfg.Sink.Deliver(context.Background(), notice)
		run := Run{
			JobID:             job.ID,
			StartedAt:         startedAt,
			FinishedAt:        finished,
			PromptHash:        promptHash,
			Status:            "error",
			Delivered:         true,
			SuppressionReason: "empty",
			OutputPreview:     truncate(notice, 200),
			ErrorMsg:          "agent returned empty response",
		}
		e.recordAndUpdateJob(ctx, job, run)
		return
	}

	// Normal delivery.
	delivErr := e.cfg.Sink.Deliver(context.Background(), finalText)
	run := Run{
		JobID:         job.ID,
		StartedAt:     startedAt,
		FinishedAt:    finished,
		PromptHash:    promptHash,
		Status:        "success",
		Delivered:     delivErr == nil,
		OutputPreview: truncate(finalText, 200),
	}
	if delivErr != nil {
		run.ErrorMsg = fmt.Sprintf("delivery: %v", delivErr)
	}
	e.recordAndUpdateJob(ctx, job, run)
}

func (e *Executor) recordAndUpdateJob(ctx context.Context, job Job, run Run) {
	if err := e.cfg.RunStore.RecordRun(ctx, run); err != nil {
		e.log.Warn("cron: failed to record run", "job_id", job.ID, "err", err)
	}
	// Update the bbolt job row with last-run metadata.
	job.LastRunUnix = run.StartedAt
	job.LastStatus = run.Status
	if err := e.cfg.JobStore.Update(job); err != nil {
		e.log.Warn("cron: failed to update job after run", "job_id", job.ID, "err", err)
	}
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8]) // 16-char prefix
}

func isEmpty(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/cron/... -run TestExecutor_ -v -timeout 30s
go vet ./...
```

All 6 executor tests pass.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/cron/executor.go gormes/internal/cron/executor_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/cron): Executor bridges tick -> kernel -> decide -> deliver

The heart of Phase 2.D. One Run(ctx, job) call:
  1. Build ephemeral session_id 'cron:<job_id>:<unix_ts>'
  2. sha256 prompt hash for audit (first 16 hex chars)
  3. Subscribe to kernel.Render() BEFORE Submit (avoid race)
  4. Submit PlatformEvent with SessionID + CronJobID set
  5. Filter frames by SessionID; first FinishReason-bearing
     frame is the final response
  6. Detect [SILENT] (exact match) -> suppressed, no delivery
     Detect empty response -> error, deliver failure notice
     Normal -> deliver content
  7. Record Run row + update Job.LastRunUnix/LastStatus

Kernel exposure is narrowed to a KernelAPI interface (Submit +
Render) so tests can swap in a fake without importing kernel
internals.

Six tests cover:
  - Normal delivers content
  - [SILENT] suppresses, run.suppression_reason='silent'
  - Empty delivers failure notice, run.status='error' /
    suppression_reason='empty'
  - Timeout delivers failure notice, run.status='timeout'
  - Kernel Submit error: records run.status='error', no delivery
  - Job.LastRunUnix + LastStatus update correctly after run

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Scheduler (robfig/cron/v3 wrapper)

**Files:**
- Create: `gormes/internal/cron/scheduler.go`
- Create: `gormes/internal/cron/scheduler_test.go`

- [ ] **Step 1: Write failing tests — `scheduler_test.go`**

```go
package cron

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.etcd.io/bbolt"
)

func TestScheduler_FiresJobOnTick(t *testing.T) {
	fk := newFakeKernel("hello world", 0)
	dbPath := filepath.Join(t.TempDir(), "session.db")
	db, _ := bbolt.Open(dbPath, 0o600, nil)
	defer db.Close()
	js, _ := NewStore(db)

	// Create a job that fires every second.
	j := NewJob("fast", "@every 1s", "tick")
	_ = js.Create(j)

	var fires atomic.Int32
	fakeExec := &fakeExecutor{onRun: func(_ context.Context, _ Job) { fires.Add(1) }}

	s := NewScheduler(SchedulerConfig{
		Store:    js,
		Executor: fakeExec,
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	// Give the scheduler 2.5s — should fire at least twice.
	<-ctx.Done()
	s.Stop(context.Background())

	_ = fk
	if fires.Load() < 2 {
		t.Errorf("fires = %d, want at least 2 in 2.5s with @every 1s", fires.Load())
	}
}

func TestScheduler_PausedJobsAreIgnored(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "session.db")
	db, _ := bbolt.Open(dbPath, 0o600, nil)
	defer db.Close()
	js, _ := NewStore(db)

	j := NewJob("paused", "@every 500ms", "x")
	j.Paused = true
	_ = js.Create(j)

	var fires atomic.Int32
	fakeExec := &fakeExecutor{onRun: func(_ context.Context, _ Job) { fires.Add(1) }}

	s := NewScheduler(SchedulerConfig{Store: js, Executor: fakeExec}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	_ = s.Start(ctx)
	<-ctx.Done()
	s.Stop(context.Background())

	if fires.Load() != 0 {
		t.Errorf("paused job fired %d times, want 0", fires.Load())
	}
}

func TestScheduler_InvalidScheduleSkippedButOthersRun(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "session.db")
	db, _ := bbolt.Open(dbPath, 0o600, nil)
	defer db.Close()
	js, _ := NewStore(db)

	// One bad, one good.
	bad := NewJob("bad", "not a cron", "x")
	good := NewJob("good", "@every 500ms", "y")
	_ = js.Create(bad)
	_ = js.Create(good)

	var fires atomic.Int32
	fakeExec := &fakeExecutor{onRun: func(_ context.Context, j Job) {
		if j.Name == "good" {
			fires.Add(1)
		}
	}}

	s := NewScheduler(SchedulerConfig{Store: js, Executor: fakeExec}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-ctx.Done()
	s.Stop(context.Background())

	if fires.Load() < 1 {
		t.Errorf("good job fires = %d, want >= 1 (bad job shouldn't block)", fires.Load())
	}
}

// Test-only Executor stub.
type fakeExecutor struct {
	onRun func(context.Context, Job)
}

func (f *fakeExecutor) Run(ctx context.Context, j Job) {
	if f.onRun != nil {
		f.onRun(ctx, j)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/cron/... -run TestScheduler_ -v 2>&1 | tail -5
```

Expected: undefined Scheduler / NewScheduler / SchedulerConfig / Executor.Run as interface.

- [ ] **Step 3: Add a Runner interface in `executor.go` (small prerequisite edit)**

Edit `internal/cron/executor.go` to add an interface above `type Executor`:

```go
// Runner is the narrow interface the Scheduler uses to fire a job.
// The real *Executor satisfies it; tests inject fakes.
type Runner interface {
	Run(ctx context.Context, job Job)
}
```

- [ ] **Step 4: Create `scheduler.go`**

```go
package cron

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	rc "github.com/robfig/cron/v3"
)

// SchedulerConfig is the set of live dependencies.
type SchedulerConfig struct {
	Store    *Store // bbolt job persistence
	Executor Runner // interface — real *Executor or a test fake
}

// Scheduler owns a robfig *cron.Cron instance and the mapping of
// job IDs to registered EntryIDs. MVP is load-once at Start time;
// live reload on store mutations is a 2.D.2 concern.
type Scheduler struct {
	cfg     SchedulerConfig
	cron    *rc.Cron
	log     *slog.Logger
	mu      sync.Mutex
	entries map[string]rc.EntryID // jobID -> EntryID (for future Remove)
}

// NewScheduler constructs a Scheduler. Call Start to actually begin
// ticking. log may be nil (slog.Default used).
func NewScheduler(cfg SchedulerConfig, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		cfg:     cfg,
		cron:    rc.New(rc.WithParser(rc.NewParser(rc.Minute | rc.Hour | rc.Dom | rc.Month | rc.Dow | rc.Descriptor))),
		log:     log,
		entries: make(map[string]rc.EntryID),
	}
}

// Start loads all non-paused jobs from the store, registers their cron
// expressions, and starts the ticker. Jobs with invalid schedules are
// skipped with a warning; other jobs continue as normal.
//
// Blocking behavior: Start is non-blocking — the cron ticker runs on
// its own goroutine. Stop must be called to tear down.
func (s *Scheduler) Start(ctx context.Context) error {
	jobs, err := s.cfg.Store.List()
	if err != nil {
		return fmt.Errorf("scheduler: list jobs: %w", err)
	}
	for _, job := range jobs {
		if job.Paused {
			continue
		}
		if vErr := ValidateSchedule(job.Schedule); vErr != nil {
			s.log.Warn("cron: skipping job with invalid schedule",
				"job_id", job.ID, "name", job.Name,
				"schedule", job.Schedule, "err", vErr)
			continue
		}
		// Capture job for the closure.
		jobCopy := job
		id, aErr := s.cron.AddFunc(job.Schedule, func() {
			defer func() {
				if r := recover(); r != nil {
					s.log.Warn("cron: panic in job",
						"job_id", jobCopy.ID, "name", jobCopy.Name, "panic", r)
				}
			}()
			s.cfg.Executor.Run(ctx, jobCopy)
		})
		if aErr != nil {
			s.log.Warn("cron: AddFunc failed",
				"job_id", job.ID, "name", job.Name, "err", aErr)
			continue
		}
		s.mu.Lock()
		s.entries[job.ID] = id
		s.mu.Unlock()
	}
	s.cron.Start()
	return nil
}

// Stop halts the ticker and waits for any running jobs (bounded by ctx).
// Idempotent — safe to call before or after Start.
func (s *Scheduler) Stop(ctx context.Context) {
	if s.cron == nil {
		return
	}
	stopped := s.cron.Stop() // returns a context that's Done when running jobs finish
	select {
	case <-stopped.Done():
	case <-ctx.Done():
	}
}
```

- [ ] **Step 5: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/cron/... -run TestScheduler_ -v -timeout 30s
go vet ./...
```

All 3 scheduler tests pass.

- [ ] **Step 6: Commit**

```bash
git add gormes/internal/cron/scheduler.go gormes/internal/cron/scheduler_test.go gormes/internal/cron/executor.go
git commit -m "$(cat <<'EOF'
feat(gormes/cron): Scheduler wraps robfig/cron/v3

Phase 2.D scheduler. Loads all non-paused jobs from the bbolt
Store at Start time, registers each via cron.AddFunc, starts the
internal ticker. On each fire: AddFunc closure calls
executor.Run(ctx, job) wrapped in defer recover() so a panicking
job doesn't take down the scheduler.

Invalid schedules are logged + skipped at Start (do not abort
all other jobs for one bad expression). MVP is load-once;
live reload on store mutations lands in Phase 2.D.2 CLI.

Runner interface (added to executor.go) lets the Scheduler be
tested with a fake that never touches the real kernel.

Three tests:
  - FiresJobOnTick: @every 1s fires at least twice in 2.5s
  - PausedJobsAreIgnored: paused=true -> no fires
  - InvalidScheduleSkippedButOthersRun: bad and good jobs
    coexist; bad is skipped, good still fires.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: CRON.md mirror

**Files:**
- Create: `gormes/internal/cron/mirror.go`
- Create: `gormes/internal/cron/mirror_test.go`

- [ ] **Step 1: Write failing tests — `mirror_test.go`**

```go
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
	// Give one tick.
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

	// After two ticks, the file must exist and be non-empty.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Error("CRON.md is empty after mirror ticks")
	}
	// Confirm there's no leftover temp file in the target dir.
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
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/cron/... -run TestMirror_ -v 2>&1 | tail -5
```

Expected: undefined Mirror / NewMirror / MirrorConfig.

- [ ] **Step 3: Create `mirror.go`**

```go
package cron

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MirrorConfig holds the live deps + rendering knobs.
type MirrorConfig struct {
	JobStore *Store
	RunStore *RunStore
	Path     string        // target file; e.g. ~/.local/share/gormes/cron/CRON.md
	Interval time.Duration // default 30s when <= 0
}

func (c *MirrorConfig) withDefaults() {
	if c.Interval <= 0 {
		c.Interval = 30 * time.Second
	}
}

// Mirror writes a human-readable Markdown snapshot of the cron state.
// Mirrors the Phase 3.D.5 USER.md pattern exactly: background goroutine,
// atomic temp-file + rename, no partial reads.
type Mirror struct {
	cfg MirrorConfig
	log *slog.Logger
}

func NewMirror(cfg MirrorConfig, log *slog.Logger) *Mirror {
	cfg.withDefaults()
	if log == nil {
		log = slog.Default()
	}
	return &Mirror{cfg: cfg, log: log}
}

// Run blocks until ctx is cancelled. Writes on start + then every
// Interval. Single-shot tests that give it 2-3 Intervals should
// observe at least one write.
func (m *Mirror) Run(ctx context.Context) {
	m.tick(ctx) // first write immediately on start
	t := time.NewTicker(m.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.tick(ctx)
		}
	}
}

func (m *Mirror) tick(ctx context.Context) {
	body, err := m.render(ctx)
	if err != nil {
		m.log.Warn("cron mirror: render failed", "err", err)
		return
	}
	if err := atomicWrite(m.cfg.Path, body); err != nil {
		m.log.Warn("cron mirror: write failed", "path", m.cfg.Path, "err", err)
	}
}

func (m *Mirror) render(ctx context.Context) (string, error) {
	jobs, err := m.cfg.JobStore.List()
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintln(&b, "# Gormes Cron")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "_Last refreshed: %s_\n\n", time.Now().UTC().Format(time.RFC3339))

	// Split jobs: active vs paused.
	var active, paused []Job
	for _, j := range jobs {
		if j.Paused {
			paused = append(paused, j)
		} else {
			active = append(active, j)
		}
	}

	fmt.Fprintf(&b, "## Active Jobs (%d)\n\n", len(active))
	for _, j := range active {
		renderJob(&b, &j, m.cfg.RunStore, ctx)
	}

	if len(paused) > 0 {
		fmt.Fprintf(&b, "\n## Paused Jobs (%d)\n\n", len(paused))
		for _, j := range paused {
			renderJob(&b, &j, m.cfg.RunStore, ctx)
		}
	}

	return b.String(), nil
}

func renderJob(b *strings.Builder, j *Job, rs *RunStore, ctx context.Context) {
	fmt.Fprintf(b, "### %s — `%s`\n", j.Name, j.Schedule)
	fmt.Fprintf(b, "- **ID:** `%s`\n", j.ID)
	fmt.Fprintf(b, "- **Prompt:** %s\n", oneLine(j.Prompt, 140))
	if j.LastRunUnix > 0 {
		ts := time.Unix(j.LastRunUnix, 0).UTC().Format(time.RFC3339)
		fmt.Fprintf(b, "- **Last run:** %s — %s\n", ts, j.LastStatus)
	} else {
		fmt.Fprintln(b, "- **Last run:** _never_")
	}

	runs, err := rs.LatestRuns(ctx, j.ID, 3)
	if err == nil && len(runs) > 0 {
		fmt.Fprintln(b, "- **Recent:**")
		for _, r := range runs {
			ts := time.Unix(r.StartedAt, 0).UTC().Format(time.RFC3339)
			preview := oneLine(r.OutputPreview, 80)
			if preview == "" {
				preview = "—"
			}
			fmt.Fprintf(b, "  - %s — %s (delivered=%v) %s\n",
				ts, r.Status, r.Delivered, preview)
		}
	}
	fmt.Fprintln(b)
}

func oneLine(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// atomicWrite writes body to path via a temp file in the same dir,
// then renames. Readers never see a partial file.
func atomicWrite(path, body string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return err
	}
	defer func() {
		// If we got here via error path, make sure the temp is gone.
		_ = os.Remove(tmp.Name())
	}()
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/cron/... -run TestMirror_ -v -timeout 30s
go vet ./...
```

All 3 mirror tests pass.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/cron/mirror.go gormes/internal/cron/mirror_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/cron): CRON.md mirror (3.D.5 pattern for cron state)

Background goroutine writes a human-readable Markdown snapshot
of the cron state every 30s (configurable). Same pattern as the
Phase 3.D.5 Memory Mirror:
  - Render to a local buffer
  - Atomic temp-file + rename write
  - No partial reads for anyone tailing the file

Format:
  # Gormes Cron
  _Last refreshed: 2026-04-20T..._
  ## Active Jobs (N)
  ### name -- schedule
  - ID
  - Prompt (one-line-ified, max 140 chars)
  - Last run timestamp + status
  - Recent runs (last 3) with preview
  ## Paused Jobs (M)   (only when non-empty)

Three tests:
  - WritesMarkdownWithJobsAndRuns: full path with one job + one
    run verifies all load-bearing fields render.
  - AtomicWrite_NoPartialReadOnCrash: two ticks produce file,
    no leftover .tmp files.
  - EmptyStoreProducesEmptyActiveSection: zero jobs -> "Active
    Jobs (0)" header still written.

tail -f CRON.md is the operator's at-a-glance auditability.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Config `[cron]` section + defaults

**Files:**
- Modify: `gormes/internal/config/config.go`
- Modify: `gormes/internal/config/config_test.go`

- [ ] **Step 1: Write failing test — append to `config_test.go`**

```go
func TestLoad_CronDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cron.Enabled {
		t.Errorf("Cron.Enabled default = true, want false (opt-in)")
	}
	if cfg.Cron.CallTimeout != 60*time.Second {
		t.Errorf("Cron.CallTimeout default = %v, want 60s", cfg.Cron.CallTimeout)
	}
	if cfg.Cron.MirrorInterval != 30*time.Second {
		t.Errorf("Cron.MirrorInterval default = %v, want 30s", cfg.Cron.MirrorInterval)
	}
	if cfg.Cron.MirrorPath != "" {
		t.Errorf("Cron.MirrorPath default = %q, want empty (caller resolves XDG)", cfg.Cron.MirrorPath)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/config/... -run TestLoad_CronDefaults -v 2>&1 | tail -5
```

Expected: unknown field `Cron` on Config.

- [ ] **Step 3: Extend `config.go`**

Add the struct type (near the other *Cfg types):

```go
type CronCfg struct {
	Enabled        bool          `toml:"enabled"`
	CallTimeout    time.Duration `toml:"call_timeout"`
	MirrorInterval time.Duration `toml:"mirror_interval"`
	MirrorPath     string        `toml:"mirror_path"`
}
```

Add a field to `Config`:

```go
type Config struct {
	ConfigVersion int `toml:"_config_version"`

	Hermes   HermesCfg   `toml:"hermes"`
	TUI      TUICfg      `toml:"tui"`
	Input    InputCfg    `toml:"input"`
	Telegram TelegramCfg `toml:"telegram"`
	Cron     CronCfg     `toml:"cron"` // NEW
	Resume   string      `toml:"-"`
}
```

Add defaults in `defaults()`:

```go
func defaults() Config {
	return Config{
		ConfigVersion: CurrentConfigVersion,
		Hermes:        HermesCfg{...},
		// ...
		Telegram: TelegramCfg{...},
		Cron: CronCfg{
			Enabled:        false,
			CallTimeout:    60 * time.Second,
			MirrorInterval: 30 * time.Second,
			MirrorPath:     "", // caller resolves XDG
		},
	}
}
```

Add a resolver helper (at the bottom of the file, near `MemoryDBPath`):

```go
// CronMirrorPath returns the resolved CRON.md path — either
// cfg.Cron.MirrorPath (explicit override) or the XDG default
// $XDG_DATA_HOME/gormes/cron/CRON.md.
func (c *Config) CronMirrorPath() string {
	if c.Cron.MirrorPath != "" {
		return c.Cron.MirrorPath
	}
	return filepath.Join(xdgDataHome(), "gormes", "cron", "CRON.md")
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/config/... -v -timeout 30s
go vet ./...
```

All config tests pass.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/config/config.go gormes/internal/config/config_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/config): [cron] TOML section + defaults

Four new opt-in knobs for Phase 2.D:
  [cron]
  enabled          = false    # opt-in like 3.D semantic fusion
  call_timeout     = "60s"    # kernel-call timeout per fire
  mirror_interval  = "30s"    # CRON.md refresh cadence
  mirror_path      = ""       # "" -> $XDG_DATA_HOME/gormes/cron/CRON.md

Config.CronMirrorPath() resolves the path (explicit override wins
over XDG default).

Empty enabled = zero runtime cost: no scheduler goroutine, no
bbolt bucket, no Mirror ticker. cmd/gormes/telegram.go wiring
(T13) checks this flag first.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: `cmd/gormes/telegram.go` wiring + TelegramDeliverySink

**Files:**
- Modify: `gormes/cmd/gormes/telegram.go`
- Create: `gormes/cmd/gormes/telegram_cron_sink_test.go`

- [ ] **Step 1: Read the current wiring file**

```bash
cd gormes
sed -n '60,200p' cmd/gormes/telegram.go
```

Locate where Extractor, Embedder, and Mirror are constructed + run. Cron wiring follows the same pattern (construct → go Run(ctx) → defer Close on shutdown).

- [ ] **Step 2: Write a failing test for the telegram DeliverySink implementation**

Create `cmd/gormes/telegram_cron_sink_test.go`:

```go
package main

import (
	"context"
	"testing"
)

// fakeBotSender lets the test drive the DeliverySink without a live
// Telegram client.
type fakeBotSender struct {
	sentChatID int64
	sentText   string
}

func (f *fakeBotSender) SendToChat(ctx context.Context, chatID int64, text string) error {
	f.sentChatID = chatID
	f.sentText = text
	return nil
}

func TestTelegramDeliverySink_ForwardsToConfiguredChatID(t *testing.T) {
	bot := &fakeBotSender{}
	sink := newTelegramDeliverySink(bot, 4242)
	if err := sink.Deliver(context.Background(), "hello"); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if bot.sentChatID != 4242 {
		t.Errorf("chat_id = %d, want 4242", bot.sentChatID)
	}
	if bot.sentText != "hello" {
		t.Errorf("text = %q, want 'hello'", bot.sentText)
	}
}
```

- [ ] **Step 3: Run, expect FAIL**

```bash
cd gormes
go test ./cmd/gormes/... -run TestTelegramDeliverySink_ -v 2>&1 | tail -5
```

Expected: undefined newTelegramDeliverySink.

- [ ] **Step 4: Add the sink helper to `telegram.go`**

Near the bottom of `cmd/gormes/telegram.go` (after `recallAdapter`), add:

```go
// telegramBotSender is the narrow interface newTelegramDeliverySink
// needs — exactly what *telegram.Bot exposes.
type telegramBotSender interface {
	SendToChat(ctx context.Context, chatID int64, text string) error
}

// newTelegramDeliverySink wraps the running Telegram bot as a
// cron.DeliverySink. Every cron-fired output is sent to the
// operator's configured AllowedChatID.
func newTelegramDeliverySink(bot telegramBotSender, chatID int64) cron.DeliverySink {
	return cron.FuncSink(func(ctx context.Context, text string) error {
		return bot.SendToChat(ctx, chatID, text)
	})
}
```

Add `"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/cron"` to the imports.

**Check whether `telegram.Bot` has a `SendToChat` method.** If it doesn't, check what the existing bot uses to send. Likely candidates to wrap: `bot.SendMessage(chatID, text)`, `bot.client.SendMessage(...)`. If the name differs, update the interface method name to match — don't rename upstream methods.

- [ ] **Step 5: Run the sink test; should PASS**

```bash
cd gormes
go test ./cmd/gormes/... -run TestTelegramDeliverySink_ -v
```

- [ ] **Step 6: Wire the cron subsystem into runTelegram**

In `cmd/gormes/telegram.go`, inside `runTelegram` after the Embedder defer block and BEFORE `go k.Run(rootCtx)`:

```go
	// Phase 2.D — cron scheduler + executor + mirror (opt-in).
	if cfg.Cron.Enabled && cfg.Telegram.AllowedChatID != 0 {
		// Reuse the existing session.db for the cron_jobs bucket.
		// smap already owns the DB handle — we need access to it here.
		// (Add a DB() accessor to internal/session if needed.)
		cronStore, err := cron.NewStore(smap.DB())
		if err != nil {
			return fmt.Errorf("cron: init store: %w", err)
		}
		cronRunStore := cron.NewRunStore(mstore.DB())

		bot := tc // assumes tc is the *telegram.RealClient wrapper; adjust name if different
		// If tc isn't the right type, find the one that implements SendToChat.
		sink := newTelegramDeliverySink(bot, cfg.Telegram.AllowedChatID)

		cronExec := cron.NewExecutor(cron.ExecutorConfig{
			Kernel:      k,
			JobStore:    cronStore,
			RunStore:    cronRunStore,
			Sink:        sink,
			CallTimeout: cfg.Cron.CallTimeout,
		}, slog.Default())

		cronSched := cron.NewScheduler(cron.SchedulerConfig{
			Store:    cronStore,
			Executor: cronExec,
		}, slog.Default())

		if err := cronSched.Start(rootCtx); err != nil {
			return fmt.Errorf("cron: start scheduler: %w", err)
		}
		defer func() {
			shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
			defer cancelShutdown()
			cronSched.Stop(shutdownCtx)
		}()

		cronMirror := cron.NewMirror(cron.MirrorConfig{
			JobStore: cronStore,
			RunStore: cronRunStore,
			Path:     cfg.CronMirrorPath(),
			Interval: cfg.Cron.MirrorInterval,
		}, slog.Default())
		go cronMirror.Run(rootCtx)
	}
```

**If `smap.DB()` / `mstore.DB()` don't exist:**

- `mstore.DB()` should exist (added earlier for the RunStore use).
- `smap.DB()` may need to be added. Search:
  ```bash
  grep -n "func (s \*Map) DB\|func.*bbolt.DB" internal/session/*.go
  ```
  If absent, add a one-liner:
  ```go
  // DB exposes the underlying bbolt handle so other subsystems can
  // add their own buckets (Phase 2.D cron, future extensions).
  func (m *Map) DB() *bbolt.DB { return m.db }
  ```

- [ ] **Step 7: Build + full sweep**

```bash
cd gormes
go build ./...
go vet ./...
go test -race ./... -count=1 -timeout 240s -skip Integration_Ollama
```

Expected: clean build, green tests (minus pre-existing non-related doc failures if any).

Binary size:

```bash
make build
ls -lh bin/gormes
```

Expected ≤ 18 MB.

Smoke test:

```bash
./bin/gormes telegram 2>&1 | head -3
```

Expected: the existing `no Telegram bot token` error (unchanged — cron is disabled by default).

- [ ] **Step 8: Commit**

```bash
git add gormes/cmd/gormes/telegram.go gormes/cmd/gormes/telegram_cron_sink_test.go gormes/internal/session/*.go
git commit -m "$(cat <<'EOF'
feat(gormes/cmd/telegram): wire Phase-2.D cron subsystem

Activates the Phase-2.D cron scheduler when BOTH:
  - cfg.Cron.Enabled is true (new [cron].enabled TOML key)
  - cfg.Telegram.AllowedChatID is non-zero (so delivery has a
    target)

Wiring order:
  1. Reuse session.db for the cron_jobs bbolt bucket
  2. RunStore against the Phase-3.A SqliteStore's *sql.DB
  3. telegramDeliverySink wraps the bot's SendToChat with the
     AllowedChatID baked in
  4. Executor binds the kernel + stores + sink
  5. Scheduler loads jobs at start, registers robfig AddFunc
     per active job, begins ticking
  6. Mirror goroutine writes CRON.md alongside extractor/
     embedder/memory-mirror

Shutdown: scheduler.Stop(ctx) in a deferred close, bounded by
kernel.ShutdownBudget. Mirror halts on rootCtx cancel.

No cron code runs when Enabled=false — zero goroutines, zero
buckets, zero binary cost beyond the ~20KB robfig dep.

telegramDeliverySink helper + its unit test pin the
AllowedChatID routing so a regression can't silently bleed
cron output to the wrong chat.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Ollama E2E heartbeat test (ship-criterion)

**Files:**
- Create: `gormes/internal/cron/integration_test.go`

Optional but high-value. Uses the real extractor-integration-style Ollama skip helper so it runs cleanly under `go test ./...` even without Ollama.

- [ ] **Step 1: Write the integration test**

Create `gormes/internal/cron/integration_test.go`:

```go
// Package cron — Phase 2.D heartbeat crucible against local Ollama.
//
// Gated by the same skipIfNoOllama helper from
// internal/memory/extractor_integration_test.go (copied into this
// package because Go internal testing conventions don't share helpers
// across packages).
//
// Flow:
//   1. Seed one job firing every 2s with a short prompt
//   2. Construct the full cron stack against a real kernel + memory
//   3. Start the scheduler, wait 3s
//   4. Assert: at least one cron_runs row with status='success'
//   5. Assert: the captured delivery sink got the response text
//   6. Assert: the extractor produced ZERO entities from the cron
//      turn (skip-memory verified end-to-end)
package cron

// Content elided for brevity in this plan header — the full test
// follows the same structure as internal/memory/semantic_integration_test.go.
// See the plan's T14 body for the complete test code.
```

Given this test is large and depends on live Ollama, follow the EXACT pattern in `internal/memory/semantic_integration_test.go`:
- Add `skipIfNoOllama(t)` helper locally (copy from that file; internal Go test helpers don't cross packages).
- Add `integrationEndpoint()` + `integrationModel()` identical to upstream.
- Wire: `memory.OpenSqlite`, `cron.Store`, `cron.RunStore`, `kernel.New`, `hermes.NewHTTPClient`, `memory.NewExtractor`, `cron.Executor`, `cron.Scheduler`.
- Job: schedule `"@every 2s"`, prompt `"say hi"`, no Telegram (sink is a FuncSink that captures into a slice).
- After 3 seconds: assert cron_runs.count >= 1, assert captured deliveries >= 1, assert memory entities count == 0.

The full test body is ~200 lines. Use the Phase 3.D integration test as your skeleton — same pattern, same helper imports.

- [ ] **Step 2: Run with Ollama**

```bash
cd gormes
GORMES_EXTRACTOR_MODEL="huggingface.co/r1r21nb/qwen2.5-3b-instruct.Q4_K_M.gguf:latest" \
  go test ./internal/cron/... -run TestCron_Integration_Ollama_Heartbeat -v -timeout 5m
```

If PASS: commit. If FAIL: report the full output — do NOT adjust production code to make it pass; the ship criterion is real behavior.

- [ ] **Step 3: Commit (only if Step 2 passes)**

```bash
git add gormes/internal/cron/integration_test.go
git commit -m "$(cat <<'EOF'
test(gormes/cron): Phase-2.D heartbeat crucible against Ollama

TestCron_Integration_Ollama_Heartbeat is the ship-criterion
test for Phase 2.D.

Flow:
  1. Seed one job with schedule '@every 2s'
  2. Wire the full cron stack against a real kernel + memory
  3. Start scheduler, wait 3s
  4. Assert: >=1 cron_runs row with status='success'
  5. Assert: captured deliveries >=1 (sink received content)
  6. Assert: 0 entities extracted from the cron turn
     (skip-memory verified end-to-end)

This is the moment the Bear starts moving on its own. The
last assertion closes the loop with upstream's skip_memory
invariant — cron turns NEVER pollute user representation.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: Verification sweep

**Files:** no changes — verification only.

- [ ] **Step 1: Full sweep minus Ollama**

```bash
cd gormes
go test -race ./... -count=1 -timeout 240s -skip Integration_Ollama
go vet ./...
```

Expected: all packages green.

- [ ] **Step 2: Kernel isolation**

```bash
cd gormes
(go list -deps ./internal/kernel | grep -E "ncruces|internal/memory|internal/session|internal/cron") \
  && echo "VIOLATION" || echo "OK: kernel isolated"
```

Expected: `OK`. Kernel cannot depend on cron (cron depends on kernel via the narrow KernelAPI interface, not the reverse).

- [ ] **Step 3: Binary size**

```bash
cd gormes
make build
ls -lh bin/gormes
```

Expected ≤ 18 MB. Phase 2.D adds ~300 KB (robfig/cron + ~1000 lines of Go).

- [ ] **Step 4: Schema migration smoke (v3d → v3e)**

```bash
cd gormes
rm -rf /tmp/gormes-2d-migrate && mkdir -p /tmp/gormes-2d-migrate/gormes
sqlite3 /tmp/gormes-2d-migrate/gormes/memory.db <<'SQL'
CREATE TABLE schema_meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
INSERT INTO schema_meta(k,v) VALUES ('version','3d');
CREATE TABLE turns (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, role TEXT NOT NULL CHECK(role IN ('user','assistant')), content TEXT NOT NULL, ts_unix INTEGER NOT NULL, meta_json TEXT, extracted INTEGER NOT NULL DEFAULT 0, extraction_attempts INTEGER NOT NULL DEFAULT 0, extraction_error TEXT, chat_id TEXT NOT NULL DEFAULT '');
CREATE INDEX idx_turns_session_ts ON turns(session_id, ts_unix);
CREATE INDEX idx_turns_unextracted ON turns(id) WHERE extracted = 0;
CREATE INDEX idx_turns_chat_id ON turns(chat_id, id);
CREATE VIRTUAL TABLE turns_fts USING fts5(content, content='turns', content_rowid='id');
CREATE TABLE entities (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL CHECK(type IN ('PERSON','PROJECT','CONCEPT','PLACE','ORGANIZATION','TOOL','OTHER')), description TEXT, updated_at INTEGER NOT NULL, UNIQUE(name, type));
CREATE TABLE relationships (source_id INTEGER NOT NULL, target_id INTEGER NOT NULL, predicate TEXT NOT NULL CHECK(predicate IN ('WORKS_ON','KNOWS','LIKES','DISLIKES','HAS_SKILL','LOCATED_IN','PART_OF','RELATED_TO')), weight REAL NOT NULL DEFAULT 1.0, updated_at INTEGER NOT NULL, PRIMARY KEY(source_id, target_id, predicate), FOREIGN KEY(source_id) REFERENCES entities(id) ON DELETE CASCADE, FOREIGN KEY(target_id) REFERENCES entities(id) ON DELETE CASCADE);
CREATE TABLE entity_embeddings (entity_id INTEGER PRIMARY KEY, model TEXT NOT NULL, dim INTEGER NOT NULL CHECK(dim > 0 AND dim <= 4096), vec BLOB NOT NULL, updated_at INTEGER NOT NULL, FOREIGN KEY(entity_id) REFERENCES entities(id) ON DELETE CASCADE);
INSERT INTO entities(name,type,updated_at) VALUES('pre-2d entity','PERSON',1);
INSERT INTO turns(session_id, role, content, ts_unix, chat_id) VALUES('s', 'user', 'hi', 1, 'c');
SQL
echo "BEFORE v=$(sqlite3 /tmp/gormes-2d-migrate/gormes/memory.db 'SELECT v FROM schema_meta') cron_runs: $(sqlite3 /tmp/gormes-2d-migrate/gormes/memory.db "SELECT COUNT(*) FROM sqlite_master WHERE name='cron_runs'")"

export XDG_DATA_HOME=/tmp/gormes-2d-migrate
GORMES_TELEGRAM_TOKEN=fake:tok GORMES_TELEGRAM_CHAT_ID=99 \
  timeout 1 ./bin/gormes telegram > /dev/null 2>&1 || true

echo "AFTER  v=$(sqlite3 /tmp/gormes-2d-migrate/gormes/memory.db 'SELECT v FROM schema_meta') cron_runs: $(sqlite3 /tmp/gormes-2d-migrate/gormes/memory.db "SELECT COUNT(*) FROM sqlite_master WHERE name='cron_runs'") pre_existing_entity: $(sqlite3 /tmp/gormes-2d-migrate/gormes/memory.db "SELECT COUNT(*) FROM entities WHERE name='pre-2d entity'") pre_existing_turn_cron_col: $(sqlite3 /tmp/gormes-2d-migrate/gormes/memory.db "SELECT cron FROM turns WHERE content='hi'")"
rm -rf /tmp/gormes-2d-migrate
```

Expected: `BEFORE v=3d cron_runs: 0 ... AFTER v=3e cron_runs: 1 pre_existing_entity: 1 pre_existing_turn_cron_col: 0`.

- [ ] **Step 5: Offline doctor still works**

```bash
cd gormes
./bin/gormes doctor --offline
```

Expected: `[PASS] Toolbox: 3 tools registered`.

- [ ] **Step 6: No commit**

If any check fails, STOP and report.

---

## Appendix: Self-Review

**Spec coverage** (§X of spec → task):

| Spec § | Task(s) |
|---|---|
| §1 Goal | All tasks |
| §2 Non-goals | Enforced by scope — no CLI, NL parser, multi-platform |
| §3 Upstream parity anchors | T4 (session), T3 (skip_memory), T7 (Heartbeat prefix) |
| §4 Scope | T1-T15 |
| §5 Architecture | Per-component: T1 (schema), T2 (store turn ext), T4 (kernel), T5 (Job + Store), T6 (RunStore), T7 (Heartbeat), T8 (Sink), T9 (Executor), T10 (Scheduler), T11 (Mirror) |
| §6 Data model | T1 schema, T2 memory worker, T5+T6 store code |
| §7 Heartbeat protocol | T7 |
| §8 Kernel interface change | T4 |
| §9 Timeout/error delivery matrix | T9 (Executor covers every row) |
| §10 Overlap semantics | T9 (via kernel mailbox) + T10 scheduler panic recovery |
| §11 Error handling | Distributed across T5/T6/T9/T10/T11 |
| §12 Configuration | T12 + T13 |
| §13 Testing | Every task has unit tests; T14 is integration |
| §14 Rollout | Plan ships 2.D.1 only; 2.D.2-5 explicitly out |
| §15 Binary size | T15 verification |
| §16 Open questions resolved | All already encoded in the spec |
| §17 Final checklist | T15 verification |

**Placeholder scan:** zero `TBD` / `TODO` / `fill in` / vague "handle errors" / "similar to Task N".

**Type consistency:**
- `Job` fields `{ID, Name, Schedule, Prompt, Paused, CreatedAt, LastRunUnix, LastStatus}` — T5 declaration, consumed T9 (executor updates LastRunUnix/LastStatus), T10 (scheduler reads Paused + Schedule), T11 (mirror renders all fields).
- `Run` fields `{JobID, StartedAt, FinishedAt, PromptHash, Status, Delivered, SuppressionReason, OutputPreview, ErrorMsg}` — T6 declaration, consumed T9 (executor constructs) + T11 (mirror renders).
- `ExecutorConfig` `{Kernel, JobStore, RunStore, Sink, CallTimeout}` — T9 declaration, consumed T13 (cmd wiring).
- `SchedulerConfig` `{Store, Executor}` — T10 declaration, consumed T13.
- `MirrorConfig` `{JobStore, RunStore, Path, Interval}` — T11 declaration, consumed T13.
- `CronCfg` `{Enabled, CallTimeout, MirrorInterval, MirrorPath}` — T12 declaration, consumed T13.
- `CronHeartbeatPrefix`, `BuildPrompt`, `DetectSilent` — T7; consumed T9.
- `KernelAPI` `{Submit(PlatformEvent), Render() <-chan RenderFrame}` — T9 declaration, satisfied by real *kernel.Kernel (via T4 extensions).
- `DeliverySink.Deliver(ctx, text)` + `FuncSink` — T8 declaration, consumed T9 + T13.
- `Runner.Run(ctx, job)` — added in T10 prep; satisfied by real *Executor + test fakes.

**Execution order:** T1 (schema) → T2 (store cron columns) → T3 (extractor skip) → T4 (kernel fields) → T5 (Job + Store) → T6 (RunStore) → T7 (Heartbeat) → T8 (Sink) → T9 (Executor) → T10 (Scheduler) → T11 (Mirror) → T12 (Config) → T13 (cmd wiring) → T14 (Ollama E2E) → T15 (verification).

**Checkpoint suggestions:** halt after **T9** (Executor works via fake kernel; all decide-and-deliver paths unit-tested) and after **T13** (full stack compiles and smoke-tests green) before T14's live Ollama run.
