---
title: "Agent Queue"
weight: 34
---

# Agent Queue

This page is generated from the canonical progress file:
`docs/content/building-gormes/architecture_plan/progress.json`.

It lists unblocked, non-umbrella contract rows that are ready for a focused
autonomous implementation attempt. Each card carries the execution owner,
slice size, contract, trust class, degraded-mode requirement, fixture target,
acceptance checks, and source references.

<!-- PROGRESS:START kind=agent-queue -->
## 1. Orchestrator failure-row stabilization for 4-8 workers

- Phase: 1 / 1.C
- Owner: `orchestrator`
- Size: `large`
- Status: `in_progress`
- Contract: Worker verification and failure-taxonomy contract
- Trust class: operator, system
- Ready when: Failure taxonomy and soft-success recovery behavior are covered by direct orchestrator unit fixtures.
- Not ready when: The row is treated as complete before direct try_soft_success_nonzero coverage lands.
- Degraded mode: The orchestrator records precise failure reasons, poisoned-task thresholds, soft-success decisions, and original exit codes instead of collapsing failures into one generic status.
- Fixture: `scripts/orchestrator unit fixtures for failure taxonomy and soft-success recovery`
- Acceptance: Failure rows emit a granular reason instead of contract_or_test_failure., Non-timeout/non-OOM codex exits can become soft_success_nonzero only after final-report and commit verification pass., Timeout and OOM exits remain hard failures.
- Source refs: docs/superpowers/specs/2026-04-24-orchestrator-oiling-release-1-design.md, scripts/orchestrator/lib/worktree.sh, scripts/gormes-auto-codexu-orchestrator.sh
- Unblocks: Soft-success-nonzero bats coverage, Planner wrapper/test consistency closeout
- Why now: Already active; contract metadata keeps execution bounded.

## 2. Provider interface + stream fixture harness

- Phase: 4 / 4.A
- Owner: `provider`
- Size: `medium`
- Status: `in_progress`
- Contract: Provider-neutral request and stream event transcript harness
- Trust class: system
- Ready when: Anthropic transcript fixtures replay request, stream, finish reason, and usage data without live credentials.
- Not ready when: A provider-specific adapter lands before shared transcript fixtures prove the contract.
- Degraded mode: Provider status reports missing fixture coverage or unavailable adapters before kernel routing can select them.
- Fixture: `internal/hermes provider transcript fixtures`
- Acceptance: Provider transcripts replay request, stream, finish reason, and usage data without live credentials., EOF after partial tool_call surfaces pending calls instead of dropping them., Duplicate tool-name deltas cannot concatenate into invented names.
- Source refs: docs/content/upstream-hermes/source-study.md, docs/content/building-gormes/architecture_plan/phase-4-brain-transplant.md
- Unblocks: Bedrock Converse payload mapping (no AWS SDK), Gemini, OpenRouter, Codex
- Why now: Already active; contract metadata keeps execution bounded.

## 3. Tool registry inventory + schema parity harness

- Phase: 5 / 5.A
- Owner: `tools`
- Size: `medium`
- Status: `planned`
- Contract: Operation and tool descriptor parity before handler ports
- Trust class: operator, gateway, child-agent, system
- Ready when: Upstream tool descriptor inventory can be captured without porting handlers in the same slice.
- Not ready when: Handler implementation starts before descriptor parity fixtures exist.
- Degraded mode: Doctor reports disabled tools, missing dependencies, schema drift, and unavailable provider-specific paths.
- Fixture: `internal/tools upstream schema parity manifest fixtures`
- Acceptance: Upstream tool names, toolsets, required env vars, schemas, result envelopes, trust classes, and degraded status are captured in fixtures., No handler port can mark complete until its descriptor parity row exists., Doctor can report missing dependencies or disabled provider-specific paths.
- Source refs: docs/content/upstream-hermes/reference/tools-reference.md, docs/content/building-gormes/architecture_plan/phase-5-final-purge.md
- Unblocks: Pure core tools first, Stateful tool migration queue, CLI command registry parity + active-turn busy policy
- Why now: Unblocks Pure core tools first, Stateful tool migration queue, CLI command registry parity + active-turn busy policy.

<!-- PROGRESS:END -->
