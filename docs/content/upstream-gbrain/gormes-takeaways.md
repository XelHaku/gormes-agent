---
title: "Gormes Takeaways"
weight: 30
---

# Gormes Takeaways

## Do Not Copy GBrain Wholesale

Gormes has a different product promise:

- one Go binary
- typed in-process tools
- SQLite-first local memory
- gateway-native operation
- explicit runtime boundaries

GBrain should be treated as a donor for architecture patterns, not as a
dependency or a subsystem to embed.

## Better Gormes Target Architecture

```text
Gateway/TUI/CLI/cron input
        |
        v
Kernel admission and trust classification
        |
        v
Operation registry
        |
        +--> model tool schema
        +--> CLI/gateway commands
        +--> doctor validation
        +--> audit taxonomy
        |
        v
Typed Go handlers
        |
        +--> memory engine
        +--> tool executor
        +--> durable jobs
        +--> subagent runner
        +--> skills runtime
        |
        v
SQLite-backed state, JSONL audit, markdown mirrors
```

The key addition is an operation registry above the current tool registry. A
Gormes operation should know:

- name and description
- input schema
- output shape
- mutating or read-only
- idempotent or not
- trust classes allowed
- default timeout
- audit event kind
- prompt-visible or operator-only
- health/doctor checks

The handler can remain ordinary Go.

## Map GBrain Ideas To Gormes

| GBrain idea | Gormes equivalent | Recommended action |
|---|---|---|
| `operations.ts` shared contract | `internal/tools` plus CLI/gateway commands | Add `internal/ops` descriptors that generate tool schemas and doctor checks. |
| `OperationContext.remote` | gateway, child, local operator, cron callers | Promote to typed `TrustClass`; enforce centrally. |
| `BrainEngine` | `internal/memory` SQLite store and GONCHO service | Define a narrower knowledge read/write interface before adding more providers. |
| `pages` plus `links` provenance | entities, relationships, USER.md mirror | Add relationship evidence/provenance fields and reviewed promotion. |
| hybrid search + source-aware ranking | FTS5, graph traversal, semantic recall, source-tier evidence | Add a retrieval eval harness, stable score breakdown, curated/reviewed boosts, bulk-source damping, hard-exclude evidence, and temporal-query bypass. |
| Code Cathedral II call graph + two-pass retrieval | optional code-context evidence for skill/retrieval explanations | Start with synthetic parent-scope/call-edge fixtures and capped high-fan-out behavior; do not embed GBrain's TypeScript indexer or tree-sitter WASM in the runtime. |
| Minions SQL queue | subagent manager, cron, audit logs | Add a durable job ledger for long work and child runs. |
| `subagent_messages` and tool ledger | run logs and transcript export | Persist child-agent messages/tool calls enough to resume or replay. |
| skills resolver and checks | `internal/skills` active/inactive store | Add resolver conformance, routing evals, conflict checks, and promotion evidence. |
| `gbrain doctor`/skillpack-check | `gormes doctor --offline` | Extend doctor to report operation registry, memory degradation, job queue health, and skill resolver health. |

## Priority Moves For Gormes

### 1. Add A Go Operation Descriptor Layer

Keep `tools.Tool` as the execution contract, but add a declarative descriptor:

```go
type TrustClass string

const (
    TrustOperator TrustClass = "operator"
    TrustGateway  TrustClass = "gateway"
    TrustChild    TrustClass = "child-agent"
    TrustSystem   TrustClass = "system"
)

type OperationSpec struct {
    Name        string
    Description string
    Schema      json.RawMessage
    Mutating    bool
    Idempotent  bool
    PromptSafe  bool
    Allowed     []TrustClass
    Timeout     time.Duration
    AuditKind   string
}
```

The shared executor should reject disallowed trust classes before a handler runs.
This prevents every tool from needing to remember the same safety checks.

### 2. Make Durable Jobs A First-Class Runtime Surface

Gormes subagents are currently strong at live lifecycle: contexts, cancellation,
depth caps, timeouts, tool allowlists, and run logs. GBrain shows the next step:
persist long work.

Recommended minimum durable job table for SQLite:

- `id`
- `name`
- `status`
- `queue`
- `priority`
- `payload_json`
- `attempts`
- `max_attempts`
- `lock_owner`
- `lock_until`
- `parent_job_id`
- `timeout_at`
- `progress_json`
- `result_json`
- `error_text`
- `created_at`
- `updated_at`

Do not start with every Minions feature. Start with the current
`minion-orchestrator` lesson: one routing policy for deterministic durable jobs
and LLM subagents, with shell-like work kept operator-trusted. Then implement
only the cron/subagent replay needs: claim, renew, complete, fail, retry,
cancel, parent child_done event, and audit.

### 3. Strengthen Memory Provenance Before More Recall Magic

GBrain's typed link provenance is directly useful. Gormes should extend
relationships with:

- origin turn id or source artifact
- extractor version
- confidence
- first seen
- last seen
- evidence text hash
- provenance kind: manual, extracted, imported, reviewed

This makes graph quality debuggable and lets Gormes avoid trusting every
LLM-extracted edge equally.

### 4. Add Retrieval Evaluation Before Adding More Retrieval Layers

GBrain has retrieval eval surfaces and public benchmark claims. Gormes should
not add graph, semantic, decay, and cross-chat scoring without a local eval
harness.

Minimum local harness:

- seed small conversations and entity facts
- define expected recall slugs or snippets
- run lexical-only, graph-only, semantic-only, and fused modes
- report precision at k, recall at k, and explanation of selected seeds
- include cross-chat privacy negative tests

This turns "memory feels better" into a testable contract.

GBrain v0.22.0 adds another retrieval lesson: source quality should participate
in scoring before bulky transcripts swamp curated knowledge. Gormes should keep
that as visible evidence, not hidden rank magic. A result should be able to say
whether it was boosted because it came from reviewed/curated memory, dampened
because it came from bulk chat/raw imports, excluded by a configured prefix, or
allowed through because the query was a high-detail temporal/history lookup.

### 5. Treat Skills As Code

Gormes already has `internal/skills` with active skills, candidate drafting, and
promotion. Borrow GBrain's checks:

- every active skill has valid frontmatter
- every resolver route points at an existing skill
- every skill declares triggers and exclusions
- routing eval fixtures cover confusing user phrases
- disabled or unreviewed skills never enter prompt injection
- usage logs tie selected skills to turn outcome

This will matter before Phase 6 learning-loop extraction writes new skills.

### 5a. Keep Code Context Optional And Explained

GBrain `f718c59` proves that code retrieval gets better when chunks know their
qualified symbol, parent scope, and call edges. Gormes should not make a code
indexer part of the hot path yet. The first Go target is only an evidence shape
that skill retrieval can consume:

- symbol name and optional parent-scope path;
- caller/callee edges with a deterministic fan-out cap;
- stale or unavailable reason;
- score contribution shown in the skill selection explanation.

That lets the learning loop benefit from code-context donors later without
silently requiring tree-sitter, WASM grammars, repository-wide backfill, or a
second database.

### 6. Define Degraded Mode As A Product Contract

Borrow GBrain's graceful fallback, but make it visible. Gormes should report:

- semantic recall disabled because no embedding model is configured
- extractor queue depth and dead-letter count
- graph extraction age
- skill resolver warnings
- durable job stalled count
- child-agent replay not available for live-only runs

The user should never have to infer whether the brain is healthy from answer
quality.

## Things To Avoid

- Do not make Postgres required for the mainline Gormes runtime.
- Do not let one provider's streaming protocol shape the job/subagent ledger.
- Do not make markdown skills the only enforcement layer for dangerous actions.
- Do not allow remote/gateway callers to exercise local operator tools through
  prompt-visible schemas.
- Do not auto-promote graph edges from untrusted text without evidence and
  review policy.
- Do not hide fallback paths. Degraded is acceptable; invisible degraded is not.
- Do not grow one giant operation file. Keep one registry, many small packages.

## Suggested Phase Alignment

Near-term fits with current Gormes roadmap:

- Phase 2.E/2.F: durable job ledger for subagent and gateway mid-run steering.
- Phase 2.G/6.C: skill resolver conformance and reviewed promotion evidence.
- Phase 3.E: relationship provenance and degraded-mode memory health.
- Phase 4: provider-neutral event/tool continuation contract before more adapters.
- Phase 5: operation registry generation for CLI, tool schemas, and doctor.
- Phase 6: learning loop only after skill storage, resolver evals, and feedback
  records are already reliable.

## Decision

The better Gormes architecture is:

```text
typed Go runtime
+ operation registry
+ trust-class enforcement
+ SQLite durable job ledger
+ provenance-rich memory graph
+ retrieval eval harness
+ reviewed skill lifecycle
+ honest degraded-mode doctor
```

That captures GBrain's useful system lessons while preserving Gormes's core
advantages: small binary, static typing, local-first operation, and simpler
runtime ownership.
