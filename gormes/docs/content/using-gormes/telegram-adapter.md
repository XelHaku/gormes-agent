---
title: "Telegram Adapter"
weight: 40
---

# Telegram Adapter

Run Gormes as a Telegram bot. Same kernel, same tools, different edge.

## Setup

1. Create a bot with [@BotFather](https://t.me/BotFather) — get the token
2. Get your chat ID (DM [@userinfobot](https://t.me/userinfobot))
3. Launch:

```bash
GORMES_TELEGRAM_TOKEN=... GORMES_TELEGRAM_CHAT_ID=123456789 gormes telegram
```

## Behaviour

- Long-poll ingress (no webhook server needed)
- Edit coalescer: streamed tokens update the same Telegram message at ~1 Hz to avoid rate limits
- Session resume: each `(platform, chat_id)` maps to a persistent session_id via bbolt

## Multiple chats

Omit `GORMES_TELEGRAM_CHAT_ID` to respond to any chat the bot is added to. Each chat gets its own session.
