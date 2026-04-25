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
## 1. Slack gateway.Channel adapter shim

- Phase: 2 / 2.B.3
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Contract: Slack Socket Mode can run under the shared gateway.Manager through a narrow gateway.Channel shim without replacing the existing Slack client or reply fixtures
- Trust class: gateway, operator
- Ready when: Slack CommandRegistry parser wiring is validated and internal/slack.Bot still owns Socket Mode acking, thread routing, placeholders, and coalesced replies.
- Not ready when: The slice changes config loading, doctor output, cmd/gormes gateway registration, or live Slack OAuth/token discovery instead of only adding the Channel shim and fake-client fixtures.
- Degraded mode: If the shim cannot start or send, gateway status reports Slack channel startup/send degradation while the standalone Slack bot tests remain unchanged.
- Fixture: `internal/slack/channel_shim_test.go`
- Write scope: `internal/slack/`, `internal/gateway/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/slack -run 'TestSlackChannel\|TestBot_MessageCommandsUseSharedGatewayRegistry' -count=1`, `go test ./internal/slack ./internal/gateway -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Slack Channel shim tests prove gateway.Manager can consume Slack ingress/send through gateway.Channel while legacy Socket Mode adapter tests remain green.
- Acceptance: A Slack channel wrapper implements gateway.Channel Name, Run, and Send against the existing internal/slack Client seam., Run translates Slack Event values into gateway.InboundEvent with platform=slack, chat_id=channel_id, optional thread_id, user_id, message_id/timestamp, and normalized text kind from gateway.ParseInboundText., Send posts plain text to the correct Slack channel/thread and returns the Slack timestamp as msgID without opening a real Slack connection in tests., Existing internal/slack bot/coalescing tests stay green, proving the shim did not rewrite the current standalone behavior.
- Source refs: internal/gateway/channel.go, internal/gateway/manager.go, internal/slack/bot.go, internal/slack/client.go, internal/slack/bot_test.go
- Unblocks: Slack config + cmd/gormes gateway registration
- Why now: Unblocks Slack config + cmd/gormes gateway registration.

## 2. Drain-timeout resume_pending recovery

- Phase: 2 / 2.F.3
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Gateway restart/shutdown drain timeouts preserve resumable session identity and inject a reason-aware resume note on the next turn without overriding hard-stuck or suspended state
- Trust class: gateway, operator
- Ready when: Graceful restart drain + managed shutdown and Active-turn follow-up queue + late-arrival drain policy are complete on main., The slice can be tested with a fake manager/session read model and a fake clock; no live Telegram, Discord, Slack, or provider process is required.
- Not ready when: The slice changes pairing approval, gateway status CLI rendering, token locks, or service-manager restart behavior instead of only freezing drain-timeout resume metadata.
- Degraded mode: Gateway status reports resume_pending, drain_timeout, and non-resumable stuck/suspended evidence instead of silently dropping in-flight session context.
- Fixture: `internal/gateway/resume_pending_test.go`
- Write scope: `internal/gateway/`, `internal/session/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/gateway -run 'Test.*ResumePending\|Test.*Drain' -count=1`, `go test ./internal/gateway ./internal/session -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Resume-pending fixtures prove drain-timeout sessions retain identity, inject exactly one restart note, and lose to hard non-resumable states.
- Acceptance: A drain timeout marks only sessions whose agent turn is still running at timeout as resume_pending with session_id, source, drain reason, and timestamp., The next EventSubmit for the same session prepends one reason-aware resume note and clears resume_pending after the handoff is accepted., Hard suspended, cancelled, or stuck-loop state takes precedence over resume_pending and emits explicit non-resumable evidence., Late-arrival queued follow-ups keep FIFO ordering and do not create a second resume note.
- Source refs: ../hermes-agent/gateway/run.py@d635e2df, ../hermes-agent/gateway/platforms/base.py@d635e2df, internal/gateway/manager.go, internal/gateway/manager_test.go, internal/gateway/resume_continuation_test.go, internal/session/directory.go
- Unblocks: Gateway /restart command + takeover markers
- Why now: Unblocks Gateway /restart command + takeover markers.

## 3. `gormes gateway status` read-only command

- Phase: 2 / 2.F.3
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: A read-only `gormes gateway status` command renders configured channels, pairing state, and runtime lifecycle from stores without starting transports or agent sessions
- Trust class: operator
- Ready when: Pairing read-model schema + atomic persistence, Pairing approval + rate-limit semantics, Unauthorized DM pairing response contract, and Channel lifecycle writers into status model are complete on main., No `cmd/gormes/gateway_status.go` or `internal/gateway/statusview.go` exists on main yet, so this slice must start with failing command/render tests.
- Not ready when: The slice starts gateway transports, opens the memory store, performs provider calls, changes pairing approval policy, or implements PID/process validation in the same change.
- Degraded mode: The command reports no-channel, missing/corrupt pairing state, missing runtime state, and failed channel lifecycle evidence without opening Telegram, Discord, Slack, memory, or provider clients.
- Fixture: `cmd/gormes/gateway_status_test.go`
- Write scope: `cmd/gormes/`, `internal/gateway/`, `internal/config/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./cmd/gormes -run TestGatewayStatusCommand -count=1`, `go test ./internal/gateway -run TestRenderStatusSummary -count=1`, `go test ./cmd/gormes ./internal/gateway ./internal/config -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Gateway status command and pure renderer tests prove configured channel, pairing, and runtime status can be inspected without starting transports.
- Acceptance: A pure renderer over configured channels, PairingStatus, and RuntimeStatus emits deterministic channel ordering and stable paired/unpaired/lifecycle text., `gormes gateway status` succeeds with no configured channels by returning a visible no-channel status instead of the runtime command's startup error., Configured Telegram/Discord/Slack rows can render running, stopped, failed, paired, pending, approved, and degraded pairing evidence from fake stores., Focused command tests prove status execution does not construct real channel clients, session maps, memory stores, providers, or gateway.Manager.
- Source refs: ../hermes-agent/gateway/status.py@d635e2df, cmd/gormes/gateway.go, internal/gateway/status.go, internal/gateway/pairing_store.go, internal/config/config.go, internal/gateway/manager_status_test.go
- Unblocks: Runtime status JSON + PID/process validation, Gateway /restart command + takeover markers
- Why now: Unblocks Runtime status JSON + PID/process validation, Gateway /restart command + takeover markers.

## 4. BlueBubbles iMessage bubble formatting parity

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
