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
| 5.A — Tool Surface Port | ⏳ planned | Keep the 61-tool registry as inventory only; execute through schema parity, then refresh the Hermes `b35d692f` manifest for Discord split/cron schema/toolset drift before pure/core tools, stateful groups, and network/browser/sandbox-dependent handlers |
| 5.B — Sandboxing Backends | ⏳ planned | Port the environment interface and file-sync contract first, then Docker, Modal, Daytona, and Singularity as backend-specific implementations |
| 5.C — Browser Automation | ⏳ planned | Freeze the browser action/result transcript, then bind Chromedp/Rod and optional Browserbase/Firecrawl provider fallbacks behind the shared contract |
| 5.D — Vision + Image Generation | ⏳ planned | Split multimodal work into vision input normalization/token budgeting and image-generation result/storage contracts |
| 5.E — TTS / Voice / Transcription | ⏳ planned | Split voice work into voice-mode state, transcription ingestion, TTS synthesis, and gateway media-delivery hooks |
| 5.F — Skills System (Remaining) | ⏳ planned | Port remaining `tools/{skill_manager_tool,skills_hub,skills_sync,skills_guard}.py` + registry metadata on top of the Phase 2.G active/inactive store, plus the new upstream `agent/skill_preprocessing.py` and skill-backed slash command preprocessing contract. **Note:** Core learning loop (skill extraction, improvement) is Phase 6 |
| 5.F.1 — Skills Hub Integration | ⏳ planned | Port skills.sh, clawhub, lobehub, hermes-index registries |
| 5.F.2 — Skill Auto-Discovery | ⏳ planned | Port auto-generated skill discovery and metadata management |
| 5.G — MCP Integration | ⏳ planned | Port MCP as dependency-ordered slices: config/env resolver first, fake stdio/HTTP discovery and schema normalization second, OAuth state/noninteractive auth errors third, then managed tool gateway bridge. Honcho MCP `HONCHO_API_URL` self-hosted drift is handled as server-local MCP config, not a hosted dependency for internal Goncho memory |
| 5.H — ACP Integration | ⏳ planned | Port `acp_adapter/` as an authenticated protocol server with session lifecycle, tool permission prompts, and event streaming tests |
| 5.I — Plugins Architecture | ⏳ planned | Port plugin manifest loading, capability registration, version checks, isolation boundaries, then extension install/enable/disable flows; upstream Spotify has moved into `plugins/spotify/`, so it is now the first first-party plugin fixture rather than a built-in tool port |
| 5.J — Approval / Security Guards | ⏳ planned | Port `tools/{approval,path_security,url_safety,tirith_security,website_policy}.py` as small guard slices: dangerous-command detection, approval-mode config, cron `approvals.cron_mode`, then combined Tirith/path/URL/website policy decisions |
| 5.K — Code Execution | ✅ complete | Go-native `execute_code` now runs guarded `sh`/`python` snippets with runtime selection, timeout/output caps, and filesystem/network blocking; broader backend-specific sandboxes remain Phase 5.B |
| 5.L — File Ops + Patches | ⏳ planned | Port `tools/{file_operations,file_tools,checkpoint_manager,patch_parser}.py` as checkpoint-first write primitives with rollback and path-policy fixtures |
| 5.M — Mixture of Agents | ⏳ planned | Port `tools/mixture_of_agents_tool.py` only after provider routing and subagent envelopes are stable |
| 5.N — Misc Operator Tools | ⏳ planned | Port todo, clarify, session search, debug helpers, send_message, cronjob tools, and interrupt as small contracts; cron parity remains split into tool API/schedule parsing, `context_from` output chaining, prompt/script safety, and multi-target/media/live-adapter delivery follow-ups |
| 5.O — Hermes CLI Parity | ⏳ planned | Port the 49-file `hermes_cli/` tree as dependency-aware command groups: deterministic helpers, PTY bridge adapter, registry/active-turn/busy policy, profile path/active-profile helpers before config/auth/setup UI, top-level `-z/--oneshot` with model/provider resolution, platform toolset persistence/MCP sentinel behavior, RestartSec-aware service restart polling, gateway management read-model closeout before mutating commands, local log snapshot diagnostics before backup/upload behavior, and later webhook/cron/platform/status command groups; replaces the upstream `hermes` binary |
| 5.P — Docker / Packaging | ⏳ planned | Mirror upstream Docker/Homebrew/release layout with `gormes doctor --offline` smoke checks and no Python runtime dependency in the final image; port upstream `install.sh`/`install.cmd`/`install.ps1` as Unix + Windows installer parity surfaces served from `www.gormes.ai/internal/site/`, and freeze the root/FHS vs user-scoped Unix layout policy from Hermes `b35d692f` |
| 5.Q — API Server + TUI Gateway Streaming Surface | ⏳ planned | Port `tui_gateway/` plus the upstream OpenAI-compatible `gateway/platforms/api_server.py` surface: Bubble Tea remote streaming, native TUI/no-Node bundle independence after Hermes `ee0728c6`, TUI startup model/provider override and static alias behavior from `283c8fd6`, native selection/copy parity-or-divergence after `edc78e25`, `/v1/chat/completions`, `/v1/responses`, `/v1/runs/{id}/events`, incomplete snapshot persistence on disconnect/cancel, gateway proxy mode, dashboard-facing API contracts, `/health/detailed`, and cron admin endpoints over the native Go runtime |
| 5.R — Code Execution Mode Policy | ⏳ planned | Split upstream PR #11971 into four dependency-ordered TDD slices — mode resolver + config precedence, strict-mode CWD/interpreter parity, project-mode CWD + active venv detection, then default-mode selection + config cut-over — without widening the shipped 5.K filesystem/network blocking contract |

## Relationship to Phase 6

Phase 5.F (Skills System port) is the mechanical work: porting the upstream Python skills plumbing. Phase 6 (The Learning Loop) is the algorithm on top — detecting complexity, distilling patterns, scoring feedback. Phase 5.F is a dependency of Phase 6, but they are not the same work.

## Planning Notes

Phase 2.D shipped the Go-native cron MVP: scheduler, job store, run audit, CRON.md mirror, and Heartbeat delivery rules. It did not port the full upstream cron operator surface. The remaining `cron/jobs.py`, `cron/scheduler.py`, and `tools/cronjob_tools.py` parity work belongs in 5.N as TDD slices so it can reuse the Phase 2.D store without reopening the shipped cron audit contract. Hermes `b35d692f` added `context_from` chaining; Gormes should map that to the native run audit/output read model rather than copying Hermes' Markdown output directory as source of truth.

Hermes `b35d692f` also reshaped the operator tool surface: Discord is now split into `discord` and `discord_admin` toolsets with platform restrictions, cronjob schemas include `context_from`, Feishu gained document/drive toolset coverage, and `hermes tools` persistence hardens MCP server names, numeric YAML keys, `no_mcp`, and restricted toolsets. The 5.A manifest-refresh row must land before handler or CLI rows use the old embedded manifest, otherwise workers will keep targeting the stale `discord_server` shape.

Hermes `5006b220` adds two CLI/service deltas that Gormes should freeze as small rows before broad CLI ports: top-level one-shot mode (`hermes -z`) with explicit model/provider/env resolution, and update restart polling that waits through systemd `RestartSec` before declaring the gateway failed to relaunch. The parser half of one-shot mode is already validated; keep final-output capture, noninteractive safety policy, RestartSec parsing, and active-status polling as separate rows so workers do not mix output plumbing with service-manager state machines.

The OpenAI-compatible API server is not the Phase 1 bridge. Phase 1 consumes Python's `api_server`; Phase 5.Q replaces that donor surface in Go. The latest upstream API server adds stored Responses snapshots for disconnect/cancel paths and gateway proxy mode; keep those as separate slices after the base chat/Responses contracts so client-resilience work does not hide basic HTTP parity failures. Keep cron admin endpoints behind the 5.N cronjob parity slices so HTTP control, CLI control, and tool control all share one scheduler/store contract. Hermes `ee0728c6` fixed a first-launch TUI rebuild bug by treating a missing `packages/hermes-ink/dist/ink-bundle.js` as stale; Gormes should not port that Node/Ink machinery, but it should fixture-lock that native TUI startup, doctor/status output, docs, and landing-page install copy never require npm, `HERMES_TUI_DIR`, or Hermes bundle files. Hermes `283c8fd6` moved model/provider override state into TUI launch and resolved short aliases without startup network lookup; Gormes should adapt that into the native Cobra/Bubble Tea path instead of leaving overrides oneshot-only. Hermes `edc78e25`/`31d7f195` tightened custom Ink selection copy over SSH, indentation, rendered spaces, and bounds; Gormes' next slice should explicitly document and fixture-lock terminal-native Bubble Tea selection with no advertised custom copy hotkey. A later Go-native copy mode can be planned separately if the product decision changes.

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
6. **5.P installer parity is site-asset-bounded.** The Unix/Windows installer work belongs under `www.gormes.ai/internal/site/` with a canonical script copy kept in sync, because the landing-page module serves and exports the public assets. The Go `install_unix_test.go`/`install_windows_test.go` tests must drive the scripts under fake toolchains — no network calls, no root-owned writes, no Python/Node side-installs — so Gormes stays a single-binary deployment target even as the installer surface widens. Hermes `b35d692f` changed root Linux installs toward an FHS layout, so the new row must either port that behavior or document and test an intentional user-scoped Gormes divergence before public install copy changes.
7. **5.Q dashboard drift is API-contract inventory.** The upstream React dashboard is not a reason to add Node/TypeScript to the Gormes runtime. Track its chat, session, model picker, OAuth, tool-progress, PTY, and plugin-panel expectations as API contracts first; a Go-native dashboard UI can be decided only after the native endpoints are stable.
8. **5.O CLI buckets stay split by side effect.** Top-level `-z/--oneshot`, final-output capture, and noninteractive safety are validated on main. Remaining broad CLI buckets must stay dependency-ordered: profile path/active-profile helpers before config/auth/setup commands, gateway read-model closeout before mutating service commands, and local log snapshot capture before backup/archive/upload behavior.
9. **5.O service restart polling is parser first, poller second.** Parse fake `RestartUSec` output and compute bounded restart delays before landing the fake active-status poller that waits `max(10s, RestartSec+10s)`. Neither row should touch real `systemctl`, Windows service control, installers, or gateway restart logic.
10. **5.Q TUI bundle drift is a deliberate divergence.** Upstream Hermes still has to defend its Node/Ink TUI bundle, including the `ee0728c6` stale-bundle rebuild check. Gormes proves the opposite with `cmd/gormes/tui_bundle_independence_test.go`: native Bubble Tea startup and offline status do not shell out to npm/node, inspect `node_modules`, or require `packages/hermes-ink/dist/ink-bundle.js`.
11. **5.Q TUI model/copy drift is native-first.** Port `283c8fd6` model/provider startup override semantics into Gormes' Cobra/Bubble Tea path with static alias fixtures and no provider catalog network calls. Treat `edc78e25`/`31d7f195` Ink selection-copy fixes as an explicit divergence row for now: docs and TUI status should say Gormes relies on terminal-native selection and should not advertise custom copy hotkeys until a Go-native copy mode exists.
