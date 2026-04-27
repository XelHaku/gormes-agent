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
| 2 / 2.B.5 | WhatsApp unsafe identifier inbound evidence | WhatsApp inbound normalization uses the validated safety predicate to drop unsafe raw sender/chat/reply IDs with explicit whatsapp_identifier_unsafe evidence before Event.ChatID, Event.UserID, Reply.ChatID, alias graph entries, or outbound pairing targets are created | gateway, operator, system | `internal/channels/whatsapp/identity_inbound_safety_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 2 / 2.B.5 | Telegram require-mention config fields | internal/config parses Telegram require_mention and bot_username fields with disabled defaults so group mention gating remains opt-in and can be tested without constructing a Telegram bot runtime | gateway, operator, system | `internal/config/config_test.go::TestLoad_TelegramRequireMentionFields` | Unblocks Telegram group require-mention bot binding. |
| 2 / 2.E.3 | Durable worker RSS drain integration | DurableWorker integrates the validated RSS watchdog policy after job completion and on an injected periodic check, starts graceful drain, cancels in-flight handlers through existing abort-slot recovery, and records watchdog drain evidence | operator, system | `internal/subagent/durable_worker_rss_drain_test.go` | Unblocks Durable worker RSS doctor/status evidence. |
| 2 / 2.F.5 | Steer slash command registry + queue fallback | Registry-owned active-turn steering command | operator, gateway | `internal/gateway/steer_queue_test.go::TestSteerCommandRegistry_*` | Unblocks Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard. |
| 4 / 4.B | ContextEngine compression-boundary notification | Kernel compression execution signals the ContextEngine when a compression boundary is crossed so cached/replayed context state cannot silently span the pre-compress and post-compress transcript | operator, system | `internal/kernel/compression_boundary_test.go::TestKernelCompressionBoundary_*` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 4 / 4.F | Title prompt and truncation contract | Native title generation exposes a pure request/response boundary that builds Hermes-compatible title prompts from bounded session history, truncates candidate titles deterministically, returns empty-title fallback evidence for empty history or blank model output, and surfaces provider failures through a typed nonfatal error result without writing session metadata | operator, system | `internal/hermes/title_generator_test.go::TestTitle*` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.N | Session search tool schema and argument validation | internal/tools defines a session_search Tool descriptor, JSON schema, timeout, and argument validator for query, scope, sources, mode, limit, and current_session_id without registering the tool globally or reading memory | operator, child-agent, system | `internal/tools/session_search_tool_schema_test.go::TestSessionSearchToolSchema_*` | Unblocks Session search tool execution wrapper. |
| 5 / 5.O | CLI OpenClaw residue detection and hint text | internal/cli exposes pure DetectOpenClawResidue(home string) bool and OpenClawResidueHint(commandName string) string helpers that detect only an existing ~/.openclaw directory and return Gormes-specific cleanup guidance without reading or writing config files | operator, system | `internal/cli/openclaw_residue_test.go::TestOpenClawResidue*` | Unblocks CLI onboarding seen-state map helpers. |
| 5 / 5.O | Custom provider model-switch key_env write guard | internal/cli exposes a pure model-switch patch helper that updates a custom provider default_model while preserving original credential storage: providers that relied on key_env and had no inline api_key/api_key_ref must not gain an api_key entry, while providers that already had an inline plaintext or `${VAR}` api_key may keep that existing api_key value without writing resolved plaintext | operator, system | `internal/cli/custom_provider_model_switch_test.go::TestCustomProviderModelSwitchPatch_*` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.Q | Native TUI conversation viewport tail helper | internal/tui exposes a pure conversation viewport helper that clips RenderFrame.History to the visible tail under width/height budgets, emits a deterministic omitted-history sentinel, and always preserves DraftText and LastError inputs | operator, system | `internal/tui/viewport_history_test.go::TestConversationViewportTail_*` | Unblocks Native TUI renderConv viewport budget binding. |
<!-- PROGRESS:END -->
