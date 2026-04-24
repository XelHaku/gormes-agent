---
title: "Next Slices"
weight: 35
---

# Next Slices

This page is generated from the canonical progress file and lists the highest
leverage contract-bearing roadmap rows to execute next.

The ordering is:

1. unblocked `P0` handoffs;
2. active `in_progress` rows;
3. `fixture_ready` rows;
4. unblocked rows that unblock other slices;
5. remaining `draft` contract rows.

Use this page when choosing implementation work. If a row is too broad, split
the row in `progress.json` before assigning it.

<!-- PROGRESS:START kind=next-slices -->
| Phase | Slice | Contract | Trust class | Fixture | Why now |
|---|---|---|---|---|---|
| 1 / 1.C | Orchestrator failure-row stabilization for 4-8 workers | Worker verification and failure-taxonomy contract | operator, system | `scripts/orchestrator unit fixtures for failure taxonomy and soft-success recovery` | Already active; contract metadata keeps execution bounded. |
| 4 / 4.A | Provider interface + stream fixture harness | Provider-neutral request and stream event transcript harness | system | `internal/hermes provider transcript fixtures` | Already active; contract metadata keeps execution bounded. |
| 5 / 5.A | Tool registry inventory + schema parity harness | Operation and tool descriptor parity before handler ports | operator, gateway, child-agent, system | `internal/tools upstream schema parity manifest fixtures` | Unblocks Pure core tools first, Stateful tool migration queue, CLI command registry parity + active-turn busy policy. |
<!-- PROGRESS:END -->
