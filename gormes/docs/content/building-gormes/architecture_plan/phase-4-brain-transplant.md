---
title: "Phase 4 — The Brain Transplant"
weight: 50
---

# Phase 4 — The Brain Transplant (Powertrain)

**Status:** ◌ in_progress

**Deliverable:** Native Go agent orchestrator + prompt builder.

Phase 4 is when Hermes becomes optional. Each sub-phase is a separable spec.

## Phase 4 Sub-phase Outline

| Subphase | Status | Deliverable |
|---|---|---|
| 4.A — Provider Adapters | ◌ in_progress | Anthropic + Bedrock are landed on a shared provider seam; the remaining closeout is a reusable stream/tool-continuation fixture harness, then Gemini, OpenRouter, Google Code Assist, and Codex over that same contract |
| 4.B — Context Engine + Compression | ⏳ planned | Port `agent/{context_engine,context_compressor,context_references}.py`; execute as smaller slices: interface/status contract, token-budget trigger, tool-result pruning with protected head/tail summary, then manual feedback/context references |
| 4.C — Native Prompt Builder | ⏳ planned | Port `agent/prompt_builder.py`; execute as smaller slices: context-file discovery and injection scan, model-specific role/tool guidance, toolset-aware skills prompt snapshots, and memory/session-search guidance assembly |
| 4.D — Smart Model Routing | ◌ in_progress | Model metadata and the pure routing/fallback selector remain planned; the kernel now supports turn-scoped model overrides that persist across tool iterations without mutating the default model |
| 4.E — Trajectory + Insights | ⏳ planned | Port the opt-in trajectory writer with redaction gates, then bridge native runtime metrics into the existing Phase 3 insights rollup |
| 4.F — Title Generation | ⏳ planned | Freeze title prompt/truncation behavior before wiring auto-naming into session persistence |
| 4.G — Credentials + OAuth | ⏳ planned | Port XDG-scoped token storage, credential-pool selection, and Google OAuth refresh/device-browser flows before provider adapters consume secrets |
| 4.H — Rate / Retry / Caching | ◌ in_progress | Provider-side resilience plus jittered retry/backoff are landed; the remaining slices are the richer structured error taxonomy, prompt-cache capability guards, and provider rate/budget telemetry |

Once 4.A–4.D are shipped Gormes can call LLMs directly. The `:8642` health check becomes optional.

Current state: the first two provider-native adapters now live in `internal/hermes`, and the shared provider seam is no longer hypothetical. `internal/hermes/client.go` already freezes the common ChatRequest/Message/Event/ToolCall contracts that Anthropic and Bedrock both exercise. Anthropic ships direct Messages API request shaping for cache-control metadata, streamed tool-use delta accumulation, stop-reason mapping, and 429 handling. Bedrock now ships a native ConverseStream transport that reuses the same message/tool contract, resolves AWS region defaults, delegates credential loading and SigV4 signing to the AWS SDK, and maps Bedrock stream/error envelopes into the shared event model. The remaining 4.A closeout is smaller now: extract a reusable transcript fixture harness and lock cross-provider tool-continuation behavior before Gemini/OpenRouter/Codex land. Phase 4.H has also moved beyond a stub: provider HTTP failures classify retryable/fatal auth/context/rate-limit signals, Retry-After hints parse from headers/body, delays are capped, and kernel reconnect retries already use the 1s/2s/4s/8s/16s budget with +/-20% jitter. Smart Model Routing has also started: `internal/kernel` now accepts a turn-scoped model override, carries it across tool iterations, and surfaces the active model in render frames and telemetry without mutating the kernel default. The remaining 4.D work is the metadata registry and pure selector.

## Build Priority Context

Phase 4 is **optimization**, not **differentiation**. The Python bridge works. Replace it only after the OS-AI spine and the wider gateway surface prove the architecture is correct. The current dependency chain is:

> 2.E0 deterministic subagent runtime → 2.G static skills + reviewed candidate flow → runner-enforced delegation policy + wider gateway surface → native agent loop

**The rule:** stabilize the runtime substrate first, then add explicit skills and the reviewed skill flow, then harden delegation policy, then widen adapters, and only then replace the Python bridge.

## TDD Handoff Notes

Phase 4 should not start with "port `run_agent.py`." The next execution agents should close the partially landed contract work with small, test-first slices that do not require live model credentials:

1. **4.A shared transcript fixture harness closeout** — harvest Anthropic + Bedrock request/stream transcripts into reusable fixtures that assert usage, finish reasons, and shared event decoding.
2. **4.A cross-provider tool-continuation fixtures** — pin assistant tool-call messages, streamed tool-call deltas, and tool-result continuation payloads before Gemini/OpenRouter/Codex reuse the seam.
3. **4.H structured error-taxonomy closeout** — extend the current retryable/fatal classification into explicit rate-limit/auth/context/non-retryable envelopes without regressing the shipped retry path.
4. **4.D model metadata registry** — context limits, pricing, capability flags, and provider-family facts in a read-only registry.
5. **4.D routing selector** — a pure fallback/override selector over the metadata registry before any automatic model choice is allowed.
6. **4.B context-engine status contract** — `get_status`, context-window updates, token-budget trigger, and compression cooldown behavior once the provider seam is frozen.
7. **4.C prompt-builder context-file discovery** — SOUL/HERMES/AGENTS/CLAUDE ordering, truncation, frontmatter stripping, and prompt-injection scans after the context-engine contract is pinned.

Only after the first five slices are green should more provider-specific adapters land. This keeps retry, role mapping, and tool-call continuation bugs visible instead of hiding them behind a large native-agent-loop rewrite.
