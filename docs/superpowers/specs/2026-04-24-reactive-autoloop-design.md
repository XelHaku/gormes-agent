# Reactive Autoloop Design

**Status:** Draft
**Author:** Codex (Claude Opus 4.7 1M)
**Date:** 2026-04-24

## Context

Autoloop (`cmd/autoloop`, `internal/autoloop`) is the multi-worker coordinator
that drives the building-gormes roadmap forward by selecting rows from
`docs/content/building-gormes/architecture_plan/progress.json`, spawning worker
agents (codexu/claudeu/opencode) in isolated git worktrees, and promoting
successful workers via the existing `promote.go` path. Architecture-planner-loop
(`cmd/architecture-planner-loop`, `internal/architectureplanner`) reshapes
`progress.json` rows when execution feedback (via `runs.jsonl` and
`SummarizeAutoloopAudit`) shows that contracts need refinement.

Recent run telemetry under `.codex/orchestrator/state/runs.jsonl` shows five
recurring effectiveness gaps:

1. **Workers do real work then trip on report parsing.** Multiple workers in
   run `20260424T140233Z` were marked `report_validation_failed` despite
   producing commits. The strict `ParseFinalReport` schema rejects
   slightly-malformed reports even when the underlying work succeeded.
2. **Total wipeout runs.** Run `20260424T140153Z-…-003` produced 8 worker
   errors with 0 promotions — a single backend hiccup wiped a full run because
   the loop has no way to degrade.
3. **The same row gets retried indefinitely.** Subphase `2.B.6` is claimed
   repeatedly across single-worker runs; nothing tells autoloop "stop trying,
   ask the planner to reshape this."
4. **Loop pauses on per-row parse hiccups.** A `progress_summary_failed` event
   for one row halts the entire loop instead of skipping that row.
5. **Planner has audit feedback but autoloop doesn't read planner state.**
   `architectureplanner/autoloop_audit.go` reads autoloop's ledger; the
   reverse direction does not exist. Workers retry rows the planner already
   knows are toxic.

The system has dense telemetry (ledger, candidate snapshots, failure records,
planner audit) but the **reactive intelligence is thin** — autoloop is
write-once-per-event but never *acts* on its own history.

## Goals

1. Stop wasting compute on rows that have failed repeatedly.
2. Salvage worker output that is correct-but-malformed.
3. Make a single bad backend afternoon recoverable instead of fatal.
4. Give the planner a clear "fix these" signal instead of a free-form audit.
5. Ship as five independently-shippable layers so each can be reverted in
   isolation if it regresses.

## Non-goals

- Planner cadence / runtime improvements (Phase C, separate spec).
- Telemetry surfaces (`gormes autoloop status` dashboard) — value-add, not
  in critical path.
- Web UI / Grafana — not happening.
- Parallel multi-row planner — planner stays single-row repair.
- Live-LLM tests against codexu/claudeu/opencode in CI.
- Performance work for `progress.json` >10× current size — premature.

## Decision Summary

The accepted design is **Reactive Autoloop** — a single-loop architecture that
adds a feedback channel between workers, the run loop, and the planner via a
typed `Health` block on each `progress.json` row.

```
┌─────────────────────┐     progress.json (with health blocks)     ┌──────────────────────────────┐
│   architecture-     │ ◀── reads health, repairs toxic rows ──── │       autoloop run loop      │
│   planner-loop      │ ── writes refined contract, ────────────▶ │  - reads health for ranking  │
│                     │     auto-clears stale quarantine          │  - skips quarantined         │
│                     │                                            │  - degrades backend          │
└─────────────────────┘                                            │  - writes health at run end  │
                                                                   └──────────────────────────────┘
                                                                            │  ▲
                                                              spawns workers │  │ classifies outcomes
                                                                            ▼  │
                                                                   ┌──────────────────────────────┐
                                                                   │      worker (codexu / …)     │
                                                                   │  emits final report          │
                                                                   │  → strict parse OR repair    │
                                                                   │    pass → success/failure    │
                                                                   └──────────────────────────────┘
```

Five layers, each independently shippable:

| Layer | Slice description | Touches |
|---|---|---|
| L1. `Health` schema + merge IO | Extend `internal/progress` with typed `RowHealth` + atomic batched write helpers | `internal/progress/progress.go`, new `internal/progress/health.go`, tests |
| L2. Run-loop writes health | Per-run accumulator, atomic flush at run end, soft-skip semantics, backend degrade | `internal/autoloop/run.go`, new `internal/autoloop/health_writer.go`, `backend.go`, `config.go` |
| L3. Selection honors health | Quarantine filter + failure penalty in candidate ranking | `internal/autoloop/candidates.go` |
| L4. Loop resilience | Soft-skip + loop-level backend degrade chain (folded into L2 commit; called out separately for clarity) | folded into L2 |
| L5. Report repair pass | Salvage successful work whose final report won't strict-parse | `internal/autoloop/report.go`, promotion path in `run.go` |
| L6. Planner consumes health | Quarantined-rows context + prompt preservation rule + post-regen validator | `internal/architectureplanner/context.go`, `prompt.go`, `run.go` |

L1 is foundational. L2-L6 can land in any order after L1; the natural ramp is
L1 → L2 → L3 → L5 → L6, with L4's behaviors landing inside the L2 commit.

## L1 — Health schema and merge IO

### New file: `internal/progress/health.go`

```go
package progress

// RowHealth is execution-history metadata about one progress.json item.
// Owned by autoloop. The planner READS it to prioritize repairs and MUST
// preserve any unknown fields verbatim across regenerations.
type RowHealth struct {
    AttemptCount        int             `json:"attempt_count,omitempty"`
    ConsecutiveFailures int             `json:"consecutive_failures,omitempty"`
    LastAttempt         string          `json:"last_attempt,omitempty"`  // RFC3339
    LastSuccess         string          `json:"last_success,omitempty"`  // RFC3339
    LastFailure         *FailureSummary `json:"last_failure,omitempty"`
    BackendsTried       []string        `json:"backends_tried,omitempty"`
    Quarantine          *Quarantine     `json:"quarantine,omitempty"`
}

type FailureSummary struct {
    RunID      string          `json:"run_id"`
    Category   FailureCategory `json:"category"`
    Backend    string          `json:"backend,omitempty"`
    StderrTail string          `json:"stderr_tail,omitempty"` // capped at 2 KiB
}

type FailureCategory string

const (
    FailureWorkerError      FailureCategory = "worker_error"
    FailureReportValidation FailureCategory = "report_validation_failed"
    FailureProgressSummary  FailureCategory = "progress_summary_failed"
    FailureTimeout          FailureCategory = "timeout"
    FailureBackendDegraded  FailureCategory = "backend_degraded"
)

type Quarantine struct {
    Reason       string          `json:"reason"`
    Since        string          `json:"since"`        // RFC3339
    AfterRunID   string          `json:"after_run_id"`
    Threshold    int             `json:"threshold"`
    SpecHash     string          `json:"spec_hash"`    // hash of (contract,contract_status,blocked_by,write_scope,fixture)
    LastCategory FailureCategory `json:"last_category"`
}
```

### Modification to `internal/progress/progress.go`

Add one typed field to `Item`:

```go
type Item struct {
    // ... existing fields preserved ...
    Health *RowHealth `json:"health,omitempty"`
}
```

`omitempty` keeps `progress.json` clean for fresh rows.

### Three new helpers in `internal/progress/health.go`

```go
// ItemSpecHash returns a stable hash of the row's spec fields used for
// quarantine auto-clear detection. Excludes Status, Health, Name.
func ItemSpecHash(item *Item) string

type HealthUpdate struct {
    PhaseID    string
    SubphaseID string
    ItemName   string
    Mutate     func(current *RowHealth) // nil current means "create one"
}

// ApplyHealthUpdates loads progress.json, applies a batch of mutations in
// a single load+save, writes back with stable key ordering. Atomic via
// temp-file + rename.
func ApplyHealthUpdates(path string, updates []HealthUpdate) error
```

### Schema invariants

1. `Health.AttemptCount` is monotonic.
2. `Health.ConsecutiveFailures` resets to 0 on EITHER a `LastSuccess` write
   OR a stale-quarantine clear (spec hash mismatch detected by L3 selection
   and committed by the L2 run-end write). The stale-clear case is what gives
   a planner-repaired row fresh runway: if the planner changed the contract,
   the failure history that justified quarantine no longer reflects the
   current row, so the counter resets along with the quarantine block.
3. `Health.Quarantine.SpecHash` is set when quarantine is created. On every
   selection pass, autoloop compares it to current `ItemSpecHash(item)` — if
   different, quarantine is cleared at run-end and `ConsecutiveFailures` is
   reset to 0 alongside.
4. Manual override: human can delete the `quarantine` block, or set
   `health: {}` to fully reset history.
5. `progress.json` writes are atomic (temp+rename); concurrent reads never
   see a partial file.

### Spec hash field set

`ItemSpecHash` covers `(contract, contract_status, blocked_by, write_scope,
fixture)`. It deliberately excludes `Status` (autoloop's own field) and `Name`
(rename ≠ reshape).

## L2 — Run-loop writes health

### Per-run accumulator

```go
// internal/autoloop/health_writer.go (new)

type healthAccumulator struct {
    runID     string
    now       func() time.Time
    threshold int                     // QUARANTINE_THRESHOLD, default 3
    updates   map[rowKey]*pendingHealth
}

type rowKey struct{ phaseID, subphaseID, itemName string }

type pendingHealth struct {
    outcome    workerOutcome  // success | failure-with-category
    backend    string
    stderrTail string
    specHash   string         // captured at selection time
}

func (a *healthAccumulator) RecordSuccess(c Candidate)
func (a *healthAccumulator) RecordFailure(c Candidate, cat progress.FailureCategory, backend, stderrTail string)
func (a *healthAccumulator) Flush(progressPath string) error
```

`RunOnce` instantiates one accumulator per run. Every `worker_success`,
`worker_failed`, `report_validation_failed`, `worker_promoted`,
`worker_promotion_failed` event also calls the matching accumulator method.
At end of `RunOnce`, single `Flush()` writes `progress.json` once.

### Quarantine evaluation rule (centralized in `Flush`)

```
For each row with at least one failure in this run:
  newConsecutive = (existing ConsecutiveFailures) + (failures this run)
  if newConsecutive >= threshold AND no successes this run:
    set Quarantine{
      Reason:       "auto: N consecutive failures, last category C",
      Since:        now,
      AfterRunID:   runID,
      Threshold:    threshold,
      SpecHash:     specHash,
      LastCategory: lastCat,
    }

For each row with at least one success in this run:
  ConsecutiveFailures = 0
  LastSuccess = now
  Quarantine = nil
```

### Soft-skip semantics

```go
// run.go RunOnce loop (pseudocode)
for _, candidate := range selected {
    if err := preflightCandidate(candidate); err != nil {
        emit(ledger.Event{Type: "candidate_skipped", Reason: err.Error(), Candidate: candidate})
        accumulator.RecordFailure(candidate, progress.FailureProgressSummary, "", err.Error())
        continue
    }
    // ... spawn worker ...
}
```

The whole-loop fail path is reserved for: (a) inability to read `progress.json`
at start; (b) inability to write health at end; (c) explicit `--fail-fast` CLI
flag (off by default).

### Loop-level backend degrade

```go
type backendDegrader struct {
    chain                    []string  // ["codexu","claudeu","opencode"] from BACKEND_FALLBACK
    current                  string    // starts at chain[0]
    consecutiveBackendErrors int
    threshold                int       // BACKEND_DEGRADE_THRESHOLD, default 3
    degraded                 bool
}

func (d *backendDegrader) ObserveOutcome(outcome workerOutcome) {
    if outcome.IsBackendError() {
        d.consecutiveBackendErrors++
        if d.consecutiveBackendErrors >= d.threshold && !d.degraded {
            d.current = next(d.chain, d.current)
            d.degraded = true
            emit(ledger.Event{Type: "backend_degraded", From: previous, To: d.current})
        }
    } else if outcome.IsSuccess() {
        d.consecutiveBackendErrors = 0
    }
}
```

`workerOutcome.IsBackendError()` returns true for: empty/missing commit hash
AND `worker_error` category AND no diff produced. Failures with a commit are
row problems, not infra problems, and do not count as backend errors.

The fallback chain is closed: once degraded past the last backend, further
backend errors fail the row but don't degrade further.

### New env vars (defaults preserve current behavior)

| Var | Default | Effect |
|---|---|---|
| `QUARANTINE_THRESHOLD` | `3` | Consecutive failures before auto-quarantine. |
| `BACKEND_DEGRADE_THRESHOLD` | `3` | Consecutive backend-errors before loop switches backends. |
| `BACKEND_FALLBACK` | `""` | Comma-separated chain. Empty = degrade is a no-op (back-compat). |

### New ledger event types

| Type | When emitted |
|---|---|
| `candidate_skipped` | Per-row preflight failure (was previously a loop pause) |
| `backend_degraded` | Backend chain advanced |
| `health_updated` | Run-end batched write succeeded; carries `{rows_updated, quarantined, cleared}` |
| `health_update_failed` | Run-end write failed; loop returns error after this event |

## L3 — Selection honors health

### Added selection rules (after existing ActiveFirst + MAX_PHASE + PRIORITY_BOOST)

```
For each candidate row:

  1. Auto-clear stale quarantine (read-only):
     if Health.Quarantine != nil
        AND Health.Quarantine.SpecHash != ItemSpecHash(item):
       → treat as not-quarantined for this selection pass
       → emit "quarantine_stale" reason (will trigger an actual clear in
         the run-end write so progress.json stays in sync)

  2. Quarantine filter (after stale-clear check):
     if Health.Quarantine != nil (and not stale):
       → exclude from candidates
       → unless GORMES_INCLUDE_QUARANTINED=1

  3. Failure penalty in scoring:
     score -= failurePenalty(Health.ConsecutiveFailures)
     where failurePenalty(n):
       0 → 0
       1 → 5
       2 → 20
       3+ → 45

  4. Backend-tried penalty (smaller, additive):
     score -= 2 * len(Health.BackendsTried)
```

### New `Candidate` fields surfaced for observability

```go
type Candidate struct {
    // ... existing fields ...
    Health          *progress.RowHealth `json:"health,omitempty"`
    StaleQuarantine bool                `json:"stale_quarantine,omitempty"`
    PenaltyApplied  int                 `json:"penalty_applied,omitempty"`
}
```

### One env var

| Var | Default | Effect |
|---|---|---|
| `GORMES_INCLUDE_QUARANTINED` | `0` | When `1`, quarantined rows are NOT excluded — used by humans debugging. |

### Selection reasons get richer

The existing `Candidate.SelectionReason()` already surfaces `active_first`,
`priority_boost`, `unblocks_downstream`. Add:

- `"quarantine_excluded"` — visible only via `--dry-run` (it's an exclusion).
- `"penalty=N"` — appended to selected-row reasons that took any failure penalty.
- `"quarantine_stale_cleared"` — when selection treated a quarantined row as live.

### What does not change

- Existing `ActiveFirst` ranking, `MAX_PHASE` cap, `PRIORITY_BOOST`,
  blocked_by enforcement.
- The `progress.json` file (selection is read-only; writes happen in L2).

## L5 — Report repair pass

### Two-stage parse, strict-first

```go
// internal/autoloop/report.go (existing strict parser stays untouched)

func ParseFinalReport(text string) (*FinalReport, error) // existing

// NEW: TryRepairReport runs ONLY when ParseFinalReport returns a parse
// error. Independently reconstructs the same FinalReport shape from
// secondary evidence.
func TryRepairReport(ctx RepairContext) (*FinalReport, []RepairNote, error)

type RepairContext struct {
    WorkerStdout    string
    WorkerStderr    string
    WorktreePath    string  // git ops happen here
    BaseBranch      string  // for diff range
    AcceptanceLines []string // expected acceptance criteria from progress.json row
}

type RepairNote struct {
    Field  string  // "commit", "evidence", "acceptance"
    Source string  // "git_log", "stdout_grep", "test_output"
    Detail string
}
```

### Reconstruction map

| Field | How reconstructed |
|---|---|
| `Commit` | `git -C $WorktreePath log --format=%H $BaseBranch..HEAD -1` |
| `RedEvidence` | First `--- FAIL:` block in `WorkerStdout`. None → repair fails. |
| `GreenEvidence` | Last `--- PASS:` block + `PASS\nok` summary. None → repair fails. |
| `AcceptanceCriteria` | Every `done_signal` string from row appears in stdout, OR acceptance set is empty. |
| `Diff` | `git diff $BaseBranch..HEAD` |

### Repair succeeds only if ALL of

1. There IS a new commit on the worker's branch.
2. The diff is non-empty.
3. There's at least one `PASS` token in the worker's test output.
4. Either every acceptance line is present in stdout, OR the acceptance set
   is empty in the row.

If repair succeeds → worker is promoted via existing `promote.go`; ledger gets
`report_repaired` event with `RepairNote`s; `Health.LastFailure` is recorded as
`report_validation_failed` so the planner can see the row produces messy
reports (data point, not a death sentence).

If repair fails → behaves exactly like today (`report_validation_failed`, no
promotion, accumulator records the failure for quarantine math).

### Repair artifact for forensics

```
.codex/orchestrator/state/repairs/<runID>-<workerID>.json
{
  "candidate":  {phaseID, subphaseID, itemName},
  "commit":     "abc123",
  "diff_lines": 47,
  "notes": [
    {"field":"commit","source":"git_log","detail":"reconstructed from $WorktreePath"},
    {"field":"acceptance","source":"stdout_grep","detail":"matched 3/3 done_signal lines"}
  ],
  "stdout_excerpt": "..."  // 4 KiB tail
}
```

### Bias guard

The repair pass must not lower the bar for what counts as "tests passed."
Same `--- PASS:` / `ok` tokens as the strict parser. If a worker actually
skipped tests, repair will still fail.

### One env var

| Var | Default | Effect |
|---|---|---|
| `GORMES_REPORT_REPAIR` | `1` | Set to `0` to disable the repair pass. |

## L6 — Planner consumes health

### Surface quarantined rows in the planner context

`context.go::CollectContext` adds one new section:

```go
type QuarantinedRowContext struct {
    PhaseID            string
    SubphaseID         string
    ItemName           string
    Contract           string
    LastCategory       progress.FailureCategory
    AttemptCount       int
    BackendsTried      []string
    QuarantinedSince   string  // RFC3339
    SpecHash           string
    LastFailureExcerpt string  // 1 KiB tail of the StderrTail
    AuditCorroboration string  // matching insight from SummarizeAutoloopAudit if any
}

func collectQuarantinedRows(doc *progress.Document, audit AutoloopAudit) []QuarantinedRowContext
```

Sort key: `(AttemptCount desc, QuarantinedSince asc)`.

### Two prompt clauses added to `prompt.go`

```text
HEALTH BLOCK PRESERVATION (HARD RULE)
Every progress.json item may carry a `health` block (RowHealth). This block
is OWNED by the autoloop runtime — you must reproduce it verbatim in your
output for any row you keep. Do not modify, omit, or reformat any field
inside `health`. If you delete a row, the health block dies with it (that
is expected). If you split a row into multiple new rows, the original
health block is dropped (the split is a new contract; quarantine resets
naturally via spec-hash detection).

QUARANTINE PRIORITY (SOFT RULE)
Rows in `quarantined_rows[]` are top priority for repair. For each one:
  - Read its `last_category` and `last_failure_excerpt`
  - Examine its `contract` and `acceptance`
  - Decide ONE of:
    (a) Sharpen the contract — make `done_signal` more concrete, add an
        explicit fixture path, narrow `write_scope`
    (b) Split the row — if it's an umbrella that workers can't complete
        atomically, split into 2-3 smaller rows with explicit dependencies
    (c) Mark it for human review — if the failure is infrastructural (e.g.
        category=worker_error or backend_degraded with no diff), set
        `contract_status: "draft"` and add a note in `degraded_mode`
        explaining what's needed
  Whatever you choose, the row's contract/contract_status/blocked_by/
  write_scope/fixture must change in some material way. Otherwise
  quarantine will not auto-clear and autoloop will keep skipping the row.
```

### Post-regeneration validator

```go
// internal/architectureplanner/run.go

func validateHealthPreservation(beforeDoc, afterDoc *progress.Document) error {
    // For every (phaseID, subphaseID, itemName) tuple that exists in BOTH
    // documents, the Health block must be byte-equal.
    //   - If the row was deleted → no check (planner intentionally removed it)
    //   - If the row was added → no check (new row, no health to preserve)
    //   - If the row's spec changed → Health is preserved verbatim; the spec
    //     hash mismatch is what triggers auto-clear in autoloop's L3.
}
```

If validation fails, the planner regeneration is rejected; `progress.json`
stays at its previous state.

### One env var

| Var | Default | Effect |
|---|---|---|
| `GORMES_PLANNER_QUARANTINE_LIMIT` | `5` | Cap how many quarantined rows the planner sees per context. Remainder still appear in the trend audit. |

### What does not change

- `SummarizeAutoloopAudit` — kept as-is; still feeds the trend view.
- `latest_planner_report.md` format.
- `planner_state.json`.
- The planner's invocation cadence (Phase C territory).

## Testing

Per-layer unit tests are enumerated within each layer above. Five
cross-cutting test surfaces catch interaction bugs:

### 1. End-to-end row lifecycle test (`internal/autoloop/lifecycle_test.go`, new)

Exercises Run 1 (failure) → Run 2 (failure) → Run 3 (failure → quarantine) →
Run 4 (excluded) → planner edits row → Run 5 (stale-clear → success → quarantine
cleared). Uses fake `Runner` interface. No live backend.

### 2. Backwards-compatibility round-trip (`internal/progress/health_compat_test.go`, new)

Loads the actual checked-in `progress.json`, saves it back through the new
helpers with an empty update set, asserts byte-equal modulo trailing-newline
normalization.

### 3. Concurrent-write safety (`internal/progress/health_concurrent_test.go`, new)

N goroutines call `ApplyHealthUpdates` with disjoint row keys. All writes
succeed. Final document contains all N updates. No partial file ever visible.

### 4. Selection determinism with health (`internal/autoloop/candidates_health_test.go`, new)

Synthetic health blocks exercise: zero penalty equivalence, tied-row
demotion, quarantine exclusion, `GORMES_INCLUDE_QUARANTINED=1` override.

### 5. Planner round-trip preservation (`internal/architectureplanner/health_preservation_test.go`, new)

Tests `validateHealthPreservation` against synthetic before/after docs:
identical (accepted), modified health (rejected), deleted row (accepted),
split row (accepted), spec changed but health preserved (accepted).

### Out of test scope

- Live-LLM tests against backends (slow, flaky).
- Performance benchmarks against `progress.json` >10× current size.
- Visual diff of dry-run output (existing smoke tests cover).
- Soak/chaos tests for systemd timer.

### CI considerations

All new tests run under `go test ./internal/autoloop/... ./internal/progress/...
./internal/architectureplanner/...`. No new CI configuration. Adds ~2-3 seconds
to existing suite.

## Rollout Notes

This redesign is intentionally additive across layers:

- L1 introduces the schema and helpers but adds no behavior.
- L2 starts writing health and degrading backends but autoloop ignores those
  fields in selection (until L3 lands).
- L3 starts honoring quarantine and penalties; rows previously selected may
  start being skipped.
- L5 starts repairing reports; promotion rate should rise visibly.
- L6 starts the planner→autoloop feedback loop; quarantined rows get
  repaired faster.

Each layer ships as one commit. If any layer regresses, it can be reverted
independently. The schema (L1) is the only layer with a migration concern,
and `omitempty` keeps the migration trivial: untouched rows look identical.

The five behavioral changes together should move the headline metric (worker
promotion rate) materially upward while making the loop's failure modes
self-limiting instead of self-perpetuating.
