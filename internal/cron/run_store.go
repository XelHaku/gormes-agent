package cron

import (
	"context"
	"database/sql"
	"fmt"
)

// Run is one scheduled fire's audit record. Written by the Executor
// after each run (success, timeout, error, or suppressed).
type Run struct {
	ID                int64  // auto-assigned by SQLite
	JobID             string // foreign reference to bbolt Job.ID; no FK enforced
	StartedAt         int64  // unix seconds at execution start
	FinishedAt        int64  // unix seconds at completion; 0 when never finished
	PromptHash        string // sha256 hex of Job.Prompt BEFORE Heartbeat prefix; 16-hex-char prefix
	Status            string // "success" | "timeout" | "error" | "suppressed"
	Delivered         bool
	SuppressionReason string // "silent" | "empty" | "" — NULL when empty
	OutputPreview     string // first 200 chars of final response (or failure notice)
	ErrorMsg          string // populated on status="error" or "timeout"
}

// RunStore writes to the cron_runs SQLite table. Read path is rare
// (CRON.md mirror only); writes happen once per job fire.
type RunStore struct {
	db *sql.DB
}

// NewRunStore wraps an open *sql.DB. The cron_runs table must exist —
// it's created by migration 3d->3e (internal/memory/schema.go).
func NewRunStore(db *sql.DB) *RunStore {
	return &RunStore{db: db}
}

// RecordRun persists one run. The SQL CHECK constraints catch invalid
// status / suppression_reason values, so the caller gets an error
// rather than garbage in the audit log.
func (s *RunStore) RecordRun(ctx context.Context, r Run) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cron_runs
		  (job_id, started_at, finished_at, prompt_hash, status,
		   delivered, suppression_reason, output_preview, error_msg)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.JobID,
		r.StartedAt,
		runFinishedAtValue(r.FinishedAt),
		r.PromptHash,
		r.Status,
		runBoolToInt(r.Delivered),
		runNullIfEmpty(r.SuppressionReason),
		runNullIfEmpty(r.OutputPreview),
		runNullIfEmpty(r.ErrorMsg),
	)
	if err != nil {
		return fmt.Errorf("cron: record run: %w", err)
	}
	return nil
}

// LatestRuns returns up to `limit` most-recent runs for the given
// job_id (started_at DESC). Used by the CRON.md mirror.
func (s *RunStore) LatestRuns(ctx context.Context, jobID string, limit int) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, job_id, started_at, COALESCE(finished_at,0), prompt_hash,
		       status, delivered,
		       COALESCE(suppression_reason, ''), COALESCE(output_preview, ''),
		       COALESCE(error_msg, '')
		FROM cron_runs
		WHERE job_id = ?
		ORDER BY started_at DESC
		LIMIT ?`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Run
	for rows.Next() {
		var r Run
		var delivered int
		if err := rows.Scan(&r.ID, &r.JobID, &r.StartedAt, &r.FinishedAt,
			&r.PromptHash, &r.Status, &delivered,
			&r.SuppressionReason, &r.OutputPreview, &r.ErrorMsg); err != nil {
			return nil, err
		}
		r.Delivered = delivered != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// LatestCompletedOutput returns the most recent non-empty output preview for a
// completed run of jobID. It intentionally reads the native cron run audit
// instead of any file-based output directory.
func (s *RunStore) LatestCompletedOutput(ctx context.Context, jobID string) (string, bool, error) {
	var output string
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(output_preview, '')
		FROM cron_runs
		WHERE job_id = ?
		  AND finished_at IS NOT NULL
		  AND COALESCE(output_preview, '') <> ''
		ORDER BY started_at DESC, id DESC
		LIMIT 1`, jobID).Scan(&output)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return output, true, nil
}

// runFinishedAtValue returns nil for a zero FinishedAt so the column
// stays NULL (distinguishable from a genuinely-zero finish timestamp).
func runFinishedAtValue(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}

// runNullIfEmpty returns nil for empty strings so the database column
// stays NULL. Used for suppression_reason, output_preview, error_msg.
// Named with a `run` prefix to avoid colliding with nullIfEmpty in
// internal/memory (different package, different semantics).
func runNullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// runBoolToInt is the SQLite-idiomatic bool -> INTEGER conversion.
func runBoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
