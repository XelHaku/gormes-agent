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
| Phase 2.B.3 — Slack on Shared Chassis | 🔨 in progress | P1 | `internal/slack` has a Socket Mode bot, threaded reply flow, and placeholder updates; the remaining work is now split into CommandRegistry parser wiring, a `gateway.Channel` shim, then config/doctor/`cmd/gormes gateway` registration |
| Phase 2.B.4 — WhatsApp Adapter | 🔨 in progress | P1 | Transport-neutral ingress normalization and command passthrough are landed in `internal/channels/whatsapp`; bridge-first runtime selection plus pairing/reconnect/send lifecycle still remain |
| Phase 2.B.5 — Session Context + Delivery Routing | ✅ complete | P1 | Session-store handle resolution, SessionContext prompt injection, typed `--deliver` parsing, and deterministic gateway stream fan-out now live together in `internal/gateway` |
| Phase 2.B.6 — Signal Adapter | 🔨 in progress | P2 | Shared ingress normalization, session identity, and reply/send semantics are landed in `internal/channels/signal`; transport/bootstrap wiring still remains |
| Phase 2.B.7 — Email + SMS Adapters | ✅ complete | P3 | RFC 822 email normalization plus SMS number/session normalization and segmented outbound delivery contracts now ride the shared gateway seam without special-casing the kernel |
| Phase 2.B.8 — Matrix + Mattermost Adapters | 🔨 in progress | P4 | The shared threaded-text contract is landed in `internal/channels/threadtext`; Matrix and Mattermost still need their platform seams plus the real client/bootstrap layers |
| Phase 2.B.9 — Webhook + Trigger Ingress | ✅ complete | P4 | Signed ingress/auth parsing plus the typed prompt-to-delivery bridge now live together in `internal/channels/webhook`, leaving only future runtime binding work |
| Phase 2.B.10 — Regional + Device Adapter Flood | 🔨 in progress | P4 | BlueBubbles and HomeAssistant ship usable edges; DingTalk now also has a contract-tested Stream Mode bootstrap + reply retry layer; Feishu still needs transport/bootstrap plus the Drive comment rules/reply workflow, and WeCom/WeiXin plus QQ Bot still have runtime bootstraps queued |
| Phase 2.B.11 — Discord Forum Channels | ⏳ planned | P3 | Port upstream forum-channel ingress + thread lifecycle on top of the shipped 2.B.2 Discord adapter, then the forum media + outbound polish slice; baseline Discord contract must not regress |
| Phase 2.C — Thin Mapping Persistence | ✅ complete | P0 | bbolt-backed `(platform, chat_id) -> session_id` resume; no transcript ownership moved into Go |
| Phase 2.D — Cron / Scheduled Automations | ✅ complete | P2 | `internal/cron` package with `robfig/cron/v3` scheduler, bbolt `cron_jobs` bucket, SQLite `cron_runs` audit table, CRON.md mirror, Heartbeat `[SYSTEM:]` prefix + exact-match `[SILENT]` suppression, kernel `PlatformEvent.SessionID`/`CronJobID` per-event override, generic `DeliverySink` interface, plus the shipped `scripts/gormes-architecture-planner-tasks-manager.sh` operator automation that writes `.codex/planner/architecture-planner-tasks.md`, report/state artifacts, validation logs, and periodic systemd/cron scheduling. Opt-in via `[cron].enabled=true` + `[telegram].allowed_chat_id`. Ship criterion proven live against Ollama (commits `e0b2fcea`…`8aa9a6e6`). Natural-language cron parsing deferred to Phase 4.C; planner-wrapper compatibility is tracked separately in Phase 1.C |
| **Phase 2.E.0 — Deterministic Subagent Runtime** | ✅ complete | **P0** | Runtime core landed: deterministic lifecycle manager, max-depth guard, bounded batch execution, timeout/cancellation scopes, typed result envelope, `[delegation]` config, Go-native `delegate_task`, and append-only run logging |
| **Phase 2.E.1 — Delegation Policy + Child Execution** | ✅ complete | **P0** | Runner-enforced blocked-tool/allowlist policy, typed child tool-call audit, and a live Hermes child stream loop are now landed |
| Phase 2.E.2 — OS-AI Spine: Concurrent-Tool Cancellation | ⏳ planned | P1 | Kernel-side interrupt propagation that fans out one cancel across parallel `tool_calls`, sidecar sandbox jobs, and delegated subagent children; blocks parallel-tool execution and 2.F.5 mid-run steering |
| **Phase 2.G — OS-AI Spine: Skills Runtime** | ✅ complete | **P0** | Static skills runtime and the first reviewed learning-loop proof are in-tree: validated `SKILL.md` parsing, active-store snapshots, deterministic selection + prompt rendering, kernel injection, append-only usage logging, delegated candidate drafting into the inactive store, and explicit promotion into the active store. |
| Phase 2.F.1 — Slash Command Registry + Gateway Dispatch | ✅ complete | P1 | Canonical command registry now drives gateway parsing, help text, Telegram menus, and Slack subcommand exposure from one shared source of truth |
| Phase 2.F.2 — Hook Registry + BOOT.md | ✅ complete | P2 | Shared gateway lifecycle hooks, live `HOOK.yaml` command loading, and the built-in `BOOT.md` startup hook with non-blocking failure semantics are landed |
| Phase 2.F.3 — Restart / Pairing / Status | 🔨 in progress | P2 | Graceful shutdown drain is landed in `internal/gateway`; next slices are adapter startup cleanup, active-turn follow-up/late-arrival drain policy, drain-timeout resume recovery, pairing persistence, pairing approval/rate-limit semantics, a read-only status command, runtime-status JSON/PID validation, token-scoped credential locks, `/restart` takeover markers, then channel lifecycle writers |
| Phase 2.F.4 — Home Channel + Operator Surfaces | ⏳ planned | P3 | Home-channel rules, notify-to routing, manager remember-source, channel-directory persistence/lookup, refresh/stale-target invalidation, mirror surfaces, and sticker-cache equivalents |
| Phase 2.F.5 — Gateway Mid-Run Steering + Active-Turn Policy | ⏳ planned | P2 | `/steer` CommandDef + queue fallback, mid-run steer injection between tool calls, and a gateway-handled slash-command bypass of the active-session guard; depends on 2.E.2 concurrent-tool cancellation for safe mid-run fan-out |

For channel-by-channel donor analysis against the all-Go PicoClaw repo, see [Gateway Donor Map](../gateway-donor-map/).

Phase 2.C is intentionally not Phase 3. It stores only session handles in bbolt. Python still owns transcript memory, transcript search, and prompt assembly; the SQLite + FTS5 memory lattice is Phase 3 (now substantially implemented).

## TDD Priority Queue

Phase 2 is no longer just "ship more adapters." The backlog is now dominated by cross-cutting contracts that future adapters depend on. The execution order is:

1. **P1 — 2.B.3 Slack CommandRegistry parser wiring**
   Keep this first and narrow: Slack ingress should call `gateway.ParseInboundText` and shared registry helpers before any `gateway.Channel` shim or config registration hides command divergences.
2. **P1 — 2.B.3 Slack gateway.Channel shim**
   Adapt the existing Socket Mode bot to the `gateway.Channel`/`Manager` lifecycle after parser behavior is green; only then add config loading, doctor checks, and `cmd/gormes gateway` registration.
3. **P2 — 2.F.3 pairing read model + approval semantics**
   Port the XDG-backed `pairing.json` read model first, then freeze code generation, expiry, max-pending, failed-attempt lockout, and rate-limit behavior from upstream `gateway/pairing.py` before any adapter issues codes.
4. **P2 — 2.F.3 startup/drain correctness before richer lifecycle UX**
   Freeze adapter startup failure cleanup first, then define active-turn follow-up queue/late-arrival behavior and drain-timeout `resume_pending` recovery. These are upstream race fixes from `gateway/platforms/base.py` and `gateway/run.py`; keeping them separate prevents the status/pairing work from hiding concurrency regressions.
5. **P2 — 2.F.3 status readout + runtime convergence**
   Add `gormes gateway status` as a read-only view over configured channels plus pairing/runtime state, port runtime-status JSON/PID validation, then add token-scoped credential locks and `/restart` takeover/dedup markers before threading live lifecycle updates into that model.
6. **P3 — 2.F.4 Home Channel + Operator Surfaces**
   Keep this decomposed: home-channel ownership rules, notify-to delivery routing, manager remember-source, channel-directory persistence/lookup, refresh/stale-target invalidation, then mirror/sticker-cache surfaces.
7. **P1 — 2.B.4 WhatsApp runtime closeout**
   `NormalizeInbound` is landed; the next slices are bridge-first runtime selection and the pairing/reconnect/send contract on top of that ingress seam.
8. **P2/P4 — 2.B.6 plus 2.B.8 transport/bot closeout**
   Signal still needs real transport/bootstrap work; Matrix and Mattermost only have the shared threaded-text contract today, so land each platform seam before client/bootstrap code.
9. **P4 — 2.B.10 regional runtime layers**
   DingTalk now has the first real bootstrap contract but still needs real SDK binding. Feishu needs three distinct follow-ups: transport/bootstrap, Drive comment rule/pairing resolution, then Drive comment reply workflow. WeCom/WeiXin and QQ should advance as separate transport/bootstrap slices on top of their existing shared-bot seams.

The subagent runtime, shared gateway chassis, reviewed procedural skill runtime, session-context routing, and registry-backed slash command layer now exist as stable substrates. The next leverage move is freezing the operator/runtime read models, then widening adapter-specific runtime slices on top of those fixed contracts instead of letting each channel invent its own bootstrap semantics.

## Operator Automation Notes

Three docs/runtime maintenance scripts are now part of the shipped operational surface, but only the main entrypoints are stable:

- `scripts/gormes-architecture-planner-tasks-manager.sh` is the Phase 2.D planner automation entrypoint. It collects progress and architecture context, writes `.codex/planner/architecture-planner-tasks.md`, runs the required progress/doc validations, stores reports under `.codex/planner/`, and can install a periodic systemd or cron schedule.
- `scripts/documentation-improver.sh` is the documentation-maintenance runner. It builds the docs/progress context bundle, runs a Codex documentation pass, records `.codex/doc-improver` state/report/log artifacts, reports active lock owners, and runs the same progress/doc validation set.
- `scripts/landingpage-improver.sh` is the landing-page maintenance runner. It mirrors the documentation-improver contract (context bundle, Codex pass, artifacts under `.codex/`) but targets `www.gormes.ai/` content and progress-derived copy. The auto-codexu orchestrator runs it daily as a companion, gated by `LANDINGPAGE_EVERY_N_HOURS` (default 24).
- The auto-codexu orchestrator (`scripts/gormes-auto-codexu-orchestrator.sh` plus `scripts/orchestrator/`) is commit-frozen (`scripts/orchestrator/FROZEN.md`). Its audit surface writes `~/.cache/gormes-orchestrator-audit/report.csv` with per-window `tokens_estimated`/`dollars_estimated` columns, and `scripts/orchestrator/daily-digest.sh` produces a 24-hour activity summary over the same ledger. The `claudeu` shim translates codexu-style argv to `claude --print`, emits a synthetic `thread.started` event so the orchestrator captures a session id, and auto-falls back to the real `codexu` binary when Claude reports credit exhaustion or 429/quota errors so a single backend outage does not halt the loop.
- Do not call the old wrapper names stable yet. `progress.json` keeps "Planner wrapper/test consistency closeout" planned because this worktree does not contain `scripts/gormes-architecture-task-manager.sh` or `scripts/architectureplanneragent.sh`, while `internal/architectureplanneragent_test.go` still exercises wrapper compatibility.

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
