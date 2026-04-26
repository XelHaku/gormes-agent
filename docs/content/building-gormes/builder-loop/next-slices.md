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
| 1 / 1.C | Watchdog checkpoint coalescing | When a worker stalls, the watchdog produces at most one dirty-worktree checkpoint commit per stall window — successive stuck ticks amend or no-op instead of stacking new commits | operator, system | `internal/builderloop/watchdog_coalesce_test.go` | Unblocks Watchdog dead-process vs slow-progress separation. |
| 1 / 1.C | PR-intake idle backoff | When pr_intake list returns zero PRs N consecutive times, the next poll backs off to a configurable idle interval (default 5 min) instead of running on every loop cycle; the first non-empty list resets the backoff to baseline cadence | system | `internal/builderloop/pr_intake_backoff_test.go` | Unblocks Builder-loop self-improvement vs user-feature ratio metric. |
| 1 / 1.C | Watchdog dead-process vs slow-progress separation | The watchdog distinguishes a dead worker (no PID, exited, or PID gone from os.findprocess) from a slow worker (PID alive but no commits) and applies independently configurable thresholds, with the dead-process check firing in <2 minutes | operator, system | `internal/builderloop/watchdog_state_test.go` | Unblocks Builder-loop self-improvement vs user-feature ratio metric. |
| 1 / 1.C | Builder-loop self-improvement vs user-feature ratio metric | record_run_health carries a self_improvement vs user_feature ship ratio over a configurable window, classifying each shipped row by which subphase prefix it landed under, so post-mortems can detect when the loop is mostly working on itself | system | `internal/builderloop/ship_ratio_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 4 / 4.A | Azure Foundry probe — path sniffing | Azure Foundry endpoint URLs are classified into anthropic_messages, openai_chat_completions, or unknown solely by URL path/host inspection, with no HTTP, no credential reads, and no config writes | operator, system | `internal/hermes/azure_foundry_path_sniff_test.go` | Unblocks Azure Foundry probe — /models classification + Anthropic fallback. |
| 4 / 4.A | Azure Foundry probe — /models classification + Anthropic fallback | Given a fake /models response (or its absence), classify an Azure Foundry deployment as openai_chat_completions, anthropic_messages, or manual-required, with explicit probe evidence and zero credential or config writes | operator, system | `internal/hermes/azure_foundry_models_probe_test.go` | Unblocks Azure Foundry transport probe read model. |
| 4 / 4.H | Provider rate guard — x-ratelimit header classification | Pure functions parse x-ratelimit-* headers and classify a 429 as genuine_quota, upstream_capacity, or insufficient_evidence with redacted reset windows, no sleeps, and no shared breaker writes | system | `internal/hermes/provider_rate_guard_classification_test.go` | Unblocks Provider rate guard — degraded-state + last-known-good evidence. |
| 4 / 4.H | Provider rate guard — degraded-state + last-known-good evidence | Last-known-good bucket state distinguishes bare 429s; degraded modes (rate_guard_unavailable, budget_header_missing) are reported as visible provider status evidence without retry timing changes or shared mutable breaker state | system | `internal/hermes/provider_rate_guard_degraded_test.go` | Unblocks Provider rate guard + budget telemetry. |
| 5 / 5.Q | Native TUI terminal-selection divergence contract | Gormes documents and fixture-locks a terminal-native selection model for the Bubble Tea TUI, with no advertised custom copy hotkey until a Go-native implementation exists | operator | `internal/tui/selection_copy_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
<!-- PROGRESS:END -->
