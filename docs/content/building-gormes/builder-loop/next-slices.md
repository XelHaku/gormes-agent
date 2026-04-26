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
| 4 / 4.A | Azure Foundry probe — path sniffing | Azure Foundry endpoint URLs are classified into anthropic_messages, openai_chat_completions, or unknown solely by URL path/host inspection, with no HTTP, no credential reads, and no config writes | operator, system | `internal/hermes/azure_foundry_path_sniff_test.go` | Unblocks Azure Foundry probe — /models classification + Anthropic fallback. |
| 4 / 4.A | Azure Foundry probe — /models classification + Anthropic fallback | Given a fake /models response (or its absence), classify an Azure Foundry deployment as openai_chat_completions, anthropic_messages, or manual-required, with explicit probe evidence and zero credential or config writes | operator, system | `internal/hermes/azure_foundry_models_probe_test.go` | Unblocks Azure Foundry transport probe read model. |
| 4 / 4.H | Provider rate guard — x-ratelimit header classification | Pure functions parse x-ratelimit-* headers and classify a 429 as genuine_quota, upstream_capacity, or insufficient_evidence with redacted reset windows, no sleeps, and no shared breaker writes | system | `internal/hermes/provider_rate_guard_classification_test.go` | Unblocks Provider rate guard — degraded-state + last-known-good evidence. |
| 4 / 4.H | Provider rate guard — degraded-state + last-known-good evidence | Last-known-good bucket state distinguishes bare 429s; degraded modes (rate_guard_unavailable, budget_header_missing) are reported as visible provider status evidence without retry timing changes or shared mutable breaker state | system | `internal/hermes/provider_rate_guard_degraded_test.go` | Unblocks Provider rate guard + budget telemetry. |
| 5 / 5.Q | Native TUI terminal-selection divergence contract | Gormes documents and fixture-locks a terminal-native selection model for the Bubble Tea TUI, with no advertised custom copy hotkey until a Go-native implementation exists | operator | `internal/tui/selection_copy_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
| 5 / 5.O | CLI profile path and active-profile store | Gormes models Hermes profile names, active-profile selection, and profile-root resolution as pure XDG-scoped helpers before command UI, alias wrappers, cloning, or export/import behavior is ported | operator, system | `internal/cli/profile_store_test.go` | Unblocks CLI auth status read model before provider setup, Setup/uninstall dry-run command contracts. |
| 5 / 5.O | Gateway management CLI read-model closeout | Gateway management CLI exposes read-only status, pairing, runtime-validation, and channel-availability evidence over existing Gormes stores before mutating start/stop/restart commands widen the surface | operator, gateway, system | `cmd/gormes/gateway_management_cli_test.go` | Unblocks Webhook/platform management CLI helpers, Cron management CLI over native store. |
| 5 / 5.O | Doctor custom endpoint provider readiness | gormes doctor accepts custom endpoint/provider-style configuration as operator intent and reports credential/readiness evidence without requiring a named provider registry match | operator, system | `cmd/gormes/doctor_custom_provider_test.go` | Unblocks CLI status summary over native stores. |
| 5 / 5.O | CLI log snapshot reader | Gormes captures redacted local log snapshots for agent, gateway, error, tool-audit, and builder-loop logs from XDG paths without network upload or archive creation | operator, system | `internal/cli/log_snapshot_test.go` | Unblocks CLI status summary over native stores, Backup manifest dry-run contract. |
<!-- PROGRESS:END -->
