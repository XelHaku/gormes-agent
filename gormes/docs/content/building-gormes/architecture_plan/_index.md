---
title: "Architecture Plan"
weight: 10
---

# Gormes — Executive Roadmap

**Single source of truth:** [`progress.json`](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/data/progress.json) — machine-readable, auto-updated on build

**Linked surfaces:**
- [README.md](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/README.md) — Quick start + binary claims
- [Landing page](https://gormes.ai) — Marketing + feature list
- [docs.gormes.ai](https://docs.gormes.ai/building-gormes/architecture_plan/) — This page
- [Source code](https://github.com/TrebuchetDynamics/gormes-agent) — Implementation

---

## Progress Summary

| Phase | Status | Shipped |
|-------|--------|---------|
| Phase 1 — The Dashboard | ✅ Complete | 5 items |
| Phase 2 — The Gateway | 🔨 In Progress | 4 of 8 subphases |
| Phase 3 — The Black Box | 🔨 Substantially Complete | 5 of 12 subphases |
| Phase 4 — The Brain Transplant | ⏳ Planned | 0 of 8 subphases |
| Phase 5 — The Final Purge | ⏳ Planned | 0 of 17 subphases |
| Phase 6 — The Learning Loop | ⏳ Planned | 0 of 6 subphases |

**Overall:** 9/52 subphases shipped (17%) · 2 in progress · 41 planned

---

## Detailed Checklist

{{< progress-checklist >}}

---

## Data Format

The [`progress.json`](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/data/progress.json) file is the machine-readable source of truth. Updated automatically on `make build`.
