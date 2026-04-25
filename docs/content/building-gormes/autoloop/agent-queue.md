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

## 2. Aux compression single-prompt threshold reconciliation

- Phase: 4 / 4.B
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P1`
- Contract: Auxiliary compression budgeting follows Hermes 5006b220 by treating the summarizer request as raw messages plus one small user instruction, not as a system-prompt-plus-tool-schema memory-flush request
- Trust class: operator, system
- Ready when: Compression token-budget trigger + summary sizing and Aux compression provider-aware context cap are validated on main., The row can be proven with pure ContextCompressorBudget fixtures and synthetic tool descriptors; no summarizer, provider call, or kernel history mutation is required.
- Not ready when: The slice ports history pruning, manual compression feedback, context references, provider routing, or Goncho memory extraction instead of only reconciling auxiliary threshold math/status.
- Degraded mode: Context status reports whether a threshold used legacy_headroom, single_prompt_aux, provider_cap, or unavailable evidence so operators can see why compression starts early or late.
- Fixture: `internal/hermes/context_compressor_single_prompt_test.go`
- Write scope: `internal/hermes/context_compressor_budget.go`, `internal/hermes/context_compressor_headroom_test.go`, `internal/hermes/context_compressor_provider_cap_test.go`, `internal/hermes/context_compressor_single_prompt_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run 'TestContextCompressor(SinglePrompt\|ProviderCap\|Headroom\|Budget)' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Single-prompt compression fixtures prove auxiliary-bound thresholds use aux_context directly, preserve provider-cap evidence, and no longer reserve tool-schema or flush-memory headroom.
- Acceptance: A 200000-token main window with a 128000-token auxiliary context and 50 synthetic tool schemas sets threshold_tokens to the auxiliary context, not aux minus tool schema plus 12000 fixed headroom., The budget status no longer reports request_headroom_tokens or headroom_clamped for compression-only auxiliary calls, but preserves provider_cap evidence from the resolver., A compatibility fixture documents the removed legacy headroom behavior and fails if flush_memories assumptions are reintroduced into compression budgeting., Existing provider-cap and compressor-budget tests remain green after the threshold/status expectation update.
- Source refs: ../hermes-agent/run_agent.py@5006b220, ../hermes-agent/tests/run_agent/test_compression_feasibility.py@5006b220, ../hermes-agent/hermes_cli/config.py@5006b220, internal/hermes/context_compressor_budget.go, internal/hermes/context_compressor_headroom_test.go, internal/hermes/context_compressor_provider_cap_test.go
- Unblocks: Tool-result pruning + protected head/tail summary, Manual compression feedback + context references
- Why now: Unblocks Tool-result pruning + protected head/tail summary, Manual compression feedback + context references.

## 3. Codex Responses temperature guard after flush removal

- Phase: 4 / 4.H
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Codex Responses payload conversion keeps omitting temperature while removing obsolete flush_memories fixture names, source references, and docs language after Hermes 5006b220 deleted memory flush
- Trust class: system
- Ready when: Unsupported temperature retry + Codex flush guard and Generic unsupported-parameter retry + max_tokens guard are validated on main., The change can be proven by renaming/splitting fake payload fixtures; no live Codex, OAuth, or memory provider is required.
- Not ready when: The slice changes Codex OAuth, provider retries, memory extraction, or Responses streaming behavior instead of only removing flush-specific assumptions from temperature fixtures and docs.
- Degraded mode: Provider status and docs explain no-temperature behavior as a Codex Responses transport rule, not as a memory-flush fallback path.
- Fixture: `internal/hermes/codex_responses_temperature_test.go`
- Write scope: `internal/hermes/unsupported_temperature_retry_test.go`, `internal/hermes/codex_responses_adapter_test.go`, `internal/hermes/codex_responses_temperature_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run 'Test.*Temperature\|TestBuildCodexResponsesPayload' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Codex Responses temperature fixtures and generated progress docs explain the no-temperature rule without any flush_memories donor references.
- Acceptance: Codex Responses request-builder fixtures still prove temperature is never emitted even when ChatRequest.Temperature is set., Test names, progress source_refs, and generated docs no longer cite deleted upstream tests/run_agent/test_flush_memories_codex.py as a current donor., Unsupported-temperature retry fixtures still cover the Hermes 5006b220 auxiliary task names without auxiliary.flush_memories., Progress validation and internal/hermes tests pass without changing runtime retry semantics.
- Source refs: ../hermes-agent/run_agent.py@5006b220, ../hermes-agent/tests/agent/test_unsupported_temperature_retry.py@5006b220, ../hermes-agent/hermes_cli/config.py@5006b220, internal/hermes/unsupported_temperature_retry_test.go, internal/hermes/codex_responses_adapter.go, docs/content/building-gormes/architecture_plan/progress.json
- Unblocks: Codex OAuth state + stale-token relogin
- Why now: Unblocks Codex OAuth state + stale-token relogin.

## 4. Top-level oneshot flag and model/provider resolver

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Gormes accepts a top-level `-z/--oneshot` prompt plus `--model`, `--provider`, GORMES_INFERENCE_MODEL, and GORMES_INFERENCE_PROVIDER overrides with Hermes-compatible ambiguity errors before any agent execution starts
- Trust class: operator, system
- Ready when: The Cobra root command can parse top-level flags without starting the TUI, api_server health check, or gateway transports., The first slice owns only argument/env/config resolution and does not need to execute a model turn.
- Not ready when: The slice opens the TUI, starts the kernel, sends provider requests, changes config file schema broadly, or implements stdout capture and tool approval behavior in the same change.
- Degraded mode: CLI parse/status output returns exit code 2 and an actionable stderr error when --provider is set without an explicit model, rather than silently pairing it with a stale configured model.
- Fixture: `cmd/gormes/oneshot_flags_test.go`
- Write scope: `cmd/gormes/main.go`, `cmd/gormes/oneshot_flags_test.go`, `internal/config/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./cmd/gormes -run TestOneshotFlags -count=1`, `go test ./cmd/gormes ./internal/config -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Oneshot flag fixtures prove top-level parsing, provider-without-model exit 2, env fallback, and no TUI/gateway startup during resolution.
- Acceptance: `gormes -z 'hi' --model fixture-model` parses without invoking runTUI or api_server health checks., `gormes -z 'hi' --provider openrouter` exits 2 unless --model or GORMES_INFERENCE_MODEL is present., Environment model/provider overrides are read only for oneshot and do not mutate persisted config., The resolver records whether the model came from flag, env, or config and whether provider auto-detection is required for the execution slice.
- Source refs: ../hermes-agent/hermes_cli/main.py@5006b220, ../hermes-agent/hermes_cli/oneshot.py@5006b220, cmd/gormes/main.go, internal/config/config.go, internal/hermes/model_routing.go
- Unblocks: Oneshot stdout-only kernel execution
- Why now: Unblocks Oneshot stdout-only kernel execution.

## 5. Session expiry finalization without memory flush

- Phase: 2 / 2.F.3
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Gateway session expiry finalizes hooks, cleanup, and cache eviction once per expired session without launching a model-driven memory flush agent
- Trust class: gateway, system
- Ready when: Gateway session metadata can persist one-shot finalization evidence and a fake expiry scanner can run without live channel transports., The slice owns only expiry finalization state and hook/cleanup evidence; scheduled reset policy and live idle scanning can remain future rows.
- Not ready when: The slice adds a model-driven memory flush, calls Goncho/Honcho extractors, changes pairing/restart status, or starts real Telegram/Discord/Slack adapters in tests.
- Degraded mode: Gateway status reports expiry_finalize_pending, expiry_finalize_failed, expiry_finalize_gave_up, and legacy memory_flushed migration evidence instead of retrying hidden memory writes forever.
- Fixture: `internal/gateway/session_expiry_finalize_test.go`
- Write scope: `internal/gateway/`, `internal/session/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/gateway -run TestSessionExpiryFinalize -count=1`, `go test ./internal/session ./internal/gateway -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Expiry-finalization fixtures prove legacy migration, one-shot hook/cleanup persistence, retry/give-up evidence, and zero memory-flush task launches.
- Acceptance: A legacy session record with memory_flushed=true migrates to expiry_finalized=true when read, but new writes use only expiry_finalized., Expired-session fixtures invoke finalize hooks and cached-agent cleanup exactly once, then persist expiry_finalized evidence across reload., Transient finalize failures retry up to three fake-clock attempts and then mark gave-up evidence so the gateway does not spin forever., Resume/switch-session fixtures prove no memory flush task is started before switching sessions.
- Source refs: ../hermes-agent/gateway/run.py@5006b220, ../hermes-agent/gateway/session.py@5006b220, ../hermes-agent/tests/gateway/test_session_boundary_hooks.py@5006b220, ../hermes-agent/tests/gateway/test_resume_command.py@5006b220, internal/gateway/manager.go, internal/session/directory.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 6. Update service restart active polling

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Update and service-management flows verify restarted gateway services by polling active status for at least RestartSec plus slack instead of racing the systemd cooldown window
- Trust class: operator, system
- Ready when: This slice owns a pure service-manager helper with a fake runner and no command wiring; later Gateway/platform/webhook/cron CLI rows can consume it., Installer source-backed update flow remains documented as Gormes' current public update path.
- Not ready when: The slice invokes live systemctl/sc.exe in unit tests, changes installer layout policy, edits gateway restart command semantics, or combines restart polling with unrelated setup/auth command ports.
- Degraded mode: CLI update/status output reports service restart timeout, retry, missing service manager, or unsupported platform evidence instead of claiming a restarted gateway that immediately crashed.
- Fixture: `internal/cli/service_restart_test.go`
- Write scope: `internal/cli/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run 'TestServiceRestart(ActivePolling\|RestartSec)' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/autoloop progress validate`
- Done signal: Service restart fixtures prove RestartSec parsing, cooldown-aware active-status polling, delayed-active success, timeout evidence, retry boundaries, and no live service-manager dependency.
- Acceptance: A pure helper parses RestartUSec values such as 30s, 100ms, 1min 30s, infinity, blank, and malformed output into a bounded restart delay or default., After graceful exit-75, the active-status poll timeout is max(10s, parsed_restart_sec+10s), and fake 500ms polling continues through the cooldown window., Hard restart flows can reuse the same helper with the original 10s timeout when no RestartSec evidence exists., Fixtures distinguish active-after-delay, active-after-RestartSec, timeout, missing service manager, malformed RestartUSec, and crashed-after-restart outcomes with operator-readable evidence., Tests run without live systemd, Windows service control, root privileges, or network access.
- Source refs: ../hermes-agent/hermes_cli/main.py@5006b220, internal/cli/, scripts/install.sh, scripts/install.ps1, docs/content/building-gormes/architecture_plan/progress.json
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

<!-- PROGRESS:END -->
