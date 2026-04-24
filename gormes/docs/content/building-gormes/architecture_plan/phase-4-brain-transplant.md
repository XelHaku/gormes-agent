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
| 4.A — Provider Adapters | ◌ in_progress | Anthropic is landed on the shared provider seam; Bedrock is fully planned (none of the five codexu/* worker attempts on 2026-04-23 merged to main) and now tracked as three dependency-ordered TDD slices in the ledger — payload mapping, stream decoding, SigV4/credential seam. OpenRouter and Codex are also not landed on main — another five codexu/* worker attempts on 2026-04-23 for OpenRouter and four for Codex all failed to merge, and the OpenRouter slice note previously cited a nonexistent `agent/openrouter_client.py`; both notes now enumerate the real upstream surfaces (OpenRouter is routed through `agent/auxiliary_client.py` + `agent/model_metadata.py` + `agent/usage_pricing.py` + `hermes_cli/runtime_provider.py`, Codex wraps the Responses API via `agent/auxiliary_client.py:483 CodexAuxiliaryClient` + `hermes_cli/auth.py` token refresh + `hermes_cli/codex_models.py`). Gemini and Google Code Assist remain single planned slices over the same contract. The cross-provider reasoning/`<think>`-tag sanitization contract (upstream PRs #ec48ec55, #9489d157, #bd01ec78) is tracked as a separate TDD slice so every future provider inherits the same storage-vs-render behavior instead of re-implementing it |
| 4.B — Context Engine + Compression | ⏳ planned | Port `agent/{context_engine,context_compressor,context_references}.py`; execute as smaller slices: interface/status contract, token-budget trigger, tool-result pruning with protected head/tail summary, then manual feedback/context references |
| 4.C — Native Prompt Builder | ⏳ planned | Port `agent/prompt_builder.py`; execute as smaller slices: context-file discovery and injection scan, model-specific role/tool guidance, toolset-aware skills prompt snapshots, and memory/session-search guidance assembly |
| 4.D — Smart Model Routing | ⏳ planned | Model metadata, the pure routing/fallback selector, and per-turn model selection are all planned. Per-turn selection was previously marked complete in error: only the `PlatformEvent.Model` struct field is on main — the wiring (`runTurn` plumbing, `hermes.ChatRequest.Model` per-turn override, `model_selection.go`/`ModelSelector`) lives only on an unmerged `codexu/*` worker branch. Re-land as one small TDD slice. |
| 4.E — Trajectory + Insights | ⏳ planned | Port the opt-in trajectory writer with redaction gates, then bridge native runtime metrics into the existing Phase 3 insights rollup |
| 4.F — Title Generation | ⏳ planned | Freeze title prompt/truncation behavior before wiring auto-naming into session persistence |
| 4.G — Credentials + OAuth | ⏳ planned | Port XDG-scoped token storage, credential-pool selection, and Google OAuth refresh/device-browser flows before provider adapters consume secrets |
| 4.H — Rate / Retry / Caching | ◌ in_progress | Jittered reconnect backoff (1s/2s/4s/8s/16s +/-20%) is landed and wired into the kernel; Retry-After header parsing on `HTTPError` and kernel consumption of the provider hint are still planned, so the retry half is partial. Remaining slices: Retry-After header parsing + HTTPError hint, kernel retry honors Retry-After hint, richer structured error taxonomy, prompt-cache capability guards, and provider rate/budget telemetry |

Once 4.A–4.D are shipped Gormes can call LLMs directly. The `:8642` health check becomes optional.

Current state: the first provider-native adapter now lives in `internal/hermes`, and the shared provider seam is no longer hypothetical. `internal/hermes/client.go` freezes the common ChatRequest/Message/Event/ToolCall contracts, and Anthropic (`anthropic_client.go` + `anthropic_stream.go` + `anthropic_client_test.go`) ships direct Messages API request shaping for cache-control metadata, streamed tool-use delta accumulation, stop-reason mapping, and 429 handling. Bedrock is **not yet landed on main** — five codexu/* worker attempts on 2026-04-23 produced candidate adapters (`feat(phase4): add bedrock provider adapter` x2, `feat(provider): add bedrock client`, `feat(provider): add native Bedrock adapter`, `feat(gormes): add native bedrock provider adapter`) but none merged, and no `bedrock_*.go` file is tracked in `internal/hermes/`. The remaining 4.A closeout is: extract a reusable transcript fixture harness and lock cross-provider tool-continuation behavior over the Anthropic adapter first, then land Bedrock as three small TDD slices (payload mapping, stream event decoding, minimal SigV4/credential seam) before Gemini/OpenRouter/Codex. Phase 4.H is partial: provider HTTP failures classify retryable vs fatal auth/context/rate-limit signals via `internal/hermes/errors.go` and the kernel reconnect budget applies 1s/2s/4s/8s/16s +/-20% jitter; Retry-After header/body parsing is **not** yet landed — HTTPError currently exposes only `{Status, Body}`, so provider hints cannot cap kernel retry delays. Smart Model Routing has NOT started on main: `internal/kernel/frame.go:84` advertises a `PlatformEvent.Model` field, but `internal/kernel/kernel.go:179` calls `runTurn` without `e.Model` and the `hermes.ChatRequest` at `internal/kernel/kernel.go:296` is built with `Model: k.cfg.Model`, so the override is dropped on every turn. The companion `internal/kernel/model_selection.go` and `Config.ModelSelector` field referenced by the prior planner note live only on the unmerged `codexu/20260423T112611Z-3659166-014/worker4` branch. The remaining 4.D work is therefore (1) the metadata registry, (2) the pure routing/fallback selector, and (3) re-landing per-turn model selection as one small TDD slice that wires `e.Model` through `runTurn` into the `ChatRequest` build site without re-introducing `Config.ModelSelector` (which belongs in the selector slice).

## Build Priority Context

Phase 4 is **optimization**, not **differentiation**. The Python bridge works. Replace it only after the OS-AI spine and the wider gateway surface prove the architecture is correct. The current dependency chain is:

> 2.E0 deterministic subagent runtime → 2.G static skills + reviewed candidate flow → runner-enforced delegation policy + wider gateway surface → native agent loop

**The rule:** stabilize the runtime substrate first, then add explicit skills and the reviewed skill flow, then harden delegation policy, then widen adapters, and only then replace the Python bridge.

## TDD Handoff Notes

Phase 4 should not start with "port `run_agent.py`." The next execution agents should close the partially landed contract work with small, test-first slices that do not require live model credentials:

1. **4.A shared transcript fixture harness closeout** — harvest Anthropic request/stream transcripts into reusable fixtures that assert usage, finish reasons, and shared event decoding, so Bedrock/Gemini/OpenRouter/Codex can be added against one replayable contract. Must include regression coverage for upstream PR #12072 (dropped tool_call on mid-stream stall — partial `pendingCalls` must surface on stream EOF, not be silently dropped) and PR #0f778f77 (tool-name duplication in the streaming accumulator from MiniMax/NVIDIA NIM-style providers).
2. **4.A cross-provider tool-continuation fixtures** — pin assistant tool-call messages, streamed tool-call deltas, and tool-result continuation payloads against the Anthropic adapter before adding a second provider to the harness. Same stall/accumulator regression cases as above must carry through so no adapter reintroduces them.
3. **4.A cross-provider reasoning/`<think>`-tag sanitization** — port upstream PRs #ec48ec55, #9489d157, and #bd01ec78 as one shared sanitization contract over `internal/hermes` event/assistant-content boundaries. Cover closed `<think>...</think>` blocks, unterminated `<think>...` at stream EOF, and mixed reasoning-tag variants (`<think>`, `<reasoning>`, `<thought>`). The contract must separate what is stored into assistant history, what is rendered to operators, and what `/resume`-style recap surfaces expose, so every future adapter inherits one consistent behavior.
4. **4.H Retry-After header parsing + HTTPError hint** — add Retry-After header (integer-seconds and HTTP-date) plus optional JSON body-hint parsing to `internal/hermes/errors.go` and surface a capped hint on `HTTPError`. Pure unit tests over table-driven fixtures; do **not** couple to the kernel in this slice.
5. **4.H Kernel retry honors Retry-After hint** — once `HTTPError` carries a capped hint, make `internal/kernel` open-stream retry prefer the provider hint over the jittered schedule when present and fall back cleanly when absent; do **not** widen the already-green `Jittered reconnect backoff schedule`.
6. **4.A Bedrock payload mapping (no AWS SDK)** — port only the Converse request shaping and canonical Message→Bedrock tool-aware mapping with pure fixtures, proving the 5 prior codexu/* attempts can be replaced by a tightly scoped slice.
7. **4.A Bedrock stream event decoding (SSE fixtures)** — decode reasoning/text/tool-use deltas into the shared `hermes.Event` model from recorded SSE fixtures, without binding the real AWS SDK yet.
8. **4.A Bedrock minimal SigV4 + credential seam** — land credential discovery and signing behind a small dep-isolated helper so regional/error handling can follow without dragging the full AWS SDK into kernel paths.
9. **4.H structured error-taxonomy closeout** — extend the current retryable/fatal classification into explicit rate-limit/auth/context/non-retryable envelopes without regressing the shipped retry path.
10. **4.D model metadata registry** — context limits, pricing, capability flags, and provider-family facts in a read-only registry.
11. **4.D routing selector** — a pure fallback/override selector over the metadata registry before any automatic model choice is allowed.
12. **4.B tool-result pruning + protected head/tail summary** — freeze old tool-output pruning and head/tail invariants, and upstream PR #3128d9fc — tool-call arguments must remain JSON-valid after shrinking so continuation payloads do not break.
13. **4.B context-engine status contract** — `get_status`, context-window updates, token-budget trigger, and compression cooldown behavior once the provider seam is frozen.
14. **4.C prompt-builder context-file discovery** — SOUL/HERMES/AGENTS/CLAUDE ordering, truncation, frontmatter stripping, and prompt-injection scans after the context-engine contract is pinned.

Only after the Bedrock slices (6–8) are green should more provider-specific adapters land. This keeps retry, role mapping, and tool-call continuation bugs visible instead of hiding them behind a large native-agent-loop rewrite.
