---
title: "Contract Readiness"
weight: 30
---

# Contract Readiness

Building Gormes now treats `docs/content/building-gormes/architecture_plan/progress.json`
as the canonical roadmap. Priority roadmap items can carry optional contract
metadata:

- `contract`
- `contract_status`
- `trust_class`
- `degraded_mode`
- `fixture`
- `source_refs`
- `blocked_by`
- `unblocks`
- `acceptance`

Those fields turn upstream study into execution rules. A subsystem is not ready
for implementation just because a donor file exists. It is ready when the
contract is named, the allowed caller class is explicit, degraded mode is
operator-visible, and a local fixture proves compatibility.

## Current Contract Rows

<!-- PROGRESS:START kind=contract-readiness -->
| Phase | Progress item | Contract status | Owner | Size | Trust class | Fixture | Degraded mode |
|---|---|---|---|---|---|---|---|
| 1 / 1.C | Orchestrator failure-row stabilization for 4-8 workers — Worker verification and failure-taxonomy contract | `validated` | `orchestrator` | `large` | operator, system | `scripts/orchestrator/tests/unit/{failures,soft-success}.bats` | The orchestrator records precise failure reasons, poisoned-task thresholds, soft-success decisions, and original exit codes instead of collapsing failures into one generic status. |
| 1 / 1.C | Soft-success-nonzero bats coverage — Soft-success nonzero recovery guard | `validated` | `orchestrator` | `small` | operator, system | `scripts/orchestrator/tests/unit/soft-success.bats` | When the recovery guard refuses a non-zero exit, the worker state keeps the original exit reason and does not promote the run. |
| 2 / 2.B.4 | WhatsApp identity resolution + self-chat guard — WhatsApp bot identity, self-chat suppression, and peer mapping stay deterministic across bridge and native runtimes | `draft` | `gateway` | `small` | gateway, operator | `internal/channels/whatsapp identity fixtures` | Gateway status reports unresolved WhatsApp bot identity instead of accepting self-chat loops or ambiguous peer mappings. |
| 2 / 2.F.3 | Unauthorized DM pairing response contract — Unknown direct-message users receive the configured deny, pair, or ignore response without leaking authorized-session state | `draft` | `gateway` | `small` | gateway, operator | `internal/gateway unauthorized DM pairing fixtures` | Gateway status and logs show denied or pending-pair users without starting agent sessions. |
| 2 / 2.F.5 | Steer slash command registry + queue fallback — Registry-owned active-turn steering command | `draft` | `gateway` | `small` | operator, gateway | `internal/gateway active-turn command registry fixtures` | Gateway returns visible usage, busy, or queued status instead of dropping steer text when the command cannot run immediately. |
| 3 / 3.E.7 | Interrupted-turn memory sync suppression — Interrupted or cancelled turns cannot flush partial observations into GONCHO or external Honcho-compatible memory | `fixture_ready` | `memory` | `small` | system | `internal/memory/interrupted_sync_test.go` | Memory status reports skipped or interrupted sync attempts without promoting partial facts to recall. |
| 3 / 3.E.7 | Honcho-compatible scope/source tool schema — Honcho-compatible tool schemas expose GONCHO scope and source allowlist controls without renaming public tools | `fixture_ready` | `memory` | `small` | operator, system | `internal/tools/honcho_tools_test.go` | Memory status and tool schema evidence show when user-scope or source-filtered recall is unavailable. |
| 3 / 3.E.7 | Honcho host integration compatibility fixtures — GONCHO preserves host-facing Honcho session, peer, and tool semantics needed by current OpenCode and SillyTavern integrations | `draft` | `memory` | `small` | operator, system | `internal/goncho host integration mapping fixtures` | Doctor and memory status explain which Honcho-compatible host mappings are unsupported instead of silently accepting incompatible config. |
| 3 / 3.E.7 | Cross-chat deny-path fixtures — Same-chat default recall with explicit user-scope widening | `draft` | `memory` | `small` | operator, system | `internal/memory cross-chat allow-deny recall fixtures` | Memory status and operator evidence report unresolved, conflicting, or denied cross-chat identity bindings. |
| 4 / 4.A | Provider interface + stream fixture harness — Provider-neutral request and stream event transcript harness | `validated` | `provider` | `medium` | system | `internal/hermes provider transcript fixtures` | Provider status reports missing fixture coverage or unavailable adapters before kernel routing can select them. |
| 4 / 4.A | Tool-call normalization + continuation contract — Cross-provider tool-call continuation contract | `validated` | `provider` | `medium` | system | `internal/hermes cross-provider tool continuation fixtures` | Provider status reports transcript or continuation fixture gaps before adapters can be selected for tool-capable turns. |
| 4 / 4.A | Bedrock Converse payload mapping (no AWS SDK) — Pure Bedrock Converse request mapping over the shared provider message/tool contract | `fixture_ready` | `provider` | `small` | system | `internal/hermes/bedrock_converse_mapping_test.go` | Provider status reports Bedrock as unavailable until request mapping fixtures pass and credential wiring lands. |
| 4 / 4.A | Bedrock stale-client eviction + retry classification — Bedrock runtime clients evict stale transport state without hiding request or validation failures | `draft` | `provider` | `small` | system | `internal/hermes/bedrock_stale_client_test.go` | Provider logs and status distinguish stale transport recovery from non-retryable Bedrock request failures. |
| 4 / 4.A | Codex Responses pure conversion harness — OpenAI Responses request/response conversion for Codex-compatible providers without live OAuth | `fixture_ready` | `provider` | `small` | system | `internal/hermes/codex_responses_adapter_test.go` | Provider status reports Codex unavailable until Responses conversion fixtures pass and auth wiring is configured. |
| 4 / 4.A | Codex OAuth state + stale-token relogin — Codex OAuth state is Gormes-owned and stale refresh failures force explicit relogin | `draft` | `provider` | `small` | operator, system | `internal/hermes/codex_oauth_state_test.go` | Auth status explains missing, stale, imported, or relogin-required Codex credentials without touching ~/.codex. |
| 4 / 4.A | Codex stream repair + tool-call leak sanitizer — Codex Responses streams repair empty output and reject leaked function-call text before parent history is updated | `draft` | `provider` | `small` | system | `internal/hermes/codex_stream_repair_test.go` | Provider logs explain repaired empty output, leaked tool-call text, and unsupported Codex stream items. |
| 4 / 4.A | Tool-call argument repair + schema sanitizer — Provider tool-call arguments are repaired or rejected against available tool schemas before execution | `fixture_ready` | `provider` | `small` | system, child-agent | `internal/hermes/tool_call_argument_repair_test.go` | Tool execution status reports schema-repair failures before a malformed provider call reaches the executor. |
| 4 / 4.B | ContextEngine interface + status tool contract — Stable context engine status and compression boundary | `draft` | `provider` | `medium` | operator, system | `internal/contextengine status and compression replay fixtures` | Context status reports disabled compression, cooldowns, unknown tools, token-budget pressure, and replay gaps. |
| 4 / 4.G | Anthropic OAuth/keychain credential discovery — Anthropic credential discovery prefers OS keychain when present and preserves corrupt local auth state for operator recovery | `draft` | `provider` | `small` | operator, system | `internal/hermes/anthropic_auth_state_test.go` | Auth status reports keychain unavailable, corrupt auth backup, or relogin-required without deleting credentials. |
| 4 / 4.H | Provider-side resilience — Provider resilience umbrella over retry, cache, rate, and budget behavior | `draft` | `provider` | `large` | system | `internal/hermes and internal/kernel provider resilience fixtures` | Provider and kernel status expose retry schedule, Retry-After hints, cache disabled paths, rate guards, and budget telemetry gaps. |
| 4 / 4.H | Classified provider-error taxonomy — Structured provider error classification contract | `validated` | `provider` | `small` | system | `internal/hermes provider error-classification fixture table` | Provider status and logs expose auth, rate-limit, context, retryable, and non-retryable classes instead of raw opaque errors. |
| 5 / 5.A | Tool registry inventory + schema parity harness — Operation and tool descriptor parity before handler ports | `draft` | `tools` | `medium` | operator, gateway, child-agent, system | `internal/tools upstream schema parity manifest fixtures` | Doctor reports disabled tools, missing dependencies, schema drift, and unavailable provider-specific paths. |
| 5 / 5.F | Skill preprocessing + dynamic slash commands — Skill content preprocessing and skill-backed slash commands are deterministic, disabled-skill aware, and prompt-safe | `fixture_ready` | `skills` | `small` | operator, gateway, system | `internal/skills/preprocessing_commands_test.go` | Skill status reports disabled, missing-prerequisite, or preprocessing-failed skills without injecting them into prompts. |
| 5 / 5.I | First-party Spotify plugin fixture — First-party plugin manifests and tool packages load through the plugin SDK without reverting to built-in tool registration | `draft` | `tools` | `small` | operator, system | `internal/plugins/spotify_plugin_test.go` | Plugin status reports missing environment or auth setup without registering broken prompt-visible tools. |
| 5 / 5.O | PTY bridge protocol adapter — Dashboard/TUI PTY sessions expose bounded read, write, resize, close, and unavailable-state behavior through a testable adapter | `fixture_ready` | `tools` | `small` | operator | `internal/cli/pty_bridge_test.go` | Dashboard or CLI status reports PTY unavailable instead of falling back to unsafe shell execution. |
| 5 / 5.O | Busy command guard for compression and long CLI actions — Long-running CLI commands set busy input state and reject overlapping user input until the command exits | `draft` | `tools` | `small` | operator | `internal/cli/busy_command_test.go` | CLI/TUI status reports command-busy state instead of accepting overlapping input that can corrupt turn state. |
| 5 / 5.Q | OpenAI-compatible chat-completions API server — OpenAI-compatible chat.completions HTTP surface over the native Gormes turn loop | `fixture_ready` | `gateway` | `medium` | operator, gateway | `internal/apiserver/chat_completions_test.go` | API health and error envelopes report auth, body-size, content-normalization, and streaming failures without starting hidden sessions. |
| 5 / 5.Q | Responses API store + run event stream — Stateful OpenAI Responses and runs APIs over the same native session chain as chat completions | `draft` | `gateway` | `medium` | operator, gateway | `internal/apiserver/responses_runs_test.go` | API status reports response-store disabled, LRU eviction, orphaned runs, and previous_response_id misses. |
| 5 / 5.Q | API server disconnect snapshot persistence — Streaming disconnects and server cancellations persist incomplete Responses snapshots when store=true | `draft` | `gateway` | `small` | operator, gateway | `internal/apiserver/disconnect_snapshot_test.go` | Stored response status distinguishes incomplete disconnect snapshots from failed or completed responses. |
| 5 / 5.Q | Gateway proxy mode forwarding contract — Gateway adapters can forward turns to a remote OpenAI-compatible Gormes API server while preserving session IDs and safe history filtering | `draft` | `gateway` | `small` | gateway, operator | `internal/gateway/proxy_mode_test.go` | Gateway status reports proxy unreachable, stale generation, or missing proxy credentials without dropping local audit records. |
| 5 / 5.Q | Dashboard API client contract — Dashboard-facing API helpers consume native chat, Responses, model, provider, OAuth, session, and tool-progress endpoints without importing the upstream React app | `draft` | `gateway` | `small` | operator | `internal/apiserver/dashboard_contract_test.go` | Dashboard status reports missing native endpoints or disabled panels instead of assuming the upstream Node/React server exists. |
| 5 / 5.Q | Dashboard PTY chat sidecar contract — Dashboard chat sidecar bridges PTY bytes and structured tool events without merging terminal transport into API server state | `draft` | `gateway` | `small` | operator | `internal/apiserver/dashboard_pty_test.go` | Dashboard status reports PTY or sidecar event publication unavailable while preserving normal API chat. |
| 6 / 6.C | Portable SKILL.md format — Reviewed skill-as-code storage format | `draft` | `skills` | `medium` | operator, system | `internal/skills SKILL.md metadata, provenance, and review-state fixtures` | Skill status excludes unreviewed or invalid drafts from prompt injection and records resolver or metadata failures. |
<!-- PROGRESS:END -->

## Authoring Rule

New priority progress rows should add contract metadata when the row is used as
an implementation handoff. The fields are optional only for historical rows and
inventory buckets.

Required handoff shape:

```json
{
  "name": "Short executable slice",
  "status": "planned",
  "contract_status": "draft",
  "contract": "The upstream behavior being preserved",
  "slice_size": "small",
  "execution_owner": "gateway",
  "trust_class": ["operator"],
  "degraded_mode": "How partial capability becomes visible",
  "fixture": "The replayable local fixture proving compatibility",
  "source_refs": ["docs/content/upstream-hermes/source-study.md"],
  "blocked_by": ["optional dependency"],
  "unblocks": ["optional downstream slice"],
  "ready_when": ["The dependency or handoff condition is true"],
  "acceptance": ["fixture or behavior that proves this row is done"]
}
```

## Canonical Progress Source

There is one docs-side progress source:

```text
docs/content/building-gormes/architecture_plan/progress.json
```

Do not reintroduce `docs/data/progress.json`. The website can keep an embedded
copy under `www.gormes.ai/internal/site/data/progress.json`, but that file is a
generated site asset, not a planning source.
