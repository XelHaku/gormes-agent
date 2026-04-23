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
| `${XDG_DATA_HOME}/gormes/sessions.db` | bbolt session resume map |
| `${XDG_DATA_HOME}/gormes/memory.db` | SQLite memory store |
| `${XDG_DATA_HOME}/gormes/memory/USER.md` | Human-readable entity/relationship mirror |
| `${XDG_DATA_HOME}/gormes/auth.json` | Shared token vault for persisted provider credentials |
| `${XDG_DATA_HOME}/gormes/auth/` | Per-provider OAuth/device token files such as `google_oauth.json` |
| `${XDG_DATA_HOME}/gormes/crash-*.log` | Crash dumps |

Vault-backed credentials are loaded after `config.toml` account selection and before env/CLI overrides, so persisted tokens work by default while `GORMES_API_KEY` and other runtime overrides still win when set.
