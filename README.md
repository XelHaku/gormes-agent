<p align="center">
  <img src="assets/gormes-agent-logo.png" alt="GORMES-AGENT" width="600">
</p>

<p align="center">
  <strong>The Go-native runtime for Hermes Agent. One binary, zero dependencies.</strong>
</p>

<p align="center">
  <a href="https://docs.gormes.ai/"><img src="https://img.shields.io/badge/Docs-docs.gormes.ai-FFD700?style=for-the-badge" alt="Documentation"></a>
  <a href="https://github.com/TrebuchetDynamics/gormes-agent"><img src="https://img.shields.io/badge/GitHub-TrebuchetDynamics%2Fgormes--agent-181717?style=for-the-badge&logo=github&logoColor=white" alt="GitHub"></a>
  <a href="https://github.com/TrebuchetDynamics/gormes-agent/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License: MIT"></a>
</p>

---

## Quick Start

```bash
# Install the single static binary (Linux/macOS/Windows)
curl -fsSL https://gormes.ai/install.sh | sh

# Start chatting
gormes
```

That's it. No Python, no virtualenv, no dependency drift.

---

## What This Is

Gormes is a complete Go rewrite of [Hermes Agent](https://github.com/NousResearch/hermes-agent)'s runtime infrastructure. While Hermes pioneered the multi-platform agent gateway (Python), Gormes moves the critical surfaces — TUI, tool registry, gateway adapters, memory layer — into a Go-native stack that deploys as a single static binary.

Gormes began as an independent Go port/rework of ideas and architecture from Hermes-Agent, with upstream Git history preserved for attribution. It is being rebuilt around a single Go binary and Gormes-native runtime boundaries.

**Current state:** Phase 2 (Gateway) is shipping now. Phase 3 (Memory) and Phase 4 (Brain) are active development.

---

## Why Go?

| Problem | How Go Solves It |
|---------|------------------|
| **Deployment friction** | Single ~17.7 MB static binary. `scp` it anywhere, it runs. |
| **Runtime reliability** | No Python GIL. Goroutines handle concurrent streams without contention. |
| **Local validation** | `gormes doctor --offline` validates tool schemas before burning tokens. |
| **Stream resilience** | Route-B reconnect treats dropped SSE as recoverable, not fatal. |

---

## Architecture

Gormes is not a wrapper around Hermes. It is a **strangler fig rewrite** — each phase ships standalone value while moving the full stack toward pure Go.

<!-- PROGRESS:START kind=readme-rollup -->
| Phase | Status | Shipped |
|-------|--------|---------|
| Phase 1 — The Dashboard | ✅ | 3/3 subphases |
| Phase 2 — The Gateway | 🔨 | 11/19 subphases |
| Phase 3 — The Black Box (Memory) | 🔨 | 11/13 subphases |
| Phase 4 — The Brain Transplant | 🔨 | 0/8 subphases |
| Phase 5 — The Final Purge | 🔨 | 1/18 subphases |
| Phase 6 — The Learning Loop (Soul) | ⏳ | 0/6 subphases |
| Phase 7 — Paused Channel Backlog | 🔨 | 2/5 subphases |
<!-- PROGRESS:END -->

Full item-level checklist and stats: **[docs.gormes.ai/building-gormes/architecture_plan](https://docs.gormes.ai/building-gormes/architecture_plan/)**

---

## Core Features

- **Single Static Binary** — Zero CGO. ~17.7 MB. Deploy to Termux, Alpine, a fresh VPS — it runs.
- **Always-On Runtime** — Survives restarts, reconnects dropped streams, runs for months unattended.
- **Multi-Platform Gateway** — Telegram and Discord run through the shared gateway today; Slack shared-runtime wiring, WhatsApp, and WeChat are the active channel priorities while the other adapters sit in Phase 7.
- **In-Process Tool Loop** — Streamed tool calls execute against a Go-native registry.
- **Scheduled Automations** — Built-in cron scheduler delivering to any platform.
- **Isolated Subagents** — Parallel workstreams with resource isolation and bounded memory.

---

## Common Commands

```bash
gormes                  # Start the TUI
gormes model            # Choose your LLM provider
gormes tools            # Configure enabled tools
gormes gateway          # Start the messaging gateway
gormes setup            # Run the full setup wizard
gormes doctor           # Validate local tool wiring
gormes claw migrate     # Migrate from OpenClaw
```

📖 **[Full documentation →](https://docs.gormes.ai/)**

---

## Documentation

| Resource | Link |
|----------|------|
| **Quick Start** | [docs.gormes.ai/getting-started/quickstart](https://docs.gormes.ai/getting-started/quickstart) |
| **CLI Reference** | [docs.gormes.ai/reference/cli-commands](https://docs.gormes.ai/reference/cli-commands) |
| **Architecture** | [docs.gormes.ai/developer-guide/architecture](https://docs.gormes.ai/developer-guide/architecture) |
| **Roadmap** | [Full architecture plan + checklist](https://docs.gormes.ai/building-gormes/architecture_plan/) |

---

## Contributing

Contributions are welcome. If you have ideas for new features, integrations, documentation improvements, or fixes, open an issue or submit a pull request.

Start here:

- [CONTRIBUTING.md](CONTRIBUTING.md) for repository contribution guidelines and PR workflow
- [Gormes developer docs](https://docs.gormes.ai/developer-guide/contributing) for setup and project-specific context

Quick start:

```bash
git clone https://github.com/TrebuchetDynamics/gormes-agent.git
cd gormes-agent
make build
./bin/gormes
```

Join the discussion and help shape the future of Gormes.

---

Built by [Trebuchet Dynamics](https://trebuchetdynamics.com/). Original Hermes Agent lineage by [Nous Research](https://nousresearch.com). MIT License.
