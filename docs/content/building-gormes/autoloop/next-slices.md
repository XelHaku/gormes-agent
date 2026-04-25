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
| 2 / 2.B.3 | Slack gateway.Channel adapter shim | Slack Socket Mode can run under the shared gateway.Manager through a narrow gateway.Channel shim without replacing the existing Slack client or reply fixtures | gateway, operator | `internal/slack/channel_shim_test.go` | Unblocks Slack config + cmd/gormes gateway registration. |
| 2 / 2.F.3 | Drain-timeout resume_pending recovery | Gateway restart/shutdown drain timeouts preserve resumable session identity and inject a reason-aware resume note on the next turn without overriding hard-stuck or suspended state | gateway, operator | `internal/gateway/resume_pending_test.go` | Unblocks Gateway /restart command + takeover markers. |
| 2 / 2.F.3 | `gormes gateway status` read-only command | A read-only `gormes gateway status` command renders configured channels, pairing state, and runtime lifecycle from stores without starting transports or agent sessions | operator | `cmd/gormes/gateway_status_test.go` | Unblocks Runtime status JSON + PID/process validation, Gateway /restart command + takeover markers. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
<!-- PROGRESS:END -->
