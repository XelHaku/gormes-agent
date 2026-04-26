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
## 1. Provider rate guard — degraded-state + last-known-good evidence

- Phase: 4 / 4.H
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: TDD packet for a missing pure state-transition helper. The prerequisite (RateLimitClass + Classify429 in internal/hermes/provider_rate_guard.go) is shipped on main as of commit a1d7d928; do NOT stub or duplicate those constants. STEP 1: write internal/hermes/provider_rate_guard_degraded_test.go in package hermes (importing only `testing` and `time`). Define t0 and t1 once at the top of TestApplyClassification: `t0 := time.Date(2026,4,26,12,0,0,0,time.UTC); t1 := t0.Add(5*time.Minute)`. Add six t.Run subtests in this order — preserves_last_known_when_insufficient, treats_empty_class_as_insufficient, fresh_genuine_quota_clears_unavailable, fresh_upstream_capacity_clears_unavailable, input_immutability_via_struct_value, transitions_back_to_available_on_fresh_evidence — each calling ApplyClassification with explicit GuardState literals and asserting the returned struct field-by-field with t.Fatalf on mismatch. STEP 2: write internal/hermes/provider_rate_guard_degraded.go in package hermes (importing only `time`). Define `type GuardState struct { LastKnownClass RateLimitClass; LastKnownAt time.Time; Unavailable bool }` and `func ApplyClassification(state GuardState, now time.Time, class RateLimitClass) GuardState`. Algorithm: if class == RateLimitInsufficientEvidence OR class == RateLimitClass(""), return GuardState{LastKnownClass: state.LastKnownClass, LastKnownAt: state.LastKnownAt, Unavailable: true}. Otherwise return GuardState{LastKnownClass: class, LastKnownAt: now, Unavailable: false}. The helper takes its argument by value, never mutates it, never reads time.Now, never spawns a goroutine, and never touches retry/breaker code.
- Trust class: system
- Ready when: Provider rate guard — x-ratelimit header classification is complete on main (commit a1d7d928); RateLimitClass and the three RateLimit* constants are importable directly from package hermes., internal/hermes/provider_rate_guard_degraded.go and provider_rate_guard_degraded_test.go are absent on main; the worker's first edit is the focused failing test file.
- Not ready when: The slice writes a process-global or cross-session breaker., The slice changes retry policy or treats every 429 as account-level quota exhaustion., The slice redeclares RateLimitGenuineQuota / RateLimitUpstreamCapacity / RateLimitInsufficientEvidence; reuse the constants from provider_rate_guard.go.
- Degraded mode: Provider status reports last_known_good=present\|absent, plus rate_guard_unavailable or budget_header_missing when classification is unsafe; no retry amplification.
- Fixture: `internal/hermes/provider_rate_guard_degraded_test.go (new file)::TestApplyClassification/preserves_last_known_when_insufficient`
- Write scope: `internal/hermes/provider_rate_guard_degraded.go`, `internal/hermes/provider_rate_guard_degraded_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run '^TestApplyClassification$' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/hermes/provider_rate_guard_degraded_test.go fixtures prove last-known-good preservation and degraded-mode transitions across all three RateLimitClass values., internal/hermes/provider_rate_guard_degraded.go contains only GuardState and ApplyClassification, reusing RateLimitClass from provider_rate_guard.go with no duplicated constants.
- Acceptance: preserves_last_known_when_insufficient: ApplyClassification(GuardState{LastKnownClass: RateLimitGenuineQuota, LastKnownAt: t0, Unavailable: false}, t1, RateLimitInsufficientEvidence) == GuardState{RateLimitGenuineQuota, t0, true}; LastKnownAt is unchanged from t0, Unavailable flipped to true., treats_empty_class_as_insufficient: ApplyClassification(GuardState{LastKnownClass: RateLimitUpstreamCapacity, LastKnownAt: t0, Unavailable: false}, t1, RateLimitClass("")) preserves LastKnownClass=RateLimitUpstreamCapacity and LastKnownAt=t0 while setting Unavailable=true., fresh_genuine_quota_clears_unavailable: ApplyClassification(GuardState{}, t1, RateLimitGenuineQuota) == GuardState{RateLimitGenuineQuota, t1, false}., fresh_upstream_capacity_clears_unavailable: ApplyClassification(GuardState{}, t1, RateLimitUpstreamCapacity) == GuardState{RateLimitUpstreamCapacity, t1, false}., input_immutability_via_struct_value: declare `original := GuardState{LastKnownClass: RateLimitGenuineQuota, LastKnownAt: t0, Unavailable: true}`, call ApplyClassification(original, t1, RateLimitUpstreamCapacity), then assert original is unchanged via direct field comparison (LastKnownClass==RateLimitGenuineQuota, LastKnownAt==t0, Unavailable==true)., transitions_back_to_available_on_fresh_evidence: ApplyClassification(GuardState{LastKnownClass: RateLimitGenuineQuota, LastKnownAt: t0, Unavailable: true}, t1, RateLimitUpstreamCapacity) == GuardState{RateLimitUpstreamCapacity, t1, false}.
- Source refs: ../hermes-agent/agent/nous_rate_guard.py@192e7eb2:record_nous_rate_limit, ../hermes-agent/agent/nous_rate_guard.py@192e7eb2:nous_rate_limit_remaining, internal/hermes/provider_rate_guard.go::RateLimitClass, internal/hermes/provider_rate_guard.go::Classify429
- Unblocks: Provider rate guard + budget telemetry
- Why now: Unblocks Provider rate guard + budget telemetry.

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

## 3. CLI profile name validator

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

## 4. doctorCustomEndpointReadiness check function

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

## 5. Custom provider model-switch credential preservation

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: internal/cli adds a pure function `ResolveCustomProviderSecret(ref CustomProviderRef, env map[string]string) (CustomProviderResolution, error)` where CustomProviderRef has fields {Name string, BaseURL string, APIKey string, KeyEnv string} and CustomProviderResolution has fields {EffectiveSecret string, PersistAsRef string, Evidence string}; the function reads env-template `${VAR}` from APIKey via env, prefers KeyEnv when APIKey is empty, and never returns plaintext in PersistAsRef when the input was a reference
- Trust class: operator, system
- Ready when: internal/cli already exposes pure helpers (banner.go, output.go, parity.go) so adding a single new file with one exported function compiles cleanly., This slice only defines a pure resolver over Go map/struct literals; no config reader, /model command handler, fake catalog server, or TUI dispatch is required.
- Not ready when: The slice ports model_switch.py wholesale, opens a fake /v1/models server, modifies internal/config or internal/hermes, or wires the resolver into command handlers in the same change., The slice returns plaintext in CustomProviderResolution.PersistAsRef when the input APIKey was an env-template `${VAR}` reference or KeyEnv was set.
- Degraded mode: Resolution returns Evidence='credential_missing', 'secret_ref_preserved', 'plaintext_provided', or 'env_var_unset' so callers can distinguish persistable references from resolved secrets without writing plaintext to config.
- Fixture: `internal/cli/custom_provider_secret_test.go`
- Write scope: `internal/cli/custom_provider_secret.go`, `internal/cli/custom_provider_secret_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run 'TestResolveCustomProviderSecret_' -count=1`, `go test ./internal/cli -count=1`, `go vet ./internal/cli`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/cli/custom_provider_secret.go declares ResolveCustomProviderSecret, CustomProviderRef, CustomProviderResolution, ErrCustomProviderEnvUnset, and ErrCustomProviderCredentialMissing; five named tests pass; no internal/config, internal/hermes, or cmd/gormes file is modified.
- Acceptance: TestResolveCustomProviderSecret_EnvTemplatePreserved: ref={Name:'acme',APIKey:'${ACME_KEY}'}, env={'ACME_KEY':'sk-real'} returns {EffectiveSecret:'sk-real', PersistAsRef:'${ACME_KEY}', Evidence:'secret_ref_preserved'}., TestResolveCustomProviderSecret_KeyEnvFallback: ref={Name:'acme',APIKey:'',KeyEnv:'ACME_KEY'}, env={'ACME_KEY':'sk-real'} returns {EffectiveSecret:'sk-real', PersistAsRef:'${ACME_KEY}', Evidence:'secret_ref_preserved'}., TestResolveCustomProviderSecret_PlaintextProvided: ref={Name:'acme',APIKey:'sk-plain'}, env={} returns {EffectiveSecret:'sk-plain', PersistAsRef:'sk-plain', Evidence:'plaintext_provided'} (the function does not invent a reference)., TestResolveCustomProviderSecret_EnvVarUnset: ref={Name:'acme',APIKey:'${ACME_KEY}'}, env={} returns {EffectiveSecret:'', PersistAsRef:'${ACME_KEY}', Evidence:'env_var_unset'} and a non-nil error of class ErrCustomProviderEnvUnset., TestResolveCustomProviderSecret_BothEmpty: ref={Name:'acme',APIKey:'',KeyEnv:''}, env={} returns {EffectiveSecret:'', PersistAsRef:'', Evidence:'credential_missing'} and a non-nil error of class ErrCustomProviderCredentialMissing.
- Source refs: ../hermes-agent/hermes_cli/main.py@1fdc31b2:_custom_provider_api_key_config_value, ../hermes-agent/hermes_cli/main.py@8bbeaea6:_named_custom_provider_map, ../hermes-agent/tests/hermes_cli/test_custom_provider_model_switch.py@8bbeaea6, internal/cli/banner.go, internal/cli/output.go
- Unblocks: CLI command registry parity + active-turn busy policy
- Why now: Unblocks CLI command registry parity + active-turn busy policy.

## 6. [IMPORTANT:] prompt prefix for cron and skill commands

- Phase: 5 / 5.F
- Owner: `skills`
- Size: `small`
- Status: `planned`
- Contract: internal/cron.CronHeartbeatPrefix and internal/skills.BuildSkillSlashCommandMessage emit `[IMPORTANT:` instead of `[SYSTEM:` so Azure OpenAI Default/DefaultV2 content filters do not reject Gormes prompts as prompt-injection (HTTP 400) — same semantic meta-instruction, different bracketed marker; tests update in lockstep so the byte-match assertions still cover drift
- Trust class: operator, system
- Ready when: Upstream Hermes shipped this rename across two commits (d7a34682 + 20cb706e) on 2026-04-09 / 2026-04-26 with explicit cause (Azure content filter HTTP 400 on `[SYSTEM:` markers)., Gormes uses the same marker pattern in exactly two production code paths today: internal/cron/heartbeat.go (CronHeartbeatPrefix) and internal/skills/commands.go (BuildSkillSlashCommandMessage).
- Not ready when: The slice changes the `[SILENT]` token semantics, the skill body trimming, the cron prompt structure beyond the bracketed marker word, or the `Heartbeat [SYSTEM:] + [SILENT] delivery contract` row name in 2.D (that row name is a historical record; only the runtime constant + the byte-match tests change)., The slice introduces a new Azure provider adapter, a content-filter-detection layer, or a configurable marker word — the change is a hardcoded literal rename only., The slice updates internal/progress/progress_test.go literal assertions for the 2.D row name (the row name in progress.json must stay as `Heartbeat [SYSTEM:] + [SILENT] delivery contract` for historical accuracy).
- Degraded mode: Operator-visible prompt text changes from `[SYSTEM: ...]` to `[IMPORTANT: ...]`; behavior is otherwise identical, including the `[SILENT]` suppression contract and the skill body trimming.
- Fixture: `internal/cron/heartbeat_test.go`
- Write scope: `internal/cron/heartbeat.go`, `internal/cron/heartbeat_test.go`, `internal/skills/commands.go`, `internal/skills/preprocessing_commands_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cron ./internal/skills -count=1`, `go test ./internal/progress -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: go test ./internal/cron and ./internal/skills both pass after the marker rename; `grep -rn '\[SYSTEM:' internal/cron/ internal/skills/` returns no matches in production code; `grep -rn '\[IMPORTANT:' internal/cron/ internal/skills/` returns at least 4 matches (constant + tests in both packages).
- Acceptance: internal/cron/heartbeat.go:CronHeartbeatPrefix starts with `[IMPORTANT:` (replacing `[SYSTEM:`) and the load-bearing phrases (`DELIVERY:`, `SILENT:`, `[SILENT]`) are byte-identical to the prior version., internal/cron/heartbeat_test.go asserts `strings.HasPrefix(full, "[IMPORTANT:")` (not `[SYSTEM:`); the existing TestHeartbeatPrefix_ContainsLoadBearingPhrases load-bearing phrase set updates only its first member., internal/skills/commands.go:BuildSkillSlashCommandMessage emits `[IMPORTANT: The user has invoked the "<name>" skill, ...` (replacing `[SYSTEM:`)., internal/skills/preprocessing_commands_test.go updates its expected golden string to `[IMPORTANT:` for the affected fixtures., DetectSilent semantics in internal/cron/heartbeat.go are unchanged (the `[SILENT]` token is independent of the leading marker).
- Source refs: ../hermes-agent/cron/scheduler.py@d7a34682, ../hermes-agent/agent/skill_commands.py@d7a34682, ../hermes-agent/cli.py@20cb706e, ../hermes-agent/gateway/run.py@20cb706e, ../hermes-agent/tools/process_registry.py@20cb706e, internal/cron/heartbeat.go:CronHeartbeatPrefix, internal/cron/heartbeat_test.go, internal/skills/commands.go:BuildSkillSlashCommandMessage, internal/skills/preprocessing_commands_test.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 7. TUI TerminalNativeSelectionHelp constant + help-string fixture

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Contract: internal/tui declares an exported string constant TerminalNativeSelectionHelp = 'Selection: use your terminal's native selection (Shift-drag in most terminals; iTerm Cmd-drag, tmux copy-mode). Gormes does not advertise an in-app copy hotkey.' and a pure helper SelectionHelpLine() that returns it; one fixture asserts the constant exists, mentions 'terminal' but not 'Cmd+C'/'Ctrl+C'/'Ctrl-Shift-C'/'OSC 52'/'clipboard hotkey'/'Ink', and another asserts no advertised copy shortcut leaks anywhere else in the package
- Trust class: operator
- Ready when: internal/tui already exposes Bubble Tea model/view/update files and a mouse tracking config; adding a single new file with one constant compiles cleanly alongside them., phase-5-final-purge.md already documents the terminal-native selection divergence, so this row is mechanical: lift that statement into a typed Go constant and a regression test.
- Not ready when: The slice ports Hermes Ink, calls OSC 52, adds clipboard libraries, modifies internal/tui/update.go input handling, or changes remote TUI transport., The slice introduces a Cobra command flag for copy mode or a configuration key., The slice modifies cmd/gormes/ files.
- Degraded mode: If a future row adds a real Go-native copy mode, it must replace this constant rather than extend it; until then, the help-string fixture prevents accidental advertising of unimplemented Ink shortcuts.
- Fixture: `internal/tui/selection_help_test.go`
- Write scope: `internal/tui/selection_help.go`, `internal/tui/selection_help_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tui -run 'TestTerminalNativeSelectionHelpExists\|TestTerminalNativeSelectionHelpNoFakeShortcuts\|TestTUIPackageDoesNotAdvertiseCopyHotkey' -count=1`, `go test ./internal/tui -count=1`, `go vet ./internal/tui`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/tui/selection_help.go declares TerminalNativeSelectionHelp and SelectionHelpLine; three named tests pass; no other internal/tui or cmd/gormes file is modified.
- Acceptance: TestTerminalNativeSelectionHelpExists: TerminalNativeSelectionHelp is a non-empty string constant exported from internal/tui, contains the substring 'terminal', and SelectionHelpLine() returns the same value., TestTerminalNativeSelectionHelpNoFakeShortcuts: TerminalNativeSelectionHelp does not contain any of: 'Cmd+C', 'Ctrl+C', 'Ctrl-Shift-C', 'Cmd-Shift-C', 'OSC 52', 'clipboard hotkey', 'Ink' (case-insensitive)., TestTUIPackageDoesNotAdvertiseCopyHotkey: walking internal/tui/*.go files, no string literal in the package contains the same forbidden shortcuts above (test reads the package source via os.ReadFile, not a runtime check)., go vet ./internal/tui passes; no other package is imported by the new file beyond stdlib.
- Source refs: ../hermes-agent/ui-tui/packages/hermes-ink/src/ink/selection.ts@edc78e25, ../hermes-agent/ui-tui/packages/hermes-ink/src/ink/selection.ts@31d7f195, internal/tui/view.go, internal/tui/model.go, internal/tui/mouse_tracking.go, docs/content/building-gormes/architecture_plan/phase-5-final-purge.md
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

<!-- PROGRESS:END -->
