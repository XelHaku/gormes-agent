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
| 2 / 2.B.5 | Telegram group bot-command mention gate helper | Telegram ingress exposes a pure mention-gate helper that treats `/cmd@botname` bot_command entities as direct address when group require-mention policy is enabled, rejects commands addressed to other bots, and keeps bare group slash commands gated | gateway, operator, system | `internal/channels/telegram/group_mention_test.go` | Unblocks Telegram group mention gate config binding. |
| 2 / 2.E.3 | Durable worker RSS watchdog policy helper | Gormes durable worker exposes a pure RSS watchdog policy helper that classifies disabled mode, unavailable RSS reads, threshold exceeded evidence, and stable watchdog restart reset without integrating with the worker loop yet | operator, system | `internal/subagent/durable_worker_rss_watchdog_test.go::TestDurableWorkerRSSWatchdogPolicy` | Unblocks Durable worker RSS drain integration. |
| 2 / 2.F.5 | Steer slash command registry + queue fallback | Registry-owned active-turn steering command | operator, gateway | `internal/gateway/steer_queue_test.go` | Unblocks Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard. |
| 4 / 4.F | Title prompt and truncation contract | Native title generation exposes a pure request/response boundary that builds Hermes-compatible title prompts from bounded session history, truncates candidate titles deterministically, returns empty-title fallback evidence for empty history or blank model output, and surfaces provider failures through a typed nonfatal error result without writing session metadata | operator, system | `internal/hermes/title_generator_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 4 / 4.H | Provider timeout config fail-closed helper | Provider timeout lookup handles missing or failed config loading by returning explicit unset evidence, and parses provider request/stale timeout overrides without panicking or applying stale defaults | operator, system | `internal/hermes/provider_timeout_config_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.N | Session search | Native session_search tool uses the Phase 3 session catalog with same-chat defaults, opt-in user/source filters, and deterministic recent-mode exclusion of the current lineage root | operator, child-agent, system | `internal/tools/session_search_tool_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.O | CLI OpenClaw residue onboarding hint | internal/cli exposes pure OpenClaw-residue onboarding helpers: DetectOpenClawResidue(home string) bool returns true only for an existing ~/.openclaw directory, OpenClawResidueHint(commandName string) string returns a Gormes-specific one-time cleanup hint, and OnboardingSeen/MarkOnboardingSeen operate on an in-memory map shape compatible with config onboarding.seen without reading or writing real config files | operator, system | `internal/cli/onboarding_test.go` | Unblocks OpenClaw residue startup banner binding. |
| 5 / 5.O | CLI bracketed-paste wrapper sanitizer | internal/cli exposes StripLeakedBracketedPasteWrappers(text string) string, a pure sanitizer that removes canonical ESC [200~/[201~ wrappers, visible caret-escape wrappers, degraded boundary [200~/[201~ wrappers, and boundary 00~/01~ fragments while preserving non-wrapper substrings inside ordinary text | operator, system | `internal/cli/paste_sanitizer_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.Q | Native TUI /save XDG file writer binding | cmd/gormes binds the native TUI /save SessionExportFunc to the canonical persisted transcript reader and writes markdown exports under the Gormes XDG data directory, never under upstream HERMES_HOME or the process working directory | operator, system | `cmd/gormes/tui_save_export_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.Q | Native TUI bounded conversation viewport | Native Bubble Tea conversation rendering limits each frame to the visible tail of RenderFrame.History plus DraftText/LastError under a caller-provided height budget, emits a stable omitted-history sentinel when earlier turns are hidden, and avoids rebuilding unbounded conversation strings during long sessions | operator, system | `internal/tui/viewport_history_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
<!-- PROGRESS:END -->
