package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestDurableWorkerRunOne_ClaimsWaitingJobAndCompletesResult(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.WithValue(context.Background(), testDurableWorkerContextKey{}, "handler-context")
	if _, err := ledger.Submit(ctx, DurableJobSubmission{
		ID:       "cron:daily",
		Kind:     WorkKindCronJob,
		Progress: json.RawMessage(`{"phase":"queued"}`),
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	heartbeatAt := time.Now().UTC().Add(-time.Minute)
	var handled bool
	worker := DurableWorker{
		Ledger:   ledger,
		WorkerID: "worker-a",
		Kinds:    []WorkKind{WorkKindCronJob},
		Now:      func() time.Time { return heartbeatAt },
		Handler: func(ctx context.Context, job DurableJob, progress DurableWorkerProgressFunc) (json.RawMessage, error) {
			handled = true
			if got := ctx.Value(testDurableWorkerContextKey{}); got != "handler-context" {
				t.Fatalf("handler context value = %v, want handler-context", got)
			}
			if job.ID != "cron:daily" || job.Status != DurableJobActive || job.LockOwner != "worker-a" {
				t.Fatalf("handler job = %+v, want active cron:daily locked by worker-a", job)
			}
			return json.RawMessage(`{"delivered":true}`), nil
		},
	}

	result, err := worker.RunOne(ctx)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if !handled {
		t.Fatal("handler was not called")
	}
	if result.Status != DurableWorkerRunCompleted {
		t.Fatalf("result status = %q, want %q", result.Status, DurableWorkerRunCompleted)
	}
	if result.JobID != "cron:daily" || result.WorkerID != "worker-a" || result.LockOwner != "worker-a" {
		t.Fatalf("result evidence = %+v, want job/worker/lock owner evidence", result)
	}

	got, err := ledger.Get(ctx, "cron:daily")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != DurableJobCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	assertJSONEqual(t, "result", got.Result, `{"delivered":true}`)
	if got.LockOwner != "" {
		t.Fatalf("lock owner after completion = %q, want cleared terminal lock", got.LockOwner)
	}

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Worker.WorkerID != "worker-a" || !status.Worker.LastHeartbeat.Equal(heartbeatAt) {
		t.Fatalf("worker heartbeat = id %q at %v, want worker-a at %v", status.Worker.WorkerID, status.Worker.LastHeartbeat, heartbeatAt)
	}
}

func TestDurableWorkerRunOne_HandlerProgressPersists(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, DurableJobSubmission{
		ID:       "cron:progress",
		Kind:     WorkKindCronJob,
		Progress: json.RawMessage(`{"phase":"queued"}`),
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	worker := DurableWorker{
		Ledger:   ledger,
		WorkerID: "worker-a",
		Kinds:    []WorkKind{WorkKindCronJob},
		Now:      func() time.Time { return time.Now().UTC().Add(-time.Minute) },
		Handler: func(ctx context.Context, job DurableJob, progress DurableWorkerProgressFunc) (json.RawMessage, error) {
			if err := progress(json.RawMessage(`{"phase":"rendered","pct":50}`)); err != nil {
				t.Fatalf("progress: %v", err)
			}
			return json.RawMessage(`{"done":true}`), nil
		},
	}

	result, err := worker.RunOne(ctx)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if result.Status != DurableWorkerRunCompleted {
		t.Fatalf("result status = %q, want %q", result.Status, DurableWorkerRunCompleted)
	}

	got, err := ledger.Get(ctx, "cron:progress")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != DurableJobCompleted {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	assertJSONEqual(t, "progress", got.Progress, `{"phase":"rendered","pct":50}`)
	assertJSONEqual(t, "result", got.Result, `{"done":true}`)
}

func TestDurableWorkerRunOne_HandlerErrorFailsJob(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, DurableJobSubmission{
		ID:   "cron:fail",
		Kind: WorkKindCronJob,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	worker := DurableWorker{
		Ledger:   ledger,
		WorkerID: "worker-a",
		Kinds:    []WorkKind{WorkKindCronJob},
		Now:      func() time.Time { return time.Now().UTC().Add(-time.Minute) },
		Handler: func(context.Context, DurableJob, DurableWorkerProgressFunc) (json.RawMessage, error) {
			return nil, errors.New("fake handler boom")
		},
	}

	result, err := worker.RunOne(ctx)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if result.Status != DurableWorkerRunHandlerFailed {
		t.Fatalf("result status = %q, want %q", result.Status, DurableWorkerRunHandlerFailed)
	}
	if result.ErrorText != "fake handler boom" {
		t.Fatalf("error text = %q, want fake handler boom", result.ErrorText)
	}

	got, err := ledger.Get(ctx, "cron:fail")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != DurableJobFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.ErrorText != "fake handler boom" {
		t.Fatalf("job error text = %q, want fake handler boom", got.ErrorText)
	}
}

func TestDurableWorkerRunOne_NoJobReturnsIdle(t *testing.T) {
	ledger, _, cleanup := newTestDurableLedger(t)
	defer cleanup()

	ctx := context.Background()
	if _, err := ledger.Submit(ctx, DurableJobSubmission{
		ID:   "cron:terminal",
		Kind: WorkKindCronJob,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	claimed, ok, err := ledger.Claim(ctx, DurableClaim{
		WorkerID:  "setup-worker",
		LockUntil: time.Now().UTC().Add(time.Minute),
		Kinds:     []WorkKind{WorkKindCronJob},
	})
	if err != nil {
		t.Fatalf("Claim setup: %v", err)
	}
	if !ok {
		t.Fatal("Claim setup ok = false, want true")
	}
	if _, ok, err := ledger.Complete(ctx, claimed.ID, "setup-worker", json.RawMessage(`{"terminal":true}`)); err != nil || !ok {
		t.Fatalf("Complete setup ok=%v err=%v, want true nil", ok, err)
	}

	handlerCalled := false
	worker := DurableWorker{
		Ledger:   ledger,
		WorkerID: "worker-a",
		Kinds:    []WorkKind{WorkKindCronJob},
		Now:      func() time.Time { return time.Now().UTC().Add(-time.Minute) },
		Handler: func(context.Context, DurableJob, DurableWorkerProgressFunc) (json.RawMessage, error) {
			handlerCalled = true
			return json.RawMessage(`{"unexpected":true}`), nil
		},
	}

	result, err := worker.RunOne(ctx)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if result.Status != DurableWorkerRunIdle {
		t.Fatalf("result status = %q, want %q", result.Status, DurableWorkerRunIdle)
	}
	if result.JobID != "" || result.LockOwner != "" {
		t.Fatalf("idle result = %+v, want no job or lock owner", result)
	}
	if handlerCalled {
		t.Fatal("handler was called for idle run")
	}

	got, err := ledger.Get(ctx, "cron:terminal")
	if err != nil {
		t.Fatalf("Get terminal: %v", err)
	}
	if got.Status != DurableJobCompleted {
		t.Fatalf("terminal status = %q, want completed", got.Status)
	}
	assertJSONEqual(t, "terminal result", got.Result, `{"terminal":true}`)

	status, err := ledger.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Worker.Liveness != DurableWorkerNoWorker {
		t.Fatalf("worker liveness = %q, want %q", status.Worker.Liveness, DurableWorkerNoWorker)
	}
	if !status.Worker.LastHeartbeat.IsZero() {
		t.Fatalf("last heartbeat = %v, want zero", status.Worker.LastHeartbeat)
	}
}

type testDurableWorkerContextKey struct{}
