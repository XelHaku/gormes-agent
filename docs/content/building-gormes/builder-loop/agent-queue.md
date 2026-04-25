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
## 1. Native TUI bundle independence check

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Gormes TUI startup and install/update status stay Go-native and never depend on Hermes' Node/Ink dist bundle freshness checks
- Trust class: operator, system
- Ready when: Bubble Tea shell and local TUI startup seams exist in Gormes., Hermes ee0728c6 adds a Python/Node `_tui_build_needed` regression around missing `packages/hermes-ink/dist/ink-bundle.js` even when `dist/entry.js` exists.
- Not ready when: The slice ports Hermes `_tui_build_needed`, adds Node/npm/package-lock/node_modules checks to Gormes startup, or edits remote TUI SSE transport behavior.
- Degraded mode: TUI and doctor/status output report native Go TUI availability instead of asking operators to run npm install/build or repair packages/hermes-ink/dist/ink-bundle.js.
- Fixture: `cmd/gormes/tui_bundle_independence_test.go`
- Write scope: `cmd/gormes/`, `internal/tui/`, `internal/cli/`, `docs/content/building-gormes/architecture_plan/progress.json`, `docs/content/building-gormes/architecture_plan/phase-5-final-purge.md`, `www.gormes.ai/internal/site/content.go`
- Test commands: `go test ./cmd/gormes ./internal/tui ./internal/cli -run 'Test.*TUI.*Bundle\|Test.*Native.*TUI\|Test.*Doctor.*TUI' -count=1`, `go test ./cmd/gormes ./internal/tui ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Focused fixtures prove Gormes TUI startup/status remains independent from Hermes Node/Ink bundle rebuild checks and public install copy still promises no runtime npm dependency.
- Acceptance: A focused fixture runs the Gormes TUI startup/build-preflight path from a temp working directory that contains Hermes-style `ui-tui/dist/entry.js` and `node_modules/ink` but lacks `packages/hermes-ink/dist/ink-bundle.js`, and proves no npm/node/package-manager command is invoked., Offline doctor or startup status describes the TUI as native Go/Bubble Tea and does not require `HERMES_TUI_DIR`, `package-lock.json`, `node_modules`, or `ink-bundle.js`., Landing/docs install messaging continues to state no runtime npm/Node dependency and does not inherit Hermes first-launch TUI rebuild instructions.
- Source refs: ../hermes-agent/hermes_cli/main.py@ee0728c6, ../hermes-agent/tests/hermes_cli/test_tui_npm_install.py@ee0728c6, cmd/gormes/main.go, internal/tui/, www.gormes.ai/internal/site/content.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 2. TUI launch model override + static alias resolver

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Gormes TUI honors top-level --model/--provider and GORMES_INFERENCE_* overrides at startup without network model lookup or oneshot-only coupling
- Trust class: operator, system
- Ready when: Top-level oneshot model/provider resolution is validated, but runTUI currently ignores --model/--provider flags and uses cfg.Hermes.Model directly., Gormes has a static embedded model registry that can resolve short aliases without OpenRouter or provider network calls.
- Not ready when: The slice changes oneshot stdout capture, opens provider catalog network calls during startup, or ports Hermes Node/Ink UI code.
- Degraded mode: Startup returns an actionable model/provider ambiguity error instead of silently using stale configured defaults or performing network catalog lookup.
- Fixture: `cmd/gormes/tui_model_override_test.go`
- Write scope: `cmd/gormes/main.go`, `cmd/gormes/tui_model_override_test.go`, `internal/config/`, `internal/hermes/model_registry.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./cmd/gormes -run TestTUIModelOverride -count=1`, `go test ./cmd/gormes ./internal/config ./internal/hermes -run 'Test.*TUI.*Model\|Test.*OneshotInference\|Test.*ModelRegistry' -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: TUI startup fixtures prove --model/--provider/env override behavior, static alias resolution, exit-code-2 ambiguity errors, and no network catalog lookup.
- Acceptance: `gormes --offline --model sonnet --provider anthropic` starts the TUI/kernel with the resolved Anthropic model without calling api_server health or network model catalogs in the focused fixture., A provider override without an explicit model returns the same exit-code-2 ambiguity class as oneshot resolution., Resume/session startup still persists the same TUI session key and does not mutate persisted config defaults.
- Source refs: ../hermes-agent/hermes_cli/main.py@283c8fd6, ../hermes-agent/hermes_cli/models.py@283c8fd6, ../hermes-agent/tui_gateway/server.py@283c8fd6, ../hermes-agent/tests/hermes_cli/test_models.py@283c8fd6, ../hermes-agent/tests/hermes_cli/test_tui_resume_flow.py@283c8fd6, cmd/gormes/main.go, internal/config/config.go, internal/hermes/model_registry.go
- Unblocks: SSE streaming to Bubble Tea TUI
- Why now: Unblocks SSE streaming to Bubble Tea TUI.

## 3. Native TUI terminal-selection divergence contract

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

## 5. DeepSeek/Kimi cross-provider reasoning isolation

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Contract: DeepSeek and Kimi replay of assistant tool-call turns injects an empty reasoning_content placeholder before promoting generic stored reasoning from another provider
- Trust class: system
- Ready when: DeepSeek/Kimi reasoning_content echo for tool-call replay is validated, so provider detection and basic empty-placeholder behavior already exist., The slice can use the existing captureRequestHTTPClient fixture; no live provider credentials or provider adapter rewrites are required.
- Not ready when: The slice changes non-thinking provider payloads, strips explicit same-provider reasoning_content, or stores hidden reasoning text in ordinary assistant content., The slice attempts OpenRouter, Codex Responses, Bedrock, or generic reasoning-tag sanitization behavior beyond the DeepSeek/Kimi replay ordering bug.
- Degraded mode: Provider status reports cross-provider reasoning isolation as unavailable until replay fixtures prove prior-provider reasoning is not forwarded to DeepSeek/Kimi tool-call continuations.
- Fixture: `internal/hermes/reasoning_content_echo_test.go`
- Write scope: `internal/hermes/http_client.go`, `internal/hermes/reasoning_content_echo_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run 'TestReasoningContentEcho.*Isolation\|TestReasoningContentEcho.*Preserves\|TestReasoningContentEcho.*Untouched' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Reasoning echo fixtures prove DeepSeek/Kimi replay sends empty reasoning_content for prior-provider generic reasoning, preserves explicit reasoning_content, and leaves non-thinking payloads untouched.
- Acceptance: A DeepSeek replay fixture with an assistant tool-call message containing generic Reasoning but no explicit ReasoningContent sends reasoning_content="" instead of the generic reasoning text., The same isolation fixture passes for Kimi/Moonshot provider detection., An explicit ReasoningContent string on the assistant tool-call message is still preserved verbatim., Non-tool assistant turns and non-thinking providers still omit reasoning_content and the storage-only reasoning field from outgoing API payloads.
- Source refs: ../hermes-agent/run_agent.py@f93d4624:_copy_reasoning_content_for_api, ../hermes-agent/run_agent.py@f93d4624:_needs_deepseek_tool_reasoning, internal/hermes/http_client.go:openAICompatibleReasoningContent, internal/hermes/reasoning_content_echo_test.go, docs/content/upstream-hermes/source-study.md
- Unblocks: OpenRouter, Cross-provider reasoning-tag sanitization, Codex stream repair + tool-call leak sanitizer
- Why now: Unblocks OpenRouter, Cross-provider reasoning-tag sanitization, Codex stream repair + tool-call leak sanitizer.

## 6. MCP server config/env resolver

- Phase: 5 / 5.G
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: Gormes parses Hermes-compatible mcp_servers config into safe stdio/HTTP server definitions with env validation, timeout defaults, header redaction, and Honcho MCP self-hosted URL support before any live MCP connection starts
- Trust class: operator, system
- Ready when: Platform toolset config persistence + MCP sentinel is validated on main, so unknown MCP server names and no_mcp suppression already round-trip safely., This slice can use in-memory YAML/JSON fixtures and must not import an MCP SDK or spawn subprocesses.
- Not ready when: The slice opens stdio/HTTP transports, performs OAuth, probes tools, starts managed gateways, or treats HONCHO_API_URL as global Goncho configuration instead of a server-local MCP env binding.
- Degraded mode: MCP status reports disabled, invalid transport, invalid env, missing SDK, or secret-redacted config errors instead of launching partial servers.
- Fixture: `internal/tools/mcp_config_test.go`
- Write scope: `internal/tools/mcp_config.go`, `internal/tools/mcp_config_test.go`, `internal/config/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools ./internal/config -run 'Test.*MCP.*Config\|Test.*MCP.*Env\|Test.*Honcho.*MCP' -count=1`, `go test ./internal/tools ./internal/config -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: MCP config fixtures prove transport/env parsing, redaction, invalid-config degradation, no live SDK startup, and HONCHO_API_URL server-local handling.
- Acceptance: Config fixtures parse stdio servers with command, args, env, timeout, connect_timeout, and sampling fields., HTTP fixtures parse url, headers, timeout, and connect_timeout while redacting Authorization and token-like values from status/errors., Invalid env var names, servers with both command and url, and servers with neither command nor url fail before runtime startup., A Honcho MCP fixture preserves HONCHO_API_URL only as the configured server env value and never exposes it as a request header or Goncho memory setting.
- Source refs: ../hermes-agent/hermes_cli/mcp_config.py@edc78e25, ../hermes-agent/tools/mcp_tool.py@edc78e25, ../hermes-agent/tests/hermes_cli/test_mcp_config.py@edc78e25, ../honcho/mcp/src/config.ts@51497f1, ../honcho/mcp/README.md@51497f1, internal/cli/toolset_config.go
- Unblocks: MCP fake-server discovery + tool schema normalization, MCP OAuth state store + noninteractive auth errors
- Why now: Unblocks MCP fake-server discovery + tool schema normalization, MCP OAuth state store + noninteractive auth errors.

## 7. CLI banner/output formatting helpers

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: Pure CLI banner and output formatting helpers match upstream deterministic text behavior without terminal or command-registry coupling
- Trust class: operator, system
- Ready when: Only pure formatting helpers are needed; no command registry, config read/write, TTY prompt, or live gateway state is required.
- Not ready when: The slice opens a terminal, reads real config files, registers Cobra commands, or ports tips/webhook/dump helpers in the same change.
- Degraded mode: CLI command rows continue to render minimal fallback output until banner/version/toolset formatting helpers are fixture-backed.
- Fixture: `internal/cli/banner_output_test.go`
- Write scope: `internal/cli/banner.go`, `internal/cli/banner_output_test.go`, `internal/cli/output.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run 'Test.*Banner\|Test.*Output\|Test.*ToolsetName\|Test.*ContextLength' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Focused internal/cli fixtures prove banner and output helper formatting with no TTY, config, or command-registry dependency.
- Acceptance: FormatContextLength, DisplayToolsetName, and banner version labels match upstream golden fixtures., Output helpers return strings or writer-bound functions with deterministic newline and prompt behavior., Tests run without terminal/TTY detection, clock access, config files, or network calls.
- Source refs: ../hermes-agent/hermes_cli/banner.py@edc78e25, ../hermes-agent/hermes_cli/cli_output.py@edc78e25, internal/cli/parity.go, internal/cli/root_test.go
- Unblocks: CLI tips/dump/webhook deterministic helpers
- Why now: Unblocks CLI tips/dump/webhook deterministic helpers.

## 8. CLI profile path and active-profile store

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

## 9. Gateway management CLI read-model closeout

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
