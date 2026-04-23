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
| 5.C — Browser Automation | 🔨 in progress | `internal/tools/browser_chromedp.go` now ships the first Go-native Chromedp slice via `browser_navigate` with local Chrome/Chromium launch, `BROWSER_CDP_URL` CDP attach support, and deterministic metadata/HTML capture. Rod plus cloud-provider/browserbase/camofox parity remain pending. |
| 5.D — Vision + Image Generation | ⏳ planned | Port `tools/vision_tools.py` + `tools/image_generation_tool.py`; multimodal in/out |
| 5.E — TTS / Voice / Transcription | ⏳ planned | Port `tools/{tts_tool,voice_mode,transcription_tools,neutts_synth}.py`; may stay as sidecar processes |
| 5.F — Skills System (Remaining) | ⏳ planned | Port remaining `tools/{skill_manager_tool,skills_hub,skills_sync,skills_guard}.py` + skill registries + 26 skill categories. **Note:** Core learning loop (skill extraction, improvement) is Phase 2.G (P0) |
| 5.F.1 — Skills Hub Integration | ⏳ planned | Port skills.sh, clawhub, lobehub, hermes-index registries |
| 5.F.2 — Skill Auto-Discovery | ⏳ planned | Port auto-generated skill discovery and metadata management |
| 5.G — MCP Integration | ⏳ planned | Port `tools/{mcp_tool,mcp_oauth,mcp_oauth_manager,managed_tool_gateway}.py`; Model Context Protocol client + OAuth flows |
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
