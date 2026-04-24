---
title: "Gormes Takeaways"
weight: 4
---

# Gormes Takeaways

## Do Not Rebuild The Hermes Monolith

Hermes proves the feature set. It does not prove the right implementation
shape for Gormes. The better Gormes architecture is smaller and stricter:

```text
Gateway/TUI/CLI/cron input
        |
        v
admission, trust class, command policy
        |
        v
single-owner kernel turn loop
        |
        +--> stable prompt assembler
        +--> provider-neutral stream contract
        +--> typed tool executor
        +--> memory/context providers
        +--> durable audit and session state
        |
        v
SQLite, JSONL audit, gateway delivery, optional external integrations
```

The kernel should never become a Go version of `run_agent.py`. Its job is to
own turn state, stream events, tool continuation, cancellation, and finalization.
Prompt assembly, provider adapters, tools, memory, gateway commands, and
plugins should remain separately testable packages.

## Recommended Architecture Decisions

### 1. Keep The Provider Contract Small

`internal/hermes/client.go` is the right direction. Preserve a provider-neutral
contract around:

- request messages;
- stream events;
- reasoning deltas;
- final finish reason;
- assistant tool calls;
- tool-result continuation messages;
- token usage;
- retry/error classification.

Provider adapters should own API-mode details. The kernel should not know
whether the backend is Anthropic Messages, OpenAI Responses, Bedrock Converse,
OpenRouter, Gemini, or a custom OpenAI-compatible server.

### 2. Build A Real Prompt Assembly Package

Port Hermes prompt behavior as a contract:

- stable identity/system layers;
- memory and user profile blocks;
- skills index, when enabled;
- context files with injection scanning;
- platform formatting hints;
- model/provider execution guidance;
- timestamp/model/provider metadata;
- ephemeral memory/plugin recall injected into the current user turn, not the
  system prompt.

This should be snapshot-tested. The output matters as much as code.

### 3. Put Active-Turn Policy In The Command Registry

Hermes's active-running-agent command chain should become data in Gormes:

```go
type ActiveTurnPolicy string

const (
    ActiveReject   ActiveTurnPolicy = "reject"
    ActiveBypass   ActiveTurnPolicy = "bypass"
    ActiveQueue    ActiveTurnPolicy = "queue"
    ActiveInterrupt ActiveTurnPolicy = "interrupt"
    ActiveSteer    ActiveTurnPolicy = "steer"
)
```

Every command should declare:

- canonical name and aliases;
- gateway/TUI/CLI visibility;
- argument hint;
- active-turn policy;
- trust class allowed;
- handler owner;
- platform exposure rules.

Then Telegram, Discord, Slack, API server, and future adapters can expose the
same command surface without copying policy.

### 4. Extend Tool Descriptors Before Porting More Tools

`internal/tools.Tool` is intentionally small. Add a descriptor layer around it
before porting the large upstream tool surface:

- toolset;
- availability check;
- mutating/read-only flag;
- trust classes allowed;
- timeout;
- result size budget;
- audit kind;
- prompt-visible or operator-only;
- dynamic schema builder, when needed.

The executor should enforce trust and timeout before a handler runs. Handlers
should not each remember the same safety policy.

### 5. Keep Gateway Adapters Thin

Hermes platform adapters carry a lot of special behavior. Gormes should route
that through shared contracts:

- inbound normalization to a single event shape;
- session key derivation;
- media/document preprocessing;
- command parsing;
- authorization and pairing;
- progress delivery;
- streaming edit support;
- delivery target resolution;
- active-turn control.

Adapters should translate platform SDK events into these contracts and nothing
more unless the platform truly needs a special case.

### 6. Make Memory Provider Scope Explicit

Hermes's `MemoryProvider` interface is a useful donor, but Gormes should bind it
to existing GONCHO and SQLite scope rules:

- one built-in local memory provider is always active;
- one external provider may be active beside it;
- provider tools are opt-in and schema-budgeted;
- prefetch has a strict latency budget;
- same-chat, same-user, and source allowlist scopes are explicit;
- degraded mode is visible in `gormes memory status` and `gormes doctor`.

The `honcho_*` tool names can remain externally compatible while the internal
implementation stays `internal/goncho`.

### 7. Add Context Compression As A Separate Engine

Do not bury compression inside the kernel. Use a contract similar to Hermes's
`ContextEngine`:

- track model context window;
- decide preflight compression;
- preserve head and tail messages;
- keep tool call/result pairs together;
- write a compression lineage record;
- preserve memory-provider pre-compress observations;
- expose manual compression status.

Compression must be replayable from fixtures because it changes future model
behavior.

### 8. Treat Plugins As Capabilities, Not Imports

Hermes plugin discovery is flexible because it imports Python code directly.
Gormes should use a stricter model:

- manifest first;
- capabilities declared before loading;
- operator enablement required;
- trust class and permission fields;
- isolated execution for untrusted plugins;
- clear status in `gormes plugins` and `gormes doctor`;
- no prompt-visible tools until activation is explicit.

This keeps extension power without weakening the single-binary trusted core.

### 9. Use Hermes Tests As Fixture Inventory

Before porting a subsystem, extract the upstream behavior into fixtures:

- provider transcript fixtures for tool calls and reasoning;
- gateway active-turn command fixtures;
- command registry exposure fixtures;
- tool schema parity fixtures;
- session replay and reasoning preservation fixtures;
- memory provider lifecycle fixtures;
- context compression fixtures;
- plugin manifest and hook fixtures.

Gormes should prove compatibility with behavior, not file structure.

## Things To Avoid

- Do not make `internal/kernel` absorb provider-specific API modes.
- Do not grow one large Go file that mirrors `run_agent.py` or `gateway/run.py`.
- Do not let platform adapters each implement their own command rules.
- Do not expose operator-local tools to gateway or child-agent trust classes.
- Do not rely on markdown skills as enforcement for dangerous actions.
- Do not import or execute arbitrary plugins inside the trusted core by default.
- Do not hide degraded memory, provider, tool, or plugin states.
- Do not make optional cloud/backends mandatory for the base binary.

## Latest Sync Lessons

The 2026-04-24 upstream sync adds several contract slices, not new monoliths:

- Interrupted or cancelled turns must not flush partial observations into GONCHO or external Honcho-compatible memory.
- Bedrock needs a stale-client eviction/retry-classification slice after request mapping, stream decoding, and SigV4 seams exist.
- Codex should start with pure Responses conversion fixtures before OAuth state, stale-token relogin, and stream/tool-call repair.
- The API server now carries disconnect/cancel snapshot persistence and proxy-mode behavior; the React dashboard is endpoint/stream contract inventory, not a Node runtime target.
- Skill preprocessing, dynamic slash commands, pluginized Spotify, and PTY bridge behavior should all land as narrow Phase 5 fixtures.
- Honcho integration docs for OpenCode and SillyTavern reinforce external `honcho_*` compatibility while Gormes keeps the internal package named `goncho`.

## Phase Alignment

Near-term Gormes work can use this study directly:

- Phase 2 gateway: expand `internal/gateway/commands.go` with active-turn
  policy and adapter exposure rules from Hermes.
- Phase 3 memory: keep Honcho-compatible tools, but enforce GONCHO scope,
  source allowlists, and visible degraded state.
- Phase 4 providers: add adapter fixtures for Anthropic, OpenAI Responses,
  Bedrock, Gemini, OpenRouter, and custom endpoints before broad routing.
- Phase 5 tools/plugins/CLI: build tool descriptors, plugin manifests, and CLI
  command groups from contracts instead of porting Python control flow.
- Phase 6 learning loop: only generate or improve skills after resolver,
  promotion, feedback, and audit records are already reliable.

## Decision

The better Gormes target is:

```text
Hermes capability parity
+ Go single-owner kernel
+ provider-neutral event fixtures
+ registry-owned command policy
+ descriptor-owned tool safety
+ GONCHO-scoped memory providers
+ isolated plugin capabilities
+ visible degraded-mode doctor checks
```

That preserves the useful Hermes product while making the architecture easier
to test, ship, and eventually run without Python.
