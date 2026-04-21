---
title: "Upstream Subsystem Inventory"
weight: 80
---

## 7. Upstream Subsystem Inventory

The complete picture of what Gormes must absorb to retire the Python `hermes-agent` runtime. Each row is one upstream module or capability, mapped to its target phase. This inventory is the source of truth for "what's left" — when a subsystem is shipped in Go, mark it ✅ and link the spec.

### Gateway platforms (24 connectors — 21 unshipped)

| Platform | Upstream file | Target phase | Status | Landed Go surface |
|---|---|---|---|---|
| Telegram | `gateway/platforms/telegram.py` | 2.B.1 | ✅ shipped | `gormes/internal/telegram/*`, `gormes/cmd/gormes/telegram.go` |
| Discord | `gateway/platforms/discord.py` | 2.B.2 | ✅ shipped | `gormes/internal/discord/*`, `gormes/cmd/gormes/discord.go` |
| Slack | `gateway/platforms/slack.py` | 2.B.3 | ✅ shipped | `gormes/internal/slack/*`, `gormes/cmd/gormes/slack.go` |
| WhatsApp | `gateway/platforms/whatsapp.py` | 2.B.4 | ⏳ planned | |
| Signal | `gateway/platforms/signal.py` | 2.B.5 | ⏳ planned | |
| Email | `gateway/platforms/email.py` | 2.B.6 | ⏳ planned | |
| SMS | `gateway/platforms/sms.py` | 2.B.7 | ⏳ planned | |
| Matrix | `gateway/platforms/matrix.py` | 2.B.8 | ⏳ planned | |
| Mattermost | `gateway/platforms/mattermost.py` | 2.B.9 | ⏳ planned | |
| Webhook | `gateway/platforms/webhook.py` | 2.B.10 | ⏳ planned | |
| BlueBubbles (iMessage) | `gateway/platforms/bluebubbles.py` | 2.B.11 | ⏳ planned | |
| HomeAssistant | `gateway/platforms/homeassistant.py` | 2.B.12 | ⏳ planned | |
| Feishu | `gateway/platforms/feishu*.py` | 2.B.13 | ⏳ planned | |
| WeChat (WeCom + WeiXin) | `gateway/platforms/wecom*.py`, `weixin.py` | 2.B.14 | ⏳ planned | |
| DingTalk | `gateway/platforms/dingtalk.py` | 2.B.15 | ⏳ planned | |
| QQ Bot | `gateway/platforms/qqbot/` | 2.B.16 | ⏳ planned | |

### Operational layer (cross-cutting, mostly Phase 2.D–2.F)

| Subsystem | Upstream | Target phase | Status |
|---|---|---|---|
| Gateway runtime entry (main loop + slash-command dispatch) | `gateway/run.py` + `gateway/config.py` | 2.B/2.F | ⏳ planned |
| Gateway session store (conversation persistence across platforms) | `gateway/session.py` (`SessionStore`, `SessionEntry`, `SessionSource`, `SessionResetPolicy`) | 2.B/2.F | ⏳ planned |
| Gateway session context | `gateway/session_context.py` (`SessionContext`) | 2.B/2.F | ⏳ planned |
| Delivery router (`--deliver <platform>` abstraction) | `gateway/delivery.py` (`DeliveryRouter`, `DeliveryTarget`) | 2.B/2.F | ⏳ planned |
| Stream consumer (SSE agent-event fan-out to gateway) | `gateway/stream_consumer.py` (`GatewayStreamConsumer`, `StreamConsumerConfig`, `StreamingConfig`) | 2.B/2.F | ⏳ planned |
| Home channel (operator's primary notify-to chat) | `gateway/*` — `HomeChannel` class | 2.F | ⏳ planned |
| Channel / contact directory | `gateway/channel_directory.py` | 2.F | ⏳ planned |
| Platform enum + per-platform config | `gateway/*` — `Platform` (enum), `PlatformConfig` | 2.B | ⏳ planned |
| Cron / scheduled automations | `cron/scheduler.py`, `cron/jobs.py`, `tools/cronjob_tools.py` | 2.D | ✅ shipped (scheduler + bbolt `cron_jobs` bucket + SQLite `cron_runs` audit + CRON.md mirror + Heartbeat prefix + exact-match `[SILENT]` suppression + kernel `PlatformEvent.SessionID/CronJobID` per-event override; upstream's file tick lock not needed — single-process) |
| Webhook subscription system (GitHub events / API triggers → prompt → deliver) | `hermes_cli/webhook.py` + gateway routing | 2.D | ⏳ planned |
| Subagent delegation | `tools/delegate_tool.py` | 2.E | ⏳ planned |
| Hooks system (`HookRegistry`) | `gateway/hooks.py`, `gateway/builtin_hooks/{boot_md}.py` | 2.F | ⏳ planned |
| Restart / pairing / lifecycle | `gateway/{restart,pairing,status}.py` + `PairingStore` | 2.F | ⏳ planned |
| Mirror / sticker cache | `gateway/{mirror,sticker_cache}.py` | 2.F | ⏳ planned |
| Display config + KawaiiSpinner + tool preview formatting | `gateway/display_config.py`, `agent/display.py` (`KawaiiSpinner`) | 2.F / 5.Q | ⏳ planned |
| Iteration budget tracker | `run_agent.py` (`iteration_budget`) — inline class | 4.C | ⏳ planned |

### Memory + state (Phase 3 — 3.A–3.D shipped; 3.E pending)

Upstream splits memory across three stores that Gormes compresses into two:

- **`hermes_state.py` — `SessionDB`** (SQLite + FTS5) holds every session's message history, model config, parent-session chains for compression splits, and source tagging (`cli`, `telegram`, etc.). Gormes Phase 2.C uses bbolt for (platform, chat_id) → session_id mapping; Phase 3.A's SqliteStore holds turns + FTS5. Together they cover SessionDB's responsibilities, but the parent-session chains and cross-source search need explicit 3.E work.
- **`agent/memory_manager.py` — `MemoryManager`** owns the entity graph + USER.md mirror.
- **`agent/memory_provider.py` — `MemoryProvider` (ABC)** owns recall-time seed selection + fence assembly.

| Subsystem | Upstream | Target phase | Status |
|---|---|---|---|
| SQLite + FTS5 lattice | `agent/memory_provider.py` (lexical half) + `hermes_state.py` (SessionDB FTS5) | 3.A | ✅ shipped |
| Ontological graph + extractor | `agent/memory_manager.py` | 3.B | ✅ shipped |
| Recall + context injection | `agent/memory_provider.py` (recall half) | 3.C | ✅ shipped |
| Semantic / embeddings | (not in upstream; Gormes-original) | 3.D | ✅ shipped |
| USER.md mirror | `agent/memory_manager.py` (mirror writer) | 3.D.5 | ✅ shipped |
| Session index mirror | None (closes bbolt opacity gap) | 3.E.1 | ⏳ planned |
| Tool execution audit log | None (exceeds Hermes) | 3.E.2 | ⏳ planned |
| Transcript export command | None (exceeds Hermes; Hermes has no text export) | 3.E.3 | ⏳ planned |
| Extraction state visibility | None (debug visibility) | 3.E.4 | ⏳ planned |
| Insights audit log (lightweight) | `agent/insights.py` (preview; full port in 4.E) | 3.E.5 | ⏳ planned |
| Memory decay | None (Gormes-original) | 3.E.6 | ⏳ planned |
| Cross-chat synthesis | `agent/memory_manager.py` (cross-session) | 3.E.7 | ⏳ planned |
| Parent-session chains (compression splits) | `hermes_state.py` (`SessionDB.parent_session_id`) | 3.E.8 | ⏳ planned (pairs with 4.B context compression) |
| Cross-source session search | `hermes_state.py` (FTS5 across source-tagged messages) | 3.E.8 | ⏳ planned |

### Cross-cutting registries (used by multiple phases)

These are single source-of-truth registries that drive multiple downstream consumers. A Go port must preserve "one registry, many consumers" so that adding a slash command / tool / skill lights up everywhere automatically.

| Subsystem | Upstream | Target phase | Status | Why it's cross-cutting |
|---|---|---|---|---|
| Slash command registry | `hermes_cli/commands.py` (`COMMAND_REGISTRY`, `CommandDef`, `resolve_command`, `gateway_help_lines`, `telegram_bot_commands`, `slack_subcommand_map`, `COMMANDS_BY_CATEGORY`, `SlashCommandCompleter`) | 2.F / 5.O | ⏳ planned | One `CommandDef` entry drives CLI dispatch, gateway dispatch, Telegram BotCommand menu, Slack `/hermes` subcommand map, autocomplete, and `/help` output |
| Tool registry + dispatch orchestrator | `tools/registry.py` + `model_tools.py` (`get_tool_definitions`, `handle_function_call`, `TOOL_TO_TOOLSET_MAP`, `TOOLSET_REQUIREMENTS`, `check_toolset_requirements`) | 2.A (partial ✅) / 5.A | 🔨 Gormes `internal/tools` covers the core dispatch; toolset grouping + requirements check not ported | Every tool self-registers at import time; `model_tools` exposes the API consumed by run_agent, cli, batch_runner, RL environments, and doctor |
| Toolset definitions (enabled/disabled groupings) | `toolsets.py` + `toolset_distributions.py` (`_HERMES_CORE_TOOLS` list) | 4.C / 5.A | ⏳ planned | Agent init accepts `enabled_toolsets` / `disabled_toolsets` lists — drives what tools the LLM sees per run |
| Canonical OpenAI-format message schema | `run_agent.py` — `{role, content, tool_calls, reasoning}` | 4.C | 🔨 partial (kernel already uses this shape) | Every provider adapter in 4.A must translate to/from this shape |

### Agent orchestration core (Phase 4 — the thing Phase 4 ultimately replaces)

The biggest single file upstream is `run_agent.py` at **12,113 lines** — the `AIAgent` orchestrator that owns the full agent loop. `agent/` is its partial decomposition. Phase 4 is the gradual absorption of this orchestrator into native Go; Phases 4.A–4.H each carve off a responsibility.

| Subsystem | Upstream | Target phase | Status |
|---|---|---|---|
| **AIAgent orchestrator** | `run_agent.py` (12,113 lines) | 4.C / 4.E | ⏳ planned (the Phase 4 centerpiece) |
| Top-level CLI dispatcher | `cli.py` (10,570 lines) | 5.O | ⏳ planned |
| MCP server mode | `mcp_serve.py` | 5.G | ⏳ planned |
| Toolset configuration | `toolsets.py`, `toolset_distributions.py` | 4.C / 5.A | ⏳ planned |
| Model / provider admin tools | `model_tools.py` | 5.O | ⏳ planned |
| Batch runner | `batch_runner.py` | 5.O | ⏳ planned |
| Mini SWE runner | `mini_swe_runner.py` | 5.M or 5.O | ⏳ planned |
| RL training CLI + compressor | `rl_cli.py`, `trajectory_compressor.py` | 5.M | ⏳ deferred (research) |
| Runtime shared helpers | `hermes_constants.py`, `hermes_logging.py`, `hermes_state.py`, `hermes_time.py`, `utils.py` | 5.O | ⏳ planned |
| Per-model tool-call parsers | `environments/tool_call_parsers/{deepseek_v3_parser,deepseek_v3_1_parser,glm45_parser,glm47_parser,hermes_parser,kimi_k2_parser,llama_parser,longcat_parser,mistral_parser}.py` | 4.A | ⏳ planned |
| Agent loop environment | `environments/agent_loop.py`, `environments/tool_context.py`, `environments/patches.py`, `environments/hermes_base_env.py`, `environments/agentic_opd_env.py`, `environments/web_research_env.py` | 4.C / 5.A | ⏳ planned |

### Brain (Phase 4 — sub-phases 4.A–4.H)

| Subsystem | Upstream | Target phase | Status |
|---|---|---|---|
| Anthropic adapter | `agent/anthropic_adapter.py` | 4.A | ⏳ planned |
| Bedrock adapter | `agent/bedrock_adapter.py` | 4.A | ⏳ planned |
| Gemini Cloud Code adapter | `agent/gemini_cloudcode_adapter.py` | 4.A | ⏳ planned |
| OpenRouter client | `agent/openrouter_client.py` | 4.A | ⏳ planned |
| Google Code Assist | `agent/google_code_assist.py` | 4.A | ⏳ planned |
| Copilot ACP client | `agent/copilot_acp_client.py` | 4.A | ⏳ planned |
| Auxiliary client (multi-provider: Anthropic, Codex, xAI) | `agent/auxiliary_client.py` (`AnthropicAuxiliaryClient`, `AsyncAnthropicAuxiliaryClient`, `CodexAuxiliaryClient`, `AsyncCodexAuxiliaryClient`) + `tools/xai_http.py` | 4.A | ⏳ planned |
| Auxiliary chat completion shims (ACP / Anthropic / Codex / Gemini) | `agent/*_adapter.py` internal `_*ChatShim`, `_*ChatCompletions`, `_*CompletionsAdapter`, `_*StreamChunk` classes | 4.A | ⏳ planned |
| Billing + cost + usage types | `agent/*` — `BillingRoute`, `CanonicalUsage`, `CostResult` classes | 4.E / 4.H | ⏳ planned |
| Provider failover | `agent/*` — `FailoverReason` enum + routing logic | 4.H | ⏳ planned |
| Model metadata types | `agent/model_metadata.py` — `ModelCapabilities`, `ModelInfo` classes | 4.D | ⏳ planned |
| Error classifier output type | `agent/error_classifier.py` — `ClassifiedError` class | 4.H | ⏳ planned |
| Local edit snapshot | `agent/*` — `LocalEditSnapshot` (for checkpoint rewind) | 5.L | ⏳ planned |
| Context engine | `agent/context_engine.py` | 4.B | ⏳ planned |
| Context compressor | `agent/context_compressor.py` + `manual_compression_feedback.py` | 4.B | ⏳ planned |
| Context references | `agent/context_references.py` | 4.B | ⏳ planned |
| Prompt builder | `agent/prompt_builder.py` | 4.C | ⏳ planned |
| Smart model routing | `agent/smart_model_routing.py` + `model_metadata.py` + `models_dev.py` | 4.D | ⏳ planned |
| Trajectory | `agent/trajectory.py` | 4.E | ⏳ planned |
| Insights | `agent/insights.py` | 4.E | ⏳ planned |
| Title generator | `agent/title_generator.py` | 4.F | ⏳ planned |
| Google OAuth | `agent/google_oauth.py` | 4.G | ⏳ planned |
| Credential pool | `agent/credential_pool.py` | 4.G | ⏳ planned |
| Credential files | `tools/credential_files.py` | 4.G | ⏳ planned |
| Rate limit tracker | `agent/rate_limit_tracker.py` + `nous_rate_guard.py` | 4.H | ⏳ planned |
| Retry utils | `agent/retry_utils.py` | 4.H | ⏳ planned |
| Prompt caching | `agent/prompt_caching.py` | 4.H | ⏳ planned |
| Subdirectory hints | `agent/subdirectory_hints.py` | 4.B | ⏳ planned |
| Skill commands / utils | `agent/skill_commands.py`, `agent/skill_utils.py` | 4.C | ⏳ planned |
| Error classifier | `agent/error_classifier.py` | 4.H | ⏳ planned |
| Redaction | `agent/redact.py` | 4.B | ⏳ planned |
| Usage / pricing | `agent/usage_pricing.py` | 4.E | ⏳ planned |

### Tools surface (Phase 5 — 61 upstream tool files)

| Category | Upstream tools | Target phase | Status |
|---|---|---|---|
| Sandboxing backends | `tools/environments/{base,local,docker,modal,managed_modal,modal_utils,daytona,singularity,ssh,file_sync}.py` | 5.B | ⏳ planned |
| Browser automation | `tools/browser_tool.py`, `browser_camofox*.py`, `browser_providers/{base,browserbase,browser_use,firecrawl}.py` | 5.C | ⏳ planned |
| Vision | `tools/vision_tools.py` | 5.D | ⏳ planned |
| Image generation | `tools/image_generation_tool.py` | 5.D | ⏳ planned |
| TTS / voice / transcription | `tools/{tts_tool,voice_mode,transcription_tools,neutts_synth}.py` + `neutts_samples/` | 5.E | ⏳ planned |
| Audio recorder (general + Termux) | `tools/*` — `AudioRecorder`, `TermuxAudioRecorder` | 5.E | ⏳ planned |
| Skills system (core) | `tools/{skill_manager_tool,skills_hub,skills_sync,skills_tool,skills_guard}.py`; `skills/` (26 categories) + `optional-skills/` (10+ categories) | 5.F | ⏳ planned |
| Skill metadata types | `tools/*` — `SkillMeta`, `SkillBundle`, `SkillReadinessStatus`, `HubLockFile` | 5.F | ⏳ planned |
| Skill source: SkillSource (ABC) | `tools/*` — `SkillSource` base | 5.F | ⏳ planned |
| Skill source: Claude Marketplace | `tools/*` — `ClaudeMarketplaceSource(SkillSource)` | 5.F | ⏳ planned |
| Skill source: ClawHub | `tools/*` — `ClawHubSource(SkillSource)` | 5.F | ⏳ planned |
| Skill source: GitHub | `tools/*` — `GitHubSource(SkillSource)` | 5.F | ⏳ planned |
| Skill source: Hermes Index | `tools/*` — `HermesIndexSource(SkillSource)` | 5.F | ⏳ planned |
| Skill source: LobeHub | `tools/*` — `LobeHubSource(SkillSource)` | 5.F | ⏳ planned |
| Skill source: Optional skills | `tools/*` — `OptionalSkillSource(SkillSource)` + `optional-skills/` tree | 5.F | ⏳ planned |
| Skill source: skills.sh | `tools/*` — `SkillsShSource(SkillSource)` | 5.F | ⏳ planned |
| Taps manager (plugin-source management) | `tools/*` — `TapsManager` | 5.F / 5.I | ⏳ planned |
| MCP integration | `tools/{mcp_tool,mcp_oauth,mcp_oauth_manager,managed_tool_gateway}.py` + `mcp_serve.py` + `MCPOAuthManager`, `MCPServerTask`, `ManagedToolGatewayConfig`, `SamplingHandler`, `OAuthNonInteractiveError`, `_ManagedFalSyncClient` classes | 5.G | ⏳ planned |
| ACP integration (IDE: VS Code / Zed / JetBrains) | `acp_adapter/{auth,entry,events,permissions,server,session,tools}.py` (runnable as `python -m acp_adapter`), `acp_registry/{agent.json,icon.svg}` | 5.H | ⏳ planned |
| Plugins architecture | `plugins/context_engine/`, `plugins/example-dashboard/` + plugin SDK | 5.I | ⏳ planned |
| Memory plugin: Byterover | `plugins/memory/byterover/` | 5.I | ⏳ planned |
| Memory plugin: Hindsight | `plugins/memory/hindsight/` | 5.I | ⏳ planned |
| Memory plugin: Holographic | `plugins/memory/holographic/` | 5.I | ⏳ planned |
| Memory plugin: Honcho (dialectic user modeling) | `plugins/memory/honcho/` | 5.I | ⏳ planned |
| Memory plugin: Mem0 | `plugins/memory/mem0/` | 5.I | ⏳ planned |
| Memory plugin: OpenViking | `plugins/memory/openviking/` | 5.I | ⏳ planned |
| Memory plugin: RetainDB | `plugins/memory/retaindb/` | 5.I | ⏳ planned |
| Memory plugin: Supermemory | `plugins/memory/supermemory/` | 5.I | ⏳ planned |
| Memory tool (plugin gateway) | `tools/memory_tool.py` | 5.I | ⏳ planned |
| Approval / security | `tools/{approval,path_security,url_safety,tirith_security,website_policy}.py` + `_ApprovalEntry`, `ScanResult` classes | 5.J | ⏳ planned |
| Code execution | `tools/{code_execution_tool,process_registry}.py` + `ProcessRegistry`, `ProcessSession`, `ExecuteResult`, `DebugSession`, `RunState` classes | 5.K | ⏳ planned |
| File operations | `tools/{file_operations,file_tools,fuzzy_match,checkpoint_manager,patch_parser,binary_extensions}.py` + `FileOperations`/`ShellFileOperations`/`PatchOperation`/`PatchResult`/`CheckpointManager`/`Hunk`/`HunkLine`/`SearchMatch`/`SearchResult`/`ReadResult`/`LintResult`/`Finding`/`OperationType`/`EnvironmentInfo` classes | 5.L | ⏳ planned |
| Mixture of agents | `tools/mixture_of_agents_tool.py` | 5.M | ⏳ planned |
| Operator tools | `tools/{todo_tool,clarify_tool,session_search_tool,send_message_tool,debug_helpers,interrupt,ansi_strip}.py` + `TodoStore`, `_ThreadAwareEventProxy` classes | 5.N | ⏳ planned |
| Auth storage (GitHub + Hermes token) | `tools/*` — `GitHubAuth`, `HermesTokenStorage` classes | 4.G / 5.O | ⏳ planned |
| Budget config + provider entries | `tools/budget_config.py` — `BudgetConfig`, `_ProviderEntry` classes | 4.H / 5.A | ⏳ planned |
| Tool entry metadata (registry row schema) | `tools/registry.py` — `ToolEntry` class | 5.A | ⏳ planned |
| Web tools / search (Parallel + Firecrawl providers) | `tools/web_tools.py` | 5.A | ⏳ planned |
| Terminal tool | `tools/terminal_tool.py` | 5.A | ⏳ planned |
| Send message (cross-platform) | `tools/send_message_tool.py` | 5.N | ⏳ planned |
| Feishu doc/drive tools | `tools/{feishu_doc_tool,feishu_drive_tool}.py` | 5.A | ⏳ planned |
| HomeAssistant tool | `tools/homeassistant_tool.py` | 5.A | ⏳ planned |
| OSV vulnerability check | `tools/osv_check.py` | 5.J | ⏳ planned |
| Budget config | `tools/budget_config.py` + `tool_backend_helpers.py` + `tool_result_storage.py` | 5.A | ⏳ planned |
| Env passthrough | `tools/env_passthrough.py` | 5.B | ⏳ planned |
| RL training tool | `tools/rl_training_tool.py` | 5.M | ⏳ deferred (specialized) |
| Datagen examples | `datagen-config-examples/` | 5.M | ⏳ deferred (specialized) |
| Batch runner | `batch_runner.py` | 5.O | ⏳ planned |
| Mini SWE runner | `mini_swe_runner.py` | 5.O | ⏳ planned (or 5.M) |
| Model tools (admin) | `model_tools.py` | 5.O | ⏳ planned |

### TUI + Interactive Surfaces (Phase 5.Q — new)

Upstream ships a dedicated `tui_gateway/` — a 3,094-line Python TUI server that streams live agent state over SSE to the Ink-based Node TUI. Gormes has its own Bubble Tea TUI (shipped Phase 1), but the gateway-side streaming surface is not yet ported.

| Subsystem | Upstream | Target phase | Status |
|---|---|---|---|
| TUI gateway server | `tui_gateway/server.py` (2,931 lines) + `render.py`, `slash_worker.py`, `entry.py` | 5.Q | ⏳ planned |
| TUI skin engine | `hermes_cli/skin_engine.py` | 5.Q | ⏳ planned |
| Default persona file | `hermes_cli/default_soul.py`, `docker/SOUL.md` | 5.Q / 4.B | ⏳ planned |

### CLI + packaging (Phase 5.O–5.P)

The upstream `hermes_cli/` has 49 Python files. Grouped by capability:

| Subsystem | Upstream | Target phase | Status |
|---|---|---|---|
| CLI entry + setup + uninstall | `hermes_cli/{main,setup,uninstall,env_loader,commands,callbacks,completion}.py` | 5.O | ⏳ planned |
| Auth commands (base) | `hermes_cli/{auth,auth_commands}.py` | 5.O | ⏳ planned |
| Provider-specific auth | `hermes_cli/{copilot_auth,dingtalk_auth}.py` + (`hermes_cli/nous_subscription.py` for Nous) | 5.O | ⏳ planned |
| Backup / dump / debug | `hermes_cli/{backup,dump,debug,logs,doctor,status}.py` | 5.O | ⏳ planned |
| Display / TUI | `hermes_cli/{banner,cli_output,clipboard,colors,curses_ui,tips}.py` | 5.O | ⏳ planned |
| Model selection + normalization | `hermes_cli/{model_switch,model_normalize,models,codex_models}.py` | 5.O | ⏳ planned |
| Providers + runtime routing | `hermes_cli/{providers,runtime_provider}.py` | 5.O | ⏳ planned |
| Profiles + config | `hermes_cli/{profiles,config}.py` | 5.O | ⏳ planned |
| Platforms + pairing + webhook | `hermes_cli/{platforms,pairing,webhook}.py` | 5.O | ⏳ planned |
| Gateway CLI + cron | `hermes_cli/{gateway,cron}.py` | 5.O | ⏳ planned |
| Plugins + skills CLI | `hermes_cli/{plugins,plugins_cmd,skills_config,skills_hub}.py` | 5.O | ⏳ planned |
| MCP + memory setup | `hermes_cli/{mcp_config,memory_setup}.py` | 5.O | ⏳ planned |
| Web server + TUI skin | `hermes_cli/{web_server,skin_engine,claw}.py` | 5.O / 5.Q | ⏳ planned |
| Tools config | `hermes_cli/tools_config.py` | 5.O | ⏳ planned |
| Dockerfile / packaging | `Dockerfile`, `docker/{entrypoint.sh,SOUL.md}`, `packaging/homebrew`, `nix/`, `flake.nix` | 5.P | ⏳ planned |
| Install scripts | `scripts/{install.sh,install.cmd,install.ps1,release.py,build_skills_index.py}` | 5.P | ⏳ planned |
| MANIFEST / constraints | `MANIFEST.in`, `constraints-termux.txt` | 5.P | ⏳ planned |
| Benchmarks | `environments/benchmarks/` | 5.M | ⏳ deferred (research) |
| SWE / terminal test envs | `environments/hermes_swe_env/`, `environments/terminal_test_env/` | 5.M | ⏳ deferred (research) |

### Out of scope for the runtime port

These upstream paths exist but are not part of the runtime that Gormes must absorb. Listed for completeness so future contributors don't mistake them for missing work:

- `agent/`, `cli.py`, `run_agent.py`, `gateway/`, `hermes/`, `hermes_cli/`, `tools/`, `cron/`, `acp_adapter/`, `acp_registry/`, `plugins/`, `tui_gateway/`, `environments/` — runtime paths covered by the phases above. Listed here so future contributors don't re-add them to "out of scope" by accident.
- `tests/` — Python tests are not ported; Gormes has its own Go test suite per spec.
- `docs/` (upstream documentation), `assets/`, `optional-skills/`, `skills/` — content corpus; mirrored separately by docs.gormes.ai (Phase 1.5) and skill packs. Skill-pack categories in `optional-skills/` (autonomous-ai-agents, blockchain, communication, creative, devops, email, health, mcp, migration, …) track Hermes' `skills/` categories but as opt-in packages.
- `ui-tui/`, `web/`, `website/` — Node.js/TypeScript frontends. Gormes has its own Go `cmd/gormes/tui` Bubble Tea UI (shipped Phase 1) and `www.gormes.ai/` Go-templated landing page. Upstream's React/TS frontends are not part of the Go runtime port.
- `tinker-atropos/` — upstream research sandbox (currently empty); no runtime content.
- `datagen-config-examples/` — RL/data-generation research examples; deferred to 5.M.
- `scripts/` (selectively) — `scripts/{install.sh,install.cmd,install.ps1,release.py,build_skills_index.py}` ARE ported in 5.P; `scripts/{contributor_audit.py,discord-voice-doctor.py,kill_modal.sh,lib/}` remain upstream-only contributor tooling.
- `plans/` (upstream plans directory), `package.json`, `package-lock.json`, `flake.lock`, `flake.nix` — build/packaging metadata; partially mirrored at Phase 5.P.
- `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `GOOD-PRACTICES.md`, `hermes-already-has-routines.md` — upstream contributor docs; not runtime.

### Runtime contracts (config schema, env var surface, on-disk state)

Equally important as the code inventory above is the **runtime contract surface** — the shape of configuration, environment, and on-disk state that a Hermes user has built muscle memory around. A Gormes binary that ignores these contracts is technically a port but operationally a foreign object.

#### Configuration files

| Path (upstream default) | Format | Upstream source | Gormes equivalent |
|---|---|---|---|
| `~/.hermes/config.yaml` | YAML | `hermes_cli/config.py` (`DEFAULT_CONFIG`, 117 top-level keys; `_config_version` for migrations) | `~/.config/gormes/config.toml` (TOML, simpler schema; `internal/config/config.go`) |
| `~/.hermes/.env` | `KEY=value` pairs | `hermes_cli/env_loader.py` (read on startup) | `GORMES_*` env vars + stdlib `os.Getenv` (no dotenv by default) |
| `~/.hermes/auth.json` | JSON | `hermes_cli/auth.py` + `agent/credential_pool.py` | Planned 4.G — token vault |
| `~/.hermes/.anthropic_oauth.json` | JSON | `hermes_cli/auth.py` | Planned 4.G — per-provider token files |
| `~/.hermes/context_length_cache.yaml` | YAML | `agent/model_metadata.py` | Planned 4.D — replace YAML with embedded `models_dev_cache.go` |
| `~/.hermes/models_dev_cache.json` | JSON | `agent/models_dev.py` | Planned 4.D |
| `~/.hermes/ollama_cloud_models_cache.json` | JSON | `agent/models_dev.py` / Ollama adapter | Planned 4.D |
| `~/.hermes/.skills_prompt_snapshot.json` | JSON | `agent/skill_commands.py` | Planned 5.F |

#### Environment variable surface

Upstream honors **~170 environment variables** across three layers. Gormes must re-expose the operator-facing ones without breaking muscle memory.

| Layer | Env var count | Representative examples | Target phase |
|---|---|---|---|
| **Hermes runtime toggles** (`HERMES_*`) | ~47 | `HERMES_HOME` (state root override), `HERMES_MAX_ITERATIONS`, `HERMES_QUIET`, `HERMES_HEADLESS`, `HERMES_MANAGED`, `HERMES_YOLO_MODE`, `HERMES_TIMEZONE`, `HERMES_REDACT_SECRETS`, `HERMES_TOOL_PROGRESS`, `HERMES_CA_BUNDLE`, `HERMES_INTERACTIVE`, `HERMES_DEV`, `HERMES_EPHEMERAL_SYSTEM_PROMPT`, `HERMES_PREFILL_MESSAGES_FILE`, `HERMES_OAUTH_TRACE`, `HERMES_RESTART_DRAIN_TIMEOUT`, `HERMES_SESSION_PLATFORM`, `HERMES_SESSION_SOURCE`, `HERMES_CODEX_BASE_URL`, `HERMES_GEMINI_CLIENT_ID`/`_SECRET`/`_PROJECT_ID`, `HERMES_QWEN_BASE_URL`, `HERMES_PORTAL_BASE_URL`, `HERMES_INFERENCE_PROVIDER`, `HERMES_ENABLE_PROJECT_PLUGINS`, `HERMES_COPILOT_ACP_COMMAND`/`_ARGS`, `HERMES_NOUS_MIN_KEY_TTL_SECONDS`, `HERMES_NOUS_TIMEOUT_SECONDS`, `HERMES_TUI`, `HERMES_TUI_DIR`, `HERMES_TUI_RESUME`, `HERMES_WEB_DIST`, `HERMES_NODE`, `HERMES_PYTHON`, `HERMES_CWD`, `HERMES_CONTAINER`, `HERMES_PLATFORM`, `HERMES_SKIP_CHMOD`, `HERMES_SKIP_NODE_BOOTSTRAP`, `HERMES_SPINNER_PAUSE`, `HERMES_TOOL_PROGRESS_MODE`, `HERMES_HOME_MODE`, `HERMES_PYTHON_SRC_ROOT` | 5.O (config port) |
| **Provider API keys + base URLs** | ~50 | `ANTHROPIC_API_KEY` / `ANTHROPIC_TOKEN`, `OPENAI_API_KEY` / `OPENAI_BASE_URL`, `GEMINI_API_KEY` / `GEMINI_BASE_URL`, `GOOGLE_API_KEY`, `DEEPSEEK_API_KEY` / `_BASE_URL`, `GLM_API_KEY` / `_BASE_URL`, `DASHSCOPE_API_KEY` / `_BASE_URL`, `ARCEEAI_API_KEY` / `ARCEE_BASE_URL`, `AWS_PROFILE` / `AWS_REGION` (Bedrock), `EXA_API_KEY`, `FIRECRAWL_API_KEY` / `_API_URL` / `_GATEWAY_URL` / `_BROWSER_TTL`, `BROWSERBASE_API_KEY` / `_PROJECT_ID`, `BROWSER_USE_API_KEY`, `CAMOFOX_URL`, `FAL_KEY`, `ELEVENLABS_API_KEY`, `GITHUB_TOKEN` | 4.A (per-adapter) |
| **Per-platform credentials** (listed in `_EXTRA_ENV_KEYS`) | ~70 | `DISCORD_BOT_TOKEN` / `DISCORD_ALLOWED_USERS` / `DISCORD_HOME_CHANNEL` / `DISCORD_REPLY_TO_MODE`, `TELEGRAM_HOME_CHANNEL`, `SIGNAL_ACCOUNT` / `_HTTP_URL` / `_ALLOWED_USERS` / `_GROUP_ALLOWED_USERS`, `DINGTALK_CLIENT_ID` / `_SECRET`, `FEISHU_APP_ID` / `_APP_SECRET` / `_ENCRYPT_KEY` / `_VERIFICATION_TOKEN`, `WECOM_BOT_ID` / `_SECRET` + 8 `WECOM_CALLBACK_*` keys, 14 `WEIXIN_*` keys, `BLUEBUBBLES_SERVER_URL` / `_PASSWORD` / `_ALLOW_ALL_USERS` / `_ALLOWED_USERS`, `QQ_APP_ID` / `_CLIENT_SECRET` / `QQBOT_HOME_CHANNEL` / `QQBOT_HOME_CHANNEL_NAME` + legacy `QQ_HOME_CHANNEL` aliases | 2.B.2+ (per platform) |
| **Gateway-level** | ~4 | `GATEWAY_ALLOW_ALL_USERS`, `GATEWAY_PROXY_URL`, `GATEWAY_PROXY_KEY`, plus `API_SERVER_{ENABLED,HOST,PORT,KEY,MODEL_NAME}` | 2.F |

#### On-disk state layout

Upstream uses `~/.hermes/` as the state root (overridable via `HERMES_HOME`). Gormes uses `${XDG_DATA_HOME}/gormes/` (default `~/.local/share/gormes/`) and `${XDG_CONFIG_HOME}/gormes/` (default `~/.config/gormes/`).

| Upstream path | Contents | Target phase | Gormes equivalent |
|---|---|---|---|
| `~/.hermes/state.db` | SessionDB (SQLite + FTS5 for session history) | 3.A (partial ✅), 3.E.8 | `~/.local/share/gormes/memory/memory.db` (turns + entities) + `~/.local/share/gormes/sessions.db` (bbolt) |
| `~/.hermes/sessions/` | Per-session exports + transcripts (JSONL) | 3.E.3 | Planned — Transcript Export Command |
| `~/.hermes/auth/` | Per-provider OAuth tokens | 4.G | Planned — token vault |
| `~/.hermes/memories/` | Per-backend memory plugin storage (8 backends) | 5.I | Planned — plugin directories |
| `~/.hermes/skills/` | Installed skills (26 upstream categories) | 5.F | Planned — `~/.local/share/gormes/skills/` |
| `~/.hermes/optional-skills/` | Optional skill packs (10+ categories) | 5.F | Planned |
| `~/.hermes/plugins/` | Plugin installs (context_engine, memory/*, example-dashboard) | 5.I | Planned |
| `~/.hermes/hooks/` | User hook scripts (per-event `HOOK.yaml` + scripts) | 2.F | Planned |
| `~/.hermes/cron/` | Cron job output Markdown files (one per job run) | 2.D | ✅ Shipped as Gormes equivalent: per-run audit in SQLite `cron_runs` table (not per-file) + aggregated `${XDG_DATA_HOME}/gormes/cron/CRON.md` mirror (3.D.5 pattern — atomic temp-file + rename; refreshed every 30s). Structured table is source of truth; Markdown is derived |
| `~/.hermes/logs/` | Agent run logs (per-session, rotated) | 2.F / 5.O | Planned — `${XDG_STATE_HOME}/gormes/logs/` |
| `~/.hermes/images/` | Generated images from image-generation tool | 5.D | Planned |
| `~/.hermes/pastes/` | Paste cache (large clipboard content spill-over) | 2.F | Planned |
| `~/.hermes/skins/` | CLI skin definition files | 5.Q | Planned |
| `~/.hermes/dashboard-themes/` | Example-dashboard plugin themes | 5.I | Planned |
| `~/.hermes/whatsapp/` | WhatsApp platform session state | 2.B.4 | Planned |
| `~/.hermes/channel_directory.json` | Cached channel/contact mappings | 2.F | Planned — existing `channel_directory.py` row |
| `~/.hermes/sticker_cache.json` | Telegram sticker lookup cache | 2.F | Planned |
| `~/.hermes/.container-mode` | Sentinel: "running inside container" | 2.F | Planned — Gormes can detect `/.dockerenv` or use its own sentinel |
| `~/.hermes/.managed` | Sentinel: "managed by external orchestrator" | 2.F | Planned |
| `~/.hermes/.update_exit_code` | Last update attempt's exit code | 5.O | Planned — auto-update subsystem |

#### Runtime contract implications for Gormes

1. **`HERMES_HOME` vs `XDG_DATA_HOME`**: Gormes MUST respect XDG by default, but should honor `HERMES_HOME` as a migration alias so operators switching over don't lose state.
2. **`.env` dotenv support**: Gormes currently expects env vars in the shell. Operators who have a working `~/.hermes/.env` will not want to re-key ~170 variables. Phase 5.O should add a dotenv loader that reads `~/.hermes/.env` and `~/.config/gormes/.env` at startup.
3. **Config migration**: Upstream `_config_version` key + migration helpers. Gormes must add a similar versioning scheme before the config schema stabilizes — otherwise TOML-key renames break users.
4. **`$EDITOR` for `hermes config edit`**: operator UX affordance; parity expected at 5.O.
5. **Platform-specific home channel pattern**: EVERY platform supports `<PLATFORM>_HOME_CHANNEL` + `<PLATFORM>_HOME_CHANNEL_NAME`. Gormes should generalize rather than re-implement per-platform.

### Inventory cadence

Re-run the upstream survey when a major Hermes release lands, when a new platform connector is added upstream, or when a Gormes phase ships and we need to mark its rows ✅. The survey is mechanical:

1. `find upstream root -name "*.py" -newer last-survey-date` for new Python files
2. `ls gateway/platforms/*.py` for new platform connectors
3. `ls plugins/memory/` for new memory backends (currently 8)
4. `ls tools/environments/*.py` for new sandbox backends (currently 10 including `ssh.py`)
5. `ls hermes_cli/*.py` for new CLI subcommands (currently 49)
6. `ls environments/tool_call_parsers/*.py` for new per-model parsers (currently 9)
7. `wc -l run_agent.py cli.py tui_gateway/server.py` to track orchestrator size growth
8. `grep -oE '^class ' agent/*.py tools/*.py gateway/*.py | sort -u | wc -l` — class count drift signals new subsystem surface (round-3 audit found 30 classes not previously mapped)
9. `grep -oE '"[A-Z_]{4,}":' hermes_cli/config.py | sort -u | wc -l` — current: 117 top-level config keys
10. `grep -oE 'HERMES_[A-Z_]+' hermes_cli/*.py agent/*.py | sort -u | wc -l` — current: ~47 `HERMES_*` env vars
11. `grep -oE 'get_hermes_home\(\) / "[a-z_./\-]+"' agent/*.py hermes_cli/*.py gateway/*.py | sort -u` — current: 28 known paths/files under `~/.hermes/` (round-4 audit)

The survey from 2026-04-20 caught **42 items** previously under-specified:

- **Round 1 (spec-level):** Phase 3.D semantic fusion ship criterion, Phase 3.E ledger (7 subphases).
- **Round 2 (file-level, 12 finds):** `run_agent.py` (12,113 lines), `cli.py` (10,570 lines), `tui_gateway/server.py` (2,931 lines), 9 per-model tool-call parsers, 8 third-party memory plugins, SSH sandbox, SkillSources, TUI skin engine, install scripts, `hermes_cli/` expansion from ~15 to 49 files.
- **Round 3 (class-level, 30 finds):** Slash command registry cross-cutting concern, tool registry orchestrator, toolset definitions, `HomeChannel` / `DeliveryRouter` / `GatewayStreamConsumer` / `SessionStore`, webhook subscription system, iteration budget, 3 new `AuxiliaryClient` classes (Anthropic + Codex, not just xAI), billing / cost / failover / metadata types, 7 `SkillSource` subclasses, `AudioRecorder` + `TermuxAudioRecorder`, 15+ file-operation classes, MCP OAuth / Sampling / FAL sync, `GitHubAuth` + `HermesTokenStorage`.
- **Round 4 (contract-level, this pass):** 117 config keys, ~170 env vars across 4 layers (HERMES_*, provider keys, platform credentials, gateway-level), 28 state-directory entries under `~/.hermes/`, config migration system (`_config_version`), XDG vs `HERMES_HOME` reconciliation, dotenv support gap, cron output filesystem mirror (`~/.hermes/cron/`).

Next survey: when upstream tags a new release, OR when any single round's find count exceeds 5 new subsystems.
