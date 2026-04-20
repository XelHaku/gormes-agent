# Gormes — Executive Roadmap (ARCH_PLAN)

**Public site:** https://gormes.ai
**Source:** https://github.com/TrebuchetDynamics/gormes-agent
**Upstream reference:** https://github.com/NousResearch/hermes-agent

---

## 0. Operational Moat Thesis

When intelligence becomes abundant, operational friction becomes the bottleneck.

That is the reason Gormes exists.

If models keep improving, the differentiator stops being whether an agent can produce a clever answer and starts being whether the system can stay alive, recover fast, deploy cleanly, and run everywhere without constant babysitting. Gormes is built for that era.

The strategic target is not "a Go wrapper around Hermes." The strategic target is a pure Go binary that owns the full lifecycle of a serious always-on agent.

---

## 1. Rosetta Stone Declaration

The repository root is the **Reference Implementation** (Python, upstream `NousResearch/hermes-agent`). The `gormes/` directory is the **High-Performance Implementation** (Go). Neither replaces the other during Phases 1–4; they co-evolve as a translation pair until Phase 5's final purge completes the migration.

---

## 2. Why Go — for a Python developer

Five concrete bullets, no hype:

1. **Binary portability.** One ~12 MB static binary (CGO-free). No `uv`, `pip`, venv, or system Python on the target host. `scp`-and-run on a $5 VPS or Termux.
2. **Static types and compile-time contracts.** Tool schemas, Provider envelopes, and MCP payloads become typed structs. Schema drift is a compile error, not a silent agent-loop failure.
3. **True concurrency.** Goroutines over channels replace `asyncio`. The gateway scales to 10+ platform connections without event-loop starvation.
4. **Lower idle footprint.** Target ≈ 10 MB RSS at idle vs. ≈ 80+ MB for Python Hermes. Meaningful on always-on or low-spec hosts.
5. **Explicit trade-off.** The Python AI-library moat (`litellm`, `instructor`, heavyweight ML, research skills) stays in Python until Phase 4–5.

---

## 3. Hybrid Manifesto — the Motherboard Strategy

The hybrid is **temporary**. The long-term state is 100% Go.

During Phases 1–4, Go is the chassis (orchestrator, state, persistence, platform I/O, agent cognition) and Python is the peripheral library (research tools, legacy skills, ML heavy lifting). Each phase shrinks Python's footprint. Phase 5 deletes the last Python dependency.

Phase 3 (The Black Box) is substantially delivered as of 2026-04-20: the SQLite + FTS5 lattice (3.A), ontological graph with async LLM extraction (3.B), lexical/FTS5 recall with `<memory-context>` fence injection (3.C), semantic fusion via Ollama embeddings with cosine similarity recall (3.D), and the operator-facing memory mirror (3.D.5) are all implemented. Remaining Phase 3 work is 3.E — decay, cross-chat synthesis, and the operational-visibility mirrors (session index, insights audit, tool audit, transcript export).

Phase 1 should be read correctly: it is a tactical Strangler Fig bridge, not a philosophical compromise. It exists to deliver immediate value to existing Hermes users while preserving a clean migration path toward a pure Go runtime that owns the entire lifecycle end to end.

---

## 4. Milestone Status

| Phase | Status | Deliverable |
|---|---|---|
| Phase 1 — The Dashboard (Face) | ✅ complete | Tactical bridge: Go TUI over Python's `api_server` HTTP+SSE boundary |
| Phase 2 — The Wiring Harness (Gateway) | 🔨 in progress | Go-native wiring harness: tools, Telegram, and thin session resume land before the wider gateway surface |
| Phase 3 — The Black Box (Memory) | 🔨 3.A–3.D shipped; 3.E planned | SQLite + FTS5 + ontological graph + semantic fusion in Go; 3.E adds decay, cross-chat synthesis, and operational-visibility mirrors |
| Phase 4 — The Powertrain (Brain Transplant) | ⏳ planned | Native Go agent orchestrator + prompt builder |
| Phase 5 — The Final Purge (100% Go) | ⏳ planned | Python tool scripts ported to Go or WASM |

Legend: 🔨 in progress · ✅ complete · ⏳ planned · ⏸ deferred.

**Phase 3 sub-status (as of 2026-04-20):**
- **3.A — SQLite + FTS5 Lattice** — ✅ implemented (`internal/memory`, `SqliteStore`, FTS5 triggers, fire-and-forget worker, schema v3a→v3d migrations)
- **3.B — Ontological Graph + LLM Extractor** — ✅ implemented (`Extractor`, entity/relationship upsert, dead-letter queue, validator with weight-floor patch)
- **3.C — Neural Recall + Context Injection** — ✅ implemented (`RecallProvider`, 2-layer seed selection, CTE traversal, `<memory-context>` fence matching Python's `build_memory_context_block`)
- **3.D — Semantic Fusion + Local Embeddings** — ✅ implemented (`entity_embeddings` table with L2-normalized float32 LE BLOBs; `Embedder` background worker calls Ollama `/v1/embeddings` with labeled template `Entity: {Name}. Type: {Type}. Context: {Description}`; in-memory vector cache with monotonic graph-version counter; `semanticSeeds` flat cosine scan (dot product on normalized vectors); hybrid fusion in `Provider.GetContext` chains lexical → FTS5 → semantic with dedup + MaxSeeds cap; opt-in via `semantic_enabled=true` + `semantic_model="<tag>"`; empty model is a complete no-op — zero HTTP calls, zero goroutine, zero cache RAM. Ship criterion proven live against Ollama: query `"tell me about my projects"` (no lexical match) surfaces `AzulVigia` via cosine in 7s.)
- **3.D.5 — Memory Mirror (USER.md sync)** — ✅ implemented (async background goroutine exports SQLite entities/rels → Markdown every 30s; configurable path; atomic writes; SQLite remains source of truth; zero impact on 250ms latency moat)
- **3.E — Decay + Cross-Chat + Operational Mirrors** — ⏳ planned (see §7 Phase 3.E Ledger below)

### Phase 2 Ledger

| Subphase | Status | Deliverable |
|---|---|---|
| Phase 2.A — Tool Registry | ✅ complete | In-process Go tool registry, streamed `tool_calls` accumulation, kernel tool loop, and doctor verification |
| Phase 2.B.1 — Telegram Scout | ✅ complete | Telegram adapter over the existing kernel, long-poll ingress, edit coalescing at the messaging edge |
| Phase 2.C — Thin Mapping Persistence | ✅ complete | bbolt-backed `(platform, chat_id) -> session_id` resume; no transcript ownership moved into Go |
| Phase 2.B.2+ — Wider Gateway Surface | ⏳ planned | Additional platform adapters (see §7 Subsystem Inventory for the upstream list of 24 platforms) |
| Phase 2.D — Cron / Scheduled Automations | ⏳ planned | Port `cron/scheduler.py` + `cron/jobs.py` to a Go ticker + bbolt job store; natural-language cron parsing via the brain (Phase 4) once available |
| Phase 2.E — Subagent Delegation | ⏳ planned | Port `tools/delegate_tool.py`: spawn isolated subagents with their own conversations + sandboxed terminals, merge results back to the parent loop |
| Phase 2.F — Hooks + Lifecycle | ⏳ planned | Port `gateway/hooks.py`, `builtin_hooks/`, `restart.py`, `pairing.py`, `status.py`, `mirror.py`, `sticker_cache.py`; per-event extension points and managed restarts |

Phase 2.C is intentionally not Phase 3. It stores only session handles in bbolt. Python still owns transcript memory, transcript search, and prompt assembly; the SQLite + FTS5 memory lattice is Phase 3 (now substantially implemented).

> **Note on binary size:** The static CGO-free binary currently builds at **~17 MB** (measured: `bin/gormes` from `make build` with `-trimpath -ldflags="-s -w"` at commit `4a25542c`, post-3.D). This reflects all Phase 3 additions (extractor, recall, mirror, Embedder, semantic fusion) atop the original TUI + Telegram base. Remains well within the 25 MB hard moat with 8 MB headroom.

### Phase 3.E Ledger

Phase 3.E is the final Black Box milestone. It closes three orthogonal gaps: **memory decay** (old facts fade), **cross-chat synthesis** (one user, multiple chats, one graph), and **operational-visibility mirrors** (session index, insights audit, tool audit, transcript export). Each row is a separable spec.

| Subphase | Status | Upstream reference | Deliverable |
|---|---|---|---|
| 3.E.1 — Session Index Mirror | ⏳ planned | None (Gormes-original) | Read-only YAML mirror of bbolt `sessions.db` at `~/.local/share/gormes/sessions/index.yaml`; closes the bbolt opacity gap |
| 3.E.2 — Tool Execution Audit Log | ⏳ planned | None (exceeds Hermes) | Append-only JSONL at `~/.local/share/gormes/tools/audit.jsonl`; persistent record of every tool call with timing + outcome |
| 3.E.3 — Transcript Export Command | ⏳ planned | Exceeds Hermes (no upstream equivalent) | `gormes session export <id> --format=markdown` renders SQLite turns as human-readable Markdown; snapshot for sharing/backup |
| 3.E.4 — Extraction State Visibility | ⏳ planned | None (debug only) | Optional dead-letter footer in USER.md OR `gormes memory status` command showing extraction queue depth + recent errors |
| 3.E.5 — Insights Audit Log | ⏳ planned | `agent/insights.py` (preview) | Lightweight append-only JSONL at `~/.local/share/gormes/insights/usage.jsonl`; accumulates session counts, token totals, cost estimates per day. Full `InsightsEngine` port lands in 4.E |
| 3.E.6 — Memory Decay | ⏳ planned | None (Gormes-original) | Weight attenuation on relationships + `last_seen` tracking; stale facts age out of recall without deletion (reversible, audit-preserving) |
| 3.E.7 — Cross-Chat Synthesis | ⏳ planned | `agent/memory_manager.py` (cross-session) | Graph unification across `chat_id` boundaries for a single operator; query "what is Juan working on?" returns facts from Telegram, Discord, Slack in one fence. Requires a `user_id` concept above `chat_id` |

The 3.E ship criterion: the operator runs `cat ~/.local/share/gormes/sessions/index.yaml` and sees every active chat/session mapping in plain YAML; runs `cat ~/.local/share/gormes/tools/audit.jsonl` and sees a full history of tool invocations; a fact mentioned once six months ago and never again no longer dominates recall results; and asking the same question across two different chats surfaces the same entity graph.

### Phase 4 Sub-phase Outline

Phase 4 is when Hermes becomes optional. Each sub-phase is a separable spec.

| Subphase | Status | Deliverable |
|---|---|---|
| 4.A — Provider Adapters | ⏳ planned | Native Go adapters for Anthropic, Bedrock, Gemini, OpenRouter, Google Code Assist, Codex (mirrors `agent/{anthropic,bedrock,gemini_cloudcode,openrouter_client,google_code_assist}_adapter.py`) |
| 4.B — Context Engine + Compression | ⏳ planned | Port `agent/{context_engine,context_compressor,context_references}.py`; manage long sessions without blowing the model context window |
| 4.C — Native Prompt Builder | ⏳ planned | Port `agent/prompt_builder.py`; assemble system + memory + tool + history into a model-ready prompt |
| 4.D — Smart Model Routing | ⏳ planned | Port `agent/smart_model_routing.py` + `agent/model_metadata.py` + `agent/models_dev.py`; pick the right model per turn |
| 4.E — Trajectory + Insights | ⏳ planned | Port `agent/trajectory.py` + `agent/insights.py`; self-monitoring telemetry surface |
| 4.F — Title Generation | ⏳ planned | Port `agent/title_generator.py`; auto-name new sessions |
| 4.G — Credentials + OAuth | ⏳ planned | Port `agent/google_oauth.py`, `agent/credential_pool.py`, `tools/credential_files.py`; token vault + multi-account auth |
| 4.H — Rate / Retry / Caching | ⏳ planned | Port `agent/{rate_limit_tracker,retry_utils,nous_rate_guard,prompt_caching}.py`; provider-side resilience |

Once 4.A–4.D are shipped Gormes can call LLMs directly. The `:8642` health check becomes optional.

### Phase 5 Sub-phase Outline

Phase 5 is when Python disappears entirely from the runtime path. Each sub-phase is a separable spec.

| Subphase | Status | Deliverable |
|---|---|---|
| 5.A — Tool Surface Port | ⏳ planned | Port the 61-tool `tools/` registry. Most tools are tractable Go ports; a few (browser, voice) split into 5.C–5.E. |
| 5.B — Sandboxing Backends | ⏳ planned | Port `tools/environments/{local,docker,modal,daytona,singularity}.py` + `file_sync.py`. Five execution backends with namespace isolation and container hardening. |
| 5.C — Browser Automation | ⏳ planned | Port `tools/browser_tool.py` + `tools/browser_camofox*.py` + `tools/browser_providers/{browserbase,browser_use,firecrawl}.py` to Go (Chromedp, Rod) or sidecar process |
| 5.D — Vision + Image Generation | ⏳ planned | Port `tools/vision_tools.py` + `tools/image_generation_tool.py`; multimodal in/out |
| 5.E — TTS / Voice / Transcription | ⏳ planned | Port `tools/{tts_tool,voice_mode,transcription_tools,neutts_synth}.py`; may stay as sidecar processes |
| 5.F — Skills System | ⏳ planned | Port `tools/{skill_manager_tool,skills_hub,skills_sync,skills_tool,skills_guard}.py` + auto-generated skill discovery; the `skills/` directory has 26 categories |
| 5.G — MCP Integration | ⏳ planned | Port `tools/{mcp_tool,mcp_oauth,mcp_oauth_manager,managed_tool_gateway}.py`; Model Context Protocol client + OAuth flows |
| 5.H — ACP Integration | ⏳ planned | Port `acp_adapter/` + `acp_registry/`; Agent Communication Protocol server side |
| 5.I — Plugins Architecture | ⏳ planned | Port `plugins/{context_engine,memory,example-dashboard}` + the plugin SDK; let third parties extend Gormes without forking |
| 5.J — Approval / Security Guards | ⏳ planned | Port `tools/{approval,path_security,url_safety,tirith_security,website_policy}.py`; gate dangerous actions |
| 5.K — Code Execution | ⏳ planned | Port `tools/code_execution_tool.py` + `tools/process_registry.py`; sandboxed exec |
| 5.L — File Ops + Patches | ⏳ planned | Port `tools/{file_operations,file_tools,checkpoint_manager,patch_parser}.py`; file editing with atomic checkpoints |
| 5.M — Mixture of Agents | ⏳ planned | Port `tools/mixture_of_agents_tool.py`; multi-model coordination |
| 5.N — Misc Operator Tools | ⏳ planned | Port `tools/{todo_tool,clarify_tool,session_search_tool,send_message_tool,cronjob_tools,debug_helpers,interrupt}.py` |
| 5.O — Hermes CLI Parity | ⏳ planned | Port `hermes_cli/` (auth, backup, banner, codex_models, etc.); replaces the upstream `hermes` binary |
| 5.P — Docker / Packaging | ⏳ planned | Mirror `Dockerfile` + `docker/` for Gormes; OCI image with same volume layout as upstream |

---

## 5. Project Boundaries

Hard rule: no Python file in this repository is modified. All Gormes work lives under `gormes/`. Upstream rebases against `NousResearch/hermes-agent` cannot conflict with Gormes because paths do not overlap.

The bridge is allowed to exist. The bridge is not allowed to become the destination.

---

## 6. Documentation

This `ARCH_PLAN.md` is the executive roadmap. It defines the strategic conquest of the operational bottleneck: first UI, then gateway, then memory and state, then cognition, then the final removal of Python from the runtime path. Per-milestone specs live at `docs/superpowers/specs/YYYY-MM-DD-*.md`. Per-milestone implementation plans live at `docs/superpowers/plans/YYYY-MM-DD-*.md`.

Public-site (`gormes.io`) deployment is **Phase 1.5** work. The documentation is authored in CommonMark + GFM so every mainstream static-site generator (Hugo, MkDocs Material, Astro Starlight) can render it without rewrites. Phase 1 ships a Goldmark-based validation test — Goldmark is the exact renderer Hugo uses, so passing the test guarantees Hugo-renderability.

**Active spec inventory (2026-04-20):**
- Phase 1: [`superpowers/specs/2026-04-18-gormes-frontend-adapter-design.md`](superpowers/specs/2026-04-18-gormes-frontend-adapter-design.md) — ✅ shipped
- Phase 2.A: [`superpowers/specs/2026-04-19-gormes-phase2-tools-design.md`](superpowers/specs/2026-04-19-gormes-phase2-tools-design.md) — ✅ shipped
- Phase 2.B.1: [`superpowers/specs/2026-04-19-gormes-phase2b-telegram.md`](superpowers/specs/2026-04-19-gormes-phase2b-telegram.md) — ✅ shipped
- Phase 2.C: [`superpowers/specs/2026-04-19-gormes-phase2c-persistence-design.md`](superpowers/specs/2026-04-19-gormes-phase2c-persistence-design.md) — ✅ shipped
- Phase 3.A: [`superpowers/specs/2026-04-20-gormes-phase3a-memory-design.md`](superpowers/specs/2026-04-20-gormes-phase3a-memory-design.md) — ✅ shipped
- Phase 3.B: [`superpowers/specs/2026-04-20-gormes-phase3b-graph.md`](superpowers/specs/2026-04-20-gormes-phase3b-graph.md) — ✅ shipped
- Phase 3.C: [`superpowers/specs/2026-04-20-gormes-phase3c-recall-design.md`](superpowers/specs/2026-04-20-gormes-phase3c-recall-design.md) — ✅ shipped
- Phase 3.D: [`superpowers/specs/2026-04-20-gormes-phase3d-semantic-design.md`](superpowers/specs/2026-04-20-gormes-phase3d-semantic-design.md) + [`superpowers/plans/2026-04-20-gormes-phase3d-semantic.md`](superpowers/plans/2026-04-20-gormes-phase3d-semantic.md) — ✅ shipped (10 TDD tasks across commits `1c859ea6`…`4a25542c`)
- Phase 3.D.5: [`superpowers/specs/2026-04-20-gormes-phase3d5-mirror-design.md`](superpowers/specs/2026-04-20-gormes-phase3d5-mirror-design.md) — ✅ shipped
- Phase 3.E: spec pending — see §4 Phase 3.E Ledger for scope

---

## 7. Upstream Subsystem Inventory

The complete picture of what Gormes must absorb to retire the Python `hermes-agent` runtime. Each row is one upstream module or capability, mapped to its target phase. This inventory is the source of truth for "what's left" — when a subsystem is shipped in Go, mark it ✅ and link the spec.

### Gateway platforms (24 connectors — 23 unshipped)

| Platform | Upstream file | Target phase | Status |
|---|---|---|---|
| Telegram | `gateway/platforms/telegram.py` | 2.B.1 | ✅ shipped |
| Discord | `gateway/platforms/discord.py` | 2.B.2 | ⏳ planned |
| Slack | `gateway/platforms/slack.py` | 2.B.3 | ⏳ planned |
| WhatsApp | `gateway/platforms/whatsapp.py` | 2.B.4 | ⏳ planned |
| Signal | `gateway/platforms/signal.py` | 2.B.5 | ⏳ planned |
| Email | `gateway/platforms/email.py` | 2.B.6 | ⏳ planned |
| SMS | `gateway/platforms/sms.py` | 2.B.7 | ⏳ planned |
| Matrix | `gateway/platforms/matrix.py` | 2.B.8 | ⏳ planned |
| Mattermost | `gateway/platforms/mattermost.py` | 2.B.9 | ⏳ planned |
| Webhook | `gateway/platforms/webhook.py` | 2.B.10 | ⏳ planned |
| BlueBubbles (iMessage) | `gateway/platforms/bluebubbles.py` | 2.B.11 | ⏳ planned |
| HomeAssistant | `gateway/platforms/homeassistant.py` | 2.B.12 | ⏳ planned |
| Feishu | `gateway/platforms/feishu*.py` | 2.B.13 | ⏳ planned |
| WeChat (WeCom + WeiXin) | `gateway/platforms/wecom*.py`, `weixin.py` | 2.B.14 | ⏳ planned |
| DingTalk | `gateway/platforms/dingtalk.py` | 2.B.15 | ⏳ planned |
| QQ Bot | `gateway/platforms/qqbot/` | 2.B.16 | ⏳ planned |

### Operational layer (cross-cutting, mostly Phase 2.D–2.F)

| Subsystem | Upstream | Target phase | Status |
|---|---|---|---|
| Cron / scheduled automations | `cron/scheduler.py`, `cron/jobs.py`, `tools/cronjob_tools.py` | 2.D | ⏳ planned |
| Subagent delegation | `tools/delegate_tool.py` | 2.E | ⏳ planned |
| Hooks system | `gateway/hooks.py`, `gateway/builtin_hooks/` | 2.F | ⏳ planned |
| Restart / pairing / lifecycle | `gateway/{restart,pairing,status}.py` | 2.F | ⏳ planned |
| Mirror / sticker cache | `gateway/{mirror,sticker_cache}.py` | 2.F | ⏳ planned |
| Display config | `gateway/display_config.py`, `agent/display.py` | 2.F | ⏳ planned |

### Memory + state (Phase 3 — 3.A–3.D shipped; 3.E pending)

| Subsystem | Upstream | Target phase | Status |
|---|---|---|---|
| SQLite + FTS5 lattice | `agent/memory_provider.py` (lexical half) | 3.A | ✅ shipped |
| Ontological graph + extractor | `agent/memory_manager.py` | 3.B | ✅ shipped |
| Recall + context injection | `agent/memory_provider.py` (recall half) | 3.C | ✅ shipped |
| Semantic / embeddings | (not in upstream; Gormes-original) | 3.D | ✅ shipped |
| USER.md mirror | `agent/memory_manager.py` (mirror writer) | 3.D.5 | ✅ shipped |
| Session index mirror | None (closes bbolt opacity gap) | 3.E.1 | ⏳ planned |
| Tool execution audit log | None (exceeds Hermes) | 3.E.2 | ⏳ planned |
| Transcript export command | None (exceeds Hermes; Hermes has no text export) | 3.E.3 | ⏳ planned |
| Extraction state visibility | None (debug visibility) | 3.E.4 | ⏳ planned |
| Insights audit log (lightweight) | `agent/insights.py` (preview; full port in 4.E) | 3.E.5 | ⏳ planned |
| Memory decay | None (Gormes-original) | 3.E.6 | ⏳ planned |
| Cross-chat synthesis | `agent/memory_manager.py` (cross-session) | 3.E.7 | ⏳ planned |

### Brain (Phase 4 — sub-phases 4.A–4.H)

| Subsystem | Upstream | Target phase | Status |
|---|---|---|---|
| Anthropic adapter | `agent/anthropic_adapter.py` | 4.A | ⏳ planned |
| Bedrock adapter | `agent/bedrock_adapter.py` | 4.A | ⏳ planned |
| Gemini Cloud Code adapter | `agent/gemini_cloudcode_adapter.py` | 4.A | ⏳ planned |
| OpenRouter client | `agent/openrouter_client.py` | 4.A | ⏳ planned |
| Google Code Assist | `agent/google_code_assist.py` | 4.A | ⏳ planned |
| Copilot ACP client | `agent/copilot_acp_client.py` | 4.A | ⏳ planned |
| Auxiliary client (xAI etc.) | `agent/auxiliary_client.py` + `tools/xai_http.py` | 4.A | ⏳ planned |
| Context engine | `agent/context_engine.py` | 4.B | ⏳ planned |
| Context compressor | `agent/context_compressor.py` + `manual_compression_feedback.py` | 4.B | ⏳ planned |
| Context references | `agent/context_references.py` | 4.B | ⏳ planned |
| Prompt builder | `agent/prompt_builder.py` | 4.C | ⏳ planned |
| Smart model routing | `agent/smart_model_routing.py` + `model_metadata.py` + `models_dev.py` | 4.D | ⏳ planned |
| Trajectory | `agent/trajectory.py` | 4.E | ⏳ planned |
| Insights | `agent/insights.py` | 4.E | ⏳ planned |
| Title generator | `agent/title_generator.py` | 4.F | ⏳ planned |
| Google OAuth | `agent/google_oauth.py` | 4.G | ⏳ planned |
| Credential pool | `agent/credential_pool.py` | 4.G | ⏳ planned |
| Credential files | `tools/credential_files.py` | 4.G | ⏳ planned |
| Rate limit tracker | `agent/rate_limit_tracker.py` + `nous_rate_guard.py` | 4.H | ⏳ planned |
| Retry utils | `agent/retry_utils.py` | 4.H | ⏳ planned |
| Prompt caching | `agent/prompt_caching.py` | 4.H | ⏳ planned |
| Subdirectory hints | `agent/subdirectory_hints.py` | 4.B | ⏳ planned |
| Skill commands / utils | `agent/skill_commands.py`, `agent/skill_utils.py` | 4.C | ⏳ planned |
| Error classifier | `agent/error_classifier.py` | 4.H | ⏳ planned |
| Redaction | `agent/redact.py` | 4.B | ⏳ planned |
| Usage / pricing | `agent/usage_pricing.py` | 4.E | ⏳ planned |

### Tools surface (Phase 5 — 61 upstream tool files)

| Category | Upstream tools | Target phase | Status |
|---|---|---|---|
| Sandboxing backends | `tools/environments/{base,local,docker,modal,managed_modal,modal_utils,daytona,singularity,file_sync}.py` | 5.B | ⏳ planned |
| Browser automation | `tools/browser_tool.py`, `browser_camofox*.py`, `browser_providers/{base,browserbase,browser_use,firecrawl}.py` | 5.C | ⏳ planned |
| Vision | `tools/vision_tools.py` | 5.D | ⏳ planned |
| Image generation | `tools/image_generation_tool.py` | 5.D | ⏳ planned |
| TTS / voice / transcription | `tools/{tts_tool,voice_mode,transcription_tools,neutts_synth}.py` + `neutts_samples/` | 5.E | ⏳ planned |
| Skills system | `tools/{skill_manager_tool,skills_hub,skills_sync,skills_tool,skills_guard}.py`; `skills/` (26 categories) | 5.F | ⏳ planned |
| MCP integration | `tools/{mcp_tool,mcp_oauth,mcp_oauth_manager,managed_tool_gateway}.py` + `mcp_serve.py` | 5.G | ⏳ planned |
| ACP integration | `acp_adapter/`, `acp_registry/` | 5.H | ⏳ planned |
| Plugins architecture | `plugins/{context_engine,memory,example-dashboard}/` + plugin SDK | 5.I | ⏳ planned |
| Approval / security | `tools/{approval,path_security,url_safety,tirith_security,website_policy}.py` | 5.J | ⏳ planned |
| Code execution | `tools/{code_execution_tool,process_registry}.py` | 5.K | ⏳ planned |
| File operations | `tools/{file_operations,file_tools,fuzzy_match,checkpoint_manager,patch_parser,binary_extensions}.py` | 5.L | ⏳ planned |
| Mixture of agents | `tools/mixture_of_agents_tool.py` | 5.M | ⏳ planned |
| Operator tools | `tools/{todo_tool,clarify_tool,session_search_tool,send_message_tool,debug_helpers,interrupt,ansi_strip}.py` | 5.N | ⏳ planned |
| Web tools / search | `tools/web_tools.py` | 5.A | ⏳ planned |
| Terminal tool | `tools/terminal_tool.py` | 5.A | ⏳ planned |
| Send message (cross-platform) | `tools/send_message_tool.py` | 5.N | ⏳ planned |
| Feishu doc/drive tools | `tools/{feishu_doc_tool,feishu_drive_tool}.py` | 5.A | ⏳ planned |
| HomeAssistant tool | `tools/homeassistant_tool.py` | 5.A | ⏳ planned |
| OSV vulnerability check | `tools/osv_check.py` | 5.J | ⏳ planned |
| Budget config | `tools/budget_config.py` + `tool_backend_helpers.py` + `tool_result_storage.py` | 5.A | ⏳ planned |
| Env passthrough | `tools/env_passthrough.py` | 5.B | ⏳ planned |
| RL training tool | `tools/rl_training_tool.py` | 5.M | ⏳ deferred (specialized) |
| Datagen examples | `datagen-config-examples/` | 5.M | ⏳ deferred (specialized) |
| Batch runner | `batch_runner.py` | 5.O | ⏳ planned |
| Mini SWE runner | `mini_swe_runner.py` | 5.O | ⏳ planned (or 5.M) |
| Model tools (admin) | `model_tools.py` | 5.O | ⏳ planned |

### CLI + packaging (Phase 5.O–5.P)

| Subsystem | Upstream | Target phase | Status |
|---|---|---|---|
| Hermes CLI | `hermes_cli/{auth,auth_commands,backup,banner,callbacks,claw,cli_output,clipboard,codex_models,colors,...}.py` | 5.O | ⏳ planned |
| Top-level CLI | `cli.py` | 5.O | ⏳ planned |
| Hermes runtime helpers | `hermes_constants.py`, `hermes_logging.py`, `hermes_state.py`, `hermes_time.py` | 5.O | ⏳ planned |
| Dockerfile / packaging | `Dockerfile`, `docker/`, `packaging/`, `nix/`, `flake.nix` | 5.P | ⏳ planned |
| MANIFEST / constraints | `MANIFEST.in`, `constraints-termux.txt` | 5.P | ⏳ planned |
| Environments + agent loop | `environments/{agent_loop,patches,hermes_base_env,agentic_opd_env}.py`, `tool_call_parsers/` | 5.A | ⏳ planned |
| Benchmarks | `environments/benchmarks/` | 5.M | ⏳ deferred (research) |
| SWE env | `environments/hermes_swe_env/`, `environments/terminal_test_env/` | 5.M | ⏳ deferred (research) |

### Out of scope for the runtime port

These upstream paths exist but are not part of the runtime that Gormes must absorb. Listed for completeness so future contributors don't mistake them for missing work:

- `agent/`, `cli.py`, `gateway/`, `hermes/`, `hermes_cli/`, `tools/`, `cron/`, `acp_adapter/`, `acp_registry/`, `plugins/`, `tests/`, `tui_gateway/` — runtime paths covered by the phases above.
- `docs/` (upstream documentation), `assets/`, `optional-skills/`, `skills/` — content corpus; mirrored separately by docs.gormes.ai (Phase 1.5) and skill packs.
- `package.json`, `package-lock.json`, `nix/`, `flake.lock`, `flake.nix` — build/packaging metadata; partially mirrored at Phase 5.P.
- `tests/` — Python tests are not ported; Gormes has its own Go test suite per spec.
- `AGENTS.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `GOOD-PRACTICES.md`, `hermes-already-has-routines.md` — upstream contributor docs; not runtime.

### Inventory cadence

Re-run the upstream survey when a major Hermes release lands, when a new platform connector is added upstream, or when a Gormes phase ships and we need to mark its rows ✅. The survey is mechanical: `find upstream root -name "*.py" -newer last-survey-date` plus a `gateway/platforms/` directory listing usually catches everything.

---

## 8. Mirror Strategy — Auditability Roadmap

Phase 3.D.5 (Memory Mirror) closes the transparency gap for entities/relationships. Based on comprehensive Hermes parity research, here is the complete mirror strategy.

### 8.1 What Hermes Actually Has vs Gormes

| Data | Hermes Format | Gormes Format | Gap Analysis |
|------|--------------|---------------|--------------|
| **Entities/Relationships** | SQLite + USER.md (text) | SQLite + USER.md (via Mirror) | ✅ **Parity achieved (3.D.5)** |
| **Turns/Transcripts** | SQLite + JSONL | SQLite only | 🟡 Gormes has parity; no text export in either |
| **Sessions** | SQLite (queryable) | bbolt (opaque binary) | 🔴 **Gap: bbolt is human-opaque** |
| **Tool Execution** | SessionDB (persisted) | In-memory only | 🔴 **Gap: no tool audit trail** |
| **Extraction State** | SQLite columns | SQLite columns | 🟡 Invisible in both; parity |
| **Skills** | SKILL.md (text files) | Not implemented | ⏳ Phase 5 |
| **Cron Output** | Markdown files | Not implemented | ⏳ Phase 4 |
| **Config** | YAML | TOML | ✅ Both human-readable |
| **Logs** | Text files (agent.log, etc.) | Text file (gormes.log) | ✅ Parity |

**Key Finding**: Hermes does **not** have human-readable transcript exports. Transcripts live in SQLite/JSONL only. The `export_session()` method returns JSON (machine-readable), not formatted text. **Gormes already exceeds Hermes parity** with the USER.md mirror for memory entities.

### 8.2 Remaining Mirror Candidates (Ranked by Priority)

#### 🔴 High Priority: Session Index Mirror (Phase 3.E.1)

**Problem**: Sessions stored in bbolt (`~/.local/share/gormes/sessions.db`) are opaque. Operators cannot `cat`, `grep`, or audit their session mappings without binary tools.

**Solution**: Mirror the bbolt session map to `~/.local/share/gormes/sessions/index.yaml`:
```yaml
# Auto-generated session index
# This file is a read-only mirror of sessions.db for operator auditability
sessions:
  telegram:123456789: session_abc123
  telegram:987654321: session_def456
updated_at: 2026-04-20T09:30:00Z
```

**Implementation**: Background goroutine (like 3.D.5 Mirror) triggered on session write; atomic temp+rename; 30s sync interval.

**Rationale**: Hermes uses queryable SQLite for sessions; Gormes uses binary bbolt. This provides human-readable session auditability that Hermes has via SQL but Gormes lacks via bbolt opacity.

#### 🟡 Medium Priority: Tool Execution Audit Log (Phase 3.E.2)

**Problem**: Tool calls are ephemeral. The Bear runs `terminal()`, produces output, but no persistent record exists. An operator cannot audit "what did the agent do yesterday?"

**Solution**: Append-only log at `~/.local/share/gormes/tools/audit.logl` (JSONL):
```json
{"ts":"2026-04-20T09:30:00Z","session":"abc123","turn":5,"tool":"terminal","cmd":"ls -la","duration_ms":150,"status":"ok"}
{"ts":"2026-04-20T09:30:05Z","session":"abc123","turn":6,"tool":"web_search","query":"golang embed","results":3,"duration_ms":2500}
```

**Rationale**: This exceeds Hermes capabilities. Python Hermes stores tool results in SessionDB messages table, but there's no separate audit trail for tool execution. This is new operational visibility.

#### 🟡 Medium Priority: Transcript Export Command (Phase 3.E.3)

**Problem**: While Hermes has no human-readable transcript export, operators may want to export a conversation for sharing, backup, or analysis.

**Solution**: Add `gormes session export <session_id> --format=markdown` command that renders:
```markdown
# Session session_abc123 (2026-04-20)

## Turn 1
**User**: Hello Bear

**Assistant**: Hello! How can I help?

## Turn 2
**User**: What's my name?
...memory-context appears here...

**Assistant**: You're the user who...

---
*Exported from Gormes on 2026-04-20T09:30:00Z*
```

**Rationale**: This is a **Gormes-only feature** that exceeds Hermes capabilities. Hermes has no equivalent human-readable export.

#### 🟢 Low Priority: Extraction State Visibility (Phase 3.E.4)

**Problem**: `turns.extracted`, `extraction_attempts`, `extraction_error` columns are invisible to operators. A dead-lettered turn (`extracted=2`) requires SQLite inspection.

**Solution**: Optional: add extraction failures to the USER.md mirror footer, or provide `gormes memory status` command showing extraction queue depth and recent errors.

**Rationale**: This is debugging/operational visibility. Can be deferred until extraction issues become painful.

### 8.3 Hermes Files Gormes Does Not Need to Mirror (Yet)

Based on the comprehensive Hermes file inventory, these Hermes files do not need Gormes mirrors today, but may become relevant as features land:

| Hermes File | Why Not Mirrored in Gormes | Future Consideration |
|-------------|---------------------------|-------------------|
| `MEMORY.md` | Superseded by USER.md + entity graph (structured > flat) | N/A — entity graph is superior |
| `sessions.json` | Legacy Hermes format; Gormes uses bbolt (better concurrency) | **Session Index Mirror (3.E.1)** closes bbolt opacity |
| `*.jsonl` transcripts | Machine-readable only | **Transcript Export (3.E.3)** adds human-readable option |
| `jobs.json` + cron output | Cron not yet implemented in Gormes (Phase 4) | Cron output mirroring when Phase 4 lands |
| `SKILL.md` files | Skills not yet implemented (Phase 5) | Skill audit trail when Phase 5 lands |
| `HOOK.yaml` | Hook system not yet implemented (Phase 2.F) | Hook activity log when hooks land |
| `BOOT.md` | Boot hooks not yet implemented | Boot sequence audit when Phase 2.F lands |
| `SOUL.md` | Personality system not yet implemented (Phase 4+) | Persona versioning when Phase 4 lands |
| `gateway_voice_mode.json` | Voice mode not implemented (Phase 5.E) | Voice state mirroring if voice features land |
| Platform state JSON files | Platform adapters not yet implemented (Phase 2.B.2+) | Per-platform state audit when platforms land |

**Operational State Files Discovered in Additional Research:**

| Hermes File | Purpose | Gormes Status |
|-------------|---------|---------------|
| `gateway_voice_mode.json` | Per-chat voice mode state (off/voice_only/all) | Not implemented (Phase 5.E) |
| `display_config` (in config.yaml) | Per-platform display settings | Partial — TUI theme only |
| `active_profile` | Currently active profile name | Not implemented |
| `channel_directory.json` | Cached channel/contact mappings | Not implemented |
| `pairing.json` | Device/pairing state per platform | Not implemented (Phase 2.F) |

**Additional Subsystems with Audit Potential:**

| Hermes Subsystem | Data Produced | Mirror Potential | Phase |
|------------------|---------------|------------------|-------|
| `agent/insights.py` | Usage analytics (tokens, costs, trends, tool patterns) | 🔴 **High** — Operators need visibility into spend and usage patterns | 4.E |
| `agent/trajectory.py` | RL training trajectories (JSONL) | 🟡 Medium — Machine-readable; research use case | 4.E |
| `agent/usage_pricing.py` | Per-request cost calculations | 🔴 **High** — Cost audit trail for operational monitoring | 4.E |

**Insights Engine Gap**: Hermes has a comprehensive `InsightsEngine` (`agent/insights.py`, 768 lines) that analyzes historical session data to produce:
- Token consumption reports
- Cost estimates by model/provider
- Tool usage patterns
- Activity trends over time
- Platform breakdowns
- Session metrics (duration, turns, success rate)

Gormes currently has only basic in-memory telemetry (`internal/telemetry/telemetry.go`) that does not persist. **This is a significant operational visibility gap** — operators cannot audit their usage, costs, or trends without reimplementing the insights analysis themselves.

**Recommended Mirror Addition — Phase 3.E.5: Insights Audit Log**

Export aggregated session metrics to `~/.local/share/gormes/insights/usage.jsonl`:
```json
{"date":"2026-04-20","session_count":5,"total_tokens_in":45000,"total_tokens_out":12000,"estimated_cost_usd":0.45,"model_breakdown":{"claude-opus":3,"gpt-4":2}}
```

This would provide a lightweight, append-only cost and usage audit trail that accumulates over time, even before the full InsightsEngine is ported in Phase 4.E.

### 8.4 Mirror Implementation Principles

All mirrors must follow the 3.D.5 design constraints:
1. **Source of truth remains database** — mirrors are read-only exports
2. **Fire-and-forget** — never block the 250ms kernel latency budget
3. **Atomic writes** — temp file + rename (readers never see partial files)
4. **Change detection** — hash comparison to avoid redundant writes
5. **Graceful degradation** — log warnings on errors, never crash the bot
6. **Configurable paths** — respect XDG directories and config overrides

---

*Mirror Strategy v1.0 — Synthesized from parallel audit of Hermes Python codebase and Gormes Go implementation.*

---

## 9. Technology Radar — Package & Tool Research

Continuous research into the Go ecosystem for Gormes-relevant packages, techniques, and upstream developments.

### 9.1 Vector Embedding Libraries (Phase 3.D Research — 2026-04-20)

Evaluated pure-Go vector databases for semantic recall layer:

| Library | License | Storage | Index | Size Impact | Notes |
|---------|---------|---------|-------|-------------|-------|
| **[chromem-go](https://github.com/TIANLI0/chromem-go)** | Apache-2.0 | In-memory + optional persist | HNSW, IVF, PQ, BM25, hybrid | ~200KB | Zero third-party deps; SIMD on amd64; BM25 for lexical+semantic fusion |
| **[veclite](https://github.com/abdul-hamid-achik/veclite)** | MIT | Single `.veclite` file | HNSW + BM25 | ~150KB | Zero deps (stdlib only); auto-embedding with Ollama/OpenAI; single-file portability |
| **[vecgo](https://github.com/hupe1980/vecgo)** | Apache-2.0 | Commit-oriented durability | HNSW + DiskANN/Vamana | ~300KB | Production-focused; 16-way sharded HNSW; arena allocator; PQ/RaBitQ quantization |
| **[govector](https://github.com/DotNetAge/govector)** | MIT | bbolt + Protobuf | HNSW | ~250KB | "SQLite for Vectors"; Qdrant-compatible API; uses `github.com/coder/hnsw` |
| **[goformersearch](https://github.com/MichaelAyles/goformersearch)** | MIT | In-memory | Brute-force + HNSW | ~100KB | Minimal surface; designed for 10k-50k docs at 384d; single-core optimized |

**Recommendation**: **chromem-go** or **veclite** for Phase 3.D. Both offer:
- Pure Go (CGO-free, static binary compatible)
- HNSW for O(log n) ANN search
- BM25 for hybrid lexical+semantic search
- Zero additional dependencies
- MIT/Apache-2.0 licenses (compatible with Gormes)

**Ollama Integration**: Ollama supports OpenAI-compatible `/v1/embeddings` endpoint ([docs](https://ollama.readthedocs.io/en/openai/)). Go client libraries: [`go-embeddings`](https://github.com/milosgajdos/go-embeddings) (multi-provider, includes Ollama), [`go-ollama`](https://github.com/eslider/go-ollama) (streaming support).

### 9.2 SQLite Driver Landscape

Current: `github.com/ncruces/go-sqlite3` (WASM-based, CGO-free)

Alternatives monitored:
- `modernc.org/sqlite` (C-to-Go transpiled, larger binary impact)
- `github.com/mattn/go-sqlite3` (CGO, not static-binary friendly)

**Status**: ncruces driver remains optimal for CGO-free static builds.

### 9.3 Upstream Hermes-Agent Tracking

**Repository**: https://github.com/NousResearch/hermes-agent  
**License**: MIT (compatible)  
**Porting Strategy**: Strangler Fig — Gormes phases gradually subsume Python subsystems (§1 Rosetta Stone)

**Recent upstream additions to monitor** (inventory from parallel codebase audit):
- Gateway platforms: 24 adapters including Telegram, Discord, Slack, WhatsApp, Signal, Email, SMS, Feishu, Matrix, Weixin, BlueBubbles, QQ
- RL training environments (`environments/`)
- ACP adapter for IDE integration (`acp_adapter/`)
- Honcho dialectic user modeling integration
- Skills Hub registries (skills.sh, clawhub, lobehub, hermes-index)

**Cadence**: Re-run upstream survey on major Hermes releases or when new platform connectors land. See §7 Subsystem Inventory for complete upstream file mapping.

---

*Technology Radar v1.0 — Research synthesized from web searches and parallel codebase audit.*

---

## 10. Executive Summary — Mirror Implementation Status

This section provides a quick-reference dashboard of all mirror-related work: what's shipped, what's planned, and what was researched.

### Shipped Mirrors + Recall Layers (✅)

| Phase | Name | What It Does | Where to Find It |
|-------|------|--------------|------------------|
| 3.A | **SQLite + FTS5 Lattice** | Transcript memory with full-text search | `internal/memory/{memory,schema,migrate}.go` |
| 3.B | **Ontological Graph Extractor** | Async LLM-assisted entity/relationship extraction | `internal/memory/{extractor,graph,validator}.go` |
| 3.C | **Neural Recall** | Lexical + FTS5 seeds → CTE neighborhood → `<memory-context>` fence | `internal/memory/{recall,recall_format,recall_sql}.go` |
| 3.D | **Semantic Fusion** | Ollama embeddings + cosine similarity; closes the "my projects" gap | `internal/memory/{embed_client,cosine,semantic_sql,embedder}.go` |
| 3.D.5 | **Memory Mirror** | Exports SQLite entities/relationships → `~/.local/share/gormes/memory/USER.md` | `internal/memory/mirror.go` |

### Planned Mirrors + 3.E Roadmap (Ranked)

| Priority | Phase | Name | Problem Solved | Effort |
|----------|-------|------|----------------|--------|
| 🔴 High | 3.E.1 | Session Index Mirror | bbolt session map is opaque; needs human-readable YAML index | ~150 lines |
| 🔴 High | 3.E.5 | Insights Audit Log | No usage/cost analytics; operators cannot audit spend | ~100 lines |
| 🔴 High | 3.E.6 | Memory Decay | Stale facts dominate recall; need weight attenuation + `last_seen` | ~200 lines |
| 🟡 Medium | 3.E.2 | Tool Audit Log | No record of what tools the agent executed | ~100 lines |
| 🟡 Medium | 3.E.3 | Transcript Export | No human-readable conversation export (exceeds Hermes) | ~200 lines |
| 🟡 Medium | 3.E.7 | Cross-Chat Synthesis | One user with N chats has N disjoint graphs; needs `user_id` above `chat_id` | ~400 lines |
| 🟢 Low | 3.E.4 | Extraction State | Dead-lettered turns invisible without SQLite query | ~50 lines |

### Research Synthesized

**Parallel Agent Research Conducted:**
1. ✅ Hermes human-readable file inventory (complete — 13 categories of files)
2. ✅ Gormes binary-only data audit (complete — 6 gaps identified)
3. ✅ Hermes transcript handling analysis (complete — NO text export exists)
4. ✅ Hermes skill storage research (complete — SKILL.md format, Hub lock file)
5. ✅ **ADDITIONAL**: Hermes insights/telemetry systems discovered (`agent/insights.py` 768 lines, `agent/trajectory.py`, `agent/usage_pricing.py`)

**Package Research Conducted:**
1. ✅ Vector embedding libraries (5 evaluated — chromem-go/veclite recommended)
2. ✅ Ollama integration (OpenAI-compatible `/v1/embeddings` confirmed)
3. ✅ Go SQLite driver landscape (ncruces remains optimal)

### Key Findings Documented

**Critical Insight**: Hermes does **not** have human-readable transcript exports. All conversation history is in SQLite/JSONL. **Gormes already exceeds Hermes parity** with the Memory Mirror (3.D.5).

**Insights Engine Discovery**: Hermes has a comprehensive `InsightsEngine` (`agent/insights.py`, 768 lines) that produces usage analytics, cost estimates, and trend reports. **Gormes currently lacks any persisted telemetry** — only in-memory counters that vanish on restart. This is a 🔴 **high-priority operational visibility gap**.

**Binary Size**: Currently ~17 MB (CGO-free). 25 MB hard moat leaves 8 MB headroom. Phase 3.D semantic embeddings add <250 KB.

**Vector Library Recommendation**: **chromem-go** or **veclite** for Phase 3.D — both pure Go, zero deps, HNSW+BM25 hybrid search.

### Next Actions (Suggested)

Phase 3.D shipped `4a25542c` on 2026-04-20. Remaining Phase 3 work:

1. **3.E.6 Memory Decay** — weight attenuation + `last_seen` tracking; prevents stale-fact dominance in recall. Roughly ~200 lines in `internal/memory/decay.go` + a periodic goroutine.
2. **3.E.1 Session Index Mirror** — ~150 lines in `internal/session/mirror.go`; closes bbolt opacity. Low-risk parity with 3.D.5 implementation pattern.
3. **3.E.7 Cross-Chat Synthesis** — requires a `user_id` concept above `chat_id`; graph unification so one operator sees one unified memory across platforms. Biggest design spike remaining in Phase 3.
4. **3.E.5 Insights Audit Log** — lightweight JSONL preview while the full `InsightsEngine` port waits for 4.E.
5. **3.E.2 Tool Audit Log + 3.E.3 Transcript Export** — operational niceties; ship after the three above.

After Phase 3.E is complete, the next strategic pivot is **Phase 4 — The Powertrain**: native Go provider adapters (4.A), context engine (4.B), prompt builder (4.C), smart routing (4.D), and the full InsightsEngine port (4.E). Phase 4 is when the Hermes `:8642` health check becomes optional.

---

*ARCH_PLAN.md is the living document. Update this summary as mirrors ship and research expands.*
