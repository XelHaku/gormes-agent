---
title: "Phase 5 — The Final Purge"
weight: 60
---

# Phase 5 — The Final Purge (100% Go)

**Status:** 🔨 in progress

**Deliverable:** Python tool scripts ported to Go or WASM. Python disappears entirely from the runtime path.

Phase 5 is when Python disappears entirely from the runtime path. Each sub-phase is a separable spec.

## Phase 5 Sub-phase Outline

| Subphase | Status | Deliverable |
|---|---|---|
| 5.A — Tool Surface Port | ✅ complete | `internal/tools` now ships the Go-native registry row (`ToolEntry`), toolset-scoped descriptor filtering, env/check-fn availability gates, and default runtime wiring for the built-in + delegation toolsets. |
| 5.B — Sandboxing Backends | ✅ complete | `internal/tools/environments/{daytona,docker,modal,singularity}` now cover the tracked 5.B backend seams with tested resource normalization, snapshot restore fallback, exec translation, lifecycle cleanup, Docker hardening/persistence mounts, and Singularity scratch/overlay management. SSH and shared file-sync remain future follow-on ports outside the current checklist. |
| 5.C — Browser Automation | ✅ complete | `internal/tools/browser_{chromedp,rod}.go` now cover the tracked Go-native `browser_navigate` scope with shared `driver=chromedp|rod` selection, local Chrome/Chromium launch, remote `BROWSER_CDP_URL` attach support, and deterministic metadata/HTML capture. Cloud-provider/browserbase/camofox parity remains future follow-on scope outside the current checklist. |
| 5.D — Vision + Image Generation | ✅ complete (tracked scope) | `internal/hermes` now translates canonical text+image message parts across the default HTTP client plus OpenRouter, Codex Responses, Anthropic, Gemini/Google Code Assist, and Bedrock request builders. Dedicated `tools/vision_tools.py` + `tools/image_generation_tool.py` ports remain future follow-on scope outside this checklist item. |
| 5.E — TTS / Voice / Transcription | ✅ complete (tracked scope) | `internal/gateway/voice_mode.go` now ports the shared `/voice` control plane with persisted per-chat `off/voice_only/all` state at `${XDG_DATA_HOME}/gormes/gateway_voice_mode.json`; STT/TTS/transcription engines remain explicit follow-on sidecar scope |
| 5.F — Skills System (Remaining) | 🔨 in progress | `internal/skills` now ships the typed skill-registry layer (`SkillMeta`, `SkillBundle`, `SkillReadinessStatus`, `HubLockFile`), deterministic filesystem/static source discovery, and the canonical bundled/official/Claude Marketplace/ClawHub/GitHub/Hermes Index/LobeHub/skills.sh registry catalog. Remaining work is the port of `tools/{skill_manager_tool,skills_hub,skills_sync,skills_guard}.py` plus the hub/install UX and 26 skill categories. **Note:** Core learning loop (skill extraction, improvement) is Phase 2.G (P0) |
| 5.F.1 — Skills Hub Integration | ⏳ planned | Consume the shipped registry catalog through the skills hub/install UX and sync skills.sh, clawhub, lobehub, hermes-index catalogs into the local skill stores |
| 5.F.2 — Skill Auto-Discovery | ⏳ planned | Port auto-generated skill discovery and metadata management |
| 5.G — MCP Integration | 🔨 in progress | `internal/mcp/client.go` now ships the Go-native stdio MCP client core (`initialize` + `notifications/initialized`, negotiated protocol validation, `tools/list`, `tools/call`). OAuth, HTTP transport, dynamic toolset injection, and sampling remain follow-on scope. |
| 5.H — ACP Integration | ⏳ planned | Port `acp_adapter/` + `acp_registry/`; Agent Communication Protocol server side |
| 5.I — Plugins Architecture | ⏳ planned | Port `plugins/{context_engine,memory,example-dashboard}` + the plugin SDK; let third parties extend Gormes without forking |
| 5.J — Approval / Security Guards | ⏳ planned | Port `tools/{approval,path_security,url_safety,tirith_security,website_policy}.py`; gate dangerous actions |
| 5.K — Code Execution | ⏳ planned | Port `tools/code_execution_tool.py` + `tools/process_registry.py`; sandboxed exec |
| 5.L — File Ops + Patches | ⏳ planned | Port `tools/{file_operations,file_tools,checkpoint_manager,patch_parser}.py`; file editing with atomic checkpoints |
| 5.M — Mixture of Agents | ⏳ planned | Port `tools/mixture_of_agents_tool.py`; multi-model coordination |
| 5.N — Misc Operator Tools | ⏳ planned | Port `tools/{todo_tool,clarify_tool,session_search_tool,send_message_tool,cronjob_tools,debug_helpers,interrupt}.py` |
| 5.O — Hermes CLI Parity | ⏳ planned | Port the 49-file `hermes_cli/` tree (auth, backup, banner, codex_models, profiles, platforms, pairing, webhook, models, providers, runtime_provider, skills_hub, mcp_config, plugins, …); replaces the upstream `hermes` binary |
| 5.P — Docker / Packaging | ⏳ planned | Mirror `Dockerfile` + `docker/{entrypoint.sh,SOUL.md}` + `packaging/homebrew` + `scripts/{install.sh,install.cmd,install.ps1,release.py,build_skills_index.py}` for Gormes; OCI image with same volume layout as upstream |
| 5.Q — TUI Gateway Streaming Surface | ⏳ planned | Port `tui_gateway/` (2,931-line `server.py` + `render.py`, `slash_worker.py`, `entry.py`) + `hermes_cli/skin_engine.py`; SSE streaming to the existing Bubble Tea TUI (shipped Phase 1) so remote TUIs see live agent state |

## Relationship to Phase 6

Phase 5.F (Skills System port) is the mechanical work: porting the upstream Python skills plumbing. Phase 6 (The Learning Loop) is the algorithm on top — detecting complexity, distilling patterns, scoring feedback. Phase 5.F is a dependency of Phase 6, but they are not the same work.
