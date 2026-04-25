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
| 5 / 5.Q | Native TUI bundle independence check | Gormes TUI startup and install/update status stay Go-native and never depend on Hermes' Node/Ink dist bundle freshness checks | operator, system | `cmd/gormes/tui_bundle_independence_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.Q | TUI launch model override + static alias resolver | Gormes TUI honors top-level --model/--provider and GORMES_INFERENCE_* overrides at startup without network model lookup or oneshot-only coupling | operator, system | `cmd/gormes/tui_model_override_test.go` | Unblocks SSE streaming to Bubble Tea TUI. |
| 5 / 5.Q | Native TUI terminal-selection divergence contract | Gormes documents and fixture-locks a terminal-native selection model for the Bubble Tea TUI, with no advertised custom copy hotkey until a Go-native implementation exists | operator | `internal/tui/selection_copy_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
| 4 / 4.A | DeepSeek/Kimi cross-provider reasoning isolation | DeepSeek and Kimi replay of assistant tool-call turns injects an empty reasoning_content placeholder before promoting generic stored reasoning from another provider | system | `internal/hermes/reasoning_content_echo_test.go` | Unblocks OpenRouter, Cross-provider reasoning-tag sanitization, Codex stream repair + tool-call leak sanitizer. |
| 5 / 5.G | MCP server config/env resolver | Gormes parses Hermes-compatible mcp_servers config into safe stdio/HTTP server definitions with env validation, timeout defaults, header redaction, and Honcho MCP self-hosted URL support before any live MCP connection starts | operator, system | `internal/tools/mcp_config_test.go` | Unblocks MCP fake-server discovery + tool schema normalization, MCP OAuth state store + noninteractive auth errors. |
| 5 / 5.O | CLI banner/output formatting helpers | Pure CLI banner and output formatting helpers match upstream deterministic text behavior without terminal or command-registry coupling | operator, system | `internal/cli/banner_output_test.go` | Unblocks CLI tips/dump/webhook deterministic helpers. |
| 5 / 5.O | CLI profile path and active-profile store | Gormes models Hermes profile names, active-profile selection, and profile-root resolution as pure XDG-scoped helpers before command UI, alias wrappers, cloning, or export/import behavior is ported | operator, system | `internal/cli/profile_store_test.go` | Unblocks CLI auth status read model before provider setup, Setup/uninstall dry-run command contracts. |
| 5 / 5.O | Gateway management CLI read-model closeout | Gateway management CLI exposes read-only status, pairing, runtime-validation, and channel-availability evidence over existing Gormes stores before mutating start/stop/restart commands widen the surface | operator, gateway, system | `cmd/gormes/gateway_management_cli_test.go` | Unblocks Webhook/platform management CLI helpers, Cron management CLI over native store. |
| 5 / 5.O | CLI log snapshot reader | Gormes captures redacted local log snapshots for agent, gateway, error, tool-audit, and builder-loop logs from XDG paths without network upload or archive creation | operator, system | `internal/cli/log_snapshot_test.go` | Unblocks CLI status summary over native stores, Backup manifest dry-run contract. |
<!-- PROGRESS:END -->
