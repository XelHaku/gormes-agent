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
| 2 / 2.B.5 | WhatsApp unsafe sender/chat inbound evidence | WhatsApp inbound normalization in internal/channels/whatsapp/inbound.go uses the validated NormalizeSafeWhatsAppIdentifier helper to reject unsafe raw sender, chat, and reply target identifiers before Event.UserID, Event.ChatID, Reply.ChatID, or outbound pairing targets are created | gateway, operator, system | `internal/channels/whatsapp/identity_inbound_safety_test.go::TestNormalizeInboundWithIdentity_UnsafeRawIDs` | Unblocks WhatsApp unsafe alias endpoint inbound evidence. |
| 4 / 4.B | ContextEngine compression-boundary callback vocabulary | internal/hermes defines a compression-boundary callback vocabulary on ContextEngine with stable lineage evidence and status fields, without binding kernel compression execution yet | operator, system | `internal/hermes/context_engine_boundary_test.go::TestContextEngineCompressionBoundaryVocabulary` | Unblocks ContextEngine compression-boundary notification. |
| 5 / 5.O | Custom provider model-switch key_env write guard | internal/cli exposes a pure model-switch patch helper that accepts an in-memory custom provider ref plus a target model and returns the config patch/evidence for default_model changes while preserving original credential storage: providers that relied on key_env and had no inline api_key/api_key_ref must not gain an api_key entry, while providers that already had inline plaintext or `${VAR}` api_key may keep that existing value without writing resolved plaintext | operator, system | `internal/cli/custom_provider_model_switch_test.go::TestCustomProviderModelSwitchPatch_*` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.Q | API server detailed health endpoint | Native API server binds the validated DetailedHealthSnapshot value model to unauthenticated GET /health/detailed and /v1/health/detailed routes while preserving existing flat /health and /v1/health behavior | operator, gateway, system | `internal/apiserver/detailed_health_endpoint_test.go` | Unblocks API server cron admin read-only endpoints. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
| 7 / 7.E | Yuanbao protocol envelope + markdown fixtures | Gormes parses Yuanbao websocket/protobuf-style envelopes and Markdown message fragments into gateway-neutral events using fixture data only | gateway, system | `internal/channels/yuanbao/proto_test.go` | Unblocks Yuanbao media/sticker attachment normalization, Yuanbao gateway runtime + toolset registration. |
<!-- PROGRESS:END -->
