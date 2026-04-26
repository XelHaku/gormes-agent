# Builder Loop Internals

The orchestrator wrapper and CLI implementation now live in Go under
`cmd/builder-loop` and `internal/builderloop`. This directory contains
transitional wrappers, systemd templates, and historical notes for the old
shell entrypoints. Full runtime parity remains staged follow-up work.

## Layout

- `*.sh` — tiny compatibility wrappers that exec `go run ./cmd/builder-loop ...`
  for implemented Go commands.
- `systemd/` — templates rendered or installed by `builder-loop service ...`.
- `FROZEN.md` — freeze policy and the active Go-port exception.

## Watchdog

Install the 10-minute production watchdog with:

```sh
scripts/orchestrator/install-watchdog.sh --force
```

The watchdog is intentionally repair-oriented: every tick checkpoints dirty
loop output, restarts `gormes-orchestrator.service` if inactive, runs
`builder-loop doctor` and `planner-loop doctor`, writes the existing audit
report, and exits zero so the timer keeps firing after recoverable failures.

## Running tests

```sh
go test ./internal/builderloop ./cmd/builder-loop -count=1
```

## Legacy shell

Long-form frozen shell retained for parity lives under
`testdata/legacy-shell/scripts/` and is marked vendored for language reporting.
The root `scripts/gormes-auto-codexu-orchestrator.sh` wrapper runs the Go port
for default execution, backend flags, help, and implemented Go subcommands.
Legacy management/resume invocations (`status`, `tail`, `abort`, `cleanup`,
`promote-commit`, `verify-gh-auth`, and `--resume`) temporarily exec the
vendored shell with the original arguments until full runtime parity lands.

The live companion scripts `scripts/gormes-architecture-planner-tasks-manager.sh`,
`scripts/documentation-improver.sh`, and `scripts/landingpage-improver.sh`
remain shell outside this cutover.

## Backends

`internal/builderloop` owns backend adapters. `BACKEND` (env var) or the equivalent
CLI flag selects which agent CLI drives workers. The worker contract is
unchanged across backends; each backend only translates argv.

| Backend | Binary | CLI flag | Notes |
|---|---|---|---|
| `codexu` (default) | `codexu` | `--backend codexu` | Native codex-cli non-interactive mode. |
| `claudeu` | `claudeu` shim (PATH) | `--backend claudeu` | Shim translates codexu-style argv to `claude --print`. |
| `opencode` | `opencode` | `--backend opencode` | Uses `opencode run --no-interactive`; shape approximate (builder-loop only). |

Switch via env (`BACKEND=claudeu $0`) or flag (`$0 --backend claudeu`). CLI flag wins.

## Companion scheduling

The legacy orchestrator loop interleaves three companion scripts between cycles.
The Go port has typed companion scheduling primitives, but full runtime wiring
remains staged:

| Companion | Predicate | Typical cadence |
|---|---|---|
| `gormes-architecture-planner-tasks-manager.sh` | exhaustion (<10% candidates remain) OR cycles since last ≥ `PLANNER_EVERY_N_CYCLES` (default 4). Skipped if external systemd timer ran within `PLANNER_EVERY_N_CYCLES × LOOP_SLEEP_SECONDS × 2` seconds. | ~ every 4 cycles |
| `documentation-improver.sh` | cycles since last ≥ `DOC_IMPROVER_EVERY_N_CYCLES` (default 6) AND last cycle promoted ≥ 1 commit. | ~ every 6 productive cycles |
| `landingpage-improver.sh` | hours since last ≥ `LANDINGPAGE_EVERY_N_HOURS` (default 24). | daily |

Companions run serially on the integration worktree with `AUTO_COMMIT=1 AUTO_PUSH=0 PLANNER_INSTALL_SCHEDULE=0`, so their commits become the next cycle's `BASE_COMMIT`.

Escape hatches: `DISABLE_COMPANIONS=1` fully disables. `COMPANION_ON_IDLE=0` allows companions to run on any cycle (default `1` gates them to idle/post-promotion cycles).
