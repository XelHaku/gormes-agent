---
title: "Agent Work Packets"
weight: 4
---

# 04 - Agent Work Packets

Last studied: 2026-04-25.

Source root: `/home/xel/git/sages-openclaw/workspace-mineru/honcho/docs`.

This page turns the Honcho docs study into executable work packets. A future
agent should be able to pick one packet, write the named red test, edit only the
listed files, run the listed validation, and update `progress.json` without
re-reading the full Honcho docs tree.

## Always Read First

Before starting any packet, read:

- `docs/content/building-gormes/goncho_honcho_memory/03-honcho-docs-study.md`
- `docs/content/building-gormes/architecture_plan/progress.json`
- `internal/goncho/types.go`
- `internal/goncho/service.go`
- `internal/goncho/sql.go`
- `internal/gonchotools/honcho_tools.go`
- `internal/memory/schema.go`

If the work touches generated roadmap pages or the site, also run:

- `go run ./cmd/autoloop progress validate`
- `go run ./cmd/autoloop progress write`
- `go test ./docs -count=1`
- `(cd www.gormes.ai && go test ./... -count=1)`

## Packet 1 - Context Representation Options

Progress row: `3.F / Goncho context representation options`.

Purpose: make `honcho_context` accept the Honcho v3 `session.context()`
representation controls while preserving current same-chat behavior.

Source docs:

- `../honcho/docs/v3/documentation/features/get-context.mdx`
- `../honcho/docs/v3/documentation/features/advanced/representation-scopes.mdx`
- `../honcho/docs/v3/openapi.json` schemas `SessionContext` and
  `PeerRepresentationGet`

Current Gormes files:

- `internal/goncho/types.go`
- `internal/goncho/service.go`
- `internal/gonchotools/honcho_tools.go`
- `internal/gonchotools/honcho_tools_test.go`

Red tests:

- `internal/goncho/context_options_test.go`
  - omitted options preserve the current same-chat result;
  - `limit_to_session=true` does not widen `scope=user`;
  - unsupported representation-only fields return structured unavailable
    evidence instead of being silently ignored.
- `internal/gonchotools/honcho_tools_test.go`
  - `HonchoContextTool.Schema()` exposes optional `summary`, `tokens`,
    `peer_target`, `peer_perspective`, `search_query`, `limit_to_session`,
    `search_top_k`, `search_max_distance`, `include_most_frequent`, and
    `max_conclusions`;
  - none of these fields are required.

Implementation boundaries:

- Add typed fields to `goncho.ContextParams`.
- Keep the existing `peer`, `query`, `max_tokens`, `session_key`, `scope`, and
  `sources` behavior.
- Return deterministic degraded evidence for fields that require future
  observation, summary, or semantic representation tables.

Do not implement:

- asynchronous summaries;
- directional storage migration;
- dialectic LLM tool loop;
- HTTP SDK adapter.

Validation:

- `go test ./internal/goncho ./internal/gonchotools -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(goncho): expose context representation options`

Acceptance:

- tool schema visibility matches Honcho docs;
- unsupported fields fail visible and narrow;
- current same-chat context fixtures keep passing.

## Packet 2 - Search Filter AST

Progress row: `3.F / Goncho search filter grammar`.

Purpose: add a typed Honcho-style filter parser before widening search surfaces.
Unsupported filters must fail closed because silent widening can leak memory
across sessions or peers.

Source docs:

- `../honcho/docs/v3/documentation/features/advanced/search.mdx`
- `../honcho/docs/v3/documentation/features/advanced/using-filters.mdx`
- `../honcho/docs/v3/openapi.json` schema `MessageSearchOptions`

Current Gormes files:

- `internal/goncho/types.go`
- `internal/goncho/service.go`
- `internal/memory/session_catalog.go`
- `internal/memory/session_catalog_test.go`

Red tests:

- `internal/goncho/filter_grammar_test.go`
  - parses `AND`, `OR`, `NOT`, `gt`, `gte`, `lt`, `lte`, `ne`, `in`,
    `contains`, `icontains`, nested `metadata`, and wildcard `"*"`;
  - rejects unknown fields and unknown operators with a typed error;
  - defaults limit to `10` and clamps to `100`.
- `internal/memory/session_catalog_test.go`
  - supported source/session filters preserve same-chat and user-scope fences.

Implementation boundaries:

- Build an internal AST first; keep SQL support to a documented subset.
- Use structured errors for unsupported operators.
- Do not expose a public HTTP endpoint in this packet.

Do not implement:

- full workspace/peer/session pagination;
- metadata indexing;
- OpenAPI SDK compatibility.

Validation:

- `go test ./internal/goncho ./internal/memory -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(goncho): add fail-closed search filters`

Acceptance:

- supported filters narrow results;
- unsupported filters never widen recall;
- default and maximum limits match Honcho docs.

## Packet 3 - Directional Peer Cards And Representation Scopes

Progress row: `3.F / Directional peer cards and representation scopes`.

Purpose: move cards and future representations from flat `(workspace, peer)` to
Honcho's `(workspace, observer, observed)` model.

Source docs:

- `../honcho/docs/v3/documentation/features/advanced/representation-scopes.mdx`
- `../honcho/docs/v3/documentation/features/advanced/peer-card.mdx`
- `../honcho/docs/v3/documentation/reference/sdk.mdx`
- `../honcho/src/crud/peer_card.py`
- `../honcho/src/models.py`

Current Gormes files:

- `internal/memory/schema.go`
- `internal/goncho/sql.go`
- `internal/goncho/service.go`
- `internal/goncho/types.go`
- `internal/gonchotools/honcho_tools.go`

Red tests:

- `internal/goncho/directional_peer_card_test.go`
  - self card and target card are stored independently;
  - card set replaces the whole card;
  - cards are capped at 40 facts;
  - degraded context reports the observer/observed pair used.
- `internal/memory/migrate_test.go`
  - migration preserves existing flat cards as `observer=gormes`,
    `observed=<peer>`.

Implementation boundaries:

- Add observer/observed columns or a new table shape through a migration.
- Keep `gormes` as the default observer for existing callers.
- Add a `target` field to peer-card tool params only after storage tests pass.

Do not implement:

- `observe_others` scheduling;
- late-join reasoning replay;
- dreamer card updates.

Validation:

- `go test ./internal/goncho ./internal/memory ./internal/gonchotools -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(goncho): key peer cards by observer and target`

Acceptance:

- observer/observed isolation is proven by tests;
- max-40 replacement semantics match Honcho;
- no existing flat-card caller breaks.

## Packet 4 - Summary Context Budget

Progress row: `3.F / Goncho summary context budget`.

Purpose: model Honcho's short/long session summaries as a separate prompt
component instead of folding them into last-N-turn recall.

Source docs:

- `../honcho/docs/v3/documentation/features/advanced/summarizer.mdx`
- `../honcho/docs/v3/documentation/features/get-context.mdx`
- `../honcho/docs/v3/openapi.json` schemas `Summary`, `SessionSummaries`, and
  `SessionContext`
- `../honcho/src/utils/summarizer.py`

Current Gormes files:

- `internal/memory/schema.go`
- `internal/goncho/service.go`
- `internal/goncho/types.go`
- `internal/gonchotools/honcho_tools.go`

Red tests:

- `internal/goncho/summary_context_test.go`
  - short summary triggers every 20 messages by default;
  - long summary triggers every 60 messages by default;
  - summaries store `message_id`, `summary_type`, `created_at`, and
    `token_count`;
  - `summary=true` reserves 40 percent of tokens for summary and 60 percent for
    recent messages;
  - `summary=false` spends the budget only on recent messages;
  - newest-message-over-budget returns empty or partial context with
    `summary_absent` evidence.

Implementation boundaries:

- Add a separate `goncho_session_summaries` storage surface.
- Context assembly must be separate from `RecallProvider.GetContext`.
- Summary generation can be a deterministic stub until the queue packet lands.

Do not implement:

- LLM summarizer prompts beyond fixture stubs;
- dream scheduling;
- HTTP endpoint parity.

Validation:

- `go test ./internal/goncho ./internal/memory ./internal/gonchotools -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(goncho): add session summary context slots`

Acceptance:

- token budgeting is deterministic;
- summary slots are independent of recent turns;
- missing or oversized summaries degrade visibly.

## Packet 5 - Queue Status Read Model

Progress row: `3.F / Goncho queue status read model`.

Purpose: expose Honcho-style queue status as operator evidence, not a turn
synchronization primitive.

Source docs:

- `../honcho/docs/v3/documentation/features/advanced/queue-status.mdx`
- `../honcho/docs/v3/documentation/core-concepts/reasoning.mdx`
- `../honcho/docs/v3/openapi.json` schemas `QueueStatus` and
  `SessionQueueStatus`

Current Gormes files:

- `internal/memory/status.go`
- `internal/memory/status_test.go`
- `internal/goncho/service.go`
- `cmd/gormes/memory.go`
- `cmd/gormes/memory_test.go`

Red tests:

- `internal/goncho/queue_status_test.go`
  - returns `completed_work_units`, `in_progress_work_units`,
    `pending_work_units`, `total_work_units`, and optional per-session counts;
  - excludes webhook, deletion, and vector reconciliation work;
  - zero-state read model is deterministic before a Goncho queue exists.
- `cmd/gormes/memory_test.go`
  - CLI output states queue status is observability only.

Implementation boundaries:

- Count only `representation`, `summary`, and `dream`.
- Keep existing extractor queue status unchanged.
- Add per-session status only if the data model can report it deterministically.

Do not implement:

- waiting for queue drain;
- worker orchestration;
- webhook or reconciler counters.

Validation:

- `go test ./cmd/gormes ./internal/goncho ./internal/memory -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(goncho): report honcho queue status`

Acceptance:

- status fields match Honcho docs;
- queue-empty is not used as correctness;
- operator output explains degraded zero-state.

## Packet 6 - Dialectic Chat Contract

Progress row: `3.F / Goncho dialectic chat contract`.

Purpose: align Gormes's reasoning tool with Honcho's `peer.chat()` contract and
the host integrations that expose `honcho_chat`.

Source docs:

- `../honcho/docs/v3/documentation/features/chat.mdx`
- `../honcho/docs/v3/documentation/features/advanced/representation-scopes.mdx`
- `../honcho/docs/v3/documentation/reference/sdk.mdx`
- `../honcho/docs/v3/guides/integrations/opencode.mdx`
- `../honcho/docs/v3/guides/integrations/claude-code.mdx`
- `../honcho/docs/v3/guides/integrations/mcp.mdx`
- `../honcho/docs/v3/openapi.json` schema `DialecticOptions`
- `docs/content/building-gormes/goncho_honcho_memory/01-prompts.md`
- `docs/content/building-gormes/goncho_honcho_memory/02-tool-schemas.md`

Current Gormes files:

- `internal/goncho/types.go`
- `internal/goncho/service.go`
- `internal/gonchotools/honcho_tools.go`
- `internal/gonchotools/honcho_tools_test.go`

Red tests:

- `internal/goncho/chat_contract_test.go`
  - request accepts `query`, `session_id`, `target`, `reasoning_level`, and
    `stream`;
  - default `reasoning_level` is `low`;
  - invalid reasoning levels are rejected;
  - `stream=true` returns structured unsupported evidence until streaming is
    implemented;
  - response shape is `{ "content": "..." }`.
- `internal/gonchotools/honcho_tools_test.go`
  - `honcho_chat` exists as a host-compatible alias while
    `honcho_reasoning` remains available.

Implementation boundaries:

- Add a `ChatParams` and `ChatResult` service method.
- Reuse deterministic synthesis until the real dialectic loop is ported.
- Keep dialectic off the kernel's fast recall path.

Do not implement:

- streaming transport;
- model-routed tool loop;
- source-message grep tools;
- HTTP server adapter.

Validation:

- `go test ./internal/goncho ./internal/gonchotools -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(goncho): add honcho chat contract`

Acceptance:

- host plugins can discover a `honcho_chat`-shaped tool;
- query-specific reasoning remains separate from `honcho_context`;
- unsupported streaming and target behavior is explicit.

## Packet 7 - File Upload Import Ingestion

Progress row: `3.F / Goncho file upload import ingestion`.

Purpose: support the Honcho/OpenClaw migration path where legacy memory files
are uploaded into sessions and become normal messages for reasoning.

Source docs:

- `../honcho/docs/v3/documentation/features/advanced/file-uploads.mdx`
- `../honcho/docs/v3/documentation/reference/sdk.mdx`
- `../honcho/docs/v3/guides/integrations/openclaw.mdx`
- `../honcho/docs/v3/openapi.json` route
  `/v3/workspaces/{workspace_id}/sessions/{session_id}/messages/upload`
- `../honcho/src/utils/files.py`
- `../honcho/src/routers/messages.py`
- `../honcho/src/config.py`

Current Gormes files:

- `internal/goncho/service.go`
- `internal/goncho/types.go`
- `internal/memory/schema.go`
- `internal/memory/worker.go`
- `cmd/gormes/memory.go`

Red tests:

- `internal/goncho/file_import_test.go`
  - text, markdown, and JSON imports create session messages;
  - unsupported content types fail before writing;
  - original file bytes are not persisted;
  - chunks include `file_id`, `filename`, `chunk_index`, `total_chunks`,
    `original_file_size`, `content_type`, and `chunk_character_range`;
  - `peer_id` is required;
  - `created_at`, metadata, and message configuration are preserved when
    provided.
- `internal/memory/migrate_test.go`
  - required metadata columns or JSON paths are available after migration.

Implementation boundaries:

- Start with text, markdown, and JSON. Leave PDF extraction unavailable until a
  local extractor choice is made.
- Use Honcho source behavior for runtime chunking: `settings.MAX_MESSAGE_SIZE`
  is 25,000 characters even though the prose file-upload page mentions a
  49,500-character chunk target.
- Import should enqueue normal Goncho reasoning work, or report queue-unavailable
  evidence until the queue exists.

Do not implement:

- remote file storage;
- PDF OCR;
- web upload UI;
- managed Honcho API client.

Validation:

- `go test ./internal/goncho ./internal/memory ./cmd/gormes -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(goncho): import uploaded files as messages`

Acceptance:

- imported files become ordinary session messages;
- file metadata is queryable;
- unsupported formats fail before memory writes;
- legacy memory migration has a documented non-destructive path.

## Packet 8 - Manual Conclusions API

Progress row: add only after Packet 3 lands or when HTTP adapter work starts.

Purpose: align manual conclusion create/list/query/delete behavior with Honcho
without pretending the full observation table exists.

Source docs:

- `../honcho/docs/v3/documentation/reference/sdk.mdx`
- `../honcho/docs/v3/openapi.json` schemas `ConclusionCreate`,
  `ConclusionBatchCreate`, `Conclusion`, and `ConclusionQuery`
- `../honcho/docs/v3/api-reference/endpoint/conclusions/*.mdx`

Current Gormes files:

- `internal/goncho/types.go`
- `internal/goncho/service.go`
- `internal/goncho/sql.go`
- `internal/gonchotools/honcho_tools.go`

Red tests:

- `internal/goncho/conclusions_contract_test.go`
  - create accepts up to 100 conclusions;
  - content is required and capped to Honcho's documented constraints;
  - `observer_id`, `observed_id`, and optional `session_id` are preserved;
  - query searches only the observer/observed pair requested;
  - delete is idempotent only if the product decision says so.

Implementation boundaries:

- Keep current `honcho_conclude` for simple tool writes.
- Add batch/list/query/delete service methods behind internal types first.
- Defer public HTTP routing until the API adapter packet.

Do not implement:

- generated SDK compatibility;
- inductive/deductive provenance trees;
- vector embeddings for conclusions.

Validation:

- `go test ./internal/goncho ./internal/gonchotools -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(goncho): add conclusion contract methods`

Acceptance:

- manual facts can be imported without model reasoning;
- observer/observed scoping is explicit;
- current `honcho_conclude` remains stable.

## Packet 9 - Host Integration Matrix

Progress row: keep linked to `3.E / Honcho host integration compatibility
fixtures` unless it becomes code.

Purpose: give implementers a single map for Hermes, OpenCode, Claude Code,
OpenClaw, and MCP host behavior.

Source docs:

- `../honcho/docs/v3/guides/integrations/hermes.mdx`
- `../honcho/docs/v3/guides/integrations/opencode.mdx`
- `../honcho/docs/v3/guides/integrations/claude-code.mdx`
- `../honcho/docs/v3/guides/integrations/openclaw.mdx`
- `../honcho/docs/v3/guides/integrations/mcp.mdx`

Matrix to preserve in tests and docs:

| Host | Workspace default | AI peer | Session strategies | Tool names |
|---|---|---|---|---|
| Hermes | `hermes` | `hermes` | `per-directory`, `per-repo`, `per-session`, `global` | `honcho_profile`, `honcho_search`, `honcho_context`, `honcho_conclude` |
| OpenCode | `opencode` | `opencode` | `per-directory`, `per-repo`, `git-branch`, `per-session`, `chat-instance`, `global` | `honcho_search`, `honcho_chat`, `honcho_create_conclusion`, config/status tools |
| Claude Code | `claude_code` | `claude` | `per-directory`, `git-branch`, `chat-instance` | `search`, `chat`, `create_conclusion`, config/status MCP tools |
| OpenClaw | `openclaw` | `agent-{id}` or `openclaw` | host/gateway session mapping | `honcho_context`, `honcho_search_conclusions`, `honcho_search_messages`, `honcho_session`, `honcho_ask` |
| MCP | header driven | header driven | client/project driven | workspace, peer, session, conclusion, queue, and dream tools |

Implementation boundaries:

- Gormes should not install Node, Bun, or external plugins.
- Gormes should expose compatible memory concepts and status evidence.
- Writes stay in the current host workspace unless explicit linked-host read
  support is implemented.

Validation:

- `go test ./internal/goncho ./internal/gonchotools ./internal/memory -count=1`
- `go run ./cmd/autoloop progress validate`

Acceptance:

- future host fixtures have a canonical table to cite;
- tool aliases are intentional rather than accidental;
- linked-host reads are documented as unsupported until implemented.

## Packet 10 - OpenAPI Adapter Audit

Progress row: create a new row only when the optional HTTP surface becomes the
active implementation target.

Purpose: keep optional HTTP work honest by using Honcho's OpenAPI file as the
contract source.

Source docs:

- `../honcho/docs/v3/openapi.json`
- `../honcho/docs/v3/api-reference/endpoint/**/*.mdx`
- `docs/content/building-gormes/goncho_honcho_memory/03-honcho-docs-study.md`

Endpoint groups to audit:

- workspaces: get/create, list, update, delete, search, queue status,
  schedule dream;
- peers: get/create, list, update, sessions, chat, representation, card,
  context, search;
- sessions: get/create, list, update, delete, clone, peers, peer config,
  context, summaries, search;
- messages: create batch, upload file, list, get, update;
- conclusions: create, list, query, delete;
- webhooks and keys: document as intentionally unsupported in local Goncho until
  a managed deployment exists.

Implementation boundaries:

- HTTP handlers must be thin adapters over `goncho.Service`.
- Do not introduce a second store, second process, or loopback dependency.
- Unsupported managed features return explicit local-Goncho unavailable errors.

Validation:

- contract tests generated or table-driven from `openapi.json`;
- `go test ./internal/goncho ./cmd/gormes -count=1`;
- `go run ./cmd/autoloop progress validate`.

Acceptance:

- optional HTTP parity is measurable route by route;
- local-only limitations are visible to operators;
- SDK compatibility does not mutate the in-binary service contract.

## Packet 11 - Topology Design Fixtures

Progress row: `3.F / Goncho topology design fixtures`.

Purpose: fixture-lock how Gormes maps workspaces, peers, sessions, sources, and
subagents before more host integrations depend on those assumptions.

Source docs:

- `../honcho/docs/v3/documentation/features/storing-data.mdx`
- `../honcho/docs/v3/documentation/core-concepts/design-patterns.mdx`
- `../honcho/docs/v3/guides/integrations/openclaw.mdx`
- `../honcho/docs/v3/guides/integrations/paperclip.mdx`
- `../honcho/docs/v3/guides/integrations/sillytavern.mdx`
- `docs/content/building-gormes/goncho_honcho_memory/05-operator-playbook.md`
- `internal/session/directory.go`
- `internal/memory/session_catalog.go`

Current Gormes files:

- `internal/session/directory.go`
- `internal/session/directory_test.go`
- `internal/memory/session_catalog.go`
- `internal/memory/session_catalog_test.go`
- `internal/goncho/types.go`

Red tests:

- `internal/goncho/topology_contract_test.go`
  - default workspace is `gormes`;
  - human peer comes from canonical `user_id`;
  - platform fallback peer uses `<source>:<chat_id>` only when `user_id` is
    unavailable;
  - assistant peer defaults to `gormes`;
  - subagent peer IDs preserve parent lineage metadata when supplied.
- `internal/session/directory_test.go`
  - conflicting `source/chat_id -> user_id` bindings fail visibly;
  - sessions are returned newest-first for a canonical user.

Implementation boundaries:

- Add pure mapping helpers before touching gateway runtime code.
- Keep existing `user_id > chat_id > session_id` semantics.
- Add degraded evidence for unknown peer/session topology instead of guessing.

Do not implement:

- new gateway adapters;
- subagent runtime changes;
- automatic workspace linking.

Validation:

- `go test ./internal/goncho ./internal/session ./internal/memory -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(goncho): fixture memory topology mapping`

Acceptance:

- topology decisions are deterministic and tested;
- workspace-per-user is rejected in docs/tests as a bad default;
- source filters remain compatible with current session catalog behavior.

## Packet 12 - Operator Diagnostics Contract

Progress row: `3.F / Goncho operator diagnostics contract`.

Purpose: make Goncho status and doctor output as useful as Honcho's CLI
diagnostic ladder without copying the Python runtime.

Source docs:

- `../honcho/docs/v3/documentation/reference/cli.mdx`
- `../honcho/docs/v3/contributing/self-hosting.mdx`
- `../honcho/docs/v3/contributing/configuration.mdx`
- `../honcho/docs/v3/contributing/troubleshooting.mdx`
- `docs/content/building-gormes/goncho_honcho_memory/05-operator-playbook.md`
- `cmd/gormes/doctor.go`
- `cmd/gormes/memory.go`
- `internal/memory/status.go`

Current Gormes files:

- `cmd/gormes/doctor.go`
- `cmd/gormes/doctor_test.go`
- `cmd/gormes/memory.go`
- `cmd/gormes/memory_test.go`
- `internal/goncho/service.go`
- `internal/memory/status.go`

Red tests:

- `cmd/gormes/memory_test.go`
  - `gormes memory status` includes Goncho queue zero-state when no Goncho
    queue exists;
  - output distinguishes extractor queue from representation, summary, and
    dream work.
- `cmd/gormes/doctor_test.go`
  - doctor reports config path, memory DB path, Goncho table presence, tool
    schema registration, and unavailable features;
  - JSON output is machine-parseable when the command adds `--json`.

Implementation boundaries:

- Prefer extending existing `memory status` and `doctor` before adding a new
  command namespace.
- Provider reachability checks only run for enabled features that call models.
- Exit codes follow the playbook: 0 usable, 1 bad input/missing files, 2 local
  DB/schema/tool failure, 3 provider/auth failure.

Do not implement:

- model-calling health checks for inactive deriver/dreamer/dialectic workers;
- a remote Honcho client;
- a second standalone CLI binary.

Validation:

- `go test ./cmd/gormes ./internal/goncho ./internal/memory -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(goncho): expose operator diagnostics`

Acceptance:

- operators can see why memory is not learning without reading logs first;
- degraded Goncho features are named in output;
- queue status remains observability, not synchronization.

## Packet 13 - Streaming Chat Persistence

Progress row: `3.F / Goncho streaming chat persistence contract`.

Purpose: prevent partial streamed dialectic responses from entering memory and
prepare for future `stream=true` support.

Source docs:

- `../honcho/docs/v3/documentation/features/advanced/streaming-response.mdx`
- `../honcho/docs/v3/documentation/features/chat.mdx`
- `docs/content/building-gormes/goncho_honcho_memory/05-operator-playbook.md`
- `docs/content/building-gormes/architecture_plan/phase-3-memory.md`

Current Gormes files:

- `internal/goncho/types.go`
- `internal/goncho/service.go`
- `internal/gonchotools/honcho_tools.go`
- `internal/memory/`

Red tests:

- `internal/goncho/streaming_contract_test.go`
  - `stream=true` is accepted by params;
  - unsupported streaming returns explicit degraded evidence;
  - interrupted streams do not write assistant messages;
  - completed streams write exactly one final assistant message when storage
    support exists.

Implementation boundaries:

- Start with contract and degraded mode only.
- Reuse interrupted-turn memory sync suppression rules.
- Store final accumulated response only after successful completion.

Do not implement:

- SSE transport;
- websocket gateway changes;
- partial chunk persistence.

Validation:

- `go test ./internal/goncho ./internal/memory ./internal/gonchotools -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(goncho): guard streaming chat persistence`

Acceptance:

- partial responses never become memory;
- `stream=true` behavior is explicit at the tool edge;
- future transport work has a storage contract to preserve.

## Packet 14 - Configuration Namespace

Progress row: `3.F / Goncho configuration namespace`.

Purpose: add a Gormes-native `[goncho]` config namespace before deriver,
dialectic, summary, dream, or import settings scatter into channel-specific
blocks.

Source docs:

- `../honcho/docs/v3/contributing/configuration.mdx`
- `../honcho/docs/v3/contributing/self-hosting.mdx`
- `docs/content/building-gormes/goncho_honcho_memory/05-operator-playbook.md`
- `internal/config/config.go`
- `internal/config/config_test.go`

Current Gormes files:

- `internal/config/config.go`
- `internal/config/config_test.go`
- `cmd/gormes/doctor.go`
- `cmd/gormes/telegram.go`
- `cmd/gormes/gateway.go`

Red tests:

- `internal/config/config_test.go`
  - defaults include `[goncho]` workspace `gormes`, observer peer `gormes`,
    max message size `25000`, max file size `5242880`, context max tokens
    `100000`, dialectic default `low`, and dream disabled until fixtures;
  - env vars like `GORMES_GONCHO_WORKSPACE` override TOML;
  - invalid reasoning levels fail config validation once validation exists.
- `cmd/gormes/doctor_test.go`
  - effective Goncho config is visible in doctor output.

Implementation boundaries:

- Use the existing Gormes loader; do not add Honcho's `.env` parser.
- Add config fields only when a planned packet needs them or the doctor must
  report them.
- Keep current `goncho.Config` constructor defaults backward-compatible.

Do not implement:

- provider model configs for inactive workers;
- JWT auth config;
- vector-store selection config.

Validation:

- `go test ./internal/config ./cmd/gormes ./internal/goncho -count=1`
- `go run ./cmd/autoloop progress validate`

Commit message:

- `fix(config): add goncho namespace`

Acceptance:

- Goncho settings have one documented namespace;
- env/TOML/default precedence matches the rest of Gormes;
- doctor output makes inactive features visible instead of ambiguous.

## Execution Order

1. Packet 1 - context options.
2. Packet 3 - directional peer-card storage.
3. Packet 11 - topology design fixtures.
4. Packet 14 - configuration namespace.
5. Packet 6 - dialectic chat contract.
6. Packet 13 - streaming chat persistence.
7. Packet 2 - filter AST.
8. Packet 4 - summary slots and budget.
9. Packet 5 - queue status read model.
10. Packet 12 - operator diagnostics contract.
11. Packet 7 - file upload import.
12. Packet 8 - manual conclusions API.
13. Packet 9 - host integration matrix fixtures.
14. Packet 10 - optional OpenAPI adapter audit.

The order is chosen to minimize rework: expose request shapes first, fix
storage identity next, lock topology/config, then add runtime behavior and
operator surfaces.

## Definition Of Done For Any Packet

A packet is done only when:

- the first commit contains a failing test or fixture for the named behavior;
- the implementation edits stay inside the packet's write scope;
- unsupported Honcho behavior fails closed with structured evidence;
- `progress.json` source refs include this page when the roadmap row changes;
- generated progress docs/site data are refreshed when `progress.json` changes;
- validation commands from the packet pass;
- the commit message matches the packet or explains why it differs.
