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
var ErrDurableBackpressure = errors.New("subagent: durable queue backpressure")
var ErrDurableLifecycleDenied = errors.New("subagent: durable lifecycle control denied")
var ErrDurableReplayUnavailable = errors.New("subagent: durable replay unavailable")
var ErrDurableInboxClaimDenied = errors.New("subagent: durable inbox claim denied")

const durableStaleWaitingAfter = time.Hour

type DurableJobStatus string

const (
	DurableJobWaiting         DurableJobStatus = "waiting"
	DurableJobActive          DurableJobStatus = "active"
	DurableJobWaitingChildren DurableJobStatus = "waiting-children"
	DurableJobPaused          DurableJobStatus = "paused"
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

const durableWorkerHeartbeatStaleAfter = 5 * time.Minute

type DurableWorkerLiveness string

const (
	DurableWorkerNoWorker       DurableWorkerLiveness = "no-worker"
	DurableWorkerHealthy        DurableWorkerLiveness = "healthy"
	DurableWorkerStaleHeartbeat DurableWorkerLiveness = "stale-heartbeat"
)

type DurableSupervisorAvailability string

const (
	DurableSupervisorAvailable   DurableSupervisorAvailability = "available"
	DurableSupervisorUnavailable DurableSupervisorAvailability = "supervisor-unavailable"
)

type DurableJobSubmission struct {
	ID         string
	Kind       WorkKind
	ParentID   string
	Depth      int
	Progress   json.RawMessage
	MaxWaiting int
}

type DurableReplayRequest struct {
	ID            string
	DataOverrides json.RawMessage
	RequestedAt   time.Time
}

type DurableClaim struct {
	WorkerID  string
	LockUntil time.Time
	TimeoutAt time.Time
	Timeout   time.Duration
	Kinds     []WorkKind
}

type DurableJobListFilter struct {
	Kind   WorkKind
	Status DurableJobStatus
}

type DurableInboxMessageSubmission struct {
	JobID       string
	Sender      string
	SenderTrust TrustClass
	Payload     json.RawMessage
	SentAt      time.Time
}

type DurableInboxClaim struct {
	Trust     TrustClass
	JobID     string
	Actor     string
	ClaimedAt time.Time
}

type DurableLedgerOptions struct {
	MaxWaiting int
}

type DurableBackpressureError struct {
	JobID      string
	Waiting    int
	MaxWaiting int
}

func (e DurableBackpressureError) Error() string {
	return fmt.Sprintf("subagent: durable queue backpressure: %d waiting jobs reached maxWaiting=%d", e.Waiting, e.MaxWaiting)
}

func (e DurableBackpressureError) Is(target error) bool {
	return target == ErrDurableBackpressure
}

type DurableJob struct {
	ID                string
	Kind              WorkKind
	Status            DurableJobStatus
	ParentID          string
	ReplayOf          string
	Depth             int
	Progress          json.RawMessage
	DataOverrides     json.RawMessage
	Result            json.RawMessage
	ErrorText         string
	CancelRequested   bool
	CancelReason      string
	PauseActor        string
	PauseReason       string
	PausedAt          time.Time
	ResumeActor       string
	ResumeReason      string
	ResumeRequestedAt time.Time
	LockOwner         string
	LockUntil         time.Time
	TimeoutAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	StartedAt         time.Time
	FinishedAt        time.Time
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

type DurableInboxMessage struct {
	ID          int64
	JobID       string
	Sender      string
	SenderTrust TrustClass
	Payload     json.RawMessage
	UnreadAt    time.Time
	ReadAt      time.Time
	ReadBy      string
}

type DurableWorkerHeartbeat struct {
	WorkerID    string
	HeartbeatAt time.Time
}

type DurableSupervisorReport struct {
	Available  bool
	Reason     string
	ReportedAt time.Time
}

type DurableWorkerRestartIntent struct {
	WorkerID     string
	Reason       string
	SupervisorID string
	RequestedAt  time.Time
}

type DurableLifecycleIntent struct {
	Trust       TrustClass
	Actor       string
	Reason      string
	RequestedAt time.Time
}

type DurableWorkerRestartStatus struct {
	Requested    bool
	WorkerID     string
	Reason       string
	SupervisorID string
	RequestedAt  time.Time
	AuditEvents  int
}

type DurableWorkerAbortEvent struct {
	JobID     string
	WorkerID  string
	Reason    string
	CreatedAt time.Time
}

type DurableWorkerAbortRecoveryStatus struct {
	AbortSignalSent          int
	AbortSlotRecovered       int
	HandlerIgnoredAbort      int
	AbortRecoveryUnavailable int
	LastEvent                string
	LastJobID                string
	LastWorkerID             string
	LastReason               string
	LastEventAt              time.Time
}

type DurableWorkerStatus struct {
	Liveness             DurableWorkerLiveness
	WorkerID             string
	LastHeartbeat        time.Time
	HeartbeatStaleAfter  time.Duration
	DegradedReason       string
	Supervisor           DurableSupervisorAvailability
	SupervisorReason     string
	SupervisorReportedAt time.Time
	RestartIntent        DurableWorkerRestartStatus
	AbortRecovery        DurableWorkerAbortRecoveryStatus
}

type DurableLedgerStatus struct {
	ReplayAvailable             bool
	ReplayUnavailable           int
	Total                       int
	Waiting                     int
	Active                      int
	Claimed                     int
	Stalled                     int
	TimeoutScheduled            int
	TimedOut                    int
	StaleWaiting                int
	BackpressureDenied          int
	Paused                      int
	ResumePending               int
	LifecycleControlUnsupported int
	QueueFull                   bool
	MaxWaiting                  int
	CancelRequested             int
	InboxUnread                 int
	ProtectedSubmitDenied       int
	Worker                      DurableWorkerStatus
}

type DurableLedger struct {
	db         *sql.DB
	maxWaiting int
}

func NewDurableLedger(db *sql.DB) (*DurableLedger, error) {
	return NewDurableLedgerWithOptions(db, DurableLedgerOptions{})
}

func NewDurableLedgerWithOptions(db *sql.DB, opts DurableLedgerOptions) (*DurableLedger, error) {
	if db == nil {
		return nil, errors.New("subagent: durable ledger db is nil")
	}
	if opts.MaxWaiting < 0 {
		return nil, errors.New("subagent: durable ledger max waiting cannot be negative")
	}
	if _, err := db.Exec(durableLedgerSchema); err != nil {
		return nil, fmt.Errorf("subagent: init durable ledger: %w", err)
	}
	if err := durableLedgerMigrate(db); err != nil {
		return nil, fmt.Errorf("subagent: migrate durable ledger: %w", err)
	}
	if _, err := db.Exec(durableLedgerPostMigrationSchema); err != nil {
		return nil, fmt.Errorf("subagent: index durable ledger: %w", err)
	}
	return &DurableLedger{db: db, maxWaiting: opts.MaxWaiting}, nil
}

func (l *DurableLedger) SubmitWithTrust(ctx context.Context, trust TrustClass, sub DurableJobSubmission) (DurableJob, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, errors.New("subagent: durable ledger is nil")
	}
	policy := DefaultMinionRoutingPolicy()
	if !policy.CanSubmit(trust, sub.Kind) {
		if err := l.recordProtectedSubmitDenied(ctx, trust, sub); err != nil {
			return DurableJob{}, err
		}
		return DurableJob{}, fmt.Errorf("%w: %s cannot submit %s", ErrDurableRouteDenied, trust, sub.Kind)
	}
	return l.Submit(ctx, sub)
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

	maxWaiting := sub.MaxWaiting
	if maxWaiting <= 0 {
		maxWaiting = l.maxWaiting
	}
	if maxWaiting > 0 {
		waiting, err := durableCountWaiting(ctx, tx)
		if err != nil {
			return DurableJob{}, err
		}
		if waiting >= maxWaiting {
			if err := durableInsertBackpressureEvent(ctx, tx, id, sub.Kind, waiting, maxWaiting); err != nil {
				return DurableJob{}, err
			}
			if err := tx.Commit(); err != nil {
				return DurableJob{}, err
			}
			return DurableJob{}, DurableBackpressureError{
				JobID:      id,
				Waiting:    waiting,
				MaxWaiting: maxWaiting,
			}
		}
	}

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

func (l *DurableLedger) Pause(ctx context.Context, id string, intent DurableLifecycleIntent) (DurableJob, bool, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, false, errors.New("subagent: durable ledger is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return DurableJob{}, false, errors.New("subagent: durable job id is empty")
	}
	now := durableNow()
	requestedAt := durableLifecycleRequestedAt(intent, now)
	actor := durableLifecycleActor(intent)
	reason := strings.TrimSpace(intent.Reason)

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return DurableJob{}, false, err
	}
	defer tx.Rollback()

	job, err := durableGet(ctx, tx, id)
	if errors.Is(err, ErrDurableJobNotFound) {
		return DurableJob{}, false, nil
	}
	if err != nil {
		return DurableJob{}, false, err
	}
	if !durableLifecycleTrustAllowed(intent.Trust) {
		if err := durableInsertLifecycleEvent(ctx, tx, job, "lifecycle_control_unsupported", "pause", actor, reason, intent.Trust, requestedAt); err != nil {
			return DurableJob{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return DurableJob{}, false, err
		}
		return DurableJob{}, false, ErrDurableLifecycleDenied
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE durable_jobs
		SET status = ?, pause_actor = ?, pause_reason = ?, paused_at = ?,
		    resume_actor = '', resume_reason = '', resume_requested_at = NULL,
		    updated_at = ?
		WHERE id = ? AND cancel_requested = 0 AND status IN (?, ?)`,
		DurableJobPaused, actor, reason, requestedAt, now, id, DurableJobWaiting, DurableJobActive)
	if err != nil {
		return DurableJob{}, false, err
	}
	ok, err := durableRowsAffected(res)
	if err != nil || !ok {
		return DurableJob{}, ok, err
	}
	if err := durableInsertLifecycleEvent(ctx, tx, job, "pause_intent", "pause", actor, reason, intent.Trust, requestedAt); err != nil {
		return DurableJob{}, false, err
	}
	paused, err := durableGet(ctx, tx, id)
	if err != nil {
		return DurableJob{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return DurableJob{}, false, err
	}
	return paused, true, nil
}

func (l *DurableLedger) Resume(ctx context.Context, id string, intent DurableLifecycleIntent) (DurableJob, bool, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, false, errors.New("subagent: durable ledger is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return DurableJob{}, false, errors.New("subagent: durable job id is empty")
	}
	now := durableNow()
	requestedAt := durableLifecycleRequestedAt(intent, now)
	actor := durableLifecycleActor(intent)
	reason := strings.TrimSpace(intent.Reason)

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return DurableJob{}, false, err
	}
	defer tx.Rollback()

	job, err := durableGet(ctx, tx, id)
	if errors.Is(err, ErrDurableJobNotFound) {
		return DurableJob{}, false, nil
	}
	if err != nil {
		return DurableJob{}, false, err
	}
	if !durableLifecycleTrustAllowed(intent.Trust) {
		if err := durableInsertLifecycleEvent(ctx, tx, job, "lifecycle_control_unsupported", "resume", actor, reason, intent.Trust, requestedAt); err != nil {
			return DurableJob{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return DurableJob{}, false, err
		}
		return DurableJob{}, false, ErrDurableLifecycleDenied
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE durable_jobs
		SET status = ?, resume_actor = ?, resume_reason = ?, resume_requested_at = ?,
		    updated_at = ?
		WHERE id = ? AND cancel_requested = 0 AND status = ?`,
		DurableJobWaiting, actor, reason, requestedAt, now, id, DurableJobPaused)
	if err != nil {
		return DurableJob{}, false, err
	}
	ok, err := durableRowsAffected(res)
	if err != nil || !ok {
		return DurableJob{}, ok, err
	}
	if err := durableInsertLifecycleEvent(ctx, tx, job, "resume_intent", "resume", actor, reason, intent.Trust, requestedAt); err != nil {
		return DurableJob{}, false, err
	}
	resumed, err := durableGet(ctx, tx, id)
	if err != nil {
		return DurableJob{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return DurableJob{}, false, err
	}
	return resumed, true, nil
}

func (l *DurableLedger) Replay(ctx context.Context, sourceID string, req DurableReplayRequest) (DurableJob, bool, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, false, errors.New("subagent: durable ledger is nil")
	}
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return DurableJob{}, false, errors.New("subagent: durable replay source id is empty")
	}
	newID := strings.TrimSpace(req.ID)
	if newID == "" {
		return DurableJob{}, false, errors.New("subagent: durable replay id is empty")
	}
	dataOverrides, err := durableJSON(req.DataOverrides, `{}`)
	if err != nil {
		return DurableJob{}, false, fmt.Errorf("subagent: data_overrides json: %w", err)
	}
	now := durableNow()
	requestedAt := now
	if !req.RequestedAt.IsZero() {
		requestedAt = req.RequestedAt.UTC().UnixNano()
	}

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return DurableJob{}, false, err
	}
	defer tx.Rollback()

	source, err := durableGet(ctx, tx, sourceID)
	if errors.Is(err, ErrDurableJobNotFound) {
		return DurableJob{}, false, nil
	}
	if err != nil {
		return DurableJob{}, false, err
	}
	if source.Status != DurableJobCompleted && source.Status != DurableJobFailed {
		if err := durableInsertReplayUnavailableEvent(ctx, tx, source, requestedAt); err != nil {
			return DurableJob{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return DurableJob{}, false, err
		}
		return DurableJob{}, false, ErrDurableReplayUnavailable
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO durable_jobs
			(id, kind, status, parent_id, replay_of, depth, progress_json,
			 data_overrides_json, result_json, error_text, cancel_requested,
			 cancel_reason, lock_owner, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '{}', '', 0, '', '', ?, ?)`,
		newID, source.Kind, DurableJobWaiting, source.ParentID, source.ID, source.Depth,
		string(source.Progress), dataOverrides, now, now)
	if err != nil {
		return DurableJob{}, false, fmt.Errorf("subagent: replay durable job: %w", err)
	}
	if err := durableInsertReplayCreatedEvent(ctx, tx, source, newID, dataOverrides, requestedAt); err != nil {
		return DurableJob{}, false, err
	}
	replayed, err := durableGet(ctx, tx, newID)
	if err != nil {
		return DurableJob{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return DurableJob{}, false, err
	}
	return replayed, true, nil
}

func (l *DurableLedger) Get(ctx context.Context, id string) (DurableJob, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, errors.New("subagent: durable ledger is nil")
	}
	return durableGet(ctx, l.db, id)
}

func (l *DurableLedger) List(ctx context.Context, filter DurableJobListFilter) ([]DurableJob, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("subagent: durable ledger is nil")
	}
	query, args := durableJobListSQL(filter)
	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []DurableJob
	for rows.Next() {
		job, err := durableScanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (l *DurableLedger) Progress(ctx context.Context, id string) (json.RawMessage, error) {
	job, err := l.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), job.Progress...), nil
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

func (l *DurableLedger) SendInboxMessage(ctx context.Context, sub DurableInboxMessageSubmission) (DurableInboxMessage, error) {
	if l == nil || l.db == nil {
		return DurableInboxMessage{}, errors.New("subagent: durable ledger is nil")
	}
	jobID := strings.TrimSpace(sub.JobID)
	if jobID == "" {
		return DurableInboxMessage{}, errors.New("subagent: durable inbox job id is empty")
	}
	payload, err := durableJSON(sub.Payload, `{}`)
	if err != nil {
		return DurableInboxMessage{}, fmt.Errorf("subagent: inbox payload json: %w", err)
	}
	sender := strings.TrimSpace(sub.Sender)
	if sender == "" {
		sender = string(sub.SenderTrust)
	}
	if sender == "" {
		return DurableInboxMessage{}, errors.New("subagent: durable inbox sender is empty")
	}
	unreadAt := durableNow()
	if !sub.SentAt.IsZero() {
		unreadAt = sub.SentAt.UTC().UnixNano()
	}

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return DurableInboxMessage{}, err
	}
	defer tx.Rollback()
	if _, err := durableGet(ctx, tx, jobID); err != nil {
		return DurableInboxMessage{}, err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO durable_job_inbox
			(job_id, sender, sender_trust, payload_json, unread_at)
		VALUES (?, ?, ?, ?, ?)`,
		jobID, sender, sub.SenderTrust, payload, unreadAt)
	if err != nil {
		return DurableInboxMessage{}, fmt.Errorf("subagent: send durable inbox message: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return DurableInboxMessage{}, err
	}
	msg, err := durableInboxMessageByID(ctx, tx, id)
	if err != nil {
		return DurableInboxMessage{}, err
	}
	if err := tx.Commit(); err != nil {
		return DurableInboxMessage{}, err
	}
	return msg, nil
}

func (l *DurableLedger) ClaimInboxMessages(ctx context.Context, jobID string, claim DurableInboxClaim) ([]DurableInboxMessage, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("subagent: durable ledger is nil")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, errors.New("subagent: durable inbox job id is empty")
	}
	if !durableInboxClaimAllowed(jobID, claim) {
		return nil, ErrDurableInboxClaimDenied
	}
	claimedAt := durableNow()
	if !claim.ClaimedAt.IsZero() {
		claimedAt = claim.ClaimedAt.UTC().UnixNano()
	}
	actor := durableInboxClaimActor(claim)

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := durableGet(ctx, tx, jobID); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id
		FROM durable_job_inbox
		WHERE job_id = ? AND read_at IS NULL
		ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	updateSQL, updateArgs := durableInboxClaimUpdateSQL(ids, claimedAt, actor)
	if _, err := tx.ExecContext(ctx, updateSQL, updateArgs...); err != nil {
		return nil, err
	}
	msgs, err := durableInboxMessagesByIDs(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (l *DurableLedger) InboxMessages(ctx context.Context, jobID string) ([]DurableInboxMessage, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("subagent: durable ledger is nil")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, errors.New("subagent: durable inbox job id is empty")
	}
	if _, err := l.Get(ctx, jobID); err != nil {
		return nil, err
	}
	rows, err := l.db.QueryContext(ctx, `
		SELECT id, job_id, sender, sender_trust, payload_json, unread_at, read_at, read_by
		FROM durable_job_inbox
		WHERE job_id = ?
		ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DurableInboxMessage
	for rows.Next() {
		msg, err := durableScanInboxMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (l *DurableLedger) RecordWorkerHeartbeat(ctx context.Context, heartbeat DurableWorkerHeartbeat) error {
	if l == nil || l.db == nil {
		return errors.New("subagent: durable ledger is nil")
	}
	workerID := strings.TrimSpace(heartbeat.WorkerID)
	if workerID == "" {
		return errors.New("subagent: durable worker id is empty")
	}
	heartbeatAt := heartbeat.HeartbeatAt
	if heartbeatAt.IsZero() {
		heartbeatAt = durableTime(durableNow())
	}
	now := durableNow()
	_, err := l.db.ExecContext(ctx, `
		INSERT INTO durable_worker_heartbeats
			(worker_id, heartbeat_at, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(worker_id) DO UPDATE SET
			heartbeat_at = excluded.heartbeat_at,
			updated_at = excluded.updated_at`,
		workerID, heartbeatAt.UnixNano(), now)
	if err != nil {
		return fmt.Errorf("subagent: record durable worker heartbeat: %w", err)
	}
	return nil
}

func (l *DurableLedger) RecordSupervisorStatus(ctx context.Context, report DurableSupervisorReport) error {
	if l == nil || l.db == nil {
		return errors.New("subagent: durable ledger is nil")
	}
	reportedAt := report.ReportedAt
	if reportedAt.IsZero() {
		reportedAt = durableTime(durableNow())
	}
	available := 0
	if report.Available {
		available = 1
	}
	now := durableNow()
	_, err := l.db.ExecContext(ctx, `
		INSERT INTO durable_supervisor_status
			(id, available, reason, reported_at, updated_at)
		VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			available = excluded.available,
			reason = excluded.reason,
			reported_at = excluded.reported_at,
			updated_at = excluded.updated_at`,
		available, strings.TrimSpace(report.Reason), reportedAt.UnixNano(), now)
	if err != nil {
		return fmt.Errorf("subagent: record durable supervisor status: %w", err)
	}
	return nil
}

func (l *DurableLedger) RecordWorkerRestartIntent(ctx context.Context, intent DurableWorkerRestartIntent) error {
	if l == nil || l.db == nil {
		return errors.New("subagent: durable ledger is nil")
	}
	workerID := strings.TrimSpace(intent.WorkerID)
	if workerID == "" {
		return errors.New("subagent: durable worker id is empty")
	}
	requestedAt := intent.RequestedAt
	if requestedAt.IsZero() {
		requestedAt = durableTime(durableNow())
	}
	payload := map[string]string{
		"worker_id":     workerID,
		"reason":        strings.TrimSpace(intent.Reason),
		"supervisor_id": strings.TrimSpace(intent.SupervisorID),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = l.db.ExecContext(ctx, `
		INSERT INTO durable_worker_events
			(type, worker_id, supervisor_id, reason, payload_json, created_at)
		VALUES ('restart_intent', ?, ?, ?, ?, ?)`,
		workerID, payload["supervisor_id"], payload["reason"], string(raw), requestedAt.UnixNano())
	if err != nil {
		return fmt.Errorf("subagent: record durable worker restart intent: %w", err)
	}
	return nil
}

func (l *DurableLedger) RecordWorkerAbortSignal(ctx context.Context, event DurableWorkerAbortEvent) error {
	if l == nil || l.db == nil {
		return errors.New("subagent: durable ledger is nil")
	}
	return durableInsertWorkerAbortEvent(ctx, l.db, string(DurableWorkerRunAbortSignalSent), event)
}

func (l *DurableLedger) RecordWorkerAbortRecoveryUnavailable(ctx context.Context, event DurableWorkerAbortEvent) error {
	if l == nil || l.db == nil {
		return errors.New("subagent: durable ledger is nil")
	}
	return durableInsertWorkerAbortEvent(ctx, l.db, string(DurableWorkerRunAbortRecoveryUnavailable), event)
}

func (l *DurableLedger) AbortWorkerJob(ctx context.Context, event DurableWorkerAbortEvent) (DurableJob, bool, error) {
	return l.abortWorkerJob(ctx, event, false)
}

func (l *DurableLedger) RecoverWorkerAbortSlot(ctx context.Context, event DurableWorkerAbortEvent) (DurableJob, bool, error) {
	return l.abortWorkerJob(ctx, event, true)
}

func (l *DurableLedger) Status(ctx context.Context) (DurableLedgerStatus, error) {
	if l == nil || l.db == nil {
		return DurableLedgerStatus{}, errors.New("subagent: durable ledger is nil")
	}
	st := DurableLedgerStatus{ReplayAvailable: true}
	st.MaxWaiting = l.maxWaiting
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
			st.Claimed = n
		case DurableJobPaused:
			st.Paused = n
		}
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_jobs
		WHERE status = ? AND lock_until IS NOT NULL AND lock_until < ?`,
		DurableJobActive, durableNow()).Scan(&st.Stalled)
	now := durableNow()
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_jobs
		WHERE timeout_at IS NOT NULL`).Scan(&st.TimeoutScheduled)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_jobs
		WHERE status = ? AND timeout_at IS NOT NULL AND timeout_at < ?`,
		DurableJobActive, now).Scan(&st.TimedOut)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_jobs
		WHERE status = ? AND created_at < ?`,
		DurableJobWaiting, now-int64(durableStaleWaitingAfter)).Scan(&st.StaleWaiting)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_job_events
		WHERE type = 'backpressure_denied'`).Scan(&st.BackpressureDenied)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_job_events
		WHERE type = 'replay_unavailable'`).Scan(&st.ReplayUnavailable)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_job_events
		WHERE type = 'protected_submit_denied'`).Scan(&st.ProtectedSubmitDenied)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_jobs
		WHERE cancel_requested = 1`).Scan(&st.CancelRequested)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_job_inbox
		WHERE read_at IS NULL`).Scan(&st.InboxUnread)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_jobs
		WHERE status = ? AND resume_requested_at IS NOT NULL`,
		DurableJobWaiting).Scan(&st.ResumePending)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_job_events
		WHERE type = 'lifecycle_control_unsupported'`).Scan(&st.LifecycleControlUnsupported)
	st.QueueFull = st.MaxWaiting > 0 && st.Waiting >= st.MaxWaiting
	if err := l.populateDurableWorkerStatus(ctx, &st); err != nil {
		return st, err
	}
	return st, nil
}

func (l *DurableLedger) populateDurableWorkerStatus(ctx context.Context, st *DurableLedgerStatus) error {
	worker := DurableWorkerStatus{
		Liveness:            DurableWorkerNoWorker,
		HeartbeatStaleAfter: durableWorkerHeartbeatStaleAfter,
		DegradedReason:      string(DurableWorkerNoWorker),
		Supervisor:          DurableSupervisorUnavailable,
		SupervisorReason:    "no-supervisor-status",
	}

	var workerID string
	var heartbeatAt int64
	err := l.db.QueryRowContext(ctx, `
		SELECT worker_id, heartbeat_at
		FROM durable_worker_heartbeats
		ORDER BY heartbeat_at DESC
		LIMIT 1`).Scan(&workerID, &heartbeatAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("subagent: durable worker heartbeat status: %w", err)
	}
	if err == nil {
		worker.WorkerID = workerID
		worker.LastHeartbeat = durableTime(heartbeatAt)
		if durableTime(durableNow()).Sub(worker.LastHeartbeat) > durableWorkerHeartbeatStaleAfter {
			worker.Liveness = DurableWorkerStaleHeartbeat
			worker.DegradedReason = string(DurableWorkerStaleHeartbeat)
		} else {
			worker.Liveness = DurableWorkerHealthy
			worker.DegradedReason = ""
		}
	}

	var available int
	var supervisorReason string
	var supervisorReportedAt int64
	err = l.db.QueryRowContext(ctx, `
		SELECT available, reason, reported_at
		FROM durable_supervisor_status
		WHERE id = 1`).Scan(&available, &supervisorReason, &supervisorReportedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("subagent: durable supervisor status: %w", err)
	}
	if err == nil {
		worker.SupervisorReportedAt = durableTime(supervisorReportedAt)
		if available == 1 {
			worker.Supervisor = DurableSupervisorAvailable
			worker.SupervisorReason = strings.TrimSpace(supervisorReason)
		} else {
			worker.Supervisor = DurableSupervisorUnavailable
			worker.SupervisorReason = strings.TrimSpace(supervisorReason)
			if worker.SupervisorReason == "" {
				worker.SupervisorReason = string(DurableSupervisorUnavailable)
			}
		}
	}

	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM durable_worker_events
		WHERE type = 'restart_intent'`).Scan(&worker.RestartIntent.AuditEvents)
	var restartWorkerID, restartReason, supervisorID string
	var requestedAt int64
	err = l.db.QueryRowContext(ctx, `
		SELECT worker_id, reason, supervisor_id, created_at
		FROM durable_worker_events
		WHERE type = 'restart_intent'
		ORDER BY id DESC
		LIMIT 1`).Scan(&restartWorkerID, &restartReason, &supervisorID, &requestedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("subagent: durable worker restart intent status: %w", err)
	}
	if err == nil {
		worker.RestartIntent.Requested = true
		worker.RestartIntent.WorkerID = restartWorkerID
		worker.RestartIntent.Reason = restartReason
		worker.RestartIntent.SupervisorID = supervisorID
		worker.RestartIntent.RequestedAt = durableTime(requestedAt)
	}

	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM durable_worker_events
		WHERE type = ?`, string(DurableWorkerRunAbortSignalSent)).Scan(&worker.AbortRecovery.AbortSignalSent)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM durable_worker_events
		WHERE type = ?`, string(DurableWorkerRunAbortSlotRecovered)).Scan(&worker.AbortRecovery.AbortSlotRecovered)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM durable_worker_events
		WHERE type = 'handler_ignored_abort'`).Scan(&worker.AbortRecovery.HandlerIgnoredAbort)
	_ = l.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM durable_worker_events
		WHERE type = ?`, string(DurableWorkerRunAbortRecoveryUnavailable)).Scan(&worker.AbortRecovery.AbortRecoveryUnavailable)

	var abortEventType, abortWorkerID, abortReason, abortPayload string
	var abortCreatedAt int64
	err = l.db.QueryRowContext(ctx, `
		SELECT type, worker_id, reason, payload_json, created_at
		FROM durable_worker_events
		WHERE type IN (?, ?, 'handler_ignored_abort', ?)
		ORDER BY id DESC
		LIMIT 1`,
		string(DurableWorkerRunAbortSignalSent),
		string(DurableWorkerRunAbortSlotRecovered),
		string(DurableWorkerRunAbortRecoveryUnavailable),
	).Scan(&abortEventType, &abortWorkerID, &abortReason, &abortPayload, &abortCreatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("subagent: durable worker abort recovery status: %w", err)
	}
	if err == nil {
		worker.AbortRecovery.LastEvent = abortEventType
		worker.AbortRecovery.LastWorkerID = abortWorkerID
		worker.AbortRecovery.LastReason = abortReason
		worker.AbortRecovery.LastEventAt = durableTime(abortCreatedAt)
		var payload struct {
			JobID string `json:"job_id"`
		}
		if json.Unmarshal([]byte(abortPayload), &payload) == nil {
			worker.AbortRecovery.LastJobID = payload.JobID
		}
	}

	st.Worker = worker
	return nil
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

func (l *DurableLedger) abortWorkerJob(ctx context.Context, event DurableWorkerAbortEvent, handlerIgnored bool) (DurableJob, bool, error) {
	if l == nil || l.db == nil {
		return DurableJob{}, false, errors.New("subagent: durable ledger is nil")
	}
	event.JobID = strings.TrimSpace(event.JobID)
	if event.JobID == "" {
		return DurableJob{}, false, errors.New("subagent: durable abort job id is empty")
	}
	event.WorkerID = strings.TrimSpace(event.WorkerID)
	if event.WorkerID == "" {
		return DurableJob{}, false, errors.New("subagent: durable worker id is empty")
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = durableTime(durableNow())
	}
	if strings.TrimSpace(event.Reason) == "" {
		event.Reason = string(DurableWorkerRunAbortSignalSent)
		if handlerIgnored {
			event.Reason = "handler_ignored_abort"
		}
	}
	eventAt := event.CreatedAt.UTC().UnixNano()
	reason := durableWorkerAbortReason(event)

	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return DurableJob{}, false, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE durable_jobs
		SET status = ?, cancel_requested = 1, cancel_reason = ?, error_text = ?,
		    lock_owner = '', lock_until = NULL, finished_at = COALESCE(finished_at, ?),
		    updated_at = ?
		WHERE id = ? AND status = ? AND lock_owner = ?`,
		DurableJobCancelled, reason, reason, eventAt, eventAt,
		event.JobID, DurableJobActive, event.WorkerID)
	if err != nil {
		return DurableJob{}, false, err
	}
	ok, err := durableRowsAffected(res)
	if err != nil || !ok {
		return DurableJob{}, ok, err
	}
	job, err := durableGet(ctx, tx, event.JobID)
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
	if handlerIgnored {
		if err := durableInsertWorkerAbortEvent(ctx, tx, "handler_ignored_abort", event); err != nil {
			return DurableJob{}, false, err
		}
		if err := durableInsertWorkerAbortEvent(ctx, tx, string(DurableWorkerRunAbortSlotRecovered), DurableWorkerAbortEvent{
			JobID:     event.JobID,
			WorkerID:  event.WorkerID,
			Reason:    string(DurableWorkerRunAbortSlotRecovered),
			CreatedAt: event.CreatedAt,
		}); err != nil {
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

type durableExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type durableScanner interface {
	Scan(...any) error
}

func durableGet(ctx context.Context, q durableQuerier, id string) (DurableJob, error) {
	return durableScanJob(q.QueryRowContext(ctx, durableJobSelectSQL+` WHERE id = ?`, id))
}

func durableScanJob(scanner durableScanner) (DurableJob, error) {
	var j DurableJob
	var progress, dataOverrides, result string
	var cancelRequested int
	var created, updated int64
	var started, finished, lockUntil, timeoutAt, pausedAt, resumeRequestedAt sql.NullInt64
	err := scanner.Scan(
		&j.ID, &j.Kind, &j.Status, &j.ParentID, &j.ReplayOf, &j.Depth, &progress,
		&dataOverrides, &result,
		&j.ErrorText, &cancelRequested, &j.CancelReason,
		&j.PauseActor, &j.PauseReason, &pausedAt,
		&j.ResumeActor, &j.ResumeReason, &resumeRequestedAt, &j.LockOwner,
		&lockUntil, &timeoutAt, &created, &updated, &started, &finished,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DurableJob{}, ErrDurableJobNotFound
	}
	if err != nil {
		return DurableJob{}, err
	}
	j.Progress = json.RawMessage(progress)
	j.DataOverrides = json.RawMessage(dataOverrides)
	j.Result = json.RawMessage(result)
	j.CancelRequested = cancelRequested != 0
	j.CreatedAt = durableTime(created)
	j.UpdatedAt = durableTime(updated)
	j.PausedAt = durableNullTime(pausedAt)
	j.ResumeRequestedAt = durableNullTime(resumeRequestedAt)
	j.LockUntil = durableNullTime(lockUntil)
	j.TimeoutAt = durableNullTime(timeoutAt)
	j.StartedAt = durableNullTime(started)
	j.FinishedAt = durableNullTime(finished)
	return j, nil
}

func durableJobListSQL(filter DurableJobListFilter) (string, []any) {
	query := durableJobSelectSQL
	var clauses []string
	var args []any
	if filter.Kind != "" {
		clauses = append(clauses, "kind = ?")
		args = append(args, filter.Kind)
	}
	if filter.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, filter.Status)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at, id"
	return query, args
}

func durableClaimJob(ctx context.Context, tx *sql.Tx, id string, claim DurableClaim, now int64) (DurableJob, bool, error) {
	workerID := strings.TrimSpace(claim.WorkerID)
	if workerID == "" {
		return DurableJob{}, false, errors.New("subagent: worker id is empty")
	}
	lockUntil := durableLockUntil(claim.LockUntil)
	timeoutAt := durableClaimTimeoutAt(claim, now)
	res, err := tx.ExecContext(ctx, `
		UPDATE durable_jobs
		SET status = ?, lock_owner = ?, lock_until = ?, timeout_at = ?,
		    resume_actor = '', resume_reason = '', resume_requested_at = NULL,
		    started_at = COALESCE(started_at, ?), updated_at = ?
		WHERE id = ? AND cancel_requested = 0 AND (
			status = ? OR (status = ? AND lock_until IS NOT NULL AND lock_until < ?)
		)`,
		DurableJobActive, workerID, lockUntil, timeoutAt, now, now, id,
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

func durableCountWaiting(ctx context.Context, q durableQuerier) (int, error) {
	var waiting int
	err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM durable_jobs
		WHERE status = ?`, DurableJobWaiting).Scan(&waiting)
	return waiting, err
}

func durableInsertBackpressureEvent(ctx context.Context, tx *sql.Tx, id string, kind WorkKind, waiting, maxWaiting int) error {
	payload := map[string]any{
		"type":          "backpressure_denied",
		"job_id":        id,
		"job_kind":      string(kind),
		"waiting_count": waiting,
		"max_waiting":   maxWaiting,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO durable_job_events
			(job_id, type, job_kind, payload_json, created_at)
		VALUES (?, 'backpressure_denied', ?, ?, ?)`,
		id, kind, string(raw), durableNow())
	return err
}

func durableInsertWorkerAbortEvent(ctx context.Context, q durableExecer, eventType string, event DurableWorkerAbortEvent) error {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return errors.New("subagent: durable worker abort event type is empty")
	}
	workerID := strings.TrimSpace(event.WorkerID)
	if workerID == "" {
		return errors.New("subagent: durable worker id is empty")
	}
	jobID := strings.TrimSpace(event.JobID)
	if jobID == "" {
		return errors.New("subagent: durable abort job id is empty")
	}
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = durableTime(durableNow())
	}
	reason := strings.TrimSpace(event.Reason)
	payload := map[string]any{
		"type":      eventType,
		"job_id":    jobID,
		"worker_id": workerID,
		"reason":    reason,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `
		INSERT INTO durable_worker_events
			(type, worker_id, reason, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		eventType, workerID, reason, string(raw), createdAt.UTC().UnixNano())
	return err
}

func durableWorkerAbortReason(event DurableWorkerAbortEvent) string {
	reason := strings.TrimSpace(event.Reason)
	if reason == "" {
		reason = string(DurableWorkerRunAbortSignalSent)
	}
	if !strings.Contains(reason, string(DurableWorkerRunAbortSignalSent)) {
		reason = string(DurableWorkerRunAbortSignalSent) + ": " + reason
	}
	return reason
}

func (l *DurableLedger) recordProtectedSubmitDenied(ctx context.Context, trust TrustClass, sub DurableJobSubmission) error {
	id := strings.TrimSpace(sub.ID)
	if id == "" {
		id = "protected-submit-denied"
	}
	payload := map[string]any{
		"type":     "protected_submit_denied",
		"job_id":   id,
		"job_kind": string(sub.Kind),
		"trust":    string(trust),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = l.db.ExecContext(ctx, `
		INSERT INTO durable_job_events
			(job_id, type, job_kind, payload_json, created_at)
		VALUES (?, 'protected_submit_denied', ?, ?, ?)`,
		id, sub.Kind, string(raw), durableNow())
	return err
}

func durableInsertReplayUnavailableEvent(ctx context.Context, tx *sql.Tx, source DurableJob, requestedAt int64) error {
	payload := map[string]any{
		"type":      "replay_unavailable",
		"job_id":    source.ID,
		"job_kind":  string(source.Kind),
		"status":    string(source.Status),
		"replay_of": source.ReplayOf,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO durable_job_events
			(job_id, type, job_kind, payload_json, created_at)
		VALUES (?, 'replay_unavailable', ?, ?, ?)`,
		source.ID, source.Kind, string(raw), requestedAt)
	return err
}

func durableInsertReplayCreatedEvent(ctx context.Context, tx *sql.Tx, source DurableJob, replayID, dataOverrides string, requestedAt int64) error {
	var overrides any
	if err := json.Unmarshal([]byte(dataOverrides), &overrides); err != nil {
		return err
	}
	payload := map[string]any{
		"type":           "replay_created",
		"job_id":         replayID,
		"job_kind":       string(source.Kind),
		"replay_of":      source.ID,
		"data_overrides": overrides,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO durable_job_events
			(job_id, type, child_id, job_kind, payload_json, created_at)
		VALUES (?, 'replay_created', ?, ?, ?, ?)`,
		source.ID, replayID, source.Kind, string(raw), requestedAt)
	return err
}

func durableInsertLifecycleEvent(ctx context.Context, tx *sql.Tx, job DurableJob, eventType, action, actor, reason string, trust TrustClass, requestedAt int64) error {
	payload := map[string]any{
		"type":         eventType,
		"action":       action,
		"job_id":       job.ID,
		"job_kind":     string(job.Kind),
		"actor":        actor,
		"trust":        string(trust),
		"reason":       reason,
		"requested_at": durableTime(requestedAt).Format(time.RFC3339Nano),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO durable_job_events
			(job_id, type, job_kind, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		job.ID, eventType, job.Kind, string(raw), requestedAt)
	return err
}

func durableInboxClaimAllowed(jobID string, claim DurableInboxClaim) bool {
	switch claim.Trust {
	case TrustOperator, TrustSystem:
		return true
	case TrustChildAgent:
		return strings.TrimSpace(claim.JobID) == jobID
	default:
		return false
	}
}

func durableInboxClaimActor(claim DurableInboxClaim) string {
	actor := strings.TrimSpace(claim.Actor)
	if actor != "" {
		return actor
	}
	if jobID := strings.TrimSpace(claim.JobID); jobID != "" {
		return jobID
	}
	return string(claim.Trust)
}

func durableInboxClaimUpdateSQL(ids []int64, claimedAt int64, actor string) (string, []any) {
	var b strings.Builder
	b.WriteString(`UPDATE durable_job_inbox SET read_at = ?, read_by = ? WHERE id IN (`)
	args := []any{claimedAt, actor}
	for i, id := range ids {
		if i > 0 {
			b.WriteString(`,`)
		}
		b.WriteString(`?`)
		args = append(args, id)
	}
	b.WriteString(`)`)
	return b.String(), args
}

func durableInboxMessagesByIDs(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, ids []int64) ([]DurableInboxMessage, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var b strings.Builder
	b.WriteString(`
		SELECT id, job_id, sender, sender_trust, payload_json, unread_at, read_at, read_by
		FROM durable_job_inbox
		WHERE id IN (`)
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		if i > 0 {
			b.WriteString(`,`)
		}
		b.WriteString(`?`)
		args = append(args, id)
	}
	b.WriteString(`)
		ORDER BY id`)
	rows, err := q.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DurableInboxMessage
	for rows.Next() {
		msg, err := durableScanInboxMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func durableInboxMessageByID(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) (DurableInboxMessage, error) {
	return durableScanInboxMessage(q.QueryRowContext(ctx, `
		SELECT id, job_id, sender, sender_trust, payload_json, unread_at, read_at, read_by
		FROM durable_job_inbox
		WHERE id = ?`, id))
}

func durableScanInboxMessage(scanner durableScanner) (DurableInboxMessage, error) {
	var msg DurableInboxMessage
	var payload string
	var unreadAt int64
	var readAt sql.NullInt64
	err := scanner.Scan(&msg.ID, &msg.JobID, &msg.Sender, &msg.SenderTrust, &payload, &unreadAt, &readAt, &msg.ReadBy)
	if err != nil {
		return DurableInboxMessage{}, err
	}
	msg.Payload = json.RawMessage(payload)
	msg.UnreadAt = durableTime(unreadAt)
	msg.ReadAt = durableNullTime(readAt)
	return msg, nil
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

func durableClaimTimeoutAt(claim DurableClaim, now int64) any {
	if !claim.TimeoutAt.IsZero() {
		return claim.TimeoutAt.UTC().UnixNano()
	}
	if claim.Timeout > 0 {
		return durableTime(now).Add(claim.Timeout).UnixNano()
	}
	return nil
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

func durableLifecycleRequestedAt(intent DurableLifecycleIntent, fallback int64) int64 {
	if !intent.RequestedAt.IsZero() {
		return intent.RequestedAt.UTC().UnixNano()
	}
	return fallback
}

func durableLifecycleActor(intent DurableLifecycleIntent) string {
	actor := strings.TrimSpace(intent.Actor)
	if actor != "" {
		return actor
	}
	return string(intent.Trust)
}

func durableLifecycleTrustAllowed(trust TrustClass) bool {
	return trust == TrustOperator || trust == TrustSystem
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
	SELECT id, kind, status, parent_id, replay_of, depth, progress_json, data_overrides_json, result_json,
	       error_text, cancel_requested, cancel_reason, pause_actor, pause_reason, paused_at,
	       resume_actor, resume_reason, resume_requested_at, lock_owner, lock_until, timeout_at,
	       created_at, updated_at, started_at, finished_at
	FROM durable_jobs`

const durableLedgerSchema = `
CREATE TABLE IF NOT EXISTS durable_jobs (
	id               TEXT PRIMARY KEY,
	kind             TEXT    NOT NULL CHECK(kind IN ('shell_command','cron_job','llm_subagent')),
	status           TEXT    NOT NULL CHECK(status IN ('waiting','active','waiting-children','paused','completed','failed','cancelled')),
	parent_id        TEXT    NOT NULL DEFAULT '',
	replay_of        TEXT    NOT NULL DEFAULT '',
	depth            INTEGER NOT NULL DEFAULT 0 CHECK(depth >= 0),
	progress_json    TEXT    NOT NULL DEFAULT '{}',
	data_overrides_json TEXT NOT NULL DEFAULT '{}',
	result_json      TEXT    NOT NULL DEFAULT '{}',
	error_text       TEXT    NOT NULL DEFAULT '',
	cancel_requested INTEGER NOT NULL DEFAULT 0 CHECK(cancel_requested IN (0,1)),
	cancel_reason    TEXT    NOT NULL DEFAULT '',
	pause_actor      TEXT    NOT NULL DEFAULT '',
	pause_reason     TEXT    NOT NULL DEFAULT '',
	paused_at        INTEGER,
	resume_actor     TEXT    NOT NULL DEFAULT '',
	resume_reason    TEXT    NOT NULL DEFAULT '',
	resume_requested_at INTEGER,
	lock_owner       TEXT    NOT NULL DEFAULT '',
	lock_until       INTEGER,
	timeout_at       INTEGER,
	created_at       INTEGER NOT NULL,
	updated_at       INTEGER NOT NULL,
	started_at       INTEGER,
	finished_at      INTEGER
);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_claim
	ON durable_jobs(status, kind, created_at);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_parent
	ON durable_jobs(parent_id);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_replay
	ON durable_jobs(replay_of);

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

CREATE TABLE IF NOT EXISTS durable_job_inbox (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	job_id       TEXT    NOT NULL,
	sender       TEXT    NOT NULL,
	sender_trust TEXT    NOT NULL DEFAULT '',
	payload_json TEXT    NOT NULL DEFAULT '{}',
	unread_at    INTEGER NOT NULL,
	read_at      INTEGER,
	read_by      TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_durable_job_inbox_unread
	ON durable_job_inbox(job_id, read_at, id);

CREATE TABLE IF NOT EXISTS durable_worker_heartbeats (
	worker_id    TEXT PRIMARY KEY,
	heartbeat_at INTEGER NOT NULL,
	updated_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS durable_supervisor_status (
	id          INTEGER PRIMARY KEY CHECK(id = 1),
	available   INTEGER NOT NULL CHECK(available IN (0,1)),
	reason      TEXT    NOT NULL DEFAULT '',
	reported_at INTEGER NOT NULL,
	updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS durable_worker_events (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	type          TEXT    NOT NULL,
	worker_id     TEXT    NOT NULL DEFAULT '',
	supervisor_id TEXT    NOT NULL DEFAULT '',
	reason        TEXT    NOT NULL DEFAULT '',
	payload_json  TEXT    NOT NULL DEFAULT '{}',
	created_at    INTEGER NOT NULL
);
`

const durableJobsLifecycleMigrationSchema = `
DROP TABLE IF EXISTS durable_jobs_lifecycle_migration;
CREATE TABLE durable_jobs_lifecycle_migration (
	id               TEXT PRIMARY KEY,
	kind             TEXT    NOT NULL CHECK(kind IN ('shell_command','cron_job','llm_subagent')),
	status           TEXT    NOT NULL CHECK(status IN ('waiting','active','waiting-children','paused','completed','failed','cancelled')),
	parent_id        TEXT    NOT NULL DEFAULT '',
	replay_of        TEXT    NOT NULL DEFAULT '',
	depth            INTEGER NOT NULL DEFAULT 0 CHECK(depth >= 0),
	progress_json    TEXT    NOT NULL DEFAULT '{}',
	data_overrides_json TEXT NOT NULL DEFAULT '{}',
	result_json      TEXT    NOT NULL DEFAULT '{}',
	error_text       TEXT    NOT NULL DEFAULT '',
	cancel_requested INTEGER NOT NULL DEFAULT 0 CHECK(cancel_requested IN (0,1)),
	cancel_reason    TEXT    NOT NULL DEFAULT '',
	pause_actor      TEXT    NOT NULL DEFAULT '',
	pause_reason     TEXT    NOT NULL DEFAULT '',
	paused_at        INTEGER,
	resume_actor     TEXT    NOT NULL DEFAULT '',
	resume_reason    TEXT    NOT NULL DEFAULT '',
	resume_requested_at INTEGER,
	lock_owner       TEXT    NOT NULL DEFAULT '',
	lock_until       INTEGER,
	timeout_at       INTEGER,
	created_at       INTEGER NOT NULL,
	updated_at       INTEGER NOT NULL,
	started_at       INTEGER,
	finished_at      INTEGER
);
`

const durableLedgerPostMigrationSchema = `
CREATE INDEX IF NOT EXISTS idx_durable_jobs_claim
	ON durable_jobs(status, kind, created_at);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_parent
	ON durable_jobs(parent_id);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_replay
	ON durable_jobs(replay_of);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_timeout
	ON durable_jobs(status, timeout_at);
CREATE INDEX IF NOT EXISTS idx_durable_jobs_lifecycle
	ON durable_jobs(status, resume_requested_at);
CREATE INDEX IF NOT EXISTS idx_durable_job_events_type
	ON durable_job_events(type, created_at);
CREATE INDEX IF NOT EXISTS idx_durable_job_inbox_unread
	ON durable_job_inbox(job_id, read_at, id);
CREATE INDEX IF NOT EXISTS idx_durable_worker_heartbeats_seen
	ON durable_worker_heartbeats(heartbeat_at);
CREATE INDEX IF NOT EXISTS idx_durable_worker_events_type
	ON durable_worker_events(type, created_at);
`

func durableLedgerMigrate(db *sql.DB) error {
	if err := durableEnsureColumn(db, "durable_jobs", "timeout_at", "INTEGER"); err != nil {
		return err
	}
	for _, col := range []struct {
		name string
		spec string
	}{
		{name: "replay_of", spec: "TEXT NOT NULL DEFAULT ''"},
		{name: "data_overrides_json", spec: "TEXT NOT NULL DEFAULT '{}'"},
		{name: "pause_actor", spec: "TEXT NOT NULL DEFAULT ''"},
		{name: "pause_reason", spec: "TEXT NOT NULL DEFAULT ''"},
		{name: "paused_at", spec: "INTEGER"},
		{name: "resume_actor", spec: "TEXT NOT NULL DEFAULT ''"},
		{name: "resume_reason", spec: "TEXT NOT NULL DEFAULT ''"},
		{name: "resume_requested_at", spec: "INTEGER"},
	} {
		if err := durableEnsureColumn(db, "durable_jobs", col.name, col.spec); err != nil {
			return err
		}
	}
	return durableEnsurePausedStatus(db)
}

func durableEnsurePausedStatus(db *sql.DB) error {
	var sqlText string
	err := db.QueryRow(`
		SELECT sql
		FROM sqlite_master
		WHERE type = 'table' AND name = 'durable_jobs'`).Scan(&sqlText)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.Contains(sqlText, "'paused'") {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(durableJobsLifecycleMigrationSchema); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO durable_jobs_lifecycle_migration
			(id, kind, status, parent_id, replay_of, depth, progress_json,
			 data_overrides_json, result_json, error_text, cancel_requested, cancel_reason, pause_actor,
			 pause_reason, paused_at, resume_actor, resume_reason, resume_requested_at,
			 lock_owner, lock_until, timeout_at, created_at, updated_at, started_at, finished_at)
		SELECT id, kind, status, parent_id, replay_of, depth, progress_json,
		       data_overrides_json, result_json, error_text, cancel_requested, cancel_reason, pause_actor,
		       pause_reason, paused_at, resume_actor, resume_reason, resume_requested_at,
		       lock_owner, lock_until, timeout_at, created_at, updated_at, started_at, finished_at
		FROM durable_jobs`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE durable_jobs`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE durable_jobs_lifecycle_migration RENAME TO durable_jobs`); err != nil {
		return err
	}
	return tx.Commit()
}

func durableEnsureColumn(db *sql.DB, table, column, spec string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + spec)
	return err
}
