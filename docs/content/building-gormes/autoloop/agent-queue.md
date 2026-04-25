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
## 1. DeepSeek/Kimi reasoning_content echo for tool-call replay

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Contract: Thinking-mode providers that require reasoning_content on assistant tool-call turns receive an echoed value during persistence and API replay
- Trust class: system
- Ready when: Shared provider continuation fixtures can serialize assistant tool-call messages and replay them without live provider credentials.
- Not ready when: The slice stores hidden reasoning text in ordinary assistant content or changes non-thinking providers' replay payloads.
- Degraded mode: Provider status explains when a thinking-mode provider requires reasoning echo padding and when a stored transcript was repaired for replay.
- Fixture: `internal/hermes/reasoning_content_echo_test.go`
- Write scope: `internal/hermes/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -count=1`
- Done signal: Reasoning echo fixtures prove DeepSeek and Kimi tool-call replays include provider-required reasoning_content without mutating ordinary assistant content.
- Acceptance: DeepSeek is detected by provider name, model substring, or api.deepseek.com host and gets reasoning_content="" on assistant tool-call replay when no reasoning exists., Kimi/Moonshot detection keeps the existing reasoning_content padding behavior., Explicit reasoning_content or reasoning fields are preserved, while non-tool assistant turns and non-thinking providers are left untouched.
- Source refs: upstream Hermes d58b305a, ../hermes-agent/run_agent.py, ../hermes-agent/tests/run_agent/test_deepseek_reasoning_content_echo.py, docs/content/building-gormes/architecture_plan/phase-4-brain-transplant.md
- Unblocks: Cross-provider reasoning-tag sanitization, OpenRouter, Codex stream repair + tool-call leak sanitizer
- Why now: Unblocks Cross-provider reasoning-tag sanitization, OpenRouter, Codex stream repair + tool-call leak sanitizer.

## 2. Bedrock Converse payload mapping (no AWS SDK)

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Contract: Pure Bedrock Converse request mapping over the shared provider message/tool contract
- Trust class: system
- Ready when: Provider interface + stream fixture harness and tool-call continuation contract are complete.
- Not ready when: The slice imports AWS SDK clients or signs live requests before pure request-body mapping is fixture-locked.
- Degraded mode: Provider status reports Bedrock as unavailable until request mapping fixtures pass and credential wiring lands.
- Fixture: `internal/hermes/bedrock_converse_mapping_test.go`
- Write scope: `internal/hermes/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -count=1`
- Done signal: Bedrock request-body golden fixtures prove Converse mapping without AWS credentials or SDK clients.
- Acceptance: System, user, assistant, and tool-result messages map to Bedrock Converse roles and content blocks., Tool definitions map to Bedrock toolSpec inputSchema without dropping required fields., Golden request fixtures pin max_tokens, temperature, cache/reasoning passthrough, and empty-content placeholders.
- Source refs: ../hermes-agent/agent/bedrock_adapter.py, ../hermes-agent/agent/transports/bedrock.py, ../hermes-agent/tests/agent/test_bedrock_adapter.py, docs/content/building-gormes/architecture_plan/phase-4-brain-transplant.md
- Unblocks: Bedrock stream event decoding (SSE fixtures)
- Why now: Unblocks Bedrock stream event decoding (SSE fixtures).

## 3. Codex Responses pure conversion harness

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Contract: OpenAI Responses request/response conversion for Codex-compatible providers without live OAuth
- Trust class: system
- Ready when: Shared provider transcript and tool-call continuation fixtures are complete.
- Not ready when: The slice performs OAuth/device login, imports ~/.codex/auth.json, or opens a live Responses request.
- Degraded mode: Provider status reports Codex unavailable until Responses conversion fixtures pass and auth wiring is configured.
- Fixture: `internal/hermes/codex_responses_adapter_test.go`
- Write scope: `internal/hermes/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -count=1`
- Done signal: Codex Responses fixtures convert chat input, tool schemas, output items, usage, and tool calls without live credentials.
- Acceptance: Chat messages and multimodal content parts convert to Responses input items deterministically., Function tools convert to Responses function-tool schemas with deterministic call IDs., Responses output items normalize back to shared provider events, messages, usage, and tool calls.
- Source refs: ../hermes-agent/agent/codex_responses_adapter.py, ../hermes-agent/tests/run_agent/test_run_agent_codex_responses.py, ../hermes-agent/tests/run_agent/test_tool_call_args_sanitizer.py, docs/content/building-gormes/architecture_plan/phase-4-brain-transplant.md
- Unblocks: Codex stream repair + tool-call leak sanitizer, Codex OAuth state + stale-token relogin
- Why now: Unblocks Codex stream repair + tool-call leak sanitizer, Codex OAuth state + stale-token relogin.

## 4. Tool-call argument repair + schema sanitizer

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Contract: Provider tool-call arguments are repaired or rejected against available tool schemas before execution
- Trust class: system, child-agent
- Ready when: Shared provider tool-call continuation fixtures are complete and tool descriptors expose required argument schemas.
- Not ready when: The slice silently invents missing required arguments or changes tool executor behavior instead of validating provider output.
- Degraded mode: Tool execution status reports schema-repair failures before a malformed provider call reaches the executor.
- Fixture: `internal/hermes/tool_call_argument_repair_test.go`
- Write scope: `internal/hermes/`, `internal/tools/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes ./internal/tools -count=1`
- Done signal: Tool-call repair fixtures prove malformed arguments are repaired or rejected before execution using the advertised schema.
- Acceptance: Malformed JSON argument fragments from streamed tool calls are repaired only when the repair is deterministic., Impossible repairs return a provider/tool-call error before execution., Repair decisions use the current advertised tool schema so disabled or unavailable tools cannot be hallucinated into execution.
- Source refs: ../hermes-agent/tests/run_agent/test_repair_tool_call_arguments.py, ../hermes-agent/tests/run_agent/test_streaming_tool_call_repair.py, ../hermes-agent/tests/run_agent/test_tool_call_args_sanitizer.py, ../hermes-agent/tools/schema_sanitizer.py
- Unblocks: Codex stream repair + tool-call leak sanitizer, OpenRouter, Bedrock stream event decoding (SSE fixtures)
- Why now: Unblocks Codex stream repair + tool-call leak sanitizer, OpenRouter, Bedrock stream event decoding (SSE fixtures).

## 5. Provider-enforced context-length resolver

- Phase: 4 / 4.D
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Contract: Displayed and budgeted context windows prefer provider-enforced limits over raw models.dev metadata
- Trust class: operator, system
- Ready when: Provider status and model metadata can be tested as pure functions without live provider credentials.
- Not ready when: The slice implements routing/fallback policy or pulls live models.dev/network data during unit tests.
- Degraded mode: Model status reports whether the context length came from provider-specific caps, models.dev fallback, or an unknown model.
- Fixture: `internal/hermes/model_context_resolver_test.go`
- Write scope: `internal/hermes/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -count=1`
- Done signal: Context resolver fixtures prove provider caps, models.dev fallback, and unknown-model reporting are deterministic without network calls.
- Acceptance: openai-codex gpt-5.5 displays and budgets the provider cap (272000 tokens) instead of the raw models.dev 1050000-token window., Provider-specific caps for Codex OAuth, Copilot, and Nous win over model-info fallbacks when present., Unknown resolver failures fall back to fixture model metadata and report unknown when both sources are empty.
- Source refs: upstream Hermes 05d8f110, ../hermes-agent/hermes_cli/model_switch.py, ../hermes-agent/cli.py, ../hermes-agent/gateway/run.py, ../hermes-agent/tests/hermes_cli/test_model_switch_context_display.py
- Unblocks: Compression token-budget trigger + summary sizing, Routing policy and fallback selector
- Why now: Unblocks Compression token-budget trigger + summary sizing, Routing policy and fallback selector.

## 6. Skill preprocessing + dynamic slash commands

- Phase: 5 / 5.F
- Owner: `skills`
- Size: `small`
- Status: `planned`
- Contract: Skill content preprocessing and skill-backed slash commands are deterministic, disabled-skill aware, and prompt-safe
- Trust class: operator, gateway, system
- Ready when: Phase 2.G parser/store and inactive candidate flow are complete.
- Not ready when: Inline shell preprocessing can execute during prompt assembly or disabled skills remain invokable through slash commands.
- Degraded mode: Skill status reports disabled, missing-prerequisite, or preprocessing-failed skills without injecting them into prompts.
- Fixture: `internal/skills/preprocessing_commands_test.go`
- Write scope: `internal/skills/`, `internal/gateway/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/skills ./internal/gateway -count=1`
- Done signal: Skill preprocessing and slash-command fixtures prove disabled/incompatible skills do not enter prompt or command surfaces.
- Acceptance: Template variable preprocessing is deterministic and fixture-covered., Inline shell preprocessing is disabled by default and bounded when explicitly enabled., Skill slash commands skip disabled/incompatible skills and build stable user-message content.
- Source refs: ../hermes-agent/agent/skill_preprocessing.py, ../hermes-agent/agent/skill_commands.py, ../hermes-agent/tools/skills_tool.py, ../hermes-agent/tests/tools/test_skills_tool.py, ../hermes-agent/tests/agent/test_skill_commands.py
- Unblocks: Toolset-aware skills prompt snapshot, TUI + Telegram browsing
- Why now: Unblocks Toolset-aware skills prompt snapshot, TUI + Telegram browsing.

## 7. PTY bridge protocol adapter

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: Dashboard/TUI PTY sessions expose bounded read, write, resize, close, and unavailable-state behavior through a testable adapter
- Trust class: operator
- Ready when: Deterministic CLI helper ports are understood and PTY behavior can be isolated from the web dashboard transport.
- Not ready when: The slice starts the web dashboard, opens network listeners, or binds to a real TUI process in unit tests.
- Degraded mode: Dashboard or CLI status reports PTY unavailable instead of falling back to unsafe shell execution.
- Fixture: `internal/cli/pty_bridge_test.go`
- Write scope: `internal/cli/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -count=1`
- Done signal: PTY bridge fixtures prove read/write/resize/close/unavailable behavior without network or live dashboard dependencies.
- Acceptance: Reads are bounded by timeout and chunk size., Writes and resize messages validate input before reaching the PTY., Unsupported platforms return PtyUnavailable-style errors without starting a shell.
- Source refs: ../hermes-agent/hermes_cli/pty_bridge.py, ../hermes-agent/tests/hermes_cli/test_pty_bridge.py, ../hermes-agent/hermes_cli/web_server.py
- Unblocks: SSE streaming to Bubble Tea TUI, Dashboard PTY chat sidecar contract
- Why now: Unblocks SSE streaming to Bubble Tea TUI, Dashboard PTY chat sidecar contract.

## 8. OpenAI-compatible chat-completions API server

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `medium`
- Status: `planned`
- Contract: OpenAI-compatible chat.completions HTTP surface over the native Gormes turn loop
- Trust class: operator, gateway
- Ready when: Native kernel turn loop, gateway session handles, and provider event streaming are stable enough for HTTP replay fixtures.
- Not ready when: The server shells out to Python api_server or creates a second session store instead of using native Gormes state.
- Degraded mode: API health and error envelopes report auth, body-size, content-normalization, and streaming failures without starting hidden sessions.
- Fixture: `internal/apiserver/chat_completions_test.go`
- Write scope: `internal/gateway/`, `internal/kernel/`, `internal/apiserver/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/gateway ./internal/kernel ./internal/apiserver -count=1`
- Done signal: Chat-completions HTTP fixtures prove auth, body limits, content normalization, streaming envelopes, and session continuity over native Gormes state.
- Acceptance: Bearer/API-key auth, request body limits, and OpenAI error envelopes are fixture-covered., Chat content parts normalize to the same user-message shape used by gateway sessions., Streaming and non-streaming responses include stable X-Hermes-Session-Id continuity.
- Source refs: ../hermes-agent/gateway/platforms/api_server.py, ../hermes-agent/tests/gateway/test_api_server.py, docs/content/upstream-hermes/user-guide/features/api-server.md, docs/content/building-gormes/architecture_plan/phase-5-final-purge.md
- Unblocks: Responses API store + run event stream, Gateway proxy mode forwarding contract, Dashboard API client contract
- Why now: Unblocks Responses API store + run event stream, Gateway proxy mode forwarding contract, Dashboard API client contract.

## 9. BlueBubbles iMessage bubble formatting parity

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

## 10. Tool registry inventory + schema parity harness

- Phase: 5 / 5.A
- Owner: `tools`
- Size: `medium`
- Status: `planned`
- Contract: Operation and tool descriptor parity before handler ports
- Trust class: operator, gateway, child-agent, system
- Ready when: Upstream tool descriptor inventory can be captured without porting handlers in the same slice.
- Not ready when: Handler implementation starts before descriptor parity fixtures exist.
- Degraded mode: Doctor reports disabled tools, missing dependencies, schema drift, and unavailable provider-specific paths.
- Fixture: `internal/tools upstream schema parity manifest fixtures`
- Write scope: `internal/tools/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools -count=1`
- Done signal: Tool descriptor parity fixtures capture names, schemas, trust classes, dependencies, and degraded status before handler ports.
- Acceptance: Upstream tool names, toolsets, required env vars, schemas, result envelopes, trust classes, and degraded status are captured in fixtures., No handler port can mark complete until its descriptor parity row exists., Doctor can report missing dependencies or disabled provider-specific paths.
- Source refs: docs/content/upstream-hermes/reference/tools-reference.md, docs/content/building-gormes/architecture_plan/phase-5-final-purge.md
- Unblocks: Pure core tools first, Stateful tool migration queue, CLI command registry parity + active-turn busy policy
- Why now: Unblocks Pure core tools first, Stateful tool migration queue, CLI command registry parity + active-turn busy policy.

<!-- PROGRESS:END -->
