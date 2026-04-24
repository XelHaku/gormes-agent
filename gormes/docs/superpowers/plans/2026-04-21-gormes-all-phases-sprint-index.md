# Gormes All-Phases Sprint Index

This index points to one sprint plan per architecture phase.

## Cross-phase queue

- Execution queue: `<repo>/gormes/docs/superpowers/plans/2026-04-22-gormes-cross-phase-execution-queue.md`

## Phase sprint files

1. Phase 1: `<repo>/gormes/docs/superpowers/plans/2026-04-21-gormes-phase1-finish-sprint.md`
2. Phase 2: `<repo>/gormes/docs/superpowers/plans/2026-04-21-gormes-phase2-finish-sprint.md`
3. Phase 3: `<repo>/gormes/docs/superpowers/plans/2026-04-21-gormes-phase3-finish-sprint.md`
4. Phase 4: `<repo>/gormes/docs/superpowers/plans/2026-04-21-gormes-phase4-finish-sprint.md`
5. Phase 5: `<repo>/gormes/docs/superpowers/plans/2026-04-21-gormes-phase5-finish-sprint.md`
6. Phase 6: `<repo>/gormes/docs/superpowers/plans/2026-04-21-gormes-phase6-finish-sprint.md`

## Global execution rule

After each slice in any phase:

```bash
cd <repo>/gormes
go test ./... -count=1
```

If red, land a regression fix before new feature work.
