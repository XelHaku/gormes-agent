---
title: "Features Overview"
weight: 1
---

# Features Overview

Hermes Agent includes a rich set of capabilities that extend far beyond basic chat. From persistent memory and file-aware context to browser automation and voice conversations, these features work together to make Hermes a powerful autonomous assistant.

## Porting Goal

For Gormes, this page should be read as more than a feature list. It is the upstream capability ledger for the Go port.

- **Feature** tells you what user-visible capability exists upstream.
- **Method used** tells you the main upstream mechanism Hermes relies on to deliver it.
- The right Go port preserves the capability boundary and behavior contract, not necessarily the Python implementation shape.

## Core

- **[Tools & Toolsets](../tools)** — Tools are functions that extend the agent's capabilities. They're organized into logical toolsets that can be enabled or disabled per platform, covering web search, terminal execution, file editing, memory, delegation, and more.
- **[Skills System](../skills)** — On-demand knowledge documents the agent can load when needed. Skills follow a progressive disclosure pattern to minimize token usage and are compatible with the [agentskills.io](https://agentskills.io/specification) open standard.
- **[Persistent Memory](../memory)** — Bounded, curated memory that persists across sessions. Hermes remembers your preferences, projects, environment, and things it has learned via `MEMORY.md` and `USER.md`.
- **[Context Files](../context-files)** — Hermes automatically discovers and loads project context files (`.hermes.md`, `AGENTS.md`, `CLAUDE.md`, `SOUL.md`, `.cursorrules`) that shape how it behaves in your project.
- **[Context References](../context-references)** — Type `@` followed by a reference to inject files, folders, git diffs, and URLs directly into your messages. Hermes expands the reference inline and appends the content automatically.
- **[Checkpoints](../../checkpoints-and-rollback)** — Hermes automatically snapshots your working directory before making file changes, giving you a safety net to roll back with `/rollback` if something goes wrong.

## Automation

- **[Scheduled Tasks (Cron)](../cron)** — Schedule tasks to run automatically with natural language or cron expressions. Jobs can attach skills, deliver results to any platform, and support pause/resume/edit operations.
- **[Subagent Delegation](../delegation)** — The `delegate_task` tool spawns child agent instances with isolated context, restricted toolsets, and their own terminal sessions. Run up to 3 concurrent subagents for parallel workstreams.
- **[Code Execution](../code-execution)** — The `execute_code` tool lets the agent write Python scripts that call Hermes tools programmatically, collapsing multi-step workflows into a single LLM turn via sandboxed RPC execution.
- **[Event Hooks](../hooks)** — Run custom code at key lifecycle points. Gateway hooks handle logging, alerts, and webhooks; plugin hooks handle tool interception, metrics, and guardrails.
- **[Batch Processing](../batch-processing)** — Run the Hermes agent across hundreds or thousands of prompts in parallel, generating structured ShareGPT-format trajectory data for training data generation or evaluation.

## Media & Web

- **[Voice Mode](../voice-mode)** — Full voice interaction across CLI and messaging platforms. Talk to the agent using your microphone, hear spoken replies, and have live voice conversations in Discord voice channels.
- **[Browser Automation](../browser)** — Full browser automation with multiple backends: Browserbase cloud, Browser Use cloud, local Chrome via CDP, or local Chromium. Navigate websites, fill forms, and extract information.
- **[Vision & Image Paste](../vision)** — Multimodal vision support. Paste images from your clipboard into the CLI and ask the agent to analyze, describe, or work with them using any vision-capable model.
- **[Image Generation](../image-generation)** — Generate images from text prompts using FAL.ai. Eight models supported (FLUX 2 Klein/Pro, GPT-Image 1.5, Nano Banana Pro, Ideogram V3, Recraft V4 Pro, Qwen, Z-Image Turbo); pick one via `hermes tools`.
- **[Voice & TTS](../tts)** — Text-to-speech output and voice message transcription across all messaging platforms, with five provider options: Edge TTS (free), ElevenLabs, OpenAI TTS, MiniMax, and NeuTTS.

## Integrations

- **[MCP Integration](../mcp)** — Connect to any MCP server via stdio or HTTP transport. Access external tools from GitHub, databases, file systems, and internal APIs without writing native Hermes tools. Includes per-server tool filtering and sampling support.
- **[Provider Routing](../provider-routing)** — Fine-grained control over which AI providers handle your requests. Optimize for cost, speed, or quality with sorting, whitelists, blacklists, and priority ordering.
- **[Fallback Providers](../fallback-providers)** — Automatic failover to backup LLM providers when your primary model encounters errors, including independent fallback for auxiliary tasks like vision and compression.
- **[Credential Pools](../credential-pools)** — Distribute API calls across multiple keys for the same provider. Automatic rotation on rate limits or failures.
- **[Memory Providers](../memory-providers)** — Plug in external memory backends (Honcho, OpenViking, Mem0, Hindsight, Holographic, RetainDB, ByteRover) for cross-session user modeling and personalization beyond the built-in memory system.
- **[Honcho Memory](../honcho)** — AI-native memory provider with dialectic reasoning, peer cards, multi-agent profile isolation, and Honcho-owned tools.
- **[API Server](../api-server)** — Expose Hermes as an OpenAI-compatible HTTP endpoint. Connect any frontend that speaks the OpenAI format — Open WebUI, LobeChat, LibreChat, and more.
- **[IDE Integration (ACP)](../acp)** — Use Hermes inside ACP-compatible editors such as VS Code, Zed, and JetBrains. Chat, tool activity, file diffs, and terminal commands render inside your editor.
- **[RL Training](../rl-training)** — Generate trajectory data from agent sessions for reinforcement learning and model fine-tuning.
- **[Nous Tool Gateway](../tool-gateway)** — Route selected tool traffic through the Nous subscription gateway instead of direct third-party API keys.
- **[Web Dashboard](../web-dashboard)** — Local browser UI for configuration, keys, sessions, logs, analytics, cron jobs, and skills.
- **[Dashboard Plugins](../dashboard-plugins)** — Extend the web dashboard with custom tabs, JavaScript bundles, and optional backend endpoints.

## Customization

- **[Personality & SOUL.md](../personality)** — Fully customizable agent personality. `SOUL.md` is the primary identity file — the first thing in the system prompt — and you can swap in built-in or custom `/personality` presets per session.
- **[Skins & Themes](../skins)** — Customize the CLI's visual presentation: banner colors, spinner faces and verbs, response-box labels, branding text, and the tool activity prefix.
- **[Plugins](../plugins)** — Add custom tools, hooks, and integrations without modifying core code. Three plugin types: general plugins (tools/hooks), memory providers (cross-session knowledge), and context engines (alternative context management). Managed via the unified `hermes plugins` interactive UI.

## Full Feature Inventory for Go Porting

The table below is the exhaustive upstream feature surface documented under `user-guide/features/`. Use it as the checklist for Hermes-to-Gormes parity work.

| Feature | Method used upstream | Porting implication for Gormes |
|---|---|---|
| [Tools & Toolsets](../tools) | Self-registering tool modules via `registry.register(...)`, grouped in `toolsets.py`, dispatched by `model_tools.handle_function_call()` | Port the registry and toolset model before chasing individual tools |
| [Skills System](../skills) | On-demand markdown skills loaded through slash commands and skills directories using progressive disclosure | Preserve lazy loading and low-token activation, not just skill files |
| [Persistent Memory](../memory) | Built-in curated memory files plus memory-manager prompt injection | Keep memory as a first-class prompt layer with durable storage |
| [Context Files](../context-files) | Prompt builder auto-discovers files like `AGENTS.md`, `.hermes.md`, `CLAUDE.md`, and `SOUL.md` | Port discovery rules and merge order, not just filenames |
| [Context References](../context-references) | Input preprocessor expands `@` references for files, folders, diffs, and URLs inline into the turn | Preserve the reference grammar and expansion pipeline |
| [Checkpoints](../../checkpoints-and-rollback) | Working-directory snapshots taken before file-changing actions with rollback commands | Keep edit safety and rollback semantics around destructive changes |
| [Scheduled Tasks (Cron)](../cron) | Long-lived scheduler plus persisted job definitions and delivery routing | Port scheduler, job store, and delivery abstraction together |
| [Subagent Delegation](../delegation) | `delegate_task` spawns child agent instances with isolated context, toolsets, and terminals | Preserve isolation boundaries and cancellation scopes |
| [Code Execution](../code-execution) | Sandbox writes temporary Python and exposes Hermes tools through RPC inside the script | Rebuild the execution bridge, not necessarily Python-as-the-language |
| [Event Hooks](../hooks) | Filesystem-discovered lifecycle hooks for gateway and plugin events | Keep lifecycle interception points and stable event names |
| [Batch Processing](../batch-processing) | Batch runner fans prompts out into many agent runs and saves trajectory output | Port concurrency, deterministic output shape, and dataset generation flow |
| [Voice Mode](../voice-mode) | STT/TTS pipeline plus CLI voice loop and Discord voice-channel support | Split local voice UX from platform voice integrations, then port both |
| [Browser Automation](../browser) | Browser tools wrap cloud and local browser backends behind one tool surface | Preserve backend abstraction and common browser tool contract |
| [Vision & Image Paste](../vision) | Multimodal model calls and clipboard/image attachment ingestion | Port attachment ingest and model capability routing together |
| [Image Generation](../image-generation) | Image tool delegates to external generation providers, especially FAL | Keep provider abstraction; vendor choice can differ in Go |
| [Voice & TTS](../tts) | Provider-swappable TTS and transcription adapters across CLI and messaging | Treat speech I/O as a provider interface, not a hardcoded vendor |
| [MCP (Model Context Protocol)](../mcp) | Dynamic MCP client loads external tool surfaces over stdio or HTTP transport | Port the client runtime and server config model as a core extension seam |
| [Provider Routing](../provider-routing) | Runtime resolver sorts, whitelists, blacklists, and prioritizes providers/models | Keep routing policy separate from agent-loop execution |
| [Fallback Providers](../fallback-providers) | Agent loop retries against backup providers, including auxiliary tasks | Preserve failure policy and per-task fallback chains |
| [Credential Pools](../credential-pools) | Multiple keys per provider rotate on rate limits or failures | Port pool selection and failover, not just env var parsing |
| [Memory Providers](../memory-providers) | Single-select memory-provider plugin interface augments or replaces built-in memory | Keep provider boundaries explicit so local and external memory remain swappable |
| [Honcho Memory](../honcho) | Memory-provider plugin specializing in dialectic user modeling and peer cards | Port as a provider-compatible surface, not a special case bolted onto core memory |
| [API Server](../api-server) | OpenAI-compatible HTTP server wraps Hermes agent turns for web frontends | Preserve request and response compatibility if you want drop-in frontend support |
| [ACP Editor Integration](../acp) | ACP server over stdio/JSON-RPC with a curated editor toolset | Port as an editor-facing transport over the same core agent runtime |
| [RL Training](../rl-training) | Environments, batch runs, and trajectory capture produce training and eval data | Treat this as a data pipeline, not just a docs feature |
| [Nous Tool Gateway](../tool-gateway) | Tool runtimes optionally route through a subscription-backed proxy gateway | Preserve the per-tool routing switch if the business model matters |
| [Web Dashboard](../web-dashboard) | Local FastAPI plus frontend dashboard edits config, keys, sessions, cron, and analytics | Port only after the underlying state surfaces are stable in Go |
| [Dashboard Plugins](../dashboard-plugins) | Dashboard plugin SDK loads JS tabs and optional backend routes from plugin dirs | Keep dashboard extensibility if the web UI remains a product surface |
| [Personality & SOUL.md](../personality) | `SOUL.md` and preset personas are injected at the top of the system prompt | Preserve identity precedence in prompt assembly |
| [Skins & Themes](../skins) | CLI skin engine changes rendering data without changing agent behavior | Port as presentation-layer config, not part of cognition |
| [Plugins](../plugins) | Plugin manager discovers filesystem and entry-point plugins for tools, hooks, memory, and context engines | Keep extension boundaries small and explicit so third parties can port cleanly |

## Honcho as a Cross-Cutting Port Target

Honcho deserves explicit tracking because upstream Hermes exposes it through several separate surfaces:

- **Feature surface** — [Honcho Memory](../honcho)
- **Provider surface** — [Memory Providers](../memory-providers#honcho)
- **Tool surface** — `honcho_profile`, `honcho_search`, `honcho_context`, `honcho_reasoning`, `honcho_conclude`
- **CLI surface** — `hermes honcho ...`
- **Config surface** — `HONCHO_API_KEY`, `HONCHO_BASE_URL`, and `honcho.json`

If Gormes wants real Honcho parity, it has to account for all five, not just "memory provider exists."

## Practical Read Order for Porting

If the goal is native Go parity, the docs are easiest to consume in this order:

1. [Tools & Toolsets](../tools)
2. [Skills System](../skills)
3. [Persistent Memory](../memory)
4. [Memory Providers](../memory-providers)
5. [MCP (Model Context Protocol)](../mcp)
6. [Scheduled Tasks (Cron)](../cron)
7. [Subagent Delegation](../delegation)
8. [API Server](../api-server)
9. [ACP Editor Integration](../acp)
10. [Messaging Gateway](../../messaging/)

That order tracks the core abstraction seams Hermes relies on, not just the most visible user features.
