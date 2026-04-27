package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDurableWorkerRSSDrain_PostJobThresholdAbortsSibling(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	for _, id := range []string{"cron:slow", "cron:quick"} {
		if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: id, Kind: WorkKindCronJob}); err != nil {
			t.Fatalf("Submit %s: %v", id, err)
		}
	}

	drain := NewDurableWorkerRSSDrain()
	startedSlow := make(chan struct{})
	cancelObserved := make(chan error, 1)
	slowWorker := DurableWorker{
		Ledger:      ledger,
		WorkerID:    "worker-a",
		Kinds:       []WorkKind{WorkKindCronJob},
		RSSWatchdog: DurableWorkerRSSWatchdogPolicy{MaxRSSMB: 100},
		RSSReader: func() (uint64, error) {
			return 151 * 1024 * 1024, nil
		},
		RSSDrain: drain,
		Handler: func(ctx context.Context, job DurableJob, progress DurableWorkerProgressFunc) (json.RawMessage, error) {
			if job.ID != "cron:slow" {
				t.Fatalf("slow handler job = %q, want cron:slow", job.ID)
			}
			close(startedSlow)
			<-ctx.Done()
			cancelObserved <- ctx.Err()
			return nil, ctx.Err()
		},
	}

	slowResultCh := make(chan durableWorkerRunOutcome, 1)
	go func() {
		result, err := slowWorker.RunOne(ctx)
		slowResultCh <- durableWorkerRunOutcome{result: result, err: err}
	}()
	<-startedSlow

	quickWorker := DurableWorker{
		Ledger:      ledger,
		WorkerID:    "worker-a",
		Kinds:       []WorkKind{WorkKindCronJob},
		RSSWatchdog: DurableWorkerRSSWatchdogPolicy{MaxRSSMB: 100},
		RSSReader: func() (uint64, error) {
			return 151 * 1024 * 1024, nil
		},
		RSSDrain: drain,
		Handler: func(ctx context.Context, job DurableJob, progress DurableWorkerProgressFunc) (json.RawMessage, error) {
			if job.ID != "cron:quick" {
				t.Fatalf("quick handler job = %q, want cron:quick", job.ID)
			}
			return json.RawMessage(`{"quick":true}`), nil
		},
	}
	quickResult, err := quickWorker.RunOne(ctx)
	if err != nil {
		t.Fatalf("quick RunOne: %v", err)
	}
	if quickResult.Status != DurableWorkerRunCompleted {
		t.Fatalf("quick status = %q, want %q", quickResult.Status, DurableWorkerRunCompleted)
	}

	if err := <-cancelObserved; err == nil {
		t.Fatal("slow handler observed nil ctx.Err, want RSS drain cancellation")
	}
	slow := <-slowResultCh
	if slow.err != nil {
		t.Fatalf("slow RunOne: %v", slow.err)
	}
	if slow.result.Status != DurableWorkerRunRSSHandlerAbortSent {
		t.Fatalf("slow status = %q, want %q", slow.result.Status, DurableWorkerRunRSSHandlerAbortSent)
	}

	gotSlow, err := ledger.Get(ctx, "cron:slow")
	if err != nil {
		t.Fatalf("Get slow: %v", err)
	}
	if gotSlow.Status != DurableJobCancelled || !gotSlow.CancelRequested {
		t.Fatalf("slow status/cancel = %q/%v, want cancelled by RSS drain", gotSlow.Status, gotSlow.CancelRequested)
	}
	if !strings.Contains(gotSlow.CancelReason, string(DurableWorkerRunRSSHandlerAbortSent)) {
		t.Fatalf("slow cancel reason = %q, want %q evidence", gotSlow.CancelReason, DurableWorkerRunRSSHandlerAbortSent)
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Worker.AbortRecovery.LastReason != string(DurableWorkerRunRSSHandlerAbortSent) {
		t.Fatalf("last abort reason = %q, want %q", status.Worker.AbortRecovery.LastReason, DurableWorkerRunRSSHandlerAbortSent)
	}
	if status.Worker.AbortRecovery.LastJobID != "cron:slow" {
		t.Fatalf("last abort job = %q, want cron:slow", status.Worker.AbortRecovery.LastJobID)
	}
	if got := durableWorkerEventCount(t, ledger, string(DurableWorkerRSSDrainStarted)); got != 1 {
		t.Fatalf("rss_drain_started events = %d, want 1", got)
	}
}

func TestDurableWorkerRSSDrain_PeriodicZeroCompletionsTriggersDrain(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: "cron:periodic", Kind: WorkKindCronJob}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	drain := NewDurableWorkerRSSDrain()
	rssChecks := make(chan time.Time, 1)
	started := make(chan struct{})
	cancelObserved := make(chan error, 1)
	worker := DurableWorker{
		Ledger:      ledger,
		WorkerID:    "worker-a",
		Kinds:       []WorkKind{WorkKindCronJob},
		RSSWatchdog: DurableWorkerRSSWatchdogPolicy{MaxRSSMB: 100},
		RSSReader: func() (uint64, error) {
			return 151 * 1024 * 1024, nil
		},
		RSSCheck: rssChecks,
		RSSDrain: drain,
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
	rssChecks <- time.Now().UTC()
	if err := <-cancelObserved; err == nil {
		t.Fatal("handler observed nil ctx.Err, want RSS drain cancellation")
	}
	out := <-resultCh
	if out.err != nil {
		t.Fatalf("RunOne: %v", out.err)
	}
	if out.result.Status != DurableWorkerRunRSSHandlerAbortSent {
		t.Fatalf("result status = %q, want %q", out.result.Status, DurableWorkerRunRSSHandlerAbortSent)
	}
	if got := durableWorkerEventCount(t, ledger, string(DurableWorkerRSSDrainStarted)); got != 1 {
		t.Fatalf("rss_drain_started events = %d, want 1", got)
	}
	if got := durableWorkerEventCount(t, ledger, string(DurableWorkerRunRSSHandlerAbortSent)); got != 0 {
		t.Fatalf("rss_handler_abort_sent typed events = %d, want 0 because handler abort reuses abort-slot event type", got)
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Worker.AbortRecovery.LastReason != string(DurableWorkerRunRSSHandlerAbortSent) {
		t.Fatalf("last abort reason = %q, want %q", status.Worker.AbortRecovery.LastReason, DurableWorkerRunRSSHandlerAbortSent)
	}
}

func TestDurableWorkerRSSDrain_ReadFailureDoesNotCancel(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: "cron:read-failure", Kind: WorkKindCronJob}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	drain := NewDurableWorkerRSSDrain()
	rssChecks := make(chan time.Time, 1)
	started := make(chan struct{})
	readAttempted := make(chan struct{}, 1)
	cancelObserved := make(chan error, 1)
	releaseHandler := make(chan struct{})
	worker := DurableWorker{
		Ledger:      ledger,
		WorkerID:    "worker-a",
		Kinds:       []WorkKind{WorkKindCronJob},
		RSSWatchdog: DurableWorkerRSSWatchdogPolicy{MaxRSSMB: 100},
		RSSReader: func() (uint64, error) {
			select {
			case readAttempted <- struct{}{}:
			default:
			}
			return 0, errors.New("rss unavailable")
		},
		RSSCheck: rssChecks,
		RSSDrain: drain,
		Handler: func(ctx context.Context, job DurableJob, progress DurableWorkerProgressFunc) (json.RawMessage, error) {
			close(started)
			select {
			case <-ctx.Done():
				cancelObserved <- ctx.Err()
				return nil, ctx.Err()
			case <-releaseHandler:
				return json.RawMessage(`{"released":true}`), nil
			}
		},
	}

	resultCh := make(chan durableWorkerRunOutcome, 1)
	go func() {
		result, err := worker.RunOne(ctx)
		resultCh <- durableWorkerRunOutcome{result: result, err: err}
	}()

	<-started
	rssChecks <- time.Now().UTC()
	<-readAttempted
	select {
	case err := <-cancelObserved:
		t.Fatalf("handler was cancelled after RSS read failure: %v", err)
	default:
	}

	close(releaseHandler)
	out := <-resultCh
	if out.err != nil {
		t.Fatalf("RunOne: %v", out.err)
	}
	if out.result.Status != DurableWorkerRunCompleted {
		t.Fatalf("result status = %q, want %q", out.result.Status, DurableWorkerRunCompleted)
	}
	if got := durableWorkerEventCount(t, ledger, string(DurableWorkerRSSWatchdogUnavailable)); got == 0 {
		t.Fatal("rss_watchdog_unavailable events = 0, want degradation evidence")
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Worker.AbortRecovery.AbortSignalSent != 0 {
		t.Fatalf("AbortSignalSent = %d, want 0 after RSS read failure", status.Worker.AbortRecovery.AbortSignalSent)
	}
	got, err := ledger.Get(ctx, "cron:read-failure")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != DurableJobCompleted {
		t.Fatalf("job status = %q, want completed", got.Status)
	}
}

func durableWorkerEventCount(t *testing.T, ledger *DurableLedger, eventType string) int {
	t.Helper()
	var count int
	if err := ledger.db.QueryRow(`
		SELECT COUNT(*)
		FROM durable_worker_events
		WHERE type = ?`, eventType).Scan(&count); err != nil {
		t.Fatalf("count durable worker events %q: %v", eventType, err)
	}
	return count
}
