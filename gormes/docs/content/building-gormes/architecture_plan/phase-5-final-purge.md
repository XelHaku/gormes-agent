---
title: "Phase 5 — The Final Purge"
weight: 60
---

# Phase 5 — The Final Purge (100% Go)

**Status:** ⏳ planned

**Deliverable:** Python tool scripts ported to Go or WASM. Python disappears entirely from the runtime path.

Phase 5 is when Python disappears entirely from the runtime path. Each sub-phase is a separable spec.

## Phase 5 Sub-phase Outline

| Subphase | Status | Deliverable |
|---|---|---|
| 5.A — Tool Surface Port | ⏳ planned | Port the 61-tool `tools/` registry. Most tools are tractable Go ports; a few (browser, voice) split into 5.C–5.E. |
| 5.B — Sandboxing Backends | ⏳ planned | Port `tools/environments/{local,docker,modal,daytona,singularity}.py` + `file_sync.py`. Five execution backends with namespace isolation and container hardening. |
| 5.C — Browser Automation | ⏳ planned | Port `tools/browser_tool.py` + `tools/browser_camofox*.py` + `tools/browser_providers/{browserbase,browser_use,firecrawl}.py` to Go (Chromedp, Rod) or sidecar process |
| 5.D — Vision + Image Generation | ⏳ planned | Port `tools/vision_tools.py` + `tools/image_generation_tool.py`; multimodal in/out |
| 5.E — TTS / Voice / Transcription | ⏳ planned | Port `tools/{tts_tool,voice_mode,transcription_tools,neutts_synth}.py`; may stay as sidecar processes |
| 5.F — Skills System (Remaining) | ⏳ planned | Port remaining `tools/{skill_manager_tool,skills_hub,skills_sync,skills_guard}.py` + skill registries + 26 skill categories. **Note:** Core learning loop (skill extraction, improvement) is Phase 2.G (P0) |
| 5.F.1 — Skills Hub Integration | ⏳ planned | Port skills.sh, clawhub, lobehub, hermes-index registries |
| 5.F.2 — Skill Auto-Discovery | ⏳ planned | Port auto-generated skill discovery and metadata management |
| 5.G — MCP Integration | ⏳ planned | Port `tools/{mcp_tool,mcp_oauth,mcp_oauth_manager,managed_tool_gateway}.py`; Model Context Protocol client + OAuth flows |
| 5.H — ACP Integration | ⏳ planned | Port `acp_adapter/` + `acp_registry/`; Agent Communication Protocol server side |
| 5.I — Plugins Architecture | ⏳ planned | Port `plugins/{context_engine,memory,example-dashboard}` + the plugin SDK; let third parties extend Gormes without forking |
| 5.J — Approval / Security Guards | ⏳ planned | Port `tools/{approval,path_security,url_safety,tirith_security,website_policy}.py` as small guard slices: dangerous-command detection, approval-mode config, cron `approvals.cron_mode`, then combined Tirith/path/URL/website policy decisions |
| 5.K — Code Execution | ⏳ planned | Port `tools/code_execution_tool.py` + `tools/process_registry.py`; sandboxed exec |
| 5.L — File Ops + Patches | ⏳ planned | Port `tools/{file_operations,file_tools,checkpoint_manager,patch_parser}.py`; file editing with atomic checkpoints |
| 5.M — Mixture of Agents | ⏳ planned | Port `tools/mixture_of_agents_tool.py`; multi-model coordination |
| 5.N — Misc Operator Tools | ⏳ planned | Port `tools/{todo_tool,clarify_tool,session_search_tool,send_message_tool,cronjob_tools,debug_helpers,interrupt}.py`; cron parity is now split into tool API/schedule parsing, prompt/script safety, and multi-target/media/live-adapter delivery follow-ups |
| 5.O — Hermes CLI Parity | ⏳ planned | Port the 49-file `hermes_cli/` tree as dependency-aware command groups: registry/active-turn policy, config/profile/auth/setup, gateway/platform/webhook/cron management, diagnostics/backup/logs/status; replaces the upstream `hermes` binary |
| 5.P — Docker / Packaging | ⏳ planned | Mirror `Dockerfile` + `docker/{entrypoint.sh,SOUL.md}` + `packaging/homebrew` + `scripts/{install.sh,install.cmd,install.ps1,release.py,build_skills_index.py}` for Gormes; OCI image with same volume layout as upstream |
| 5.Q — API Server + TUI Gateway Streaming Surface | ⏳ planned | Port `tui_gateway/` plus the upstream OpenAI-compatible `gateway/platforms/api_server.py` surface: Bubble Tea remote streaming, `/v1/chat/completions`, `/v1/responses`, `/v1/runs/{id}/events`, `/health/detailed`, and cron admin endpoints over the native Go runtime |

## Relationship to Phase 6

Phase 5.F (Skills System port) is the mechanical work: porting the upstream Python skills plumbing. Phase 6 (The Learning Loop) is the algorithm on top — detecting complexity, distilling patterns, scoring feedback. Phase 5.F is a dependency of Phase 6, but they are not the same work.

## Planning Notes

Phase 2.D shipped the Go-native cron MVP: scheduler, job store, run audit, CRON.md mirror, and Heartbeat delivery rules. It did not port the full upstream cron operator surface. The remaining `cron/jobs.py`, `cron/scheduler.py`, and `tools/cronjob_tools.py` parity work belongs in 5.N as TDD slices so it can reuse the Phase 2.D store without reopening the shipped cron audit contract.

The OpenAI-compatible API server is not the Phase 1 bridge. Phase 1 consumes Python's `api_server`; Phase 5.Q replaces that donor surface in Go. Keep its cron admin endpoints behind the 5.N cronjob parity slices so HTTP control, CLI control, and tool control all share one scheduler/store contract.
