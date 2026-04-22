---
title: "Phase 3 — The Black Box (Memory)"
weight: 40
---

# Phase 3 — The Black Box (Memory)

**Status:** 🔨 3.A–3.D shipped; 3.E planned

**Deliverable:** SQLite + FTS5 + ontological graph + semantic fusion in Go; 3.E adds decay, cross-chat synthesis, operational-visibility mirrors, and the Honcho-shaped session/user boundaries the future plugin layer will depend on.

Phase 3 (The Black Box) is substantially delivered as of 2026-04-20: the SQLite + FTS5 lattice (3.A), ontological graph with async LLM extraction (3.B), lexical/FTS5 recall with `<memory-context>` fence injection (3.C), semantic fusion via Ollama embeddings with cosine similarity recall (3.D), and the operator-facing memory mirror (3.D.5) are all implemented. Remaining Phase 3 work is 3.E — decay, cross-chat synthesis, and the operational-visibility mirrors (session index, insights audit, tool audit, transcript export). Architecturally, this is the phase where Gormes finishes the memory substrate that a Honcho-style integration would stand on, without yet claiming full Honcho provider or plugin parity.

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

## Honcho architecture as the reference model

Honcho is useful here not just as a future plugin target, but as a clean reference architecture for what an agent-memory system needs to separate:

- **Workspace** — namespace and tenancy boundary for all memory objects
- **Peer** — the durable identity being modeled (human, agent, or other participant)
- **Session** — the temporal boundary of one conversation/thread/import
- **Background derivation** — async pipelines that convert raw messages into representations and summaries
- **Two read paths** — a fast session-context path for prompt assembly and a slower dialectic path for natural-language introspection over a peer's memory

That decomposition matters because it keeps three concerns distinct that are easy to blur in a simpler local-memory design:

1. **Who** is being modeled (`peer`)
2. **Where** an interaction happened (`session`)
3. **How** the system reconstructs useful context later (`representation`, `summary`, `dialectic`)

Gormes Phase 3 is converging toward that same separation in Go. The local SQLite lattice, graph extractor, and semantic recall already cover most of the "representation substrate" layer. The remaining 3.E work closes the gaps around session visibility, cross-chat identity, and decay so that future Honcho parity is a thin integration layer rather than a redesign of the memory core.

## Honcho-to-Gormes mapping

| Honcho concept | Role in Honcho | Phase 3 implication for Gormes |
|---|---|---|
| **Workspace** | Top-level namespace containing peers, sessions, and derived memory | Today this is effectively the local Gormes data root plus config scope. Full provider-facing workspace semantics remain a later integration concern, but Phase 3 must keep schemas and mirrors partitionable at that boundary. |
| **Peer** | Durable participant identity whose representation evolves over time | The closest Phase 3 equivalent is the entity/relationship graph plus USER.md mirror. Phase 3 builds the durable facts; explicit Honcho-style peer objects and peer-management UX are deferred. |
| **Session** | Bounded conversation/import container with messages and summary slots | 3.E.1 and 3.E.3 exist largely to make this boundary visible: operators need an inspectable session index and export path instead of opaque storage. |
| **Shared peer across many sessions** | One user can appear in many sessions/channels while preserving a single long-lived representation | 3.E.7 is the direct analogue: introduce a `user_id` concept above `chat_id` so Gormes can unify facts across Telegram, Discord, Slack, and future gateways without flattening all sessions into one stream. |
| **Background representation + summary pipeline** | Messages are written once, then async workers derive peer representations and session summaries | 3.B and 3.D already match the async side of this architecture via extractor and embedder workers. 3.E.4 adds the missing observability so operators can see queue depth and failures. |
| **`session.context()` path** | Fast, prompt-facing retrieval of summary + recent messages scoped to a session | Gormes already injects `<memory-context>` fences from lexical/graph/semantic recall. What is still missing is clearer session-boundary visibility and, if desired later, a fuller summary-oriented context contract. |
| **`peer.chat()` dialectic path** | Higher-level natural-language reasoning over a peer's learned representation | Not a Phase 3 deliverable. Phase 3 supplies the graph, decay model, and cross-chat identity that such a layer would query later, whether via Honcho plugin parity or a Gormes-native dialectic surface. |
| **Observation topology** | Asymmetric peer observation (`observe_me`, `observe_others`), dynamic agent peers, subagent hierarchies | Important for Honcho integration design, but outside the local memory-core mandate of Phase 3. That belongs with gateway/plugin parity once the substrate is stable. |

## Scope guard

Phase 3 should therefore be read as **Honcho-aligned substrate work**, not "port all of Honcho now."

- If the work is about local persistence, graph formation, semantic recall, session inspectability, cross-chat identity, or decay, it belongs in Phase 3.
- If the work is about Honcho API parity, peer-management commands, plugin wiring, dialectic tools, or remote workspace/session orchestration, it belongs later with the provider/plugin surface.

This boundary is deliberate. Phase 3 makes Gormes memory structurally compatible with Honcho's architecture without paying Honcho's full integration cost before the local Go memory core is finished.
