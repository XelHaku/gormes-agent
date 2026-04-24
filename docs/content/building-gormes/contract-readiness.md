---
title: "Contract Readiness"
weight: 30
---

# Contract Readiness

Building Gormes now treats `docs/content/building-gormes/architecture_plan/progress.json`
as the canonical roadmap. Priority roadmap items can carry optional contract
metadata:

- `contract`
- `contract_status`
- `trust_class`
- `degraded_mode`
- `fixture`
- `source_refs`
- `blocked_by`
- `unblocks`
- `acceptance`

Those fields turn upstream study into execution rules. A subsystem is not ready
for implementation just because a donor file exists. It is ready when the
contract is named, the allowed caller class is explicit, degraded mode is
operator-visible, and a local fixture proves compatibility.

## Current Contract Rows

<!-- PROGRESS:START kind=contract-readiness -->
| Phase | Progress item | Contract status | Owner | Size | Trust class | Fixture | Degraded mode |
|---|---|---|---|---|---|---|---|
| 1 / 1.C | Orchestrator failure-row stabilization for 4-8 workers — Worker verification and failure-taxonomy contract | `fixture_ready` | `orchestrator` | `large` | operator, system | `scripts/orchestrator unit fixtures for failure taxonomy and soft-success recovery` | The orchestrator records precise failure reasons, poisoned-task thresholds, soft-success decisions, and original exit codes instead of collapsing failures into one generic status. |
| 1 / 1.C | Soft-success-nonzero bats coverage — Soft-success nonzero recovery guard | `draft` | `orchestrator` | `small` | operator, system | `scripts/orchestrator tests that call try_soft_success_nonzero directly` | When the recovery guard refuses a non-zero exit, the worker state keeps the original exit reason and does not promote the run. |
| 2 / 2.F.5 | Steer slash command registry + queue fallback — Registry-owned active-turn steering command | `draft` | `gateway` | `small` | operator, gateway | `internal/gateway active-turn command registry fixtures` | Gateway returns visible usage, busy, or queued status instead of dropping steer text when the command cannot run immediately. |
| 3 / 3.E.7 | Cross-chat deny-path fixtures — Same-chat default recall with explicit user-scope widening | `draft` | `memory` | `small` | operator, system | `internal/memory cross-chat allow-deny recall fixtures` | Memory status and operator evidence report unresolved, conflicting, or denied cross-chat identity bindings. |
| 4 / 4.A | Provider interface + stream fixture harness — Provider-neutral request and stream event transcript harness | `fixture_ready` | `provider` | `medium` | system | `internal/hermes provider transcript fixtures` | Provider status reports missing fixture coverage or unavailable adapters before kernel routing can select them. |
| 4 / 4.A | Tool-call normalization + continuation contract — Cross-provider tool-call continuation contract | `fixture_ready` | `provider` | `medium` | system | `internal/hermes cross-provider tool continuation fixtures` | Provider status reports transcript or continuation fixture gaps before adapters can be selected for tool-capable turns. |
| 4 / 4.B | ContextEngine interface + status tool contract — Stable context engine status and compression boundary | `draft` | `provider` | `medium` | operator, system | `internal/contextengine status and compression replay fixtures` | Context status reports disabled compression, cooldowns, unknown tools, token-budget pressure, and replay gaps. |
| 4 / 4.H | Provider-side resilience — Provider resilience umbrella over retry, cache, rate, and budget behavior | `draft` | `provider` | `large` | system | `internal/hermes and internal/kernel provider resilience fixtures` | Provider and kernel status expose retry schedule, Retry-After hints, cache disabled paths, rate guards, and budget telemetry gaps. |
| 4 / 4.H | Classified provider-error taxonomy — Structured provider error classification contract | `draft` | `provider` | `small` | system | `internal/hermes provider error-classification fixture table` | Provider status and logs expose auth, rate-limit, context, retryable, and non-retryable classes instead of raw opaque errors. |
| 5 / 5.A | Tool registry inventory + schema parity harness — Operation and tool descriptor parity before handler ports | `draft` | `tools` | `medium` | operator, gateway, child-agent, system | `internal/tools upstream schema parity manifest fixtures` | Doctor reports disabled tools, missing dependencies, schema drift, and unavailable provider-specific paths. |
| 6 / 6.C | Portable SKILL.md format — Reviewed skill-as-code storage format | `draft` | `skills` | `medium` | operator, system | `internal/skills SKILL.md metadata, provenance, and review-state fixtures` | Skill status excludes unreviewed or invalid drafts from prompt injection and records resolver or metadata failures. |
<!-- PROGRESS:END -->

## Authoring Rule

New priority progress rows should add contract metadata when the row is used as
an implementation handoff. The fields are optional only for historical rows and
inventory buckets.

Required handoff shape:

```json
{
  "name": "Short executable slice",
  "status": "planned",
  "contract_status": "draft",
  "contract": "The upstream behavior being preserved",
  "slice_size": "small",
  "execution_owner": "gateway",
  "trust_class": ["operator"],
  "degraded_mode": "How partial capability becomes visible",
  "fixture": "The replayable local fixture proving compatibility",
  "source_refs": ["docs/content/upstream-hermes/source-study.md"],
  "blocked_by": ["optional dependency"],
  "unblocks": ["optional downstream slice"],
  "ready_when": ["The dependency or handoff condition is true"],
  "acceptance": ["fixture or behavior that proves this row is done"]
}
```

## Canonical Progress Source

There is one docs-side progress source:

```text
docs/content/building-gormes/architecture_plan/progress.json
```

Do not reintroduce `docs/data/progress.json`. The website can keep an embedded
copy under `www.gormes.ai/internal/site/data/progress.json`, but that file is a
generated site asset, not a planning source.
