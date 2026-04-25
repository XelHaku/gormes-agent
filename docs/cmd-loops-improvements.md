# cmd/builder-loop and cmd/planner-loop — proposed improvements

Audit conducted 2026-04-25 against `cmd/builder-loop/` and `cmd/planner-loop/`
after the autoloop→builder-loop / architecture-planner-loop→planner-loop
rename.

Each item lists the rationale, current code reference, and a one-line outline
of the fix. Items are grouped by tier; lower-numbered items are higher
leverage.

## Status

| # | Title                                                                | Status       |
|---|----------------------------------------------------------------------|--------------|
| 1 | Break the planner→builder import (extract runner plumbing)           | done         |
| 2 | Replace mutually-exclusive backend flags with `--backend <name>`     | done         |
| 3 | Push env reads into `*.ConfigFromEnv` (kill cmd/-side allowlists)    | done         |
| 4 | Graceful shutdown via `signal.NotifyContext`                         | done         |
| 5 | Per-subcommand `--help`                                              | done         |
| 6 | `planner-loop doctor` actually diagnoses drift                       | open         |
| 7 | Show keywords in planner run summary                                 | done         |
| 8 | `planner-loop trigger <reason>` verb                                 | done         |
| 9 | Move `progress` and `repo` subcommands out of `builder-loop`         | open         |
| 10 | Collapse `progress write` to a table-driven loop                    | open         |
| 11 | Replace package-level test-seam globals with a `cliDeps` struct     | open         |
| 12 | Structured exit codes                                                | partial      |
| 13 | `--format json` for read-only commands                               | open         |
| 14 | `--repo-root` / `REPO_ROOT` flag                                     | done         |
| 15 | `digest --output` should refuse to clobber unless `--force`          | done         |

## Highest leverage

### 1. Break the planner→builder import

`cmd/planner-loop/main.go` and `internal/plannerloop/*.go` import
`internal/builderloop` only for the `Runner`, `ExecRunner`, `FakeRunner`,
`Command`, `Result`, and `ErrUnexpectedCommand` types. That is plumbing, not
domain — and creates a backwards dependency from planning into building.

Domain dependencies (`builderloop.Candidate`, `builderloop.LedgerEvent`,
`builderloop.MergeOpenPullRequests`, `builderloop.PRConflictAction*`) are
legitimate and stay.

**Fix.** Move the runner plumbing types into a new `internal/cmdrunner`
package. Replace builderloop's `runner.go` definitions with type aliases so
existing `builderloop.Runner` references inside builderloop continue to
compile. Update plannerloop and both cmd/ binaries to depend on
`internal/cmdrunner` directly.

### 2. Replace mutually-exclusive backend flags with `--backend <name>`

`parseRunOptions` in both binaries hand-rolls `--codexu | --claudeu |
--opencode`. Every new backend means a parser change in two places. Today,
that means `cmd/builder-loop/main.go:108-128` and
`cmd/planner-loop/main.go:108-141`.

**Fix.** Single `--backend <name>` flag with validation against an allowlist.
The existing `BACKEND` environment variable continues to work as an override
fallback. Update tests; collapse `TestRunBackendFlagUsesClaudeu` and friends
into a table-driven case.

### 3. Push env reads into `*.ConfigFromEnv` itself

`autoloopEnv()` (`cmd/builder-loop/main.go:269`) and `plannerEnv()`
(`cmd/planner-loop/main.go:143`) maintain hand-edited allowlists of 14 and 17
environment variable names respectively. Every new env knob (and there are
many — see today's commits) requires editing these allowlists or the new knob
silently has no effect.

**Fix.** Change `ConfigFromEnv(repoRoot, env map[string]string)` to
`ConfigFromEnv(repoRoot string, lookup func(string) string)`. cmd/ binaries
pass `os.Getenv` directly. Tests pass a closure over a test map. The cmd/
allowlist functions are deleted.

## Operator UX

### 4. Graceful shutdown

`runAutoloop` and `plannerloop.RunOnce` are invoked with
`context.Background()`. SIGINT/SIGTERM during a 20-30 minute backend run
cannot propagate cleanly — systemd resorts to SIGKILL when the unit's
TimeoutStopSec expires.

**Fix.** Wire `signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)`
once in each `main`, pass the resulting context through. The internal
packages already accept `context.Context` and forward it to `exec.CommandContext`.

### 5. Per-subcommand `--help`

`builder-loop` only honors `--help` under `run`; everything else prints the
full single-line `usage` const via `fmt.Errorf(usage)` on stderr — which
also routes legitimate parse errors as if the user had asked for help.

**Fix.** Split usage into per-verb strings. Route `--help`/`-h` for every
subcommand. Distinguish help (stdout, exit 0) from parse errors
(`error: <message>\n\n<usage>` on stderr, exit 2).

### 6. `planner-loop doctor` actually diagnoses drift

`doctor` (`cmd/planner-loop/main.go:313`) only `os.Stat`s a few directories
and looks up the backend on PATH. It cannot detect "the loop has been
running but not progressing" — the actual operational concern.

**Fix.** Add checks for: `progress.json` parses, `PLANNER_TRIGGERS_PATH` is
writable, last `health_updated` event in the planner ledger is fresher than
2× `PLANNER_INTERVAL`, systemd timer is loaded if installed. Mirror as
`builder-loop doctor` (currently absent).

### 7. Show keywords in planner run summary

`printRunSummary` (`cmd/planner-loop/main.go:176`) omits
`summary.Keywords` even though they meaningfully steer planner behavior. An
operator running `planner-loop run hermes-issues` cannot confirm from the
output that topical mode actually engaged.

**Fix.** Echo `keywords: hermes-issues` in the summary when present.

### 8. `planner-loop trigger <reason>`

There is no first-class verb for manually firing the planner. Operators
either wait for the timer or `touch` `PLANNER_TRIGGERS_PATH` by hand.

**Fix.** Add `planner-loop trigger <reason>` that appends a single-line
JSON event to the trigger ledger with `ts`, `reason`, and `source: "manual"`.
The .path watcher fires immediately. Reason gets surfaced in the next planner
prompt as the trigger label.

## Structure

### 9. Move `progress` and `repo` subcommands out of `builder-loop`

`cmd/builder-loop/progress.go` and `cmd/builder-loop/repo.go` do not
exercise the loop. They are doc-generation and repo-maintenance utilities
that just happen to share the binary. This forces `builder-loop` to import
`internal/progress` and `internal/repoctl`, expanding what changes when the
loop changes.

**Fix.** Promote to `cmd/progress` and `cmd/repoctl`, or fold all verbs
under a single `cmd/gormes <verb>` parent. Update Makefile,
documentation, and any wrapper scripts.

### 10. Collapse `progress write` to a table-driven loop

`cmd/builder-loop/progress.go:42-94` is nine near-identical
`if err := rewriteProgressMarker(...); err != nil` blocks.

**Fix.** Define
`[]struct{ path, kind, label string; render func(*progress.Progress) string }`
and drive it with a single loop. Adding a new generated marker becomes a
one-liner.

## Polish

### 11. Replace package-level test-seam globals with a `cliDeps` struct

`commandStdout` and `commandRunner`/`serviceRunner` are mutable
package-level vars used as test seams (cmd/builder-loop/main.go:14-15,
cmd/planner-loop/main.go:16-17). They prevent `t.Parallel()` and require
`t.Cleanup` dance in every test.

**Fix.** A `cliDeps` struct (or interface) carrying `stdout`, `stderr`,
`runner`, and `env` passed into `run(args, deps)`. Tests construct their
own deps; production constructs from `os.Stdout`/`os.Stderr`/`ExecRunner{}`.

### 12. Structured exit codes

Every error path funnels through `os.Exit(1)`. systemd `Restart=on-failure`
and `OnFailure=` directives cannot distinguish "config error" (don't retry)
from "no candidates this cycle" (retry quickly) from "backend timeout"
(retry with backoff) from "verify failed" (page).

**Fix.** Define a small `exitCode` enum (config=2, no-work=10,
backend-timeout=20, verify-failed=30, internal=1) and `os.Exit` with the
right value.

### 13. `--format json` for read-only commands

`digest`, `audit`, `status`, `doctor`, and `progress validate` print human
prose. External tooling that wants this state currently parses
`runs.jsonl` directly because there is no machine-readable cmd surface.

**Fix.** Each command grows a `--format text|json` flag (default text).
JSON output is documented and stable enough to script against.

### 14. `--repo-root` / `REPO_ROOT` flag

Both binaries hard-fail if `os.Getwd()` is not the repo root. The systemd
units rely on `WorkingDirectory=` being set correctly — a fragile
implicit dependency.

**Fix.** Accept `--repo-root <path>` and `REPO_ROOT` env. systemd units
set this explicitly rather than depending on `WorkingDirectory=`.
Smoke-testing from anywhere becomes possible.

### 15. `digest --output` should refuse to clobber unless `--force`

`os.WriteFile(outputPath, ..., 0o644)` (`cmd/builder-loop/main.go:65`)
silently overwrites whatever path the caller provides. Low-stakes
locally, but the CLI sets a precedent.

**Fix.** Use `os.OpenFile` with `O_CREATE|O_EXCL|O_WRONLY`. Add `--force`
to opt into overwrite.
