# Builder-Owned Planner Cycle Design

## Goal

`cmd/builder-loop` should own the long-running delivery loop: run one builder cycle, schedule one synchronous planner cycle after it finishes, then start the next builder cycle. The process should keep cycling until the operator stops it or a configured hard failure requires attention.

## Context

Today `builder-loop run` is a single cycle. The installed builder service makes it long-running with systemd `Restart=always`, so process restarts are the loop mechanism. The planner is a separate timer/path-triggered one-shot. Both loops share the planner-loop `run.lock`; non-dry-run builder cycles hold it while mutating the control plane, and planner runs hold it while regenerating planning artifacts.

This design moves the primary loop cadence into the Go command instead of relying on service restart timing. The builder cycle remains the unit of implementation work. The planner cycle becomes the post-builder control-plane refresh.

## Operator Contract

Add a `builder-loop run --loop` mode.

In loop mode:

1. Run one normal builder cycle with the same selection, worker, promotion, verification, and health-writing behavior as `builder-loop run`.
2. After the builder cycle returns successfully, run `planner-loop run` synchronously.
3. Start the next builder cycle only after the planner exits successfully.
4. Continue until the command receives SIGINT/SIGTERM or a hard failure is returned.

Default one-shot behavior stays unchanged: `builder-loop run` without `--loop` still executes exactly one builder cycle.

## Locking And Data Flow

The builder cycle must release the shared planner-loop `run.lock` before launching the planner. That naturally happens if loop mode calls the existing `builderloop.RunOnce` as a complete cycle and only invokes the planner after `RunOnce` returns.

The next builder cycle should read the planner's updated `progress.json` and generated docs. The sequence is therefore:

```text
builder RunOnce -> health/progress commit -> unlock -> planner run -> next builder RunOnce
```

The planner command should run in the repository root and inherit the builder process environment, including backend and mode defaults unless the operator overrides them.

## Configuration

The initial implementation should keep configuration small:

- CLI flag: `--loop` enables infinite cycles.
- Environment variable: `BUILDER_LOOP_SLEEP`, parsed as a Go duration, controls the delay between successful planner completion and the next builder cycle.
- Default sleep: `30s`, matching the current systemd restart delay.
- Planner command: default to `go run ./cmd/planner-loop run`.

Planner command customization is intentionally out of scope for this slice. Keeping the planner invocation explicit and local reduces the chance of drifting from the checked-in `cmd/planner-loop` behavior.

## Failure Semantics

Loop mode should stop on hard failures instead of hiding them.

- Builder parse/config errors stop the process.
- Builder post-promotion verify failures stop the process, preserving the existing exit classification for operator attention.
- Builder backend timeout or cancellation returns the existing error; service policy can decide whether to restart.
- Planner failure stops the process and returns a wrapped error that names the planner command.
- SIGINT/SIGTERM cancels the current child operation and exits cleanly through the existing context path.

This keeps failure evidence visible in the existing ledgers and avoids spinning on a broken control plane.

## Service Behavior

The installed builder service should use `builder-loop run --loop` once loop mode exists. `Restart=always` can remain as a crash recovery guard, but it should no longer be the normal cycle driver. The existing `RestartPreventExitStatus=2 30` remains valid because parse/config and post-promotion verify failures should not be silently restarted.

Planner timer/path units can continue to exist. They are still useful for manual/reactive planning signals, but the builder-owned loop becomes the main steady-state cadence. If a separate planner timer fires during a builder cycle, the existing shared lock still prevents concurrent mutation.

## Testing

Tests should prove the contract without running real agents:

- CLI parsing accepts `builder-loop run --loop` and rejects unsupported run flags.
- One-shot `builder-loop run` remains unchanged.
- Loop mode runs builder once, then planner once, then sleeps before the next iteration.
- Planner invocation happens only after `builderloop.RunOnce` returns.
- A planner failure stops loop mode with a useful error.
- Context cancellation stops the loop without starting another cycle.
- Rendered service units include `run --loop`.

Use injected runners and small loop hooks in tests so the suite does not sleep, spawn real planner backends, or run forever.

## Out Of Scope

- Rewriting planner-loop scheduling.
- Removing planner timer/path units.
- Adding a custom planner command flag.
- Changing worker selection, promotion, health writing, or `progress.json` schema.
- Changing backend behavior or noninteractive backend safeguards.
