---
title: "Building Gormes"
weight: 200
---

# Building Gormes

Contributor-facing documentation. If you're reading because you want to **use** Gormes, start at [Using Gormes](../using-gormes/).

## Gormes in one sentence

**Gormes is the production runtime for self-improving agents.** Four core systems live inside the binary:

1. **Learning Loop** — detect complex tasks, distill reusable skills, improve them over time ([Phase 6](./architecture_plan/phase-6-learning-loop/))
2. **Memory** — SQLite + FTS5 + ontological graph, with a human-readable USER.md mirror ([Phase 3](./architecture_plan/phase-3-memory/))
3. **Tool Execution** — typed Go interfaces, in-process registry, no Python bounce ([Phase 2.A](./architecture_plan/phase-2-gateway/))
4. **Gateway** — one runtime, many interfaces: TUI plus shipped Telegram/Discord, with Slack and long-tail adapters advancing as contract-first Phase 2 slices ([Phase 2.B](./architecture_plan/phase-2-gateway/))

## Core thesis

Gormes ports upstream contracts, not upstream monoliths.

Hermes proves the breadth of the agent runtime: gateway, prompt assembly,
provider routing, tool continuations, memory providers, plugins, skills, cron,
and operator commands. GBrain proves the value of contract-first operations,
durable jobs, graph provenance, and skills as auditable runtime knowledge.
Gormes should absorb those durable contracts into a small Go runtime instead of
copying Python mega-files or TypeScript database gravity.

Every subsystem plan should answer four questions before implementation:

1. What upstream contract are we porting?
2. Which trust class can call it: operator, gateway, child-agent, or system?
3. How does degraded mode show up in `gormes doctor`, status, audit, or logs?
4. What fixture proves compatibility without a live provider or platform?

For autonomous agents, the canonical progress row must also name the
`execution_owner`, `slice_size`, `ready_when`, `not_ready_when`, `write_scope`,
`test_commands`, and `done_signal` conditions. Assignable slices are
small/medium/large rows; `umbrella` rows stay inventory until split.

## Contributor path

Use the planning docs in this order:

1. Read [Upstream Lessons](./upstream-lessons/) to understand which contracts
   Gormes absorbs from Hermes and GBrain.
2. Check [Autoloop Handoff](./autoloop-handoff/) for the unattended-loop
   entrypoint, orchestrator plan, candidate source, generated docs, tests, and
   candidate policy.
3. Pick work from [Agent Queue](./agent-queue/) for an autonomous-worker-ready
   handoff, then use [Next Slices](./next-slices/) for the shorter ranking.
4. Run through `scripts/gormes-auto-codexu-orchestrator.sh` when using the
   unattended loop; it consumes the same canonical progress rows and injects
   row-specific handoff fields into worker prompts.
5. Check [Contract Readiness](./contract-readiness/) before implementation; an
   active or P0 row must name its contract, trust class, degraded mode, fixture,
   source references, and acceptance checks.
6. Check [Blocked Slices](./blocked-slices/) and
   [Umbrella Cleanup](./umbrella-cleanup/) before assigning a row.
7. Use [Progress Schema](./progress-schema/) when editing canonical progress.
8. Write the spec/plan from [Porting a Subsystem](./porting-a-subsystem/),
   then implement with the fixture classes in [Testing](./testing/).

## Contents

- [Core Systems](./core-systems/) — one page per system, how they work today
- [Upstream Lessons](./upstream-lessons/) — what GBrain and Hermes taught the Gormes architecture
- [Contract Readiness](./contract-readiness/) — the enforceable upstream-contract checklist for priority subsystems
- [Autoloop Handoff](./autoloop-handoff/) — the main script, plan link, candidate source, generated docs, tests, and selection policy
- [Agent Queue](./agent-queue/) — autonomous-worker-ready handoff cards from canonical progress
- [Next Slices](./next-slices/) — the highest-leverage contract-bearing progress rows to execute next
- [Blocked Slices](./blocked-slices/) — contract-bearing rows waiting on prerequisites
- [Umbrella Cleanup](./umbrella-cleanup/) — rows too broad to assign directly
- [Progress Schema](./progress-schema/) — canonical progress field reference and validation rules
- [What Hermes Gets Wrong](./what-hermes-gets-wrong/) — the opportunities that justify Gormes's existence
- [Architecture Plan](./architecture_plan/) — full roadmap, phase-by-phase, with subsystem inventory
- [Porting a Subsystem](./porting-a-subsystem/) — the contribution path: pick from §7, write spec + plan, open PR
- [Gateway Donor Map](./gateway-donor-map/) — prescriptive PicoClaw-to-Gormes channel reuse dossiers
- [Testing](./testing/) — Go test suite, Playwright smoke, Hugo build rig
