package subagent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrDurableJobNotFound = errors.New("subagent: durable job not found")

type DurableJobStatus string

const (
	DurableJobWaiting         DurableJobStatus = "waiting"
	DurableJobActive          DurableJobStatus = "active"
	DurableJobWaitingChildren DurableJobStatus = "waiting-children"
	DurableJobCompleted       DurableJobStatus = "completed"
	DurableJobFailed          DurableJobStatus = "failed"
	DurableJobCancelled       DurableJobStatus = "cancelled"
)

type DurableChildOutcome string

const (
	DurableChildCompleted DurableChildOutcome = "complete"
	DurableChildFailed    DurableChildOutcome = "failed"
	DurableChildCancelled DurableChildOutcome = "cancelled"
)

type DurableJobSubmission struct {
	ID       string
	Kind     WorkKind
	ParentID string
	Depth    int
	Progress json.RawMessage
}

type DurableClaim struct {
	WorkerID  string
	LockUntil time.Time
	Kinds     []WorkKind
}

type DurableJob struct {
	ID              string
	Kind            WorkKind
	Status          DurableJobStatus
	ParentID        string
	Depth           int
	Progress        json.RawMessage
	Result          json.RawMessage
	ErrorText       string
	CancelRequested bool
	CancelReason    string
	LockOwner       string
	LockUntil       time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	StartedAt       time.Time
	FinishedAt      time.Time
}

type DurableChildEvent struct {
	ID        int64
	ParentID  string
	ChildID   string
	JobKind   WorkKind
	Outcome   DurableChildOutcome
	ErrorText string
	Payload   json.RawMessage
	CreatedAt time.Time
}

type DurableLedgerStatus struct {
	ReplayAvailable bool
	Total           int
	Waiting         int
	Active          int
	Stalled         int
	CancelRequested int
}

type DurableLedger struct {
	db *sql.DB
}

func NewDurableLedger(db *sql.DB) (*DurableLedger, error) {
	if db == nil {
		return nil, errors.New("subagent: durable ledger db is nil")
	}
	if _, err := db.Exec(durableLedgerSchema); err != nil {
		return nil, fmt.Errorf("subagent: init durable ledger: %w", err)
	}
	return &DurableLedger{db: db}, nil
}

func (l *DurableLedger) Submit(ctx context.Context, sub DurableJobSubmission) (DurableJob, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, errors.New("subagent: durable ledger is nil")
	}
	id := strings.TrimSpace(sub.ID)
	if id == "" {
		return DurableJob{}, errors.New("subagent: durable job id is empty")
	}
	if !validDurableKind(sub.Kind) {
		return DurableJob{}, fmt.Errorf("subagent: unsupported durable job kind %q", sub.Kind)
	}
	progress, err := durableJSON(sub.Progress, `{}`)
	if err != nil {
		return DurableJob{}, fmt.Errorf("subagent: progress json: %w", err)
	}

	now := durableNow()
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return DurableJob{}, err
	}
	defer tx.Rollback()

	parentID := strings.TrimSpace(sub.ParentID)
	depth := sub.Depth
	if parentID != "" {
		parent, err := durableGet(ctx, tx, parentID)
		if err != nil && !errors.Is(err, ErrDurableJobNotFound) {
			return DurableJob{}, err
		}
		if err == nil {
			depth = parent.Depth + 1
			if _, err := tx.ExecContext(ctx, `
				UPDATE durable_jobs
				SET status = ?, updated_at = ?
				WHERE id = ? AND status IN (?, ?)`,
				DurableJobWaitingChildren, now, parentID, DurableJobWaiting, DurableJobActive); err != nil {
				return DurableJob{}, err
			}
		}
	}
	if depth < 0 {
		return DurableJob{}, errors.New("subagent: durable job depth cannot be negative")
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO durable_jobs
			(id, kind, status, parent_id, depth, progress_json, result_json,
			 error_text, cancel_requested, cancel_reason, lock_owner,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, '{}', '', 0, '', '', ?, ?)`,
		id, sub.Kind, DurableJobWaiting, parentID, depth, progress, now, now)
	if err != nil {
		return DurableJob{}, fmt.Errorf("subagent: submit durable job: %w", err)
	}

	job, err := durableGet(ctx, tx, id)
	if err != nil {
		return DurableJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return DurableJob{}, err
	}
	return job, nil
}

func (l *DurableLedger) Claim(ctx context.Context, claim DurableClaim) (DurableJob, bool, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, false, errors.New("subagent: durable ledger is nil")
	}
	now := durableNow()
	query, args := durableClaimSelectSQL(claim.Kinds, now)

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return DurableJob{}, false, err
	}
	defer tx.Rollback()

	var id string
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DurableJob{}, false, nil
		}
		return DurableJob{}, false, err
	}
	job, ok, err := durableClaimJob(ctx, tx, id, claim, now)
	if err != nil || !ok {
		return DurableJob{}, ok, err
	}
	if err := tx.Commit(); err != nil {
		return DurableJob{}, false, err
	}
	return job, true, nil
}

func (l *DurableLedger) ClaimJob(ctx context.Context, id string, claim DurableClaim) (DurableJob, bool, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, false, errors.New("subagent: durable ledger is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return DurableJob{}, false, errors.New("subagent: durable job id is empty")
	}
	now := durableNow()

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return DurableJob{}, false, err
	}
	defer tx.Rollback()

	job, ok, err := durableClaimJob(ctx, tx, id, claim, now)
	if err != nil || !ok {
		return DurableJob{}, ok, err
	}
	if err := tx.Commit(); err != nil {
		return DurableJob{}, false, err
	}
	return job, true, nil
}

func (l *DurableLedger) Renew(ctx context.Context, id, workerID string, lockUntil time.Time) (bool, error) {
	if l == nil || l.db == nil {
		return false, errors.New("subagent: durable ledger is nil")
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return false, errors.New("subagent: worker id is empty")
	}
	res, err := l.db.ExecContext(ctx, `
		UPDATE durable_jobs
		SET lock_until = ?, updated_at = ?
		WHERE id = ? AND status = ? AND lock_owner = ? AND cancel_requested = 0`,
		durableLockUntil(lockUntil), durableNow(), id, DurableJobActive, workerID)
	if err != nil {
		return false, err
	}
	return durableRowsAffected(res)
}

func (l *DurableLedger) UpdateProgress(ctx context.Context, id, workerID string, progress json.RawMessage) (bool, error) {
	if l == nil || l.db == nil {
		return false, errors.New("subagent: durable ledger is nil")
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return false, errors.New("subagent: worker id is empty")
	}
	progressJSON, err := durableJSON(progress, `{}`)
	if err != nil {
		return false, fmt.Errorf("subagent: progress json: %w", err)
	}
	res, err := l.db.ExecContext(ctx, `
		UPDATE durable_jobs
		SET progress_json = ?, updated_at = ?
		WHERE id = ? AND status = ? AND lock_owner = ? AND cancel_requested = 0`,
		progressJSON, durableNow(), id, DurableJobActive, workerID)
	if err != nil {
		return false, err
	}
	return durableRowsAffected(res)
}

func (l *DurableLedger) Complete(ctx context.Context, id, workerID string, result json.RawMessage) (DurableJob, bool, error) {
	return l.terminal(ctx, id, workerID, DurableJobCompleted, result, "", DurableChildCompleted)
}

func (l *DurableLedger) Fail(ctx context.Context, id, workerID, errorText string) (DurableJob, bool, error) {
	return l.terminal(ctx, id, workerID, DurableJobFailed, nil, errorText, DurableChildFailed)
}

func (l *DurableLedger) Cancel(ctx context.Context, id, reason string) (DurableJob, bool, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, false, errors.New("subagent: durable ledger is nil")
	}
	now := durableNow()
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return DurableJob{}, false, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE durable_jobs
		SET status = ?, cancel_requested = 1, cancel_reason = ?, lock_owner = '',
		    lock_until = NULL, finished_at = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?, ?)`,
		DurableJobCancelled, strings.TrimSpace(reason), now, now, id,
		DurableJobWaiting, DurableJobActive, DurableJobWaitingChildren)
	if err != nil {
		return DurableJob{}, false, err
	}
	ok, err := durableRowsAffected(res)
	if err != nil || !ok {
		return DurableJob{}, ok, err
	}
	job, err := durableGet(ctx, tx, id)
	if err != nil {
		return DurableJob{}, false, err
	}
	if job.ParentID != "" {
		if err := durableInsertChildEvent(ctx, tx, job, DurableChildCancelled, reason); err != nil {
			return DurableJob{}, false, err
		}
		if err := durableResolveParent(ctx, tx, job.ParentID); err != nil {
			return DurableJob{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return DurableJob{}, false, err
	}
	return job, true, nil
}

func (l *DurableLedger) Get(ctx context.Context, id string) (DurableJob, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, errors.New("subagent: durable ledger is nil")
	}
	return durableGet(ctx, l.db, id)
}

func (l *DurableLedger) ChildEvents(ctx context.Context, parentID string) ([]DurableChildEvent, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("subagent: durable ledger is nil")
	}
	rows, err := l.db.QueryContext(ctx, `
		SELECT id, job_id, child_id, job_kind, outcome, error_text, payload_json, created_at
		FROM durable_job_events
		WHERE job_id = ? AND type = 'child_done'
		ORDER BY id`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DurableChildEvent
	for rows.Next() {
		var ev DurableChildEvent
		var payload string
		var created int64
		if err := rows.Scan(&ev.ID, &ev.ParentID, &ev.ChildID, &ev.JobKind,
			&ev.Outcome, &ev.ErrorText, &payload, &created); err != nil {
			return nil, err
		}
		ev.Payload = json.RawMessage(payload)
		ev.CreatedAt = durableTime(created)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (l *DurableLedger) Status(ctx context.Context) (DurableLedgerStatus, error) {
	if l == nil || l.db == nil {
		return DurableLedgerStatus{}, errors.New("subagent: durable ledger is nil")
	}
	st := DurableLedgerStatus{ReplayAvailable: true}
	rows, err := l.db.QueryContext(ctx, `
		SELECT status, COUNT(*)
		FROM durable_jobs
		GROUP BY status`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var status DurableJobStatus
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return st, err
		}
		st.Total += n
		switch status {
		case DurableJobWaiting:
			st.Waiting = n
		case DurableJobActive:
			st.Active = n
		}
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_jobs
		WHERE status = ? AND lock_until IS NOT NULL AND lock_until < ?`,
		DurableJobActive, durableNow()).Scan(&st.Stalled)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_jobs
		WHERE cancel_requested = 1`).Scan(&st.CancelRequested)
	return st, nil
}

func (l *DurableLedger) terminal(ctx context.Context, id, workerID string, status DurableJobStatus, result json.RawMessage, errorText string, outcome DurableChildOutcome) (DurableJob, bool, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, false, errors.New("subagent: durable ledger is nil")
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return DurableJob{}, false, errors.New("subagent: worker id is empty")
	}
	resultJSON, err := durableJSON(result, `{}`)
	if err != nil {
		return DurableJob{}, false, fmt.Errorf("subagent: result json: %w", err)
	}
	now := durableNow()
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return DurableJob{}, false, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE durable_jobs
		SET status = ?, result_json = ?, error_text = ?, lock_owner = '',
		    lock_until = NULL, finished_at = ?, updated_at = ?
		WHERE id = ? AND status = ? AND lock_owner = ? AND cancel_requested = 0`,
		status, resultJSON, strings.TrimSpace(errorText), now, now, id, DurableJobActive, workerID)
	if err != nil {
		return DurableJob{}, false, err
	}
	ok, err := durableRowsAffected(res)
	if err != nil || !ok {
		return DurableJob{}, ok, err
	}
	job, err := durableGet(ctx, tx, id)
	if err != nil {
		return DurableJob{}, false, err
	}
	if job.ParentID != "" {
		if err := durableInsertChildEvent(ctx, tx, job, outcome, errorText); err != nil {
			return DurableJob{}, false, err
		}
		if err := durableResolveParent(ctx, tx, job.ParentID); err != nil {
			return DurableJob{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return DurableJob{}, false, err
	}
	return job, true, nil
}

type durableQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func durableGet(ctx context.Context, q durableQuerier, id string) (DurableJob, error) {
	var j DurableJob
	var progress, result string
	var cancelRequested int
	var created, updated int64
	var started, finished, lockUntil sql.NullInt64
	err := q.QueryRowContext(ctx, durableJobSelectSQL+` WHERE id = ?`, id).Scan(
		&j.ID, &j.Kind, &j.Status, &j.ParentID, &j.Depth, &progress, &result,
		&j.ErrorText, &cancelRequested, &j.CancelReason, &j.LockOwner,
		&lockUntil, &created, &updated, &started, &finished,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DurableJob{}, ErrDurableJobNotFound
	}
	if err != nil {
		return DurableJob{}, err
	}
	j.Progress = json.RawMessage(progress)
	j.Result = json.RawMessage(result)
	j.CancelRequested = cancelRequested != 0
	j.CreatedAt = durableTime(created)
	j.UpdatedAt = durableTime(updated)
	j.LockUntil = durableNullTime(lockUntil)
	j.StartedAt = durableNullTime(started)
	j.FinishedAt = durableNullTime(finished)
	return j, nil
}

func durableClaimJob(ctx context.Context, tx *sql.Tx, id string, claim DurableClaim, now int64) (DurableJob, bool, error) {
	workerID := strings.TrimSpace(claim.WorkerID)
	if workerID == "" {
		return DurableJob{}, false, errors.New("subagent: worker id is empty")
	}
	lockUntil := durableLockUntil(claim.LockUntil)
	res, err := tx.ExecContext(ctx, `
		UPDATE durable_jobs
		SET status = ?, lock_owner = ?, lock_until = ?,
		    started_at = COALESCE(started_at, ?), updated_at = ?
		WHERE id = ? AND cancel_requested = 0 AND (
			status = ? OR (status = ? AND lock_until IS NOT NULL AND lock_until < ?)
		)`,
		DurableJobActive, workerID, lockUntil, now, now, id,
		DurableJobWaiting, DurableJobActive, now)
	if err != nil {
		return DurableJob{}, false, err
	}
	ok, err := durableRowsAffected(res)
	if err != nil || !ok {
		return DurableJob{}, ok, err
	}
	job, err := durableGet(ctx, tx, id)
	if err != nil {
		return DurableJob{}, false, err
	}
	return job, true, nil
}

func durableClaimSelectSQL(kinds []WorkKind, now int64) (string, []any) {
	var b strings.Builder
	b.WriteString(`SELECT id FROM durable_jobs
		WHERE cancel_requested = 0
		  AND (status = ? OR (status = ? AND lock_until IS NOT NULL AND lock_until < ?))`)
	args := []any{DurableJobWaiting, DurableJobActive, now}
	if len(kinds) > 0 {
		b.WriteString(` AND kind IN (`)
		for i, kind := range kinds {
			if i > 0 {
				b.WriteString(`,`)
			}
			b.WriteString(`?`)
			args = append(args, kind)
		}
		b.WriteString(`)`)
	}
	b.WriteString(` ORDER BY created_at, id LIMIT 1`)
	return b.String(), args
}

func durableInsertChildEvent(ctx context.Context, tx *sql.Tx, job DurableJob, outcome DurableChildOutcome, errorText string) error {
	payload := map[string]any{
		"type":     "child_done",
		"child_id": job.ID,
		"job_kind": string(job.Kind),
		"outcome":  string(outcome),
	}
	if strings.TrimSpace(errorText) != "" {
		payload["error"] = strings.TrimSpace(errorText)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO durable_job_events
			(job_id, type, child_id, job_kind, outcome, error_text, payload_json, created_at)
		VALUES (?, 'child_done', ?, ?, ?, ?, ?, ?)`,
		job.ParentID, job.ID, job.Kind, outcome, strings.TrimSpace(errorText), string(raw), durableNow())
	return err
}

func durableResolveParent(ctx context.Context, tx *sql.Tx, parentID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE durable_jobs
		SET status = ?, updated_at = ?
		WHERE id = ? AND status = ? AND NOT EXISTS (
			SELECT 1 FROM durable_jobs
			WHERE parent_id = ?
			  AND status NOT IN (?, ?, ?)
		)`,
		DurableJobWaiting, durableNow(), parentID, DurableJobWaitingChildren, parentID,
		DurableJobCompleted, DurableJobFailed, DurableJobCancelled)
	return err
}

func durableRowsAffected(res sql.Result) (bool, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func durableJSON(raw json.RawMessage, fallback string) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		trimmed = fallback
	}
	if !json.Valid([]byte(trimmed)) {
		return "", errors.New("invalid JSON")
	}
	return trimmed, nil
}

func durableLockUntil(t time.Time) int64 {
	if t.IsZero() {
		t = time.Now().UTC().Add(5 * time.Minute)
	}
	return t.UTC().UnixNano()
}

func durableNow() int64 {
	return time.Now().UTC().UnixNano()
}

func durableTime(ns int64) time.Time {
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns).UTC()
}

func durableNullTime(ns sql.NullInt64) time.Time {
	if !ns.Valid {
		return time.Time{}
	}
	return durableTime(ns.Int64)
}

func validDurableKind(kind WorkKind) bool {
	switch kind {
	case WorkKindShellCommand, WorkKindCronJob, WorkKindLLMSubagent:
		return true
	default:
		return false
	}
}

const durableJobSelectSQL = `
	SELECT id, kind, status, parent_id, depth, progress_json, result_json,
	       error_text, cancel_requested, cancel_reason, lock_owner, lock_until,
	       created_at, updated_at, started_at, finished_at
	FROM durable_jobs`

const durableLedgerSchema = `
CREATE TABLE IF NOT EXISTS durable_jobs (
	id               TEXT PRIMARY KEY,
	kind             TEXT    NOT NULL CHECK(kind IN ('shell_command','cron_job','llm_subagent')),
	status           TEXT    NOT NULL CHECK(status IN ('waiting','active','waiting-children','completed','failed','cancelled')),
	parent_id        TEXT    NOT NULL DEFAULT '',
	depth            INTEGER NOT NULL DEFAULT 0 CHECK(depth >= 0),
	progress_json    TEXT    NOT NULL DEFAULT '{}',
	result_json      TEXT    NOT NULL DEFAULT '{}',
	error_text       TEXT    NOT NULL DEFAULT '',
	cancel_requested INTEGER NOT NULL DEFAULT 0 CHECK(cancel_requested IN (0,1)),
	cancel_reason    TEXT    NOT NULL DEFAULT '',
	lock_owner       TEXT    NOT NULL DEFAULT '',
	lock_until       INTEGER,
	created_at       INTEGER NOT NULL,
	updated_at       INTEGER NOT NULL,
	started_at       INTEGER,
	finished_at      INTEGER
);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_claim
	ON durable_jobs(status, kind, created_at);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_parent
	ON durable_jobs(parent_id);

CREATE TABLE IF NOT EXISTS durable_job_events (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	job_id       TEXT    NOT NULL,
	type         TEXT    NOT NULL,
	child_id     TEXT    NOT NULL DEFAULT '',
	job_kind     TEXT    NOT NULL DEFAULT '',
	outcome      TEXT    NOT NULL DEFAULT '',
	error_text   TEXT    NOT NULL DEFAULT '',
	payload_json TEXT    NOT NULL,
	created_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_durable_job_events_job
	ON durable_job_events(job_id, id);
`
