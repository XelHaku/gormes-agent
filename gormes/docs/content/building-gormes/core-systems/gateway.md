---
title: "Gateway"
weight: 50
---

# Gateway

One runtime, multiple interfaces. The agent lives in the kernel; each gateway is a thin edge adapter over the same loop.

## Shipped

- **TUI** (Phase 1) — Bubble Tea interactive shell
- **Shared gateway chassis** (Phase 2.B.2) — one `gormes gateway` runtime owns the manager loop, session mapping, and Telegram/Discord multi-channel boot path
- **Telegram adapter** (Phase 2.B.1) — long-poll ingress, edit coalescing, session resume
- **Discord adapter** (Phase 2.B.2) — mention-aware ingress and reply delivery on the shared chassis
- **Slack Socket Mode bot** (Phase 2.B.3) — `internal/slack` has ingress, threaded reply flow, placeholder updates, and session persistence; shared `gateway.Channel` registration still remains
- **Contract-first connector wave** (Phase 2.B.4, 2.B.6–2.B.10) — WhatsApp ingress normalization, Signal/Feishu/WeCom/WeiXin/QQ shared-bot seams, the shared threaded-text contract for Matrix/Mattermost, and DingTalk's Stream Mode bootstrap + session-webhook retry layer now freeze ingress/reply behavior ahead of full transports
- **slash-command registry** (Phase 2.F.1) — one canonical command registry drives parsing, help text, Telegram menus, and Slack subcommands
- **SessionContext prompt injection + delivery target parsing** (Phase 2.B.5) — stable Current Session Context block, typed `--deliver` parsing, and a deterministic Gateway stream consumer contract
- **HOOK.yaml loading + BOOT.md startup hook** (Phase 2.F.2) — live hook manifest discovery, per-event registry hooks, and non-blocking BOOT.md startup execution
- **Cron delivery bridge** (Phase 2.D) — scheduled runs, SQLite `cron_runs` audit, `CRON.md` mirror, and Heartbeat `[SYSTEM:]` / `[SILENT]` delivery rules
- **Operator automation runners** (Phase 2.D / Phase 1.C) — `scripts/gormes-architecture-planner-tasks-manager.sh`, `scripts/documentation-improver.sh`, and `scripts/landingpage-improver.sh` now emit context bundles, reports, state files, validation logs, and verbose progress checkpoints under `.codex/`. The auto-codexu orchestrator runs them as interleaved companions (planner roughly every four cycles; doc improver every six productive cycles; landingpage improver daily), streams effectiveness audits into `~/.cache/gormes-orchestrator-audit/report.csv` with per-window token/cost estimates, and supports a 24-hour activity summary via `scripts/orchestrator/daily-digest.sh`. Wrapper compatibility and high-parallelism false-failure handling remain open under 1.C.

## Planned

- **Remaining connector runtime work** — Slack still needs CommandRegistry parser wiring, a `gateway.Channel` shim, and shared `cmd/gormes gateway` registration; WhatsApp still needs a runtime-selection seam plus pairing/reconnect/send lifecycle; Signal needs transport/bootstrap wiring; Matrix and Mattermost need both platform seams and client/bootstrap layers; Feishu still needs transport/bootstrap plus Drive comment rule/reply contracts; WeCom/WeiXin and QQ still need transport/bootstrap code; DingTalk still needs the real SDK binding. See [§7 Subsystem Inventory](../architecture_plan/subsystem-inventory/).
- **Phase 2.F.3–2.F.4** — adapter startup cleanup, active-turn follow-up/late-arrival drain policy, drain-timeout resume recovery, pairing persistence, approval/rate-limit semantics, status JSON/PID validation, token-scoped credential locks, `/restart` takeover/dedup markers, channel lifecycle writers, home-channel routing, channel/contact directory refresh, mirror surfaces, and sticker-cache equivalents.
- **Native API server replacement** — Phase 1 still consumes Python's `api_server`; Phase 5.Q must port the OpenAI-compatible chat-completions, Responses API, run-event SSE, detailed health, and cron-admin HTTP surfaces over the Go runtime before Python leaves the gateway path.

## Why this matters

Agents that only live in a terminal are academic. Agents that live where the operator lives — on their phone, in their team chat — are infrastructure. Gormes's split-binary-then-unified design lets each adapter ship independently without dragging the TUI's deps.

See [Phase 2](../architecture_plan/phase-2-gateway/) for the Gateway ledger.
For donor-code reconnaissance against PicoClaw's Go adapters, see [Gateway Donor Map](../gateway-donor-map/).
