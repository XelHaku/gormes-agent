---
title: "Porting a Subsystem from Upstream"
weight: 40
---

# Porting a Subsystem from Upstream

The contribution path. Use this when you want to port a piece of Hermes into Gormes.

## 1. Pick your target

Open [Subsystem Inventory](../architecture_plan/subsystem-inventory/). Every row is a Hermes subsystem with a target Gormes sub-phase. Pick one that:

- Carries a ⏳ planned status (not already shipped)
- Has no hard dependency on a later phase (check the "Target phase" column)
- You have context on (voice/vision are big lifts; a platform adapter is a reasonable first PR)

## 2. Do the source-study checklist

Before writing implementation tasks, answer these in the spec or plan:

1. **Contract:** what upstream behavior is being ported? Name the source files
   and the external contract, not just the donor implementation.
2. **Trust class:** who can call it: `operator`, `gateway`, `child-agent`, or
   `system`? What is rejected before handler code runs?
3. **Fixture:** what replayable fixture proves compatibility without live
   credentials, live platforms, or a real provider?
4. **Degraded mode:** how does partial capability show up in status, doctor,
   audit, or logs?
5. **Boundary:** what stays out of the kernel, gateway adapter, or trusted
   plugin surface?

Useful donor study pages:

- [Upstream Hermes Source Study](../../upstream-hermes/source-study/)
- [Upstream GBrain Architecture](../../upstream-gbrain/architecture/)
- [Upstream Lessons](../upstream-lessons/)

## 3. Write a spec

`docs/superpowers/specs/YYYY-MM-DD-<subsystem>-design.md`. Use the brainstorming skill if you want guided design; otherwise mirror the shape of an existing spec. Get maintainer approval before writing the plan.

## 4. Write a plan

`docs/superpowers/plans/YYYY-MM-DD-<subsystem>.md`. Break into tasks small enough for subagent-driven execution (5–10 tasks, 2–5 minute steps). See existing plans under `docs/superpowers/plans/` for examples.

## 5. Implement

Bite-sized commits. Tests first (TDD). Mirror the existing Go package layout under `internal/`.

## 6. Open a PR

Target `main`. Title convention: `feat(gormes/<subsystem>): port <capability> from upstream`. Reference the spec + plan in the description.

## 7. Update the inventory

Flip your row in [Subsystem Inventory](../architecture_plan/subsystem-inventory/) from ⏳ planned to ✅ shipped, with a link to the shipped spec.
