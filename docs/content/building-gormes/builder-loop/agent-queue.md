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
## 1. Native TUI terminal-selection divergence contract

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

## 3. Azure OpenAI query/default_query transport contract

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: OpenAI-compatible Azure endpoints preserve api-version query parameters and keep GPT-5.x deployments on chat/completions with max_completion_tokens semantics instead of upgrading to Codex Responses
- Trust class: system, operator
- Ready when: Provider interface + stream fixture harness, generic unsupported-parameter retry, and Codex Responses conversion fixtures are validated on main., The slice can use httptest to inspect request URLs and bodies; no Azure account, deployment, or live provider credential is required.
- Not ready when: The slice implements the Azure Foundry setup wizard, Anthropic Messages transport, live model discovery, or credential storage instead of only freezing OpenAI-compatible request construction., The slice removes the existing generic unsupported-parameter retry instead of adding Azure-specific first-request and routing fixtures.
- Degraded mode: Provider status reports azure_query_preserved, azure_chat_completions, or azure_transport_unavailable evidence instead of silently dropping api-version or switching GPT-5.x to Responses.
- Fixture: `internal/hermes/azure_openai_transport_test.go`
- Write scope: `internal/hermes/http_client.go`, `internal/hermes/azure_openai_transport_test.go`, `internal/hermes/unsupported_parameter_retry_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run 'TestAzureOpenAI\|TestUnsupportedParameterRetry' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Azure OpenAI fixtures prove api-version query preservation, GPT-5.x chat/completions routing, max_completion_tokens behavior, and zero live Azure dependency.
- Acceptance: A legacy Azure base URL with ?api-version=... sends POST requests to a clean /v1/chat/completions path while preserving the query parameters., openai.azure.com GPT-5.x provider/model fixtures remain on chat_completions and never call the Codex Responses conversion path., Azure OpenAI o-series and GPT-5 fixtures emit or retry to max_completion_tokens without changing non-Azure generic providers., All Azure OpenAI transport tests run through httptest with no network access or live Azure credentials.
- Source refs: ../hermes-agent/agent/auxiliary_client.py@24b4b24d:_extract_url_query_params, ../hermes-agent/run_agent.py@ac571142:_is_azure_openai_url, ../hermes-agent/tests/run_agent/test_run_agent.py@ac571142:TestAzureOpenAIRouting, internal/hermes/http_client.go, internal/hermes/unsupported_parameter_retry_test.go
- Unblocks: Azure Foundry endpoint autodetect + model context read model
- Why now: Unblocks Azure Foundry endpoint autodetect + model context read model.

## 4. Azure Anthropic Messages endpoint contract

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Azure Anthropic endpoints use static Azure API keys, strip trailing /v1 before Messages requests, and pass api-version through query parameters without invoking Anthropic OAuth refresh
- Trust class: system, operator
- Ready when: Direct Anthropic Messages adapter fixtures are validated and can be exercised with httptest., The slice can inject fake Azure keys and base URLs through config/request helpers; no Claude Code OAuth store or live Azure endpoint is required.
- Not ready when: The slice ports the full Azure Foundry wizard, OpenAI-style Azure transport, or XDG token vault in the same change., The slice reads or mutates ~/.claude, ~/.hermes, or hosted Anthropic OAuth state in unit tests.
- Degraded mode: Provider status reports azure_anthropic_key_missing, azure_api_version_query, or azure_oauth_bypassed evidence before any live Messages request is attempted.
- Fixture: `internal/hermes/azure_anthropic_transport_test.go`
- Write scope: `internal/hermes/anthropic_client.go`, `internal/hermes/azure_anthropic_transport_test.go`, `internal/config/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run 'TestAzureAnthropic\|TestAnthropic' -count=1`, `go test ./internal/hermes ./internal/config -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Azure Anthropic fixtures prove base-URL cleanup, api-version query handling, static Azure-key auth, hosted-OAuth bypass, and no regression to normal Anthropic requests.
- Acceptance: A https://.../anthropic/v1 base URL stores and uses the Azure Anthropic base stripped to /anthropic before appending /v1/messages., Azure Anthropic requests include api-version=2025-04-15 as query/default_query and never produce /anthropic?api-version=.../v1/messages., Azure Anthropic auth uses AZURE_ANTHROPIC_KEY or a configured static key before any Anthropic OAuth refresh path and reports missing keys clearly., Non-Azure Anthropic requests keep existing x-api-key and bearer behavior.
- Source refs: ../hermes-agent/agent/anthropic_adapter.py@c15064fa, ../hermes-agent/hermes_cli/runtime_provider.py@7bfa9442, ../hermes-agent/hermes_cli/runtime_provider.py@d8e4c721, ../hermes-agent/website/docs/guides/azure-foundry.md@7c50ed70, internal/hermes/anthropic_client.go
- Unblocks: Azure Foundry endpoint autodetect + model context read model
- Why now: Unblocks Azure Foundry endpoint autodetect + model context read model.

## 5. CLI profile path and active-profile store

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

## 6. Gateway management CLI read-model closeout

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

## 7. Doctor custom endpoint provider readiness

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

## 8. CLI log snapshot reader

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

## 9. TUI gateway progress/completion helpers

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Contract: Pure TUI gateway helper functions normalize tool-progress mode, completion paths, and tool summary formatting from fixed inputs
- Trust class: operator, system
- Ready when: No transport or lifecycle code is required; helpers can be implemented as pure functions under internal/tuigateway with table tests.
- Not ready when: The slice opens HTTP/SSE connections, starts a Bubble Tea program, adds a remote client, or ports image/personality/platform-event helpers.
- Degraded mode: Remote TUI streaming remains unavailable while status can report missing progress/completion helper coverage.
- Fixture: `internal/tuigateway/progress_completion_test.go`
- Write scope: `internal/tuigateway/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tuigateway -run 'Test.*Progress\|Test.*Completion\|Test.*ToolSummary' -count=1`, `go test ./internal/tuigateway -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Pure internal/tuigateway fixtures prove progress mode, completion path, and tool-summary helpers without transport or Bubble Tea dependencies.
- Acceptance: Tool-progress mode parsing and enabled/disabled decisions match upstream fixtures., Completion paths normalize consistently for empty, relative, absolute, and home-directory-shaped inputs., Tool duration/count/list summary helpers are deterministic and side-effect free.
- Source refs: ../hermes-agent/tui_gateway/server.py@edc78e25, ../hermes-agent/tui_gateway/render.py@edc78e25, ../hermes-agent/tests/test_tui_gateway_server.py@edc78e25, internal/tui/
- Unblocks: TUI gateway image/personality/platform-event helpers
- Why now: Unblocks TUI gateway image/personality/platform-event helpers.

## 10. Planner backend noninteractive stdin failure guard

- Phase: 5 / 5.N
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Planner-loop and builder-loop backend launches fail fast with classified backend_failed evidence when Codex-style backends wait for stdin or emit no progress, without producing blank subphase audit rows
- Trust class: operator, system
- Ready when: The last-7-day audit shows 60 blank-subphase claims with 31 worker/backend failures and repeated backend_failed detail containing `Reading additional input from stdin`., The slice can use fake backend commands and fixture JSONL ledgers; no live Codex backend, upstream repo sync, or progress row mutation is required.
- Not ready when: The slice changes roadmap selection, worker prompt content, progress item contracts, or runtime feature code outside builder/planner loop backend failure classification., The fix hides backend stdout/stderr or rewrites blank ledger entries instead of preserving evidence and classifying them deterministically.
- Degraded mode: Planner status reports backend_waiting_for_stdin, backend_no_progress, backend_killed, and missing-task metadata separately so toxic-subphase analysis does not collapse into blank row IDs.
- Fixture: `internal/builderloop/backend_noninteractive_test.go and internal/plannerloop/autoloop_audit_test.go`
- Write scope: `internal/builderloop/backend.go`, `internal/builderloop/backend_noninteractive_test.go`, `internal/builderloop/failures.go`, `internal/plannerloop/autoloop_audit.go`, `internal/plannerloop/autoloop_audit_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/builderloop -run 'Test.*Backend.*Noninteractive\|Test.*Backend.*Failure\|Test.*Backend.*Degraded' -count=1`, `go test ./internal/plannerloop -run 'Test.*Autoloop.*Audit\|Test.*Blank.*Subphase\|Test.*Backend.*Failure' -count=1`, `go test ./internal/builderloop ./internal/plannerloop -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Backend/audit fixtures prove stdin-waiting and killed backends are classified with preserved evidence and no blank toxic-subphase buckets.
- Acceptance: A fake backend that prints `Reading additional input from stdin` exits with a classified backend_waiting_for_stdin failure, non-empty task metadata when the selected row is known, and the original stderr excerpt preserved., A killed backend or backend with no progress heartbeat records backend_killed or backend_no_progress without producing an empty subphase_id in planner audit summaries., Planner audit fixtures group missing task metadata under an explicit control-plane bucket instead of the empty string and include remediation text for backend infrastructure failures., No progress.json health block is created, removed, or modified by this backend failure classification path.
- Source refs: .codex/planner-loop/state/runs.jsonl:20260425T210430Z backend_failed Reading additional input from stdin, .codex/planner-loop/state/runs.jsonl:20260425T233746Z backend_failed signal: killed: Reading additional input from stdin, .codex/orchestrator/state/runs.jsonl, internal/builderloop/backend.go, internal/builderloop/failures.go, internal/plannerloop/autoloop_audit.go, internal/plannerloop/run.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

<!-- PROGRESS:END -->
