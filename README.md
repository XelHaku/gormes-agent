<p align="center">
  <img src="assets/gormes-agent-logo.png" alt="GORMES-AGENT" width="600">
</p>

# GORMES-AGENT

A Go-native runtime for AI agents.

A single static binary for the Gormes runtime surface. No Python inside the shipped binary. No virtualenvs.

Built to fix the reliability and deployment problems that break Python-stack agents in production.

**Early-stage. Not production-ready yet.** Live turns still need a Hermes-compatible backend while the Go-native brain is being built.

<p align="center">
  <a href="https://docs.gormes.ai/"><img src="https://img.shields.io/badge/Docs-docs.gormes.ai-FFD700?style=for-the-badge" alt="Documentation"></a>
  <a href="https://github.com/TrebuchetDynamics/gormes-agent"><img src="https://img.shields.io/badge/GitHub-TrebuchetDynamics%2Fgormes--agent-181717?style=for-the-badge&logo=github&logoColor=white" alt="GitHub"></a>
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License: MIT">
</p>

---

## Quick Start

Try the local TUI and diagnostics first.

### Unix (Linux / macOS / Termux)

```bash
curl -fsSL https://gormes.ai/install.sh | sh
gormes --offline
gormes doctor --offline
```

### Windows (PowerShell)

```powershell
irm https://gormes.ai/install.ps1 | iex
gormes --offline
gormes doctor --offline
```

The installer manages a source checkout under `~/.gormes/gormes-agent` or `%LOCALAPPDATA%\gormes\gormes-agent`, installs Git and Go when missing where possible, builds `gormes`, and updates in place on rerun.

For live turns today, start a Hermes-compatible backend and run `gormes` without `--offline`:

```bash
API_SERVER_ENABLED=true hermes gateway start
gormes
```

---

## Core Features

- **Single static binary** - current Gormes build is ~17.7 MB, stripped, static, and zero-CGO.
- **No Gormes runtime drift** - the Go binary you test is the Go binary you run.
- **Stream resilience** - Route-B reconnect treats dropped SSE streams as recoverable.
- **Local validation** - `gormes doctor --offline` catches tool and config issues before runtime.
- **Multi-platform gateway** - Telegram and Discord ship on the shared gateway; Slack, WhatsApp, and WeChat are active.
- **Isolated subagents** - bounded parallel workstreams with durable job metadata.
- **Goncho memory layer** - Honcho-style peer context, search, profiles, and diagnostics inside the Gormes binary.

---

## Why Gormes Exists

Gormes is not about smarter agents.

It is about agents that:

- do not fail to install
- do not drift between environments
- do not crash mid-run
- do not lose work on dropped connections

### Why Python-stack agents break

- Python environments drift between dev, staging, and prod.
- npm and Nix builds break on host package skew.
- Multi-process orchestration crashes or hangs under load.
- SSE streams drop and kill long-running turns.
- Debugging spans Python, Node, shell, and OS runtimes.

Gormes fixes this by:

- Single static binary -> fewer broken installs
- Pure Go runtime surfaces -> no `pip`, no `npm`, no `activate`
- Route-B reconnect -> dropped streams become recoverable events
- Local doctor checks -> issues fail before tokens burn
- In-binary memory and gateway seams -> less cross-runtime debugging

---

## Build State

Gormes is a strangler-fig rewrite of Hermes-Agent, with upstream Git history preserved for attribution.

Today:

- Dashboard: shipping
- Gateway: partial
- Memory: active
- Brain: not complete
- Live turns: still require a Hermes-compatible backend

Next milestone:

- Fully Go-native agent runtime with no Hermes backend requirement.

Full progress: [docs.gormes.ai/building-gormes/architecture_plan](https://docs.gormes.ai/building-gormes/architecture_plan/)

<details>
<summary>Generated phase rollup</summary>

<!-- PROGRESS:START kind=readme-rollup -->
| Phase | Status | Shipped |
|-------|--------|---------|
| Phase 1 — The Dashboard | 🔨 | 2/3 subphases |
| Phase 2 — The Gateway | 🔨 | 16/20 subphases |
| Phase 3 — The Black Box (Memory) | ✅ | 14/14 subphases |
| Phase 4 — The Brain Transplant | 🔨 | 0/8 subphases |
| Phase 5 — The Final Purge | 🔨 | 1/18 subphases |
| Phase 6 — The Learning Loop (Soul) | ⏳ | 0/6 subphases |
| Phase 7 — Paused Channel Backlog | 🔨 | 2/5 subphases |
<!-- PROGRESS:END -->

</details>

---

## Goncho (Honcho -> Go)

Gormes includes Goncho: an in-binary Go port of Honcho's peer-centric memory and context model.

Goncho is not a sidecar, second database, or loopback service. It runs inside the Gormes binary on the same SQLite memory substrate and exposes Honcho-compatible tools:

- `honcho_profile`
- `honcho_search`
- `honcho_context`
- `honcho_chat`
- `honcho_reasoning`
- `honcho_conclude`

This gives Gormes a local memory layer for peer profiles, session context, retrieval, conclusions, queue status, and degraded-mode diagnostics.

Docs: [Goncho Honcho Memory](https://docs.gormes.ai/building-gormes/goncho_honcho_memory/)

---

## Who Gormes Is For

- **Operators of long-running agents** - systems that must survive restarts, flaky networks, and host changes.
- **Developers tired of Python/Nix/npm breakage** - environments that worked yesterday and fail today.
- **Builders who want one deployable artifact** - ship a Go binary instead of reconstructing a runtime.

---

## Basic Usage

```bash
gormes --offline
```

Use `gormes` without `--offline` when a Hermes-compatible backend is running.

More commands: [cmd/README.md](cmd/README.md)

---

## Documentation

- [Quickstart](https://docs.gormes.ai/using-gormes/quickstart/)
- [Install](https://docs.gormes.ai/using-gormes/install/)
- [Configuration](https://docs.gormes.ai/using-gormes/configuration/)
- [Core systems](https://docs.gormes.ai/building-gormes/core-systems/)
- [Architecture plan](https://docs.gormes.ai/building-gormes/architecture_plan/)
- [Goncho Honcho Memory](https://docs.gormes.ai/building-gormes/goncho_honcho_memory/)

---

## Contributing

Contributions are welcome. Build the binary and run the offline UI first:

```bash
git clone https://github.com/TrebuchetDynamics/gormes-agent.git
cd gormes-agent
make build
./bin/gormes --offline
```

Contributor roadmap: [Building Gormes](https://docs.gormes.ai/building-gormes/)

---

Built by [Trebuchet Dynamics](https://trebuchetdynamics.com/). Original Hermes Agent lineage by [Nous Research](https://nousresearch.com). MIT License.
