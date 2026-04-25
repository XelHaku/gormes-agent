---
title: "Phase 2 — The Gateway"
weight: 30
---

# Phase 2 — The Gateway (Wiring Harness)

**Status:** 🔨 in progress

**Deliverable:** Go-native operator wiring harness: tools, Telegram, shared gateway chassis, shipped cron, and the first OS-AI spine slices before focused channel closeout.

## Phase 2 Ledger

| Subphase | Status | Priority | Deliverable |
|---|---|---|---|
| Phase 2.A — Tool Registry | ✅ complete | P0 | In-process Go tool registry, streamed `tool_calls` accumulation, kernel tool loop, and doctor verification |
| Phase 2.B.1 — Telegram Scout | ✅ complete | P1 | Telegram adapter over the existing kernel, long-poll ingress, edit coalescing at the messaging edge |
| Phase 2.B.2 — Gateway Chassis + Discord | ✅ complete | P1 | Shared gateway manager, Telegram migrated onto the chassis, `gormes gateway` multi-channel entrypoint, and Discord as the second real adapter |
| Phase 2.B.3 — Slack on Shared Chassis | 🔨 in progress | P1 | `internal/slack` has a Socket Mode bot, threaded reply flow, placeholder updates, and shared CommandRegistry parser wiring; the remaining work is now split into a `gateway.Channel` shim, then config/doctor/`cmd/gormes gateway` registration |
| Phase 2.B.4 — WhatsApp Adapter | ✅ complete | P1 | Transport-neutral runtime selection, ingress normalization, command passthrough, identity/self-chat guards, outbound pairing gates, raw peer mapping, and bounded reconnect/send retry contracts are fixture-locked in `internal/channels/whatsapp`; live bridge/native transport startup and QR UX belong in new follow-up rows rather than the closed 2.B.4 umbrella |
| Phase 2.B.5 — Session Context + Delivery Routing | 🔨 in progress | P1 | Session-store handle resolution, baseline SessionContext prompt injection, typed `--deliver` parsing, and deterministic gateway stream fan-out now live together in `internal/gateway`; BlueBubbles/iMessage prompt guidance and non-editable progress/commentary fallback fixtures remain narrow follow-up slices |
| Phase 2.B.10 — WeChat Adapter | ✅ complete | P1 | WeCom/WeiXin shared-bot ingress, reply-path contracts, WebSocket/callback bootstrap, credential validation, and outbound push/reply lifecycle seams are landed |
| Phase 2.B.11 — Discord Forum Channels | 🔨 in progress | P3 | Forum-channel detection, parent-forum ingress routing, canonical thread IDs, and thread lifecycle gateway events are landed; Hermes `b35d692f` adds guild/parent/message source metadata as the next fixture before media, outbound polish, or Discord admin tools widen the surface |
| Phase 2.C — Thin Mapping Persistence | ✅ complete | P0 | bbolt-backed `(platform, chat_id) -> session_id` resume; no transcript ownership moved into Go |
| Phase 2.D — Cron / Scheduled Automations | ✅ complete | P2 | `internal/cron` package with `robfig/cron/v3` scheduler, bbolt `cron_jobs` bucket, SQLite `cron_runs` audit table, CRON.md mirror, Heartbeat `[SYSTEM:]` prefix + exact-match `[SILENT]` suppression, kernel `PlatformEvent.SessionID`/`CronJobID` per-event override, generic `DeliverySink` interface, plus the shipped `scripts/gormes-architecture-planner-tasks-manager.sh` operator automation that writes `.codex/planner/architecture-planner-tasks.md`, report/state artifacts, validation logs, and periodic systemd/cron scheduling. Opt-in via `[cron].enabled=true` + `[telegram].allowed_chat_id`. Ship criterion proven live against Ollama (commits `e0b2fcea`…`8aa9a6e6`). Natural-language cron parsing is deferred to Phase 4.C; planner-wrapper compatibility is now complete under Phase 1.C |
| **Phase 2.E.0 — Deterministic Subagent Runtime** | ✅ complete | **P0** | Runtime core landed: deterministic lifecycle manager, max-depth guard, bounded batch execution, timeout/cancellation scopes, typed result envelope, `[delegation]` config, Go-native `delegate_task`, and append-only run logging |
| **Phase 2.E.1 — Delegation Policy + Child Execution** | 🔨 in progress | **P0** | Runner-enforced blocked-tool/allowlist policy, typed child tool-call audit, and a live Hermes child stream loop are landed; GBrain's unified `minion-orchestrator` now adds a routing-policy slice plus a blocked durable-job ledger slice |
| Phase 2.E.2 — OS-AI Spine: Concurrent-Tool Cancellation | ✅ complete | P1 | Kernel-side interrupt propagation now fans one cancel across parallel `tool_calls`, sidecar sandbox jobs, and delegated subagent children; fixture tests freeze the cancel envelope before 2.F.5 mid-run steering |
| **Phase 2.G — OS-AI Spine: Skills Runtime** | ✅ complete | **P0** | Static skills runtime and the first reviewed learning-loop proof are in-tree: validated `SKILL.md` parsing, active-store snapshots, deterministic selection + prompt rendering, kernel injection, append-only usage logging, delegated candidate drafting into the inactive store, and explicit promotion into the active store. |
| Phase 2.F.1 — Slash Command Registry + Gateway Dispatch | ✅ complete | P1 | Canonical command registry now drives gateway parsing, help text, Telegram menus, and Slack subcommand exposure from one shared source of truth |
| Phase 2.F.2 — Hook Registry + BOOT.md | ✅ complete | P2 | Shared gateway lifecycle hooks, live `HOOK.yaml` command loading, and the built-in `BOOT.md` startup hook with non-blocking failure semantics are landed |
| Phase 2.F.3 — Restart / Pairing / Status | ✅ complete | P2 | Graceful shutdown drain, adapter startup cleanup, active-turn follow-up queuing, drain-timeout `resume_pending` recovery, pairing persistence/approval, unauthorized-DM responses, read-only `gormes gateway status`, runtime-status PID validation, token-scoped credential locks, `/restart` takeover markers, session-expiry finalization evidence, and channel lifecycle status writes are landed |
| Phase 2.F.4 — Home Channel + Operator Surfaces | ⏳ planned | P3 | Home-channel rules, notify-to routing, manager remember-source, channel-directory persistence/lookup, refresh/stale-target invalidation, mirror surfaces, and sticker-cache equivalents |
| Phase 2.F.5 — Gateway Mid-Run Steering + Active-Turn Policy | ⏳ planned | P2 | `/steer` CommandDef + queue fallback, mid-run steer injection between tool calls, and a gateway-handled slash-command bypass of the active-session guard; depends on 2.E.2 concurrent-tool cancellation for safe mid-run fan-out |

For channel-by-channel donor analysis against the all-Go PicoClaw repo, see [Gateway Donor Map](../../gateway-donor-map/).

Phase 2.C is intentionally not Phase 3. It stores only session handles in bbolt. Python still owns transcript memory, transcript search, and prompt assembly; the SQLite + FTS5 memory lattice is Phase 3 (now substantially implemented).

Signal, Email/SMS, Matrix/Mattermost, Webhook/trigger ingress, and the non-WeChat regional/device adapters are paused into Phase 7. Keep Phase 2 channel work focused on Telegram, Discord, Slack, WhatsApp, and WeChat until those paths stabilize.

## Hermes Gateway Lessons Now Imported

The Hermes gateway source shows that messaging-native agents need an explicit
active-turn contract, not just adapters that forward text. Gormes Phase 2 should
keep gateway policy in shared Go data, then let each channel consume it.

Command definitions should declare:

- canonical name and aliases;
- gateway/TUI/CLI visibility;
- argument hint;
- active-turn policy;
- trust class allowed;
- handler owner;
- platform exposure rules.

The active-turn policy should distinguish at least:

| Policy | Meaning |
|---|---|
| `reject` | visible busy response; do not interrupt or queue discardable text |
| `bypass` | safe read/control command can run while an agent is active |
| `queue` | follow-up prompt waits for the next turn |
| `interrupt` | `/stop` or replacement input cancels the active run |
| `steer` | guidance is injected after the next tool result without a new user turn |

This is the main Hermes lesson for 2.F.5: `/stop`, `/queue`, `/steer`,
approvals, status, restart, background jobs, and ordinary follow-up text need
different semantics. The command registry, not each adapter, should own those
semantics.

GBrain adds one complementary lesson: gateway-triggered work that can outlive a
single turn should enter a durable job/subagent ledger instead of depending on
one in-memory process. Its current `minion-orchestrator` skill merged the older
jobs lane and LLM subagent lane behind one policy surface. Gormes should borrow
that routing discipline first, then add the smallest SQLite-first ledger needed
for cron/subagent replay without importing the whole Minions queue.

## TDD Priority Queue

Phase 2 is no longer just "ship more adapters." The remaining backlog is dominated by cross-cutting contracts that future adapters depend on. The execution order is:

1. **P1 — 2.B.3 Slack gateway.Channel shim**
   The parser row is complete: Slack message ingress calls `gateway.ParseInboundText`, help renders `gateway.GatewayHelpLines`, and slash-command envelopes are forwarded as shared parser text. Next, adapt the existing Socket Mode bot to the `gateway.Channel`/`Manager` lifecycle without changing config registration.
2. **P2 — 2.B.3 Slack config + cmd/gormes registration**
   Register Slack in `cmd/gormes gateway` only after the Channel shim is green; config and doctor/status tests should use fake Slack clients and keep Telegram/Discord-only startup unchanged.
3. **P3 — 2.B.11 Discord SessionSource metadata**
   Port Hermes `b35d692f` source fields (`guild_id`, `parent_chat_id`, `message_id`) through `InboundEvent`, `SessionSource`, and session-context rendering before media polish or Discord tool/admin rows rely on current-server/current-message IDs. This is source metadata only; keep send behavior and REST/tool handlers out of the slice.
4. **P3 — 2.F.4 Home Channel + Operator Surfaces**
   Keep this decomposed: home-channel ownership rules, notify-to delivery routing, manager remember-source, channel-directory persistence/lookup, refresh/stale-target invalidation, then mirror/sticker-cache surfaces.
5. **P1 — closed 2.B.4 WhatsApp contract closeout**
   `NormalizeInbound`, `DecideRuntime`, identity/self-chat guards, outbound pairing gates, raw peer mapping, and reconnect/send retry contracts are landed. Do not reopen the umbrella row for live bridge/native startup or QR UX; add a new small Phase 7/backlog row with explicit transport scope if upstream drift requires it.
6. **P7 — paused channel backlog**
   Signal, Email/SMS, Matrix/Mattermost, Webhook, BlueBubbles/HomeAssistant, Feishu, DingTalk, and QQ stay in Phase 7 until Telegram, Discord, Slack, WhatsApp, and WeChat are stable. Keep the backlog split into platform seams before client/bootstrap code: Signal transport, Matrix/Mattermost bot seams and clients, Feishu transport plus Drive-comment rules/replies, DingTalk real SDK binding, and QQ transport/bootstrap.

The subagent runtime, shared gateway chassis, reviewed procedural skill runtime, session-context routing, registry-backed slash command layer, and operator/runtime read models now exist as stable substrates. The next leverage move is closing the priority channel set and the home/operator surfaces before widening adapter-specific runtime slices.

## Operator Automation Notes

Three docs/runtime maintenance scripts are now part of the shipped operational surface, with the planner manager plus its compatibility wrappers covered by tests:

- `scripts/gormes-architecture-planner-tasks-manager.sh` is the Phase 2.D planner automation entrypoint. It collects progress and architecture context, writes `.codex/planner/architecture-planner-tasks.md`, runs the required progress/doc validations, stores reports under `.codex/planner/`, and can install a periodic systemd or cron schedule.
- `scripts/documentation-improver.sh` is the documentation-maintenance runner. It builds the docs/progress context bundle, runs a Codex documentation pass, records `.codex/doc-improver` state/report/log artifacts, reports active lock owners, and runs the same progress/doc validation set.
- `scripts/landingpage-improver.sh` is the landing-page maintenance runner. It mirrors the documentation-improver contract (context bundle, Codex pass, artifacts under `.codex/`) but targets `www.gormes.ai/` content and progress-derived copy. The auto-codexu orchestrator runs it daily as a companion, gated by `LANDINGPAGE_EVERY_N_HOURS` (default 24).
- The auto-codexu orchestrator (`scripts/gormes-auto-codexu-orchestrator.sh` plus `scripts/orchestrator/`) is commit-frozen (`scripts/orchestrator/FROZEN.md`). Its Go audit writes staged cursor/report artifacts with minimal ledger counts under `~/.cache/gormes-orchestrator-audit/`, and `scripts/orchestrator/daily-digest.sh` produces a 24-hour activity summary over the same ledger. Legacy service-health, integration-head, companion-status, and token/cost audit telemetry are not yet reproduced in the Go audit. The `claudeu` shim translates codexu-style argv to `claude --print`, emits a synthetic `thread.started` event so the orchestrator captures a session id, and auto-falls back to the real `codexu` binary when Claude reports credit exhaustion or 429/quota errors so a single backend outage does not halt the loop.
- Legacy planner wrapper names are stable compatibility shims: `scripts/gormes-architecture-task-manager.sh` and `scripts/architectureplanneragent.sh` exec the planner tasks manager and are covered by `internal/architectureplanneragent_test.go`.

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

For channel-by-channel donor analysis against the all-Go PicoClaw repo, see [Gateway Donor Map](../../gateway-donor-map/).
