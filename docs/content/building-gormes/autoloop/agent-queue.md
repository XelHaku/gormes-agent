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
## 1. BlueBubbles iMessage bubble formatting parity

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

## 2. WhatsApp outbound pairing gate + raw peer mapping

- Phase: 2 / 2.B.4
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Contract: WhatsApp outbound sends are gated by pairing state and map normalized gateway chat IDs back to bridge/native raw peers
- Trust class: gateway, operator
- Ready when: Bridge/native runtime selection, bot identity, self-chat suppression, and inbound normalization are already fixture-locked.
- Not ready when: The slice adds reconnect loops, process supervision, or native transport startup instead of only freezing send gating and raw peer mapping.
- Degraded mode: Gateway status reports unpaired or unresolved WhatsApp targets before attempting a transport send.
- Fixture: `internal/channels/whatsapp/send_contract_test.go`
- Write scope: `internal/channels/whatsapp/`, `internal/gateway/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/channels/whatsapp ./internal/gateway -count=1`
- Done signal: WhatsApp send fixtures prove unpaired targets are blocked and paired raw peers preserve DM/group plus reply metadata.
- Acceptance: Unpaired outbound targets return a visible degraded result and do not call the send transport., Bridge and native raw DM/group peer IDs are reconstructed from the normalized gateway chat/session handle., Reply metadata from the originating inbound event is preserved in the outbound send request.
- Source refs: ../hermes-agent/gateway/platforms/whatsapp.py, ../hermes-agent/gateway/platforms/base.py, internal/channels/whatsapp/, docs/content/building-gormes/architecture_plan/phase-2-gateway.md
- Unblocks: WhatsApp reconnect backoff + send retry policy
- Why now: Unblocks WhatsApp reconnect backoff + send retry policy.

## 3. Durable job backpressure + timeout audit

- Phase: 2 / 2.E.3
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Gormes durable jobs expose GBrain-style max-waiting backpressure, wall-clock timeout evidence, and operator-readable queue health without importing Minions runtime code
- Trust class: operator, system
- Ready when: The SQLite-first durable subagent/job ledger is complete and records claim, progress, completion, failure, and cancellation intent.
- Not ready when: The slice ports GBrain's PGLite/Postgres queue, CLI worker loop, or shell handler instead of adding bounded queue admission and audit fields to the existing Go ledger.
- Degraded mode: Doctor/status reports queue full, timeout-at, and stale waiting counts before accepting more durable work.
- Fixture: `internal/subagent/durable_backpressure_test.go`
- Write scope: `internal/subagent/`, `internal/doctor/`, `internal/config/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/subagent ./internal/doctor ./internal/config -count=1`
- Done signal: Durable backpressure fixtures prove max-waiting rejection, timeout_at audit fields, and doctor/status queue health over the Go ledger.
- Acceptance: Submit fixtures reject or degrade when waiting durable jobs exceed a configured maxWaiting limit., Wall-clock timeout_at is recorded and surfaced separately from context cancellation., Doctor/status evidence reports waiting, claimed, timed-out, and backpressure-denied counts.
- Source refs: ../gbrain/src/core/minions/backpressure-audit.ts, ../gbrain/src/core/minions/queue.ts, ../gbrain/docs/guides/queue-operations-runbook.md, docs/content/upstream-gbrain/architecture.md, internal/subagent/durable_ledger.go
- Unblocks: Durable worker supervisor status seam
- Why now: Unblocks Durable worker supervisor status seam.

## 4. TUI mouse tracking config + slash toggle

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Contract: Mouse/wheel tracking is config-backed, runtime-toggleable, and emits terminal enable/disable state without restarting the TUI
- Trust class: operator, system
- Ready when: Bubble Tea shell and config loading exist locally; no remote TUI gateway transport is required for this slice.
- Not ready when: The slice starts SSE/JSON-RPC remote TUI work, rewrites layout rendering, or relies on a real terminal in unit tests.
- Degraded mode: TUI status reports mouse tracking unavailable or disabled instead of leaving terminal mouse mode stale.
- Fixture: `internal/tui/mouse_tracking_test.go`
- Write scope: `internal/tui/`, `internal/config/`, `cmd/gormes/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tui ./internal/config ./cmd/gormes -count=1`
- Done signal: TUI mouse fixtures prove config defaulting, slash parsing, runtime toggling, and terminal mode enable/disable output.
- Acceptance: Config supports a persistent TUI mouse tracking boolean with default-on compatibility., A `/mouse` or equivalent command parses on/off/toggle and updates runtime state deterministically., Terminal mouse enable and disable sequences are emitted only when state changes, including explicit disable on alt-screen entry when tracking is off.
- Source refs: ../hermes-agent/tui_gateway/server.py@6407b3d5, ../hermes-agent/ui-tui/src/app/slash/commands/core.ts@6407b3d5, ../hermes-agent/ui-tui/packages/hermes-ink/src/ink/ink.tsx@6407b3d5, ../hermes-agent/ui-tui/src/app/useConfigSync.ts@6407b3d5, internal/tui/, internal/config/config.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

<!-- PROGRESS:END -->
