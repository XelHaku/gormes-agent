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
## 1. WhatsApp unsafe identifier inbound evidence

- Phase: 2 / 2.B.5
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: WhatsApp inbound normalization uses the validated safety predicate to drop unsafe raw sender/chat/reply IDs with explicit whatsapp_identifier_unsafe evidence before Event.ChatID, Event.UserID, Reply.ChatID, alias graph entries, or outbound pairing targets are created
- Trust class: gateway, operator, system
- Ready when: WhatsApp identifier safety predicate is validated on main; workers can start from internal/channels/whatsapp/identity.go without rediscovering the helper., The worker should reuse the helper and extend inbound fixtures only; no platform runtime, bridge process, native WhatsApp client, or gateway manager dispatch is needed., Safe identity_contract.json fixtures must remain unchanged unless the test adds a separate unsafe fixture file.
- Not ready when: The slice rewrites safe identity fixtures, changes send/reconnect behavior, or accepts unsafe IDs after stripping dangerous characters., The slice opens live WhatsApp clients, writes Hermes lid-mapping files, or changes non-WhatsApp gateway session-key behavior.
- Degraded mode: Unsafe inbound WhatsApp IDs produce unresolved/degraded event evidence instead of a session key, reply target, or pairing target.
- Fixture: `internal/channels/whatsapp/identity_inbound_safety_test.go`
- Write scope: `internal/channels/whatsapp/identity.go`, `internal/channels/whatsapp/identity_inbound_safety_test.go`, `internal/channels/whatsapp/identity_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/channels/whatsapp -run '^TestNormalizeInboundWithIdentity_Unsafe\|^TestWhatsAppIdentifierSafetyPredicate_' -count=1`, `go test ./internal/channels/whatsapp -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Inbound safety fixtures prove unsafe WhatsApp sender/chat/alias values are rejected with whatsapp_identifier_unsafe evidence while safe identity contracts stay green.
- Acceptance: TestNormalizeInboundWithIdentity_UnsafeRawSenderRejected proves unsafe sender/user IDs return whatsapp_identifier_unsafe evidence and leave Event.UserID empty., TestNormalizeInboundWithIdentity_UnsafeChatRejected proves unsafe chat IDs do not produce Event.ChatID, Reply.ChatID, or an outbound pairing target., TestNormalizeInboundWithIdentity_UnsafeAliasEndpointRejected proves alias graph expansion skips unsafe endpoints while preserving safe aliases., Existing identity_contract.json fixtures still pass unchanged for safe bridge/native DM and group cases.
- Source refs: ../hermes-agent/gateway/whatsapp_identity.py@91512b82:expand_whatsapp_aliases path traversal guard, ../hermes-agent/gateway/whatsapp_identity.py@6993e566:_SAFE_IDENTIFIER_RE ASCII guard, internal/channels/whatsapp/identity.go:NormalizeInboundWithIdentity, internal/channels/whatsapp/testdata/identity_contract.json
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 2. Telegram require-mention config fields

- Phase: 2 / 2.B.5
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: internal/config parses Telegram require_mention and bot_username fields with disabled defaults so group mention gating remains opt-in and can be tested without constructing a Telegram bot runtime
- Trust class: gateway, operator, system
- Ready when: Telegram group bot-command mention gate helper is validated on main; config parsing can land independently before runtime binding., The worker only touches internal/config and uses temp TOML/string fixtures; no live Telegram token, BotFather username lookup, gateway.Manager, or provider runtime is required., Defaults must preserve current behavior: require_mention=false and bot_username empty.
- Not ready when: The slice edits internal/channels/telegram, starts a bot, changes allowed_chat_id or first_run_discovery, or wires runtime message dropping., The config loader enables require_mention by default or requires bot_username for DMs., The slice changes gateway pairing approval, channel session-key generation, fresh-final delete behavior, or command registry semantics.
- Degraded mode: When the fields are absent or malformed, Telegram group mention gating remains disabled with config evidence instead of changing DM, allowed-chat, or first-run discovery behavior.
- Fixture: `internal/config/config_test.go::TestLoad_TelegramRequireMentionFields`
- Write scope: `internal/config/config.go`, `internal/config/config_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/config -run TestLoad_TelegramRequireMentionFields -count=1`, `go test ./internal/config -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Config fixtures prove Telegram require_mention and bot_username parse with disabled defaults and no Telegram runtime edits.
- Acceptance: TestLoad_TelegramRequireMentionFields proves require_mention=true and bot_username="gormes_bot" parse into TelegramCfg., TestLoad_TelegramRequireMentionDefaults proves missing fields keep require_mention=false and bot_username empty., TestLoad_TelegramRequireMentionInvalidType proves malformed require_mention returns config evidence/error without enabling the gate., Existing Telegram config tests remain green and no internal/channels/telegram files change.
- Source refs: ../hermes-agent/gateway/platforms/telegram.py@3ff3dfb5:TelegramAdapter._should_process_message, ../hermes-agent/tests/gateway/test_telegram_group_gating.py@3ff3dfb5, internal/config/config.go:TelegramCfg
- Unblocks: Telegram group require-mention bot binding
- Why now: Unblocks Telegram group require-mention bot binding.

## 3. Durable worker RSS drain integration

- Phase: 2 / 2.E.3
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: DurableWorker integrates the validated RSS watchdog policy after job completion and on an injected periodic check, starts graceful drain, cancels in-flight handlers through existing abort-slot recovery, and records watchdog drain evidence
- Trust class: operator, system
- Ready when: Durable worker RSS watchdog policy helper is validated on main and exposes threshold/read-failure evidence., Durable worker abort-slot recovery safety net is complete, so handler cancellation and slot release already have ledger evidence., The worker can inject a fake RSS reader and fake ticker/channel; no real periodic sleep, process RSS, subprocess, or supervisor is required.
- Not ready when: The slice changes doctor/status rendering, cmd/gormes lifecycle, builder-loop backend watchdogs, live delegate_task behavior, cron execution semantics, or main-process termination., The slice reimplements abort-slot recovery instead of reusing the existing cancellation/grace path.
- Degraded mode: Durable-worker status reports rss_drain_started or rss_handler_abort_sent instead of wedging capacity after threshold evidence.
- Fixture: `internal/subagent/durable_worker_rss_drain_test.go`
- Write scope: `internal/subagent/durable_worker.go`, `internal/subagent/durable_worker_rss_watchdog.go`, `internal/subagent/durable_worker_rss_drain_test.go`, `internal/subagent/durable_worker_abort_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/subagent -run '^TestDurableWorkerRSSDrain_\|^TestDurableWorker_' -count=1`, `go test ./internal/subagent -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Durable worker fixtures prove post-job and injected periodic RSS drains cancel handlers through the existing abort path, preserve read-failure degradation, and avoid real RSS/sleeps/supervisors.
- Acceptance: TestDurableWorkerRSSDrain_PostJobThresholdAbortsSibling runs two fake jobs; after the quick job completes and injected RSS exceeds threshold, the sibling handler observes ctx cancellation and ledger/audit evidence records rss_handler_abort_sent., TestDurableWorkerRSSDrain_PeriodicZeroCompletionsTriggersDrain runs one fake handler that never completes; an injected periodic check exceeds threshold, starts drain, and records rss_drain_started without waiting for a post-job hook., TestDurableWorkerRSSDrain_ReadFailureDoesNotCancel proves rss_watchdog_unavailable evidence does not cancel handlers., Existing DurableWorker abort-slot recovery tests remain green without sleeps longer than 100ms.
- Source refs: ../gbrain/src/core/minions/worker.ts@c78c3d0:gracefulShutdown, ../gbrain/test/minions.test.ts@c78c3d0:graceful memory shutdown, internal/subagent/durable_worker.go, internal/subagent/durable_worker_abort_test.go, internal/subagent/durable_worker_rss_watchdog.go
- Unblocks: Durable worker RSS doctor/status evidence
- Why now: Unblocks Durable worker RSS doctor/status evidence.

## 4. Steer slash command registry + queue fallback

- Phase: 2 / 2.F.5
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Contract: Registry-owned active-turn steering command
- Trust class: operator, gateway
- Ready when: Steer slash command parser + preview helper is validated on main., The parser helper is already complete on main, so workers should start from internal/gateway/steer_command.go and add only registry/queue behavior in this row., This slice only registers /steer and queue fallback behavior; the live between-tool-call injection hook remains in the dependent row., Tests can use fake running-agent state and fake command dispatch; no provider, active tool loop, TUI, Slack, or Telegram transport is required.
- Not ready when: The implementation tries to inject mid-run prompts instead of only registering /steer and queue fallback behavior., The slice changes /queue, /bg, /busy config persistence, TUI keybindings, or platform adapter code., The parser row is not present in the worker checkout.
- Degraded mode: Gateway returns visible usage, busy, steer_unavailable, or queued status instead of dropping steer text when the command cannot run immediately. Backend usage-limit/stdin outages are now handled by the validated run-level health bypass, so feature workers should not re-block this row on quota noise.
- Fixture: `internal/gateway/steer_queue_test.go::TestSteerCommandRegistry_*`
- Write scope: `internal/gateway/commands.go`, `internal/gateway/manager.go`, `internal/gateway/steer_command.go`, `internal/gateway/steer_queue_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/gateway -run '^TestSteerCommandRegistry_' -count=1`, `go test ./internal/gateway -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Gateway command fixtures prove /steer registry exposure, parsed queue fallback, running-agent degraded evidence, and no live mid-run injection.
- Acceptance: TestSteerCommandRegistry_RegisteredAsBusyAware exposes /steer through the shared registry without changing existing command names., TestSteerCommandRegistry_NoRunningAgentQueuesGuidance queues parsed follow-up guidance and returns the parser preview., TestSteerCommandRegistry_RunningAgentFallbackDoesNotInject proves running-agent paths return explicit steer_unavailable/queued evidence until the mid-run hook row lands., Existing gateway command parsing and active-session policy tests remain green.
- Source refs: ../hermes-agent/cli.py@635253b9:busy_input_mode=steer, ../hermes-agent/gateway/run.py@635253b9:running_agent.steer, ../hermes-agent/tests/gateway/test_busy_session_ack.py@635253b9, internal/gateway/commands.go, internal/gateway/manager.go, internal/gateway/steer_command.go
- Unblocks: Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard
- Why now: Unblocks Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard.

## 5. ContextEngine compression-boundary notification

- Phase: 4 / 4.B
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Kernel compression execution signals the ContextEngine when a compression boundary is crossed so cached/replayed context state cannot silently span the pre-compress and post-compress transcript
- Trust class: operator, system
- Ready when: ContextEngine interface + status tool contract is validated on main., Compression token-budget and single-prompt threshold fixtures are validated, so this slice only adds boundary notification semantics and does not invent compression budgeting., A fake ContextEngine can record boundary notifications without calling a live provider or mutating real transcript storage., internal/kernel/context_engine.go owns the kernel-side context-engine adapter seam; workers should not edit internal/kernel/kernel.go for this row.
- Not ready when: The slice implements summarization, tool-result pruning, manual compression, or context references instead of the boundary callback., The slice hides boundary failures or lets a context cache survive across a compression lineage change without observable status evidence., The slice edits internal/kernel/kernel.go, internal/hermes/context_compressor_budget.go, transcript storage, or session lineage helpers instead of the context-engine adapter files named in write_scope., The slice changes Goncho/Honcho memory extraction semantics; memory pre/post-compression observation remains a separate Phase 3/4 concern.
- Degraded mode: Context status reports compression_boundary_unavailable or last_boundary_missing evidence instead of implying context-engine caches were reset after compression. Backend quota/stdin outages are now run-level degradation and should not block this adapter-only row.
- Fixture: `internal/kernel/compression_boundary_test.go::TestKernelCompressionBoundary_*`
- Write scope: `internal/hermes/context_engine.go`, `internal/hermes/context_engine_test.go`, `internal/kernel/context_engine.go`, `internal/kernel/contextengine_test.go`, `internal/kernel/compression_boundary_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/kernel -run 'Test.*CompressionBoundary\|Test.*ContextEngine' -count=1`, `go test ./internal/hermes ./internal/kernel -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Fake ContextEngine fixtures prove successful compression emits one boundary notification with lineage evidence, skipped/failed compression does not, and context status exposes degraded evidence.
- Acceptance: A fake ContextEngine receives exactly one CompressionBoundary notification after a successful compression result is accepted into the kernel transcript lineage., A failed or skipped compression path does not emit the boundary notification and reports compression_boundary_unavailable in context status., The notification includes old_session_id/new_session_id or equivalent lineage evidence so caches can distinguish pre-compress and post-compress context., Existing context_status tool fixtures remain stable except for the added boundary evidence fields.
- Source refs: ../hermes-agent/run_agent.py@e85b7525, ../hermes-agent/tests/run_agent/test_compression_boundary_hook.py@e85b7525, internal/hermes/context_engine.go, internal/kernel/context_engine.go, internal/kernel/contextengine_test.go, internal/hermes/context_compressor_budget.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 6. Title prompt and truncation contract

- Phase: 4 / 4.F
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Contract: Native title generation exposes a pure request/response boundary that builds Hermes-compatible title prompts from bounded session history, truncates candidate titles deterministically, returns empty-title fallback evidence for empty history or blank model output, and surfaces provider failures through a typed nonfatal error result without writing session metadata
- Trust class: operator, system
- Ready when: TUI prompt-submit auto-title eligibility helper is validated, so callers can produce a TitleRequest without starting provider work inside the TUI update loop., The worker can use a fake title model function and synthetic history; no provider credential, goroutine, TUI program, or session DB write is required.
- Not ready when: The slice writes session.Metadata, starts background title workers, changes TUI submit behavior, or calls a live LLM., The slice swallows provider failures without returning typed evidence that the CLI/gateway can surface later., The slice mutates transcript history or uses Goncho/Honcho memory as a title source.
- Degraded mode: Title status reports auto_title_skipped, title_provider_failed, or title_blank_result evidence instead of silently leaving NULL titles with no operator-visible cause.
- Fixture: `internal/hermes/title_generator_test.go::TestTitle*`
- Write scope: `internal/hermes/title_generator.go`, `internal/hermes/title_generator_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run '^TestTitle' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Title generator fixtures prove bounded prompt construction, candidate cleanup/truncation, empty-history skip, blank-output evidence, and provider-failure evidence with a fake model only.
- Acceptance: TestTitlePrompt_BuildsFromBoundedHistory proves only the configured recent user/assistant turns enter the prompt and long content is truncated before model invocation., TestTitleGenerator_TruncatesAndCleansCandidate proves whitespace/newline/quote cleanup plus deterministic max-title-length truncation., TestTitleGenerator_EmptyHistorySkipsModel returns skipped evidence and never calls the fake model., TestTitleGenerator_BlankModelOutput returns title_blank_result evidence without writing metadata., TestTitleGenerator_ProviderFailureReturnsTypedEvidence proves the fake model error is returned as nonfatal title_provider_failed evidence.
- Source refs: ../hermes-agent/agent/title_generator.py@4a2ee6c1:generate_title, ../hermes-agent/agent/title_generator.py@4a2ee6c1:maybe_auto_title, ../hermes-agent/tests/agent/test_title_generator.py@4a2ee6c1, internal/tui/auto_title.go, internal/session/, internal/transcript/
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 7. Session search tool schema and argument validation

- Phase: 5 / 5.N
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: internal/tools defines a session_search Tool descriptor, JSON schema, timeout, and argument validator for query, scope, sources, mode, limit, and current_session_id without registering the tool globally or reading memory
- Trust class: operator, child-agent, system
- Ready when: internal/tools.Tool is stable and Honcho-compatible scope/source schema language is validated in internal/gonchotools., This slice is descriptor/argument-only: no memory store, SQLite fixture, global registry binding, gateway runtime, or Goncho service construction is required., The tool name must be session_search; honcho_* external tool names remain reserved for Goncho/Honcho compatibility.
- Not ready when: The slice calls internal/memory.SearchSessions/SearchMessages, changes ranking, opens SQLite, or registers the tool in builtin/global toolsets., The schema renames public Goncho/Honcho tools or removes same-chat defaults from the later execution row., The slice includes todo/debug/clarify tools or cronjob tools.
- Degraded mode: Invalid input returns session_search_invalid_args evidence instead of widening recall or falling back to global search.
- Fixture: `internal/tools/session_search_tool_schema_test.go::TestSessionSearchToolSchema_*`
- Write scope: `internal/tools/session_search_tool.go`, `internal/tools/session_search_tool_schema_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools -run '^TestSessionSearchToolSchema_' -count=1`, `go test ./internal/tools -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: session_search descriptor and argument fixtures pass without SQLite, memory ranking, Goncho service construction, or global registry binding.
- Acceptance: TestSessionSearchToolSchema_Descriptor proves Name()=="session_search", Description is non-empty, Timeout is deterministic, and Schema exposes query/scope/sources/mode/limit/current_session_id., TestSessionSearchToolSchema_DefaultArgs proves omitted scope/sources/mode normalize to same-chat/default mode without memory access., TestSessionSearchToolSchema_RejectsUnsafeScope proves unknown scope, unknown mode, negative limit, and non-string sources return session_search_invalid_args evidence., TestSessionSearchToolSchema_NotRegisteredGlobally proves the row does not edit internal/tools/builtin.go or register the tool outside the local test registry.
- Source refs: ../hermes-agent/tools/session_search_tool.py@dbe50155, ../hermes-agent/tests/tools/test_session_search.py@dbe50155, internal/tools/tool.go, internal/gonchotools/honcho_tools.go:HonchoSearchTool.Schema
- Unblocks: Session search tool execution wrapper
- Why now: Unblocks Session search tool execution wrapper.

## 8. CLI OpenClaw residue detection and hint text

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: internal/cli exposes pure DetectOpenClawResidue(home string) bool and OpenClawResidueHint(commandName string) string helpers that detect only an existing ~/.openclaw directory and return Gormes-specific cleanup guidance without reading or writing config files
- Trust class: operator, system
- Ready when: internal/cli already has pure helper files and tests; this slice adds a sibling helper without command registration, config I/O, or startup wiring., Use Gormes-facing text such as the command name injected by tests; do not copy Hermes `hermes claw cleanup` wording., Filesystem checks use a temp HOME provided by tests; no real operator home directory is inspected.
- Not ready when: The slice edits cmd/gormes startup, reads/writes config files, renames ~/.openclaw, migrates OpenClaw data, or ports the full optional migration script., The hint text mentions Hermes commands, HERMES_HOME, or writes under upstream Hermes paths., The helper treats a regular file named .openclaw as residue.
- Degraded mode: If HOME cannot be inspected, the helper returns false and never blocks CLI startup; startup binding can decide later whether to persist a seen flag.
- Fixture: `internal/cli/openclaw_residue_test.go::TestOpenClawResidue*`
- Write scope: `internal/cli/openclaw_residue.go`, `internal/cli/openclaw_residue_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run '^Test(OpenClawResidue\|DetectOpenClawResidue)' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: CLI fixtures prove directory-only OpenClaw residue detection and Gormes-specific cleanup hint text without real HOME/config writes.
- Acceptance: TestDetectOpenClawResidue_DirectoryOnly returns true for a temp HOME containing a .openclaw directory and false for missing path or a regular file., TestDetectOpenClawResidue_UnreadableHomeReturnsFalse proves stat errors degrade to false without panic., TestOpenClawResidueHint_MentionsInjectedCleanupCommand proves the hint contains ~/.openclaw, the injected Gormes cleanup command, and no `hermes claw cleanup` substring., No test touches the real user home, config.yaml, or cmd/gormes startup.
- Source refs: ../hermes-agent/agent/onboarding.py@e63929d4:detect_openclaw_residue,openclaw_residue_hint_cli, ../hermes-agent/tests/agent/test_onboarding.py@e63929d4:TestOpenClawResidue, ../hermes-agent/cli.py@e63929d4:first-time OpenClaw-residue banner, internal/cli/tips.go
- Unblocks: CLI onboarding seen-state map helpers
- Why now: Unblocks CLI onboarding seen-state map helpers.

## 9. Custom provider model-switch key_env write guard

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: internal/cli exposes a pure model-switch patch helper that updates a custom provider default_model while preserving original credential storage: providers that relied on key_env and had no inline api_key/api_key_ref must not gain an api_key entry, while providers that already had an inline plaintext or `${VAR}` api_key may keep that existing api_key value without writing resolved plaintext
- Trust class: operator, system
- Ready when: Custom provider model-switch credential preservation is validated on main and provides the resolver vocabulary for env-template, plaintext, key_env, unset, and missing credentials., This slice only adds a pure patch/model-switch helper under internal/cli; no config reader, /model command handler, TUI picker, fake /v1/models server, provider routing, or cmd/gormes wiring is required., Table tests should construct input provider maps/structs in memory and assert the planned write shape; no process environment, filesystem, or network access is needed.
- Not ready when: The slice changes internal/config, internal/hermes, provider catalog probing, TUI model picker behavior, or command wiring., The helper writes an api_key field for a provider whose original config relied only on key_env., The helper writes resolved plaintext when the original provider used `${VAR}` or key_env references.
- Degraded mode: Model-switch planning returns credential_write_skipped_key_env, credential_ref_preserved, plaintext_preserved, or credential_missing evidence so setup/status surfaces can explain why api_key was not written. The credential-preservation prerequisite and backend health bypass are now validated, so this row should run as a pure internal/cli patch-helper fixture.
- Fixture: `internal/cli/custom_provider_model_switch_test.go::TestCustomProviderModelSwitchPatch_*`
- Write scope: `internal/cli/custom_provider_model_switch.go`, `internal/cli/custom_provider_model_switch_test.go`, `internal/cli/custom_provider_secret.go`, `internal/cli/custom_provider_secret_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run 'TestCustomProviderModelSwitchPatch_\|TestResolveCustomProviderSecret_' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: internal/cli custom-provider model-switch fixtures prove key_env-backed providers update default_model without adding api_key, existing inline references/plaintext are preserved without resolution, and resolver tests still pass.
- Acceptance: TestCustomProviderModelSwitchPatch_KeyEnvDoesNotSynthesizeAPIKey starts with {default_model:'old', key_env:'ACME_KEY'} and proves the patch sets default_model='new', preserves key_env, omits api_key, and returns credential_write_skipped_key_env evidence., TestCustomProviderModelSwitchPatch_InlineEnvRefPreserved starts with {api_key:'${ACME_KEY}'} and proves the patch keeps api_key='${ACME_KEY}' without resolving or overwriting it., TestCustomProviderModelSwitchPatch_PlaintextPreserved starts with {api_key:'sk-plain'} and proves plaintext is preserved only because it was already present., TestCustomProviderModelSwitchPatch_MissingCredentialStillUpdatesModelWithEvidence proves model changes remain possible while credential_missing evidence is returned for setup/status guidance., Existing TestResolveCustomProviderSecret_* fixtures remain green; this row does not redefine resolver semantics.
- Source refs: ../hermes-agent/hermes_cli/main.py@8258f4dc:_model_flow_named_custom, ../hermes-agent/tests/hermes_cli/test_custom_provider_model_switch.py@8258f4dc, ../hermes-agent/hermes_cli/main.py@8bbeaea6:_named_custom_provider_map, internal/cli/custom_provider_secret.go, internal/cli/custom_provider_secret_test.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 10. Native TUI conversation viewport tail helper

- Phase: 5 / 5.Q
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: internal/tui exposes a pure conversation viewport helper that clips RenderFrame.History to the visible tail under width/height budgets, emits a deterministic omitted-history sentinel, and always preserves DraftText and LastError inputs
- Trust class: operator, system
- Ready when: internal/tui/view.go currently renders every RenderFrame.History message into one joined string; the worker can add a pure helper and table tests without changing kernel history, persistence, or provider streaming., Synthetic RenderFrame fixtures with 100+ messages are enough; no Bubble Tea program, terminal, Node/Ink runtime, or Hermes profiling script needs to run., This row is a Gormes-native performance guard, not a port of Hermes React/Ink virtualization internals.
- Not ready when: The slice imports React/Ink concepts, starts Node, changes kernel.RenderFrame shape, truncates stored session/transcript history, or edits kernel/session/transcript persistence., The slice wires renderConv integration, slash-command, remote TUI SSE, API server, or dashboard behavior in the same diff., The helper silently drops DraftText or LastError when history is long.
- Degraded mode: If height is tiny or width is narrow, the helper renders the latest visible turn plus compact draft/error/sentinel evidence instead of panicking or allocating the full history body.
- Fixture: `internal/tui/viewport_history_test.go::TestConversationViewportTail_*`
- Write scope: `internal/tui/view.go`, `internal/tui/viewport_history_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tui -run '^TestConversationViewportTail_' -count=1`, `go test ./internal/tui -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Native TUI viewport helper fixtures prove bounded tail rendering, omitted-count sentinel, draft/error preservation, and small-size clamps without render integration changes.
- Acceptance: TestConversationViewportTail_OmitsEarlierHistory builds 120 alternating user/assistant messages and asserts latest turns remain, earliest turn body is excluded, and a deterministic omitted-history sentinel includes the hidden count., TestConversationViewportTail_AlwaysIncludesDraftAndLastError proves DraftText and LastError survive when history is clipped., TestConversationViewportTail_HeightAndWidthClamp proves width<4 and tiny height do not panic and still render a compact latest-message view., TestConversationViewportTail_RenderedLineBudget asserts helper output stays within the requested visible budget plus a small sentinel allowance.
- Source refs: ../hermes-agent/ui-tui/src/hooks/useVirtualHistory.ts@e63929d4, ../hermes-agent/ui-tui/src/lib/virtualHeights.ts@e63929d4, ../hermes-agent/ui-tui/src/__tests__/virtualHeights.test.ts@e63929d4, ../hermes-agent/scripts/profile-tui.py@e63929d4, internal/tui/view.go:renderConv
- Unblocks: Native TUI renderConv viewport budget binding
- Why now: Unblocks Native TUI renderConv viewport budget binding.

<!-- PROGRESS:END -->
