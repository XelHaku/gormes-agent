# Orchestrator Oil Release 1 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Each item is one commit.

**Goal:** Land 8 concrete improvements to the orchestrator so it runs productively unattended overnight.

**Architecture:** All changes live under `gormes/scripts/orchestrator/` (libs + tests) and in the entry script `gormes/scripts/gormes-auto-codexu-orchestrator.sh`. Each fix is independent within the sequenced order; bats tests cover the new paths.

**Tech Stack:** Bash 5.2, jq, git, GNU timeout, bats-core, systemd --user.

**Reference spec:** `gormes/docs/superpowers/specs/2026-04-24-orchestrator-oiling-release-1-design.md`

**Baseline commit:** `820350a2` (the markdown-header regex fix). All commits land on `main`.

## Conventions

- Each task below is ONE commit. Commit messages: `feat(orchestrator):` for new behavior, `fix(orchestrator):` for bugs, `refactor(orchestrator):` for internal changes.
- Every commit ends with `Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>`.
- After every commit, `make orchestrator-test` must pass. Before merging the full batch, `make orchestrator-test-all` (integration too).
- **After the LAST commit, `systemctl --user restart gormes-orchestrator`** — bash sources libs at process startup, not on every invocation. This has bitten us once before; do not forget.

---

## Task 1 — Item #1: cherry-pick `-Xtheirs` for `progress.json` class

**Files:**
- Modify: `gormes/scripts/orchestrator/lib/promote.sh`
- Modify: `gormes/scripts/orchestrator/tests/unit/promote.bats` (add new case)
- Modify: `gormes/scripts/orchestrator/tests/integration/cherry-pick-conflict.bats` (may now pass the previously-conflicting case; either assert new behavior or replace with a truly uncombinable fixture)

**Change:** locate `git -C "$GIT_ROOT" cherry-pick "$commit"` in `promote_successful_workers`, change to `git -C "$GIT_ROOT" cherry-pick -Xtheirs "$commit"`.

**New unit test:** two commits that both edit the same line of `progress.fixture.json` — second cherry-pick now succeeds. Assert integration branch has 2 commits past base.

**Integration test update:** the existing `cherry-pick-conflict.bats` pins the OLD behavior (workers conflict). Update its assertion: with `-Xtheirs`, both commits should land. Or rewrite using `--strategy=ours` for a truly incompatible case. Easiest: change the assertion to "both land" and rename the test to `cherry-pick-resolves-theirs.bats`.

**Commit message:** `fix(orchestrator): resolve cherry-pick conflicts with -Xtheirs strategy`

---

## Task 2 — Item #8: granular failure taxonomy (prereq for #2)

**Files:**
- Modify: `gormes/scripts/gormes-auto-codexu-orchestrator.sh` (the failure branch of `run_worker`)
- Modify: `gormes/scripts/orchestrator/lib/worktree.sh` (`verify_worker_commit` outputs the specific reason)
- Modify: `gormes/scripts/orchestrator/tests/unit/worktree.bats`

**Change:**
- `verify_worker_commit` exports `LAST_VERIFY_REASON` (global) on failure, set to one of: `no_commit_made | wrong_commit_count | worktree_dirty | branch_mismatch | scope_violation | report_commit_mismatch`.
- In `run_worker`, after the `verify_worker_commit` call fails, read `$LAST_VERIFY_REASON` and pass it into the subsequent `log_event "worker_failed"` call as the status (overrides `classify_worker_failure`'s rc-based mapping).
- After `wait_for_valid_final_report` fails, set status `report_validation_failed`.

**Test:** `unit/worktree.bats` — for each rejection path, assert `LAST_VERIFY_REASON` matches.

**Commit message:** `feat(orchestrator): granular failure taxonomy in runs.jsonl status`

---

## Task 3 — Item #2: retry-prompt context (depends on Task 2)

**Files:**
- Create: `gormes/scripts/orchestrator/lib/failures.sh`
- Create: `gormes/scripts/orchestrator/tests/unit/failures.bats`
- Modify: `gormes/scripts/orchestrator/lib/report.sh` (`build_prompt`)
- Modify: `gormes/scripts/orchestrator/tests/unit/report.bats`
- Modify: `gormes/scripts/gormes-auto-codexu-orchestrator.sh` (hook increment/reset)

**Change:**
- New `lib/failures.sh` with `failure_record_read <slug>`, `failure_record_write <slug> <rc> <reason> <stderr_file> <final_errors_json>`, `failure_record_reset <slug>`, `is_poisoned <slug> <max>`.
- Source from entry script after `lib/report.sh`.
- `run_worker` on failure: `failure_record_write "$slug" "$rc" "$reason" "$stderr_file" "$final_errors_json"`.
- `run_worker` on success: `failure_record_reset "$slug"`.
- `build_prompt` checks `failure_record_read <slug>`; if count > 0, injects the "PRIOR ATTEMPT FEEDBACK" section before the TDD protocol. Cap total injection at `RETRY_CONTEXT_MAX_KB` (default 5).

**Tests:**
- `unit/failures.bats` — record CRUD, read-after-write, reset clears, is_poisoned threshold.
- `unit/report.bats` — stub a failure record + call `build_prompt` → prompt contains the feedback section.

**Commit message:** `feat(orchestrator): retry-prompt context injection`

---

## Task 4 — Item #6: soften `verify_worker_commit`

**Files:**
- Modify: `gormes/scripts/orchestrator/lib/worktree.sh`
- Modify: `gormes/scripts/orchestrator/tests/unit/worktree.bats`

**Change:**
- `verify_worker_commit` reads `$ALLOW_MULTI_COMMIT` (default 0) and `$TOLERATE_WORKTREE_UNTRACKED` (default 0).
- If `ALLOW_MULTI_COMMIT=1`, accept any positive count of commits past `BASE_COMMIT` (still must be >0).
- If `TOLERATE_WORKTREE_UNTRACKED=1`, run `git status --short --uno` (uno = untracked-no) for the dirty check instead of `git status --short`.
- Parse an optional `9) Runtime flags` section in the final report:
  ```
  9) Runtime flags (optional)
  AllowMultiCommit: true
  ```
  If present with value `true`, override the env default for that worker only.

**Tests:**
- `unit/worktree.bats`: `ALLOW_MULTI_COMMIT=1` + 2 commits → accept. `TOLERATE_WORKTREE_UNTRACKED=1` + untracked file → accept. Default env + either anomaly → reject.
- `unit/report.bats`: `collect_final_report_issues` does NOT reject reports with or without section 9.

**Commit message:** `feat(orchestrator): optional softer verify_worker_commit via env + report flags`

---

## Task 5 — Item #5: proper BACKEND adapter

**Files:**
- Create: `gormes/scripts/orchestrator/lib/backend.sh`
- Create: `gormes/scripts/orchestrator/tests/unit/backend.bats`
- Modify: `gormes/scripts/gormes-auto-codexu-orchestrator.sh` (delete inline `build_codex_cmd`, add `--claudeu` / `--opencode` / `--codexu` flag parsing)
- Modify: `gormes/scripts/orchestrator/README.md`

**Change:**
- `lib/backend.sh` exposes `build_backend_cmd` that reads `$BACKEND` (default `codexu`) and emits the right argv via `printf '%s\0'`. Supports `codexu`, `claudeu` (via the existing shim), `opencode`.
- Keep `build_codex_cmd` as an alias for one release: `build_codex_cmd() { build_backend_cmd "$@"; }`.
- `parse_cli_args` learns `--codexu` / `--claudeu` / `--opencode` which set `BACKEND` before run.
- README documents the three backends and how they differ.

**Test:** `unit/backend.bats` asserts that `BACKEND=codexu build_backend_cmd | tr '\0' '\n' | head -1` is `codexu` etc.

**Commit message:** `feat(orchestrator): BACKEND adapter + --claudeu/--opencode CLI flags`

---

## Task 6 — Item #3: background companions with short timeout

**Files:**
- Modify: `gormes/scripts/orchestrator/lib/companions.sh`
- Modify: `gormes/scripts/orchestrator/tests/unit/companions.bats`
- Modify: `gormes/scripts/orchestrator/tests/integration/companion-trigger.bats`

**Change:**
- `run_companion` becomes fire-and-forget. Structure:
  ```bash
  run_companion() {
    local name="$1"
    local cmd; cmd="$(resolve_companion_cmd "$name")"
    local pid_file="$(companion_state_dir)/${name}.pid"
    local log_file="$LOGS_DIR/companion_${name}.$(date -u +%Y%m%dT%H%M%SZ).log"
    # Skip if already running
    if [[ -f "$pid_file" ]]; then
      local prev_pid; prev_pid="$(cat "$pid_file")"
      if proc_alive "$prev_pid"; then
        return 0
      fi
      rm -f "$pid_file"
    fi
    # Detached launch
    (
      setsid nohup bash -c "
        cd '$GIT_ROOT' && \
        AUTO_COMMIT=1 AUTO_PUSH=0 PLANNER_INSTALL_SCHEDULE=0 \
        timeout '${COMPANION_TIMEOUT_SECONDS:-600}' bash '$cmd'
      " >"$log_file" 2>&1 &
      echo $! > "$pid_file"
    )
    log_event "companion_${name}_started" null "pid=$(cat "$pid_file")" "started"
  }
  ```
- New `companion_reap_stale` called at top of `maybe_run_companions`: for each PID file, if process dead, record state + `rm -f pid_file`.
- `proc_alive` used to be in entry script; move to `lib/common.sh` if not already.
- Default `COMPANION_TIMEOUT_SECONDS` → 600 (10 min).

**Tests:**
- `unit/companions.bats`: `run_companion` returns <1s regardless of companion runtime; pid file created.
- `integration/companion-trigger.bats`: launch fake-companion that sleeps 5s → orchestrator cycle continues immediately; after cycle, reap sees it completed.

**Commit message:** `feat(orchestrator): background companions with detached launch + short timeout`

---

## Task 7 — Item #4: candidate-pool refill trigger (depends on Task 6)

**Files:**
- Modify: `gormes/scripts/gormes-auto-codexu-orchestrator.sh` (`run_once`)
- Modify: `gormes/scripts/orchestrator/lib/companions.sh` (add sync-mode to `run_companion`)
- Create: `gormes/scripts/orchestrator/tests/integration/candidate-refill.bats`

**Change:**
- `run_companion` gains optional `--sync` flag: when set, runs foreground (same as pre-Task-6 behavior), with `COMPANION_TIMEOUT_SECONDS` still applied.
- New helper in entry script: `maybe_refill_candidates`. Called in `run_once` right after `write_candidates_file`:
  ```bash
  maybe_refill_candidates() {
    local watermark="${CANDIDATE_LOW_WATERMARK:-5}"
    local count; count="$(candidate_count)"
    if (( count >= watermark )); then return 0; fi
    if [[ "${DISABLE_COMPANIONS:-0}" == "1" ]]; then return 0; fi
    echo "Candidate pool low ($count < $watermark); running planner companion sync"
    run_companion planner --sync || true
    write_candidates_file
    log_event "candidate_refilled" null "before=$count after=$(candidate_count)" "refilled"
  }
  ```
- Env: `CANDIDATE_LOW_WATERMARK` (default 5).

**Test:** `integration/candidate-refill.bats` — start with a progress fixture of 2 tasks + fake-planner that appends a new task to it; expect a `candidate_refilled` event with `after > before`.

**Commit message:** `feat(orchestrator): candidate-pool refill trigger via synchronous planner run`

---

## Task 8 — Item #7: audit CSV output

**Files:**
- Modify: `gormes/scripts/orchestrator/audit.sh`

**Change:**
- At the bottom of `audit.sh`, after writing the ndjson line, also append a CSV row to `$AUDIT_DIR/report.csv`.
- Columns: `ts,active,uptime_s,nrestarts,claimed,success,failed,promoted,cherry_pick_failed,productivity_pct,integration_head_short`.
- Write headers only if file doesn't yet exist.
- ndjson continues unchanged.

**Test:** manual smoke — run audit.sh twice, inspect `report.csv` for header + 2 data rows.

**Commit message:** `feat(orchestrator): audit.sh emits CSV for easy trending`

---

## Post-rollout

1. Push all 8 commits to origin/main.
2. `systemctl --user restart gormes-orchestrator` (so the new libs are in memory).
3. Watch `journalctl --user -u gormes-orchestrator -f` for 1 cycle (~15 min).
4. Run the audit manually and inspect `report.csv` + `runs.jsonl` for new granular statuses.
5. If audit shows `cherry_pick_failed` is gone and productivity rate is up, Oil Release 1 is done.

## Known follow-ups

- Oil Release 2: per-task marker files replacing monolithic progress.json edits (the "medium-term" option we deferred in #1).
- Companion-output visibility in ledger (currently only pid file tracks state).
- Notification when ERROR/WARN audit lines fire (Telegram or Slack).
