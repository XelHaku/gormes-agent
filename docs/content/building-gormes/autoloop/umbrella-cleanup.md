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
| 5 / 5.O | 49-file CLI tree port | `tools` | The row is assigned as a whole hermes_cli tree migration instead of command-group slices. | Deterministic helper-file ports (banner/output/tips/webhook/dump), CLI command registry parity + active-turn busy policy, Config, profile, auth, and setup command surfaces, Gateway, platform, webhook, and cron management CLI, Diagnostics, backup, logs, and status CLI |
<!-- PROGRESS:END -->
