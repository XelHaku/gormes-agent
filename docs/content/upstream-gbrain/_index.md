---
title: "Upstream GBrain Study"
weight: 350
---

# Upstream GBrain Study

This section studies upstream `gbrain` as an architecture donor for Gormes.
It is not a porting instruction to copy GBrain wholesale. The goal is to
extract the useful ideas, name the failure modes, and apply them to a better
Go-native `gormes-agent`.

## Study Snapshot

- Upstream studied: `/home/xel/git/sages-openclaw/workspace-mineru/gbrain`
- Upstream commit: `e2961c0`
- Gormes repo studied: `/home/xel/git/sages-openclaw/workspace-mineru/gormes-agent`
- Date: 2026-04-26

## 2026-04-26 v0.22.1 Drift Check

GBrain `e2961c0` ships a maintenance-loop hotfix wave: cooperative cycle abort,
worker slot recovery after ignored aborts, incremental extraction bounded by
changed slugs, server-side stale embedding filters, and Postgres session
timeouts. Gormes should not import the Minions worker or Postgres runtime, but
the architecture lessons matter. The roadmap now records a blocked Phase 2.E.3
durable-worker abort-slot recovery slice for the first real durable worker loop,
and a Phase 6.D delta-bounded retrieval maintenance slice so future learning
passes do not reprocess the whole local memory/skills corpus when only a small
set changed.

## 2026-04-26 v0.22.0 Drift Check

GBrain `172b55b` ships v0.22.0 source-aware search ranking. The new lesson is
source-swamp resistance: curated sources win over bulky chat/daily/raw imports
by default, hard-exclude prefixes are applied before ranking, and high-detail or
temporal lookups can bypass the damping. Gormes should port this as a small
Phase 6.D source-tier evidence contract for skill and memory retrieval, not as
GBrain's Postgres/PGLite SQL builders.

## 2026-04-25 Drift Check

GBrain `f718c59` ships Code Cathedral II: tree-sitter-backed code chunking,
qualified symbols, call-graph edges, parent-scope chunks, two-pass retrieval,
and explicit `reindex-code` backfill tooling. Gormes should treat this as donor
evidence for explainable retrieval, not as a dependency to embed. The roadmap
now tracks a small Phase 6.D row that defines optional code-context evidence
for skill retrieval before any Go-native code indexer is designed.

## Documents

- [Architecture](./architecture/) maps the GBrain runtime, data model, search
  stack, skills layer, and Minions job system.
- [Good and Bad](./good-and-bad/) lists the design moves worth stealing and the
  traps Gormes should avoid.
- [Gormes Takeaways](./gormes-takeaways/) translates the study into concrete
  Gormes architecture decisions.

## One-Line Read

GBrain's best idea is not "Postgres brain." It is the combination of
contract-first operations, a brain-first agent loop, fat procedural skills, and
a durable job ledger for deterministic work. Gormes should keep the single Go
binary and typed tool contracts, then borrow those ideas in a smaller,
auditable, SQLite-first shape.
