# Command Entry Points

This directory contains runnable command folders for Gormes. You do not need to
know Go to run them, but you do need the `go` tool installed.

Run every command below from the repository root:

```sh
cd /home/xel/git/sages-openclaw/workspace-mineru/gormes-agent
```

If you are already inside this `cmd/` directory, run:

```sh
cd ..
```

## Start Here

Check that Go is available:

```sh
go version
```

This repo declares Go `1.25.0` in `go.mod`.

To run the main app UI without starting the backend:

```sh
go run ./cmd/gormes --offline
```

To preview what the autoloop would select without starting worker agents:

```sh
go run ./cmd/autoloop run --dry-run
```

To preview the architecture planner context without starting a planner agent:

```sh
go run ./cmd/architecture-planner-loop run --dry-run
```

Optional repo health checks:

```sh
make validate-progress
go run ./cmd/autoloop progress validate
```

`make validate-progress` is not required to run Gormes. It is a read-only check
for this repository's roadmap/progress data. The Makefile expands it to:

```sh
go run ./cmd/autoloop progress validate
```

The shape of a Go command is:

```sh
go run ./cmd/autoloop progress write
```

Read it as: "compile and run the command in `./cmd/autoloop`, then pass it the
arguments `progress write`." `go run` uses a temporary build; it does not
install anything.

To build real binaries instead:

```sh
go build -o bin/gormes ./cmd/gormes
go build -o bin/autoloop ./cmd/autoloop
go build -o bin/architecture-planner-loop ./cmd/architecture-planner-loop

./bin/gormes --offline
./bin/autoloop run --dry-run
./bin/autoloop progress validate
./bin/architecture-planner-loop run --dry-run
```

## Commands

| Command | Role | Typical invocation |
|---|---|---|
| `gormes` | User-facing runtime and TUI. Use `--offline` when you only want to see the UI without a running API server. | `go run ./cmd/gormes --offline` |
| `autoloop` | Self-development and repo control-plane CLI. It executes roadmap phase work, audits/digests runs, validates/regenerates progress docs, and records repo benchmark/readme metadata. | `go run ./cmd/autoloop run --dry-run` |
| `architecture-planner-loop` | Planning improvement CLI. It studies local Hermes/GBrain/Honcho sources plus upstream and building-gormes docs, then asks `codexu` or `claudeu` to refine the architecture plan and progress rows. Dry-run mode only writes planner context and prompt artifacts. | `go run ./cmd/architecture-planner-loop run --dry-run` |

## Common Recipes

Validate the canonical progress file without changing files:

```sh
make validate-progress
```

Regenerate progress-driven Markdown and site data:

```sh
make generate-progress
```

Build the main Gormes binary and run it in offline UI mode:

```sh
make build
./bin/gormes --offline
```

Update README benchmark text from `benchmarks.json`:

```sh
go run ./cmd/autoloop repo readme update
```

Preview what autoloop would select:

```sh
go run ./cmd/autoloop run --dry-run
```

Preview what the architecture planner would study:

```sh
go run ./cmd/architecture-planner-loop run --dry-run
```

## How They Fit Together

`gormes` is the product runtime. The other commands support the repository and
the self-development loop; they should not add behavior to the user-facing
runtime unless that behavior belongs in the shipped Gormes binary.

`autoloop` is the executor for the building-gormes roadmap. It uses
`docs/content/building-gormes/architecture_plan/progress.json` as the canonical
candidate queue and the generated `docs/content/building-gormes/` pages as the
operator-facing handoff for developing the full `gormes-agent`. See
[`cmd/autoloop/README.md`](./autoloop/README.md) for the command-specific
contract. It also owns progress validation/regeneration and repo-maintenance
helpers so the root `cmd/` surface stays small: `autoloop progress validate`,
`autoloop progress write`, `autoloop repo benchmark record`, and
`autoloop repo readme update`.

`architecture-planner-loop` is the planner-improvement loop. It does not execute
roadmap implementation rows. Instead it builds a context bundle from
`hermes-agent`, `gbrain`, `honcho`, `docs/content/upstream-hermes`,
`docs/content/upstream-gbrain`, and `docs/content/building-gormes`, then asks a
planner backend to refine the architecture plan and progress rows that autoloop
will later execute. See
[`cmd/architecture-planner-loop/README.md`](./architecture-planner-loop/README.md)
for the command-specific contract.

## Build Integration

The root `Makefile` wires these commands into normal contributor workflows:

```sh
make validate-progress  # go run ./cmd/autoloop progress validate
make generate-progress  # go run ./cmd/autoloop progress write
make build              # build gormes, record repo metrics, refresh generated docs
```

Compatibility wrapper scripts under `scripts/` and `scripts/orchestrator/` exec
`autoloop` during the transition. New repo-maintenance or orchestrator behavior
should be implemented in Go here first, with shell kept as a thin wrapper only
when an existing entrypoint must be preserved.
