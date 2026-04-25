---
title: "Next Slices"
weight: 30
aliases:
  - /building-gormes/next-slices/
---

# Next Slices

This page is generated from the canonical progress file and lists the highest
leverage contract-bearing roadmap rows to execute next.

The ordering is:

1. unblocked `P0` handoffs;
2. active `in_progress` rows;
3. `fixture_ready` rows;
4. unblocked rows that unblock other slices;
5. remaining `draft` contract rows.

Use this page when choosing implementation work. If a row is too broad, split
the row in `progress.json` before assigning it.

<!-- PROGRESS:START kind=next-slices -->
| Phase | Slice | Contract | Trust class | Fixture | Why now |
|---|---|---|---|---|---|
| 4 / 4.A | DeepSeek/Kimi reasoning_content echo for tool-call replay | Thinking-mode providers that require reasoning_content on assistant tool-call turns receive an echoed value during persistence and API replay | system | `internal/hermes/reasoning_content_echo_test.go` | Unblocks Cross-provider reasoning-tag sanitization, OpenRouter, Codex stream repair + tool-call leak sanitizer. |
| 4 / 4.A | Bedrock Converse payload mapping (no AWS SDK) | Pure Bedrock Converse request mapping over the shared provider message/tool contract | system | `internal/hermes/bedrock_converse_mapping_test.go` | Unblocks Bedrock stream event decoding (SSE fixtures). |
| 4 / 4.A | Codex Responses pure conversion harness | OpenAI Responses request/response conversion for Codex-compatible providers without live OAuth | system | `internal/hermes/codex_responses_adapter_test.go` | Unblocks Codex stream repair + tool-call leak sanitizer, Codex OAuth state + stale-token relogin. |
| 4 / 4.A | Tool-call argument repair + schema sanitizer | Provider tool-call arguments are repaired or rejected against available tool schemas before execution | system, child-agent | `internal/hermes/tool_call_argument_repair_test.go` | Unblocks Codex stream repair + tool-call leak sanitizer, OpenRouter, Bedrock stream event decoding (SSE fixtures). |
| 4 / 4.D | Provider-enforced context-length resolver | Displayed and budgeted context windows prefer provider-enforced limits over raw models.dev metadata | operator, system | `internal/hermes/model_context_resolver_test.go` | Unblocks Compression token-budget trigger + summary sizing, Routing policy and fallback selector. |
| 5 / 5.F | Skill preprocessing + dynamic slash commands | Skill content preprocessing and skill-backed slash commands are deterministic, disabled-skill aware, and prompt-safe | operator, gateway, system | `internal/skills/preprocessing_commands_test.go` | Unblocks Toolset-aware skills prompt snapshot, TUI + Telegram browsing. |
| 5 / 5.O | PTY bridge protocol adapter | Dashboard/TUI PTY sessions expose bounded read, write, resize, close, and unavailable-state behavior through a testable adapter | operator | `internal/cli/pty_bridge_test.go` | Unblocks SSE streaming to Bubble Tea TUI, Dashboard PTY chat sidecar contract. |
| 5 / 5.Q | OpenAI-compatible chat-completions API server | OpenAI-compatible chat.completions HTTP surface over the native Gormes turn loop | operator, gateway | `internal/apiserver/chat_completions_test.go` | Unblocks Responses API store + run event stream, Gateway proxy mode forwarding contract, Dashboard API client contract. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
| 5 / 5.A | Tool registry inventory + schema parity harness | Operation and tool descriptor parity before handler ports | operator, gateway, child-agent, system | `internal/tools upstream schema parity manifest fixtures` | Unblocks Pure core tools first, Stateful tool migration queue, CLI command registry parity + active-turn busy policy. |
<!-- PROGRESS:END -->
