package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const defaultDurableWorkerLease = 5 * time.Minute

type DurableWorkerRunStatus string

const (
	DurableWorkerRunCompleted            DurableWorkerRunStatus = "completed"
	DurableWorkerRunIdle                 DurableWorkerRunStatus = "idle"
	DurableWorkerRunClaimUnavailable     DurableWorkerRunStatus = "claim_unavailable"
	DurableWorkerRunHandlerFailed        DurableWorkerRunStatus = "handler_failed"
	DurableWorkerRunHeartbeatUnavailable DurableWorkerRunStatus = "heartbeat_unavailable"
)

type DurableWorkerProgressFunc func(json.RawMessage) error

type DurableWorkerHandler func(context.Context, DurableJob, DurableWorkerProgressFunc) (json.RawMessage, error)

type DurableWorker struct {
	Ledger   *DurableLedger
	WorkerID string
	Kinds    []WorkKind
	Handler  DurableWorkerHandler
	Now      func() time.Time
	Lease    time.Duration
	Timeout  time.Duration
}

type DurableWorkerRunResult struct {
	Status      DurableWorkerRunStatus
	JobID       string
	WorkerID    string
	LockOwner   string
	ErrorText   string
	HeartbeatAt time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
}

func (w DurableWorker) RunOne(ctx context.Context) (DurableWorkerRunResult, error) {
	workerID := strings.TrimSpace(w.WorkerID)
	result := DurableWorkerRunResult{
		WorkerID:   workerID,
		StartedAt:  w.now(),
		FinishedAt: w.now(),
	}
	if w.Ledger == nil {
		result.Status = DurableWorkerRunClaimUnavailable
		result.ErrorText = "subagent: durable worker ledger is nil"
		return result, errors.New(result.ErrorText)
	}
	if workerID == "" {
		result.Status = DurableWorkerRunClaimUnavailable
		result.ErrorText = "subagent: durable worker id is empty"
		return result, errors.New(result.ErrorText)
	}
	if w.Handler == nil {
		result.Status = DurableWorkerRunHandlerFailed
		result.ErrorText = "subagent: durable worker handler is nil"
		return result, errors.New(result.ErrorText)
	}

	claimAt := w.now()
	job, ok, err := w.Ledger.Claim(ctx, DurableClaim{
		WorkerID:  workerID,
		LockUntil: claimAt.Add(w.lease()),
		Timeout:   w.Timeout,
		Kinds:     w.Kinds,
	})
	if err != nil {
		result.Status = DurableWorkerRunClaimUnavailable
		result.ErrorText = err.Error()
		result.FinishedAt = w.now()
		return result, err
	}
	if !ok {
		result.Status = DurableWorkerRunIdle
		result.FinishedAt = w.now()
		return result, nil
	}
	result.JobID = job.ID
	result.LockOwner = job.LockOwner
	heartbeatAt := w.now()
	if err := w.Ledger.RecordWorkerHeartbeat(ctx, DurableWorkerHeartbeat{
		WorkerID:    workerID,
		HeartbeatAt: heartbeatAt,
	}); err != nil {
		result.Status = DurableWorkerRunHeartbeatUnavailable
		result.ErrorText = err.Error()
		result.FinishedAt = w.now()
		return result, err
	}
	result.HeartbeatAt = heartbeatAt

	handlerResult, err := w.Handler(ctx, job, func(progress json.RawMessage) error {
		ok, err := w.Ledger.UpdateProgress(ctx, job.ID, workerID, progress)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("subagent: durable worker could not update progress for claimed job %q", job.ID)
		}
		return nil
	})
	if err != nil {
		result.Status = DurableWorkerRunHandlerFailed
		result.ErrorText = err.Error()
		result.FinishedAt = w.now()
		if _, ok, failErr := w.Ledger.Fail(ctx, job.ID, workerID, err.Error()); failErr != nil {
			return result, failErr
		} else if !ok {
			result.ErrorText = fmt.Sprintf("%s; failed to mark claimed job %q failed", result.ErrorText, job.ID)
			return result, errors.New(result.ErrorText)
		}
		return result, nil
	}
	if _, ok, err := w.Ledger.Complete(ctx, job.ID, workerID, handlerResult); err != nil {
		result.Status = DurableWorkerRunClaimUnavailable
		result.ErrorText = err.Error()
		result.FinishedAt = w.now()
		return result, err
	} else if !ok {
		result.Status = DurableWorkerRunClaimUnavailable
		result.ErrorText = fmt.Sprintf("subagent: durable worker could not complete claimed job %q", job.ID)
		result.FinishedAt = w.now()
		return result, errors.New(result.ErrorText)
	}

	result.Status = DurableWorkerRunCompleted
	result.FinishedAt = w.now()
	return result, nil
}

func (w DurableWorker) lease() time.Duration {
	if w.Lease > 0 {
		return w.Lease
	}
	return defaultDurableWorkerLease
}

func (w DurableWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}
