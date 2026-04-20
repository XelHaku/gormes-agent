---
title: "Configuration"
weight: 50
---

# Configuration

Gormes reads config from TOML files, env vars, and CLI flags — in that precedence.

## Config files

| Path | Purpose |
|---|---|
| `$XDG_CONFIG_HOME/gormes/config.toml` | User-level defaults |
| `./gormes.toml` | Project-local overrides (checked into the repo you're working in) |

Example:

```toml
[hermes]
endpoint = "http://127.0.0.1:8642"
api_key = ""
model = "claude-4-sonnet"

[input]
max_bytes = 65536
max_lines = 500
```

## Env vars

| Var | Purpose |
|---|---|
| `GORMES_HERMES_ENDPOINT` | Override Hermes backend URL |
| `GORMES_HERMES_API_KEY` | Hermes auth token |
| `GORMES_TELEGRAM_TOKEN` | Telegram bot token |
| `GORMES_TELEGRAM_CHAT_ID` | Telegram chat ID (optional) |

## State directories

| Path | Contents |
|---|---|
| `~/.gormes/sessions.db` | bbolt session resume map |
| `~/.hermes/memory/memory.db` | SQLite memory store |
| `~/.hermes/memory/USER.md` | Human-readable entity/relationship mirror |
| `~/.hermes/crash-*.log` | Crash dumps |
