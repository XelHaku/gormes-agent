---
title: "Operator Playbook"
weight: 5
---

# 05 - Operator Playbook

Last studied: 2026-04-26.

Source root: `/home/xel/git/sages-openclaw/workspace-mineru/honcho/docs`.

This page records the operational and integration rules learned from the
Honcho docs that are not obvious from source code alone. Its purpose is to keep
future agents from rediscovering the same workspace, peer, session, CLI,
configuration, and streaming decisions before each implementation slice.

## Source Corpus

Study these docs before changing this page:

- `v3/documentation/features/storing-data.mdx`
- `v3/documentation/features/advanced/streaming-response.mdx`
- `v3/documentation/core-concepts/design-patterns.mdx`
- `v3/documentation/reference/cli.mdx`
- `v3/contributing/self-hosting.mdx`
- `v3/contributing/configuration.mdx`
- `v3/contributing/troubleshooting.mdx`
- `v3/guides/migrations/mem0.mdx`
- `v3/migrations/from-mem0.mdx`
- `v3/guides/integrations/crewai.mdx`
- `v3/guides/integrations/langgraph.mdx`
- `v3/guides/integrations/n8n.mdx`
- `v3/guides/integrations/paperclip.mdx`
- `v3/guides/integrations/sillytavern.mdx`
- `v3/guides/integrations/zo-computer.mdx`

## Go-Native Boundary

Honcho self-hosting runs an API server, a deriver process, PostgreSQL with
pgvector, and optionally Redis. Goncho must not copy that runtime topology.

Goncho keeps these Go-native substitutions:

| Honcho runtime expectation | Goncho/Gormes plan |
|---|---|
| FastAPI server | Optional in-binary HTTP adapter mounted by `gormes`, off by default |
| Separate deriver process | In-process worker pool over SQLite queues |
| PostgreSQL + pgvector | Existing SQLite + FTS5 + semantic embedding substrate |
| Redis cache | In-process cache or existing Bolt/session mirrors only when needed |
| `.env` / `config.toml` | Existing Gormes precedence: CLI flags > env > TOML > defaults |
| `honcho doctor` | Extend `gormes doctor` or add `gormes goncho doctor` |
| `honcho workspace queue-status` | Extend `gormes memory status` with Goncho queue status |

The Honcho docs still matter because they define what operators expect to
observe: startup readiness, queue backlog, deriver progress, session context,
peer cards, conclusions, and configuration source precedence.

## Topology Decisions

### Workspace

Default local workspace should be `gormes`. Do not create one workspace per
user. Use additional workspaces only for hard isolation:

- different application;
- different deployment environment;
- explicitly separate host/tool memory;
- tests that need isolated fixtures.

### Peer

Use stable, scoped peer IDs:

- Human user: canonical `internal/session.Metadata.UserID`.
- Platform-specific fallback: `<source>:<chat_id>` only until canonical
  `user_id` is known.
- Gormes assistant: `gormes`.
- Subagent: `agent:<run_id>` or `agent:<role>:<run_id>` once the subagent
  runtime exposes durable IDs.
- Imported document owner: the peer the document describes, not the importer
  process.

Observation defaults:

- user peers: `observe_me=true`;
- deterministic bot/transport peers: `observe_me=false`;
- Gormes self peer: configurable, default off until self-representation
  fixtures exist;
- parent agents observing subagent sessions: `observe_me=false`,
  `observe_others=true`;
- cross-peer observation: opt-in only, never inferred from group membership.

### Session

Use one session for each context boundary:

- chat thread or platform channel for gateway conversations;
- project directory or repository for coding-agent hosts;
- git branch only when branch-specific context matters;
- import batch for legacy files, email batches, or Mem0 exports;
- child session for delegated/subagent work with parent lineage metadata.

Avoid too many tiny sessions. Summaries and `session.context()` are scoped to a
session, so splitting a continuous conversation into many sessions fragments the
context and delays summary usefulness.

### SillyTavern Host Mapping

Goncho fixture-locks the Honcho SillyTavern integration contract without
porting the browser extension or Node server plugin.

- Peer mode `Single peer for all personas` maps to one durable user peer.
- Peer mode `Separate peer per persona` maps to a persona-scoped user peer and
  degrades if the persona name is missing instead of merging back to the shared
  peer.
- Session naming `Auto` maps to one session per chat hash, `Per character` maps
  to one persistent session per character, and `Custom` maps to a user-named
  session. Existing sessions stay frozen; reset reports the orphaned active
  session and resolves a new session for the next chat message.
- Group chats map each character to a distinct peer and lazy-add characters
  who join mid-chat on their first message. Never collapse a group chat into one
  synthetic group peer.
- Enrichment modes map to base prompt context, `honcho_chat`-style reasoning,
  or honcho-prefixed tool exposure. Unsupported panel knobs are reported as
  degraded host evidence until a Goncho status surface implements them.

## Ingestion Rules

Storing a message is the canonical way to trigger future memory. A message is
always `(workspace, session, peer, content, metadata, created_at?)`.

Goncho import paths should follow this order:

1. **Raw messages** when timestamps and session structure are available. This
   gives summaries and derivation the best evidence.
2. **File uploads/imports** for legacy Markdown, JSON, text, and documentation.
   These create normal session messages with file metadata and no stored
   original bytes.
3. **Manual conclusions** for quick migrations from Mem0-like memory blobs when
   raw transcripts are unavailable.

Do not treat conclusion import as equivalent to raw-message import. It skips
the session history needed for summaries and future derivation.

## Streaming Contract

Honcho streams only the dialectic chat response. It does not stream
`session.context()`. Streaming callers are expected to accumulate the full
assistant response and then persist that response as a normal message.

Goncho's first streaming slice should therefore implement the storage contract
before a transport:

- `stream=true` is accepted at the tool/service edge;
- unsupported streaming returns explicit degraded evidence until implemented;
- once streaming exists, partial chunks are not stored as messages;
- interrupted streams do not flush partial assistant messages into memory;
- the final accumulated assistant response is stored once, after successful
  completion.

This aligns with the existing interrupted-turn memory sync suppression row in
Phase 3.E.

## Configuration Map

Honcho config priority is environment > `.env` > `config.toml` > defaults.
Gormes already uses CLI flags > env > TOML > defaults, with dotenv files loaded
before env parsing. Goncho should use the same Gormes loader and add a
`[goncho]` namespace instead of spreading settings into `[telegram]`.

Planned `[goncho]` keys:

| Key | Default | Honcho source |
|---|---|---|
| `enabled` | `true` | local feature gate |
| `workspace` | `gormes` | SDK default workspace behavior |
| `observer_peer` | `gormes` | current `goncho.Config.ObserverPeerID` |
| `recent_messages` | `4` | current service default |
| `max_message_size` | `25000` | `MAX_MESSAGE_SIZE` |
| `max_file_size` | `5242880` | `MAX_FILE_SIZE` |
| `get_context_max_tokens` | `100000` | `GET_CONTEXT_MAX_TOKENS` |
| `reasoning_enabled` | `true` | `reasoning.enabled` |
| `peer_card_enabled` | `true` | `PEER_CARD_ENABLED` |
| `summary_enabled` | `true` | `SUMMARY_ENABLED` |
| `dream_enabled` | `false` until fixtures | `DREAM_ENABLED` |
| `deriver_workers` | `1` | `DERIVER_WORKERS` |
| `representation_batch_max_tokens` | `1024` | `DERIVER_REPRESENTATION_BATCH_MAX_TOKENS` |
| `dialectic_default_level` | `low` | chat endpoint default |

Environment names should follow the existing Gormes pattern:
`GORMES_GONCHO_WORKSPACE`, `GORMES_GONCHO_OBSERVER_PEER`,
`GORMES_GONCHO_DIALECTIC_DEFAULT_LEVEL`, etc.

Do not add provider-specific model config until the actual deriver, dialectic,
or dream worker uses it. Early config fields should be observable but not
pretend inactive agents are running.

## Operator Diagnostics

Honcho's CLI docs define a useful diagnostic ladder. Goncho should expose the
same evidence through `gormes doctor`, `gormes memory status`, and a future
`gormes goncho` namespace.

Minimum checks:

1. **Config:** show loaded config path, `[goncho]` effective workspace,
   observer peer, and feature gates.
2. **Database:** memory DB path exists; schema version is current; FTS tables
   exist; Goncho tables exist.
3. **Session catalog:** session metadata can be read; canonical
   `user_id > chat_id > session_id` bindings are not conflicting.
4. **Tools:** Honcho-compatible tool schemas are registered.
5. **Context:** render a dry-run context for a supplied session/peer without
   calling an LLM.
6. **Queue:** show representation, summary, and dream work units separately
   from the existing extractor queue.
7. **Conclusions:** show conclusion count by observer/observed pair.
8. **Summaries:** show short/long summary freshness per session when the table
   exists.
9. **Provider readiness:** only check LLM/embedder reachability for enabled
   Goncho agents that actually call models.
10. **Degraded modes:** print unavailable features explicitly instead of
    silently omitting them.

Recommended exit codes:

- `0`: all enabled features are usable;
- `1`: bad operator input or missing local files;
- `2`: local database/schema/tool failure;
- `3`: configured model/provider/auth failure.

## Common Mistakes To Fixture-Lock

Turn these into tests before implementing the related feature:

- workspace-per-user instead of peer-per-user;
- assistant peers observed by default when they are deterministic;
- cross-chat recall widened without `scope=user`;
- source filters silently ignored;
- context split into too many tiny sessions;
- queue-empty used as a correctness condition;
- partial streaming responses written as memory;
- import files stored as raw bytes;
- manual conclusions treated as raw message history;
- `localhost` assumptions in host/container integration docs;
- config fields accepted but not shown in doctor/status output.

## Future-Agent Checklist

Before changing Goncho memory behavior:

1. Name the workspace/peer/session topology the change assumes.
2. State whether it touches raw messages, conclusions, summaries, peer cards,
   observations, or queue status.
3. Add a red test for the topology or degraded mode first.
4. Update `04-agent-work-packets.md` if the packet boundaries change.
5. Update `progress.json` when a row becomes more specific or a new row is
   needed.
6. Regenerate progress outputs if `progress.json` changes.
7. Run the packet's tests plus `go run ./cmd/builder-loop progress validate`.
