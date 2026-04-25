---
title: "Goncho Honcho Memory"
weight: 46
---

# Goncho — The Honcho Port

**Status:** Living document. First pass covers philosophy, Honcho architecture (extracted from upstream `plastic-labs/honcho`), the internal Go port plan, and its relationship to Phase 3. The latest docs-driven delta from Honcho v3 is captured in [Honcho Docs Study Plan](./03-honcho-docs-study/), worker-ready execution slices are captured in [Agent Work Packets](./04-agent-work-packets/), and operator/runtime decisions are captured in [Operator Playbook](./05-operator-playbook/).

**Audience:** Gormes contributors and future AI agents continuing the port. This is an **architecture reference**, not a migration cookbook — the cookbook lives in `docs/superpowers/specs/2026-04-21-goncho-architecture-design.md` and the Phase 3 ledger in `architecture_plan/phase-3-memory.md`.

> **Instruction to future agents:** This document is **not complete**. It is explicitly too large to write in one pass. Each top-level section has a **"Coverage / TODO"** footer calling out what is still missing. When you open this file, pick one TODO, fill it in against the real upstream source at `/workspace-mineru/honcho/src/**` (not from memory or training), and cross it off. Do not rewrite existing sections without a reason — they are cross-referenced from Phase 3 and the Goncho spec. If a section drifts out of date with upstream Honcho, add a `> **Drift note (YYYY-MM-DD):** ...` admonition rather than silently rewriting.

---

## How To Use This Reference

This is the long-form Goncho port reference. Use it as a map, not as a single
linear read.

| Need | Read |
|---|---|
| Boundary and packaging rules | §0, §12, §14 |
| Honcho concepts to preserve | §1, §2, §6, §7 |
| API and tool edge | §3, [Tool Schemas](./02-tool-schemas/) |
| Deriver, dialectic, and dreamer mechanics | §5, [Prompts](./01-prompts/) |
| Latest docs-driven implementation gaps | [Honcho Docs Study Plan](./03-honcho-docs-study/) |
| Worker-ready memory implementation packets | [Agent Work Packets](./04-agent-work-packets/) |
| Workspace, peer, session, config, and diagnostics rules | [Operator Playbook](./05-operator-playbook/) |
| Implementation order | §13, then the Phase 3 ledger |
| Open follow-up work | Coverage/TODO footers and §15 |

When adding detail, update the smallest relevant section and cross off its
Coverage/TODO item in the same change. If a new topic needs more than a short
subsection, create a sibling page and link it from this index.

---

## 0. What Goncho Is (and Is Not)

**Goncho** is Gormes's internal, in-binary, Go-native port of [Honcho](https://github.com/plastic-labs/honcho) — the peer-centric memory and social-cognition substrate from Plastic Labs. Goncho's goals are:

1. **Integrated by default.** Goncho is not a sidecar. It is a Go package (`internal/goncho/`) that runs inside the same gormes binary, on the same SQLite substrate as Phase 3 memory, and uses the **same LLM pipeline** as the rest of gormes (`internal/hermes/` clients, `internal/kernel/` tool loop). No loopback HTTP, no second process, no second DB.
2. **Honcho-compatible at the tool edge.** The public tool surface (`honcho_profile`, `honcho_search`, `honcho_context`, `honcho_reasoning`, `honcho_conclude`) matches Honcho's mental model so callers — including other Claude/Honcho-literate models — keep the same vocabulary. Internally, the package is named `goncho` to make the port status explicit.
3. **Optionally exposed over HTTP.** As with Hermes, the Goncho service must be reachable over a minimal HTTP surface for external tools, agents, or a future managed deployment. The HTTP layer is a thin adapter over the same `goncho.Service` — never a parallel implementation.
4. **Non-destructive to Phase 3.** Goncho stands **on top of** the Phase 3 memory lattice (SQLite + FTS5 + graph + semantic fusion + memory mirror). It does not replace it. The substrate stays the source of truth; Goncho provides the Honcho-shaped read/write seams (peers, sessions, representations, dialectic, peer cards) that Phase 3 does not itself model.

Goncho is **not**:

- A 1:1 reimplementation of every Honcho SaaS feature. Managed workspaces, JWT key vending, webhook fan-out, pgvector/Turbopuffer/LanceDB adapters, and the full v3 REST surface are deferred behind flags until the local slice is stable.
- A drop-in server for Honcho's Python/TypeScript SDKs. The Honcho SDKs talk to `https://api.honcho.dev` v3 REST. Parity with that wire format is a **later** milestone (see §13.2).
- A standalone public repo or service in the first port. Goncho starts in-tree under `internal/goncho/`; extraction is allowed only after the core API has stabilized and only as an importable Go library.
- A prerequisite to any Phase 3 deliverable. Phase 3.E can ship without Goncho touching production paths.

### 0.1 Packaging Decision: In-Tree First, One Binary Always

> **Decision (2026-04-24):** Build Goncho in-tree inside Gormes first. Keep the package boundary extraction-ready, but do not create a standalone public Goncho repo or service until slices A-D are stable.

The invariant is that `go build ./cmd/gormes` produces the deployable artifact. Goncho may eventually live at `github.com/.../goncho`, but only as a Go module imported by Gormes. It must never become a required sidecar, daemon, loopback HTTP dependency, second database, or separate migration command.

The extraction-ready boundary is:

- `Service` — Honcho-shaped application API: profile, search, context, reasoning, conclude, deriver/dialectic/dreamer entry points.
- `Store` — observation/session/peer persistence backed by the same SQLite database and migration runner Gormes already owns.
- `LLM` — adapter over the existing Gormes/Hermes model pipeline, not a new provider stack.
- `Embedder` — adapter over the existing memory embedder and vector cache.
- `Clock` and `Logger` — injectable deterministic utilities for tests and operational integration.
- `migrations` — embedded into the Gormes binary and run through the Gormes migration path.
- `prompt fixtures` and `tool schema fixtures` — checked against `01-prompts.md` and `02-tool-schemas.md`.

If Goncho is extracted later, the public module should expose library packages only. Optional HTTP support remains an adapter package that Gormes can mount from inside the same process.

### Coverage / TODO

- [x] Add a one-paragraph "why a Go port at all, instead of calling Honcho over HTTP" answer citing the Phase 2→3 decision record. Replaced with the stronger one-binary packaging decision above.
- [ ] Link to the Hermes "optional-API" pattern so readers see the precedent for the optional-HTTP layer.

---

## 1. Honcho Philosophy

Honcho is marketed as "the identity layer for the agentic world." The substantive ideas underneath that tagline — the ones we need to preserve in Goncho — are:

### 1.1 The Peer Paradigm

Honcho treats **every participant as a peer**: a human user, an AI agent, a sub-agent, a tool, a group. There is no privileged `user`/`assistant` binary. This is explicit in upstream's blog post "Beyond the User-Assistant Paradigm" and in the data model (`src/models.py::Peer`). Peers are the durable identity unit; everything else (sessions, messages, observations) hangs off peers.

Consequence for Goncho: our identity unit is a `peer_name` scoped to a `workspace_name`, **not** `user_id` alone. Phase 3.E.7 introduces `user_id > chat_id > session_id` for recall scoping; Goncho layers `peer_name` on top of that so human users, the gormes agent itself, and any future sub-agents are all addressable the same way.

### 1.2 Asymmetric Observation

Peers can **observe** other peers. Honcho stores observations keyed by `(observer, observed)` — the facts "alice-according-to-tutor" and "tutor-according-to-alice" are different rows, different collections, different peer cards. Self-observation (`observer == observed`) is the common case; cross-peer observation is what makes multi-agent cognition possible.

See: `src/models.py::Collection` (`(workspace_name, observer, observed)` composite unique), `src/crud/peer_card.py::set_peer_card(observer, observed)`, `src/crud/representation.py::RepresentationManager(workspace_name, observer, observed)`.

Consequence for Goncho: peer cards and representations are keyed `(workspace, observer, observed)`, not just `(workspace, peer)`. This is already reflected in `internal/goncho/service.go`'s notion of an `observer` (defaults to `"gormes"`) and the `peer` target on requests.

### 1.3 Continual Learning via Derivation

Honcho's central claim on its [evals page](https://evals.honcho.dev/) is that **representations improve over time** because a background pipeline continuously derives facts from raw messages. The pipeline has three agents (see §6):

- **Deriver** — ingests messages, writes *explicit* and *deductive* observations, maintains peer cards.
- **Dialectic** — reads those observations plus fresh tool-search results to answer natural-language questions about a peer.
- **Dreamer** — periodically consolidates the observation pool: prunes redundancies, runs deductive and inductive specialists, updates peer cards.

Consequence for Goncho: a single "write message → vector store" path is not enough. We need all three roles, even if early Goncho collapses some of them into one-shot inline calls. The substrate (entities, relationships, embeddings, summaries) already exists in Phase 3; what Goncho adds is the **role separation** and the **observation level hierarchy** (explicit / deductive / inductive / contradiction).

### 1.4 Two Read Paths

Honcho exposes context through two complementary endpoints, and this distinction matters:

- **`session.context(summary=True, tokens=N)`** — fast, prompt-facing assembly of summary + recent messages up to a token budget. Sub-second. Used every turn.
- **`peer.chat(query)`** — slow, dialectic reasoning over a peer's learned representation. Uses a tool-calling loop and a reasoning-level-routed model. Used when the agent wants a "second opinion" about a peer or needs to hydrate prompts with *inferred* facts, not just stored text.

Consequence for Goncho: don't collapse these. The kernel already uses the fast path via `RecallProvider.GetContext()`. Goncho must keep the slow path (`honcho_reasoning`, later a full dialectic tool) available for callers who want model-mediated introspection — and must not block the kernel's 100ms recall budget on it.

### 1.5 Representations as Living Documents

A "representation" in Honcho is not a single string. It is a bundle:

```
Representation:
  explicit:  [ExplicitObservation{content}]
  deductive: [DeductiveObservation{conclusion, premises[]}]
```

plus the peer card (a bounded list of stable biographical facts, max 40 entries) plus session summaries (short every 20 msgs, long every 60). The dialectic agent reconstitutes these on demand; nothing is ever flattened into a single opaque "memory blob."

Consequence for Goncho: the internal representation type must preserve this shape. `internal/goncho/types.go` already exposes a `ContextResult` with `peer_card`, `representation`, `summary`, `conclusions`, `recent_messages` — keep those fields even when some are empty.

### Coverage / TODO

- [ ] Cite the specific Plastic Labs blog posts (peer paradigm, dialectic API introduction) with stable URLs once confirmed.
- [ ] Add a short "why this is different from RAG" subsection — Honcho is not a vector database with UX on top; the derivation agents are the product.

---

## 2. Honcho's Data Model (Upstream Reference)

Upstream source of truth: `/workspace-mineru/honcho/src/models.py`.

```
Workspace (tenant root, name-unique)
├── Peer (workspace-scoped name)
│   ├── internal_metadata["peer_card"]                 # self peer card
│   ├── internal_metadata["{observed}_peer_card"]      # observed peer card
│   └── Collections (one per (observer, observed) pair)
│       └── Documents (observations, embedded, levelled)
├── Session (workspace-scoped name, many-to-many with peers)
│   ├── SessionPeer (join row, per-peer configuration + joined_at/left_at)
│   ├── Message (session-scoped, seq_in_session, tokenized)
│   └── MessageEmbedding (1536-dim pgvector HNSW)
├── QueueItem (background task row: work_unit_key + payload)
├── ActiveQueueSession (worker ownership lease)
└── WebhookEndpoint
```

### 2.1 Core tables (abridged)

| Table | Purpose | Composite key notes |
|---|---|---|
| `workspaces` | Tenant root. Has `name` (unique) and `configuration` JSONB. | Single PK on `id` (nanoid). |
| `peers` | Durable identity. Name unique per workspace. `internal_metadata` JSONB stores peer cards. | `UniqueConstraint(name, workspace_name)`. |
| `sessions` | Conversation/thread container. `is_active` flag. | `UniqueConstraint(name, workspace_name)`. |
| `session_peers` | Association table (peer membership per session) with per-peer `configuration`, `joined_at`, `left_at`. | Composite FKs to both `sessions` and `peers`. |
| `messages` | Session-scoped. Tracks `seq_in_session`, `token_count`, `content` (≤65535), FTS GIN index on `to_tsvector('english', content)`. | Composite FKs to `sessions` and `peers`. |
| `message_embeddings` | 1536-dim HNSW-indexed embeddings, plus `sync_state` / `last_sync_at` / `sync_attempts` for the reconciler. | Tied to `messages.public_id`. |
| `collections` | One container per `(workspace, observer, observed)`. | `UniqueConstraint(observer, observed, workspace_name)`. |
| `documents` | The observation row. Key columns: `content`, `level` (explicit/deductive/inductive/contradiction), `times_derived`, `source_ids` (JSONB, GIN), `embedding` (1536-dim HNSW), `deleted_at` (soft delete), sync fields. | Composite FKs to collection + observer + observed + session. |
| `queue` | Task queue row. Key columns: `work_unit_key`, `task_type`, `payload` JSONB, `processed`, `error`, partial unique indexes for `reconciler` and `dream` pending dedup. | Separate from SQL row ordering via partial indexes. |
| `active_queue_sessions` | Worker lease: one row per `work_unit_key` a worker is holding. `last_updated` drives stale reclamation. | `work_unit_key` UNIQUE. |
| `webhook_endpoints` | Registered webhook targets per workspace. | — |

### 2.2 Enums worth memorising

From `src/utils/types.py`:

```python
TaskType        = "webhook" | "summary" | "representation" | "dream" | "deletion" | "reconciler"
DocumentLevel   = "explicit" | "deductive" | "inductive" | "contradiction"
VectorSyncState = "synced"  | "pending"    | "failed"
ReasoningLevel  = "minimal" | "low" | "medium" | "high" | "max"
```

Goncho must preserve `DocumentLevel` and `ReasoningLevel` verbatim. `TaskType` and `VectorSyncState` can stay internal but keep the same semantics when we eventually port the deriver and reconciler.

### 2.3 Goncho mapping

The Phase 3 doc already contains the canonical `GONCHO concept → Gormes implication` table (see `architecture_plan/phase-3-memory.md` §"GONCHO-to-Gormes mapping"). Goncho builds on the same table with one addition: the **observations / documents / collections** triad is not yet explicitly modeled in Gormes. Phase 3 stores facts as entities + relationships + embeddings; Goncho needs an explicit observation table with a `level` column (see §13 port plan).

### Coverage / TODO

- [ ] Add an ER diagram (Mermaid) of the upstream schema.
- [ ] Note which tables are strictly required for a Goncho MVP (hint: workspaces, peers, sessions, observations, peer cards) vs. optional (queue, active_queue_sessions, webhook_endpoints, message_embeddings).
- [ ] Record the exact `metadata` / `internal_metadata` split convention so we don't leak private keys into the public JSON surface.

---

## 3. API Surface

Upstream mounts all routers under `/v3` (confirmed in `src/main.py` lines 187–193). Every route is `/v3/{resource}/{id}/{action}`.

### 3.1 Router inventory

| Router | Prefix | Headline endpoints |
|---|---|---|
| `workspaces` | `/v3/workspaces` | `POST /` (get-or-create), `POST /list`, `PUT /{id}`, `DELETE /{id}`, `POST /{id}/search` |
| `peers` | `/v3/workspaces/{workspace_id}/peers` | `POST /list`, `POST /`, `PUT /{peer_id}`, **`POST /{peer_id}/chat`** (dialectic), `POST /{peer_id}/representation`, `GET/PUT /{peer_id}/card`, `GET /{peer_id}/context`, `POST /{peer_id}/search` |
| `sessions` | `/v3/workspaces/{workspace_id}/sessions` | `POST /list`, `POST /`, `PUT /{id}`, `DELETE /{id}`, `POST /{id}/clone`, `POST /{id}/peers`, `GET /{id}/context`, `POST /{id}/search` |
| `messages` | `/v3/workspaces/{workspace_id}/sessions/{session_id}/messages` | `POST /` (batch up to 100), `GET /list`, `POST /upload`, `GET/PUT/DELETE /{id}` |
| `conclusions` | `/v3/workspaces/{workspace_id}/documents` | Internal observation CRUD (mostly deriver-facing) |
| `keys` | `/v3/keys` | JWT provisioning |
| `webhooks` | `/v3/workspaces/{workspace_id}/webhooks` | Webhook endpoint registration |

### 3.2 The two endpoints that matter most

**Dialectic (`POST /v3/workspaces/{w}/peers/{p}/chat`)**

Request body: `{ query, session_id?, target?, reasoning_level? }` where `reasoning_level ∈ {minimal, low, medium, high, max}`.
Response: `{ content: str }`. Streaming variant returns `text/event-stream`.

Behavior: looks up peer cards, prefetches a pool of relevant observations (10 for `minimal`, 25 otherwise), then hands the query to `DialecticAgent.answer()` which runs a tool-calling loop (`execute_tool_loop`) against a per-level model config. Current executable tools are `search_memory`, `search_messages`, `get_observation_context`, `grep_messages`, `get_messages_by_date_range`, `search_messages_temporal`, and `get_reasoning_chain`; see [`02-tool-schemas.md`](./02-tool-schemas) for the verbatim schemas and the prompt/tool drift note.

**Session context (`GET /v3/workspaces/{w}/sessions/{s}/context`)**

Query params: `tokens`, `summary` (bool), `search_query`, `peer_target`, `peer_perspective`, `limit_to_session`, `search_top_k`, `search_max_distance`, `include_most_frequent`, `max_conclusions`.
Response: `SessionContext { summary?, messages[] }`.

Allocation: 40% of the token budget to the summary (when enabled), 60% to the newest messages that fit. If a long summary exists and fits, it is preferred over the short summary.

These two endpoints are the **fast path** (session context) and **slow path** (dialectic chat) from §1.4.

### 3.3 Search surface

Three symmetrical endpoints — workspace-wide, session-scoped, peer-scoped — all take `{ query, filters?, limit }` and fan through a hybrid strategy: FTS GIN on `messages.content` joined with pgvector HNSW on `message_embeddings.embedding`, fused via reciprocal-rank (RRF). External vector stores (Turbopuffer, LanceDB) can replace the HNSW side; see §5.

Honcho's docs also expose a filter grammar that Goncho does not yet model: logical `AND`/`OR`/`NOT`, comparison operators (`gt`, `gte`, `lt`, `lte`, `ne`, `in`), text operators (`contains`, `icontains`), nested `metadata`, and wildcard `"*"`. Goncho's first filter slice should add a typed AST and explicit unsupported-filter errors before any public HTTP parity claim.

### Coverage / TODO

- [ ] Enumerate every endpoint with its request/response Pydantic schema name. (Start by grepping `src/routers/*.py` for `@router.post/get/put/delete`.)
- [ ] Catalogue every `MessageSearchOptions` filter field — this is the biggest Honcho feature we haven't documented.
- [ ] Document the `/peers/{peer_id}/representation` endpoint shape (separate from `/chat`): it returns a static `Representation` doc for low-latency prompt hydration.
- [ ] Describe the batch-upload (`POST /messages/upload`) path and its max size.

---

## 4. Storage & Multi-Tenancy

### 4.1 Multi-tenant keying

Every row in Honcho is keyed by some subset of `(workspace_name, peer_name, session_name)`. The composite foreign keys in `src/models.py` are the authoritative spec. Goncho **must** keep that composite keying even though SQLite will flatten some of it into a single column for ergonomics — the invariant is that a workspace boundary is never crossed implicitly.

### 4.2 Vector store abstraction

Upstream supports **three** vector backends, selected via `config.VECTOR_STORE.TYPE`:

| Backend | Notes |
|---|---|
| `pgvector` | Default. 1536-dim, HNSW (`m=16`, `ef_construction=64`), cosine distance. Lives in-database. |
| `turbopuffer` | Managed cloud. Namespaces are `{prefix}.{type}.{base64url(sha256(workspace,observer,observed))}`. |
| `lancedb` | Local embedded. Useful for self-hosted deployments without a Postgres dep. |

The abstract interface (shape distilled from `src/vector_store/__init__.py`):

```
VectorStore:
  GetVectorNamespace(namespaceType: "document"|"message",
                     workspace, observer?, observed?) → string
  UpsertMany(namespace, vectors: [{id, embedding, metadata}]) → error
  Query(namespace, embedding, topK?, filters?, maxDistance?) → [{id, score, metadata}]
  DeleteMany(namespace, ids) → error
  DeleteNamespace(namespace) → error
  Close() → error
```

Goncho mapping: today our substrate uses Ollama embeddings stored as normalized float32 BLOBs on the `entity_embeddings` table (Phase 3.D). A Goncho-grade port needs either:

1. A second embeddings table scoped to observations (not entities), or
2. A polymorphic embeddings table keyed by `kind ∈ {entity, observation, message}`.

The Phase 3 embedder + in-memory cache + background reconciliation already handle the scaffolding; what's missing is the **namespace abstraction** and the `sync_state`/`sync_attempts` reconciliation loop.

### 4.3 Reconciler

Source: `src/reconciler/sync_vectors.py`.

- Polls `Document`/`MessageEmbedding` rows with `sync_state = "pending"` using `FOR UPDATE SKIP LOCKED`.
- Batch size 50, time budget 240s per cycle, cycle interval 300s (configurable).
- After `MAX_SYNC_ATTEMPTS = 5` failures, rows are marked `failed` and require manual intervention.
- Exposes a `ReconciliationMetrics` snapshot per cycle.

Goncho can skip this entirely as long as the write path is synchronous. Once we bolt on an external vector store, the reconciler becomes mandatory.

### Coverage / TODO

- [ ] Specify the Go equivalent interface (signatures in Go syntax, not Python-ish prose) for `VectorStore`.
- [ ] Describe the exact pgvector HNSW index (m, ef_construction, ops class) and the performance implications if we back Goncho with sqlite-vec or chromem-go instead.
- [ ] Document the namespace hashing collision profile (SHA256 truncated to base64url fits Turbopuffer's 128-char cap — preserve this even if we don't use Turbopuffer).

---

## 5. Reasoning Pipeline — The Three Agents

This is the part of Honcho most people underestimate. The "memory" lives in the observations table; the **intelligence** lives in the agents that write to and read from it.

### 5.1 Deriver (`src/deriver/`)

**Role:** ingest raw messages and turn them into observations.

**Trigger:** `POST /messages` enqueues one `QueueItem` per task type per new message. A long-running worker process (started with `uv run python -m src.deriver`) polls the queue.

**Work-unit partitioning (`src/utils/work_unit.py`):**

```
representation : "representation:{workspace}:{session}:{observed}"       # observer omitted on purpose
summary        : "summary:{workspace}:{session}:{observer}:{observed}"
dream          : "dream:{dream_type}:{workspace}:{observer}:{observed}"
webhook        : "webhook:{workspace}:{...}"
deletion       : "deletion:{workspace}:{...}"
reconciler     : "reconciler:{workspace}:{...}"
```

Per-work-unit FIFO ordering is enforced by `ActiveQueueSession` leases: a worker holding the lease on `work_unit_key` has exclusive rights to process pending rows for that key until the lease goes stale (`STALE_SESSION_TIMEOUT_MINUTES`).

**Representation batching:** representation tasks do not fire per-message. They accumulate until `REPRESENTATION_BATCH_MAX_TOKENS` is reached (or `FLUSH_ENABLED` forces drain). The worker then pulls a token-bounded slice of context + unprocessed queue items via `get_queue_item_batch()` and calls the deriver agent once for the whole slice.

**Deriver output path (`src/deriver/deriver.py`, current minimal deriver):**

- `minimal_deriver_prompt(peer_id, messages)` builds one prompt for the whole batch.
- `honcho_llm_call(..., response_model=PromptRepresentation, json_mode=True)` returns structured explicit observations.
- `Representation.from_prompt_representation(...)` converts the LLM output into the representation object.
- `RepresentationManager.save_representation(...)` writes the resulting observations into every observer collection.

> **Drift note (2026-04-24):** The current Honcho deriver does **not** run an agent tool loop. Observation creation tools exist in `src/utils/agent_tools.py`, but they are used by dreamer/dialectic-adjacent paths, not the minimal deriver ingestion path.

**Observation creation is locked.** Upstream uses a `WeakValueDictionary` of asyncio locks keyed by `(workspace, observer, observed)` so two concurrent workers cannot create duplicate rows for the same peer pair. Goncho needs the same locking, scoped to the same tuple.

### 5.2 Dialectic (`src/dialectic/`)

**Role:** answer natural-language queries about a peer using tools, observations, and peer cards.

**Entry points:**

- `agentic_chat(workspace, session, query, observer, observed, reasoning_level)` → `str` (non-streaming).
- `agentic_chat_stream(...)` → `AsyncIterator[str]`.

**Flow:**

1. Load `observer_peer_card` and `observed_peer_card` (list[str]).
2. `_initialize_session_history()` pulls recent messages up to `SESSION_HISTORY_MAX_TOKENS` (4096).
3. `_prefetch_relevant_observations(query)` runs **two** semantic searches — one over explicit observations, one over deductive — so neither category dilutes the other.
4. Route to model via `settings.DIALECTIC.LEVELS[reasoning_level].MODEL_CONFIG`.
5. `execute_tool_loop()` runs the tool-calling loop with `max_tool_iterations` and `tool_choice` from the level config.

**Level defaults (`src/config.py`):**

| Level | Max iterations | Tool choice | Prefetch size | Typical model |
|---|---|---|---|---|
| `minimal` | 1 | `any` | 10 | fastest/cheapest |
| `low` | 5 | `any` | 25 | default |
| `medium` | 2 | auto | 25 | reasoning-capable |
| `high` | 4 | auto | 25 | high-reasoning |
| `max` | 10 | auto | 25 | top-tier |

**Tool set (full, `DIALECTIC_TOOLS`):** `search_memory`, `search_messages`, `get_observation_context`, `grep_messages`, `get_messages_by_date_range`, `search_messages_temporal`, `get_reasoning_chain`. `DIALECTIC_TOOLS_MINIMAL` is `search_memory`, `search_messages`.

> **Drift note (2026-04-24):** `src/dialectic/prompts.py` still tells the agent it may use `create_observations_deductive`, but `src/utils/agent_tools.py::DIALECTIC_TOOLS` has that entry commented out. Port the executable tool list first; treat the prompt mismatch as upstream behavior unless deliberately fixed.

Goncho's current `honcho_search` / `honcho_context` / `honcho_reasoning` tools are a **subset** of this surface. Expansion strategy is in §13.

### 5.3 Dreamer (`src/dreamer/`)

**Role:** periodic consolidation and self-improvement of the observation pool.

**Trigger:** `DreamScheduler` (`src/dreamer/dream_scheduler.py`). A singleton asyncio coordinator that:

- `schedule_dream(work_unit_key, workspace, delay_minutes, dream_type, observer, observed)` — queues a dream after a delay.
- `cancel_dreams_for_observed(workspace, observed)` — fired from `enqueue.py` whenever new activity arrives; the dreamer should not run on a peer actively being observed.
- Uses `WeakValueDictionary` for self-cleaning task references.

**Orchestration (`src/dreamer/orchestrator.py::run_dream`):**

Phase 0 — *Surprisal sampling* (optional, `DREAM.SURPRISAL.ENABLED`). `src/dreamer/surprisal.py` loads observations + embeddings, builds a tree (RPTree / KDTree / BallTree / CoverTree / LSH / graph / prototype — `src/dreamer/trees/`), and computes a path-based surprisal score per observation. High-surprisal observations become *hints* for the specialists.

Phase 1 — **Deduction specialist** (`src/dreamer/specialists.py::DeductionSpecialist`). Tool set includes search, `create_observations_deductive`, `delete_observations`, `update_peer_card`.

Phase 2 — **Induction specialist**. Tool set includes search + `create_observations_inductive` + `update_peer_card`. Runs after deduction so it can see newly-inferred facts.

Both specialists hold **no DB session during LLM calls** — they use `tracked_db()` scoped contexts to fetch state, release, then call the model. This is the Honcho equivalent of the gormes "never hold a DB session during external calls" rule in our root CLAUDE.md.

**Result shape:** `DreamResult { run_id, deduction_result, induction_result, input_tokens, output_tokens, duration_ms }`.

### 5.4 Shared agent infrastructure

All three agents go through the same core:

```
honcho_llm_call(model_config, prompt, tools?, tool_executor?, max_tool_iterations, stream_final?, ...)
  └── if tools: execute_tool_loop(...)
         └── for each iteration:
                LLM call → parse tool_calls → tool_executor(name, args) → append results
                if no tool_calls: return final content (optionally streamed)
```

- `max_tool_iterations` is clamped to `[1, 100]`.
- `tool_choice` defaults to `required`/`any` on iteration 1, then switches to `auto` so the model can exit gracefully.
- `iteration_callback` hook + `set_current_iteration(...)` context var give per-iteration telemetry.
- Empty-response retry: one nudge if the LLM returns empty content.

Goncho's port of this layer must live alongside the existing `internal/kernel/` tool loop — ideally as a thin wrapper that reuses the same kernel primitives rather than introducing a second tool-execution engine. See §13.4.

### Coverage / TODO

- [ ] Spell out the exact JSON schema for each agent tool (input/output) — this is the single most useful artifact for porters.
- [ ] Diagram the queue state machine: `pending → claimed(lease) → processed | error | stale-reclaimed`.
- [ ] Document the deriver's "messages context" assembly (the CTE + cumulative-token window inside `get_queue_item_batch`).
- [ ] Describe the `finish_consolidation` tool (specialist exit signal).
- [ ] Capture the dreamer scheduling parameters: default `delay_minutes`, per-dream-type overrides, cancellation cascading.

---

## 6. Peer Cards

A peer card is a bounded list of short biographical strings (max 40) kept on `peers.internal_metadata`:

- Key `"peer_card"` when `observer == observed` (self-card).
- Key `"{observed}_peer_card"` when `observer ≠ observed` (relationship card).

**Lifecycle:**

1. Dreamer specialists call `update_peer_card(content=[...])` during consolidation; the tool truncates to `MAX_PEER_CARD_FACTS = 40` before write.
2. Dialectic preflight loads peer cards directly with `crud.get_peer_card(...)` and injects them into the system prompt; API callers read via `GET /peers/{peer_id}/card?target={observed}`.
3. `get_peer_card()` remains available to the omni dreamer tool set, but not to current `DIALECTIC_TOOLS`.

**API surface:** `GET /v3/workspaces/{w}/peers/{p}/card`, `PUT /v3/workspaces/{w}/peers/{p}/card { peer_card: [str] }`.

Goncho already implements `HonchoProfileTool` / `Profile()` / `SetProfile()` on top of the SQLite peer table. What's missing is the observer/observed distinction (today Goncho assumes `observer == "gormes"`) — that's a §13 port task.

### Coverage / TODO

- [ ] Document the dedup + truncation algorithm used by `update_peer_card` (likely fuzzy match + recency; verify by reading the tool implementation).
- [ ] Specify the JSON shape stored in `internal_metadata` so we don't invent our own.

---

## 7. Summaries

Source: `src/utils/summarizer.py`.

Two cadences, stored on `Session.metadata["summaries"]`:

- **Short summary** — every 20 messages (`SUMMARY.MESSAGES_PER_SHORT_SUMMARY`).
- **Long summary** — every 60 messages (`SUMMARY.MESSAGES_PER_LONG_SUMMARY`).

Summary record shape:

```
Summary {
  content: str
  message_id: int                 # last message covered
  summary_type: "honcho_chat_summary_short" | "honcho_chat_summary_long"
  created_at: iso8601
  token_count: int
  message_public_id: str
}
```

The session-context endpoint (`GET /sessions/{id}/context`) allocates 40% of the token budget to a summary and 60% to messages, preferring the long summary when it fits.

Goncho: Phase 3 does not yet generate summaries. When we add them, they live as a new `session_summaries` table on the same SQLite DB, with the same two-tier cadence. The summarizer is its own queue task type (`TaskType.summary`) in upstream.

### Coverage / TODO

- [ ] Specify the summarizer prompt contract.
- [ ] Describe how the summarizer coexists with Phase 3's existing last-N-turns recall so we don't double-bill the context budget.

---

## 8. Webhooks

Sources: `src/webhooks/events.py`, `src/webhooks/webhook_delivery.py`.

Event types (as of upstream `3.0.6`): `queue.empty`, `test.event`. Payload envelope:

```
{ "type": "queue.empty", "data": {...}, "timestamp": "YYYY-MM-DDThh:mm:ss.sssZ" }
```

Delivery:

- Publishing queues a `QueueItem` with `task_type="webhook"`.
- The webhook worker fetches registered endpoints for the workspace and POSTs via `httpx.AsyncClient` (gathered tasks).
- Signature header `X-Honcho-Signature` is HMAC-SHA256 over the body with `WEBHOOK_SECRET`.
- At-least-once semantics; there is no in-delivery retry — failed deliveries rely on the queue row staying pending.

Goncho: webhooks are explicitly deferred. The tool-edge callers inside gormes do not need them. If/when Goncho grows an HTTP surface (§13.2) and someone wants to subscribe to `queue.empty`, port this module verbatim.

### Coverage / TODO

- [ ] Catalogue every event type upstream plans to emit (watch for new ones in CHANGELOG.md).
- [ ] Document the exact body that is signed (raw JSON with keys in insertion order? canonicalised?) to avoid signing-mismatch bugs.

---

## 9. Configuration

Sources: `src/config.py`, `config.toml.example`, `README.md` §Configuration.

Priority order: **env vars > .env > config.toml > defaults.**

Section inventory (roughly one Pydantic model per section):

| Section | Key settings of note |
|---|---|
| `[app]` | `LOG_LEVEL`, `SESSION_OBSERVERS_LIMIT`, `MAX_MESSAGE_SIZE`, `NAMESPACE`, `EMBED_MESSAGES` |
| `[db]` | `CONNECTION_URI` (must start with `postgresql+psycopg`), `SCHEMA`, `POOL_SIZE`, `MAX_OVERFLOW`, `SQL_DEBUG` |
| `[auth]` | `USE_AUTH`, `JWT_SECRET` |
| `[cache]` | Redis connection for `cashews`; gracefully degrades when unreachable |
| `[llm]` | Provider API keys (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `LLM_GEMINI_API_KEY`), `DEFAULT_MAX_TOKENS` |
| `[embedding]` | `MODEL_CONFIG` (provider, model), `VECTOR_DIMENSIONS` (default 1536) |
| `[deriver]` | `ENABLED`, `WORKERS`, `MODEL_CONFIG`, `MAX_INPUT_TOKENS` (23k), `WORKING_REPRESENTATION_MAX_OBSERVATIONS` (100), `REPRESENTATION_BATCH_MAX_TOKENS`, `FLUSH_ENABLED`, `STALE_SESSION_TIMEOUT_MINUTES`, `DEDUPLICATE` |
| `[peer_card]` | `ENABLED` |
| `[dialectic]` | `LEVELS` dict (`minimal`/`low`/`medium`/`high`/`max`), each with `MODEL_CONFIG`, `MAX_TOOL_ITERATIONS`, `TOOL_CHOICE`, `MAX_OUTPUT_TOKENS` override; top-level `SESSION_HISTORY_MAX_TOKENS` (4096) |
| `[summary]` | `ENABLED`, `MESSAGES_PER_SHORT_SUMMARY`, `MESSAGES_PER_LONG_SUMMARY`, `MAX_TOKENS_SHORT`, `MAX_TOKENS_LONG` |
| `[dream]` | `ENABLED`, `DEDUCTION_MODEL_CONFIG`, `INDUCTION_MODEL_CONFIG`, `SURPRISAL.*` (tree type, k, top-percent) |
| `[webhook]` | `SECRET`, `MAX_WORKSPACE_LIMIT` |
| `[metrics]` | Prometheus pull settings |
| `[telemetry]` | CloudEvents push settings |
| `[vector_store]` | `TYPE` ∈ {`pgvector`, `turbopuffer`, `lancedb`}, `NAMESPACE`, `DIMENSIONS`, `RECONCILIATION_INTERVAL_SECONDS` (300) |
| `[sentry]` | `ENABLED`, DSN, traces sample rate |

**Model config structure (used everywhere):**

```
ModelConfig {
  model: str
  transport: "anthropic" | "openai" | "gemini"
  api_key?: str
  temperature?: float
  top_p?: float
  thinking_effort?: "none" | "minimal" | ... | "max"
  thinking_budget_tokens?: int       # ≥1024 for Anthropic
  max_output_tokens?: int
  cache_policy?: PromptCachePolicy
  fallback?: ResolvedFallbackConfig
}
```

Goncho must carry this shape (minus `fallback` for now) so that a user who wants Goncho to use a reasoning model on the dialectic path just sets `[goncho.dialectic.levels.high]` the same way they would in Honcho.

### Coverage / TODO

- [ ] Map each Honcho config section to the gormes `internal/config` section it will live under.
- [ ] Decide whether Goncho gets its own `[goncho]` namespace or piggybacks on the existing `[memory]` block; record the decision here.
- [ ] Document the env-var pattern (`DERIVER_MODEL_CONFIG__TRANSPORT`, `DIALECTIC_LEVELS__low__MODEL_CONFIG__MODEL`, etc.) so we can build a faithful equivalent or deliberately diverge.

---

## 10. Security

Source: `src/security.py`.

Auth is opt-in (`AUTH.USE_AUTH`). When on, Honcho issues JWTs with the following payload:

```
JWTParams {
  t:  iso timestamp (created)
  exp? iso timestamp (expires)
  ad? bool    # admin
  w?  str     # workspace scope
  p?  str     # peer scope
  s?  str     # session scope
}
```

Scope hierarchy (most→least powerful):

1. **Admin** (`ad=True`) — all workspaces.
2. **Workspace** (`w=X`) — everything inside workspace X.
3. **Peer** (`p=X`) — scoped to peer X only.
4. **Session** (`s=X`) — scoped to session X only.

Algorithm: HS256 via `PyJWT`; expiration checked when `exp` is present.

Goncho: we can keep `USE_AUTH=false` indefinitely inside the binary — the internal tool layer is already inside the gormes trust boundary. The JWT layer matters only when the optional HTTP surface (§13.2) ships.

### Coverage / TODO

- [ ] Describe the per-endpoint scope requirement table (e.g., which endpoints accept peer-scoped JWTs and which require workspace).
- [ ] Record the exact JWT claim names to stay wire-compatible with Honcho SDKs.

---

## 11. Storage / LLM / Deployment quick facts

- **Runtime:** Python 3.10+, FastAPI, SQLAlchemy, asyncio.
- **DB:** Postgres (required; `postgresql+psycopg` driver) with `pgvector`.
- **Worker:** separate process, `uv run python -m src.deriver`. Horizontally scalable (`DERIVER.WORKERS`).
- **Cache:** Redis via `cashews` (optional).
- **Migrations:** Alembic.
- **Observability:** Prometheus metrics at `/metrics`, CloudEvents telemetry, optional Sentry.
- **Deployment targets:** `docker-compose.yml.example`, `fly.toml` (API only; DB brought separately).

Goncho deliberately does **not** match most of this. Our runtime is Go, our DB is SQLite, our worker is an in-process goroutine pool, our metrics surface is gormes's existing telemetry stack. The mapping:

| Honcho runtime piece | Goncho equivalent |
|---|---|
| FastAPI routers | Optional `internal/goncho/http/` adapter (§13.2) |
| SQLAlchemy / Alembic | `internal/memory/schema.go` + `migrate.go` |
| pgvector HNSW | Phase 3.D: L2-normalized float32 BLOBs + cosine scan; future: sqlite-vec / chromem-go |
| Redis (`cashews`) | In-process LRU + Bolt session map |
| `uv run python -m src.deriver` | Goroutines off the same SQLite DB + fire-and-forget channels |
| Alembic migrations | `internal/memory/migrate.go` v3a→v3g+ |
| Prometheus `/metrics` | Existing gormes telemetry + `~/.local/share/gormes/tools/audit.jsonl` |

### Coverage / TODO

- [ ] Capture Honcho's migration strategy (autogenerate flow, downgrade story, data migration patterns) so we can parallel it in Go.

---

## 12. What's Already in Gormes

As of 2026-04-24 the scaffolding Goncho stands on is mostly built. Summaries below; details in the linked files.

### 12.1 Memory substrate (`internal/memory/`)

- `memory.go` — `SqliteStore` + fire-and-forget channel-based worker (bounded 1024-slot queue). Single writer.
- `schema.go` / `migrate.go` — v3a → v3g migrations. Tables: `messages`, `turns`, `entities`, `relationships`, `entity_embeddings`, session catalog rows.
- `graph.go` — entity/relationship upsert.
- `embedder.go` / `embed_client.go` — Ollama `/v1/embeddings`, L2-normalized float32 storage; opt-in, complete no-op when disabled.
- `recall.go` / `recall_sql.go` — hybrid lexical + FTS5 + semantic recall. Returns up to 12 seeds per query (configurable).
- `session_catalog.go` — `ListMetadataByUserID()` for cross-chat search; user/chat/session binding enforcement.
- `extractor.go` — LLM-driven entity/relationship extraction with dead-letter queue.
- `mirror.go` — Phase 3.D.5 atomic USER.md export every 30s.
- `cosine.go` — normalized dot-product math.

### 12.2 Session layer (`internal/session/`)

- `session.go` — `Map` interface (BoltMap prod, MemMap tests).
- `bolt.go` — bbolt-persisted `(platform:chat_id) → session_id` mappings plus metadata buckets.
- `directory.go` — `Directory.ListMetadataByUserID()`. `Metadata { SessionID, Source, ChatID, UserID, UpdatedAt }`. Enforces `user_id > chat_id` via `ErrUserBindingConflict`.

### 12.3 Goncho package (`internal/goncho/`) — already exists

- `service.go` — `Service { db, workspaceID, observer, recentLimit, sessions }`. Methods: `Profile`, `SetProfile`, `Search`, `Context`, `Conclude`.
- `types.go` — JSON-serializable `SearchParams`, `ContextParams`, `ConcludeParams`, `ContextResult { peer_card, representation, summary, conclusions, recent_messages }`. `SearchParams` and `ContextParams` already carry `scope ("user")` and `sources[]` fields for Phase 3.E.7 cross-chat opt-in.
- `sql.go` — raw SQL for peer cards, conclusions (via FTS5), turns fallback.
- `contracts_test.go` — parity contract tests against the Python Honcho shape.

### 12.4 Tool surface (`internal/tools/`)

`honcho_tools.go` exports five `honcho_*` tools backed by `goncho.Service`:

1. `honcho_profile` — read/update peer card.
2. `honcho_search` — search conclusions, fall back to turns.
3. `honcho_context` — structured context block.
4. `honcho_reasoning` — deterministic synthesis from context.
5. `honcho_conclude` — create/delete manual conclusions.

All are 5s-timeout-gated, JSON-marshaled, with hardcoded schemas. **The `scope` and `sources` params exist on the service but are not yet advertised in the tool schemas** — that's the Phase 3.E.7 closeout gate.

### 12.5 LLM pipeline (`internal/hermes/` + `internal/kernel/`)

- `internal/hermes/client.go` — `Client { OpenStream, OpenRunEvents, Health }`.
- `http_client.go` — generic OpenAI-compatible HTTP (Hermes, LM Studio, Open WebUI, upstream-compatible servers).
- `anthropic_client.go` — direct Anthropic SDK integration.
- `internal/kernel/kernel.go` — wires Client + `RecallProvider.GetContext()` with a 100ms deadline; executes tools via `tools.Registry`; injects memory as a `<memory-context>` fence.

**This is the LLM pipeline Goncho will reuse.** No parallel model-calling stack.

### 12.6 Config & tests

- `internal/config/config.go` — `TelegramCfg` already has Phase 3 memory fields (`RecallEnabled`, `RecallMaxFacts`, `SemanticEnabled`, `MirrorEnabled`, etc.). No dedicated `[goncho]` block yet.
- Build: `make build` (CGO_ENABLED=0 static binary at `./bin/gormes`).
- Test: `make test` (unit) / `make test-live` (integration, build tag `live`).

### 12.7 Pre-existing design docs (read these before editing)

- `docs/superpowers/specs/2026-04-21-goncho-architecture-design.md` — full design spec for Goncho.
- `docs/superpowers/plans/2026-04-21-goncho-immediate-slice.md` — minimum first slice.
- `docs/superpowers/plans/2026-04-21-gormes-goncho-momentum-sprint.md` — sprint plan.
- `docs/content/building-gormes/architecture_plan/phase-3-memory.md` — Phase 3 ledger and GONCHO-to-Gormes mapping table.
- `docs/content/building-gormes/porting-a-subsystem.md` — generic porting guidance.

### Coverage / TODO

- [ ] Keep this inventory synced. When `internal/goncho/` gains a new file, add it here with a one-line description.

---

## 13. Port Plan — From Phase-3 Substrate to Full Goncho

The port is intentionally staged so that each slice lands behind a flag and can be backed out. Numbering mirrors the upstream subsystem it ports.

### 13.1 Slice A — Observation table (blocking for everything else)

**Gap:** Phase 3 stores facts as entity/relationship rows; Honcho stores them as levelled `Document` rows. These are not the same thing. A conclusion "alice wants to learn calculus" is an explicit observation, not an entity.

**Tasks:**

- New table `observations` keyed `(workspace, observer, observed)` with columns `{ id, content, level, source_ids JSON, premises JSON, embedding BLOB?, times_derived, session_id?, deleted_at? }`.
- `DocumentLevel` enum = `explicit | deductive | inductive | contradiction`. Keep the exact names.
- FTS5 trigger on `content`.
- CRUD: `CreateObservations`, `DeleteObservations`, `SearchObservations(query, level?, top_k, scope)`.
- Wire `Service.Conclude` to this table instead of (or alongside) the existing graph.

**Owner:** Phase 3.E.7 closeout naturally forks into this slice; treat it as 3.F.1.

### 13.2 Slice B — Optional HTTP surface

**Gap:** Goncho is only reachable via in-process tools. Hermes precedent says we can expose an optional HTTP API, but it must stay an embedded adapter in the Gormes binary.

**Tasks:**

- `internal/goncho/http/` — handlers mounted under `/v3/workspaces/{w}/…` for a minimal subset: `GET/PUT /peers/{p}/card`, `GET /sessions/{s}/context`, `POST /peers/{p}/chat`, `POST /workspaces/{w}/search`.
- Reuse `Service` methods verbatim.
- Auth gated by `[goncho.auth]` section mirroring Honcho's `AUTH.USE_AUTH` + JWT flow (§10).
- Off by default. If exposed, `gormes goncho serve` starts the adapter inside the existing Gormes binary; it must not require a separate Goncho executable or process.

### 13.3 Slice C — Deriver (background representation)

**Gap:** Today, observations are created synchronously during tool calls. Honcho derives them in the background from raw message ingestion.

**Tasks:**

- `internal/goncho/deriver/` goroutine worker off the existing Phase 3 channel.
- `TaskType` enum and `work_unit_key` partitioning — **keep the upstream keys verbatim** so we can stand up the Python deriver against the same DB for side-by-side validation if we ever want to.
- Deriver uses one JSON-mode model call returning `PromptRepresentation`; port that exact structured-output path before adding any tool-loop behavior.
- Observation-level locking: keyed `(workspace, observer, observed)`, `sync.Map` + `*sync.Mutex` is the Go equivalent of upstream's `WeakValueDictionary[asyncio.Lock]`.
- Token-batched representation processing (`REPRESENTATION_BATCH_MAX_TOKENS`) — do **not** skip this; upstream batches for a reason (tool-loop cost).

### 13.4 Slice D — Dialectic agent

**Gap:** `honcho_reasoning` is a deterministic synthesis, not a full agentic dialectic.

**Tasks:**

- New `internal/goncho/dialectic/` package with `Agent { answer(query) string, answerStream(query) <-chan string }`.
- Per-level model config (`minimal`/`low`/`medium`/`high`/`max`) pulled from `[goncho.dialectic.levels.*]`. Defaults should mirror upstream (see §5.2 table).
- Prefetch: two separate semantic searches (explicit + deductive).
- Tool surface: start with the upstream minimal set; expand to the full `DIALECTIC_TOOLS` list as each tool is ported.
- Wire `honcho_reasoning` to call this agent when `agentic=true` is set.

### 13.5 Slice E — Dreamer

**Gap:** No consolidation pass exists. Facts accumulate until recall is slow.

**Tasks:**

- `internal/goncho/dreamer/scheduler.go` — delayed task dispatch keyed by `work_unit_key`; cancels on new activity for the observed peer.
- Orchestrator runs deduction specialist then induction specialist, each with its own tool set.
- Surprisal sampling is **deferred**. Start with "run deduction on the N newest observations since last dream." Add tree-based surprisal once the rest is stable.
- Never hold a DB session across LLM calls (matches our repo-wide rule from `CLAUDE.md`).

### 13.6 Slice F — Summaries

**Gap:** No session summaries yet.

**Tasks:**

- `session_summaries` table: `{ session_id, content, summary_type, message_id (last covered), created_at, token_count }`.
- Cadence: short every 20 messages, long every 60 — match upstream exactly to keep behavior predictable.
- Wire `Service.Context()` to allocate 40%/60% summary/messages, preferring long summary when it fits.

### 13.7 Slice G — Reconciler + external vector store (optional)

**Gap:** Today embeddings live in SQLite. Upstream lets operators swap in Turbopuffer or LanceDB.

**Tasks:**

- Add `sync_state` / `sync_attempts` / `last_sync_at` columns to the embeddings table.
- Port the `FOR UPDATE SKIP LOCKED` polling loop into a Go reconciler (modulo SQLite not supporting SKIP LOCKED — use an in-process lease).
- `VectorStore` interface behind an adapter. Provide a pgvector-compatible Go adapter (for users who run Postgres) and keep sqlite-vec as the default local backend.

### 13.8 Slice H — Webhooks

**Gap:** None of gormes needs webhooks internally.

**Tasks:**

- Defer until the HTTP surface (13.2) has real external subscribers.
- When built, match upstream payload envelope and signature algorithm exactly to preserve SDK compatibility.

### 13.9 Slice I — SDK-compatible wire format

**Gap:** Honcho's Python and TypeScript SDKs expect the v3 REST contract exactly.

**Tasks:**

- Once 13.1–13.6 are stable, promote the `internal/goncho/http/` adapter to v3 wire parity.
- Run the upstream SDKs against it as integration tests (see `sdks/python/tests/` and `sdks/typescript/tests/` in the Honcho repo for expected flows).
- This is the point at which we can honestly claim "drop-in self-host" parity.

### 13.10 Docs-driven planner cutlines

The Honcho v3 docs study split the next Goncho work into smaller rows in
`architecture_plan/progress.json` under `3.F - Goncho Honcho Memory Parity`.
Those rows are the current autoloop entry points:

1. `Goncho context representation options`
2. `Goncho search filter grammar`
3. `Directional peer cards and representation scopes`
4. `Goncho queue status read model`
5. `Goncho summary context budget`

Keep these rows synchronized with [Honcho Docs Study Plan](./03-honcho-docs-study/). If upstream docs add a new public parameter, add it to the study page first, then either refine one of these rows or add a new row with source refs, fixtures, write scope, tests, and done signal.

### Coverage / TODO

- [ ] For each slice above, link to (or create) a superpowers spec/plan under `docs/superpowers/specs/`.
- [ ] Capture explicit acceptance criteria per slice ("slice done" definition).
- [ ] Add a migration-risk column (what breaks if we land this slice half-done).

---

## 14. Relationship to Phase 3

Phase 3 (The Black Box) is deliberately narrower than Goncho. Phase 3 builds the **substrate**; Goncho is the **Honcho-shaped surface** on top of it. The boundary, copied from `architecture_plan/phase-3-memory.md` and sharpened here:

- **In Phase 3:** local persistence, graph formation, FTS5 + semantic recall, session inspectability, cross-chat identity (`user_id > chat_id > session_id`), memory decay, memory mirror.
- **In Goncho (post-Phase-3):** observation table with `level` column, peer-cards with observer/observed split, deriver/dialectic/dreamer agents, summaries, optional HTTP surface, JWT auth, vector-store abstraction, webhooks, SDK wire compatibility.

The Phase 3 scope guard is explicit: "Phase 3 makes Gormes memory structurally compatible with the Honcho-style architecture without paying the full provider-integration cost before the local Go memory core is finished." Goncho is exactly that deferred provider-integration cost, reclaimed slice by slice.

**Practical consequence for planners:**

- If a ticket is about persistence, recall, decay, or operator visibility of the memory core — file it under Phase 3 (likely 3.E).
- If a ticket is about peer cards with observers, dialectic reasoning levels, representation/summary endpoints, cross-peer observation, or wire compatibility with Honcho SDKs — file it under Goncho (§13 above).

Once Phase 3 ships cleanly, Goncho slice A (the observation table) can start immediately; it does not need to wait for Phase 4.

### Coverage / TODO

- [ ] Keep a running crosswalk between Phase 3.E ledger rows and Goncho slice IDs so the two documents do not drift.

---

## 15. Open Questions

These are un-resolved and should be answered before the corresponding slice lands:

1. **Entities vs. observations.** Should Phase 3 entities back-fill into `observations(level='explicit')` automatically, or stay as a parallel graph? Current working hypothesis: parallel graph, with observations generated from it lazily.
2. **Embedding dimensionality.** Upstream uses 1536 (OpenAI text-embedding-3-small shape). Phase 3.D uses whatever Ollama returns (varies by model, typically 768 / 1024). When observations get their own embeddings, do they share the entity embedder or use a separate, higher-dim one?
3. **Single writer vs. pooled writers.** Phase 3's SQLite writer is single-threaded by design. Upstream's deriver runs N workers partitioned by `work_unit_key`. SQLite can support the latter via WAL + per-work-unit mutexes, but the first correctness bug in that territory is expensive — this needs a decision with an owner.
4. **Dialectic reasoning-level naming.** Do we keep `minimal/low/medium/high/max` verbatim (Honcho-compatible) or align with the gormes kernel's own reasoning tiers? Verbatim is the current default.
5. **Peer card max size.** Upstream caps at 40 entries. Is that the right cap for long-lived gormes users across multiple years of sessions? We may need a per-workspace override.

Resolved: packaging is no longer open. Goncho is in-tree first, extraction-ready as a library, and Gormes remains one binary.

### Coverage / TODO

- [ ] Resolve each open question with a decision + date + author, then convert to a "Decisions" subsection at the top of this doc.

---

## 16. Instructions for Future Agents Continuing This Document

This file is intentionally incomplete. When you pick it up:

1. **Read, don't recall.** Always open the actual upstream files under `/workspace-mineru/honcho/src/**` before writing. Honcho ships fast; training data goes stale. Cite file paths.
2. **One TODO at a time.** Each section has a `Coverage / TODO` footer. Pick the smallest one that still matters. Cross it off in the same commit you fill it in.
3. **Preserve cross-references.** This doc is linked from `architecture_plan/phase-3-memory.md` and `docs/superpowers/specs/2026-04-21-goncho-architecture-design.md`. If you restructure top-level sections, update both references in the same PR.
4. **Don't collapse sections for brevity.** The goal is a reference an engineer can port from without re-reading the Python source. Terseness is not a virtue here.
5. **Drift notes over silent rewrites.** If upstream changes contradict what's already here, add a `> **Drift note (YYYY-MM-DD):** …` admonition below the affected paragraph rather than deleting the old text. Later readers need the audit trail.
6. **Mirror the enums verbatim.** `DocumentLevel`, `ReasoningLevel`, `TaskType`, `VectorSyncState`, `WebhookEventType` — never translate, never abbreviate. A Goncho install must feel Honcho-shaped at the edge.
7. **Respect the CLAUDE.md rules** — no new top-level files, no unrequested README edits, no amending prior commits. Extend this doc in place.
8. **Prefer editing over adding new docs.** If a new subsection of Goncho needs 20+ pages, create a sibling doc (e.g. `goncho_deriver.md`) and link to it from the relevant section here. Keep this file as the index + philosophy + port plan.
9. **Keep the language sober.** Goncho is a port with open questions, not a finished product. Claims like "supports X" must be backed by a pointer into Go source. Otherwise say "planned" or "deferred."

### Priority order for the next few passes

1. Keep [`01-prompts.md`](./01-prompts) and [`02-tool-schemas.md`](./02-tool-schemas) synchronized with upstream first. These close the prompt/tool-schema replication blockers; any drift changes agent behavior.
2. Keep [Honcho Docs Study Plan](./03-honcho-docs-study/) synchronized with the v3 docs so planner rows reflect the public SDK-facing contract.
3. Fill §3 Coverage TODOs — exhaustive route+schema catalogue is high-leverage for the HTTP surface slice.
4. Fill §5 Coverage TODOs that are not already covered by `01-prompts.md` / `02-tool-schemas.md` — focus on runtime mechanics, output formats, and failure modes.
5. Resolve §15 open questions one at a time.
6. Draft §9 env-var naming decision so we stop blocking on config bikeshed.
7. Once slice A (observation table) has a spec in `docs/superpowers/specs/`, turn §13.1 into a link rather than an inline task list.

---

**Cross-references**

- Upstream: [`plastic-labs/honcho`](https://github.com/plastic-labs/honcho), local mirror `/workspace-mineru/honcho/`.
- Replication kit: [`01-prompts.md`](./01-prompts) for verbatim prompts and [`02-tool-schemas.md`](./02-tool-schemas) for verbatim Honcho agent tool schemas.
- Docs study: [`03-honcho-docs-study.md`](./03-honcho-docs-study) maps Honcho v3 docs to Goncho planner rows.
- Goncho service: `internal/goncho/service.go`, `types.go`, `sql.go`.
- Tool layer: `internal/gonchotools/honcho_tools.go`.
- Memory substrate: `internal/memory/` (full Phase 3 inventory in §12.1).
- Design spec: `docs/superpowers/specs/2026-04-21-goncho-architecture-design.md`.
- Phase 3 ledger: `docs/content/building-gormes/architecture_plan/phase-3-memory.md`.
- Sprint plan: `docs/superpowers/plans/2026-04-21-gormes-goncho-momentum-sprint.md`.
- Porting guide: `docs/content/building-gormes/porting-a-subsystem.md`.
