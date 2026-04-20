# Gormes Phase 3.B — Ontological Graph + Async LLM Extraction Design

**Status:** Approved 2026-04-20 · implementation plan pending
**Depends on:** Phase 3.A (Lattice Memory Foundation) green on `main`

## Related Documents

- [`gormes/docs/ARCH_PLAN.md`](../../ARCH_PLAN.md) — Phase 3 = "The Black Box (Memory): SQLite + FTS5 + ontological graph in Go". 3.A delivered the foundation (SQLite + FTS5). This spec delivers the ontological graph layer + LLM-assisted population.
- Phase 3.A — [`2026-04-20-gormes-phase3a-memory-design.md`](2026-04-20-gormes-phase3a-memory-design.md) — SQLite engine choice, schema v3a, async worker pattern. This spec extends schema to v3b and adds a second worker (the "Brain") alongside the existing persistence worker.
- Phase 2.C — [`2026-04-19-gormes-phase2c-persistence-design.md`](2026-04-19-gormes-phase2c-persistence-design.md) — bbolt session mapping. Unchanged by this spec.

---

## 1. Goal

Native, zero-dependency replacement for third-party memory services (Honcho, Hindsight, Mem0). Every turn that lands in `turns` is asynchronously analyzed by an LLM extractor that:

1. Identifies entities (people, projects, concepts, places, organizations, tools).
2. Identifies relationships between those entities (`Jose WORKS_ON Gormes`, `Gormes USES Bbolt`).
3. Persists both into a local SQLite graph that can later be queried via FTS5 or direct SQL.

The kernel's 250 ms `StoreAckDeadline` remains unconditionally honored: extraction never runs on the hot path. The kernel is not modified by this phase.

## 2. Non-Goals

- **No recall or retrieval tools.** `search_past_conversations(query)`, `recall_entity(name)`, pre-turn context injection — Phase 3.C.
- **No vector / embedding / semantic search.** Only textual + relational graph in 3.B. Embeddings may come in 3.C or later.
- **No cross-conversation entity linking across platforms.** "Jose" mentioned in the TUI and "Jose" mentioned in Telegram are treated as the same entity (shared `memory.db`). But no cross-device sync.
- **No graph UI.** Entities live in SQLite; operators inspect via `sqlite3` or the recall tools that arrive in 3.C.
- **No Python-side changes.** Python's `SessionDB` is still canonical for LLM conversation context. Our graph is a *local derivative* of what we already wrote to `turns` during 3.A.
- **No retraining, no model fine-tuning.** We use the same Hermes endpoint with a different system prompt.

## 3. Scope

Four units of work, one spec:

1. Schema extension: `turns.extracted` column + two new tables `entities` and `relationships`. Version bump `3a → 3b` with gated migration.
2. An `Extractor` goroutine that polls `turns WHERE extracted=0 AND extraction_attempts<N`, calls the LLM, writes entities/relationships/marks turns complete.
3. A strict LLM I/O contract: a system prompt and a JSON output schema. Malformed output is rejected and the turn batch is retried up to `MaxAttempts` before being marked terminally failed.
4. Operational resilience: exponential backoff on 429, timeouts, malformed JSON; a dead-letter state (`extracted=2`); graceful shutdown that completes the in-flight batch or aborts cleanly.

## 4. Schema — v3b Migration

### 4.1 `turns` gains `extracted`, `extraction_attempts`, `extraction_error`

```sql
ALTER TABLE turns ADD COLUMN extracted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE turns ADD COLUMN extraction_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE turns ADD COLUMN extraction_error TEXT;
CREATE INDEX IF NOT EXISTS idx_turns_unextracted
    ON turns(id) WHERE extracted = 0;
```

Values of `turns.extracted`:
- `0` — unprocessed (default)
- `1` — processed successfully
- `2` — permanently failed after `MaxAttempts` retries (dead-letter; manual reset via `UPDATE turns SET extracted=0, extraction_attempts=0 WHERE id=…`)

The partial index `idx_turns_unextracted` makes the worker's polling query a point-lookup on a tiny index regardless of how large `turns` grows.

**Why three columns instead of one JSON blob**: queries like "how many unprocessed turns?" and "which turns failed?" are pure SQL; no JSON parsing overhead on the hot path; clear diagnostic trail.

### 4.2 New table `entities`

```sql
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
```

- **Primary key:** `id` autoincrement integer — cheap FK target.
- **Natural uniqueness:** `(name, type)`. If the extractor sees "Jose"/PERSON twice across turns, we upsert rather than insert a duplicate.
- **Type enumeration** constrained at the column level. `OTHER` is the catch-all so the LLM can never trip the CHECK by inventing a type we didn't anticipate. New types added in a future migration require a `3c` bump.
- **`description`** is a free-text ≤512-char summary of the entity; replaced on upsert if the LLM produces a non-empty one. Empty descriptions do not overwrite existing non-empty ones (see §7 upsert logic).
- **`updated_at`** is UNIX seconds, refreshed on every upsert. Useful for TTL policies in a future phase.

### 4.3 New table `relationships`

```sql
CREATE TABLE IF NOT EXISTS relationships (
    source_id   INTEGER NOT NULL,
    target_id   INTEGER NOT NULL,
    predicate   TEXT    NOT NULL,
    weight      REAL    NOT NULL DEFAULT 1.0,
    updated_at  INTEGER NOT NULL,
    PRIMARY KEY(source_id, target_id, predicate),
    FOREIGN KEY(source_id) REFERENCES entities(id) ON DELETE CASCADE,
    FOREIGN KEY(target_id) REFERENCES entities(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_relationships_target ON relationships(target_id);
CREATE INDEX IF NOT EXISTS idx_relationships_predicate ON relationships(predicate);
```

- **Composite primary key** `(source_id, target_id, predicate)` — the same predicate between the same two entities cannot be duplicated; instead weight is incremented on re-extraction (see §7).
- **Foreign keys with `ON DELETE CASCADE`**: if an entity is deleted, dangling relationships are cleaned up. SQLite FK enforcement is already enabled by the `PRAGMA foreign_keys = ON` set in Phase 3.A.
- **`predicate`** free text — we don't constrain the vocabulary. The LLM is instructed to emit verb-phrase predicates in SCREAMING_SNAKE_CASE (`WORKS_ON`, `DISLIKES`, `USES`). Free-text lets us grow without schema bumps.
- **`weight`** REAL — starts at 1.0, accumulates on re-extraction. Phase 3.C recall can rank by weight.

### 4.4 `schema_meta` bump + idempotent migration

```sql
-- Inside the Phase 3.B migration function, run in a single transaction:
UPDATE schema_meta SET v = '3b' WHERE k = 'version' AND v = '3a';
```

Migration runs at `OpenSqlite` time; the path is version-gated:

```
pseudocode:
  read schema_meta.v
  if v == "3a":  run the 3a→3b migration DDL in one transaction, UPDATE version
  if v == "3b":  no-op (fresh open on already-migrated DB)
  if v unknown:  return ErrSchemaUnknown (refuse to operate)
```

Idempotency contract: opening a v3a DB repeatedly after the 3.B binary lands must result in exactly one successful migration; opening a v3b DB is a no-op; opening a v-future DB refuses to start.

## 5. The Brain Worker

### 5.1 Package layout

Extractor lives in `gormes/internal/memory/extractor.go` alongside the existing persistence `worker.go`. Rationale: both workers share the `*SqliteStore` handle and the same underlying connection pool; co-locating them keeps the single-writer discipline trivially enforceable.

### 5.2 Type surface

```go
// internal/memory/extractor.go

type ExtractorConfig struct {
    Model          string        // empty = reuse kernel's Hermes model
    PollInterval   time.Duration // default 10s
    BatchSize      int           // default 5 turns per LLM call
    MaxAttempts    int           // default 5 before dead-letter
    CallTimeout    time.Duration // default 30s per LLM call
    BackoffBase    time.Duration // default 2s; doubles per attempt
    BackoffMax     time.Duration // default 60s cap
}

// Extractor runs the LLM-assisted entity/relationship extraction loop.
// Single-owner goroutine. Owns a read-only view of the *SqliteStore's
// *sql.DB via the same connection (SetMaxOpenConns(1) pool).
type Extractor struct {
    store *SqliteStore
    llm   hermes.Client
    cfg   ExtractorConfig
    log   *slog.Logger
    done  chan struct{}
}

func NewExtractor(s *SqliteStore, llm hermes.Client, cfg ExtractorConfig, log *slog.Logger) *Extractor
func (e *Extractor) Run(ctx context.Context)
func (e *Extractor) Close(ctx context.Context) error
```

`Run` blocks until `ctx` is cancelled. `Close` waits for the current in-flight batch to finish (subject to its own ctx budget), then returns. If the caller wants to abandon the batch mid-call, cancel the ctx passed to `Run` instead.

### 5.3 The main loop

```
loop:
  select on ctx.Done(), return
  select on timer:
    batch = SELECT id, role, content FROM turns
            WHERE extracted = 0 AND extraction_attempts < MaxAttempts
            ORDER BY id LIMIT BatchSize
    if len(batch) == 0: continue (wait PollInterval)
    result, err = callLLM(ctx, batch)
    if err is retriable (429 / timeout / transient net): backoff, increment attempts, continue
    if err is terminal (malformed JSON, context canceled): increment attempts, log, continue
    if ok: upsert entities + relationships + mark batch extracted=1 in ONE transaction
    reset backoff on success
```

**Single-writer invariance**: the upsert transaction holds the only write slot on the `*sql.DB` (`SetMaxOpenConns(1)`). The persistence worker from Phase 3.A acquires the same slot for `INSERT INTO turns`. They serialize naturally; no lock tuning required. Because writes are tens of microseconds and the extractor pauses for `PollInterval`s between batches, contention is effectively zero.

### 5.4 Batch size reasoning

- Too small (1 turn): LLM has no context; quality drops; token cost amortizes poorly.
- Too large (50 turns): prompts approach LLM context limits; latency spikes; one malformed output poisons many turns.
- **Default 5**: one conversational exchange (≈2-3 turns) plus some lookback, ~2-5K input tokens — well within any LLM's context budget, large enough to see cross-turn references.

Configurable via `ExtractorConfig.BatchSize` for operators who want different tradeoffs.

## 6. LLM I/O Contract

### 6.1 System prompt (verbatim)

```
You are an ontological entity extractor. You read conversation turns
between a user and an AI assistant, and you emit a structured JSON
summary of the entities mentioned and the relationships between them.

Rules:
1. Output ONLY valid JSON. No prose. No markdown fences. Start with '{'.
2. The JSON object has exactly two keys: "entities" and "relationships".
3. Each entity is {"name": string, "type": one of
   ["PERSON","PROJECT","CONCEPT","PLACE","ORGANIZATION","TOOL","OTHER"],
   "description": string (<= 512 chars, optional, empty string if absent)}.
4. Each relationship is {"source": string (entity name), "target": string
   (entity name), "predicate": string (SCREAMING_SNAKE_CASE verb phrase),
   "weight": number between 0.0 and 1.0}.
5. Relationship source/target names MUST match entity names in this
   same response exactly. Do not reference entities not in entities[].
6. Predicates are in present tense, factual, and in the form of a verb
   or verb phrase: "WORKS_ON", "USES", "DISLIKES", "IS_PART_OF", etc.
7. Deduplicate within the response: do not emit the same entity twice
   or the same (source, target, predicate) triple twice.
8. If no entities are present, emit {"entities": [], "relationships": []}.
```

### 6.2 User message (per batch)

```
Conversation turns to analyze (role: content):

[user]: <turn 1 content>
[assistant]: <turn 2 content>
[user]: <turn 3 content>
...
```

Turns are joined in id-ascending order, role-prefixed, separated by blank lines. Truncate any single turn to 4000 chars to protect against accidental megablobs (same cap as the Telegram renderer).

### 6.3 Expected output schema (Go struct)

```go
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
```

### 6.4 Validation layer

After `json.Unmarshal` into `extractorOutput`:

1. Every entity's `Type` must match the CHECK constraint whitelist. Invalid type → demote to `OTHER` (do not reject the whole batch).
2. Every entity `Name` must be non-empty, ≤255 chars after trimming. Empty names: drop the entity.
3. Every relationship's `Source` and `Target` must refer to names present in `Entities`. Orphan relationships: drop.
4. `Weight` clamped to `[0.0, 1.0]`. NaN or negative: replace with `1.0`.
5. `Predicate` normalized: uppercase, non-alphanumerics → `_`, trimmed. Empty after normalization: drop the relationship.

Dropped items are logged at DEBUG but do not fail the batch. A batch with **any** valid entity counts as a successful extraction. A batch that yields zero valid entities AND zero valid relationships is still marked `extracted=1` — we saw the turns, we extracted nothing. No retry.

### 6.5 LLM invocation

Reuses the existing `hermes.Client.OpenStream` — same path the kernel uses for real turns. Request shape:

```go
hermes.ChatRequest{
    Model: cfg.Model,           // or kernel's model if empty
    Stream: true,                // ncruces requires streaming; we collect all tokens
    Messages: []hermes.Message{
        {Role: "system", Content: extractorSystemPrompt},
        {Role: "user",   Content: formattedBatch},
    },
    // No tools — pure JSON extraction, no tool_calls allowed.
}
```

Collect all streamed tokens into a buffer, join, `json.Unmarshal`. The streaming overhead is immaterial (full response typically <1 s); we stream only to reuse the existing client contract.

**SessionID:** deliberately empty — extractor invocations don't share session state with the user's conversation. Python's api_server will start a fresh throwaway session per extraction batch.

## 7. Upsert Logic

One transaction per batch. Pseudocode:

```
BEGIN;
for each entity in validated batch:
  INSERT INTO entities(name, type, description, updated_at)
    VALUES(?, ?, ?, strftime('%s','now'))
    ON CONFLICT(name, type) DO UPDATE SET
      description = CASE WHEN excluded.description != '' THEN excluded.description ELSE entities.description END,
      updated_at  = excluded.updated_at;

  map entity name/type → id (SELECT after upsert)

for each relationship in validated batch:
  resolve source_name → source_id from the entity-id map
  resolve target_name → target_id from the entity-id map
  skip if either is unresolved
  INSERT INTO relationships(source_id, target_id, predicate, weight, updated_at)
    VALUES(?, ?, ?, ?, strftime('%s','now'))
    ON CONFLICT(source_id, target_id, predicate) DO UPDATE SET
      weight     = MIN(relationships.weight + excluded.weight, 10.0),
      updated_at = excluded.updated_at;

UPDATE turns SET extracted = 1, extraction_error = NULL WHERE id IN (...);
COMMIT;
```

**Weight accumulation:** capped at 10.0 to prevent unbounded growth. When we see the same `(source, target, predicate)` triple 11 times, it's already "strong" — no need to keep incrementing. Phase 3.C recall can use weight for ranking but doesn't rely on unbounded arithmetic.

**Description preservation:** a non-empty description overrides, an empty one does not. Protects against the LLM "forgetting" details in a later extraction.

## 8. Operational Resilience

### 8.1 Error taxonomy

| Class | Trigger | Behavior |
|---|---|---|
| **Retriable** | HTTP 429 with `Retry-After` header | sleep `max(Retry-After, backoff)`, do NOT increment attempts, retry same batch |
| **Retriable** | Transient network error / 5xx | sleep `backoff`, increment attempts, retry same batch next loop |
| **Retriable** | Context deadline (CallTimeout) | sleep `backoff`, increment attempts, retry same batch next loop |
| **Terminal (batch)** | `json.Unmarshal` error | log WARN with first 200 chars of output, increment attempts, continue to next batch (don't sleep) |
| **Terminal (batch)** | Zero valid entities + zero valid relationships | log INFO "nothing to extract", mark batch `extracted=1`, no retry |
| **Permanent** | `extraction_attempts >= MaxAttempts` | mark batch `extracted=2`, log ERROR with last error message, never retry |
| **Fatal** | `ctx.Done()` | exit the loop, do NOT increment attempts; graceful shutdown |

The kernel is **never** signaled. The extractor's failures are purely observational — the user's real-time conversation continues regardless.

### 8.2 Backoff curve

```
attempt 0: sleep BackoffBase (2s)
attempt 1: sleep 4s
attempt 2: sleep 8s
attempt 3: sleep 16s
attempt 4: sleep min(32s, BackoffMax=60s) = 32s
```

Resets on first successful batch extraction.

### 8.3 Dead-letter mechanism

Once `extraction_attempts >= MaxAttempts`, the worker sets `extracted = 2`. A subsequent SELECT skips these rows (`WHERE extracted = 0`), so they never waste LLM tokens again. An operator can manually reset via:

```sql
UPDATE turns SET extracted = 0, extraction_attempts = 0, extraction_error = NULL
WHERE extracted = 2;
```

A Phase-3.C recall tool might offer a CLI subcommand for this, but it's out of 3.B scope.

### 8.4 Graceful shutdown

`Extractor.Close(ctx)` signals the loop to exit after the current batch completes. If the batch is mid-LLM-call, the in-flight LLM ctx is cancelled, the attempt is NOT incremented (so the batch stays `extracted=0` for next boot), and Close returns. If `ctx` passed to Close expires first, Close returns immediately and the LLM call is cancelled, with the same no-increment rule.

## 9. Integration

### 9.1 `cmd/gormes-telegram` wiring

```go
// After memory.OpenSqlite and before bot.Run:
ext := memory.NewExtractor(mstore, hc, memory.ExtractorConfig{
    Model:         cfg.Hermes.Model,
    BatchSize:     cfg.Telegram.ExtractorBatchSize,      // new config field
    PollInterval:  cfg.Telegram.ExtractorPollInterval,   // new config field
    MaxAttempts:   5,
    CallTimeout:   30 * time.Second,
    BackoffBase:   2 * time.Second,
    BackoffMax:    60 * time.Second,
}, slog.Default())

go ext.Run(rootCtx)
defer func() {
    shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), kernel.ShutdownBudget)
    defer cancelShutdown()
    _ = ext.Close(shutdownCtx)
}()
```

Both the persistence worker (inside `SqliteStore`) and the extractor (standalone) live on the same `*sql.DB`. The kernel remains oblivious.

### 9.2 `cmd/gormes` (TUI)

**No change.** The TUI keeps `NoopStore`. Phase 3.B adds zero bytes to the TUI binary.

### 9.3 New config fields

```go
type TelegramCfg struct {
    // ... existing fields ...
    ExtractorBatchSize     int           `toml:"extractor_batch_size"`     // default 5
    ExtractorPollInterval  time.Duration `toml:"extractor_poll_interval"`  // default 10s
}
```

`time.Duration` fields in TOML accept strings like `"10s"`, `"2m"`. `go-toml/v2` handles this natively.

### 9.4 Build isolation

- **Kernel** MUST still not import `internal/memory`. Unchanged.
- **TUI** MUST still not import `internal/memory`. Unchanged.
- **New assertion**: `internal/memory/extractor.go` imports `internal/hermes` — which the persistence worker already does indirectly. This is acceptable because extractor runs in `cmd/gormes-telegram`, which already pulls in the full hermes + telegram stack.

## 10. Testing Strategy

### 10.1 Unit tests — migration

- `TestMigrate_3aTo3b` — open a DB at v3a (install old schema manually), call OpenSqlite, assert `entities`, `relationships`, `turns.extracted` all exist and `schema_meta.v = '3b'`.
- `TestMigrate_AlreadyMigrated` — open a v3b DB, verify no-op.
- `TestMigrate_UnknownVersion` — set `schema_meta.v = '3z'`, verify OpenSqlite refuses with a clear sentinel error `ErrSchemaUnknown`.

### 10.2 Unit tests — validation layer

- `TestValidate_RejectsEmptyNames` — extractor gets an entity with `name=""`, drops it silently.
- `TestValidate_CoercesInvalidType` — `"type": "BUILDING"` demotes to `OTHER`.
- `TestValidate_DropsOrphanRelationships` — relationship whose source doesn't match any entity name is dropped.
- `TestValidate_ClampsWeight` — `weight: 1.5` clamped to `1.0`; `weight: -0.3` clamped to `1.0`; `weight: NaN` replaced with `1.0`.
- `TestValidate_NormalizesPredicate` — `"works on"` → `WORKS_ON`.

### 10.3 Unit tests — upsert

- `TestUpsert_Deduplicates` — same entity inserted twice ends up as one row; `updated_at` refreshed.
- `TestUpsert_AccumulatesRelationshipWeight` — same `(source, target, predicate)` twice ends up as one row with `weight=2.0`.
- `TestUpsert_WeightCap` — twelve accumulations yield `weight=10.0` not `12.0`.
- `TestUpsert_PreservesNonEmptyDescription` — first upsert with `description="Hi"`, second with `description=""` → stored description remains `"Hi"`.

### 10.4 Unit tests — worker loop

- `TestExtractor_SkipsWhenNoUnprocessed` — empty `turns` → zero LLM calls.
- `TestExtractor_MarksTurnsExtracted` — mock LLM returns valid JSON, verify `turns.extracted = 1` after batch commit.
- `TestExtractor_IncrementsAttemptsOnMalformedJSON` — mock LLM returns garbage, verify `extraction_attempts=1`, `extracted=0`.
- `TestExtractor_DeadLettersAfterMaxAttempts` — mock LLM always returns garbage, verify after 5 attempts `extracted=2`.
- `TestExtractor_ResetsAttemptsOnSuccess` — after several failures then one success, `extraction_attempts` returns to 0? **NO** — attempts stay; only new turns start at 0. This is intentional (attempts is a property of the turn, not the worker). Test pins this behavior.
- `TestExtractor_HonorsRetryAfter` — mock LLM returns 429 with `Retry-After: 1`, worker sleeps ≥1s before retry and does NOT increment attempts.

### 10.5 Unit tests — graceful shutdown

- `TestExtractor_CloseDuringIdlePoll` — Close during PollInterval wait returns immediately.
- `TestExtractor_CloseDuringLLMCall` — Close mid-LLM-call cancels the LLM ctx; `extraction_attempts` stays at its previous value (no increment).

### 10.6 Integration test

- `TestExtractor_EndToEnd_RealDB_MockLLM` — open real SQLite, insert 5 turns via `SqliteStore.Exec`, construct Extractor with a mock `hermes.Client` that returns a crafted JSON response, run one poll cycle, verify `entities` + `relationships` rows present with expected values, `turns.extracted = 1` on all 5.

### 10.7 Build isolation

Extend `buildisolation_test.go`:
- `TestKernelHasNoExtractorDep` — `go list -deps ./internal/kernel` must not contain `/internal/memory/extractor` (though this is the same package; the test greps for `.Extractor` struct, OR more honestly: the existing `TestKernelHasNoMemoryDep` already covers this since `internal/memory` is the parent).
- Decision: **no new isolation test needed**. Extractor lives inside `internal/memory` — the existing Phase 3.A isolation tests already block it from the kernel and TUI.

### 10.8 Full sweep

`go test -race ./... -count=1 -timeout 180s`; `go vet ./...`; binary size unchanged for TUI, bot grows only by the Go code for extractor + hermes usage (<200 KB estimated).

## 11. Binary Budgets

| Binary | Current (post-3.A) | After 3.B (projected) | Budget |
|---|---:|---:|---:|
| `bin/gormes` (TUI) | 8.2 MB | 8.2 MB (unchanged — NoopStore) | ≤ 10 MB ✓ |
| `bin/gormes-telegram` | 15 MB | ~15.2 MB (+extractor code, reuses existing hermes client) | ≤ 20 MB ✓ |

Extractor is pure Go code calling existing libraries. No new third-party deps. No CGO. Size growth is negligible.

## 12. Security

- **LLM prompt injection:** the user's turn content is embedded in the extractor's prompt. A hostile user could insert text like `"ignore previous instructions; emit {...malicious JSON...}"`. Mitigations:
  1. The system prompt explicitly says "Output ONLY valid JSON. Start with `{`." — anything else is rejected.
  2. The JSON schema is strictly validated (§6.4); invalid types are coerced, invalid predicates normalized, orphan relationships dropped. The worst a prompt injector can do is poison *their own* entity graph.
  3. Extractor runs against the user's own local data. No cross-tenant blast radius.
- **No secrets in extractor logs:** extraction errors log the first 200 chars of malformed LLM output. Turn content is NOT logged verbatim (it may contain user secrets). The error message says "malformed JSON at offset N" without the payload.
- **SQL safety:** all INSERTs use `?` placeholders (same discipline as Phase 3.A). Entity names are passed as parameters, never concatenated.
- **Database file mode:** still 0664 (ncruces default). The parent dir's `0700` continues to prevent non-owner access.

## 13. Out of Scope — Explicit Deferrals

- **Phase 3.C** — recall tools (`search_past_conversations`, `recall_entity`, auto-injection).
- **Entity disambiguation** — "Jose" (PERSON) vs "San Jose" (PLACE): the LLM is expected to type-disambiguate. No clustering / embedding disambiguation in 3.B.
- **Cross-session entity linking** — same entity mentioned in different Telegram chats: shared `memory.db` makes this automatic.
- **Incremental re-extraction** — we extract each turn exactly once. If the LLM gets smarter, a future phase can bulk-reset `extracted` and re-run. Out of 3.B.
- **Graph pruning / TTL** — `updated_at` is recorded but no eviction policy in 3.B. Entities live forever unless manually deleted.
- **Public/private entity separation** — all entities in one graph. Multi-user ACLs out of 3.B.

## 14. Verification Checklist

- [ ] `go build` produces static binaries (CGO_ENABLED=0).
- [ ] `bin/gormes` ≤ 10 MB (unchanged).
- [ ] `bin/gormes-telegram` ≤ 20 MB (target ~15.2 MB).
- [ ] `go list -deps ./internal/kernel` — still no memory/sqlite/extractor.
- [ ] `go list -deps ./cmd/gormes` — still no memory/sqlite/extractor.
- [ ] `schema_meta.v = '3b'` after OpenSqlite.
- [ ] Inserting unextracted turns + running Extractor.Run briefly populates entities + relationships.
- [ ] All tests in §10 green under `-race`.
- [ ] `go vet ./...` clean.

## 15. Rollout

- One PR, linear commit series under `subagent-driven-development` (same cadence as 3.A).
- **First boot on existing 3.A installs:** `OpenSqlite` runs the 3a→3b migration idempotently. Schema bumps from `3a` to `3b`. No user data loss; existing turns are marked `extracted=0` and the extractor will begin populating the graph on next poll cycle.
- **Rollback from 3.B to 3.A:** requires a manual `ALTER TABLE turns DROP COLUMN extracted`-style reversal plus `DROP TABLE entities; DROP TABLE relationships;` — documented in the migration file header for operators. Not auto-reversible.
- **Disabling extraction without rollback:** set `cfg.Telegram.ExtractorBatchSize = 0` and the worker skips every poll (early-return check). Effectively a toggle without schema change.
