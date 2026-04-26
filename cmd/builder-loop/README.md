# Builder Loop Command

`cmd/builder-loop` is the executor side of the Planner-Builder Loop (see
`AGENTS.md` at the repo root). It executes the building-gormes roadmap.
It is not a generic task runner and it should not maintain a private
backlog.

## Control Plane

The builder loop uses the building-gormes docs tree as its development
control plane:

- Canonical queue: `docs/content/building-gormes/architecture_plan/progress.json`
- Human handoff: `docs/content/building-gormes/`
- Builder-loop handoff: `docs/content/building-gormes/builder-loop/builder-loop-handoff.md`
- Worker-ready rows: `docs/content/building-gormes/builder-loop/agent-queue.md`
- Schema contract: `docs/content/building-gormes/builder-loop/progress-schema.md`

`progress.json` is the machine-readable source of truth. Generated
building-gormes pages are the operator-facing explanation of the same rows.
When the command selects work, it should select from progress rows and pass row
metadata into worker prompts so worker agents develop the full `gormes-agent`
one phase slice at a time.

Non-dry-run builder cycles take the shared planner-loop `run.lock` before
checkpointing, PR intake, worker claims, promotions, or health writes. If the
planner loop is already regenerating the control plane, builder emits
`run_blocked:control_plane_locked` and exits before touching the queue.

## Run Modes

Preview selected work without launching worker agents:

```sh
go run ./cmd/builder-loop run --dry-run
```

Run the selected work through the configured backend:

```sh
go run ./cmd/builder-loop run
```

Validate or regenerate the progress control plane:

```sh
go run ./cmd/builder-loop progress validate
go run ./cmd/builder-loop progress write
```

Record repository maintenance metadata:

```sh
go run ./cmd/builder-loop repo benchmark record
go run ./cmd/builder-loop repo readme update
```

Useful environment variables:

- `PROGRESS_JSON`: override the canonical progress file path.
- `RUN_ROOT`: override the builder-loop runtime state/log root.
- `BACKEND`: select `codexu`, `claudeu`, or `opencode`.
- `MODE`: select `safe`, `unattended`, or `full`.
- `BUILDER_LOOP_BACKEND_TIMEOUT`: cap each worker or repair backend invocation
  so a stuck agent cannot park the infinite loop forever. Defaults to `30m`.
  The legacy `AUTOLOOP_BACKEND_TIMEOUT` is read as a fallback for back-compat.
- `MAX_AGENTS`: cap selected rows for one run. If fewer rows are metadata-ready,
  the builder loop runs fewer workers instead of choosing filler or random work. In a
  git checkout, selected workers run concurrently when this is greater than
  one.
- `MAX_PHASE`: cap eligible roadmap phases. Defaults to `4` so current
  unattended runs include the active Phase 4 queue without opening later
  phases. Set `0` only for an explicit unbounded run.
- `PRIORITY_BOOST`: comma-separated subphase IDs to pull ahead of equally ready
  work. Defaults to the active priority channels: `2.B.3,2.B.4,2.B.10,2.B.11`.
- `POST_PROMOTION_VERIFY_COMMANDS`: override the mandatory post-promotion
  full-suite gate. Separate shell commands with `;;` or newlines. Defaults to
  `go test ./... -count=1`, `www.gormes.ai` Go tests, progress validation,
  builder-loop dry-run, and the site Playwright e2e suite.
- `PRE_PROMOTION_VERIFY_COMMANDS`: optional worker-branch gate that runs inside
  the worker worktree before the worker commit is cherry-picked to the control
  checkout. Separate commands with `;;` or newlines. A failing gate keeps main
  untouched.
- `PRE_PROMOTION_REPAIR`: enable or disable the automatic repair backend after
  a failed pre-promotion gate. Defaults to enabled when pre-promotion verify
  commands are configured.
- `PRE_PROMOTION_REPAIR_ATTEMPTS`: number of worker-branch repair attempts
  before the worker is recorded as failed. Defaults to `1`.
- `POST_PROMOTION_REPAIR`: enable or disable the automatic repair backend after
  a failed post-promotion gate. Defaults to enabled.
- `POST_PROMOTION_REPAIR_ATTEMPTS`: number of repair attempts before the run is
  recorded as failed. Defaults to `1`.
- `AUTO_COMMIT_DIRTY_WORKTREE`: checkpoint dirty control-checkout changes
  before preflight. Defaults to enabled for CLI cycles. the builder loop stages all
  unignored changes with `git add -A` and commits them as
  `builder-loop: checkpoint dirty worktree <run-id>` so the next cycle can keep
  moving. Set to `0` only when you want strict manual dirty-worktree refusal.

## Speculative Execution

The builder loop supports speculative execution for improved throughput when
rows have serial dependencies. When enabled, the loop can start work on rows
whose `blocked_by` dependencies are not yet complete, provided their `ready_when`
conditions are satisfied. This enables 2-3x speedup for dependency chains.

Enable speculative execution:

```sh
GORMES_SPECULATIVE_EXECUTION=1 go run ./cmd/builder-loop run
```

Configuration:

- `GORMES_SPECULATIVE_EXECUTION`: enable speculative execution. Default: `0` (disabled).
- `GORMES_MAX_SPECULATIVE_WORKERS`: max speculative workers per run. Default: `2`.
- `GORMES_SPECULATIVE_GRACE_PERIOD`: how long to wait for blockers before aborting.
  Default: `1h`.

Safety guarantees:

1. **Spec hash verification**: Before promoting a speculative worker, the loop
   verifies the row's spec hash hasn't changed since claim. If the planner
   modified the row during speculative execution, the worker fails with
   `speculative_verify_failed`.

2. **Blocker completion check**: All `blocked_by` dependencies must complete
   successfully before promotion. If a blocker fails, the speculative worker
   aborts.

3. **Ledger tracking**: Speculative workers are tracked in `runs.jsonl` with
   `speculative: true` and `spec_hash_at_claim` for auditability.

## Worker isolation and promotion

Each selected worker runs in its own git worktree under
`$RUN_ROOT/worktrees/<run-id>/w<worker>` on a branch named
`builder-loop/<run-id>/w<worker>/<slug>` cut from the builder-loop base branch. The
backend command runs with that worktree as its working directory, so worker
edits cannot dirty the control checkout while the builder loop is monitoring them.
When more than one worker is selected, the builder loop prepares all worker worktrees,
launches their backend commands concurrently, then validates and promotes the
finished branches in worker order. Before candidate selection, the builder
loop checkpoints any dirty control-checkout changes by default, then runs
the clean-worktree preflight. The flow per worker is:

1. Pre-flight: refuse to launch if the repo has unmerged paths, after the
   checkpoint attempt still has uncommitted changes, or the current branch is
   behind its upstream (emits `run_failed:worktree_unmerged`,
   `run_failed:worktree_dirty`, or `run_failed:branch_behind_upstream`).
2. Create a fresh worker branch in an isolated worktree and record its base
   commit.
3. Run the backend with the worker prompt (which tells the agent the branch
   name and allowed write scope) under `BUILDER_LOOP_BACKEND_TIMEOUT`.
4. Verify no merge conflicts, require the worker worktree to stay on its
   assigned branch, and require it to be clean. Dirty output emits
   `worker_failed:worktree_dirty` and the run stops for inspection without
   dirtying the base checkout.
5. Diff the worker commit against its base commit and reject any changed path
   outside the selected row's `write_scope`
   (`worker_failed:write_scope_violation`).
6. When `PRE_PROMOTION_VERIFY_COMMANDS` is configured, run every verify command
   inside the worker worktree before promotion. A failure emits
   `worker_failed:pre_promotion_verify_failed`; the repair backend gets the
   full captured failure detail, commits any repair on the worker branch, and
   the gate reruns before main is touched.
7. From the clean control checkout, call `PromoteWorker`: `git push origin
   <branch>`, create a review PR with `gh pr create --fill --head <branch>`,
   then land the worker commit locally with `git cherry-pick -Xtheirs
   <commit>`. If push or `gh` fails, the builder loop still attempts the same local
   cherry-pick fallback. Clean successful/no-change worktrees are removed;
   failed worktrees stay in `$RUN_ROOT/worktrees/` for inspection.
8. After all worker promotions land, run the mandatory post-promotion full-suite
   gate before emitting `run_completed` or `health_updated`. A gate failure
   emits `post_promotion_verify_failed`, starts one repair backend by default,
   requires the repair to leave the checkout clean, reruns the full suite, and
   records final health only after the gate passes.

Each promotion attempt emits a `worker_promoted` or `worker_promotion_failed`
ledger event so the audit's `productivity` metric reflects work that actually
landed in the control checkout, not just claims that survived a backend call
or opened a PR.

When `MAX_AGENTS > 1`, candidate selection diversifies across subphases
(one slot per distinct subphase first) before stacking additional workers,
so eight workers don't all collide on the same subphase's hot files.

## Documentation Contract

If the builder loop chooses the wrong work, lacks enough worker context, or cannot tell
whether a row is ready, update the building-gormes source documents instead of
adding side-channel instructions. The expected fix path is:

1. Edit `docs/content/building-gormes/architecture_plan/progress.json`.
2. Validate or regenerate progress docs with `make validate-progress` or
   `make generate-progress`.
3. Re-run `go run ./cmd/builder-loop run --dry-run` and confirm the selected rows
   match the intended phase work.

This keeps human contributors, generated docs, and autonomous workers aligned
on the same roadmap for completing `gormes-agent`.
