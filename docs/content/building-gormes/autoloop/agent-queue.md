---
title: "Agent Queue"
weight: 20
aliases:
  - /building-gormes/agent-queue/
---

# Agent Queue

This page is generated from the canonical progress file:
`docs/content/building-gormes/architecture_plan/progress.json`.

It lists unblocked, non-umbrella contract rows that are ready for a focused
autonomous implementation attempt. Each card carries the execution owner,
slice size, contract, trust class, degraded-mode requirement, fixture target,
write scope, test commands, done signal, acceptance checks, and source
references.

Shared unattended-loop facts live in [Autoloop Handoff](../autoloop-handoff/):
the main entrypoint, orchestrator plan, candidate source, generated docs,
tests, and candidate policy. Keep those control-plane facts in
`meta.autoloop`, and keep row-specific execution facts in `progress.json`.

<!-- PROGRESS:START kind=agent-queue -->
## 1. BlueBubbles iMessage bubble formatting parity

- Phase: 7 / 7.E
- Owner: `gateway`
- Size: `small`
- Status: `planned`
- Priority: `P3`
- Contract: BlueBubbles outbound iMessage sends are non-editable, markdown-stripped, paragraph-split bubbles without pagination suffixes
- Trust class: gateway, system
- Ready when: The first-pass BlueBubbles adapter already owns Send, markdown stripping, cached GUID resolution, and home-channel fallback in internal/channels/bluebubbles.
- Not ready when: The slice attempts to add live BlueBubbles HTTP/webhook registration, attachment download, reactions, typing indicators, or edit-message support.
- Degraded mode: BlueBubbles remains a usable first-pass adapter, but long replies may still arrive as one stripped text send until paragraph splitting and suffix-free chunking are fixture-locked.
- Fixture: `internal/channels/bluebubbles/bot_test.go`
- Write scope: `internal/channels/bluebubbles/bot.go`, `internal/channels/bluebubbles/bot_test.go`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/channels/bluebubbles -count=1`
- Done signal: BlueBubbles adapter tests prove paragraph-to-bubble sends, suffix-free chunking, and no edit/placeholder capability.
- Acceptance: Send splits blank-line-separated paragraphs into separate SendText calls while preserving existing chat GUID resolution and home-channel fallback., Long paragraph chunks omit `(n/m)` pagination suffixes and concatenate back to the stripped original text., Bot does not implement gateway.MessageEditor or gateway.PlaceholderCapable, preserving non-editable iMessage semantics.
- Source refs: ../hermes-agent/gateway/platforms/bluebubbles.py@f731c2c2, ../hermes-agent/tests/gateway/test_bluebubbles.py@f731c2c2, internal/channels/bluebubbles/bot.go, internal/gateway/channel.go
- Unblocks: BlueBubbles iMessage session-context prompt guidance
- Why now: Unblocks BlueBubbles iMessage session-context prompt guidance.

## 2. Compression token-budget trigger + summary sizing

- Phase: 4 / 4.B
- Owner: `provider`
- Size: `small`
- Status: `planned`
- Priority: `P1`
- Contract: Go context compression budget state recalculates threshold, tail, and summary token budgets whenever the active model context window changes
- Trust class: operator, system
- Ready when: ContextEngine interface + status tool contract is validated on main., Provider-enforced context-length resolver is validated on main., The implementation can be tested as pure internal/hermes budget logic with no live provider call.
- Not ready when: The slice mutates kernel message history, calls a summarizer LLM, ports context references, or changes provider routing instead of only implementing compressor budget/status behavior.
- Degraded mode: Context status reports compression disabled or unavailable instead of using stale budget values from a previous model window.
- Fixture: `internal/hermes/context_compressor_budget_test.go`
- Write scope: `internal/hermes/`, `docs/content/building-gormes/architecture_plan/progress.json`
- Test commands: `go test ./internal/hermes -count=1`
- Done signal: internal/hermes/context_compressor_budget_test.go proves initialization and UpdateModelContext-style model switches recalculate threshold, tail, and max-summary budgets from the active context window.
- Acceptance: Initializing the budget helper for a 200000-token context with threshold_percent=0.50 and summary_target_ratio=0.20 reports threshold_tokens=100000, tail_token_budget=20000, and max_summary_tokens=min(context_length*0.05, 12000)., Updating the active model context from 200000 tokens to 32000 tokens recalculates threshold_tokens, tail_token_budget, and max_summary_tokens from the new context length instead of preserving old budgets., Summary target ratio is clamped to the upstream 0.10-0.80 range and the threshold floor preserves MINIMUM_CONTEXT_LENGTH-equivalent behavior., The focused test mirrors Hermes TestUpdateModelBudgets without wiring compression into the kernel or requiring provider credentials.
- Source refs: ../hermes-agent/agent/context_compressor.py@5401a008, ../hermes-agent/tests/agent/test_context_compressor.py@5401a008, docs/content/upstream-hermes/developer-guide/context-compression-and-caching.md, internal/hermes/context_engine.go, internal/hermes/model_context_resolver.go
- Unblocks: Tool-result pruning + protected head/tail summary, Manual compression feedback + context references
- Why now: Unblocks Tool-result pruning + protected head/tail summary, Manual compression feedback + context references.

<!-- PROGRESS:END -->
