---
title: "Phase 2 — The Gateway"
weight: 30
---

# Phase 2 — The Gateway (Wiring Harness)

**Status:** 🔨 in progress

**Deliverable:** Go-native operator wiring harness: tools, Telegram, shipped cron, thin session resume, then the first OS-AI spine slices before the wider adapter flood.

## Phase 2 Ledger

| Subphase | Status | Priority | Deliverable |
|---|---|---|---|
| Phase 2.A — Tool Registry | ✅ complete | P0 | In-process Go tool registry, streamed `tool_calls` accumulation, kernel tool loop, and doctor verification |
| Phase 2.B.1 — Telegram Scout | ✅ complete | P1 | Telegram adapter over the existing kernel, long-poll ingress, edit coalescing at the messaging edge |
| Phase 2.C — Thin Mapping Persistence | ✅ complete | P0 | bbolt-backed `(platform, chat_id) -> session_id` resume; no transcript ownership moved into Go |
| Phase 2.D — Cron / Scheduled Automations | ✅ complete | P2 | `internal/cron` package with `robfig/cron/v3` scheduler, bbolt `cron_jobs` bucket, SQLite `cron_runs` audit table, CRON.md mirror, Heartbeat `[SYSTEM:]` prefix + exact-match `[SILENT]` suppression, kernel `PlatformEvent.SessionID`/`CronJobID` per-event override, generic `DeliverySink` interface. Opt-in via `[cron].enabled=true` + `[telegram].allowed_chat_id`. Ship criterion proven live against Ollama (commits `e0b2fcea`…`8aa9a6e6`). Natural-language cron parsing deferred to Phase 4.C |
| **Phase 2.E — OS-AI Spine: Subagent Runtime** | 🔨 in progress | **P0** | Runtime core landed: `internal/subagent` lifecycle manager, max-depth guard, bounded batch execution, typed result envelope, `[delegation]` config, and Go-native `delegate_task`. Runner-enforced tool policy, real child LLM execution, and append-only run logging remain follow-up slices. |
| **Phase 2.G — OS-AI Spine: Skills Runtime** | ⏳ planned | **P0** | **Planned next slice:** validated `SKILL.md` loading, deterministic selection/injection, candidate drafting to an inactive store, and explicit promotion. This is the next approved OS-AI spine cut after `2.E0`, not a landed subsystem yet. |
| Phase 2.B.2+ — Gateway Chassis + Wider Gateway Surface | 🔨 in progress | P1 | Shared `internal/gateway` chassis landed; Telegram now runs on that chassis; Discord is the second real adapter; and `gormes gateway` is the multi-channel entrypoint. Remaining adapters (Slack, WhatsApp, Signal, Email, SMS, etc.) are follow-up consumers of the same contract. |
| Phase 2.F — Hooks + Lifecycle | ⏳ planned | P2 | Port `gateway/hooks.py`, `builtin_hooks/`, `restart.py`, `pairing.py`, `status.py`, `mirror.py`, `sticker_cache.py`; per-event extension points and managed restarts |

For channel-by-channel donor analysis against the all-Go PicoClaw repo, see [Gateway Donor Map](../gateway-donor-map/).

Phase 2.C is intentionally not Phase 3. It stores only session handles in bbolt. Python still owns transcript memory, transcript search, and prompt assembly; the SQLite + FTS5 memory lattice is Phase 3 (now substantially implemented).

## Current Execution Order

Phase 2 is no longer just "ship more adapters." With 2.D shipped and the 2.E runtime core now in-tree, the next concrete cuts are:

1. **2.G0 — static skill runtime**
2. **2.E1 / 2.G1-lite — reviewed vertical proof** (delegated work -> inactive candidate skill)
3. **2.B.3+ — remaining adapters on the chassis**
4. **2.F — hooks, lifecycle, pairing, restart, and managed runtime glue**

The subagent process model and the first shared gateway chassis now both exist as stable substrates. The next leverage moves are the static procedural skill runtime, then the reviewed vertical proof that ties delegation to inactive skill drafting, and only then the remaining adapters on the same channel contract.

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
