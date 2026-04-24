# Goncho Architecture Design

## Goal

Embed Honcho-like memory behavior directly inside the `gormes` binary with no sidecar API, while keeping external compatibility where it matters today:

- Tool names stay `honcho_profile`, `honcho_search`, `honcho_context`, `honcho_reasoning`, `honcho_conclude`
- CLI naming stays `gormes honcho ...` for now
- Internal naming becomes `goncho`

This slice is the first durable foundation for a full in-binary memory system, not a temporary wrapper.

## Final Naming Decision

- Internal package: `gormes/internal/goncho`
- Internal docs term: `Goncho`
- External compatibility surface:
  - tools: `honcho_*`
  - CLI: `gormes honcho ...`
- Future aliases are allowed:
  - `goncho_*`
  - `gormes goncho ...`

## Scope Guardrails

Track separation is explicit:

- Track A: Phase 2.E subagents
- Track B: gateway/adapters
- Goncho work is a separate memory track and must not be used to mix the two

Gateway/adapters direction is fixed for later work:

- Choose gateway chassis + one real adapter
- Reuse Telegram scaffolding first
- Land one real adapter next, preferring Discord unless Slack credentials are already available

This document only covers Goncho.

## Runtime Shape

Goncho is an internal subsystem layered on top of the existing SQLite-backed Gormes memory runtime.

Packaging decision, 2026-04-24:

- Build Goncho in-tree inside Gormes first.
- Keep the package boundary extraction-ready, but do not publish a standalone Goncho repo before the observation table, deriver, dialectic, and tool-schema contract are stable.
- If extracted later, Goncho must be a Go library imported by Gormes, not a required service, daemon, sidecar, loopback API, second database, or separate migration command.
- `go build ./cmd/gormes` remains the deployable artifact.

The extraction-ready boundary is:

- `Service`: Honcho-shaped application API.
- `Store`: persistence over the same SQLite database and migration runner Gormes owns.
- `LLM`: adapter over the existing Gormes/Hermes model pipeline.
- `Embedder`: adapter over the existing memory embedder.
- `Clock` and `Logger`: injectable runtime utilities.
- `migrations`, prompt fixtures, and tool-schema fixtures embedded into the Gormes build.

The binary shape is:

1. `internal/store` and `internal/memory`
   - raw turn persistence
   - bounded worker queues
   - FTS and semantic substrate
2. `internal/goncho`
   - Honcho-like domain model and service contracts
   - projector, derivation, retrieval, context assembly, reasoning
3. `internal/tools`
   - `honcho_*` compatibility tools backed by `internal/goncho`
4. `cmd/gormes` and adapter runtimes
   - runtime wiring
   - operator commands
   - diagnostics

No HTTP loopback, no extra process, no RPC bridge.

## Artifact Model

### Stable Keys

- `workspace_id`: required
- `peer_id`: required
- `session_key`: optional and explicit

`session_key` is the only session scoping identifier Goncho should rely on for derived facts. It is more stable for Gormes than pushing everything through a nullable `session_id`.

### Core Artifacts

- `peer_card`
  - global by peer
  - compact grounding facts
  - updated over time
- `conclusion`
  - durable derived fact or operator-authored fact
  - scoped by `workspace_id`, `observer_peer_id`, `peer_id`
  - optionally scoped by `session_key`
- `representation`
  - derived on read
  - perspective-sensitive
  - not persisted as the primary source of truth
- `summary`
  - session-level compressed history
  - short and long variants
- `context`
  - token-budgeted read product composed from the above artifacts plus recent messages

### Default Identity Mapping

- workspace: Gormes profile namespace
- ai peer: active profile or agent identity
- user peer: stable platform identity such as `telegram:6586915095`
- session strategy:
  - gateway: per chat
  - CLI: per directory
  - manual override when needed

## Pipeline

The write path must stay cheap and must never block the kernel on Goncho derivation.

### Write Path

1. Kernel persists raw turn
2. Goncho projector resolves workspace, session, and peer identity
3. Worker batches turns by session scope
4. Extractor emits observations and evidence links
5. Deriver consolidates conclusions with dedupe
6. Summary scheduler produces short and long summaries
7. Embedder computes vectors for conclusions and summaries
8. Caches are invalidated or refreshed
9. Recall builder serves the next turn
10. Tools and CLI consume the same service layer

### Operational Invariants

- bounded channels only
- kernel must never wait on Goncho derivation
- idempotency keys on derived writes
- queue item states:
  - `pending`
  - `processed`
  - `dead_letter`
- retries use capped backoff
- failures are observable through doctor and operator commands

## Read Path

### `honcho_search`

Primary retrieval order:

1. conclusion embeddings
2. session summaries
3. raw message excerpts as fallback

### `honcho_context`

Token budget is split across:

- session summary
- peer card
- representation
- retrieved conclusions
- recent messages

### `honcho_reasoning`

- with model availability: synthesize from the same context blocks
- without model availability: return a deterministic summary built from those blocks

### Auto Injection

Kernel auto-injection must call the same `Context()` builder with:

- small token budget
- hard deadline
- no alternate hidden path

## Immediate Slice

The approved first implementation slice is intentionally narrower than the full architecture.

### In Scope Now

1. Honcho/Goncho parity contract
   - spec
   - contract tests
2. schema migration for:
   - peer cards
   - conclusions
3. minimal `internal/goncho` service:
   - profile
   - search
   - context
   - conclude
4. `honcho_*` tools backed by that service

### Explicitly Deferred

- advanced reasoning orchestration
- projector worker
- extraction worker
- automatic summary generation
- embeddings-backed conclusion ranking
- operator CLI commands
- doctor checks

These are the next slice after the minimal service is stable.

## First Schema Slice

The first migration should add internal Goncho tables, using Goncho naming rather than Honcho naming.

Minimum tables:

- `goncho_peer_cards`
  - `workspace_id`
  - `peer_id`
  - `card_json`
  - `updated_at`
- `goncho_conclusions`
  - `id`
  - `workspace_id`
  - `observer_peer_id`
  - `peer_id`
  - `session_key`
  - `content`
  - `kind`
  - `status`
  - `source`
  - `idempotency_key`
  - `evidence_json`
  - `created_at`
  - `updated_at`

Recommended now:

- FTS over `goncho_conclusions.content`

Deferred:

- summaries table
- embeddings table for conclusions
- queue and dead-letter tables

## Minimal Service Contract

### Profile

- read global peer card
- update card atomically

### Search

- search conclusions first
- fall back to raw turns when session context is provided
- cap results by token budget

### Context

- build a deterministic response object
- include empty summary fields until summary generation lands
- derive representation from peer card plus retrieved conclusions

### Conclude

- create durable manual conclusions
- delete by id
- generate deterministic idempotency keys for repeated inserts

## Open Risks

- The current tool execution path does not automatically inject runtime `session_key` into tool execution. This first slice should keep `session_key` optional and explicit in the contract rather than guessing.
- The current TUI runtime still uses `NoopStore`, so live Goncho tools will first be registered where SQLite memory actually exists.
- Raw turns do not yet carry full Goncho identity metadata, so projector work remains necessary for full parity.

## Next Slice

After the immediate slice is stable:

1. projector + retry state machine
2. extraction and derivation workers
3. summary generation
4. conclusion embeddings
5. advanced `honcho_reasoning`
6. operator CLI and doctor checks
