---
title: "Umbrella Cleanup"
weight: 50
aliases:
  - /building-gormes/umbrella-cleanup/
---

# Umbrella Cleanup

Umbrella rows are inventory or tracking rows, not executable implementation
slices. Split these into smaller rows with contracts, fixtures, trust classes,
and acceptance checks before assigning them to an implementation agent.

<!-- PROGRESS:START kind=umbrella-cleanup -->
| Phase | Umbrella row | Owner | Not ready when | Split into |
|---|---|---|---|---|
| 4 / 4.A | Codex | `provider` | The row is assigned as one large Codex provider implementation instead of the Responses conversion, auth, and stream-repair slices below. | Codex Responses pure conversion harness, Codex Responses assistant content role types, Codex OAuth state + stale-token relogin, Codex stream repair + tool-call leak sanitizer |
| 4 / 4.B | Long session management | `provider` | The row is assigned as one implementation task instead of being split through context engine, token-budget, reference, and compression slices. | ContextEngine interface + status tool contract, Compression token-budget trigger + summary sizing, Manual compression feedback + context references |
| 4 / 4.D | Model metadata registry + context limits | `provider` | The row is assigned as one metadata/routing implementation instead of the resolver, pricing/capability, and selector slices below. | Provider-enforced context-length resolver, Model pricing/capability registry fixtures, Routing policy and fallback selector |
| 5 / 5.A | 61-tool registry port | `tools` | The row is treated as a bulk 61-handler port before descriptor parity and trust classes are frozen. | Tool registry inventory + schema parity harness, Pure core tools first, Stateful tool migration queue |
| 5 / 5.G | MCP client | `tools` | The row is assigned as one all-MCP migration instead of the config, discovery, OAuth, and managed-gateway slices below. | MCP server config/env resolver, MCP stdio transport + tool/list discovery, MCP HTTP transport + tool/list discovery, MCP schema normalization + structured-content adapter, MCP OAuth state store + noninteractive auth errors, Managed tool gateway bridge |
| 5 / 5.O | 49-file CLI tree port | `tools` | The row is assigned as a whole hermes_cli tree migration instead of command-group slices. | Deterministic helper-file ports (banner/output/tips/webhook/dump), CLI command registry parity + active-turn busy policy, Config, profile, auth, and setup command surfaces, Gateway, platform, webhook, and cron management CLI, Diagnostics, backup, logs, and status CLI |
| 5 / 5.O | Deterministic helper-file ports (banner/output/tips/webhook/dump) | `tools` | The row is assigned as one combined hermes_cli helper-file migration instead of the four pure-helper slices below. | CLI banner/output formatting helpers, CLI deterministic tip selector, CLI webhook URL normalizer, CLI dump support-summary helper |
| 5 / 5.O | Config, profile, auth, and setup command surfaces | `tools` | The row is assigned as one combined config/profile/auth/setup migration instead of the pure profile, auth-status, setup, and uninstall slices. | CLI profile name validator, CLI profile root resolver, CLI active-profile store, CLI auth status read model before provider setup, Setup/uninstall dry-run command contracts |
| 5 / 5.O | CLI profile path and active-profile store (deprecated umbrella) | `tools` | The row is selected at all — execute the three sibling rows above (CLI profile name validator, CLI profile root resolver, CLI active-profile store) instead. | - |
| 5 / 5.O | Gateway, platform, webhook, and cron management CLI | `tools` | The row is assigned as one management-CLI migration instead of separate gateway read-model, cron admin, webhook helper, and platform command slices. | Gateway management CLI read-model closeout, Cron management CLI over native store, Webhook/platform management CLI helpers |
| 5 / 5.O | Diagnostics, backup, logs, and status CLI | `tools` | The row is assigned as one combined diagnostics/backup/logs/status migration instead of log snapshot, status summary, backup manifest, and optional upload slices. | CLI log snapshot reader, CLI status summary over native stores, Backup manifest dry-run contract |
| 5 / 5.Q | Deterministic helper-file ports (tool-progress/image/completion-path/personality/platform-event) | `gateway` | The row is assigned as one combined tui_gateway helper migration instead of the two pure-helper slices below. | TUI gateway progress/completion helpers, TUI gateway image/personality/platform-event helpers |
<!-- PROGRESS:END -->
