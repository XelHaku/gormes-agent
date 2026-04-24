# Gormes

Gormes is the Go operator console for Hermes — a single static binary that boots fast, runs tools in-process, and treats dropped SSE streams as a resilience problem instead of a happy-path omission. No Python runtime, no Node runtime, no per-host dependency stack once the binary is built.

The current `cmd/gormes` build is a zero-CGO static binary built with Go 1.25+. A single binary ships the TUI, the Telegram bot adapter (`gormes telegram`), and the Wire Doctor (`gormes doctor`). Gormes still interoperates with Hermes' OpenAI-compatible `api_server` on port 8642 — transcript memory still mirrors upstream, while prompt assembly now runs natively in the Go kernel.

## Install

Quick install (source-backed today; requires Go 1.25+ on PATH):

```bash
curl -fsSL https://gormes.ai/install.sh | sh
# if the installer printed an export PATH line, run it in this shell now
gormes doctor --offline
gormes
```

The installer currently wraps:

```bash
go install github.com/TrebuchetDynamics/gormes-agent/gormes/cmd/gormes@latest
```

Native Windows is not supported — install WSL2 and run inside it.

## Build From Source

```bash
git clone https://github.com/TrebuchetDynamics/gormes-agent
cd gormes-agent/gormes
make build
./bin/gormes doctor --offline
./bin/gormes
```

This produces a single static binary at `./bin/gormes` (CGO disabled, trimpath, stripped).

## Quick Start

Start the Hermes backend (upstream Python, separate repo):

```bash
API_SERVER_ENABLED=true hermes gateway start
```

Validate the local tool wiring before spending a cent on API traffic:

```bash
gormes doctor --offline        # or: ./bin/gormes doctor --offline if built locally
```

Run the TUI:

```bash
gormes                        # or: ./bin/gormes
```

Run the Telegram bot adapter:

```bash
GORMES_TELEGRAM_TOKEN=... GORMES_TELEGRAM_CHAT_ID=123456789 gormes telegram
```

## Architectural Edge

- **Wire Doctor** — `gormes doctor` validates the local Go-native tool registry and schema shape before a live turn burns tokens.
- **Go-native tool loop** — streamed `tool_calls` are accumulated in Go, executed against the in-process registry, and fed back into the turn loop without bouncing tool execution through Python.
- **Route-B reconnect** — dropped SSE streams are treated as a resilience problem to solve, not a happy-path omission to ignore.
- **16 ms coalescing mailbox** — the kernel uses a replace-latest render mailbox so stalled consumers do not trigger a thundering herd of stale frames.
- **Unified binary with isolated adapters** — TUI, Telegram adapter, and doctor all ship in one `gormes` binary with a subcommand per edge, so deployment stays simple while each adapter keeps its own dependency surface.
- **Thin bbolt session resume** — `gormes` persists the active `session_id` through a bbolt map so a crash or reboot reattaches to the same Hermes session instead of starting a new one.

## Architecture

<!-- PROGRESS:START kind=readme-rollup -->
| Phase | Status | Shipped |
|-------|--------|---------|
| Phase 1 — The Dashboard | ✅ | 2/2 subphases |
| Phase 2 — The Gateway | ✅ | 20/20 subphases |
| Phase 3 — The Black Box (Memory) | ✅ | 13/13 subphases |
| Phase 4 — The Brain Transplant | ✅ | 8/8 subphases |
| Phase 5 — The Final Purge | 🔨 | 15/17 subphases |
| Phase 6 — The Learning Loop (Soul) | 🔨 | 5/6 subphases |
<!-- PROGRESS:END -->

## Landing Page

The public site at `gormes.ai` is a Go-rendered landing page living under `www.gormes.ai/`. To hack on it:

```bash
cd www.gormes.ai
make run
```

The install script served at `https://gormes.ai/install.sh` is embedded from `www.gormes.ai/internal/site/install.sh`.

## Further Reading

- [Why Gormes](docs/content/why-gormes.md)
- [Executive Roadmap](docs/ARCH_PLAN.md)
- [Phase 2.A — Tool Registry](docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md)
- [Phase 2.B.1 — Telegram Scout](docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md)
- [Phase 2.C — Thin Mapping Persistence](docs/superpowers/specs/2026-04-19-gormes-phase2c-persistence-design.md)

## License

MIT — see `../LICENSE`.
