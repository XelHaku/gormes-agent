# AGENTS.md — gormes-agent

This file briefs every agent (codexu, claudeu, claude-code, opencode, or
any future backend) that runs against this repository. Read it before
touching code or docs in `cmd/`, `internal/`, `docs/content/building-gormes/`,
or `progress.json`.

## Planner-Builder Loop (the core architecture)

Gormes' self-development is a **dual-loop autonomous delivery
architecture**. Two coordinated loops run continuously and inform each
other in real time through a shared progress representation.

> Planner-Builder Loop is a recursive, closed-loop architecture for
> autonomous software delivery in which an outer **Planner** continuously
> maintains a prioritized project trajectory, while an inner **Builder**
> continuously executes the highest-value feasible task from that
> trajectory, validates outcomes, and feeds structured results back into
> planning. The system converges through repeated
> plan → build → evaluate → replan cycles, enabling adaptive execution,
> stable progress, and long-running autonomy without separating strategy
> from implementation.

```
                +---------------------+
                |   Planner Loop      |
                |  cmd/planner-loop/  |
                +----------+----------+
                           |
                  refines  |  reads ledger
                           v
   progress.json  <---> runs.jsonl + triggers.jsonl
                           ^
                  selects  |  emits events
                           |
                +----------+----------+
                |   Builder Loop      |
                |  cmd/builder-loop/  |
                +---------------------+
```

In steady-state service mode, `cmd/builder-loop run --loop` owns the cadence:
each successful builder cycle releases the shared control-plane lock, runs one
synchronous `cmd/planner-loop run`, then starts the next builder cycle from the
planner-refreshed `progress.json`. Independent planner timer/path triggers may
still fire, but the shared `run.lock` prevents concurrent control-plane writes.

### Two loops, one shared contract

| Loop | Command | Internal package | Owns |
|---|---|---|---|
| **Planner Loop** (outer) | `cmd/planner-loop` | `internal/plannerloop` | `progress.json` priorities and trajectory; reads upstream sources, current implementation, and the builder-loop ledger; refines roadmap rows. |
| **Builder Loop** (inner) | `cmd/builder-loop` | `internal/builderloop` | Selecting ready rows from `progress.json`, running backend workers in isolated worktrees, validating, promoting, and emitting ledger events. |

### Shared progress representation

Both loops talk through these files. **Do not bypass them.**

- `docs/content/building-gormes/architecture_plan/progress.json` — canonical
  prioritized trajectory. Planner writes; builder reads to select work.
  Schema lives at `internal/progress/`; rendered surfaces under
  `docs/content/building-gormes/builder-loop/`.
- `.codex/builder-loop/state/runs.jsonl` — builder-loop ledger of every
  worker claim, promotion, failure, and post-promotion gate. Planner reads
  the last 7 days each run to surface toxic / hot subphases.
- `.codex/planner-loop/triggers.jsonl` — builder → planner reactive signal
  ledger (e.g., quarantine state changes). Planner consumes via cursor.
- Path defaults fall back to legacy `.codex/orchestrator/` and
  `.codex/architecture-planner/` for back-compat; environment overrides
  are `BUILDER_LOOP_RUN_ROOT` and `RUN_ROOT`.

## Standing directive for any agent working here

1. **Preserve the contract.** When extending either loop, do not break the
   shared progress representation. New fields must round-trip through the
   typed structs in `internal/progress/`. New ledger event kinds must be
   accepted by the planner's evaluation logic.
2. **Use the right loop.** Implementation work goes through the builder
   loop. Roadmap shape, row priorities, source references, ready-when /
   not-ready-when conditions, and trajectory go through the planner loop.
   The planner does not implement runtime feature code; the builder does
   not invent its own backlog.
3. **Improve the loop pattern itself.** When you notice a feedback gap
   (planner can't see something it needs, builder repeats a class of
   failure, gate produces ambiguous signals, audit metric misleads),
   propose a loop-level change. Update this file when the architecture
   changes, not just the code.
4. **Don't introduce a parallel queue.** Side-channel TODO files,
   private prompt instructions, or hand-curated row lists outside
   `progress.json` are explicitly out of bounds. Fix the canonical row
   instead.

## Where to look first

| If you're … | Read this first |
|---|---|
| Adding worker behavior, promotion gates, ledger events | `cmd/builder-loop/README.md` |
| Adding planner heuristics, audit feedback, divergence handling | `cmd/planner-loop/README.md` |
| Changing the row schema or rendered docs | `internal/progress/` and the schema doc rendered at `docs/content/building-gormes/builder-loop/progress-schema.md` |
| Wiring agents into the system from a new backend | both READMEs above plus `internal/builderloop/backend.go` |
| Onboarding to the architecture with no prior context | this file, then `docs/content/building-gormes/_index.md` |
