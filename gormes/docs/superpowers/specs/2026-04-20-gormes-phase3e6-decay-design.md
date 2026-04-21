# Gormes Phase 3.E.6 — Memory Decay Design Spec

**Date:** 2026-04-20
**Phase:** 3.E.6 — Memory Decay (linear-with-horizon, SQL-native)
**Upstream reference:** none — this is a Gormes-original feature. `agent/memory_manager.py` + `agent/memory_provider.py` have zero decay logic.
**Status:** Approved for implementation plan

---

## 1. Goal

Make old relationships fade from the recall fence as they age, without deleting any data. Raw `weight` + `updated_at` columns stay untouched (reversible, audit-preserving); the recall-path SELECT expressions substitute in a linear decay factor so ancient edges get filtered and outranked by fresh ones.

**Ship criterion:** seed two relationships with the same predicate and weight but different ages — one fresh (age=0), one stale (age=300 days) — against the same entity pair. Run recall with a seed that expands to them. The fresh edge appears in the `<memory-context>` fence; the stale edge does not.

## 2. Non-Goals

- **Entity-level decay.** Only relationships decay. Entities stay eligible for the cosine scan (3.D) regardless of age — an operator may ask about an old project by name and still expect the semantic layer to find it.
- **`last_referenced_at` column + write-on-read reinforcement.** Deferred to 3.E.6b. For MVP, decay is driven purely by the extractor-side `updated_at`, which advances via `ON CONFLICT DO UPDATE` whenever a new turn mentions the edge.
- **Exponential / stepped decay curves.** Swap-in later via a one-expression SQL change once operator feedback shows linear-with-horizon is wrong.
- **Decay for `cron_runs`, `turns`, `entity_embeddings`, or entity descriptions.** Other tables are not in scope.
- **CLI tooling** (`gormes memory decay --preview`). Phase 2.D.2-era operator feature.
- **Per-predicate decay rates.** A single horizon knob applies to all predicates. `WORKS_ON` decays at the same rate as `LIKES`.

## 3. Scope

| In | Out |
|---|---|
| Linear SQL decay expression `MAX(0, weight * (1 - age/horizon))` | Exponential / stepped curves |
| New `RecallConfig.DecayHorizonDays` (default 180) | `last_referenced_at` write-on-read column |
| Parallel `TelegramCfg.RecallDecayHorizonDays` TOML knob | Entity decay |
| Substitute into `traverseNeighborhood` WHERE | Schema migration (zero changes required) |
| Substitute into `enumerateRelationships` WHERE + ORDER BY | CLI preview tooling |
| Disable path when `DecayHorizonDays <= 0` (legacy raw-weight) | Per-predicate rates |
| 4 unit tests locking the invariants | Per-chat-key decay rates |
| 1 config test locking the default | Gossip / telemetry of decay application |

## 4. Architecture

### 4.1 The decay expression

One canonical expression, substituted as a string wherever the recall path currently references `r.weight`:

```sql
MAX(0, r.weight * (1 - CAST(strftime('%s','now') - r.updated_at AS REAL) / ?))
```

The `?` binds to `horizonSec = DecayHorizonDays * 86400` (int64). The `CAST ... AS REAL` forces float division — without it, SQLite does integer division and the factor snaps to 0 or 1 at the day boundary. `MAX(0, ...)` clamps negative values (rows older than horizon) to exactly zero — no wrap-around surprises in the ORDER BY.

### 4.2 When decay is disabled

When `cfg.DecayHorizonDays <= 0`, the Go code substitutes the legacy raw expression `r.weight` at both sites. No decay, no extra bound parameter. This is the backward-compat path for operators who want to turn the feature off. It is NOT the default — the default is 180 days.

### 4.3 Single-site helper

Rather than inline the two expressions in two SQL builders, add a private helper in `recall_sql.go`:

```go
// weightExpr returns the SQL expression that substitutes for r.weight
// in WHERE / ORDER BY clauses. When horizonDays <= 0, decay is
// disabled and the raw column reference is returned. Otherwise a
// linear-decay expression with one bound parameter (horizonSec) is
// returned; callers are responsible for binding horizonSec in the
// correct argument position.
func weightExpr(horizonDays int) string {
    if horizonDays <= 0 {
        return "r.weight"
    }
    return "MAX(0, r.weight * (1 - CAST(strftime('%s','now') - r.updated_at AS REAL) / ?))"
}
```

This localizes the decay mechanic so future changes (exponential, per-predicate) touch one function.

## 5. Application Sites

### 5.1 `traverseNeighborhood`

Current query (in `recall_sql.go`):

```sql
WITH RECURSIVE neighborhood(entity_id, depth) AS (
    SELECT entity_id, 0 FROM seeds
    UNION
    SELECT CASE WHEN r.source_id = n.entity_id THEN r.target_id
                ELSE r.source_id END,
           n.depth + 1
    FROM neighborhood n
    JOIN relationships r ON (r.source_id = n.entity_id OR r.target_id = n.entity_id)
                       AND r.weight >= ?
    WHERE n.depth < ?
)
...
```

After decay:

```sql
...
    JOIN relationships r ON (r.source_id = n.entity_id OR r.target_id = n.entity_id)
                       AND <weightExpr> >= ?
...
```

where `<weightExpr>` is the string returned by `weightExpr(horizonDays)`. When decay is active, the existing `threshold` bind is preceded by a new `horizonSec` bind. When disabled, argument ordering is unchanged.

Only the CTE's expansion condition changes. The outer SELECT (`ORDER BY dn.depth ASC, e.updated_at DESC`) is weight-agnostic and stays as-is.

### 5.2 `enumerateRelationships`

Current:

```sql
SELECT e1.name, r.predicate, e2.name, r.weight
FROM relationships r
JOIN entities e1 ON r.source_id = e1.id
JOIN entities e2 ON r.target_id = e2.id
WHERE r.source_id IN (...) AND r.target_id IN (...)
  AND r.weight >= ?
ORDER BY r.weight DESC, e1.name ASC, e2.name ASC
LIMIT ?
```

After decay:

```sql
SELECT e1.name, r.predicate, e2.name, r.weight
FROM relationships r
JOIN entities e1 ON r.source_id = e1.id
JOIN entities e2 ON r.target_id = e2.id
WHERE r.source_id IN (...) AND r.target_id IN (...)
  AND <weightExpr> >= ?
ORDER BY <weightExpr> DESC, e1.name ASC, e2.name ASC
LIMIT ?
```

Two substitutions of the same expression — once in WHERE, once in ORDER BY. When decay is active, **two** new `horizonSec` binds are needed (SQLite positional placeholders don't share across clauses unless named). The Go code builds the argument slice to match.

**Note:** the SELECT column `r.weight` is the **raw** weight, not the decayed one. The fence presents raw weight to the operator (they asked for the data). The filter uses decayed weight (for ranking). This is deliberate: the operator sees `Acme LOCATED_IN Springfield [weight=1.0]` with the real weight, not a time-dependent number that changes hour-to-hour.

## 6. Config

### 6.1 RecallConfig

New field in `internal/memory/recall.go`:

```go
type RecallConfig struct {
    // ... existing fields ...
    // DecayHorizonDays — Phase 3.E.6. An edge's effective weight
    // decays linearly from 1.0×raw at age=0 to 0.0 at
    // age=DecayHorizonDays days. Applied to the recall path's
    // relationship WHERE/ORDER BY; the raw weight column is
    // untouched (decay is reversible by tweaking this knob).
    // <= 0 disables decay — recall uses the legacy raw-weight
    // filter. Default 180.
    DecayHorizonDays int
}
```

Default in `withDefaults()`: `DecayHorizonDays = 180` when `<= 0`... **wait — that's contradictory.** Correction: `withDefaults()` sets `DecayHorizonDays = 180` **only when the field is zero-valued** (i.e., unset). An operator who explicitly sets it to a negative value (the "disable" signal) keeps that value.

Resolution: use a sentinel. `DecayHorizonDays == 0` → default to 180. `DecayHorizonDays < 0` → disabled. Document this clearly:

```go
func (c *RecallConfig) withDefaults() {
    // ... existing defaults ...
    if c.DecayHorizonDays == 0 {
        c.DecayHorizonDays = 180 // default 6 months
    }
    // Negative values are preserved as the "disabled" sentinel.
}
```

The recall-path helper `weightExpr` treats `<= 0` as disabled, so `-1` and any other negative sentinel both disable. `0` is reserved for "unset → apply default."

### 6.2 TelegramCfg

Parallel TOML knob in `internal/config/config.go`:

```go
type TelegramCfg struct {
    // ... existing fields ...
    // RecallDecayHorizonDays maps to RecallConfig.DecayHorizonDays.
    // Default 180. Set to -1 to disable decay entirely.
    RecallDecayHorizonDays int `toml:"recall_decay_horizon_days"`
}
```

Default in `defaults()`: `RecallDecayHorizonDays: 180`.

### 6.3 cmd/gormes/telegram.go plumbing

In `runTelegram`, the existing `memory.NewRecall(...)` call gains one line:

```go
memProv := memory.NewRecall(mstore, memory.RecallConfig{
    // ... existing fields ...
    DecayHorizonDays: cfg.Telegram.RecallDecayHorizonDays,
}, slog.Default())
```

The TUI (non-telegram) entry point does not currently construct a RecallProvider, so no change there.

## 7. Testing

### 7.1 Unit tests (in `internal/memory/recall_sql_test.go`)

**`TestTraverseNeighborhood_DecayFiltersStaleEdges`**
Seed entities A, B, C. Create edges: A→B (updated_at=now, weight=1.0), A→C (updated_at=now-400d, weight=5.0). Call `traverseNeighborhood` with `seedIDs=[A], depth=2, threshold=0.5, horizonDays=180`. Assert: returned entity IDs include B (fresh edge survives) and exclude C (stale edge's effective weight = 0 < 0.5).

**`TestEnumerateRelationships_DecayOrdersByEffectiveWeight`**
Seed entities X, Y, Z. Create edges: X→Y weight=5.0 updated_at=now-300d, X→Z weight=2.0 updated_at=now-30d. With horizon=180, X→Y effective ≈ MAX(0, 5*(1-300/180)) = 0; X→Z effective ≈ 2*(1-30/180) ≈ 1.67. Call `enumerateRelationships` with `neighborhoodIDs=[X,Y,Z], threshold=0.5, limit=10, horizonDays=180`. Assert: result contains X→Z but NOT X→Y; order is fresh-first.

**`TestRecall_DecayDisabledWhenHorizonNegative`**
Same setup as the first test. Call `traverseNeighborhood` with `horizonDays=-1`. Assert: BOTH B and C returned — decay disabled means raw-weight filter only, and the stale edge's raw weight (5.0) passes.

**`TestRecall_DecayRawWeightInFenceUnchanged`**
Enumerate a stale-but-still-visible edge (e.g., age=30d, weight=5.0, horizon=180 → effective ≈ 4.17). Assert the returned row's `Weight` field is **5.0** (the raw), not 4.17 (the decayed). Operator-facing numbers stay honest.

### 7.2 Config tests (in `internal/config/config_test.go`)

**`TestLoad_RecallDecayHorizonDays`**
`t.Setenv("XDG_CONFIG_HOME", t.TempDir())`. Call `Load(nil)`. Assert `cfg.Telegram.RecallDecayHorizonDays == 180`.

**Additionally** — in `internal/memory/recall_test.go`, verify `withDefaults` behavior:

**`TestRecallConfig_WithDefaults_DecayHorizon`**
Construct a zero-value `RecallConfig`, call `withDefaults()`, assert `DecayHorizonDays == 180`. Construct one with `DecayHorizonDays = -1`, call `withDefaults()`, assert it stays `-1` (disabled sentinel preserved).

### 7.3 No new integration test

The existing Phase 3.C recall integration test (the fence-format test) keeps passing unchanged because its seeded `updated_at` values are current. 3.D's Ollama recall E2E also keeps passing — the entities it exercises are <1 minute old. No new Ollama test required for decay.

## 8. Error Handling

Decay is a SELECT-path feature. No writes. No new error surface.

- Invalid horizon (negative via misconfiguration) is a **feature**, not an error — it's the documented disable sentinel.
- A horizon of 1 second (or any absurdly small value) produces the correct mathematical result: every edge older than 1 second is fully decayed. The operator who set it gets what they asked for; no guard rail.
- Numerical edge cases (`updated_at` in the future because of clock skew): `strftime('%s','now') - r.updated_at` goes negative, the `(1 - age/horizon)` term goes above 1, `MAX(0, ...)` doesn't clip it — the effective weight rises slightly above raw. This is mathematically correct behavior for "future-dated" edges; clock skew should be rare and the effect is small.

## 9. Rollout

Ship as one self-contained commit. No migration, no flag day, no staged rollout.

Operator experience when the binary is updated:
- Existing config without `recall_decay_horizon_days` gets the 180-day default. Edges >180 days old disappear from the fence. This may be a visible behavior change if any operator has been keeping an ancient graph.
- An operator who wants the legacy behavior sets `[telegram].recall_decay_horizon_days = -1` in `config.toml`.

**No breaking change to the TOML schema version** (`_config_version` stays at 1). The new key is additive and has a default.

## 10. Binary Size Impact

Zero. No new dependencies. ~20 lines of Go in `recall_sql.go` + ~10 lines of config + ~120 lines of tests. Binary stays at ~17 MB.

## 11. Architecture Invariants Preserved

- Kernel isolation: kernel doesn't import cron/memory — still holds. Decay is pure memory-package change.
- Raw `weight` + `updated_at` columns untouched — reversibility maintained. Any operator can `SELECT weight FROM relationships` and get the pre-decay value.
- Memory worker writes unchanged. The extractor's `ON CONFLICT DO UPDATE SET weight = MIN(weight + excluded.weight, 10.0), updated_at = excluded.updated_at` continues to reinforce edges on every mention.
- `<memory-context>` fence format unchanged: the rendered weight is still the raw one.
- No new goroutines, no new background work.

## 12. Open Questions Resolved

| Question | Resolution |
|---|---|
| Decay basis — `updated_at` vs new `last_referenced_at`? | `updated_at` only. Write-on-read deferred to 3.E.6b. |
| Function shape — linear vs exponential vs step? | Linear with horizon. Swap later if operator feedback warrants. |
| Apply where — WHERE only, or WHERE + ORDER BY? | Both. Stale high-weight edges must be outranked, not just filtered. |
| Schema change? | None. Decay is a query-time SELECT expression. |
| Disable sentinel? | `DecayHorizonDays < 0`. `0` is reserved for "unset → default 180". |
| Show raw or decayed weight in fence? | Raw. Operator-facing numbers stay honest. |
| Per-predicate rates? | Out of scope. Single horizon knob for all predicates. |

---

## 13. Final Checklist

- [x] Zero schema change
- [x] One SQL expression, substituted at two sites via one helper
- [x] Default 180 days (6 months)
- [x] Disabled via negative horizon (`0` reserved for "use default")
- [x] Raw weight preserved in fence
- [x] 4 unit tests + 2 config tests + 0 integration tests
- [x] No new goroutines, no new deps, no binary growth
- [x] Reversible and audit-preserving (raw data untouched)
- [x] Ships as one commit

**Ready for implementation plan.**
