---
title: "Memory"
weight: 30
---

# Memory

Persistent, searchable state that outlives the process. Structured enough for graph traversal; flat enough for `grep`.

## Components shipped today

- **SQLite + FTS5 lattice** (3.A) — `internal/memory/SqliteStore`. Schema migrations, fire-and-forget worker, lexical search.
- **Ontological graph** (3.B) — entities, relationships, LLM-assisted extractor with dead-letter queue.
- **Neural recall** (3.C) — 2-layer seed selection, CTE traversal, `<memory-context>` fence injection matching Hermes's `build_memory_context_block`.
- **Semantic fusion** (3.D) — Ollama embeddings, cosine recall, and hybrid lexical+semantic seed fusion.
- **USER.md mirror** (3.D.5) — async export of entity/relationship graph to human-readable Markdown. Gormes-original; no upstream equivalent.
- **Tool audit JSONL** (3.E.2) — append-only JSONL from kernel and `delegate_task` tool execution with timing, outcome, and error fields.
- **Transcript export** (3.E.3) — `gormes session export <id> --format=markdown` renders SQLite turns, timestamps, and tool calls for operator sharing.
- **Operator visibility** (3.E.4, 3.E.5) — `gormes memory status` is shipped, and the local insights layer now persists append-only daily `usage.jsonl` records from `telemetry.Snapshot` rollups.
- **GONCHO compatibility seam** — internal memory work lives behind the `goncho` service, while the exported tool surface remains Honcho-compatible (`honcho_*`).

## Phase 3 closeout queue

- **Shipped visibility spine** (3.E.1–3.E.5) — session index mirror, tool audit, transcript export, memory status, and daily insights logging are landed.
- **`last_seen` closeout** (3.E.6) — shipped: schema v3g backfills `relationships.last_seen`, repeated relationship observations advance it without rewriting legacy `updated_at`, and recall attenuation uses `COALESCE(NULLIF(last_seen, 0), updated_at)`.
- **Cross-chat identity closeout** (3.E.7) — GONCHO identity hierarchy is `user_id > chat_id > session_id`; `internal/session` persists canonical chat-to-user bindings, and `internal/memory` now has the same-chat default fence plus opt-in canonical user/source-filtered recall. The internal GONCHO service accepts those parameters, but Honcho-compatible tool schema exposure plus deny-path fixtures and operator evidence still remain.
- **Session lineage + cross-source search closeout** (3.E.8) — source-filtered search spans one canonical `user_id` across chats inside `internal/memory` and the internal GONCHO service; `parent_session_id`, lineage-aware hits, and operator-auditable search evidence still remain.

## Identity + lineage contract

- **GONCHO identity hierarchy** — `user_id > chat_id > session_id`.
- **Recall fence** — same-chat by default; opt-in cross-chat only when a canonical `user_id` resolves.
- **Tool boundary** — `honcho_search` and `honcho_context` unmarshal `scope` / `sources` today through the internal service types, but their schemas do not advertise those fields yet.
- **Lineage rule** — `parent_session_id` is append-only metadata on descendants, not a rewrite of ancestor history.
- **Implementation plan** — `docs/superpowers/plans/2026-04-22-gormes-phase3-identity-lineage-plan.md`.
- **Execution plan** — `docs/superpowers/plans/2026-04-22-gormes-phase3-identity-lineage-execution-plan.md`; closeout order is `3.E.6.1 -> 3.E.7.2 -> 3.E.8.1 -> 3.E.8.2`.

## Why this is not just "chat logs"

Chat logs are append-only. Memory has schema. You query it, derive from it, inject it back into the context window. The SQLite + FTS5 combination gives you ACID durability *and* full-text search in a single ~100 KB binary dependency.

See [Phase 3](../architecture_plan/phase-3-memory/) for the full sub-status.
