# Orchestrator Test Harness + Modular Refactor + Companion Seam

**Status:** Draft
**Author:** xel
**Date:** 2026-04-23
**Spec ID:** C (of the C → A → B sequence)

## Context

`gormes/scripts/gormes-auto-codexu-orchestrator.sh` is a 2,034-line bash orchestrator running in a forever loop, spawning up to 4 parallel `codexu exec` workers against isolated git worktrees, then cherry-picking successful commits onto an integration branch.

A 24-hour effectiveness audit from `runs.jsonl` (2026-04-22 18:25 → 2026-04-23 18:25) showed:

| Metric | Count | Note |
|---|---|---|
| Runs started | 71 | |
| Worker claims | 264 | |
| Worker reported success | 94 (36%) | |
| **Cherry-pick failed on success** | **34 of 43 attempts (79%)** | Dominant waste |
| Worker hard failure | 122 (46%) | 119 `contract_or_test_failure` + 3 timeouts |
| Actually promoted commits | 43 in 24h | ~1.8/hour across 4 parallel agents |
| Productive claim rate | **~16%** | |

Same task claimed up to 28 times without landing (e.g. `2/2.B.4/Pairing, reconnect, and send contract`).

Root pains (ranked):
1. Cherry-pick conflicts on the "happy path" (workers all edit `progress.json` / phase docs from the same `BASE_COMMIT`).
2. Retry storm on impossible tasks — no failure memory / poison-pill / backoff.
3. Phase-level lock + candidate sort piles workers onto the same phase, leaving 2-3 of 4 workers idle.
4. Identical prompts across 264 attempts — retries don't learn from prior failure signals.
5. Untested 2,034-line bash script makes every change risky.

The user chose sequence **D: C → A → B**. This spec covers **C**: test harness + minimal modular refactor + periodic-companion invocation seam. No runtime behavior change except the new companion seam (gated behind a default-conservative flag).

Specs A (effectiveness fixes — serialized/rebased promotion, poison-pill cap, candidate re-ranking, per-worker file-scope partitioning) and B (`--claudeu` / `--opencode` backend adapter) follow in separate specs and gate on C's tests.

## Goals

1. Ship a bats-based test harness (unit + 3-4 integration smoke tests) so Specs A and B can be developed TDD-style.
2. Extract 6 modules from the monolith — only the ones needed to make the 24h pain points unit-testable. No behavior change in existing code paths.
3. Add a new `lib/companions.sh` seam that periodically invokes the three existing companion scripts (`gormes-architecture-planner-tasks-manager.sh`, `documentation-improver.sh`, `landingpage-improver.sh`) on the integration worktree with smart cadence. This both normalizes state between cycles (reducing conflicts for Spec A) and centralizes the scheduling that previously relied on systemd timers.
4. Keep the currently-running orchestrator working — each commit passes tests, production forever-loop picks up new libs via `source` on next cycle.

## Non-goals

- Fixing cherry-pick conflicts (Spec A).
- Adding `--claudeu` / `--opencode` backend flags (Spec B).
- Changing the prompt format or TDD contract.
- Renaming or relocating the orchestrator entry script.
- Changing `runs.jsonl` schema.
- Adding `shfmt` auto-format (may add later).

## Architecture

### Module split

Entry point stays at `gormes/scripts/gormes-auto-codexu-orchestrator.sh`. It keeps:
- `main`, `parse_cli_args`, `usage`
- `run_once`, `run_worker`, `run_worker_resume`
- `heartbeat_loop`, `emit_heartbeat_json`, `get_worker_task`, `read_progress_summary`
- `cmd_status`, `cmd_tail`, `cmd_abort`, `cmd_cleanup`, `cmd_promote_commit`
- `claim_run_lock`, `release_run_lock`, `reset_run_scope`, `fresh_run_id`
- All env var defaults and the top-level `source` statements for the libs below.

New sourced libs under `gormes/scripts/orchestrator/lib/`:

| Module | Functions | Why extractable |
|---|---|---|
| `common.sh` | `log_info`, `log_debug`, `log_warn`, `log_error`, `safe_path_token`, `require_cmd`, `classify_worker_failure`, `show_progress`, `available_mem_mb` | Pure, no side effects beyond stdio |
| `candidates.sh` | `normalize_candidates`, `write_candidates_file`, `candidate_count`, `candidate_at`, `task_slug` | Pure jq transforms over a file path |
| `report.sh` | `build_prompt`, `collect_final_report_issues`, `verify_final_report`, `extract_report_field`, `extract_report_commit`, `extract_report_branch`, `extract_session_id`, `wait_for_valid_final_report`, `print_final_report_diagnostics` | Pure string/regex parsing; takes file paths |
| `claim.sh` | `claim_task`, `release_task`, `cleanup_stale_locks` | Fixturable with tmp `LOCKS_DIR` |
| `worktree.sh` | `create_worker_worktree`, `maybe_remove_worker_worktree`, `enforce_worktree_dir_cap`, `verify_worker_commit`, `worker_branch_name`, `worker_worktree_root`, `worker_repo_root`, `branch_worktree_path` | Fixturable with tmp git repo |
| `promote.sh` | `setup_integration_root`, `promote_successful_workers`, `push_integration_branch`, `cmd_promote_commit`, `promotion_enabled` | Fixturable with tmp git repo; isolates the 79%-failure surface |
| `companions.sh` | See §Companion seam below | New |

Everything else (`run_once`, `run_worker`, heartbeat loop, CLI routing, logging setup) stays inline. **This is the tight list** — not a full bash re-architecture.

Entry script gains one block near the top:

```bash
ORCHESTRATOR_LIB_DIR="${ORCHESTRATOR_LIB_DIR:-$SCRIPT_DIR/orchestrator/lib}"
# shellcheck source=/dev/null
for _lib in common candidates report claim worktree promote companions; do
  source "$ORCHESTRATOR_LIB_DIR/${_lib}.sh"
done
unset _lib
```

### Companion seam

New module `orchestrator/lib/companions.sh` exposes:

- `companion_state_dir` — defaults to `$RUN_ROOT/companions/`, contains `<name>.last.json` files tracking (`ts`, `rc`, `cycle`, `promoted_commit_count`) per companion.
- `companion_last_ts <name>` — returns epoch of last successful run, `0` if never.
- `companion_cycles_since <name>` — cycles since last run.
- `should_run_planner` — true if:
  - `unclaimed_candidates / total < 0.10` (exhaustion trigger), OR
  - `companion_cycles_since planner >= PLANNER_EVERY_N_CYCLES`
  - AND external systemd-timer run (tracked in `$PLANNER_ROOT/planner_state.json`) is older than `PLANNER_EVERY_N_CYCLES * LOOP_SLEEP_SECONDS * 2` seconds (no double-run).
- `should_run_doc_improver` — true if `cycles_since >= DOC_IMPROVER_EVERY_N_CYCLES` AND `last_cycle_promoted_commits >= 1` (skip noop docs passes).
- `should_run_landingpage` — true if `hours_since >= LANDINGPAGE_EVERY_N_HOURS`.
- `run_companion <name>` — wraps the configured companion command with `timeout $COMPANION_TIMEOUT_SECONDS`, `cd $GIT_ROOT` (integration worktree after `setup_integration_root`), and a scoped env: `AUTO_COMMIT=1 AUTO_PUSH=0 PLANNER_INSTALL_SCHEDULE=0` (the last prevents the planner from re-installing its systemd timer on every invocation — the external timer remains the authority for that side effect). Captures exit code and updates state file. Emits `log_event` with `companion_*` event names (`companion_started`, `companion_succeeded`, `companion_failed`, `companion_skipped`).
- `maybe_run_companions <cycle> <promoted_last_cycle>` — called by `main` loop between `run_once` and `sleep LOOP_SLEEP_SECONDS`. Order: planner → doc-improver → landing-page. Each gated by its predicate. Serial (all edit overlapping files).

Integration point in `main()`:

```bash
while true; do
  cycle=$((cycle + 1))
  reset_run_scope "$cycle"
  ...
  run_once
  cycle_rc="$?"
  maybe_run_companions "$cycle" "$PROMOTED_LAST_CYCLE"
  # Exhaustion trigger skips sleep when planner just refreshed candidates
  if (( EXHAUSTION_TRIGGERED == 1 )); then
    EXHAUSTION_TRIGGERED=0
    continue
  fi
  sleep "$LOOP_SLEEP_SECONDS"
done
```

`promote_successful_workers` is updated to export `PROMOTED_LAST_CYCLE` (count of successfully promoted commits this cycle). Companion predicates read it.

### Data flow

```
     ┌─────────────────┐
     │  main() loop    │
     └────────┬────────┘
              │ cycle N
              ▼
     ┌─────────────────┐     claim locks       ┌──────────────┐
     │   run_once      │────────────────────▶  │  4 workers   │
     │                 │◀──────── promote ──── │ (parallel)   │
     └────────┬────────┘                       └──────────────┘
              │ promoted_count
              ▼
     ┌─────────────────────────────────────────┐
     │ maybe_run_companions(cycle, promoted)   │
     │   ┌───────────────┐                     │
     │   │ should_run_X? │── yes ─▶ run_companion
     │   └───────────────┘                     │
     │   order: planner → doc → landing        │
     │   serial on integration worktree        │
     │   edits auto-committed as next BASE     │
     └─────────────────────────────────────────┘
              │
              ▼
     ┌─────────────────┐
     │  sleep / loop   │
     └─────────────────┘
```

### Env vars (additions only)

```
# Companion scheduling
DISABLE_COMPANIONS=0              # escape hatch; 1 = fully disable seam
COMPANION_ON_IDLE=1               # only trigger when main cycle was idle/exhausted
COMPANION_TIMEOUT_SECONDS=1800
PLANNER_EVERY_N_CYCLES=4
DOC_IMPROVER_EVERY_N_CYCLES=6
LANDINGPAGE_EVERY_N_HOURS=24

# Command overrides (default: resolve relative to SCRIPT_DIR)
COMPANION_PLANNER_CMD=            # default: $SCRIPT_DIR/gormes-architecture-planner-tasks-manager.sh
COMPANION_DOC_IMPROVER_CMD=       # default: $SCRIPT_DIR/documentation-improver.sh
COMPANION_LANDINGPAGE_CMD=        # default: $SCRIPT_DIR/landingpage-improver.sh

# Testing hook
ORCHESTRATOR_LIB_DIR=             # override lib path; tests point to fixture copies
```

No existing env var changes behavior. All new vars default-safe (companions run on cadence that won't thrash a 24h loop).

## Test harness

### Layout

```
gormes/scripts/orchestrator/tests/
├── bootstrap.sh                    # downloads + caches bats-core, bats-assert, bats-support
├── run.sh                          # entry: PATH setup + bats invocation
├── vendor/                         # gitignored, populated by bootstrap
│   ├── bats-core/
│   ├── bats-assert/
│   └── bats-support/
├── unit/
│   ├── common.bats
│   ├── candidates.bats
│   ├── report.bats
│   ├── claim.bats
│   ├── worktree.bats
│   ├── promote.bats
│   └── companions.bats
├── integration/
│   ├── happy-path.bats
│   ├── cherry-pick-conflict.bats
│   ├── poison-task-retry.bats      # initially @skip with TODO → Spec A target
│   ├── resume.bats
│   └── companion-trigger.bats
└── fixtures/
    ├── fake-codexu                 # mock backend bash script
    ├── fake-planner                # writes marker + exits; asserts inputs via env
    ├── fake-doc-improver
    ├── fake-landingpage
    ├── progress.fixture.json       # deterministic 3 phases × 2 items
    ├── planner_state.fixture.json
    └── reports/
        ├── good.final.md
        ├── bad-missing-section.final.md
        ├── bad-no-commit-hash.final.md
        ├── bad-all-zero-exits.final.md
        ├── bad-no-red-exit.final.md
        ├── bad-missing-branch.final.md
        └── bad-empty.final.md
```

### `bootstrap.sh` contract

- Downloads pinned versions of bats-core (`v1.11.0`), bats-assert (`v2.1.0`), bats-support (`v0.3.0`) into `vendor/` using `curl` + tarball extraction.
- Idempotent — checks vendor presence before downloading.
- `checksum` step validates SHA256 of each tarball.
- No root install, no network access required after first run.
- Offline-mode: if `$BATS_OFFLINE=1`, fails loudly instead of trying network.

### `fake-codexu` contract

Bash script on `PATH` (via `tests/fixtures/bin/:$PATH`) with same CLI signature as `codexu exec --json -c ... --output-last-message <file> <prompt>`. Behavior controlled by `FAKE_CODEXU_MODE`:

| Mode | Effect |
|---|---|
| `success` | Writes valid `final.md` with RED/GREEN/REFACTOR/REGRESSION + Commit hash, makes 1 file edit + commit, exits 0 |
| `contract_fail` | Returns 1, writes incomplete `final.md` (missing REFACTOR section) |
| `timeout` | `sleep $((WORKER_TIMEOUT_SECONDS + 60))` — triggers GNU timeout 124 |
| `dirty_worktree` | Edits file but doesn't commit; leaves status dirty |
| `no_commit_advance` | Commits but doesn't change HEAD (empty commit disabled, so `--allow-empty` then no-advance sentinel) |
| `conflict` | Edits `progress.fixture.json` on a line another worker also edits, so cherry-pick conflicts |
| `bad_report_no_red` | Writes report with all zero exits (no failing RED) |

### Integration smoke tests

1. **`happy-path.bats`**
   - Setup: tmp git repo with `progress.fixture.json`, fake-codexu=success on PATH.
   - Run: `ORCHESTRATOR_ONCE=1 MAX_AGENTS=1 <orchestrator>`.
   - Assert: 1 commit lands on integration branch, `runs.jsonl` has `worker_success` + `worker_promoted`, exit 0.

2. **`cherry-pick-conflict.bats`**
   - Setup: two workers both configured to edit the same line of `progress.fixture.json` (fake-codexu=conflict).
   - Run: `ORCHESTRATOR_ONCE=1 MAX_AGENTS=2 <orchestrator>`.
   - Assert: worker 1 promotes; worker 2 emits `worker_promotion_failed / cherry_pick_failed`; integration branch head matches worker 1's commit; no cherry-pick state leaks (`.git/CHERRY_PICK_HEAD` absent).

3. **`poison-task-retry.bats`** — `@skip "Spec A target"`
   - Documents the regression gate. When Spec A lands the cap, this test gets enabled.

4. **`resume.bats`**
   - Setup: start orchestrator with `fake-codexu=timeout`, kill mid-flight via SIGTERM to heartbeat.
   - Run: `<orchestrator> --resume <run_id>` with fake-codexu=success.
   - Assert: workers resume with correct task slugs, no duplicate claims in `runs.jsonl`.

5. **`companion-trigger.bats`**
   - Setup: fake-planner/doc/landing on PATH, force cycle counters via env (`PLANNER_EVERY_N_CYCLES=1`, `DOC_IMPROVER_EVERY_N_CYCLES=1`, `LANDINGPAGE_EVERY_N_HOURS=0`).
   - Run: 2 forever-loop cycles (patched to exit after 2).
   - Assert: planner marker file exists; doc-improver marker exists only if a promotion happened; landing-page marker exists; state files updated with correct timestamps; `runs.jsonl` has `companion_*` events.

### Unit test coverage targets

- `common.bats` — `classify_worker_failure` (124→timeout, 137→killed, 1→contract, other→error); `safe_path_token` (special chars, unicode); `available_mem_mb` (regex guard).
- `candidates.bats` — `normalize_candidates` against 4 fixture progress files: happy, empty, all-complete, mixed statuses, `ACTIVE_FIRST=0/1` ordering.
- `report.bats` — `collect_final_report_issues` against 1 good + 6 bad fixtures; `extract_report_commit`/`extract_report_branch` with backticks and without.
- `claim.bats` — `claim_task` acquires both lockfiles; re-entry returns 1; `release_task` frees FDs; `cleanup_stale_locks` removes dead-pid locks, keeps live-pid locks.
- `worktree.bats` — `create_worker_worktree` creates branch from `BASE_COMMIT`; `verify_worker_commit` rejects: unchanged HEAD, >1 commit, dirty status, scope-escaping file, missing report-commit-match, branch mismatch.
- `promote.bats` — `promote_successful_workers` cherry-picks one success, records `worker_promoted`; on conflict aborts cleanly + records `cherry_pick_failed`; skips already-promoted commits.
- `companions.bats` — each `should_run_*` predicate matrix (last-ts old/new × cycles old/new × promoted 0/≥1 × exhaustion flag true/false).

### Make targets

Added to `gormes/Makefile`:

```makefile
orchestrator-test:
	@bash scripts/orchestrator/tests/run.sh unit

orchestrator-test-all:
	@bash scripts/orchestrator/tests/run.sh unit integration

orchestrator-lint:
	@if command -v shellcheck >/dev/null 2>&1; then \
	  shellcheck scripts/gormes-auto-codexu-orchestrator.sh scripts/orchestrator/lib/*.sh; \
	else \
	  echo "shellcheck not installed; skipping"; \
	fi
```

## TDD commit sequence (each commit green)

1. Add `scripts/orchestrator/tests/bootstrap.sh`, vendor pin docs, `.gitignore` for `vendor/`, empty `run.sh`, trivial `unit/noop.bats` that asserts `true`. `make orchestrator-test` exits 0.
2. Extract `lib/common.sh` (log, safe_path_token, require_cmd, classify_worker_failure, show_progress, available_mem_mb). Add `unit/common.bats`. Source from entry script. Run all tests; run `ORCHESTRATOR_ONCE=1` smoke once against a tmp repo.
3. Extract `lib/candidates.sh` (normalize_candidates, write_candidates_file, candidate_count, candidate_at, task_slug). Add `unit/candidates.bats` with 4 fixture progress files.
4. Extract `lib/report.sh`. Add `unit/report.bats` with 1 good + 6 bad fixtures.
5. Extract `lib/claim.sh`. Add `unit/claim.bats` with tmp `LOCKS_DIR`.
6. Extract `lib/worktree.sh`. Add `unit/worktree.bats` with tmp git repo fixture.
7. Extract `lib/promote.sh`. Add `unit/promote.bats` with tmp git repo fixture (happy + conflict).
8. Add `fixtures/fake-codexu` + `integration/happy-path.bats`. Add `make orchestrator-test-all`.
9. Add `integration/cherry-pick-conflict.bats`.
10. Add `integration/resume.bats` and skipped `integration/poison-task-retry.bats`.
11. Add `lib/companions.sh` skeleton (predicates + state I/O, no invocation). Add `unit/companions.bats`.
12. Add `fake-planner`/`fake-doc-improver`/`fake-landingpage` + `integration/companion-trigger.bats`. Wire `maybe_run_companions` into `main` loop, gated by `DISABLE_COMPANIONS=0` default. Update `promote_successful_workers` to export `PROMOTED_LAST_CYCLE`.
13. Add Makefile targets. Update orchestrator `usage()` with new env vars. Add `scripts/orchestrator/README.md` explaining layout, companion cadence, and how to add a new module.
14. Add commit-hook smoke: on CI `make orchestrator-test` runs; on full pushes `make orchestrator-test-all`. (Actual CI wiring deferred — just doc the target.)

Each commit updates `docs/content/building-gormes/architecture_plan/progress.json` under the orchestrator subphase so the orchestrator itself can track its own refactor.

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| Prod orchestrator is running now — `source` path must stay stable | Each commit keeps the `source` block at the top; prod loop picks up changes at next cycle boundary |
| Refactor silently breaks a function extracted to a lib | Every extraction adds unit tests for that module before the next one starts |
| `fake-codexu` drift from real `codexu exec` signature | One integration test runs against real `codexu --version` presence and asserts signature compatibility; fake-codexu documents supported flags |
| Companion seam runs too often and thrashes | Default cadences are conservative (planner every 4 cycles, docs every 6 cycles with promotion gate, landing every 24h). `DISABLE_COMPANIONS=1` is an escape hatch. `COMPANION_ON_IDLE=1` (default) further restricts triggering to cycles with no claimable tasks or completed early. Planner-state file (`$PLANNER_ROOT/planner_state.json`) prevents double-run when the external systemd timer already ran recently |
| `bats` network download in bootstrap.sh fails in sandboxed CI | Pinned checksums; `BATS_OFFLINE=1` fails loudly; tarballs vendorable into repo later if needed |
| Shell portability (GNU vs BSD) | Targeting Linux only (user is on Linux); `timeout`, `sed -i`, `find -printf` etc. stay GNU. Documented in README. |

## Acceptance criteria

- `make orchestrator-test` passes in <5s.
- `make orchestrator-test-all` passes in <2 minutes.
- `diff` between old `gormes-auto-codexu-orchestrator.sh` and new entry script shows only: header sourcing libs, removal of extracted function bodies, companion-seam hook in `main`, and `PROMOTED_LAST_CYCLE` export in `promote_successful_workers`. No logic changes to `run_worker`, `run_once`, heartbeat, CLI routing, or prompt generation.
- One 30-minute soak run of the forever loop against the real repo produces the same `runs.jsonl` event schema as before the refactor (companion events additive only).
- `shellcheck` reports no new warnings on the entry script or any new lib module.

## Out of scope (deferred)

- Spec A: effectiveness fixes (promotion serialization, poison-pill, candidate re-ranking).
- Spec B: `--claudeu` / `--opencode` backend adapter.
- `shfmt` automated formatting.
- CI wiring beyond providing Make targets.
- Migration of companion-script scheduling entirely away from systemd (they still keep their own timers; the orchestrator only *additionally* coordinates them).

## Follow-ups (Spec A preview — not in scope)

- Enable `integration/poison-task-retry.bats`.
- Add per-task failure-count memory in state.
- Rebase workers onto integration HEAD before promotion instead of cherry-picking.
- Partition `progress.json` edits by phase-file to let 4 workers touch non-overlapping files.
- Add failure-context to prompt on retry (prior stderr tail, prior exit code, prior diff).
