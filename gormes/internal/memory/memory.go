// Package memory is the SQLite-backed Phase-3.A Lattice Foundation.
// It implements store.Store with fire-and-forget semantics: Exec returns
// an Ack in microseconds after enqueueing a Command on a bounded channel;
// a single-owner background worker performs all SQL I/O. On queue-full:
// log + drop. See gormes/docs/superpowers/specs/2026-04-20-gormes-phase3a-memory-design.md.
package memory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	_ "github.com/ncruces/go-sqlite3/driver"

	"github.com/XelHaku/golang-hermes-agent/gormes/internal/store"
)

// defaultQueueCap is used when OpenSqlite receives queueCap <= 0.
const defaultQueueCap = 1024

// Stats exposes counters for tests and future telemetry. Not part of
// store.Store — consumers must hold a concrete *SqliteStore to call Stats.
type Stats struct {
	QueueLen int
	QueueCap int
	Drops    uint64
	Accepted uint64
}

// SqliteStore is a fire-and-forget store.Store backed by SQLite + FTS5.
type SqliteStore struct {
	db    *sql.DB
	queue chan store.Command
	done  chan struct{}
	log   *slog.Logger

	drops    atomic.Uint64
	accepted atomic.Uint64

	closeOnce sync.Once
}

// Compile-time interface check.
var _ store.Store = (*SqliteStore)(nil)

// OpenSqlite opens/creates the SQLite file at path, applies the schema,
// and starts the background worker goroutine. queueCap <= 0 falls back
// to defaultQueueCap. log == nil falls back to slog.Default().
func OpenSqlite(path string, queueCap int, log *slog.Logger) (*SqliteStore, error) {
	if log == nil {
		log = slog.Default()
	}
	if queueCap <= 0 {
		queueCap = defaultQueueCap
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("memory: create parent dir for %s: %w", path, err)
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("memory: open %s: %w", path, err)
	}
	// Single writer connection (ncruces/go-sqlite3 WASM: each connection owns
	// its own WASM memory; one connection keeps the footprint minimal and
	// matches our single-owner worker goroutine anyway).
	db.SetMaxOpenConns(1)

	if err := applyPragmas(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory: pragmas: %w", err)
	}
	if _, err := db.Exec(schemaDDL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("memory: apply schema: %w", err)
	}

	s := &SqliteStore{
		db:    db,
		queue: make(chan store.Command, queueCap),
		done:  make(chan struct{}, 1),
		log:   log,
	}
	go s.run()
	return s, nil
}

func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 2000",
		"PRAGMA foreign_keys = ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

// Exec is Task 4's scope — stub for Task 3.
func (s *SqliteStore) Exec(ctx context.Context, cmd store.Command) (store.Ack, error) {
	return store.Ack{}, nil
}

// Stats returns a snapshot of worker counters.
func (s *SqliteStore) Stats() Stats {
	return Stats{
		QueueLen: len(s.queue),
		QueueCap: cap(s.queue),
		Drops:    s.drops.Load(),
		Accepted: s.accepted.Load(),
	}
}

// DB returns the underlying *sql.DB handle. Exposed for read-only test
// verification; production callers should not depend on this.
func (s *SqliteStore) DB() *sql.DB { return s.db }

// Close is Task 7's scope — simple stub for Task 3: close queue, wait
// for worker to exit, close DB. The ctx-deadline honouring comes in T7.
func (s *SqliteStore) Close(ctx context.Context) error {
	var err error
	s.closeOnce.Do(func() {
		close(s.queue)
		<-s.done
		err = s.db.Close()
	})
	return err
}

// run is Task 5's scope — stub for Task 3: drain the queue (drop everything)
// so Close can return cleanly.
func (s *SqliteStore) run() {
	defer close(s.done)
	for range s.queue {
		// Task 5 replaces with real handleCommand dispatch.
	}
}
