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
| 4.D — Smart Model Routing | ✅ complete | `internal/kernel/model_routing.go` now applies conservative same-provider per-turn model selection via `[hermes.smart_routing]`, and the kernel carries that effective model through request assembly, telemetry, and render frames |
| 4.E — Trajectory + Insights | ✅ complete | `internal/telemetry` now tracks per-session turn outcomes plus tool execution totals/failures/cancellations, and the TUI renders that live self-monitoring surface for later trajectory/insights work |
| 4.F — Title Generation | ✅ complete | `internal/session/title.go` now derives deterministic first-exchange titles, and the gateway/TUI persistence paths store them in session metadata plus the audit mirror |
| 4.G — Credentials + OAuth | 🔨 in progress | Named multi-account selection plus the XDG token vault are live; interactive OAuth browser/login flow still remains |
| 4.H — Rate / Retry / Caching | ⏳ planned | Port `agent/{rate_limit_tracker,retry_utils,nous_rate_guard,prompt_caching}.py`; provider-side resilience |

Once 4.A–4.D are shipped Gormes can call LLMs directly. The `:8642` health check becomes optional.

Phase 4.A is now complete. Anthropic, Bedrock, Gemini, OpenRouter, Codex, and Google Code Assist all have native provider paths in `internal/hermes.NewClient`. Each adapter preserves the kernel's canonical `ChatRequest`/`Event` contract while speaking the provider's native wire format: Anthropic via Messages, Bedrock via ConverseStream, Gemini via `streamGenerateContent`, OpenRouter via `/api/v1/chat/completions` plus `/api/v1/models`, Codex via Responses, and Google Code Assist via `cloudcode-pa`'s wrapped `streamGenerateContent` plus `loadCodeAssist` health checks. The new Google Code Assist path intentionally stops at the provider-adapter seam for Phase 4.A: it accepts an already-issued bearer token, resolves optional project IDs from environment overrides, translates canonical tool-aware history through the Gemini request mapper, and leaves the OAuth/browser login port to Phase 4.G.

4.B is now started as well: `internal/contextengine/compressor_budget.go` ports the donor compressor's threshold floor, summary-budget sizing, context-length probe step-down, and anti-thrashing cooldown into a pure Go slice with fixed-token tests, without wiring live history mutation into the kernel yet.

4.C is now complete as well: `internal/kernel/prompt_builder.go` lifts prompt assembly into a dedicated helper that prepends per-turn system blocks (session context, recall output, and any skill block), preserves accumulated multi-turn history, and carries the registered tool surface into `hermes.ChatRequest`. `internal/kernel/prompt_builder_test.go` locks the missing second-turn behavior so follow-up turns keep prior user/assistant context instead of collapsing to the latest message only.

4.D is now complete as well: `internal/kernel/model_routing.go` ports a conservative slice of the donor smart-routing behavior by switching short, non-code, non-tool-heavy turns onto an optional `simple_model` configured under `[hermes.smart_routing]`. `internal/kernel/kernel.go` sets that effective model per turn before the stream opens, `internal/kernel/prompt_builder.go` forwards it into `hermes.ChatRequest`, and the same selected model flows through telemetry plus `RenderFrame` so the TUI shows what the turn actually used. `internal/kernel/model_routing_test.go` locks the heuristic and the per-turn reset-to-primary behavior, while `internal/config/config_test.go` proves the TOML surface that enables it. The broader donor metadata catalogs remain tracked separately in the subsystem inventory.

4.E is now complete as well: `internal/telemetry` now records per-session turn outcomes, total tool executions, failed/cancelled tool calls, and the last completed turn state alongside the existing token/latency counters. `internal/kernel` wires those counters into live render frames and `internal/tui/view.go` surfaces them in the sidebar, giving operators a built-in self-monitoring view before the historical `trajectory.py` / `insights.py` donor ports land.

4.F is now complete as well: `internal/session/title.go` ports the donor title-generator intent as a deterministic first-exchange summarizer, so the opening user/assistant pair auto-names a new session without needing an auxiliary model. `internal/gateway/manager.go` and `cmd/gormes/main.go` run that helper when a turn settles to idle, `internal/session/directory.go` persists the title alongside the existing session metadata, and `internal/session/index_mirror.go` includes the title in the YAML audit mirror.

4.G is now further along as well: `internal/config/config.go` still resolves a named `[hermes].account` / `[[hermes.accounts]]` selection into the active provider, endpoint, model, and API key before the existing env/flag overrides apply, and `GORMES_ACCOUNT` can switch accounts per run without rewriting `config.toml`. On top of that, `internal/config/token_vault.go` now loads persisted credentials from `${XDG_DATA_HOME}/gormes/auth.json` plus provider token files under `${XDG_DATA_HOME}/gormes/auth/*.json` before env/flag overrides, so previously-issued bearer/API tokens can reach the live provider adapters without being duplicated in `config.toml`. `internal/config/config_test.go` locks auth-vault fallback, provider-file precedence, and the XDG path helpers, while `cmd/gormes/llm_client_test.go` proves a vault-loaded token actually reaches the provider health check path. The remaining 4.G work is the interactive OAuth browser/login flow and refresh lifecycle.

## Build Priority Context

Phase 4 is **optimization**, not **differentiation**. The Python bridge works. Replace it only after the OS-AI spine and the wider gateway surface prove the architecture is correct. The current dependency chain is:

> 2.E0 deterministic subagent runtime → 2.G static skills + reviewed candidate flow → runner-enforced delegation policy + wider gateway surface → native agent loop

**The rule:** stabilize the runtime substrate first, then add explicit skills and the reviewed skill flow, then harden delegation policy, then widen adapters, and only then replace the Python bridge.
