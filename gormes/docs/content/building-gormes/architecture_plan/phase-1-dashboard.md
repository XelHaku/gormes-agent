---
title: "Phase 1 — The Dashboard"
weight: 20
---

# Phase 1 — The Dashboard

**Status:** ✅ complete · evolving (polish, bug fixes, TUI ergonomics ongoing)

Phase 1 is a tactical Strangler Fig bridge, not a philosophical compromise. It exists to deliver immediate value to existing Hermes users while preserving a clean migration path toward a pure Go runtime that owns the entire lifecycle end to end.

The hybrid is **temporary**. The long-term state is 100% Go. During Phases 1–4, Go is the chassis (orchestrator, state, persistence, platform I/O, agent cognition) and Python is the peripheral library (research tools, legacy skills, ML heavy lifting). Each phase shrinks Python's footprint. Phase 5 deletes the last Python dependency.

Phase 1 should be read correctly: it is a tactical Strangler Fig bridge, not a philosophical compromise. It exists to deliver immediate value to existing Hermes users while preserving a clean migration path toward a pure Go runtime that owns the entire lifecycle end to end.

**Deliverable:** Tactical bridge: Go TUI over Python's `api_server` HTTP+SSE boundary.

## What shipped

- Bubble Tea TUI shell
- Kernel with 16 ms render mailbox (coalescing)
- Route-B SSE reconnect (dropped streams recover)
- Wire Doctor — offline tool-registry validation
- Streaming token renderer

## What's ongoing

- Polish, bug fixes, and TUI ergonomics stay on the maintenance lane.
- Automation reliability now has its own progress-ledger lane (1.C) for planner/orchestrator operational fixes. Keep those slices TDD-first and do not treat them as product-dashboard scope.
