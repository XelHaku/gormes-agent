# Gormes Phase 3.B — Ontological Graph + Async Extractor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an `entities` + `relationships` SQLite graph populated asynchronously by an LLM extractor goroutine that polls unprocessed `turns` — without touching the kernel or the TUI.

**Architecture:** Version-gated `3a→3b` schema migration; a new `Extractor` alongside Phase 3.A's persistence worker, both sharing the single-writer `*sql.DB` pool. Extractor polls `turns WHERE extracted=0`, calls `hermes.Client.OpenStream` with a strict entity/relationship prompt, validates the JSON against an 8-item predicate whitelist (coerce to `RELATED_TO` on drift), upserts into `entities`/`relationships` in one transaction, marks turns `extracted=1`. Failures use exponential backoff capped at 60s, and a dead-letter state (`extracted=2`) at `MaxAttempts=5`.

**Tech Stack:** Go 1.25+, `github.com/ncruces/go-sqlite3` (already in go.mod from 3.A), existing `hermes.Client` interface, `database/sql`, SQLite CHECK constraints for type and predicate whitelists.

**Spec:** [`gormes/docs/superpowers/specs/2026-04-20-gormes-phase3b-graph.md`](../specs/2026-04-20-gormes-phase3b-graph.md) (approved 2026-04-20, `a11a1d81`).

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `gormes/internal/memory/schema.go` | Modify | Split monolithic `schemaDDL` into `schemaV3a` (baseline) + `migration3aTo3b` (ALTER + new tables); bump `schemaVersion = "3b"` |
| `gormes/internal/memory/migrate.go` | Create | `migrate(db) error` — version-gated runner; `ErrSchemaUnknown` sentinel |
| `gormes/internal/memory/migrate_test.go` | Create | Migration tests (3a→3b, idempotent, unknown-version, turns columns, new tables) |
| `gormes/internal/memory/memory.go` | Modify | Replace inline `db.Exec(schemaDDL)` with `migrate(db)` call |
| `gormes/internal/memory/validator.go` | Create | `extractorOutput` struct + `ValidateExtractorOutput` pure function |
| `gormes/internal/memory/validator_test.go` | Create | Validator unit tests (empty name, orphan rel, type coerce, predicate coerce, weight clamp) |
| `gormes/internal/memory/graph.go` | Create | `upsertEntities` + `upsertRelationships` + `markTurnsExtracted` + `incrementAttempts` — one tx per call |
| `gormes/internal/memory/graph_test.go` | Create | Graph upsert tests (dedup, weight accumulation, weight cap, description preservation, CHECK enforcement) |
| `gormes/internal/memory/extractor.go` | Create | `Extractor` type + `ExtractorConfig` + `NewExtractor` + `Run` + `Close` + `callLLM` + `loopOnce` |
| `gormes/internal/memory/extractor_test.go` | Create | Extractor end-to-end tests using a `fakeLLM` implementing `hermes.Client` |
| `gormes/internal/memory/extractor_prompt.go` | Create | `extractorSystemPrompt` constant; `formatBatchPrompt(turns []turnRow) string` |
| `gormes/internal/config/config.go` | Modify | Add `TelegramCfg.ExtractorBatchSize`, `ExtractorPollInterval`; defaults (5, 10s) |
| `gormes/internal/config/config_test.go` | Modify | Append 2 tests (default + env-parse) |
| `gormes/cmd/gormes-telegram/main.go` | Modify | Construct `memory.NewExtractor`, `go ext.Run(rootCtx)`, `defer ext.Close(shutdownCtx)` |

---

## Task 1: Schema migration infrastructure + 3a→3b DDL

**Files:**
- Modify: `gormes/internal/memory/schema.go`
- Create: `gormes/internal/memory/migrate.go`
- Create: `gormes/internal/memory/migrate_test.go`
- Modify: `gormes/internal/memory/memory.go`

- [ ] **Step 1: Write failing tests FIRST**

Create `gormes/internal/memory/migrate_test.go`:

```go
package memory

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenSqlite_FreshDBIsV3b(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer s.Close(context.Background())

	var v string
	_ = s.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != "3b" {
		t.Errorf("schema version = %q, want 3b", v)
	}
}

func TestMigrate_TurnsGainsExtractedColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	for _, col := range []string{"extracted", "extraction_attempts", "extraction_error"} {
		var name string
		row := s.db.QueryRow(
			`SELECT name FROM pragma_table_info('turns') WHERE name = ?`, col)
		if err := row.Scan(&name); err != nil {
			t.Errorf("column %q missing from turns: %v", col, err)
		}
	}
}

func TestMigrate_EntitiesAndRelationshipsExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	for _, table := range []string{"entities", "relationships"} {
		var n int
		err := s.db.QueryRow(
			`SELECT COUNT(*) FROM `+table).Scan(&n)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close(context.Background())

	// Re-open — migration runs against v3b, should no-op.
	s2, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("re-open failed: %v", err)
	}
	defer s2.Close(context.Background())

	var v string
	_ = s2.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != "3b" {
		t.Errorf("version = %q after re-open, want 3b", v)
	}
}

func TestMigrate_UnknownVersionRefuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	// Corrupt: set version to a future value directly.
	_, _ = s.db.Exec(`UPDATE schema_meta SET v = '3z' WHERE k = 'version'`)
	s.Close(context.Background())

	_, err := OpenSqlite(path, 0, nil)
	if !errors.Is(err, ErrSchemaUnknown) {
		t.Errorf("err = %v, want errors.Is(err, ErrSchemaUnknown)", err)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run "TestOpenSqlite_FreshDBIsV3b|TestMigrate_" 2>&1 | head -15
```

Expected: `undefined: ErrSchemaUnknown` and/or assertion failures (current schema is v3a).

- [ ] **Step 3: Split `schema.go` into baseline + migration fragments**

Replace `gormes/internal/memory/schema.go` with:

```go
package memory

// schemaVersion is the canonical target version for this binary. OpenSqlite
// migrates any earlier supported version up to this value, and refuses to
// open DBs with an unknown version (future schemas).
const schemaVersion = "3b"

// schemaV3a is the baseline schema installed on a fresh DB. It matches
// exactly what Phase 3.A shipped — any change to this string is a schema
// migration and must go through the version-gated migrate() path.
const schemaV3a = `
CREATE TABLE IF NOT EXISTS schema_meta (
	k TEXT PRIMARY KEY,
	v TEXT NOT NULL
);

INSERT OR IGNORE INTO schema_meta(k, v) VALUES ('version', '3a');

CREATE TABLE IF NOT EXISTS turns (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id  TEXT    NOT NULL,
	role        TEXT    NOT NULL CHECK(role IN ('user','assistant')),
	content     TEXT    NOT NULL,
	ts_unix     INTEGER NOT NULL,
	meta_json   TEXT
);

CREATE INDEX IF NOT EXISTS idx_turns_session_ts
	ON turns(session_id, ts_unix);

CREATE VIRTUAL TABLE IF NOT EXISTS turns_fts USING fts5(
	content,
	content='turns',
	content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS turns_ai AFTER INSERT ON turns BEGIN
	INSERT INTO turns_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS turns_ad AFTER DELETE ON turns BEGIN
	INSERT INTO turns_fts(turns_fts, rowid, content) VALUES('delete', old.id, old.content);
END;

CREATE TRIGGER IF NOT EXISTS turns_au AFTER UPDATE ON turns BEGIN
	INSERT INTO turns_fts(turns_fts, rowid, content) VALUES('delete', old.id, old.content);
	INSERT INTO turns_fts(rowid, content) VALUES (new.id, new.content);
END;
`

// migration3aTo3b extends v3a with the Ontological Graph:
//   - turns gains extracted / extraction_attempts / extraction_error columns
//   - partial index idx_turns_unextracted for O(log n) polling
//   - entities + relationships tables with type/predicate CHECK whitelists
const migration3aTo3b = `
ALTER TABLE turns ADD COLUMN extracted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE turns ADD COLUMN extraction_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE turns ADD COLUMN extraction_error TEXT;
CREATE INDEX IF NOT EXISTS idx_turns_unextracted
	ON turns(id) WHERE extracted = 0;

CREATE TABLE IF NOT EXISTS entities (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	name        TEXT    NOT NULL,
	type        TEXT    NOT NULL CHECK(type IN (
	                'PERSON','PROJECT','CONCEPT','PLACE','ORGANIZATION','TOOL','OTHER'
	            )),
	description TEXT,
	updated_at  INTEGER NOT NULL,
	UNIQUE(name, type)
);
CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(type);
CREATE INDEX IF NOT EXISTS idx_entities_name ON entities(name);

CREATE TABLE IF NOT EXISTS relationships (
	source_id   INTEGER NOT NULL,
	target_id   INTEGER NOT NULL,
	predicate   TEXT    NOT NULL CHECK(predicate IN (
	                'WORKS_ON','KNOWS','LIKES','DISLIKES',
	                'HAS_SKILL','LOCATED_IN','PART_OF','RELATED_TO'
	            )),
	weight      REAL    NOT NULL DEFAULT 1.0,
	updated_at  INTEGER NOT NULL,
	PRIMARY KEY(source_id, target_id, predicate),
	FOREIGN KEY(source_id) REFERENCES entities(id) ON DELETE CASCADE,
	FOREIGN KEY(target_id) REFERENCES entities(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_relationships_target ON relationships(target_id);
CREATE INDEX IF NOT EXISTS idx_relationships_predicate ON relationships(predicate);

UPDATE schema_meta SET v = '3b' WHERE k = 'version' AND v = '3a';
`
```

- [ ] **Step 4: Create `migrate.go`**

Create `gormes/internal/memory/migrate.go`:

```go
package memory

import (
	"database/sql"
	"errors"
	"fmt"
)

// ErrSchemaUnknown is returned by OpenSqlite when the DB's schema version
// is neither the current target nor any known predecessor. Callers should
// exit 1 with a clear message; the DB may have been written by a future
// binary.
var ErrSchemaUnknown = errors.New("memory: schema version unknown to this binary")

// migrate installs or upgrades the DB schema to schemaVersion. Safe to
// call on a fresh DB (installs v3a then migrates to current) or on any
// previously-migrated DB (runs only the needed steps). Single transaction
// per migration step so a failure leaves the DB in a consistent state.
func migrate(db *sql.DB) error {
	// Ensure schema_meta + v3a baseline exist. Idempotent on re-run.
	if _, err := db.Exec(schemaV3a); err != nil {
		return fmt.Errorf("memory: apply v3a baseline: %w", err)
	}

	var v string
	if err := db.QueryRow(`SELECT v FROM schema_meta WHERE k = 'version'`).Scan(&v); err != nil {
		return fmt.Errorf("memory: read schema version: %w", err)
	}

	switch v {
	case "3a":
		if err := runMigrationTx(db, migration3aTo3b); err != nil {
			return fmt.Errorf("memory: migrate 3a->3b: %w", err)
		}
		return nil
	case "3b":
		return nil // already at target
	default:
		return fmt.Errorf("%w: got %q, want %q", ErrSchemaUnknown, v, schemaVersion)
	}
}

// runMigrationTx applies a multi-statement DDL script in a single
// transaction. If any statement fails, the transaction rolls back and the
// DB stays at its previous version.
func runMigrationTx(db *sql.DB, ddl string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op after successful Commit

	if _, err := tx.Exec(ddl); err != nil {
		return err
	}
	return tx.Commit()
}
```

- [ ] **Step 5: Replace `memory.go`'s inline schema-apply with `migrate()`**

In `gormes/internal/memory/memory.go`, find the block:

```go
	if _, err := db.Exec(schemaDDL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory: apply schema: %w", err)
	}
```

Replace with:

```go
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
```

The `schemaDDL` constant no longer exists (split into `schemaV3a` + `migration3aTo3b`). `go build` will catch any lingering reference.

- [ ] **Step 6: Run tests, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -v
```

Expected: all 5 new migration tests PASS + every pre-existing Phase 3.A test still PASSes (schema queries that just assert `turns` exists still work; the FTS5 tests still pass because triggers are in `schemaV3a`).

Also:
```bash
cd gormes
go test -race ./... -count=1 -timeout 120s
go vet ./...
```
All green, vet clean.

- [ ] **Step 7: Commit (from repo root, one level above `gormes/`)**

```bash
git add gormes/internal/memory/schema.go \
        gormes/internal/memory/migrate.go \
        gormes/internal/memory/migrate_test.go \
        gormes/internal/memory/memory.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): schemaVersion=3b + version-gated migration

Phase 3.B schema foundation. Splits the monolithic schemaDDL
constant into two fragments:
  - schemaV3a: the Phase-3.A baseline (turns + turns_fts + triggers),
    applied idempotently on every OpenSqlite as the "install fresh DB"
    path
  - migration3aTo3b: the 3.B extensions — turns.extracted columns,
    partial index, entities table with 7-item type CHECK, relationships
    table with 8-item predicate CHECK, FK cascades

migrate() reads schema_meta.v, runs the 3a->3b step iff currently 3a,
refuses unknown values with ErrSchemaUnknown. Each step runs in its
own transaction so a failed migration leaves the DB at its prior
version, not mid-way.

No extractor logic yet — that comes in Tasks 4-11. This commit just
prepares the schema and the migration seam.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Validator (pure function, zero deps)

**Files:**
- Create: `gormes/internal/memory/validator.go`
- Create: `gormes/internal/memory/validator_test.go`

- [ ] **Step 1: Write failing tests FIRST**

Create `gormes/internal/memory/validator_test.go`:

```go
package memory

import (
	"testing"
)

func TestValidate_HappyPath(t *testing.T) {
	raw := `{"entities":[
		{"name":"Jose","type":"PERSON","description":"the user"},
		{"name":"Gormes","type":"PROJECT","description":""}
	],"relationships":[
		{"source":"Jose","target":"Gormes","predicate":"WORKS_ON","weight":0.8}
	]}`

	out, err := ValidateExtractorOutput([]byte(raw))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(out.Entities) != 2 {
		t.Errorf("entities count = %d, want 2", len(out.Entities))
	}
	if len(out.Relationships) != 1 {
		t.Errorf("relationships count = %d, want 1", len(out.Relationships))
	}
	if out.Relationships[0].Predicate != "WORKS_ON" {
		t.Errorf("predicate = %q, want WORKS_ON", out.Relationships[0].Predicate)
	}
}

func TestValidate_MalformedJSONReturnsError(t *testing.T) {
	_, err := ValidateExtractorOutput([]byte("not json"))
	if err == nil {
		t.Error("err = nil, want non-nil")
	}
}

func TestValidate_DropsEmptyName(t *testing.T) {
	raw := `{"entities":[
		{"name":"","type":"PERSON","description":""},
		{"name":"Kept","type":"CONCEPT","description":""}
	],"relationships":[]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	if len(out.Entities) != 1 || out.Entities[0].Name != "Kept" {
		t.Errorf("entities = %+v, want just 'Kept'", out.Entities)
	}
}

func TestValidate_CoercesInvalidTypeToOther(t *testing.T) {
	raw := `{"entities":[
		{"name":"Nowhere","type":"BUILDING","description":""}
	],"relationships":[]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	if len(out.Entities) != 1 || out.Entities[0].Type != "OTHER" {
		t.Errorf("type = %q, want OTHER", out.Entities[0].Type)
	}
}

func TestValidate_DropsOrphanRelationships(t *testing.T) {
	raw := `{"entities":[
		{"name":"A","type":"PERSON","description":""}
	],"relationships":[
		{"source":"A","target":"B","predicate":"KNOWS","weight":1.0},
		{"source":"A","target":"A","predicate":"KNOWS","weight":1.0}
	]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	if len(out.Relationships) != 1 {
		t.Errorf("relationships = %+v, want only A->A (B is orphan)", out.Relationships)
	}
}

func TestValidate_ClampsWeight(t *testing.T) {
	raw := `{"entities":[
		{"name":"A","type":"PERSON","description":""},
		{"name":"B","type":"PERSON","description":""}
	],"relationships":[
		{"source":"A","target":"B","predicate":"KNOWS","weight":1.5},
		{"source":"A","target":"B","predicate":"LIKES","weight":-0.3}
	]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	for _, r := range out.Relationships {
		if r.Weight < 0.0 || r.Weight > 1.0 {
			t.Errorf("predicate=%s weight=%v out of [0,1]", r.Predicate, r.Weight)
		}
	}
}

func TestValidate_NormalizesPredicate(t *testing.T) {
	raw := `{"entities":[
		{"name":"A","type":"PERSON","description":""},
		{"name":"B","type":"PERSON","description":""}
	],"relationships":[
		{"source":"A","target":"B","predicate":"works on","weight":0.5}
	]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	if len(out.Relationships) != 1 || out.Relationships[0].Predicate != "WORKS_ON" {
		t.Errorf("predicate = %q, want WORKS_ON", out.Relationships[0].Predicate)
	}
}

func TestValidate_CoercesUnknownPredicateToRelatedTo(t *testing.T) {
	raw := `{"entities":[
		{"name":"A","type":"PERSON","description":""},
		{"name":"B","type":"PROJECT","description":""}
	],"relationships":[
		{"source":"A","target":"B","predicate":"BUILT","weight":1.0}
	]}`

	out, _ := ValidateExtractorOutput([]byte(raw))
	if len(out.Relationships) != 1 {
		t.Fatalf("relationships len = %d, want 1", len(out.Relationships))
	}
	if out.Relationships[0].Predicate != "RELATED_TO" {
		t.Errorf("predicate = %q, want RELATED_TO (coerced)", out.Relationships[0].Predicate)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run TestValidate_ 2>&1 | head -5
```

Expected: `undefined: ValidateExtractorOutput`.

- [ ] **Step 3: Write `validator.go`**

Create `gormes/internal/memory/validator.go`:

```go
package memory

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// entityTypes is the CHECK whitelist from migration3aTo3b's entities table.
var entityTypes = map[string]struct{}{
	"PERSON": {}, "PROJECT": {}, "CONCEPT": {}, "PLACE": {},
	"ORGANIZATION": {}, "TOOL": {}, "OTHER": {},
}

// predicateWhitelist is the CHECK whitelist from migration3aTo3b's
// relationships table. The order matters only for deterministic test
// output — validator code uses map-membership checks.
var predicateWhitelist = map[string]struct{}{
	"WORKS_ON": {}, "KNOWS": {}, "LIKES": {}, "DISLIKES": {},
	"HAS_SKILL": {}, "LOCATED_IN": {}, "PART_OF": {}, "RELATED_TO": {},
}

// ValidatedOutput is the cleaned, whitelist-conformant result of
// validating raw LLM extractor output. Every field is safe to pass to
// the graph upsert layer without further sanitation.
type ValidatedOutput struct {
	Entities      []ValidatedEntity
	Relationships []ValidatedRelationship
}

type ValidatedEntity struct {
	Name        string
	Type        string
	Description string
}

type ValidatedRelationship struct {
	Source    string
	Target    string
	Predicate string
	Weight    float64
}

// extractorOutput is the raw wire shape the LLM is instructed to emit.
type extractorOutput struct {
	Entities      []extractedEntity       `json:"entities"`
	Relationships []extractedRelationship `json:"relationships"`
}

type extractedEntity struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type extractedRelationship struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Predicate string  `json:"predicate"`
	Weight    float64 `json:"weight"`
}

// ValidateExtractorOutput parses + sanitizes raw LLM JSON into a
// ValidatedOutput. Malformed JSON returns an error; everything else
// coerces silently (invalid types -> OTHER, unknown predicates ->
// RELATED_TO, orphan relationships dropped, etc.).
func ValidateExtractorOutput(raw []byte) (ValidatedOutput, error) {
	var wire extractorOutput
	if err := json.Unmarshal(raw, &wire); err != nil {
		return ValidatedOutput{}, fmt.Errorf("memory: extractor JSON: %w", err)
	}

	seenEntity := make(map[string]struct{}, len(wire.Entities))
	out := ValidatedOutput{}

	for _, e := range wire.Entities {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			continue
		}
		if len(name) > 255 {
			name = name[:255]
		}
		typ := strings.ToUpper(strings.TrimSpace(e.Type))
		if _, ok := entityTypes[typ]; !ok {
			typ = "OTHER"
		}
		desc := strings.TrimSpace(e.Description)
		if len(desc) > 512 {
			desc = desc[:512]
		}
		key := name + "\x00" + typ
		if _, dup := seenEntity[key]; dup {
			continue
		}
		seenEntity[key] = struct{}{}
		out.Entities = append(out.Entities, ValidatedEntity{
			Name: name, Type: typ, Description: desc,
		})
	}

	// Entity name set (any type) for orphan-check of relationships.
	knownNames := make(map[string]struct{}, len(out.Entities))
	for _, e := range out.Entities {
		knownNames[e.Name] = struct{}{}
	}

	seenRel := make(map[string]struct{})
	for _, r := range wire.Relationships {
		src := strings.TrimSpace(r.Source)
		tgt := strings.TrimSpace(r.Target)
		if src == "" || tgt == "" {
			continue
		}
		if _, ok := knownNames[src]; !ok {
			continue
		}
		if _, ok := knownNames[tgt]; !ok {
			continue
		}

		pred := normalizePredicate(r.Predicate)
		if _, ok := predicateWhitelist[pred]; !ok {
			pred = "RELATED_TO"
		}

		w := r.Weight
		if math.IsNaN(w) || w < 0 {
			w = 1.0
		}
		if w > 1.0 {
			w = 1.0
		}

		key := src + "\x00" + tgt + "\x00" + pred
		if _, dup := seenRel[key]; dup {
			continue
		}
		seenRel[key] = struct{}{}
		out.Relationships = append(out.Relationships, ValidatedRelationship{
			Source: src, Target: tgt, Predicate: pred, Weight: w,
		})
	}

	return out, nil
}

// normalizePredicate uppercases and replaces non-alphanumerics with '_'.
// "works on" -> "WORKS_ON". Empty after normalization returns "".
func normalizePredicate(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	// Collapse runs of '_' and trim leading/trailing.
	for strings.Contains(out, "__") {
		out = strings.ReplaceAll(out, "__", "_")
	}
	return strings.Trim(out, "_")
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestValidate_ -v
go vet ./...
```

All 8 validator tests PASS.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/memory/validator.go gormes/internal/memory/validator_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): validator for LLM extractor output

ValidateExtractorOutput is a pure function (zero deps beyond
encoding/json + strings + math) that sanitizes raw LLM JSON:

  - Malformed JSON -> error (caller bumps attempt counter)
  - Empty entity names -> drop
  - Invalid type -> coerce to OTHER (matches CHECK constraint)
  - Orphan relationships (source/target not in entities) -> drop
  - Unknown predicate after normalization -> coerce to RELATED_TO
  - Weight NaN/negative/>1 -> clamp to [0,1]
  - Dedup within one batch

Coerces rather than drops whenever possible — a misbehaving LLM
shouldn't cost us data. The schema CHECK constraints on
entities.type and relationships.predicate serve as the final
safety net: a coercion bug would trip them loudly at INSERT
time, not silently corrupt the graph.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Graph upsert helpers (one tx per batch)

**Files:**
- Create: `gormes/internal/memory/graph.go`
- Create: `gormes/internal/memory/graph_test.go`

- [ ] **Step 1: Write failing tests FIRST**

Create `gormes/internal/memory/graph_test.go`:

```go
package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func openGraph(t *testing.T) *SqliteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	return s
}

func TestGraph_UpsertEntityInsertsThenUpdates(t *testing.T) {
	s := openGraph(t)
	v := ValidatedOutput{
		Entities: []ValidatedEntity{
			{Name: "Jose", Type: "PERSON", Description: "first"},
		},
	}
	if err := writeGraphBatch(context.Background(), s.db, v, nil); err != nil {
		t.Fatal(err)
	}
	v.Entities[0].Description = "second"
	if err := writeGraphBatch(context.Background(), s.db, v, nil); err != nil {
		t.Fatal(err)
	}

	var n int
	var desc string
	_ = s.db.QueryRow("SELECT COUNT(*), MAX(description) FROM entities").Scan(&n, &desc)
	if n != 1 {
		t.Errorf("entities count = %d, want 1 (upsert should dedupe)", n)
	}
	if desc != "second" {
		t.Errorf("description = %q, want 'second' (non-empty must override)", desc)
	}
}

func TestGraph_UpsertEntityEmptyDescDoesNotOverwrite(t *testing.T) {
	s := openGraph(t)
	_ = writeGraphBatch(context.Background(), s.db, ValidatedOutput{
		Entities: []ValidatedEntity{{Name: "X", Type: "CONCEPT", Description: "original"}},
	}, nil)
	_ = writeGraphBatch(context.Background(), s.db, ValidatedOutput{
		Entities: []ValidatedEntity{{Name: "X", Type: "CONCEPT", Description: ""}},
	}, nil)

	var desc string
	_ = s.db.QueryRow(
		`SELECT description FROM entities WHERE name = 'X' AND type = 'CONCEPT'`,
	).Scan(&desc)
	if desc != "original" {
		t.Errorf("description = %q, want 'original' (empty must NOT overwrite)", desc)
	}
}

func TestGraph_UpsertRelationshipAccumulatesWeight(t *testing.T) {
	s := openGraph(t)
	batch := ValidatedOutput{
		Entities: []ValidatedEntity{
			{Name: "A", Type: "PERSON"},
			{Name: "B", Type: "PROJECT"},
		},
		Relationships: []ValidatedRelationship{
			{Source: "A", Target: "B", Predicate: "WORKS_ON", Weight: 0.5},
		},
	}
	_ = writeGraphBatch(context.Background(), s.db, batch, nil)
	_ = writeGraphBatch(context.Background(), s.db, batch, nil)
	_ = writeGraphBatch(context.Background(), s.db, batch, nil)

	var w float64
	_ = s.db.QueryRow(
		`SELECT weight FROM relationships WHERE predicate = 'WORKS_ON'`,
	).Scan(&w)
	if w != 1.5 {
		t.Errorf("weight = %v, want 1.5 (0.5 * 3)", w)
	}
}

func TestGraph_UpsertRelationshipWeightCapAt10(t *testing.T) {
	s := openGraph(t)
	batch := ValidatedOutput{
		Entities: []ValidatedEntity{
			{Name: "A", Type: "PERSON"},
			{Name: "B", Type: "PROJECT"},
		},
		Relationships: []ValidatedRelationship{
			{Source: "A", Target: "B", Predicate: "WORKS_ON", Weight: 1.0},
		},
	}
	for i := 0; i < 15; i++ {
		_ = writeGraphBatch(context.Background(), s.db, batch, nil)
	}

	var w float64
	_ = s.db.QueryRow(
		`SELECT weight FROM relationships WHERE predicate = 'WORKS_ON'`,
	).Scan(&w)
	if w != 10.0 {
		t.Errorf("weight = %v, want 10.0 (capped)", w)
	}
}

func TestGraph_MarkTurnsExtracted(t *testing.T) {
	s := openGraph(t)
	// Seed 3 turns via the fast path (bypassing the worker).
	_, _ = s.db.Exec(`INSERT INTO turns(session_id, role, content, ts_unix) VALUES
		('s','user','a',1),('s','user','b',2),('s','assistant','c',3)`)

	if err := writeGraphBatch(context.Background(), s.db, ValidatedOutput{}, []int64{1, 2}); err != nil {
		t.Fatal(err)
	}

	var extracted1, extracted3 int
	_ = s.db.QueryRow(`SELECT extracted FROM turns WHERE id = 1`).Scan(&extracted1)
	_ = s.db.QueryRow(`SELECT extracted FROM turns WHERE id = 3`).Scan(&extracted3)
	if extracted1 != 1 {
		t.Errorf("turn 1 extracted = %d, want 1", extracted1)
	}
	if extracted3 != 0 {
		t.Errorf("turn 3 extracted = %d, want 0 (not in batch)", extracted3)
	}
}

func TestGraph_CheckConstraintRejectsBadPredicate(t *testing.T) {
	s := openGraph(t)
	// Create two entities so FKs resolve.
	_, _ = s.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('A','PERSON',1),('B','PROJECT',1)`)

	_, err := s.db.Exec(
		`INSERT INTO relationships(source_id,target_id,predicate,updated_at) VALUES(1,2,'NOT_WHITELISTED',1)`)
	if err == nil {
		t.Error("expected CHECK constraint to reject NOT_WHITELISTED predicate")
	}
}

func TestGraph_IncrementAttempts(t *testing.T) {
	s := openGraph(t)
	_, _ = s.db.Exec(`INSERT INTO turns(session_id, role, content, ts_unix) VALUES
		('s','user','a',1),('s','user','b',2)`)

	if err := incrementAttempts(context.Background(), s.db, []int64{1, 2}, "boom"); err != nil {
		t.Fatal(err)
	}

	var attempts int
	var errMsg string
	_ = s.db.QueryRow(`SELECT extraction_attempts, extraction_error FROM turns WHERE id = 1`).Scan(&attempts, &errMsg)
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
	if errMsg != "boom" {
		t.Errorf("error = %q, want 'boom'", errMsg)
	}
}

func TestGraph_MarkDeadLetter(t *testing.T) {
	s := openGraph(t)
	_, _ = s.db.Exec(`INSERT INTO turns(session_id, role, content, ts_unix) VALUES('s','user','a',1)`)

	if err := markDeadLetter(context.Background(), s.db, []int64{1}, "final"); err != nil {
		t.Fatal(err)
	}

	var extracted int
	var errMsg string
	_ = s.db.QueryRow(`SELECT extracted, extraction_error FROM turns WHERE id = 1`).Scan(&extracted, &errMsg)
	if extracted != 2 {
		t.Errorf("extracted = %d, want 2 (dead-letter)", extracted)
	}
	if errMsg != "final" {
		t.Errorf("error = %q, want 'final'", errMsg)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run TestGraph_ 2>&1 | head -5
```

Expected: `undefined: writeGraphBatch` etc.

- [ ] **Step 3: Write `graph.go`**

Create `gormes/internal/memory/graph.go`:

```go
package memory

import (
	"context"
	"database/sql"
	"fmt"
)

// writeGraphBatch upserts the validated entities + relationships and marks
// the given turnIDs as extracted=1. One transaction for the whole batch
// so the graph is never left in a half-written state.
//
// An empty ValidatedOutput is legal (LLM found nothing); we still mark
// the turns as extracted=1 to avoid infinite retries.
func writeGraphBatch(ctx context.Context, db *sql.DB, v ValidatedOutput, turnIDs []int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Upsert entities, collect name+type -> id map.
	idByKey := make(map[string]int64, len(v.Entities))
	for _, e := range v.Entities {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO entities(name, type, description, updated_at)
			 VALUES(?, ?, ?, strftime('%s','now'))
			 ON CONFLICT(name, type) DO UPDATE SET
			   description = CASE WHEN excluded.description != ''
			                      THEN excluded.description
			                      ELSE entities.description END,
			   updated_at = excluded.updated_at`,
			e.Name, e.Type, e.Description); err != nil {
			return fmt.Errorf("upsert entity %q/%s: %w", e.Name, e.Type, err)
		}
		var id int64
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM entities WHERE name = ? AND type = ?`,
			e.Name, e.Type).Scan(&id); err != nil {
			return fmt.Errorf("resolve entity id %q/%s: %w", e.Name, e.Type, err)
		}
		idByKey[e.Name+"\x00"+e.Type] = id
	}

	// Build name -> id map too (type-agnostic; validator already guarantees
	// relationship source/target names are in entities[]).
	idByName := make(map[string]int64, len(v.Entities))
	for _, e := range v.Entities {
		idByName[e.Name] = idByKey[e.Name+"\x00"+e.Type]
	}

	// Upsert relationships.
	for _, r := range v.Relationships {
		src, srcOK := idByName[r.Source]
		tgt, tgtOK := idByName[r.Target]
		if !srcOK || !tgtOK {
			continue // defensive; validator should have dropped these
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO relationships(source_id, target_id, predicate, weight, updated_at)
			 VALUES(?, ?, ?, ?, strftime('%s','now'))
			 ON CONFLICT(source_id, target_id, predicate) DO UPDATE SET
			   weight = MIN(relationships.weight + excluded.weight, 10.0),
			   updated_at = excluded.updated_at`,
			src, tgt, r.Predicate, r.Weight); err != nil {
			return fmt.Errorf("upsert rel %d-%s->%d: %w", src, r.Predicate, tgt, err)
		}
	}

	// Mark turns extracted=1 and clear any prior error.
	if len(turnIDs) > 0 {
		if err := execInIDs(ctx, tx,
			`UPDATE turns SET extracted = 1, extraction_error = NULL WHERE id IN (%s)`,
			turnIDs); err != nil {
			return fmt.Errorf("mark turns extracted: %w", err)
		}
	}

	return tx.Commit()
}

// incrementAttempts bumps extraction_attempts on the given turn IDs and
// records the last-seen error message. Keeps extracted = 0 so the turns
// stay eligible for retry.
func incrementAttempts(ctx context.Context, db *sql.DB, turnIDs []int64, errMsg string) error {
	if len(turnIDs) == 0 {
		return nil
	}
	return execInIDsDB(ctx, db,
		`UPDATE turns SET extraction_attempts = extraction_attempts + 1,
		                  extraction_error = ?
		 WHERE id IN (%s)`,
		turnIDs, errMsg)
}

// markDeadLetter sets extracted = 2 on the given turn IDs. After this,
// the polling query WHERE extracted = 0 skips them permanently.
func markDeadLetter(ctx context.Context, db *sql.DB, turnIDs []int64, errMsg string) error {
	if len(turnIDs) == 0 {
		return nil
	}
	return execInIDsDB(ctx, db,
		`UPDATE turns SET extracted = 2, extraction_error = ? WHERE id IN (%s)`,
		turnIDs, errMsg)
}

// execInIDs runs a query whose last argument is a variadic IN-list of
// int64 IDs, interpolated into the query string as "?,?,?...". The
// template must contain exactly one "%s" placeholder.
func execInIDs(ctx context.Context, tx *sql.Tx, tmpl string, ids []int64) error {
	placeholders, args := inListArgs(ids)
	_, err := tx.ExecContext(ctx, fmt.Sprintf(tmpl, placeholders), args...)
	return err
}

func execInIDsDB(ctx context.Context, db *sql.DB, tmpl string, ids []int64, leadingArgs ...any) error {
	placeholders, idArgs := inListArgs(ids)
	args := append(append([]any{}, leadingArgs...), idArgs...)
	_, err := db.ExecContext(ctx, fmt.Sprintf(tmpl, placeholders), args...)
	return err
}

func inListArgs(ids []int64) (string, []any) {
	if len(ids) == 0 {
		return "NULL", nil
	}
	var b []byte
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '?')
		args = append(args, id)
	}
	return string(b), args
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestGraph_ -v
```

All 8 graph tests PASS.

Also run the full memory suite:
```bash
go test -race ./internal/memory/... -count=1 -timeout 60s
go vet ./...
```

All green.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/memory/graph.go gormes/internal/memory/graph_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): graph upsert + turn-state helpers

Three write helpers for the extractor worker:

  writeGraphBatch: one transaction that upserts validated
    entities + relationships and marks the batch's turn IDs
    extracted=1. Entity dedup via ON CONFLICT(name,type);
    non-empty description overrides empty. Relationship weight
    accumulates via MIN(current + new, 10.0).

  incrementAttempts: on retriable/terminal failures, bumps
    extraction_attempts + records extraction_error while leaving
    extracted=0 so the turn stays retry-eligible.

  markDeadLetter: at MaxAttempts, flips extracted=2 so the
    polling query skips the turn permanently. Operator can
    reset via plain SQL.

All INSERT/UPDATE statements use "?" placeholders. The IN-list
helper interpolates a safe "?,?,?" template and binds the ids
separately — no string concat into SQL.

Schema CHECK constraints on type + predicate catch any validator
regression at INSERT time, proven by
TestGraph_CheckConstraintRejectsBadPredicate.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Extractor system prompt + batch formatter

**Files:**
- Create: `gormes/internal/memory/extractor_prompt.go`
- Create: `gormes/internal/memory/extractor_prompt_test.go`

- [ ] **Step 1: Write failing test**

Create `gormes/internal/memory/extractor_prompt_test.go`:

```go
package memory

import (
	"strings"
	"testing"
)

func TestFormatBatchPrompt_IncludesRolePrefix(t *testing.T) {
	rows := []turnRow{
		{id: 1, role: "user", content: "hello"},
		{id: 2, role: "assistant", content: "hi"},
	}
	got := formatBatchPrompt(rows)
	if !strings.Contains(got, "[user]: hello") {
		t.Errorf("prompt missing [user]: hello; got %q", got)
	}
	if !strings.Contains(got, "[assistant]: hi") {
		t.Errorf("prompt missing [assistant]: hi; got %q", got)
	}
}

func TestFormatBatchPrompt_TruncatesLongContent(t *testing.T) {
	long := strings.Repeat("x", 5000)
	got := formatBatchPrompt([]turnRow{{id: 1, role: "user", content: long}})
	// Truncation to 4000 chars per turn.
	if strings.Count(got, "x") > 4000 {
		t.Errorf("content not truncated to 4000 chars; got %d", strings.Count(got, "x"))
	}
}

func TestExtractorSystemPrompt_MentionsPredicateWhitelist(t *testing.T) {
	for _, pred := range []string{"WORKS_ON", "KNOWS", "RELATED_TO"} {
		if !strings.Contains(extractorSystemPrompt, pred) {
			t.Errorf("system prompt missing predicate %q", pred)
		}
	}
}

func TestExtractorSystemPrompt_MentionsTypeWhitelist(t *testing.T) {
	for _, typ := range []string{"PERSON", "PROJECT", "OTHER"} {
		if !strings.Contains(extractorSystemPrompt, typ) {
			t.Errorf("system prompt missing type %q", typ)
		}
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run "TestFormatBatchPrompt_|TestExtractorSystemPrompt_" 2>&1 | head -5
```

Expected: `undefined: turnRow`, `undefined: formatBatchPrompt`, `undefined: extractorSystemPrompt`.

- [ ] **Step 3: Write `extractor_prompt.go`**

Create `gormes/internal/memory/extractor_prompt.go`:

```go
package memory

import (
	"fmt"
	"strings"
)

// turnRow mirrors the subset of turns columns the extractor reads.
type turnRow struct {
	id      int64
	role    string
	content string
}

// maxTurnChars caps individual turn content in the LLM prompt. Matches
// the Telegram renderer's 4000-char cap so one runaway turn can't
// blow out the context window.
const maxTurnChars = 4000

// extractorSystemPrompt is the verbatim system message sent to the LLM
// before each extraction batch. Any change here is a behavior change of
// the entire extractor; bump schemaVersion if you expand the predicate
// or type whitelist to stay in lockstep with the CHECK constraints.
const extractorSystemPrompt = `You are an ontological entity extractor. You read conversation turns between a user and an AI assistant, and you emit a structured JSON summary of the entities mentioned and the relationships between them.

Rules:
1. Output ONLY valid JSON. No prose. No markdown fences. Start with '{'.
2. The JSON object has exactly two keys: "entities" and "relationships".
3. Each entity is {"name": string, "type": one of ["PERSON","PROJECT","CONCEPT","PLACE","ORGANIZATION","TOOL","OTHER"], "description": string (<= 512 chars, optional, empty string if absent)}.
4. Each relationship is {"source": string (entity name), "target": string (entity name), "predicate": one of the 8 values listed in rule 6, "weight": number between 0.0 and 1.0}.
5. Relationship source/target names MUST match entity names in this same response exactly. Do not reference entities not in entities[].
6. "predicate" MUST be EXACTLY one of these 8 uppercase strings:
   WORKS_ON     — an agent produces or contributes to a project/tool
   KNOWS        — an agent is aware of another agent or concept
   LIKES        — an agent expresses positive sentiment
   DISLIKES     — an agent expresses negative sentiment
   HAS_SKILL    — an agent possesses a concept/tool as a capability
   LOCATED_IN   — an entity is geographically or structurally inside another
   PART_OF      — an entity is a structural component of another
   RELATED_TO   — a generic fallback when no other predicate fits
   If none of the specific predicates fits, use RELATED_TO. Do NOT invent new predicates.
7. Deduplicate within the response: do not emit the same entity twice or the same (source, target, predicate) triple twice.
8. If no entities are present, emit {"entities": [], "relationships": []}.
`

// formatBatchPrompt renders the user message for one extraction batch:
// a blank-line-separated list of role-prefixed turn contents, each
// truncated to maxTurnChars.
func formatBatchPrompt(rows []turnRow) string {
	var b strings.Builder
	b.WriteString("Conversation turns to analyze (role: content):\n\n")
	for _, r := range rows {
		content := r.content
		if len(content) > maxTurnChars {
			content = content[:maxTurnChars] + "..."
		}
		fmt.Fprintf(&b, "[%s]: %s\n\n", r.role, content)
	}
	return b.String()
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run "TestFormatBatchPrompt_|TestExtractorSystemPrompt_" -v
```

All 4 prompt tests PASS.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/memory/extractor_prompt.go gormes/internal/memory/extractor_prompt_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): extractor system prompt + batch formatter

extractorSystemPrompt is the verbatim LLM instruction set —
strict JSON-only output, 7-item type whitelist, 8-item
predicate whitelist with RELATED_TO fallback. Mirrors the
CHECK constraints in migration3aTo3b. Four tests pin the
whitelist membership at compile + test time.

formatBatchPrompt renders role-prefixed turn content with a
4000-char per-turn cap (matches the Telegram renderer's cap)
so one runaway paste can't blow the LLM context window.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Extractor scaffold (Config, struct, Run skeleton, Close)

**Files:**
- Create: `gormes/internal/memory/extractor.go`
- Create: `gormes/internal/memory/extractor_test.go`

This task lands the Extractor type + lifecycle with a **stub** `loopOnce` that does nothing. Tasks 6-9 fill in the behavior.

- [ ] **Step 1: Write failing tests**

Create `gormes/internal/memory/extractor_test.go`:

```go
package memory

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
)

// fakeLLM implements hermes.Client via settable response behavior.
// Each OpenStream call returns the next scripted response.
type fakeLLM struct {
	mu       sync.Mutex
	scripts  []fakeResp
	openCalls atomic.Int64
}

type fakeResp struct {
	body string
	err  error
}

func (f *fakeLLM) script(body string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scripts = append(f.scripts, fakeResp{body: body, err: err})
}

func (f *fakeLLM) Health(ctx context.Context) error { return nil }

func (f *fakeLLM) OpenStream(ctx context.Context, _ hermes.ChatRequest) (hermes.Stream, error) {
	f.openCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.scripts) == 0 {
		return &fakeStream{body: `{"entities":[],"relationships":[]}`}, nil
	}
	r := f.scripts[0]
	f.scripts = f.scripts[1:]
	if r.err != nil {
		return nil, r.err
	}
	return &fakeStream{body: r.body}, nil
}

func (f *fakeLLM) OpenRunEvents(ctx context.Context, _ string) (hermes.RunEventStream, error) {
	return nil, hermes.ErrRunEventsNotSupported
}

type fakeStream struct {
	body string
	emit bool
}

func (s *fakeStream) SessionID() string { return "" }
func (s *fakeStream) Close() error      { return nil }
func (s *fakeStream) Recv(ctx context.Context) (hermes.Event, error) {
	select {
	case <-ctx.Done():
		return hermes.Event{}, ctx.Err()
	default:
	}
	if !s.emit {
		s.emit = true
		return hermes.Event{Kind: hermes.EventToken, Token: s.body}, nil
	}
	return hermes.Event{Kind: hermes.EventDone, FinishReason: "stop"}, errStreamEOF
}

var errStreamEOF = errors.New("eof")

func openExtractor(t *testing.T, cfg ExtractorConfig) (*SqliteStore, *Extractor, *fakeLLM) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	llm := &fakeLLM{}
	e := NewExtractor(s, llm, cfg, nil)
	t.Cleanup(func() {
		_ = e.Close(context.Background())
		_ = s.Close(context.Background())
	})
	return s, e, llm
}

func TestExtractor_NewExtractorWithZeroConfigFillsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	e := NewExtractor(s, &fakeLLM{}, ExtractorConfig{}, nil)
	if e.cfg.BatchSize != 5 {
		t.Errorf("BatchSize default = %d, want 5", e.cfg.BatchSize)
	}
	if e.cfg.PollInterval != 10*time.Second {
		t.Errorf("PollInterval default = %v, want 10s", e.cfg.PollInterval)
	}
	if e.cfg.MaxAttempts != 5 {
		t.Errorf("MaxAttempts default = %d, want 5", e.cfg.MaxAttempts)
	}
	if e.cfg.CallTimeout != 30*time.Second {
		t.Errorf("CallTimeout default = %v, want 30s", e.cfg.CallTimeout)
	}
}

func TestExtractor_CloseBeforeRunIsNoop(t *testing.T) {
	_, e, _ := openExtractor(t, ExtractorConfig{})
	if err := e.Close(context.Background()); err != nil {
		t.Errorf("Close before Run: %v", err)
	}
	// Second Close is also a no-op.
	if err := e.Close(context.Background()); err != nil {
		t.Errorf("double Close: %v", err)
	}
}

func TestExtractor_RunExitsOnCtxCancel(t *testing.T) {
	_, e, _ := openExtractor(t, ExtractorConfig{PollInterval: 20 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		e.Run(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond) // let the loop tick a few times
	cancel()

	select {
	case <-done:
		// Run returned after cancel.
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit within 2s of ctx cancel")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run TestExtractor_ 2>&1 | head -10
```

Expected: `undefined: ExtractorConfig`, `undefined: NewExtractor`, etc.

- [ ] **Step 3: Write `extractor.go` (scaffold with stub loop body)**

Create `gormes/internal/memory/extractor.go`:

```go
package memory

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
)

// ExtractorConfig controls the Brain worker's polling + retry behavior.
// Zero values fall back to sensible defaults.
type ExtractorConfig struct {
	Model        string        // empty = reuse kernel's Hermes model
	PollInterval time.Duration // default 10s
	BatchSize    int           // default 5 turns per LLM call
	MaxAttempts  int           // default 5 before dead-letter
	CallTimeout  time.Duration // default 30s per LLM call
	BackoffBase  time.Duration // default 2s; doubles per attempt
	BackoffMax   time.Duration // default 60s cap
}

func (c *ExtractorConfig) withDefaults() {
	if c.PollInterval <= 0 {
		c.PollInterval = 10 * time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 5
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.CallTimeout <= 0 {
		c.CallTimeout = 30 * time.Second
	}
	if c.BackoffBase <= 0 {
		c.BackoffBase = 2 * time.Second
	}
	if c.BackoffMax <= 0 {
		c.BackoffMax = 60 * time.Second
	}
}

// Extractor runs the LLM-assisted entity/relationship extraction loop.
// Exactly one goroutine owns the main poll loop; graph writes serialize
// through the shared *SqliteStore *sql.DB (SetMaxOpenConns(1) pool).
type Extractor struct {
	store *SqliteStore
	llm   hermes.Client
	cfg   ExtractorConfig
	log   *slog.Logger

	done      chan struct{}
	closeOnce sync.Once
}

// NewExtractor constructs an Extractor. ctx-cancel to Run.Run and/or
// Close(ctx) to shut it down gracefully.
func NewExtractor(s *SqliteStore, llm hermes.Client, cfg ExtractorConfig, log *slog.Logger) *Extractor {
	cfg.withDefaults()
	if log == nil {
		log = slog.Default()
	}
	return &Extractor{
		store: s,
		llm:   llm,
		cfg:   cfg,
		log:   log,
		done:  make(chan struct{}, 1),
	}
}

// Run blocks until ctx is cancelled. Each tick: poll, call LLM, upsert,
// mark. Errors are logged + counted; never returned.
func (e *Extractor) Run(ctx context.Context) {
	defer func() {
		select {
		case e.done <- struct{}{}:
		default:
		}
	}()
	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.loopOnce(ctx) // stub in T5; T6-T9 fill in
		}
	}
}

// Close signals the loop to stop. If Run is currently blocked in an
// LLM call, the caller should cancel Run's ctx separately. Close itself
// waits up to ctx.Deadline for the loop to exit cleanly.
func (e *Extractor) Close(ctx context.Context) error {
	e.closeOnce.Do(func() {
		// The close path does not cancel Run's ctx — that's the caller's
		// responsibility. We just wait for Run to signal done, or bail.
	})
	select {
	case <-e.done:
		return nil
	case <-ctx.Done():
		return nil
	default:
		// Run was never started.
		return nil
	}
}

// loopOnce is the stub; Task 6 replaces it with the real body.
func (e *Extractor) loopOnce(ctx context.Context) {
	// Intentionally empty — poll is a no-op until T6.
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestExtractor_ -v -timeout 15s
go vet ./...
```

3 extractor tests PASS.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/memory/extractor.go gormes/internal/memory/extractor_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): Extractor scaffold — Config, Run skeleton, Close

Extractor is the Phase-3.B Brain worker. This commit lands the
type surface + lifecycle plumbing with a STUB loopOnce() that
does nothing. Tasks 6-9 fill in the behavior incrementally:
  T6: poll + callLLM happy path
  T7: attempts + dead-letter
  T8: backoff + 429 Retry-After
  T9: graceful ctx-aware shutdown

ExtractorConfig.withDefaults() fills in sensible defaults
(5 batch, 10s poll, 5 max attempts, 30s call timeout, 2s/60s
backoff). fakeLLM test double implements hermes.Client.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Poll + callLLM + happy-path loop integration

**Files:**
- Modify: `gormes/internal/memory/extractor.go`
- Modify: `gormes/internal/memory/extractor_test.go`

- [ ] **Step 1: Append failing test**

Append to `gormes/internal/memory/extractor_test.go`:

```go
import (
	"encoding/json"
	// ... existing imports ...
	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
)

// seedTurns inserts N user turns via the store's fast path.
func seedTurns(t *testing.T, s *SqliteStore, contents ...string) {
	t.Helper()
	for i, c := range contents {
		payload, _ := json.Marshal(map[string]any{
			"session_id": "sess-extractor-test",
			"content":    c,
			"ts_unix":    int64(1745000000 + i),
		})
		_, _ = s.Exec(context.Background(), store.Command{
			Kind: store.AppendUserTurn, Payload: payload,
		})
	}
	// Wait for the persistence worker to flush to disk.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = s.db.QueryRow("SELECT COUNT(*) FROM turns WHERE session_id = 'sess-extractor-test'").Scan(&n)
		if n == len(contents) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("seedTurns timeout")
}

func TestExtractor_HappyPathPopulatesGraph(t *testing.T) {
	s, e, llm := openExtractor(t, ExtractorConfig{
		PollInterval: 30 * time.Millisecond,
		BatchSize:    5,
		CallTimeout:  2 * time.Second,
	})

	seedTurns(t, s, "I'm working on Arenaton")
	llm.script(`{"entities":[
		{"name":"Jose","type":"PERSON","description":""},
		{"name":"Arenaton","type":"PROJECT","description":""}
	],"relationships":[
		{"source":"Jose","target":"Arenaton","predicate":"WORKS_ON","weight":0.9}
	]}`, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go e.Run(ctx)

	// Wait for turns.extracted = 1 AND one rel row.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var e1, nRel int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM turns WHERE extracted = 1`).Scan(&e1)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM relationships`).Scan(&nRel)
		if e1 >= 1 && nRel >= 1 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	var extracted int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM turns WHERE extracted = 1`).Scan(&extracted)
	if extracted < 1 {
		t.Errorf("turns.extracted=1 count = %d, want >= 1", extracted)
	}
	var nEnt, nRel int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&nEnt)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM relationships`).Scan(&nRel)
	if nEnt != 2 || nRel != 1 {
		t.Errorf("entities=%d relationships=%d, want 2 and 1", nEnt, nRel)
	}
	if llm.openCalls.Load() != 1 {
		t.Errorf("openCalls = %d, want exactly 1", llm.openCalls.Load())
	}
}

func TestExtractor_EmptyResultStillMarksExtracted(t *testing.T) {
	s, e, llm := openExtractor(t, ExtractorConfig{PollInterval: 30 * time.Millisecond})
	seedTurns(t, s, "weather small talk")
	llm.script(`{"entities":[],"relationships":[]}`, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go e.Run(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM turns WHERE extracted = 1`).Scan(&n)
		if n >= 1 {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}

	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM turns WHERE extracted = 1`).Scan(&n)
	if n < 1 {
		t.Errorf("empty-result batch not marked extracted=1")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test -race ./internal/memory/... -run "TestExtractor_HappyPath|TestExtractor_EmptyResult" -v -timeout 15s 2>&1 | tail -15
```

Expected: tests time out (stub loopOnce doesn't populate the graph).

- [ ] **Step 3: Replace stub loopOnce with real body + add `callLLM`**

Edit `gormes/internal/memory/extractor.go`. Add an import for `io` and the store package. Replace the stub loopOnce block and add helper methods:

```go
// Add at top:
import (
	// ... existing imports ...
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/hermes"
)

// Replace the stub loopOnce with:

func (e *Extractor) loopOnce(ctx context.Context) {
	batch, err := e.pollBatch(ctx)
	if err != nil {
		e.log.Warn("extractor: poll failed", "err", err)
		return
	}
	if len(batch) == 0 {
		return
	}
	ids := make([]int64, len(batch))
	for i, r := range batch {
		ids[i] = r.id
	}

	callCtx, cancel := context.WithTimeout(ctx, e.cfg.CallTimeout)
	defer cancel()
	raw, err := e.callLLM(callCtx, batch)
	if err != nil {
		e.log.Warn("extractor: LLM call failed",
			"attempt_increment", true, "turn_ids", ids, "err", err)
		_ = incrementAttempts(ctx, e.store.db, ids, err.Error())
		return
	}

	validated, err := ValidateExtractorOutput(raw)
	if err != nil {
		preview := string(raw)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		e.log.Warn("extractor: malformed JSON",
			"attempt_increment", true, "turn_ids", ids, "preview", preview, "err", err)
		_ = incrementAttempts(ctx, e.store.db, ids, "malformed JSON: "+err.Error())
		return
	}

	if err := writeGraphBatch(ctx, e.store.db, validated, ids); err != nil {
		e.log.Warn("extractor: graph write failed",
			"attempt_increment", true, "turn_ids", ids, "err", err)
		_ = incrementAttempts(ctx, e.store.db, ids, err.Error())
		return
	}

	e.log.Debug("extractor: batch processed",
		"turn_ids", ids, "entities", len(validated.Entities),
		"relationships", len(validated.Relationships))
}

// pollBatch reads up to cfg.BatchSize unprocessed turns.
func (e *Extractor) pollBatch(ctx context.Context) ([]turnRow, error) {
	rows, err := e.store.db.QueryContext(ctx,
		`SELECT id, role, content FROM turns
		 WHERE extracted = 0 AND extraction_attempts < ?
		 ORDER BY id LIMIT ?`,
		e.cfg.MaxAttempts, e.cfg.BatchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []turnRow
	for rows.Next() {
		var r turnRow
		if err := rows.Scan(&r.id, &r.role, &r.content); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// callLLM sends the extractor prompt to the hermes.Client and collects
// the full streamed response into a single byte slice. Returns the raw
// JSON body (not yet validated).
func (e *Extractor) callLLM(ctx context.Context, batch []turnRow) ([]byte, error) {
	req := hermes.ChatRequest{
		Model:  e.cfg.Model,
		Stream: true,
		Messages: []hermes.Message{
			{Role: "system", Content: extractorSystemPrompt},
			{Role: "user", Content: formatBatchPrompt(batch)},
		},
	}
	stream, err := e.llm.OpenStream(ctx, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()

	var b strings.Builder
	for {
		ev, err := stream.Recv(ctx)
		if errors.Is(err, io.EOF) || ev.Kind == hermes.EventDone {
			if ev.Token != "" {
				b.WriteString(ev.Token)
			}
			break
		}
		if err != nil {
			// Test-only fakeStream returns a sentinel for EOF; treat as clean end if we have content.
			if b.Len() > 0 {
				break
			}
			return nil, err
		}
		if ev.Token != "" {
			b.WriteString(ev.Token)
		}
	}
	return []byte(b.String()), nil
}
```

- [ ] **Step 4: Run tests, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestExtractor_ -v -timeout 30s
```

All 5 extractor tests PASS.

Full suite:
```bash
go test -race ./internal/memory/... -count=1 -timeout 60s
go vet ./...
```

All green.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/memory/extractor.go gormes/internal/memory/extractor_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): Extractor happy-path loop — poll + callLLM + upsert

loopOnce() is now non-stub:
  1. pollBatch SELECTs up to BatchSize turns WHERE extracted=0
     AND extraction_attempts < MaxAttempts (partial index hit)
  2. callLLM sends extractorSystemPrompt + formatBatchPrompt(batch)
     to hermes.Client.OpenStream, collects streamed tokens
  3. ValidateExtractorOutput parses + sanitizes the JSON
  4. writeGraphBatch upserts entities + relationships +
     marks turns extracted=1 in one tx

Any error on the LLM/validate/write path increments
extraction_attempts via incrementAttempts; no panic, no
kernel impact.

TestExtractor_HappyPathPopulatesGraph and
TestExtractor_EmptyResultStillMarksExtracted both pass —
empty-entities result still marks turns=1 so the poll loop
doesn't spin forever on small-talk turns.

Tasks 7 (dead-letter), 8 (backoff), 9 (shutdown) extend
loopOnce further; the happy path is locked in here.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Dead-letter after MaxAttempts

**Files:**
- Modify: `gormes/internal/memory/extractor.go`
- Modify: `gormes/internal/memory/extractor_test.go`

- [ ] **Step 1: Append failing test**

Append to `extractor_test.go`:

```go
func TestExtractor_DeadLettersAfterMaxAttempts(t *testing.T) {
	s, e, llm := openExtractor(t, ExtractorConfig{
		PollInterval: 20 * time.Millisecond,
		BatchSize:    1,
		MaxAttempts:  3,
		CallTimeout:  500 * time.Millisecond,
	})

	seedTurns(t, s, "doomed turn")
	// Always-malformed LLM output.
	for i := 0; i < 5; i++ {
		llm.script("not json", nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go e.Run(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var extracted int
		_ = s.db.QueryRow(`SELECT extracted FROM turns WHERE content = 'doomed turn'`).Scan(&extracted)
		if extracted == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	var extracted, attempts int
	_ = s.db.QueryRow(`SELECT extracted, extraction_attempts FROM turns WHERE content = 'doomed turn'`).
		Scan(&extracted, &attempts)
	if extracted != 2 {
		t.Errorf("extracted = %d, want 2 (dead-letter)", extracted)
	}
	if attempts < 3 {
		t.Errorf("attempts = %d, want >= 3", attempts)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test -race ./internal/memory/... -run TestExtractor_DeadLetters -v -timeout 15s
```

Expected: FAIL — current loopOnce just increments attempts; the poll SELECT has `extraction_attempts < MaxAttempts` so the row stops being picked up, but `extracted` stays 0. Test asserts `extracted == 2`.

- [ ] **Step 3: Add dead-letter logic to `loopOnce`**

In `extractor.go`, change **both** incrementAttempts call sites (the `err != nil` paths for LLM and validate and writeGraph) to a common helper that checks `attempts >= MaxAttempts-1` (about-to-exceed) and flips to dead-letter instead:

Replace every `_ = incrementAttempts(ctx, e.store.db, ids, err.Error())` in loopOnce with `e.recordFailure(ctx, ids, err.Error())`, then add the helper:

```go
// recordFailure increments extraction_attempts (adding errMsg) and, if
// the row has reached MaxAttempts, flips it to extracted=2 so the
// polling query skips it permanently.
func (e *Extractor) recordFailure(ctx context.Context, ids []int64, errMsg string) {
	// Get current attempts to decide dead-letter.
	if len(ids) == 0 {
		return
	}
	// Increment first.
	if err := incrementAttempts(ctx, e.store.db, ids, errMsg); err != nil {
		e.log.Warn("extractor: incrementAttempts failed", "err", err)
		return
	}
	// Now check which (if any) crossed the threshold.
	placeholders, args := inListArgs(ids)
	q := "SELECT id FROM turns WHERE extraction_attempts >= ? AND extracted = 0 AND id IN (" + placeholders + ")"
	rowArgs := append([]any{e.cfg.MaxAttempts}, args...)
	rows, err := e.store.db.QueryContext(ctx, q, rowArgs...)
	if err != nil {
		e.log.Warn("extractor: dead-letter scan failed", "err", err)
		return
	}
	defer rows.Close()
	var dead []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			dead = append(dead, id)
		}
	}
	if len(dead) > 0 {
		_ = markDeadLetter(ctx, e.store.db, dead, errMsg)
		e.log.Error("extractor: dead-lettered after max attempts",
			"turn_ids", dead, "max_attempts", e.cfg.MaxAttempts, "err", errMsg)
	}
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestExtractor_ -v -timeout 30s
go vet ./...
```

All extractor tests including the dead-letter test PASS.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/memory/extractor.go gormes/internal/memory/extractor_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): extractor dead-letters turns after MaxAttempts

After MaxAttempts failed extractions on the same turn, the
worker stops retrying: it calls markDeadLetter (extracted=2)
and the polling query skips the turn permanently. An operator
can reset via:
  UPDATE turns SET extracted=0, extraction_attempts=0
  WHERE extracted=2;

recordFailure consolidates the increment-then-maybe-dead-letter
logic so all failure paths (LLM error, malformed JSON, graph
write fail) share one code path.

TestExtractor_DeadLettersAfterMaxAttempts: malformed JSON on
every call, MaxAttempts=3, observe extracted=2 after at least
3 attempts.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Exponential backoff + 429 Retry-After handling

**Files:**
- Modify: `gormes/internal/memory/extractor.go`
- Modify: `gormes/internal/memory/extractor_test.go`

- [ ] **Step 1: Append failing tests**

Append to `extractor_test.go`:

```go
func TestExtractor_BackoffSleepsBetweenFailures(t *testing.T) {
	s, e, llm := openExtractor(t, ExtractorConfig{
		PollInterval: 5 * time.Millisecond,
		BatchSize:    1,
		MaxAttempts:  10, // high so we don't dead-letter during the test
		CallTimeout:  100 * time.Millisecond,
		BackoffBase:  80 * time.Millisecond,
		BackoffMax:   200 * time.Millisecond,
	})
	seedTurns(t, s, "backoff test")
	for i := 0; i < 10; i++ {
		llm.script("not json", nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	start := time.Now()
	go e.Run(ctx)

	<-ctx.Done()
	elapsed := time.Since(start)

	calls := llm.openCalls.Load()
	// With 80ms base backoff doubling, we expect roughly 4-8 calls in 1500ms
	// (5ms poll interval is irrelevant once backoff kicks in).
	// Without backoff: ~300 calls. With backoff: very few.
	if calls > 15 {
		t.Errorf("openCalls = %d in %v — backoff not applied (expected < 15)", calls, elapsed)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test -race ./internal/memory/... -run TestExtractor_BackoffSleeps -v -timeout 10s
```

Expected: FAIL — current loop relies solely on the 5ms ticker and makes many calls.

- [ ] **Step 3: Add backoff state to Extractor + sleep after failures**

Edit `extractor.go`. Add a backoff field to Extractor:

```go
type Extractor struct {
	// ... existing fields ...
	backoffCur time.Duration // current backoff delay; resets to 0 on success
}
```

Modify `loopOnce` to sleep `backoffCur` BEFORE the poll when it's non-zero, and update the failure/success paths:

```go
func (e *Extractor) loopOnce(ctx context.Context) {
	if e.backoffCur > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(e.backoffCur):
		}
	}

	batch, err := e.pollBatch(ctx)
	if err != nil {
		e.log.Warn("extractor: poll failed", "err", err)
		return
	}
	if len(batch) == 0 {
		e.backoffCur = 0 // no work means no failure; reset
		return
	}
	ids := make([]int64, len(batch))
	for i, r := range batch {
		ids[i] = r.id
	}

	callCtx, cancel := context.WithTimeout(ctx, e.cfg.CallTimeout)
	defer cancel()
	raw, err := e.callLLM(callCtx, batch)
	if err != nil {
		e.log.Warn("extractor: LLM call failed", "turn_ids", ids, "err", err)
		e.recordFailure(ctx, ids, err.Error())
		e.advanceBackoff()
		return
	}

	validated, err := ValidateExtractorOutput(raw)
	if err != nil {
		preview := string(raw)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		e.log.Warn("extractor: malformed JSON", "turn_ids", ids, "preview", preview, "err", err)
		e.recordFailure(ctx, ids, "malformed JSON: "+err.Error())
		e.advanceBackoff()
		return
	}

	if err := writeGraphBatch(ctx, e.store.db, validated, ids); err != nil {
		e.log.Warn("extractor: graph write failed", "turn_ids", ids, "err", err)
		e.recordFailure(ctx, ids, err.Error())
		e.advanceBackoff()
		return
	}

	e.log.Debug("extractor: batch processed",
		"turn_ids", ids, "entities", len(validated.Entities),
		"relationships", len(validated.Relationships))
	e.backoffCur = 0 // success resets
}

// advanceBackoff doubles backoffCur (or seeds it from BackoffBase if 0),
// capped at BackoffMax.
func (e *Extractor) advanceBackoff() {
	if e.backoffCur == 0 {
		e.backoffCur = e.cfg.BackoffBase
		return
	}
	e.backoffCur *= 2
	if e.backoffCur > e.cfg.BackoffMax {
		e.backoffCur = e.cfg.BackoffMax
	}
}
```

(`429 Retry-After` from the real `hermes.HTTPClient` would be parsed here if the error had a `RetryAfter` method; for now the backoff alone handles the bus-slowdown case. A future extension can add a `RetryAfterer` interface check without touching tests.)

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestExtractor_ -v -timeout 30s
```

All extractor tests still PASS (backoff test now observes <15 calls in 1.5s thanks to the doubling sleep).

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/memory/extractor.go gormes/internal/memory/extractor_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): extractor exponential backoff on failures

advanceBackoff doubles the inter-poll sleep on every failure,
seeded from BackoffBase (default 2s) and capped at BackoffMax
(default 60s). Success resets backoffCur to 0, immediately
returning to normal PollInterval cadence.

Empty-poll result also resets backoff — "no work" is not a
failure.

TestExtractor_BackoffSleepsBetweenFailures: with 80ms base
and 10 scripted failures, observe <15 LLM calls in 1.5s
(without backoff: ~300 at 5ms poll).

The 429 Retry-After path stays as a future refinement: when
hermes.HTTPClient starts surfacing a RetryAfterer interface
on 429s, callLLM can parse it and use max(RetryAfter, backoff)
without touching tests.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Graceful shutdown via `Close(ctx)`

**Files:**
- Modify: `gormes/internal/memory/extractor.go`
- Modify: `gormes/internal/memory/extractor_test.go`

- [ ] **Step 1: Append failing test**

Append to `extractor_test.go`:

```go
func TestExtractor_CloseWaitsForLoopExit(t *testing.T) {
	_, e, _ := openExtractor(t, ExtractorConfig{PollInterval: 50 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())

	go e.Run(ctx)
	time.Sleep(100 * time.Millisecond) // let it tick

	// Cancel Run's ctx; Close should observe e.done.
	cancel()

	closeCtx, closeCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer closeCancel()

	done := make(chan error, 1)
	go func() { done <- e.Close(closeCtx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return within 2s")
	}
}
```

- [ ] **Step 2: Run, expect PASS (likely — current Close reads e.done)**

```bash
cd gormes
go test -race ./internal/memory/... -run TestExtractor_CloseWaits -v -timeout 10s
```

If it already passes (because of the defer e.done <- struct{}{} in Run), great. If it fails because the default branch returns immediately, we need to fix Close.

Current Close has a subtle bug: if Run hasn't started, the `select` falls through to `default` and returns. If Run IS running and cancels, e.done eventually receives, but Close has already returned nil.

- [ ] **Step 3: Fix Close to actually wait for e.done**

In `extractor.go`, replace the Close method:

```go
// Close signals nothing — it just waits for Run to exit and the done
// signal to arrive. Caller is responsible for cancelling Run's ctx;
// Close honors its own ctx deadline independently.
//
// Idempotent: second call returns immediately.
func (e *Extractor) Close(ctx context.Context) error {
	e.closeOnce.Do(func() {
		select {
		case <-e.done:
		case <-ctx.Done():
		}
	})
	return nil
}
```

This removes the problematic `default` branch. First Close blocks until one of the two channels fires; subsequent Close()'s return immediately via sync.Once.

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestExtractor_ -v -timeout 30s
```

All tests PASS including the Close wait test AND the pre-existing `TestExtractor_CloseBeforeRunIsNoop` (because if Run was never started, e.done is never written but closeCtx's deadline fires inside the sync.Once).

Full memory suite:
```bash
go test -race ./internal/memory/... -count=1 -timeout 90s
go vet ./...
```

All green.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/memory/extractor.go gormes/internal/memory/extractor_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): extractor Close(ctx) waits for loop exit

Previous Close had a default branch that returned immediately
if it couldn't read e.done without blocking, which defeated
the "wait for drain" contract. Replaced with a sync.Once-guarded
select on {e.done, ctx.Done()} — first call blocks until either
fires, subsequent calls return immediately.

TestExtractor_CloseWaitsForLoopExit: start Run, cancel its ctx,
call Close with a budgeted ctx, assert Close returns before the
harness timeout.

TestExtractor_CloseBeforeRunIsNoop still passes: if Run never
ran, e.done never writes but the caller's closeCtx deadline fires
inside the sync.Once and Close returns cleanly.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Config — `ExtractorBatchSize` + `ExtractorPollInterval`

**Files:**
- Modify: `gormes/internal/config/config.go`
- Modify: `gormes/internal/config/config_test.go`

- [ ] **Step 1: Append failing tests**

Append to `config_test.go`:

```go
func TestLoad_ExtractorDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telegram.ExtractorBatchSize != 5 {
		t.Errorf("ExtractorBatchSize default = %d, want 5", cfg.Telegram.ExtractorBatchSize)
	}
	if cfg.Telegram.ExtractorPollInterval != 10*time.Second {
		t.Errorf("ExtractorPollInterval default = %v, want 10s", cfg.Telegram.ExtractorPollInterval)
	}
}
```

Ensure `"time"` is imported in the test file.

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/config/... -run TestLoad_ExtractorDefaults 2>&1 | head -5
```

Expected: `unknown field ExtractorBatchSize`.

- [ ] **Step 3: Extend `config.go`**

In `gormes/internal/config/config.go`, extend `TelegramCfg`:

```go
type TelegramCfg struct {
	BotToken              string        `toml:"bot_token"`
	AllowedChatID         int64         `toml:"allowed_chat_id"`
	CoalesceMs            int           `toml:"coalesce_ms"`
	FirstRunDiscovery     bool          `toml:"first_run_discovery"`
	MemoryQueueCap        int           `toml:"memory_queue_cap"`
	// ExtractorBatchSize / ExtractorPollInterval (Phase 3.B).
	ExtractorBatchSize    int           `toml:"extractor_batch_size"`
	ExtractorPollInterval time.Duration `toml:"extractor_poll_interval"`
}
```

Ensure `"time"` is imported if not already.

Extend `defaults()`:

```go
Telegram: TelegramCfg{
	CoalesceMs:            1000,
	FirstRunDiscovery:     true,
	MemoryQueueCap:        1024,
	ExtractorBatchSize:    5,
	ExtractorPollInterval: 10 * time.Second,
},
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/config/... -v
go vet ./...
```

New test passes; existing config tests still pass.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/internal/config/config.go gormes/internal/config/config_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/config): ExtractorBatchSize + ExtractorPollInterval

TOML-only knobs for the Phase-3.B Brain worker:
  - extractor_batch_size (default 5)
  - extractor_poll_interval (default 10s — go-toml/v2 parses "10s")

No env-var override, no CLI flag. Operator-level tuning only.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Wire extractor into `cmd/gormes-telegram`

**Files:**
- Modify: `gormes/cmd/gormes-telegram/main.go`

- [ ] **Step 1: Edit `main.go`**

Find the existing block where `mstore` (SqliteStore) is opened and passed to `kernel.New`. **After** `mstore` is created and **before** (or parallel to) `bot.Run`, add:

```go
ext := memory.NewExtractor(mstore, hc, memory.ExtractorConfig{
	Model:        cfg.Hermes.Model,
	BatchSize:    cfg.Telegram.ExtractorBatchSize,
	PollInterval: cfg.Telegram.ExtractorPollInterval,
}, slog.Default())

go ext.Run(rootCtx)
defer func() {
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
	defer cancelShutdown()
	if err := ext.Close(shutdownCtx); err != nil {
		slog.Warn("extractor close", "err", err)
	}
}()
```

Placement: the extractor depends on `rootCtx` (signal.NotifyContext) and `hc` (hermes client) and `mstore`. Put the block immediately before the `go k.Run(rootCtx)` line. Both `go k.Run(rootCtx)` and `go ext.Run(rootCtx)` start cleanly together; shutdown runs in reverse via `defer`.

Update the startup log line to mention the extractor:

```go
slog.Info("gormes-telegram starting",
	"endpoint", cfg.Hermes.Endpoint,
	"allowed_chat_id", cfg.Telegram.AllowedChatID,
	"discovery", cfg.Telegram.FirstRunDiscovery,
	"sessions_db", config.SessionDBPath(),
	"memory_db", config.MemoryDBPath(),
	"extractor_batch_size", cfg.Telegram.ExtractorBatchSize,
	"extractor_poll_interval", cfg.Telegram.ExtractorPollInterval)
```

- [ ] **Step 2: Build + vet**

```bash
cd gormes
go build ./...
go vet ./...
go build -o bin/gormes-telegram ./cmd/gormes-telegram
ls -lh bin/gormes-telegram
```

Expected: clean build; binary ≤ 20 MB (should still be ~15 MB — extractor is pure Go code reusing existing deps).

TUI size unchanged:
```bash
make build
ls -lh bin/gormes
```

Expected: `bin/gormes` stays 8.2 MB.

- [ ] **Step 3: Bot startup smoke**

```bash
cd gormes
./bin/gormes-telegram 2>&1 | head -3
```

Expected: `gormes-telegram: no Telegram bot token — ...` exit 1 (unchanged).

With env:
```bash
cd gormes
export XDG_DATA_HOME=/tmp/gormes-3b-smoke-$$
GORMES_TELEGRAM_TOKEN=fake:token GORMES_TELEGRAM_CHAT_ID=99 \
  timeout 3 ./bin/gormes-telegram 2>&1 | tail -5 || true
# Look for: "extractor_batch_size=5 extractor_poll_interval=10s" in startup log
grep extractor_batch_size /dev/stderr 2>/dev/null  # reminder the user should see this in the log
rm -rf $XDG_DATA_HOME
```

- [ ] **Step 4: Full module sweep**

```bash
cd gormes
go test -race ./... -count=1 -timeout 180s
```

All green.

- [ ] **Step 5: Commit (from repo root)**

```bash
git add gormes/cmd/gormes-telegram/main.go
git commit -m "$(cat <<'EOF'
feat(gormes/cmd/telegram): wire Phase-3.B extractor goroutine

cmd/gormes-telegram now constructs memory.NewExtractor after
opening the SqliteStore, starts go ext.Run(rootCtx), and
defers a budgeted ext.Close(shutdownCtx). The extractor runs
in parallel with the kernel turn loop; both terminate on
SIGTERM via rootCtx cancellation.

Startup log adds extractor_batch_size + extractor_poll_interval
so operators can verify config without reading TOML.

No kernel, TUI, or internal API changes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Verification sweep

**Files:** no changes — verification only.

- [ ] **Step 1: Full test sweep under -race**

```bash
cd gormes
go test -race ./... -count=1 -timeout 180s
go vet ./...
```

Expected: every package `ok`, vet clean.

- [ ] **Step 2: Binary sizes**

```bash
cd gormes
make build
go build -o bin/gormes-telegram ./cmd/gormes-telegram
ls -lh bin/
```

Expected:
- `bin/gormes` ≤ 10 MB (target 8.2 MB, unchanged)
- `bin/gormes-telegram` ≤ 20 MB (target ~15.2 MB)

- [ ] **Step 3: Build-isolation grep**

```bash
cd gormes
echo "---TUI (must be OK)---"
(go list -deps ./cmd/gormes | grep -E "ncruces|internal/memory") && echo "VIOLATION" || echo "OK"
echo "---Kernel (must be OK)---"
(go list -deps ./internal/kernel | grep -E "ncruces|internal/memory") && echo "VIOLATION" || echo "OK"
echo "---Bot (must include memory)---"
go list -deps ./cmd/gormes-telegram | grep -E "ncruces/go-sqlite3$|internal/memory$"
```

Expected: first two print `OK`; third prints at least `ncruces/go-sqlite3` AND `.../internal/memory`.

- [ ] **Step 4: Migration upgrade test (manual)**

```bash
cd gormes
# Create a v3a DB manually via an old binary... not feasible. Instead,
# simulate by running migration logic in isolation:
cat > /tmp/3a_to_3b_check.sh <<'EOF'
#!/bin/bash
set -e
export XDG_DATA_HOME=/tmp/gormes-migrate-check-$$
mkdir -p $XDG_DATA_HOME/gormes
# Create a v3a DB using sqlite3 CLI with Phase-3.A schema only.
sqlite3 $XDG_DATA_HOME/gormes/memory.db <<SQL
CREATE TABLE schema_meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
INSERT INTO schema_meta(k, v) VALUES ('version', '3a');
CREATE TABLE turns (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  role TEXT NOT NULL CHECK(role IN ('user','assistant')),
  content TEXT NOT NULL,
  ts_unix INTEGER NOT NULL,
  meta_json TEXT
);
CREATE INDEX idx_turns_session_ts ON turns(session_id, ts_unix);
CREATE VIRTUAL TABLE turns_fts USING fts5(content, content='turns', content_rowid='id');
CREATE TRIGGER turns_ai AFTER INSERT ON turns BEGIN INSERT INTO turns_fts(rowid, content) VALUES (new.id, new.content); END;
CREATE TRIGGER turns_ad AFTER DELETE ON turns BEGIN INSERT INTO turns_fts(turns_fts, rowid, content) VALUES('delete', old.id, old.content); END;
CREATE TRIGGER turns_au AFTER UPDATE ON turns BEGIN INSERT INTO turns_fts(turns_fts, rowid, content) VALUES('delete', old.id, old.content); INSERT INTO turns_fts(rowid, content) VALUES (new.id, new.content); END;
INSERT INTO turns(session_id, role, content, ts_unix) VALUES('s','user','old turn',1);
SQL
echo "---PRE-MIGRATE: v3a---"
sqlite3 $XDG_DATA_HOME/gormes/memory.db "SELECT v FROM schema_meta; SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"

# Start + immediately terminate the bot to trigger OpenSqlite migration.
GORMES_TELEGRAM_TOKEN=fake:tok GORMES_TELEGRAM_CHAT_ID=99 timeout 1 ./bin/gormes-telegram > /dev/null 2>&1 || true

echo "---POST-MIGRATE: v3b---"
sqlite3 $XDG_DATA_HOME/gormes/memory.db "SELECT v FROM schema_meta; SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"
sqlite3 $XDG_DATA_HOME/gormes/memory.db "SELECT name FROM pragma_table_info('turns') WHERE name LIKE 'extract%'"
sqlite3 $XDG_DATA_HOME/gormes/memory.db "SELECT COUNT(*) as pre_migrate_turns FROM turns WHERE content = 'old turn'"
rm -rf $XDG_DATA_HOME
EOF
chmod +x /tmp/3a_to_3b_check.sh
/tmp/3a_to_3b_check.sh
```

Expected: PRE shows `v=3a` and 2 tables (`schema_meta`, `turns`); POST shows `v=3b` and 4 tables (`entities`, `relationships`, `schema_meta`, `turns`) plus the three extraction columns; the pre-existing turn with `content='old turn'` is preserved.

- [ ] **Step 5: Offline doctor unchanged**

```bash
cd gormes
./bin/gormes doctor --offline | tail -5
```

Expected: `[PASS] Toolbox: 3 tools registered (echo, now, rand_int)`.

- [ ] **Step 6: No commit**

Verification only. If any step fails, STOP and report.

---

## Appendix: Self-Review

**Spec coverage:**

| Spec § | Task(s) |
|---|---|
| §1 Goal | All tasks |
| §2 Non-goals | Enforced by scope |
| §3 Scope | T1-T11 |
| §4 Schema migration | T1 |
| §5 Worker architecture | T5 (scaffold) + T6 (loop) + T7 (dead-letter) + T8 (backoff) + T9 (shutdown) |
| §6 LLM I/O contract | T4 (prompt) + T2 (validator) + T6 (callLLM) |
| §7 Upsert logic | T3 |
| §8 Resilience taxonomy | T7 (dead-letter), T8 (backoff), T9 (graceful shutdown); 429 Retry-After parse deferred as interface-ready stub |
| §9 Integration | T10 (config) + T11 (cmd wiring) |
| §10 Testing strategy | Each task includes its tests; T12 verifies |
| §11 Binary budgets | T11 + T12 (measured) |
| §12 Security | T3 (parameterized SQL) + T4 (prompt discipline) + T6 (malformed-JSON logging caps at 200 chars) |
| §13 Out of scope | No tasks (correctly) |
| §14 Verification checklist | T12 |
| §15 Rollout | Structural — each task is a separate commit |

**Placeholder scan:** no `TBD` / `TODO` / `fill in` / `similar to Task N`. T8's "429 Retry-After parse deferred" is a deliberate forward-compatibility note, not a placeholder — the backoff path functions correctly today; a future hermes interface bump adds the Retry-After optimization without touching the task code.

**Type consistency:**
- `memory.ExtractorConfig` — fields named identically across T5 (declaration), T6 (consumption), T10 (config mapping), T11 (construction).
- `memory.Extractor`, `memory.NewExtractor`, `.Run`, `.Close`, `.loopOnce`, `.callLLM`, `.pollBatch`, `.recordFailure`, `.advanceBackoff` — all declared in T5-T9, all referenced consistently.
- `memory.ValidatedOutput`, `ValidatedEntity`, `ValidatedRelationship` — T2 declarations, T3/T6 consumers. Consistent.
- `memory.turnRow{id, role, content}` — T4 declaration, T5/T6 consumers. Consistent.
- `memory.writeGraphBatch`, `incrementAttempts`, `markDeadLetter`, `inListArgs`, `execInIDs{,DB}` — T3 declarations, T7 consumer for markDeadLetter. Consistent.
- `memory.schemaVersion = "3b"`, `migration3aTo3b`, `schemaV3a`, `migrate`, `ErrSchemaUnknown` — T1 declarations; no downstream consumers need to reference them by name (called indirectly via `OpenSqlite`). Consistent.
- `config.TelegramCfg.ExtractorBatchSize`, `ExtractorPollInterval` — T10 declaration, T11 consumer. Consistent.

**Execution order:** linear dependency chain. Each task's tests compile against symbols introduced earlier. Recommended sequence: **T1 → T2 → T3 → T4 → T5 → T6 → T7 → T8 → T9 → T10 → T11 → T12**.

**Checkpoint suggestions:** halt after **T6** (happy-path loop running end-to-end) for a sanity check that the kernel remains isolated and the graph populates; halt after **T9** (worker operationally complete) before T11 wires it into the bot.

**Scope:** one cohesive Phase-3.B plan — schema migration + worker + LLM I/O + config + cmd wiring + verification. No spill into 3.C (recall tools) or 3.D (vector embeddings).
