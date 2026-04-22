---
title: "Gateway"
weight: 50
---

# Gateway

One runtime, multiple interfaces. The agent lives in the kernel; each gateway is a thin edge adapter over the same loop.

## Shipped

- **TUI** (Phase 1) — Bubble Tea interactive shell
- **Shared gateway chassis** (Phase 2.B.2–2.B.3) — one `gormes gateway` runtime owns the manager loop, session mapping, and multi-channel boot path
- **Telegram adapter** (Phase 2.B.1) — long-poll ingress, edit coalescing, session resume
- **Discord adapter** (Phase 2.B.2) — mention-aware ingress and reply delivery on the shared chassis
- **Slack adapter** (Phase 2.B.3) — Socket Mode ingress, threaded reply flow, and shared gateway command wiring
- **slash-command registry** (Phase 2.F.1) — one canonical command registry drives parsing, help text, Telegram menus, and Slack subcommands
- **SessionContext prompt injection + delivery target parsing** (Phase 2.B.5) — stable Current Session Context block, typed `--deliver` parsing, and a deterministic Gateway stream consumer contract
- **HOOK.yaml loading + BOOT.md startup hook** (Phase 2.F.2) — live hook manifest discovery, per-event registry hooks, and non-blocking BOOT.md startup execution
- **Cron delivery bridge** (Phase 2.D) — scheduled runs, SQLite `cron_runs` audit, `CRON.md` mirror, and Heartbeat `[SYSTEM:]` / `[SILENT]` delivery rules

## Planned

- **Phase 2.B.4–2.B.10** — WhatsApp, Signal, Email, SMS, Matrix, Mattermost, Webhook, BlueBubbles, HomeAssistant, and the remaining long-tail connectors. See [§7 Subsystem Inventory](../architecture_plan/subsystem-inventory/).
- **Phase 2.F.3–2.F.4** — pairing/status, home-channel routing, channel/contact directory, mirror surfaces, and sticker-cache equivalents.

## Why this matters

Agents that only live in a terminal are academic. Agents that live where the operator lives — on their phone, in their team chat — are infrastructure. Gormes's split-binary-then-unified design lets each adapter ship independently without dragging the TUI's deps.

See [Phase 2](../architecture_plan/phase-2-gateway/) for the Gateway ledger.
For donor-code reconnaissance against PicoClaw's Go adapters, see [Gateway Donor Map](../gateway-donor-map/).
