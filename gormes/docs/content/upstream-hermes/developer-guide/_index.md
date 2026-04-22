---
title: "Developer Guide"
description: "Contribute to Hermes Agent — architecture, tools, skills, and more."
weight: 3
---

# Developer Guide

This section documents how Hermes is actually built upstream. For Gormes, that makes it the subsystem inventory for the Go port.

## Porting Goal

Read this section as the implementation map for Hermes:

- **Surface** tells you which upstream subsystem or extension seam exists.
- **Method used** tells you the dominant upstream runtime mechanism or implementation pattern.
- **Porting implication** tells you what the Go port must preserve, even if the Python code itself is not reused.

The key Gortex-grounded anchors for this section are:

- `run_agent.py::AIAgent`
- `gateway/run.py::GatewayRunner`
- `hermes_state.py::SessionDB`
- `model_tools.py::handle_function_call`
- `hermes_cli/runtime_provider.py::resolve_runtime_provider`
- `tools/registry.py::discover_builtin_tools`

## Full Developer-Guide Inventory for Go Porting

| Surface | Method used upstream | Porting implication for Gormes |
|---|---|---|
| [Architecture](./architecture/) | Top-level subsystem map centered on `AIAgent`, provider resolution, tool dispatch, SQLite persistence, and the messaging gateway | Preserve the big subsystem boundaries before re-implementing internals |
| [Agent Loop Internals](./agent-loop/) | Synchronous `AIAgent.run_conversation()` loop with provider-mode switching, tool execution, retries, fallback, compression, and persistence | The Go port needs one coherent orchestration loop, not scattered feature handlers |
| [Prompt Assembly](./prompt-assembly/) | Layered prompt builder combining personality, memory, skills, context files, tool guidance, and provider-specific instructions | Preserve prompt precedence and assembly order as a contract |
| [Provider Runtime Resolution](./provider-runtime/) | `resolve_runtime_provider()` maps provider and model choices to API mode, credentials, and base URL behavior | Keep routing and credential resolution separate from inference execution |
| [Tools Runtime](./tools-runtime/) | Self-registering tool modules discovered by `discover_builtin_tools()` and dispatched by `handle_function_call()` | Rebuild the registry and dispatch seam before porting individual tools |
| [Session Storage](./session-storage/) | `SessionDB` plus gateway session storage, backed by SQLite and FTS5 with lineage tracking | Keep durable session identity and searchable history as first-class infrastructure |
| [Gateway Internals](./gateway-internals/) | Long-lived `GatewayRunner` with platform adapters, authorization, session routing, hooks, cron ticking, and delivery | The Go port needs a real runtime supervisor for adapters, not just chat endpoints |
| [Context Compression & Prompt Caching](./context-compression-and-caching/) | Lossy conversation summarization plus Anthropic prefix caching | Separate "reduce context" logic from "cache prompt prefixes" logic |
| [ACP Internals](./acp-internals/) | ACP server over stdio and JSON-RPC with editor-facing callbacks and curated toolsets | Port as a transport layer over the same core agent runtime |
| [Cron Internals](./cron-internals/) | Scheduler loop plus persisted jobs and message delivery targets | Preserve both schedule semantics and delivery integration |
| [Environments](./environments/) | RL and training environment surface around Hermes session execution | Treat this as an experimentation and data-generation subsystem, not a user feature |
| [Trajectory Format](./trajectory-format/) | Structured capture of prompts, tool calls, and outcomes for training data | Keep output shape stable if trajectory generation remains part of the platform |
| [Adding Tools](./adding-tools/) | Module-level `registry.register(...)` pattern with schema plus handler plus availability checks | Keep tool extension ergonomics simple and explicit in Go |
| [Adding Providers](./adding-providers/) | Provider registry plus runtime resolver and API-mode integration | Preserve provider onboarding as a bounded extension surface |
| [Adding Platform Adapters](./adding-platform-adapters/) | Adapter interface normalizes inbound events and outbound delivery across gateways | Port one clean adapter contract, then implement platforms against it |
| [Extending the CLI](./extending-the-cli/) | Central command registry and shared command resolution across CLI and gateway | Keep one command-definition source of truth |
| [Creating Skills](./creating-skills/) | Markdown skill documents with discovery rules and slash-command access | Preserve skill packaging and lazy activation, not just docs files |
| [Memory Provider Plugin](./memory-provider-plugin/) | Single-select provider plugin API for external memory systems; Honcho is the canonical high-complexity example with provider-owned tools and peer-scoped state | Keep memory provider boundaries narrow and swappable, but treat Honcho as a first-class parity target |
| [Context Engine Plugin](./context-engine-plugin/) | Single-select context engine abstraction behind the prompt pipeline | Preserve one replaceable context-engine seam instead of hardcoding compression |
| [Contributing](./contributing/) | Repository workflow, tests, docs flow, and contribution boundaries | Useful for process parity, but lower priority than runtime subsystem parity |

## Honcho Implementation Notes

Honcho is not just "one more memory plugin." In upstream Hermes it cuts across several implementation seams:

- memory provider plugin lifecycle
- prompt assembly and context injection
- provider-owned tool routing
- profile-aware peer identity
- dedicated CLI and config handling

That means a Go port cannot evaluate Honcho only inside the memory-provider abstraction. It also has to preserve the places where Honcho participates in prompt construction, gateway session cleanup, and operator commands.

## Porting Priority Read Order

If the goal is Hermes-to-Gormes subsystem parity, the developer guide is easiest to consume in this order:

1. [Architecture](./architecture/)
2. [Agent Loop Internals](./agent-loop/)
3. [Prompt Assembly](./prompt-assembly/)
4. [Provider Runtime Resolution](./provider-runtime/)
5. [Tools Runtime](./tools-runtime/)
6. [Session Storage](./session-storage/)
7. [Gateway Internals](./gateway-internals/)
8. [Context Compression & Prompt Caching](./context-compression-and-caching/)
9. [Cron Internals](./cron-internals/)
10. [ACP Internals](./acp-internals/)

That order follows the actual runtime spine Hermes depends on.
