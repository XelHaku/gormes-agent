# Orchestrator "Oil Release 1" — Consolidated Design

**Status:** Draft
**Author:** xel + Claude
**Date:** 2026-04-24
**Supersedes:** part of the earlier Spec A; follows on Spec C (test harness) and the claudeu shim.

## Why this spec

After today's regex fix, the orchestrator genuinely produces promoted commits (audit shows 3 commits landed in 30 min). But observed cycle output also shows recurring residual leaks:

1. Two of three `worker_success` events in cycle 1 ended in `worker_promotion_failed / cherry_pick_failed` — both workers had edited the same `progress.json` hunks.
2. The previously-dropped A2 (retry-prompt context) would sharply improve per-task success once poisoned-task retries get the benefit.
3. Companions are fully disabled (`DISABLE_COMPANIONS=1`) because a single slow `landingpage-improver` call blocked worker cycles for 23 minutes. They need to run without blocking before we can re-enable them.
4. The candidate pool is small (~6 non-poisoned). Once cycle traffic exhausts it, the orchestrator will silently idle.
5. The `claudeu` shim is a working hack but has no tests, no CLI surface, and pretends to be codex.
6. `verify_worker_commit` is strict in ways that occasionally reject legitimate work (e.g. when a task genuinely needs >1 commit, or leaves a benign file in the worktree).
7. `audit.sh`'s ndjson is good for grep but not for graphing throughput over time.
8. `classify_worker_failure` only distinguishes `timeout | killed | contract_or_test_failure | worker_error`. "contract_or_test_failure" conflates report-malformed, commit-count-wrong, worktree-dirty, and real test failures — harder to triage.

This spec bundles all 8 fixes so the autoloop becomes something the human can walk away from overnight. Each fix is independently landable; order matters for #3 + #4.

## Goals

- Cut the remaining promotion waste (items #1, #2) so audit productivity trends above 50%.
- Let companions run without blocking worker throughput (#3) so the planner keeps candidates fresh without the 23-min stall pattern.
- Prevent silent idleness when candidates exhaust (#4) so the forever loop actually has work.
- Harden the backend adapter (#5) so switching agents doesn't require PATH shim magic.
- Reduce false-negative rejections (#6) that waste worker cycles.
- Make the audit output graphable (#7) for long-run trending.
- Distinguish failure modes (#8) so poison-pill decisions and retry-prompt augmentation can target root cause.

## Non-goals

- Re-architecting the phase-parallelism model (that's future Spec B: per-phase worker pools, dependency graphs).
- Changing the `runs.jsonl` schema's core keys (only additive fields allowed).
- Supporting non-Linux hosts.

---

## #1 — `progress.json` conflict class fix

**Problem:** All workers start from the same `BASE_COMMIT`, all edit `docs/content/building-gormes/architecture_plan/progress.json` to flip their item status, then sequential cherry-picks after the first one conflict on overlapping hunks.

**Fix — two-tier:**

1. **Short-term (ship first):** In `promote_successful_workers`, use `git cherry-pick -Xtheirs <commit>` instead of plain cherry-pick. `theirs` resolves conflicts in favor of the worker's version. This is safe for `progress.json` (the worker's intent is to mark its own task complete) and for any non-overlapping other files.
2. **Medium-term (if #1-short doesn't resolve 80%+):** Introduce per-task marker files at `gormes/.codex/orchestrator/progress-claims/<slug>.json` that workers create during their commit. Add a post-promotion step in `run_once` that regenerates the central `progress.json` from the marker files via an extension to `cmd/progress-gen`. Workers stop editing `progress.json` directly.

The short-term approach lands in this spec; medium-term is deferred to Oil Release 2 if needed.

**Implementation points:**
- Change `git -C "$GIT_ROOT" cherry-pick "$commit"` → `git -C "$GIT_ROOT" cherry-pick -Xtheirs "$commit"` at the single callsite in `lib/promote.sh`.
- Update existing `cherry-pick-conflict.bats` integration test; the conflict case it pins may now succeed instead of fail. Either assert new behavior or add a "truly uncombinable" fixture.

**Test:** new bats case in `promote.bats` — two worker branches that both edit the same line of progress.json; both cherry-picks must succeed with `-Xtheirs`.

---

## #2 — Retry-prompt context

**Problem:** When a task fails, the orchestrator claims it again in a later cycle with a byte-identical prompt. The agent has no memory of its prior error; 79%+ of the 24h retry storm was identical attempts.

**Fix:**

- New file `$STATE_DIR/task-failures/<slug>.json` tracking `{count, last_rc, last_reason, last_ts, last_stderr_path, last_final_errors: [strings]}`.
- Hook increments/reset:
  - On `worker_failed`: increment `count`, persist last_rc + last_reason + stderr tail path + `collect_final_report_issues` output.
  - On `worker_promotion_failed`: increment `count`, record "cherry_pick_failed".
  - On `worker_promoted`: delete the file (clean slate for future tasks).
- `build_prompt` in `lib/report.sh` checks for `$STATE_DIR/task-failures/<slug>.json`. If present AND `count > 0`, inject a new section before the TDD Protocol:
  ```
  ==================================================
  PRIOR ATTEMPT FEEDBACK
  ==================================================
  This task has been attempted N times before. The last attempt failed.

  Previous exit code: <rc>
  Previous failure reason: <reason>

  Specific validation errors from the last attempt:
  - <each from last_final_errors>

  Last ~40 lines of stderr from the previous attempt:
  <tail>

  Focus on addressing these specific gaps. Do not repeat the same mistake.
  ==================================================
  ```

**Implementation points:**
- New `lib/failures.sh` with `failure_record_read`, `_write`, `_reset`, `is_poisoned`. Source from entry script.
- Hook calls in `run_worker`'s success/failure branches.
- `build_prompt` gains the injection block (controlled by a `RETRY_CONTEXT_ENABLED=1` env var, default on).
- Cap injected context at 5KB to avoid runaway bloat (`RETRY_CONTEXT_MAX_KB`).

**Test:** `unit/failures.bats` for the record CRUD; extend `unit/report.bats` to assert retry context appears in the generated prompt when a record exists.

---

## #3 — Background companions with short timeout

**Problem:** The current `maybe_run_companions` calls `run_companion` synchronously. A single slow companion (landingpage at 23 min today) blocks all subsequent worker cycles. Hence the current `DISABLE_COMPANIONS=1`.

**Fix:**

- Rewrite `run_companion` to launch via `setsid nohup bash -c "... companion …" >log 2>&1 &` — fully detached from the orchestrator's process group.
- Track companion PIDs in `$STATE_DIR/companions/<name>.pid`. `should_run_<name>` predicates additionally check: "is there already a live PID for this companion?" → if yes, skip (don't launch a second).
- Reduce `COMPANION_TIMEOUT_SECONDS` default from 1800s → 600s (10 min). The detached process still gets killed via `timeout` inside the detached wrapper.
- Add a reaper step in `maybe_run_companions`: before deciding whether to launch, inspect PID files, clean up dead ones, update state files accordingly.

**Implementation points:**
- All changes confined to `lib/companions.sh`.
- `run_companion` becomes fire-and-forget: records the launch, returns immediately.
- A new `companion_reap_stale` function runs at the top of `maybe_run_companions` to update state files for PIDs that are no longer live.
- Default override: `DISABLE_COMPANIONS=0`, `COMPANION_ON_IDLE=0`, `COMPANION_TIMEOUT_SECONDS=600` so the user gets the new behavior automatically after restart.

**Test:** `unit/companions.bats` gets a case where `run_companion` returns immediately (not blocks) and a PID file is created. `integration/companion-trigger.bats` extended to verify concurrent companion launches don't stack.

---

## #4 — Candidate-pool refill trigger

**Problem:** Only ~6 non-poisoned candidates remain. Once they all succeed or fail three times, workers will claim nothing and the forever loop will spin silently.

**Fix:**

- In `run_once`, after `write_candidates_file`, check `candidate_count`. If below `CANDIDATE_LOW_WATERMARK` (default 5) AND `DISABLE_COMPANIONS != 1`, **synchronously** run the planner companion (exception to #3's detach-by-default rule) with `COMPANION_TIMEOUT_SECONDS` applied. The planner reads upstream Hermes + progress.json and expands the task list.
- After the planner finishes, re-run `write_candidates_file` from the freshly-edited progress.json.
- If the planner itself fails or produces no new candidates, log `candidate_starvation` and sleep `LOOP_SLEEP_SECONDS * 4` to avoid burning CPU on an empty pool.

**Implementation points:**
- `run_once` gets a new helper `maybe_refill_candidates` called after `write_candidates_file`.
- `maybe_refill_candidates` invokes `run_companion planner --sync` (new flag to opt into synchronous mode).
- Emits a `candidate_refilled` event with before/after counts.

**Test:** `integration/candidate-refill.bats` — fake-planner that appends a new task to the progress fixture. Orchestrator starts with ≤3 candidates; after refill triggers, candidates increase.

---

## #5 — Proper backend adapter (Spec B, lightweight)

**Problem:** Today's `claudeu` shim works but is a hack: a PATH-prefix trick that pretends to be `codexu`. No tests. No CLI flag. User can't easily switch between backends.

**Fix:**

- New `lib/backend.sh`:
  - `build_backend_cmd` — reads `$BACKEND` env var (default `codexu`), emits the right argv.
  - `backend_session_id <jsonl_file>` — extracts a session id per-backend.
- `$BACKEND` values: `codexu | claudeu | opencode`.
- Replace `build_codex_cmd` with a thin wrapper that delegates to `build_backend_cmd`; keep the function name for backwards compat.
- CLI flags `--claudeu`, `--opencode`, `--codexu` as shortcuts that set `$BACKEND` before parse.
- The `claudeu` bin-shim stays (it's still the translator from codexu argv → claude argv) but is only invoked when `BACKEND=claudeu`.

**Implementation points:**
- `parse_cli_args` gains the three flag branches.
- `build_codex_cmd` moved to `lib/backend.sh` and renamed `build_backend_cmd`; old name kept as an alias for 1 release.
- Usage text and README updated.

**Test:** `unit/backend.bats` — `BACKEND=X build_backend_cmd` emits expected argv per backend.

---

## #6 — Soften `verify_worker_commit`

**Problem:** Reject reasons that should sometimes be accepted:
- Worker made 2 commits because it intentionally split a large change. Rejected.
- Worker left a generated file in the worktree that shouldn't be committed but doesn't affect the task. Rejected.

**Fix:**

- Add optional env knobs read by `verify_worker_commit`:
  - `ALLOW_MULTI_COMMIT` (default 0): if 1, accept >1 commit as long as all commits are on the worker branch past `BASE_COMMIT`.
  - `TOLERATE_WORKTREE_UNTRACKED` (default 0): if 1, don't reject if only untracked files exist after the commit (still reject if modifications to tracked files are pending).
- Workers signal these exceptions via `--output-last-message` metadata section at the end:
  ```
  9) Runtime flags (optional)
  AllowMultiCommit: true
  ```
  `verify_worker_commit` parses this optional section and overrides its defaults only for that worker.

**Implementation points:**
- `verify_worker_commit` gets two early-return paths when the new env vars / flags are enabled.
- `collect_final_report_issues` must not reject reports that include or omit the new section (it's truly optional).

**Test:** extend `unit/worktree.bats` — fixtures with 2 commits + `AllowMultiCommit: true` → accept. Without flag → reject.

---

## #7 — audit.sh CSV output

**Problem:** ndjson is great for grep but bad for `gnuplot` / `csv-to-chart`. Want to plot throughput over time.

**Fix:**

- `audit.sh` also writes to `$AUDIT_DIR/report.csv` with headers:
  ```
  ts,active,uptime_s,nrestarts,claimed,success,failed,promoted,cherry_pick_failed,productivity_pct,integration_head_short
  ```
- Append on each audit run. Header only written if file doesn't exist.
- Existing ndjson unchanged (backward compat).

**Implementation:** 10 lines at the end of `audit.sh`. No new tests (format change only).

---

## #8 — Failure-mode taxonomy

**Problem:** Everything non-timeout ends up as `contract_or_test_failure`. This hides whether the actual root cause is report validation, commit verification, or test execution.

**Fix:**

- Instead of just using the exit code, check the reasons after the worker process exits:
  - If `wait_for_valid_final_report` failed → `report_validation_failed`
  - If `verify_worker_commit` failed, distinguish:
    - HEAD didn't advance → `no_commit_made`
    - Commit count wrong → `wrong_commit_count`
    - Worktree dirty → `worktree_dirty`
    - Branch mismatch → `branch_mismatch`
    - Scope-escaping files → `scope_violation`
  - If none of the above and rc != 0 → keep `contract_or_test_failure`.
- These feed into the `worker_failed` event's `status` field in `runs.jsonl` and into `lib/failures.sh` records.
- `classify_worker_failure` still handles rc-only mapping; the new logic lives in `run_worker` post-verification.

**Implementation points:**
- `verify_worker_commit` gains an output channel for the specific reason (via env var or file).
- Extend the fail-status breakdown in `audit.sh` to surface the new categories.

**Test:** `unit/worktree.bats` asserts the new reason strings are emitted.

---

## Ordering & interdependencies

Must happen in this order (some items gate others):

1. #1 (progress.json conflict) — independent, land first; biggest immediate effect.
2. #8 (failure taxonomy) — needed before #2 so retry-prompt context knows what the prior failure was.
3. #2 (retry-prompt context) — builds on #8.
4. #6 (verify_worker_commit softening) — independent, can parallelize.
5. #5 (backend adapter) — independent, can parallelize.
6. #3 (background companions) — prereq for #4.
7. #4 (candidate refill) — depends on #3 (sync mode of run_companion).
8. #7 (CSV) — independent, can parallelize anywhere.

Realistic single-PR bundle: **#1 → #8 → #2 → #3 → #4 → #6 → #5 → #7**. Each a separate commit for rollback granularity.

## Acceptance criteria

- Audit productivity rate sustained above 50% over 1 hour of runtime.
- No `cherry_pick_failed` events for the common `progress.json`-only case.
- `candidate_refilled` event fires at least once when the candidate pool drops; pool re-populates without manual intervention.
- Companions run in the background: worker cycles never delayed by more than `LOOP_SLEEP_SECONDS + 30s`.
- `make orchestrator-test-all` passes after every commit.
- `runs.jsonl` now contains granular failure statuses (`report_validation_failed`, `worktree_dirty`, etc.) instead of the conflated bucket.

## Out of scope (future Oil Release 2)

- Per-phase worker pools with dependency graph (A4).
- Move away from progress.json-as-source-of-truth to per-task marker files.
- Web UI for the audit.
- Telegram/Slack notification on ERROR/WARN from audit.

## Rollout

- Commit to `main` directly — the orchestrator is already surviving its own iteration in prod.
- Each commit runs the unit tests; integration tests mandatory before merging #3 (companion detach — highest blast radius).
- After the last commit, `systemctl --user restart gormes-orchestrator` to pick up lib changes (bash sources at startup — this has bitten us once; do not forget).
- First post-rollout audit at the next 20-min tick should show the new statuses and ideally a higher productivity rate.
