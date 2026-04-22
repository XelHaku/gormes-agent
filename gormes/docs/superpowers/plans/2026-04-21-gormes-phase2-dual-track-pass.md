# Phase 2 Dual-Track Pass Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land one bounded implementation pass that finishes the Phase 2.E runtime-core closeout and ships the Phase 2.B.2 gateway chassis with Discord as the second real channel.

**Architecture:** This pass is intentionally split into two independent execution tracks. Track A closes the existing subagent runtime over the current in-tree code. Track B executes the already-detailed chassis extraction and Discord port. The tracks do not depend on each other at the package level; they only meet at final verification and docs status updates.

**Tech Stack:** Go 1.25+, existing `internal/subagent`, new `internal/gateway`, existing `internal/telegram`, new Discord adapter package, `cobra`, `bbolt`, `discordgo`, repo docs tooling.

**Spec:** [`../specs/2026-04-21-gormes-phase2-dual-track-pass-design.md`](../specs/2026-04-21-gormes-phase2-dual-track-pass-design.md)

---

## File Structure

**Detailed execution plans used by this pass:**
- `gormes/docs/superpowers/plans/2026-04-21-gormes-phase2e-runtime-closeout.md`
- `gormes/docs/superpowers/plans/2026-04-21-gormes-phase2b2-chassis.md`

**Final pass-level verification and docs touchpoints:**
- `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md`
- `gormes/docs/content/building-gormes/architecture_plan/progress.json`
- `gormes/docs/content/building-gormes/architecture_plan/_index.md`

---

## Task 1: Execute Track A first — subagent runtime closeout

**Files:**
- Follow: `gormes/docs/superpowers/plans/2026-04-21-gormes-phase2e-runtime-closeout.md`

- [ ] **Step 1: Read the Track A plan**

Run:

```bash
sed -n '1,260p' gormes/docs/superpowers/plans/2026-04-21-gormes-phase2e-runtime-closeout.md
```

Expected: clear closeout steps for comments, binary wiring proof, docs status, and focused verification.

- [ ] **Step 2: Execute Track A completely**

Run the steps from:

```text
gormes/docs/superpowers/plans/2026-04-21-gormes-phase2e-runtime-closeout.md
```

Expected: Track A leaves `internal/subagent`, config wiring, and Phase 2 docs in a consistent state before gateway churn begins.

- [ ] **Step 3: Verify Track A checkpoint**

Run:

```bash
cd gormes && go test ./internal/subagent ./internal/config ./cmd/gormes -count=1 -race
```

Expected: all three packages pass before Track B starts.

---

## Task 2: Execute Track B second — gateway chassis + Discord

**Files:**
- Follow: `gormes/docs/superpowers/plans/2026-04-21-gormes-phase2b2-chassis.md`

- [ ] **Step 1: Read the Track B plan**

Run:

```bash
sed -n '1,260p' gormes/docs/superpowers/plans/2026-04-21-gormes-phase2b2-chassis.md
```

Expected: detailed chassis, Telegram migration, Discord adapter, config, CLI, doctor, and test tasks.

- [ ] **Step 2: Execute Track B completely**

Run the steps from:

```text
gormes/docs/superpowers/plans/2026-04-21-gormes-phase2b2-chassis.md
```

Expected: `internal/gateway/` lands, Telegram is migrated onto it, and Discord works end-to-end on the shared manager.

- [ ] **Step 3: Verify Track B checkpoint**

Run at least the focused packages called out by the plan. Minimum expected checkpoint:

```bash
cd gormes && go test ./internal/gateway/... ./internal/telegram ./cmd/gormes -count=1
```

Expected: chassis and Telegram path green before broad verification.

---

## Task 3: Finish pass-level migration notes and ledger updates

**Files:**
- Modify: `gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/progress.json`
- Modify: `gormes/docs/content/building-gormes/architecture_plan/_index.md`

- [ ] **Step 1: Ensure the docs reflect both tracks**

The final docs state after this pass should read roughly like:

```md
- Phase 2.E runtime core is landed/in progress with `delegate_task`, registry, lifecycle manager, and bounded concurrency.
- Phase 2.B.2 has a shared gateway chassis and Discord as the second real adapter.
- Remaining channels (Slack/WhatsApp/Signal/Email/SMS) are follow-up consumers of the chassis.
```

- [ ] **Step 2: Regenerate any derived progress output**

Run:

```bash
cd gormes && make generate-progress
```

Expected: generated progress pages updated or command succeeds as a no-op.

- [ ] **Step 3: Commit docs if they changed outside prior track commits**

```bash
git add \
  gormes/docs/content/building-gormes/architecture_plan/phase-2-gateway.md \
  gormes/docs/content/building-gormes/architecture_plan/progress.json \
  gormes/docs/content/building-gormes/architecture_plan/_index.md
git commit -m "docs(phase2): record dual-track gateway and subagent progress"
```

Only do this commit if Track A / Track B commits did not already cover the same docs.

---

## Task 4: Final verification gate

**Files:**
- Verify only: whole repo under `gormes/`

- [ ] **Step 1: Run focused regression suites before the broad run**

Run:

```bash
cd gormes && go test ./internal/subagent ./internal/config ./internal/telegram ./cmd/gormes -count=1 -race
```

Expected: all focused packages pass with race detection enabled.

- [ ] **Step 2: Run the broad suite**

Run:

```bash
cd gormes && go test ./... -race
```

Expected: full repository suite green.

- [ ] **Step 3: Capture any final tiny fixes and commit if required**

```bash
git add -A
git commit -m "test: close phase 2 dual-track verification"
```

Only do this commit if verification surfaced real edits.

---

## Self-Review Notes

- The spec covers two independent subsystems, so this plan deliberately splits execution across one dedicated Track A plan and one dedicated Track B plan.
- No new third subsystem is introduced here.
- Track A uses a regenerated closeout plan because the existing tree has already implemented much of the original runtime plan.
- Track B reuses the existing detailed gateway chassis plan because it is already the correct level of specificity for implementation.
