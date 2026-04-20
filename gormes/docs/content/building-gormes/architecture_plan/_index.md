---
title: "Architecture Plan"
weight: 10
---

# Gormes — Executive Roadmap

**Public site:** https://gormes.ai
**Source:** https://github.com/TrebuchetDynamics/gormes-agent
**Upstream reference:** https://github.com/NousResearch/hermes-agent

**Single source of truth:** [`progress.json`](progress.json) — machine-readable, auto-updated on build

**Linked surfaces:**
- [README.md](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/README.md) — Quick start + binary claims
- [Landing page](https://gormes.ai) — Marketing + feature list
- [docs.gormes.ai](https://docs.gormes.ai/building-gormes/architecture_plan/) — This page
- [Source code](https://github.com/TrebuchetDynamics/gormes-agent) — Implementation

---

## Progress Summary

| Phase | Status | Shipped |
|-------|--------|---------|
| Phase 1 — The Dashboard | ✅ Complete | 5 items |
| Phase 2 — The Gateway | 🔨 In Progress | 4 of 8 subphases |
| Phase 3 — The Black Box | 🔨 Substantially Complete | 5 of 12 subphases |
| Phase 4 — The Brain Transplant | ⏳ Planned | 0 of 8 subphases |
| Phase 5 — The Final Purge | ⏳ Planned | 0 of 17 subphases |
| Phase 6 — The Learning Loop | ⏳ Planned | 0 of 6 subphases |

**Overall:** 9/52 subphases shipped (17%) · 2 in progress · 41 planned

---

## Phase 1 — The Dashboard ✅ Complete

*Tactical bridge: Go TUI over Python's `api_server` HTTP+SSE boundary*

| Item | Status |
|------|--------|
| Bubble Tea TUI shell | ✅ |
| Kernel with 16 ms render mailbox | ✅ |
| Route-B SSE reconnect | ✅ |
| Wire Doctor — offline tool validation | ✅ |
| Streaming token renderer | ✅ |

**Ongoing:** Polish, bug fixes, TUI ergonomics

---

## Phase 2 — The Gateway 🔨 In Progress

*Go-native wiring harness: tools, Telegram, and thin session resume*

| Subphase | Status | Priority | Deliverable |
|----------|--------|---------|-------------|
| 2.A — Tool Registry | ✅ | P0 | In-process Go tool registry, streamed tool_calls |
| 2.B.1 — Telegram Scout | ✅ | P1 | Telegram adapter, long-poll, edit coalescing |
| 2.C — Thin Mapping Persistence | ✅ | P0 | bbolt session resume |
| 2.D — Cron / Scheduled Automations | ✅ | P2 | robfig/cron/v3 scheduler + bbolt cron_jobs bucket + cron_runs audit + CRON.md mirror + Heartbeat [SYSTEM:] prefix + [SILENT] suppression; Ollama E2E proven |
| 2.B.2+ — Wider Gateway Surface | ⏳ | P1 | Discord, Slack, WhatsApp, Signal... |
| 2.E — Subagent System | ⏳ | **P0** | Execution isolation, resource boundaries |
| 2.F — Hooks + Lifecycle | ⏳ | P2 | Per-event extension points |
| 2.G — Skills System | ⏳ | **P0** | Learning loop foundation |

---

## Phase 3 — The Black Box (Memory) 🔨 In Progress

*SQLite + FTS5 + ontological graph + semantic fusion in Go*

| Subphase | Status | Deliverable |
|----------|--------|-------------|
| 3.A — SQLite + FTS5 Lattice | ✅ | SqliteStore, FTS5 triggers, migrations |
| 3.B — Ontological Graph + LLM Extractor | ✅ | Extractor, entity/rel upsert |
| 3.C — Neural Recall + Context Injection | ✅ | RecallProvider, CTE, memory-context fence |
| 3.D — Semantic Fusion | ✅ | Ollama embeddings, cosine recall |
| 3.D.5 — Memory Mirror (USER.md) | ✅ | Async export, SQLite source of truth |
| 3.E.1 — Session Index Mirror | ⏳ | bbolt → YAML export |
| 3.E.2 — Tool Audit Log | ⏳ | JSONL audit trail |
| 3.E.3 — Transcript Export | ⏳ | Markdown export command |
| 3.E.4 — Extraction Visibility | ⏳ | `gormes memory status` |
| 3.E.5 — Insights Audit Log | ⏳ | Usage JSONL |
| 3.E.6 — Memory Decay | ⏳ | Weight attenuation, last_seen |
| 3.E.7 — Cross-Chat Synthesis | ⏳ | Graph unification across chats |

---

## Phase 4 — The Brain Transplant ⏳ Planned

*Native Go agent orchestrator + prompt builder. Hermes becomes optional.*

**Build priority:** Skills → Subagents → Gateway → Native Agent Loop

| Subphase | Deliverable |
|----------|-------------|
| 4.A — Provider Adapters | Anthropic, Bedrock, Gemini, OpenRouter... |
| 4.B — Context Engine | Long session management |
| 4.C — Native Prompt Builder | System + memory + tools + history |
| 4.D — Smart Model Routing | Per-turn model selection |
| 4.E — Trajectory + Insights | Self-monitoring telemetry |
| 4.F — Title Generation | Auto-naming sessions |
| 4.G — Credentials + OAuth | Token vault, multi-account auth |
| 4.H — Rate / Retry / Caching | Provider-side resilience |

---

## Phase 5 — The Final Purge ⏳ Planned

*Python disappears entirely from the runtime path*

17 subphases (5.A–5.Q): Tool Surface, Sandboxing, Browser, Vision, Voice, Skills, MCP, ACP, Plugins, Security, Code Exec, File Ops, MoA, Operator Tools, CLI Parity, Docker, TUI Gateway Streaming

---

## Phase 6 — The Learning Loop (Soul) ⏳ Planned

*The first Gormes-original core system — not a port*

| Subphase | Deliverable |
|----------|-------------|
| 6.A — Complexity Detector | Heuristic signal for worth-learning |
| 6.B — Skill Extractor | LLM-assisted pattern distillation |
| 6.C — Skill Storage Format | Portable SKILL.md format |
| 6.D — Skill Retrieval + Matching | Hybrid lexical + semantic |
| 6.E — Feedback Loop | Effectiveness scoring |
| 6.F — Skill Surface | TUI + Telegram browsing |

---

## Data Format

The [`progress.json`](progress.json) file is the machine-readable source of truth. It contains:
- Phase and subphase status for each item
- Links to all public surfaces
- Stats for automated dashboards

Updated automatically on `make build` via `scripts/record-progress.sh`.
