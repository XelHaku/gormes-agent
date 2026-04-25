package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var diagnosticTables = []string{
	"turns",
	"turns_fts",
	"goncho_peer_cards",
	"goncho_conclusions",
	"goncho_conclusions_fts",
}

// DeadLetterSummary is the operator-facing shape for one recent turn that
// exhausted extractor retries and was parked in the dead-letter state.
type DeadLetterSummary struct {
	ID        int64
	SessionID string
	ChatID    string
	Attempts  int
	Error     string
}

// DeadLetterErrorSummary groups dead-letter turns by the persisted extractor
// error message so operators can spot repeated failure modes quickly.
type DeadLetterErrorSummary struct {
	Error string
	Count int
}

// SkippedSyncSummary is one interrupted/cancelled turn that deliberately
// stayed out of extraction and recall.
type SkippedSyncSummary struct {
	ID        int64
	SessionID string
	ChatID    string
	Reason    string
}

// ExtractorStatus is the Phase 3.E.4 read model behind `gormes memory status`.
type ExtractorStatus struct {
	QueueDepth         int
	DeadLetterCount    int
	SkippedSyncCount   int
	WorkerHealth       string
	ErrorSummary       []DeadLetterErrorSummary
	RecentDeadLetters  []DeadLetterSummary
	RecentSkippedSyncs []SkippedSyncSummary
}

// SchemaStatus is the operator-facing memory schema snapshot used by doctor
// commands. It intentionally reads the already-open database instead of
// opening or migrating a second store.
type SchemaStatus struct {
	Version        string          `json:"version"`
	CurrentVersion string          `json:"current_version"`
	Current        bool            `json:"current"`
	Tables         map[string]bool `json:"tables"`
}

// CurrentSchemaVersion returns the schema version this binary expects.
func CurrentSchemaVersion() string {
	return schemaVersion
}

// ReadSchemaStatus reports schema version and key table presence for memory
// and Goncho diagnostics.
func ReadSchemaStatus(ctx context.Context, db *sql.DB) (SchemaStatus, error) {
	if db == nil {
		return SchemaStatus{}, errors.New("memory: nil db")
	}

	status := SchemaStatus{
		CurrentVersion: schemaVersion,
		Tables:         make(map[string]bool, len(diagnosticTables)),
	}
	for _, table := range diagnosticTables {
		status.Tables[table] = false
	}

	if err := db.QueryRowContext(ctx, `SELECT v FROM schema_meta WHERE k = 'version'`).Scan(&status.Version); err != nil {
		return SchemaStatus{}, fmt.Errorf("memory: schema version: %w", err)
	}
	status.Current = status.Version == schemaVersion

	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return SchemaStatus{}, fmt.Errorf("memory: schema tables: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return SchemaStatus{}, fmt.Errorf("memory: scan schema table: %w", err)
		}
		if _, ok := status.Tables[name]; ok {
			status.Tables[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		return SchemaStatus{}, fmt.Errorf("memory: schema table rows: %w", err)
	}

	return status, nil
}

// ReadExtractorStatus summarizes extractor backlog and recent dead letters from
// the persisted SQLite turns table. The worker is async and ephemeral, so
// health is inferred from durable queue/dead-letter state instead of process
// liveness.
func ReadExtractorStatus(ctx context.Context, db *sql.DB, deadLetterLimit int) (ExtractorStatus, error) {
	if db == nil {
		return ExtractorStatus{}, errors.New("memory: nil db")
	}
	if deadLetterLimit <= 0 {
		deadLetterLimit = 5
	}

	var status ExtractorStatus
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM turns WHERE extracted = 0 AND cron = 0 AND memory_sync_status = 'ready'`).Scan(&status.QueueDepth); err != nil {
		return ExtractorStatus{}, fmt.Errorf("memory: queue depth: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM turns WHERE extracted = 2 AND cron = 0 AND memory_sync_status = 'ready'`).Scan(&status.DeadLetterCount); err != nil {
		return ExtractorStatus{}, fmt.Errorf("memory: dead-letter count: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM turns WHERE memory_sync_status = 'skipped' AND cron = 0`).Scan(&status.SkippedSyncCount); err != nil {
		return ExtractorStatus{}, fmt.Errorf("memory: skipped sync count: %w", err)
	}
	status.WorkerHealth = extractorWorkerHealth(status.QueueDepth, status.DeadLetterCount)

	summaryRows, err := db.QueryContext(ctx,
		`SELECT COALESCE(extraction_error, ''), COUNT(*)
		 FROM turns
		 WHERE extracted = 2 AND cron = 0 AND memory_sync_status = 'ready'
		 GROUP BY COALESCE(extraction_error, '')
		 ORDER BY COUNT(*) DESC, COALESCE(extraction_error, '') ASC`,
	)
	if err != nil {
		return ExtractorStatus{}, fmt.Errorf("memory: dead-letter summary: %w", err)
	}
	defer summaryRows.Close()
	for summaryRows.Next() {
		var item DeadLetterErrorSummary
		if err := summaryRows.Scan(&item.Error, &item.Count); err != nil {
			return ExtractorStatus{}, fmt.Errorf("memory: scan dead-letter summary: %w", err)
		}
		status.ErrorSummary = append(status.ErrorSummary, item)
	}
	if err := summaryRows.Err(); err != nil {
		return ExtractorStatus{}, fmt.Errorf("memory: dead-letter summary rows: %w", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT id, session_id, chat_id, extraction_attempts, COALESCE(extraction_error, '')
		 FROM turns
		 WHERE extracted = 2 AND cron = 0 AND memory_sync_status = 'ready'
		 ORDER BY id DESC
		 LIMIT ?`,
		deadLetterLimit,
	)
	if err != nil {
		return ExtractorStatus{}, fmt.Errorf("memory: recent dead letters: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dl DeadLetterSummary
		if err := rows.Scan(&dl.ID, &dl.SessionID, &dl.ChatID, &dl.Attempts, &dl.Error); err != nil {
			return ExtractorStatus{}, fmt.Errorf("memory: scan dead letter: %w", err)
		}
		status.RecentDeadLetters = append(status.RecentDeadLetters, dl)
	}
	if err := rows.Err(); err != nil {
		return ExtractorStatus{}, fmt.Errorf("memory: recent dead letters rows: %w", err)
	}

	skippedRows, err := db.QueryContext(ctx,
		`SELECT id, session_id, chat_id, COALESCE(memory_sync_reason, '')
		 FROM turns
		 WHERE memory_sync_status = 'skipped' AND cron = 0
		 ORDER BY id DESC
		 LIMIT ?`,
		deadLetterLimit,
	)
	if err != nil {
		return ExtractorStatus{}, fmt.Errorf("memory: recent skipped syncs: %w", err)
	}
	defer skippedRows.Close()
	for skippedRows.Next() {
		var item SkippedSyncSummary
		if err := skippedRows.Scan(&item.ID, &item.SessionID, &item.ChatID, &item.Reason); err != nil {
			return ExtractorStatus{}, fmt.Errorf("memory: scan skipped sync: %w", err)
		}
		status.RecentSkippedSyncs = append(status.RecentSkippedSyncs, item)
	}
	if err := skippedRows.Err(); err != nil {
		return ExtractorStatus{}, fmt.Errorf("memory: recent skipped sync rows: %w", err)
	}

	return status, nil
}

func extractorWorkerHealth(queueDepth, deadLetterCount int) string {
	switch {
	case deadLetterCount > 0:
		return "degraded"
	case queueDepth > 0:
		return "backlog"
	default:
		return "idle"
	}
}
