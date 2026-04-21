---
title: "Phase 3 — The Black Box (Memory)"
weight: 40
---

# Phase 3 — The Black Box (Memory)

**Status:** 🔨 3.A–3.D shipped; 3.E planned

**Deliverable:** SQLite + FTS5 + ontological graph + semantic fusion in Go; 3.E adds decay, cross-chat synthesis, and operational-visibility mirrors.

Phase 3 (The Black Box) is substantially delivered as of 2026-04-20: the SQLite + FTS5 lattice (3.A), ontological graph with async LLM extraction (3.B), lexical/FTS5 recall with `<memory-context>` fence injection (3.C), semantic fusion via Ollama embeddings with cosine similarity recall (3.D), and the operator-facing memory mirror (3.D.5) are all implemented. Remaining Phase 3 work is 3.E — decay, cross-chat synthesis, and the operational-visibility mirrors (session index, insights audit, tool audit, transcript export).

## Phase 3 sub-status (as of 2026-04-20)

- **3.A — SQLite + FTS5 Lattice** — ✅ implemented (`internal/memory`, `SqliteStore`, FTS5 triggers, fire-and-forget worker, schema v3a→v3d migrations)
- **3.B — Ontological Graph + LLM Extractor** — ✅ implemented (`Extractor`, entity/relationship upsert, dead-letter queue, validator with weight-floor patch)
- **3.C — Neural Recall + Context Injection** — ✅ implemented (`RecallProvider`, 2-layer seed selection, CTE traversal, `<memory-context>` fence matching Python's `build_memory_context_block`)
- **3.D — Semantic Fusion + Local Embeddings** — ✅ implemented (`entity_embeddings` table with L2-normalized float32 LE BLOBs; `Embedder` background worker calls Ollama `/v1/embeddings` with labeled template `Entity: {Name}. Type: {Type}. Context: {Description}`; in-memory vector cache with monotonic graph-version counter; `semanticSeeds` flat cosine scan (dot product on normalized vectors); hybrid fusion in `Provider.GetContext` chains lexical → FTS5 → semantic with dedup + MaxSeeds cap; opt-in via `semantic_enabled=true` + `semantic_model="<tag>"`; empty model is a complete no-op — zero HTTP calls, zero goroutine, zero cache RAM. Ship criterion proven live against Ollama: query `"tell me about my projects"` (no lexical match) surfaces the seeded project entity via cosine in 7s.)
- **3.D.5 — Memory Mirror (USER.md sync)** — ✅ implemented (async background goroutine exports SQLite entities/rels → Markdown every 30s; configurable path; atomic writes; SQLite remains source of truth; zero impact on 250ms latency moat)
- **3.E — Decay + Cross-Chat + Operational Mirrors** — ⏳ planned (see Phase 3.E Ledger below)

## Phase 3.E Ledger

Phase 3.E is the final Black Box milestone. It closes three orthogonal gaps: **memory decay** (old facts fade), **cross-chat synthesis** (one user, multiple chats, one graph), and **operational-visibility mirrors** (session index, insights audit, tool audit, transcript export). Each row is a separable spec.

| Subphase | Status | Upstream reference | Deliverable |
|---|---|---|---|
| 3.E.1 — Session Index Mirror | ⏳ planned | None (Gormes-original) | Read-only YAML mirror of bbolt `sessions.db` at `~/.local/share/gormes/sessions/index.yaml`; closes the bbolt opacity gap |
| 3.E.2 — Tool Execution Audit Log | ⏳ planned | None (exceeds Hermes) | Append-only JSONL at `~/.local/share/gormes/tools/audit.jsonl`; persistent record of every tool call with timing + outcome |
| 3.E.3 — Transcript Export Command | ⏳ planned | Exceeds Hermes (no upstream equivalent) | `gormes session export <id> --format=markdown` renders SQLite turns as human-readable Markdown; snapshot for sharing/backup |
| 3.E.4 — Extraction State Visibility | ⏳ planned | None (debug only) | Optional dead-letter footer in USER.md OR `gormes memory status` command showing extraction queue depth + recent errors |
| 3.E.5 — Insights Audit Log | ⏳ planned | `agent/insights.py` (preview) | Lightweight append-only JSONL at `~/.local/share/gormes/insights/usage.jsonl`; accumulates session counts, token totals, cost estimates per day. Full `InsightsEngine` port lands in 4.E |
| 3.E.6 — Memory Decay | ⏳ planned | None (Gormes-original) | Weight attenuation on relationships + `last_seen` tracking; stale facts age out of recall without deletion (reversible, audit-preserving) |
| 3.E.7 — Cross-Chat Synthesis | ⏳ planned | `agent/memory_manager.py` (cross-session) | Graph unification across `chat_id` boundaries for a single operator; query "what is Juan working on?" returns facts from Telegram, Discord, Slack in one fence. Requires a `user_id` concept above `chat_id` |

The 3.E ship criterion: the operator runs `cat ~/.local/share/gormes/sessions/index.yaml` and sees every active chat/session mapping in plain YAML; runs `cat ~/.local/share/gormes/tools/audit.jsonl` and sees a full history of tool invocations; a fact mentioned once six months ago and never again no longer dominates recall results; and asking the same question across two different chats surfaces the same entity graph.
