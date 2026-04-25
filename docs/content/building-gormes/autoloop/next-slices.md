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
| 2 / 2.F.3 | Session expiry finalized-flag migration | Gateway session metadata migrates legacy memory_flushed state into expiry_finalized evidence without adding new hidden memory-flush writes | gateway, system | `internal/session/expiry_finalized_migration_test.go` | Unblocks Session expiry hook cleanup retry evidence. |
| 4 / 4.A | Codex Responses assistant content role types | Codex Responses payload conversion emits role-correct text content parts: input_text for user messages and output_text for assistant replay messages | system | `internal/hermes/codex_responses_role_content_test.go` | Unblocks Codex OAuth state + stale-token relogin. |
| 5 / 5.O | Oneshot final-output writer boundary | One-shot mode runs one native Gormes kernel turn over a fake provider and writes only final assistant content plus one trailing newline to stdout | operator, system | `cmd/gormes/oneshot_output_test.go` | Unblocks Oneshot noninteractive safety and clarify policy. |
| 5 / 5.O | Service RestartSec parser helper | Service-management helpers parse systemd RestartUSec/RestartSec evidence into a bounded restart delay without invoking live service managers | operator, system | `internal/cli/service_restart_parse_test.go` | Unblocks Service restart active-status poller. |
| 4 / 4.H | Streaming interrupt retry suppression | Kernel stream cancellation and /stop-style events abort retry loops before any fresh provider stream is opened | operator, system | `internal/kernel/stream_interrupt_retry_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
<!-- PROGRESS:END -->
