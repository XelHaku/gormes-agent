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
| 5 / 5.Q | Native TUI terminal-selection divergence contract | Gormes documents and fixture-locks a terminal-native selection model for the Bubble Tea TUI, with no advertised custom copy hotkey until a Go-native implementation exists | operator | `internal/tui/selection_copy_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
| 5 / 5.O | CLI profile path and active-profile store | Gormes models Hermes profile names, active-profile selection, and profile-root resolution as pure XDG-scoped helpers before command UI, alias wrappers, cloning, or export/import behavior is ported | operator, system | `internal/cli/profile_store_test.go` | Unblocks CLI auth status read model before provider setup, Setup/uninstall dry-run command contracts. |
| 5 / 5.O | Gateway management CLI read-model closeout | Gateway management CLI exposes read-only status, pairing, runtime-validation, and channel-availability evidence over existing Gormes stores before mutating start/stop/restart commands widen the surface | operator, gateway, system | `cmd/gormes/gateway_management_cli_test.go` | Unblocks Webhook/platform management CLI helpers, Cron management CLI over native store. |
| 5 / 5.O | Doctor custom endpoint provider readiness | gormes doctor accepts custom endpoint/provider-style configuration as operator intent and reports credential/readiness evidence without requiring a named provider registry match | operator, system | `cmd/gormes/doctor_custom_provider_test.go` | Unblocks CLI status summary over native stores. |
| 5 / 5.O | CLI log snapshot reader | Gormes captures redacted local log snapshots for agent, gateway, error, tool-audit, and builder-loop logs from XDG paths without network upload or archive creation | operator, system | `internal/cli/log_snapshot_test.go` | Unblocks CLI status summary over native stores, Backup manifest dry-run contract. |
| 5 / 5.Q | TUI gateway progress/completion helpers | Pure TUI gateway helper functions normalize tool-progress mode, completion paths, and tool summary formatting from fixed inputs | operator, system | `internal/tuigateway/progress_completion_test.go` | Unblocks TUI gateway image/personality/platform-event helpers. |
<!-- PROGRESS:END -->
