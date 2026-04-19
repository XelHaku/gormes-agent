<p align="center">
  <img src="assets/banner.png" alt="Gormes Agent" width="100%">
</p>

# Gormes Agent

<p align="center"><strong>The Agent That GOes With You.</strong></p>

<p align="center">
  <a href="https://docs.gormes.ai/"><img src="https://img.shields.io/badge/Docs-docs.gormes.ai-FFD700?style=for-the-badge" alt="Documentation"></a>
  <a href="https://github.com/TrebuchetDynamics/gormes"><img src="https://img.shields.io/badge/GitHub-TrebuchetDynamics%2Fgormes-181717?style=for-the-badge&logo=github&logoColor=white" alt="GitHub"></a>
  <a href="https://github.com/TrebuchetDynamics/gormes/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License: MIT"></a>
  <a href="https://gormes.ai"><img src="https://img.shields.io/badge/Built%20by-Trebuchet%20Dynamics-0A84FF?style=for-the-badge" alt="Built by Trebuchet Dynamics"></a>
</p>

**Built by Trebuchet Dynamics. A high-performance, 100% Go port of the original Nous Research Hermes agent.** A zero-entropy, single-binary, high-concurrency LLM gateway. Gormes takes the brilliant agentic loop of Hermes and rebuilds it in pure Go. No Python environments, no `pip install` hell, no GIL bottlenecks. Deploy a 20MB static binary that runs on a fraction of the memory footprint, from a $4 VPS to an Android phone.

Gormes carries forward the MIT-licensed Hermes lineage with a cleaner operational model: one binary, fast cold starts, lower memory pressure, and concurrency that scales without ceremony. The command surface stays familiar, the feature set stays broad, and the runtime stops fighting you.

Use any model you want — [Nous Portal](https://portal.nousresearch.com), [OpenRouter](https://openrouter.ai) (200+ models), [NVIDIA NIM](https://build.nvidia.com), [Xiaomi MiMo](https://platform.xiaomimimo.com), [z.ai/GLM](https://z.ai), [Kimi/Moonshot](https://platform.moonshot.ai), [MiniMax](https://www.minimax.io), [Hugging Face](https://huggingface.co), OpenAI, or your own endpoint. Switch with `gormes model` — no code changes, no lock-in.

<table>
<tr><td><b>A real terminal interface</b></td><td>Full TUI with multiline editing, slash-command autocomplete, conversation history, interrupt-and-redirect, and streaming tool output.</td></tr>
<tr><td><b>Lives where you do</b></td><td>Telegram, Discord, Slack, WhatsApp, Signal, and CLI from one runtime. Voice memo transcription and cross-platform conversation continuity stay intact.</td></tr>
<tr><td><b>A closed learning loop</b></td><td>Agent-curated memory with periodic nudges. Autonomous skill creation after complex tasks. Skills self-improve during use. FTS5 session search with LLM summarization for cross-session recall. <a href="https://github.com/plastic-labs/honcho">Honcho</a> dialectic user modeling. Compatible with the <a href="https://agentskills.io">agentskills.io</a> open standard.</td></tr>
<tr><td><b>Scheduled automations</b></td><td>Built-in cron scheduler with delivery to any platform. Daily reports, nightly backups, and weekly audits in natural language, running unattended.</td></tr>
<tr><td><b>Delegates and parallelizes</b></td><td>Spawn isolated subagents for parallel workstreams. Compose RPC-driven automation pipelines while the Go runtime keeps latency and memory overhead under control.</td></tr>
<tr><td><b>Runs anywhere, not just your laptop</b></td><td>Deploy the same static binary across local machines, containers, VPS instances, WSL, and Android/Termux. Serverless and remote execution patterns still fit; the packaging friction disappears.</td></tr>
<tr><td><b>Research-ready</b></td><td>Batch trajectory generation, Atropos RL environments, and trajectory compression for training the next generation of tool-calling models.</td></tr>
</table>

---

## Install

```bash
# Download the single static binary (Linux/macOS/Windows)
curl -fsSL https://gormes.ai/install.sh | sh

# Or build from source
go install github.com/TrebuchetDynamics/gormes@latest
```

No Python bootstrap, no virtualenv activation, no dependency pin fallout. Install one binary and run it.

After installation:

```bash
gormes              # start chatting
```

---

## Getting Started

```bash
gormes              # Interactive CLI — start a conversation
gormes model        # Choose your LLM provider and model
gormes tools        # Configure which tools are enabled
gormes config set   # Set individual config values
gormes gateway      # Start the messaging gateway (Telegram, Discord, etc.)
gormes setup        # Run the full setup wizard (configures everything at once)
gormes claw migrate # Migrate from OpenClaw (if coming from OpenClaw)
gormes update       # Update to the latest version
gormes doctor       # Diagnose any issues
```

📖 **[Full documentation →](https://docs.gormes.ai/)**

## Architecture

Gormes preserves the Hermes operator experience and replaces the engine room with pure Go.

- Single-binary runtime: CLI, gateway, cron, and tool orchestration ship as one deployable artifact.
- Go concurrency model: streaming turns, background jobs, gateway fan-out, and tool calls share one process without GIL contention.
- Phase 1 Strangler Fig: gateways, memory, cron, MCP, skills, and the familiar surface area remain true while the Go core takes over subsystem by subsystem.
- Drop-in command parity: `gormes setup`, `gormes doctor`, `gormes tools`, and the rest map 1:1 to the original Hermes workflows.

## CLI vs Messaging Quick Reference

Gormes has two entry points: start the terminal UI with `gormes`, or run the gateway and talk to it from Telegram, Discord, Slack, WhatsApp, Signal, or Email. Once you're in a conversation, many slash commands are shared across both interfaces.

| Action | CLI | Messaging platforms |
|---------|-----|---------------------|
| Start chatting | `gormes` | Run `gormes gateway setup` + `gormes gateway start`, then send the bot a message |
| Start fresh conversation | `/new` or `/reset` | `/new` or `/reset` |
| Change model | `/model [provider:model]` | `/model [provider:model]` |
| Set a personality | `/personality [name]` | `/personality [name]` |
| Retry or undo the last turn | `/retry`, `/undo` | `/retry`, `/undo` |
| Compress context / check usage | `/compress`, `/usage`, `/insights [--days N]` | `/compress`, `/usage`, `/insights [days]` |
| Browse skills | `/skills` or `/<skill-name>` | `/skills` or `/<skill-name>` |
| Interrupt current work | `Ctrl+C` or send a new message | `/stop` or send a new message |
| Platform-specific status | `/platforms` | `/status`, `/sethome` |

For the full command lists, see the [CLI guide](https://docs.gormes.ai/user-guide/cli) and the [Messaging Gateway guide](https://docs.gormes.ai/user-guide/messaging).

---

## Documentation

All documentation lives at **[docs.gormes.ai](https://docs.gormes.ai/)**:

| Section | What's Covered |
|---------|---------------|
| [Quickstart](https://docs.gormes.ai/getting-started/quickstart) | Install → setup → first conversation in 2 minutes |
| [CLI Usage](https://docs.gormes.ai/user-guide/cli) | Commands, keybindings, personalities, sessions |
| [Configuration](https://docs.gormes.ai/user-guide/configuration) | Config file, providers, models, all options |
| [Messaging Gateway](https://docs.gormes.ai/user-guide/messaging) | Telegram, Discord, Slack, WhatsApp, Signal, Home Assistant |
| [Security](https://docs.gormes.ai/user-guide/security) | Command approval, DM pairing, container isolation |
| [Tools & Toolsets](https://docs.gormes.ai/user-guide/features/tools) | 40+ tools, toolset system, terminal backends |
| [Skills System](https://docs.gormes.ai/user-guide/features/skills) | Procedural memory, Skills Hub, creating skills |
| [Memory](https://docs.gormes.ai/user-guide/features/memory) | Persistent memory, user profiles, best practices |
| [MCP Integration](https://docs.gormes.ai/user-guide/features/mcp) | Connect any MCP server for extended capabilities |
| [Cron Scheduling](https://docs.gormes.ai/user-guide/features/cron) | Scheduled tasks with platform delivery |
| [Context Files](https://docs.gormes.ai/user-guide/features/context-files) | Project context that shapes every conversation |
| [Architecture](https://docs.gormes.ai/developer-guide/architecture) | System design, Go runtime model, key subsystems |
| [Contributing](https://docs.gormes.ai/developer-guide/contributing) | Development setup, PR process, code style |
| [CLI Reference](https://docs.gormes.ai/reference/cli-commands) | All commands and flags |
| [Environment Variables](https://docs.gormes.ai/reference/environment-variables) | Complete env var reference |

---

## Migrating from OpenClaw

If you're coming from OpenClaw, Gormes can automatically import your settings, memories, skills, and API keys.

**During first-time setup:** The setup wizard (`gormes setup`) automatically detects `~/.openclaw` and offers to migrate before configuration begins.

**Anytime after install:**

```bash
gormes claw migrate                       # Interactive migration (full preset)
gormes claw migrate --dry-run             # Preview what would be migrated
gormes claw migrate --preset user-data    # Migrate without secrets
gormes claw migrate --overwrite           # Overwrite existing conflicts
```

What gets imported:
- **SOUL.md** — persona file
- **Memories** — MEMORY.md and USER.md entries
- **Skills** — user-created skills imported into the local Gormes skill store
- **Command allowlist** — approval patterns
- **Messaging settings** — platform configs, allowed users, working directory
- **API keys** — allowlisted secrets (Telegram, OpenRouter, OpenAI, Anthropic, ElevenLabs)
- **TTS assets** — workspace audio files
- **Workspace instructions** — AGENTS.md (with `--workspace-target`)

See `gormes claw migrate --help` for all options, or use the `openclaw-migration` skill for an interactive agent-guided migration with dry-run previews.

---

## Contributing

We welcome contributions. See the [Contributing Guide](https://docs.gormes.ai/developer-guide/contributing) for development setup, code style, and PR process.

Quick start for contributors:

```bash
git clone https://github.com/TrebuchetDynamics/gormes.git
cd gormes
make build
./bin/gormes
```

---

## Community

- 🌐 [Website](https://gormes.ai)
- 📖 [Documentation](https://docs.gormes.ai/)
- 📚 [Skills Hub](https://agentskills.io)
- 🐛 [Issues](https://github.com/TrebuchetDynamics/gormes/issues)
- 💡 [Discussions](https://github.com/TrebuchetDynamics/gormes/discussions)

---

## License

MIT — see [LICENSE](LICENSE).

Built by [Trebuchet Dynamics](https://gormes.ai). Original Hermes Agent lineage by [Nous Research](https://nousresearch.com), carried forward under the MIT License.
