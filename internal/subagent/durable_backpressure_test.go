package subagent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDurableLedgerRejectsSubmitWhenMaxWaitingExceededAndAudits(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedgerWithOptions(t, DurableLedgerOptions{
		MaxWaiting: 2,
	})
	defer cleanup()

	ctx := context.Background()
	for _, id := range []string{"job-1", "job-2"} {
		if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: id, Kind: WorkKindCronJob}); err != nil {
			t.Fatalf("Submit(%s): %v", id, err)
		}
	}

	if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: "job-3", Kind: WorkKindCronJob}); !errors.Is(err, ErrDurableBackpressure) {
		t.Fatalf("Submit over max waiting err = %v, want ErrDurableBackpressure", err)
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Waiting != 2 {
		t.Fatalf("Waiting = %d, want 2", status.Waiting)
	}
	if status.BackpressureDenied != 1 {
		t.Fatalf("BackpressureDenied = %d, want 1", status.BackpressureDenied)
	}
	if !status.QueueFull || status.MaxWaiting != 2 {
		t.Fatalf("queue full/max waiting = %v/%d, want true/2", status.QueueFull, status.MaxWaiting)
	}
}

func TestDurableLedgerRecordsTimeoutAtAndStaleWaitingHealth(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: "timed", Kind: WorkKindCronJob}); err != nil {
		t.Fatalf("Submit timed: %v", err)
	}
	timeoutAt := time.Now().UTC().Add(-time.Minute)
	claimed, ok, err := ledger.ClaimJob(ctx, "timed", DurableClaim{
		WorkerID:  "worker-a",
		LockUntil: time.Now().UTC().Add(time.Hour),
		TimeoutAt: timeoutAt,
	})
	if err != nil {
		t.Fatalf("ClaimJob timed: %v", err)
	}
	if !ok {
		t.Fatal("ClaimJob ok = false, want true")
	}
	if claimed.TimeoutAt.IsZero() || !claimed.TimeoutAt.Equal(timeoutAt) {
		t.Fatalf("TimeoutAt = %v, want %v", claimed.TimeoutAt, timeoutAt)
	}
	if claimed.CancelRequested {
		t.Fatal("CancelRequested = true, want timeout evidence separate from cancellation intent")
	}

	if _, err := ledger.Submit(ctx, DurableJobSubmission{ID: "stale", Kind: WorkKindCronJob}); err != nil {
		t.Fatalf("Submit stale: %v", err)
	}
	staleCreatedAt := durableNow() - int64(2*time.Hour)
	if _, err := ledger.db.ExecContext(ctx,
		`UPDATE durable_jobs SET created_at = ?, updated_at = ? WHERE id = ?`,
		staleCreatedAt, staleCreatedAt, "stale"); err != nil {
		t.Fatalf("mark stale waiting: %v", err)
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Claimed != 1 || status.Active != 1 {
		t.Fatalf("claimed/active = %d/%d, want 1/1", status.Claimed, status.Active)
	}
	if status.TimeoutScheduled != 1 || status.TimedOut != 1 {
		t.Fatalf("timeout scheduled/timed out = %d/%d, want 1/1", status.TimeoutScheduled, status.TimedOut)
	}
	if status.StaleWaiting != 1 {
		t.Fatalf("StaleWaiting = %d, want 1", status.StaleWaiting)
	}
}
