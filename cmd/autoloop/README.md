# Autoloop Command

`cmd/autoloop` is the Go command for executing the building-gormes roadmap. It
is not a generic task runner and it should not maintain a private backlog.

## Control Plane

Autoloop uses the building-gormes docs tree as its development control plane:

- Canonical queue: `docs/content/building-gormes/architecture_plan/progress.json`
- Human handoff: `docs/content/building-gormes/`
- Autoloop handoff: `docs/content/building-gormes/autoloop/autoloop-handoff.md`
- Worker-ready rows: `docs/content/building-gormes/autoloop/agent-queue.md`
- Schema contract: `docs/content/building-gormes/autoloop/progress-schema.md`

`progress.json` is the machine-readable source of truth. Generated
building-gormes pages are the operator-facing explanation of the same rows.
When the command selects work, it should select from progress rows and pass row
metadata into worker prompts so worker agents develop the full `gormes-agent`
one phase slice at a time.

## Run Modes

Preview selected work without launching worker agents:

```sh
go run ./cmd/autoloop run --dry-run
```

Run the selected work through the configured backend:

```sh
go run ./cmd/autoloop run
```

Validate or regenerate the progress control plane:

```sh
go run ./cmd/autoloop progress validate
go run ./cmd/autoloop progress write
```

Record repository maintenance metadata:

```sh
go run ./cmd/autoloop repo benchmark record
go run ./cmd/autoloop repo readme update
```

Useful environment variables:

- `PROGRESS_JSON`: override the canonical progress file path.
- `RUN_ROOT`: override the autoloop runtime state/log root.
- `BACKEND`: select `codexu`, `claudeu`, or `opencode`.
- `MODE`: select `safe`, `unattended`, or `full`.
- `MAX_AGENTS`: cap selected rows for one run. If fewer rows are metadata-ready,
  autoloop runs fewer workers instead of choosing filler or random work. In a
  git checkout, selected workers run concurrently when this is greater than
  one.
- `MAX_PHASE`: cap eligible roadmap phases. Defaults to `3` so current
  unattended runs stay inside Phases 1-3. Set `0` only for an explicit
  unbounded run.
- `PRIORITY_BOOST`: comma-separated subphase IDs to pull ahead of equally ready
  work. Defaults to the active priority channels: `2.B.3,2.B.4,2.B.10,2.B.11`.
- `POST_PROMOTION_VERIFY_COMMANDS`: override the mandatory post-promotion
  full-suite gate. Separate shell commands with `;;` or newlines. Defaults to
  `go test ./... -count=1`, `www.gormes.ai` Go tests, progress validation,
  autoloop dry-run, and the site Playwright e2e suite.
- `POST_PROMOTION_REPAIR`: enable or disable the automatic repair backend after
  a failed post-promotion gate. Defaults to enabled.
- `POST_PROMOTION_REPAIR_ATTEMPTS`: number of repair attempts before the run is
  recorded as failed. Defaults to `1`.
- `AUTO_COMMIT_DIRTY_WORKTREE`: checkpoint dirty control-checkout changes
  before preflight. Defaults to enabled for CLI cycles. Autoloop stages all
  unignored changes with `git add -A` and commits them as
  `autoloop: checkpoint dirty worktree <run-id>` so the next cycle can keep
  moving. Set to `0` only when you want strict manual dirty-worktree refusal.

## Worker isolation and promotion

Each selected worker runs in its own git worktree under
`$RUN_ROOT/worktrees/<run-id>/w<worker>` on a branch named
`autoloop/<run-id>/w<worker>/<slug>` cut from the autoloop base branch. The
backend command runs with that worktree as its working directory, so worker
edits cannot dirty the control checkout while autoloop is monitoring them.
When more than one worker is selected, autoloop prepares all worker worktrees,
launches their backend commands concurrently, then validates and promotes the
finished branches in worker order. Before candidate selection, autoloop
checkpoints any dirty control-checkout changes by default, then runs the
clean-worktree preflight. The flow per worker is:

1. Pre-flight: refuse to launch if the repo has unmerged paths or, after the
   checkpoint attempt, still has uncommitted changes (emits
   `run_failed:worktree_unmerged` / `run_failed:worktree_dirty`).
2. Create a fresh worker branch in an isolated worktree and record its base
   commit.
3. Run the backend with the worker prompt (which tells the agent the branch
   name and allowed write scope).
4. Verify no merge conflicts, require the worker worktree to stay on its
   assigned branch, and require it to be clean. Dirty output emits
   `worker_failed:worktree_dirty` and the run stops for inspection without
   dirtying the base checkout.
5. Diff the worker commit against its base commit and reject any changed path
   outside the selected row's `write_scope`
   (`worker_failed:write_scope_violation`).
6. From the clean control checkout, call `PromoteWorker`: `git push origin
   <branch>`, create a review PR with `gh pr create --fill --head <branch>`,
   then land the worker commit locally with `git cherry-pick -Xtheirs
   <commit>`. If push or `gh` fails, autoloop still attempts the same local
   cherry-pick fallback. Clean successful/no-change worktrees are removed;
   failed worktrees stay in `$RUN_ROOT/worktrees/` for inspection.
7. After all worker promotions land, run the mandatory post-promotion full-suite
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

If autoloop chooses the wrong work, lacks enough worker context, or cannot tell
whether a row is ready, update the building-gormes source documents instead of
adding side-channel instructions. The expected fix path is:

1. Edit `docs/content/building-gormes/architecture_plan/progress.json`.
2. Validate or regenerate progress docs with `make validate-progress` or
   `make generate-progress`.
3. Re-run `go run ./cmd/autoloop run --dry-run` and confirm the selected rows
   match the intended phase work.

This keeps human contributors, generated docs, and autonomous workers aligned
on the same roadmap for completing `gormes-agent`.
