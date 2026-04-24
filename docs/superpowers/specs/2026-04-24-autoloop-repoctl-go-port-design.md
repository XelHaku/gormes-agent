# Autoloop + Repoctl Go Port Design

**Status:** Repoctl cut over; autoloop Go CLI/primitives staged
**Author:** Codex
**Date:** 2026-04-24

## Context

Gormes is now flattened to the repository root and is mostly Go by tracked
bytes, but shell still accounts for a large visible slice of the repository.
The shell is not the user-facing runtime. It is mostly repo self-development
automation:

1. The autoloop/orchestrator that uses agent CLIs such as `codexu`, `claudeu`,
   and `opencode` to develop `gormes-agent` from `progress.json` work items.
2. Audit, digest, systemd, and companion scheduler scripts around that loop.
3. Small repo-maintenance scripts for benchmark, progress, README, and Go
   compatibility checks.

The existing orchestrator directory is commit-frozen for casual refactors.
This port is an explicit user-requested feature, so it qualifies as a freeze
exception. The port must improve language stats and reliability without
pretending shell never existed: legacy behavior remains the parity oracle until
the Go replacement proves equivalent.

## Goals

1. Port all production shell automation to Go under a single coordinated port
   effort.
2. Keep `gormes` as the user-facing runtime binary; automation must not bloat
   or complicate that command.
3. Replace the autoloop/orchestrator runtime with a Go binary named
   `autoloop`.
4. Replace repo-maintenance shell scripts with a Go binary named `repoctl`.
5. Keep temporary shell wrappers or legacy fixtures only long enough to prove
   parity.
6. Improve reliability by replacing stringly shell pipelines and `jq`-driven
   state mutation with typed Go data structures and tests.
7. Improve GitHub language stats honestly by moving legacy long-form shell into
   parity fixtures marked as vendored, then deleting it after cutover.

## Non-goals

- Changing the public `gormes` runtime behavior.
- Removing external dependencies that are true runtime requirements for
  automation, such as `git`, selected agent CLIs, `gh` when PR mode is enabled,
  or `systemctl` when installing user services.
- Replacing GitHub, git, or systemd APIs with hosted services.
- Rewriting the progress schema as part of this port.
- Hiding active production logic from GitHub Linguist. Vendored marking is only
  for frozen legacy parity fixtures, not for live Go or shell wrappers.

## Decision Summary

The accepted design is partially cut over:

1. `cmd/autoloop` provides the autoloop CLI, wrappers, audit/digest/service
   commands, and typed primitives for candidates, locks, worktrees, promotion,
   companions, and backends.
2. `cmd/repoctl` replaces build/progress/README/compat maintenance scripts.
3. Full `autoloop run` runtime parity is still staged follow-up work: the
   current Go command is a run skeleton over candidate/backend invocation and
   does not yet wire the full legacy loop, worktree, claim, ledger, promotion,
   and companion runtime end-to-end.
4. `internal/autoloop` owns typed building blocks for the loop runtime, ledger,
   candidates, locks, worktrees, promotion, backend adapters, companions, audit,
   digest, and systemd rendering.
5. `internal/repoctl` owns benchmark, progress, README, and compatibility
   maintenance logic.
6. Repoctl and orchestrator entrypoints under `scripts/` are tiny compatibility
   wrappers or test harnesses. The long-form companion scripts
   `scripts/gormes-architecture-planner-tasks-manager.sh`,
   `scripts/documentation-improver.sh`, and `scripts/landingpage-improver.sh`
   remain live shell outside this cutover and need a later port.
7. Long legacy shell lives under `testdata/legacy-shell/` as parity fixtures and
   is marked `linguist-vendored` in `.gitattributes`.
8. The production target remains Go-first automation. New or changed
   orchestrator/repoctl shell under `scripts/` must stay a tiny wrapper that
   execs a Go command unless it is one of the explicitly pending companion
   scripts.

## Command Surface

### `autoloop`

`autoloop` is the self-development automation binary. It is not shipped as the
main user runtime. It coordinates agent workers that develop this repository.

Implemented subcommands:

```text
autoloop run
autoloop audit
autoloop digest [--output FILE]
autoloop service install [--force] [--no-start]
autoloop service install-audit [--force] [--no-start]
autoloop service disable legacy-timers
```

`autoloop run` currently replaces the shell entrypoint with a Go wrapper/CLI and
run skeleton. Full replacement of the legacy orchestrator loop remains staged
until the Go command wires worktree creation, claims, ledger recording,
promotion, and companion execution end-to-end.

`autoloop audit` and `autoloop digest` replace `scripts/orchestrator/audit.sh`
and `scripts/orchestrator/daily-digest.sh`.

The `service` subcommands replace `install-service.sh`, `install-audit.sh`, and
`disable-legacy-timers.sh`.

### `repoctl`

`repoctl` is repo maintenance tooling. It does not spawn agent workers.

Implemented subcommands:

```text
repoctl benchmark record
repoctl progress sync
repoctl readme update
repoctl compat go122
```

These replace:

```text
scripts/record-benchmark.sh
scripts/record-progress.sh
scripts/update-readme.sh
scripts/check-go1.22-compat.sh
```

The Makefile calls `go run ./cmd/repoctl ...` for repo maintenance during this
transition. Compiled `repoctl` binaries may replace those calls later where
appropriate.

## Package Layout

Start with a deliberately small package split:

```text
cmd/autoloop/
cmd/repoctl/
internal/autoloop/
internal/repoctl/
testdata/legacy-shell/
```

Avoid creating many packages before the port has tests proving the boundaries.
Within `internal/autoloop`, use focused files first:

```text
backend.go
candidates.go
claims.go
companions.go
config.go
digest.go
gitops.go
ledger.go
locks.go
promote.go
run.go
service.go
systemd.go
worktree.go
```

Extraction to `internal/autoloop/backend`, `internal/autoloop/gitops`, or
similar subpackages is allowed later only when package boundaries are clear and
tests become easier to maintain.

## Autoloop Runtime Model

The Go autoloop keeps the existing high-level behavior:

1. Load repo paths, progress path, run root, mode, backend, branch, and
   promotion settings from flags and environment.
2. Normalize candidate work from `docs/content/building-gormes/architecture_plan/progress.json`.
3. Claim tasks with phase/task locks and stale-lock cleanup.
4. Create isolated worker branches and worktrees.
5. Invoke the selected agent backend with the same worker contract.
6. Parse final reports and acceptance sections.
7. Record ledger events and failure records.
8. Promote successful worker commits through PR mode or cherry-pick fallback.
9. Run companion jobs according to cadence and productivity rules.
10. Refill candidate pools with the existing backoff semantics.

Typed Go structs replace `jq` mutations. External commands are still used where
they are semantically required: `git`, selected agent CLI, optional `gh`, and
optional `systemctl`.

All external command execution must go through an injectable runner interface so
tests can verify argv, environment, stdout, stderr, and exit-code behavior
without invoking real tools.

## Repoctl Runtime Model

`repoctl` is intentionally smaller:

1. `benchmark record` measures `bin/gormes` and updates `benchmarks.json`.
2. `progress sync` updates progress metadata and syncs progress data into the
   docs/site mirrors used by Gormes pages.
3. `readme update` updates README benchmark text from `benchmarks.json`.
4. `compat go122` reproduces the current Go 1.22 compatibility check using
   Docker when available and the `golang.org/dl/go1.22.10` fallback otherwise.

The first three subcommands should be pure file transformations except for
reading file metadata. The compatibility check may invoke Docker or a downloaded
Go toolchain, matching current behavior.

## Legacy Shell Handling

For parity, legacy shell moved instead of disappearing:

```text
testdata/legacy-shell/scripts/...
```

`.gitattributes` should mark that directory as vendored for language reporting:

```text
testdata/legacy-shell/** linguist-vendored
```

Repoctl and orchestrator entrypoints under `scripts/` are small wrappers after
their Go replacements passed targeted parity checks:

```sh
#!/usr/bin/env sh
exec go run ./cmd/autoloop run "$@"
```

Wrappers are transitional. They must not contain production logic beyond path
resolution and exec. The companion scripts
`scripts/gormes-architecture-planner-tasks-manager.sh`,
`scripts/documentation-improver.sh`, and `scripts/landingpage-improver.sh`
remain live shell outside this cutover. The final cleanup can delete wrappers
once Makefile, systemd templates, docs, and operator habits point at Go binaries
directly.

## Parity Strategy

This is a full port, but not a blind big-bang cutover. The repository should
land the Go implementation with parity tests while preserving legacy behavior as
the oracle until cutover.

Required parity classes:

1. Candidate normalization: Go output equals legacy shell output for fixture
   progress files.
2. Backend argv: `codexu`, `claudeu`, and `opencode` command construction
   matches the shell contract.
3. Claims and locks: stale PID, TTL, missing claim file, and live process cases
   match legacy behavior.
4. Failure records: retry counts, stderr tails, malformed JSON handling, and
   poison-pill behavior match legacy behavior.
5. Report parsing: good and bad final report fixtures produce the same accept or
   reject decisions.
6. Worktree lifecycle: branch naming, repo-subdir handling, cleanup, and pinned
   runs match legacy behavior.
7. Promotion: PR mode, `gh` failure fallback, cherry-pick conflict paths, and
   PR-body truncation match legacy behavior.
8. Companions: planner, doc improver, landing-page cadence and disable switches
   match legacy behavior.
9. Audit/digest: ledger fixture outputs match legacy report summaries.
10. Repo maintenance: benchmarks, progress sync, README update, and Go 1.22
    decision summaries match legacy behavior.

Where legacy shell behavior is clearly defective, preserve the old behavior in
a fixture first, then document and test the intentional Go correction in the
same slice. This prevents accidental drift from being mislabeled as cleanup.

## Migration Phases

### Phase 1: Parity Harness

Move frozen long-form shell copies into `testdata/legacy-shell/`, add
`.gitattributes`, and build Go tests that can compare legacy fixture behavior
against Go functions. Do not change production entrypoints in this phase.

### Phase 2: Repoctl Port

Implement `repoctl` and switch the Makefile from shell scripts to Go commands.
Keep shell wrappers temporarily. This phase should quickly reduce shell that is
called during normal builds.

### Phase 3: Autoloop Core Port

Implement candidate extraction, claims, ledger, backend command construction,
report parsing, failure records, worktree lifecycle, and run-loop orchestration.
Run fixture parity tests and dry-run tests before live worker execution.

### Phase 4: Promotion + Companion Port

Implement PR/cherry-pick promotion, companion scheduling, refill backoff, and
final report validation. Existing integration fixtures become Go integration
tests.

### Phase 5: Audit + Service Port

Implement audit, digest, systemd template rendering, service install, audit
timer install, and legacy timer disable behavior under `autoloop service`.

### Phase 6: Cutover + Shell Reduction

Completed for `repoctl` and the orchestrator wrapper/CLI surface. Docs,
Makefile targets, systemd rendering, and orchestrator/repoctl operator
entrypoints now route through `cmd/repoctl` or `cmd/autoloop`. Full autoloop
runtime parity and the three live companion scripts remain staged follow-up
work. Legacy long-form orchestrator shell remains as vendored parity fixtures
under `testdata/legacy-shell/`.

## Testing Requirements

Use TDD for every behavior slice:

1. Write a Go test against the desired parity behavior.
2. Verify it fails before implementation.
3. Implement minimal Go behavior.
4. Verify the targeted test passes.
5. Run the broader package tests before each commit.

Required commands by milestone:

```text
go test ./internal/repoctl ./cmd/repoctl
go test ./internal/autoloop ./cmd/autoloop
go test ./...
```

Existing Bats tests should be treated as behavior inventory. They can be kept
temporarily, but the destination is Go tests with no Bats dependency for
production validation.

## Risks And Mitigations

1. **Risk: big-bang rewrite changes production behavior.** Mitigation: use
   legacy fixtures as parity oracles and keep wrappers until live use proves the
   Go port.
2. **Risk: package split becomes ceremony.** Mitigation: start with
   `internal/autoloop` and focused files; extract subpackages only after tests
   justify them.
3. **Risk: wrappers keep Shell percentage high.** Mitigation: wrappers must be
   tiny and transitional; long legacy shell moves to vendored testdata or is
   deleted after parity.
4. **Risk: Go port still shells out heavily.** Mitigation: shelling out is
   allowed for external tools only; JSON, state, path, scheduling, locking, and
   report logic must be native Go.
5. **Risk: systemd behavior differs across Linux environments.** Mitigation:
   render unit files as pure Go outputs in tests, and execute `systemctl` only
   through an injectable runner.
6. **Risk: autoloop failures become harder to diagnose.** Mitigation: preserve
   structured ledger events and write deterministic stdout/stderr logs for every
   external command.

## Acceptance Criteria

1. `repoctl` replaces all Makefile calls to repo-maintenance shell scripts.
2. `autoloop run` has a Go CLI/skeleton and typed primitives; full dry-run and
   fixture parity with the current orchestrator remains staged follow-up work.
3. `autoloop audit` and `autoloop digest` reproduce current ledger reports for
   fixtures.
4. `autoloop service ...` renders and installs the same user service/timer
   units as the current shell scripts.
5. Long-form orchestrator shell no longer exists under `scripts/`.
6. Repoctl and orchestrator shell entrypoints under `scripts/` are short
   compatibility wrappers; the three companion scripts remain live shell pending
   a later port.
7. Legacy shell retained for parity lives under `testdata/legacy-shell/` and is
   marked vendored for GitHub language reporting.
8. `go test ./internal/repoctl ./cmd/repoctl ./internal/autoloop ./cmd/autoloop`
   passes.
9. Active docs and operator instructions reference `autoloop` and `repoctl`,
   not the old shell entrypoints.
