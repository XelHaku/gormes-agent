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

## Contents

- [Core Systems](./core-systems/) — one page per system, how they work today
- [Upstream Lessons](./upstream-lessons/) — what GBrain and Hermes taught the Gormes architecture
- [What Hermes Gets Wrong](./what-hermes-gets-wrong/) — the opportunities that justify Gormes's existence
- [Architecture Plan](./architecture_plan/) — full roadmap, phase-by-phase, with subsystem inventory
- [Porting a Subsystem](./porting-a-subsystem/) — the contribution path: pick from §7, write spec + plan, open PR
- [Gateway Donor Map](./gateway-donor-map/) — prescriptive PicoClaw-to-Gormes channel reuse dossiers
- [Testing](./testing/) — Go test suite, Playwright smoke, Hugo build rig
