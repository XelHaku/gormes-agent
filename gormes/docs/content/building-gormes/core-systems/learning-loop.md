---
title: "Learning Loop"
weight: 20
---

# The Learning Loop (The Soul)

Detects when a task was complex enough to learn from, distills the solution into a reusable skill, stores it, and improves the skill over successive runs.

## Simplified flow

```go
if taskComplexity(turn) > threshold {
    skill := extractSkill(conversation, toolCalls)
    store.Save(skill)
}
```

## Why this is load-bearing

Without a learning loop you lose:

- **Compounding intelligence** — the bot doesn't get smarter at *your* workflows over time
- **Differentiation** — every agent looks the same at turn zero
- **Long-term value** — you pay the same token tax on turn 1000 as on turn 1

Upstream Hermes has a `skills/` directory with hand-authored SKILL.md files. It does not have an algorithm that decides what's worth writing down. That's what Phase 6 delivers.

## Current status

⏳ Planned — see [Phase 6](../architecture_plan/phase-6-learning-loop/) for the sub-phase breakdown.

Execution should be TDD-first and local-signal-first:

- Start with deterministic complexity signals from transcript length, tool-call count, retries, edits, and operator feedback.
- Extend the Phase 2.G SKILL.md store with versioned metadata, provenance, review state, and atomic writes before generated skills persist.
- Use fake-model extraction fixtures to prove secret stripping and one-off task rejection before live LLM generation.
- Keep disabled or unreviewed skills out of prompt injection until retrieval, feedback, and operator review surfaces are all test-covered.
