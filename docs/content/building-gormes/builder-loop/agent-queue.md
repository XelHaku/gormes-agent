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
## 1. Watchdog checkpoint coalescing

- Phase: 1 / 1.C
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P1`
- Contract: When a worker stalls, the watchdog produces at most one dirty-worktree checkpoint commit per stall window — successive stuck ticks amend or no-op instead of stacking new commits
- Trust class: operator, system
- Ready when: Watchdog dirty-worktree checkpointing is in place (commit ff96a5d94) and emits a record_run_health event we can key off of., Tests can use a fake clock and an in-memory git repo or a synthetic commit-recorder seam — no live system clock or systemd is required.
- Not ready when: The slice changes the watchdog stall threshold, the dead-process detection, or the planner cadence., The slice silently drops the dirty-worktree checkpoint when a stall is real — the first tick of every distinct stall window must still produce a single observable checkpoint.
- Degraded mode: Watchdog status reports checkpoint_coalesce_active, checkpoint_coalesce_window_seconds, and the existing dirty-worktree checkpoint commit ID instead of emitting a fresh commit on every tick.
- Fixture: `internal/builderloop/watchdog_coalesce_test.go`
- Write scope: `internal/builderloop/run.go`, `internal/builderloop/watchdog_coalesce.go`, `internal/builderloop/watchdog_coalesce_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/builderloop -run TestWatchdogCoalesce -count=1`, `go test ./internal/builderloop -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: watchdog_coalesce_test fixtures prove first-tick-commits, later-ticks-noop-or-amend, fresh-window-allowed, and a coalesce_decision field in record_run_health.
- Acceptance: Within one stall window (default 10 min, configurable), only the first watchdog tick produces a new commit; later ticks no-op or amend so `git log --oneline` shows at most one checkpoint per window., When the worker resumes (commits or exits cleanly) and a fresh stall begins later, the next stall window is allowed a new checkpoint commit., The coalesce decision is logged as record_run_health with a coalesce_decision field (`first\|amend\|noop`) so post-mortems can verify the behavior., No test in this slice runs systemd, sleeps wall-clock, or mutates the live .codex worktree.
- Source refs: internal/builderloop/run.go, docs/superpowers/specs/2026-04-25-builder-owned-planner-cycle-design.md
- Unblocks: Watchdog dead-process vs slow-progress separation
- Why now: Unblocks Watchdog dead-process vs slow-progress separation.

## 2. PR-intake idle backoff

- Phase: 1 / 1.C
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P1`
- Contract: When pr_intake list returns zero PRs N consecutive times, the next poll backs off to a configurable idle interval (default 5 min) instead of running on every loop cycle; the first non-empty list resets the backoff to baseline cadence
- Trust class: system
- Ready when: pr_intake already emits pr_intake_started and pr_intake_completed ledger events, and pr_intake_completed carries listed=N evidence., A fake clock and a stub gh-list backend make backoff state observable without live network calls.
- Not ready when: The slice changes how PRs are merged, classified, or evicted — only the schedule of intake runs is in scope., The slice removes pr_intake calls entirely or makes them event-driven (gh webhook); event-driven is a separate row.
- Degraded mode: pr_intake_started/completed events carry an idle_backoff field with the active interval and the consecutive-empty count so dashboards can distinguish "intake was suppressed" from "intake ran and found nothing".
- Fixture: `internal/builderloop/pr_intake_backoff_test.go`
- Write scope: `internal/builderloop/pr_intake.go`, `internal/builderloop/pr_intake_backoff.go`, `internal/builderloop/pr_intake_backoff_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/builderloop -run TestPRIntakeBackoff -count=1`, `go test ./internal/builderloop -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: pr_intake backoff fixtures prove suppression after consecutive empties, reset on first non-empty result, and idle_backoff evidence in pr_intake_completed events.
- Acceptance: After N (default 3) consecutive listed=0 results, the next pr_intake poll is suppressed until the idle interval (default 5 min) elapses., A non-zero listed result on any subsequent poll resets the consecutive-empty counter and restores baseline cadence on the next cycle., pr_intake_completed events carry idle_backoff={active: bool, interval_seconds: int, consecutive_empty: int} so the suppression is observable., No test in this slice talks to GitHub, runs gh, or sleeps wall-clock; tests inject a fake list backend and a controllable clock.
- Source refs: internal/builderloop/pr_intake.go
- Unblocks: Builder-loop self-improvement vs user-feature ratio metric
- Why now: Unblocks Builder-loop self-improvement vs user-feature ratio metric.

## 3. Watchdog dead-process vs slow-progress separation

- Phase: 1 / 1.C
- Owner: `orchestrator`
- Size: `medium`
- Status: `planned`
- Priority: `P1`
- Contract: The watchdog distinguishes a dead worker (no PID, exited, or PID gone from os.findprocess) from a slow worker (PID alive but no commits) and applies independently configurable thresholds, with the dead-process check firing in <2 minutes
- Trust class: operator, system
- Ready when: Existing watchdog timer (commit f96a5d94) emits stall events at a single threshold; this slice carves the threshold into two independent ones., Watchdog checkpoint coalescing is fixture-ready or validated, so the dead-process tick does not amplify the commit storm.
- Not ready when: The slice changes how worker output is rejected or how dirty worktrees are committed — only worker liveness detection is in scope., The slice introduces process-group signal sending or container-aware death detection (those belong to a separate sandboxing row).
- Degraded mode: Watchdog status reports worker_state ∈ {alive_progressing, alive_silent, dead, unknown} and the threshold each one tripped; record_run_health carries the worker_state and which threshold (dead_after_seconds, silent_after_seconds) fired.
- Fixture: `internal/builderloop/watchdog_state_test.go`
- Write scope: `internal/builderloop/run.go`, `internal/builderloop/watchdog_state.go`, `internal/builderloop/watchdog_state_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/builderloop -run TestWatchdogState -count=1`, `go test ./internal/builderloop -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: watchdog_state fixtures prove dead-fast, silent-slow, recovery, and worker_state evidence on record_run_health are all reachable with synthetic PIDs and a fake clock.
- Acceptance: Watcher with worker PID=0 or worker_pid_gone reports worker_state=dead within dead_after_seconds (default 90s)., Watcher with PID alive but zero commits in silent_after_seconds (default 600s) reports worker_state=alive_silent — independently of the dead threshold., A worker that recovers (commits) before dead_after_seconds resets both timers and reports worker_state=alive_progressing., record_run_health carries worker_state and the threshold that fired; no test in this slice runs a real subprocess or signal.
- Source refs: internal/builderloop/run.go, internal/builderloop/run_health_test.go
- Unblocks: Builder-loop self-improvement vs user-feature ratio metric
- Why now: Unblocks Builder-loop self-improvement vs user-feature ratio metric.

## 4. Builder-loop self-improvement vs user-feature ratio metric

- Phase: 1 / 1.C
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: record_run_health carries a self_improvement vs user_feature ship ratio over a configurable window, classifying each shipped row by which subphase prefix it landed under, so post-mortems can detect when the loop is mostly working on itself
- Trust class: system
- Ready when: record_run_health is the canonical health signal already (commit 2653a7b6 etc.) and runs.jsonl carries shipped row evidence., A subphase classifier mapping (e.g., 1.C, 5.* operator-tools, etc. → self_improvement; 4.*, 6.*, 7.* → user_feature) can live in a small in-package table for now.
- Not ready when: The slice changes ship-detection or ledger-write semantics — only adds a derived counter., The slice depends on a yet-unbuilt classifier service or external store.
- Degraded mode: When the ship_ratio cannot be computed (insufficient history, classification ambiguous), record_run_health carries ship_ratio=null and reports ship_ratio_evidence with the reason instead of fabricating zero.
- Fixture: `internal/builderloop/ship_ratio_test.go`
- Write scope: `internal/builderloop/ship_ratio.go`, `internal/builderloop/ship_ratio_test.go`, `internal/builderloop/health_writer.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/builderloop -run TestShipRatio -count=1`, `go test ./internal/builderloop -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: ship_ratio fixtures prove self_improvement vs user_feature classification, unclassified bucketing, null evidence on insufficient history, and ratio computed against synthetic ship events.
- Acceptance: Over a configurable window (default last 24h), record_run_health carries ship_ratio={self_improvement_count, user_feature_count, ratio, window_seconds}., A subphase prefix table (initially: 1.C/5.N/5.O = self_improvement; 4.*/6.*/7.* = user_feature; everything else = unclassified) determines the bucket per shipped row., Unclassified rows are visible as ship_ratio.unclassified_count, never silently bucketed., No test in this slice reads runs.jsonl from disk; tests inject synthetic ship events.
- Source refs: internal/builderloop/run.go, internal/builderloop/health_writer.go, internal/progress/health.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 5. Azure Foundry probe — path sniffing

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Azure Foundry endpoint URLs are classified into anthropic_messages, openai_chat_completions, or unknown solely by URL path/host inspection, with no HTTP, no credential reads, and no config writes
- Trust class: operator, system
- Ready when: Azure OpenAI query/default_query transport contract and Azure Anthropic Messages endpoint contract are validated; this slice classifies URLs into one of those two known transports., Pure-function table-driven tests with synthetic URLs are sufficient — no httptest server is required.
- Not ready when: The slice opens HTTP connections, performs a /models probe, reads AZURE_FOUNDRY_BASE_URL or AZURE_FOUNDRY_API_KEY, or mutates config., The slice introduces detection of any third transport family (Bedrock, Vertex, etc.).
- Degraded mode: Probe status reports azure_path_sniff_unknown when no path heuristic matches, and azure_path_sniff_evidence with detected scheme/host/path otherwise.
- Fixture: `internal/hermes/azure_foundry_path_sniff_test.go`
- Write scope: `internal/hermes/azure_foundry_path_sniff.go`, `internal/hermes/azure_foundry_path_sniff_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run TestAzureFoundryPathSniff -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: azure_foundry_path_sniff fixtures prove anthropic-path, openai-deployment-path, and unknown-host classifications are deterministic and side-effect-free.
- Acceptance: URLs whose path contains /anthropic/ classify as anthropic_messages with the matched-token evidence preserved., URLs whose path contains /openai/deployments/.../chat/completions classify as openai_chat_completions with deployment-name evidence preserved., Other URLs classify as unknown without panicking and surface the host/path that was inspected., No test in this slice creates an httptest.Server or reads OS env.
- Source refs: ../hermes-agent/hermes_cli/azure_detect.py@731e1ef8, ../hermes-agent/tests/hermes_cli/test_azure_detect.py@731e1ef8, internal/hermes/azure_openai_transport_test.go, internal/hermes/azure_anthropic_transport_test.go
- Unblocks: Azure Foundry probe — /models classification + Anthropic fallback
- Why now: Unblocks Azure Foundry probe — /models classification + Anthropic fallback.

## 6. Azure Foundry probe — /models classification + Anthropic fallback

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Given a fake /models response (or its absence), classify an Azure Foundry deployment as openai_chat_completions, anthropic_messages, or manual-required, with explicit probe evidence and zero credential or config writes
- Trust class: operator, system
- Ready when: Azure Foundry probe — path sniffing is fixture-ready or validated, so this slice consumes a typed sniff result instead of duplicating URL inspection., httptest.Server-driven fakes are acceptable; no live Azure call is permitted.
- Not ready when: The slice persists deployment lists, mutates AZURE_FOUNDRY_* env, or runs interactive setup., The slice changes existing Azure OpenAI or Azure Anthropic request-builder behavior.
- Degraded mode: Probe status reports azure_models_probe_failed, azure_anthropic_probe_failed, or azure_detect_manual_required when classification cannot be made; manual api_mode entry remains available.
- Fixture: `internal/hermes/azure_foundry_models_probe_test.go`
- Write scope: `internal/hermes/azure_foundry_models_probe.go`, `internal/hermes/azure_foundry_models_probe_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run TestAzureFoundryModelsProbe -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: azure_foundry_models_probe fixtures prove OpenAI-shape detection, Anthropic-shape fallback, and manual-required all return typed evidence with no credential/config side effects.
- Acceptance: A fake /models OpenAI-shaped response selects openai_chat_completions and records advisory model IDs without persisting them., A failed /models call followed by a fake Anthropic Messages-shaped error selects anthropic_messages with explicit probe evidence., Total probe failure returns manual-required evidence rather than a fatal error, and does not hide manual api_mode selection., No test in this slice writes to AZURE_FOUNDRY_API_KEY, real config files, or a live Azure host.
- Source refs: ../hermes-agent/hermes_cli/azure_detect.py@731e1ef8, ../hermes-agent/tests/hermes_cli/test_azure_detect.py@731e1ef8, internal/hermes/http_client.go, internal/hermes/azure_openai_transport_test.go, internal/hermes/azure_anthropic_transport_test.go
- Unblocks: Azure Foundry transport probe read model
- Why now: Unblocks Azure Foundry transport probe read model.

## 7. Provider rate guard — x-ratelimit header classification

- Phase: 4 / 4.H
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Pure functions parse x-ratelimit-* headers and classify a 429 as genuine_quota, upstream_capacity, or insufficient_evidence with redacted reset windows, no sleeps, and no shared breaker writes
- Trust class: system
- Ready when: Provider-side resilience and classified provider-error taxonomy are validated, so this slice only adds a pure header-parsing classifier on top., Tests use synthetic headers and a fake clock; no live Nous Portal or wall-clock sleep is required.
- Not ready when: The slice changes retry timing, provider routing, or model fallback policy., The slice writes process-global breaker state in unit tests or sleeps to simulate reset windows.
- Degraded mode: Provider status reports rate_guard_classified as one of {genuine_quota, upstream_capacity, insufficient_evidence}, plus reset-window evidence when present, instead of silently tripping a global breaker.
- Fixture: `internal/hermes/provider_rate_guard_classification_test.go`
- Write scope: `internal/hermes/provider_rate_guard.go`, `internal/hermes/provider_rate_guard_classification_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run TestProviderRateGuardClassification -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: rate_guard classification fixtures prove genuine_quota, upstream_capacity, and insufficient_evidence outcomes are all reachable from synthetic headers with no side effects.
- Acceptance: Headers with remaining=0 and reset window >=60s classify as genuine_quota with redacted reset evidence., 429 responses with healthy x-ratelimit buckets classify as upstream_capacity and do not trip cross-session breaker state., Missing or unparseable headers classify as insufficient_evidence and surface which header(s) failed parsing., No test in this slice opens a network connection or calls time.Sleep.
- Source refs: ../hermes-agent/agent/nous_rate_guard.py@192e7eb2, ../hermes-agent/tests/agent/test_nous_rate_guard.py@192e7eb2, internal/hermes/errors.go
- Unblocks: Provider rate guard — degraded-state + last-known-good evidence
- Why now: Unblocks Provider rate guard — degraded-state + last-known-good evidence.

## 8. Provider rate guard — degraded-state + last-known-good evidence

- Phase: 4 / 4.H
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Last-known-good bucket state distinguishes bare 429s; degraded modes (rate_guard_unavailable, budget_header_missing) are reported as visible provider status evidence without retry timing changes or shared mutable breaker state
- Trust class: system
- Ready when: Provider rate guard — x-ratelimit header classification is fixture-ready or validated, so this slice composes a typed classification result with last-known-good state.
- Not ready when: The slice writes a process-global or cross-session breaker., The slice changes retry policy or treats every 429 as account-level quota exhaustion.
- Degraded mode: Provider status reports last_known_good=present\|absent, plus rate_guard_unavailable or budget_header_missing when classification is unsafe; no retry amplification.
- Fixture: `internal/hermes/provider_rate_guard_degraded_test.go`
- Write scope: `internal/hermes/provider_rate_guard_degraded.go`, `internal/hermes/provider_rate_guard_degraded_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run TestProviderRateGuardDegraded -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: rate_guard degraded fixtures prove last-known-good composition, rate_guard_unavailable, and budget_header_missing evidence are all reachable without shared state or retry timing changes.
- Acceptance: A bare 429 with last-known-good=exhausted classifies as genuine_quota; with last-known-good=healthy classifies as upstream_capacity., A bare 429 with no last-known-good record classifies as insufficient_evidence and reports rate_guard_unavailable., Malformed budget headers degrade visibly with budget_header_missing evidence and do not amplify retries or block unrelated models., No test in this slice writes process-global state or sleeps.
- Source refs: ../hermes-agent/agent/nous_rate_guard.py@192e7eb2, ../hermes-agent/tests/agent/test_nous_rate_guard.py@192e7eb2, ../hermes-agent/run_agent.py@192e7eb2, internal/hermes/errors.go, internal/hermes/client.go
- Unblocks: Provider rate guard + budget telemetry
- Why now: Unblocks Provider rate guard + budget telemetry.

## 9. Native TUI terminal-selection divergence contract

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Contract: Gormes documents and fixture-locks a terminal-native selection model for the Bubble Tea TUI, with no advertised custom copy hotkey until a Go-native implementation exists
- Trust class: operator
- Ready when: Gormes native TUI has mouse tracking toggles but no custom selection/copy implementation., Upstream Hermes edc78e25 and 31d7f195 fixed SSH copy shortcuts, rendered-space preservation, indentation, and bounds clamping in the Node/Ink TUI.
- Not ready when: The slice ports Hermes Ink, adds Node/npm dependencies, calls OSC clipboard APIs, changes remote TUI transport, or implements custom selection copying in the same change.
- Degraded mode: TUI status/help reports terminal-native selection behavior and does not advertise SSH/local copy shortcuts, rendered-space copy, or clipboard semantics that Gormes cannot honor.
- Fixture: `internal/tui/selection_copy_test.go`
- Write scope: `internal/tui/`, `cmd/gormes/`, `docs/content/building-gormes/architecture_plan/progress.json`, `docs/content/building-gormes/architecture_plan/phase-5-final-purge.md`
- Test commands: `go test ./internal/tui ./cmd/gormes -run 'Test.*Selection\|Test.*Copy\|Test.*TUI.*Help' -count=1`, `go test ./internal/tui ./cmd/gormes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Native TUI fixtures and docs prove Gormes advertises terminal-native selection honestly and remains independent from Hermes Ink/Node clipboard behavior.
- Acceptance: TUI help/status fixtures say selection uses the operator's terminal selection until a native Gormes copy mode is explicitly implemented., No fake copy hotkey, SSH copy shortcut, or clipboard command is advertised in help/status output., Docs state the divergence from Hermes' custom Ink selection stack and point future parity work at a separate Go-native fixture., The solution remains native Go/Bubble Tea with no Hermes Ink bundle, Node, OSC clipboard dependency, or npm runtime requirement.
- Source refs: ../hermes-agent/ui-tui/packages/hermes-ink/src/ink/selection.ts@edc78e25, ../hermes-agent/ui-tui/packages/hermes-ink/src/ink/selection.test.ts@edc78e25, ../hermes-agent/ui-tui/src/lib/platform.ts@edc78e25, ../hermes-agent/ui-tui/src/app/useInputHandlers.ts@edc78e25, ../hermes-agent/ui-tui/packages/hermes-ink/src/ink/selection.ts@31d7f195, internal/tui/, docs/content/upstream-hermes/user-guide/tui.md
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 10. BlueBubbles iMessage bubble formatting parity

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
