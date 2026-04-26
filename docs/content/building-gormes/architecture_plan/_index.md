---
title: "Architecture Plan"
weight: 20
---

# Gormes — Executive Roadmap

**Single source of truth:** [`progress.json`](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/docs/content/building-gormes/architecture_plan/progress.json) — machine-readable, validated + regenerated on build.

**Public site:** https://gormes.ai

**Linked surfaces:**
- [README.md](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/README.md) — Quick start + rollup phase table
- [Landing page](https://gormes.ai) — Marketing + roadmap section
- [docs.gormes.ai](https://docs.gormes.ai/building-gormes/architecture_plan/) — This page
- [Source code](https://github.com/TrebuchetDynamics/gormes-agent) — Implementation

**Execution control plane:** `cmd/builder-loop` consumes this `progress.json` and
the generated `docs/content/building-gormes/` pages to select and execute
eligible phase work. The roadmap is not only status reporting; it is the
machine-readable queue for developing the full `gormes-agent`.

## How To Read This Roadmap

- The generated checklist below is rebuilt from `progress.json`; do not hand-edit
  content between `PROGRESS` markers.
- Use the phase pages for design intent and boundaries, then use
  [Contract Readiness](../contract-readiness/) and [Agent Queue](../builder-loop/agent-queue/)
  for assignable work.
- When a row is too broad for one agent, split it in `progress.json` first and
  let [Umbrella Cleanup](../builder-loop/umbrella-cleanup/) show the remaining inventory.
- When a row is blocked, keep the unblock condition explicit so
  [Blocked Slices](../builder-loop/blocked-slices/) stays useful to operators and autoloop.

---

## Progress

<!-- PROGRESS:START kind=docs-full-checklist -->
**Overall:** 36/74 subphases shipped · 15 in progress · 23 planned

| Phase | Status | Shipped |
|-------|--------|---------|
| Phase 1 — The Dashboard | ✅ | 3/3 subphases |
| Phase 2 — The Gateway | 🔨 | 16/20 subphases |
| Phase 3 — The Black Box (Memory) | ✅ | 14/14 subphases |
| Phase 4 — The Brain Transplant | 🔨 | 0/8 subphases |
| Phase 5 — The Final Purge | 🔨 | 1/18 subphases |
| Phase 6 — The Learning Loop (Soul) | ⏳ | 0/6 subphases |
| Phase 7 — Paused Channel Backlog | 🔨 | 2/5 subphases |

---

## Phase 1 — The Dashboard ✅

*Tactical bridge: Go TUI over Python's api_server HTTP+SSE boundary*

### 1.A — Core TUI ✅

- [x] Bubble Tea shell
- [x] 16ms coalescing mailbox
- [x] SSE reconnect

### 1.B — Wire Doctor ✅

- [x] Offline tool validation

### 1.C — Automation Reliability ✅

- [x] Orchestrator failure-row stabilization for 4-8 workers
- [x] Soft-success-nonzero bats coverage
- [x] Planner wrapper/test consistency closeout
- [x] Autoloop row health and quarantine contract
- [x] Planner self-healing verdict loop
- [x] Planner divergence and provenance awareness

## Phase 2 — The Gateway 🔨

*Go-native operator wiring harness: tools, Telegram, shared gateway chassis, shipped cron, and the first OS-AI spine slices before focused channel closeout*

### 2.A — Tool Registry ✅

- [x] In-process Go tool registry
- [x] Streamed tool_calls accumulation
- [x] Kernel tool loop
- [x] Doctor verification

### 2.B.1 — Telegram Scout ✅

- [x] Telegram adapter
- [x] Long-poll ingress
- [x] Edit coalescing

### 2.B.2 — Gateway Chassis + Discord ✅

- [x] Reusable gateway chassis
- [x] Telegram on shared chassis
- [x] gormes gateway multi-channel entrypoint
- [x] Discord

### 2.B.3 — Slack on Shared Chassis ✅

- [x] Slack Socket Mode adapter
- [x] Thread routing + coalesced reply flow
- [x] Slack CommandRegistry parser wiring
- [x] Slack gateway.Channel adapter shim
- [x] Slack config + cmd/gormes gateway registration

### 2.B.4 — WhatsApp Adapter ✅

- [x] Bridge-vs-native runtime decision
- [x] WhatsApp identity resolution + self-chat guard
- [x] Inbound normalization + command passthrough
- [x] Pairing, reconnect, and send contract
- [x] WhatsApp outbound pairing gate + raw peer mapping
- [x] WhatsApp reconnect backoff + send retry policy

### 2.B.5 — Session Context + Delivery Routing 🔨

- [x] Gateway session store + SessionSource parity
- [x] SessionContext prompt injection
- [ ] BlueBubbles iMessage session-context prompt guidance
- [x] DeliveryRouter + --deliver target parsing
- [x] Gateway stream consumer for agent-event fan-out
- [x] Non-editable gateway progress/commentary send fallback

### 2.B.10 — WeChat Adapter ✅

- [x] WeCom + WeiXin shared-chassis bot seam
- [x] WeCom + WeiXin transport/bootstrap layer

### 2.B.11 — Discord Forum Channels 🔨

- [x] Discord forum channel ingress + thread lifecycle
- [x] Discord SessionSource guild/parent/message evidence
- [ ] Discord forum media + polish parity

### 2.C — Thin Mapping Persistence ✅

- [x] bbolt session resume
- [x] (platform, chat_id) -> session_id

### 2.D — Cron / Scheduled Automations ✅

- [x] robfig/cron scheduler + bbolt job store
- [x] SQLite cron_runs audit + CRON.md mirror
- [x] Heartbeat [SYSTEM:] + [SILENT] delivery contract
- [x] Architecture planner tasks manager script

### 2.E.0 — OS-AI Spine: Deterministic Subagent Runtime ✅

- [x] Deterministic subagent runtime
- [x] Max-depth guard + bounded batch execution
- [x] Timeout + cancellation scopes
- [x] Typed result envelope
- [x] Append-only run log

### 2.E.1 — OS-AI Spine: Delegation Policy + Child Execution ✅

- [x] Runner-enforced tool allowlists + blocked-tool policy
- [x] Tool-call audit in typed child results
- [x] Real child Hermes stream loop
- [x] GBrain minion-orchestrator routing policy
- [x] Durable subagent/job ledger

### 2.E.2 — OS-AI Spine: Concurrent-Tool Cancellation ✅

- [x] Interrupt propagation to concurrent-tool workers

### 2.E.3 — OS-AI Spine: Durable Job Resilience ✅

- [x] Durable job backpressure + timeout audit
- [x] Durable worker supervisor status seam
- [x] Durable pause/resume intent contract
- [x] Durable replay and inbox message contract

### 2.F.1 — Slash Command Registry + Gateway Dispatch ✅

- [x] Canonical CommandDef registry
- [x] Gateway slash dispatch + per-platform exposure

### 2.F.2 — Hook Registry + BOOT.md ✅

- [x] Gateway per-event hook registry
- [x] Hook manifest discovery + handler loading
- [x] Built-in BOOT.md startup hook

### 2.F.3 — Restart / Pairing / Status ✅

- [x] Graceful restart drain + managed shutdown
- [x] Adapter startup failure cleanup contract
- [x] Active-turn follow-up queue + late-arrival drain policy
- [x] Drain-timeout resume_pending recovery
- [x] Pairing read-model schema + atomic persistence
- [x] Pairing approval + rate-limit semantics
- [x] Unauthorized DM pairing response contract
- [x] `gormes gateway status` read-only command
- [x] Runtime status JSON + PID/process validation
- [x] Token-scoped gateway locks
- [x] Gateway /restart command + takeover markers
- [x] Session expiry finalized-flag migration
- [x] Session expiry hook cleanup retry evidence
- [x] Channel lifecycle writers into status model

### 2.F.4 — Home Channel + Operator Surfaces ⏳

- [ ] Home channel ownership rules
- [ ] Notify-to delivery routing
- [ ] Channel directory atomic persistence + lookup
- [ ] Channel directory refresh + stale-target invalidation
- [ ] Manager remember-source hook
- [ ] Mirror + sticker cache surfaces

### 2.F.5 — Gateway Mid-Run Steering + Active-Turn Policy ⏳

- [ ] Steer slash command registry + queue fallback
- [ ] Mid-run steer injection between tool calls
- [ ] Gateway-handled slash commands bypass active-session guard

### 2.G — OS-AI Spine: Skills Runtime ✅

- [x] SKILL.md parsing + active store
- [x] Deterministic selection + prompt block
- [x] Kernel injection + usage log
- [x] Inactive candidate drafting
- [x] Explicit promotion flow

## Phase 3 — The Black Box (Memory) ✅

*SQLite + FTS5 + ontological graph + semantic fusion in Go; 3.E closes session visibility, audit trails, decay, and cross-chat/session boundaries*

### 3.A — SQLite + FTS5 Lattice ✅

- [x] SqliteStore
- [x] FTS5 triggers
- [x] Schema migrations v3a->v3d

### 3.B — Ontological Graph + LLM Extractor ✅

- [x] Extractor
- [x] Entity/relationship upsert
- [x] Dead-letter queue

### 3.C — Neural Recall + Context Injection ✅

- [x] RecallProvider
- [x] 2-layer seed selection
- [x] CTE traversal
- [x] <memory-context> fence

### 3.D — Semantic Fusion + Local Embeddings ✅

- [x] Ollama embeddings
- [x] Vector cache
- [x] Cosine similarity recall
- [x] Hybrid fusion

### 3.D.5 — Memory Mirror (USER.md sync) ✅

- [x] Async background export
- [x] SQLite as source of truth

### 3.E.1 — Session Index Mirror ✅

- [x] Read-only bbolt sessions.db -> index.yaml mirror
- [x] Deterministic mirror refresh without mutating session state

### 3.E.2 — Tool Execution Audit Log ✅

- [x] Append-only JSONL writer + schema
- [x] Kernel + delegate_task audit hooks
- [x] Outcome, duration, and error capture

### 3.E.3 — Transcript Export Command ✅

- [x] gormes session export <id> --format=markdown
- [x] Render turns, tool calls, and timestamps from SQLite

### 3.E.4 — Extraction State Visibility ✅

- [x] gormes memory status command
- [x] Extractor queue depth + dead-letter summary

### 3.E.5 — Insights Audit Log ✅

- [x] Append-only daily usage.jsonl writer
- [x] Session, token, and cost rollups from local runtime

### 3.E.6 — Memory Decay ✅

- [x] relationships.last_seen schema + backfill
- [x] Relationship writer freshness updates
- [x] Deterministic weight attenuation at recall time

### 3.E.7 — Cross-Chat Synthesis ✅

- [x] user_id concept above chat_id
- [x] Same-chat default recall fence
- [x] Opt-in user-scope recall + source filters
- [x] Interrupted-turn memory sync suppression
- [x] Honcho-compatible scope/source tool schema
- [x] Honcho host integration compatibility fixtures
- [x] SillyTavern persona and group-chat mapping fixtures
- [x] Cross-chat deny-path fixtures
- [x] Cross-chat operator evidence

### 3.E.8 — Session Lineage + Cross-Source Search ✅

- [x] parent_session_id lineage for compression splits
- [x] Gateway resume follows compression continuation
- [x] Source-filtered session/message search core
- [x] GONCHO user-scope search/context parameters
- [x] Lineage-aware source-filtered search hits
- [x] Operator-auditable search evidence

### 3.F — Goncho Honcho Memory Parity ✅

- [x] Goncho context representation options
- [x] Goncho search filter grammar
- [x] Directional peer cards and representation scopes
- [x] Goncho queue status read model
- [x] Goncho summary context budget
- [x] Goncho dialectic chat contract
- [x] Goncho file upload import ingestion
- [x] Goncho topology design fixtures
- [x] Goncho operator diagnostics contract
- [x] Goncho streaming chat persistence contract
- [x] Goncho configuration namespace
- [x] Goncho dreaming scheduler contract

## Phase 4 — The Brain Transplant 🔨

*Native Go agent orchestrator + prompt builder*

### 4.A — Provider Adapters 🔨

- [x] Provider interface + stream fixture harness
- [x] Tool-call normalization + continuation contract
- [x] DeepSeek/Kimi reasoning_content echo for tool-call replay
- [x] DeepSeek/Kimi cross-provider reasoning isolation
- [ ] DeepSeek/Kimi all-assistant reasoning_content replay
- [x] Anthropic
- [ ] Azure OpenAI query/default_query transport contract
- [ ] Azure Anthropic Messages endpoint contract
- [ ] Azure Foundry endpoint autodetect + model context read model
- [ ] Bedrock
- [x] Bedrock Converse payload mapping (no AWS SDK)
- [ ] Bedrock stream event decoding (SSE fixtures)
- [ ] Bedrock SigV4 + credential seam
- [ ] Bedrock stale-client eviction + retry classification
- [ ] Gemini
- [ ] OpenRouter
- [ ] Google Code Assist
- [ ] Codex
- [x] Codex Responses pure conversion harness
- [x] Codex Responses assistant content role types
- [ ] Codex OAuth state + stale-token relogin
- [x] Codex stream repair + tool-call leak sanitizer
- [ ] Cross-provider reasoning-tag sanitization
- [x] Tool-call argument repair + schema sanitizer

### 4.B — Context Engine + Compression 🔨

- [ ] Long session management
- [ ] Context compression
- [x] ContextEngine interface + status tool contract
- [x] Compression token-budget trigger + summary sizing
- [x] Aux compression headroom for system and tool schemas
- [x] Aux compression provider-aware context cap
- [ ] Tool-result pruning + protected head/tail summary
- [x] Aux compression single-prompt threshold reconciliation
- [ ] Manual compression feedback + context references

### 4.C — Native Prompt Builder ⏳

- [ ] System + memory + tools + history assembly
- [ ] Context-file discovery + injection scan
- [ ] Model-specific role and tool-use guidance
- [ ] Toolset-aware skills prompt snapshot
- [ ] Memory and session-search guidance assembly

### 4.D — Smart Model Routing 🔨

- [ ] Model metadata registry + context limits
- [x] Provider-enforced context-length resolver
- [x] Model pricing/capability registry fixtures
- [x] Routing policy and fallback selector
- [x] Per-turn model selection
- [ ] Per-turn reasoning effort propagation

### 4.E — Trajectory + Insights ⏳

- [ ] Trajectory writer + redaction gates
- [ ] Self-monitoring telemetry

### 4.F — Title Generation ⏳

- [ ] Title prompt and truncation contract
- [ ] Auto-naming sessions

### 4.G — Credentials + OAuth ⏳

- [ ] Token vault
- [ ] Anthropic OAuth/keychain credential discovery
- [ ] Multi-account auth
- [ ] Google OAuth flow + refresh seam

### 4.H — Rate / Retry / Caching 🔨

- [x] Provider-side resilience
- [x] Classified provider-error taxonomy
- [x] Unsupported temperature retry + Codex no-temperature guard
- [x] Codex Responses temperature guard after flush removal
- [x] Generic unsupported-parameter retry + max_tokens guard
- [x] Jittered reconnect backoff schedule
- [x] Retry-After header parsing + HTTPError hint
- [x] Kernel retry honors Retry-After hint
- [x] Streaming interrupt retry suppression
- [ ] Prompt-cache capability guard
- [ ] Provider rate guard + budget telemetry

## Phase 5 — The Final Purge 🔨

*Python tool scripts ported to Go or WASM*

### 5.A — Tool Surface Port 🔨

- [ ] 61-tool registry port
- [x] Tool registry inventory + schema parity harness
- [x] Tool parity manifest refresh for Hermes b35d692f
- [x] Discord tool split + platform-scoped toolsets
- [ ] Pure core tools first
- [ ] Stateful tool migration queue
- [x] Terminal process watch notification throttle contract

### 5.B — Sandboxing Backends ⏳

- [ ] Environment interface + file sync contract
- [ ] Docker
- [ ] Modal
- [ ] Daytona
- [ ] Singularity

### 5.C — Browser Automation ⏳

- [ ] Browser action contract + event transcript
- [ ] Chromedp
- [ ] Rod
- [ ] Browser provider bridge + Firecrawl fallback

### 5.D — Vision + Image Generation ⏳

- [ ] Multimodal in/out
- [ ] Vision input normalization + token budget
- [ ] Image generation result contract

### 5.E — TTS / Voice / Transcription ⏳

- [ ] Voice mode port
- [ ] Transcription tool contract
- [ ] TTS synthesis + voice-mode state

### 5.F — Skills System (Remaining) 🔨

- [ ] Skills hub
- [ ] Skill registries
- [x] Skill preprocessing + dynamic slash commands

### 5.G — MCP Integration 🔨

- [ ] MCP client
- [x] MCP server config/env resolver
- [ ] MCP fake-server discovery + tool schema normalization
- [ ] MCP OAuth state store + noninteractive auth errors
- [ ] Managed tool gateway bridge

### 5.H — ACP Integration ⏳

- [ ] ACP server side

### 5.I — Plugins Architecture 🔨

- [x] Plugin SDK
- [x] Dashboard theme/plugin extension status contract
- [x] Dashboard page-scoped plugin slot inventory
- [ ] Third-party extensions
- [x] First-party Spotify plugin fixture

### 5.J — Approval / Security Guards ⏳

- [ ] Dangerous action gating
- [ ] Dangerous-command detector + blocked-result schema
- [ ] Approval mode config normalization
- [ ] Subagent dangerous-command non-interactive approval policy
- [ ] Cron dangerous-command approval mode
- [ ] Tirith, path, URL, and website policy integration

### 5.K — Code Execution ✅

- [x] Sandboxed exec

### 5.L — File Ops + Patches ⏳

- [ ] Atomic checkpoints

### 5.M — Mixture of Agents ⏳

- [ ] Multi-model coordination

### 5.N — Misc Operator Tools ⏳

- [ ] Todo
- [ ] Clarify
- [ ] Session search
- [ ] Debug helpers
- [ ] Planner backend noninteractive stdin failure guard
- [ ] Cronjob tool API + schedule parser parity
- [ ] Cron context_from output chaining
- [ ] Cron prompt/script safety + pre-run script contract
- [ ] Cron multi-target delivery + media/live-adapter fallback

### 5.O — Hermes CLI Parity 🔨

- [ ] 49-file CLI tree port
- [ ] Deterministic helper-file ports (banner/output/tips/webhook/dump)
- [x] CLI banner/output formatting helpers
- [ ] CLI tips/dump/webhook deterministic helpers
- [x] PTY bridge protocol adapter
- [ ] CLI command registry parity + active-turn busy policy
- [ ] Gateway /reasoning session override command
- [ ] Busy command guard for compression and long CLI actions
- [ ] Config, profile, auth, and setup command surfaces
- [ ] CLI profile path and active-profile store
- [x] Top-level oneshot flag and model/provider resolver
- [x] Oneshot final-output writer boundary
- [x] Oneshot noninteractive safety and clarify policy
- [x] Platform toolset config persistence + MCP sentinel
- [x] Effective toolset picker dedupes bundled plugin keys
- [ ] Gateway, platform, webhook, and cron management CLI
- [ ] Gateway management CLI read-model closeout
- [x] Service RestartSec parser helper
- [x] Service restart active-status poller
- [ ] Diagnostics, backup, logs, and status CLI
- [ ] Doctor custom endpoint provider readiness
- [ ] Custom provider model-switch credential preservation
- [ ] CLI log snapshot reader

### 5.P — Docker / Packaging ⏳

- [ ] OCI image
- [ ] Homebrew
- [ ] Unix installer (install.sh) source-backed update flow
- [ ] Unix installer root/FHS layout policy
- [ ] Windows installer (install.ps1 + install.cmd) parity
- [ ] Installer site asset/route coverage

### 5.Q — API Server + TUI Gateway Streaming 🔨

- [ ] Deterministic helper-file ports (tool-progress/image/completion-path/personality/platform-event)
- [ ] TUI gateway progress/completion helpers
- [ ] TUI gateway image/personality/platform-event helpers
- [x] TUI mouse tracking config + slash toggle
- [x] Native TUI bundle independence check
- [x] TUI launch model override + static alias resolver
- [ ] Native TUI terminal-selection divergence contract
- [ ] Native TUI /save canonical session export
- [ ] SSE streaming to Bubble Tea TUI
- [x] OpenAI-compatible chat-completions API server
- [x] Responses API store + run event stream
- [x] API server disconnect snapshot persistence
- [x] Gateway proxy mode forwarding contract
- [x] Dashboard API client contract
- [ ] Dashboard PTY chat sidecar contract
- [ ] API server health + cron admin endpoints

### 5.R — Code Execution Mode Policy ⏳

- [ ] Execution-mode resolver + config precedence
- [ ] Strict-mode CWD + interpreter parity
- [ ] Project-mode CWD + active venv detection
- [ ] Default mode selection + config cut-over

## Phase 6 — The Learning Loop (Soul) ⏳

*Native skill extraction. Compounding intelligence. The feature Hermes doesn't have.*

### 6.A — Complexity Detector ⏳

- [ ] Heuristic or LLM-scored signal

### 6.B — Skill Extractor ⏳

- [ ] LLM-assisted pattern distillation

### 6.C — Skill Storage Format ⏳

- [ ] Portable SKILL.md format

### 6.D — Skill Retrieval + Matching ⏳

- [ ] Hybrid lexical + semantic lookup
- [ ] Code Cathedral II code-context retrieval fixtures

### 6.E — Feedback Loop ⏳

- [ ] Skill effectiveness scoring

### 6.F — Skill Surface ⏳

- [ ] TUI + Telegram browsing

## Phase 7 — Paused Channel Backlog 🔨

*Deferred non-priority channel adapters after Telegram, Discord, Slack, WhatsApp, and WeChat stabilize*

### 7.A — Signal Adapter 🔨

- [x] Inbound event normalization + session identity
- [x] Reply/send contract on shared chassis
- [ ] Signal transport/bootstrap layer

### 7.B — Email + SMS Adapters ✅

- [x] Email ingress + outbound delivery contract
- [x] SMS ingress + outbound delivery contract

### 7.C — Matrix + Mattermost Adapters 🔨

- [x] Threaded text adapter contract suite
- [ ] Matrix shared-chassis bot seam
- [ ] Mattermost shared-chassis bot seam
- [ ] Matrix real client/bootstrap layer
- [ ] Matrix E2EE device-id crypto-store binding
- [ ] Mattermost REST/WS bootstrap layer

### 7.D — Webhook + Trigger Ingress ✅

- [x] Signed event parsing + auth gates
- [x] Prompt-to-delivery routing bridge

### 7.E — Regional + Device Adapter Backlog 🔨

- [x] BlueBubbles + HomeAssistant adapters
- [ ] BlueBubbles iMessage bubble formatting parity
- [x] Feishu shared-chassis bot seam
- [x] DingTalk shared-chassis bot seam
- [x] QQ Bot shared-chassis bot seam
- [ ] Feishu transport/bootstrap layer
- [ ] Feishu drive-comment rule + pairing seam
- [ ] Feishu drive-comment reply workflow
- [x] DingTalk transport/bootstrap layer
- [ ] DingTalk real SDK binding
- [x] DingTalk AI Cards streaming-update contract
- [x] DingTalk emoji reaction send/receive parity
- [x] DingTalk media (image/file) attachment routing
- [ ] QQ Bot transport/bootstrap layer

<!-- PROGRESS:END -->

---

## Phase 3 Deep Dive

`3.E.7` and `3.E.8` now have a frozen architecture target in `docs/superpowers/plans/2026-04-22-gormes-phase3-identity-lineage-plan.md`. The contract is `user_id > chat_id > session_id`, recall remains same-chat default, cross-chat recall is opt-in, and `parent_session_id` is reserved for compression/fork descendants instead of becoming a generic session rewrite mechanism.

Execution is now sequenced in `docs/superpowers/plans/2026-04-22-gormes-phase3-identity-lineage-execution-plan.md`, with the closeout order fixed as `3.E.6.1 -> 3.E.7.2 -> 3.E.8.1 -> 3.E.8.2` so freshness, fence safety, lineage metadata, and search/observability land in that order.

---

## Phase 4 Entry Gate

Before any Phase 4 coding starts, the [Pre-Phase-4 E2E Gate](./phase-3-memory/) must be green. Freeze the Hermes-backed hybrid baseline for delivery envelopes, `<memory-context>` fences, and transcript/export artifacts first, then follow the entry rule in [Phase 4 — The Brain Transplant](./phase-4-brain-transplant/).

---

## Data Format

[`progress.json`](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/docs/content/building-gormes/architecture_plan/progress.json) is the machine-readable source of truth. Top-level structure:

- `meta` — schema version, last-updated timestamp, canonical URLs
- `phases` — six phases keyed `"1"`..`"6"`, each containing `subphases`
- each subphase carries either `items` (the normal case) or an explicit `status`

Stats (complete/in-progress/planned counts) are **not stored** — they are computed on render. Updated automatically on `make build`.
