---
title: "Architecture"
weight: 10
---

# GBrain Architecture

## High-Level Shape

GBrain is a TypeScript/Bun personal knowledge runtime with three overlapping
surfaces:

1. A deterministic brain engine: import, parse, chunk, embed, store, search,
   link, and report health.
2. An agent-facing tool layer: one operation catalog generates CLI behavior,
   MCP tool definitions, and subagent-safe tool allowlists.
3. A skillpack and orchestration layer: markdown skills teach agents workflows,
   while Minions runs durable deterministic jobs and resumable LLM subagents.

The important architectural pattern is "thin harness, fat skills." The code
handles deterministic state transitions and safety boundaries. Markdown skills
carry most of the workflow intelligence.

## Source Map

Core evidence files in upstream GBrain:

- `src/core/operations.ts` - contract-first operation definitions; 1334 lines.
- `src/core/engine.ts` - `BrainEngine` interface; 272 lines.
- `src/core/postgres-engine.ts` - Postgres implementation; 1112 lines.
- `src/core/pglite-engine.ts` - PGLite implementation; 1060 lines.
- `src/core/import-file.ts` - parse, hash, chunk, embed, transact import path.
- `src/core/search/hybrid.ts` - keyword plus vector search, RRF, boosts, dedup.
- `src/core/link-extraction.ts` - markdown/wiki/frontmatter graph extraction.
- `src/core/minions/queue.ts` - durable SQL job queue; 1281 lines.
- `src/core/minions/worker.ts` - in-process worker with locks and aborts.
- `src/core/minions/handlers/subagent.ts` - resumable Anthropic LLM loop.
- `src/mcp/server.ts` and `src/mcp/tool-defs.ts` - MCP generation from ops.
- `src/schema.sql` - Postgres schema for pages, chunks, links, jobs, subagents.
- `skills/` - shipped procedural skills and conventions.

Repository size at study time:

- `src`: 154 files.
- `test`: 171 files.
- `docs`: 59 files.
- `skills`: 60 files.

## Layer Diagram

```text
User, agent, cron, or webhook
        |
        v
Markdown skill workflow
        |
        v
Operation catalog: src/core/operations.ts
        |
        +--> CLI: src/cli.ts
        +--> MCP: src/mcp/server.ts
        +--> subagent tool allowlist: src/core/minions/tools/brain-allowlist.ts
        |
        v
BrainEngine interface
        |
        +--> PGLiteEngine, local zero-config
        +--> PostgresEngine, remote or Supabase scale path
        |
        v
Database schema: pages, chunks, links, timeline, files, jobs, subagent ledgers
```

This is the part Gormes should study most closely. One operation definition
drives multiple external surfaces, so tool drift is testable instead of hidden.

## Data Model

GBrain stores knowledge as pages, not as raw chat transcripts only.

Key tables:

- `sources`: multi-source namespace boundary.
- `pages`: slug, type, title, compiled truth, timeline, frontmatter, hash.
- `content_chunks`: chunk text, chunk source, embedding, model, token count.
- `links`: typed page-to-page edges with provenance fields.
- `timeline_entries`: structured events.
- `raw_data`: sidecar source data.
- `files`: binary attachment metadata.
- `page_versions`: snapshot history.
- `ingest_log`: audit trail.
- `config`: brain-level settings.

The knowledge-page pattern is:

```text
frontmatter
compiled_truth: current best assessment
timeline: append-only evidence trail
typed links: graph relationships to other pages
chunks: indexed read model for retrieval
```

Gormes already has SQLite turns, FTS5, entities, relationships, semantic
embeddings, and GONCHO-aligned session/user boundaries. GBrain adds a stronger
"knowledge page" read model and explicit link provenance.

## Import And Write Path

The page write path in `src/core/import-file.ts` is:

```text
content
  -> size guard
  -> markdown/frontmatter parse
  -> content hash for idempotency
  -> recursive chunking of compiled truth and timeline
  -> embeddings, when OPENAI_API_KEY is present
  -> transaction:
       create version when replacing
       put page
       reconcile tags
       upsert or delete chunks
```

`put_page` in `operations.ts` then optionally runs post-write auto-link and
auto-timeline extraction. Remote MCP writes skip auto-link/timeline because a
remote caller could plant link text that affects future search ranking. That is
a useful trust-boundary lesson for Gormes.

## Search Path

`src/core/search/hybrid.ts` implements a layered retrieval pipeline:

```text
query
  -> intent/detail detection
  -> keyword search
  -> optional query expansion
  -> optional embedding and vector search
  -> Reciprocal Rank Fusion
  -> compiled-truth boost
  -> source-aware curated/bulk weighting
  -> cosine re-score
  -> backlink boost
  -> dedup
  -> detail fallback
```

If no OpenAI key exists, GBrain falls back to keyword search. That is the right
operator posture: capability degrades, but the tool still returns useful local
state.

For Gormes, the equivalent should be:

```text
FTS5 lexical seeds
  + graph traversal
  + local semantic seeds
  + recency/freshness weighting
  + source-tier boosts/excludes with temporal bypass
  -> deterministic fusion
  -> context fence
```

## Operation Contract

`operations.ts` defines each operation with:

- `name`
- `description`
- `params`
- `handler`
- optional `mutating`
- optional CLI hints

Then:

- CLI parsing maps `cliHints` to commands.
- MCP maps `params` to JSON schema.
- Subagent tools select from an explicit allowlist.
- Tests pin operation uniqueness and MCP schema parity.

This is better than having separate CLI, HTTP, MCP, and model-tool definitions
drift independently.

## Trust Boundary

GBrain has an explicit caller flag:

```text
OperationContext.remote = false  -> trusted local CLI
OperationContext.remote = true   -> untrusted agent-facing MCP
```

Examples:

- `file_upload` confines remote paths and rejects symlink escapes.
- `shell` jobs are CLI-only and require a worker environment flag.
- subagent `put_page` writes are forced under `wiki/agents/<subagentId>/`.
- auto-link is disabled for remote writes.

For Gormes, the equivalent should be a first-class trust class on every tool:

```text
operator-local
gateway-remote
child-agent
scheduled-system
```

The trust class should affect schema, filesystem access, network access,
mutating permissions, audit fields, and prompt-visible tool lists.

## Minions Job Queue

Minions is GBrain's durable work system. It lives in `src/core/minions`.
The current operator-facing skill is `skills/minion-orchestrator/SKILL.md`,
which merged the older `gbrain-jobs` routing intent into one control surface
for deterministic shell jobs and LLM subagent jobs. That naming matters for
Gormes planning: borrow the unified policy surface, but do not rename Go-native
`delegate_task` or `internal/subagent` APIs to Minions.

Important mechanics:

- SQL-backed `minion_jobs` state machine.
- statuses: waiting, active, delayed, waiting-children, paused, completed,
  failed, dead, cancelled.
- lock tokens and lock renewal.
- stall detection, wall-clock timeouts, retries, backoff, jitter.
- parent-child jobs, depth caps, max-children caps.
- idempotency keys and `maxWaiting` backpressure.
- per-job progress JSON.
- token counters.
- side-channel inbox with `child_done` messages.
- attachments.
- supervisor process wrapper for worker restart.

The subagent runtime adds:

- `subagent_messages` persisted by message index.
- `subagent_tool_executions` two-phase ledger: pending, complete, failed.
- provider rate leases.
- transcript rendering and audit JSONL.

This is the strongest donor idea for Gormes subagents and cron. Gormes already
has goroutine subagents with context cancellation, timeouts, run logs, and tool
allowlists. The missing class is durable rehydration after process death, plus
a trust-aware routing rule that keeps privileged deterministic work separate
from child-agent judgment work.

## Skills Layer

GBrain's `skills/` directory is not just docs. It is a procedural runtime for
agents. The resolver and conventions route work to skill files that encode:

- when to activate
- required tool order
- quality gates
- storage rules
- migration instructions
- operational disciplines

The strongest idea is the resolver-plus-conformance discipline:

- tests verify resolver references.
- routing eval fixtures catch phrasings that route to the wrong skill.
- skillpack install/update tracks managed blocks.
- `skillify` turns repeated failures into new skills.

Gormes already has a static `internal/skills` runtime. The next step is not more
prompt text; it is conformance, routing evals, active/inactive promotion rules,
and operator-visible evidence.

## Documentation For Agents

GBrain is deliberately agent-readable:

- `AGENTS.md` gives non-Claude agents install and operating protocol.
- `CLAUDE.md` is a dense source map.
- `llms.txt` and `llms-full.txt` provide ingestible documentation indexes.
- docs describe troubleshooting, migration, MCP setup, jobs, skill development,
  and operational disciplines.

This matters because agent platforms fail when the repo does not teach them
where to start. Gormes should keep this pattern, but with smaller docs that stay
closer to code ownership and tests.
