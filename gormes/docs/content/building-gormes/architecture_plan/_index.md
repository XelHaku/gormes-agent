---
title: "Architecture Plan"
weight: 10
---

# Gormes — Executive Roadmap

**Single source of truth:** [`progress.json`](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/content/building-gormes/architecture_plan/progress.json) — machine-readable, validated + regenerated on build.

**Public site:** https://gormes.ai

**Linked surfaces:**
- [README.md](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/README.md) — Quick start + rollup phase table
- [Landing page](https://gormes.ai) — Marketing + roadmap section
- [docs.gormes.ai](https://docs.gormes.ai/building-gormes/architecture_plan/) — This page
- [Source code](https://github.com/TrebuchetDynamics/gormes-agent) — Implementation

---

## Progress

<!-- PROGRESS:START kind=docs-full-checklist -->
**Overall:** 42/66 subphases shipped · 2 in progress · 22 planned

| Phase | Status | Shipped |
|-------|--------|---------|
| Phase 1 — The Dashboard | ✅ | 2/2 subphases |
| Phase 2 — The Gateway | ✅ | 20/20 subphases |
| Phase 3 — The Black Box (Memory) | ✅ | 13/13 subphases |
| Phase 4 — The Brain Transplant | 🔨 | 6/8 subphases |
| Phase 5 — The Final Purge | 🔨 | 1/17 subphases |
| Phase 6 — The Learning Loop (Soul) | ⏳ | 0/6 subphases |

---

## Phase 1 — The Dashboard ✅

*Tactical bridge: Go TUI over Python's api_server HTTP+SSE boundary*

### 1.A — Core TUI ✅

- [x] Bubble Tea shell
- [x] 16ms coalescing mailbox
- [x] SSE reconnect

### 1.B — Wire Doctor ✅

- [x] Offline tool validation

## Phase 2 — The Gateway ✅

*Go-native operator wiring harness: tools, Telegram, shared gateway chassis, shipped cron, and the first OS-AI spine slices before the long-tail adapter flood*

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
- [x] Gateway command wiring

### 2.B.4 — WhatsApp Adapter ✅

- [x] Bridge-vs-native runtime decision
- [x] Inbound normalization + command passthrough
- [x] Pairing, reconnect, and send contract

### 2.B.5 — Session Context + Delivery Routing ✅

- [x] Gateway session store + SessionSource parity
- [x] SessionContext prompt injection
- [x] DeliveryRouter + --deliver target parsing
- [x] Gateway stream consumer for agent-event fan-out

### 2.B.6 — Signal Adapter ✅

- [x] Inbound event normalization + session identity
- [x] Reply/send contract on shared chassis

### 2.B.7 — Email + SMS Adapters ✅

- [x] Email ingress + outbound delivery contract
- [x] SMS ingress + outbound delivery contract

### 2.B.8 — Matrix + Mattermost Adapters ✅

- [x] Threaded text adapter contract suite
- [x] Matrix + Mattermost transport wiring

### 2.B.9 — Webhook + Trigger Ingress ✅

- [x] Signed event parsing + auth gates
- [x] Prompt-to-delivery routing bridge

### 2.B.10 — Regional + Device Adapter Flood ✅

- [x] BlueBubbles + HomeAssistant adapters
- [x] Feishu + WeChat/WeCom adapters
- [x] DingTalk + QQ Bot adapters

### 2.C — Thin Mapping Persistence ✅

- [x] bbolt session resume
- [x] (platform, chat_id) -> session_id

### 2.D — Cron / Scheduled Automations ✅

- [x] robfig/cron scheduler + bbolt job store
- [x] SQLite cron_runs audit + CRON.md mirror
- [x] Heartbeat [SYSTEM:] + [SILENT] delivery contract

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

### 2.F.1 — Slash Command Registry + Gateway Dispatch ✅

- [x] Canonical CommandDef registry
- [x] Gateway slash dispatch + per-platform exposure

### 2.F.2 — Hook Registry + BOOT.md ✅

- [x] Gateway per-event hook registry
- [x] Hook manifest discovery + handler loading
- [x] Built-in BOOT.md startup hook

### 2.F.3 — Restart / Pairing / Status ✅

- [x] Graceful restart drain + managed shutdown
- [x] Pairing state + status surfaces

### 2.F.4 — Home Channel + Operator Surfaces ✅

- [x] Home channel ownership + notify-to routing
- [x] Channel/contact directory
- [x] Mirror + sticker cache surfaces

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

- [x] Relationship last_seen tracking
- [x] Deterministic weight attenuation at recall time

### 3.E.7 — Cross-Chat Synthesis ✅

- [x] user_id concept above chat_id
- [x] Cross-chat entity merge + recall fence

### 3.E.8 — Session Lineage + Cross-Source Search ✅

- [x] parent_session_id lineage for compression splits
- [x] Source-filtered FTS/session search across chats

## Phase 4 — The Brain Transplant 🔨

*Native Go agent orchestrator + prompt builder*

### 4.A — Provider Adapters ✅

- [x] Anthropic
- [x] Bedrock
- [x] Gemini
- [x] OpenRouter
- [x] Google Code Assist
- [x] Codex

### 4.B — Context Engine + Compression ✅

- [x] Long session management
- [x] Context compression

### 4.C — Native Prompt Builder ✅

- [x] System + memory + tools + history assembly

### 4.D — Smart Model Routing ✅

- [x] Per-turn model selection

### 4.E — Trajectory + Insights ✅

- [x] Self-monitoring telemetry

### 4.F — Title Generation ✅

- [x] Auto-naming sessions

### 4.G — Credentials + OAuth 🔨

- [ ] Token vault
- [x] Multi-account auth

### 4.H — Rate / Retry / Caching ⏳

- [ ] Provider-side resilience

## Phase 5 — The Final Purge 🔨

*Python tool scripts ported to Go or WASM*

### 5.A — Tool Surface Port ✅

- [x] 61-tool registry port

### 5.B — Sandboxing Backends 🔨

- [ ] Docker
- [ ] Modal
- [x] Daytona
- [ ] Singularity

### 5.C — Browser Automation ⏳

- [ ] Chromedp
- [ ] Rod

### 5.D — Vision + Image Generation ⏳

- [ ] Multimodal in/out

### 5.E — TTS / Voice / Transcription ⏳

- [ ] Voice mode port

### 5.F — Skills System (Remaining) ⏳

- [ ] Skills hub
- [ ] Skill registries

### 5.G — MCP Integration ⏳

- [ ] MCP client
- [ ] OAuth flows

### 5.H — ACP Integration ⏳

- [ ] ACP server side

### 5.I — Plugins Architecture ⏳

- [ ] Plugin SDK
- [ ] Third-party extensions

### 5.J — Approval / Security Guards ⏳

- [ ] Dangerous action gating

### 5.K — Code Execution ⏳

- [ ] Sandboxed exec

### 5.L — File Ops + Patches ⏳

- [ ] Atomic checkpoints

### 5.M — Mixture of Agents ⏳

- [ ] Multi-model coordination

### 5.N — Misc Operator Tools ⏳

- [ ] Todo
- [ ] Clarify
- [ ] Session search
- [ ] Debug helpers

### 5.O — Hermes CLI Parity ⏳

- [ ] 49-file CLI tree port

### 5.P — Docker / Packaging ⏳

- [ ] OCI image
- [ ] Homebrew

### 5.Q — TUI Gateway Streaming ⏳

- [ ] SSE streaming to Bubble Tea TUI

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

### 6.E — Feedback Loop ⏳

- [ ] Skill effectiveness scoring

### 6.F — Skill Surface ⏳

- [ ] TUI + Telegram browsing

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

[`progress.json`](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/content/building-gormes/architecture_plan/progress.json) is the machine-readable source of truth. Top-level structure:

- `meta` — schema version, last-updated timestamp, canonical URLs
- `phases` — six phases keyed `"1"`..`"6"`, each containing `subphases`
- each subphase carries either `items` (the normal case) or an explicit `status`

Stats (complete/in-progress/planned counts) are **not stored** — they are computed on render. Updated automatically on `make build`.
