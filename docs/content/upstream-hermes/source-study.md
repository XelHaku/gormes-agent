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
- Upstream commit: `e5d41f05d47ed2e8b80a61625f2c48ae58b45b86`
- Gormes repo studied: `/home/xel/git/sages-openclaw/workspace-mineru/gormes-agent`
- Gormes commit: `c97c9c37aab4996e814ec7c80f59575a3ff0621b`
- Date: 2026-04-24

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
  fallback, interrupts, memory hooks, and persistence; 12,441 lines.
- `cli.py` - legacy interactive terminal experience; 11,078 lines.
- `gateway/run.py` - `GatewayRunner`, session routing, slash commands,
  platform adapters, cached agents, progress delivery, active-turn control;
  11,308 lines.
- `hermes_cli/main.py` - top-level `hermes` command implementation and setup
  flows; 9,120 lines.
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
- `tests/` - 731 `test*.py` files across agent, gateway, CLI, tools, cron,
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
