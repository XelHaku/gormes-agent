<p align="center">
  <img src="assets/gormes-agent-logo.png" alt="GORMES-AGENT" width="600">
</p>

<p align="center">
  <strong>A Go-native runtime for AI agents — one binary, no Python, no virtualenvs.</strong><br>
  Built to fix the reliability and deployment problems that break Python-stack agents in production.
</p>

<p align="center">
  <em>Early-stage. Built for developers who care about reliability over polish.</em>
</p>

<p align="center">
  <a href="https://docs.gormes.ai/"><img src="https://img.shields.io/badge/Docs-docs.gormes.ai-FFD700?style=for-the-badge" alt="Documentation"></a>
  <a href="https://github.com/TrebuchetDynamics/gormes-agent"><img src="https://img.shields.io/badge/GitHub-TrebuchetDynamics%2Fgormes--agent-181717?style=for-the-badge&logo=github&logoColor=white" alt="GitHub"></a>
  <a href="https://github.com/TrebuchetDynamics/gormes-agent/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License: MIT"></a>
</p>

---

> 🚧 **Under construction.** Hermes is no longer required. The Go-native runtime that replaces it is still being wired up — **Gormes is not yet usable end-to-end**. Memory and Brain phases are in active development. Expect rough edges; expect the API to change. See [Build State](#build-state) below for what works today and what doesn't.

---

## Quick Start

> The installer is the source-of-truth for trying Gormes locally. The TUI runs and the gateway adapters stream, but the agent loop is incomplete — install today to follow along, not to deploy.

**Linux / macOS / Termux:**

```bash
curl -fsSL https://gormes.ai/install.sh | sh
gormes
```

**Windows (PowerShell):**

```powershell
irm https://gormes.ai/install.ps1 | iex
gormes
```

The installer auto-installs `git` and Go 1.25+ when missing (apt/dnf/pacman/brew/pkg
on Unix, winget/choco on Windows, with a managed go.dev fallback on either) and
keeps a managed checkout under `~/.gormes` (or `%LOCALAPPDATA%\gormes`). Rerun
the same command to update — local edits in the managed checkout are autostashed
and reapplied. No Python, no virtualenv, no dependency drift.

---

## What Gormes Is

Gormes is a Go-native rewrite of [Hermes Agent](https://github.com/NousResearch/hermes-agent)'s runtime infrastructure. It started as an independent Go port of ideas and architecture from Hermes-Agent, with upstream Git history preserved for attribution, and is being rebuilt around a single static binary and Gormes-native runtime boundaries.

**Gormes solves an operations problem, not an AI problem.** The thesis isn't "smarter agents." It's agents that survive deployment, don't crash mid-stream, and don't break when a Python dependency drifts on a host you SSH into six months from now.

Hermes is no longer a runtime dependency. The Go-native pieces that replace it are still being built — see the build state below.

---

## Why Gormes Exists

> **Gormes is not about smarter agents.**
>
> It's about agents that:
> - don't fail to install
> - don't drift between environments
> - don't crash after six hours
> - don't lose work on dropped connections

### Why Hermes-stack agents break in production

- Python environments drift between dev, staging, and prod.
- npm and Nix builds break silently on host package skew.
- Multi-process Python orchestration crashes or hangs under load.
- SSE streams drop on flaky networks and kill long-running agents.
- Debugging a single failure spans Python, Node, and OS runtimes.

### How Gormes fixes it

| Problem | Gormes |
|---|---|
| **Broken installs** | Single ~17.7 MB static binary |
| **Runtime drift** | Pure Go. No `pip`, no `npm`, no `activate` |
| **Process crashes** | One runtime, one process tree |
| **Dropped SSE streams** | Route-B auto-reconnect, no lost responses |
| **3am debugging** | `gormes doctor --offline` validates locally first |

---

## Who Gormes Is For

- **Operators of long-running agents** — you need agents that survive restarts, network blips, and host upgrades, not just impressive demos.
- **Developers tired of Python/Nix/npm breakage** — you're tired of an agent that worked yesterday breaking today because a transitive dep ticked over.
- **Builders who want one binary that just runs** — you'd rather `scp` one file to a Termux session or Alpine VPS than reproduce a virtualenv.

---

## Build State

Gormes is a **strangler-fig rewrite**. Each phase ships a self-contained surface in Go and removes the corresponding Python surface from the runtime. Today the dashboard, gateway, and most of the memory layer are working in Go. The brain — the agent loop itself — is not.

| Phase | Status | What's in scope |
|---|---|---|
| **Phase 1** — The Dashboard | shipping | Go-native TUI, render mailbox, settings surfaces |
| **Phase 2** — The Gateway | partial | Telegram + Discord shipping; Slack/WhatsApp/WeChat in progress |
| **Phase 3** — The Black Box (Memory) | active | SQLite + FTS5 lattice, ontological graph, neural recall |
| **Phase 4** — The Brain Transplant | active | Native prompt building, agent orchestration in Go |
| **Phase 5** — The Final Purge | planned | Last Python tool scripts ported; 100% Go runtime |
| **Phase 6** — The Learning Loop | planned | Self-improvement loop |

<!-- PROGRESS:START kind=readme-rollup -->
| Phase | Status | Shipped |
|-------|--------|---------|
| Phase 1 — The Dashboard | ✅ | 3/3 subphases |
| Phase 2 — The Gateway | 🔨 | 12/19 subphases |
| Phase 3 — The Black Box (Memory) | 🔨 | 11/14 subphases |
| Phase 4 — The Brain Transplant | 🔨 | 0/8 subphases |
| Phase 5 — The Final Purge | 🔨 | 1/18 subphases |
| Phase 6 — The Learning Loop (Soul) | ⏳ | 0/6 subphases |
| Phase 7 — Paused Channel Backlog | 🔨 | 2/5 subphases |
<!-- PROGRESS:END -->

Full item-level checklist and stats: **[docs.gormes.ai/building-gormes/architecture_plan](https://docs.gormes.ai/building-gormes/architecture_plan/)**

---

## Core Features

- **Single Static Binary** — Zero CGO. ~17.7 MB. Deploy to Termux, Alpine, a fresh VPS — it runs. No Python, no virtualenv, no Nix.
- **No Runtime Drift** — Pure Go. The binary you tested is the binary that deploys.
- **Streams That Don't Drop** — Route-B reconnect treats SSE drops as recoverable, not fatal. Your agent doesn't lose work to a flaky network.
- **Local Validation** — `gormes doctor --offline` checks tool schemas before you burn tokens.
- **Multi-Platform Gateway** — Telegram and Discord run through the shared gateway today; Slack shared-runtime wiring, WhatsApp, and WeChat are the active channel priorities while the other adapters sit in Phase 7.
- **Scheduled Automations** — Built-in cron scheduler delivering to any platform.
- **Isolated Subagents** — Parallel workstreams with bounded memory and controlled execution.

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
