# Gormes

Gormes is the operational moat strategy for Hermes: a Go-native, single-binary agent host built for the era where runtime quality matters more than demo quality.

Phase 1 is a tactical bridge, not the final shape. Today Gormes renders a Bubble Tea Dashboard TUI and talks to Python's OpenAI-compatible `api_server` on port 8642. That bridge exists to give immediate value to existing Hermes users while the long-term target state stays fixed: a pure Go binary that owns the entire agent lifecycle.

The project thesis is simple: as models improve, operational friction becomes the bottleneck. Gormes is built to eliminate that friction.

## Install

```bash
cd gormes
make build
./bin/gormes
```

Requires Go 1.22+ and a running Python `api_server`:

```bash
API_SERVER_ENABLED=true hermes gateway start
```

## Architecture

See [`docs/ARCH_PLAN.md`](docs/ARCH_PLAN.md) for the 5-phase roadmap, the Operational Moat thesis, and the path from the temporary Python bridge to a 100% Go runtime.

## License

MIT — see `../LICENSE`.
