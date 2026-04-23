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
| 4.A — Provider Adapters | 🔨 in progress | Native Go adapters for Anthropic, Bedrock, Gemini, OpenRouter, Google Code Assist, Codex (mirrors `agent/{anthropic,bedrock,gemini_cloudcode,openrouter_client,google_code_assist}_adapter.py`) |
| 4.B — Context Engine + Compression | ⏳ planned | Port `agent/{context_engine,context_compressor,context_references}.py`; manage long sessions without blowing the model context window |
| 4.C — Native Prompt Builder | ⏳ planned | Port `agent/prompt_builder.py`; assemble system + memory + tool + history into a model-ready prompt |
| 4.D — Smart Model Routing | ⏳ planned | Port `agent/smart_model_routing.py` + `agent/model_metadata.py` + `agent/models_dev.py`; pick the right model per turn |
| 4.E — Trajectory + Insights | ⏳ planned | Port `agent/trajectory.py` + `agent/insights.py`; self-monitoring telemetry surface |
| 4.F — Title Generation | ⏳ planned | Port `agent/title_generator.py`; auto-name new sessions |
| 4.G — Credentials + OAuth | ⏳ planned | Port `agent/google_oauth.py`, `agent/credential_pool.py`, `tools/credential_files.py`; token vault + multi-account auth |
| 4.H — Rate / Retry / Caching | ⏳ planned | Port `agent/{rate_limit_tracker,retry_utils,nous_rate_guard,prompt_caching}.py`; provider-side resilience |

Once 4.A–4.D are shipped Gormes can call LLMs directly. The `:8642` health check becomes optional.

Anthropic, Codex, and Bedrock are now the first native provider paths to land. `internal/hermes.NewClient` can route directly to the Anthropic Messages API, the OpenAI Responses API, or Amazon Bedrock's ConverseStream API while preserving the kernel's canonical `ChatRequest`/`Event` contract. The Bedrock path resolves AWS region defaults, relies on the AWS SDK for credential loading and SigV4 signing, translates canonical system/user/assistant/tool history into Bedrock's tool-aware conversation payload, and maps streamed reasoning/text/tool-use deltas plus Bedrock error envelopes back into the same canonical event/error surface.

## Build Priority Context

Phase 4 is **optimization**, not **differentiation**. The Python bridge works. Replace it only after the OS-AI spine and the wider gateway surface prove the architecture is correct. The current dependency chain is:

> 2.E0 deterministic subagent runtime → 2.G static skills + reviewed candidate flow → runner-enforced delegation policy + wider gateway surface → native agent loop

**The rule:** stabilize the runtime substrate first, then add explicit skills and the reviewed skill flow, then harden delegation policy, then widen adapters, and only then replace the Python bridge.
