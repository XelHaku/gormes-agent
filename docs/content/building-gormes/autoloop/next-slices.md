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
| 2 / 2.B.4 | WhatsApp outbound pairing gate + raw peer mapping | WhatsApp outbound sends are gated by pairing state and map normalized gateway chat IDs back to bridge/native raw peers | gateway, operator | `internal/channels/whatsapp/send_contract_test.go` | Unblocks WhatsApp reconnect backoff + send retry policy. |
| 2 / 2.E.3 | Durable job backpressure + timeout audit | Gormes durable jobs expose GBrain-style max-waiting backpressure, wall-clock timeout evidence, and operator-readable queue health without importing Minions runtime code | operator, system | `internal/subagent/durable_backpressure_test.go` | Unblocks Durable worker supervisor status seam. |
| 5 / 5.Q | TUI mouse tracking config + slash toggle | Mouse/wheel tracking is config-backed, runtime-toggleable, and emits terminal enable/disable state without restarting the TUI | operator, system | `internal/tui/mouse_tracking_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
<!-- PROGRESS:END -->
