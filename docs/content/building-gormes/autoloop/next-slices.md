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
<!-- PROGRESS:END -->
