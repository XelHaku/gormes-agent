# Gormes — Executive Roadmap (ARCH_PLAN)

**Public site:** https://gormes.ai
**Source:** https://github.com/XelHaku/golang-hermes-agent
**Upstream reference:** https://github.com/NousResearch/hermes-agent

---

## 0. Operational Moat Thesis

When intelligence becomes abundant, operational friction becomes the bottleneck.

That is the reason Gormes exists.

If models keep improving, the differentiator stops being whether an agent can produce a clever answer and starts being whether the system can stay alive, recover fast, deploy cleanly, and run everywhere without constant babysitting. Gormes is built for that era.

The strategic target is not "a Go wrapper around Hermes." The strategic target is a pure Go binary that owns the full lifecycle of a serious always-on agent.

---

## 1. Rosetta Stone Declaration

The repository root is the **Reference Implementation** (Python, upstream `NousResearch/hermes-agent`). The `gormes/` directory is the **High-Performance Implementation** (Go). Neither replaces the other during Phases 1–4; they co-evolve as a translation pair until Phase 5's final purge completes the migration.

---

## 2. Why Go — for a Python developer

Five concrete bullets, no hype:

1. **Binary portability.** One 15–30 MB static binary. No `uv`, `pip`, venv, or system Python on the target host. `scp`-and-run on a $5 VPS or Termux.
2. **Static types and compile-time contracts.** Tool schemas, Provider envelopes, and MCP payloads become typed structs. Schema drift is a compile error, not a silent agent-loop failure.
3. **True concurrency.** Goroutines over channels replace `asyncio`. The gateway scales to 10+ platform connections without event-loop starvation.
4. **Lower idle footprint.** Target ≈ 10 MB RSS at idle vs. ≈ 80+ MB for Python Hermes. Meaningful on always-on or low-spec hosts.
5. **Explicit trade-off.** The Python AI-library moat (`litellm`, `instructor`, heavyweight ML, research skills) stays in Python until Phase 4–5.

---

## 3. Hybrid Manifesto — the Motherboard Strategy

The hybrid is **temporary**. The long-term state is 100% Go.

During Phases 1–4, Go is the chassis (orchestrator, state, persistence, platform I/O, agent cognition) and Python is the peripheral library (research tools, legacy skills, ML heavy lifting). Each phase shrinks Python's footprint. Phase 5 deletes the last Python dependency.

Phase 1 should be read correctly: it is a tactical Strangler Fig bridge, not a philosophical compromise. It exists to deliver immediate value to existing Hermes users while preserving a clean migration path toward a pure Go runtime that owns the entire lifecycle end to end.

---

## 4. Milestone Status

| Phase | Status | Deliverable |
|---|---|---|
| Phase 1 — The Dashboard (Face) | ✅ complete | Tactical bridge: Go TUI over Python's `api_server` HTTP+SSE boundary |
| Phase 2 — The Wiring Harness (Gateway) | 🔨 in progress | Go-native wiring harness: tools, Telegram, and thin session resume land before the wider gateway surface |
| Phase 3 — The Black Box (Memory) | ⏳ planned | SQLite + FTS5 + ontological graph in Go; Phase 2.C's bbolt layer is not transcript memory ownership |
| Phase 4 — The Powertrain (Brain Transplant) | ⏳ planned | Native Go agent orchestrator + prompt builder |
| Phase 5 — The Final Purge (100% Go) | ⏳ planned | Python tool scripts ported to Go or WASM |

Legend: 🔨 in progress · ✅ complete · ⏳ planned · ⏸ deferred.

### Phase 2 Ledger

| Subphase | Status | Deliverable |
|---|---|---|
| Phase 2.A — Tool Registry | ✅ complete | In-process Go tool registry, streamed `tool_calls` accumulation, kernel tool loop, and doctor verification |
| Phase 2.B.1 — Telegram Scout | ✅ complete | Split-binary Telegram adapter over the existing kernel, long-poll ingress, and edit coalescing at the messaging edge |
| Phase 2.C — Thin Mapping Persistence | ✅ complete | bbolt-backed `(platform, chat_id) -> session_id` resume only; no transcript ownership moved into Go |
| Phase 2.B.2+ — Wider Gateway Surface | ⏳ planned | Additional platform hands such as Discord, Slack, and the broader gateway perimeter |

Phase 2.C is intentionally not Phase 3. It stores only session handles in bbolt. Python still owns transcript memory, transcript search, and prompt assembly; the real SQLite + FTS5 memory lattice remains future work.

---

## 5. Project Boundaries

Hard rule: no Python file in this repository is modified. All Gormes work lives under `gormes/`. Upstream rebases against `NousResearch/hermes-agent` cannot conflict with Gormes because paths do not overlap.

The bridge is allowed to exist. The bridge is not allowed to become the destination.

---

## 6. Documentation

This `ARCH_PLAN.md` is the executive roadmap. It defines the strategic conquest of the operational bottleneck: first UI, then gateway, then memory and state, then cognition, then the final removal of Python from the runtime path. Per-milestone specs live at `docs/superpowers/specs/YYYY-MM-DD-*.md`. Per-milestone implementation plans live at `docs/superpowers/plans/YYYY-MM-DD-*.md`.

Public-site (`gormes.io`) deployment is **Phase 1.5** work. The documentation is authored in CommonMark + GFM so every mainstream static-site generator (Hugo, MkDocs Material, Astro Starlight) can render it without rewrites. Phase 1 ships a Goldmark-based validation test — Goldmark is the exact renderer Hugo uses, so passing the test guarantees Hugo-renderability.
