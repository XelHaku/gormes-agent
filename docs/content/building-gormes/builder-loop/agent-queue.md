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

## 2. Watchdog dead-process vs slow-progress separation

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

## 3. Builder-loop self-improvement vs user-feature ratio metric

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

## 4. Azure Foundry probe — path sniffing

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Pure helper internal/hermes/azure_foundry_path_sniff.go exposes type AzureTransport string with constants AzureTransportAnthropic, AzureTransportOpenAI, AzureTransportUnknown, and one function ClassifyAzurePath(rawURL string) AzureTransport. Returns AzureTransportAnthropic when (case-insensitive) the URL path ends in /anthropic or contains /anthropic/. Every other URL returns AzureTransportUnknown. The OpenAI constant is reserved for the next slice; this slice never returns it. No HTTP, no env reads, no config writes. Use net/url + strings.ToLower; reject parse errors by returning Unknown.
- Trust class: operator, system
- Ready when: internal/hermes already compiles and has no azure_foundry_path_sniff.go file yet — this row creates the file plus a sibling _test.go., No upstream gating: this is a pure URL inspector with synthetic input.
- Not ready when: The slice opens HTTP connections, performs a /models probe, reads AZURE_FOUNDRY_BASE_URL or AZURE_FOUNDRY_API_KEY, or mutates config., The slice introduces detection of any third transport family (Bedrock, Vertex, etc.).
- Degraded mode: Probe status reports azure_path_sniff_unknown when no path heuristic matches, and azure_path_sniff_evidence with detected scheme/host/path otherwise.
- Fixture: `internal/hermes/azure_foundry_path_sniff_test.go`
- Write scope: `internal/hermes/azure_foundry_path_sniff.go`, `internal/hermes/azure_foundry_path_sniff_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run TestClassifyAzurePath -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/hermes/azure_foundry_path_sniff_test.go fixtures prove anthropic-path classification across suffix, mid-path, and case variants without HTTP.
- Acceptance: TestClassifyAzurePath_AnthropicSuffix: https://x.openai.azure.com/openai/deployments/y/anthropic returns AzureTransportAnthropic., TestClassifyAzurePath_AnthropicMidPath: https://x/openai/anthropic/v1/messages returns AzureTransportAnthropic., TestClassifyAzurePath_CaseInsensitive: /AnthrOPic and /ANTHROPIC both return AzureTransportAnthropic., TestClassifyAzurePath_OpenAIDefault: https://x.openai.azure.com/openai/v1/chat/completions returns AzureTransportUnknown., TestClassifyAzurePath_MalformedReturnsUnknown: empty string and "::garbage::" return AzureTransportUnknown.
- Source refs: ../hermes-agent/hermes_cli/azure_detect.py:_looks_like_anthropic_path:114
- Unblocks: Azure Foundry probe — /models classification + Anthropic fallback
- Why now: Unblocks Azure Foundry probe — /models classification + Anthropic fallback.

## 5. Provider rate guard — x-ratelimit header classification

- Phase: 4 / 4.H
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Pure helper internal/hermes/provider_rate_guard.go exposes type RateLimitClass string (RateLimitGenuineQuota, RateLimitUpstreamCapacity, RateLimitInsufficientEvidence) and Classify429(headers http.Header) RateLimitClass. Reads x-ratelimit-remaining-{1h,1m,requests,tokens} as integers; returns RateLimitGenuineQuota if any present remaining<=0, RateLimitUpstreamCapacity if all present are >0, RateLimitInsufficientEvidence if no x-ratelimit-* headers at all. No Bucket parsing, no reset evidence, no clock, no shared state. Bucket/reset detail is the next slice.
- Trust class: system
- Ready when: internal/hermes already compiles; the row creates a new file and a sibling _test.go., No upstream gate; pure header parsing with synthetic http.Header values.
- Not ready when: The slice changes retry timing, provider routing, or model fallback policy., The slice writes process-global breaker state in unit tests or sleeps to simulate reset windows.
- Degraded mode: Provider status reports rate_guard_classified as one of {genuine_quota, upstream_capacity, insufficient_evidence}, plus reset-window evidence when present, instead of silently tripping a global breaker.
- Fixture: `internal/hermes/provider_rate_guard_classification_test.go`
- Write scope: `internal/hermes/provider_rate_guard.go`, `internal/hermes/provider_rate_guard_classification_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run TestClassify429 -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/hermes/provider_rate_guard_classification_test.go fixtures prove genuine_quota / upstream_capacity / insufficient_evidence classification with redacted reset windows under a fake clock.
- Acceptance: TestClassify429_GenuineQuotaWhenAnyBucketExhausted (remaining-1h=0) returns RateLimitGenuineQuota., TestClassify429_UpstreamCapacityWhenAllBucketsHaveRemaining returns RateLimitUpstreamCapacity., TestClassify429_InsufficientEvidenceWhenNoRateHeaders returns RateLimitInsufficientEvidence., TestClassify429_IgnoresUnknownHeaders preserves the classification when other headers are present.
- Source refs: ../hermes-agent/agent/nous_rate_guard.py:is_genuine_nous_rate_limit:191
- Unblocks: Provider rate guard — degraded-state + last-known-good evidence
- Why now: Unblocks Provider rate guard — degraded-state + last-known-good evidence.

## 6. Skills list — enabled/disabled status column + --enabled-only filter

- Phase: 5 / 5.F
- Owner: `skills`
- Size: `small`
- Status: `planned`
- Contract: internal/skills/list.go exposes type SkillStatus string ("enabled", "disabled"), extends SkillRow with a Status field and adds ListOptions{Source string; EnabledOnly bool}. ListInstalledSkills(opts ListOptions, disabled map[string]struct{}) []SkillRow returns every installed skill annotated with Status from the disabled set; when opts.EnabledOnly is true, disabled rows are filtered out. The CLI surface (gormes skills list --source <s> --enabled-only) calls this helper and prints a table with a Status column plus a summary "N enabled, M disabled" (or "K enabled shown" when --enabled-only). No platform-aware override read in this slice — disabled set comes from the active profile only, mirroring upstream do_list semantics.
- Trust class: operator, system
- Ready when: internal/skills already lists installed skills (existing list.go or equivalent) and has a typed disabled-skill set the active-profile config exposes., CLI table rendering exists for skills already (status column is an additional column).
- Not ready when: The slice plumbs a HERMES_PLATFORM-style platform override into list — upstream test_do_list_platform_env_is_ignored asserts the platform arg stays nil here., The slice rewrites do_check, do_install, or do_search behavior.
- Degraded mode: Status column makes disabled skills visible without forcing the operator to read config; --enabled-only matches the upstream "what will load" introspection question.
- Fixture: `internal/skills/list_test.go`
- Write scope: `internal/skills/list.go`, `internal/skills/list_test.go`, `internal/cli/skills_command.go`, `internal/cli/skills_command_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/skills -run 'TestListInstalledSkills' -count=1`, `go test ./internal/cli -run 'TestSkillsListCommand' -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/skills/list_test.go and internal/cli/skills_command_test.go fixtures prove the Status column, --enabled-only filter, and platform-arg guard.
- Acceptance: TestListInstalledSkills_StatusColumnPopulated annotates every row with Status="enabled" when disabled is empty., TestListInstalledSkills_DisabledRowsCarryDisabledStatus marks rows whose name is in the disabled set as Status="disabled"., TestListInstalledSkills_EnabledOnlyFilter hides disabled rows when opts.EnabledOnly is true., TestListInstalledSkills_SourceFilterRespected restricts rows to the requested source ("hub"\|"builtin"\|"local")., TestSkillsListCommand_RendersStatusColumnAndSummary prints the Status column and "N enabled, M disabled" footer (or "K enabled shown" with --enabled-only)., TestSkillsListCommand_PlatformArgNotPropagated proves the disabled-set lookup does not pass a platform override.
- Source refs: ../hermes-agent/hermes_cli/skills_hub.py:do_list@0e2a53ea, ../hermes-agent/hermes_cli/main.py:skills_list_parser@0e2a53ea, ../hermes-agent/tests/hermes_cli/test_skills_hub.py:test_do_list_renders_status_column, ../hermes-agent/tests/hermes_cli/test_skills_hub.py:test_do_list_marks_disabled_skills, ../hermes-agent/tests/hermes_cli/test_skills_hub.py:test_do_list_enabled_only_hides_disabled, internal/skills/store.go, internal/skills/list.go
- Unblocks: Skills hub search read-model function over registry providers
- Why now: Unblocks Skills hub search read-model function over registry providers.

## 7. CLI log redactor for known secret shapes

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: internal/cli/log_redact.go exposes RedactLine(line []byte) ([]byte, int) where the int is the number of redactions applied. Matches and replaces with "[REDACTED]": (1) Bearer XXX in any header line, (2) api_key=VALUE or x-api-key: VALUE, (3) Telegram bot tokens NN:XXXXXXXX (digits + colon + >=20 alnum/_/-), (4) Slack xoxb-/xoxp-/xoxs- tokens, (5) OpenAI sk-* keys longer than 16 chars. Returns input unchanged with count=0 if no match. Pure: only regexp + bytes packages from stdlib.
- Trust class: operator, system
- Ready when: internal/cli already compiles; this row adds a sibling log_redact.go + _test.go., Tests use fixed []byte literals — no file I/O.
- Not ready when: The slice reads files, walks XDG paths, or uploads anywhere., The slice adds new secret shapes beyond the five listed.
- Degraded mode: Redactor counts replacements per line so the snapshot caller can attach a per-section Redacted field without re-scanning.
- Fixture: `internal/cli/log_redact_test.go`
- Write scope: `internal/cli/log_redact.go`, `internal/cli/log_redact_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run 'TestRedactLine' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/cli/log_redact_test.go fixtures prove redaction across the five secret shapes plus no-match preservation.
- Acceptance: TestRedactLine_BearerToken returns redacted line and count=1 for "Authorization: Bearer abc123def456"., TestRedactLine_ApiKeyEqualsValue covers "api_key=sk-prod-XYZ" and "x-api-key: sk-test-..."., TestRedactLine_TelegramBotToken redacts "12345:ABCDEFGHabcdefgh1234567890"., TestRedactLine_SlackTokens covers xoxb-, xoxp-, xoxs- tokens., TestRedactLine_OpenAIStyleKey redacts sk-* longer than 16 chars only., TestRedactLine_NoMatchPreservesInput returns input unchanged with count=0.
- Source refs: ../hermes-agent/hermes_cli/logs.py
- Unblocks: CLI log snapshot reader using shared redactor
- Why now: Unblocks CLI log snapshot reader using shared redactor.

## 8. BlueBubbles iMessage bubble formatting parity

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

## 9. CLI profile name validator

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: internal/cli adds a pure function `ValidateProfileName(name string) error` and an exported sentinel error set: ErrProfileNameEmpty, ErrProfileNameTooLong, ErrProfileNameInvalidChars, ErrProfileNameReserved; the function accepts names matching `^[a-z0-9][a-z0-9_-]{0,63}$`, treats 'default' as valid (special alias), and rejects the reserved subcommand names {'create','delete','list','use','export','import','show'}
- Trust class: operator, system
- Ready when: internal/cli already exposes pure helpers; adding one new file with one validator + sentinel errors compiles cleanly alongside them., This slice only defines validation; no path resolution, active-profile read/write, command wiring, alias wrapper, or env mutation is required.
- Not ready when: The slice resolves filesystem paths, creates wrapper scripts, mutates provider credentials, modifies internal/config, or registers a Cobra command., The slice modifies any other internal/cli file beyond the new profile_name.go and profile_name_test.go.
- Degraded mode: Callers report a typed sentinel error class instead of free-form text so the CLI can render uniform error messages later without re-parsing strings.
- Fixture: `internal/cli/profile_name_test.go`
- Write scope: `internal/cli/profile_name.go`, `internal/cli/profile_name_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run 'TestValidateProfileName_' -count=1`, `go test ./internal/cli -count=1`, `go vet ./internal/cli`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/cli/profile_name.go declares ValidateProfileName plus the four sentinel errors; five named tests pass; no other internal/cli, internal/config, or cmd/gormes file is modified.
- Acceptance: TestValidateProfileName_AcceptsValid: ValidateProfileName each of {'default','coder','work-1','tier_2','a','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'} returns nil (the last is exactly 64 chars)., TestValidateProfileName_RejectsEmpty: ValidateProfileName('') and ValidateProfileName('   ') (after caller-side trim) both return ErrProfileNameEmpty., TestValidateProfileName_RejectsTooLong: ValidateProfileName(strings.Repeat('a', 65)) returns ErrProfileNameTooLong., TestValidateProfileName_RejectsInvalidChars: each of {'Coder','my profile','-leading','_leading','dot.name','slash/name','tab\tname'} returns ErrProfileNameInvalidChars., TestValidateProfileName_RejectsReserved: each of {'create','delete','list','use','export','import','show'} returns ErrProfileNameReserved (these collide with subcommand names).
- Source refs: ../hermes-agent/hermes_cli/profiles.py@edc78e25:_PROFILE_ID_RE, ../hermes-agent/hermes_cli/profiles.py@edc78e25:validate_profile_name, ../hermes-agent/tests/hermes_cli/test_profiles.py@edc78e25, internal/cli/banner.go
- Unblocks: CLI active-profile store, CLI profile root resolver
- Why now: Unblocks CLI active-profile store, CLI profile root resolver.

## 10. doctorCustomEndpointReadiness check function

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: cmd/gormes adds a pure function `doctorCustomEndpointReadiness(cfg config.Config) doctor.CheckResult` that returns Name='Custom endpoint', Status=Pass when Hermes.Endpoint and Hermes.APIKey and Hermes.Model are all non-empty, Status=Warn when any one is missing (with itemized evidence), and Status=Fail when Endpoint is set but Model is empty; doctorCmd RunE invokes this function after the existing Goncho/Slack checks; --offline still skips network probes elsewhere
- Trust class: operator, system
- Ready when: cmd/gormes/doctor.go already calls doctorGonchoConfig(cfg) and doctorSlackGatewayConfig(cfg, runtimeStatus) — adding a third helper alongside them is mechanical., internal/config/config.go declares HermesCfg{Endpoint, APIKey, Model} so the check has a stable typed input., internal/doctor/doctor.go already exposes CheckResult, ItemInfo, StatusPass/StatusWarn/StatusFail; this row only composes them.
- Not ready when: The slice changes config schema, adds new HermesCfg fields, modifies provider routing, or introduces a live /v1/models or auth lookup., The slice changes any other doctor check's behaviour., The slice ports Hermes Python config.yaml reading.
- Degraded mode: When endpoint is set but credentials or model are missing, the check emits Status=Warn with item-level notes (api_key=missing, model=missing) instead of exiting non-zero, so operators see precisely which field needs attention.
- Fixture: `cmd/gormes/doctor_custom_provider_test.go`
- Write scope: `cmd/gormes/doctor.go`, `cmd/gormes/doctor_custom_provider_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./cmd/gormes -run 'TestDoctorCustomEndpoint\|TestDoctorCmdInvokesCustomEndpointReadiness' -count=1`, `go test ./cmd/gormes -count=1`, `go vet ./cmd/gormes`, `go run ./cmd/builder-loop progress validate`
- Done signal: doctorCustomEndpointReadiness is a pure function with five named tests; doctorCmd invokes it; no internal/config or internal/hermes files are modified.
- Acceptance: TestDoctorCustomEndpointAllSet: cfg with Endpoint='https://example.invalid', APIKey='secret', Model='m' returns CheckResult{Name='Custom endpoint', Status=StatusPass, Summary contains 'configured'} and no items are flagged Warn., TestDoctorCustomEndpointMissingAPIKey: cfg with Endpoint set, APIKey empty, Model='m' returns Status=StatusWarn with an item Name='api_key' Status=StatusWarn Note='missing'., TestDoctorCustomEndpointMissingModel: cfg with Endpoint set, APIKey set, Model empty returns Status=StatusFail with an item Name='model' Status=StatusFail Note='missing' (Hermes considers this a hard error since requests cannot route)., TestDoctorCustomEndpointAllEmpty: cfg with all three empty returns Status=StatusWarn Summary='disabled' so doctor stays useful even when no endpoint is configured., TestDoctorCmdInvokesCustomEndpointReadiness: running doctorCmd.RunE against an in-memory cfg with custom endpoint emits the new check's Format() block to stdout in --offline mode and exits 0 when Status<=Warn.
- Source refs: ../hermes-agent/hermes_cli/doctor.py@b2d3308f:_run_doctor, ../hermes-agent/tests/hermes_cli/test_doctor.py@b2d3308f:test_run_doctor_accepts_bare_custom_provider, cmd/gormes/doctor.go, cmd/gormes/goncho_doctor_test.go, internal/config/config.go:HermesCfg, internal/doctor/doctor.go:CheckResult
- Unblocks: CLI status summary over native stores
- Why now: Unblocks CLI status summary over native stores.

<!-- PROGRESS:END -->
