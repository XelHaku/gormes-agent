# Architecture Planner Loop Command

`cmd/architecture-planner-loop` improves the building-gormes architecture plan.
It is the planning counterpart to `cmd/autoloop`: autoloop executes roadmap
rows, while this command asks a planner agent to study source/reference context
and refine `docs/content/building-gormes/architecture_plan/progress.json`.

This command is the long-term architecture prompt owner for Gormes. It must stay
self-sufficient: every real run synchronizes upstream source repos, records sync
evidence in planner context, inventories the current Gormes implementation, and
prompts the planner to keep `progress.json` aligned with both upstream changes
and local implementation reality. It also owns public web alignment for the
`www.gormes.ai` landing page and the Hugo docs site under `docs/`.

Run from the repository root:

```sh
go run ./cmd/architecture-planner-loop run --dry-run
go run ./cmd/architecture-planner-loop run --codexu
go run ./cmd/architecture-planner-loop run --claudeu
go run ./cmd/architecture-planner-loop status
go run ./cmd/architecture-planner-loop show-report
```

## Sources

The planner context includes:

- `../hermes-agent`
- `../gbrain`
- `../honcho`
- `docs/content/upstream-hermes`
- `docs/content/upstream-gbrain`
- `docs/content/building-gormes`
- `www.gormes.ai`
- `docs/` (`docs/hugo.toml`, layouts, static assets, and content)

Override source paths with `HERMES_DIR`, `GBRAIN_DIR`, and `HONCHO_DIR`.

Real `run` executions synchronize the three external source repos before
building planner context:

- existing git repo: `git -C <path> pull --ff-only`
- missing repo: `git clone <url> <path>`

Default clone URLs:

- Hermes: `https://github.com/NousResearch/hermes-agent.git`
- GBrain: `https://github.com/garrytan/gbrain.git`
- Honcho: `https://github.com/plastic-labs/honcho`

Override clone URLs with `HERMES_REPO_URL`, `GBRAIN_REPO_URL`, and
`HONCHO_REPO_URL`. `PLANNER_SYNC_REPOS=0` is reserved for tests and controlled
local debugging.

Dry-run mode writes planner context and prompt artifacts without pulling or
cloning external repositories.

`context.json` records sync results from the latest real run. If Hermes,
Honcho, or GBrain moved, the planner prompt includes the `git pull`/`git clone`
evidence so the agent can add or refine TDD-ready roadmap rows instead of
allowing silent drift.

## Current Implementation Inventory

Each run also records a lightweight Gormes implementation inventory:

- command directories under `cmd/`
- internal packages under `internal/`
- top-level building-gormes docs
- `www.gormes.ai` landing page files and tests
- Hugo docs files, layouts, static assets, and content

The planner prompt uses this inventory to synchronize
`docs/content/building-gormes/architecture_plan/progress.json` with the current
implementation. If source code has advanced, the planner updates progress
status, notes, acceptance, and source references. If upstream has advanced but
Gormes has not, the planner adds or refines small execution rows for autoloop.
When roadmap or implementation changes affect public messaging, installation
flows, architecture milestones, or progress totals, the planner also updates the
landing page and Hugo docs so `www.gormes.ai`, generated docs pages, and
`progress.json` do not drift apart.

## Autoloop Handoff Quality

Planner rows must be detailed enough that autoloop workers do not need to
rediscover the architecture before starting TDD. Every new or refined executable
row should include concrete `source_refs`, `write_scope`, `test_commands`,
`acceptance`, `ready_when`, `not_ready_when`, and `done_signal` fields whenever
the schema allows them. Broad goals should be split into dependency-aware slices
with explicit `blocked_by` and `unblocks` relationships.

Prefer exact file paths, function/type names, upstream commits, fixture names,
and validation commands over generic notes. The planner may update docs,
generated pages, and progress metadata, but it must not implement runtime
feature code.

## Artifacts

By default artifacts are written under `.codex/architecture-planner/`:

- `context.json`
- `latest_prompt.txt`
- `latest_planner_report.md`
- `latest_planner_report.raw.md`
- `planner_state.json`
- `validation.log`

Override the artifact root with `RUN_ROOT`.

## Backends

The default backend is `codexu`. Use `--claudeu` only on hosts where `claudeu`
is installed.

## Validation

Real runs validate planner edits with:

```sh
go run ./cmd/progress-gen -write
go run ./cmd/progress-gen -validate
go test ./internal/progress -count=1
go test ./docs -count=1
(cd www.gormes.ai && go test ./... -count=1)
```

Set `PLANNER_VALIDATE=0` only for tests or controlled local debugging.
