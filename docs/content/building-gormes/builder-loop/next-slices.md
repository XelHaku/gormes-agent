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
| 2 / 2.B.5 | Telegram group mention gate config binding | Telegram runtime can opt into Hermes-style group require-mention policy by using the validated bot-command mention helper to drop unaddressed group text and bare slash commands while leaving DMs, allowed-chat gating, first-run discovery, and fresh-final streaming unchanged | gateway, operator, system | `internal/channels/telegram/group_mention_binding_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 2 / 2.B.5 | Gateway inbound dedup evidence wiring | Gateway manager applies the shared MessageDeduplicator to inbound events with stable message IDs, drops duplicate submissions with visible evidence, and degrades missing message IDs without suppressing the turn | gateway, system | `internal/gateway/message_deduplicator_manager_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 2 / 2.E.3 | Durable worker RSS drain integration | DurableWorker integrates the validated RSS watchdog policy after job completion and on an injected periodic check, starts graceful drain, cancels in-flight handlers through existing abort-slot recovery, and records watchdog drain evidence | operator, system | `internal/subagent/durable_worker_rss_drain_test.go` | Unblocks Durable worker RSS doctor/status evidence. |
| 2 / 2.F.5 | Steer slash command registry + queue fallback | Registry-owned active-turn steering command | operator, gateway | `internal/gateway/steer_queue_test.go::TestSteerCommandRegistry_*` | Unblocks Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard. |
| 4 / 4.B | ContextEngine compression-boundary notification | Kernel compression execution signals the ContextEngine when a compression boundary is crossed so cached/replayed context state cannot silently span the pre-compress and post-compress transcript | operator, system | `internal/kernel/compression_boundary_test.go::TestKernelCompressionBoundary_*` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 4 / 4.F | Title prompt and truncation contract | Native title generation exposes a pure request/response boundary that builds Hermes-compatible title prompts from bounded session history, truncates candidate titles deterministically, returns empty-title fallback evidence for empty history or blank model output, and surfaces provider failures through a typed nonfatal error result without writing session metadata | operator, system | `internal/hermes/title_generator_test.go::TestTitle*` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.N | Session search | internal/tools exposes a wrapper-only session_search Tool over existing internal/memory SearchSessions/SearchMessages APIs, preserving same-chat defaults, explicit user/source widening, lineage-root exclusion in recent mode, and Goncho/Honcho-compatible evidence without changing ranking or persistence | operator, child-agent, system | `internal/tools/session_search_tool_test.go::TestSessionSearchTool_*` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.O | CLI OpenClaw residue onboarding hint | internal/cli exposes pure OpenClaw-residue onboarding helpers: DetectOpenClawResidue(home string) bool returns true only for an existing ~/.openclaw directory, OpenClawResidueHint(commandName string) string returns a Gormes-specific one-time cleanup hint, and OnboardingSeen/MarkOnboardingSeen operate on an in-memory map shape compatible with config onboarding.seen without reading or writing real config files | operator, system | `internal/cli/onboarding_test.go::Test{DetectOpenClawResidue,OpenClawResidueHint,OnboardingSeen,MarkOnboardingSeen}_*` | Unblocks OpenClaw residue startup banner binding. |
| 5 / 5.O | Custom provider model-switch key_env write guard | internal/cli exposes a pure model-switch patch helper that updates a custom provider default_model while preserving original credential storage: providers that relied on key_env and had no inline api_key/api_key_ref must not gain an api_key entry, while providers that already had an inline plaintext or `${VAR}` api_key may keep that existing api_key value without writing resolved plaintext | operator, system | `internal/cli/custom_provider_model_switch_test.go::TestCustomProviderModelSwitchPatch_*` | Contract metadata is present; ready for a focused spec or fixture slice. |
<!-- PROGRESS:END -->
