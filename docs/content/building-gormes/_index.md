---
title: "Building Gormes"
weight: 200
---

# Building Gormes

Contributor-facing documentation for the Go runtime, roadmap, autonomous work
queue, and upstream-porting research. If you want to **use** Gormes, start at
[Using Gormes](../using-gormes/).

## Runtime thesis

**Gormes is the production runtime for self-improving agents.** Four core systems live inside the binary:

- **Learning Loop** — detect complex tasks, distill reusable skills, improve them over time ([Phase 6](./architecture_plan/phase-6-learning-loop/)).
- **Memory** — SQLite + FTS5 + ontological graph, with a human-readable USER.md mirror ([Phase 3](./architecture_plan/phase-3-memory/)).
- **Tool Execution** — typed Go interfaces, in-process registry, no Python bounce ([Phase 2.A](./architecture_plan/phase-2-gateway/)).
- **Gateway** — one runtime, many interfaces: TUI plus shipped Telegram/Discord, with Slack and long-tail adapters advancing as contract-first Phase 2 slices ([Phase 2.B](./architecture_plan/phase-2-gateway/)).

Gormes ports upstream contracts, not upstream monoliths. Hermes proves the
breadth of the agent runtime: gateway, prompt assembly, provider routing, tool
continuations, memory providers, plugins, skills, cron, and operator commands.
GBrain proves the value of contract-first operations, durable jobs, graph
provenance, and skills as auditable runtime knowledge. Gormes absorbs those
durable contracts into a small Go runtime instead of copying Python mega-files
or TypeScript database gravity.

## Section map

| Need | Start with | Then use |
|---|---|---|
| Understand the runtime shape | [Core Systems](./core-systems/) | [Architecture Plan](./architecture_plan/), [Why Go](./architecture_plan/why-go/) |
| Choose implementation work | [Agent Queue](./builder-loop/agent-queue/) | [Next Slices](./builder-loop/next-slices/), [Blocked Slices](./builder-loop/blocked-slices/), [Umbrella Cleanup](./builder-loop/umbrella-cleanup/) |
| Prepare an autonomous-worker handoff | [Contract Readiness](./contract-readiness/) | [Progress Schema](./builder-loop/progress-schema/), [Builder Loop Handoff](./builder-loop/builder-loop-handoff/) |
| Port an upstream subsystem | [Porting a Subsystem](./porting-a-subsystem/) | [Upstream Lessons](./upstream-lessons/), [Testing](./testing/) |
| Reuse gateway adapter ideas | [Gateway Donor Map](./gateway-donor-map/) | [Shared Adapter Patterns](./gateway-donor-map/shared-adapter-patterns/), then the channel dossier |
| Continue Goncho/Honcho memory work | [Goncho Honcho Memory](./goncho_honcho_memory/) | [Prompts](./goncho_honcho_memory/01-prompts/), [Tool Schemas](./goncho_honcho_memory/02-tool-schemas/) |

## Planning rules

Every subsystem plan answers four questions before implementation:

1. What upstream contract are we porting?
2. Which trust class can call it: operator, gateway, child-agent, or system?
3. How does degraded mode show up in `gormes doctor`, status, audit, or logs?
4. What fixture proves compatibility without a live provider or platform?

For autonomous agents, the canonical progress row must also name the
`execution_owner`, `slice_size`, `ready_when`, `not_ready_when`, `write_scope`,
`test_commands`, and `done_signal` conditions. Assignable slices are
small/medium/large rows; `umbrella` rows stay inventory until split.

## Builder-loop execution contract

`cmd/builder-loop` is the executor for this roadmap, not a separate backlog. Its
job is to read `docs/content/building-gormes/architecture_plan/progress.json`,
use the generated `docs/content/building-gormes/` pages as the human-readable
handoff surface, select eligible phase rows, and launch worker agents that
develop the full `gormes-agent` toward the architecture plan.

The building-gormes docs are therefore part of the control plane. When a phase,
subphase, or task is unclear to an autonomous worker, fix the canonical
progress row and regenerate the derived docs instead of adding private
instructions elsewhere. `builder-loop` should consume the same source of truth that
contributors read: progress rows for machine selection, generated pages for
operator review, and row metadata for worker prompts.

Each CLI cycle checkpoints dirty control-checkout changes before the clean
preflight by default. That means self-improvement edits from the previous
agent run are staged with `git add -A` and committed as
`builder-loop: checkpoint dirty worktree <run-id>` before the next phase slice is
selected.

Worker execution is isolated: `cmd/builder-loop` creates a git worktree under
`RUN_ROOT/worktrees` for each selected row, runs the backend there, and rejects
committed paths outside that row's `write_scope` before promotion.

Final run health is gated after promotion. Once worker commits are integrated,
`cmd/builder-loop` runs the mandatory full-suite post-promotion verification before
it emits `run_completed` or `health_updated`. If the suite fails, the builder loop runs
one backend repair attempt by default, requires the checkout to be clean, reruns
the suite, and records final health only after the repaired integration passes.

For long-running operation, `cmd/builder-loop run --loop` is the steady-state
cadence owner: one builder cycle completes, run health and promotions are
recorded, the shared planner lock is released, one synchronous
`cmd/planner-loop run` refreshes the control plane, and then the next builder
cycle starts from the updated `progress.json`.

## Contributor path

Use the planning docs in this order:

1. Read [Upstream Lessons](./upstream-lessons/) to understand which contracts
   Gormes absorbs from Hermes and GBrain.
2. Check [Builder Loop Handoff](./builder-loop/builder-loop-handoff/) for the unattended-loop
   entrypoint, orchestrator plan, candidate source, generated docs, tests, and
   candidate policy.
3. Pick work from [Agent Queue](./builder-loop/agent-queue/) for an autonomous-worker-ready
   handoff, then use [Next Slices](./builder-loop/next-slices/) for the shorter ranking.
4. Run through `scripts/gormes-auto-codexu-orchestrator.sh` when using the
   unattended loop; it consumes the same canonical progress rows and injects
   row-specific handoff fields into worker prompts.
5. Check [Contract Readiness](./contract-readiness/) before implementation; an
   active or P0 row must name its contract, trust class, degraded mode, fixture,
   source references, and acceptance checks.
6. Check [Blocked Slices](./builder-loop/blocked-slices/) and
   [Umbrella Cleanup](./builder-loop/umbrella-cleanup/) before assigning a row.
7. Use [Progress Schema](./builder-loop/progress-schema/) when editing canonical progress.
8. Write the spec/plan from [Porting a Subsystem](./porting-a-subsystem/),
   then implement with the fixture classes in [Testing](./testing/).

## Reference groups

**Architecture:** [Architecture Plan](./architecture_plan/), [Core Systems](./core-systems/), [What Hermes Gets Wrong](./what-hermes-gets-wrong/), [Upstream Lessons](./upstream-lessons/).

**Execution queue:** [Contract Readiness](./contract-readiness/), [Builder Loop Handoff](./builder-loop/builder-loop-handoff/), [Agent Queue](./builder-loop/agent-queue/), [Next Slices](./builder-loop/next-slices/), [Blocked Slices](./builder-loop/blocked-slices/), [Umbrella Cleanup](./builder-loop/umbrella-cleanup/), [Progress Schema](./builder-loop/progress-schema/).

**Implementation help:** [Porting a Subsystem](./porting-a-subsystem/), [Testing](./testing/), [Gateway Donor Map](./gateway-donor-map/), [Goncho Honcho Memory](./goncho_honcho_memory/).
