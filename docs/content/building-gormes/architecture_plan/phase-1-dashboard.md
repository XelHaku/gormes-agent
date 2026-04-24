---
title: "Phase 1 — The Dashboard"
weight: 20
---

# Phase 1 — The Dashboard

**Status:** ✅ complete overall · 1.A, 1.B, and 1.C shipped

Phase 1 is a tactical Strangler Fig bridge, not a philosophical compromise. It exists to deliver immediate value to existing Hermes users while preserving a clean migration path toward a pure Go runtime that owns the entire lifecycle end to end.

The hybrid is **temporary**. The long-term state is 100% Go. During Phases 1–4, Go is the chassis (orchestrator, state, persistence, platform I/O, agent cognition) and Python is the peripheral library (research tools, legacy skills, ML heavy lifting). Each phase shrinks Python's footprint. Phase 5 deletes the last Python dependency.

**Deliverable:** Tactical bridge: Go TUI over Python's `api_server` HTTP+SSE boundary.

## What shipped

- Bubble Tea TUI shell
- Kernel with 16 ms render mailbox (coalescing)
- Route-B SSE reconnect (dropped streams recover)
- Wire Doctor — offline tool-registry validation
- Streaming token renderer

## What's ongoing

- Core TUI polish, bug fixes, and ergonomics stay on the maintenance lane, but no Phase 1 ledger gate remains open.
- Automation reliability is tracked as Phase 1.C because it affects whether planner/orchestrator runs can be trusted at scale. The false-failure stabilization row, soft-success bats row, and architecture-planner wrapper consistency row are complete; the renamed manager script is stable with compatibility wrappers.
- Evidence in tree: `internal/buildscripts_test.go` covers heartbeat progress, integration-worktree reuse, promotion-before-next-cycle behavior, and the soft-success case. Recent orchestrator landings flipped the conflated `contract_or_test_failure` status into a granular taxonomy (`no_commit_made|wrong_commit_count|worktree_dirty|branch_mismatch|report_commit_mismatch|scope_violation|report_validation_failed`) — `scripts/orchestrator/tests/unit/failures.bats` covers the failure-record writer, reader, reset, and poisoned-task thresholds over the new reason set — and `scripts/orchestrator/tests/unit/soft-success.bats` now directly covers `try_soft_success_nonzero` for rc=124/137 rejection, opt-out rejection, invalid-report rejection, dirty-commit rejection, valid report + clean commit success, and worker-state `original_rc` preservation. `internal/architectureplanneragent_test.go` now copies and runs `scripts/gormes-architecture-planner-tasks-manager.sh`, `scripts/gormes-architecture-task-manager.sh`, and `scripts/architectureplanneragent.sh` so legacy wrapper callers stay covered.
- `scripts/orchestrator/FROZEN.md` now declares a commit freeze on the orchestrator entry script plus `lib/*.sh`, `audit.sh`, `claudeu`, and the systemd templates: only production-incident hotfixes or user-requested features landed via scoped spec + plan are allowed, so future 1.C slices must come in through that gate rather than drive-by cleanup.
- Orchestrator Final Polish capabilities landed alongside the commit freeze (spec: `docs/superpowers/specs/2026-04-24-orchestrator-final-polish-design.md`). Shipped: PR-based promotion gate with cherry-pick fallback (`PROMOTION_MODE={pr,cherry-pick}`, `worker_pr_opened` ledger events); mandatory self-verified acceptance criteria in worker reports (section 9 "Acceptance check" validated by `collect_final_report_issues`); staged Go audit cursor/report artifacts with minimal ledger counts; `scripts/orchestrator/daily-digest.sh` for a 24-hour review summary; background companions via `setsid nohup` with PID tracking, exponential backoff on empty candidate-pool refills, and a startup env banner + `startup_env` ledger event; and a `claudeu` shim that streams Claude events and auto-falls back to `codexu` when Claude CLI reports credit exhaustion or 429/quota errors.
