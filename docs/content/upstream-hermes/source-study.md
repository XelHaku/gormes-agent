---
title: "Source Study"
weight: 2
---

# Hermes Source Study

This page studies the local upstream `hermes-agent` source as an architecture
donor for Gormes. It is not a request to port Hermes line-by-line. The goal is
to keep the good contracts while avoiding the Python runtime coupling that made
Hermes hard to shrink.

## Study Snapshot

- Upstream studied: `/home/xel/git/sages-openclaw/workspace-mineru/hermes-agent`
- Upstream commit: `b2d3308f`
- Gormes repo studied: `/home/xel/git/sages-openclaw/workspace-mineru/gormes-agent`
- Gormes commit: `b56a81ef`
- Date: 2026-04-26

## 2026-04-26 Drift Check

The synchronized Hermes head is now `b2d3308f` after the local upstream moved
from `dc4d92f1` to `b2d3308f`. The new drift is narrow but execution-relevant:

- `ad0ac894` widens DeepSeek/Kimi/Moonshot replay padding from assistant
  tool-call turns to all assistant messages. Gormes already landed tool-call
  padding and cross-provider reasoning isolation, but its current
  `internal/hermes/http_client.go` fixtures still leave plain assistant turns
  unpadded. The roadmap now tracks this as a small Phase 4.A provider row.
- `25ba6a4a` makes gateway `/reasoning <level>` session-scoped by default,
  keeps `--global` as the persistence opt-in, and clears model plus reasoning
  session overrides on `/new`/reset flows. Gormes has per-turn model override
  plumbing but no typed reasoning-effort request field or gateway command
  state yet, so the plan splits this into a Phase 4.D request-propagation row
  followed by a Phase 5.O gateway command row.
- `b2d3308f` teaches Hermes doctor to accept bare `custom` provider
  configuration as valid operator intent instead of requiring a provider
  registry match. Gormes uses a different TOML/XDG config shape, so the plan
  captures the equivalent custom-endpoint readiness contract under Phase 5.O
  rather than porting Hermes config parsing.

Honcho and GBrain were unchanged in this sync. The internal memory direction
therefore remains Goncho as the Go implementation name, with Honcho-compatible
public tool names and data shapes preserved where external contracts require
`honcho_*`.

## 2026-04-25 Drift Check

The synchronized Hermes head is now `f93d4624`. New drift since the earlier
provider/compression notes is concentrated in the TUI, CLI startup, and
provider replay surface:

- `283c8fd6` and its child commits move model/provider overrides into TUI
  launch, resolve short model aliases statically, and avoid provider catalog
  network lookup during startup.
- `edc78e25`, `31d7f195`, `bd66e55a`, and `1735ced9` harden the Node/Ink TUI
  selection-copy path: SSH shortcut handling, rendered spaces, code-block
  indentation, and bounds clamping.
- `f93d4624` reorders `_copy_reasoning_content_for_api` so DeepSeek/Kimi
  replay of assistant tool-call turns injects an empty `reasoning_content`
  placeholder before promoting generic stored `reasoning`. That prevents
  reasoning emitted by one provider from leaking into another provider's
  continuation request.

Gormes should not import Hermes' Node/Ink runtime. The planner split the drift
into native Go rows under Phase 5.Q: one for TUI model override/static alias
startup, one for native selection/copy parity-or-divergence, and the existing
no-Node bundle independence row. The provider replay drift is tracked as a
small Phase 4.A row against `internal/hermes/http_client.go` and
`internal/hermes/reasoning_content_echo_test.go`.

## High-Level Shape

Hermes is a feature-rich Python agent runtime with many entry points feeding one
large orchestration class:

```text
CLI, TUI, gateway, cron, ACP, batch, API server
        |
        v
session, command, provider, and config resolution
        |
        v
AIAgent in run_agent.py
        |
        +--> prompt assembly and context compression
        +--> provider transport selection
        +--> tool schema selection and dispatch
        +--> memory providers and plugin hooks
        +--> session persistence and trajectory logs
        |
        v
provider APIs, tools, gateway delivery, SQLite state, files
```

The important lesson is not "copy `AIAgent`." The lesson is that Hermes had to
solve real integration contracts: provider-neutral tool continuations, stable
prompt prefixes, gateway active-turn control, dynamic tools, persistent memory,
and operator command parity. Gormes should keep those contracts in smaller Go
packages owned by the kernel, gateway, tools, memory, and provider layers.

## Source Map

Core upstream evidence files at study time:

- `run_agent.py` - `AIAgent`, provider routing, prompt use, tool loop,
  fallback, interrupts, memory hooks, and persistence; 12,662 lines.
- `cli.py` - legacy interactive terminal experience; 11,116 lines.
- `gateway/run.py` - `GatewayRunner`, session routing, slash commands,
  platform adapters, cached agents, progress delivery, active-turn control;
  11,147 lines.
- `hermes_cli/main.py` - top-level `hermes` command implementation and setup
  flows; 9,272 lines.
- `hermes_cli/commands.py` - central slash command registry feeding CLI,
  gateway help, Telegram menus, Discord commands, and Slack mappings.
- `hermes_cli/runtime_provider.py` and `hermes_cli/auth.py` - provider,
  credential, base URL, API mode, and custom endpoint resolution.
- `agent/prompt_builder.py` - system prompt layers, skills prompt cache,
  context-file discovery, tool-use guidance, and injection scanning.
- `agent/context_engine.py` and `agent/context_compressor.py` - pluggable
  context engine contract plus the default summarizing compressor.
- `agent/memory_provider.py` and `agent/memory_manager.py` - built-in memory
  plus one external provider with prompt blocks, prefetch, sync, tools, and
  lifecycle hooks.
- `model_tools.py` and `tools/registry.py` - self-registering tool modules,
  toolset filtering, availability checks, schema adjustments, plugin hooks,
  async bridging, and dispatch.
- `tools/delegate_tool.py` - subagent launch, progress relay, depth limits,
  child credentials, and timeout diagnostics.
- `plugins/`, `skills/`, and `optional-skills/` - extension and procedural
  knowledge surfaces.
- `tests/` - 741 `test*.py` files across agent, gateway, CLI, tools, cron,
  plugins, memory, ACP, provider, and session storage behavior.

Gormes already has smaller equivalents for several contracts:

- `internal/hermes/client.go` owns the provider-neutral stream, tool-call, and
  tool-result event contract.
- `internal/kernel/kernel.go` owns a single-goroutine turn loop and tool
  continuation loop.
- `internal/tools/tool.go` owns compiled Go tool descriptors and execution.
- `internal/gateway/commands.go` owns a small shared command registry.
- `internal/goncho/types.go` owns Honcho-compatible search/context shapes.

## Agent Loop Lessons

`AIAgent` does too much, but the lifecycle it encodes is valuable:

```text
sanitize user input
  -> restore primary runtime after fallback
  -> append user turn
  -> reuse or build stable system prompt
  -> preflight context compression
  -> inject ephemeral memory/plugin context into the user message
  -> make provider call
  -> parse text, reasoning, and tool calls
  -> execute tools sequentially or concurrently
  -> append tool results
  -> loop until final response, interrupt, fallback, or budget exhaustion
  -> persist session and memory side effects
```

The strongest design move is prompt stability. Hermes caches the system prompt
and injects volatile memory or plugin context into the current user message
instead of mutating the system prompt every turn. That preserves prompt-cache
prefixes and keeps plugin context out of stored system prompts.

The riskiest move is class gravity. `AIAgent` owns provider quirks, fallback,
memory, tools, compression, prompt assembly, session persistence, callbacks,
child agents, stream parsing, retries, and shutdown cleanup. Gormes should keep
the same lifecycle but distribute ownership across typed packages.

## Provider Runtime

Hermes supports several API modes:

- `chat_completions` for OpenAI-compatible endpoints.
- `codex_responses` for OpenAI Responses/Codex-shaped providers.
- `anthropic_messages` for native Anthropic-compatible message APIs.
- `bedrock_converse` for AWS Bedrock.

Runtime resolution is shared across CLI, gateway, cron, ACP, and auxiliary
tasks. It inspects explicit args, saved config, credential pools, custom
provider definitions, base URL hostnames, and provider-specific OAuth stores.

The good contract for Gormes is a provider-neutral stream/event shape plus
adapter-specific request builders and response decoders. The bad pattern to
avoid is letting provider-specific exceptions sprawl through the main kernel.

## Tool Runtime

Hermes tools are dynamic:

```text
tools/*.py self-register at import time
        |
        v
tools.registry.ToolRegistry
        |
        v
model_tools.get_tool_definitions()
        |
        +--> toolset filtering
        +--> availability checks
        +--> dynamic schema patches
        +--> schema sanitization
        |
        v
AIAgent tool execution and result append
```

Some tools are intercepted before registry dispatch because they need agent
state: `todo`, `memory`, `session_search`, `delegate_task`, `clarify`, and
external memory-provider tools. Hermes also runs independent tool batches
concurrently when safe and preserves tool-result order for the provider.

Gormes should keep explicit compiled tools as the default. It should borrow the
descriptor richness: availability, toolset membership, result budget,
prompt-visible safety, dynamic schema generation, and audit fields.

## Gateway Lessons

`GatewayRunner` is the most important upstream source for Gormes gateway work.
It normalizes inbound `MessageEvent` values, checks authorization and pairing,
dispatches slash commands, manages active sessions, queues or interrupts
running agents, wires progress callbacks, streams partial replies, and caches
`AIAgent` instances per session to preserve stable prompts.

Good mechanics to keep:

- a unified event shape per platform;
- explicit active-turn behavior for `/stop`, `/queue`, `/steer`, approvals,
  status, restart, background tasks, and ordinary follow-up text;
- progress callbacks that do not block the agent loop;
- session keys stable across platform/chat/thread boundaries;
- cached session runtime only when the effective configuration signature
  matches.

Recent gateway sync delta (2026-04-24): upstream commit `f731c2c2` tightened
BlueBubbles/iMessage behavior. `gateway/platforms/bluebubbles.py` now treats
BlueBubbles as non-editable, splits blank-line paragraphs into separate
iMessage bubbles, and removes pagination suffixes from long-message chunks.
`gateway/session.py` also adds a platform note that asks the model for short,
conversational, blank-line-separated replies. Gormes should keep this as
capability-based gateway behavior plus a BlueBubbles formatter fixture, not a
special-case branch in the kernel.

Recent upstream sync delta (2026-04-25): upstream commit `b35d692f` adds five
planner-relevant contracts:

- auxiliary LLM calls retry once without `temperature` when a provider rejects
  that parameter, while Codex Responses keeps the no-temperature transport
  rule independent of the now-deleted memory-flush path;
- cron jobs gained `context_from` so one scheduled job can inject bounded
  output from previous jobs without blocking on same-tick runs;
- Discord session sources now preserve `guild_id`, `parent_chat_id`, and
  `message_id` for tool/action context;
- Discord tools split into least-privilege `discord` and `discord_admin`
  toolsets, and `hermes tools` persistence now hardens MCP names, numeric
  keys, `no_mcp`, and platform-scoped toolsets;
- root Linux `install.sh` behavior now has an FHS-style layout decision that
  Gormes must either port or explicitly diverge from in tests and public docs.

Gormes tracks these as small rows: 4.H unsupported-temperature retry, 5.N
`context_from`, 2.B.11 Discord source metadata, 5.A/5.O toolset rows, and 5.P
installer policy.

Latest upstream sync delta (2026-04-25): commits `6e561ffa`, `97d54f0e`,
`af22421e`, `cf2fabc4`, `d635e2df`, `7c17accb`, `648b8991`, and
`ee0728c6` add
planner-relevant contracts tracked in progress.json:

- update/service restart verification now polls active status instead of doing
  a one-shot sleep check;
- terminal process watch-pattern notifications now need throttling, post-exit
  suppression, and notify-on-complete promotion;
- dashboard page-scoped plugin slots should be preserved as metadata/API
  contracts without importing the upstream React runtime;
- auxiliary compression feasibility must pass provider identity into context
  length resolution so Codex uses the provider cap before headroom math;
- stream retries must check cancellation before opening a fresh connection, so
  `/stop` cannot be negated by a retry loop;
- Codex Responses assistant replay messages must serialize text parts as
  `output_text`, while user messages keep `input_text`;
- Hermes TUI first-launch rebuild checks now treat a missing
  `packages/hermes-ink/dist/ink-bundle.js` as stale even when `dist/entry.js`
  exists, which Gormes tracks as an explicit native-TUI/no-Node divergence row
  rather than a Node/Ink build port.

Gormes tracks these as small rows in 5.O service restart polling, 5.A terminal
notification throttling, 5.I dashboard slot inventory, and 4.B provider-aware
auxiliary compression, with a new 5.Q native TUI bundle-independence fixture
for the `ee0728c6` drift. The 4.B provider-cap slice is complete on main; the
remaining rows stay fixture-first until their local substrates exist.

Previous context sync delta (2026-04-25): upstream commit `5401a008` changed
`agent/context_compressor.py` so `ContextCompressor.update_model()` recalculates
threshold, tail, and max-summary token budgets when the active model window
changes. Gormes has already mirrored the pure budget behavior in
`internal/hermes/context_compressor_budget_test.go`; remaining work is kernel
model-override wiring and real history pruning/summary feedback, not another
metadata-only row.

Risk to avoid: the running-agent command path is a long imperative chain inside
one very large file. Gormes should put active-turn policy in the command
registry itself, then let adapters and `gateway.Manager` consume that policy.

## Memory And Context

Hermes has two extension points that Gormes should preserve in Go:

- `ContextEngine`: one active compaction engine decides when and how to compact
  message history and may expose context-specific tools.
- `MemoryProvider`: built-in memory is always active, and at most one external
  provider runs beside it. Providers can add prompt text, prefetch recall,
  sync completed turns, expose tools, observe compression, and observe
  delegation.

The best pattern is lifecycle clarity. The dangerous pattern is prompt/tool
surface growth. Every active provider can add prompt blocks and model-visible
tools, so Gormes should gate provider tools, budget prefetch latency, and make
degraded memory state visible in `gormes memory status` and `gormes doctor`.

## Tests And Architecture Readiness

Hermes has meaningful tests around architectural edge cases: session storage,
tool-call serialization, reasoning preservation, CJK search fallback, gateway
command safety, fallback providers, plugin loading, context compression, tool
schema filtering, cron, ACP, and provider adapters.

The Gormes lesson is to keep writing architecture tests, not just unit tests:

- provider transcript fixtures for each adapter;
- tool schema parity fixtures against upstream;
- active-turn gateway policy fixtures;
- prompt assembly snapshots;
- memory scope negative tests;
- plugin/skill resolver conformance;
- context compression replay fixtures.

## Bottom Line

Hermes is the upstream capability ledger. Gormes should port:

- provider-neutral event contracts;
- gateway command/session semantics;
- prompt assembly rules;
- tool descriptor metadata;
- memory/context provider lifecycle;
- skill and plugin operator surfaces;
- tests for edge behavior.

Gormes should not port:

- the giant Python class layout;
- import-time tool registration as the core model;
- provider quirks inside the kernel;
- gateway command behavior as a long if/elif chain;
- silent fallback and degraded modes;
- arbitrary plugin code inside the trusted runtime by default.
