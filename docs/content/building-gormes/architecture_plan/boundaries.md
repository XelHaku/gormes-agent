---
title: "Project Boundaries"
weight: 110
---

## 5. Project Boundaries

Hard rule: no Python file in this repository is modified. Gormes is now the repository root: Go runtime code lives under `cmd/`, `internal/`, and `pkg/`; operator and contributor docs live under `docs/`; site code lives under `www.gormes.ai/`.

The bridge is allowed to exist. The bridge is not allowed to become the destination.

## Upstream Contract Boundary

Gormes studies Hermes and GBrain as donors, but only ports contracts that make
the Go runtime better:

- provider-neutral stream and tool-call events;
- stable prompt assembly rules;
- gateway command/session semantics;
- operation and tool descriptors;
- memory/context provider lifecycle;
- durable job and subagent ledgers;
- graph provenance and retrieval evaluation.

It does not port upstream file shape. `run_agent.py`, `gateway/run.py`, and
GBrain's large operation and queue files are evidence, not templates.

## Trust-Class Boundary

Every operation should be classified before handler code runs:

| Trust class | Caller | Default posture |
|---|---|---|
| `operator` | local CLI/TUI/admin process | broadest access, still audited |
| `gateway` | Telegram/Discord/Slack/API user input | no local-operator tools without explicit allowlist |
| `child-agent` | delegated subagent | bounded tools, depth, timeout, and workspace scope |
| `system` | cron, boot hooks, maintenance jobs | deterministic payloads, audit required |

The executor should reject disallowed trust classes centrally. Handler-local
checks are still useful, but they are defense in depth rather than the primary
boundary.

## Provider Boundary

Provider quirks stay out of the kernel. Anthropic Messages, OpenAI Responses,
Bedrock Converse, OpenRouter, Gemini, Codex, and custom OpenAI-compatible
servers should all collapse into the shared `internal/hermes` event contract:

- text and reasoning deltas;
- final finish reason;
- assistant tool calls;
- tool-result continuation payloads;
- token usage;
- classified retry/auth/rate/context errors.

Adapters own request shaping and protocol oddities. The kernel owns turn state,
cancellation, retry orchestration, tool execution, and finalization.
