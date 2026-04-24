# Gormes Phase 1 Finish Sprint

> **Execution mode:** strict TDD (`RED -> GREEN -> REFACTOR`) and atomic commits.

**Goal:** Lock Phase 1 as operationally finished: stable Dashboard (TUI + SSE bridge + Wire Doctor) with regression-proof tests and consistent docs.

**Scope:** Phase 1 only.

---

## Verified baseline

- `progress.json` Phase 1 status: **4/4 complete**.
- Phase 1 page status: `✅ complete · evolving`.
- Focused Phase-1 packages pass:

```bash
cd <repo>/gormes
go test ./cmd/gormes ./internal/tui ./internal/kernel ./internal/doctor ./docs -count=1
```

---

## Phase 1 Definition of Done

1. Focused Phase 1 gate remains green.
2. Startup health-check, SSE streaming render, and Wire Doctor have deterministic tests.
3. Docs clearly separate “complete” from “ongoing polish”.
4. No Phase 2+ changes mixed into this sprint.

---

## Slice P1-A — Regression test hardening

**Files**
- `cmd/gormes/*_test.go`
- `internal/kernel/*_test.go`
- `internal/tui/*_test.go`
- `internal/doctor/*_test.go`

**Checklist**
- [ ] Add/strengthen startup health-check contract tests.
- [ ] Add/strengthen SSE frame/render continuity tests.
- [ ] Add/strengthen doctor offline-check tests.

**Verify**
```bash
go test ./cmd/gormes ./internal/tui ./internal/kernel ./internal/doctor -count=1
```

**Commit**
`test(phase1): harden dashboard regression coverage`

---

## Slice P1-B — Runtime polish without behavior drift

**Files**
- `cmd/gormes/main.go`
- `internal/tui/*`
- `internal/kernel/*`

**Checklist**
- [ ] Keep startup/offline UX explicit and tested.
- [ ] Keep session resume behavior deterministic.
- [ ] Keep shutdown budget handling bounded and tested.

**Verify**
```bash
go test ./cmd/gormes ./internal/tui ./internal/kernel -count=1
```

**Commit**
`refactor(phase1): polish tui-kernel runtime path`

---

## Slice P1-C — Wire Doctor closeout polish

**Files**
- `cmd/gormes/doctor.go`
- `internal/doctor/*`

**Checklist**
- [ ] Deterministic, screen-reader-friendly output.
- [ ] Actionable failure text (exact next step).
- [ ] No network dependency for offline validations.

**Verify**
```bash
go test ./internal/doctor ./cmd/gormes -count=1
```

**Commit**
`feat(doctor): finalize phase1 operator diagnostics`

---

## Slice P1-D — Docs/ledger consistency

**Files**
- `docs/content/building-gormes/architecture_plan/phase-1-dashboard.md`
- `docs/content/building-gormes/architecture_plan/progress.json`
- `docs/content/building-gormes/architecture_plan/_index.md` (if regenerated)

**Checklist**
- [ ] Keep phase page + ledger wording fully consistent.
- [ ] Regenerate index page if docs pipeline requires it.

**Verify**
```bash
go test ./docs -count=1
go test ./cmd/gormes ./internal/tui ./internal/kernel ./internal/doctor ./docs -count=1
```

**Commit**
`docs(phase1): close dashboard sprint with aligned status`

---

## Global guardrail

After every slice:

```bash
go test ./cmd/gormes ./internal/tui ./internal/kernel ./internal/doctor ./docs -count=1
```

If red, stop and fix immediately in:

`fix(regression): phase1 <short-description>`
