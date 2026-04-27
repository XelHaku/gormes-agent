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
## 1. Gateway message deduplicator bounded helper

- Phase: 2 / 2.B.5
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Shared gateway exposes a pure in-memory MessageDeduplicator that caps tracked message IDs at max_size, evicts the oldest ID deterministically, and returns duplicate/evicted/disabled evidence without touching manager routing
- Trust class: gateway, system
- Ready when: The helper can be tested as a pure in-memory structure with synthetic message IDs; no live platform SDK or manager run loop is required., internal/gateway/message_deduplicator.go and internal/gateway/message_deduplicator_test.go are the only runtime files needed for this first slice., When closing the row, set status to complete and keep contract_status as validated; never use status=validated.
- Not ready when: The slice changes gateway.Manager, channel session keys, authorization/pairing policy, message rendering, or outbound coalescing., The slice adds platform-specific Telegram/Slack/Discord dedup state instead of a shared helper., The slice stores dedup history on disk or wires the helper into inbound dispatch; manager wiring is the dependent row.
- Degraded mode: Gateway helper reports deduplicator_disabled, duplicate_message, or deduplicator_evicted evidence instead of growing unbounded in long-running platform adapters.
- Fixture: `internal/gateway/message_deduplicator_test.go::TestMessageDeduplicator_MaxSizeEvictsOldest`
- Write scope: `internal/gateway/message_deduplicator.go`, `internal/gateway/message_deduplicator_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/gateway -run '^TestMessageDeduplicator_' -count=1`, `go test ./internal/gateway -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Gateway helper fixtures prove bounded oldest eviction, duplicate rejection, disabled mode, and evidence values without platform SDKs, manager wiring, or disk state.
- Acceptance: TestMessageDeduplicator_MaxSizeEvictsOldest adds max_size+1 distinct IDs and proves the first ID is no longer considered duplicate while the newest IDs remain tracked., TestMessageDeduplicator_DuplicateReturnsSeen proves repeated IDs are rejected before eviction and emits duplicate evidence., TestMessageDeduplicator_ZeroMaxSizeDisabled treats max_size=0 as disabled, never rejects messages, and never allocates a seen-ID queue., The helper exposes enough evidence for a later manager row to distinguish duplicate_message, deduplicator_evicted, and deduplicator_disabled without importing platform adapters.
- Source refs: ../hermes-agent/gateway/platforms/helpers.py@cebf9585:MessageDeduplicator, ../hermes-agent/tests/gateway/test_message_deduplicator.py@cebf9585, internal/gateway/event.go, internal/gateway/manager.go
- Unblocks: Gateway inbound dedup evidence wiring
- Why now: Unblocks Gateway inbound dedup evidence wiring.

## 2. Durable worker RSS watchdog policy helper

- Phase: 2 / 2.E.3
- Owner: `orchestrator`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: Gormes durable worker exposes a pure RSS watchdog policy helper that classifies disabled mode, unavailable RSS reads, threshold exceeded evidence, and stable watchdog restart reset without integrating with the worker loop yet
- Trust class: operator, system
- Ready when: Durable worker execution loop and Durable worker abort-slot recovery safety net are validated on main., The worker should read sibling GBrain files through the absolute /home/xel/git/sages-openclaw/workspace-mineru/gbrain path above, not by running git show in the Gormes checkout., The helper can be tested with fake RSS reader, fake clock, and value-only policy structs; no real process RSS, worker goroutine, external supervisor, shell handler, or GBrain TypeScript runtime is required.
- Not ready when: The slice changes DurableWorker.RunOne, doctor/status output, cmd/gormes lifecycle, builder-loop backend watchdogs, live delegate_task behavior, or cron execution semantics., The tests require real RSS measurements, sleeps longer than 100ms, subprocess workers, systemd, Postgres/PGLite, or importing GBrain code., The slice adds process self-termination to the main Gormes process or internal Goncho memory service.
- Degraded mode: Durable-worker policy reports rss_watchdog_disabled, rss_threshold_exceeded, rss_watchdog_unavailable, or stable_watchdog_restart before runtime drain wiring exists.
- Fixture: `internal/subagent/durable_worker_rss_watchdog_test.go::TestDurableWorkerRSSWatchdogPolicy`
- Write scope: `internal/subagent/durable_worker_rss_watchdog.go`, `internal/subagent/durable_worker_rss_watchdog_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/subagent -run 'TestDurableWorkerRSSWatchdog_\|TestDurableWorkerWatchdogRestartPolicy_' -count=1`, `go test ./internal/subagent -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Durable worker RSS policy fixtures prove disabled mode, threshold evidence, read-failure degradation, and stable watchdog restart classification without runtime drain or doctor/status edits.
- Acceptance: TestDurableWorkerRSSWatchdog_DisabledAtZero proves max_rss_mb=0 records rss_watchdog_disabled and never reads the RSS seam., TestDurableWorkerRSSWatchdog_ThresholdExceeded injects an RSS value over max_rss_mb and returns rss_threshold_exceeded evidence with observed_mb, max_mb, and checked_at from the fake clock., TestDurableWorkerRSSWatchdog_RSSReadFailure returns rss_watchdog_unavailable evidence and does not request a drain., TestDurableWorkerWatchdogRestartPolicy_StableRunReset classifies a watchdog exit after at least five minutes as stable_watchdog_restart and does not increment crash count past one.
- Source refs: ../gbrain/CHANGELOG.md@c78c3d0:v0.22.2, /home/xel/git/sages-openclaw/workspace-mineru/gbrain/src/core/minions/types.ts@c78c3d0:MinionWorkerOpts.maxRssMb/getRss/rssCheckInterval, /home/xel/git/sages-openclaw/workspace-mineru/gbrain/src/core/minions/worker.ts@c78c3d0:checkMemoryLimit, /home/xel/git/sages-openclaw/workspace-mineru/gbrain/src/commands/autopilot.ts@c78c3d0:stable-run reset, /home/xel/git/sages-openclaw/workspace-mineru/gbrain/test/minions.test.ts@c78c3d0:--max-rss watchdog, ../gbrain/src/core/minions/types.ts@c78c3d0:MinionWorkerOpts.maxRssMb/getRss/rssCheckInterval, ../gbrain/src/core/minions/worker.ts@c78c3d0:checkMemoryLimit/gracefulShutdown, ../gbrain/src/commands/autopilot.ts@c78c3d0:stable-run reset, ../gbrain/test/minions.test.ts@c78c3d0:MinionWorker --max-rss watchdog, internal/subagent/durable_worker.go, internal/subagent/durable_worker_abort_test.go
- Unblocks: Durable worker RSS drain integration
- Why now: Unblocks Durable worker RSS drain integration.

## 3. Steer slash command registry + queue fallback

- Phase: 2 / 2.F.5
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Contract: Registry-owned /steer command accepts operator/gateway text during a busy turn, validates/truncates preview text, and queues visible guidance when no running-agent steer hook is available
- Trust class: operator, gateway
- Ready when: Concurrent-tool cancellation (2.E.2) and the shared CommandDef registry are complete on main., This slice only registers /steer and queue fallback behavior; the live between-tool-call injection hook remains in the dependent row., Tests can use fake running-agent state and fake command dispatch; no provider, active tool loop, TUI, Slack, or Telegram transport is required.
- Not ready when: The implementation tries to inject mid-run prompts instead of only registering /steer and queue fallback behavior., The slice changes /queue, /bg, /busy config persistence, TUI keybindings, or platform adapter code., The slice accepts empty/image-bearing steer payloads instead of returning usage/fallback evidence.
- Degraded mode: Gateway returns visible usage, busy, or queued status instead of dropping steer text when the command cannot run immediately.
- Fixture: `internal/gateway/steer_command_test.go`
- Write scope: `internal/gateway/commands.go`, `internal/gateway/manager.go`, `internal/gateway/steer_command_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/gateway -run '^TestSteerCommandRegistry_' -count=1`, `go test ./internal/gateway -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Gateway command fixtures prove /steer argument validation, preview truncation, queued fallback, and no live mid-run injection.
- Acceptance: TestSteerCommandRegistry_ValidatesArgs exposes /steer with missing-argument usage text and rejects image-bearing payloads., TestSteerCommandRegistry_PreviewTruncation returns deterministic preview text for long guidance., TestSteerCommandRegistry_NoRunningAgentQueuesGuidance queues follow-up guidance instead of dropping it., TestSteerCommandRegistry_RunningAgentFallbackDoesNotInject proves running-agent paths return explicit steer_unavailable/queued evidence until the mid-run hook row lands.
- Source refs: ../hermes-agent/cli.py@635253b9:busy_input_mode=steer, ../hermes-agent/gateway/run.py@635253b9:running_agent.steer, ../hermes-agent/hermes_cli/commands.py@635253b9:/busy steer, ../hermes-agent/tests/gateway/test_busy_session_ack.py@635253b9, internal/gateway/commands.go, internal/gateway/manager.go
- Unblocks: Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard
- Why now: Unblocks Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard.

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

## 5. MCP stdio orphan cleanup after cron ticks

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

## 6. Checkpoint shadow-repo GC policy

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

## 7. Session search

- Phase: 5 / 5.N
- Owner: `tools`
- Size: `small`
- Status: `planned`
- Contract: Native session_search tool uses the Phase 3 session catalog with same-chat defaults, opt-in user/source filters, and deterministic recent-mode exclusion of the current lineage root
- Trust class: operator, child-agent, system
- Ready when: Source-filtered session/message search core and Lineage-aware source-filtered search hits are validated on main., Operator-auditable search evidence is validated on main, so this row is unblocked for the tool wrapper only., The tool can use seeded SQLite/session.Metadata fixtures and does not need a live gateway, provider, or Goncho cloud service., Existing Goncho/Honcho-compatible scope rules remain the authority for user/source widening.
- Not ready when: The slice changes ranking, default same-chat recall fences, or Goncho/Honcho memory persistence instead of wrapping existing search results., The slice shells out to Hermes Python or reads ~/.hermes session logs., The slice includes todo/debug/clarify tools or cronjob tools in the same change.
- Degraded mode: Tool result reports session_search_unavailable, source_filter_denied, or lineage_root_excluded evidence instead of widening recall silently.
- Fixture: `internal/tools/session_search_tool_test.go`
- Write scope: `internal/tools/session_search_tool.go`, `internal/tools/session_search_tool_test.go`, `internal/memory/session_catalog.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/tools -run '^TestSessionSearchTool_' -count=1`, `go test ./internal/tools ./internal/memory ./internal/goncho -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: Session search tool fixtures prove same-chat defaults, opt-in user/source widening, deterministic current-lineage-root exclusion in recent mode, and degraded evidence for unavailable/denied widening.
- Acceptance: TestSessionSearchTool_SameChatDefault seeds two chats and proves no cross-chat hit appears without explicit scope=user or sources., TestSessionSearchTool_UserScopeSourceFilter passes scope=user sources=[telegram] and proves only the allowed source's sessions are returned with source evidence., TestSessionSearchTool_RecentModeExcludesCurrentLineageRoot seeds root and compressed child sessions, runs recent mode from the child, and proves the current root is excluded deterministically per Hermes dbe50155., TestSessionSearchTool_DegradedEvidence covers missing session directory and denied source widening without panics or hidden fallback widening.
- Source refs: ../hermes-agent/tools/session_search_tool.py@dbe50155, ../hermes-agent/tests/tools/test_session_search.py@dbe50155, internal/memory/session_catalog.go, internal/memory/session_lineage_search_test.go, internal/goncho/service.go
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 8. Native TUI /save XDG file writer binding

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: cmd/gormes binds the native TUI /save SessionExportFunc to the canonical persisted transcript reader and writes markdown exports under the Gormes XDG data directory, never under upstream HERMES_HOME or the process working directory
- Trust class: operator, system
- Ready when: Native TUI /save canonical session export is validated on main and exposes SessionExportFunc on Model options., The prior /save handler write-scope issue is closed; slash_dispatch_test.go was already included in the completed handler row and should not be touched here., cmd/gormes/session.go already proves transcript.ExportMarkdown works against config.MemoryDBPath for the CLI export command., Gormes intentionally uses XDG_DATA_HOME/gormes instead of HERMES_HOME; the fixture should assert that divergence.
- Not ready when: The slice reopens internal/tui slash handler behavior or changes transcript.ExportMarkdown output format., The slice writes exports under HERMES_HOME, the repository root, or the current working directory., The slice starts a provider, API server, remote TUI gateway, or live session browser.
- Degraded mode: TUI status reports `save: store unavailable` or `save: write failed: <err>` with partial-file cleanup instead of exposing an unwired /save handler or writing to an ambiguous location.
- Fixture: `cmd/gormes/tui_save_export_test.go`
- Write scope: `cmd/gormes/main.go`, `cmd/gormes/tui_save_export_test.go`, `internal/tui/slash_save.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./cmd/gormes -run '^TestTUISaveExport_' -count=1`, `go test ./cmd/gormes ./internal/tui ./internal/transcript -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: cmd/gormes TUI save-export fixtures prove the real SessionExportFunc writes under XDG_DATA_HOME/gormes/sessions/exports, ignores HERMES_HOME for runtime state, cleans partial files, and returns the exported path to /save.
- Acceptance: cmd/gormes constructs a SessionExportFunc for the local TUI Model that opens config.MemoryDBPath, calls transcript.ExportMarkdown, and writes `<session-id>.md` or a collision-safe variant under `$XDG_DATA_HOME/gormes/sessions/exports/`., TestTUISaveExport_WritesUnderXDGDataHome sets XDG_DATA_HOME and HERMES_HOME to different temp roots, invokes the bound SessionExportFunc, and proves the file lands under the Gormes XDG export directory only., TestTUISaveExport_RemovesPartialOnFailure injects a write failure after creating a partial file and proves /save removes it through the existing slash handler cleanup path., The status message returned by /save contains the XDG export path and no test opens a network connection or starts the remote TUI gateway.
- Source refs: ../hermes-agent/tests/cli/test_save_conversation_location.py@5eb6cd82, ../hermes-agent/cli.py@5eb6cd82:save conversation location, ../hermes-agent/tui_gateway/server.py@5eb6cd82:session.save, internal/tui/slash_save.go:SessionExportFunc, cmd/gormes/session.go:sessionExportCmd, internal/config/config.go:MemoryDBPath
- Why now: Contract metadata is present; ready for a focused spec or fixture slice.

## 9. API server detailed health read-model

- Phase: 5 / 5.Q
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P2`
- Contract: Native API server exposes a detailed health read-model that reports provider, response-store, run-event stream, gateway, and cron availability/degradation without leaking secrets or adding cron mutations
- Trust class: operator, gateway, system
- Ready when: OpenAI-compatible chat-completions API server, Responses API store + run event stream, and Gateway proxy mode forwarding contract are validated on main., Cronjob tool action envelope over native store, Cron context_from output chaining, and Cron multi-target delivery + media/live-adapter fallback are validated on main, so cron health can be read without defining admin writes., Tests can inject fake provider/gateway/cron/read-store health snapshots; no provider, live gateway, scheduler goroutine, or cron mutation endpoint is required.
- Not ready when: The slice creates /api/jobs endpoints, mutates cron jobs, starts a scheduler, changes OpenAI-style error envelopes, or weakens API auth/body-size checks., The slice imports Hermes Python, starts the remote TUI gateway, or changes dashboard client behavior., Health output includes plaintext tokens, provider API keys, cron script bodies, or raw request payloads.
- Degraded mode: HTTP health reports cron_unavailable, response_store_disabled, run_events_unavailable, gateway_unavailable, or provider_unconfigured evidence instead of returning a flat OK/failed bit.
- Fixture: `internal/apiserver/detailed_health_test.go`
- Write scope: `internal/apiserver/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/apiserver -run '^TestAPIServerDetailedHealth_' -count=1`, `go test ./internal/apiserver -count=1`, `go run ./cmd/builder-loop progress validate`
- Done signal: API server fixtures prove detailed health sections, degraded evidence, secret redaction, and existing auth/error-envelope behavior without cron admin writes.
- Acceptance: TestAPIServerDetailedHealth_AllSystemsReady returns provider, response_store, run_events, gateway, and cron sections with status=ready and stable JSON field names., TestAPIServerDetailedHealth_DegradedEvidence seeds disabled response store, missing cron store, and unavailable gateway status and proves each appears as separate degraded evidence., TestAPIServerDetailedHealth_RedactsSecrets proves provider keys, cron script bodies, and gateway tokens are absent from the response body., Auth failure, request-size, and method-not-allowed behavior match the existing API server error-envelope fixtures.
- Source refs: ../hermes-agent/gateway/platforms/api_server.py@755a2804, internal/apiserver/, internal/cron/, internal/gateway/status.go
- Unblocks: API server cron admin read-only endpoints
- Why now: Unblocks API server cron admin read-only endpoints.

## 10. BlueBubbles iMessage bubble formatting parity

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

<!-- PROGRESS:END -->
