package memory

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TrebuchetDynamics/gormes-agent/internal/hermes"
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

	backoffCur time.Duration // current per-loop sleep; resets to 0 on success

	done      chan struct{}
	closeOnce sync.Once
	running   atomic.Bool
}

// NewExtractor constructs an Extractor. Caller drives lifecycle via
// Run(ctx) and Close(ctx).
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

// Run blocks until ctx is cancelled. Each tick: loopOnce.
func (e *Extractor) Run(ctx context.Context) {
	e.running.Store(true)
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
			e.loopOnce(ctx) // stub in T5; T6 fills in
		}
	}
}

// Close waits for Run to exit if Run is currently executing, bounded by
// ctx. If Run has never been called, returns immediately. Idempotent.
func (e *Extractor) Close(ctx context.Context) error {
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
		e.backoffCur = 0 // no work means no failure
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
		e.log.Warn("extractor: malformed JSON",
			"turn_ids", ids, "preview", preview, "err", err)
		e.recordFailure(ctx, ids, "malformed JSON: "+err.Error())
		e.advanceBackoff()
		return
	}

	// writeGraphBatch is ONE transaction: upserts + mark-extracted commit
	// atomically. See graph.go.
	if err := writeGraphBatch(ctx, e.store.db, validated, ids); err != nil {
		e.log.Warn("extractor: graph write failed",
			"turn_ids", ids, "err", err)
		e.recordFailure(ctx, ids, err.Error())
		e.advanceBackoff()
		return
	}

	e.log.Debug("extractor: batch processed",
		"turn_ids", ids,
		"entities", len(validated.Entities),
		"relationships", len(validated.Relationships))
	e.backoffCur = 0 // success resets
}

// pollBatch reads up to cfg.BatchSize unprocessed turns.
func (e *Extractor) pollBatch(ctx context.Context) ([]turnRow, error) {
	rows, err := e.store.db.QueryContext(ctx,
		`SELECT id, role, content FROM turns
		 WHERE extracted = 0 AND cron = 0 AND memory_sync_status = 'ready' AND extraction_attempts < ?
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

// recordFailure increments extraction_attempts (with errMsg) and, if any
// turn has reached MaxAttempts, flips those turns to extracted=2 (dead-
// letter) so the polling query skips them permanently.
func (e *Extractor) recordFailure(ctx context.Context, ids []int64, errMsg string) {
	if len(ids) == 0 {
		return
	}
	if err := incrementAttempts(ctx, e.store.db, ids, errMsg); err != nil {
		e.log.Warn("extractor: incrementAttempts failed", "err", err)
		return
	}
	placeholders, idArgs := inListArgs(ids)
	q := "SELECT id FROM turns WHERE extraction_attempts >= ? AND extracted = 0 AND id IN (" + placeholders + ")"
	args := append([]any{e.cfg.MaxAttempts}, idArgs...)
	rows, err := e.store.db.QueryContext(ctx, q, args...)
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
		if err := markDeadLetter(ctx, e.store.db, dead, errMsg); err != nil {
			e.log.Warn("extractor: markDeadLetter failed", "err", err)
			return
		}
		e.log.Error("extractor: dead-lettered after max attempts",
			"turn_ids", dead, "max_attempts", e.cfg.MaxAttempts, "err", errMsg)
	}
}

// advanceBackoff doubles backoffCur (seeding from BackoffBase if currently
// 0), capped at BackoffMax. If BackoffBase is 0 (not configured), this is a
// no-op — backoff is disabled. Integer-overflow-safe: we compare against
// BackoffMax BEFORE the multiply, so a future BackoffMax of max-int64 -
// which is unrepresentable in reasonable configs but we check anyway -
// does not wrap.
func (e *Extractor) advanceBackoff() {
	if e.cfg.BackoffBase <= 0 {
		return // backoff not configured; no-op
	}
	if e.backoffCur <= 0 {
		e.backoffCur = e.cfg.BackoffBase
		if e.cfg.BackoffMax > 0 && e.backoffCur > e.cfg.BackoffMax {
			e.backoffCur = e.cfg.BackoffMax
		}
		return
	}
	if e.cfg.BackoffMax > 0 && e.backoffCur >= e.cfg.BackoffMax/2 {
		// Doubling would exceed or equal BackoffMax; clamp directly.
		e.backoffCur = e.cfg.BackoffMax
		return
	}
	e.backoffCur *= 2
	if e.cfg.BackoffMax > 0 && e.backoffCur > e.cfg.BackoffMax {
		e.backoffCur = e.cfg.BackoffMax
	}
}

// callLLM sends the extractor prompt to the hermes.Client and collects
// the full streamed response. Returns raw JSON (not yet validated).
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
			// fakeStream in tests returns a sentinel when exhausted; if we
			// already have content, treat as clean end.
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
