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

Shared unattended-loop facts live in [Builder Loop Handoff](../builder-loop-handoff/):
the main entrypoint, orchestrator plan, candidate source, generated docs,
tests, and candidate policy. Keep those control-plane facts in
`meta.builder_loop`, and keep row-specific execution facts in `progress.json`.

<!-- PROGRESS:START kind=agent-queue -->
## 1. Gateway fresh-final stream coalescer policy

- Phase: 2 / 2.B.5
- Owner: `gateway`
- Size: `medium`
- Status: `planned`
- Priority: `P0`
- Contract: Gateway streaming finalization can replace an old editable preview with a fresh final message when the preview age is at or above a configured threshold, while preserving the legacy edit-in-place path when the threshold is zero, the preview is too young, the channel cannot send a fresh final, or the fresh send fails
- Trust class: operator, gateway, system
- Ready when: Gateway stream consumer for agent-event fan-out and Non-editable gateway progress/commentary send fallback are complete on main., internal/gateway/coalesce.go currently owns placeholder send, edit cadence, and final flush; fresh-final should be implemented there with an injected clock in tests, not in Telegram-specific code., The worker can use fake gateway.Channel values only; no Telegram SDK, network call, or real provider stream is needed for this row.
- Not ready when: The slice changes provider streaming, kernel.RenderFrame phases, non-editable channel send fallback, or Slack/Discord channel implementations., The slice adds a Telegram config field or Telegram delete API call; those are in the dependent Telegram row., The slice sends duplicate final messages when fresh send fails instead of falling back to the existing edit finalization path.
- Degraded mode: Until this lands, Telegram and other editable channels always finalize by editing the original preview, so long-running Telegram replies keep the first-token visible timestamp.
- Fixture: `internal/gateway/coalesce_fresh_final_test.go::TestCoalescerFreshFinal`
- Write scope: `internal/gateway/channel.go`, `internal/gateway/coalesce.go`, `internal/gateway/coalesce_fresh_final_test.go`, `internal/gateway/manager.go`, `internal/gateway/manager_test.go`, `internal/gateway/fake_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/gateway -run 'TestCoalescerFreshFinal' -count=1`, `go test ./internal/gateway -run 'TestManager_Outbound\|TestGatewayStreamConsumer\|TestCoalescer' -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Gateway coalescer fixtures prove old-preview fresh finalization, delete best-effort behavior, young-preview edit-in-place behavior, and fresh-send failure fallback without touching Telegram SDK code.
- Acceptance: internal/gateway/channel.go adds a small optional MessageDeleter interface with DeleteMessage(ctx, chatID, msgID string) error; existing channels need not implement it., ManagerConfig carries FreshFinalAfter time.Duration and the coalescer tracks the placeholder creation time via an injected now function so tests do not sleep., TestCoalescerFreshFinal_DisabledThresholdEditsInPlace proves FreshFinalAfter=0 keeps the existing EditMessageFinal path., TestCoalescerFreshFinal_YoungPreviewEditsInPlace proves a final flush before the threshold edits the preview and does not call Channel.Send., TestCoalescerFreshFinal_OldPreviewSendsFreshAndDeletesOld proves a final flush at or beyond the threshold calls Send for the final text, skips EditMessageFinal for the old preview, adopts the new message id, and best-effort calls DeleteMessage on the old id., TestCoalescerFreshFinal_DeleteUnsupportedStillSucceeds proves a channel without MessageDeleter still delivers the fresh final., TestCoalescerFreshFinal_FreshSendFailureFallsBackToEdit proves a failed fresh Send falls back to EditMessageFinal and returns success when the edit succeeds., Existing manager outbound tests for non-editable channels and editable streaming remain green.
- Source refs: ../hermes-agent/gateway/stream_consumer.py@b16f9d43:GatewayStreamConsumer._should_send_fresh_final, ../hermes-agent/gateway/stream_consumer.py@b16f9d43:GatewayStreamConsumer._try_fresh_final, ../hermes-agent/tests/gateway/test_stream_consumer_fresh_final.py@b16f9d43:TestFreshFinalForLongLivedPreviews, internal/gateway/coalesce.go, internal/gateway/channel.go, internal/gateway/manager.go
- Unblocks: Telegram fresh-final delete and config exposure
- Why now: P0 handoff; needs contract proof before closeout.

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
