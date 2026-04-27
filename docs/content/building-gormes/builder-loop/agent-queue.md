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

## 2. Telegram group mention gate config binding

- Phase: 2 / 2.B.5
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: Telegram runtime can opt into Hermes-style group require-mention policy by using the validated bot-command mention helper to drop unaddressed group text and bare slash commands while leaving DMs, allowed-chat gating, first-run discovery, and fresh-final streaming unchanged
- Trust class: gateway, operator, system
- Ready when: Telegram group bot-command mention gate helper is validated on main; workers can bind config/runtime behavior directly against internal/channels/telegram/group_mention.go., The binding can be tested through synthetic tgbotapi.Update values and Config fields only; no live Telegram token, BotFather username lookup, gateway.Manager, pairing store, or provider runtime is required., The worker should keep group gating opt-in and leave existing allowed_chat_id / first_run_discovery tests unchanged.
- Not ready when: The slice changes gateway pairing approval, channel session-key generation, fresh-final deleteMessage behavior, Slack/Discord policy, or command registry semantics., The slice requires a live getMe call or network-derived bot username in unit tests., The slice drops DMs or explicitly mentioned group commands when require_mention is disabled.
- Degraded mode: Telegram status reports telegram_group_mention_gate_disabled, telegram_group_mention_gate_unavailable, or telegram_group_message_unaddressed instead of silently processing unaddressed group traffic.
- Fixture: `internal/channels/telegram/group_mention_binding_test.go`
- Write scope: `internal/config/config.go`, `internal/config/config_test.go`, `internal/channels/telegram/bot.go`, `internal/channels/telegram/bot_test.go`, `internal/channels/telegram/group_mention.go`, `internal/channels/telegram/group_mention_binding_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/config -run TestLoad_TelegramRequireMentionFields -count=1`, `go test ./internal/channels/telegram -run '^TestBot_ToInboundEvent_Group\|^TestBot_ToInboundEvent_DMBypassesMentionGate' -count=1`, `go test ./internal/config ./internal/channels/telegram -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Config and Telegram adapter fixtures prove opt-in group require-mention policy gates bare group commands, accepts `/cmd@botname` bot_command entities, preserves DM behavior, and leaves existing Telegram streaming/command tests green.
- Acceptance: TestLoad_TelegramRequireMentionFields proves TelegramCfg parses require_mention and bot_username with safe defaults that preserve current behavior., TestBot_ToInboundEvent_GroupBareCommandDroppedWhenRequireMentionEnabled proves a group `/status` command without @botname is dropped when require_mention=true., TestBot_ToInboundEvent_GroupBotCommandSuffixAccepted proves `/status@gormes_bot` with a bot_command entity reaches gateway.ParseInboundText when configured bot_username matches., TestBot_ToInboundEvent_DMBypassesMentionGate proves private chats still process commands/text without mention., Existing Telegram fresh-final delete and command parsing tests keep passing.
- Source refs: ../hermes-agent/gateway/platforms/telegram.py@3ff3dfb5:TelegramAdapter._should_process_message, ../hermes-agent/tests/gateway/test_telegram_group_gating.py@3ff3dfb5, internal/channels/telegram/group_mention.go, internal/channels/telegram/bot.go, internal/config/config.go:TelegramCfg
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 3. Gateway inbound dedup evidence wiring

- Phase: 2 / 2.B.5
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Gateway manager applies the shared MessageDeduplicator to inbound events with stable message IDs, drops duplicate submissions with visible evidence, and degrades missing message IDs without suppressing the turn
- Trust class: gateway, system
- Ready when: Gateway message deduplicator bounded helper is validated on main and exposes duplicate/evicted/disabled evidence., Shared gateway inbound event normalization is validated and platform adapters already pass stable message IDs where available., The worker can use fake gateway.Channel and fake kernel fixtures only; no Telegram, Slack, Discord, or provider SDK is needed.
- Not ready when: The slice changes channel session keys, authorization/pairing policy, message rendering, outbound coalescing, or platform-specific adapter state., The slice stores dedup history on disk or treats empty message IDs as duplicates.
- Degraded mode: Gateway status reports dedup_unavailable for missing IDs and duplicate_message for dropped repeats instead of silently replaying duplicate platform events.
- Fixture: `internal/gateway/message_deduplicator_manager_test.go`
- Write scope: `internal/gateway/message_deduplicator.go`, `internal/gateway/message_deduplicator_manager_test.go`, `internal/gateway/event.go`, `internal/gateway/manager.go`, `internal/gateway/fake_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/gateway -run '^TestGatewayInbound_Dedup' -count=1`, `go test ./internal/gateway -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Gateway manager fixtures prove duplicate drop, scoped IDs, missing-ID degraded evidence, and unchanged command/follow-up/fresh-final behavior.
- Acceptance: TestGatewayInbound_DedupDropsRepeatedMessageID submits the same channel/chat/message ID twice and proves the fake kernel receives only the first turn while status evidence records duplicate_message., TestGatewayInbound_DedupScopesByChannelChatAndThread proves identical platform message IDs in different chats or threads do not collide., TestGatewayInbound_DedupMissingMessageIDDegrades proves empty message IDs do not drop messages but expose dedup_unavailable evidence., Existing gateway command parsing, follow-up queue, and fresh-final coalescer tests remain green.
- Source refs: ../hermes-agent/gateway/platforms/helpers.py@cebf9585:MessageDeduplicator, ../hermes-agent/tests/gateway/test_message_deduplicator.py@cebf9585, internal/gateway/message_deduplicator.go, internal/gateway/event.go, internal/gateway/manager.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 4. Durable worker RSS drain integration

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

## 5. Steer slash command registry + queue fallback

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

## 6. ContextEngine compression-boundary notification

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

## 7. Title prompt and truncation contract

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

## 8. Session search

- Phase: 5 / 5.N
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: internal/tools exposes a wrapper-only session_search Tool over existing internal/memory SearchSessions/SearchMessages APIs, preserving same-chat defaults, explicit user/source widening, lineage-root exclusion in recent mode, and Goncho/Honcho-compatible evidence without changing ranking or persistence
- Trust class: operator, child-agent, system
- Ready when: Source-filtered session/message search core and Lineage-aware source-filtered search hits are validated on main., Operator-auditable search evidence is validated on main, so this row is unblocked for the tool wrapper only., The tool can use seeded SQLite/session.Metadata fixtures and does not need a live gateway, provider, or Goncho cloud service., Existing Goncho/Honcho-compatible scope rules remain the authority for user/source widening., Implement against the internal/tools.Tool interface in internal/tools/tool.go; this row creates the tool type and tests, not cmd/gormes registry binding., This row must wrap existing internal/memory.SearchSessions/SearchMessages and must not change ranking, lineage construction, default same-chat fences, or Goncho persistence.
- Not ready when: The slice changes ranking, default same-chat recall fences, or Goncho/Honcho memory persistence instead of wrapping existing search results., The slice shells out to Hermes Python or reads ~/.hermes session logs., The slice edits internal/memory/session_catalog.go or internal/goncho/service.go; those prerequisites are already validated and should be treated as read-only donor code., The slice edits internal/tools/builtin.go, cmd/gormes, gateway runtime registration, or the global toolset manifest; registration is a later row after the wrapper fixtures pass., The slice includes todo/debug/clarify tools or cronjob tools in the same change.
- Degraded mode: Tool result reports session_search_unavailable, source_filter_denied, or lineage_root_excluded evidence instead of widening recall silently.
- Fixture: `internal/tools/session_search_tool_test.go::TestSessionSearchTool_*`
- Write scope: `internal/tools/session_search_tool.go`, `internal/tools/session_search_tool_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools -run '^TestSessionSearchTool_' -count=1`, `go test ./internal/tools ./internal/memory ./internal/goncho -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Session search tool fixtures prove same-chat defaults, opt-in user/source widening, deterministic current-lineage-root exclusion in recent mode, and degraded evidence for unavailable/denied widening.
- Acceptance: TestSessionSearchTool_SameChatDefault seeds two chats and proves no cross-chat hit appears without explicit scope=user or sources., TestSessionSearchTool_UserScopeSourceFilter passes scope=user sources=[telegram] and proves only the allowed source's sessions are returned with source evidence., TestSessionSearchTool_RecentModeExcludesCurrentLineageRoot seeds root and compressed child sessions, runs recent mode from the child, and proves the current root is excluded deterministically per Hermes dbe50155., TestSessionSearchTool_DegradedEvidence covers missing session directory and denied source widening without panics or hidden fallback widening., TestSessionSearchTool_DescriptorAndSchema proves the wrapper satisfies tools.Tool with Name() == "session_search", a JSON schema that exposes query/scope/sources/mode/current_session_id, and a deterministic timeout without registering it globally.
- Source refs: ../hermes-agent/tools/session_search_tool.py@dbe50155, ../hermes-agent/tests/tools/test_session_search.py@dbe50155, internal/memory/session_catalog.go, internal/memory/session_lineage_search_test.go, internal/goncho/service.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 9. CLI OpenClaw residue onboarding hint

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: internal/cli exposes pure OpenClaw-residue onboarding helpers: DetectOpenClawResidue(home string) bool returns true only for an existing ~/.openclaw directory, OpenClawResidueHint(commandName string) string returns a Gormes-specific one-time cleanup hint, and OnboardingSeen/MarkOnboardingSeen operate on an in-memory map shape compatible with config onboarding.seen without reading or writing real config files
- Trust class: operator, system
- Ready when: internal/cli already has pure helper files and tests; this slice adds another helper without command registration, config I/O, or startup wiring., Use Gormes-facing text (`gormes openclaw cleanup` or the command name injected by tests) rather than copying Hermes' `hermes claw cleanup` string., Filesystem checks use a temp HOME provided by tests; no real operator home directory is inspected in unit tests.
- Not ready when: The slice edits cmd/gormes startup, reads/writes config files, renames ~/.openclaw, migrates OpenClaw data, or ports the full optional migration script., The hint text mentions Hermes commands, HERMES_HOME, or writes under upstream Hermes paths., The helper treats a regular file named .openclaw as residue.
- Degraded mode: If HOME cannot be inspected or onboarding config is malformed, the helper reports unseen/false and never blocks CLI startup; command wiring can decide whether to persist the seen flag later.
- Fixture: `internal/cli/onboarding_test.go::Test{DetectOpenClawResidue,OpenClawResidueHint,OnboardingSeen,MarkOnboardingSeen}_*`
- Write scope: `internal/cli/onboarding.go`, `internal/cli/onboarding_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run '^Test.*OpenClaw\|^TestOnboardingSeen\|^TestMarkOnboardingSeen' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: CLI onboarding fixtures prove directory-only OpenClaw residue detection, Gormes-specific cleanup hint text, in-memory seen-state handling, malformed-config tolerance, and no real HOME/config writes.
- Acceptance: TestDetectOpenClawResidue_DirectoryOnly returns true for a temp HOME containing a .openclaw directory and false for missing path or a regular file., TestOpenClawResidueHint_MentionsInjectedCleanupCommand proves the hint contains ~/.openclaw, the injected Gormes cleanup command, and no `hermes claw cleanup` substring., TestOnboardingSeen_MalformedConfigUnseen covers missing onboarding, non-map onboarding, non-map seen, false values, and unrelated flags., TestMarkOnboardingSeen_InMemoryPreservesOtherFlags sets openclaw_residue_cleanup=true in the provided map without removing existing seen flags., No test touches the real user home or writes config.yaml.
- Source refs: ../hermes-agent/agent/onboarding.py@e63929d4:OPENCLAW_RESIDUE_FLAG,detect_openclaw_residue,openclaw_residue_hint_cli,is_seen, ../hermes-agent/tests/agent/test_onboarding.py@e63929d4:TestOpenClawResidue, ../hermes-agent/cli.py@e63929d4:first-time OpenClaw-residue banner, internal/cli/tips.go
- Unblocks: OpenClaw residue startup banner binding
- Why now: Unblocks OpenClaw residue startup banner binding.

## 10. Custom provider model-switch key_env write guard

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

<!-- PROGRESS:END -->
