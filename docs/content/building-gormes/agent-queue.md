---
title: "Agent Queue"
weight: 34
---

# Agent Queue

This page is generated from the canonical progress file:
`docs/content/building-gormes/architecture_plan/progress.json`.

It lists unblocked, non-umbrella contract rows that are ready for a focused
autonomous implementation attempt. Each card carries the execution owner,
slice size, contract, trust class, degraded-mode requirement, fixture target,
write scope, test commands, done signal, acceptance checks, and source
references.

Shared unattended-loop facts live in [Autoloop Handoff](./autoloop-handoff/):
the main entrypoint, orchestrator plan, candidate source, generated docs,
tests, and candidate policy. Keep those control-plane facts in
`meta.autoloop`, and keep row-specific execution facts in `progress.json`.

<!-- PROGRESS:START kind=agent-queue -->
## 1. Interrupted-turn memory sync suppression

- Phase: 3 / 3.E.7
- Owner: `memory`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Interrupted or cancelled turns cannot flush partial observations into GONCHO or external Honcho-compatible memory
- Trust class: system
- Ready when: The turn-finalization path can tell a normal completion from an interrupt, cancellation, or client disconnect.
- Not ready when: The slice rewrites extraction, recall, or provider-plugin storage instead of only gating sync/finalization on interrupted turns.
- Degraded mode: Memory status reports skipped or interrupted sync attempts without promoting partial facts to recall.
- Fixture: `internal/memory/interrupted_sync_test.go`
- Write scope: `internal/kernel/`, `internal/memory/`, `internal/goncho/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/kernel ./internal/memory ./internal/goncho -count=1`
- Done signal: Interrupted-turn fixtures prove partial memory observations are skipped while completed turns still sync.
- Acceptance: Interrupted turns record a skipped-sync reason and do not create new GONCHO conclusions., Completed turns still sync/extract normally., Operator status can distinguish skipped interrupted sync from extractor failures.
- Source refs: ../hermes-agent/agent/memory_manager.py, ../hermes-agent/tests/run_agent/test_memory_sync_interrupted.py, docs/content/building-gormes/architecture_plan/phase-3-memory.md
- Unblocks: Honcho host integration compatibility fixtures, Cross-chat operator evidence
- Why now: Unblocks Honcho host integration compatibility fixtures, Cross-chat operator evidence.

## 2. Honcho-compatible scope/source tool schema

- Phase: 3 / 3.E.7
- Owner: `memory`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Honcho-compatible tool schemas expose GONCHO scope and source allowlist controls without renaming public tools
- Trust class: operator, system
- Ready when: internal/goncho SearchParams and ContextParams already accept scope and sources fields.
- Not ready when: The slice renames public honcho_* tools, changes internal goncho storage, or bundles deny-path/operator evidence work.
- Degraded mode: Memory status and tool schema evidence show when user-scope or source-filtered recall is unavailable.
- Fixture: `internal/tools/honcho_tools_test.go`
- Write scope: `internal/tools/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools ./internal/goncho -count=1`
- Done signal: Honcho-compatible tool schema tests prove scope and sources are optional, discoverable, and routed through existing GONCHO params.
- Acceptance: honcho_search and honcho_context JSON Schemas include optional scope and sources fields., scope and sources are not required and omitted calls preserve same-chat default behavior., The internal implementation package and tables remain named goncho while external tool names remain honcho_*.
- Source refs: docs/content/upstream-hermes/gormes-takeaways.md, docs/content/building-gormes/architecture_plan/phase-3-memory.md, ../honcho/docs/v3/guides/integrations/hermes.mdx
- Unblocks: Cross-chat deny-path fixtures, Honcho host integration compatibility fixtures
- Why now: Unblocks Cross-chat deny-path fixtures, Honcho host integration compatibility fixtures.

## 3. Bedrock Converse payload mapping (no AWS SDK)

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

## 4. Codex Responses pure conversion harness

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

## 5. Tool-call argument repair + schema sanitizer

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

## 9. Tool registry inventory + schema parity harness

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
