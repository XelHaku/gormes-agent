# Gormes Phase 3.D — Semantic Fusion & Local Embeddings Design

**Status:** Approved 2026-04-20 · implementation plan pending
**Depends on:** Phase 3.C (Neural Recall) green on `main`

## Related Documents

- [`gormes/docs/ARCH_PLAN.md`](../../ARCH_PLAN.md) — Phase 3 memory trilogy. 3.A = Lattice (SQLite + FTS5). 3.B = Brain (LLM extractor). 3.C = Recall (lexical + FTS5 seed → CTE walk → fence injection). 3.D = **Semantic Fusion** (embeddings + hybrid seed selection).
- Phase 3.C — [`2026-04-20-gormes-phase3c-recall-design.md`](2026-04-20-gormes-phase3c-recall-design.md) — the recall pipeline this spec augments. 3.C's "tell me about my projects" → empty block is the gap 3.D closes.
- Phase 3.B — [`2026-04-20-gormes-phase3b-graph-design.md`](2026-04-20-gormes-phase3b-graph-design.md) — graph schema + async extractor. Embedding worker mirrors the extractor's lifecycle.

---

## 1. Goal

Bridge the lexical gap in Phase 3.C's recall. Today: "tell me about Acme" populates the fence; "tell me about my projects" returns empty. After 3.D: the latter query embeds to a vector, cosine-scans `entity_embeddings`, finds `Acme (PROJECT)` as top-K semantic match, feeds it into the existing Recursive CTE, and the fence comes back populated — with the same 100ms recall deadline respected.

## 2. Non-Goals

- **No decay** — Phase 3.E. Entities don't get forgotten in 3.D regardless of age.
- **No cross-chat synthesis** — Phase 3.E. Per-chat scoping on turns stays as-is.
- **No ANN index** (HNSW, IVF, etc.) — flat cosine scan is fast enough for Gormes's target scale (≤10k entities = ~5ms scan). ANN comes if we ever hit 100k entities.
- **No sqlite-vss / sqlite-vec** — C extensions not available in ncruces WASM. Cosine is computed in Go.
- **No re-vectorization on model change** — if the user switches embedding models, stored vectors become mismatched; the similarity scan skips them silently and the embedder slowly re-populates with the new model. No migration tool.
- **No forced embed-model download** — users can opt into semantic recall with whatever LLM they already have loaded (chat models return pooled hidden-state vectors). Dedicated embedding models (nomic-embed-text, mxbai-embed-large) are RECOMMENDED for quality but NOT REQUIRED.
- **No cloud embeddings** — Gormes stays local-first. The embed model is whichever Ollama instance `GORMES_SEMANTIC_ENDPOINT` points at.

## 3. Scope

1. Schema `v3d` migration: new `entity_embeddings` table.
2. A new `memory.Embedder` background goroutine — polls `entities` for rows lacking a current-model embedding, calls Ollama's `/v1/embeddings`, INSERTs the result.
3. An `embedQuery(userMessage) → []float32` helper that the recall provider uses at query-time, bounded by a hard timeout.
4. A `semanticSeeds(queryVec) → []int64` function that scans the embeddings table and returns top-K entity IDs by cosine similarity.
5. `Provider.GetContext` gains a semantic branch that runs in parallel with (or as fallback to) the existing lexical + FTS5 seeds.
6. Config: `semantic_enabled`, `semantic_model`, `semantic_endpoint`, `semantic_top_k`, `embedder_poll_interval`.
7. Kernel: **no changes**. The recall provider continues to satisfy `kernel.RecallProvider`; all new logic is internal to `memory.Provider.GetContext`.

## 4. Ollama Embedding Integration

### 4.1 Endpoint contract

Ollama's `/v1/embeddings` is OpenAI-compatible. Verified empirically against local Ollama 2026-04-20:

Request:
```json
POST /v1/embeddings
Content-Type: application/json
{"model": "nomic-embed-text", "input": "tell me about my projects"}
```

Response:
```json
{
  "object": "list",
  "data": [{"object": "embedding", "embedding": [0.0172, -0.0208, ...], "index": 0}],
  "model": "nomic-embed-text",
  "usage": {"prompt_tokens": 6, "total_tokens": 6}
}
```

### 4.2 HTTP client

A new narrow method on a local-only embeddings client — NOT added to `hermes.Client` (keeping the kernel's hermes.Client interface focused on chat streaming). Create `internal/memory/embed_client.go` with:

```go
type embedClient struct {
    baseURL string
    apiKey  string
    http    *http.Client
}

func newEmbedClient(baseURL, apiKey string) *embedClient
func (c *embedClient) Embed(ctx context.Context, model, input string) ([]float32, error)
```

Failure modes: connection refused, 404 (model not loaded), 5xx, timeout → typed error that `Provider.GetContext` can classify and fall through to lexical-only.

### 4.3 Model selection

| Model | Dim | Size | Recommended for |
|---|---:|---:|---|
| `nomic-embed-text` | 768 | ~274 MB | Balanced quality/size — **default recommendation** |
| `mxbai-embed-large` | 1024 | ~670 MB | Higher quality for larger graphs |
| `qwen2.5:3b` (chat model pooled) | 2048 | already loaded | Fallback when no embed model is available |

Operator chooses via `[telegram].semantic_model` in config.toml. The embedder stores the model name alongside each vector so a later model switch can be detected (and mismatched vectors skipped).

## 5. Schema v3d

### 5.1 New table

```sql
CREATE TABLE IF NOT EXISTS entity_embeddings (
    entity_id   INTEGER PRIMARY KEY,
    model       TEXT    NOT NULL,
    dim         INTEGER NOT NULL CHECK(dim > 0 AND dim <= 4096),
    vec         BLOB    NOT NULL,            -- L2-normalized float32 LE
    updated_at  INTEGER NOT NULL,
    FOREIGN KEY(entity_id) REFERENCES entities(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_entity_embeddings_model ON entity_embeddings(model);
```

- **PRIMARY KEY on `entity_id`** — one embedding per entity. Re-vectorizing an entity replaces (ON CONFLICT DO UPDATE) the row rather than accumulating.
- **`model` column** — detects model mismatches. The similarity scan filters `WHERE model = ?` at query time.
- **`dim` column** — defense against corrupt writes. The scan asserts `dim == len(queryVec)` before doing the dot product.
- **`vec` BLOB** — packed little-endian float32. **Stored L2-normalized** (unit vectors) so cosine similarity reduces to a dot product in the scan.
- **FK cascade** — if an entity is deleted (manual `DELETE FROM entities` or future pruning), its embedding goes with it.

### 5.2 Migration

```sql
-- migration3cTo3d:
CREATE TABLE IF NOT EXISTS entity_embeddings (... see 5.1 ...);
CREATE INDEX IF NOT EXISTS idx_entity_embeddings_model ON entity_embeddings(model);
UPDATE schema_meta SET v = '3d' WHERE k = 'version' AND v = '3c';
```

Idempotent. Pre-existing installs get the table with zero rows; the background embedder populates it lazily.

## 6. The Embedder Worker

### 6.1 Lifecycle

Same pattern as the Phase 3.B extractor:
- A goroutine `memory.Embedder` started by `cmd/gormes/telegram.go` alongside the extractor.
- `Run(ctx)` blocks until ctx-cancel; `Close(ctx)` waits for in-flight embedding to finish (bounded).
- Polls every `EmbedderPollInterval` (default 30s — less aggressive than the extractor's 10s since embeddings are eventually consistent and lazy).

### 6.2 Poll query

```sql
SELECT e.id, e.name, e.type, COALESCE(e.description, '')
FROM entities e
LEFT JOIN entity_embeddings ee
    ON ee.entity_id = e.id AND ee.model = ?   -- current semantic_model
WHERE ee.entity_id IS NULL
ORDER BY e.updated_at DESC
LIMIT ?;                                      -- BatchSize (default 10)
```

"Needs embedding" = no row in `entity_embeddings` for this entity_id + current model. The LEFT JOIN / `IS NULL` pattern is the canonical SQL "find what's missing" query.

Ordered by `updated_at DESC` so freshly-extracted entities get embedded first — the user just mentioned them and might ask about them in the next turn.

### 6.3 Embedding input construction

For each entity, the input to `/v1/embeddings` is a labeled template that fuses name + type + description:

```
Entity: {Name}. Type: {Type}. Context: {Description}
```

Examples:
- `Entity: Acme. Type: PROJECT. Context: sports analytics platform`
- `Entity: Springfield. Type: PLACE.` (empty description → the `Context:` clause is omitted)

The explicit `Entity:` / `Type:` / `Context:` labels help the embedding model keep the category signal (e.g. `PROJECT`) from overwhelming the identity signal (e.g. `Acme`). Tests during Phase 3.D development observed that a raw `"X is a TYPE"` template caused distant-but-same-type entities to cluster too tightly (every PROJECT got a similar vector regardless of its actual meaning). The labeled template pushes the model's attention back onto the entity's specific meaning.

**Implementation rule:** if `Description` is empty after sanitization, the trailing `Context: ...` clause is omitted ENTIRELY — don't emit `Context: ` with nothing after the colon, which small models can interpret as a "fill in the blank" prompt.

### 6.4 Storage

After a successful `/v1/embeddings` call:

1. L2-normalize the vector: `v[i] /= sqrt(sum(v[i]²))`.
2. Pack as little-endian float32 bytes.
3. `INSERT OR REPLACE INTO entity_embeddings(entity_id, model, dim, vec, updated_at) VALUES(?, ?, ?, ?, strftime('%s','now'))`.

`INSERT OR REPLACE` semantics on the PK means a model switch (same entity, different model) overwrites cleanly without a two-step upsert.

### 6.5 Error handling

| Condition | Behavior |
|---|---|
| Ollama 404 (`"model not found"`) | Log WARN once per minute, skip batch, wait 60s before retry. Operator needs to `ollama pull <model>`. |
| Ollama connection refused | Same as above. |
| 5xx / transient | Increment in-memory retry counter; backoff before next poll. No dead-letter state for embeddings — they're optional. |
| Vector dim > 4096 | Log WARN, skip entity (CHECK constraint would also catch it). Defense against model weirdness. |
| Malformed response | Log WARN, skip entity. |

Unlike the extractor, **the embedder has no dead-letter state on entities**. An unembedded entity simply doesn't participate in semantic recall — lexical match still works for it. The worker keeps polling until Ollama comes back.

## 7. Similarity Scan in Go

### 7.1 In-memory cache

On the first `semanticSeeds` call, the Provider loads `entity_embeddings` into memory. The cache is a slice of `{entityID int64, vec []float32}`. Invalidated after any write (extractor commit, embedder insert) via a monotonic "graph version" counter — simple atomic increment on every `writeGraphBatch` + every embedder INSERT.

For ≤10k entities × 768-dim float32 that's ~30 MB RAM. Acceptable.

### 7.2 Cosine reduces to dot product

Because we store L2-normalized vectors, cosine similarity is a plain dot product:

```go
func cosineNormalized(a, b []float32) float32 {
    var dot float32
    for i := range a {
        dot += a[i] * b[i]
    }
    return dot
}
```

No square roots, no per-comparison normalization. Expected cost: ~1-5 µs per entity on modern CPUs in pure Go. 10,000 entities = 50ms worst case. Dedicated SIMD paths are available via `gonum` but out of 3.D scope.

### 7.3 Top-K selection

```go
// pairs: (entityID, similarity)
// Maintain a heap of size K, drop anything below the heap's min.
func topK(pairs []scoreEntry, k int) []scoreEntry
```

A min-heap of size K is O(n log k). For k=3 and n=10000 that's ~10k × 2 comparisons — negligible.

### 7.4 Threshold filter

The user is interested in RELEVANT semantic hits, not the top-K regardless of quality. Apply a minimum cosine threshold (`semantic_min_similarity`, default `0.35`). If the Top-K's similarity is below the threshold, drop it. For normalized vectors, cosine ∈ [-1, 1]; 0.35 is a reasonable floor.

## 8. Hybrid Fusion

### 8.1 Flow inside `Provider.GetContext`

```
1. Lexical seeds   (from extractCandidates → seedsExactName)
2. FTS5 seeds      (if lexical < 2)
3. Semantic seeds  ⟵ NEW: embedQuery(msg) → semanticSeeds
4. Union all seeds, dedup, cap at MaxSeeds
5. CTE traverse     (unchanged from 3.C)
6. Enumerate rels   (unchanged)
7. Format block     (unchanged)
```

### 8.2 Parallelism — sequential is fine

Running the three seed sources in parallel goroutines would theoretically save ~30ms on the embedding call. But:
- Lexical is ~1ms, FTS5 is ~10ms, semantic is ~30-50ms. Sequential = ~50-60ms total; well under the 100ms budget.
- Parallel goroutines add goroutine-leak risk if one blocks on a slow Ollama.
- Sequential lets us skip semantic entirely if lexical found ≥ MaxSeeds hits — saves a round trip most of the time.

**Sequential**: lexical → (if < 2) FTS5 → (if still < MaxSeeds AND semantic enabled) semantic. Semantic is the most expensive layer so it runs last and only when needed.

### 8.3 Semantic-only fallback path

If lexical + FTS5 returned zero seeds but the user's message is non-trivial (length > 10 chars), still run semantic. This is the "tell me about my projects" case — zero lexical hits, but semantic can still surface Acme.

### 8.4 Deduplication

Seeds are `int64` entity IDs. A `map[int64]struct{}` dedup preserves Layer-1 order, then appends FTS5, then semantic. Stops at MaxSeeds.

## 9. Kernel: Unchanged

The kernel's `RecallProvider` interface from 3.C is unchanged. All new logic lives inside `memory.Provider.GetContext`. No config changes to `kernel.Config`. T12 build-isolation stays green.

## 10. Configuration

New `TelegramCfg` fields (TOML-only, operator-tunable):

```go
type TelegramCfg struct {
    // ... existing including 3.C recall_* fields ...

    // SemanticEnabled (Phase 3.D). Default false — opt-in because it
    // requires an embedding model loaded in Ollama + extra RAM + CPU.
    SemanticEnabled        bool          `toml:"semantic_enabled"`
    // SemanticEndpoint — usually same as Hermes.Endpoint when Gormes
    // talks to Ollama directly. Falls back to Hermes.Endpoint if empty.
    SemanticEndpoint       string        `toml:"semantic_endpoint"`
    // SemanticModel — Ollama tag. "nomic-embed-text" recommended.
    SemanticModel          string        `toml:"semantic_model"`
    // SemanticTopK — how many semantic seeds to merge into the seed set.
    // Default 3. Larger values widen recall but risk injecting noise.
    SemanticTopK           int           `toml:"semantic_top_k"`
    // SemanticMinSimilarity — cosine threshold. Default 0.35.
    SemanticMinSimilarity  float64       `toml:"semantic_min_similarity"`
    // EmbedderPollInterval — how often the background embedder polls for
    // unembedded entities. Default 30s.
    EmbedderPollInterval   time.Duration `toml:"embedder_poll_interval"`
    // EmbedderBatchSize — entities per poll. Default 10.
    EmbedderBatchSize      int           `toml:"embedder_batch_size"`
    // EmbedderCallTimeout — per-entity Ollama call timeout. Default 10s.
    EmbedderCallTimeout    time.Duration `toml:"embedder_call_timeout"`
    // QueryEmbedTimeout — budget for embedding the user's message during
    // recall. Default 60ms (leaves 40ms for the rest of recall in the
    // 100ms kernel budget). If exceeded, semantic seeds are skipped.
    QueryEmbedTimeout      time.Duration `toml:"query_embed_timeout"`
}
```

## 11. Error Handling — honest fallback chain

Recall's promise: **best-effort, never block the turn**. All semantic failures fall through to 3.C behavior:

| Scenario | Recall result |
|---|---|
| Semantic disabled in config | Lexical-only (3.C parity) |
| Ollama down | Lexical-only; WARN log |
| Model not loaded | Lexical-only; WARN log |
| Query-embed timeout (>60ms) | Lexical-only |
| No embeddings in DB yet (cold start) | Lexical-only; embedder populating in background |
| Dim mismatch (model switch in flight) | Semantic scan skips mismatched rows; whatever matches the current model contributes |
| Semantic cosine scan throws (should not happen) | Lexical-only; ERROR log |

Each failure class is DISTINCT in logs so operators can diagnose. No failure blocks the turn.

## 12. Security

- **Embedding content = entity name + type + description.** All three fields are already LLM-extracted and validated by T2's sanitizer (no raw user content). No new PII surface beyond what the extractor already exposes.
- **The user's query message DOES go to the embedding endpoint.** Same privacy posture as the chat call — nothing new.
- **No vector leakage in logs.** Log format for embedder: `{entity_id, model, dim, similarity_top1}` — never the vector itself.
- **Cosine scan is constant-time w.r.t. the query.** No timing-attack surface because the scan always examines every row.
- **Cache invalidation cannot be poisoned.** The graph-version counter is atomic and monotonic; a malicious entity INSERT can cause a cache rebuild but not skip one.

## 13. Testing Strategy

### 13.1 Unit — pure math

- `TestCosineNormalized_IdenticalVectorsIsOne` — `cos(a,a) == 1.0` for unit vectors.
- `TestCosineNormalized_OrthogonalIsZero` — `cos([1,0], [0,1]) == 0.0`.
- `TestCosineNormalized_OppositeIsMinusOne` — `cos(a, -a) == -1.0`.
- `TestL2Normalize_UnitMagnitude` — any input normalized to length 1.0 ± 1e-6.
- `TestTopK_ReturnsKHighestScores` — heap correctness.
- `TestTopK_KLargerThanInput` — returns all input, sorted.
- `TestEncodeFloat32LE_RoundTrip` — pack-then-unpack preserves the floats bit-exact.

### 13.2 Unit — embedClient against httptest

- `TestEmbedClient_ParsesOpenAIResponse` — mock OpenAI-format server returns a vector; client returns `[]float32`.
- `TestEmbedClient_ModelNotFoundError` — mock returns 404 with Ollama's error shape; client returns a typed error.
- `TestEmbedClient_Timeout` — mock sleeps; ctx-deadline fires; client returns ctx.Err().

### 13.3 Unit — SQL against real tempdir DB

- `TestSchema_V3dHasEntityEmbeddingsTable` — migration adds the table + index.
- `TestEmbedder_PollsOnlyMissingEntities` — insert 3 entities, seed 2 with embeddings, assert the poll query returns 1.
- `TestEmbedder_ReplaceOnModelChange` — entity has embedding from model A, embedder runs with model B, row is REPLACE'd.
- `TestSemanticSeeds_FiltersByModel` — two embedding rows, one for current model, one for a stale model; scan returns only current.
- `TestSemanticSeeds_RespectsThreshold` — top hit at similarity 0.2 with threshold 0.35 → empty result.

### 13.4 Unit — Provider hybrid fusion

- `TestProvider_LexicalOnlyWhenSemanticDisabled` — unchanged 3.C behavior.
- `TestProvider_SemanticFillsWhenLexicalEmpty` — with an embedded graph and a query that lexically matches nothing, semantic produces seeds.
- `TestProvider_SeedsAreDeduped` — same entity ID surfaces via lexical AND semantic; one entry in final seed list.

### 13.5 Integration — real Ollama

Gated by `skipIfNoOllama` (reuse 3.B helper). Pull `nomic-embed-text` before running; test skips if not loaded.

- `TestSemantic_Integration_Ollama_MyProjectsFindsAcme`:
  1. Seed turns as in 3.C's test.
  2. Run extractor to populate entities.
  3. Run embedder to populate embeddings (wait for coverage == 100% with 2-minute budget).
  4. Call `Provider.GetContext` with "tell me about my projects".
  5. Assert fence contains "Acme".

If this passes, 3.D is functionally done.

### 13.6 Build isolation

- Kernel still must not import `internal/memory` (T12 — unchanged).
- Embed client is package-private to `internal/memory`; kernel never sees it.

## 14. Binary Budget

Pure Go additions: `embed_client.go`, `embedder.go`, `semantic_sql.go`, `cosine.go` + tests. Expected size delta: **< 250 KB**. `bin/gormes` stays at ~17 MB (well under 100 MB ceiling).

Runtime RAM for the embedding cache: 3 KB/entity × 500 entities ≈ 1.5 MB with nomic-embed-text's 768-dim vectors. For 10k entities: ~30 MB. Acceptable.

## 15. Out of Scope — Explicit Deferrals

- **Decay / forgetting curve** → Phase 3.E. Entities with stale `updated_at` don't get down-weighted.
- **Cross-chat synthesis** → Phase 3.E. Entities remain globally-scoped; seed selection remains per-chat.
- **ANN index (HNSW, IVF)** → future phase when we hit 100k+ entities.
- **Embedding re-vectorization tool** → operator does `DELETE FROM entity_embeddings; UPDATE schema_meta SET v = '3d'`; embedder re-populates.
- **Multi-model fusion** — only one active embedding model at a time; stored vectors from other models are dead weight until deleted.
- **Vector quantization** — Q8 or Q4 could cut RAM 4-8× but adds dequantization cost on every scan. Revisit if RAM becomes a concern.
- **Embedding user turns directly** (not just entities) — could widen FTS5's role. 3.E consideration.

## 16. Rollout

- One PR, subagent-driven same cadence as 3.A/3.B/3.C.
- **First boot on existing 3.C installs:** migration3cTo3d runs; `entity_embeddings` starts empty; embedder worker launches; semantic recall is effectively lexical-only until the embedder catches up (30s × batches of 10 = ~5 minutes to embed 100 entities).
- **Feature flag:** `semantic_enabled=false` default. Operators opt in by setting `true` in config.toml. Keeps 3.D gated to users who explicitly want it — and have an embedding model loaded in Ollama.
- **Disabling post-enable:** set `semantic_enabled=false` and restart. The embedder stops; stored vectors stay in place (ready for re-enable).
