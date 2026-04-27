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
| 2 / 2.E.3 | Durable worker RSS watchdog policy helper | Gormes durable worker exposes a pure RSS watchdog policy helper that classifies disabled mode, unavailable RSS reads, threshold exceeded evidence, and stable watchdog restart reset without integrating with the worker loop yet | operator, system | `internal/subagent/durable_worker_rss_watchdog_test.go::TestDurableWorkerRSSWatchdogPolicy` | Unblocks Durable worker RSS drain integration. |
| 2 / 2.F.5 | Steer slash command registry + queue fallback | Registry-owned active-turn steering command | operator, gateway | `internal/gateway/steer_command_test.go` | Unblocks Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard. |
| 4 / 4.H | Provider timeout config fail-closed helper | Provider timeout lookup handles missing or failed config loading by returning explicit unset evidence, and parses provider request/stale timeout overrides without panicking or applying stale defaults | operator, system | `internal/hermes/provider_timeout_config_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.C | Browser SSRF quoted-false guard | Browser and URL safety helpers coerce quoted false-like config values (`"false"`, `'false'`, `0`, `no`, `off`) to disabled booleans before private/local URL SSRF guards decide whether cloud navigation is allowed | operator, system | `internal/tools/browser_ssrf_guard_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.G | MCP stdio orphan cleanup after cron ticks | Cron and MCP stdio tooling track orphaned stdio server PIDs after cancellation/timeout and sweep only orphaned children after a cron tick joins, without killing active MCP sessions from parallel work | operator, system | `internal/tools/mcp_orphan_cleanup_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.L | Checkpoint shadow-repo GC policy | Native checkpoint manager prunes orphan and stale shadow repositories at startup using a deterministic policy before any write-capable file tools depend on rollback state | operator, child-agent, system | `internal/tools/checkpoint_manager_test.go` | Unblocks File read dedup cache invalidation and wrapper guard. |
| 5 / 5.N | Session search | Native session_search tool uses the Phase 3 session catalog with same-chat defaults, opt-in user/source filters, and deterministic recent-mode exclusion of the current lineage root | operator, child-agent, system | `internal/tools/session_search_tool_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.Q | Native TUI /save XDG file writer binding | cmd/gormes binds the native TUI /save SessionExportFunc to the canonical persisted transcript reader and writes markdown exports under the Gormes XDG data directory, never under upstream HERMES_HOME or the process working directory | operator, system | `cmd/gormes/tui_save_export_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.Q | API server detailed health read-model | Native API server exposes a detailed health read-model that reports provider, response-store, run-event stream, gateway, and cron availability/degradation without leaking secrets or adding cron mutations | operator, gateway, system | `internal/apiserver/detailed_health_test.go` | Unblocks API server cron admin read-only endpoints. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
<!-- PROGRESS:END -->
