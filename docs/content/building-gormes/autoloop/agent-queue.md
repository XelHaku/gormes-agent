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

## 2. Session expiry finalized-flag migration

- Phase: 2 / 2.F.3
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Gateway session metadata migrates legacy memory_flushed state into expiry_finalized evidence without adding new hidden memory-flush writes
- Trust class: gateway, system
- Ready when: Session metadata load/save can be exercised with temp directories and fake clocks without starting Telegram, Discord, Slack, or a live gateway manager., This slice owns only metadata migration and write-shape evidence; hook execution and retry policy live in the dependent expiry hook row.
- Not ready when: The slice calls Goncho/Honcho extractors, launches a model-driven memory flush, edits pairing/restart behavior, or implements idle scanning.
- Degraded mode: Gateway status reports migrated legacy memory_flushed evidence separately from current expiry_finalized state so operators can tell old session records from new finalization writes.
- Fixture: `internal/session/expiry_finalized_migration_test.go`
- Write scope: `internal/session/`, `internal/gateway/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/session -run TestExpiryFinalizedMigration -count=1`, `go test ./internal/session ./internal/gateway -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Session metadata fixtures prove legacy memory_flushed records migrate to expiry_finalized and new writes contain no hidden memory-flush fields.
- Acceptance: A legacy session record with memory_flushed=true loads as expiry_finalized=true and records migrated_memory_flushed evidence., New session metadata writes use expiry_finalized and never emit a memory_flushed field., Resume/switch-session fixtures prove no memory-flush task is queued or called while reading or rewriting migrated session metadata.
- Source refs: ../hermes-agent/gateway/run.py@648b8991, ../hermes-agent/gateway/session.py@648b8991, ../hermes-agent/tests/gateway/test_resume_command.py@648b8991, internal/session/directory.go, internal/gateway/manager.go
- Unblocks: Session expiry hook cleanup retry evidence
- Why now: Unblocks Session expiry hook cleanup retry evidence.

## 3. Codex Responses assistant content role types

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P1`
- Contract: Codex Responses payload conversion emits role-correct text content parts: input_text for user messages and output_text for assistant replay messages
- Trust class: system
- Ready when: Codex Responses pure conversion harness is validated on main., The slice can use pure payload-conversion fixtures and does not require Codex OAuth, live Responses requests, or ChatGPT backend credentials.
- Not ready when: The slice changes Codex OAuth, live streaming repair, tool-call argument repair, or generic OpenAI chat-completions content mapping.
- Degraded mode: Codex provider status reports codex_responses_role_content_unavailable if assistant list-content replay would still send input_text to the Responses API.
- Fixture: `internal/hermes/codex_responses_role_content_test.go`
- Write scope: `internal/hermes/codex_responses_adapter.go`, `internal/hermes/codex_responses_adapter_test.go`, `internal/hermes/codex_responses_role_content_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run 'TestBuildCodexResponsesPayload\|TestCodexResponsesRoleContent' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Codex Responses role-content fixtures prove assistant list-content replay uses output_text while user list-content remains input_text.
- Acceptance: User Message.ContentParts with text/text-like parts serialize as input_text and keep image parts as input_image., Assistant Message.ContentParts with text, input_text, or output_text parts serialize as output_text and never input_text., A user -> assistant -> user round-trip fixture proves the preflight/payload builder preserves role-correct content part types before tool-call replay., Existing temperature, stream-repair, and tool-call conversion fixtures remain green without live Codex credentials.
- Source refs: ../hermes-agent/agent/codex_responses_adapter.py@648b8991, ../hermes-agent/tests/run_agent/test_provider_parity.py@648b8991, internal/hermes/codex_responses_adapter.go, internal/hermes/codex_responses_adapter_test.go
- Unblocks: Codex OAuth state + stale-token relogin
- Why now: Unblocks Codex OAuth state + stale-token relogin.

## 4. Oneshot final-output writer boundary

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: One-shot mode runs one native Gormes kernel turn over a fake provider and writes only final assistant content plus one trailing newline to stdout
- Trust class: operator, system
- Ready when: Top-level oneshot flag and model/provider resolver is validated on main., A fake hermes.Client can stream one assistant final response through the existing kernel without live provider credentials.
- Not ready when: The slice implements interactive TUI behavior, starts gateway transports, changes tool registry defaults, or decides noninteractive dangerous-command approval policy.
- Degraded mode: CLI output reports provider/client setup failures on stderr with nonzero exit codes while banners, render frames, tool previews, logs, and session IDs never enter stdout.
- Fixture: `cmd/gormes/oneshot_output_test.go`
- Write scope: `cmd/gormes/main.go`, `cmd/gormes/oneshot_output_test.go`, `internal/kernel/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./cmd/gormes -run TestOneshotFinalOutput -count=1`, `go test ./cmd/gormes ./internal/kernel -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Oneshot output fixtures prove one fake kernel turn, stdout-only final content, stderr-only setup failures, and no TUI/gateway/provider startup.
- Acceptance: A fake-client `gormes -z 'hi' --model fixture-model` command prints exactly the final assistant content plus one trailing newline to stdout., Status frames, tool-progress frames, banners, session IDs, and logs are absent from stdout and either suppressed or routed to stderr under explicit fixtures., Provider/client setup failures return nonzero exit status with operator-readable stderr and empty stdout., The command starts no TUI, gateway transport, api_server bridge, or live provider request in the focused fixture.
- Source refs: ../hermes-agent/hermes_cli/oneshot.py@648b8991, ../hermes-agent/hermes_cli/main.py@648b8991, cmd/gormes/main.go, internal/kernel/kernel.go, internal/hermes/client.go
- Unblocks: Oneshot noninteractive safety and clarify policy
- Why now: Unblocks Oneshot noninteractive safety and clarify policy.

## 5. Service RestartSec parser helper

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Service-management helpers parse systemd RestartUSec/RestartSec evidence into a bounded restart delay without invoking live service managers
- Trust class: operator, system
- Ready when: This slice owns only a pure parser/helper with fake command output fixtures; no command wiring or real service control is needed., Installer source-backed update flow remains documented as Gormes' current public update path.
- Not ready when: The slice invokes live systemctl/sc.exe, changes installer layout policy, edits gateway restart command semantics, or implements active polling.
- Degraded mode: CLI service status reports restart_delay_defaulted, restart_delay_malformed, restart_delay_infinite, or service_manager_unavailable evidence instead of hiding parser failures.
- Fixture: `internal/cli/service_restart_parse_test.go`
- Write scope: `internal/cli/service_restart.go`, `internal/cli/service_restart_parse_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run TestServiceRestartParser -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: RestartSec parser fixtures prove bounded duration parsing, default/malformed evidence, and no live service-manager dependency.
- Acceptance: A pure helper parses RestartUSec/RestartSec values such as 30s, 100ms, 1min 30s, infinity, blank, zero, and malformed output into a bounded restart delay or documented default., Parser fixtures preserve operator-readable evidence for defaulted, malformed, missing, unsupported, and infinite restart-delay cases., Tests run without live systemd, Windows service control, root privileges, or network access.
- Source refs: ../hermes-agent/hermes_cli/main.py@648b8991, internal/cli/, scripts/install.sh, scripts/install.ps1
- Unblocks: Service restart active-status poller
- Why now: Unblocks Service restart active-status poller.

## 6. Streaming interrupt retry suppression

- Phase: 4 / 4.H
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P1`
- Contract: Kernel stream cancellation and /stop-style events abort retry loops before any fresh provider stream is opened
- Trust class: operator, system
- Ready when: Jittered reconnect backoff schedule and Kernel retry honors Retry-After hint are validated on main., A fake hermes.Client can make the first stream attempt return a retryable error while the test submits PlatformEventCancel or cancels the parent context.
- Not ready when: The slice changes provider error classification, retry timing constants, tool execution cancellation, or gateway command policy instead of only proving retry suppression after cancellation.
- Degraded mode: Render/status frames report interrupted retry suppression instead of reconnecting after the operator has cancelled a running turn.
- Fixture: `internal/kernel/stream_interrupt_retry_test.go`
- Write scope: `internal/kernel/stream_interrupt_retry_test.go`, `internal/kernel/kernel.go`, `internal/kernel/retry.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/kernel -run 'TestKernel_StreamInterruptSuppressesRetry\|TestRetryBudget_WaitRespectsContextCancel' -count=1`, `go test ./internal/kernel -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Kernel interrupt retry fixtures prove cancel-before-retry, context-cancel-during-backoff, no-cancel retry recovery, and interrupted memory-sync suppression.
- Acceptance: If PlatformEventCancel arrives during a stream attempt that then returns a retryable error, the kernel enters cancellation/finalization and does not call OpenStream again., If the parent context is cancelled before reconnect backoff completes, Wait returns immediately and no retry attempt is opened., A no-cancel control fixture still retries normally after a transient stream error so the fix does not disable Route-B recovery., Interrupted turns continue to skip memory sync instead of persisting partial assistant content.
- Source refs: ../hermes-agent/run_agent.py@7c17accb, ../hermes-agent/tests/run_agent/test_stream_interrupt_retry.py@7c17accb, internal/kernel/kernel.go, internal/kernel/retry.go, internal/kernel/reset_test.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

<!-- PROGRESS:END -->
