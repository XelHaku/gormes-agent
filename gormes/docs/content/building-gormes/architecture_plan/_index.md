---
title: "Architecture Plan"
weight: 10
---

# Gormes — Executive Roadmap

**Single source of truth:** `progress.json` — machine-readable, validated + regenerated on build.
**Public site:** https://gormes.ai

**Linked surfaces:**
- [README.md](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/README.md) — Quick start + rollup phase table
- [Landing page](https://gormes.ai) — Marketing + roadmap section
- [docs.gormes.ai](https://docs.gormes.ai/building-gormes/architecture_plan/) — This page
- [Source code](https://github.com/TrebuchetDynamics/gormes-agent) — Implementation

---

## Progress

<!-- PROGRESS:START kind=docs-full-checklist -->
**Overall:** 10/53 subphases shipped · 0 in progress · 43 planned

| Phase | Status | Shipped |
|-------|--------|---------|
| Phase 1 — The Dashboard | ✅ | 2/2 subphases |
| Phase 2 — The Gateway | 🔨 | 3/8 subphases |
| Phase 3 — The Black Box (Memory) | 🔨 | 5/12 subphases |
| Phase 4 — The Brain Transplant | ⏳ | 0/8 subphases |
| Phase 5 — The Final Purge | ⏳ | 0/17 subphases |
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

## Phase 2 — The Gateway 🔨

*Go-native tools + Telegram + session resume + wider adapters*

### 2.A — Tool Registry ✅

- [x] In-process Go tool registry
- [x] Streamed tool_calls accumulation
- [x] Kernel tool loop
- [x] Doctor verification

### 2.B.1 — Telegram Scout ✅

- [x] Telegram adapter
- [x] Long-poll ingress
- [x] Edit coalescing

### 2.B.2 — Wider Gateway Surface ⏳

- [ ] Discord
- [ ] Slack
- [ ] WhatsApp
- [ ] Signal
- [ ] Email
- [ ] SMS

### 2.C — Thin Mapping Persistence ✅

- [x] bbolt session resume
- [x] (platform, chat_id) -> session_id

### 2.D — Cron / Scheduled Automations ⏳

- [ ] Go ticker + bbolt job store
- [ ] Natural-language cron parsing (Phase 4)

### 2.E — Subagent System ⏳

- [ ] Execution isolation
- [ ] Resource boundaries
- [ ] Context isolation
- [ ] Cancellation scopes

### 2.F — Hooks + Lifecycle ⏳

- [ ] Per-event extension points
- [ ] Managed restarts

### 2.G — Skills System ⏳

- [ ] Learning loop foundation
- [ ] Pattern extraction

## Phase 3 — The Black Box (Memory) 🔨

*SQLite + FTS5 + ontological graph + semantic fusion in Go*

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

### 3.E.1 — Session Index Mirror ⏳

- [ ] bbolt sessions.yaml export

### 3.E.2 — Tool Execution Audit Log ⏳

- [ ] JSONL audit trail

### 3.E.3 — Transcript Export Command ⏳

- [ ] Markdown export

### 3.E.4 — Extraction State Visibility ⏳

- [ ] gormes memory status

### 3.E.5 — Insights Audit Log ⏳

- [ ] Usage JSONL

### 3.E.6 — Memory Decay ⏳

- [ ] Weight attenuation
- [ ] last_seen tracking

### 3.E.7 — Cross-Chat Synthesis ⏳

- [ ] Graph unification across chats

## Phase 4 — The Brain Transplant ⏳

*Native Go agent orchestrator + prompt builder*

### 4.A — Provider Adapters ⏳

- [ ] Anthropic
- [ ] Bedrock
- [ ] Gemini
- [ ] OpenRouter
- [ ] Google Code Assist
- [ ] Codex

### 4.B — Context Engine + Compression ⏳

- [ ] Long session management
- [ ] Context compression

### 4.C — Native Prompt Builder ⏳

- [ ] System + memory + tools + history assembly

### 4.D — Smart Model Routing ⏳

- [ ] Per-turn model selection

### 4.E — Trajectory + Insights ⏳

- [ ] Self-monitoring telemetry

### 4.F — Title Generation ⏳

- [ ] Auto-naming sessions

### 4.G — Credentials + OAuth ⏳

- [ ] Token vault
- [ ] Multi-account auth

### 4.H — Rate / Retry / Caching ⏳

- [ ] Provider-side resilience

## Phase 5 — The Final Purge ⏳

*Python tool scripts ported to Go or WASM*

### 5.A — Tool Surface Port ⏳

- [ ] 61-tool registry port

### 5.B — Sandboxing Backends ⏳

- [ ] Docker
- [ ] Modal
- [ ] Daytona
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

## Data Format

`progress.json` is the machine-readable source of truth. Top-level structure:

- `meta` — schema version, last-updated timestamp, canonical URLs
- `phases` — six phases keyed `"1"`..`"6"`, each containing `subphases`
- each subphase carries either `items` (the normal case) or an explicit `status`

Stats (complete/in-progress/planned counts) are **not stored** — they are computed on render. Updated automatically on `make build`.
