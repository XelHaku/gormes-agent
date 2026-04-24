---
title: "Phase 6 — The Learning Loop (Soul)"
weight: 70
---

# Phase 6 — The Learning Loop (Soul)

**Status:** ⏳ planned · 0/6 sub-phases

The Learning Loop is the first Gormes-original core system — not a port. It detects when a task is complex enough to be worth learning from, distills the solution into a reusable skill, stores it, and improves the skill over successive runs. Upstream Hermes alludes to self-improvement; Gormes implements it as a dedicated subsystem.

> "Agents are not prompts. They are systems. Memory + skills > raw model intelligence."

## Sub-phase outline

| Subphase | Status | Deliverable |
|---|---|---|
| 6.A — Complexity Detector | ⏳ planned | Deterministic local signals first: transcript length, tool-call count, retries, edits, and operator feedback before any LLM scorer |
| 6.B — Skill Extractor | ⏳ planned | LLM-assisted pattern distillation from the conversation + tool-call trace, with fake-model fixtures and secret/noise rejection gates |
| 6.C — Skill Storage Format | ⏳ planned | Portable, human-editable SKILL.md with versioned metadata, provenance, review state, and atomic writes |
| 6.D — Skill Retrieval + Matching | ⏳ planned | Hybrid lexical + Phase 3 semantic lookup for relevant reviewed skills at turn start |
| 6.E — Feedback Loop | ⏳ planned | Persist skill-use outcomes, explicit operator feedback, and auditable weight adjustments |
| 6.F — Skill Surface (TUI + Telegram) | ⏳ planned | Browse, edit, disable, and review skills from the TUI or messaging edge after store/feedback contracts are stable |

## Why this is Phase 6 and not Phase 5.F

Phase 5.F (Skills system) was previously scoped as "port the upstream Python skills plumbing". That's mechanical. Phase 6 is the algorithm on top — detecting complexity, distilling patterns, scoring feedback. It depends on 5.F (needs the storage format), but it's not the same work.

Positioning: **Gormes's moat over Hermes**. Hermes has a skills directory; it does not have a native learning loop that decides what's worth writing down.

## TDD Execution Notes

Do not begin Phase 6 with live LLM extraction. The dependency order is:

1. **6.A deterministic detector** — prove the local trigger signals are explainable and replayable from transcript/tool-call fixtures.
2. **6.C storage extension** — extend the Phase 2.G store with versioned metadata, provenance, review state, and atomic writes before generated skills can persist.
3. **6.B extractor schema** — use fake model outputs to prove accepted/rejected skill drafts, secret stripping, and one-off task rejection.
4. **6.D retrieval scorer** — combine lexical and semantic signals while excluding disabled or unreviewed skills from prompt injection.
5. **6.E feedback records** — persist outcomes before any automatic promotion/demotion or weight change.
6. **6.F operator surfaces** — expose review/edit/disable flows only after the underlying store and feedback records are stable.
