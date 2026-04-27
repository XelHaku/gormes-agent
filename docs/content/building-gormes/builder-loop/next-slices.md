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
| 4 / 4.F | Title prompt and truncation contract | Native title generation exposes a pure request/response boundary that builds Hermes-compatible title prompts from bounded session history, truncates candidate titles deterministically, returns empty-title fallback evidence for empty history or blank model output, and surfaces provider failures through a typed nonfatal error result without writing session metadata | operator, system | `internal/hermes/title_generator_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 4 / 4.H | Provider timeout config fail-closed helper | Provider timeout lookup handles missing or failed config loading by returning explicit unset evidence, and parses provider request/stale timeout overrides without panicking or applying stale defaults | operator, system | `internal/hermes/provider_timeout_config_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.C | Browser SSRF quoted-false guard | Browser and URL safety helpers coerce quoted false-like config values (`"false"`, `'false'`, `0`, `no`, `off`) to disabled booleans before private/local URL SSRF guards decide whether cloud navigation is allowed | operator, system | `internal/tools/browser_ssrf_guard_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.F | Skills hub direct URL candidate parser | internal/skills exposes a pure ParseURLSkillCandidate(rawURL string, skillMD []byte) (URLSkillCandidate, error) helper that mirrors Hermes UrlSource.fetch metadata without network or store writes: HTTPS SKILL.md URLs only, source=url, trust=community, files={SKILL.md}, resolved name from valid frontmatter or URL slug, awaiting_name evidence when neither produces a safe install name, and no path traversal in name/category candidates | operator, system | `internal/skills/url_candidate_test.go` | Unblocks Skills hub direct URL install name/category guard. |
| 5 / 5.G | MCP stdio orphan cleanup after cron ticks | Cron and MCP stdio tooling track orphaned stdio server PIDs after cancellation/timeout and sweep only orphaned children after a cron tick joins, without killing active MCP sessions from parallel work | operator, system | `internal/tools/mcp_orphan_cleanup_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.L | Checkpoint shadow-repo GC policy | Native checkpoint manager prunes orphan and stale shadow repositories at startup using a deterministic policy before any write-capable file tools depend on rollback state | operator, child-agent, system | `internal/tools/checkpoint_manager_test.go` | Unblocks File read dedup cache invalidation and wrapper guard. |
| 5 / 5.N | Session search | Native session_search tool uses the Phase 3 session catalog with same-chat defaults, opt-in user/source filters, and deterministic recent-mode exclusion of the current lineage root | operator, child-agent, system | `internal/tools/session_search_tool_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.O | CLI OpenClaw residue onboarding hint | internal/cli exposes pure OpenClaw-residue onboarding helpers: DetectOpenClawResidue(home string) bool returns true only for an existing ~/.openclaw directory, OpenClawResidueHint(commandName string) string returns a Gormes-specific one-time cleanup hint, and OnboardingSeen/MarkOnboardingSeen operate on an in-memory map shape compatible with config onboarding.seen without reading or writing real config files | operator, system | `internal/cli/onboarding_test.go` | Unblocks OpenClaw residue startup banner binding. |
| 5 / 5.O | CLI bracketed-paste wrapper sanitizer | internal/cli exposes StripLeakedBracketedPasteWrappers(text string) string, a pure sanitizer that removes canonical ESC [200~/[201~ wrappers, visible caret-escape wrappers, degraded boundary [200~/[201~ wrappers, and boundary 00~/01~ fragments while preserving non-wrapper substrings inside ordinary text | operator, system | `internal/cli/paste_sanitizer_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
<!-- PROGRESS:END -->
