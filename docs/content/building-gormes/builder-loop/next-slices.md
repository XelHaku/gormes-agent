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
| 1 / 1.C | Backend usage-limit stdin health bypass | Builder-loop treats backend usage-limit/stdin-wait exits that produce no worker diff as backend infrastructure degradation, emits run-level backend evidence, and avoids charging the selected feature row as a worker_error quarantine candidate | system | `internal/builderloop/run_health_test.go::TestRunOnce_BackendUsageLimitDoesNotQuarantineRow` | Unblocks Steer slash command registry + queue fallback, ContextEngine compression-boundary notification, Custom provider model-switch key_env write guard, Native TUI /save XDG file writer binding, Native TUI bounded conversation viewport. |
| 4 / 4.F | Title prompt and truncation contract | Native title generation exposes a pure request/response boundary that builds Hermes-compatible title prompts from bounded session history, truncates candidate titles deterministically, returns empty-title fallback evidence for empty history or blank model output, and surfaces provider failures through a typed nonfatal error result without writing session metadata | operator, system | `internal/hermes/title_generator_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.N | Session search | Native session_search tool uses the Phase 3 session catalog with same-chat defaults, opt-in user/source filters, and deterministic recent-mode exclusion of the current lineage root | operator, child-agent, system | `internal/tools/session_search_tool_test.go` | Contract metadata is present; ready for a focused spec or fixture slice. |
| 5 / 5.O | CLI OpenClaw residue onboarding hint | internal/cli exposes pure OpenClaw-residue onboarding helpers: DetectOpenClawResidue(home string) bool returns true only for an existing ~/.openclaw directory, OpenClawResidueHint(commandName string) string returns a Gormes-specific one-time cleanup hint, and OnboardingSeen/MarkOnboardingSeen operate on an in-memory map shape compatible with config onboarding.seen without reading or writing real config files | operator, system | `internal/cli/onboarding_test.go` | Unblocks OpenClaw residue startup banner binding. |
| 7 / 7.E | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes | gateway, system | `internal/channels/bluebubbles/bot_test.go` | Unblocks BlueBubbles iMessage session-context prompt guidance. |
| 7 / 7.E | Yuanbao protocol envelope + markdown fixtures | Gormes parses Yuanbao websocket/protobuf-style envelopes and Markdown message fragments into gateway-neutral events using fixture data only | gateway, system | `internal/channels/yuanbao/proto_test.go` | Unblocks Yuanbao media/sticker attachment normalization, Yuanbao gateway runtime + toolset registration. |
<!-- PROGRESS:END -->
