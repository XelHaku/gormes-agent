---
title: "Phase 4 — The Brain Transplant"
weight: 50
---

# Phase 4 — The Brain Transplant (Powertrain)

**Status:** 🔨 in progress

**Deliverable:** Native Go agent orchestrator + prompt builder.

Phase 4 is when Hermes becomes optional. Each sub-phase is a separable spec.

## Phase 4 Sub-phase Outline

| Subphase | Status | Deliverable |
|---|---|---|
| 4.A — Provider Adapters | ✅ complete | Native Go adapters for Anthropic, Bedrock, Gemini, OpenRouter, Google Code Assist, Codex (mirrors the upstream provider-adapter surfaces plus `tools/openrouter_client.py`) |
| 4.B — Context Engine + Compression | ✅ complete | `internal/contextengine` now owns provider-free budget/status planning while `internal/kernel` trims old history on the request path, preserving system context and newest turn groups before provider calls |
| 4.C — Native Prompt Builder | ✅ complete | `internal/kernel/prompt_builder.go` now assembles session context + recall output + skill blocks ahead of accumulated history and tool descriptors inside `hermes.ChatRequest` |
| 4.D — Smart Model Routing | ⏳ planned | Port `agent/smart_model_routing.py` + `agent/model_metadata.py` + `agent/models_dev.py`; pick the right model per turn |
| 4.E — Trajectory + Insights | ✅ complete | `internal/telemetry` now tracks per-session turn outcomes plus tool execution totals/failures/cancellations, and the TUI renders that live self-monitoring surface for later trajectory/insights work |
| 4.F — Title Generation | ⏳ planned | Port `agent/title_generator.py`; auto-name new sessions |
| 4.G — Credentials + OAuth | ⏳ planned | Port `agent/google_oauth.py`, `agent/credential_pool.py`, `tools/credential_files.py`; token vault + multi-account auth |
| 4.H — Rate / Retry / Caching | ⏳ planned | Port `agent/{rate_limit_tracker,retry_utils,nous_rate_guard,prompt_caching}.py`; provider-side resilience |

Once 4.A–4.D are shipped Gormes can call LLMs directly. The `:8642` health check becomes optional.

Phase 4.A is now complete. Anthropic, Bedrock, Gemini, OpenRouter, Codex, and Google Code Assist all have native provider paths in `internal/hermes.NewClient`. Each adapter preserves the kernel's canonical `ChatRequest`/`Event` contract while speaking the provider's native wire format: Anthropic via Messages, Bedrock via ConverseStream, Gemini via `streamGenerateContent`, OpenRouter via `/api/v1/chat/completions` plus `/api/v1/models`, Codex via Responses, and Google Code Assist via `cloudcode-pa`'s wrapped `streamGenerateContent` plus `loadCodeAssist` health checks. The new Google Code Assist path intentionally stops at the provider-adapter seam for Phase 4.A: it accepts an already-issued bearer token, resolves optional project IDs from environment overrides, translates canonical tool-aware history through the Gemini request mapper, and leaves the OAuth/browser login port to Phase 4.G.

4.B is now started as well: `internal/contextengine/compressor_budget.go` ports the donor compressor's threshold floor, summary-budget sizing, context-length probe step-down, and anti-thrashing cooldown into a pure Go slice with fixed-token tests, without wiring live history mutation into the kernel yet.

4.C is now complete as well: `internal/kernel/prompt_builder.go` lifts prompt assembly into a dedicated helper that prepends per-turn system blocks (session context, recall output, and any skill block), preserves accumulated multi-turn history, and carries the registered tool surface into `hermes.ChatRequest`. `internal/kernel/prompt_builder_test.go` locks the missing second-turn behavior so follow-up turns keep prior user/assistant context instead of collapsing to the latest message only.

4.E is now complete as well: `internal/telemetry` now records per-session turn outcomes, total tool executions, failed/cancelled tool calls, and the last completed turn state alongside the existing token/latency counters. `internal/kernel` wires those counters into live render frames and `internal/tui/view.go` surfaces them in the sidebar, giving operators a built-in self-monitoring view before the historical `trajectory.py` / `insights.py` donor ports land.

## Build Priority Context

Phase 4 is **optimization**, not **differentiation**. The Python bridge works. Replace it only after the OS-AI spine and the wider gateway surface prove the architecture is correct. The current dependency chain is:

> 2.E0 deterministic subagent runtime → 2.G static skills + reviewed candidate flow → runner-enforced delegation policy + wider gateway surface → native agent loop

**The rule:** stabilize the runtime substrate first, then add explicit skills and the reviewed skill flow, then harden delegation policy, then widen adapters, and only then replace the Python bridge.
