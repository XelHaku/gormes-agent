# Gormes Phase 1 Finish Sprint (polished closeout)

> **For agentic workers:** REQUIRED: strict `test-driven-development` and atomic commits. Start every behavior change with a failing test.

**Goal:** Correct and polish the sprint plan to **finish Phase 1** cleanly: stabilize the Dashboard path (TUI + kernel SSE bridge + Wire Doctor), close doc drift, and lock a repeatable Phase 1 verification gate.

**Scope lock:** Phase 1 only. Do **not** include Phase 2/3 features in this sprint.

---

## 0) Verified baseline (today)

- `progress.json` shows **Phase 1 fully complete (4/4)**:
  - 1.A Bubble Tea shell ✅
  - 1.A 16ms mailbox ✅
  - 1.A SSE reconnect ✅
  - 1.B Wire Doctor offline validation ✅
- `phase-1-dashboard.md` status: `✅ complete · evolving`.
- Focused Phase 1 gate is green:

```bash
cd <repo>/gormes
go test ./cmd/gormes ./internal/tui ./internal/kernel ./internal/doctor ./docs -count=1
```

- Full suite currently has a non-Phase-1 flaky failure in `internal/cron` tempdir cleanup; treat as **out of scope** for this plan.

---

## 1) Phase 1 Definition of Done

Phase 1 is considered finished when all conditions hold:

1. Phase 1 focused gate stays green:
   - `go test ./cmd/gormes ./internal/tui ./internal/kernel ./internal/doctor ./docs -count=1`
2. Phase 1 docs are consistent and explicit about boundaries (done vs ongoing polish).
3. A deterministic smoke path exists for:
   - startup health check behavior,
   - SSE streaming render behavior,
   - Wire Doctor offline checks.
4. No Phase 2+ feature work is mixed into Phase 1 closeout commits.

---

## 2) Execution order (strict)

1. Slice A — Phase 1 regression-proof test hardening
2. Slice B — TUI/kernal bridge polish (no new features)
3. Slice C — Wire Doctor polish + diagnostics text
4. Slice D — Docs + progress consistency closeout

---

## Slice A — Regression-proof Phase 1 tests

**Objective:** Strengthen Phase 1 tests to prevent accidental breakage from unrelated work.

**Files (target):**
- `internal/tui/*_test.go`
- `internal/kernel/*_test.go`
- `cmd/gormes/*_test.go`
- `internal/doctor/*_test.go`

### Required outcomes
- [ ] Add/adjust tests for startup health check error message contract (`api_server not reachable ...`).
- [ ] Add/adjust tests for SSE render frame continuity assumptions (no blank final frame regressions in TUI bridge path).
- [ ] Add/adjust tests for doctor command offline checks.

### Verify
```bash
cd <repo>/gormes
go test ./cmd/gormes ./internal/tui ./internal/kernel ./internal/doctor -count=1
```

### Commit
`test(phase1): harden dashboard regression coverage`

---

## Slice B — TUI + kernel bridge polish

**Objective:** Improve reliability/readability in existing Phase 1 path without expanding scope.

**Files (target):**
- `cmd/gormes/main.go`
- `internal/tui/*`
- `internal/kernel/*`

### Required outcomes
- [ ] Keep startup/offline behavior explicit and stable.
- [ ] Keep session resume hook behavior deterministic.
- [ ] Ensure shutdown budget logic remains bounded and tested.
- [ ] Any refactor must preserve public behavior and be test-backed.

### Verify
```bash
cd <repo>/gormes
go test ./cmd/gormes ./internal/tui ./internal/kernel -count=1
```

### Commit
`refactor(phase1): polish tui-kernel bridge without behavior drift`

---

## Slice C — Wire Doctor polish

**Objective:** Finish operator-facing diagnostics quality for Phase 1 baseline.

**Files (target):**
- `cmd/gormes/doctor.go`
- `internal/doctor/*`
- `internal/doctor/*_test.go`

### Required outcomes
- [ ] Doctor output is deterministic and screen-reader friendly.
- [ ] Error text is actionable (exact missing component and next step).
- [ ] Offline validation remains independent from network/provider availability.

### Verify
```bash
cd <repo>/gormes
go test ./internal/doctor ./cmd/gormes -count=1
```

### Commit
`feat(doctor): finalize phase1 diagnostics polish`

---

## Slice D — Docs and ledger closeout

**Objective:** Ensure Phase 1 narrative and status are exact, concise, and aligned with code.

**Files:**
- `docs/content/building-gormes/architecture_plan/phase-1-dashboard.md`
- `docs/content/building-gormes/architecture_plan/progress.json`
- `docs/content/building-gormes/architecture_plan/_index.md` (if regenerated)

### Required outcomes
- [ ] `phase-1-dashboard.md` clearly states: complete, with only polish/bugfix ongoing.
- [ ] No contradictory wording between phase page and progress ledger.
- [ ] If generator is used for index pages, regenerate and verify docs tests.

### Verify
```bash
cd <repo>/gormes
go test ./docs -count=1
go test ./cmd/gormes ./internal/tui ./internal/kernel ./internal/doctor ./docs -count=1
```

### Commit
`docs(phase1): finalize dashboard closeout and consistency`

---

## Global gate (Phase 1 finish)

After every slice:

```bash
cd <repo>/gormes
go test ./cmd/gormes ./internal/tui ./internal/kernel ./internal/doctor ./docs -count=1
```

If red:
- stop feature/polish work,
- fix immediately in a dedicated commit:

`fix(regression): <phase1 breakage>`

No stacking on top of a red Phase 1 gate.
