---
title: "Phase 6 — The Learning Loop (Soul)"
weight: 70
---

# Phase 6 — The Learning Loop (Soul)

**Status:** ⏳ planned · 4/6 sub-phases

The Learning Loop is the first Gormes-original core system — not a port. It detects when a task is complex enough to be worth learning from, distills the solution into a reusable skill, stores it, and improves the skill over successive runs. Upstream Hermes alludes to self-improvement; Gormes implements it as a dedicated subsystem.

> "Agents are not prompts. They are systems. Memory + skills > raw model intelligence."

## Sub-phase outline

| Subphase | Status | Deliverable |
|---|---|---|
| 6.A — Complexity Detector | ✅ complete | Heuristic signal for "this turn was worth learning from" now ships via `internal/learning/runtime.go`, with kernel-written JSONL decisions under `${XDG_DATA_HOME}/gormes/learning/complexity.jsonl` |
| 6.B — Skill Extractor | ✅ complete | LLM-assisted pattern distillation now ships via `internal/learning/extractor.go`, gating on the 6.A signal and persisting JSONL skill candidates for 6.C |
| 6.C — Skill Storage Format | ⏳ planned | Portable, human-editable Markdown (SKILL.md) with structured metadata |
| 6.D — Skill Retrieval + Matching | ⏳ planned | Hybrid lexical + semantic lookup for relevant skills at turn start |
| 6.E — Feedback Loop | ✅ complete | Per-skill outcome log + Laplace-smoothed effectiveness score now ships via `internal/learning/feedback.go` |
| 6.F — Skill Surface (TUI + Telegram) | ✅ complete (browsing only) | Shared `skills.BrowseView` now powers the gateway `/skills` command and `tui.RenderSkillsPane`; edit + disable flows remain follow-on |

## Why this is Phase 6 and not Phase 5.F

Phase 5.F (Skills system) was previously scoped as "port the upstream Python skills plumbing". That's mechanical. Phase 6 is the algorithm on top — detecting complexity, distilling patterns, scoring feedback. It depends on 5.F (needs the storage format), but it's not the same work.

Positioning: **Gormes's moat over Hermes**. Hermes has a skills directory; it does not have a native learning loop that decides what's worth writing down.

## 6.B Closeout

Phase 6.B lands the LLM-assisted extractor half of the learning loop. `internal/learning/extractor.go` introduces a narrow `LLM` seam (`Distill(ctx, prompt) (DistillResponse, error)`) and an `Extractor` that:

- Gates on the 6.A `Signal` — turns that did not cross the worth-learning threshold short-circuit before any prompt is built or any file is touched, so the extractor inherits 6.A's auditability contract.
- Renders a deterministic prompt from the `Source` — session ID, signal reasons and tool names, the user and assistant messages, and each tool event's args + result — so replaying the same turn reproduces byte-identical input to the model.
- Validates the returned `DistillResponse` (Name, Description, Body must all be non-blank) before accepting the distillation; invalid responses and LLM errors propagate upward without leaving a partial JSONL line behind.
- Appends each accepted skill proposal as a `Candidate` JSONL record alongside the scoring metadata (score, threshold, reasons, tool names, distilled-at timestamp), matching the append-only `${XDG_DATA_HOME}/gormes/learning/...` convention already set by 6.A and 6.E.

Wiring the live LLM seam into the kernel is deferred to the 6.C storage-format slice, which will resolve how `Candidate` records become SKILL.md artefacts on disk. 6.B ships the algorithm; 6.C will ship the file layout.

## 6.A Closeout

Phase 6.A now lands a deterministic heuristic detector instead of waiting for a full extractor pipeline. `internal/learning/runtime.go` scores each successful turn on five cheap signals: any tool use, multi-tool use, prompt/completion token volume, transcript size, and wall-clock duration. `internal/kernel/kernel.go` records that decision after successful turns, while the shared TUI, gateway, Telegram, ACP, and BOOT entrypoints all wire the same runtime so the detector is active everywhere the kernel runs.

The output is deliberately narrow and auditable: one append-only JSONL record per completed turn at `${XDG_DATA_HOME}/gormes/learning/complexity.jsonl`, including the score, threshold, reasons, tool names, and raw metrics. That gives Phase 6.B a stable gate for "worth learning from" without prematurely coupling this slice to skill extraction or promotion logic.

## 6.E Closeout

Phase 6.E lands the scoring half of the feedback loop ahead of the remaining extractor work. `internal/learning/feedback.go` adds a `FeedbackStore` that appends one `Outcome` record per (skill, turn) pair as JSONL, then replays that log to produce per-skill `EffectivenessScore` aggregates. The score uses Laplace smoothing — `(successes + 1) / (uses + 2)` — so a brand-new skill starts at the neutral prior `0.5` and converges to the observed success ratio as samples accumulate.

Callers who rank skills at selection time can consult `FeedbackStore.Weight(ctx, name)` and multiply the returned weight directly into relevance scores without special-casing fresh skills: unknown names, blank names, and log read errors all fall back to `0.5` instead of returning a zero that would suppress untested skills. The store is append-only and self-contained, matching the auditability contract already set by the Phase 6.A complexity log.

## 6.F Closeout (browsing)

Phase 6.F lands the browsing half of the Skill Surface. `internal/skills/browse.go` introduces a shared `BrowseView` plus `FormatBrowseSummary` helper that sorts installed and hub-available skills deterministically and paginates them into one Telegram-sized or TUI-pane-sized page. Both edges now consume the same helper so operators see identical listings:

- Gateway `/skills` is wired through `internal/gateway/skills_command.go` and a new `SkillsBrowser` seam on `ManagerConfig`; `cmd/gormes/skills_browser.go` backs it with `skills.Hub` so Telegram, Discord, and any future shared-chassis adapter deliver the same text on demand.
- TUI `internal/tui.RenderSkillsPane(view, width)` renders the same summary through `lipgloss` width-aware wrapping, keeping the TUI surface ready to reuse the Telegram payload without re-implementing formatting.

Editing and disabling skills from the TUI or messaging edge remain explicit follow-on scope — the browsing contract shipped here gives those flows a single source of truth to build against.
