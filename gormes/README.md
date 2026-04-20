# Gormes

Gormes is the operational moat strategy for Hermes: a tool-capable Go-native agent host for the era where runtime quality matters more than demo quality. The current `cmd/gormes` build fits in a 7.9 MB static binary built with Go 1.22+, using a zero-CGO, Zero-dependencies inside the process boundary stack: no Python runtime, no Node runtime, and no per-host dependency stack once the binary is built.

Today Gormes is more than a Phase-1 shell. The tree ships a Go-native tool registry, Route-B reconnect, a split-binary Telegram Scout alongside the TUI, and thin bbolt-backed session resume while still interoperating with Python's OpenAI-compatible `api_server` on port 8642. Python still owns transcript memory and prompt assembly until the later roadmap phases land.

## Quick Start

Start the existing Hermes backend:

```bash
API_SERVER_ENABLED=true hermes gateway start
```

Build the Go binaries:

```bash
cd gormes
make build
```

Validate the local tool wiring before spending a cent on API traffic:

```bash
./bin/gormes doctor --offline
```

Run the TUI:

```bash
./bin/gormes
```

Run the Telegram Scout:

```bash
GORMES_TELEGRAM_TOKEN=... GORMES_TELEGRAM_CHAT_ID=123456789 ./bin/gormes-telegram
```

## Architectural Edge

- **Wire Doctor** — `gormes doctor` validates the local Go-native tool registry and schema shape before a live turn burns tokens.
- **Go-native tool loop** — streamed `tool_calls` are accumulated in Go, executed against the in-process registry, and fed back into the turn loop without bouncing tool execution through Python.
- **Route-B reconnect** — dropped SSE streams are treated as a resilience problem to solve, not a happy-path omission to ignore.
- **16 ms coalescing mailbox** — the kernel uses a replace-latest render mailbox so stalled consumers do not trigger a thundering herd of stale frames.
- **Split-binary Telegram Scout** — the messaging edge stays isolated from the 7.9 MB TUI binary, with a 1-second Telegram edit coalescer and thin bbolt-backed session resume.

## Further Reading

- [Why Gormes](docs/content/why-gormes.md)
- [Executive Roadmap](docs/ARCH_PLAN.md)
- [Phase 2.A — Tool Registry](docs/superpowers/specs/2026-04-19-gormes-phase2-tools-design.md)
- [Phase 2.B.1 — Telegram Scout](docs/superpowers/specs/2026-04-19-gormes-phase2b-telegram.md)
- [Phase 2.C — Thin Mapping Persistence](docs/superpowers/specs/2026-04-19-gormes-phase2c-persistence-design.md)

## License

MIT — see `../LICENSE`.
