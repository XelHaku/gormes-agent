---
title: "Blocked Slices"
weight: 40
aliases:
  - /building-gormes/blocked-slices/
---

# Blocked Slices

This page is generated from canonical `progress.json` rows that declare
`blocked_by`.

Use it to avoid assigning work before the dependency chain is ready.

<!-- PROGRESS:START kind=blocked-slices -->
| Phase | Slice | Blocked by | Ready when | Unblocks |
|---|---|---|---|---|
| 2 / 2.B.5 | BlueBubbles iMessage session-context prompt guidance | BlueBubbles iMessage bubble formatting parity | BlueBubbles outbound formatting splits blank-line paragraphs into separate iMessage sends, so prompt guidance has a matching delivery contract. | - |
| 2 / 2.F.3 | Unauthorized DM pairing response contract | Pairing approval + rate-limit semantics | Pairing approval, rate limiting, and allowlist checks are fixture-locked. | - |
| 2 / 2.F.5 | Steer slash command registry + queue fallback | 2.E.2 | 2.E.2 is complete and the shared CommandDef registry is stable for gateway commands. | Mid-run steer injection between tool calls, Gateway-handled slash commands bypass active-session guard |
| 4 / 4.A | Bedrock stale-client eviction + retry classification | Bedrock SigV4 + credential seam | A Bedrock client/cache seam exists behind the provider adapter and can be exercised without live AWS credentials. | - |
| 4 / 4.A | Codex OAuth state + stale-token relogin | Token vault, Multi-account auth, Codex Responses pure conversion harness | Gormes has an XDG-scoped token vault and account-selection seam for provider credentials. | - |
| 4 / 4.A | Codex stream repair + tool-call leak sanitizer | Codex Responses pure conversion harness | Codex Responses conversion fixtures can replay streamed and non-streamed output without live credentials. | - |
| 4 / 4.B | ContextEngine interface + status tool contract | Provider interface + stream fixture harness | Provider interface + stream fixture harness can replay context status without live provider calls. | Compression token-budget trigger + summary sizing, Tool-result pruning + protected head/tail summary |
| 4 / 4.D | Model pricing/capability registry fixtures | Provider-enforced context-length resolver | Provider-enforced context resolver fixtures establish the metadata package shape and fallback semantics. | Routing policy and fallback selector |
| 4 / 4.D | Routing policy and fallback selector | Provider-enforced context-length resolver, Model pricing/capability registry fixtures | Context limits, pricing, capabilities, and provider-family metadata are fixture-backed. | - |
| 4 / 4.G | Anthropic OAuth/keychain credential discovery | Token vault | Token vault owns XDG-scoped credential files and can expose provider auth status without live credentials. | - |
| 4 / 4.H | Provider-side resilience | Provider interface + stream fixture harness | Provider interface + stream fixture harness is available for resilience fixture coverage. | Retry-After header parsing + HTTPError hint, Kernel retry honors Retry-After hint, Provider rate guard + budget telemetry |
| 5 / 5.I | First-party Spotify plugin fixture | Plugin SDK | Plugin manifest loading and capability registration are fixture-locked by the Plugin SDK slice. | - |
| 5 / 5.O | Busy command guard for compression and long CLI actions | CLI command registry parity + active-turn busy policy | The CLI command registry has a shared active-turn/busy policy surface. | - |
| 5 / 5.Q | Responses API store + run event stream | OpenAI-compatible chat-completions API server | Chat-completions HTTP surface is native and response storage can reuse its auth, session, and error-envelope contracts. | API server disconnect snapshot persistence, Dashboard API client contract |
| 5 / 5.Q | API server disconnect snapshot persistence | Responses API store + run event stream | Responses store and run event stream can persist terminal and non-terminal snapshots. | - |
| 5 / 5.Q | Gateway proxy mode forwarding contract | OpenAI-compatible chat-completions API server | Native chat-completions API server accepts X-Hermes-Session-Id and streaming SSE fixtures. | - |
| 5 / 5.Q | Dashboard API client contract | OpenAI-compatible chat-completions API server, Responses API store + run event stream | Native API server exposes stable chat/Responses/session endpoints that dashboard contracts can call. | - |
| 5 / 5.Q | Dashboard PTY chat sidecar contract | PTY bridge protocol adapter, SSE streaming to Bubble Tea TUI | PTY bridge behavior and TUI gateway event streaming are each fixture-locked. | - |
| 6 / 6.C | Portable SKILL.md format | Phase 2.G skills runtime | Phase 2.G skills runtime is complete and the parser/store seam is stable enough for versioned metadata. | LLM-assisted pattern distillation, Hybrid lexical + semantic lookup, Skill effectiveness scoring |
<!-- PROGRESS:END -->
