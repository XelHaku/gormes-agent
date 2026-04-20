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
//
// Polling is deliberately less aggressive than the extractor (30s vs 10s
// default): embedding is eventually-consistent. A few seconds of lag
// between entity creation and availability for semantic recall is fine;
// hammering the embed endpoint is not.
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
// The running flag is set before entering the tick loop so that Close
// can distinguish "never started" from "started but ctx cancelled quickly".
func (e *Embedder) Run(ctx context.Context) {
	e.running.Store(true)
	defer func() {
		// Non-blocking send: done is buffered 1, so this never blocks even if
		// nobody is waiting in Close yet.
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
// started. Bounded by ctx. Idempotent — the once-guard ensures multiple
// callers never double-wait on the done channel.
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
			// Keep going — one bad entity (e.g. transient network error,
			// model not yet loaded) shouldn't block the rest of the batch.
		}
	}
}

// pollMissing finds up to BatchSize entities that lack an embedding for
// the current model. The LEFT JOIN / IS NULL pattern is the canonical
// anti-join in SQLite: rows where no matching entity_embeddings row exists
// for cfg.Model are returned; already-embedded entities are excluded.
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
//
//	Entity: {Name}. Type: {Type}. Context: {Description}
//
// When Description is empty, the "Context: ..." clause is omitted
// entirely (never emit "Context: " with nothing after it — the model
// would learn to treat a trailing period as noise).
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
// result into entity_embeddings via ON CONFLICT DO UPDATE.
//
// REPLACE semantics (entity_id is PRIMARY KEY): switching models means
// the new call produces a new row for the same entity_id, which replaces
// the old-model row. We never accumulate one row per model — the schema
// is keyed on entity_id, not (entity_id, model). This is deliberate:
// gormes only uses one model at a time; multi-model accumulation would
// bloat the table and require cache eviction logic.
func (e *Embedder) embedAndStore(ctx context.Context, row embedderRow) error {
	callCtx, cancel := context.WithTimeout(ctx, e.cfg.CallTimeout)
	defer cancel()

	input := buildEmbedInput(row)
	vec, err := e.ec.Embed(callCtx, e.cfg.Model, input)
	if err != nil {
		// TODO(T8): differentiate errEmbedModelNotFound from transient errors for
		// log throttling — both currently surface as WARN in loopOnce.
		return fmt.Errorf("embed: %w", err)
	}
	if len(vec) == 0 {
		return fmt.Errorf("embed: empty vector")
	}
	// L2-normalize in-place before storage. Stored vectors must be unit
	// vectors so that the recall provider can use dot product as cosine
	// similarity without an extra division.
	l2Normalize(vec)
	blob := encodeFloat32LE(vec)

	// NOTE: uses parent ctx, not callCtx — the per-call timeout applies only
	// to the HTTP embed round-trip. Once we've computed a vector, don't let
	// a tight timeout strand it in memory.
	_, err = e.store.db.ExecContext(ctx, `
		INSERT INTO entity_embeddings(entity_id, model, dim, vec, updated_at)
		VALUES(?, ?, ?, ?, strftime('%s','now'))
		ON CONFLICT(entity_id) DO UPDATE SET
			model      = excluded.model,
			dim        = excluded.dim,
			vec        = excluded.vec,
			updated_at = excluded.updated_at`,
		row.ID, e.cfg.Model, len(vec), blob)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	// Bump the cache so the recall provider's next semanticSeeds call
	// rebuilds from SQLite and sees the new embedding.
	e.cache.bump()
	return nil
}
