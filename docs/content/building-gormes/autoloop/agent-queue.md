---
title: "Agent Queue"
weight: 20
aliases:
  - /building-gormes/agent-queue/
---

# Agent Queue

This page is generated from the canonical progress file:
`docs/content/building-gormes/architecture_plan/progress.json`.

It lists unblocked, non-umbrella contract rows that are ready for a focused
autonomous implementation attempt. Each card carries the execution owner,
slice size, contract, trust class, degraded-mode requirement, fixture target,
write scope, test commands, done signal, acceptance checks, and source
references.

Shared unattended-loop facts live in [Autoloop Handoff](../autoloop-handoff/):
the main entrypoint, orchestrator plan, candidate source, generated docs,
tests, and candidate policy. Keep those control-plane facts in
`meta.autoloop`, and keep row-specific execution facts in `progress.json`.

<!-- PROGRESS:START kind=agent-queue -->
## 1. Native TUI bundle independence check

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Gormes TUI startup and install/update status stay Go-native and never depend on Hermes' Node/Ink dist bundle freshness checks
- Trust class: operator, system
- Ready when: Bubble Tea shell and local TUI startup seams exist in Gormes., Hermes ee0728c6 adds a Python/Node `_tui_build_needed` regression around missing `packages/hermes-ink/dist/ink-bundle.js` even when `dist/entry.js` exists.
- Not ready when: The slice ports Hermes `_tui_build_needed`, adds Node/npm/package-lock/node_modules checks to Gormes startup, or edits remote TUI SSE transport behavior.
- Degraded mode: TUI and doctor/status output report native Go TUI availability instead of asking operators to run npm install/build or repair packages/hermes-ink/dist/ink-bundle.js.
- Fixture: `cmd/gormes/tui_bundle_independence_test.go`
- Write scope: `cmd/gormes/`, `internal/tui/`, `internal/cli/`, `docs/content/building-gormes/architecture_plan/progress.json`, `docs/content/building-gormes/architecture_plan/phase-5-final-purge.md`, `www.gormes.ai/internal/site/content.go`
- Test commands: `go test ./cmd/gormes ./internal/tui ./internal/cli -run 'Test.*TUI.*Bundle\|Test.*Native.*TUI\|Test.*Doctor.*TUI' -count=1`, `go test ./cmd/gormes ./internal/tui ./internal/cli -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Focused fixtures prove Gormes TUI startup/status remains independent from Hermes Node/Ink bundle rebuild checks and public install copy still promises no runtime npm dependency.
- Acceptance: A focused fixture runs the Gormes TUI startup/build-preflight path from a temp working directory that contains Hermes-style `ui-tui/dist/entry.js` and `node_modules/ink` but lacks `packages/hermes-ink/dist/ink-bundle.js`, and proves no npm/node/package-manager command is invoked., Offline doctor or startup status describes the TUI as native Go/Bubble Tea and does not require `HERMES_TUI_DIR`, `package-lock.json`, `node_modules`, or `ink-bundle.js`., Landing/docs install messaging continues to state no runtime npm/Node dependency and does not inherit Hermes first-launch TUI rebuild instructions.
- Source refs: ../hermes-agent/hermes_cli/main.py@ee0728c6, ../hermes-agent/tests/hermes_cli/test_tui_npm_install.py@ee0728c6, cmd/gormes/main.go, internal/tui/, www.gormes.ai/internal/site/content.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 2. BlueBubbles iMessage bubble formatting parity

- Phase: 7 / 7.E
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes
- Trust class: gateway, system
- Ready when: The first-pass BlueBubbles adapter already owns Send, markdown stripping, cached GUID resolution, and home-channel fallback in internal/channels/bluebubbles.
- Not ready when: The slice attempts to add live BlueBubbles HTTP/webhook registration, attachment download, reactions, typing indicators, or edit-message support.
- Degraded mode: BlueBubbles remains a usable first-pass adapter, but long replies may still arrive as one stripped text send until paragraph splitting and suffix-free chunking are fixture-locked.
- Fixture: `internal/channels/bluebubbles/bot_test.go`
- Write scope: `internal/channels/bluebubbles/bot.go`, `internal/channels/bluebubbles/bot_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/channels/bluebubbles -count=1`
- Done signal: BlueBubbles adapter tests prove paragraph-to-bubble sends, suffix-free chunking, and no edit/placeholder capability.
- Acceptance: Send splits blank-line-separated paragraphs into separate SendText calls while preserving existing chat GUID resolution and home-channel fallback., Long paragraph chunks omit `(n/m)` pagination suffixes and concatenate back to the stripped original text., Bot does not implement gateway.MessageEditor or gateway.PlaceholderCapable, preserving non-editable iMessage semantics.
- Source refs: ../hermes-agent/gateway/platforms/bluebubbles.py@f731c2c2, ../hermes-agent/tests/gateway/test_bluebubbles.py@f731c2c2, internal/channels/bluebubbles/bot.go, internal/gateway/channel.go
- Unblocks: BlueBubbles iMessage session-context prompt guidance
- Why now: Unblocks BlueBubbles iMessage session-context prompt guidance.

<!-- PROGRESS:END -->
