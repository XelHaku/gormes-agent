---
title: "Autoloop Handoff"
weight: 33
---

# Autoloop Handoff

This page is generated from `meta.autoloop` in the canonical progress file:
`docs/content/building-gormes/architecture_plan/progress.json`.

It keeps shared unattended-loop facts in one place so autonomous workers do not
guess the entrypoint, plan, candidate source, generated docs, or selection
policy from scattered prose. Row-specific execution facts stay in
[Agent Queue](./agent-queue/) and canonical progress rows.

<!-- PROGRESS:START kind=autoloop-handoff -->
## Control Plane

- Entrypoint: `scripts/gormes-auto-codexu-orchestrator.sh`
- Plan: `docs/superpowers/plans/2026-04-24-orchestrator-oiling-release-1-plan.md`
- Candidate source: `docs/content/building-gormes/architecture_plan/progress.json`
- Agent queue: `docs/content/building-gormes/agent-queue.md`
- Progress schema: `docs/content/building-gormes/progress-schema.md`
- Unit tests: `scripts/orchestrator/tests/run.sh unit`

## Candidate Policy

- Skip rows with blocked_by until ready_when is satisfied.
- Skip slice_size=umbrella rows until they are split.
- Prefer contract rows with write_scope, test_commands, and done_signal.
- Inject selected progress metadata into the worker prompt instead of asking workers to rescan the whole roadmap.
<!-- PROGRESS:END -->
