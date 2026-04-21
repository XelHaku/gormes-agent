---
title: "Phase 2 — The Gateway"
weight: 30
---

# Phase 2 — The Gateway (Wiring Harness)

**Status:** 🔨 in progress

**Deliverable:** Go-native wiring harness: tools, Telegram, and thin session resume land before the wider gateway surface.

## Phase 2 Ledger

| Subphase | Status | Priority | Deliverable |
|---|---|---|---|
| Phase 2.A — Tool Registry | ✅ complete | P0 | In-process Go tool registry, streamed `tool_calls` accumulation, kernel tool loop, and doctor verification |
| Phase 2.B.1 — Telegram Scout | ✅ complete | P1 | Telegram adapter over the existing kernel, long-poll ingress, edit coalescing at the messaging edge |
| Phase 2.C — Thin Mapping Persistence | ✅ complete | P0 | bbolt-backed `(platform, chat_id) -> session_id` resume; no transcript ownership moved into Go |
| **Phase 2.E — Subagent System** | ⏳ planned | **P0** | **Execution isolation model:** spawn parallel workstreams with resource boundaries, context isolation, cancellation scopes, and failure containment. NOT a port of Python's loose process model—Gormes implements real subagents with deterministic lifecycle management |
| Phase 2.D — Cron / Scheduled Automations | ✅ complete | P2 | `internal/cron` package with `robfig/cron/v3` scheduler, bbolt `cron_jobs` bucket, SQLite `cron_runs` audit table, CRON.md mirror, Heartbeat `[SYSTEM:]` prefix + exact-match `[SILENT]` suppression, kernel `PlatformEvent.SessionID`/`CronJobID` per-event override, generic `DeliverySink` interface. Opt-in via `[cron].enabled=true` + `[telegram].allowed_chat_id`. Ship criterion proven live against Ollama (commits `e0b2fcea`…`8aa9a6e6`). Natural-language cron parsing deferred to Phase 4.C |
| Phase 2.B.2+ — Wider Gateway Surface | ⏳ planned | P1 | Additional platform adapters (Discord, Slack, WhatsApp, Signal, Email, SMS, etc.) |
| Phase 2.F — Hooks + Lifecycle | ⏳ planned | P2 | Port `gateway/hooks.py`, `builtin_hooks/`, `restart.py`, `pairing.py`, `status.py`, `mirror.py`, `sticker_cache.py`; per-event extension points and managed restarts |
| **Phase 2.G — Skills System** | ⏳ planned | **P0** | **The Learning Loop:** detect complex tasks, extract reusable patterns, save as versioned skills, improve over time. This is THE differentiation—without it, Gormes is just a chatbot with tools. See Phase 6 for the full learning loop architecture |

Phase 2.C is intentionally not Phase 3. It stores only session handles in bbolt. Python still owns transcript memory, transcript search, and prompt assembly; the SQLite + FTS5 memory lattice is Phase 3 (now substantially implemented).

> **Note on binary size:** The static CGO-free binary currently builds at **~17 MB** (measured: `bin/gormes` from `make build` with `-trimpath -ldflags="-s -w"` at commit `8aa9a6e6`, post-2.D). Phase 2.D added `robfig/cron/v3` (~20 KB) and ~1500 lines of Go across `internal/cron/`. The 3.D semantic-fusion additions (Embedder, `entity_embeddings` table, cosine scan) were absorbed within the same 17 MB envelope. Remains well within the 25 MB hard moat with ~8 MB headroom.

For channel-by-channel donor analysis against the all-Go PicoClaw repo, see [Gateway Donor Map](../gateway-donor-map/).
