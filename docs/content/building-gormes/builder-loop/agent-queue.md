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
## 1. Title prompt and truncation contract

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

## 2. Session search

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

## 3. Backend usage-limit stdin health bypass

- Phase: 5 / 5.N
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P1`
- Contract: Builder-loop treats backend usage-limit/stdin-wait exits that produce no worker diff as backend infrastructure degradation, emits run-level backend evidence, and avoids charging the selected feature row as a worker_error quarantine candidate
- Trust class: system
- Ready when: Autoloop audit shows backend_waiting_for_stdin rows whose detail contains `You've hit your usage limit` and no promoted diff., Existing backend failure classification already detects killed, deadline, and stdin-wait exits; this slice only adds the usage-limit/no-diff health bypass around that path., The test can use FakeRunner with a backend Result{Err: errors.New("exit status 1"), Stdout: usage-limit text plus `Reading additional input from stdin...`} and a one-row progress fixture; no real backend, git remote, provider, or worktree promotion is required.
- Not ready when: The slice changes feature-row contracts, candidate ranking, post-promotion gates, planner prompt generation, or backend command construction., The slice suppresses real row failures that produced a commit, dirty worktree, write-scope violation, validation failure, or no-progress diff., The slice retries forever instead of emitting bounded backend_degraded evidence when no fallback backend remains.
- Degraded mode: When every configured backend is usage-limited or waiting for stdin, the loop emits backend_degraded/backend_waiting_for_stdin evidence and leaves feature-row health unchanged so the planner sees an infrastructure outage instead of a toxic implementation row.
- Fixture: `internal/builderloop/run_health_test.go::TestRunOnce_BackendUsageLimitDoesNotQuarantineRow`
- Write scope: `internal/builderloop/backend_failure.go`, `internal/builderloop/backend_failure_test.go`, `internal/builderloop/run.go`, `internal/builderloop/run_health_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/builderloop -run '^(TestClassifyBackendFailureDetectsUsageLimitStdinExit\|TestRunOnce_BackendUsageLimitDoesNotQuarantineRow\|TestRunOnce_BackendDegradedEventEmittedAfterThreshold)$' -count=1`, `go test ./internal/builderloop -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Builder-loop usage-limit fixtures prove backend stdin/usage-limit outages produce run-level degraded evidence without mutating feature-row health, while ordinary worker_error paths still update row health.
- Acceptance: TestClassifyBackendFailureDetectsUsageLimitStdinExit proves usage-limit text is classified distinctly from ordinary stdin wait while preserving the original detail tail., TestRunOnce_BackendUsageLimitDoesNotQuarantineRow runs a selected feature row through FakeRunner usage-limit/stdin output and proves the row's health block is not incremented or quarantined., The same run emits a ledger event with backend_waiting_for_stdin or backend_usage_limited detail plus backend_degraded evidence when fallback switching is configured., A separate worker_error fixture with a non-backend row failure still increments row health, so real implementation failures remain visible to quarantine math., Existing TestRunOnce_BackendDegradedEventEmittedAfterThreshold and backend_failure_test.go fixtures remain green.
- Source refs: internal/builderloop/backend_failure.go:ClassifyBackendFailure, internal/builderloop/backend_failure_test.go, internal/builderloop/run.go:recordWorkerOutcome, internal/builderloop/run_health_test.go:TestRunOnce_BackendDegradedEventEmittedAfterThreshold, internal/builderloop/health_writer.go:RecordFailure, .codex/builder-loop/state/runs.jsonl recent backend_waiting_for_stdin usage-limit entries
- Unblocks: Steer slash command registry + queue fallback, ContextEngine compression-boundary notification, Custom provider model-switch key_env write guard, Native TUI /save XDG file writer binding, Native TUI bounded conversation viewport
- Why now: Unblocks Steer slash command registry + queue fallback, ContextEngine compression-boundary notification, Custom provider model-switch key_env write guard, Native TUI /save XDG file writer binding, Native TUI bounded conversation viewport.

## 4. CLI OpenClaw residue onboarding hint

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

## 5. BlueBubbles iMessage bubble formatting parity

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

## 6. Yuanbao protocol envelope + markdown fixtures

- Phase: 7 / 7.E
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P4`
- Contract: Gormes parses Yuanbao websocket/protobuf-style envelopes and Markdown message fragments into gateway-neutral events using fixture data only
- Trust class: gateway, system
- Ready when: The Phase 2 shared gateway event shape and Regional + Device Adapter Backlog are available; this row does not need a live Yuanbao account., Workers can start with captured JSON/proto/markdown testdata under internal/channels/yuanbao/testdata copied or minimized from upstream fixtures., No send loop, login flow, tool registration, media download, or sticker parsing is required for this first slice.
- Not ready when: The slice opens a websocket, performs login, calls Tencent/Yuanbao endpoints, downloads media, or registers user-visible tools., The slice stores credentials or changes shared gateway session policy., The slice combines protocol parsing with send/reply runtime behavior.
- Degraded mode: Yuanbao adapter status reports protocol_unavailable or markdown_parse_failed evidence instead of starting a live session with unparsed payloads.
- Fixture: `internal/channels/yuanbao/proto_test.go`
- Write scope: `internal/channels/yuanbao/proto.go`, `internal/channels/yuanbao/proto_test.go`, `internal/channels/yuanbao/markdown.go`, `internal/channels/yuanbao/markdown_test.go`, `internal/channels/yuanbao/testdata/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/channels/yuanbao -run 'TestYuanbao(Proto\|Markdown)' -count=1`, `go test ./internal/channels/yuanbao -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Yuanbao protocol/markdown fixtures prove inbound text event normalization and degraded parse evidence with no live Yuanbao network call.
- Acceptance: TestYuanbaoProto_DecodesInboundTextFixture loads a captured fixture and returns source, conversation id, message id, author role, and text content., TestYuanbaoMarkdown_RendersCodeAndLinks proves code blocks, links, mentions, and list fragments are normalized into plain prompt-safe text without losing URLs., Malformed/unknown envelope fixtures return typed degraded evidence and do not panic., No test imports a generated protobuf runtime unless a local generated fixture file is checked in under internal/channels/yuanbao.
- Source refs: ../hermes-agent/gateway/platforms/yuanbao_proto.py@ab687963, ../hermes-agent/gateway/platforms/yuanbao.py@ab687963, ../hermes-agent/tests/test_yuanbao_proto.py@ab687963, ../hermes-agent/tests/test_yuanbao_markdown.py@ab687963, ../hermes-agent/website/docs/user-guide/messaging/yuanbao.md@ab687963
- Unblocks: Yuanbao media/sticker attachment normalization, Yuanbao gateway runtime + toolset registration
- Why now: Unblocks Yuanbao media/sticker attachment normalization, Yuanbao gateway runtime + toolset registration.

<!-- PROGRESS:END -->
