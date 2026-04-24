# Orchestrator Internals

> This directory is **commit-frozen**. See [FROZEN.md](./FROZEN.md) before
> changing any `.sh` under `lib/` or the entry script.

Companion libraries and tests for `gormes/scripts/gormes-auto-codexu-orchestrator.sh`.

## Layout

- `lib/` — sourced modules. Each file is side-effect-free; they declare functions that the entry script or tests call. Module docstrings at the top of each file list env vars they read.
- `tests/bootstrap.sh` — downloads and verifies vendored bats-core, bats-assert, bats-support into `tests/vendor/` (gitignored).
- `tests/run.sh unit` / `tests/run.sh integration` / `tests/run.sh unit integration` — test runner.
- `tests/fixtures/` — canned progress JSON, report markdown (good + 6 bad), and mock backend binaries (`fake-codexu`, `fake-planner`, `fake-doc-improver`, `fake-landingpage`).

## Running tests

```sh
make -C .. orchestrator-test        # unit only, <5s
make -C .. orchestrator-test-all    # unit + integration, <2min
```

## Adding a new library module

1. Create `lib/<name>.sh`. Start with the standard header (module doc + depends-on list).
2. Add a `unit/<name>.bats` test file, load `'../lib/test_env'` in setup, call `source_lib <name>` after `load_helpers`.
3. Add the new name to the `for _lib in ...` loop at the top of `gormes-auto-codexu-orchestrator.sh`.
4. Run `make orchestrator-test-all`.

## Backends

`lib/backend.sh` is the backend adapter. `BACKEND` (env var) or the equivalent CLI flag selects which agent CLI drives workers. The orchestrator's worker contract is unchanged across backends; each backend only translates argv.

| Backend | Binary | CLI flag | Notes |
|---|---|---|---|
| `codexu` (default) | `codexu` | `--codexu` | Native codex-cli non-interactive mode. |
| `claudeu` | `claudeu` shim (PATH) | `--claudeu` | Shim translates codexu-style argv to `claude --print`. |
| `opencode` | `opencode` | `--opencode` | Uses `opencode run --no-interactive`; shape approximate. |

Switch via env (`BACKEND=claudeu $0`) or flag (`$0 --claudeu`). CLI flag wins.

## Companion scheduling

The orchestrator's forever loop interleaves three companion scripts between cycles:

| Companion | Predicate | Typical cadence |
|---|---|---|
| `gormes-architecture-planner-tasks-manager.sh` | exhaustion (<10% candidates remain) OR cycles since last ≥ `PLANNER_EVERY_N_CYCLES` (default 4). Skipped if external systemd timer ran within `PLANNER_EVERY_N_CYCLES × LOOP_SLEEP_SECONDS × 2` seconds. | ~ every 4 cycles |
| `documentation-improver.sh` | cycles since last ≥ `DOC_IMPROVER_EVERY_N_CYCLES` (default 6) AND last cycle promoted ≥ 1 commit. | ~ every 6 productive cycles |
| `landingpage-improver.sh` | hours since last ≥ `LANDINGPAGE_EVERY_N_HOURS` (default 24). | daily |

Companions run serially on the integration worktree with `AUTO_COMMIT=1 AUTO_PUSH=0 PLANNER_INSTALL_SCHEDULE=0`, so their commits become the next cycle's `BASE_COMMIT`.

Escape hatches: `DISABLE_COMPANIONS=1` fully disables. `COMPANION_ON_IDLE=0` allows companions to run on any cycle (default `1` gates them to idle/post-promotion cycles).
