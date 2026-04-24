# Autoloop Control Plane Docs Refactor Design

## Goal

Make `cmd/autoloop` easier and more effective by turning the existing
building-gormes docs into a clearer execution control plane. The refactor
should improve worker selection, worker prompts, and operator review without
destabilizing the canonical progress file during the first pass.

## Current Shape

The canonical queue is
`docs/content/building-gormes/architecture_plan/progress.json`. It contains the
rich metadata autoloop needs: owner, size, contract, trust class, degraded
mode, fixture, source references, blockers, readiness, write scope, test
commands, done signal, and acceptance criteria.

`cmd/autoloop` currently reads that JSON directly, but normalizes each row down
to only phase, subphase, item name, and status. The generated docs already
explain the richer handoff surface, especially `agent-queue.md`, but autoloop
does not yet pass that same context to worker agents.

## Design

Use a control-plane-first refactor.

Keep `progress.json` at
`docs/content/building-gormes/architecture_plan/progress.json` for this pass.
That path is referenced by tests, scripts, generated docs, website data sync,
and progress metadata, so moving it now would create churn before the autoloop
behavior improves.

Create a focused docs section at:

`docs/content/building-gormes/autoloop/`

Move or regenerate execution-control pages there:

- `autoloop-handoff.md`
- `agent-queue.md`
- `next-slices.md`
- `blocked-slices.md`
- `umbrella-cleanup.md`
- `progress-schema.md`

Leave the architecture roadmap pages under `architecture_plan` for now. A later
URL cleanup can rename `architecture_plan` once the execution loop no longer
depends on path-specific assumptions.

Each moved page should keep a Hugo alias for its old URL where practical. The
generated text and all links should point to the new `autoloop/` location, while
the canonical progress source remains explicitly named.

## Autoloop Behavior

Update the autoloop candidate model so each selected candidate carries the
execution metadata already present in `progress.json`:

- `contract`
- `contract_status`
- `slice_size`
- `execution_owner`
- `trust_class`
- `degraded_mode`
- `fixture`
- `source_refs`
- `blocked_by`
- `ready_when`
- `not_ready_when`
- `acceptance`
- `write_scope`
- `test_commands`
- `done_signal`
- `priority`
- `unblocks`
- `note`

Selection should keep skipping complete rows. It should also avoid blocked rows
and umbrella rows by default, matching the generated agent queue. Dry-run output
should identify why a row was selected using the same priority logic as
`next-slices.md`: P0, already active, fixture-ready, unblocking, then draft.

Worker prompts should be generated from the selected row metadata, not from a
short label. The prompt should include:

- the phase, subphase, item, status, priority, owner, and size
- the contract and caller trust class
- readiness and not-ready conditions
- allowed write scope
- required test commands
- done signal and acceptance criteria
- source references
- degraded-mode expectations

Generated markdown stays the human mirror. Autoloop should continue reading
structured JSON directly, not scrape generated markdown.

## Data Flow

1. `progress.json` remains the source of truth.
2. `progress-gen` validates the schema and regenerates the docs pages under
   `building-gormes/autoloop/`.
3. `cmd/autoloop run --dry-run` loads `progress.json`, selects worker-ready
   rows, and prints selection details.
4. `cmd/autoloop run` builds one metadata-rich prompt per selected row and sends
   it to the configured backend.
5. Worker agents edit only the selected write scope, run the listed tests, and
   report against the row's done signal.

## Validation

The implementation should add or update tests for:

- generated docs paths and old URL aliases
- progress-gen output targets
- autoloop candidate normalization preserving row metadata
- blocked and umbrella rows excluded from default run selection
- dry-run output showing enough context to audit selected work
- worker prompt including write scope, tests, source refs, and done signal
- existing Hugo build and progress validation commands

Expected verification commands:

```sh
go test ./internal/progress ./internal/autoloop ./cmd/autoloop ./docs
make validate-progress
make generate-progress
```

If `make generate-progress` rewrites generated docs, the implementation should
inspect those diffs before finalizing.

## Compatibility

This pass should avoid moving the canonical `progress.json`. It should update
known references to the moved generated pages, including docs tests, sidebar
expectations, `cmd/autoloop/README.md`, `building-gormes/_index.md`, and
`meta.autoloop` paths inside `progress.json`.

Existing unstaged user edits must be preserved. Implementation should touch
only the docs control-plane files, autoloop/progress code, and tests needed for
this refactor.

## Out Of Scope

- Moving `progress.json` to a non-Hugo data directory.
- Renaming `architecture_plan` to `architecture-plan`.
- Rewriting the overall docs UI or navigation model beyond adding the autoloop
  subsection.
- Changing backend execution semantics for `codexu`, `claudeu`, or `opencode`.
- Replacing `progress.json` with a database or separate queue format.
