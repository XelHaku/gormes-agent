# Upstream Hermes Feature Parity Dashboard

**Date:** 2026-04-20
**Status:** Approved

## Overview

Add `port_status` frontmatter to each of the 123 upstream Hermes docs. A Hugo dashboard page reads all upstream docs and displays feature parity — what's been ported to Go vs what's still Python-only.

## Frontmatter Schema

Every markdown file under `docs/content/upstream-hermes/` gets this frontmatter:

```yaml
port_status: NOT_STARTED  # NOT_STARTED | IN_PROGRESS | PORTED
port_phase: ""            # e.g., "5.C" — empty if NOT_STARTED
go_equivalent: ""         # e.g., "internal/tools/" — empty if not ported
```

## Files

### New files

| File | Purpose |
|------|---------|
| `docs/layouts/shortcodes/port-status-badge.html` | Badge shortcode: {{< port-status-badge "PORTED" >}} |
| `docs/layouts/shortcodes/parity-dashboard.html` | Full dashboard shortcode |
| `docs/content/upstream-hermes/parity.md` | The dashboard page at /upstream-hermes/parity/ |
| `scripts/update-port-status.sh` | Reads progress.json → updates upstream doc frontmatter |

### Modified files

| File | Change |
|------|--------|
| All 123 upstream docs | Add `port_status`, `port_phase`, `go_equivalent` frontmatter fields |
| `docs/data/port_status.json` | Auto-generated summary of parity stats |

## Scripts

### scripts/update-port-status.sh

Reads `docs/data/progress.json`. When a subphase is marked `complete`, updates the relevant upstream docs' `port_status` to `PORTED`.

Example: When 5.C "Browser Automation" is marked complete:
1. Finds all docs with `port_phase: "5.C"`
2. Updates their `port_status` to `PORTED`
3. Sets `go_equivalent` to the Go implementation path

Runs automatically on `make build`.

## Dashboard Page

**URL:** `/upstream-hermes/parity/`

**Content:**
- Overall parity bar (X/Y docs ported)
- Per-section breakdown with progress bars
- Tables: PORTED items, IN_PROGRESS items, NOT_STARTED items
- Each entry links to upstream doc + Go equivalent when available

## Build Integration

```
make build
    → record-progress.sh (updates progress.json timestamp)
    → update-port-status.sh (reads progress.json, updates upstream doc frontmatter)
    → Hugo builds site (dashboard reads updated frontmatter)
```

## Initial Population

All 123 upstream docs start with `port_status: NOT_STARTED`. As Phase 5 subphases complete, update manually or via `update-port-status.sh`.

## Frontmatter Fields Explained

| Field | Values | When to set |
|-------|--------|-------------|
| `port_status` | NOT_STARTED, IN_PROGRESS, PORTED | Set when work begins/completes |
| `port_phase` | e.g., "5.A", "5.C" | Which Phase 5 subphase covers this doc |
| `go_equivalent` | Path like "internal/tools/" | Set when Go implementation exists |

## Section Mapping

| Section | Path | Subdir count |
|---------|------|--------------|
| Features | upstream-hermes/user-guide/features/ | 32 |
| Reference | upstream-hermes/reference/ | 11 |
| User Guide | upstream-hermes/user-guide/ (non-features) | 13 |
| Getting Started | upstream-hermes/getting-started/ | 6 |
| Developer Guide | upstream-hermes/developer-guide/ | ~8 |
| Integrations | upstream-hermes/integrations/ | ~5 |

Total: ~75 docs (features + reference + user guide main files)

## Out of Scope

- Auto-detecting upstream doc changes (manual sync)
- Item-level tracking within docs (doc-level only)
- Modifying upstream doc content (frontmatter only)
