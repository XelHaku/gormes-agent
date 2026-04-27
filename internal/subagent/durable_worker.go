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
const defaultDurableWorkerAbortGrace = 50 * time.Millisecond

type DurableWorkerRunStatus string

const (
	DurableWorkerRunCompleted                DurableWorkerRunStatus = "completed"
	DurableWorkerRunIdle                     DurableWorkerRunStatus = "idle"
	DurableWorkerRunClaimUnavailable         DurableWorkerRunStatus = "claim_unavailable"
	DurableWorkerRunHandlerFailed            DurableWorkerRunStatus = "handler_failed"
	DurableWorkerRunHeartbeatUnavailable     DurableWorkerRunStatus = "heartbeat_unavailable"
	DurableWorkerRunAbortSignalSent          DurableWorkerRunStatus = "abort_signal_sent"
	DurableWorkerRunAbortSlotRecovered       DurableWorkerRunStatus = "abort_slot_recovered"
	DurableWorkerRunAbortRecoveryUnavailable DurableWorkerRunStatus = "abort_recovery_unavailable"
)

type DurableWorkerProgressFunc func(json.RawMessage) error

type DurableWorkerHandler func(context.Context, DurableJob, DurableWorkerProgressFunc) (json.RawMessage, error)

type DurableWorker struct {
	Ledger     *DurableLedger
	WorkerID   string
	Kinds      []WorkKind
	Handler    DurableWorkerHandler
	Now        func() time.Time
	Lease      time.Duration
	Timeout    time.Duration
	AbortGrace time.Duration
	After      func(time.Duration) <-chan time.Time
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

	progress := func(progress json.RawMessage) error {
		ok, err := w.Ledger.UpdateProgress(ctx, job.ID, workerID, progress)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("subagent: durable worker could not update progress for claimed job %q", job.ID)
		}
		return nil
	}
	timeout := w.handlerTimeout(job)
	if timeout <= 0 {
		handlerResult, err := w.Handler(ctx, job, progress)
		return w.finishHandler(ctx, result, job, workerID, durableWorkerHandlerResult{
			result: handlerResult,
			err:    err,
		})
	}

	handlerCtx, cancelHandler := context.WithCancel(ctx)
	defer cancelHandler()
	handlerDone := make(chan durableWorkerHandlerResult, 1)
	go func() {
		handlerResult, err := w.Handler(handlerCtx, job, progress)
		handlerDone <- durableWorkerHandlerResult{result: handlerResult, err: err}
	}()

	select {
	case handlerResult := <-handlerDone:
		return w.finishHandler(ctx, result, job, workerID, handlerResult)
	case <-w.after(timeout):
		return w.abortTimedOutHandler(ctx, result, job, workerID, cancelHandler, handlerDone)
	}
}

type durableWorkerHandlerResult struct {
	result json.RawMessage
	err    error
}

func (w DurableWorker) finishHandler(ctx context.Context, result DurableWorkerRunResult, job DurableJob, workerID string, handlerResult durableWorkerHandlerResult) (DurableWorkerRunResult, error) {
	if handlerResult.err != nil {
		result.Status = DurableWorkerRunHandlerFailed
		result.ErrorText = handlerResult.err.Error()
		result.FinishedAt = w.now()
		if _, ok, failErr := w.Ledger.Fail(ctx, job.ID, workerID, handlerResult.err.Error()); failErr != nil {
			return result, failErr
		} else if !ok {
			result.ErrorText = fmt.Sprintf("%s; failed to mark claimed job %q failed", result.ErrorText, job.ID)
			return result, errors.New(result.ErrorText)
		}
		return result, nil
	}
	if _, ok, err := w.Ledger.Complete(ctx, job.ID, workerID, handlerResult.result); err != nil {
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

func (w DurableWorker) abortTimedOutHandler(ctx context.Context, result DurableWorkerRunResult, job DurableJob, workerID string, cancelHandler context.CancelFunc, handlerDone <-chan durableWorkerHandlerResult) (DurableWorkerRunResult, error) {
	cancelHandler()
	event := DurableWorkerAbortEvent{
		JobID:     job.ID,
		WorkerID:  workerID,
		Reason:    string(DurableWorkerRunAbortSignalSent),
		CreatedAt: w.now(),
	}
	if err := w.Ledger.RecordWorkerAbortSignal(ctx, event); err != nil {
		result.Status = DurableWorkerRunAbortRecoveryUnavailable
		result.ErrorText = err.Error()
		result.FinishedAt = w.now()
		return result, err
	}

	select {
	case <-handlerDone:
		if _, ok, err := w.Ledger.AbortWorkerJob(ctx, event); err != nil {
			_ = w.Ledger.RecordWorkerAbortRecoveryUnavailable(ctx, DurableWorkerAbortEvent{
				JobID:     job.ID,
				WorkerID:  workerID,
				Reason:    err.Error(),
				CreatedAt: w.now(),
			})
			result.Status = DurableWorkerRunAbortRecoveryUnavailable
			result.ErrorText = err.Error()
			result.FinishedAt = w.now()
			return result, err
		} else if !ok {
			reason := fmt.Sprintf("subagent: durable worker could not abort timed-out job %q", job.ID)
			_ = w.Ledger.RecordWorkerAbortRecoveryUnavailable(ctx, DurableWorkerAbortEvent{
				JobID:     job.ID,
				WorkerID:  workerID,
				Reason:    reason,
				CreatedAt: w.now(),
			})
			result.Status = DurableWorkerRunAbortRecoveryUnavailable
			result.ErrorText = reason
			result.FinishedAt = w.now()
			return result, nil
		}
		result.Status = DurableWorkerRunAbortSignalSent
		result.ErrorText = string(DurableWorkerRunAbortSignalSent)
		result.FinishedAt = w.now()
		return result, nil
	case <-w.after(w.abortGrace()):
		_, ok, err := w.Ledger.RecoverWorkerAbortSlot(ctx, DurableWorkerAbortEvent{
			JobID:     job.ID,
			WorkerID:  workerID,
			Reason:    "handler_ignored_abort",
			CreatedAt: w.now(),
		})
		if err != nil {
			_ = w.Ledger.RecordWorkerAbortRecoveryUnavailable(ctx, DurableWorkerAbortEvent{
				JobID:     job.ID,
				WorkerID:  workerID,
				Reason:    err.Error(),
				CreatedAt: w.now(),
			})
			result.Status = DurableWorkerRunAbortRecoveryUnavailable
			result.ErrorText = err.Error()
			result.FinishedAt = w.now()
			return result, err
		}
		if !ok {
			reason := fmt.Sprintf("subagent: durable worker abort recovery unavailable for job %q", job.ID)
			_ = w.Ledger.RecordWorkerAbortRecoveryUnavailable(ctx, DurableWorkerAbortEvent{
				JobID:     job.ID,
				WorkerID:  workerID,
				Reason:    reason,
				CreatedAt: w.now(),
			})
			result.Status = DurableWorkerRunAbortRecoveryUnavailable
			result.ErrorText = reason
			result.FinishedAt = w.now()
			return result, nil
		}
		result.Status = DurableWorkerRunAbortSlotRecovered
		result.ErrorText = "handler_ignored_abort"
		result.FinishedAt = w.now()
		return result, nil
	}
}

func (w DurableWorker) lease() time.Duration {
	if w.Lease > 0 {
		return w.Lease
	}
	return defaultDurableWorkerLease
}

func (w DurableWorker) abortGrace() time.Duration {
	if w.AbortGrace > 0 {
		return w.AbortGrace
	}
	return defaultDurableWorkerAbortGrace
}

func (w DurableWorker) handlerTimeout(job DurableJob) time.Duration {
	if w.Timeout > 0 {
		return w.Timeout
	}
	if job.TimeoutAt.IsZero() {
		return 0
	}
	timeout := job.TimeoutAt.Sub(w.now())
	if timeout <= 0 {
		return time.Nanosecond
	}
	return timeout
}

func (w DurableWorker) after(d time.Duration) <-chan time.Time {
	if d <= 0 {
		ch := make(chan time.Time, 1)
		ch <- w.now()
		return ch
	}
	if w.After != nil {
		return w.After(d)
	}
	return time.After(d)
}

func (w DurableWorker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}
