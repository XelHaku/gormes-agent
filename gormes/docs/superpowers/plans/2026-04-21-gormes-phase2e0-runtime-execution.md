# Gormes Phase 2.E0 Runtime Execution Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the first deterministic subagent runtime slice: bounded child runs with caller-aware cancellation, stable timeout/cancel exit reasons, preserved batch concurrency limits, and a green test suite around the runtime seam.

**Architecture:** Keep the slice inside `gormes/internal/subagent` so it can ship independently of the future skills runtime. The manager remains the control plane, the runner remains swappable, and `delegate_task` remains the only public surface. The first implementation slice tightens cancellation semantics before adding new persistence or richer policy layers.

**Tech Stack:** Go stdlib (`context`, `sync`, `sync/atomic`, `time`, `errors`), existing `gormes/internal/subagent` package, existing `delegate_task` tool tests, existing docs test harness.

---

## File Map

- Modify: `gormes/internal/subagent/manager.go`
- Modify: `gormes/internal/subagent/subagent.go`
- Modify: `gormes/internal/subagent/manager_test.go`
- Modify: `gormes/internal/subagent/batch_test.go`
- Modify: `gormes/internal/subagent/delegate_tool_test.go`
- Modify: `gormes/internal/subagent/types.go` if stable exit-reason constants are introduced
- Create later slice: `gormes/internal/subagent/runlog.go`
- Create later slice: `gormes/internal/subagent/runlog_test.go`

## Task 1: Deterministic caller cancellation plumbing

**Acceptance criteria**

- A child spawned with `Spawn(ctx, cfg)` is cancelled when the spawn caller context is cancelled.
- Batch cancellation does not leave child goroutines running in the registry.
- Parent-manager cancellation still cascades exactly as before.
- No unrelated package outside `gormes/internal/subagent` is touched for this slice.

- [ ] **Step 1: Write the failing tests**

Add these tests:

- `TestManagerSpawnCallerCtxCancellationCascades` in `gormes/internal/subagent/manager_test.go`
- `TestSpawnBatchContextCancellationCancelsChildren` in `gormes/internal/subagent/batch_test.go`

Test intent:

- Spawn one blocking subagent with a dedicated caller `context.WithCancel`.
- Cancel the caller context.
- Assert `WaitForResult(context.Background())` returns a terminal interrupted result instead of hanging.
- Assert batch cancellation drains the live registry instead of returning while children keep running.

- [ ] **Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd gormes
go test ./internal/subagent -run 'TestManagerSpawnCallerCtxCancellationCascades|TestSpawnBatchContextCancellationCancelsChildren' -count=1 -v
```

Expected:

- `TestManagerSpawnCallerCtxCancellationCascades` fails because `Spawn` currently ignores the caller `ctx`.
- `TestSpawnBatchContextCancellationCancelsChildren` fails because batch cancellation can return before the child itself is cancelled.

- [ ] **Step 3: Implement minimal context plumbing**

Modify `gormes/internal/subagent/manager.go` and `gormes/internal/subagent/subagent.go`:

- Derive the child runtime from both `ManagerOpts.ParentCtx` and the per-call `Spawn(ctx, cfg)` context.
- Use stop hooks or equivalent cleanup so bridge callbacks do not leak after child completion.
- Preserve the existing public API; do not widen `SubagentManager`.

- [ ] **Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd gormes
go test ./internal/subagent -run 'TestManagerSpawnCallerCtxCancellationCascades|TestSpawnBatchContextCancellationCancelsChildren' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/manager.go gormes/internal/subagent/subagent.go gormes/internal/subagent/manager_test.go gormes/internal/subagent/batch_test.go
git commit -m "feat(subagent): honor caller cancellation in runtime"
```

**Risk:** child contexts may now terminate earlier than older tests assumed.

**Rollback:** revert this commit only; no schema or config migration is involved.

## Task 2: Stable timeout and cancellation exit reasons

**Acceptance criteria**

- Timed-out children report a canonical exit reason such as `timeout`.
- Explicit interrupts report a canonical interrupted exit reason instead of inheriting runner-specific strings.
- `delegate_task` JSON output reflects the canonical exit reason.

- [ ] **Step 1: Write the failing tests**

Add or update:

- `TestManagerTimeoutProducesCanonicalExitReason` in `gormes/internal/subagent/manager_test.go`
- `TestDelegateToolUsesManagerDefaultTimeout` in `gormes/internal/subagent/delegate_tool_test.go`

Test intent:

- Use `blockingRunner` with `ManagerOpts.DefaultTimeout`.
- Assert the terminal result status is interrupted and `ExitReason` is `timeout`.
- Assert `delegate_task` returns the same canonical reason in JSON.

- [ ] **Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd gormes
go test ./internal/subagent -run 'TestManagerTimeoutProducesCanonicalExitReason|TestDelegateToolUsesManagerDefaultTimeout' -count=1 -v
```

Expected: FAIL because the current runtime leaks runner-specific `ctx_cancelled`.

- [ ] **Step 3: Implement result normalization**

Modify `gormes/internal/subagent/manager.go` and `gormes/internal/subagent/types.go` as needed:

- Normalize terminal results after runner completion.
- Map deadline expiry to `timeout`.
- Map explicit interrupt to `interrupted`.
- Keep success and real failures untouched.

- [ ] **Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd gormes
go test ./internal/subagent -run 'TestManagerTimeoutProducesCanonicalExitReason|TestDelegateToolUsesManagerDefaultTimeout' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/manager.go gormes/internal/subagent/types.go gormes/internal/subagent/manager_test.go gormes/internal/subagent/delegate_tool_test.go
git commit -m "feat(subagent): normalize timeout and interrupt reasons"
```

**Risk:** existing downstream callers may have assertions on legacy exit-reason strings.

**Rollback:** revert this commit only; the runtime falls back to old strings without data repair.

## Task 3: Concurrency-bound batch defaults

**Acceptance criteria**

- `SpawnBatch(..., 0)` honors `ManagerOpts.DefaultMaxConcurrent` when set.
- The package default remains `DefaultMaxConcurrent` when the option is unset.
- Cancellation still drains children under both paths.

- [ ] **Step 1: Write the failing test**

Add `TestSpawnBatchUsesConfiguredDefaultMaxConcurrent` in `gormes/internal/subagent/batch_test.go`.

- [ ] **Step 2: Run the targeted test and verify RED**

Run:

```bash
cd gormes
go test ./internal/subagent -run 'TestSpawnBatchUsesConfiguredDefaultMaxConcurrent' -count=1 -v
```

Expected: FAIL if the runtime does not honor the configured default semaphore width.

- [ ] **Step 3: Implement the minimal fix**

Modify only `gormes/internal/subagent/batch.go` if needed.

- [ ] **Step 4: Run the targeted test and verify GREEN**

Run:

```bash
cd gormes
go test ./internal/subagent -run 'TestSpawnBatchUsesConfiguredDefaultMaxConcurrent' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/batch.go gormes/internal/subagent/batch_test.go
git commit -m "feat(subagent): honor configured batch concurrency defaults"
```

**Risk:** tighter concurrency can change benchmark timing.

**Rollback:** revert the commit; no API migration.

## Task 4: Append-only runtime log

**Acceptance criteria**

- Every finished child writes one append-only JSONL record.
- Records include `id`, `parent_id`, `goal`, `status`, `exit_reason`, `duration_ms`, and a timestamp.
- Log writing failure does not corrupt child completion.

- [ ] **Step 1: Write the failing tests**

Create:

- `gormes/internal/subagent/runlog_test.go`

Test intent:

- Complete one child run.
- Assert exactly one JSONL line is appended.
- Assert log-write failure is surfaced as an internal warning path, not a hung subagent.

- [ ] **Step 2: Run the targeted tests and verify RED**

Run:

```bash
cd gormes
go test ./internal/subagent -run 'TestRunLog' -count=1 -v
```

Expected: FAIL because the logger does not exist yet.

- [ ] **Step 3: Implement the append-only logger**

Create `gormes/internal/subagent/runlog.go` and wire it from `manager.go`.

- [ ] **Step 4: Run the targeted tests and verify GREEN**

Run:

```bash
cd gormes
go test ./internal/subagent -run 'TestRunLog' -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/subagent/runlog.go gormes/internal/subagent/runlog_test.go gormes/internal/subagent/manager.go
git commit -m "feat(subagent): append runtime run logs"
```

**Risk:** file-path policy may need to move to config in the next slice.

**Rollback:** revert the commit and remove the created log file during manual cleanup.

## Validation Gate After Each Task

Run:

```bash
cd <repo>/gormes
go test ./internal/subagent -count=1
go test ./docs -count=1
```

For the final `2.E0` branch gate, run:

```bash
cd <repo>/gormes
go test ./... -count=1
go test ./docs -count=1
```

If `go test ./...` is temporarily too expensive during an intermediate slice, the report must include the exact scoped command used and why full-suite verification is deferred.
