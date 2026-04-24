# Gormes Architecture Planner Tasks Manager Script Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the architecture planner runner to a clearer planner-tasks-manager entrypoint, preserve old callers, and emit a Markdown task artifact.

**Architecture:** `gormes-architecture-planner-tasks-manager.sh` is the real implementation. `gormes-architecture-task-manager.sh` and `architectureplanneragent.sh` remain tiny compatibility wrappers. The implementation keeps the existing planner flow but adds explicit commands and writes `.codex/planner/architecture-planner-tasks.md` from the generated context.

**Tech Stack:** Bash, jq, git, codexu, Go test harness.

**Audit Note (2026-04-23):** This plan's wrapper-preservation assumption is currently stale against the worktree: `scripts/gormes-architecture-task-manager.sh` and `scripts/architectureplanneragent.sh` are deleted while `internal/architectureplanneragent_test.go` still copies/runs them. Execution agents must first choose one TDD policy: restore the tiny exec wrappers and keep the tests, or update tests/docs to a single-entrypoint policy. Do not cite this plan as complete evidence until that consistency slice is closed.

---

### Task 1: Tests for Rename and Compatibility

**Files:**
- Modify: `gormes/internal/architectureplanneragent_test.go`
- Move: `gormes/scripts/architectureplanneragent.sh`
- Create: `gormes/scripts/gormes-architecture-planner-tasks-manager.sh`
- Create wrapper: `gormes/scripts/gormes-architecture-task-manager.sh`

- [ ] **Step 1: Write the failing test**

Add assertions that copy and run `scripts/gormes-architecture-planner-tasks-manager.sh`, and also run `scripts/gormes-architecture-task-manager.sh` plus `scripts/architectureplanneragent.sh` as wrappers.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal -run TestArchitecturePlannerAgentRunsCodexuAndInstallsPeriodicTimer -count=1`

Expected: FAIL because the new script path is absent.

- [ ] **Step 3: Move implementation and add wrapper**

Move the existing script to `gormes-architecture-planner-tasks-manager.sh`. Replace `gormes-architecture-task-manager.sh` and `architectureplanneragent.sh` with `exec` wrappers that forward all arguments to the new script.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal -run TestArchitecturePlannerAgentRunsCodexuAndInstallsPeriodicTimer -count=1`

Expected: PASS.

### Task 2: Commands and Markdown Task Artifact

**Files:**
- Modify: `gormes/scripts/gormes-architecture-planner-tasks-manager.sh`
- Modify: `gormes/internal/architectureplanneragent_test.go`

- [ ] **Step 1: Write the failing test**

Add coverage for `--help`, `status`, `show-report`, `doctor`, and `.codex/planner/architecture-planner-tasks.md`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal -run 'TestArchitecturePlannerAgent|TestArchitectureTaskManager' -count=1`

Expected: FAIL until commands and `architecture-planner-tasks.md` exist.

- [ ] **Step 3: Implement command dispatch and task Markdown generation**

Add `usage`, command parsing, `write_tasks_markdown`, `cmd_status`, `cmd_show_report`, and `cmd_doctor`. Keep default `run` behavior compatible.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal -run 'TestArchitecturePlannerAgent|TestArchitectureTaskManager' -count=1`

Expected: PASS.

### Task 3: Verification and Commit

**Files:**
- Modify/Create paths from Tasks 1 and 2.

- [ ] **Step 1: Shell syntax validation**

Run: `bash -n scripts/gormes-architecture-planner-tasks-manager.sh`, `bash -n scripts/gormes-architecture-task-manager.sh`, and `bash -n scripts/architectureplanneragent.sh`.

- [ ] **Step 2: Focused Go verification**

Run: `go test ./internal -run 'TestArchitecturePlannerAgent|TestArchitectureTaskManager' -count=1`.

- [ ] **Step 3: Commit**

Run: `git add gormes/scripts/gormes-architecture-planner-tasks-manager.sh gormes/scripts/gormes-architecture-task-manager.sh gormes/scripts/architectureplanneragent.sh gormes/internal/architectureplanneragent_test.go gormes/docs/superpowers/plans/2026-04-23-gormes-architecture-task-manager-script.md && git commit -m "feat(gormes): rename architecture planner tasks manager script"`.
