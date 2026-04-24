---
title: "Good and Bad"
weight: 3
---

# Good And Bad

## What Hermes Gets Right

### 1. Real Capability Ledger

Hermes is not a toy agent loop. It has CLI, gateway, cron, ACP, API server,
batch generation, provider routing, dynamic tools, memory providers, context
compression, skills, plugins, and many platform adapters. For Gormes, it is the
right upstream inventory of user-visible capability.

### 2. Stable Prompt Prefix Discipline

Hermes keeps the system prompt stable across turns and injects volatile memory
or plugin context into the current user message. This is a serious production
optimization: prompt caching, replay, and session storage all benefit when the
cached prefix is not rebuilt every turn.

Gormes should keep this rule: system prompt layers are stable; turn-local
context is fenced and ephemeral.

### 3. Shared Command Registry

`hermes_cli/commands.py` is a strong architectural contract. CLI help,
gateway dispatch, Telegram menus, Discord slash commands, Slack subcommands,
aliases, categories, and active-session bypass decisions all derive from one
registry.

Gormes already started this with `internal/gateway/commands.go`; it should grow
that registry instead of letting each adapter invent command behavior.

### 4. Provider-Aware Runtime Resolution

Hermes has a practical provider resolver. It handles saved config, environment
variables, credential pools, custom endpoints, OAuth-like providers, native
Anthropic, Codex/Responses, Bedrock, and auxiliary model routes.

The Gormes takeaway is not the exact Python resolver. It is the need for one
runtime contract consumed by CLI, gateway, cron, background jobs, and provider
adapters.

### 5. Tool Registry With Availability And Schema Repair

Hermes does more than list tools. It filters by toolset, runs availability
checks, blocks schema drift, rewrites dynamic schemas when related tools are
disabled, sanitizes schemas for strict backends, and routes plugin hooks before
and after execution.

Gormes should keep compiled Go tools, but the descriptor should know more than
name, description, and schema.

### 6. Gateway Active-Turn Semantics

Hermes has hard-won gateway behavior: `/stop` interrupts, `/queue` waits,
`/steer` injects after a tool result, approvals bypass interrupt, safe status
commands can run mid-turn, and unsafe commands return visible busy responses.

This is exactly the kind of behavior a messaging-native agent needs. It should
be moved into a typed command policy in Gormes.

### 7. Memory And Context Extension Contracts

`MemoryProvider` and `ContextEngine` are good seams. They define lifecycle
hooks, prompt blocks, prefetch, sync, compression, provider tools, and shutdown
without forcing every memory backend into the core loop.

Gormes should keep one external memory provider active at a time until schema
and prompt budgets prove that multiple providers are safe.

### 8. Tests Around Edge Cases

The test tree is large and focused on the kind of failures real agents hit:
session replay, tool-call ordering, reasoning fields, gateway command routing,
fallback providers, stale locks, context compression, plugin discovery, and
provider-specific transport quirks.

This validates architecture boundaries, not just functions.

## What Hermes Gets Wrong Or Risky

### 1. Central Files Are Too Large

`run_agent.py`, `gateway/run.py`, and `cli.py` are each over 11,000 lines.
That makes behavior discoverable by search but hard to reason about, review,
or port safely.

Gormes should preserve one logical kernel, one logical gateway, and one command
registry, but split implementations by domain.

### 2. Python Sync/Async/Thread Complexity Leaks Everywhere

Hermes bridges sync agent code, async tools, gateway event loops, thread-pool
tool execution, long-lived asyncio loops, provider SDKs, and interrupt flags.
This creates many subtle lifecycle paths: abandoned API threads, thread-local
tool interrupts, worker event loops, cached clients, and cleanup races.

Gormes's single-owner kernel goroutine and typed context cancellation are a
better base. Do not reintroduce Hermes's thread model unless a backend truly
requires it.

### 3. Provider Quirks Leak Into Shared Code

Hermes handles many providers, but provider-specific mode detection, message
repair, reasoning fields, cache markers, URL conventions, auth refresh, and
tool-call quirks appear across resolver, agent, adapters, and gateway paths.

Gormes should force quirks behind provider adapter fixtures and a small shared
event contract.

### 4. Import-Time Tool Discovery Is Hard To Audit

Hermes tools self-register when modules import. This is flexible, but it makes
static inventory, trusted execution, and plugin isolation harder. Plugins and
MCP can also mutate the registry at runtime.

Gormes should keep explicit core registration, then add plugin capability
registration behind manifests, trust classes, and reviewable activation.

### 5. Plugin Power Is Broader Than The Trust Model

Hermes plugins can register tools, slash commands, CLI commands, hooks, context
engines, image providers, and skills. That is powerful, but it means extension
code can sit close to the trusted runtime.

Gormes should treat plugins as a capability boundary: manifest first,
permissioned registration, optional subprocess or WASM isolation for untrusted
extensions, and operator-visible status.

### 6. Gateway Logic Is Correct But Entangled

The gateway has many good behaviors, but the control flow is hard to isolate:
authorization, pairing, active session policy, command handling, plugin hooks,
skill slash commands, media preprocessing, streaming, progress, session reset,
agent caching, and platform-specific rules live in one huge runner.

Gormes should make platform adapters boring and put shared behavior in small
gateway packages: admission, commands, session store, delivery, streaming,
active-turn policy, hooks, and media preprocessors.

### 7. Memory Surface Can Bloat The Prompt

Hermes memory can inject built-in memory, user profile, external provider
blocks, prefetched context, memory tools, and context-engine tools. That is
useful, but prompt bloat and stale recall are real risks.

Gormes should keep recall latency budgets, source fences, scope controls,
token budgets, and health reporting as first-class contracts.

### 8. Feature Surface Depends On Optional Dependencies

Hermes supports many extras: messaging, cron, Matrix, voice, TTS, Honcho, MCP,
ACP, Bedrock, web, RL, Daytona, Modal, and more. This is useful upstream, but
it creates install and packaging friction.

Gormes should keep the single-binary promise. Optional integrations should
compile cleanly or activate through isolated helpers without making the base
runtime fragile.

### 9. Docs Can Drift From Fast-Moving Code

The upstream docs are broad and helpful, but the source already shows details
that outpace older summaries, such as larger file sizes and added API modes.

Gormes should generate capability inventories from code where possible and keep
human narrative docs focused on decisions, not exhaustive mutable lists.

## Bottom Line

Hermes's best ideas are production contracts:

- stable prompt prefix;
- shared command registry;
- provider runtime resolution;
- tool descriptor filtering;
- gateway active-turn policy;
- memory/context provider lifecycle;
- broad edge-case tests.

Hermes's risks are implementation shape:

- large central Python files;
- sync/async/thread complexity;
- provider quirks in shared code;
- dynamic import-time registration;
- broad plugin trust;
- gateway entanglement;
- prompt and dependency sprawl.

Gormes should port the contracts and reject the shape.
