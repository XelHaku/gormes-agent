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
| 5.A — Tool Surface Port | ⏳ planned | Keep the 61-tool registry as inventory only; execute through schema parity, pure/core tools, stateful tool groups, then network/browser/sandbox-dependent handlers |
| 5.B — Sandboxing Backends | ⏳ planned | Port the environment interface and file-sync contract first, then Docker, Modal, Daytona, and Singularity as backend-specific implementations |
| 5.C — Browser Automation | ⏳ planned | Freeze the browser action/result transcript, then bind Chromedp/Rod and optional Browserbase/Firecrawl provider fallbacks behind the shared contract |
| 5.D — Vision + Image Generation | ⏳ planned | Split multimodal work into vision input normalization/token budgeting and image-generation result/storage contracts |
| 5.E — TTS / Voice / Transcription | ⏳ planned | Split voice work into voice-mode state, transcription ingestion, TTS synthesis, and gateway media-delivery hooks |
| 5.F — Skills System (Remaining) | ⏳ planned | Port remaining `tools/{skill_manager_tool,skills_hub,skills_sync,skills_guard}.py` + registry metadata on top of the Phase 2.G active/inactive store. **Note:** Core learning loop (skill extraction, improvement) is Phase 6 |
| 5.F.1 — Skills Hub Integration | ⏳ planned | Port skills.sh, clawhub, lobehub, hermes-index registries |
| 5.F.2 — Skill Auto-Discovery | ⏳ planned | Port auto-generated skill discovery and metadata management |
| 5.G — MCP Integration | ⏳ planned | Port the MCP client, OAuth state store, then managed tool gateway bridge as separate fixtures over fake MCP servers |
| 5.H — ACP Integration | ⏳ planned | Port `acp_adapter/` as an authenticated protocol server with session lifecycle, tool permission prompts, and event streaming tests |
| 5.I — Plugins Architecture | ⏳ planned | Port plugin manifest loading, capability registration, version checks, isolation boundaries, then extension install/enable/disable flows |
| 5.J — Approval / Security Guards | ⏳ planned | Port `tools/{approval,path_security,url_safety,tirith_security,website_policy}.py` as small guard slices: dangerous-command detection, approval-mode config, cron `approvals.cron_mode`, then combined Tirith/path/URL/website policy decisions |
| 5.K — Code Execution | ✅ complete | Go-native `execute_code` now runs guarded `sh`/`python` snippets with runtime selection, timeout/output caps, and filesystem/network blocking; broader backend-specific sandboxes remain Phase 5.B |
| 5.L — File Ops + Patches | ⏳ planned | Port `tools/{file_operations,file_tools,checkpoint_manager,patch_parser}.py` as checkpoint-first write primitives with rollback and path-policy fixtures |
| 5.M — Mixture of Agents | ⏳ planned | Port `tools/mixture_of_agents_tool.py` only after provider routing and subagent envelopes are stable |
| 5.N — Misc Operator Tools | ⏳ planned | Port todo, clarify, session search, debug helpers, send_message, cronjob tools, and interrupt as small contracts; cron parity remains split into tool API/schedule parsing, prompt/script safety, and multi-target/media/live-adapter delivery follow-ups |
| 5.O — Hermes CLI Parity | ⏳ planned | Port the 49-file `hermes_cli/` tree as dependency-aware command groups: registry/active-turn policy, config/profile/auth/setup, gateway/platform/webhook/cron management, diagnostics/backup/logs/status; replaces the upstream `hermes` binary |
| 5.P — Docker / Packaging | ⏳ planned | Mirror upstream Docker/Homebrew/release layout with `gormes doctor --offline` smoke checks and no Python runtime dependency in the final image |
| 5.Q — API Server + TUI Gateway Streaming Surface | ⏳ planned | Port `tui_gateway/` plus the upstream OpenAI-compatible `gateway/platforms/api_server.py` surface: Bubble Tea remote streaming, `/v1/chat/completions`, `/v1/responses`, `/v1/runs/{id}/events`, `/health/detailed`, and cron admin endpoints over the native Go runtime |

## Relationship to Phase 6

Phase 5.F (Skills System port) is the mechanical work: porting the upstream Python skills plumbing. Phase 6 (The Learning Loop) is the algorithm on top — detecting complexity, distilling patterns, scoring feedback. Phase 5.F is a dependency of Phase 6, but they are not the same work.

## Planning Notes

Phase 2.D shipped the Go-native cron MVP: scheduler, job store, run audit, CRON.md mirror, and Heartbeat delivery rules. It did not port the full upstream cron operator surface. The remaining `cron/jobs.py`, `cron/scheduler.py`, and `tools/cronjob_tools.py` parity work belongs in 5.N as TDD slices so it can reuse the Phase 2.D store without reopening the shipped cron audit contract.

The OpenAI-compatible API server is not the Phase 1 bridge. Phase 1 consumes Python's `api_server`; Phase 5.Q replaces that donor surface in Go. Keep its cron admin endpoints behind the 5.N cronjob parity slices so HTTP control, CLI control, and tool control all share one scheduler/store contract.

## TDD Execution Notes

Phase 5 is the highest-risk place to accidentally create giant porting tasks. Treat every broad row above as an inventory bucket, not a direct worker assignment.

1. **Start with schema parity, not handlers.** The first 5.A slice should snapshot upstream tool names, toolsets, required env vars, and JSON result shapes so later ports can prove compatibility without reading the Python tree repeatedly.
2. **Land shared contracts before heavy backends.** The environment/file-sync interface must precede Docker/Modal/Daytona/Singularity. The browser action transcript must precede Chromedp/Rod/provider bindings. The checkpoint/path-policy contract must precede write-capable file tools.
3. **Keep external-service work fixture-first.** Browserbase, Firecrawl, MCP OAuth, ACP, cloud sandboxes, image models, TTS, and transcription should all land with fake clients before any live credential smoke test.
4. **Reuse existing Gormes substrates.** Cron tool/API parity should reuse Phase 2.D stores. Session search should reuse Phase 3 catalog and GONCHO scoping. Skills hub work should reuse the Phase 2.G active/inactive store. CLI parity should consume the same gateway/pairing/status read models as the runtime.
