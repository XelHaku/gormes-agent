package subagent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDurableWorker_TimeoutPropagatesCancel(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: "cron:abort", Kind: WorkKindCronJob}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	timers := newManualDurableWorkerTimers()
	started := make(chan struct{})
	cancelObserved := make(chan error, 1)
	worker := DurableWorker{
		Ledger:     ledger,
		WorkerID:   "worker-a",
		Kinds:      []WorkKind{WorkKindCronJob},
		Timeout:    10 * time.Millisecond,
		AbortGrace: 20 * time.Millisecond,
		After:      timers.After,
		Handler: func(ctx context.Context, job DurableJob, progress DurableWorkerProgressFunc) (json.RawMessage, error) {
			close(started)
			<-ctx.Done()
			cancelObserved <- ctx.Err()
			return nil, ctx.Err()
		},
	}

	resultCh := make(chan durableWorkerRunOutcome, 1)
	go func() {
		result, err := worker.RunOne(ctx)
		resultCh <- durableWorkerRunOutcome{result: result, err: err}
	}()

	<-started
	timers.fireTimeout()
	if err := <-cancelObserved; err == nil {
		t.Fatal("handler observed nil ctx.Err, want cancellation error")
	}
	out := <-resultCh
	if out.err != nil {
		t.Fatalf("RunOne: %v", out.err)
	}
	if out.result.Status != DurableWorkerRunAbortSignalSent {
		t.Fatalf("result status = %q, want %q", out.result.Status, DurableWorkerRunAbortSignalSent)
	}

	got, err := ledger.Get(ctx, "cron:abort")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != DurableJobCancelled || !got.CancelRequested {
		t.Fatalf("job status/cancel = %q/%v, want cancelled with cancel evidence", got.Status, got.CancelRequested)
	}
	if got.LockOwner != "" || !strings.Contains(got.CancelReason, "abort_signal_sent") {
		t.Fatalf("job lock/cancel reason = owner %q reason %q, want cleared abort_signal_sent evidence", got.LockOwner, got.CancelReason)
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Worker.AbortRecovery.AbortSignalSent != 1 {
		t.Fatalf("AbortSignalSent = %d, want 1", status.Worker.AbortRecovery.AbortSignalSent)
	}
	if status.Worker.AbortRecovery.AbortSlotRecovered != 0 {
		t.Fatalf("AbortSlotRecovered = %d, want 0 for cooperative cancellation", status.Worker.AbortRecovery.AbortSlotRecovered)
	}
}

func TestDurableWorker_IgnoredAbortReleasesSlot(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: "cron:ignored", Kind: WorkKindCronJob}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	timers := newManualDurableWorkerTimers()
	started := make(chan struct{})
	cancelObserved := make(chan struct{})
	releaseHandler := make(chan struct{})
	handlerReturned := make(chan struct{})
	worker := DurableWorker{
		Ledger:     ledger,
		WorkerID:   "worker-a",
		Kinds:      []WorkKind{WorkKindCronJob},
		Timeout:    10 * time.Millisecond,
		AbortGrace: 20 * time.Millisecond,
		After:      timers.After,
		Handler: func(ctx context.Context, job DurableJob, progress DurableWorkerProgressFunc) (json.RawMessage, error) {
			defer close(handlerReturned)
			close(started)
			<-ctx.Done()
			close(cancelObserved)
			<-releaseHandler
			return json.RawMessage(`{"late":true}`), nil
		},
	}

	resultCh := make(chan durableWorkerRunOutcome, 1)
	go func() {
		result, err := worker.RunOne(ctx)
		resultCh <- durableWorkerRunOutcome{result: result, err: err}
	}()

	<-started
	timers.fireTimeout()
	<-cancelObserved
	timers.fireGrace()
	out := <-resultCh
	if out.err != nil {
		t.Fatalf("RunOne: %v", out.err)
	}
	if out.result.Status != DurableWorkerRunAbortSlotRecovered {
		t.Fatalf("result status = %q, want %q", out.result.Status, DurableWorkerRunAbortSlotRecovered)
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Worker.AbortRecovery.AbortSignalSent != 1 {
		t.Fatalf("AbortSignalSent = %d, want 1", status.Worker.AbortRecovery.AbortSignalSent)
	}
	if status.Worker.AbortRecovery.HandlerIgnoredAbort != 1 {
		t.Fatalf("HandlerIgnoredAbort = %d, want 1", status.Worker.AbortRecovery.HandlerIgnoredAbort)
	}
	if status.Worker.AbortRecovery.AbortSlotRecovered != 1 {
		t.Fatalf("AbortSlotRecovered = %d, want 1", status.Worker.AbortRecovery.AbortSlotRecovered)
	}

	got, err := ledger.Get(ctx, "cron:ignored")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != DurableJobCancelled || got.LockOwner != "" {
		t.Fatalf("job status/owner = %q/%q, want cancelled with released slot", got.Status, got.LockOwner)
	}

	close(releaseHandler)
	<-handlerReturned
	got, err = ledger.Get(ctx, "cron:ignored")
	if err != nil {
		t.Fatalf("Get after late handler: %v", err)
	}
	if got.Status != DurableJobCancelled {
		t.Fatalf("late handler changed status = %q, want still cancelled", got.Status)
	}
	assertJSONEqual(t, "late result", got.Result, `{}`)
	if _, ok, err := ledger.Complete(ctx, "cron:ignored", "worker-a", json.RawMessage(`{"double":true}`)); err != nil || ok {
		t.Fatalf("late Complete ok=%v err=%v, want false nil", ok, err)
	}

	status, err = ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status after late handler: %v", err)
	}
	if status.Worker.AbortRecovery.AbortSlotRecovered != 1 {
		t.Fatalf("AbortSlotRecovered after late handler = %d, want one-shot evidence", status.Worker.AbortRecovery.AbortSlotRecovered)
	}
}

func TestDurableWorker_ClaimsNextJobAfterRecovery(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	for _, id := range []string{"cron:wedged", "cron:next"} {
		if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: id, Kind: WorkKindCronJob}); err != nil {
			t.Fatalf("Submit %s: %v", id, err)
		}
	}

	timers := newManualDurableWorkerTimers()
	started := make(chan struct{})
	cancelObserved := make(chan struct{})
	releaseHandler := make(chan struct{})
	worker := DurableWorker{
		Ledger:     ledger,
		WorkerID:   "worker-a",
		Kinds:      []WorkKind{WorkKindCronJob},
		Timeout:    10 * time.Millisecond,
		AbortGrace: 20 * time.Millisecond,
		After:      timers.After,
		Handler: func(ctx context.Context, job DurableJob, progress DurableWorkerProgressFunc) (json.RawMessage, error) {
			close(started)
			<-ctx.Done()
			close(cancelObserved)
			<-releaseHandler
			return json.RawMessage(`{"late":true}`), nil
		},
	}

	resultCh := make(chan durableWorkerRunOutcome, 1)
	go func() {
		result, err := worker.RunOne(ctx)
		resultCh <- durableWorkerRunOutcome{result: result, err: err}
	}()

	<-started
	timers.fireTimeout()
	<-cancelObserved
	timers.fireGrace()
	first := <-resultCh
	if first.err != nil {
		t.Fatalf("first RunOne: %v", first.err)
	}
	if first.result.Status != DurableWorkerRunAbortSlotRecovered {
		t.Fatalf("first status = %q, want %q", first.result.Status, DurableWorkerRunAbortSlotRecovered)
	}

	nextWorker := DurableWorker{
		Ledger:   ledger,
		WorkerID: "worker-a",
		Kinds:    []WorkKind{WorkKindCronJob},
		Handler: func(ctx context.Context, job DurableJob, progress DurableWorkerProgressFunc) (json.RawMessage, error) {
			if job.ID != "cron:next" {
				t.Fatalf("next handler job = %q, want cron:next", job.ID)
			}
			return json.RawMessage(`{"claimed":true}`), nil
		},
	}
	second, err := nextWorker.RunOne(ctx)
	if err != nil {
		t.Fatalf("second RunOne: %v", err)
	}
	if second.Status != DurableWorkerRunCompleted || second.JobID != "cron:next" {
		t.Fatalf("second result = %+v, want completed cron:next", second)
	}

	close(releaseHandler)
	got, err := ledger.Get(ctx, "cron:next")
	if err != nil {
		t.Fatalf("Get next: %v", err)
	}
	if got.Status != DurableJobCompleted {
		t.Fatalf("next status = %q, want completed", got.Status)
	}
	assertJSONEqual(t, "next result", got.Result, `{"claimed":true}`)
}

type durableWorkerRunOutcome struct {
	result DurableWorkerRunResult
	err    error
}

type manualDurableWorkerTimers struct {
	timeout chan time.Time
	grace   chan time.Time

	mu    sync.Mutex
	calls []time.Duration
}

func newManualDurableWorkerTimers() *manualDurableWorkerTimers {
	return &manualDurableWorkerTimers{
		timeout: make(chan time.Time, 1),
		grace:   make(chan time.Time, 1),
	}
}

func (t *manualDurableWorkerTimers) After(d time.Duration) <-chan time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls = append(t.calls, d)
	switch len(t.calls) {
	case 1:
		return t.timeout
	case 2:
		return t.grace
	default:
		ch := make(chan time.Time)
		return ch
	}
}

func (t *manualDurableWorkerTimers) fireTimeout() {
	t.timeout <- time.Now().UTC()
}

func (t *manualDurableWorkerTimers) fireGrace() {
	t.grace <- time.Now().UTC()
}
