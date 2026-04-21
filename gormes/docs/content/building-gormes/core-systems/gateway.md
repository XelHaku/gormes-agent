---
title: "Gateway"
weight: 50
---

# Gateway

One runtime, multiple interfaces. The agent lives in the kernel; each gateway is a thin edge adapter over the same loop.

## Shipped

- **TUI** (Phase 1) — Bubble Tea interactive shell
- **Telegram adapter** (Phase 2.B.1) — long-poll ingress, 1-second edit coalescer, session resume

## Planned

- **Phase 2.B.2+** — Discord, Slack, WhatsApp, Signal, Email, SMS, Matrix, Feishu, WeChat, DingTalk, QQ, BlueBubbles, HomeAssistant, Webhook (14 more connectors). See [§7 Subsystem Inventory](../architecture_plan/subsystem-inventory/).

## Why this matters

Agents that only live in a terminal are academic. Agents that live where the operator lives — on their phone, in their team chat — are infrastructure. Gormes's split-binary-then-unified design lets each adapter ship independently without dragging the TUI's deps.

See [Phase 2](../architecture_plan/phase-2-gateway/) for the Gateway ledger.
