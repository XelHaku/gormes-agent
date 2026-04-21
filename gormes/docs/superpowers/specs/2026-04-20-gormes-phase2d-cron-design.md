# Gormes Phase 2.D — Cron (Proactive Heartbeat) Design Spec

**Date:** 2026-04-20
**Phase:** 2.D — Proactive Scheduled Automations
**Upstream reference:** `cron/scheduler.py` (1091 lines) + `cron/jobs.py` (768 lines) + `tools/cronjob_tools.py`
**Status:** Approved for implementation plan

---

## 1. Goal

Make the Gormes bot **proactive** instead of purely reactive. The operator defines scheduled jobs (cron expression + prompt). At each scheduled time, Gormes wakes up, runs the prompt through the full agent loop (memory recall, entity graph, tool registry), and delivers the result via the existing Telegram adapter — unless the agent suppresses delivery with the `[SILENT]` control token.

**Ship criterion:** operator manually inserts a row into the `cron_jobs` bbolt bucket with schedule `0 8 * * *` and prompt `"Give me a status summary"`. At 08:00 local time, the Telegram bot posts the agent's report to the configured chat. No CLI required for MVP.

## 2. Non-Goals

- **Natural-language schedule parsing** (`"every day at 8am"` → `0 8 * * *`). Agent-tool territory; defer to Phase 4.C.
- **`gormes cron` CLI subcommand** (create/list/pause/remove). Phase 2.D.2.
- **Multi-platform delivery routing.** Only Telegram is a live adapter. Discord/Slack adapters land in 2.B.2+.
- **Per-job skill injection** (upstream's `skill_view` preload). Phase 5.F.
- **Per-job `quiet_on_failure` flag.** Future enhancement; MVP always notifies on timeout/error.
- **`[SILENT]` rate-limit guards.** If a job is silent 100×/day, MVP just records it. Phase 2.D.3.

## 3. Upstream Parity Anchors

The design ports upstream behavior verbatim at these three load-bearing points:

| Upstream invariant | Rationale | Gormes equivalent |
|---|---|---|
| `session_id = cron_<job_id>_<timestamp>` per run | Isolates cron execution context; avoids sliding-window contamination with live chat | `cron:<job_id>:<unix_ts>` injected via new `PlatformEvent.SessionID` field |
| `skip_memory=True` for cron runs | Quote: *"Cron system prompts would corrupt user representations"* | New `cron=1` column on `turns`; extractor `WHERE cron = 0` excludes cron turns |
| `[SYSTEM: ... DELIVERY: ... SILENT: ...]` prompt prefix | Tells the LLM delivery is automatic, reserves `[SILENT]` as suppression token | Exact string copy; see §7 |

Upstream divergences from the thin-slice MVP (acceptable for now):
- No `skill_view` preload — ported with Phase 5.F.
- No file-based tick lock — single-process, not needed.
- No data-collection script injection (`run.sh` before prompt) — future enhancement.

## 4. Scope

| In | Out |
|---|---|
| bbolt `cron_jobs` bucket (job definitions) | CLI subcommand for job CRUD |
| SQLite `cron_runs` table (per-run audit) | Natural-language schedule parser |
| `internal/cron` package with `robfig/cron/v3` scheduler | Per-job retries on error |
| Kernel `PlatformEvent.SessionID` + `CronJobID` override fields | Multi-platform delivery |
| Heartbeat `[SYSTEM: ...]` prefix + `[SILENT]` exact-match detection | `quiet_on_failure` flag |
| `DeliverySink` interface (generic, not Telegram-specific) | Sub-minute scheduling precision |
| CRON.md mirror (3.D.5 pattern, derived from `cron_runs`) | Per-run output Markdown files |
| Timeout delivery: short failure notice to Telegram | Per-job skill injection |
| `turns` table extension: `cron`, `cron_job_id` columns | Audit log rate limits |

## 5. Architecture

Five components, all inside the `gormes telegram` subcommand process (where the kernel lives):

```
                   ┌──────────────────────────────────────┐
                   │  telegram subcommand process          │
                   │                                       │
  bbolt           ┌┴─────────────┐     ┌────────────────┐ │
  cron_jobs ────► │ Job Store    │ ──► │  Scheduler     │ │
  bucket          │ (CRUD)       │     │ (robfig/cron)  │ │
                  └──────────────┘     └───────┬────────┘ │
                                               │ fires    │
                                               ▼          │
                  ┌──────────────┐    ┌─────────────────┐ │
                  │ Kernel       │ ◄── │  Executor      │ │
                  │ Submit       │     │ Heartbeat +    │ │
                  │  (+session   │     │ [SILENT] gate  │ │
                  │   override)  │     └───┬─────────┬──┘ │
                  └──┬───────────┘         │ run     │    │
                     │                     │ record  │    │
                     │ turn+frame          │         │    │
  SQLite             ▼                     ▼         │    │
  memory.db    ┌──────────────┐    ┌──────────────┐ │    │
  cron=1       │ finalizeStore│    │ cron_runs    │ │    │
               └──────────────┘    │ table        │ │    │
                                   └──────┬───────┘ │    │
                                          │         │    │
                                          ▼         ▼    │
                                   ┌──────────────┐ ┌────┴─────┐
                                   │ CRON.md      │ │ Delivery │
                                   │ mirror (30s) │ │ Sink (if │
                                   └──────────────┘ │ not      │
                                                    │ silent)  │
                                                    └──────────┘
                                                          │
                                                          ▼
                                                  Telegram bot
                                                  (allowed_chat_id)
```

### 5.1 Job Store (`internal/cron/store.go`)

New bbolt bucket `cron_jobs` in the existing `session.db`. Key = job ID (ULID string). Value = JSON blob:

```go
type Job struct {
    ID           string `json:"id"`             // ULID
    Name         string `json:"name"`           // operator-friendly label; unique
    Schedule     string `json:"schedule"`       // cron expression ("0 8 * * *")
    Prompt       string `json:"prompt"`         // user-facing prompt, NO [SYSTEM:] prefix here
    Paused       bool   `json:"paused"`
    CreatedAt    int64  `json:"created_at"`     // unix seconds
    LastRunUnix  int64  `json:"last_run_unix"`  // 0 if never run
    LastStatus   string `json:"last_status"`    // "success"|"timeout"|"error"|"suppressed"|"" (never run)
}
```

API (package-private for MVP; CLI lands in 2.D.2):
```go
func (s *Store) Create(job Job) error
func (s *Store) Get(id string) (Job, error)
func (s *Store) List() ([]Job, error)
func (s *Store) Update(job Job) error
func (s *Store) Delete(id string) error
```

All operations transactional (`bolt.Tx`). No CLI plumbing in this PR — operator inserts jobs by hand until 2.D.2 ships.

### 5.2 Scheduler (`internal/cron/scheduler.go`)

Wraps `robfig/cron/v3`. On startup: loads every `Paused=false` job from the store, registers an `AddFunc` per job. On job fire: the `AddFunc` closure calls `executor.Run(ctx, job)`.

- Library: `github.com/robfig/cron/v3` (pure Go, ~20KB to binary). Supports standard Unix cron syntax + `@every 1h` / `@daily` shortcuts.
- Lifecycle: `Start(ctx)` spawns the scheduler goroutine, `Stop(ctx)` waits for running jobs up to the shutdown budget.
- Hot reload: MVP is load-once at startup. Job edits require a process restart. (2.D.2 will add live reload.)
- Panic isolation: each `AddFunc` wrapped in `defer recover()` that logs and records the job as `status=error, error_msg=panic: ...`.

### 5.3 Executor (`internal/cron/executor.go`)

The bridge from scheduler tick → kernel → delivery decision → record. One method:

```go
func (e *Executor) Run(ctx context.Context, job Job)
```

Steps (per-fire):

1. **Build synthetic session ID:** `sid := fmt.Sprintf("cron:%s:%d", job.ID, time.Now().Unix())`.
2. **Prepend Heartbeat prefix:** `fullPrompt := cronHeartbeatPrefix + job.Prompt` (see §7).
3. **Hash original prompt:** `h := sha256.Sum256([]byte(job.Prompt))`; store hex prefix in `cron_runs.prompt_hash`.
4. **Open a one-shot collector** on `kernel.Render()` for this session_id (`collector := e.collectFinal(sid, ctx)`).
5. **Submit to kernel:** `k.Submit(PlatformEvent{Kind: PlatformEventSubmit, Text: fullPrompt, SessionID: sid, CronJobID: job.ID})`.
6. **Wait** for final response (with configurable timeout, default 60s). The collector reads the `k.Render()` stream and captures the final assistant text for this session_id.
7. **Detect `[SILENT]`** (exact match, see §7.2).
8. **Write `cron_runs` row** with full outcome (status, delivered, suppression_reason, output_preview).
9. **Update `cron_jobs.LastRunUnix + LastStatus`** in bbolt.
10. **Deliver unless suppressed** — call `sink.Deliver(ctx, finalText)` or skip.

### 5.4 Delivery Sink (`internal/cron/sink.go`)

Generic interface — kernel + cron package know nothing about Telegram:

```go
type DeliverySink interface {
    // Deliver pushes text to the operator's primary inbox. Implementations
    // are responsible for their own rate-limiting and retries. A failure
    // return is recorded in cron_runs.delivery_status but does not retry.
    Deliver(ctx context.Context, text string) error
}
```

Telegram implementation (in `cmd/gormes/telegram.go`): wraps the existing `telegram.Bot.SendToChat(chatID, text)` call. One implementation today; future Slack/Discord drop in without touching `internal/cron`.

### 5.5 CRON.md Mirror (`internal/cron/mirror.go`)

Background goroutine mirroring the 3.D.5 USER.md pattern. Every 30s (configurable): read `cron_jobs` + last 50 `cron_runs`, render Markdown, write atomically to `~/.local/share/gormes/cron/CRON.md` (via temp-file + rename).

Format:
```markdown
# Gormes Cron

_Last refreshed: 2026-04-20T08:15:00Z_

## Active Jobs (2)

### morning-report — `0 8 * * *`
- **ID:** `01JXXX...`
- **Prompt:** Give me a summary of RF anomalies in Springfield from the last 24h
- **Next run:** 2026-04-21T08:00:00Z
- **Last run:** 2026-04-20T08:00:00Z — success (delivered, 142 chars)

### backup-check — `0 3 * * *`
- **ID:** `01JYYY...`
- **Prompt:** Run the nightly backup check tool
- **Next run:** 2026-04-21T03:00:00Z
- **Last run:** 2026-04-20T03:00:00Z — suppressed (silent)

## Recent Runs (last 10)

| Started | Job | Status | Delivered | Preview |
|---|---|---|---|---|
| 2026-04-20T08:00:00Z | morning-report | success | yes | No anomalies detected in the last 24h. Systems nominal. |
| 2026-04-20T03:00:00Z | backup-check | suppressed | no | — |
| 2026-04-19T08:00:00Z | morning-report | success | yes | 2 anomalies: Tower-7 RSSI dropped 18dB at 04:12Z … |
```

`tail -f CRON.md` is the operator's at-a-glance auditability. No JSON, no bbolt spelunking.

## 6. Data Model

### 6.1 bbolt

New bucket `cron_jobs` in the existing `session.db` (alongside the Phase 2.C `sessions` bucket). bbolt auto-creates on first write; no migration needed.

### 6.2 SQLite schema v3e

Migration `migration3dTo3e`:

```sql
ALTER TABLE turns ADD COLUMN cron INTEGER NOT NULL DEFAULT 0;
ALTER TABLE turns ADD COLUMN cron_job_id TEXT;

CREATE TABLE IF NOT EXISTS cron_runs (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id              TEXT    NOT NULL,
    started_at          INTEGER NOT NULL,
    finished_at         INTEGER,
    prompt_hash         TEXT    NOT NULL,
    status              TEXT    NOT NULL CHECK(status IN ('success','timeout','error','suppressed')),
    delivered           INTEGER NOT NULL DEFAULT 0 CHECK(delivered IN (0,1)),
    suppression_reason  TEXT    CHECK(suppression_reason IS NULL OR suppression_reason IN ('silent','empty')),
    output_preview      TEXT,
    error_msg           TEXT
);
CREATE INDEX IF NOT EXISTS idx_cron_runs_job_started
    ON cron_runs(job_id, started_at DESC);

UPDATE schema_meta SET v = '3e' WHERE k = 'version' AND v = '3d';
```

**Extractor change** (`internal/memory/extractor.go`): the `pollMissing` query adds `AND t.cron = 0`. One-word diff to an existing SQL string. USER.md mirror is untouched because it reads entities, not turns directly.

**`finalizeStore`** path in the kernel writes `cron=1, cron_job_id=<event.CronJobID>` to the turn row when the `PlatformEvent` had those fields. Zero cost when both fields are empty (normal case).

## 7. Heartbeat Protocol

### 7.1 Prompt prefix

Verbatim port of upstream `cron/scheduler.py:_build_job_prompt`'s `cron_hint`:

```
[SYSTEM: You are running as a scheduled cron job. DELIVERY: Your final
response will be automatically delivered to the user — do NOT use
send_message or try to deliver the output yourself. Just produce your
report/output as your final response and the system handles the rest.
SILENT: If there is genuinely nothing new to report, respond with exactly
"[SILENT]" (nothing else) to suppress delivery. Never combine [SILENT]
with content — either report your findings normally, or say [SILENT]
and nothing more.]

<job.Prompt>
```

Stored as a `const cronHeartbeatPrefix = "..."` in `internal/cron/executor.go`. Verbatim-match tested against upstream's exact bytes to catch drift on future Hermes bumps.

### 7.2 `[SILENT]` detection (exact match)

```go
normalized := strings.TrimSpace(finalResponse)
suppress := normalized == "[SILENT]"
```

**Must suppress:**
- `"[SILENT]"`
- `"  [SILENT]\n"` (leading/trailing whitespace)
- `"\n[SILENT]\n\n"` (any surrounding whitespace)

**Must NOT suppress:**
- `"[SILENT] ... extra content"` (substring — not exact match)
- `"Status: [SILENT] means nothing to report"` (LLM explaining the token)
- `"<silent>"` or `"silent"` (wrong casing/tokens)
- `""` (empty string — tracked separately as `suppression_reason='empty'` but per §9, empty responses are still delivered as a failure notice)

### 7.3 Semantic-empty handling

Separate from `[SILENT]`. If the normalized final response is `""` (the LLM returned nothing), `status='error'` + `suppression_reason='empty'` + delivered=true with message `"Cron job <name> returned empty output."`. This is an *error* path, not a suppression path — the operator needs to know the job misbehaved.

## 8. Kernel Interface Change

**One struct, two new fields.** In `internal/kernel/frame.go`:

```go
type PlatformEvent struct {
    Kind      PlatformEventKind
    Text      string
    SessionID string // NEW: per-event session override; empty = use kernel's current sessionID
    CronJobID string // NEW: propagates to turn row's cron_job_id column via finalizeStore
    ack       chan<- struct{}
}
```

**Semantics:**
- `SessionID == ""` → current behavior preserved (use whatever the kernel has).
- `SessionID != ""` → the turn is processed against that session ID. **Does not mutate the kernel's resident sessionID** — per-event override only. Next non-cron event reverts to whatever the kernel had before.
- `CronJobID != ""` → when the turn is persisted via `finalizeStore`, the store writes `cron=1, cron_job_id=<v>`. When empty, both columns default to `0`/`NULL`.

**Kernel isolation invariant preserved:** the kernel still doesn't import anything from `internal/memory` or `internal/session`. The two new fields are opaque strings passed through to the store. Existing `TestKernelHasNoMemoryDep` / `TestKernelHasNoSessionDep` isolation tests still pass. New test: `TestKernel_SessionIDOverrideDoesNotLeakToNextTurn`.

**Store interface** (`internal/store/store.go`) gains two optional fields on the `Turn` struct (`Cron bool`, `CronJobID string`) with SQL writes gated on the struct's own Cron field. Zero change to existing turn writes from telegram/tui (they leave the fields zero).

## 9. Timeout & Error Delivery Policy

| Scenario | `status` | `delivered` | Delivery content |
|---|---|---|---|
| Agent returned normal text | `success` | `true` | The response |
| Agent returned `[SILENT]` | `suppressed` | `false` | Nothing |
| Agent returned `""` (empty) | `error` | `true` | `Cron job <name> returned empty output.` |
| Kernel timeout (> `CallTimeout`) | `timeout` | `true` | `Cron job <name> timed out after <N>s.` |
| Kernel `Submit()` returned error (mailbox full, ctx cancelled) | `error` | `false` | Nothing (kernel broken; better to be quiet) |
| Executor panic caught | `error` | `true` | `Cron job <name> failed: internal error. See logs.` |
| Delivery sink returned error | (status preserved) | `false` + `delivery_status="failed: <reason>"` | N/A (we tried) |

**Principle:** silent failures are worse than noisy failures for automation (Tech Lead quote). Default to a short failure notice; reserve quiet-mode as a future per-job flag.

## 10. Overlap Semantics

Cron jobs queue through the kernel's existing `PlatformEventMailboxCap=16` buffer. Rules:

- Two jobs firing at the same instant: both `k.Submit(...)` succeed, one blocks briefly until the kernel picks up the first.
- Live operator chat is mid-turn: cron job waits behind it in the mailbox. The operator never sees cron-generated intermediate chunks leak into their conversation because the executor collects output on `k.Render()` scoped to the cron session ID.
- Mailbox full (kernel catastrophically stuck): `Submit` returns error. Cron records `status=error, error_msg=kernel mailbox full` and emits the failure notice.

No retries in MVP. A timed-out or errored run is final; next tick schedules normally.

## 11. Error Handling Summary

| Failure class | Action |
|---|---|
| Invalid cron expression at job load time | Log warning; skip that job; other jobs unaffected |
| `robfig/cron/v3.AddFunc` panic | `defer recover()`; record as `status=error`; scheduler continues |
| bbolt write failure (mid-execute) | Log warning; proceed with in-memory job copy; next restart picks up whatever landed |
| SQLite write failure for `cron_runs` | Log warning; proceed with delivery decision (we'd rather miss a log entry than skip delivery) |
| CRON.md write failure | Log; retry on next 30s tick |
| Delivery sink failure | Record `delivery_status="failed: <reason>"` in the existing run row; no retry |

## 12. Configuration

New `[cron]` TOML section in `config.toml`:

```toml
[cron]
enabled = false                    # MVP default; opt-in like 3.D
call_timeout = "60s"               # per-job kernel submit timeout
mirror_interval = "30s"            # CRON.md refresh cadence
mirror_path = ""                   # "" → $XDG_DATA_HOME/gormes/cron/CRON.md
```

Go struct in `internal/config/config.go`:
```go
type CronCfg struct {
    Enabled        bool          `toml:"enabled"`
    CallTimeout    time.Duration `toml:"call_timeout"`
    MirrorInterval time.Duration `toml:"mirror_interval"`
    MirrorPath     string        `toml:"mirror_path"`
}
```

Wired from `cmd/gormes/telegram.go` alongside the Extractor/Embedder/Mirror. `Enabled=false` is a complete no-op: no scheduler goroutine, no mirror, no bbolt bucket creation.

## 13. Testing

### Unit (pure logic, no I/O)
- `TestHeartbeatPrefix_ExactByteMatchUpstream` — verbatim bytes vs a test fixture.
- `TestSilentDetection_ExactMatchOnly` — `"[SILENT]"`, `"  [SILENT]\n"`, `"[SILENT] extra"`, `"silent"`, `""`, all covered.
- `TestJob_ScheduleValidation` — bad cron expression rejected at create time.

### Component (real bbolt + real SQLite)
- `TestStore_CRUDRoundTrip` — create/get/list/update/delete a job.
- `TestMigrate_3dTo3e_AddsCronColumnsAndTable` — schema migration from 3d.
- `TestExtractor_SkipsCronTurns` — seed a normal turn + a cron turn; extractor processes only the normal one.

### Integration (real kernel)
- `TestKernel_SessionIDOverrideDoesNotLeakToNextTurn` — submit cron event w/ sessionID, submit a non-cron event next, verify the second used the kernel's original session.
- `TestExecutor_SilentSuppresses` — mock kernel returns `"[SILENT]"`, verify no sink.Deliver call, verify `cron_runs` row has `status='suppressed', delivered=0, suppression_reason='silent'`.
- `TestExecutor_NormalDelivers` — mock kernel returns text, verify sink.Deliver called with that text, `status='success', delivered=1`.
- `TestExecutor_TimeoutDeliversFailureNotice` — mock kernel never replies, verify sink.Deliver called with `"Cron job ... timed out after 60s."`.
- `TestExecutor_EmptyDeliversEmptyNotice` — mock kernel returns `""`, verify failure notice delivered, `suppression_reason='empty'`.

### E2E (Ollama-backed, skipped without live Ollama)
- `TestCron_Integration_Ollama_Heartbeat` — register a job with schedule `@every 2s`, wait 3s, verify a `cron_runs` row with `status='success'` and the `DeliverySink` received a non-empty string. Optionally assert extractor populated 0 entities from the cron turn (skip-memory verified end-to-end).

### Ship criterion test
- Manual: insert `{id: "test-job", name: "morning", schedule: "0 8 * * *", prompt: "status"}` into bbolt. Run `gormes telegram`. At 08:00, verify message lands in Telegram.

## 14. Rollout

1. **Phase 2.D.1 (this spec)**: scheduler + executor + Heartbeat + CRON.md mirror + kernel override, opt-in via `[cron].enabled=true`. Manual bbolt insertion for job CRUD.
2. **Phase 2.D.2 (future)**: `gormes cron` CLI subcommand (`list`, `create <schedule> <prompt>`, `pause <id>`, `resume <id>`, `remove <id>`, `status`, `tick` for manual test-fire). Live reload on job edits.
3. **Phase 2.D.3 (future)**: rate-limit guards (alert if silent-rate > threshold), per-job `quiet_on_failure` flag, retry policy.
4. **Phase 2.D.4 (blocked on 2.B.2+)**: multi-platform delivery via the same `DeliverySink` interface — Slack/Discord adapters drop in.
5. **Phase 2.D.5 (blocked on Phase 4.C)**: natural-language schedule parsing via the agent tool (`"every day at 8am"` → `0 8 * * *`).

## 15. Binary Size Impact

`robfig/cron/v3` is ~20 KB to the binary. New Gormes code ~1000 lines. Total expected growth < 300 KB on top of the current 17 MB. Well within the 25 MB hard moat.

## 16. Open Questions Resolved (from brainstorm)

| Question | Resolution |
|---|---|
| Shared vs dedicated session? | Dedicated per-run `cron:<job_id>:<unix_ts>` (upstream parity) |
| `skip_memory` mechanism? | `cron=1` column + extractor filter `WHERE cron = 0` |
| Transparency: CRON.md or `cron_runs` table? | **Both** — table is source of truth, CRON.md is derived mirror (3.D.5 pattern) |
| `[SILENT]` detection semantics? | **Exact match** after `TrimSpace`; never substring |
| Timeout delivery policy? | Short failure notice by default; `quiet_on_failure` deferred |
| Delivery interface boundary? | Generic `DeliverySink` interface; kernel stays platform-agnostic |
| Overlap? | Kernel mailbox queues; no per-job lock needed |

---

## 17. Final Architecture Checklist

- [x] Per-run ephemeral session IDs (`cron:<id>:<ts>`)
- [x] `cron=1` tagging on turns + extractor skip
- [x] CRON.md mirror separate from USER.md
- [x] Session override that doesn't leak to next turn
- [x] Overlap via kernel mailbox, not file lock
- [x] No Telegram-specific code in kernel or `internal/cron`
- [x] Generic `DeliverySink` interface for future Slack/Discord
- [x] `[SILENT]` = exact match after TrimSpace
- [x] `cron_runs` has explicit `status`/`delivered`/`suppression_reason`/`output_preview`
- [x] Timeout → short failure notice (noisy-by-default)
- [x] Verbatim Heartbeat prefix from upstream

**Ready for implementation plan.**
