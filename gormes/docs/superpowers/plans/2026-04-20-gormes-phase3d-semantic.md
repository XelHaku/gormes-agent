# Gormes Phase 3.D — Semantic Fusion + Local Embeddings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close Phase 3.C's lexical gap ("tell me about my projects" → empty fence) by adding a third seed source to recall: cosine similarity against local embeddings stored in a new `entity_embeddings` table, populated by a background worker talking to Ollama's `/v1/embeddings` endpoint.

**Architecture:** Hybrid — not replacement. Existing lexical + FTS5 seed selection (Phase 3.C) stays first in the chain for speed. A new semantic layer runs LAST, only when needed. Vectors are L2-normalized at storage so cosine reduces to a dot product. In-memory cache invalidated via a monotonic graph-version counter. Kernel is unchanged; all new logic is internal to `memory.Provider`.

**Tech Stack:** Go 1.25+, existing ncruces SQLite (no C extensions — cosine runs in Go), Ollama's OpenAI-compatible `/v1/embeddings` endpoint, `encoding/binary` for float32 little-endian BLOB packing.

**Module path:** `github.com/TrebuchetDynamics/gormes-agent/gormes`

**Spec:** [`docs/superpowers/specs/2026-04-20-gormes-phase3d-semantic-design.md`](../specs/2026-04-20-gormes-phase3d-semantic-design.md) (approved `be060e06`)

**Tech Lead task grouping:** the 10 micro-tasks below map to the 6 logical groupings from the approval message. Dependency ordering dictates the split (e.g. user's T5 "Ollama integration" happens first as my T2 because T3/T4/T5 all depend on the embed client existing):

| Lead's Task | Maps to plan tasks |
|---|---|
| T1 — Schema migration (entity_embeddings) | T1 |
| T2 — Embedder background worker | T5 |
| T3 — Similarity scan (Go cache + dot product) | T3, T4 |
| T4 — Recall fusion | T6 |
| T5 — Ollama /v1/embeddings client | T2 |
| T6 — Testing | T9, T10 |

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `gormes/internal/memory/schema.go` | Modify | Bump `schemaVersion = "3d"`; add `migration3cTo3d` constant |
| `gormes/internal/memory/migrate.go` | Modify | Extend switch to run `3c → 3d` step |
| `gormes/internal/memory/migrate_test.go` | Modify | Append `TestMigrate_3cTo3d` + table-shape tests |
| `gormes/internal/memory/embed_client.go` | Create | `embedClient` type + `Embed(ctx, model, input)` method |
| `gormes/internal/memory/embed_client_test.go` | Create | httptest-based unit tests |
| `gormes/internal/memory/cosine.go` | Create | `l2Normalize`, `dotProduct`, `topK`, `encodeFloat32LE`, `decodeFloat32LE` pure funcs |
| `gormes/internal/memory/cosine_test.go` | Create | Pure math + roundtrip tests |
| `gormes/internal/memory/semantic_sql.go` | Create | `semanticSeeds` + in-memory cache + graph-version counter |
| `gormes/internal/memory/semantic_sql_test.go` | Create | SQL + cache tests |
| `gormes/internal/memory/embedder.go` | Create | `Embedder` background worker — poll + embed + store |
| `gormes/internal/memory/embedder_test.go` | Create | Worker lifecycle + error-path tests |
| `gormes/internal/memory/recall.go` | Modify | `RecallConfig` gains semantic fields; `Provider.GetContext` chains semantic seeds |
| `gormes/internal/memory/recall_test.go` | Modify | Append 3 hybrid-fusion tests |
| `gormes/internal/config/config.go` | Modify | `TelegramCfg` gains semantic_* + embedder_* knobs |
| `gormes/internal/config/config_test.go` | Modify | Append `TestLoad_SemanticDefaults` |
| `gormes/cmd/gormes/telegram.go` | Modify | Construct `Embedder`, thread semantic config through `memory.NewRecall` |
| `gormes/internal/memory/semantic_integration_test.go` | Create | Ollama E2E: "tell me about my projects" → fence contains "AzulVigia" |

---

## Task 1: Schema v3d migration — `entity_embeddings` table

**Files:**
- Modify: `gormes/internal/memory/schema.go`
- Modify: `gormes/internal/memory/migrate.go`
- Modify: `gormes/internal/memory/migrate_test.go`

- [ ] **Step 1: Write failing migration tests**

Append to `gormes/internal/memory/migrate_test.go`:

```go
func TestOpenSqlite_FreshDBIsV3d(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var v string
	_ = s.db.QueryRow("SELECT v FROM schema_meta WHERE k = 'version'").Scan(&v)
	if v != "3d" {
		t.Errorf("schema version = %q, want 3d", v)
	}
}

func TestMigrate_3cTo3d_AddsEntityEmbeddingsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings`).Scan(&n)
	if err != nil {
		t.Errorf("entity_embeddings table missing: %v", err)
	}
}

func TestMigrate_3cTo3d_HasModelIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	var name string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_entity_embeddings_model'`,
	).Scan(&name)
	if err != nil {
		t.Errorf("idx_entity_embeddings_model missing: %v", err)
	}
}

func TestMigrate_3cTo3d_DimCheckConstraint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	// Must reject dim=0 and dim>4096 per the CHECK constraint.
	_, _ = s.db.Exec(`INSERT INTO entities(name, type, updated_at) VALUES('X','PERSON',1)`)
	var id int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name='X'`).Scan(&id)

	_, err := s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'm', 0, x'00', 1)`,
		id)
	if err == nil {
		t.Error("dim=0 should trip CHECK constraint")
	}

	_, err = s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'm', 5000, x'00', 1)`,
		id)
	if err == nil {
		t.Error("dim=5000 should trip CHECK(dim <= 4096)")
	}
}

func TestMigrate_3cTo3d_FKCascadeOnEntityDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	_, _ = s.db.Exec(`INSERT INTO entities(name, type, updated_at) VALUES('Y','PERSON',1)`)
	var id int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name='Y'`).Scan(&id)

	_, _ = s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'm', 4, x'00000000', 1)`,
		id)

	_, _ = s.db.Exec(`DELETE FROM entities WHERE id = ?`, id)

	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings WHERE entity_id = ?`, id).Scan(&n)
	if n != 0 {
		t.Errorf("entity_embeddings not cascaded on entity delete; found %d rows", n)
	}
}
```

**IMPORTANT:** Also update the pre-existing `TestOpenSqlite_SchemaMetaVersion` — change its expected value from `"3c"` to `"3d"`.

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run "TestOpenSqlite_FreshDBIsV3d|TestMigrate_3cTo3d" -v 2>&1 | tail -10
```

Expected: failing assertions.

- [ ] **Step 3: Update `schema.go` — bump version + add migration fragment**

In `gormes/internal/memory/schema.go`:

1. Change `const schemaVersion = "3c"` to `const schemaVersion = "3d"`.

2. Append a new migration constant at the end:

```go
// migration3cTo3d extends v3c with Phase 3.D semantic fusion:
//   - entity_embeddings table holds L2-normalized float32 vectors
//     (little-endian BLOB) alongside model name + dim for mismatch
//     detection. FK cascade cleans up if the entity is deleted.
//   - idx_entity_embeddings_model makes model-filtered scans cheap.
const migration3cTo3d = `
CREATE TABLE IF NOT EXISTS entity_embeddings (
	entity_id   INTEGER PRIMARY KEY,
	model       TEXT    NOT NULL,
	dim         INTEGER NOT NULL CHECK(dim > 0 AND dim <= 4096),
	vec         BLOB    NOT NULL,
	updated_at  INTEGER NOT NULL,
	FOREIGN KEY(entity_id) REFERENCES entities(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_entity_embeddings_model
	ON entity_embeddings(model);

UPDATE schema_meta SET v = '3d' WHERE k = 'version' AND v = '3c';
`
```

- [ ] **Step 4: Extend `migrate.go` switch**

In `gormes/internal/memory/migrate.go`, the `switch v` currently handles `"3a"`, `"3b"`, `"3c"` (no-op target), default. Change to:

```go
	switch v {
	case "3a":
		if err := runMigrationTx(db, migration3aTo3b); err != nil {
			return fmt.Errorf("memory: migrate 3a->3b: %w", err)
		}
		return migrate(db)
	case "3b":
		if err := runMigrationTx(db, migration3bTo3c); err != nil {
			return fmt.Errorf("memory: migrate 3b->3c: %w", err)
		}
		return migrate(db)
	case "3c":
		if err := runMigrationTx(db, migration3cTo3d); err != nil {
			return fmt.Errorf("memory: migrate 3c->3d: %w", err)
		}
		return nil
	case "3d":
		return nil
	default:
		return fmt.Errorf("%w: got %q, want %q", ErrSchemaUnknown, v, schemaVersion)
	}
```

Note: `case "3b"` now recurses (was `return nil` when 3b was the target). Each prior migration step now chains forward.

- [ ] **Step 5: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run "TestMigrate_|TestOpenSqlite_(Fresh|Schema)" -v -timeout 30s
```

All pass — 5 new + all pre-existing migration tests.

Full memory suite:
```bash
cd gormes
go test -race ./internal/memory/... -count=1 -timeout 60s -skip Integration_Ollama
```

Green.

- [ ] **Step 6: Commit (from repo root)**

```bash
git add gormes/internal/memory/schema.go gormes/internal/memory/migrate.go gormes/internal/memory/migrate_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): schema v3d adds entity_embeddings table

Phase 3.D semantic fusion foundation.

  migration3cTo3d:
    CREATE TABLE entity_embeddings (
      entity_id PK, model, dim CHECK>0 AND <=4096,
      vec BLOB (L2-normalized float32 LE), updated_at,
      FK entity_id -> entities ON DELETE CASCADE
    )
    CREATE INDEX idx_entity_embeddings_model(model)

  migrate() switch extended:
    3a -> 3b (existing, now recurses forward)
    3b -> 3c (existing, now recurses forward)
    3c -> 3d (NEW)
    3d = target; no-op.

Five tests lock the invariants:
  - FreshDBIsV3d: new installs land on v3d
  - 3cTo3d_AddsEntityEmbeddingsTable: table exists
  - 3cTo3d_HasModelIndex: model-filter scan index present
  - 3cTo3d_DimCheckConstraint: rejects dim=0 and dim>4096
  - 3cTo3d_FKCascadeOnEntityDelete: deleting an entity
    purges its embedding row

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `embedClient` — narrow client for `/v1/embeddings`

**Files:**
- Create: `gormes/internal/memory/embed_client.go`
- Create: `gormes/internal/memory/embed_client_test.go`

Narrow HTTP client for OpenAI-compatible `/v1/embeddings`. Deliberately separate from `hermes.Client` — keeping the kernel's chat-streaming interface focused.

- [ ] **Step 1: Write failing tests FIRST (httptest-based)**

Create `gormes/internal/memory/embed_client_test.go`:

```go
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmbedClient_ParsesOpenAIResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"object": "embedding", "embedding": []float32{0.1, 0.2, 0.3, 0.4}, "index": 0},
			},
			"model": "test-model",
		})
	}))
	defer ts.Close()

	c := newEmbedClient(ts.URL, "")
	vec, err := c.Embed(context.Background(), "test-model", "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 4 {
		t.Fatalf("len = %d, want 4", len(vec))
	}
	for i, want := range []float32{0.1, 0.2, 0.3, 0.4} {
		if vec[i] != want {
			t.Errorf("vec[%d] = %v, want %v", i, vec[i], want)
		}
	}
}

func TestEmbedClient_ModelNotFoundError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": `model "nomic-embed-text" not found, try pulling it first`,
				"type":    "not_found_error",
			},
		})
	}))
	defer ts.Close()

	c := newEmbedClient(ts.URL, "")
	_, err := c.Embed(context.Background(), "nomic-embed-text", "hello")
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !errors.Is(err, errEmbedModelNotFound) {
		t.Errorf("err = %v, want errors.Is(err, errEmbedModelNotFound)", err)
	}
}

func TestEmbedClient_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal"))
	}))
	defer ts.Close()

	c := newEmbedClient(ts.URL, "")
	_, err := c.Embed(context.Background(), "any", "x")
	if err == nil {
		t.Fatal("expected error on 5xx")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v, want mention of 500", err)
	}
}

func TestEmbedClient_CtxTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // longer than ctx budget below
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	c := newEmbedClient(ts.URL, "")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Embed(ctx, "any", "x")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestEmbedClient_AuthorizationHeader(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float32{1}}},
		})
	}))
	defer ts.Close()

	c := newEmbedClient(ts.URL, "my-key")
	_, _ = c.Embed(context.Background(), "m", "x")
	if gotAuth != "Bearer my-key" {
		t.Errorf("Authorization = %q, want 'Bearer my-key'", gotAuth)
	}
}
```

- [ ] **Step 2: Run, expect FAIL (undefined symbols)**

```bash
cd gormes
go test ./internal/memory/... -run TestEmbedClient_ 2>&1 | head -5
```

Expected: `undefined: newEmbedClient`, `undefined: errEmbedModelNotFound`.

- [ ] **Step 3: Write `embed_client.go`**

Create `gormes/internal/memory/embed_client.go`:

```go
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// errEmbedModelNotFound is returned when the Ollama endpoint reports 404
// with a "model not found" body. Callers (the Embedder worker) handle
// this by logging a one-per-minute WARN and waiting; it's not a crash.
var errEmbedModelNotFound = errors.New("memory: embed model not loaded")

// embedClient is a narrow HTTP client for the OpenAI-compatible
// /v1/embeddings endpoint. Deliberately separate from hermes.Client —
// the kernel's Client interface is focused on chat streaming; mixing
// embedding concerns in would widen that surface for a feature only
// the memory package uses.
type embedClient struct {
	baseURL string // e.g. "http://localhost:11434"
	apiKey  string // optional — "Bearer <key>" if non-empty
	http    *http.Client
}

func newEmbedClient(baseURL, apiKey string) *embedClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 10 * time.Second
	return &embedClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 0, Transport: transport},
	}
}

// Embed calls POST /v1/embeddings with the given model + input. Returns
// the first (and only) embedding vector from the response's `data[0]`.
// The caller is responsible for L2-normalizing the result before storage.
func (c *embedClient) Embed(ctx context.Context, model, input string) ([]float32, error) {
	body, err := json.Marshal(map[string]any{
		"model": model,
		"input": input,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("memory: embed HTTP: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		raw, _ := io.ReadAll(resp.Body)
		if strings.Contains(strings.ToLower(string(raw)), "not found") {
			return nil, fmt.Errorf("%w: %s", errEmbedModelNotFound, string(raw))
		}
		return nil, fmt.Errorf("memory: embed 404: %s", string(raw))
	}
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("memory: embed HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var wire struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
		return nil, fmt.Errorf("memory: embed decode: %w", err)
	}
	if len(wire.Data) == 0 || len(wire.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("memory: embed response has no vector")
	}
	return wire.Data[0].Embedding, nil
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestEmbedClient_ -v
go vet ./...
```

All 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/memory/embed_client.go gormes/internal/memory/embed_client_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): embedClient for OpenAI-compat /v1/embeddings

Narrow HTTP client for POST /v1/embeddings. Deliberately
separate from hermes.Client — keeping the kernel's chat-
streaming interface focused. Ollama's /v1/embeddings
endpoint is the verified target (empirically tested against
local Ollama 2026-04-20: any loaded LLM returns usable
vectors).

Typed error: errEmbedModelNotFound for 404-with-"not found"
body. The Embedder worker uses this to throttle WARN logs
when the model isn't pulled.

Five tests cover:
  - Happy-path response parse (float32 array)
  - Model-not-found error classification
  - Generic 5xx error
  - Context-deadline timeout
  - Authorization: Bearer header when apiKey set

Caller is responsible for L2-normalizing the returned vector
before storage — that's T3's scope.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Pure math — `cosine.go` (L2 normalize + dot product + top-K + encode/decode)

**Files:**
- Create: `gormes/internal/memory/cosine.go`
- Create: `gormes/internal/memory/cosine_test.go`

All pure functions — zero DB, zero network.

- [ ] **Step 1: Write failing tests FIRST**

Create `gormes/internal/memory/cosine_test.go`:

```go
package memory

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestL2Normalize_UnitMagnitude(t *testing.T) {
	v := []float32{3, 4} // magnitude 5
	l2Normalize(v)
	mag := math.Sqrt(float64(v[0]*v[0] + v[1]*v[1]))
	if math.Abs(mag-1.0) > 1e-6 {
		t.Errorf("magnitude = %v, want 1.0 ± 1e-6", mag)
	}
}

func TestL2Normalize_ZeroVector(t *testing.T) {
	v := []float32{0, 0, 0}
	l2Normalize(v) // must not divide by zero
	for i, x := range v {
		if x != 0 {
			t.Errorf("v[%d] = %v, want 0 (zero-vector stays zero)", i, x)
		}
	}
}

func TestDotProduct_IdenticalNormalizedIsOne(t *testing.T) {
	a := []float32{0.6, 0.8} // already unit
	got := dotProduct(a, a)
	if math.Abs(float64(got)-1.0) > 1e-6 {
		t.Errorf("a·a = %v, want 1.0", got)
	}
}

func TestDotProduct_OrthogonalIsZero(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{0, 1}
	got := dotProduct(a, b)
	if got != 0 {
		t.Errorf("orthogonal = %v, want 0", got)
	}
}

func TestDotProduct_OppositeIsMinusOne(t *testing.T) {
	a := []float32{1, 0}
	b := []float32{-1, 0}
	got := dotProduct(a, b)
	if got != -1.0 {
		t.Errorf("opposite = %v, want -1.0", got)
	}
}

func TestDotProduct_DifferentLengthsReturnsZero(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{1, 2}
	got := dotProduct(a, b)
	if got != 0 {
		t.Errorf("mismatched dim = %v, want 0 (defensive)", got)
	}
}

func TestTopK_ReturnsKHighest(t *testing.T) {
	scored := []scoredID{
		{ID: 1, Score: 0.1},
		{ID: 2, Score: 0.9},
		{ID: 3, Score: 0.5},
		{ID: 4, Score: 0.8},
	}
	got := topK(scored, 2)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// Sorted by score DESC.
	if got[0].ID != 2 || got[1].ID != 4 {
		t.Errorf("got = %+v, want [{2, 0.9}, {4, 0.8}]", got)
	}
}

func TestTopK_KLargerThanInput(t *testing.T) {
	scored := []scoredID{{ID: 1, Score: 0.5}}
	got := topK(scored, 10)
	if len(got) != 1 {
		t.Errorf("len = %d, want 1 (K > input)", len(got))
	}
}

func TestTopK_KZeroReturnsEmpty(t *testing.T) {
	scored := []scoredID{{ID: 1, Score: 0.5}}
	got := topK(scored, 0)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0 (K=0)", len(got))
	}
}

func TestEncodeFloat32LE_RoundTrip(t *testing.T) {
	in := []float32{0.1, -0.2, 3.14159, -42.5, 0}
	encoded := encodeFloat32LE(in)
	if len(encoded) != len(in)*4 {
		t.Errorf("encoded len = %d, want %d", len(encoded), len(in)*4)
	}
	out, err := decodeFloat32LE(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != len(in) {
		t.Fatalf("decoded len = %d, want %d", len(out), len(in))
	}
	for i, want := range in {
		if out[i] != want {
			t.Errorf("out[%d] = %v, want %v", i, out[i], want)
		}
	}
}

func TestDecodeFloat32LE_OddByteLengthErrors(t *testing.T) {
	_, err := decodeFloat32LE([]byte{1, 2, 3}) // not multiple of 4
	if err == nil {
		t.Error("expected error for odd-length BLOB")
	}
}

func TestEncodeFloat32LE_LittleEndianOrder(t *testing.T) {
	// 1.0 in float32 little-endian is 0x3f800000 = bytes [0x00,0x00,0x80,0x3f]
	encoded := encodeFloat32LE([]float32{1.0})
	want := []byte{0x00, 0x00, 0x80, 0x3f}
	if !bytes.Equal(encoded, want) {
		t.Errorf("encoded = %v, want %v", encoded, want)
	}
	// Double-check against encoding/binary for future maintenance.
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, float32(1.0))
	if !bytes.Equal(encoded, buf.Bytes()) {
		t.Errorf("encoded != binary.LittleEndian write")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run "TestL2Normalize_|TestDotProduct_|TestTopK_|TestEncodeFloat32LE_|TestDecodeFloat32LE_" 2>&1 | head -5
```

Expected: `undefined: l2Normalize`, `undefined: dotProduct`, etc.

- [ ] **Step 3: Write `cosine.go`**

Create `gormes/internal/memory/cosine.go`:

```go
package memory

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// scoredID is a (entity_id, similarity) pair — the output of the
// similarity scan, consumable by topK.
type scoredID struct {
	ID    int64
	Score float32
}

// l2Normalize rescales v in-place to unit magnitude. A zero vector
// stays zero (defensive — avoids NaN from 0/0).
func l2Normalize(v []float32) {
	var sumSq float32
	for _, x := range v {
		sumSq += x * x
	}
	if sumSq == 0 {
		return
	}
	inv := float32(1.0 / math.Sqrt(float64(sumSq)))
	for i := range v {
		v[i] *= inv
	}
}

// dotProduct returns sum(a[i]*b[i]). For L2-normalized vectors this
// IS the cosine similarity. Defensively returns 0 on mismatched dim
// rather than panicking — corrupt rows shouldn't crash recall.
func dotProduct(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot float32
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot
}

// topK returns the K highest-scoring entries, sorted score-descending.
// For K >= len(scored), returns all entries. For K <= 0, returns empty.
// Runs in O(n log k) via a simple sort — for Gormes scale (≤10k entities,
// K=3), the dedicated min-heap optimization would save microseconds at
// the cost of code complexity.
func topK(scored []scoredID, k int) []scoredID {
	if k <= 0 {
		return nil
	}
	out := make([]scoredID, len(scored))
	copy(out, scored)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	if len(out) > k {
		out = out[:k]
	}
	return out
}

// encodeFloat32LE packs a float32 slice into a BLOB of little-endian
// bytes (4 bytes per float). Used for entity_embeddings.vec storage.
func encodeFloat32LE(v []float32) []byte {
	out := make([]byte, 4*len(v))
	for i, f := range v {
		bits := math.Float32bits(f)
		binary.LittleEndian.PutUint32(out[i*4:], bits)
	}
	return out
}

// decodeFloat32LE is the inverse of encodeFloat32LE. Returns an error
// if the input length isn't a multiple of 4.
func decodeFloat32LE(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("memory: decodeFloat32LE: length %d not multiple of 4", len(b))
	}
	out := make([]float32, len(b)/4)
	for i := range out {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		out[i] = math.Float32frombits(bits)
	}
	return out, nil
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run "TestL2Normalize_|TestDotProduct_|TestTopK_|TestEncodeFloat32LE_|TestDecodeFloat32LE_" -v
go vet ./...
```

All 12 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/memory/cosine.go gormes/internal/memory/cosine_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): pure math helpers — cosine + normalize + topK + LE codec

Zero-dependency pure functions for the semantic scan path:

  l2Normalize(v)       in-place unit-vector scaling; zero
                       vectors stay zero (avoid 0/0 NaN)
  dotProduct(a, b)     = cosine similarity when both inputs
                       are L2-normalized; defensively returns
                       0 on mismatched dim
  topK(scored, k)      top-K by score DESC; K <= 0 -> empty;
                       K >= len -> all
  encodeFloat32LE(v)   []float32 -> []byte (4 bytes each, LE)
  decodeFloat32LE(b)   []byte -> []float32; errors on
                       non-multiple-of-4 length

Twelve tests cover math correctness + byte-level endianness
(1.0 -> 0x00 0x00 0x80 0x3f cross-checked against stdlib
binary.LittleEndian).

No new deps beyond stdlib math + sort + encoding/binary.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `semanticSeeds` + in-memory vector cache

**Files:**
- Create: `gormes/internal/memory/semantic_sql.go`
- Create: `gormes/internal/memory/semantic_sql_test.go`

Loads vectors from `entity_embeddings` into an in-memory cache on first use; serves `semanticSeeds(queryVec, topK, threshold)`; invalidated via a monotonic `graphVersion` counter bumped on every write.

- [ ] **Step 1: Write failing tests**

Create `gormes/internal/memory/semantic_sql_test.go`:

```go
package memory

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// seedEmbeddedGraph inserts N entities with fabricated embeddings for
// model "test-model". Embedding for entity i is [1, 0, 0, ..., 0] with
// the 1 in position i mod dim — so different entities are orthogonal
// (cosine = 0) and queries matching any position cosine-match exactly 1.
func seedEmbeddedGraph(t *testing.T, s *SqliteStore, model string, dim int, names []string) map[string]int64 {
	t.Helper()
	ids := make(map[string]int64)
	now := time.Now().Unix()
	for i, name := range names {
		_, err := s.db.Exec(
			`INSERT INTO entities(name, type, updated_at) VALUES(?, 'PERSON', ?)`,
			name, now)
		if err != nil {
			t.Fatal(err)
		}
		var id int64
		_ = s.db.QueryRow(`SELECT id FROM entities WHERE name = ?`, name).Scan(&id)
		ids[name] = id

		vec := make([]float32, dim)
		vec[i%dim] = 1.0
		l2Normalize(vec) // stays unit because only one nonzero
		blob := encodeFloat32LE(vec)
		_, err = s.db.Exec(
			`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at)
			 VALUES(?, ?, ?, ?, ?)`,
			id, model, dim, blob, now)
		if err != nil {
			t.Fatalf("insert embedding for %s: %v", name, err)
		}
	}
	return ids
}

func TestSemanticSeeds_MatchesExactPosition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	ids := seedEmbeddedGraph(t, s, "test-model", 4,
		[]string{"A", "B", "C", "D"}) // positions 0, 1, 2, 3

	// Query vector matches position 2: [0, 0, 1, 0]
	query := []float32{0, 0, 1, 0}
	l2Normalize(query)

	got, err := semanticSeeds(context.Background(), s.db, cache, "test-model", query, 1, 0.9)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0] != ids["C"] {
		t.Errorf("got = %d, want %d (C)", got[0], ids["C"])
	}
}

func TestSemanticSeeds_ThresholdFiltersLowScores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	seedEmbeddedGraph(t, s, "test-model", 4, []string{"A", "B"})

	// Query orthogonal to both seeded entities.
	query := []float32{0, 0, 0, 1}
	l2Normalize(query)

	got, _ := semanticSeeds(context.Background(), s.db, cache,
		"test-model", query, 5, 0.5) // threshold 0.5
	// Neither "A" nor "B" has any overlap with position 3; all cosines are 0.
	if len(got) != 0 {
		t.Errorf("threshold 0.5 should filter; got %d seeds", len(got))
	}
}

func TestSemanticSeeds_FiltersByModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	seedEmbeddedGraph(t, s, "model-A", 4, []string{"X"})
	seedEmbeddedGraph(t, s, "model-B", 4, []string{"Y"})

	// Query with model-A; should only see X, not Y (same vector).
	query := []float32{1, 0, 0, 0}
	l2Normalize(query)
	got, _ := semanticSeeds(context.Background(), s.db, cache,
		"model-A", query, 5, 0.5)
	for _, id := range got {
		var name string
		_ = s.db.QueryRow(`SELECT name FROM entities WHERE id = ?`, id).Scan(&name)
		if name == "Y" {
			t.Errorf("model-B's Y leaked into model-A scan")
		}
	}
}

func TestSemanticSeeds_CacheInvalidatesOnGraphBump(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	ids1 := seedEmbeddedGraph(t, s, "test-model", 4, []string{"A"})

	query := []float32{1, 0, 0, 0}
	got, _ := semanticSeeds(context.Background(), s.db, cache,
		"test-model", query, 5, 0.5)
	if len(got) != 1 || got[0] != ids1["A"] {
		t.Fatalf("first call: got %v, want [%d]", got, ids1["A"])
	}

	// Add a new embedding + bump the cache version.
	seedEmbeddedGraph(t, s, "test-model", 4, []string{"B"})
	cache.bump()

	query2 := []float32{0, 1, 0, 0}
	got, _ = semanticSeeds(context.Background(), s.db, cache,
		"test-model", query2, 5, 0.5)
	if len(got) == 0 {
		t.Error("expected B to surface after cache bump; got nothing")
	}
}

func TestSemanticSeeds_EmptyDatabaseReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	query := []float32{1, 0, 0, 0}
	got, err := semanticSeeds(context.Background(), s.db, cache,
		"test-model", query, 5, 0.5)
	if err != nil {
		t.Errorf("err = %v, want nil on empty DB", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestSemanticSeeds_SkipsCorruptRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	cache := newSemanticCache()
	// Good entity with correct embedding.
	_, _ = s.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('Good','PERSON',1)`)
	var goodID int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name='Good'`).Scan(&goodID)
	goodVec := []float32{1, 0, 0, 0}
	_, _ = s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at)
		 VALUES(?, 'm', 4, ?, 1)`,
		goodID, encodeFloat32LE(goodVec))

	// Corrupt entity — dim says 4 but BLOB has 3 bytes. CHECK constraint
	// on dim passes (4 > 0 and <= 4096), but decode should fail.
	_, _ = s.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('Bad','PERSON',1)`)
	var badID int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name='Bad'`).Scan(&badID)
	_, _ = s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at)
		 VALUES(?, 'm', 4, ?, 1)`,
		badID, []byte{1, 2, 3}) // not a multiple of 4

	query := []float32{1, 0, 0, 0}
	got, err := semanticSeeds(context.Background(), s.db, cache,
		"m", query, 5, 0.5)
	if err != nil {
		t.Errorf("scan should tolerate corrupt rows: %v", err)
	}
	// Good should still be found.
	if len(got) == 0 || got[0] != goodID {
		t.Errorf("got = %v, want [%d]", got, goodID)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run "TestSemanticSeeds_" 2>&1 | head -5
```

Expected: `undefined: semanticSeeds`, `undefined: newSemanticCache`.

- [ ] **Step 3: Write `semantic_sql.go`**

Create `gormes/internal/memory/semantic_sql.go`:

```go
package memory

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
)

// semanticCache is an in-memory cache of (entity_id, L2-normalized vector,
// model) rows from entity_embeddings. Loaded lazily on first semanticSeeds
// call; rebuilt when the graph-version counter changes.
type semanticCache struct {
	// graphVersion is incremented on every write to entities or
	// entity_embeddings. Cached entries are tagged with the version
	// they were loaded at; a mismatch triggers a rebuild.
	graphVersion atomic.Uint64

	mu       sync.Mutex
	loadedAt uint64 // graphVersion at last load
	entries  []cacheEntry
	byModel  string // the model whose vectors this cache currently holds
}

type cacheEntry struct {
	entityID int64
	vec      []float32
}

func newSemanticCache() *semanticCache { return &semanticCache{} }

// bump invalidates the cache. Called by writers (Embedder on insert;
// the extractor's writeGraphBatch indirectly via its own bump).
func (c *semanticCache) bump() { c.graphVersion.Add(1) }

// ensureLoaded rebuilds the cache for the given model if stale.
func (c *semanticCache) ensureLoaded(ctx context.Context, db *sql.DB, model string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	currentVersion := c.graphVersion.Load()
	if c.loadedAt == currentVersion && c.byModel == model && c.entries != nil {
		return nil // fresh + same model
	}

	rows, err := db.QueryContext(ctx,
		`SELECT entity_id, dim, vec FROM entity_embeddings WHERE model = ?`, model)
	if err != nil {
		return err
	}
	defer rows.Close()

	var entries []cacheEntry
	for rows.Next() {
		var id int64
		var dim int
		var blob []byte
		if err := rows.Scan(&id, &dim, &blob); err != nil {
			return err
		}
		vec, err := decodeFloat32LE(blob)
		if err != nil || len(vec) != dim {
			// Corrupt row; skip silently. The embedder will eventually
			// re-populate with a correct vector.
			continue
		}
		entries = append(entries, cacheEntry{entityID: id, vec: vec})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	c.loadedAt = currentVersion
	c.byModel = model
	c.entries = entries
	return nil
}

// semanticSeeds runs the Top-K cosine-similarity scan over the cached
// vectors for the given model. Query vector is expected to already be
// L2-normalized by the caller. Returns entity IDs above the threshold,
// sorted by similarity DESC.
func semanticSeeds(
	ctx context.Context,
	db *sql.DB,
	cache *semanticCache,
	model string,
	queryVec []float32,
	topKCount int,
	minSimilarity float64,
) ([]int64, error) {
	if len(queryVec) == 0 || topKCount <= 0 {
		return nil, nil
	}

	if err := cache.ensureLoaded(ctx, db, model); err != nil {
		return nil, err
	}

	cache.mu.Lock()
	// Snapshot the slice header so we can release the lock quickly.
	// Entries themselves are immutable once loaded (cache rebuilds on
	// bump) so reading them lock-free is safe.
	snapshot := cache.entries
	cache.mu.Unlock()

	if len(snapshot) == 0 {
		return nil, nil
	}

	scored := make([]scoredID, 0, len(snapshot))
	for _, e := range snapshot {
		if len(e.vec) != len(queryVec) {
			continue // dim mismatch; model may have changed mid-flight
		}
		sim := dotProduct(queryVec, e.vec)
		if float64(sim) < minSimilarity {
			continue
		}
		scored = append(scored, scoredID{ID: e.entityID, Score: sim})
	}

	top := topK(scored, topKCount)
	ids := make([]int64, len(top))
	for i, s := range top {
		ids[i] = s.ID
	}
	return ids, nil
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestSemanticSeeds_ -v -timeout 30s
go vet ./...
```

All 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/memory/semantic_sql.go gormes/internal/memory/semantic_sql_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): semanticSeeds + in-memory vector cache

semanticSeeds runs a flat cosine-similarity scan (dot product
on L2-normalized vectors) against all entity_embeddings rows
for the current model. Filtered by min-similarity threshold,
sorted, top-K.

The semanticCache holds the vectors in RAM to avoid
re-deserializing 4KB BLOBs on every recall call. Invalidated
via a monotonic graph-version counter — writers call
cache.bump() after any mutation to entities or
entity_embeddings.

Defense-in-depth:
  - Corrupt rows (decodeFloat32LE error OR len != dim) are
    silently skipped; recall doesn't crash on bad data
  - Dim mismatch between query and cached vector skips that
    row (model-switch race survives)
  - Empty DB returns (nil, nil) — no error

Six tests cover exact-position match, threshold filtering,
model filter, cache bump invalidation, empty DB, corrupt row
tolerance.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `Embedder` background worker

**Files:**
- Create: `gormes/internal/memory/embedder.go`
- Create: `gormes/internal/memory/embedder_test.go`

- [ ] **Step 1: Write failing tests FIRST**

Create `gormes/internal/memory/embedder_test.go`:

```go
package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// fakeEmbedServer returns deterministic vectors for each input.
// Entity "A" maps to [1,0,0,0]; "B" to [0,1,0,0]; everything else to zeros.
func fakeEmbedServer(t *testing.T, callCount *atomic.Int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		var req struct {
			Model string `json:"model"`
			Input string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		var vec []float32
		switch {
		case contains(req.Input, "A"):
			vec = []float32{1, 0, 0, 0}
		case contains(req.Input, "B"):
			vec = []float32{0, 1, 0, 0}
		default:
			vec = []float32{0, 0, 0, 0}
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": vec, "index": 0},
			},
		})
	}))
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestEmbedder_EmbedsMissingEntities(t *testing.T) {
	var calls atomic.Int64
	ts := fakeEmbedServer(t, &calls)
	defer ts.Close()

	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	// Seed 2 entities, no embeddings.
	_, _ = store.db.Exec(`
		INSERT INTO entities(name, type, description, updated_at) VALUES
			('A','PERSON','',1),
			('B','PERSON','',2)
	`)

	e := NewEmbedder(store, newEmbedClient(ts.URL, ""), EmbedderConfig{
		Model:        "test-model",
		PollInterval: 20 * time.Millisecond,
		BatchSize:    10,
		CallTimeout:  2 * time.Second,
	}, nil, newSemanticCache())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go e.Run(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings`).Scan(&n)
		if n >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	_ = e.Close(context.Background())

	var n int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings`).Scan(&n)
	if n != 2 {
		t.Errorf("embeddings count = %d, want 2", n)
	}
}

func TestEmbedder_SkipsAlreadyEmbeddedForCurrentModel(t *testing.T) {
	var calls atomic.Int64
	ts := fakeEmbedServer(t, &calls)
	defer ts.Close()

	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	_, _ = store.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('A','PERSON',1)`)
	var id int64
	_ = store.db.QueryRow(`SELECT id FROM entities WHERE name='A'`).Scan(&id)
	vec := []float32{1, 0, 0, 0}
	_, _ = store.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'test-model', 4, ?, 1)`,
		id, encodeFloat32LE(vec))

	e := NewEmbedder(store, newEmbedClient(ts.URL, ""), EmbedderConfig{
		Model:        "test-model",
		PollInterval: 20 * time.Millisecond,
		BatchSize:    10,
		CallTimeout:  2 * time.Second,
	}, nil, newSemanticCache())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go e.Run(ctx)
	<-ctx.Done()
	_ = e.Close(context.Background())

	// No new embedding calls (entity already covered for this model).
	if calls.Load() != 0 {
		t.Errorf("embed calls = %d, want 0 (already embedded for current model)", calls.Load())
	}
}

func TestEmbedder_ReplacesOnModelChange(t *testing.T) {
	var calls atomic.Int64
	ts := fakeEmbedServer(t, &calls)
	defer ts.Close()

	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	_, _ = store.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('A','PERSON',1)`)
	var id int64
	_ = store.db.QueryRow(`SELECT id FROM entities WHERE name='A'`).Scan(&id)
	// Pre-populate with a different model's embedding.
	_, _ = store.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'OLD-model', 4, ?, 1)`,
		id, encodeFloat32LE([]float32{0, 0, 1, 0}))

	e := NewEmbedder(store, newEmbedClient(ts.URL, ""), EmbedderConfig{
		Model:        "test-model",
		PollInterval: 20 * time.Millisecond,
		BatchSize:    10,
		CallTimeout:  2 * time.Second,
	}, nil, newSemanticCache())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go e.Run(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings WHERE entity_id=? AND model='test-model'`, id).Scan(&n)
		if n == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	_ = e.Close(context.Background())

	var n int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings WHERE entity_id = ?`, id).Scan(&n)
	if n != 1 {
		t.Errorf("after model switch, entity has %d embedding rows; want 1 (REPLACE)", n)
	}
	var model string
	_ = store.db.QueryRow(`SELECT model FROM entity_embeddings WHERE entity_id = ?`, id).Scan(&model)
	if model != "test-model" {
		t.Errorf("model = %q, want test-model", model)
	}
}

func TestEmbedder_NormalizesBeforeStorage(t *testing.T) {
	// Fake server returns a non-unit vector; the embedder must L2-normalize
	// before storage so semanticSeeds can use dot product directly.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{3, 4, 0, 0}, "index": 0}, // magnitude 5
			},
		})
	}))
	defer ts.Close()

	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	_, _ = store.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('A','PERSON',1)`)
	var id int64
	_ = store.db.QueryRow(`SELECT id FROM entities WHERE name='A'`).Scan(&id)

	e := NewEmbedder(store, newEmbedClient(ts.URL, ""), EmbedderConfig{
		Model:        "test-model",
		PollInterval: 20 * time.Millisecond,
		BatchSize:    1,
		CallTimeout:  2 * time.Second,
	}, nil, newSemanticCache())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go e.Run(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings`).Scan(&n)
		if n >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	_ = e.Close(context.Background())

	var blob []byte
	_ = store.db.QueryRow(`SELECT vec FROM entity_embeddings WHERE entity_id=?`, id).Scan(&blob)
	stored, _ := decodeFloat32LE(blob)
	// Should be [0.6, 0.8, 0, 0] — (3, 4) / 5.
	if len(stored) != 4 {
		t.Fatalf("len = %d, want 4", len(stored))
	}
	if abs32(stored[0]-0.6) > 1e-5 || abs32(stored[1]-0.8) > 1e-5 {
		t.Errorf("stored = %v, want [0.6, 0.8, 0, 0] (normalized)", stored)
	}
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func TestEmbedder_CloseIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	e := NewEmbedder(store, newEmbedClient("http://127.0.0.1:1", ""), EmbedderConfig{}, nil, newSemanticCache())

	if err := e.Close(context.Background()); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := e.Close(context.Background()); err != nil {
		t.Errorf("double Close: %v", err)
	}
}

func TestEmbedder_RunExitsOnCtxCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, _ := OpenSqlite(path, 0, nil)
	defer store.Close(context.Background())

	e := NewEmbedder(store, newEmbedClient("http://127.0.0.1:1", ""), EmbedderConfig{
		PollInterval: 50 * time.Millisecond,
	}, nil, newSemanticCache())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit within 2s of cancel")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run TestEmbedder_ 2>&1 | head -5
```

Expected: `undefined: NewEmbedder`, `undefined: EmbedderConfig`.

- [ ] **Step 3: Write `embedder.go`**

Create `gormes/internal/memory/embedder.go`:

```go
package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// EmbedderConfig controls the background embedder's polling + call
// behavior. Zero values fall back to sensible defaults.
type EmbedderConfig struct {
	Model        string        // empty = no-op worker
	PollInterval time.Duration // default 30s
	BatchSize    int           // default 10
	CallTimeout  time.Duration // default 10s per /v1/embeddings call
}

func (c *EmbedderConfig) withDefaults() {
	if c.PollInterval <= 0 {
		c.PollInterval = 30 * time.Second
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 10
	}
	if c.CallTimeout <= 0 {
		c.CallTimeout = 10 * time.Second
	}
}

// Embedder is the Phase-3.D background worker that populates
// entity_embeddings. Co-located with extractor — both share the
// single-writer *sql.DB pool.
type Embedder struct {
	store *SqliteStore
	ec    *embedClient
	cfg   EmbedderConfig
	log   *slog.Logger
	cache *semanticCache

	done      chan struct{}
	closeOnce sync.Once
	running   atomic.Bool
}

// NewEmbedder constructs an Embedder. Pass the same *semanticCache the
// recall Provider uses so bumps land in one place.
func NewEmbedder(
	s *SqliteStore,
	ec *embedClient,
	cfg EmbedderConfig,
	log *slog.Logger,
	cache *semanticCache,
) *Embedder {
	cfg.withDefaults()
	if log == nil {
		log = slog.Default()
	}
	return &Embedder{
		store: s,
		ec:    ec,
		cfg:   cfg,
		log:   log,
		cache: cache,
		done:  make(chan struct{}, 1),
	}
}

// Run blocks until ctx is cancelled. No-op if cfg.Model is empty.
func (e *Embedder) Run(ctx context.Context) {
	e.running.Store(true)
	defer func() {
		select {
		case e.done <- struct{}{}:
		default:
		}
	}()
	if e.cfg.Model == "" {
		<-ctx.Done()
		return
	}

	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.loopOnce(ctx)
		}
	}
}

// Close waits for Run to exit if Run is executing; no-op if Run never
// started. Bounded by ctx. Idempotent.
func (e *Embedder) Close(ctx context.Context) error {
	e.closeOnce.Do(func() {
		if !e.running.Load() {
			return
		}
		select {
		case <-e.done:
		case <-ctx.Done():
		}
	})
	return nil
}

type embedderRow struct {
	ID          int64
	Name        string
	Type        string
	Description string
}

func (e *Embedder) loopOnce(ctx context.Context) {
	batch, err := e.pollMissing(ctx)
	if err != nil {
		e.log.Warn("embedder: poll failed", "err", err)
		return
	}
	if len(batch) == 0 {
		return
	}
	for _, row := range batch {
		if err := e.embedAndStore(ctx, row); err != nil {
			e.log.Warn("embedder: per-entity failure",
				"entity_id", row.ID, "name", row.Name, "err", err)
			// Keep going — one bad entity shouldn't block the batch.
		}
	}
}

// pollMissing finds up to BatchSize entities that lack an embedding for
// the current model.
func (e *Embedder) pollMissing(ctx context.Context) ([]embedderRow, error) {
	rows, err := e.store.db.QueryContext(ctx, `
		SELECT e.id, e.name, e.type, COALESCE(e.description, '')
		FROM entities e
		LEFT JOIN entity_embeddings ee
		    ON ee.entity_id = e.id AND ee.model = ?
		WHERE ee.entity_id IS NULL
		ORDER BY e.updated_at DESC
		LIMIT ?`,
		e.cfg.Model, e.cfg.BatchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []embedderRow
	for rows.Next() {
		var r embedderRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Description); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// buildEmbedInput assembles the labeled template per spec §6.3:
//   Entity: {Name}. Type: {Type}. Context: {Description}
// When Description is empty, the "Context: ..." clause is omitted
// entirely (never emit "Context: " with nothing after).
func buildEmbedInput(row embedderRow) string {
	var b strings.Builder
	b.WriteString("Entity: ")
	b.WriteString(sanitizeFenceContent(row.Name))
	b.WriteString(". Type: ")
	b.WriteString(sanitizeFenceContent(row.Type))
	b.WriteString(".")
	desc := sanitizeFenceContent(row.Description)
	if desc != "" {
		b.WriteString(" Context: ")
		b.WriteString(desc)
	}
	return b.String()
}

// embedAndStore calls the embed client for ONE entity and INSERTs the
// result into entity_embeddings via ON CONFLICT DO UPDATE (same entity,
// new model or refresh → REPLACE the row cleanly).
func (e *Embedder) embedAndStore(ctx context.Context, row embedderRow) error {
	callCtx, cancel := context.WithTimeout(ctx, e.cfg.CallTimeout)
	defer cancel()

	input := buildEmbedInput(row)
	vec, err := e.ec.Embed(callCtx, e.cfg.Model, input)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if len(vec) == 0 {
		return fmt.Errorf("embed: empty vector")
	}
	l2Normalize(vec)
	blob := encodeFloat32LE(vec)

	_, err = e.store.db.ExecContext(ctx, `
		INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at)
		VALUES(?, ?, ?, ?, strftime('%s','now'))
		ON CONFLICT(entity_id) DO UPDATE SET
			model = excluded.model,
			dim = excluded.dim,
			vec = excluded.vec,
			updated_at = excluded.updated_at`,
		row.ID, e.cfg.Model, len(vec), blob)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	e.cache.bump()
	return nil
}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run TestEmbedder_ -v -timeout 30s
go vet ./...
```

All 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/memory/embedder.go gormes/internal/memory/embedder_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): Embedder background worker

Embedder is the Phase-3.D background goroutine that populates
entity_embeddings. Co-located with the Phase-3.B extractor
and sharing the single-writer *sql.DB pool.

Loop:
  1. pollMissing — LEFT JOIN entity_embeddings filtered to
     cfg.Model; rows where the join produces NULL = needs
     embedding. Limited to BatchSize per tick.
  2. For each row: build the labeled template
     "Entity: {Name}. Type: {Type}. Context: {Description}"
     (Context clause omitted on empty description per spec
     §6.3), call /v1/embeddings via embedClient, L2-normalize
     the result, INSERT OR REPLACE.
  3. Bump the semantic cache so the recall provider sees the
     new embedding on the next query.

Defaults: PollInterval=30s (less aggressive than the extractor
at 10s — embedding is eventually-consistent, no rush),
BatchSize=10, CallTimeout=10s per entity.

Per-entity failures are logged WARN but don't stop the batch —
one unreachable Ollama or one weirdly-behaving entity
shouldn't block the worker entirely.

Six tests cover: embedding missing entities, skipping already-
embedded-for-current-model, REPLACE on model switch, L2
normalization before storage (fake server returns [3,4,0,0]
magnitude 5 → stored as [0.6, 0.8, 0, 0]), idempotent Close,
ctx-cancel exit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Hybrid fusion in `Provider.GetContext`

**Files:**
- Modify: `gormes/internal/memory/recall.go`
- Modify: `gormes/internal/memory/recall_test.go`

Extend `RecallConfig` with semantic fields; thread an `*embedClient` + `*semanticCache` into `Provider`; add the semantic branch to `GetContext`.

- [ ] **Step 1: Write failing tests (append to recall_test.go)**

```go
// Append imports if missing: "encoding/json", "net/http", "net/http/httptest".

// stubEmbedServer returns a fixed vector for any input — enough to seed
// the graph with embeddings for hybrid tests.
func stubEmbedServer(t *testing.T, returnVec []float32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": returnVec, "index": 0}},
		})
	}))
}

func TestProvider_SemanticDisabledIsLexicalOnly(t *testing.T) {
	// When SemanticModel is empty, the provider must behave identically
	// to Phase 3.C — no embed calls, no semantic seeds.
	_, p := openProviderWithRichGraph(t)
	// Ensure p.ec is nil / SemanticModel empty; openProviderWithRichGraph
	// sets a default RecallConfig with no semantic fields.

	out := p.GetContext(context.Background(), RecallInput{
		UserMessage: "tell me about AzulVigia",
		ChatKey:     "telegram:42",
	})
	if out == "" {
		t.Fatal("GetContext returned empty; lexical seed should still work")
	}
	// No crash, no panic — good enough.
}

func TestProvider_SemanticSeedsAreUnioned(t *testing.T) {
	// Insert entity "Widget" with NO lexical match in the message, but
	// pre-populate an embedding that matches the query vector. The
	// semantic layer should surface it.
	path := filepath.Join(t.TempDir(), "memory.db")
	s, _ := OpenSqlite(path, 0, nil)
	defer s.Close(context.Background())

	_, _ = s.db.Exec(`INSERT INTO entities(name,type,updated_at) VALUES('Widget','PROJECT',1)`)
	var id int64
	_ = s.db.QueryRow(`SELECT id FROM entities WHERE name='Widget'`).Scan(&id)
	vec := []float32{1, 0, 0, 0}
	_, _ = s.db.Exec(
		`INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, 'stub', 4, ?, 1)`,
		id, encodeFloat32LE(vec))

	// Stub server returns the same vector so cosine(query, entity) == 1.
	ts := stubEmbedServer(t, []float32{1, 0, 0, 0})
	defer ts.Close()

	cache := newSemanticCache()
	ec := newEmbedClient(ts.URL, "")
	p := NewRecall(s, RecallConfig{
		WeightThreshold:      1.0,
		MaxFacts:             10,
		Depth:                2,
		MaxSeeds:             5,
		SemanticModel:        "stub",
		SemanticTopK:         3,
		SemanticMinSimilarity: 0.5,
		QueryEmbedTimeout:    1 * time.Second,
	}, nil)
	p.ec = ec
	p.cache = cache

	out := p.GetContext(context.Background(), RecallInput{
		UserMessage: "tell me about gadgets", // no lexical match
	})
	if !strings.Contains(out, "Widget") {
		t.Errorf("semantic-only path missed Widget; got %q", out)
	}
}

func TestProvider_SemanticFallsThroughOnEmbedFailure(t *testing.T) {
	// Unreachable embed endpoint → lexical-only behavior.
	_, p := openProviderWithRichGraph(t)
	p.ec = newEmbedClient("http://127.0.0.1:1", "")
	p.cache = newSemanticCache()
	// Also set semantic config so GetContext even attempts the call.
	p.cfg.SemanticModel = "unreachable"
	p.cfg.SemanticTopK = 3
	p.cfg.SemanticMinSimilarity = 0.5
	p.cfg.QueryEmbedTimeout = 200 * time.Millisecond

	out := p.GetContext(context.Background(), RecallInput{
		UserMessage: "tell me about AzulVigia",
		ChatKey:     "telegram:42",
	})
	// Lexical still works — the fence includes AzulVigia.
	if !strings.Contains(out, "AzulVigia") {
		t.Errorf("lexical fallback failed when embed endpoint is unreachable: %q", out)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/memory/... -run "TestProvider_Semantic" 2>&1 | head -10
```

Expected: unknown fields on RecallConfig (`SemanticModel`, `SemanticTopK`, etc.) or unknown fields on Provider (`ec`, `cache`).

- [ ] **Step 3: Extend `recall.go`**

In `gormes/internal/memory/recall.go`:

1. Extend `RecallConfig` with semantic fields:

```go
type RecallConfig struct {
	WeightThreshold float64
	MaxFacts        int
	Depth           int
	MaxSeeds        int

	// Phase 3.D semantic fusion. All zero / empty = disabled.
	SemanticModel         string        // Ollama embedding model tag
	SemanticTopK          int           // default 3 when <=0 and SemanticModel != ""
	SemanticMinSimilarity float64       // default 0.35 when <=0 and SemanticModel != ""
	QueryEmbedTimeout     time.Duration // default 60ms when <=0 and SemanticModel != ""
}
```

2. Extend `withDefaults` with semantic-only-when-model-set defaults:

```go
func (c *RecallConfig) withDefaults() {
	if c.WeightThreshold <= 0 {
		c.WeightThreshold = 1.0
	}
	if c.MaxFacts <= 0 {
		c.MaxFacts = 10
	}
	if c.Depth <= 0 {
		c.Depth = 2
	}
	if c.MaxSeeds <= 0 {
		c.MaxSeeds = 5
	}
	// Semantic defaults only apply when the feature is opted in.
	if c.SemanticModel != "" {
		if c.SemanticTopK <= 0 {
			c.SemanticTopK = 3
		}
		if c.SemanticMinSimilarity <= 0 {
			c.SemanticMinSimilarity = 0.35
		}
		if c.QueryEmbedTimeout <= 0 {
			c.QueryEmbedTimeout = 60 * time.Millisecond
		}
	}
}
```

(Add `"time"` to the file's imports if not present.)

3. Extend `Provider` with the embed client + cache:

```go
type Provider struct {
	store *SqliteStore
	cfg   RecallConfig
	log   *slog.Logger
	ec    *embedClient    // nil disables semantic recall
	cache *semanticCache  // shared with the Embedder; always non-nil for consistency
}
```

4. Extend `NewRecall` with the default cache:

```go
func NewRecall(s *SqliteStore, cfg RecallConfig, log *slog.Logger) *Provider {
	cfg.withDefaults()
	if log == nil {
		log = slog.Default()
	}
	return &Provider{store: s, cfg: cfg, log: log, cache: newSemanticCache()}
}
```

5. Add a setter for the cmd wiring to inject the embed client + shared cache:

```go
// WithEmbedClient attaches the embedding client. Call before Run() or
// any GetContext; not safe for concurrent use with in-flight recalls.
// Pass the same *semanticCache that the Embedder will bump to keep
// both consumers in sync.
func (p *Provider) WithEmbedClient(ec *embedClient, cache *semanticCache) *Provider {
	p.ec = ec
	if cache != nil {
		p.cache = cache
	}
	return p
}
```

6. Extend `GetContext` to add the semantic branch at the end of the seed-collection phase. The diff — add these lines AFTER the existing Layer-2 FTS5 fallback block and BEFORE the `if len(seeds) == 0` check:

```go
	// 3. Semantic fallback — only if enabled AND we still need more seeds.
	if p.ec != nil && p.cfg.SemanticModel != "" && len(seeds) < p.cfg.MaxSeeds {
		semCtx, semCancel := context.WithTimeout(ctx, p.cfg.QueryEmbedTimeout)
		qvec, err := p.ec.Embed(semCtx, p.cfg.SemanticModel, in.UserMessage)
		semCancel()
		if err != nil {
			p.log.Warn("recall: query embed failed", "err", err)
		} else {
			l2Normalize(qvec)
			semIDs, err := semanticSeeds(ctx, p.store.db, p.cache,
				p.cfg.SemanticModel, qvec, p.cfg.SemanticTopK, p.cfg.SemanticMinSimilarity)
			if err != nil {
				p.log.Warn("recall: semantic scan failed", "err", err)
			} else {
				seen := make(map[int64]struct{}, len(seeds))
				for _, id := range seeds {
					seen[id] = struct{}{}
				}
				for _, id := range semIDs {
					if _, dup := seen[id]; !dup {
						seeds = append(seeds, id)
						seen[id] = struct{}{}
					}
					if len(seeds) >= p.cfg.MaxSeeds {
						break
					}
				}
			}
		}
	}
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/memory/... -run "TestProvider_" -v -timeout 30s
go vet ./...
```

All Provider tests PASS (including the 3 new ones and the existing 3.C ones).

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/memory/recall.go gormes/internal/memory/recall_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/memory): Provider hybrid fusion — lexical -> FTS5 -> semantic

Phase 3.D completes the recall pipeline. GetContext now runs
three seed layers sequentially:

  1. Lexical exact-name match (unchanged from 3.C)
  2. FTS5 turns-content fallback (unchanged from 3.C)
  3. Semantic cosine scan — NEW, runs only when:
     - semantic enabled (SemanticModel != "")
     - seeds are still below MaxSeeds
     Embed the user's message via the bound *embedClient
     under QueryEmbedTimeout (default 60ms), scan the
     cached vectors via semanticSeeds, union results into
     the seed set with dedup.

RecallConfig gains:
  SemanticModel         — Ollama tag; "" disables the layer
  SemanticTopK          — default 3
  SemanticMinSimilarity — default 0.35
  QueryEmbedTimeout     — default 60ms

Provider gains private fields `ec *embedClient` and
`cache *semanticCache` — wired via the new
.WithEmbedClient(ec, cache) setter that cmd/gormes/telegram
calls during startup.

Failure chain is strict best-effort: any semantic failure
(timeout, embed HTTP error, cache error) logs WARN and
falls through to whatever lexical+FTS5 produced.

Three tests pin the hybrid behavior:
  - disabled-no-op: empty SemanticModel preserves 3.C lexical
  - semantic-fills: entity with no lexical match surfaces via
    pre-populated embedding with cosine=1
  - unreachable-embed-endpoint: lexical still works when the
    semantic layer blows up

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Config fields — `semantic_*` + `embedder_*`

**Files:**
- Modify: `gormes/internal/config/config.go`
- Modify: `gormes/internal/config/config_test.go`

- [ ] **Step 1: Write failing test**

Append to `gormes/internal/config/config_test.go`:

```go
func TestLoad_SemanticDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Semantic is opt-in: everything off by default.
	if cfg.Telegram.SemanticEnabled {
		t.Errorf("SemanticEnabled default = true, want false (opt-in)")
	}
	if cfg.Telegram.SemanticModel != "" {
		t.Errorf("SemanticModel default = %q, want empty", cfg.Telegram.SemanticModel)
	}
	// But tunables have usable defaults so a single `semantic_enabled = true`
	// + `semantic_model = "..."` in TOML is enough to light things up.
	if cfg.Telegram.SemanticTopK != 3 {
		t.Errorf("SemanticTopK default = %d, want 3", cfg.Telegram.SemanticTopK)
	}
	if cfg.Telegram.SemanticMinSimilarity != 0.35 {
		t.Errorf("SemanticMinSimilarity default = %v, want 0.35", cfg.Telegram.SemanticMinSimilarity)
	}
	if cfg.Telegram.EmbedderPollInterval != 30*time.Second {
		t.Errorf("EmbedderPollInterval default = %v, want 30s", cfg.Telegram.EmbedderPollInterval)
	}
	if cfg.Telegram.EmbedderBatchSize != 10 {
		t.Errorf("EmbedderBatchSize default = %d, want 10", cfg.Telegram.EmbedderBatchSize)
	}
	if cfg.Telegram.EmbedderCallTimeout != 10*time.Second {
		t.Errorf("EmbedderCallTimeout default = %v, want 10s", cfg.Telegram.EmbedderCallTimeout)
	}
	if cfg.Telegram.QueryEmbedTimeout != 60*time.Millisecond {
		t.Errorf("QueryEmbedTimeout default = %v, want 60ms", cfg.Telegram.QueryEmbedTimeout)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd gormes
go test ./internal/config/... -run TestLoad_SemanticDefaults 2>&1 | head -5
```

Expected: `unknown field SemanticEnabled` etc.

- [ ] **Step 3: Extend `config.go`**

Add to `TelegramCfg` (preserve existing fields):

```go
	// Phase 3.D semantic fusion — all opt-in via SemanticEnabled.
	SemanticEnabled       bool          `toml:"semantic_enabled"`
	SemanticEndpoint      string        `toml:"semantic_endpoint"`
	SemanticModel         string        `toml:"semantic_model"`
	SemanticTopK          int           `toml:"semantic_top_k"`
	SemanticMinSimilarity float64       `toml:"semantic_min_similarity"`
	EmbedderPollInterval  time.Duration `toml:"embedder_poll_interval"`
	EmbedderBatchSize     int           `toml:"embedder_batch_size"`
	EmbedderCallTimeout   time.Duration `toml:"embedder_call_timeout"`
	QueryEmbedTimeout     time.Duration `toml:"query_embed_timeout"`
```

Extend `defaults()`:

```go
		SemanticEnabled:       false,
		SemanticEndpoint:      "",    // falls back to Hermes.Endpoint at wire time
		SemanticModel:         "",    // operator must name a model to enable
		SemanticTopK:          3,
		SemanticMinSimilarity: 0.35,
		EmbedderPollInterval:  30 * time.Second,
		EmbedderBatchSize:     10,
		EmbedderCallTimeout:   10 * time.Second,
		QueryEmbedTimeout:     60 * time.Millisecond,
```

- [ ] **Step 4: Run, expect PASS**

```bash
cd gormes
go test -race ./internal/config/... -v
```

- [ ] **Step 5: Commit**

```bash
git add gormes/internal/config/config.go gormes/internal/config/config_test.go
git commit -m "$(cat <<'EOF'
feat(gormes/config): Phase-3.D semantic + embedder TOML knobs

Nine new TelegramCfg fields, all TOML-only (no env / flag):

  semantic_enabled          default false (opt-in)
  semantic_endpoint         default "" (falls back to
                              Hermes.Endpoint at wire time)
  semantic_model            default "" (operator must set to
                              activate the feature)
  semantic_top_k            default 3
  semantic_min_similarity   default 0.35
  embedder_poll_interval    default 30s
  embedder_batch_size       default 10
  embedder_call_timeout     default 10s
  query_embed_timeout       default 60ms

Opt-in discipline: empty SemanticModel keeps the feature
completely inert — no background goroutine, no network
calls to Ollama's /v1/embeddings, no RAM for the cache.
Users who haven't pulled an embedding model see zero
behavior change.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `cmd/gormes/telegram.go` wiring

**Files:**
- Modify: `gormes/cmd/gormes/telegram.go`

- [ ] **Step 1: Read the current file's kernel + recall construction section**

```bash
cat gormes/cmd/gormes/telegram.go | tail -80
```

Locate the existing block that constructs `memory.NewRecall(...)` + `recallAdapter` + `kernel.Config.Recall` (added in Phase 3.C T9).

- [ ] **Step 2: Extend it to construct the Embedder when semantic is enabled**

Before the existing `memory.NewRecall(...)` call, insert:

```go
	// Phase 3.D — semantic fusion wiring. Activated only when both the
	// feature flag is true AND an embedding model is named.
	var semCache *memory.SemanticCache // nil disables the Embedder
	var ec *memory.EmbedClient
	if cfg.Telegram.RecallEnabled && cfg.Telegram.AllowedChatID != 0 &&
		cfg.Telegram.SemanticEnabled && cfg.Telegram.SemanticModel != "" {
		endpoint := cfg.Telegram.SemanticEndpoint
		if endpoint == "" {
			endpoint = cfg.Hermes.Endpoint
		}
		ec = memory.NewEmbedClient(endpoint, cfg.Hermes.APIKey)
		semCache = memory.NewSemanticCache()
	}
```

**NOTE:** `newEmbedClient` and `newSemanticCache` are lowercase (package-private) in Tasks 2 and 4. The cmd package can't call them directly. We need to export lightweight wrappers. Update `gormes/internal/memory/embed_client.go` to also expose:

```go
// SemanticCache is the exported alias for the internal cache type.
// Exposed so cmd packages can construct one and pass it into both
// NewRecall's WithEmbedClient and NewEmbedder.
type SemanticCache = semanticCache

// EmbedClient is the exported alias for cmd use.
type EmbedClient = embedClient

// NewEmbedClient is the exported constructor.
func NewEmbedClient(baseURL, apiKey string) *EmbedClient {
	return newEmbedClient(baseURL, apiKey)
}

// NewSemanticCache is the exported constructor.
func NewSemanticCache() *SemanticCache {
	return newSemanticCache()
}
```

Add these to `embed_client.go` and update the test file (`embed_client_test.go`) to work with the exported names if needed.

Now modify the existing `memory.NewRecall(...)` block:

```go
		memProv := memory.NewRecall(mstore, memory.RecallConfig{
			WeightThreshold:       cfg.Telegram.RecallWeightThreshold,
			MaxFacts:              cfg.Telegram.RecallMaxFacts,
			Depth:                 cfg.Telegram.RecallDepth,
			SemanticModel:         cfg.Telegram.SemanticModel,
			SemanticTopK:          cfg.Telegram.SemanticTopK,
			SemanticMinSimilarity: cfg.Telegram.SemanticMinSimilarity,
			QueryEmbedTimeout:     cfg.Telegram.QueryEmbedTimeout,
		}, slog.Default())
		if ec != nil {
			memProv = memProv.WithEmbedClient(ec, semCache)
		}
		recallProv = &recallAdapter{p: memProv}
```

After the kernel is constructed and just before or after the existing `go k.Run(rootCtx)`, launch the Embedder if configured:

```go
	if ec != nil {
		embedder := memory.NewEmbedder(mstore, ec, memory.EmbedderConfig{
			Model:        cfg.Telegram.SemanticModel,
			PollInterval: cfg.Telegram.EmbedderPollInterval,
			BatchSize:    cfg.Telegram.EmbedderBatchSize,
			CallTimeout:  cfg.Telegram.EmbedderCallTimeout,
		}, slog.Default(), semCache)
		go embedder.Run(rootCtx)
		defer func() {
			shutdownCtx, cancelSd := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
			defer cancelSd()
			if err := embedder.Close(shutdownCtx); err != nil {
				slog.Warn("embedder close", "err", err)
			}
		}()
	}
```

- [ ] **Step 3: Build + smoke**

```bash
cd gormes
go build ./...
go vet ./...
make build
ls -lh bin/gormes
```

Expected: clean build; `bin/gormes` size ≤ 18 MB (pure-Go additions).

```bash
cd gormes
./bin/gormes telegram 2>&1 | head -3
```

Expected: `Error: no Telegram bot token — ...` (unchanged).

Full sweep (skip Ollama integration):
```bash
cd gormes
go test -race ./... -count=1 -timeout 240s -skip Integration_Ollama
```

Green.

- [ ] **Step 4: Commit**

```bash
git add gormes/cmd/gormes/telegram.go gormes/internal/memory/embed_client.go
git commit -m "$(cat <<'EOF'
feat(gormes/cmd/telegram): wire Phase-3.D semantic recall + Embedder

cmd/gormes/telegram.go now constructs the semantic stack
when BOTH:
  - Recall is enabled (3.C flag)
  - SemanticEnabled is true (3.D flag)
  - SemanticModel is non-empty

Wiring order:
  1. Create *memory.EmbedClient pointing at
     cfg.Telegram.SemanticEndpoint (falls back to Hermes
     endpoint — Ollama usually hosts both).
  2. Create a shared *memory.SemanticCache.
  3. memory.NewRecall(...).WithEmbedClient(ec, cache) so the
     recall provider uses the semantic layer.
  4. memory.NewEmbedder(... bound to the SAME cache ...)
     so bumps land in one place.
  5. go embedder.Run(rootCtx); defer embedder.Close(...)
     alongside the extractor.

internal/memory/embed_client.go exports SemanticCache,
EmbedClient type aliases + NewEmbedClient / NewSemanticCache
constructors so cmd packages can instantiate them without
breaking the internal-lowercase discipline of the package's
private types.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Ollama E2E integration test

**Files:**
- Create: `gormes/internal/memory/semantic_integration_test.go`

- [ ] **Step 1: Write the test**

Create `gormes/internal/memory/semantic_integration_test.go`:

```go
// Package memory — Phase 3.D semantic recall crucible against local Ollama.
//
// Gated by the same skipIfNoOllama helper from extractor_integration_test.go.
// Proves the gap closure: a query that lexically matches nothing
// ("tell me about my projects") nevertheless surfaces AzulVigia via
// the semantic seed layer.
//
// Skips cleanly if Ollama or the chosen embedding model aren't available.
// Environment:
//   GORMES_EXTRACTOR_ENDPOINT  (default http://localhost:11434)
//   GORMES_EXTRACTOR_MODEL     (chat model for extractor; see 3.B)
//   GORMES_SEMANTIC_MODEL      (embedding model; default nomic-embed-text)
package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/hermes"
)

func semanticModel() string {
	if v := os.Getenv("GORMES_SEMANTIC_MODEL"); v != "" {
		return v
	}
	return "nomic-embed-text"
}

func skipIfNoEmbeddingModel(t *testing.T) {
	t.Helper()
	// Reuse skipIfNoOllama for base reachability, then probe the embed
	// endpoint specifically.
	skipIfNoOllama(t)
	ec := newEmbedClient(integrationEndpoint(), "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := ec.Embed(ctx, semanticModel(), "probe")
	if err != nil {
		t.Skipf("embedding model %q not available at %s: %v\n"+
			"  Pull with: ollama pull %s",
			semanticModel(), integrationEndpoint(), err, semanticModel())
	}
}

func TestRecall_Integration_Ollama_MyProjectsFindsAzulVigia(t *testing.T) {
	skipIfNoEmbeddingModel(t)

	endpoint := integrationEndpoint()
	chatModel := integrationModel()
	embedModel := semanticModel()
	t.Logf("=== 3.D crucible: extractor=%s, embedder=%s @ %s ===",
		chatModel, embedModel, endpoint)

	path := filepath.Join(t.TempDir(), "semantic.db")
	store, err := OpenSqlite(path, 0, nil)
	if err != nil {
		t.Fatalf("OpenSqlite: %v", err)
	}
	defer store.Close(context.Background())

	// ── Seed 3 entity-rich turns ──────────────────────────────────────
	turns := []string{
		"I am setting up the AzulVigia project in Cadereyta.",
		"Vania is helping me test the Neovim configuration.",
		"Juan works on the Go backend of AzulVigia every day.",
	}
	for i, content := range turns {
		_, err := store.db.Exec(
			`INSERT INTO turns(session_id, role, content, ts_unix, chat_id)
			 VALUES(?, 'user', ?, ?, ?)`,
			"sem-session", content, time.Now().Unix()+int64(i), "telegram:42")
		if err != nil {
			t.Fatal(err)
		}
	}

	// ── Phase A: run extractor to populate entities ───────────────────
	hc := hermes.NewHTTPClient(endpoint, "")
	ext := NewExtractor(store, hc, ExtractorConfig{
		Model:        chatModel,
		PollInterval: 500 * time.Millisecond,
		BatchSize:    3,
		CallTimeout:  180 * time.Second,
	}, nil)
	extCtx, extCancel := context.WithTimeout(context.Background(), 4*time.Minute)
	go ext.Run(extCtx)
	for extCtx.Err() == nil {
		var n int
		_ = store.db.QueryRow(`SELECT COUNT(*) FROM turns WHERE extracted = 0`).Scan(&n)
		if n == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	extCancel()
	_ = ext.Close(context.Background())

	var entCount int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM entities`).Scan(&entCount)
	t.Logf("extractor populated %d entities", entCount)
	if entCount == 0 {
		t.Fatal("no entities extracted — cannot proceed")
	}

	// ── Phase B: run embedder to populate entity_embeddings ───────────
	cache := newSemanticCache()
	ec := newEmbedClient(endpoint, "")
	embedder := NewEmbedder(store, ec, EmbedderConfig{
		Model:        embedModel,
		PollInterval: 500 * time.Millisecond,
		BatchSize:    10,
		CallTimeout:  30 * time.Second,
	}, nil, cache)
	embCtx, embCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	go embedder.Run(embCtx)
	for embCtx.Err() == nil {
		var n int
		_ = store.db.QueryRow(
			`SELECT COUNT(*) FROM entities e
			 LEFT JOIN entity_embeddings ee ON ee.entity_id = e.id AND ee.model = ?
			 WHERE ee.entity_id IS NULL`, embedModel).Scan(&n)
		if n == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	embCancel()
	_ = embedder.Close(context.Background())

	var embCount int
	_ = store.db.QueryRow(`SELECT COUNT(*) FROM entity_embeddings`).Scan(&embCount)
	t.Logf("embedder populated %d embeddings", embCount)

	// ── Phase C: run recall with "tell me about my projects" ──────────
	prov := NewRecall(store, RecallConfig{
		WeightThreshold:       1.0,
		MaxFacts:              10,
		Depth:                 2,
		MaxSeeds:              5,
		SemanticModel:         embedModel,
		SemanticTopK:          3,
		SemanticMinSimilarity: 0.35,
		QueryEmbedTimeout:     5 * time.Second, // generous for the crucible
	}, nil).WithEmbedClient(ec, cache)

	recallCtx, recallCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer recallCancel()
	block := prov.GetContext(recallCtx, RecallInput{
		UserMessage: "tell me about my projects",
		ChatKey:     "telegram:42",
	})

	t.Logf("=== FENCE FOR 'tell me about my projects' ===")
	if block == "" {
		t.Logf("  (empty — semantic layer did not bridge the gap)")
	} else {
		for _, line := range strings.Split(block, "\n") {
			t.Logf("  %s", line)
		}
	}
	t.Logf("=== END FENCE ===")

	fmt.Printf("\n[3.D] memory.db: %s\n", path)
	fmt.Printf("[3.D] chat_model=%s embed_model=%s entities=%d embeddings=%d\n\n",
		chatModel, embedModel, entCount, embCount)

	// Core assertion: the fence for the non-lexical query contains AzulVigia.
	// This is the Phase 3.D ship criterion.
	if block == "" {
		t.Errorf("block empty for non-lexical query — semantic layer didn't fire")
	}
	if !strings.Contains(block, "AzulVigia") {
		t.Errorf("fence missing AzulVigia on non-lexical query; got %q", block)
	}
}
```

- [ ] **Step 2: Run (skips if no embedding model)**

```bash
cd gormes
GORMES_EXTRACTOR_MODEL="huggingface.co/r1r21nb/qwen2.5-3b-instruct.Q4_K_M.gguf:latest" \
GORMES_SEMANTIC_MODEL="nomic-embed-text" \
  go test ./internal/memory/... -run TestRecall_Integration_Ollama_MyProjects -v -timeout 10m
```

If the embed model isn't pulled, test skips. If it IS pulled and everything works, test PASSes and the fence telemetry is logged.

- [ ] **Step 3: Commit**

```bash
git add gormes/internal/memory/semantic_integration_test.go
git commit -m "$(cat <<'EOF'
test(gormes/memory): Phase-3.D semantic recall crucible against Ollama

TestRecall_Integration_Ollama_MyProjectsFindsAzulVigia is
the ship criterion for Phase 3.D. Flow:

  Phase A: seed 3 entity-rich turns (AzulVigia/Cadereyta/
           Vania/Juan/Go) and run the real 3.B extractor
           against Ollama until all turns are extracted.
  Phase B: run the 3.D Embedder with GORMES_SEMANTIC_MODEL
           (default nomic-embed-text) until every entity
           has an embedding for that model.
  Phase C: call Provider.GetContext("tell me about my
           projects") — a query that lexically matches NO
           entity name.
  Assert: the fence contains "AzulVigia".

This is the gap 3.C could NOT close: exact-name + FTS5 both
return empty for "my projects" → "AzulVigia". Semantic
cosine similarity bridges that distance.

skipIfNoEmbeddingModel reuses skipIfNoOllama and adds an
embed-endpoint probe so the test skips cleanly if the user
hasn't run `ollama pull nomic-embed-text`.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Verification sweep

**Files:** no changes — verification only.

- [ ] **Step 1: Full sweep under -race (skip Ollama)**

```bash
cd gormes
go test -race ./... -count=1 -timeout 240s -skip Integration_Ollama
go vet ./...
```

Expected: all packages green (except the pre-existing `docs` README text check unrelated to this phase). Vet clean.

- [ ] **Step 2: Binary size**

```bash
cd gormes
make build
ls -lh bin/gormes
```

Expected: `bin/gormes` ≤ 18 MB (pure-Go additions, expected growth <250 KB).

- [ ] **Step 3: Kernel isolation**

```bash
cd gormes
(go list -deps ./internal/kernel | grep -E "ncruces|internal/memory|internal/session") \
  && echo "VIOLATION" || echo "OK: kernel isolated from memory"
```

Expected: `OK`. Kernel's T12 isolation invariant holds.

- [ ] **Step 4: Schema migration smoke (v3c → v3d)**

```bash
cd gormes
rm -rf /tmp/gormes-3d-migrate && mkdir -p /tmp/gormes-3d-migrate/gormes
sqlite3 /tmp/gormes-3d-migrate/gormes/memory.db <<'SQL'
CREATE TABLE schema_meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
INSERT INTO schema_meta(k,v) VALUES ('version','3c');
CREATE TABLE turns (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, role TEXT NOT NULL CHECK(role IN ('user','assistant')), content TEXT NOT NULL, ts_unix INTEGER NOT NULL, meta_json TEXT, extracted INTEGER NOT NULL DEFAULT 0, extraction_attempts INTEGER NOT NULL DEFAULT 0, extraction_error TEXT, chat_id TEXT NOT NULL DEFAULT '');
CREATE INDEX idx_turns_session_ts ON turns(session_id, ts_unix);
CREATE INDEX idx_turns_unextracted ON turns(id) WHERE extracted = 0;
CREATE INDEX idx_turns_chat_id ON turns(chat_id, id);
CREATE VIRTUAL TABLE turns_fts USING fts5(content, content='turns', content_rowid='id');
CREATE TABLE entities (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL CHECK(type IN ('PERSON','PROJECT','CONCEPT','PLACE','ORGANIZATION','TOOL','OTHER')), description TEXT, updated_at INTEGER NOT NULL, UNIQUE(name, type));
CREATE TABLE relationships (source_id INTEGER NOT NULL, target_id INTEGER NOT NULL, predicate TEXT NOT NULL CHECK(predicate IN ('WORKS_ON','KNOWS','LIKES','DISLIKES','HAS_SKILL','LOCATED_IN','PART_OF','RELATED_TO')), weight REAL NOT NULL DEFAULT 1.0, updated_at INTEGER NOT NULL, PRIMARY KEY(source_id, target_id, predicate), FOREIGN KEY(source_id) REFERENCES entities(id) ON DELETE CASCADE, FOREIGN KEY(target_id) REFERENCES entities(id) ON DELETE CASCADE);
INSERT INTO entities(name,type,updated_at) VALUES('pre-3d entity','PERSON',1);
SQL
echo "BEFORE v=$(sqlite3 /tmp/gormes-3d-migrate/gormes/memory.db 'SELECT v FROM schema_meta') entity_embeddings: $(sqlite3 /tmp/gormes-3d-migrate/gormes/memory.db "SELECT COUNT(*) FROM sqlite_master WHERE name='entity_embeddings'")"

export XDG_DATA_HOME=/tmp/gormes-3d-migrate
GORMES_TELEGRAM_TOKEN=fake:tok GORMES_TELEGRAM_CHAT_ID=99 \
  timeout 1 ./bin/gormes telegram > /dev/null 2>&1 || true

echo "AFTER  v=$(sqlite3 /tmp/gormes-3d-migrate/gormes/memory.db 'SELECT v FROM schema_meta') entity_embeddings: $(sqlite3 /tmp/gormes-3d-migrate/gormes/memory.db "SELECT COUNT(*) FROM sqlite_master WHERE name='entity_embeddings'") pre_existing_entity: $(sqlite3 /tmp/gormes-3d-migrate/gormes/memory.db "SELECT COUNT(*) FROM entities WHERE name='pre-3d entity'")"
rm -rf /tmp/gormes-3d-migrate
```

Expected: `BEFORE v=3c entity_embeddings: 0 ... AFTER v=3d entity_embeddings: 1 pre_existing_entity: 1`.

- [ ] **Step 5: Offline doctor still works**

```bash
cd gormes
./bin/gormes doctor --offline
```

Expected: `[PASS] Toolbox: 3 tools registered (echo, now, rand_int)`.

- [ ] **Step 6: Live Ollama E2E (optional, with embedding model pulled)**

```bash
cd gormes
# Requires: ollama pull nomic-embed-text
GORMES_EXTRACTOR_MODEL="huggingface.co/r1r21nb/qwen2.5-3b-instruct.Q4_K_M.gguf:latest" \
GORMES_SEMANTIC_MODEL="nomic-embed-text" \
  go test ./internal/memory/... -run TestRecall_Integration_Ollama_MyProjects -v -timeout 10m
```

Expected: test PASSES with telemetry showing the fence contains "AzulVigia" from a non-lexical query.

- [ ] **Step 7: No commit**

If any check fails, STOP and report.

---

## Appendix: Self-Review

**Spec coverage** (mapping each §X of the spec to its implementing task):

| Spec § | Task(s) |
|---|---|
| §1 Goal | All tasks |
| §2 Non-goals | Enforced by scope — no decay/cross-chat/ANN tasks |
| §3 Scope | T1-T9 |
| §4 Ollama embedding integration | T2 |
| §5 Schema v3d | T1 |
| §6 Embedder worker | T5 (including buildEmbedInput §6.3 template) |
| §7 Similarity scan in Go | T3 + T4 |
| §8 Hybrid fusion | T6 |
| §9 Kernel unchanged | Verified by lack of changes to kernel/* in T1-T10 |
| §10 Configuration | T7 |
| §11 Error handling | Distributed: T2 (embed errors), T5 (per-entity failure tolerance), T6 (best-effort fallthrough) |
| §12 Security | T2 (no vector logging), T5 (no raw user content in embedder logs) |
| §13 Testing | All tasks include unit tests; T9 is integration |
| §14 Binary budget | T10 verification |
| §15 Out of scope | No tasks (correct) |
| §16 Rollout | T1 idempotent migration; T7 opt-in default |

**Placeholder scan:** zero `TBD` / `TODO` / `fill in` / vague "handle errors" / "similar to Task N".

**Type consistency:**
- `EmbedderConfig`, `NewEmbedder`, `*Embedder.Run/Close` — T5.
- `embedClient` (private) + `EmbedClient`/`SemanticCache`/`NewEmbedClient`/`NewSemanticCache` aliases (exported in T8) — T2 + T8.
- `semanticCache`, `newSemanticCache`, `.bump()`, `.ensureLoaded()` — T4.
- `scoredID{ID, Score}`, `l2Normalize`, `dotProduct`, `topK`, `encodeFloat32LE`, `decodeFloat32LE` — T3; consumed T4 + T5.
- `semanticSeeds(ctx, db, cache, model, qvec, topK, minSim)` — T4; consumed T6.
- `RecallConfig.SemanticModel/TopK/MinSimilarity/QueryEmbedTimeout` — T6 declaration; T7 config mapping; T8 cmd wiring.
- `TelegramCfg.SemanticEnabled/Endpoint/Model/TopK/MinSimilarity/EmbedderPollInterval/BatchSize/CallTimeout/QueryEmbedTimeout` — T7.
- `Provider.WithEmbedClient(ec, cache)` setter — T6 declaration; T8 consumer.
- `buildEmbedInput` template `"Entity: X. Type: T. Context: D"` with Context clause omitted when description empty — T5, matching spec §6.3 (post-revision template with labeled `Entity:`/`Type:`/`Context:` labels).

**Execution order:** linear — T1 (schema) → T2 (client) → T3 (math) → T4 (scan) → T5 (worker) → T6 (Provider integration) → T7 (config) → T8 (cmd) → T9 (crucible) → T10 (verification).

**Checkpoint suggestions:** halt after **T6** (Provider hybrid fusion verified via unit tests, semantic layer provably optional) and after **T9** (Ollama run proves "my projects" → AzulVigia closure) before T10's final sweep.
