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
  autoloop runs fewer workers instead of choosing filler or random work.
- `MAX_PHASE`: cap eligible roadmap phases. Defaults to `3` so current
  unattended runs stay inside Phases 1-3. Set `0` only for an explicit
  unbounded run.
- `PRIORITY_BOOST`: comma-separated subphase IDs to pull ahead of equally ready
  work. Defaults to the active priority channels: `2.B.3,2.B.4,2.B.10,2.B.11`.

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
