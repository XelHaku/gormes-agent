---
title: "Mirror Strategy — Auditability Roadmap"
weight: 90
---

## 8. Mirror Strategy — Auditability Roadmap

Phase 3.D.5 (Memory Mirror) closes the transparency gap for entities/relationships. Based on comprehensive Hermes parity research, here is the complete mirror strategy.

### 8.1 What Hermes Actually Has vs Gormes

| Data | Hermes Format | Gormes Format | Gap Analysis |
|------|--------------|---------------|--------------|
| **Entities/Relationships** | SQLite + USER.md (text) | SQLite + USER.md (via Mirror) | ✅ **Parity achieved (3.D.5)** |
| **Turns/Transcripts** | SQLite + JSONL | SQLite + Markdown export | ✅ **Gormes exceeds upstream with `gormes session export` (3.E.3)** |
| **Sessions** | SQLite (queryable) | bbolt (opaque binary) | 🔴 **Gap: bbolt is human-opaque** |
| **Tool Execution** | SessionDB (persisted) | JSONL audit trail + SQLite-backed transcripts | ✅ **Shipped in Gormes (3.E.2)** |
| **Extraction State** | SQLite columns | SQLite columns + `gormes memory status` | ✅ **Shipped in Gormes (3.E.4)** |
| **Skills** | SKILL.md (text files) | Active + candidate stores, usage log, reviewed promotion flow | 🔨 Core runtime shipped in Gormes (Phase 2.G); hub sync remains Phase 5.F |
| **Cron Output** | Markdown files | SQLite `cron_runs` + `CRON.md` mirror | ✅ **Shipped in Gormes (Phase 2.D)** |
| **Config** | YAML | TOML | ✅ Both human-readable |
| **Logs** | Text files (agent.log, etc.) | Text file (gormes.log) | ✅ Parity |

**Key Finding**: Hermes does **not** have human-readable transcript exports. Transcripts live in SQLite/JSONL only. The `export_session()` method returns JSON (machine-readable), not formatted text. **Gormes already exceeds Hermes parity** with the USER.md mirror for memory entities.

### 8.2 Remaining Mirror Candidates (Ranked by Priority)

#### High Priority: Session Index Mirror (Phase 3.E.1)

**Problem**: Sessions stored in bbolt (`~/.local/share/gormes/sessions.db`) are opaque. Operators cannot `cat`, `grep`, or audit their session mappings without binary tools.

**Solution**: Mirror the bbolt session map to `~/.local/share/gormes/sessions/index.yaml`:

```yaml
# Auto-generated session index
# This file is a read-only mirror of sessions.db for operator auditability
sessions:
  telegram:123456789: session_abc123
  telegram:987654321: session_def456
updated_at: 2026-04-20T09:30:00Z
```

**Implementation**: Background goroutine (like 3.D.5 Mirror) triggered on session write; atomic temp+rename; 30s sync interval.

**Rationale**: Hermes uses queryable SQLite for sessions; Gormes uses binary bbolt. This provides human-readable session auditability that Hermes has via SQL but Gormes lacks via bbolt opacity.

#### Medium Priority: Tool Execution Audit Log (Phase 3.E.2)

**Status**: ✅ **Shipped in Gormes (3.E.2)**

**Problem**: Tool calls are ephemeral. The Bear runs `terminal()`, produces output, but no persistent record exists. An operator cannot audit "what did the agent do yesterday?"

**Solution**: Append-only log at `~/.local/share/gormes/tools/audit.logl` (JSONL):

```json
{"ts":"2026-04-20T09:30:00Z","session":"abc123","turn":5,"tool":"terminal","cmd":"ls -la","duration_ms":150,"status":"ok"}
{"ts":"2026-04-20T09:30:05Z","session":"abc123","turn":6,"tool":"web_search","query":"golang embed","results":3,"duration_ms":2500}
```

**Rationale**: This exceeds Hermes capabilities. Python Hermes stores tool results in SessionDB messages table, but there's no separate audit trail for tool execution. This is new operational visibility.

#### Medium Priority: Transcript Export Command (Phase 3.E.3)

**Status**: ✅ **Shipped in Gormes (3.E.3)**

**Problem**: While Hermes has no human-readable transcript export, operators may want to export a conversation for sharing, backup, or analysis.

**Solution**: Add `gormes session export <session_id> --format=markdown` command that renders a formatted Markdown transcript.

**Rationale**: This is a **Gormes-only feature** that exceeds Hermes capabilities. Hermes has no equivalent human-readable export.

#### Low Priority: Extraction State Visibility (Phase 3.E.4)

**Status**: ✅ **Shipped in Gormes (3.E.4)** with room for richer dashboards later

**Problem**: `turns.extracted`, `extraction_attempts`, `extraction_error` columns are invisible to operators. A dead-lettered turn (`extracted=2`) requires SQLite inspection.

**Solution**: Optional: add extraction failures to the USER.md mirror footer, or provide `gormes memory status` command showing extraction queue depth and recent errors.

**Rationale**: This is debugging/operational visibility. Can be deferred until extraction issues become painful.

### 8.3 Hermes Files Gormes Does Not Need to Mirror (Yet)

Based on the comprehensive Hermes file inventory, these Hermes files do not need Gormes mirrors today, but may become relevant as features land:

| Hermes File | Why Not Mirrored in Gormes | Future Consideration |
|-------------|---------------------------|-------------------|
| `MEMORY.md` | Superseded by USER.md + entity graph (structured > flat) | N/A — entity graph is superior |
| `sessions.json` | Legacy Hermes format; Gormes uses bbolt (better concurrency) | **Session Index Mirror (3.E.1)** closes bbolt opacity |
| `*.jsonl` transcripts | Machine-readable only | **Transcript Export (3.E.3)** adds human-readable option |
| `jobs.json` + cron output | Gormes ships **Phase 2.D** as SQLite `cron_runs` + derived `CRON.md`, not the upstream file layout | Existing cron audit surface is the source of truth; optional per-job export remains future work |
| `SKILL.md` files | Gormes ships the **Phase 2.G** active + candidate skill stores, not the upstream shared tree layout | Skill audit trail and hub sync expand in Phase 5.F |
| `HOOK.yaml` | Hook manifests now load from `$XDG_DATA_HOME/gormes/hooks/`; built-in BOOT startup execution is also live in **Phase 2.F.2** | Hook activity log and richer audit surfaces remain future work |
| `BOOT.md` | Built-in startup automation file at `$XDG_DATA_HOME/gormes/BOOT.md`; run through an isolated background boot kernel on gateway start in **Phase 2.F.2** | Boot sequence audit remains future work |
| `SOUL.md` | Personality system not yet implemented (Phase 4+) | Persona versioning when Phase 4 lands |
| `gateway_voice_mode.json` | Voice mode not implemented (Phase 5.E) | Voice state mirroring if voice features land |
| Platform state JSON files | Shared gateway exists, but most remaining adapter-specific state surfaces are still planned (Phase 2.B.4–2.B.10) | Per-platform state audit when those adapters land |

**Operational State Files Discovered in Additional Research:**

| Hermes File | Purpose | Gormes Status |
|-------------|---------|---------------|
| `gateway_voice_mode.json` | Per-chat voice mode state (off/voice_only/all) | Not implemented (Phase 5.E) |
| `display_config` (in config.yaml) | Per-platform display settings | Partial — TUI theme only |
| `active_profile` | Currently active profile name | Not implemented |
| `channel_directory.json` | Cached channel/contact mappings | Not implemented |
| `pairing.json` | Device/pairing state per platform | Not implemented (Phase 2.F) |

**Additional Subsystems with Audit Potential:**

| Hermes Subsystem | Data Produced | Mirror Potential | Phase |
|------------------|---------------|------------------|-------|
| `agent/insights.py` | Usage analytics (tokens, costs, trends, tool patterns) | High — Operators need visibility into spend and usage patterns | 4.E |
| `agent/trajectory.py` | RL training trajectories (JSONL) | Medium — Machine-readable; research use case | 4.E |
| `agent/usage_pricing.py` | Per-request cost calculations | High — Cost audit trail for operational monitoring | 4.E |

**Insights Engine Gap**: Hermes has a comprehensive `InsightsEngine` (`agent/insights.py`, 768 lines) that analyzes historical session data to produce:
- Token consumption reports
- Cost estimates by model/provider
- Tool usage patterns
- Activity trends over time
- Platform breakdowns
- Session metrics (duration, turns, success rate)

Gormes now has local rollup primitives (`internal/insights/rollup.go`), `telemetry.Snapshot` bridging, and append-only JSONL persistence for daily usage records. Operators can derive usage locally and keep a durable daily audit trail without waiting for the broader Phase 4 InsightsEngine port.

**Shipped Mirror Addition — Phase 3.E.5: Insights Audit Log**

Export aggregated session metrics to `~/.local/share/gormes/insights/usage.jsonl`:

```json
{"date":"2026-04-20","session_count":5,"total_tokens_in":45000,"total_tokens_out":12000,"estimated_cost_usd":0.45,"model_breakdown":{"claude-opus":3,"gpt-4":2}}
```

This now provides a lightweight, append-only cost and usage audit trail that accumulates over time, even before the full InsightsEngine is ported in Phase 4.E.

### 8.4 Mirror Implementation Principles

All mirrors must follow the 3.D.5 design constraints:
1. **Source of truth remains database** — mirrors are read-only exports
2. **Fire-and-forget** — never block the 250ms kernel latency budget
3. **Atomic writes** — temp file + rename (readers never see partial files)
4. **Change detection** — hash comparison to avoid redundant writes
5. **Graceful degradation** — log warnings on errors, never crash the bot
6. **Configurable paths** — respect XDG directories and config overrides

---

*Mirror Strategy v1.0 — Synthesized from parallel audit of Hermes Python codebase and Gormes Go implementation.*
