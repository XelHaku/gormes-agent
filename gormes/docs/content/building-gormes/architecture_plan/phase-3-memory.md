---
title: "Phase 3 — The Black Box (Memory)"
weight: 40
---

# Phase 3 — The Black Box (Memory)

**Status:** 🔨 3.A–3.E.5 and 3.E.7–3.E.8 shipped; 3.E.6 remains

**Deliverable:** SQLite + FTS5 + ontological graph + semantic fusion in Go; 3.E closes auditability, decay, cross-chat synthesis, and the GONCHO-shaped session/user boundaries the future plugin layer will depend on while preserving Honcho-compatible interfaces.

Phase 3 (The Black Box) is substantially delivered as of 2026-04-23: the SQLite + FTS5 lattice (3.A), ontological graph with async LLM extraction (3.B), lexical/FTS5 recall with `<memory-context>` fence injection (3.C), semantic fusion via Ollama embeddings with cosine similarity recall (3.D), and the operator-facing memory mirror (3.D.5) are all implemented. The 3.E closeout queue now has one remaining gap: session index mirror (3.E.1), tool audit (3.E.2), transcript export (3.E.3), extraction visibility (3.E.4), the lightweight insights writer (3.E.5), cross-chat recall fencing over canonical `user_id > chat_id > session_id` bindings (3.E.7), and the full `3.E.8` lineage/search bundle are shipped; only `last_seen`-based decay completion (3.E.6) still remains. Architecturally, this is the phase where Gormes finishes the memory substrate that a GONCHO-style integration would stand on, without yet claiming full Honcho provider or plugin parity.

## Phase 3 sub-status (as of 2026-04-23)

- **3.A — SQLite + FTS5 Lattice** — ✅ implemented (`internal/memory`, `SqliteStore`, FTS5 triggers, fire-and-forget worker, schema v3a→v3d migrations)
- **3.B — Ontological Graph + LLM Extractor** — ✅ implemented (`Extractor`, entity/relationship upsert, dead-letter queue, validator with weight-floor patch)
- **3.C — Neural Recall + Context Injection** — ✅ implemented (`RecallProvider`, 2-layer seed selection, CTE traversal, `<memory-context>` fence matching Python's `build_memory_context_block`)
- **3.D — Semantic Fusion + Local Embeddings** — ✅ implemented (`entity_embeddings` table with L2-normalized float32 LE BLOBs; `Embedder` background worker calls Ollama `/v1/embeddings` with labeled template `Entity: {Name}. Type: {Type}. Context: {Description}`; in-memory vector cache with monotonic graph-version counter; `semanticSeeds` flat cosine scan (dot product on normalized vectors); hybrid fusion in `Provider.GetContext` chains lexical → FTS5 → semantic with dedup + MaxSeeds cap; opt-in via `semantic_enabled=true` + `semantic_model="<tag>"`; empty model is a complete no-op — zero HTTP calls, zero goroutine, zero cache RAM. Ship criterion proven live against Ollama: query `"tell me about my projects"` (no lexical match) surfaces the seeded project entity via cosine in 7s.)
- **3.D.5 — Memory Mirror (USER.md sync)** — ✅ implemented (async background goroutine exports SQLite entities/rels → Markdown every 30s; configurable path; atomic writes; SQLite remains source of truth; zero impact on 250ms latency moat)
- **3.E — Decay + Cross-Chat + Operational Mirrors** — 🔨 mixed closeout (3.E.1–3.E.5 and 3.E.7–3.E.8 are shipped; only 3.E.6 `last_seen` tracking remains)

## Phase 3.E Ledger

Phase 3.E is the final Black Box milestone. It closes four orthogonal gaps: **operational visibility** (session index, tool audit, transcript export, extractor status), **memory decay** (old facts fade), **cross-chat synthesis** (one user, multiple chats, one graph), and the remaining **SessionDB donor seams** (`parent_session_id` lineage plus cross-source search). Each row is a separable spec.

| Subphase | Status | Priority | Upstream reference | Deliverable |
|---|---|---|---|---|
| 3.E.1 — Session Index Mirror | ✅ shipped | P0 | None (Gormes-original) | Read-only YAML mirror of bbolt `sessions.db` at `~/.local/share/gormes/sessions/index.yaml`; deterministic background refresh now runs from the TUI, Telegram, and shared gateway entrypoints without rewriting unchanged snapshots |
| 3.E.2 — Tool Execution Audit Log | ✅ shipped | P0 | None (exceeds Hermes) | Append-only JSONL at `~/.local/share/gormes/tools/audit.jsonl`; persistent record of every tool call with timing + outcome |
| 3.E.3 — Transcript Export Command | ✅ shipped | P2 | Exceeds Hermes (no upstream equivalent) | `gormes session export <id> --format=markdown` renders SQLite turns as human-readable Markdown; snapshot for sharing/backup |
| 3.E.4 — Extraction State Visibility | ✅ shipped | P1 | None (debug only) | `gormes memory status` shows extractor queue depth, dead-letter summaries, and worker-health heuristics |
| 3.E.5 — Insights Audit Log | ✅ shipped | P3 | `agent/insights.py` (preview) | Local `telemetry.Snapshot` rollups plus append-only `usage.jsonl` persistence are landed |
| 3.E.6 — Memory Decay | 🔨 in progress | P1 | None (Gormes-original) | Deterministic recall-time weight attenuation is landed; `last_seen` tracking still remains so stale facts can age out without deletion |
| 3.E.7 — Cross-Chat Synthesis | ✅ shipped | P2 | `agent/memory_manager.py` (cross-session) + `SessionDB.user_id` | `internal/session` persists canonical `user_id > chat_id > session_id` metadata, and `internal/memory` recall stays same-chat by default unless callers opt into canonical user scope with optional source filters |
| 3.E.8 — Session Lineage + Cross-Source Search | ✅ shipped | P4 | `hermes_state.py` (`parent_session_id`, `search_messages`, `search_sessions`) | `internal/session.Metadata` now persists `parent_session_id` + `lineage_kind`, rejects trivial lineage loops, and the bbolt session mirror exposes lineage audit fields alongside the already-landed source-filtered search path in `internal/memory/session_catalog.go` and Goncho `scope=user` / `sources[]` |

The 3.E ship criterion: the operator runs `cat ~/.local/share/gormes/sessions/index.yaml` and sees every active chat/session mapping in plain YAML; runs `cat ~/.local/share/gormes/tools/audit.jsonl` and sees a full history of tool invocations; a fact mentioned once six months ago and never again no longer dominates recall results; asking the same question across two different chats surfaces the same entity graph; and context-compressed branches no longer disappear into opaque IDs because lineage and source-filtered search are queryable.

## TDD Priority Queue

The Phase 3 queue is not one flat backlog. The order matters because later memory features need operator visibility and stable identity seams before they can be debugged safely.

1. **P0 — 3.E.1 Session Index Mirror closeout**
   Freeze the runtime refresh semantics with failing golden tests first. The YAML shape is already shipped; the remaining risk is wiring refresh without mutating session state or blocking the hot path.
2. **P1 — 3.E.6 `last_seen` closeout**
   The read model, append-only insights writer, and recall-time attenuation are already present. Add tests around fixed-clock decay math and `last_seen` updates before touching live workers or ranking.
3. **Delivered — 3.E.7 Cross-Chat Synthesis**
   The `user_id` merge rules are pinned in `internal/session`, and `internal/memory/recall.go` now widens recall across chats only when callers opt into a canonical user scope with optional source filters.
4. **Delivered — 3.E.8 Session lineage + cross-source search**
   `internal/session` now carries durable `parent_session_id` + `lineage_kind` metadata, the session mirror exposes orphan-aware lineage audit fields, and the earlier source-filtered session/message search work closes the remaining donor seam from Hermes `SessionDB`.

## Execution blueprint (2026-04-22)

The delivery sequence is frozen in `docs/superpowers/plans/2026-04-22-gormes-phase3-identity-lineage-execution-plan.md`:

`3.E.6.1 -> 3.E.7.2 -> 3.E.8.1 -> 3.E.8.2`

That order is intentional even though some enabling code is already landed:

- `3.E.6.1` goes first so `last_seen` freshness exists before cross-chat/session widening is treated as reliable.
- `3.E.7.2` remains a closeout gate, not just a code-presence check: recall and goncho helpers exist, but tool-schema exposure and deny-path evidence still gate completion.
- `3.E.8.1` adds `parent_session_id` and lineage semantics only after the recall fence is proven safe.
- `3.E.8.2` closes the remaining lineage-aware session search and operator evidence work last.

Current code is ahead of the old narrative in `internal/memory/recall.go`, `internal/memory/session_catalog.go`, and `internal/goncho/service.go`; the ledger stays conservative until freshness, lineage metadata, and operator-auditable surfaces all line up.

## Identity + lineage architecture freeze (2026-04-22)

Before this plan, `3.E.7` and `3.E.8` were only coarse placeholders in the ledger and this page: current code had `chat_id` plus `session_id`, but no durable `user_id` or `parent_session_id` contract, and `internal/memory/recall.go` still allowed exact-name recall to cross chat boundaries when an entity was named directly. The architecture target frozen in `docs/superpowers/plans/2026-04-22-gormes-phase3-identity-lineage-plan.md` is now landed: the `user_id`, recall-fence, source-filtered search, and `parent_session_id` lineage halves are implemented via `internal/session`, `internal/memory`, and `internal/goncho`; the remaining 3.E implementation target is `last_seen` freshness in `3.E.6`.

The frozen contract is:

- Canonical GONCHO identity hierarchy is `user_id > chat_id > session_id`.
- Recall stays `same-chat default, opt-in cross-chat`.
- `parent_session_id` is append-only lineage metadata for compression/fork descendants; roots remain null.
- Source-filtered search runs across sessions for one canonical `user_id`, not by flattening all chats into one undifferentiated stream.

This matters because the current memory substrate is already strong enough to make a bad identity decision expensive: once facts, conclusions, and tool-visible context start spanning multiple chats, any ambiguity around "who" a chat belongs to becomes a correctness and privacy problem rather than a mere schema nuisance.

## Pre-Phase 4 E2E Gate (Hermes still running)

Before starting Phase 4 implementation work, run and freeze a hybrid end-to-end baseline while Hermes is still the upstream brain (`api_server`) and Gormes owns runtime/gateway/memory surfaces.

### Why this gate exists

- It gives a parity reference before the Brain Transplant introduces new failure modes.
- It separates "bridge baseline regressions" from "native orchestrator regressions."
- It locks operator-facing contracts (routing, tool loop, memory fence shape, delivery semantics) before replacing the Python core path.

### Required E2E scenarios

1. **Gateway routing path**: inbound platform event -> session resolution -> kernel turn -> outbound delivery.
2. **Tool-call loop path**: model requests tool(s) -> tool execution -> tool result continuation -> final assistant output.
3. **Delegation path**: `delegate_task` child execution, allowlist/blocked-tool enforcement, terminal result envelope.
4. **Memory path**: recall injection fence present (`<memory-context>`), expected seed/fact shape, no leak across fenced scope.
5. **Operator visibility path**: session index/tool audit/transcript export surfaces produce deterministic artifacts.

### Exit criteria (must pass)

- E2E suite is green in CI and locally against Hermes-backed runtime.
- Golden outputs are stored for key contract surfaces (memory fence + export format + delivery envelope).
- Known acceptable divergences are documented explicitly (none implied by omission).
- A "Phase 4 can start" note is added to the sprint/plan artifact with exact command set used.

This gate is a prerequisite for Phase 4 powertrain work, not optional polish.

## GONCHO architecture as the internal reference model

Internally, Gormes refers to the local memory-service seam as **GONCHO**. The exported tool surface remains **Honcho-compatible interfaces** (`honcho_*`) so callers do not lose the upstream mental model while the Go substrate hardens.

GONCHO is useful here not just as a future plugin target, but as a clean reference architecture for what an agent-memory system needs to separate:

- **Workspace** — namespace and tenancy boundary for all memory objects
- **Peer** — the durable identity being modeled (human, agent, or other participant)
- **Session** — the temporal boundary of one conversation/thread/import
- **Background derivation** — async pipelines that convert raw messages into representations and summaries
- **Two read paths** — a fast session-context path for prompt assembly and a slower dialectic path for natural-language introspection over a peer's memory

That decomposition matters because it keeps three concerns distinct that are easy to blur in a simpler local-memory design:

1. **Who** is being modeled (`peer`)
2. **Where** an interaction happened (`session`)
3. **How** the system reconstructs useful context later (`representation`, `summary`, `dialectic`)

Gormes Phase 3 is converging toward that same separation in Go. The local SQLite lattice, graph extractor, and semantic recall already cover most of the "representation substrate" layer. The remaining 3.E work closes the gaps around session visibility, cross-chat identity, and decay so that future GONCHO / Honcho parity is a thin integration layer rather than a redesign of the memory core.

## GONCHO-to-Gormes mapping

| GONCHO concept | Role in the Honcho-compatible model | Phase 3 implication for Gormes |
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

Phase 3 should therefore be read as **GONCHO-aligned substrate work**, not "port all of Honcho now."

- If the work is about local persistence, graph formation, semantic recall, session inspectability, cross-chat identity, or decay, it belongs in Phase 3.
- If the work is about Honcho API parity, peer-management commands, plugin wiring, dialectic tools, or remote workspace/session orchestration, it belongs later with the provider/plugin surface.

This boundary is deliberate. Phase 3 makes Gormes memory structurally compatible with the Honcho-style architecture without paying the full provider-integration cost before the local Go memory core is finished.
