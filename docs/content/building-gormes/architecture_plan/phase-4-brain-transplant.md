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
| 4.A — Provider Adapters | ◌ in_progress | Anthropic plus the shared provider transcript/tool-continuation harness are landed. Bedrock is partial: pure Converse request mapping is in `internal/hermes/bedrock_converse.go`, while stream decoding, SigV4/credential seam, and stale-client eviction remain dependency-ordered rows. Codex pure Responses conversion and stream repair are landed, but Hermes `648b8991` adds a missing assistant `output_text` content-part fixture before OAuth/live Codex work. OpenRouter is tracked against the real auxiliary/model/pricing/runtime-provider surfaces, not a nonexistent `agent/openrouter_client.py`. Hermes `f93d4624` adds a DeepSeek/Kimi replay-ordering regression fix so Gormes must prevent prior-provider generic reasoning from being forwarded as `reasoning_content`. New upstream tool-call argument repair/schema-sanitizer regressions are now a shared provider-boundary slice before more adapters land |
| 4.B — Context Engine + Compression | ⏳ planned | The ContextEngine status boundary is landed. The pure compressor budget and provider-cap slices are mirrored, but Hermes `5006b220` removed `flush_memories` and restored auxiliary compression to a single-prompt threshold model; reconcile that before real history pruning, summarization feedback, context references, and tool-result protection |
| 4.C — Native Prompt Builder | ⏳ planned | Port `agent/prompt_builder.py`; execute as smaller slices: context-file discovery and injection scan, model-specific role/tool guidance, toolset-aware skills prompt snapshots, and memory/session-search guidance assembly |
| 4.D — Smart Model Routing | ◌ in_progress | Provider-enforced context limits, read-only model pricing/capability fixtures, the pure routing/fallback selector, and per-turn kernel model override are complete. The umbrella remains planned only as inventory; future 4.D rows must be integration or operator-status slices, not another metadata/routing bulk port |
| 4.E — Trajectory + Insights | ⏳ planned | Port the opt-in trajectory writer with redaction gates, then bridge native runtime metrics into the existing Phase 3 insights rollup |
| 4.F — Title Generation | ⏳ planned | Freeze title prompt/truncation behavior before wiring auto-naming into session persistence |
| 4.G — Credentials + OAuth | ⏳ planned | Port XDG-scoped token storage, credential-pool selection, and Google OAuth refresh/device-browser flows before provider adapters consume secrets |
| 4.H — Rate / Retry / Caching | ◌ in_progress | Jittered reconnect backoff, Retry-After parsing on `HTTPError`, kernel consumption of capped provider hints, structured provider-error taxonomy, generic unsupported-parameter retry, and Codex no-temperature cleanup are landed. Hermes `7c17accb` adds a cancellation-before-retry regression that Gormes should freeze as a small kernel fixture. Prompt-cache guards plus provider rate/budget telemetry remain |

Once 4.A–4.D are shipped Gormes can call LLMs directly. The `:8642` health check becomes optional.

Current state: the first provider-native adapter now lives in `internal/hermes`, and the shared provider seam is no longer hypothetical. `internal/hermes/client.go` freezes the common ChatRequest/Message/Event/ToolCall contracts, and Anthropic (`anthropic_client.go` + `anthropic_stream.go` + `anthropic_client_test.go`) ships direct Messages API request shaping for cache-control metadata, streamed tool-use delta accumulation, stop-reason mapping, and 429 handling. The shared transcript and tool-continuation fixtures are complete; use them for every new adapter. Bedrock request mapping is now landed in `internal/hermes/bedrock_converse.go` and `internal/hermes/bedrock_converse_mapping_test.go`; remaining Bedrock work is stream event decoding, deterministic SigV4/credential evidence, and stale transport-client eviction, kept as three small dependency-ordered rows. Codex pure conversion and stream repair are landed, but Hermes `648b8991` shows Gormes still needs a role-specific content-part fixture so assistant replay list content uses `output_text` before Codex auth/live backend work starts. DeepSeek/Kimi reasoning echo padding is landed, but Hermes `f93d4624` shows the ordering is not fully current: if an assistant tool-call turn has generic stored `Reasoning` but no explicit `ReasoningContent`, DeepSeek/Kimi replay must send `reasoning_content=""` rather than forwarding another provider's reasoning text. Phase 4.H retry behavior is ahead of the old prose: provider HTTP failures classify retryable vs fatal auth/context/rate-limit signals, `HTTPError` carries capped Retry-After hints, the kernel consumes those hints during open-stream retry, generic unsupported-parameter retry is validated, and the next drift row should prove cancellation suppresses a fresh retry after `/stop`. The ContextEngine status boundary, pure compressor budget behavior, and provider-cap lookup are complete, but Gormes still has the legacy auxiliary headroom helper from the now-superseded flush-memory path; reconcile that before real pruning/summarization/context-reference work. Smart Model Routing is now fixture-backed on main: `internal/hermes/model_context_resolver.go`, `internal/hermes/model_registry.go`, `internal/hermes/model_routing.go`, `internal/kernel/frame.go`, `internal/kernel/kernel.go`, and `internal/kernel/per_turn_model_test.go` prove provider caps, metadata, pure fallback selection, and turn-scoped model overrides without provider calls. Remaining 4.D work should expose/integrate those decisions, not rebuild the selector.

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

1. **4.A Bedrock stream event decoding (SSE fixtures)** — next Bedrock row: decode reasoning/text/tool-use deltas into the shared `hermes.Event` model from synthetic ConverseStream fixtures, without binding the real AWS SDK yet.
2. **4.A tool-call argument repair + schema sanitizer** — port upstream repair/sanitizer tests as a shared provider boundary before Codex/OpenRouter/Bedrock adapter work executes malformed tool calls.
3. **4.A Codex Responses pure conversion harness** — port `agent/codex_responses_adapter.py` as request/response conversion fixtures only; no OAuth, no live backend, no `~/.codex` access.
4. **4.A DeepSeek/Kimi cross-provider reasoning isolation** — mirror Hermes `f93d4624` by proving DeepSeek/Kimi tool-call replay sends an empty `reasoning_content` placeholder before any generic stored `Reasoning` can leak across providers.
5. **4.A cross-provider reasoning/`<think>`-tag sanitization** — port upstream reasoning-tag strip behavior as one shared storage-vs-render contract before reasoning-model adapters multiply.
6. **4.A Bedrock minimal SigV4 + credential seam** — after stream decoding, land credential-source classification and deterministic request signing behind a small dep-isolated helper so regional/error handling can follow without dragging a client cache into kernel paths.
7. **4.A Bedrock stale-client eviction + retry classification** — after the client seam exists, evict only stale transport clients and keep validation/auth failures visible as provider errors.
8. **4.A Codex Responses assistant content role types** — update the pure converter so user text parts remain `input_text` while assistant replay text parts become `output_text`; no OAuth or live Responses request.
9. **4.A Codex OAuth state + stale-token relogin** — once the 4.G vault exists and the role-content fixture is green, persist Codex state under Gormes home and make stale refresh failures require explicit relogin.
10. **4.A Codex stream repair + tool-call leak sanitizer** — after pure conversion, prove empty Responses output, leaked `to=functions.*` text, and malformed streamed arguments cannot corrupt history.
11. **4.B aux compression single-prompt threshold reconciliation** — update the Go budget/status fixtures for Hermes `5006b220`: compression auxiliary requests no longer reserve system/tool/flush headroom because the summarizer sends a single user-role prompt with no tools.
12. **4.H streaming interrupt retry suppression** — prove a cancelled stream cannot open a fresh retry connection, while the normal no-cancel retry path still works.
13. **4.H Codex Responses temperature guard after flush removal** — keep the no-temperature transport invariant, but remove obsolete `flush_memories` fixture names and source references after upstream deleted that caller.
14. **4.D integration/status surface only** — provider caps, metadata registry, pure routing selector, and per-turn override are already complete; any new 4.D row must wire those results into operator-facing selection/status without changing the selector contract.
15. **4.B tool-result pruning + protected head/tail summary** — freeze old tool-output pruning and head/tail invariants, and upstream PR #3128d9fc - tool-call arguments must remain JSON-valid after shrinking so continuation payloads do not break.
16. **4.C prompt-builder context-file discovery** — SOUL/HERMES/AGENTS/CLAUDE ordering, truncation, frontmatter stripping, and prompt-injection scans after the context-engine contract is pinned.

Only after the Bedrock request, stream, credential, and stale-client slices are green should more provider-specific Bedrock work land. This keeps retry, role mapping, and tool-call continuation bugs visible instead of hiding them behind a large native-agent-loop rewrite.
