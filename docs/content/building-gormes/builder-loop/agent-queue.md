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
## 1. Durable worker RSS watchdog policy helper

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

## 2. Title prompt and truncation contract

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

## 3. Provider timeout config fail-closed helper

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

## 4. Browser SSRF quoted-false guard

- Phase: 5 / 5.C
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Browser and URL safety helpers coerce quoted false-like config values (`"false"`, `'false'`, `0`, `no`, `off`) to disabled booleans before private/local URL SSRF guards decide whether cloud navigation is allowed
- Trust class: operator, system
- Ready when: Browser hybrid private-URL local sidecar routing is complete and already classifies private/LAN hosts without DNS or browser startup., The worker can add a pure config coercion and guard decision helper under internal/tools; no browser runtime, network, DNS, Chromedp, Rod, Browserbase, Firecrawl, or Camofox dependency is required.
- Not ready when: The slice starts a browser, performs DNS resolution, follows redirects, or changes cloud/local routing behavior beyond quoted-false coercion., The slice weakens private host classification already validated by Browser hybrid private-URL local sidecar routing., The slice implements provider bridges or screenshot/navigation actions.
- Degraded mode: Browser safety status reports ssrf_guard_config_invalid or private_url_blocked instead of treating quoted false as truthy and sending private URLs to a cloud/browser provider.
- Fixture: `internal/tools/browser_ssrf_guard_test.go`
- Write scope: `internal/tools/browser_ssrf_guard.go`, `internal/tools/browser_ssrf_guard_test.go`, `internal/tools/browser_hybrid_routing.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools -run '^TestBrowserSSRFGuard_' -count=1`, `go test ./internal/tools -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Browser SSRF guard fixtures prove quoted false values disable cloud routing guards correctly, private URLs are blocked when they would reach cloud providers, and no browser/network runtime is used.
- Acceptance: TestBrowserSSRFGuard_CoercesQuotedFalseValues covers `"false"`, `'false'`, `0`, `no`, and `off` as disabled while true/yes/on remain enabled., TestBrowserSSRFGuard_PrivateURLBlockedWhenCloudWouldReceiveIt proves localhost/RFC1918 URLs return private_url_blocked when cloud routing is enabled and auto-local routing is disabled., TestBrowserSSRFGuard_PublicURLAllowed covers a public URL and proves no private_url_blocked evidence is emitted., Tests use synthetic strings only and do not start a browser, resolve DNS, or open network sockets.
- Source refs: ../hermes-agent/tools/browser_tool.py@7317d69f, ../hermes-agent/tools/url_safety.py@7317d69f, ../hermes-agent/tests/tools/test_browser_ssrf_local.py@7317d69f, ../hermes-agent/tests/tools/test_url_safety.py@7317d69f, internal/tools/browser_hybrid_routing.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 5. Skills hub direct URL candidate parser

- Phase: 5 / 5.F
- Owner: `skills`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: internal/skills exposes a pure ParseURLSkillCandidate(rawURL string, skillMD []byte) (URLSkillCandidate, error) helper that mirrors Hermes UrlSource.fetch metadata without network or store writes: HTTPS SKILL.md URLs only, source=url, trust=community, files={SKILL.md}, resolved name from valid frontmatter or URL slug, awaiting_name evidence when neither produces a safe install name, and no path traversal in name/category candidates
- Trust class: operator, system
- Ready when: internal/skills/parser.go already extracts SKILL.md name/description/body and enforces size limits., The row is pure parsing and validation: tests inject raw SKILL.md bytes and URLs; no HTTP client, CLI command, quarantine directory, scan policy, or active store mutation belongs in this slice., Use Gormes naming in retry hints (`gormes skills install ... --name <your-name>`), while source_refs stay pinned to Hermes behavior.
- Not ready when: The slice downloads from the network, writes files under active/candidates/.hub, shells out, or changes skill selection/preprocessing., The slice accepts sentinel names such as skill, readme, index, unnamed-skill, absolute paths, drive letters, nested paths, or `..` segments as installable names., The slice implements interactive prompting or cobra command wiring; those belong to the dependent install-binding row.
- Degraded mode: The helper returns evidence values url_skill_invalid_url, url_skill_missing_name, url_skill_invalid_name, or url_skill_invalid_frontmatter; callers can render actionable retry guidance without writing to the active skill store.
- Fixture: `internal/skills/url_candidate_test.go`
- Write scope: `internal/skills/url_candidate.go`, `internal/skills/url_candidate_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/skills -run '^TestURLSkillCandidate_' -count=1`, `go test ./internal/skills -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: URL candidate fixtures prove frontmatter-name, URL-slug, missing-name evidence, unsafe-name rejection, source/trust metadata, and no network/filesystem imports.
- Acceptance: TestURLSkillCandidate_FromFrontmatter resolves `name: sharethis-chat` to Name=sharethis-chat, AwaitingName=false, Source=url, Trust=community, and Files contains exactly SKILL.md., TestURLSkillCandidate_FromURLSlug resolves `https://example.com/tools/review-bot/SKILL.md` to Name=review-bot when frontmatter name is missing., TestURLSkillCandidate_MissingNameEvidence returns AwaitingName=true plus url_skill_missing_name for `https://example.com/SKILL.md` with no valid frontmatter name., TestURLSkillCandidate_RejectsUnsafeNames rejects sentinel, nested, absolute, drive-letter, and traversal names before any path is returned., The helper imports neither net/http nor internal/cli and performs no filesystem writes.
- Source refs: ../hermes-agent/tools/skills_hub.py@e63929d4:UrlSource.fetch,_validate_skill_name, ../hermes-agent/hermes_cli/skills_hub.py@e63929d4:_is_valid_installed_skill_name, ../hermes-agent/tests/hermes_cli/test_skills_hub.py@e63929d4:test_url_install_actionable_error_on_non_interactive_with_no_name, internal/skills/parser.go, internal/skills/types.go
- Unblocks: Skills hub direct URL install name/category guard
- Why now: Unblocks Skills hub direct URL install name/category guard.

## 6. MCP stdio orphan cleanup after cron ticks

- Phase: 5 / 5.G
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Cron and MCP stdio tooling track orphaned stdio server PIDs after cancellation/timeout and sweep only orphaned children after a cron tick joins, without killing active MCP sessions from parallel work
- Trust class: operator, system
- Ready when: MCP stdio transport + tool/list discovery and Cronjob tool action envelope over native store are validated on main., The worker can implement this as a pure process-tracker/reaper seam with fake PID liveness and fake kill functions; no real subprocess, cron goroutine, or MCP SDK is required., Cleanup after a cron tick must run only after sibling futures/jobs for that tick have joined.
- Not ready when: The slice kills every active MCP PID during normal cron ticks instead of orphan-only cleanup., The slice starts real MCP stdio subprocesses or depends on OS-specific process groups in unit tests., The slice changes MCP protocol parsing, OAuth, HTTP transport, managed gateway behavior, or cron schedule parsing.
- Degraded mode: MCP status reports mcp_orphan_reaped, mcp_orphan_reap_failed, or mcp_active_pid_preserved instead of leaking detached stdio subprocesses or killing active sessions.
- Fixture: `internal/tools/mcp_orphan_cleanup_test.go`
- Write scope: `internal/tools/mcp_orphan_cleanup.go`, `internal/tools/mcp_orphan_cleanup_test.go`, `internal/tools/mcp_stdio.go`, `internal/cron/scheduler.go`, `internal/cron/scheduler_mcp_cleanup_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools -run '^TestMCPOrphanCleanup_' -count=1`, `go test ./internal/cron -run '^TestCronSchedulerRunsMCPOrphanCleanupAfterTickJoin$' -count=1`, `go test ./internal/tools ./internal/cron -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: MCP/cron fixtures prove cancelled stdio PIDs become orphaned, cron tick cleanup reaps only orphans after join, shutdown can include active PIDs, and no real subprocess is spawned.
- Acceptance: TestMCPOrphanCleanup_MarksAlivePIDOrphanOnSessionExit moves an injected alive PID from active to orphaned when a stdio session exits through cancellation/error., TestMCPOrphanCleanup_ReapsOnlyOrphansAfterCronTick kills orphaned PIDs after a fake cron tick join while preserving active PIDs., TestMCPOrphanCleanup_ShutdownIncludesActive preserves existing shutdown behavior that can reap both active and orphaned PIDs., TestCronSchedulerRunsMCPOrphanCleanupAfterTickJoin proves cleanup is called after all fake tick jobs complete and never while a sibling job is still marked active.
- Source refs: ../hermes-agent/tools/mcp_tool.py@930494d6:_orphan_stdio_pids, ../hermes-agent/cron/scheduler.py@930494d6:tick cleanup, ../hermes-agent/tests/tools/test_mcp_stability.py@930494d6, internal/tools/mcp_stdio.go, internal/cron/scheduler.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 7. Checkpoint shadow-repo GC policy

- Phase: 5 / 5.L
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Native checkpoint manager prunes orphan and stale shadow repositories at startup using a deterministic policy before any write-capable file tools depend on rollback state
- Trust class: operator, child-agent, system
- Ready when: The row can be implemented as pure filesystem fixtures under t.TempDir with fake timestamps and no model/tool execution., internal/tools exists and can own the checkpoint manager contract without exposing write_file or patch tools yet., Rollback state paths are Gormes-owned XDG paths, not upstream ~/.hermes paths.
- Not ready when: The slice exposes write_file, patch, or checkpoint restore tools before the cleanup/read-model contract is fixture-locked., The slice shells out to git or deletes real repositories outside t.TempDir in tests., The slice copies Hermes home layout instead of documenting the Gormes XDG rollback directory decision.
- Degraded mode: Checkpoint status reports shadow_gc_unavailable, orphan_shadow_repo, or stale_shadow_repo evidence instead of silently leaving rollback directories to accumulate.
- Fixture: `internal/tools/checkpoint_manager_test.go`
- Write scope: `internal/tools/checkpoint_manager.go`, `internal/tools/checkpoint_manager_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools -run '^TestCheckpointManager' -count=1`, `go test ./internal/tools -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Checkpoint manager fixtures prove startup orphan/stale shadow cleanup, dry-run reporting, fake-clock TTL behavior, and redacted status evidence under t.TempDir only.
- Acceptance: TestCheckpointManagerPrunesOrphanShadowRepos seeds an active shadow repo and an orphan repo under t.TempDir; startup cleanup removes only the orphan and records evidence., TestCheckpointManagerPrunesStaleShadowRepos uses a fake clock to remove stale shadows older than the configured TTL while preserving fresh active shadows., TestCheckpointManagerDryRunReportsCandidates returns the same orphan/stale candidates without deleting them., Status evidence names counts and paths relative to the checkpoint root, with no absolute home-directory leakage.
- Source refs: ../hermes-agent/tools/checkpoint_manager.py@478444c2, ../hermes-agent/tests/tools/test_checkpoint_manager.py@478444c2, ../hermes-agent/cli.py@478444c2:startup checkpoint cleanup, ../hermes-agent/gateway/run.py@478444c2:startup checkpoint cleanup, docs/content/building-gormes/architecture_plan/phase-5-final-purge.md
- Unblocks: File read dedup cache invalidation and wrapper guard
- Why now: Unblocks File read dedup cache invalidation and wrapper guard.

## 8. Session search

- Phase: 5 / 5.N
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: Native session_search tool uses the Phase 3 session catalog with same-chat defaults, opt-in user/source filters, and deterministic recent-mode exclusion of the current lineage root
- Trust class: operator, child-agent, system
- Ready when: Source-filtered session/message search core and Lineage-aware source-filtered search hits are validated on main., Operator-auditable search evidence is validated on main, so this row is unblocked for the tool wrapper only., The tool can use seeded SQLite/session.Metadata fixtures and does not need a live gateway, provider, or Goncho cloud service., Existing Goncho/Honcho-compatible scope rules remain the authority for user/source widening., This row must wrap existing internal/memory.SearchSessions/SearchMessages and must not change ranking, lineage construction, default same-chat fences, or Goncho persistence.
- Not ready when: The slice changes ranking, default same-chat recall fences, or Goncho/Honcho memory persistence instead of wrapping existing search results., The slice shells out to Hermes Python or reads ~/.hermes session logs., The slice edits internal/memory/session_catalog.go or internal/goncho/service.go; those prerequisites are already validated and should be treated as read-only donor code., The slice includes todo/debug/clarify tools or cronjob tools in the same change.
- Degraded mode: Tool result reports session_search_unavailable, source_filter_denied, or lineage_root_excluded evidence instead of widening recall silently.
- Fixture: `internal/tools/session_search_tool_test.go`
- Write scope: `internal/tools/session_search_tool.go`, `internal/tools/session_search_tool_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools -run '^TestSessionSearchTool_' -count=1`, `go test ./internal/tools ./internal/memory ./internal/goncho -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Session search tool fixtures prove same-chat defaults, opt-in user/source widening, deterministic current-lineage-root exclusion in recent mode, and degraded evidence for unavailable/denied widening.
- Acceptance: TestSessionSearchTool_SameChatDefault seeds two chats and proves no cross-chat hit appears without explicit scope=user or sources., TestSessionSearchTool_UserScopeSourceFilter passes scope=user sources=[telegram] and proves only the allowed source's sessions are returned with source evidence., TestSessionSearchTool_RecentModeExcludesCurrentLineageRoot seeds root and compressed child sessions, runs recent mode from the child, and proves the current root is excluded deterministically per Hermes dbe50155., TestSessionSearchTool_DegradedEvidence covers missing session directory and denied source widening without panics or hidden fallback widening.
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
- Fixture: `internal/cli/onboarding_test.go`
- Write scope: `internal/cli/onboarding.go`, `internal/cli/onboarding_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/cli -run '^Test.*OpenClaw\|^TestOnboardingSeen\|^TestMarkOnboardingSeen' -count=1`, `go test ./internal/cli -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: CLI onboarding fixtures prove directory-only OpenClaw residue detection, Gormes-specific cleanup hint text, in-memory seen-state handling, malformed-config tolerance, and no real HOME/config writes.
- Acceptance: TestDetectOpenClawResidue_DirectoryOnly returns true for a temp HOME containing a .openclaw directory and false for missing path or a regular file., TestOpenClawResidueHint_MentionsInjectedCleanupCommand proves the hint contains ~/.openclaw, the injected Gormes cleanup command, and no `hermes claw cleanup` substring., TestOnboardingSeen_MalformedConfigUnseen covers missing onboarding, non-map onboarding, non-map seen, false values, and unrelated flags., TestMarkOnboardingSeen_InMemoryPreservesOtherFlags sets openclaw_residue_cleanup=true in the provided map without removing existing seen flags., No test touches the real user home or writes config.yaml.
- Source refs: ../hermes-agent/agent/onboarding.py@e63929d4:OPENCLAW_RESIDUE_FLAG,detect_openclaw_residue,openclaw_residue_hint_cli,is_seen, ../hermes-agent/tests/agent/test_onboarding.py@e63929d4:TestOpenClawResidue, ../hermes-agent/cli.py@e63929d4:first-time OpenClaw-residue banner, internal/cli/tips.go
- Unblocks: OpenClaw residue startup banner binding
- Why now: Unblocks OpenClaw residue startup banner binding.

## 10. CLI bracketed-paste wrapper sanitizer

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

<!-- PROGRESS:END -->
