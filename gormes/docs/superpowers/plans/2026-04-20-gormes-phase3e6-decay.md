# Gormes Phase 3.E.6 — Memory Decay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make old relationships fade from the recall fence via a query-time linear decay (`MAX(0, weight * (1 - age/horizon))`) on the existing `updated_at` column. Zero schema change, one config knob (`RecallDecayHorizonDays`, default 180 days), reversible and audit-preserving — raw `weight` + `updated_at` columns are never mutated.

**Architecture:** Add a `weightExpr(horizonDays int) string` helper in `recall_sql.go` that returns either the raw column (`r.weight`) when decay is disabled, or the decay expression when active. Substitute that string into `traverseNeighborhood`'s WHERE and `enumerateRelationships`' WHERE + ORDER BY. Pass `cfg.DecayHorizonDays` through from `RecallConfig` (library) and `TelegramCfg.RecallDecayHorizonDays` (TOML) via `cmd/gormes/telegram.go`.

**Tech Stack:** Go 1.25+, existing ncruces WASM SQLite (`strftime('%s','now')` built-in, no math extension needed), no new dependencies.

**Module path:** `github.com/TrebuchetDynamics/gormes-agent/gormes`

**Spec:** [`docs/superpowers/specs/2026-04-20-gormes-phase3e6-decay-design.md`](../specs/2026-04-20-gormes-phase3e6-decay-design.md) (approved `bdaffd25`)

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `gormes/internal/memory/recall.go` | Modify | Add `DecayHorizonDays int` to `RecallConfig`; extend `withDefaults()` with the `0 → 180, negative preserved` sentinel logic; update the two call sites to pass `p.cfg.DecayHorizonDays` |
| `gormes/internal/memory/recall_test.go` | Modify | Append `TestRecallConfig_WithDefaults_DecayHorizon` (3 cases: zero → 180, positive preserved, negative preserved as disable) |
| `gormes/internal/memory/recall_sql.go` | Modify | Add `weightExpr(horizonDays int) string` helper; add `horizonDays int` param to `traverseNeighborhood` + `enumerateRelationships`; substitute expression into WHERE (both) and ORDER BY (enumerate only) |
| `gormes/internal/memory/recall_sql_test.go` | Modify | Append 4 tests: `TestTraverseNeighborhood_DecayFiltersStaleEdges`, `TestRecall_DecayDisabledWhenHorizonNegative`, `TestEnumerateRelationships_DecayOrdersByEffectiveWeight`, `TestRecall_DecayRawWeightInFenceUnchanged` |
| `gormes/internal/config/config.go` | Modify | Add `RecallDecayHorizonDays int` to `TelegramCfg`; add `RecallDecayHorizonDays: 180` to `defaults()` |
| `gormes/internal/config/config_test.go` | Modify | Append `TestLoad_RecallDecayHorizonDays` |
| `gormes/cmd/gormes/telegram.go` | Modify | Thread `cfg.Telegram.RecallDecayHorizonDays` into `memory.RecallConfig{...}` literal |

---

## Task 1: `RecallConfig.DecayHorizonDays` + `withDefaults` sentinel logic

**Rationale:** Land the config scaffold before the SQL changes. T2 and T3 can then freely reference `p.cfg.DecayHorizonDays` without needing to add the field themselves. The field is unused at runtime until T2, so this commit is a pure no-op — perfect for isolating the config-schema decision.

**Files:**
- Modify: `gormes/internal/memory/recall.go`
- Modify: `gormes/internal/memory/recall_test.go`

- [ ] **Step 1: Locate the current `RecallConfig` and `withDefaults`**

```bash
cd <repo>/gormes
grep -nE "type RecallConfig|withDefaults" internal/memory/recall.go
```

You should see the struct definition plus a `func (c *RecallConfig) withDefaults()` method. Note the existing fields (`WeightThreshold`, `MaxFacts`, `Depth`, `MaxSeeds`, plus the 3.D semantic fields).

- [ ] **Step 2: Write failing test — append to `recall_test.go`**

```go
func TestRecallConfig_WithDefaults_DecayHorizon(t *testing.T) {
	// Zero-value field gets the default (180 days).
	cfg := RecallConfig{}
	cfg.withDefaults()
	if cfg.DecayHorizonDays != 180 {
		t.Errorf("zero-value -> withDefaults: DecayHorizonDays = %d, want 180",
			cfg.DecayHorizonDays)
	}

	// Positive field value is preserved.
	cfg = RecallConfig{DecayHorizonDays: 30}
	cfg.withDefaults()
	if cfg.DecayHorizonDays != 30 {
		t.Errorf("positive preserved: DecayHorizonDays = %d, want 30",
			cfg.DecayHorizonDays)
	}

	// Negative (disable sentinel) is preserved — NOT defaulted.
	cfg = RecallConfig{DecayHorizonDays: -1}
	cfg.withDefaults()
	if cfg.DecayHorizonDays != -1 {
		t.Errorf("negative preserved: DecayHorizonDays = %d, want -1 (disable sentinel)",
			cfg.DecayHorizonDays)
	}
}
```

- [ ] **Step 3: Run, expect FAIL**

```bash
cd <repo>/gormes
go test ./internal/memory/... -run TestRecallConfig_WithDefaults_DecayHorizon -v 2>&1 | tail -5
```

Expected: compile error — `unknown field DecayHorizonDays on RecallConfig`.

- [ ] **Step 4: Add the field to `RecallConfig`**

In `internal/memory/recall.go`, find `type RecallConfig struct { ... }`. Append **after the existing fields** (preserve 3.D semantic block placement):

```go
	// DecayHorizonDays — Phase 3.E.6. An edge's effective weight
	// decays linearly from 1.0×raw at age=0 to 0.0 at
	// age=DecayHorizonDays days. Applied to the recall path's
	// relationship WHERE/ORDER BY; the raw weight column is
	// untouched (decay is reversible by tweaking this knob).
	// Sentinel rules:
	//   0  — unset; withDefaults promotes to 180.
	//   >0 — preserved as the active horizon.
	//   <0 — preserved as the "disabled" signal; recall falls back
	//        to the legacy raw-weight filter.
	DecayHorizonDays int
```

- [ ] **Step 5: Extend `withDefaults`**

In the same file, find `func (c *RecallConfig) withDefaults()`. Append after the existing defaults (inside the method body):

```go
	// Phase 3.E.6 — only promote zero to the default. Negative
	// values are preserved as the "decay disabled" sentinel.
	if c.DecayHorizonDays == 0 {
		c.DecayHorizonDays = 180
	}
```

- [ ] **Step 6: Run, expect PASS**

```bash
cd <repo>/gormes
go test -race ./internal/memory/... -run TestRecallConfig_WithDefaults_DecayHorizon -v -timeout 30s
go vet ./...
```

All 3 sub-assertions pass. Full memory suite still green:

```bash
go test -race ./internal/memory/... -count=1 -timeout 60s -skip Integration_Ollama
```

Green.

- [ ] **Step 7: Commit**

```bash
git add gormes/internal/memory/recall.go gormes/internal/memory/recall_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): add RecallConfig.DecayHorizonDays with sentinel logic

Phase 3.E.6 config scaffold. Adds the field + withDefaults
promotion rule. Runtime behavior unchanged — the SQL changes
that consume this field land in T2/T3.

Sentinel rules:
  0  — unset; withDefaults promotes to 180 (6 months).
  >0 — preserved as the active horizon.
  <0 — preserved as "decay disabled"; recall falls back to the
       legacy raw-weight filter.

One test covers all three branches. The 0→180 branch matters
for binary upgrades: operators with pre-3.E.6 configs
automatically get the default behavior without touching their
config.toml. The negative-preserved branch is the explicit
escape hatch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `weightExpr` helper + `traverseNeighborhood` decay

**Files:**
- Modify: `gormes/internal/memory/recall_sql.go`
- Modify: `gormes/internal/memory/recall.go` (one call site)
- Modify: `gormes/internal/memory/recall_sql_test.go`

- [ ] **Step 1: Write the first failing test — append to `recall_sql_test.go`**

```go
// seedDecayGraph inserts entities + one relationship with an explicit
// updated_at timestamp so tests can age rows deterministically. Returns
// the source and target entity IDs.
func seedDecayGraph(t *testing.T, s *SqliteStore, srcName, predicate, tgtName string, weight float64, updatedAtUnix int64) (int64, int64) {
	t.Helper()
	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT INTO entities(name,type,updated_at) VALUES(?, 'PERSON', ?)`,
		srcName, now)
	if err != nil {
		t.Fatalf("insert src entity: %v", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO entities(name,type,updated_at) VALUES(?, 'PERSON', ?)`,
		tgtName, now)
	if err != nil {
		t.Fatalf("insert tgt entity: %v", err)
	}
	var srcID, tgtID int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name = ?`, srcName).Scan(&srcID)
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name = ?`, tgtName).Scan(&tgtID)
	_, err = s.db.Exec(
		`INSERT INTO relationships(source_id, target_id, predicate, weight, updated_at)
		 VALUES(?, ?, ?, ?, ?)`,
		srcID, tgtID, predicate, weight, updatedAtUnix)
	if err != nil {
		t.Fatalf("insert relationship: %v", err)
	}
	return srcID, tgtID
}

func TestTraverseNeighborhood_DecayFiltersStaleEdges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	now := time.Now().Unix()
	stale := now - 400*86400 // 400 days old

	// Fresh edge: A -> B, weight 1.0, updated_at=now.
	aID, _ := seedDecayGraph(t, s, "A", "KNOWS", "B", 1.0, now)
	// Stale edge: A -> C, weight 5.0, updated_at=400d ago.
	// With horizon=180d, effective = MAX(0, 5 * (1 - 400/180)) = 0.
	_, _ = seedDecayGraph(t, s, "A2", "KNOWS", "C", 5.0, stale)
	// Reuse the A entity — actually seedDecayGraph inserts a fresh src;
	// use the A2 handle as a separate entity. But the test cares about
	// the CTE expanding from a seed. Simpler: expand from A and include
	// A->B only; test that starting from A2 returns C ONLY when decay
	// is disabled.
	_ = aID

	// Get A2's ID for the seed.
	var a2ID int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name = 'A2'`).Scan(&a2ID)

	// Expand from A2 with horizon=180 days, threshold=0.5.
	// C's effective weight is 0, so C should NOT be in the neighborhood.
	entities, err := traverseNeighborhood(context.Background(), s.db,
		[]int64{a2ID}, 2, 0.5, 10, 180)
	if err != nil {
		t.Fatalf("traverseNeighborhood: %v", err)
	}

	// Should contain A2 (seed, depth 0) but NOT C (stale, decayed to 0).
	var foundA2, foundC bool
	for _, e := range entities {
		if e.Name == "A2" {
			foundA2 = true
		}
		if e.Name == "C" {
			foundC = true
		}
	}
	if !foundA2 {
		t.Error("expected seed A2 in result at depth 0")
	}
	if foundC {
		t.Error("stale edge expanded to C; decay should have filtered it (effective=0)")
	}
}

func TestRecall_DecayDisabledWhenHorizonNegative(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	now := time.Now().Unix()
	stale := now - 400*86400

	// Same setup as above — one stale edge.
	srcID, _ := seedDecayGraph(t, s, "X", "KNOWS", "Y", 5.0, stale)

	// horizonDays = -1 → decay disabled → raw-weight filter only.
	// With raw weight 5.0 and threshold 0.5, Y must appear.
	entities, err := traverseNeighborhood(context.Background(), s.db,
		[]int64{srcID}, 2, 0.5, 10, -1)
	if err != nil {
		t.Fatalf("traverseNeighborhood: %v", err)
	}

	var foundY bool
	for _, e := range entities {
		if e.Name == "Y" {
			foundY = true
		}
	}
	if !foundY {
		t.Error("with horizon=-1 (disabled), stale but high-weight edge must pass filter")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd <repo>/gormes
go test ./internal/memory/... -run "TestTraverseNeighborhood_DecayFiltersStaleEdges|TestRecall_DecayDisabledWhenHorizonNegative" -v 2>&1 | tail -15
```

Expected: compile error — `too many arguments in call to traverseNeighborhood` (current signature has 5 params, the tests pass 6).

- [ ] **Step 3: Add the `weightExpr` helper in `recall_sql.go`**

At the top of `recall_sql.go` (after the package declaration and imports, before the existing functions), add:

```go
// weightExpr returns the SQL expression that substitutes for r.weight
// in WHERE / ORDER BY clauses. When horizonDays <= 0, decay is
// disabled and the raw column reference is returned. Otherwise a
// linear-decay expression with one bound parameter (horizonSec) is
// returned; callers are responsible for binding horizonSec in the
// correct argument position.
//
// The expression: MAX(0, r.weight * (1 - age_seconds / horizon_seconds))
//   - CAST forces float division (integer division snaps to 0/1 at
//     the day boundary).
//   - MAX(0, ...) clamps rows older than horizon to exactly zero;
//     no wrap-around in ORDER BY.
//   - Uses r.updated_at (existing column); no schema change.
func weightExpr(horizonDays int) string {
	if horizonDays <= 0 {
		return "r.weight"
	}
	return "MAX(0, r.weight * (1 - CAST(strftime('%s','now') - r.updated_at AS REAL) / ?))"
}
```

- [ ] **Step 4: Update `traverseNeighborhood` signature + SQL**

Replace the existing `traverseNeighborhood` function (lines ~127-190) with:

```go
// traverseNeighborhood runs the Recursive CTE that expands a set of seed
// entity IDs into a depth-bounded neighborhood, filtered by relationship
// weight (decay-aware) >= threshold, sorted by depth ASC then updated_at
// DESC, capped at maxFacts.
//
// horizonDays controls Phase 3.E.6 decay. <= 0 disables decay and uses
// the raw weight column as the filter.
//
// Depth 0 = seeds themselves.
// Depth N = reachable via N hops along edges with effective weight >= threshold.
func traverseNeighborhood(
	ctx context.Context,
	db *sql.DB,
	seedIDs []int64,
	depth int,
	threshold float64,
	maxFacts int,
	horizonDays int,
) ([]recalledEntity, error) {
	if len(seedIDs) == 0 {
		return nil, nil
	}

	// Build the seeds VALUES() clause: (?), (?), ...
	seedValues := strings.Repeat("(?),", len(seedIDs))
	seedValues = seedValues[:len(seedValues)-1]

	// Args layout depends on whether decay is active:
	//   disabled: [seed IDs], threshold, depth, maxFacts
	//   enabled:  [seed IDs], horizonSec, threshold, depth, maxFacts
	args := make([]any, 0, len(seedIDs)+4)
	for _, id := range seedIDs {
		args = append(args, id)
	}
	if horizonDays > 0 {
		args = append(args, int64(horizonDays)*86400)
	}
	args = append(args, threshold, depth, maxFacts)

	q := fmt.Sprintf(`
		WITH RECURSIVE
			seeds(entity_id) AS (VALUES %s),
			neighborhood(entity_id, depth) AS (
				SELECT entity_id, 0 FROM seeds
				UNION
				SELECT
					CASE WHEN r.source_id = n.entity_id THEN r.target_id
					     ELSE r.source_id END,
					n.depth + 1
				FROM neighborhood n
				JOIN relationships r
					ON (r.source_id = n.entity_id OR r.target_id = n.entity_id)
				   AND %s >= ?
				WHERE n.depth < ?
			),
			dedup_neighborhood AS (
				SELECT entity_id, MIN(depth) AS depth
				FROM neighborhood
				GROUP BY entity_id
			)
		SELECT e.name, e.type, COALESCE(e.description, '')
		FROM dedup_neighborhood dn
		JOIN entities e ON e.id = dn.entity_id
		ORDER BY dn.depth ASC, e.updated_at DESC
		LIMIT ?`, seedValues, weightExpr(horizonDays))

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("traverseNeighborhood: %w", err)
	}
	defer rows.Close()

	var out []recalledEntity
	for rows.Next() {
		var e recalledEntity
		if err := rows.Scan(&e.Name, &e.Type, &e.Description); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Update the call site in `recall.go`**

Find the call site around line 167 in `internal/memory/recall.go`:

```go
	entities, err := traverseNeighborhood(ctx, p.store.db,
		seeds, p.cfg.Depth, p.cfg.WeightThreshold, p.cfg.MaxFacts)
```

Change to:

```go
	entities, err := traverseNeighborhood(ctx, p.store.db,
		seeds, p.cfg.Depth, p.cfg.WeightThreshold, p.cfg.MaxFacts,
		p.cfg.DecayHorizonDays)
```

- [ ] **Step 6: Run, expect PASS**

```bash
cd <repo>/gormes
go test -race ./internal/memory/... -run "TestTraverseNeighborhood_DecayFiltersStaleEdges|TestRecall_DecayDisabledWhenHorizonNegative" -v -timeout 30s
```

Both tests pass.

**Other recall tests may break** because the existing tests call `traverseNeighborhood(...)` with 5 params. Check:

```bash
go test -race ./internal/memory/... -run "TestTraverseNeighborhood" -v -timeout 30s 2>&1 | grep -E "^(---|FAIL|PASS|ok)" | head
```

If any existing test fails with a signature-mismatch compile error, add `, 0` as the last argument to those calls (horizon=0 → defaults to 180 via `withDefaults`... wait, the helper itself checks `<= 0 → disabled`, so passing 0 directly here means "disabled", which matches legacy behavior. Pre-existing tests that seeded fresh data will still pass because fresh data has effective_weight ≈ raw_weight, so threshold behavior is unchanged).

Actually the cleanest fix for pre-existing test calls: pass `0` as the horizon argument. That signals "disabled" at the helper level, which means the SQL becomes the exact legacy expression. Pre-existing tests verify legacy behavior, so `0` is the right value.

Full memory suite:

```bash
go test -race ./internal/memory/... -count=1 -timeout 60s -skip Integration_Ollama
```

Green.

- [ ] **Step 7: Commit**

```bash
git add gormes/internal/memory/recall_sql.go gormes/internal/memory/recall.go gormes/internal/memory/recall_sql_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): traverseNeighborhood gains Phase 3.E.6 decay

Adds weightExpr(horizonDays) helper that returns either the raw
r.weight column or the linear-decay expression
  MAX(0, r.weight * (1 - age_seconds / horizon_seconds))
where age = strftime('%s','now') - r.updated_at (CAST to REAL
for float division).

traverseNeighborhood signature grows by one param (horizonDays
int). The recall.go call site passes p.cfg.DecayHorizonDays.
When horizonDays <= 0, the helper substitutes the legacy raw
expression — no behavioral change, no new bound parameter.

Two tests lock the contract:
  - DecayFiltersStaleEdges: edge with weight=5 and age=400d is
    filtered out with horizon=180 (effective weight = 0).
  - DecayDisabledWhenHorizonNegative: same edge survives with
    horizon=-1 (raw-weight filter only).

Pre-existing traverseNeighborhood tests pass 0 as the horizon,
matching legacy behavior via the <=0-disabled helper branch.

enumerateRelationships gets the same treatment in T3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `enumerateRelationships` decay (WHERE + ORDER BY)

**Files:**
- Modify: `gormes/internal/memory/recall_sql.go`
- Modify: `gormes/internal/memory/recall.go` (one call site)
- Modify: `gormes/internal/memory/recall_sql_test.go`

- [ ] **Step 1: Write failing tests — append to `recall_sql_test.go`**

```go
func TestEnumerateRelationships_DecayOrdersByEffectiveWeight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	now := time.Now().Unix()

	// Two edges on the same entity pair:
	//   X -> Y weight=5, age=300d → effective = 5 * (1 - 300/180) = negative → clamped to 0
	//   X -> Z weight=2, age=30d  → effective = 2 * (1 - 30/180) ≈ 1.67
	// With horizon=180, threshold=0.5: only X->Z must appear.
	stale := now - 300*86400
	fresh := now - 30*86400
	xID, yID := seedDecayGraph(t, s, "X", "KNOWS", "Y", 5.0, stale)
	_, zID := seedDecayGraph(t, s, "X2", "KNOWS", "Z", 2.0, fresh)
	_ = xID
	_ = yID

	// Get X2's id.
	var x2ID int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name='X2'`).Scan(&x2ID)

	neighborhoodIDs := []int64{xID, yID, x2ID, zID}

	rels, err := enumerateRelationships(context.Background(), s.db,
		neighborhoodIDs, 0.5, 10, 180)
	if err != nil {
		t.Fatalf("enumerateRelationships: %v", err)
	}

	if len(rels) != 1 {
		t.Fatalf("got %d rels, want 1 (only fresh X2->Z should pass decay filter)", len(rels))
	}
	if rels[0].Source != "X2" || rels[0].Target != "Z" {
		t.Errorf("got %s -> %s, want X2 -> Z", rels[0].Source, rels[0].Target)
	}
}

func TestRecall_DecayRawWeightInFenceUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	now := time.Now().Unix()
	// Mid-age edge: 30 days old, weight=5.0.
	// With horizon=180: effective = 5 * (1 - 30/180) ≈ 4.17.
	// The returned row's .Weight field must be 5.0 (RAW), not 4.17.
	srcID, tgtID := seedDecayGraph(t, s, "Src", "KNOWS", "Tgt", 5.0, now-30*86400)

	rels, err := enumerateRelationships(context.Background(), s.db,
		[]int64{srcID, tgtID}, 0.5, 10, 180)
	if err != nil {
		t.Fatalf("enumerateRelationships: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("got %d rels, want 1", len(rels))
	}
	// Absolute equality: raw weight is an exact 5.0 float.
	if rels[0].Weight != 5.0 {
		t.Errorf("fence Weight = %v, want 5.0 (raw); decay must not leak into the displayed value",
			rels[0].Weight)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd <repo>/gormes
go test ./internal/memory/... -run "TestEnumerateRelationships_DecayOrdersByEffectiveWeight|TestRecall_DecayRawWeightInFenceUnchanged" -v 2>&1 | tail -10
```

Expected: compile error — `too many arguments in call to enumerateRelationships`.

- [ ] **Step 3: Update `enumerateRelationships` signature + SQL**

Replace the existing `enumerateRelationships` function (lines ~192-253 in `recall_sql.go`) with:

```go
// enumerateRelationships fetches all relationships where BOTH source_id
// and target_id are inside the given entity ID set, filtered by effective
// weight (decay-aware) >= threshold, sorted by effective weight DESC
// then source-name ASC then target-name ASC, capped at limit. Returns
// joined rows (source name, predicate, target name, RAW weight) ready
// for formatting into the fenced block.
//
// horizonDays controls Phase 3.E.6 decay (<= 0 disables). The SELECT
// returns r.weight (raw) so the fence shows honest pre-decay numbers
// to the operator; decay only affects ranking + filtering.
//
// AND (not OR) on the IN clauses: we want relationships WITHIN the
// neighborhood, not ones that merely touch it.
func enumerateRelationships(
	ctx context.Context,
	db *sql.DB,
	neighborhoodIDs []int64,
	threshold float64,
	limit int,
	horizonDays int,
) ([]recalledRel, error) {
	if len(neighborhoodIDs) == 0 {
		return nil, nil
	}

	placeholders := strings.Repeat("?,", len(neighborhoodIDs))
	placeholders = placeholders[:len(placeholders)-1]

	// Args layout:
	//   disabled: [source IN], [target IN], threshold, limit
	//   enabled:  [source IN], [target IN], horizonSec_WHERE, threshold,
	//             horizonSec_ORDER, limit
	// Two horizon binds because SQLite positional placeholders don't share
	// across clauses.
	args := make([]any, 0, 2*len(neighborhoodIDs)+4)
	for _, id := range neighborhoodIDs {
		args = append(args, id)
	}
	for _, id := range neighborhoodIDs {
		args = append(args, id)
	}
	horizonSec := int64(horizonDays) * 86400
	if horizonDays > 0 {
		args = append(args, horizonSec)
	}
	args = append(args, threshold)
	if horizonDays > 0 {
		args = append(args, horizonSec)
	}
	args = append(args, limit)

	expr := weightExpr(horizonDays)
	q := fmt.Sprintf(`
		SELECT e1.name, r.predicate, e2.name, r.weight
		FROM relationships r
		JOIN entities e1 ON r.source_id = e1.id
		JOIN entities e2 ON r.target_id = e2.id
		WHERE r.source_id IN (%s)
		  AND r.target_id IN (%s)
		  AND %s >= ?
		ORDER BY %s DESC, e1.name ASC, e2.name ASC
		LIMIT ?`, placeholders, placeholders, expr, expr)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("enumerateRelationships: %w", err)
	}
	defer rows.Close()

	var out []recalledRel
	for rows.Next() {
		var r recalledRel
		if err := rows.Scan(&r.Source, &r.Predicate, &r.Target, &r.Weight); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Update the call site in `recall.go`**

Find the call site around line 183:

```go
	rels, err := enumerateRelationships(ctx, p.store.db,
		neighborhoodIDs, p.cfg.WeightThreshold, p.cfg.MaxFacts)
```

Change to:

```go
	rels, err := enumerateRelationships(ctx, p.store.db,
		neighborhoodIDs, p.cfg.WeightThreshold, p.cfg.MaxFacts,
		p.cfg.DecayHorizonDays)
```

- [ ] **Step 5: Run, expect PASS**

```bash
cd <repo>/gormes
go test -race ./internal/memory/... -run "TestEnumerateRelationships_DecayOrdersByEffectiveWeight|TestRecall_DecayRawWeightInFenceUnchanged" -v -timeout 30s
```

Both new tests pass.

**Fix pre-existing `enumerateRelationships` test calls** the same way T2 did for `traverseNeighborhood`:

```bash
grep -n "enumerateRelationships(" internal/memory/recall_sql_test.go internal/memory/*_test.go 2>/dev/null | grep -v "horizonDays"
```

For any call with the old 5-arg signature, add `, 0` as the final arg (decay disabled = legacy behavior).

Full memory suite:

```bash
go test -race ./internal/memory/... -count=1 -timeout 60s -skip Integration_Ollama
```

Green.

- [ ] **Step 6: Commit**

```bash
git add gormes/internal/memory/recall_sql.go gormes/internal/memory/recall.go gormes/internal/memory/recall_sql_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): enumerateRelationships gains decay + preserves raw weight

Applies Phase 3.E.6 decay to the relationship enumeration
used by <memory-context> fence assembly. Two substitutions:
WHERE (filter) and ORDER BY (rank). Two horizon binds because
SQLite positional placeholders don't share across clauses.

The SELECT still returns r.weight (raw), not the decayed
expression. Operator-facing weight numbers in the fence stay
honest — decay is an internal ranking mechanic, not a
displayed value.

Two tests:
  - DecayOrdersByEffectiveWeight: weight=5 age=300d is
    outranked+filtered by weight=2 age=30d (effective
    weights 0 vs 1.67).
  - DecayRawWeightInFenceUnchanged: 30d-old weight=5 edge
    returns Weight=5.0 (raw), not 4.17 (decayed).

Pre-existing enumerateRelationships calls updated to pass
horizonDays=0 (disabled → legacy behavior preserved).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: TelegramCfg knob + cmd plumbing + config test

**Files:**
- Modify: `gormes/internal/config/config.go`
- Modify: `gormes/internal/config/config_test.go`
- Modify: `gormes/cmd/gormes/telegram.go`

- [ ] **Step 1: Write failing test — append to `config_test.go`**

```go
func TestLoad_RecallDecayHorizonDays(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telegram.RecallDecayHorizonDays != 180 {
		t.Errorf("RecallDecayHorizonDays default = %d, want 180",
			cfg.Telegram.RecallDecayHorizonDays)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd <repo>/gormes
go test ./internal/config/... -run TestLoad_RecallDecayHorizonDays -v 2>&1 | tail -5
```

Expected: compile error — `unknown field RecallDecayHorizonDays on TelegramCfg`.

- [ ] **Step 3: Add the field to `TelegramCfg`**

In `internal/config/config.go`, find `type TelegramCfg struct { ... }`. Append a new field (after the existing Recall fields — `RecallDepth`):

```go
	// RecallDecayHorizonDays (Phase 3.E.6) — maps to
	// RecallConfig.DecayHorizonDays. An edge's effective weight
	// decays linearly from raw at age=0 to 0 at this many days old.
	// 0 = unset (withDefaults promotes to 180). <0 = disabled.
	RecallDecayHorizonDays int `toml:"recall_decay_horizon_days"`
```

- [ ] **Step 4: Add default in `defaults()`**

In the same file, find `func defaults() Config { ... }`. Inside the `Telegram: TelegramCfg{ ... }` literal, add (near the other Recall defaults):

```go
			RecallDecayHorizonDays: 180,
```

- [ ] **Step 5: Plumb into `cmd/gormes/telegram.go`**

Find the `memory.NewRecall(mstore, memory.RecallConfig{...}` call in `cmd/gormes/telegram.go`. Inside the `RecallConfig{...}` literal, add:

```go
			DecayHorizonDays: cfg.Telegram.RecallDecayHorizonDays,
```

Place it alongside the other Recall-related fields (e.g., near `RecallDepth`).

- [ ] **Step 6: Run, expect PASS**

```bash
cd <repo>/gormes
go test -race ./internal/config/... -run TestLoad_RecallDecayHorizonDays -v
go vet ./...
go build ./...
```

Config test passes. Build clean. `go vet` clean.

Full sweep (minus Ollama):

```bash
go test -race ./... -count=1 -timeout 240s -skip Integration_Ollama
```

Green (minus pre-existing docs drift — unrelated to this task).

- [ ] **Step 7: Commit**

```bash
git add gormes/internal/config/config.go gormes/internal/config/config_test.go gormes/cmd/gormes/telegram.go
git commit -m "$(cat <<'EOF'
feat(gormes/config): Phase 3.E.6 — [telegram].recall_decay_horizon_days

Operator-visible TOML knob for Phase 3.E.6 memory decay. Default
180 days (6 months). Plumbs cfg.Telegram.RecallDecayHorizonDays
into memory.RecallConfig.DecayHorizonDays at the telegram
subcommand's NewRecall call site.

Opt-out: set '[telegram].recall_decay_horizon_days = -1' in
config.toml to preserve the legacy raw-weight recall behavior.

One test locks the default (180). Existing Recall tests
continue to pass because they construct RecallConfig directly
with test values, not via the TOML loader.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Verification sweep

**Files:** no changes — verification only.

- [ ] **Step 1: Full sweep under -race (skip Ollama)**

```bash
cd <repo>/gormes
go test -race ./... -count=1 -timeout 240s -skip Integration_Ollama
go vet ./...
```

Expected: all packages green (except pre-existing docs Hugo drift — unrelated to 3.E.6).

- [ ] **Step 2: Binary size**

```bash
cd <repo>/gormes
make build
ls -lh bin/gormes
```

Expected: `bin/gormes` stays at ~17 MB (no new deps, no new code paths).

- [ ] **Step 3: Kernel isolation**

```bash
cd <repo>/gormes
(go list -deps ./internal/kernel | grep -E "ncruces|internal/memory|internal/session|internal/cron") \
  && echo "VIOLATION" || echo "OK: kernel isolated"
```

Expected: `OK`. 3.E.6 is memory-package-local.

- [ ] **Step 4: Raw-data audit**

Confirm the `weight` and `updated_at` columns are untouched by decay (the audit-preserving invariant):

```bash
cd <repo>/gormes
git diff bdaffd25..HEAD -- internal/memory/graph.go
```

Expected: empty diff. The extractor's `ON CONFLICT DO UPDATE` path (the only writer to `relationships.weight` + `.updated_at`) is unchanged. Decay is purely a SELECT-path feature.

- [ ] **Step 5: Offline doctor still works**

```bash
cd <repo>/gormes
./bin/gormes doctor --offline
```

Expected: `[PASS] Toolbox: 3 tools registered`.

- [ ] **Step 6: Ship-criterion manual verification (optional)**

This matches the spec's ship criterion. Requires a live SQLite file with seeded data — skip if running in CI, do locally when exercising the feature:

```bash
# Create a fresh DB, seed two same-weight edges with different ages.
rm -rf /tmp/gormes-decay-test && mkdir -p /tmp/gormes-decay-test/gormes
sqlite3 /tmp/gormes-decay-test/gormes/memory.db <<'SQL'
CREATE TABLE schema_meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
INSERT INTO schema_meta(k,v) VALUES ('version','3e');
SQL

export XDG_DATA_HOME=/tmp/gormes-decay-test
# Run the gormes binary briefly to get the full schema via migration.
GORMES_TELEGRAM_TOKEN=fake:tok GORMES_TELEGRAM_CHAT_ID=99 \
  timeout 1 ./bin/gormes telegram > /dev/null 2>&1 || true

# Seed two edges.
sqlite3 /tmp/gormes-decay-test/gormes/memory.db <<SQL
INSERT INTO entities(name,type,updated_at) VALUES('Alice','PERSON',$(date +%s)),
                                                  ('Bob','PERSON',$(date +%s));
INSERT INTO relationships(source_id,target_id,predicate,weight,updated_at) VALUES
  (1,2,'KNOWS',5.0,$(date +%s)),
  (1,2,'LIKES',5.0,$(date -d '300 days ago' +%s));
SQL

echo "Fresh edge:"
sqlite3 /tmp/gormes-decay-test/gormes/memory.db "SELECT predicate, weight, (strftime('%s','now') - updated_at)/86400 AS age_days FROM relationships"

# Effective weights with horizon=180:
#   KNOWS (age 0):   5 * (1 - 0/180) = 5.0
#   LIKES (age 300): MAX(0, 5 * (1 - 300/180)) = 0
# LIKES must not appear in decay-applied recall.
echo ""
echo "Effective weights with horizon=180:"
sqlite3 /tmp/gormes-decay-test/gormes/memory.db \
  "SELECT predicate, MAX(0, weight * (1 - CAST(strftime('%s','now') - updated_at AS REAL) / (180 * 86400))) AS effective FROM relationships"

rm -rf /tmp/gormes-decay-test
```

Expected output (approximate, timestamps vary):

```
Fresh edge:
KNOWS|5.0|0
LIKES|5.0|300

Effective weights with horizon=180:
KNOWS|5.0
LIKES|0.0
```

- [ ] **Step 7: No commit**

If any check fails, STOP and report.

---

## Appendix: Self-Review

**Spec coverage** (spec § → task):

| Spec § | Task(s) |
|---|---|
| §1 Goal | T2, T3 (core decay); T1 (config scaffold); T4 (operator knob) |
| §2 Non-goals | Enforced by scope — no schema migration, no entity decay, no `last_referenced_at`, no CLI |
| §3 Scope | T1–T5 |
| §4 Architecture (decay expression + helper + disabled path) | T2 |
| §5 Application sites (traverseNeighborhood + enumerateRelationships) | T2, T3 |
| §6 Config (RecallConfig.DecayHorizonDays + TelegramCfg.RecallDecayHorizonDays + plumbing) | T1, T4 |
| §7 Testing (4 unit + 2 config) | T1 (config), T2 (2 unit), T3 (2 unit), T4 (1 config) |
| §8 Error handling | No new error surface — validated inline in T2/T3 |
| §9 Rollout | T4 ships the operator default |
| §10 Binary size | T5 verifies |
| §11 Invariants | T5.4 raw-data audit; T5.3 kernel isolation |

**Placeholder scan:** zero `TBD` / `TODO` / "fill in" / "similar to Task N" / vague "handle errors".

**Type consistency:**
- `RecallConfig.DecayHorizonDays int` — declared T1, consumed T2 + T3 call-site patches, T4 plumbing.
- `traverseNeighborhood(ctx, db, seedIDs, depth, threshold, maxFacts, horizonDays)` — 7 params, matches T2 + T5 verification call sites.
- `enumerateRelationships(ctx, db, neighborhoodIDs, threshold, limit, horizonDays)` — 6 params, matches T3 + T5 verification call sites.
- `weightExpr(horizonDays int) string` — declared T2, consumed T2 (traverseNeighborhood) + T3 (enumerateRelationships).
- `TelegramCfg.RecallDecayHorizonDays int` (TOML tag `recall_decay_horizon_days`) — declared T4, consumed T4 cmd plumbing.
- Sentinel rule: `0` → default 180 (in `withDefaults`); `<0` → disabled (preserved). Applied consistently in T1 config default, T2/T3 `weightExpr` check, T4 TelegramCfg default.

**Execution order:** T1 (config scaffold, no runtime change) → T2 (`weightExpr` + traverseNeighborhood) → T3 (enumerateRelationships) → T4 (TelegramCfg + cmd) → T5 (verification). Each task produces a self-contained commit; the build is green after every task.

**Checkpoint suggestion:** halt after **T3** (both SQL sites decay-aware, 4 tests green) before T4's operator-knob commit. T4 is low-risk config plumbing and T5 is verification only.
