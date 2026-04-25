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
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
| 4 / 4.B | Aux compression single-prompt threshold reconciliation | Auxiliary compression budgeting follows Hermes 5006b220 by treating the summarizer request as raw messages plus one small user instruction, not as a system-prompt-plus-tool-schema memory-flush request | operator, system | `internal/hermes/context_compressor_single_prompt_test.go` | Unblocks Tool-result pruning + protected head/tail summary, Manual compression feedback + context references. |
| 4 / 4.H | Codex Responses temperature guard after flush removal | Codex Responses payload conversion keeps omitting temperature while removing obsolete flush_memories fixture names, source references, and docs language after Hermes 5006b220 deleted memory flush | system | `internal/hermes/codex_responses_temperature_test.go` | Unblocks Codex OAuth state + stale-token relogin. |
| 5 / 5.O | Top-level oneshot flag and model/provider resolver | Gormes accepts a top-level `-z/--oneshot` prompt plus `--model`, `--provider`, GORMES_INFERENCE_MODEL, and GORMES_INFERENCE_PROVIDER overrides with Hermes-compatible ambiguity errors before any agent execution starts | operator, system | `cmd/gormes/oneshot_flags_test.go` | Unblocks Oneshot stdout-only kernel execution. |
| 2 / 2.F.3 | Session expiry finalization without memory flush | Gateway session expiry finalizes hooks, cleanup, and cache eviction once per expired session without launching a model-driven memory flush agent | gateway, system | `internal/gateway/session_expiry_finalize_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.O | Update service restart active polling | Update and service-management flows verify restarted gateway services by polling active status for at least RestartSec plus slack instead of racing the systemd cooldown window | operator, system | `internal/cli/service_restart_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
<!-- PROGRESS:END -->
