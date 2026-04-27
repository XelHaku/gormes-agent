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
| 4 / 4.B | ContextEngine compression-boundary callback vocabulary | internal/hermes defines a compression-boundary callback vocabulary on ContextEngine with stable lineage evidence and status fields, without binding kernel compression execution yet | operator, system | `internal/hermes/context_engine_boundary_test.go::TestContextEngineCompressionBoundaryVocabulary` | Unblocks ContextEngine compression-boundary notification. |
| 5 / 5.D | Image input mode router + native content parts | internal/hermes exposes a pure image input routing helper that resolves agent.image_input_mode auto/native/text from model vision capability and auxiliary vision override, then builds native provider content parts with text plus data-url image_url entries without invoking a live provider | operator, system | `internal/hermes/image_routing_test.go::TestImageInputRouting_*` | Unblocks Image-too-large shrink retry helper. |
| 5 / 5.O | Backup/update opt-in and exclusion policy | CLI backup/update policy defaults pre-update backups off unless explicitly requested, honors --no-backup over --backup, and excludes checkpoints plus SQLite WAL/SHM/journal sidecars from backup manifests | operator, system | `internal/cli/backup_policy_test.go::TestBackupPolicy_*` | Unblocks Backup manifest dry-run contract. |
| 5 / 5.O | Custom provider model-switch key_env write guard | internal/cli exposes a pure model-switch patch helper that accepts an in-memory custom provider ref plus a target model and returns the config patch/evidence for default_model changes while preserving original credential storage: providers that relied on key_env and had no inline api_key/api_key_ref must not gain an api_key entry, while providers that already had inline plaintext or `${VAR}` api_key may keep that existing value without writing resolved plaintext | operator, system | `internal/cli/custom_provider_model_switch_test.go::TestCustomProviderModelSwitchPatch_*` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
| 7 / 7.E | Yuanbao protocol envelope + markdown fixtures | Gormes parses Yuanbao websocket/protobuf-style envelopes and Markdown message fragments into gateway-neutral events using fixture data only | gateway, system | `internal/channels/yuanbao/proto_test.go` | Unblocks Yuanbao media/sticker attachment normalization, Yuanbao gateway runtime + toolset registration. |
<!-- PROGRESS:END -->
