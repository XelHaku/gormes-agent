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
- Contract: Pure helper internal/builderloop/watchdog_coalesce.go exposes DecideCheckpoint(now time.Time, st CheckpointState, cfg CoalesceConfig) (Decision, CheckpointState) where Decision is one of {first, amend, noop} and CheckpointState records {LastCheckpointAt, LastSubject, WindowID}; given the current state and a coalesce window (default 600s), the helper returns first on the first dirty tick of a window, amend while still inside the window with the same WindowID, and noop when no fresh state is needed. No git invocation, no shell-script change, no live filesystem mutation in this slice.
- Trust class: operator, system
- Ready when: Watchdog dirty-worktree checkpointing is in place (commit ff96a5d94) and emits a record_run_health event we can key off of., Tests can use a fake clock and an in-memory git repo or a synthetic commit-recorder seam — no live system clock or systemd is required.
- Not ready when: The slice changes the watchdog stall threshold, the dead-process detection, or the planner cadence., The slice silently drops the dirty-worktree checkpoint when a stall is real — the first tick of every distinct stall window must still produce a single observable checkpoint.
- Degraded mode: Watchdog status reports checkpoint_coalesce_active, checkpoint_coalesce_window_seconds, and the existing dirty-worktree checkpoint commit ID instead of emitting a fresh commit on every tick.
- Fixture: `internal/builderloop/watchdog_coalesce_test.go`
- Write scope: `internal/builderloop/watchdog_coalesce.go`, `internal/builderloop/watchdog_coalesce_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/builderloop -run TestDecideCheckpoint -count=1`, `go test ./internal/builderloop -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/builderloop/watchdog_coalesce_test.go fixtures prove first/amend/first-after-rotation/noop decisions across a fake clock with no live git or systemd.
- Acceptance: TestDecideCheckpoint_FirstTickInFreshWindowReturnsFirst seeded with empty state returns Decision=first and a CheckpointState with WindowID assigned and LastCheckpointAt=now., TestDecideCheckpoint_LaterTickInsideWindowReturnsAmend with prior state inside windowSeconds returns Decision=amend and the same WindowID., TestDecideCheckpoint_LaterTickPastWindowReturnsFirst rotates WindowID and LastCheckpointAt=now when now-prior > windowSeconds., TestDecideCheckpoint_NoopWhenNotDirty (helper exposes DecideCheckpointDirty=false) returns Decision=noop and unchanged state., Helper is a pure function with no time.Now, no git invocation, no os.* calls — caller passes both the clock and the dirty flag.
- Source refs: internal/builderloop/run.go:CheckpointDirtyWorktree,lastCommitIsBuilderLoopCheckpoint,isBuilderLoopCheckpointSubject, scripts/orchestrator/watchdog.sh:checkpoint_dirty, docs/superpowers/specs/2026-04-25-builder-owned-planner-cycle-design.md
- Unblocks: Watchdog dead-process vs slow-progress separation
- Why now: Unblocks Watchdog dead-process vs slow-progress separation.

## 2. PR-intake idle backoff

- Phase: 1 / 1.C
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P1`
- Contract: Pure helper internal/builderloop/pr_intake_backoff.go exposes type BackoffState struct{ConsecutiveEmpty int; SuppressUntil time.Time; Threshold int; Baseline, Idle time.Duration} and methods (s BackoffState) ShouldPoll(now time.Time) bool and (s BackoffState) RecordResult(now time.Time, listed int) BackoffState. After Threshold consecutive listed=0 results, ShouldPoll returns false until now >= SuppressUntil; any listed > 0 result resets ConsecutiveEmpty and clears SuppressUntil. No GitHub API calls, no goroutines, no live clock in this slice.
- Trust class: system
- Ready when: pr_intake already emits pr_intake_started and pr_intake_completed ledger events, and pr_intake_completed carries listed=N evidence., A fake clock and a stub gh-list backend make backoff state observable without live network calls.
- Not ready when: The slice changes how PRs are merged, classified, or evicted — only the schedule of intake runs is in scope., The slice removes pr_intake calls entirely or makes them event-driven (gh webhook); event-driven is a separate row.
- Degraded mode: pr_intake_started/completed events carry an idle_backoff field with the active interval and the consecutive-empty count so dashboards can distinguish "intake was suppressed" from "intake ran and found nothing".
- Fixture: `internal/builderloop/pr_intake_backoff_test.go`
- Write scope: `internal/builderloop/pr_intake_backoff.go`, `internal/builderloop/pr_intake_backoff_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/builderloop -run TestBackoffState -count=1`, `go test ./internal/builderloop -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/builderloop/pr_intake_backoff_test.go fixtures prove threshold/suppress/reset/reactivation behavior under a fake clock with no GitHub API and no goroutines.
- Acceptance: TestBackoffState_ShouldPollWhenNotSuppressed (fresh state, now=t0) returns true., TestBackoffState_RecordEmptyBelowThreshold accumulates ConsecutiveEmpty without suppressing., TestBackoffState_RecordEmptyAtThreshold sets SuppressUntil=now+Idle and ShouldPoll=false until SuppressUntil elapses., TestBackoffState_RecordNonEmptyResetsState restores baseline (ConsecutiveEmpty=0, SuppressUntil zero)., TestBackoffState_ShouldPollAfterSuppressionElapsed flips back to true when now >= SuppressUntil even if ConsecutiveEmpty is still at threshold., Helper is a pure function — caller injects the clock and the listed result count.
- Source refs: internal/builderloop/pr_intake.go, internal/builderloop/pr_intake_test.go
- Unblocks: Builder-loop self-improvement vs user-feature ratio metric
- Why now: Unblocks Builder-loop self-improvement vs user-feature ratio metric.

## 3. Watchdog dead-process vs slow-progress separation

- Phase: 1 / 1.C
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P1`
- Contract: Pure helper internal/builderloop/watchdog_state.go exposes type WorkerVitals struct{PID int; LastCommitAt time.Time; PIDIsLive bool} and Diagnose(now time.Time, v WorkerVitals, deadAfter, slowAfter time.Duration) Verdict where Verdict is one of {healthy, slow, dead}. dead fires when v.PIDIsLive is false and (PID == 0 OR now-LastCommitAt >= deadAfter). slow fires when v.PIDIsLive is true and now-LastCommitAt >= slowAfter. healthy otherwise. Caller injects PIDIsLive (it should wrap os.FindProcess + Signal(0)) so the helper can be tested without forking processes.
- Trust class: operator, system
- Ready when: Existing watchdog timer (commit f96a5d94) emits stall events at a single threshold; this slice carves the threshold into two independent ones., Watchdog checkpoint coalescing is fixture-ready or validated, so the dead-process tick does not amplify the commit storm.
- Not ready when: The slice changes how worker output is rejected or how dirty worktrees are committed — only worker liveness detection is in scope., The slice introduces process-group signal sending or container-aware death detection (those belong to a separate sandboxing row).
- Degraded mode: Watchdog status reports worker_state ∈ {alive_progressing, alive_silent, dead, unknown} and the threshold each one tripped; record_run_health carries the worker_state and which threshold (dead_after_seconds, silent_after_seconds) fired.
- Fixture: `internal/builderloop/watchdog_state_test.go`
- Write scope: `internal/builderloop/watchdog_state.go`, `internal/builderloop/watchdog_state_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/builderloop -run TestDiagnose -count=1`, `go test ./internal/builderloop -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/builderloop/watchdog_state_test.go fixtures prove healthy/slow/dead verdicts including the dead-vs-slow precedence rule with no os.FindProcess or signal calls.
- Acceptance: TestDiagnose_HealthyWhenRecentCommitAndAlive returns Verdict=healthy., TestDiagnose_SlowWhenAliveButOverSlowThreshold returns Verdict=slow., TestDiagnose_DeadWhenPIDNotLiveAndOverDeadThreshold returns Verdict=dead., TestDiagnose_DeadWhenPIDIsZero returns Verdict=dead even with v.PIDIsLive=true (zero PID treated as missing process)., TestDiagnose_SlowDoesNotMaskDead when both thresholds elapsed and PID not live, dead wins., Helper is pure — caller injects the clock and the PIDIsLive result.
- Source refs: internal/builderloop/run.go, internal/builderloop/run_health_test.go
- Unblocks: Builder-loop self-improvement vs user-feature ratio metric
- Why now: Unblocks Builder-loop self-improvement vs user-feature ratio metric.

## 4. Builder-loop self-improvement vs user-feature ratio metric

- Phase: 1 / 1.C
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Pure helper internal/builderloop/ship_ratio.go exposes ClassifySubphase(subphaseID string) RowKind (kinds: self_improvement, user_feature, unclassified) and ComputeShipRatio(events []ShippedRowEvent, window time.Duration, now time.Time) ShipRatio where ShipRatio counts each kind in the [now-window, now] band. The classifier table maps 1.C/control-plane/* and 5.O operator-tooling rows to self_improvement, and 4.*/6.*/7.* to user_feature, with everything else as unclassified. No file I/O, no record_run_health emission.
- Trust class: system
- Ready when: record_run_health is the canonical health signal already (commit 2653a7b6 etc.) and runs.jsonl carries shipped row evidence., A subphase classifier mapping (e.g., 1.C, 5.* operator-tools, etc. → self_improvement; 4.*, 6.*, 7.* → user_feature) can live in a small in-package table for now.
- Not ready when: The slice changes ship-detection or ledger-write semantics — only adds a derived counter., The slice depends on a yet-unbuilt classifier service or external store.
- Degraded mode: When the ship_ratio cannot be computed (insufficient history, classification ambiguous), record_run_health carries ship_ratio=null and reports ship_ratio_evidence with the reason instead of fabricating zero.
- Fixture: `internal/builderloop/ship_ratio_test.go`
- Write scope: `internal/builderloop/ship_ratio.go`, `internal/builderloop/ship_ratio_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/builderloop -run 'TestClassifySubphase\|TestComputeShipRatio' -count=1`, `go test ./internal/builderloop -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/builderloop/ship_ratio_test.go fixtures prove classifier coverage of self_improvement/user_feature/unclassified plus windowed aggregation with no record_run_health writes.
- Acceptance: TestClassifySubphase_KnownSelfImprovement covers '1.C', 'control-plane/backend', '5.O/CLI log snapshot reader' returning RowKindSelfImprovement., TestClassifySubphase_KnownUserFeature covers '4.A', '4.H', '6.A', '7.B' returning RowKindUserFeature., TestClassifySubphase_UnknownReturnsUnclassified (e.g., '99.X') returns RowKindUnclassified., TestComputeShipRatio_FiltersByWindow excludes events older than now-window., TestComputeShipRatio_CountsAllKinds returns SelfImprovement, UserFeature, Unclassified counts and a Total., Helper is a pure function — caller injects the events slice and the clock.
- Source refs: internal/builderloop/run.go, internal/builderloop/health_writer.go, internal/progress/health.go, internal/builderloop/ledger.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 5. Azure Foundry probe — path sniffing

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Pure helper internal/hermes/azure_foundry_path_sniff.go exposes type AzureTransport string (anthropic_messages, openai_chat_completions, unknown) and ClassifyAzurePath(rawURL string) (AzureTransport, string) where the second return is a human reason. Returns anthropic_messages when (case-insensitive) the URL path ends in /anthropic or contains /anthropic/, mirroring upstream _looks_like_anthropic_path (hermes_cli/azure_detect.py:114-124). Returns unknown for every other URL. No HTTP, no credential reads, no config writes.
- Trust class: operator, system
- Ready when: Azure OpenAI query/default_query transport contract and Azure Anthropic Messages endpoint contract are validated; this slice classifies URLs into one of those two known transports., Pure-function table-driven tests with synthetic URLs are sufficient — no httptest server is required.
- Not ready when: The slice opens HTTP connections, performs a /models probe, reads AZURE_FOUNDRY_BASE_URL or AZURE_FOUNDRY_API_KEY, or mutates config., The slice introduces detection of any third transport family (Bedrock, Vertex, etc.).
- Degraded mode: Probe status reports azure_path_sniff_unknown when no path heuristic matches, and azure_path_sniff_evidence with detected scheme/host/path otherwise.
- Fixture: `internal/hermes/azure_foundry_path_sniff_test.go`
- Write scope: `internal/hermes/azure_foundry_path_sniff.go`, `internal/hermes/azure_foundry_path_sniff_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run TestClassifyAzurePath -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/hermes/azure_foundry_path_sniff_test.go fixtures prove anthropic-path classification across suffix, mid-path, and case variants without HTTP.
- Acceptance: TestClassifyAzurePath_AnthropicSuffix recognises https://foo.openai.azure.com/openai/deployments/x/anthropic and …/anthropic/., TestClassifyAzurePath_AnthropicMidPath recognises https://foo/openai/anthropic/v1/messages., TestClassifyAzurePath_CaseInsensitive recognises /AnthrOPic and /ANTHROPIC., TestClassifyAzurePath_OpenAIDefaultReturnsUnknown for https://foo.openai.azure.com/openai/v1/chat/completions., TestClassifyAzurePath_MalformedReturnsUnknown for empty string and ::garbage::., Helper does not read os.Env, does not open files, does not call http.Client.
- Source refs: ../hermes-agent/hermes_cli/azure_detect.py:_looks_like_anthropic_path:114, ../hermes-agent/hermes_cli/azure_detect.py:_strip_trailing_v1:109, ../hermes-agent/tests/hermes_cli/test_azure_detect.py, internal/hermes/azure_openai_transport_test.go, internal/hermes/azure_anthropic_transport_test.go
- Unblocks: Azure Foundry probe — /models classification + Anthropic fallback
- Why now: Unblocks Azure Foundry probe — /models classification + Anthropic fallback.

## 6. Azure Foundry probe — /models classification + Anthropic fallback

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Pure helper internal/hermes/azure_foundry_models_probe.go exposes type AzureProbeResult struct{Transport AzureTransport; Models []string; Reason string; Evidence []string} and ProbeAzureFoundry(ctx, client *http.Client, base, apiKey string) (AzureProbeResult, error). Probes <base>/models first; on 200 + OpenAI-shaped JSON returns openai_chat_completions with the model IDs. On non-200 or shape mismatch, probes <base>/v1/messages with a zero-token Anthropic Messages payload and classifies anthropic_messages on any 4xx that mentions 'messages' or 'model'. Otherwise returns Transport=unknown and Reason=manual_required. Tests use httptest.Server fakes only — no live Azure call, no credential storage, no config writes.
- Trust class: operator, system
- Ready when: Azure Foundry probe — path sniffing is fixture-ready or validated, so this slice consumes a typed sniff result instead of duplicating URL inspection., httptest.Server-driven fakes are acceptable; no live Azure call is permitted.
- Not ready when: The slice persists deployment lists, mutates AZURE_FOUNDRY_* env, or runs interactive setup., The slice changes existing Azure OpenAI or Azure Anthropic request-builder behavior.
- Degraded mode: Probe status reports azure_models_probe_failed, azure_anthropic_probe_failed, or azure_detect_manual_required when classification cannot be made; manual api_mode entry remains available.
- Fixture: `internal/hermes/azure_foundry_models_probe_test.go`
- Write scope: `internal/hermes/azure_foundry_models_probe.go`, `internal/hermes/azure_foundry_models_probe_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run TestProbeAzureFoundry -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/hermes/azure_foundry_models_probe_test.go fixtures prove openai/anthropic/unknown classifications via httptest.Server fakes with no live Azure traffic.
- Acceptance: TestProbeAzureFoundry_OpenAIModels200WithList classifies as openai_chat_completions with the model list captured in Evidence., TestProbeAzureFoundry_OpenAIModels200EmptyList still classifies as openai_chat_completions with empty Models and a 'shape OK, empty list' Reason., TestProbeAzureFoundry_AnthropicMessages400ValidShape classifies as anthropic_messages when /models 404s but /v1/messages returns 400 with body containing 'messages'., TestProbeAzureFoundry_BothFailReturnUnknown returns Transport=unknown with Reason=manual_required when both probes fail., TestProbeAzureFoundry_RespectsContextCancel cancels mid-probe and returns ctx.Err., Helper does not write to disk, does not mutate config, does not retry beyond the two named probes.
- Source refs: ../hermes-agent/hermes_cli/azure_detect.py:_probe_openai_models:143, ../hermes-agent/hermes_cli/azure_detect.py:_probe_anthropic_messages:175, ../hermes-agent/hermes_cli/azure_detect.py:detect:221, ../hermes-agent/tests/hermes_cli/test_azure_detect.py, internal/hermes/http_client.go, internal/hermes/azure_openai_transport_test.go, internal/hermes/azure_anthropic_transport_test.go
- Unblocks: Azure Foundry transport probe read model
- Why now: Unblocks Azure Foundry transport probe read model.

## 7. Provider rate guard — x-ratelimit header classification

- Phase: 4 / 4.H
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Pure helper internal/hermes/provider_rate_guard.go exposes type RateLimitClass string (genuine_quota, upstream_capacity, insufficient_evidence), type Bucket struct{Tag string; Remaining int; ResetSeconds float64; HasReset bool} and Classify429(headers http.Header, now time.Time) (RateLimitClass, []Bucket, ResetEvidence). Parses x-ratelimit-remaining-{1h,1m,requests,tokens} and x-ratelimit-reset-{...} into typed Buckets; classifies genuine_quota when any bucket reports Remaining<=0 with a future reset; upstream_capacity when remaining looks normal but the 429 still fires; insufficient_evidence when no x-ratelimit-* headers were present. No sleeps, no shared state writes, no http.Get.
- Trust class: system
- Ready when: Provider-side resilience and classified provider-error taxonomy are validated, so this slice only adds a pure header-parsing classifier on top., Tests use synthetic headers and a fake clock; no live Nous Portal or wall-clock sleep is required.
- Not ready when: The slice changes retry timing, provider routing, or model fallback policy., The slice writes process-global breaker state in unit tests or sleeps to simulate reset windows.
- Degraded mode: Provider status reports rate_guard_classified as one of {genuine_quota, upstream_capacity, insufficient_evidence}, plus reset-window evidence when present, instead of silently tripping a global breaker.
- Fixture: `internal/hermes/provider_rate_guard_classification_test.go`
- Write scope: `internal/hermes/provider_rate_guard.go`, `internal/hermes/provider_rate_guard_classification_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run TestClassify429 -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/hermes/provider_rate_guard_classification_test.go fixtures prove genuine_quota / upstream_capacity / insufficient_evidence classification with redacted reset windows under a fake clock.
- Acceptance: TestClassify429_GenuineQuotaWhenAnyBucketExhausted (Remaining-1h=0, reset=120s) returns RateLimitClassGenuineQuota., TestClassify429_UpstreamCapacityWhenAllBucketsHaveRemaining returns RateLimitClassUpstreamCapacity., TestClassify429_InsufficientEvidenceWhenNoRateHeaders returns RateLimitClassInsufficientEvidence and an empty Buckets slice., TestClassify429_ParsesPerBucketTags returns one Bucket per recognised tag (1h, 1m, requests, tokens)., TestClassify429_RedactsResetWindowsAtSecondGranularity returns ResetEvidence with seconds-precision values only., Helper does not access any global breaker state; caller threads the result.
- Source refs: ../hermes-agent/agent/nous_rate_guard.py:_parse_buckets_from_headers:246, ../hermes-agent/agent/nous_rate_guard.py:is_genuine_nous_rate_limit:191, ../hermes-agent/agent/nous_rate_guard.py:_parse_reset_seconds:38, ../hermes-agent/tests/agent/test_nous_rate_guard.py, internal/hermes/errors.go
- Unblocks: Provider rate guard — degraded-state + last-known-good evidence
- Why now: Unblocks Provider rate guard — degraded-state + last-known-good evidence.

## 8. Provider rate guard — degraded-state + last-known-good evidence

- Phase: 4 / 4.H
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Pure helper internal/hermes/provider_rate_guard_degraded.go composes the Classify429 result with persistent last-known-good state. Exposes type GuardState struct{LastKnownClass RateLimitClass; LastKnownAt time.Time; DegradeMode DegradeMode; ExhaustedBuckets []string} and ApplyClassification(state GuardState, now time.Time, class RateLimitClass, buckets []Bucket, ev ResetEvidence) GuardState. DegradeMode is one of {none, rate_guard_unavailable, budget_header_missing}. When buckets is empty (insufficient_evidence) the helper preserves LastKnownClass and bumps DegradeMode to rate_guard_unavailable for that turn. No retry timing changes, no shared mutable breaker.
- Trust class: system
- Ready when: Provider rate guard — x-ratelimit header classification is fixture-ready or validated, so this slice composes a typed classification result with last-known-good state.
- Not ready when: The slice writes a process-global or cross-session breaker., The slice changes retry policy or treats every 429 as account-level quota exhaustion.
- Degraded mode: Provider status reports last_known_good=present\|absent, plus rate_guard_unavailable or budget_header_missing when classification is unsafe; no retry amplification.
- Fixture: `internal/hermes/provider_rate_guard_degraded_test.go`
- Write scope: `internal/hermes/provider_rate_guard_degraded.go`, `internal/hermes/provider_rate_guard_degraded_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run TestApplyClassification -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/hermes/provider_rate_guard_degraded_test.go fixtures prove last-known-good preservation and degraded-mode transitions across all three RateLimitClass values.
- Acceptance: TestApplyClassification_RecordsExhaustedBuckets stores tag names from buckets where Remaining<=0., TestApplyClassification_PreservesLastKnownGoodOnInsufficientEvidence keeps prior LastKnownClass and sets DegradeMode=rate_guard_unavailable., TestApplyClassification_ClearsExhaustionOnUpstreamCapacity zeros ExhaustedBuckets and sets DegradeMode=none., TestApplyClassification_BudgetHeaderMissing detects a 429 with neither x-ratelimit-* nor a budget header and sets DegradeMode=budget_header_missing., TestApplyClassification_DoesNotMutateInputBuckets (defensive copy semantics)., Helper is pure and never sleeps, retries, or mutates a global breaker.
- Source refs: ../hermes-agent/agent/nous_rate_guard.py:record_nous_rate_limit:70, ../hermes-agent/agent/nous_rate_guard.py:nous_rate_limit_remaining:138, ../hermes-agent/agent/nous_rate_guard.py:clear_nous_rate_limit:162, ../hermes-agent/tests/agent/test_nous_rate_guard.py, internal/hermes/errors.go, internal/hermes/client.go
- Unblocks: Provider rate guard + budget telemetry
- Why now: Unblocks Provider rate guard + budget telemetry.

## 9. Gateway /reasoning session override command

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Gateway /reasoning command is dispatched through internal/gateway/commands.go to ParseReasoningCommand(args []string) (ReasoningCommand, error) and ApplyReasoningCommand(state SessionReasoningState, cmd ReasoningCommand, now time.Time) (SessionReasoningState, ReasoningReply). ReasoningCommand carries {Action: show\|set\|reset, Effort: high\|low\|medium\|empty, Global bool}. Default scope is the calling session; --global persists to config; reset clears only the calling session; reset --global is rejected with the upstream warning class. /new and /reset drop only the triggering session's reasoning override. No per-turn-propagation refactor here — that work is already complete in 4.D.
- Trust class: operator, gateway
- Ready when: Per-turn reasoning effort propagation is validated so a session override can affect the next native turn through typed request metadata., The existing gateway command registry can add one recognized command and focused fake-channel fixtures without porting the whole Hermes command tree.
- Not ready when: The slice implements model switching, provider setup, interactive pickers, or broad command-registry parity in the same change., The slice persists reasoning changes without --global or clears other sessions' reasoning overrides on /new.
- Degraded mode: Gateway replies report session-only override, global-save failure fallback, invalid effort, unsupported reset --global, and reasoning-display toggle state instead of treating /reasoning as ordinary prompt text.
- Fixture: `internal/gateway/reasoning_command_test.go`
- Write scope: `internal/gateway/commands.go`, `internal/gateway/reasoning_command.go`, `internal/gateway/reasoning_command_test.go`, `internal/gateway/manager.go`, `internal/config/config.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/gateway -run 'Test.*Reasoning.*Command\|Test.*Reset.*Reasoning' -count=1`, `go test ./internal/gateway ./internal/config -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/gateway/reasoning_command_test.go fixtures prove parser correctness, session-only default, --global persistence, reset rejection of --global, and session isolation under fake config-persist.
- Acceptance: TestParseReasoningCommand_ShowFormReturnsActionShow (`/reasoning`) returns ReasoningCommand{Action: show}., TestParseReasoningCommand_SetSessionScoped (`/reasoning high`) returns Action=set, Effort=high, Global=false., TestParseReasoningCommand_GlobalPersistFlag (`/reasoning low --global`) returns Action=set, Effort=low, Global=true., TestParseReasoningCommand_ResetSession (`/reasoning reset`) returns Action=reset, Global=false., TestParseReasoningCommand_RejectGlobalReset (`/reasoning reset --global`) returns an error with the upstream warning class., TestApplyReasoningCommand_ShowReportsEffectiveScope returns ReasoningReply containing effective effort, scope, and display state from injected SessionReasoningState., TestApplyReasoningCommand_SetSessionEvictsRuntimeCache mutates only the matching session, preserving sibling sessions' overrides., TestApplyReasoningCommand_GlobalPersistFallback when persistence write fails, falls back to session-only state and surfaces the failure in ReasoningReply., TestNewOrResetClearsOnlyTriggeringSession verifies session isolation across /new and /reset., Helpers do not block on disk I/O — they accept a pluggable persistConfigFn that tests stub.
- Source refs: ../hermes-agent/gateway/run.py:_parse_reasoning_command_args, ../hermes-agent/gateway/run.py:_handle_reasoning_command, ../hermes-agent/tests/gateway/test_reasoning_command.py, ../hermes-agent/tests/gateway/test_session_model_reset.py, internal/gateway/commands.go, internal/gateway/manager.go, internal/config/config.go, internal/hermes/reasoning_effort_request_test.go
- Unblocks: CLI command registry parity + active-turn busy policy
- Why now: Unblocks CLI command registry parity + active-turn busy policy.

## 10. CLI log snapshot reader

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: internal/cli/log_snapshot.go exposes type LogClass string (main, tool_audit), type LogSnapshotRoots struct{LogPath, ToolAuditPath string}, type SnapshotOpts struct{HeadBytes, TailBytes int64} (defaults 64 KiB + 16 KiB), type LogSection struct{Class LogClass; Path string; Head, Tail []byte; Missing, Truncated bool; Redacted int; Unreadable string} and type Snapshot struct{Sections []LogSection}. SnapshotLogs(roots LogSnapshotRoots, opts SnapshotOpts) (Snapshot, error) reads the two log paths (no XDG resolution inside the helper), applies head+tail truncation when files exceed HeadBytes+TailBytes, and runs RedactLine(line []byte) ([]byte, int) over each line. Redactor matches: bearer tokens (Bearer XXX), api_key=VALUE / x-api-key: VALUE, Telegram bot tokens (NN:XXXX-XXXX), Slack xoxb-/xoxp- tokens, and OpenAI sk-* keys. No network upload, no archive write, no live provider call.
- Trust class: operator, system
- Ready when: internal/config exposes LogPath() and ToolAuditLogPath() (already present at lines 791-794 and 872-875 of internal/config/config.go)., This slice is a pure local file reader; tests pass injected `LogSnapshotRoots{LogPath, ToolAuditPath}` with files written to t.TempDir() — no paste upload, support bundle archive, live provider status, or backup write behavior is exercised.
- Not ready when: The slice adds a `gateway.log`, `errors.log`, or `builder-loop.log` file constant — those don't exist yet and are tracked under follow-up rows once the gateway/builderloop start emitting separate file logs., The slice uploads to paste.rs/dpaste, creates tar/zip backups, reads `~/.hermes/logs/` as authoritative state, or changes `gormes doctor` exit codes., The slice depends on regex packages outside the standard library or pulls in a YAML/JSON support-bundle layer.
- Degraded mode: Per-class results carry MissingPrimary, RotatedFallbackUsed, BytesTruncated, LinesTruncated, RedactionsApplied, and Unreadable booleans/counters; the top-level call never returns an error for missing or unreadable files — it embeds the evidence so callers (doctor / status / backup-manifest) can render it without failing.
- Fixture: `internal/cli/log_snapshot_test.go`
- Write scope: `internal/cli/log_snapshot.go`, `internal/cli/log_snapshot_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run 'TestRedactLine\|TestSnapshotLogs' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/cli/log_snapshot_test.go fixtures prove RedactLine coverage across all five secret shapes plus SnapshotLogs Missing/Truncated/Redacted/Unreadable evidence under t.TempDir() injection.
- Acceptance: TestRedactLine_BearerToken returns redacted line and Count=1 for `Authorization: Bearer abc123`., TestRedactLine_ApiKeyEqualsValue covers `api_key=sk-prod-XYZ` and `x-api-key: sk-test-...`., TestRedactLine_TelegramBotToken redacts `12345:ABCDEFGHabcdefgh1234567890`., TestRedactLine_SlackTokens covers xoxb-…, xoxp-…, xoxs-…., TestRedactLine_OpenAIStyleKey redacts `sk-…` longer than 16 chars only (avoids false positives)., TestRedactLine_NoMatchPreservesInput returns input unchanged with Count=0., TestSnapshotLogs_MissingFileSetsMissingTrue records LogSection{Missing: true} without an error., TestSnapshotLogs_HeadAndTailTruncation produces Head and Tail with sizes equal to opts.HeadBytes and opts.TailBytes when the file is larger than their sum., TestSnapshotLogs_AppliesRedactorPerLine sums Redacted across head and tail., TestSnapshotLogs_UnreadableFileSetsUnreadableField records the io error message and continues with the other LogClass., Helpers do not call net.*, do not invoke os/exec, do not touch real XDG paths.
- Source refs: ../hermes-agent/hermes_cli/logs.py:LOG_FILES,_TS_RE,_LEVEL_RE, ../hermes-agent/hermes_cli/debug.py, ../hermes-agent/tests/hermes_cli/test_logs.py, internal/config/config.go:LogPath:792,ToolAuditLogPath:874,xdgDataHome:770, cmd/gormes/doctor.go
- Unblocks: CLI status summary over native stores, Backup manifest dry-run contract, Gateway/error log snapshot follow-up (once those files exist)
- Why now: Unblocks CLI status summary over native stores, Backup manifest dry-run contract, Gateway/error log snapshot follow-up (once those files exist).

<!-- PROGRESS:END -->
