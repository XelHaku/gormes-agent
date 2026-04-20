---
title: "TUI Mode"
weight: 30
---

# TUI Mode

The default interface. A Bubble Tea terminal shell talking to the Hermes backend over SSE.

## Launch

```bash
gormes
```

## Keybindings

| Key | Action |
|---|---|
| `Ctrl+C` | Quit |
| `Ctrl+L` | Clear output |
| `↑` / `↓` | Cycle through history |
| `Enter` (on empty input) | Cancel current turn |
| `Enter` | Send |

## Layout

The TUI coalesces streamed tokens at 16 ms (the render mailbox), so scrolling under load stays responsive. Route-B reconnect recovers dropped SSE streams without resetting the turn.

## Session resume

Each invocation reattaches to the last session via a bbolt map at `~/.gormes/sessions.db`. To start fresh: `gormes --resume new`.
