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
## 1. Azure Foundry probe — path sniffing

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

## 2. Azure Foundry probe — /models classification + Anthropic fallback

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

## 3. Provider rate guard — x-ratelimit header classification

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

## 4. Provider rate guard — degraded-state + last-known-good evidence

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

## 5. Native TUI terminal-selection divergence contract

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

## 6. BlueBubbles iMessage bubble formatting parity

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

## 7. CLI profile path and active-profile store

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: Gormes models Hermes profile names, active-profile selection, and profile-root resolution as pure XDG-scoped helpers before command UI, alias wrappers, cloning, or export/import behavior is ported
- Trust class: operator, system
- Ready when: This slice only defines validation, path resolution, active-profile read/write, and environment resolution helpers., No command UI, alias wrapper, service file, tar export/import, clone-all copy, or provider credential migration is required.
- Not ready when: The slice creates shell wrapper scripts, copies profile directories, mutates provider credentials, or changes runtime config loading for non-profile commands.
- Degraded mode: Profile commands report invalid profile names, missing profiles, reserved-name collisions, and unset active profile state without writing outside the selected Gormes config/data roots.
- Fixture: `internal/cli/profile_store_test.go`
- Write scope: `internal/cli/profile_store.go`, `internal/cli/profile_store_test.go`, `internal/config/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli ./internal/config -run 'Test.*Profile.*Store\|Test.*Profile.*Path\|Test.*Active.*Profile' -count=1`, `go test ./internal/cli ./internal/config -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Profile helper fixtures prove validation, path resolution, active-profile persistence, environment resolution, and no writes outside selected Gormes roots.
- Acceptance: Profile name validation accepts lowercase alphanumeric, underscore, and hyphen names up to 64 characters, keeps default as a special alias, and rejects uppercase, spaces, leading punctuation, empty names, and reserved subcommand names., Default and named profile directories resolve under Gormes XDG roots without reading or writing legacy Hermes profile directories., Active-profile read/write helpers persist only the selected name plus explicit missing/unset evidence., Profile environment resolution returns the effective GORMES profile root for default and named profiles without mutating process-wide environment variables in tests.
- Source refs: ../hermes-agent/hermes_cli/profiles.py@edc78e25, ../hermes-agent/tests/hermes_cli/test_profiles.py@edc78e25, internal/config/config.go, cmd/gormes/main.go
- Unblocks: CLI auth status read model before provider setup, Setup/uninstall dry-run command contracts
- Why now: Unblocks CLI auth status read model before provider setup, Setup/uninstall dry-run command contracts.

## 8. Gateway management CLI read-model closeout

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: Gateway management CLI exposes read-only status, pairing, runtime-validation, and channel-availability evidence over existing Gormes stores before mutating start/stop/restart commands widen the surface
- Trust class: operator, gateway, system
- Ready when: `gormes gateway status` already reads the native runtime status and pairing stores., This slice is read-only; it must not start, stop, restart, install, or supervise live gateway services.
- Not ready when: The slice invokes systemd/sc.exe, opens live channel clients, changes service restart polling, or creates another gateway state file.
- Degraded mode: Gateway CLI reports missing runtime state, invalid PID/process evidence, missing pairing store, disabled channels, and unsupported mutating commands instead of inventing a second management state model.
- Fixture: `cmd/gormes/gateway_management_cli_test.go`
- Write scope: `cmd/gormes/gateway_status.go`, `cmd/gormes/gateway_management_cli_test.go`, `internal/gateway/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./cmd/gormes ./internal/gateway -run 'Test.*Gateway.*Management\|Test.*Gateway.*Status\|Test.*Pairing\|Test.*Runtime.*Validation' -count=1`, `go test ./cmd/gormes ./internal/gateway -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Gateway management fixtures prove read-only status/pairing/runtime evidence, explicit unavailable mutating commands, and no live service-manager dependency.
- Acceptance: A gateway management fixture renders configured channels, pairing status, runtime validation, Slack/Discord/Telegram availability, and missing-state evidence from fake stores., Unsupported mutating management commands return a stable unavailable error with a pointer to the existing service/restart helper rows., PID validation output reuses the existing runtime status validation model and never shells out to a live service manager in tests., The fixture proves no duplicate management state file or Python Hermes command path is read.
- Source refs: ../hermes-agent/hermes_cli/gateway.py@edc78e25, ../hermes-agent/hermes_cli/pairing.py@edc78e25, ../hermes-agent/hermes_cli/status.py@edc78e25, ../hermes-agent/tests/hermes_cli/test_gateway_runtime_health.py@edc78e25, cmd/gormes/gateway_status.go, internal/gateway/status.go, internal/gateway/pairing_store.go
- Unblocks: Webhook/platform management CLI helpers, Cron management CLI over native store
- Why now: Unblocks Webhook/platform management CLI helpers, Cron management CLI over native store.

## 9. Doctor custom endpoint provider readiness

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: gormes doctor accepts custom endpoint/provider-style configuration as operator intent and reports credential/readiness evidence without requiring a named provider registry match
- Trust class: operator, system
- Ready when: Gormes doctor already has --offline, API server health, tool registry, Goncho config, and gateway diagnostics., The slice can use temp XDG config and fake endpoint/provider metadata without network calls.
- Not ready when: The slice introduces a live provider catalog lookup, reads legacy Hermes config.yaml as authoritative state, or changes non-custom provider routing behavior., The slice ports Hermes setup/auth prompts instead of doctor readiness reporting.
- Degraded mode: Doctor output reports custom-endpoint configured, missing API key, offline-skip, or provider-registry-unavailable evidence instead of failing bare custom provider settings as unknown.
- Fixture: `cmd/gormes/doctor_custom_provider_test.go`
- Write scope: `cmd/gormes/doctor.go`, `cmd/gormes/doctor_custom_provider_test.go`, `internal/config/`, `internal/hermes/status.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./cmd/gormes -run TestDoctorCustomProvider -count=1`, `go test ./cmd/gormes ./internal/config ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Doctor fixtures prove custom endpoint/provider-style settings are accepted with explicit readiness evidence and no live provider or legacy Hermes config dependency.
- Acceptance: A config shaped as a custom endpoint with model and no named provider registry entry does not produce an unknown-provider doctor failure., Doctor output distinguishes missing credentials from unknown provider and remains usable in --offline mode., Known-provider validation, if present, remains deterministic and local-testdata backed., No test opens provider network calls, Hermes config.yaml, or live auth stores.
- Source refs: ../hermes-agent/hermes_cli/doctor.py@b2d3308f, ../hermes-agent/tests/hermes_cli/test_doctor.py@b2d3308f:test_run_doctor_accepts_bare_custom_provider, cmd/gormes/doctor.go, internal/config/config.go, internal/hermes/status.go
- Unblocks: CLI status summary over native stores
- Why now: Unblocks CLI status summary over native stores.

## 10. CLI log snapshot reader

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: Gormes captures redacted local log snapshots for agent, gateway, error, tool-audit, and builder-loop logs from XDG paths without network upload or archive creation
- Trust class: operator, system
- Ready when: This slice is a pure local file reader with injected root paths and fixture log files., No paste upload, support bundle archive, live provider status, or backup write behavior is required.
- Not ready when: The slice uploads to paste.rs/dpaste, creates tar/zip backups, reads legacy Hermes logs as authoritative state, or changes `gormes doctor` exit codes.
- Degraded mode: Diagnostics report missing logs, rotated-log fallback, truncation, redaction, and unreadable-file evidence without failing the whole doctor/status command.
- Fixture: `internal/cli/log_snapshot_test.go`
- Write scope: `internal/cli/log_snapshot.go`, `internal/cli/log_snapshot_test.go`, `internal/config/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli ./internal/config -run 'Test.*Log.*Snapshot\|Test.*Diagnostic.*Log\|Test.*Redact' -count=1`, `go test ./internal/cli ./internal/config -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Log snapshot fixtures prove local log capture, rotated fallback, truncation evidence, redaction, and no network/archive side effects.
- Acceptance: Fixtures read small log files and return full plus tail text for agent, gateway, error, tool-audit, and builder-loop log classes., Missing primary logs fall back to rotated log names when available and otherwise emit file-not-found evidence., Large logs are capped by byte and line limits with truncation evidence., Secrets shaped like API keys, bearer tokens, and configured proxy keys are redacted before rendering.
- Source refs: ../hermes-agent/hermes_cli/debug.py@edc78e25, ../hermes-agent/hermes_cli/logs.py@edc78e25, ../hermes-agent/tests/hermes_cli/test_debug.py@edc78e25, ../hermes-agent/tests/hermes_cli/test_logs.py@edc78e25, internal/cli/, internal/config/config.go, cmd/gormes/doctor.go
- Unblocks: CLI status summary over native stores, Backup manifest dry-run contract
- Why now: Unblocks CLI status summary over native stores, Backup manifest dry-run contract.

<!-- PROGRESS:END -->
