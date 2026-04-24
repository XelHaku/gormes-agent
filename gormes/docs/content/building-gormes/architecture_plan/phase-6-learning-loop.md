---
title: "Phase 6 — The Learning Loop (Soul)"
weight: 70
---

# Phase 6 — The Learning Loop (Soul)

**Status:** ⏳ planned · 2/6 sub-phases

The Learning Loop is the first Gormes-original core system — not a port. It detects when a task is complex enough to be worth learning from, distills the solution into a reusable skill, stores it, and improves the skill over successive runs. Upstream Hermes alludes to self-improvement; Gormes implements it as a dedicated subsystem.

> "Agents are not prompts. They are systems. Memory + skills > raw model intelligence."

## Sub-phase outline

| Subphase | Status | Deliverable |
|---|---|---|
| 6.A — Complexity Detector | ✅ complete | Heuristic signal for "this turn was worth learning from" now ships via `internal/learning/runtime.go`, with kernel-written JSONL decisions under `${XDG_DATA_HOME}/gormes/learning/complexity.jsonl` |
| 6.B — Skill Extractor | ⏳ planned | LLM-assisted pattern distillation from the conversation + tool-call trace |
| 6.C — Skill Storage Format | ⏳ planned | Portable, human-editable Markdown (SKILL.md) with structured metadata |
| 6.D — Skill Retrieval + Matching | ⏳ planned | Hybrid lexical + semantic lookup for relevant skills at turn start |
| 6.E — Feedback Loop | ✅ complete | Per-skill outcome log + Laplace-smoothed effectiveness score now ships via `internal/learning/feedback.go` |
| 6.F — Skill Surface (TUI + Telegram) | ⏳ planned | Browse, edit, disable skills from the CLI or messaging edge |

## Why this is Phase 6 and not Phase 5.F

Phase 5.F (Skills system) was previously scoped as "port the upstream Python skills plumbing". That's mechanical. Phase 6 is the algorithm on top — detecting complexity, distilling patterns, scoring feedback. It depends on 5.F (needs the storage format), but it's not the same work.

Positioning: **Gormes's moat over Hermes**. Hermes has a skills directory; it does not have a native learning loop that decides what's worth writing down.

## 6.A Closeout

Phase 6.A now lands a deterministic heuristic detector instead of waiting for a full extractor pipeline. `internal/learning/runtime.go` scores each successful turn on five cheap signals: any tool use, multi-tool use, prompt/completion token volume, transcript size, and wall-clock duration. `internal/kernel/kernel.go` records that decision after successful turns, while the shared TUI, gateway, Telegram, ACP, and BOOT entrypoints all wire the same runtime so the detector is active everywhere the kernel runs.

The output is deliberately narrow and auditable: one append-only JSONL record per completed turn at `${XDG_DATA_HOME}/gormes/learning/complexity.jsonl`, including the score, threshold, reasons, tool names, and raw metrics. That gives Phase 6.B a stable gate for "worth learning from" without prematurely coupling this slice to skill extraction or promotion logic.

## 6.E Closeout

Phase 6.E lands the scoring half of the feedback loop ahead of the remaining extractor work. `internal/learning/feedback.go` adds a `FeedbackStore` that appends one `Outcome` record per (skill, turn) pair as JSONL, then replays that log to produce per-skill `EffectivenessScore` aggregates. The score uses Laplace smoothing — `(successes + 1) / (uses + 2)` — so a brand-new skill starts at the neutral prior `0.5` and converges to the observed success ratio as samples accumulate.

Callers who rank skills at selection time can consult `FeedbackStore.Weight(ctx, name)` and multiply the returned weight directly into relevance scores without special-casing fresh skills: unknown names, blank names, and log read errors all fall back to `0.5` instead of returning a zero that would suppress untested skills. The store is append-only and self-contained, matching the auditability contract already set by the Phase 6.A complexity log.
