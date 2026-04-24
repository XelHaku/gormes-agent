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
| 4.A — Provider Adapters | ◌ in_progress | Anthropic plus the shared provider transcript/tool-continuation harness are landed. Bedrock remains unlanded on main and is now split into request mapping, stream decoding, SigV4/credential seam, and stale-client eviction/retry classification. Codex remains unlanded and is split into pure Responses conversion, OAuth stale-token relogin, and stream/tool-call repair. OpenRouter is tracked against the real auxiliary/model/pricing/runtime-provider surfaces, not a nonexistent `agent/openrouter_client.py`. New upstream tool-call argument repair/schema-sanitizer regressions are now a shared provider-boundary slice before more adapters land |
| 4.B — Context Engine + Compression | ⏳ planned | Port `agent/{context_engine,context_compressor,context_references}.py`; execute as smaller slices: interface/status contract, token-budget trigger, tool-result pruning with protected head/tail summary, then manual feedback/context references |
| 4.C — Native Prompt Builder | ⏳ planned | Port `agent/prompt_builder.py`; execute as smaller slices: context-file discovery and injection scan, model-specific role/tool guidance, toolset-aware skills prompt snapshots, and memory/session-search guidance assembly |
| 4.D — Smart Model Routing | ⏳ planned | Model metadata, the pure routing/fallback selector, and per-turn model selection are all planned. Per-turn selection was previously marked complete in error: only the `PlatformEvent.Model` struct field is on main — the wiring (`runTurn` plumbing, `hermes.ChatRequest.Model` per-turn override, `model_selection.go`/`ModelSelector`) lives only on an unmerged `codexu/*` worker branch. Re-land as one small TDD slice. |
| 4.E — Trajectory + Insights | ⏳ planned | Port the opt-in trajectory writer with redaction gates, then bridge native runtime metrics into the existing Phase 3 insights rollup |
| 4.F — Title Generation | ⏳ planned | Freeze title prompt/truncation behavior before wiring auto-naming into session persistence |
| 4.G — Credentials + OAuth | ⏳ planned | Port XDG-scoped token storage, credential-pool selection, and Google OAuth refresh/device-browser flows before provider adapters consume secrets |
| 4.H — Rate / Retry / Caching | ◌ in_progress | Jittered reconnect backoff, Retry-After parsing on `HTTPError`, kernel consumption of capped provider hints, and structured provider-error taxonomy are landed. Remaining slices are now prompt-cache capability guards plus provider rate/budget telemetry; Bedrock stale transport recovery is tracked under 4.A because it depends on the Bedrock client seam |

Once 4.A–4.D are shipped Gormes can call LLMs directly. The `:8642` health check becomes optional.

Current state: the first provider-native adapter now lives in `internal/hermes`, and the shared provider seam is no longer hypothetical. `internal/hermes/client.go` freezes the common ChatRequest/Message/Event/ToolCall contracts, and Anthropic (`anthropic_client.go` + `anthropic_stream.go` + `anthropic_client_test.go`) ships direct Messages API request shaping for cache-control metadata, streamed tool-use delta accumulation, stop-reason mapping, and 429 handling. The shared transcript and tool-continuation fixtures are complete; use them for every new adapter. Bedrock is **not yet landed on main** — no `bedrock_*.go` file is tracked in `internal/hermes/` — and the latest upstream Bedrock changes add stale transport-client eviction on top of request/stream/credential behavior, so the ledger now keeps those as four separate slices. Codex is also **not landed on main**; upstream extracted pure Responses conversion and added stream/tool-call repair behavior, so the ledger now splits Codex into conversion, auth, and repair slices. Phase 4.H retry behavior is ahead of the old prose: provider HTTP failures classify retryable vs fatal auth/context/rate-limit signals, `HTTPError` carries capped Retry-After hints, and the kernel consumes those hints during open-stream retry. Smart Model Routing has NOT started on main: `internal/kernel/frame.go:84` advertises a `PlatformEvent.Model` field, but `internal/kernel/kernel.go:179` calls `runTurn` without `e.Model` and the `hermes.ChatRequest` at `internal/kernel/kernel.go:296` is built with `Model: k.cfg.Model`, so the override is dropped on every turn.

## Build Priority Context

Phase 4 is **optimization**, not **differentiation**. The Python bridge works. Replace it only after the OS-AI spine and the wider gateway surface prove the architecture is correct. The current dependency chain is:

> 2.E0 deterministic subagent runtime → 2.G static skills + reviewed candidate flow → runner-enforced delegation policy + wider gateway surface → native agent loop

**The rule:** stabilize the runtime substrate first, then add explicit skills and the reviewed skill flow, then harden delegation policy, then widen adapters, and only then replace the Python bridge.

## Hermes Brain Lessons Now Imported

Phase 4 should port Hermes's contracts, not `run_agent.py`.

The native brain work must keep these boundaries explicit:

- **Stable prompt assembly:** identity, memory, skills, context files,
  platform hints, model/provider guidance, and timestamp metadata are stable
  system layers. Turn-local memory/plugin recall is injected ephemerally into
  the current user message so prompt-cache prefixes stay stable.
- **Provider fixture contract:** every provider adapter must replay the same
  transcript fixtures for tool calls, reasoning deltas, finish reasons, usage,
  retryable errors, and tool-result continuations before it can be used by the
  kernel.
- **Context engine contract:** compression is its own engine, not a hidden
  kernel side effect. It must preserve head/tail invariants, keep tool
  call/result pairs together, expose status, record lineage, and replay from
  fixtures.
- **Kernel restraint:** the kernel owns turn state, cancellation, tool
  continuation, retry orchestration, and finalization. Provider quirks, prompt
  source discovery, compression policy, and memory-provider lifecycle stay in
  their packages.

The first sign of Phase 4 going wrong is a Go `run_agent.py`: one class-shaped
file where provider, prompt, compression, memory, gateway callbacks, and tools
all merge. The desired shape is smaller contracts with transcript and snapshot
tests.

## TDD Handoff Notes

Phase 4 should not start with "port `run_agent.py`." The next execution agents should close the partially landed contract work with small, test-first slices that do not require live model credentials:

1. **4.A Bedrock payload mapping (no AWS SDK)** — port only the Converse request shaping and canonical Message→Bedrock tool-aware mapping with pure fixtures, proving the 5 prior codexu/* attempts can be replaced by a tightly scoped slice.
2. **4.A tool-call argument repair + schema sanitizer** — port upstream repair/sanitizer tests as a shared provider boundary before Codex/OpenRouter/Bedrock adapter work executes malformed tool calls.
3. **4.A Codex Responses pure conversion harness** — port `agent/codex_responses_adapter.py` as request/response conversion fixtures only; no OAuth, no live backend, no `~/.codex` access.
4. **4.A cross-provider reasoning/`<think>`-tag sanitization** — port upstream reasoning-tag strip behavior as one shared storage-vs-render contract before reasoning-model adapters multiply.
5. **4.A Bedrock stream event decoding (SSE fixtures)** — decode reasoning/text/tool-use deltas into the shared `hermes.Event` model from recorded SSE fixtures, without binding the real AWS SDK yet.
6. **4.A Bedrock minimal SigV4 + credential seam** — land credential discovery and signing behind a small dep-isolated helper so regional/error handling can follow without dragging a client cache into kernel paths.
7. **4.A Bedrock stale-client eviction + retry classification** — after the client seam exists, evict only stale transport clients and keep validation/auth failures visible as provider errors.
8. **4.A Codex OAuth state + stale-token relogin** — once the 4.G vault exists, persist Codex state under Gormes home and make stale refresh failures require explicit relogin.
9. **4.A Codex stream repair + tool-call leak sanitizer** — after pure conversion, prove empty Responses output, leaked `to=functions.*` text, and malformed streamed arguments cannot corrupt history.
10. **4.D model metadata registry** — context limits, pricing, capability flags, and provider-family facts in a read-only registry.
11. **4.D routing selector** — a pure fallback/override selector over the metadata registry before any automatic model choice is allowed.
12. **4.B tool-result pruning + protected head/tail summary** — freeze old tool-output pruning and head/tail invariants, and upstream PR #3128d9fc — tool-call arguments must remain JSON-valid after shrinking so continuation payloads do not break.
13. **4.B context-engine status contract** — `get_status`, context-window updates, token-budget trigger, and compression cooldown behavior once the provider seam is frozen.
14. **4.C prompt-builder context-file discovery** — SOUL/HERMES/AGENTS/CLAUDE ordering, truncation, frontmatter stripping, and prompt-injection scans after the context-engine contract is pinned.

Only after the Bedrock request, stream, credential, and stale-client slices are green should more provider-specific Bedrock work land. This keeps retry, role mapping, and tool-call continuation bugs visible instead of hiding them behind a large native-agent-loop rewrite.
