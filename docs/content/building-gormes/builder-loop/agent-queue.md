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
## 1. Telegram group bot-command mention gate helper

- Phase: 2 / 2.B.5
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Telegram ingress exposes a pure mention-gate helper that treats `/cmd@botname` bot_command entities as direct address when group require-mention policy is enabled, rejects commands addressed to other bots, and keeps bare group slash commands gated
- Trust class: gateway, operator, system
- Ready when: Telegram adapter already normalizes Update.Message into gateway.InboundEvent and commands route through gateway.ParseInboundText., This first slice is helper-only: workers add a pure function over text, []tgbotapi.MessageEntity, expected bot username, and a bool requireMention flag; no live Telegram client, pairing store, gateway manager, config migration, or command execution is needed., Table tests can model bot_command and mention entities directly with go-telegram-bot-api/v5 MessageEntity values.
- Not ready when: The slice wires group gating into Bot.Run/toInboundEvent production flow, changes allowed_chat_id or first_run_discovery policy, or starts responding to groups before the helper is fixture-backed., The helper treats `/status@other_bot` or bare `/status` as addressed when requireMention is true., The slice changes fresh-final streaming, deleteMessage, session keys, pairing approval, or gateway manager behavior.
- Degraded mode: Until the helper is bound into Telegram group ingress, Gormes cannot safely enable Hermes-style require_mention command gating for Telegram groups; status should report telegram_group_mention_gate_unavailable instead of silently responding to bare group commands.
- Fixture: `internal/channels/telegram/group_mention_test.go`
- Write scope: `internal/channels/telegram/group_mention.go`, `internal/channels/telegram/group_mention_test.go`, `internal/channels/telegram/bot_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/channels/telegram -run '^TestTelegramGroupMentionGate_' -count=1`, `go test ./internal/channels/telegram -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Telegram group mention fixtures prove bot_command suffix matching, other-bot rejection, bare-command gating, mention-entity compatibility, and unchanged existing Telegram command tests without production gateway binding.
- Acceptance: TestTelegramGroupMentionGate_BotCommandWithMatchingSuffixAllowsCommand builds `/status@gormes_bot` with one bot_command entity covering the whole token and returns addressed=true for expected username `gormes_bot` or `@gormes_bot`., TestTelegramGroupMentionGate_OtherBotSuffixRejected proves `/status@other_bot` with a bot_command entity returns addressed=false when expected username is `gormes_bot`., TestTelegramGroupMentionGate_BareCommandStillGated proves `/status` remains rejected when requireMention is true and accepted when requireMention is false., TestTelegramGroupMentionGate_MentionEntityStillAllowsText proves normal text containing an @gormes_bot mention entity is still accepted., Existing TestBot_ToInboundEvent_Commands and fresh-final delete tests keep passing; no live Telegram token or network call is required.
- Source refs: ../hermes-agent/gateway/platforms/telegram.py@3ff3dfb5:TelegramAdapter._has_direct_mention, ../hermes-agent/tests/gateway/test_telegram_group_gating.py@3ff3dfb5:test_group_messages_can_require_direct_trigger_via_config, internal/channels/telegram/bot.go, internal/channels/telegram/bot_test.go, github.com/go-telegram-bot-api/telegram-bot-api/v5@v5.5.1:MessageEntity
- Unblocks: Telegram group mention gate config binding
- Why now: Unblocks Telegram group mention gate config binding.

## 2. Durable worker RSS watchdog policy helper

- Phase: 2 / 2.E.3
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: Gormes durable worker exposes a pure RSS watchdog policy helper that classifies disabled mode, unavailable RSS reads, threshold exceeded evidence, and stable watchdog restart reset without integrating with the worker loop yet
- Trust class: operator, system
- Ready when: Durable worker execution loop and Durable worker abort-slot recovery safety net are validated on main., The GBrain behavior is fully summarized in this row: max_rss_mb=0 disables checks, RSS read errors degrade without cancellation, over-threshold checks request a graceful drain, and watchdog exits after stable runtime reset crash count., The helper can be tested with fake RSS reader, fake clock, and value-only policy structs; no real process RSS, worker goroutine, external supervisor, shell handler, or GBrain TypeScript runtime is required., Workers should not run git show inside the Gormes checkout for GBrain paths; the relative source_refs above point at the synchronized sibling repo only for context.
- Not ready when: The slice changes DurableWorker.RunOne, doctor/status output, cmd/gormes lifecycle, builder-loop backend watchdogs, live delegate_task behavior, or cron execution semantics., The tests require real RSS measurements, sleeps longer than 100ms, subprocess workers, systemd, Postgres/PGLite, or importing GBrain code., The slice adds process self-termination to the main Gormes process or internal Goncho memory service.
- Degraded mode: Durable-worker policy reports rss_watchdog_disabled, rss_threshold_exceeded, rss_watchdog_unavailable, or stable_watchdog_restart before runtime drain wiring exists.
- Fixture: `internal/subagent/durable_worker_rss_watchdog_test.go::TestDurableWorkerRSSWatchdogPolicy`
- Write scope: `internal/subagent/durable_worker_rss_watchdog.go`, `internal/subagent/durable_worker_rss_watchdog_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/subagent -run 'TestDurableWorkerRSSWatchdog_\|TestDurableWorkerWatchdogRestartPolicy_' -count=1`, `go test ./internal/subagent -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Durable worker RSS policy fixtures prove disabled mode, threshold evidence, read-failure degradation, and stable watchdog restart classification without runtime drain or doctor/status edits.
- Acceptance: TestDurableWorkerRSSWatchdog_DisabledAtZero proves max_rss_mb=0 records rss_watchdog_disabled and never reads the RSS seam., TestDurableWorkerRSSWatchdog_ThresholdExceeded injects an RSS value over max_rss_mb and returns rss_threshold_exceeded evidence with observed_mb, max_mb, and checked_at from the fake clock., TestDurableWorkerRSSWatchdog_RSSReadFailure returns rss_watchdog_unavailable evidence and does not request a drain., TestDurableWorkerWatchdogRestartPolicy_StableRunReset classifies a watchdog exit after at least five minutes as stable_watchdog_restart and does not increment crash count past one.
- Source refs: ../gbrain/CHANGELOG.md@c78c3d0:v0.22.2, ../gbrain/src/core/minions/types.ts@c78c3d0:MinionWorkerOpts.maxRssMb/getRss/rssCheckInterval, ../gbrain/src/core/minions/worker.ts@c78c3d0:checkMemoryLimit/gracefulShutdown, ../gbrain/src/commands/autopilot.ts@c78c3d0:stable-run reset, ../gbrain/test/minions.test.ts@c78c3d0:MinionWorker --max-rss watchdog, internal/subagent/durable_worker.go, internal/subagent/durable_worker_abort_test.go
- Unblocks: Durable worker RSS drain integration
- Why now: Unblocks Durable worker RSS drain integration.

## 3. Steer slash command registry + queue fallback

- Phase: 2 / 2.F.5
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Contract: Registry-owned active-turn steering command
- Trust class: operator, gateway
- Ready when: Steer slash command parser + preview helper is validated on main., The parser helper is already complete on main, so workers should start from internal/gateway/steer_command.go and add only registry/queue behavior in this row., This slice only registers /steer and queue fallback behavior; the live between-tool-call injection hook remains in the dependent row., Tests can use fake running-agent state and fake command dispatch; no provider, active tool loop, TUI, Slack, or Telegram transport is required.
- Not ready when: The implementation tries to inject mid-run prompts instead of only registering /steer and queue fallback behavior., The slice changes /queue, /bg, /busy config persistence, TUI keybindings, or platform adapter code., The parser row is not present in the worker checkout.
- Degraded mode: Gateway returns visible usage, busy, steer_unavailable, or queued status instead of dropping steer text when the command cannot run immediately.
- Fixture: `internal/gateway/steer_queue_test.go`
- Write scope: `internal/gateway/commands.go`, `internal/gateway/manager.go`, `internal/gateway/steer_command.go`, `internal/gateway/steer_queue_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/gateway -run '^TestSteerCommandRegistry_' -count=1`, `go test ./internal/gateway -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Gateway command fixtures prove /steer registry exposure, parsed queue fallback, running-agent degraded evidence, and no live mid-run injection.
- Acceptance: TestSteerCommandRegistry_RegisteredAsBusyAware exposes /steer through the shared registry without changing existing command names., TestSteerCommandRegistry_NoRunningAgentQueuesGuidance queues parsed follow-up guidance and returns the parser preview., TestSteerCommandRegistry_RunningAgentFallbackDoesNotInject proves running-agent paths return explicit steer_unavailable/queued evidence until the mid-run hook row lands., Existing gateway command parsing and active-session policy tests remain green.
- Source refs: ../hermes-agent/cli.py@635253b9:busy_input_mode=steer, ../hermes-agent/gateway/run.py@635253b9:running_agent.steer, ../hermes-agent/tests/gateway/test_busy_session_ack.py@635253b9, internal/gateway/commands.go, internal/gateway/manager.go, internal/gateway/steer_command.go
- Unblocks: Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard
- Why now: Unblocks Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard.

## 4. Title prompt and truncation contract

- Phase: 4 / 4.F
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Contract: Native title generation exposes a pure request/response boundary that builds Hermes-compatible title prompts from bounded session history, truncates candidate titles deterministically, returns empty-title fallback evidence for empty history or blank model output, and surfaces provider failures through a typed nonfatal error result without writing session metadata
- Trust class: operator, system
- Ready when: TUI prompt-submit auto-title eligibility helper is validated, so callers can produce a TitleRequest without starting provider work inside the TUI update loop., The worker can use a fake title model function and synthetic history; no provider credential, goroutine, TUI program, or session DB write is required.
- Not ready when: The slice writes session.Metadata, starts background title workers, changes TUI submit behavior, or calls a live LLM., The slice swallows provider failures without returning typed evidence that the CLI/gateway can surface later., The slice mutates transcript history or uses Goncho/Honcho memory as a title source.
- Degraded mode: Title status reports auto_title_skipped, title_provider_failed, or title_blank_result evidence instead of silently leaving NULL titles with no operator-visible cause.
- Fixture: `internal/hermes/title_generator_test.go`
- Write scope: `internal/hermes/title_generator.go`, `internal/hermes/title_generator_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run '^TestTitle' -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Title generator fixtures prove bounded prompt construction, candidate cleanup/truncation, empty-history skip, blank-output evidence, and provider-failure evidence with a fake model only.
- Acceptance: TestTitlePrompt_BuildsFromBoundedHistory proves only the configured recent user/assistant turns enter the prompt and long content is truncated before model invocation., TestTitleGenerator_TruncatesAndCleansCandidate proves whitespace/newline/quote cleanup plus deterministic max-title-length truncation., TestTitleGenerator_EmptyHistorySkipsModel returns skipped evidence and never calls the fake model., TestTitleGenerator_BlankModelOutput returns title_blank_result evidence without writing metadata., TestTitleGenerator_ProviderFailureReturnsTypedEvidence proves the fake model error is returned as nonfatal title_provider_failed evidence.
- Source refs: ../hermes-agent/agent/title_generator.py@4a2ee6c1:generate_title, ../hermes-agent/agent/title_generator.py@4a2ee6c1:maybe_auto_title, ../hermes-agent/tests/agent/test_title_generator.py@4a2ee6c1, internal/tui/auto_title.go, internal/session/, internal/transcript/
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 5. Provider timeout config fail-closed helper

- Phase: 4 / 4.H
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: Provider timeout lookup handles missing or failed config loading by returning explicit unset evidence, and parses provider request/stale timeout overrides without panicking or applying stale defaults
- Trust class: operator, system
- Ready when: Classified provider-error taxonomy and Retry-After hint handling are validated, so timeout evidence can reuse provider status vocabulary without changing retry loops., This row is pure lookup/parsing only; workers should not add new public config fields or wire live HTTP client timeouts until the helper behavior is fixture-locked., The helper can use injected loader callbacks and synthetic provider maps; no real config file, network call, or provider client is required.
- Not ready when: The slice changes internal/config public TOML schema, provider HTTP client construction, or kernel retry behavior in the same change., The helper panics or returns a non-zero timeout when config loading/import fails, when providers is not a map, or when the requested provider block is malformed., The tests depend on wall-clock sleeps or live provider credentials.
- Degraded mode: Provider status reports timeout_config_unavailable, timeout_config_invalid, or timeout_unset evidence instead of crashing startup or silently applying a stale provider timeout.
- Fixture: `internal/hermes/provider_timeout_config_test.go`
- Write scope: `internal/hermes/provider_timeout_config.go`, `internal/hermes/provider_timeout_config_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -run TestProviderTimeoutConfig -count=1`, `go test ./internal/hermes -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Provider timeout config fixtures prove load failures, missing providers, malformed values, and valid overrides return explicit evidence without wiring live clients or config schema.
- Acceptance: TestProviderTimeoutConfig_LoadFailureReturnsUnset injects a loader error and proves request and stale timeout lookup return unset evidence without panic., TestProviderTimeoutConfig_MissingProviderReturnsUnset proves nil providers, missing provider IDs, and non-map provider blocks return timeout_unset., TestProviderTimeoutConfig_ParsesRequestAndStaleTimeouts proves numeric seconds and duration strings resolve to deterministic time.Duration values., TestProviderTimeoutConfig_InvalidValuesFailClosed proves negative, zero when not allowed, non-numeric, and overflow values return timeout_config_invalid evidence., No HTTP client, kernel retry, or public config schema is changed by this helper-only slice.
- Source refs: ../hermes-agent/hermes_cli/timeouts.py@16e243e0:get_provider_request_timeout, ../hermes-agent/hermes_cli/timeouts.py@16e243e0:get_provider_stale_timeout, ../hermes-agent/hermes_cli/timeouts.py@366351b9, internal/config/config.go:HermesCfg, internal/hermes/http_client.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 6. Session search

- Phase: 5 / 5.N
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: Native session_search tool uses the Phase 3 session catalog with same-chat defaults, opt-in user/source filters, and deterministic recent-mode exclusion of the current lineage root
- Trust class: operator, child-agent, system
- Ready when: Source-filtered session/message search core and Lineage-aware source-filtered search hits are validated on main., Operator-auditable search evidence is validated on main, so this row is unblocked for the tool wrapper only., The tool can use seeded SQLite/session.Metadata fixtures and does not need a live gateway, provider, or Goncho cloud service., Existing Goncho/Honcho-compatible scope rules remain the authority for user/source widening., Implement against the internal/tools.Tool interface in internal/tools/tool.go; this row creates the tool type and tests, not cmd/gormes registry binding., This row must wrap existing internal/memory.SearchSessions/SearchMessages and must not change ranking, lineage construction, default same-chat fences, or Goncho persistence.
- Not ready when: The slice changes ranking, default same-chat recall fences, or Goncho/Honcho memory persistence instead of wrapping existing search results., The slice shells out to Hermes Python or reads ~/.hermes session logs., The slice edits internal/memory/session_catalog.go or internal/goncho/service.go; those prerequisites are already validated and should be treated as read-only donor code., The slice edits internal/tools/builtin.go, cmd/gormes, gateway runtime registration, or the global toolset manifest; registration is a later row after the wrapper fixtures pass., The slice includes todo/debug/clarify tools or cronjob tools in the same change.
- Degraded mode: Tool result reports session_search_unavailable, source_filter_denied, or lineage_root_excluded evidence instead of widening recall silently.
- Fixture: `internal/tools/session_search_tool_test.go`
- Write scope: `internal/tools/session_search_tool.go`, `internal/tools/session_search_tool_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools -run '^TestSessionSearchTool_' -count=1`, `go test ./internal/tools ./internal/memory ./internal/goncho -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Session search tool fixtures prove same-chat defaults, opt-in user/source widening, deterministic current-lineage-root exclusion in recent mode, and degraded evidence for unavailable/denied widening.
- Acceptance: TestSessionSearchTool_SameChatDefault seeds two chats and proves no cross-chat hit appears without explicit scope=user or sources., TestSessionSearchTool_UserScopeSourceFilter passes scope=user sources=[telegram] and proves only the allowed source's sessions are returned with source evidence., TestSessionSearchTool_RecentModeExcludesCurrentLineageRoot seeds root and compressed child sessions, runs recent mode from the child, and proves the current root is excluded deterministically per Hermes dbe50155., TestSessionSearchTool_DegradedEvidence covers missing session directory and denied source widening without panics or hidden fallback widening., TestSessionSearchTool_DescriptorAndSchema proves the wrapper satisfies tools.Tool with Name() == "session_search", a JSON schema that exposes query/scope/sources/mode/current_session_id, and a deterministic timeout without registering it globally.
- Source refs: ../hermes-agent/tools/session_search_tool.py@dbe50155, ../hermes-agent/tests/tools/test_session_search.py@dbe50155, internal/memory/session_catalog.go, internal/memory/session_lineage_search_test.go, internal/goncho/service.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 7. CLI OpenClaw residue onboarding hint

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
- Fixture: `internal/cli/onboarding_test.go`
- Write scope: `internal/cli/onboarding.go`, `internal/cli/onboarding_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run '^Test.*OpenClaw\|^TestOnboardingSeen\|^TestMarkOnboardingSeen' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: CLI onboarding fixtures prove directory-only OpenClaw residue detection, Gormes-specific cleanup hint text, in-memory seen-state handling, malformed-config tolerance, and no real HOME/config writes.
- Acceptance: TestDetectOpenClawResidue_DirectoryOnly returns true for a temp HOME containing a .openclaw directory and false for missing path or a regular file., TestOpenClawResidueHint_MentionsInjectedCleanupCommand proves the hint contains ~/.openclaw, the injected Gormes cleanup command, and no `hermes claw cleanup` substring., TestOnboardingSeen_MalformedConfigUnseen covers missing onboarding, non-map onboarding, non-map seen, false values, and unrelated flags., TestMarkOnboardingSeen_InMemoryPreservesOtherFlags sets openclaw_residue_cleanup=true in the provided map without removing existing seen flags., No test touches the real user home or writes config.yaml.
- Source refs: ../hermes-agent/agent/onboarding.py@e63929d4:OPENCLAW_RESIDUE_FLAG,detect_openclaw_residue,openclaw_residue_hint_cli,is_seen, ../hermes-agent/tests/agent/test_onboarding.py@e63929d4:TestOpenClawResidue, ../hermes-agent/cli.py@e63929d4:first-time OpenClaw-residue banner, internal/cli/tips.go
- Unblocks: OpenClaw residue startup banner binding
- Why now: Unblocks OpenClaw residue startup banner binding.

## 8. CLI bracketed-paste wrapper sanitizer

- Phase: 5 / 5.O
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: internal/cli exposes StripLeakedBracketedPasteWrappers(text string) string, a pure sanitizer that removes canonical ESC [200~/[201~ wrappers, visible caret-escape wrappers, degraded boundary [200~/[201~ wrappers, and boundary 00~/01~ fragments while preserving non-wrapper substrings inside ordinary text
- Trust class: operator, system
- Ready when: internal/cli already has pure helper/test seams; this slice adds one sanitizer file and does not need Cobra command wiring., Tests can use string literals for canonical escape, caret-escape, bracket-only, fragment-only, and normal-text inputs; no TTY, prompt toolkit, clipboard, or terminal emulator is required.
- Not ready when: The slice edits cmd/gormes startup, TUI input handling, clipboard/image attachment behavior, file-drop detection, command dispatch, or gateway parsing., The sanitizer deletes non-boundary substrings like build00~tag or literal[200~tag., The tests depend on real terminal bracketed-paste mode or prompt_toolkit behavior.
- Degraded mode: If a terminal leaks bracketed-paste markers into a CLI buffer, Gormes strips only recognized boundary wrappers before command/path detection; ambiguous inline text is preserved rather than over-sanitized.
- Fixture: `internal/cli/paste_sanitizer_test.go`
- Write scope: `internal/cli/paste_sanitizer.go`, `internal/cli/paste_sanitizer_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run '^TestStripLeakedBracketedPasteWrappers_' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Paste sanitizer fixtures prove canonical/caret/degraded wrapper stripping, boundary fragment stripping, normal-text preservation, multiline preservation, and zero TTY/clipboard dependencies.
- Acceptance: TestStripLeakedBracketedPasteWrappers_CanonicalEscape strips \u001b[200~hello\u001b[201~ to hello., TestStripLeakedBracketedPasteWrappers_CaretEscape strips ^[[200~hello^[[201~ to hello., TestStripLeakedBracketedPasteWrappers_DegradedBracketBoundaries strips [200~hello[201~ at start/whitespace boundaries while preserving literal[200~tag., TestStripLeakedBracketedPasteWrappers_FragmentBoundaries strips 00~hello01~ at start/whitespace boundaries while preserving build00~tag., TestStripLeakedBracketedPasteWrappers_MultilinePreserved keeps embedded newlines and content bytes after wrapper removal.
- Source refs: ../hermes-agent/cli.py@a0fe73ba:_strip_leaked_bracketed_paste_wrappers, ../hermes-agent/tests/cli/test_cli_bracketed_paste_sanitizer.py@a0fe73ba, internal/cli/command_registry.go, internal/cli/output.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 9. Native TUI /save XDG file writer binding

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: cmd/gormes binds the native TUI /save SessionExportFunc to the canonical persisted transcript reader and writes markdown exports under the Gormes XDG data directory, never under upstream HERMES_HOME or the process working directory
- Trust class: operator, system
- Ready when: Native TUI /save canonical session export is validated on main and exposes SessionExportFunc on Model options., The prior /save handler write-scope issue is closed; slash_dispatch_test.go was already included in the completed handler row and should not be touched here., cmd/gormes/session.go already proves transcript.ExportMarkdown works against config.MemoryDBPath for the CLI export command., internal/tui/slash_save.go already owns partial-file cleanup and should be treated as read-only in this binding slice., Gormes intentionally uses XDG_DATA_HOME/gormes instead of HERMES_HOME; the fixture should assert that divergence.
- Not ready when: The slice reopens internal/tui slash handler behavior or changes transcript.ExportMarkdown output format., The slice writes exports under HERMES_HOME, the repository root, or the current working directory., The slice starts a provider, API server, remote TUI gateway, or live session browser.
- Degraded mode: TUI status reports `save: store unavailable` or `save: write failed: <err>` with partial-file cleanup instead of exposing an unwired /save handler or writing to an ambiguous location.
- Fixture: `cmd/gormes/tui_save_export_test.go`
- Write scope: `cmd/gormes/main.go`, `cmd/gormes/session.go`, `cmd/gormes/tui_save_export_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./cmd/gormes -run '^TestTUISaveExport_' -count=1`, `go test ./cmd/gormes ./internal/tui ./internal/transcript -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: cmd/gormes TUI save-export fixtures prove the real SessionExportFunc writes under XDG_DATA_HOME/gormes/sessions/exports, ignores HERMES_HOME for runtime state, relies on the existing TUI partial-file cleanup path, and returns the exported path to /save without internal/tui edits.
- Acceptance: cmd/gormes constructs a SessionExportFunc for the local TUI Model, preferably through a small helper in cmd/gormes/session.go, that opens config.MemoryDBPath, calls transcript.ExportMarkdown, and writes `<session-id>.md` or a collision-safe variant under `$XDG_DATA_HOME/gormes/sessions/exports/`., runResolvedTUIWithRuntime passes that function through tui.Options{SessionExport: ...} only for the local TUI path; remote TUI startup remains unchanged., TestTUISaveExport_WritesUnderXDGDataHome sets XDG_DATA_HOME and HERMES_HOME to different temp roots, invokes the bound SessionExportFunc, and proves the file lands under the Gormes XDG export directory only., TestTUISaveExport_RemovesPartialOnFailure injects a write failure after creating a partial file and proves /save removes it through the existing slash handler cleanup path., The status message returned by /save contains the XDG export path and no test opens a network connection or starts the remote TUI gateway.
- Source refs: ../hermes-agent/tests/cli/test_save_conversation_location.py@5eb6cd82, ../hermes-agent/cli.py@5eb6cd82:save conversation location, ../hermes-agent/tui_gateway/server.py@5eb6cd82:session.save, internal/tui/slash_save.go:SessionExportFunc, cmd/gormes/session.go:sessionExportCmd, cmd/gormes/main.go:runResolvedTUIWithRuntime, internal/config/config.go:MemoryDBPath
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 10. Native TUI bounded conversation viewport

- Phase: 5 / 5.Q
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Native Bubble Tea conversation rendering limits each frame to the visible tail of RenderFrame.History plus DraftText/LastError under a caller-provided height budget, emits a stable omitted-history sentinel when earlier turns are hidden, and avoids rebuilding unbounded conversation strings during long sessions
- Trust class: operator, system
- Ready when: internal/tui/view.go currently renders every RenderFrame.History message into one joined string; the worker can add a pure helper and table tests without changing kernel history, persistence, or provider streaming., Synthetic RenderFrame fixtures with 100+ messages are enough; no Bubble Tea program, terminal, Node/Ink runtime, or Hermes profiling script needs to run., The row is a Gormes-native performance guard, not a direct port of Hermes' React/Ink virtualization stack.
- Not ready when: The slice imports React/Ink concepts, starts Node, changes kernel.RenderFrame shape, truncates stored session/transcript history, or edits internal/kernel/internal/transcript persistence., The slice changes slash-command, remote TUI SSE, API server, or dashboard behavior in the same diff., The slice silently drops DraftText or LastError when history is long.
- Degraded mode: If height is too small or width is narrow, the helper still renders the latest visible turn/draft/error and a compact omitted-history sentinel rather than panicking or allocating the full history body.
- Fixture: `internal/tui/viewport_history_test.go`
- Write scope: `internal/tui/view.go`, `internal/tui/viewport_history_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tui -run '^TestRenderConversationViewport_' -count=1`, `go test ./internal/tui -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Native TUI viewport fixtures prove long histories render a bounded visible tail with omitted-count sentinel, draft/error preservation, small-size clamps, and no kernel/session persistence changes.
- Acceptance: TestRenderConversationViewport_OmitsEarlierHistory builds 120 alternating user/assistant messages and asserts the rendered output contains the latest turns, excludes the earliest turn body, and includes a deterministic omitted-history sentinel with the hidden count., TestRenderConversationViewport_AlwaysIncludesDraftAndLastError proves DraftText and LastError survive even when history is clipped., TestRenderConversationViewport_HeightAndWidthClamp proves width<4 and tiny height do not panic and still render a compact latest-message view., TestRenderConversationViewport_RenderedLineBudget asserts the helper returns no more than the requested visible budget plus a small sentinel allowance for wrapped content., The implementation stays in internal/tui/view.go plus its test; no kernel/session/transcript files are edited.
- Source refs: ../hermes-agent/ui-tui/src/hooks/useVirtualHistory.ts@e63929d4, ../hermes-agent/ui-tui/src/lib/virtualHeights.ts@e63929d4, ../hermes-agent/ui-tui/src/__tests__/virtualHeights.test.ts@e63929d4, ../hermes-agent/scripts/profile-tui.py@e63929d4, internal/tui/view.go:renderConv
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

<!-- PROGRESS:END -->
