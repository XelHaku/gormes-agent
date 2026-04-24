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
| 5.F — Skills System (Remaining) | ⏳ planned | Port remaining `tools/{skill_manager_tool,skills_hub,skills_sync,skills_guard}.py` + registry metadata on top of the Phase 2.G active/inactive store, plus the new upstream `agent/skill_preprocessing.py` and skill-backed slash command preprocessing contract. **Note:** Core learning loop (skill extraction, improvement) is Phase 6 |
| 5.F.1 — Skills Hub Integration | ⏳ planned | Port skills.sh, clawhub, lobehub, hermes-index registries |
| 5.F.2 — Skill Auto-Discovery | ⏳ planned | Port auto-generated skill discovery and metadata management |
| 5.G — MCP Integration | ⏳ planned | Port the MCP client, OAuth state store, then managed tool gateway bridge as separate fixtures over fake MCP servers |
| 5.H — ACP Integration | ⏳ planned | Port `acp_adapter/` as an authenticated protocol server with session lifecycle, tool permission prompts, and event streaming tests |
| 5.I — Plugins Architecture | ⏳ planned | Port plugin manifest loading, capability registration, version checks, isolation boundaries, then extension install/enable/disable flows; upstream Spotify has moved into `plugins/spotify/`, so it is now the first first-party plugin fixture rather than a built-in tool port |
| 5.J — Approval / Security Guards | ⏳ planned | Port `tools/{approval,path_security,url_safety,tirith_security,website_policy}.py` as small guard slices: dangerous-command detection, approval-mode config, cron `approvals.cron_mode`, then combined Tirith/path/URL/website policy decisions |
| 5.K — Code Execution | ✅ complete | Go-native `execute_code` now runs guarded `sh`/`python` snippets with runtime selection, timeout/output caps, and filesystem/network blocking; broader backend-specific sandboxes remain Phase 5.B |
| 5.L — File Ops + Patches | ⏳ planned | Port `tools/{file_operations,file_tools,checkpoint_manager,patch_parser}.py` as checkpoint-first write primitives with rollback and path-policy fixtures |
| 5.M — Mixture of Agents | ⏳ planned | Port `tools/mixture_of_agents_tool.py` only after provider routing and subagent envelopes are stable |
| 5.N — Misc Operator Tools | ⏳ planned | Port todo, clarify, session search, debug helpers, send_message, cronjob tools, and interrupt as small contracts; cron parity remains split into tool API/schedule parsing, prompt/script safety, and multi-target/media/live-adapter delivery follow-ups |
| 5.O — Hermes CLI Parity | ⏳ planned | Port the 49-file `hermes_cli/` tree as dependency-aware command groups: deterministic helpers, PTY bridge adapter, registry/active-turn/busy policy, config/profile/auth/setup, gateway/platform/webhook/cron management, diagnostics/backup/logs/status; replaces the upstream `hermes` binary |
| 5.P — Docker / Packaging | ⏳ planned | Mirror upstream Docker/Homebrew/release layout with `gormes doctor --offline` smoke checks and no Python runtime dependency in the final image; also port upstream `install.sh`/`install.cmd`/`install.ps1` as Unix + Windows installer parity surfaces served from `www.gormes.ai/internal/site/` (plan: `docs/superpowers/plans/2026-04-23-gormes-installer-parity.md`) |
| 5.Q — API Server + TUI Gateway Streaming Surface | ⏳ planned | Port `tui_gateway/` plus the upstream OpenAI-compatible `gateway/platforms/api_server.py` surface: Bubble Tea remote streaming, `/v1/chat/completions`, `/v1/responses`, `/v1/runs/{id}/events`, incomplete snapshot persistence on disconnect/cancel, gateway proxy mode, dashboard-facing API contracts, `/health/detailed`, and cron admin endpoints over the native Go runtime |
| 5.R — Code Execution Mode Policy | ⏳ planned | Split upstream PR #11971 into four dependency-ordered TDD slices — mode resolver + config precedence, strict-mode CWD/interpreter parity, project-mode CWD + active venv detection, then default-mode selection + config cut-over — without widening the shipped 5.K filesystem/network blocking contract |

## Relationship to Phase 6

Phase 5.F (Skills System port) is the mechanical work: porting the upstream Python skills plumbing. Phase 6 (The Learning Loop) is the algorithm on top — detecting complexity, distilling patterns, scoring feedback. Phase 5.F is a dependency of Phase 6, but they are not the same work.

## Planning Notes

Phase 2.D shipped the Go-native cron MVP: scheduler, job store, run audit, CRON.md mirror, and Heartbeat delivery rules. It did not port the full upstream cron operator surface. The remaining `cron/jobs.py`, `cron/scheduler.py`, and `tools/cronjob_tools.py` parity work belongs in 5.N as TDD slices so it can reuse the Phase 2.D store without reopening the shipped cron audit contract.

The OpenAI-compatible API server is not the Phase 1 bridge. Phase 1 consumes Python's `api_server`; Phase 5.Q replaces that donor surface in Go. The latest upstream API server adds stored Responses snapshots for disconnect/cancel paths and gateway proxy mode; keep those as separate slices after the base chat/Responses contracts so client-resilience work does not hide basic HTTP parity failures. Keep cron admin endpoints behind the 5.N cronjob parity slices so HTTP control, CLI control, and tool control all share one scheduler/store contract.

## Operation And Tool Descriptor Rule

The GBrain study strengthens the Phase 5 rule: do not start by porting
handlers. Start by freezing operation/tool descriptors.

Each ported operation should declare:

- name and description;
- JSON schema and result envelope;
- toolset/category;
- availability check;
- mutating or read-only;
- idempotent or not;
- prompt-visible or operator-only;
- allowed trust classes;
- timeout and result-size budget;
- audit event kind;
- degraded-mode status field.

Those descriptors should drive model tool schemas, CLI/gateway command
surfaces, doctor checks, audit taxonomy, and fixture generation wherever
possible. A Python donor file remains inventory until the descriptor and parity
fixture exist.

Hermes adds the schema-repair lesson: dynamic schemas must reflect available
tools. If a related tool or provider is disabled, the prompt-visible schema must
not advertise an impossible path that causes hallucinated tool calls.

## TDD Execution Notes

Phase 5 is the highest-risk place to accidentally create giant porting tasks. Treat every broad row above as an inventory bucket, not a direct worker assignment.

1. **Start with schema parity, not handlers.** The first 5.A slice should snapshot upstream tool names, toolsets, required env vars, and JSON result shapes so later ports can prove compatibility without reading the Python tree repeatedly.
2. **Land shared contracts before heavy backends.** The environment/file-sync interface must precede Docker/Modal/Daytona/Singularity. The browser action transcript must precede Chromedp/Rod/provider bindings. The checkpoint/path-policy contract must precede write-capable file tools.
3. **Keep external-service work fixture-first.** Browserbase, Firecrawl, MCP OAuth, ACP, cloud sandboxes, image models, TTS, and transcription should all land with fake clients before any live credential smoke test.
4. **Reuse existing Gormes substrates.** Cron tool/API parity should reuse Phase 2.D stores. Session search should reuse Phase 3 catalog and GONCHO scoping. Skills hub and preprocessing work should reuse the Phase 2.G active/inactive store. CLI parity should consume the same gateway/pairing/status read models as the runtime.
5. **5.R mode policy is strictly additive.** The shipped 5.K sandboxed-exec envelope (`status`, `error`, `filesystem_access`, `network_access`, stdout/stderr caps, timeout) is a public contract. 5.R slices must not widen or renegotiate it — they only choose CWD and interpreter per mode. Land the pure resolver first, then strict parity, then project mode, then the default cut-over; the order matters because both later slices depend on the frozen resolver and the pinned strict baseline.
6. **5.P installer parity is site-asset-bounded.** The Unix/Windows installer work belongs under `www.gormes.ai/internal/site/`, not `scripts/`, because it is served from the landing-page module and exported statically. The Go `install_unix_test.go`/`install_windows_test.go` tests must drive the scripts under fake toolchains — no network calls, no root-owned paths, no Python/Node side-installs — so Gormes stays a single-binary deployment target even as the installer surface widens.
7. **5.Q dashboard drift is API-contract inventory.** The upstream React dashboard is not a reason to add Node/TypeScript to the Gormes runtime. Track its chat, session, model picker, OAuth, tool-progress, PTY, and plugin-panel expectations as API contracts first; a Go-native dashboard UI can be decided only after the native endpoints are stable.
