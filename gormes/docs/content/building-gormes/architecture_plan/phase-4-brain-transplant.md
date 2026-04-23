---
title: "Phase 4 — The Brain Transplant"
weight: 50
---

# Phase 4 — The Brain Transplant (Powertrain)

**Status:** ⏳ planned

**Deliverable:** Native Go agent orchestrator + prompt builder.

Phase 4 is when Hermes becomes optional. Each sub-phase is a separable spec.

## Phase 4 Sub-phase Outline

| Subphase | Status | Deliverable |
|---|---|---|
| 4.A — Provider Adapters | ⏳ planned | Native Go adapters for Anthropic, Bedrock, Gemini, OpenRouter, Google Code Assist, Codex (mirrors `agent/{anthropic,bedrock,gemini_cloudcode,openrouter_client,google_code_assist}_adapter.py`) |
| 4.B — Context Engine + Compression | ⏳ planned | Port `agent/{context_engine,context_compressor,context_references}.py`; execute as smaller slices: interface/status contract, token-budget trigger, tool-result pruning with protected head/tail summary, then manual feedback/context references |
| 4.C — Native Prompt Builder | ⏳ planned | Port `agent/prompt_builder.py`; execute as smaller slices: context-file discovery and injection scan, model-specific role/tool guidance, toolset-aware skills prompt snapshots, and memory/session-search guidance assembly |
| 4.D — Smart Model Routing | ⏳ planned | Port `agent/smart_model_routing.py` + `agent/model_metadata.py` + `agent/models_dev.py`; pick the right model per turn |
| 4.E — Trajectory + Insights | ⏳ planned | Port `agent/trajectory.py` + `agent/insights.py`; self-monitoring telemetry surface |
| 4.F — Title Generation | ⏳ planned | Port `agent/title_generator.py`; auto-name new sessions |
| 4.G — Credentials + OAuth | ⏳ planned | Port `agent/google_oauth.py`, `agent/credential_pool.py`, `tools/credential_files.py`; token vault + multi-account auth |
| 4.H — Rate / Retry / Caching | ⏳ planned | Port `agent/{rate_limit_tracker,retry_utils,nous_rate_guard,prompt_caching}.py`; execute as classified error taxonomy, Retry-After/jittered backoff, rate guard, and prompt-cache capability slices |

Once 4.A–4.D are shipped Gormes can call LLMs directly. The `:8642` health check becomes optional.

## Build Priority Context

Phase 4 is **optimization**, not **differentiation**. The Python bridge works. Replace it only after the OS-AI spine and the wider gateway surface prove the architecture is correct. The current dependency chain is:

> 2.E0 deterministic subagent runtime → 2.G static skills + reviewed candidate flow → runner-enforced delegation policy + wider gateway surface → native agent loop

**The rule:** stabilize the runtime substrate first, then add explicit skills and the reviewed skill flow, then harden delegation policy, then widen adapters, and only then replace the Python bridge.

## TDD Handoff Notes

Phase 4 should not start with "port `run_agent.py`." The next execution agents should first freeze provider-independent contracts: context-engine status, prompt-builder context-file discovery, and provider error classification. Those tests can run without real model credentials and reduce the risk that native provider work hides prompt-cache or compression regressions.
