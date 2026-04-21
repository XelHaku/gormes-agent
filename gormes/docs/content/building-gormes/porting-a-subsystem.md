---
title: "Porting a Subsystem from Upstream"
weight: 40
---

# Porting a Subsystem from Upstream

The contribution path. Use this when you want to port a piece of Hermes into Gormes.

## 1. Pick your target

Open [Subsystem Inventory](./architecture_plan/subsystem-inventory/). Every row is a Hermes subsystem with a target Gormes sub-phase. Pick one that:

- Carries a ⏳ planned status (not already shipped)
- Has no hard dependency on a later phase (check the "Target phase" column)
- You have context on (voice/vision are big lifts; a platform adapter is a reasonable first PR)

## 2. Write a spec

`gormes/docs/superpowers/specs/YYYY-MM-DD-<subsystem>-design.md`. Use the brainstorming skill if you want guided design; otherwise mirror the shape of an existing spec. Get maintainer approval before writing the plan.

## 3. Write a plan

`gormes/docs/superpowers/plans/YYYY-MM-DD-<subsystem>.md`. Break into tasks small enough for subagent-driven execution (5–10 tasks, 2–5 minute steps). See existing plans under `gormes/docs/superpowers/plans/` for examples.

## 4. Implement

Bite-sized commits. Tests first (TDD). Mirror the existing Go package layout under `gormes/internal/`.

## 5. Open a PR

Target `main`. Title convention: `feat(gormes/<subsystem>): port <capability> from upstream`. Reference the spec + plan in the description.

## 6. Update the inventory

Flip your row in [Subsystem Inventory](./architecture_plan/subsystem-inventory/) from ⏳ planned to ✅ shipped, with a link to the shipped spec.
