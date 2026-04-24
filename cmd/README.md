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

Optional repo health checks:

```sh
make validate-progress
go run ./cmd/repoctl progress sync
```

`make validate-progress` is not required to run Gormes. It is a read-only check
for this repository's roadmap/progress data. The Makefile expands it to:

```sh
go run ./cmd/progress-gen -validate
```

The shape of a Go command is:

```sh
go run ./cmd/repoctl progress sync
```

Read it as: "compile and run the command in `./cmd/repoctl`, then pass it the
arguments `progress sync`." `go run` uses a temporary build; it does not install
anything.

To build real binaries instead:

```sh
go build -o bin/gormes ./cmd/gormes
go build -o bin/repoctl ./cmd/repoctl
go build -o bin/autoloop ./cmd/autoloop

./bin/gormes --offline
./bin/repoctl progress sync
./bin/autoloop run --dry-run
```

## Commands

| Command | Role | Typical invocation |
|---|---|---|
| `gormes` | User-facing runtime and TUI. Use `--offline` when you only want to see the UI without a running API server. | `go run ./cmd/gormes --offline` |
| `progress-gen` | Progress document generator. It validates the roadmap progress JSON and rewrites generated Markdown pages. | `go run ./cmd/progress-gen -validate` or `make generate-progress` |
| `repoctl` | Repository maintenance CLI. It records binary benchmarks, syncs progress mirrors, updates README benchmark text, and runs the Go 1.22 compatibility check. | `go run ./cmd/repoctl progress sync` |
| `autoloop` | Self-development orchestration CLI. It consumes the building-gormes progress/docs control plane to execute roadmap phase work. Dry-run mode only lists selected work and does not start worker agents. | `go run ./cmd/autoloop run --dry-run` |

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

Sync progress JSON mirrors without regenerating Markdown:

```sh
go run ./cmd/repoctl progress sync
```

Update README benchmark text from `benchmarks.json`:

```sh
go run ./cmd/repoctl readme update
```

Preview what autoloop would select:

```sh
go run ./cmd/autoloop run --dry-run
```

## How They Fit Together

`gormes` is the product runtime. The other commands support the repository and
the self-development loop; they should not add behavior to the user-facing
runtime unless that behavior belongs in the shipped Gormes binary.

`progress-gen` is the source-of-truth generator for progress-driven docs. It
loads the canonical architecture progress file, validates contract metadata,
and regenerates pages such as the architecture checklist, autoloop handoff,
agent queue, next slices, blocked slices, umbrella cleanup, and progress schema.

`repoctl progress sync` is narrower than `progress-gen`. It updates progress
metadata and copies progress JSON into the docs/site mirror locations used by
the build and website. It does not render Markdown pages.

`autoloop` is the executor for the building-gormes roadmap. It uses
`docs/content/building-gormes/architecture_plan/progress.json` as the canonical
candidate queue and the generated `docs/content/building-gormes/` pages as the
operator-facing handoff for developing the full `gormes-agent`. See
[`cmd/autoloop/README.md`](./autoloop/README.md) for the command-specific
contract. Its Go port currently owns the CLI surface, audit/digest/service
commands, candidate normalization, backend command construction, and typed
orchestration primitives. Full end-to-end runtime parity for `autoloop run`
remains staged follow-up work, so long-form legacy shell fixtures still live
under `testdata/legacy-shell/` as the parity oracle.

## Build Integration

The root `Makefile` wires these commands into normal contributor workflows:

```sh
make validate-progress  # go run ./cmd/progress-gen -validate
make generate-progress  # go run ./cmd/progress-gen -write
make build              # build gormes, record repo metrics, sync progress, refresh generated docs
```

Compatibility wrapper scripts under `scripts/` and `scripts/orchestrator/` exec
`repoctl` or `autoloop` during the transition. New repo-maintenance or
orchestrator behavior should be implemented in Go here first, with shell kept as
a thin wrapper only when an existing entrypoint must be preserved.
