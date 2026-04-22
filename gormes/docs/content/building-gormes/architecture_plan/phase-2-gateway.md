---
title: "Phase 2 — The Gateway"
weight: 30
---

# Phase 2 — The Gateway (Wiring Harness)

**Status:** 🔨 in progress

**Deliverable:** Go-native operator wiring harness: tools, Telegram, shared gateway chassis, shipped cron, and the first OS-AI spine slices before the long-tail adapter flood.

## Phase 2 Ledger

| Subphase | Status | Priority | Deliverable |
|---|---|---|---|
| Phase 2.A — Tool Registry | ✅ complete | P0 | In-process Go tool registry, streamed `tool_calls` accumulation, kernel tool loop, and doctor verification |
| Phase 2.B.1 — Telegram Scout | ✅ complete | P1 | Telegram adapter over the existing kernel, long-poll ingress, edit coalescing at the messaging edge |
| Phase 2.B.2 — Gateway Chassis + Discord | ✅ complete | P1 | Shared gateway manager, Telegram migrated onto the chassis, `gormes gateway` multi-channel entrypoint, and Discord as the second real adapter |
| Phase 2.B.3 — Slack on Shared Chassis | ✅ complete | P1 | Slack Socket Mode adapter, threaded reply flow, and gateway command wiring on the same shared contracts |
| Phase 2.B.4 — WhatsApp Adapter | ⏳ planned | P1 | Pick the bridge-vs-native runtime seam, then port inbound normalization, command passthrough, pairing, reconnect, and send contracts |
| Phase 2.B.5 — Session Context + Delivery Routing | ⏳ planned | P1 | Port `gateway/session.py`, `session_context.py`, `delivery.py`, and `stream_consumer.py` into one tested routing layer |
| Phase 2.B.6 — Signal Adapter | ✅ complete | P2 | Signal ingress, session identity, and reply/send semantics on the shared chassis |
| Phase 2.B.7 — Email + SMS Adapters | ⏳ planned | P3 | Normalize low-bandwidth ingress and outbound delivery contracts without special-casing the kernel |
| Phase 2.B.8 — Matrix + Mattermost Adapters | 🔨 in progress | P4 | Shared threaded-text contract suite landed in `internal/channels/threadtext`; Matrix + Mattermost transport wiring remains |
| Phase 2.B.9 — Webhook + Trigger Ingress | 🔨 in progress | P4 | Typed prompt-to-delivery bridge landed in `internal/channels/webhook`; signed webhook ingress and auth gates remain |
| Phase 2.B.10 — Regional + Device Adapter Flood | ✅ complete | P4 | BlueBubbles, HomeAssistant, Feishu, WeChat/WeCom, DingTalk, and QQ Bot now have contract-tested shared-gateway adapter seams |
| Phase 2.C — Thin Mapping Persistence | ✅ complete | P0 | bbolt-backed `(platform, chat_id) -> session_id` resume; no transcript ownership moved into Go |
| Phase 2.D — Cron / Scheduled Automations | ✅ complete | P2 | `internal/cron` package with `robfig/cron/v3` scheduler, bbolt `cron_jobs` bucket, SQLite `cron_runs` audit table, CRON.md mirror, Heartbeat `[SYSTEM:]` prefix + exact-match `[SILENT]` suppression, kernel `PlatformEvent.SessionID`/`CronJobID` per-event override, generic `DeliverySink` interface. Opt-in via `[cron].enabled=true` + `[telegram].allowed_chat_id`. Ship criterion proven live against Ollama (commits `e0b2fcea`…`8aa9a6e6`). Natural-language cron parsing deferred to Phase 4.C |
| **Phase 2.E.0 — Deterministic Subagent Runtime** | ✅ complete | **P0** | Runtime core landed: deterministic lifecycle manager, max-depth guard, bounded batch execution, timeout/cancellation scopes, typed result envelope, `[delegation]` config, Go-native `delegate_task`, and append-only run logging |
| **Phase 2.E.1 — Delegation Policy + Child Execution** | ✅ complete | **P0** | Runner-enforced blocked-tool/allowlist policy, typed child tool-call audit, and a live Hermes child stream loop are now landed |
| **Phase 2.G — OS-AI Spine: Skills Runtime** | ✅ complete | **P0** | Static skills runtime and the first reviewed learning-loop proof are in-tree: validated `SKILL.md` parsing, active-store snapshots, deterministic selection + prompt rendering, kernel injection, append-only usage logging, delegated candidate drafting into the inactive store, and explicit promotion into the active store. |
| Phase 2.F.1 — Slash Command Registry + Gateway Dispatch | ⏳ planned | P1 | Port `hermes_cli/commands.py` as one canonical registry that drives gateway parsing, help, and per-platform exposure |
| Phase 2.F.2 — Hook Registry + BOOT.md | ✅ complete | P2 | Shared gateway lifecycle hooks, live `HOOK.yaml` command loading, and the built-in `BOOT.md` startup hook with non-blocking failure semantics are landed |
| Phase 2.F.3 — Restart / Pairing / Status | 🔨 in progress | P2 | Graceful shutdown drain is landed at the shared manager + signal-entrypoint seam; pairing-state storage and operator status surfaces remain |
| Phase 2.F.4 — Home Channel + Operator Surfaces | ⏳ planned | P3 | Home-channel routing, channel directory, mirror surfaces, and sticker-cache equivalents |

For channel-by-channel donor analysis against the all-Go PicoClaw repo, see [Gateway Donor Map](../gateway-donor-map/).

Phase 2.C is intentionally not Phase 3. It stores only session handles in bbolt. Python still owns transcript memory, transcript search, and prompt assembly; the SQLite + FTS5 memory lattice is Phase 3 (now substantially implemented).

## TDD Priority Queue

Phase 2 is no longer just "ship more adapters." The backlog is now dominated by cross-cutting contracts that future adapters depend on. The execution order is:

1. **P1 — 2.B.5 Session Context + Delivery Routing**
   Freeze `SessionSource`, `SessionContext`, `DeliveryTarget`, and stream-consumer contracts with tests before moving any more adapters onto them.
2. **P1 — 2.F.1 Slash Command Registry + Gateway Dispatch**
   Port `COMMAND_REGISTRY` as one tested registry so `/help`, gateway parsing, Telegram menus, and Slack subcommand exposure stop drifting.
3. **P1 — 2.B.4 WhatsApp Adapter**
   Decide bridge-first versus native-first in tests, then port only the chosen lifecycle and command-passthrough seam.
4. **P2 — 2.F.3 Restart / Pairing / Status**
   Land graceful restart drain, pairing-state persistence, and operator status surfaces now that hook and subagent seams are stable.
5. **P2/P3/P4 — lifecycle and long-tail adapters**
   Home-channel surfaces, then the remaining adapters, in that order.

The subagent runtime, shared gateway chassis, and reviewed procedural skill runtime now exist as stable substrates. The next leverage move is hardening the runtime seam and the gateway routing contracts, then widening adapters on top of those fixed contracts instead of letting each adapter invent its own behavior.

## Adapter Migration Notes

To add the next channels on top of the shared chassis:

1. Implement `gateway.Channel` first: `Name()`, `Run(ctx, inbox)`, and `Send(ctx, chatID, text)`.
2. Translate SDK-native events to `gateway.InboundEvent` and normalize `/start`, `/stop`, `/new`, and unknown slash commands to `gateway.EventKind`.
3. Implement `gateway.PlaceholderCapable` + `gateway.MessageEditor` whenever the platform supports streamed edits; otherwise the manager falls back to direct final/error sends.
4. Keep session ownership in `gateway.Manager`: adapters should not own coalescers, frame consumers, or session-map persistence.
5. Add the channel's config block and wire it in `gormes gateway` plus `gormes doctor`; avoid creating another standalone one-off adapter loop unless the channel has truly unique lifecycle needs.

## Scope Guard

Honcho and the rest of the external memory-provider parity surface are **not** Phase 2 deliverables. They ride on:

- **Phase 3** — Go-native memory substrate
- **Phase 5.I** — plugin and provider parity

Do not widen the Phase 2 OS-AI spine to absorb Honcho-specific compatibility work early.

> **Note on binary size:** The static CGO-free binary currently builds at **~17 MB** (measured: `bin/gormes` from `make build` with `-trimpath -ldflags="-s -w"` at commit `8aa9a6e6`, post-2.D). Phase 2.D added `robfig/cron/v3` (~20 KB) and ~1500 lines of Go across `internal/cron/`. The 3.D semantic-fusion additions (Embedder, `entity_embeddings` table, cosine scan) were absorbed within the same 17 MB envelope. Remains well within the 25 MB hard moat with ~8 MB headroom.

For channel-by-channel donor analysis against the all-Go PicoClaw repo, see [Gateway Donor Map](../gateway-donor-map/).
