---
title: "Quickstart"
weight: 10
---

# Quickstart

Get Gormes running in 60 seconds.

## 1. Install

```bash
curl -fsSL https://gormes.ai/install.sh | sh
```

Installs `gormes` into `$HOME/go/bin` via `go install`. Requires Go 1.25+. For other install paths see [Install](../install/).

## 2. Bring up the Hermes backend

Gormes is a Go shell that talks to Hermes over HTTP. You need Hermes running on `localhost:8642` first:

```bash
curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash
API_SERVER_ENABLED=true hermes gateway start
```

## 3. Verify the local stack

```bash
gormes doctor --offline
```

See [Wire Doctor](../wire-doctor/) for what this checks.

## 4. Run

```bash
gormes
```

You're in the TUI. Press `Ctrl+C` to exit.

## Next

- [TUI mode](../tui-mode/) — keybindings, layout
- [Telegram adapter](../telegram-adapter/) — use the same brain from Telegram
- [Configuration](../configuration/) — persistent settings
