---
title: "Upstream Lessons"
weight: 25
---

# Upstream Lessons

Gormes does not copy Hermes or GBrain. It absorbs their durable contracts.

Hermes is the capability ledger for the agent runtime: provider routing,
prompt assembly, tool continuations, gateway sessions, cron, memory providers,
skills, plugins, and operator commands. GBrain is the architecture donor for
contract-first operations, durable jobs, knowledge graph provenance, retrieval
evaluation, and skills as auditable runtime knowledge.

The combined lesson is simple:

```text
port contracts
reject monoliths
preserve Go ownership boundaries
prove behavior with fixtures
show degraded mode to operators
```

## Durable Contracts To Absorb

| Contract | Donor | Gormes target |
|---|---|---|
| Contract-first operations | GBrain | Operation/tool descriptors drive schemas, commands, doctor, audit, and fixtures. |
| Trust-class enforcement | GBrain + Hermes gateway | `operator`, `gateway`, `child-agent`, and `system` are enforced before handlers run. |
| Stable prompt assembly | Hermes | Stable system layers; ephemeral recall injected into the current user turn. |
| Provider-neutral events | Hermes | Adapters own provider quirks; `internal/hermes` emits one stream/tool-call contract. |
| Durable jobs | GBrain | Cron, long work, and subagents get restartable ledgers and operator inspection. |
| Provenance-rich memory | GBrain | Entities and relationships carry source turn, extractor, confidence, freshness, and review state. |
| Skills as reviewed code | GBrain + Hermes | Skills have metadata, resolver tests, inactive drafts, review, feedback, and version history. |
| Visible degraded mode | GBrain + Hermes | Missing embeddings, provider limits, stale extraction, plugin/tool gaps, and dead letters surface in status/doctor/audit. |

## The Four Questions

Every planned subsystem should answer these before implementation:

1. **What contract are we porting?** Name the source files and external
   behavior. Do not use "port file X" as the requirement.
2. **What trust class can call it?** Operator-local, gateway-user,
   child-agent, and system/cron paths do not share the same permissions.
3. **How is degraded mode reported?** Partial capability is acceptable only
   when operators can see it in status, doctor, audit, logs, or docs.
4. **What fixture proves compatibility?** Prefer replayable local fixtures over
   live credentials, live platforms, or a real provider.

## Phase Mapping

- [Phase 2 Gateway](../architecture_plan/phase-2-gateway/) owns command policy,
  active-turn behavior, adapter contracts, cron, and subagent runtime.
- [Phase 3 Memory](../architecture_plan/phase-3-memory/) owns provenance,
  scoped recall, retrieval evaluation, and degraded memory health.
- [Phase 4 Brain Transplant](../architecture_plan/phase-4-brain-transplant/)
  owns stable prompt assembly, context compression, provider adapters, and
  transcript fixtures.
- [Phase 5 Final Purge](../architecture_plan/phase-5-final-purge/) owns
  operation/tool descriptor parity before handler ports.
- [Phase 6 Learning Loop](../architecture_plan/phase-6-learning-loop/) owns
  skills as reviewed code, resolver evals, feedback records, and safe
  self-improvement.

## Upstream Study References

- [Upstream GBrain Study](../../upstream-gbrain/)
- [GBrain Architecture](../../upstream-gbrain/architecture/)
- [GBrain Good And Bad](../../upstream-gbrain/good-and-bad/)
- [GBrain Gormes Takeaways](../../upstream-gbrain/gormes-takeaways/)
- [Upstream Hermes Reference](../../upstream-hermes/)
- [Hermes Source Study](../../upstream-hermes/source-study/)
- [Hermes Good And Bad](../../upstream-hermes/good-and-bad/)
- [Hermes Gormes Takeaways](../../upstream-hermes/gormes-takeaways/)

## Decision

The better Gormes architecture is:

```text
Hermes-class capability
+ GBrain-style operation contracts
+ Go single-owner kernel
+ provider-neutral stream fixtures
+ registry-owned gateway policy
+ descriptor-owned tool safety
+ GONCHO-scoped memory provenance
+ reviewed skill lifecycle
+ visible degraded-mode checks
```

That keeps the upstream lessons while preserving the product promise: one
small Go runtime, explicit boundaries, local-first state, and no Python runtime
dependency in the final path.
